package docket

import (
	"context"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Deterministic random-looking data avoids hiding batch-size errors through
// compression. Seeds are explicit so a failing record can be reproduced.
func kafkaRecordPayload(size int, seed uint64) []byte {
	data := make([]byte, size)
	rng := rand.New(rand.NewPCG(seed, 1))
	for at := 0; at < len(data); at += 8 {
		var next [8]byte
		binary.LittleEndian.PutUint64(next[:], rng.Uint64())
		copy(data[at:], next[:])
	}
	return data
}

func kafkaWireClient(t *testing.T, broker *kafkaWireBroker, large bool, options ...kgo.Opt) *kgo.Client {
	t.Helper()
	options = append(options, kgo.Dialer(broker.dial))
	create := newKafkaProducerClient
	if large {
		create = newKafkaLargeProducerClient
	}
	client, err := create([]string{"pipe:9092"}, options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestKafkaOrdinaryProducerBatchesSmallRecords(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	broker := newKafkaWireBroker(t, map[string]int{"small": 1_048_588})
	client := kafkaWireClient(t, broker, false)
	value := kafkaRecordPayload(600_000, 0)
	if err := client.ProduceSync(ctx, &kgo.Record{Topic: "small", Value: value}, &kgo.Record{Topic: "small", Value: value}).FirstErr(); err != nil {
		t.Fatal(err)
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.committed != 2 || len(broker.batches) != 2 {
		t.Fatalf("committed=%d, batches=%+v; want two accepted small batches", broker.committed, broker.batches)
	}
}

func TestKafkaConcurrentAppendRecordBounds(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	broker := newKafkaWireBroker(t, map[string]int{"small": 1_048_588, "large": 1_200_000})
	log := &KafkaLog{
		producer:          kafkaWireClient(t, broker, false),
		largeProducer:     kafkaWireClient(t, broker, true),
		largeProducerSlot: make(chan struct{}, 1),
	}
	t.Cleanup(log.Close)
	const writers = 8
	results := make(chan error, 2*writers)
	var workers sync.WaitGroup
	for i := range writers {
		for _, record := range []struct {
			topic string
			size  int
		}{{"small", 600_000}, {"large", 1_100_000}} {
			value := kafkaRecordPayload(record.size, uint64(i))
			workers.Go(func() {
				_, err := log.Append(ctx, record.topic, nil, value)
				results <- err
			})
		}
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.committed != 2*writers {
		t.Fatalf("committed=%d; want %d", broker.committed, 2*writers)
	}
	for _, batch := range broker.batches {
		if batch.topic == "large" && batch.records != 1 {
			t.Fatalf("large plain records coalesced: %+v", batch)
		}
	}
}

func TestKafkaAppendMaximumRecord(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	broker := newKafkaWireBroker(t, map[string]int{"maximum": maxKafkaRecordBatchBytes})
	log := &KafkaLog{
		producer:          kafkaWireClient(t, broker, false),
		largeProducer:     kafkaWireClient(t, broker, true),
		largeProducerSlot: make(chan struct{}, 1),
	}
	t.Cleanup(log.Close)
	if _, err := log.Append(ctx, "maximum", kafkaRecordPayload(MaxRecordBytes, 2), kafkaRecordPayload(MaxRecordBytes, 3)); err != nil {
		t.Fatal(err)
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.committed != 1 || len(broker.batches) != 1 || broker.batches[0].records != 1 {
		t.Fatalf("committed=%d, batches=%+v; want a maximal individual record", broker.committed, broker.batches)
	}
}

func kafkaWireCourt(t *testing.T, broker *kafkaWireBroker, c Case) *KafkaLog {
	t.Helper()
	client := kafkaWireClient(t, broker, true, kgo.TransactionalID("the-court."+c.ID))
	log := &KafkaLog{
		adm:          kadm.NewClient(client),
		transactions: map[string]*transactionProducer{c.ID: {client: client}},
		diagnose:     func(err error) { t.Errorf("unexpected mirror failure: %v", err) },
	}
	t.Cleanup(log.Close)
	return log
}

func TestKafkaCommitBatchesSmallAndLargeRecords(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	c := Case{ID: "case-000000000000000000000001"}
	broker := newKafkaWireBroker(t, map[string]int{"small": 1_048_588, "large": 2_200_000, c.AttentionTopic(): 1_048_588})
	log := kafkaWireCourt(t, broker, c)
	small := StepAppend{Topic: "small", Value: kafkaRecordPayload(600_000, 4)}
	large := StepAppend{Topic: "large", Value: kafkaRecordPayload(2_000_000, 5)}
	if err := log.Commit(ctx, c, Step{Appends: []StepAppend{small, small, large, small}}); err != nil {
		t.Fatal(err)
	}
	// Reusing the same transactional producer after a large step must retain
	// ordinary-topic compatibility as well.
	if err := log.Commit(ctx, c, Step{Appends: []StepAppend{small, small}}); err != nil {
		t.Fatal(err)
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.committed != 8 || len(broker.transactions) != 2 {
		t.Fatalf("committed=%d, transactions=%+v", broker.committed, broker.transactions)
	}
	for i, count := range []int{5, 3} {
		if got := broker.transactions[i]; !got.commit || got.records != count {
			t.Fatalf("transaction %d = %+v; want one commit containing %d records", i, got, count)
		}
	}
}

func TestKafkaCommitAbortsEarlierChunks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	c := Case{ID: "case-000000000000000000000002"}
	broker := newKafkaWireBroker(t, map[string]int{"small": 1_048_588, "too-small": 1_048_588, c.AttentionTopic(): 1_048_588})
	log := kafkaWireCourt(t, broker, c)
	err := log.Commit(ctx, c, Step{Appends: []StepAppend{
		{Topic: "small", Value: kafkaRecordPayload(600_000, 6)},
		{Topic: "too-small", Value: kafkaRecordPayload(2_000_000, 7)},
	}})
	if !errors.Is(err, kerr.MessageTooLarge) {
		t.Fatalf("Commit error=%v; want broker size rejection", err)
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.committed != 0 || len(broker.transactions) != 1 || broker.transactions[0].commit || broker.transactions[0].records != 1 {
		t.Fatalf("committed=%d, transactions=%+v; want abort of the earlier accepted chunk", broker.committed, broker.transactions)
	}
}
