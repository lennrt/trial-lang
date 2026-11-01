package docket

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type kafkaWireBatch struct {
	topic   string
	bytes   int
	records int
}

type kafkaWireTransaction struct {
	commit  bool
	records int
}

// kafkaWireBroker inspects actual encoded batches and implements the small
// producer/transaction protocol subset used by these tests. It enforces a
// per-topic batch limit and records transaction completion. It does not replace
// the live-broker roundtrip and atomic-visibility tests.
type kafkaWireBroker struct {
	mu           sync.Mutex
	connections  []net.Conn
	workers      sync.WaitGroup
	limits       map[string]int
	next         map[string]int64
	pending      map[string]int
	batches      []kafkaWireBatch
	transactions []kafkaWireTransaction
	committed    int
	errors       []error
}

func newKafkaWireBroker(t *testing.T, limits map[string]int) *kafkaWireBroker {
	t.Helper()
	b := &kafkaWireBroker{limits: limits, next: map[string]int64{}, pending: map[string]int{}}
	t.Cleanup(func() {
		b.mu.Lock()
		connections := append([]net.Conn(nil), b.connections...)
		b.mu.Unlock()
		for _, conn := range connections {
			_ = conn.Close()
		}
		b.workers.Wait()
		for _, err := range b.errors {
			t.Errorf("Kafka wire fixture: %v", err)
		}
	})
	return b
}

func (b *kafkaWireBroker) dial(_ context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	b.mu.Lock()
	b.connections = append(b.connections, server)
	b.workers.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.workers.Done()
		defer func() { _ = server.Close() }()
		if err := b.serve(server); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, net.ErrClosed) {
			b.mu.Lock()
			b.errors = append(b.errors, err)
			b.mu.Unlock()
		}
	}()
	return client, nil
}

func (b *kafkaWireBroker) serve(conn net.Conn) error {
	for {
		var size [4]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(size[:])
		if length < 10 || length > 2*MaxRecordBytes+1<<20 {
			return fmt.Errorf("invalid request length %d", length)
		}
		raw := make([]byte, length)
		if _, err := io.ReadFull(conn, raw); err != nil {
			return err
		}
		key := int16(binary.BigEndian.Uint16(raw))
		req := kmsg.RequestForKey(key)
		if req == nil {
			return fmt.Errorf("unknown request key %d", key)
		}
		req.SetVersion(int16(binary.BigEndian.Uint16(raw[2:])))
		body := 10 + int(int16(binary.BigEndian.Uint16(raw[8:])))
		if req.IsFlexible() {
			if body >= len(raw) || raw[body] != 0 {
				return fmt.Errorf("unexpected nonempty request header tags")
			}
			body++
		}
		if body < 10 || body > len(raw) {
			return fmt.Errorf("invalid request header length %d", body)
		}
		if err := req.ReadFrom(raw[body:]); err != nil {
			return fmt.Errorf("decode %T: %w", req, err)
		}
		resp, err := b.respond(req)
		if err != nil {
			return err
		}
		data := append([]byte(nil), raw[4:8]...) // correlation ID
		if resp.IsFlexible() && key != 18 {
			data = append(data, 0) // empty response header tags
		}
		data = resp.AppendTo(data)
		binary.BigEndian.PutUint32(size[:], uint32(len(data)))
		if _, err := conn.Write(append(size[:], data...)); err != nil {
			return err
		}
	}
}

func (b *kafkaWireBroker) respond(req kmsg.Request) (kmsg.Response, error) {
	resp := req.ResponseKind()
	switch r := req.(type) {
	case *kmsg.ApiVersionsRequest:
		out := resp.(*kmsg.ApiVersionsResponse)
		for _, version := range [][2]int16{{0, 8}, {3, 1}, {8, 2}, {10, 0}, {18, 3}, {22, 0}, {24, 0}, {26, 0}} {
			out.ApiKeys = append(out.ApiKeys, kmsg.ApiVersionsResponseApiKey{ApiKey: version[0], MaxVersion: version[1]})
		}
	case *kmsg.MetadataRequest:
		out := resp.(*kmsg.MetadataResponse)
		out.Brokers = []kmsg.MetadataResponseBroker{{NodeID: 0, Host: "pipe", Port: 9092}}
		for _, topic := range r.Topics {
			out.Topics = append(out.Topics, kmsg.MetadataResponseTopic{Topic: topic.Topic, Partitions: []kmsg.MetadataResponseTopicPartition{{Partition: 0, Leader: 0, Replicas: []int32{0}, ISR: []int32{0}}}})
		}
	case *kmsg.FindCoordinatorRequest:
		out := resp.(*kmsg.FindCoordinatorResponse)
		out.NodeID, out.Host, out.Port = 0, "pipe", 9092
	case *kmsg.InitProducerIDRequest:
		resp.(*kmsg.InitProducerIDResponse).ProducerID = 1
	case *kmsg.AddPartitionsToTxnRequest:
		out := resp.(*kmsg.AddPartitionsToTxnResponse)
		for _, topic := range r.Topics {
			rt := kmsg.AddPartitionsToTxnResponseTopic{Topic: topic.Topic}
			for _, partition := range topic.Partitions {
				rt.Partitions = append(rt.Partitions, kmsg.AddPartitionsToTxnResponseTopicPartition{Partition: partition})
			}
			out.Topics = append(out.Topics, rt)
		}
	case *kmsg.ProduceRequest:
		return b.produce(r)
	case *kmsg.EndTxnRequest:
		b.mu.Lock()
		count := b.pending[r.TransactionalID]
		delete(b.pending, r.TransactionalID)
		b.transactions = append(b.transactions, kafkaWireTransaction{commit: r.Commit, records: count})
		if r.Commit {
			b.committed += count
		}
		b.mu.Unlock()
	case *kmsg.OffsetCommitRequest:
		out := resp.(*kmsg.OffsetCommitResponse)
		for _, topic := range r.Topics {
			rt := kmsg.OffsetCommitResponseTopic{Topic: topic.Topic}
			for _, partition := range topic.Partitions {
				rt.Partitions = append(rt.Partitions, kmsg.OffsetCommitResponseTopicPartition{Partition: partition.Partition})
			}
			out.Topics = append(out.Topics, rt)
		}
	default:
		return nil, fmt.Errorf("unexpected request %T", req)
	}
	return resp, nil
}

func (b *kafkaWireBroker) produce(req *kmsg.ProduceRequest) (kmsg.Response, error) {
	out := req.ResponseKind().(*kmsg.ProduceResponse)
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, topic := range req.Topics {
		rt := kmsg.ProduceResponseTopic{Topic: topic.Topic}
		for _, partition := range topic.Partitions {
			rp := kmsg.ProduceResponseTopicPartition{Partition: partition.Partition, BaseOffset: b.next[topic.Topic]}
			count := 0
			for raw := partition.Records; len(raw) > 0; {
				if len(raw) < 12 {
					return nil, fmt.Errorf("truncated record batch")
				}
				size := 12 + int(binary.BigEndian.Uint32(raw[8:]))
				if size > len(raw) || size < 61 {
					return nil, fmt.Errorf("invalid record batch length %d", size)
				}
				var batch kmsg.RecordBatch
				if err := batch.ReadFrom(raw[:size]); err != nil {
					return nil, err
				}
				b.batches = append(b.batches, kafkaWireBatch{topic: topic.Topic, bytes: size, records: int(batch.NumRecords)})
				limit, ok := b.limits[topic.Topic]
				if !ok {
					return nil, fmt.Errorf("no batch limit for topic %q", topic.Topic)
				}
				if size > limit {
					rp.ErrorCode = kerr.MessageTooLarge.Code
				}
				count += int(batch.NumRecords)
				raw = raw[size:]
			}
			if rp.ErrorCode == 0 {
				b.next[topic.Topic] += int64(count)
				if req.TransactionID == nil {
					b.committed += count
				} else {
					b.pending[*req.TransactionID] += count
				}
			}
			rt.Partitions = append(rt.Partitions, rp)
		}
		out.Topics = append(out.Topics, rt)
	}
	return out, nil
}
