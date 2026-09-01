package docket

import (
	"container/list"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	// The logical-to-physical cache is shared by the whole KafkaLog. It retains
	// offsets only, never instruction bodies, and has a fixed aggregate bound.
	maxProceedingCacheEntries = 65_536
	// A miss caches a modest forward window so sequential execution does not
	// rescan the topic for every instruction or evict every other active case.
	maxProceedingCacheWindow = 1_024
	// Idle Kafka clients are heavier than offset mappings and may retain fetch
	// buffers. Active operations are pinned and can temporarily exceed these
	// limits; once they finish, least-recently-used clients are closed.
	maxCachedConsumers    = 16
	maxCachedTransactions = 16
	// One valid record can contain a MaxRecordBytes key and value. Poll-gated
	// fetches plus this per-client broker-response bound keep idle cursors from
	// prefetching indefinitely while still accepting every valid record.
	maxKafkaFetchBytes = 2*MaxRecordBytes + 1<<20
)

var errKafkaLogClosed = errors.New("kafka log is closed")

// KafkaLog stores execution state in single-partition Kafka topics.
type KafkaLog struct {
	producer *kgo.Client // plain paperwork: filings, verdicts, summonses, markers
	adm      *kadm.Client

	brokers []string

	mu             sync.Mutex
	consumers      map[string]*cursor // one physical fetch cursor per topic
	consumerLRU    *list.List
	proceedings    map[proceedingAddress]*list.Element
	proceedingsLRU *list.List
	transactions   map[string]*transactionProducer
	transactionLRU *list.List
	diagnose       func(error)
	closed         bool
	closeOnce      sync.Once
}

// cursor is a direct (group-less) consumer pinned to partition 0 of
// one topic, plus the offset it will look for next. Jumps move it with
// SetOffsets. Transactional commit markers leave silent gaps in the
// offsets, so the cursor yields the first record at or after the
// requested position.
type cursor struct {
	mu     sync.Mutex
	topic  string
	client *kgo.Client
	next   int64
	buf    []*kgo.Record

	// refs and lru are protected by KafkaLog.mu. A positive ref count pins the
	// client so LRU trimming cannot close an active fetch or wait.
	refs int
	lru  *list.Element
}

type transactionProducer struct {
	mu     sync.Mutex // serializes complete same-case transactions and mirrors
	caseID string
	client *kgo.Client

	// refs and lru are protected by KafkaLog.mu.
	refs int
	lru  *list.Element
}

type proceedingAddress struct {
	topic string
	pc    int64
}

type proceedingCacheEntry struct {
	address  proceedingAddress
	physical int64
}

type kafkaConfig struct {
	diagnose func(error)
}

// KafkaOption configures OpenKafkaLog.
type KafkaOption func(*kafkaConfig)

// WithDiagnostic reports nonauthoritative maintenance failures, such as a
// consumer-offset mirror failing after the authoritative transaction commits.
func WithDiagnostic(report func(error)) KafkaOption {
	return func(config *kafkaConfig) {
		config.diagnose = report
	}
}

// OpenKafkaLog opens and verifies a Kafka-backed Log. It owns all clients it
// creates. Close releases them.
func OpenKafkaLog(ctx context.Context, brokers string, options ...KafkaOption) (*KafkaLog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parts := strings.Split(brokers, ",")
	seeds := make([]string, 0, len(parts))
	for _, part := range parts {
		seed := strings.TrimSpace(part)
		if seed == "" {
			return nil, errors.New("broker list contains an empty address")
		}
		seeds = append(seeds, seed)
	}
	config := kafkaConfig{diagnose: func(error) {}}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("nil Kafka option")
		}
		option(&config)
	}
	if config.diagnose == nil {
		config.diagnose = func(error) {}
	}
	producer, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := producer.Ping(pingCtx); err != nil {
		producer.Close()
		return nil, fmt.Errorf("connect to Kafka: %w", err)
	}
	return &KafkaLog{
		producer:       producer,
		adm:            kadm.NewClient(producer),
		brokers:        seeds,
		consumers:      make(map[string]*cursor),
		consumerLRU:    list.New(),
		proceedings:    make(map[proceedingAddress]*list.Element),
		proceedingsLRU: list.New(),
		transactions:   make(map[string]*transactionProducer),
		transactionLRU: list.New(),
		diagnose:       config.diagnose,
	}, nil
}

func (k *KafkaLog) Append(ctx context.Context, topic string, key, value []byte) (int64, error) {
	if err := validateRecord(key, value); err != nil {
		return 0, err
	}
	rec := &kgo.Record{Topic: topic, Key: cloneBytes(key), Value: cloneBytes(value), Partition: 0}
	res := k.producer.ProduceSync(ctx, rec)
	if err := res.FirstErr(); err != nil {
		return 0, fmt.Errorf("append record: %w", err)
	}
	return rec.Offset, nil
}

func (k *KafkaLog) AppendBatch(ctx context.Context, appends []StepAppend) ([]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateBatch(appends); err != nil {
		return nil, err
	}
	if len(appends) == 0 {
		return []int64{}, nil
	}
	records := make([]*kgo.Record, len(appends))
	for i, appendRecord := range appends {
		records[i] = &kgo.Record{
			Topic:     appendRecord.Topic,
			Partition: 0,
			Key:       cloneBytes(appendRecord.Key),
			Value:     cloneBytes(appendRecord.Value),
		}
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("create batch transaction ID: %w", err)
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(k.brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.TransactionalID("the-court.append."+hex.EncodeToString(nonce[:])),
		kgo.TransactionTimeout(60*time.Second),
	)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if err := client.BeginTransaction(); err != nil {
		return nil, fmt.Errorf("open append transaction: %w", err)
	}
	if err := client.ProduceSync(ctx, records...).FirstErr(); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		abortBufferedErr := client.AbortBufferedRecords(cleanupCtx)
		abortTransactionErr := client.EndTransaction(cleanupCtx, kgo.TryAbort)
		return nil, errors.Join(fmt.Errorf("produce append batch: %w", err), abortBufferedErr, abortTransactionErr)
	}
	if err := client.EndTransaction(ctx, kgo.TryCommit); err != nil {
		return nil, &AmbiguousCommitError{Err: err}
	}
	offsets := make([]int64, len(records))
	for i, record := range records {
		offsets[i] = record.Offset
	}
	return offsets, nil
}

// endOffset reports the next offset the topic would assign. New topics may
// take time to reach every broker view, so missing topics and partitions are
// retried for ten seconds. Three seconds was insufficient on loaded CI.
func (k *KafkaLog) endOffset(ctx context.Context, topic string) (int64, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		var reason error
		listed, err := k.adm.ListEndOffsets(ctx, topic)
		if err != nil {
			reason = err
		} else if lo, ok := listed.Lookup(topic, 0); !ok {
			reason = fmt.Errorf("%w: %q has no partition 0", ErrTopicNotFound, topic)
		} else if lo.Err != nil {
			reason = lo.Err
		} else {
			return lo.Offset, nil
		}
		if time.Now().After(deadline) {
			if errors.Is(reason, kerr.UnknownTopicOrPartition) {
				return 0, fmt.Errorf("%w: %q", ErrTopicNotFound, topic)
			}
			return 0, reason
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}

// ReadAll reads a committed snapshot of the topic. Transactional
// commit markers may occupy the tail offsets invisibly, so "have we
// reached the end" cannot be answered by offsets alone; after the fast
// path, an empty short poll against an unchanged high watermark is the end.
func (k *KafkaLog) ReadAll(ctx context.Context, topic string) ([]Record, error) {
	end, err := k.endOffset(ctx, topic)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return nil, nil
	}
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(k.brokers...),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
		kgo.FetchMaxWait(250*time.Millisecond),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{
			topic: {0: kgo.NewOffset().At(0)},
		}),
	)
	if err != nil {
		return nil, err
	}
	defer cl.Close()

	var out []Record
	bytes := 0
	for {
		pollCtx, cancel := context.WithTimeout(ctx, time.Second)
		fetches := cl.PollFetches(pollCtx)
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := firstFetchErr(fetches); err != nil {
			return nil, err
		}
		recs := fetches.Records()
		for _, r := range recs {
			if err := validateRecord(r.Key, r.Value); err != nil {
				return nil, fmt.Errorf("topic %q offset %d: %w", topic, r.Offset, err)
			}
			bytes += len(r.Key) + len(r.Value)
			if err := checkSnapshotSize(len(out)+1, bytes); err != nil {
				return nil, fmt.Errorf("topic %q: %w", topic, err)
			}
			out = append(out, Record{Offset: r.Offset, Key: cloneBytes(r.Key), Value: cloneBytes(r.Value)})
		}
		if len(out) > 0 && out[len(out)-1].Offset >= end-1 {
			return out, nil // the tail is data; nothing is hidden
		}
		if len(recs) == 0 {
			// Nothing arrived within the window. If the watermark has
			// not moved, what remains below it is commit markers.
			nowEnd, err := k.endOffset(ctx, topic)
			if err != nil {
				return nil, err
			}
			if nowEnd == end {
				return out, nil
			}
			end = nowEnd
		}
	}
}

func (k *KafkaLog) End(ctx context.Context, topic string) (int64, error) {
	return k.endOffset(ctx, topic)
}

func (k *KafkaLog) newCursorClient(topic string, offset int64) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(k.brokers...),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
		kgo.FetchMaxWait(100*time.Millisecond),
		kgo.FetchMaxBytes(maxKafkaFetchBytes),
		kgo.FetchMaxPartitionBytes(maxKafkaFetchBytes),
		// A retained cursor should fetch only while its caller is polling. This
		// prevents idle clients from accumulating broker response buffers.
		kgo.MaxConcurrentFetches(0),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{
			topic: {0: kgo.NewOffset().At(offset)},
		}),
	)
}

// acquireCursor pins one topic cursor until releaseCursor. Client creation is
// done outside the registry lock; the second lookup prevents duplicate
// retained clients when callers race to open the same topic.
func (k *KafkaLog) acquireCursor(topic string, offset int64) (*cursor, error) {
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return nil, errKafkaLogClosed
	}
	if cur, ok := k.consumers[topic]; ok {
		cur.refs++
		if cur.lru != nil {
			k.consumerLRU.MoveToFront(cur.lru)
		}
		k.mu.Unlock()
		return cur, nil
	}
	k.mu.Unlock()

	client, err := k.newCursorClient(topic, offset)
	if err != nil {
		return nil, err
	}

	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		client.Close()
		return nil, errKafkaLogClosed
	}
	if cur, ok := k.consumers[topic]; ok {
		cur.refs++
		if cur.lru != nil {
			k.consumerLRU.MoveToFront(cur.lru)
		}
		k.mu.Unlock()
		client.Close()
		return cur, nil
	}
	if k.consumers == nil {
		k.consumers = make(map[string]*cursor)
	}
	if k.consumerLRU == nil {
		k.consumerLRU = list.New()
	}
	cur := &cursor{topic: topic, client: client, next: offset, refs: 1}
	cur.lru = k.consumerLRU.PushFront(cur)
	k.consumers[topic] = cur
	evicted := k.trimConsumersLocked()
	k.mu.Unlock()
	closeKafkaClients(evicted)
	return cur, nil
}

func (k *KafkaLog) releaseCursor(cur *cursor) {
	k.mu.Lock()
	if cur.refs > 0 {
		cur.refs--
	}
	evicted := k.trimConsumersLocked()
	k.mu.Unlock()
	closeKafkaClients(evicted)
}

// trimConsumersLocked evicts only idle clients. When every excess client is
// pinned by an active fetch or wait, the registry may temporarily exceed its
// limit and is trimmed by the next release.
func (k *KafkaLog) trimConsumersLocked() []*kgo.Client {
	var clients []*kgo.Client
	for len(k.consumers) > maxCachedConsumers {
		var victim *list.Element
		for element := k.consumerLRU.Back(); element != nil; element = element.Prev() {
			if element.Value.(*cursor).refs == 0 {
				victim = element
				break
			}
		}
		if victim == nil {
			break
		}
		cur := victim.Value.(*cursor)
		delete(k.consumers, cur.topic)
		k.consumerLRU.Remove(victim)
		cur.lru = nil
		clients = append(clients, cur.client)
	}
	return clients
}

func closeKafkaClients(clients []*kgo.Client) {
	for _, client := range clients {
		if client != nil {
			client.Close()
		}
	}
}

func (k *KafkaLog) Fetch(ctx context.Context, topic string, offset int64, wait bool) (*Record, error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset must be nonnegative: %d", offset)
	}
	if !wait {
		end, err := k.endOffset(ctx, topic)
		if err != nil {
			return nil, err
		}
		if offset >= end {
			return nil, nil
		}
	}

	cur, err := k.acquireCursor(topic, offset)
	if err != nil {
		return nil, err
	}
	defer k.releaseCursor(cur)
	cur.mu.Lock()
	defer cur.mu.Unlock()

	if cur.next != offset {
		// SetOffsets purges buffered records when the requested offset moves.
		cur.client.SetOffsets(map[string]map[int32]kgo.EpochOffset{
			topic: {0: {Offset: offset, Epoch: -1}},
		})
		cur.next = offset
		cur.buf = nil
	}

	for {
		for len(cur.buf) > 0 {
			r := cur.buf[0]
			cur.buf = cur.buf[1:]
			if r.Offset >= offset {
				cur.next = r.Offset + 1
				if err := validateRecord(r.Key, r.Value); err != nil {
					return nil, fmt.Errorf("topic %q offset %d: %w", topic, r.Offset, err)
				}
				return &Record{Offset: r.Offset, Key: cloneBytes(r.Key), Value: cloneBytes(r.Value)}, nil
			}
		}
		if !wait {
			// The record at this offset is a commit marker or has been
			// compacted away; whatever exists next has not arrived in
			// the buffer yet, and the caller declined to wait.
			end, err := k.endOffset(ctx, topic)
			if err != nil {
				return nil, err
			}
			if offset >= end {
				return nil, nil
			}
		}
		pollCtx := ctx
		cancel := func() {}
		if !wait {
			pollCtx, cancel = context.WithTimeout(ctx, 250*time.Millisecond)
		}
		fetches := cur.client.PollFetches(pollCtx)
		pollErr := pollCtx.Err()
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !wait && errors.Is(pollErr, context.DeadlineExceeded) {
			return nil, nil
		}
		if err := firstFetchErr(fetches); err != nil {
			return nil, err
		}
		cur.buf = append(cur.buf, fetches.Records()...)
		if !wait && len(cur.buf) == 0 {
			return nil, nil
		}
	}
}

func (k *KafkaLog) cachedProceeding(topic string, pc int64) (int64, bool) {
	address := proceedingAddress{topic: topic, pc: pc}
	k.mu.Lock()
	defer k.mu.Unlock()
	element, ok := k.proceedings[address]
	if !ok {
		return 0, false
	}
	k.proceedingsLRU.MoveToFront(element)
	return element.Value.(proceedingCacheEntry).physical, true
}

func (k *KafkaLog) cacheProceedingLocked(address proceedingAddress, physical int64) error {
	if element, ok := k.proceedings[address]; ok {
		cached := element.Value.(proceedingCacheEntry).physical
		if cached != physical {
			return fmt.Errorf("topic %q logical proceedings address %d changed physical offset from %d to %d", address.topic, address.pc, cached, physical)
		}
		k.proceedingsLRU.MoveToFront(element)
		return nil
	}
	entry := proceedingCacheEntry{address: address, physical: physical}
	element := k.proceedingsLRU.PushFront(entry)
	k.proceedings[address] = element
	for len(k.proceedings) > maxProceedingCacheEntries {
		oldest := k.proceedingsLRU.Back()
		oldEntry := oldest.Value.(proceedingCacheEntry)
		delete(k.proceedings, oldEntry.address)
		k.proceedingsLRU.Remove(oldest)
	}
	return nil
}

func (k *KafkaLog) cacheProceeding(topic string, pc, physical int64) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.proceedings == nil {
		k.proceedings = make(map[proceedingAddress]*list.Element)
	}
	if k.proceedingsLRU == nil {
		k.proceedingsLRU = list.New()
	}
	return k.cacheProceedingLocked(proceedingAddress{topic: topic, pc: pc}, physical)
}

func (k *KafkaLog) cacheProceedingWindow(topic string, records []Record, start int) error {
	end := min(len(records), start+maxProceedingCacheWindow)
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.proceedings == nil {
		k.proceedings = make(map[proceedingAddress]*list.Element)
	}
	if k.proceedingsLRU == nil {
		k.proceedingsLRU = list.New()
	}
	for i := start; i < end; i++ {
		if err := k.cacheProceedingLocked(proceedingAddress{topic: topic, pc: int64(i)}, records[i].Offset); err != nil {
			return err
		}
	}
	return nil
}

func (k *KafkaLog) dropProceedingCache(topic string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for address, element := range k.proceedings {
		if address.topic == topic {
			delete(k.proceedings, address)
			k.proceedingsLRU.Remove(element)
		}
	}
}

func proceedingSnapshotGrowth(records, bytes int, record *Record) (int, error) {
	nextBytes := bytes + len(record.Key) + len(record.Value)
	if err := checkSnapshotSize(records+1, nextBytes); err != nil {
		return bytes, err
	}
	return nextBytes, nil
}

// fetchProceeding returns both the logical record and its physical Kafka
// offset. The latter is used only for the nonauthoritative consumer-group
// mirror.
func (k *KafkaLog) fetchProceeding(ctx context.Context, c Case, pc int64, wait bool) (*Record, int64, error) {
	if pc < 0 {
		return nil, 0, fmt.Errorf("program counter must be nonnegative: %d", pc)
	}
	topic := c.Proceedings()
	cachedPhysical, cached := k.cachedProceeding(topic, pc)
	if cached {
		record, err := k.Fetch(ctx, topic, cachedPhysical, false)
		if err != nil {
			return nil, 0, err
		}
		if record != nil {
			if record.Offset != cachedPhysical {
				return nil, 0, fmt.Errorf("proceedings logical address %d expected physical offset %d, found %d", pc, cachedPhysical, record.Offset)
			}
			record.Offset = pc
			return record, cachedPhysical, nil
		}
	}

	records, err := k.ReadAll(ctx, topic)
	if err != nil {
		return nil, 0, err
	}
	if pc < int64(len(records)) {
		physical := records[pc].Offset
		if cached && physical != cachedPhysical {
			return nil, 0, fmt.Errorf("proceedings logical address %d expected physical offset %d, found %d", pc, cachedPhysical, physical)
		}
		if err := k.cacheProceedingWindow(topic, records, int(pc)); err != nil {
			return nil, 0, err
		}
		record := records[pc]
		record.Offset = pc
		return &record, physical, nil
	}
	if cached {
		return nil, 0, fmt.Errorf("proceedings logical address %d at physical offset %d is no longer visible", pc, cachedPhysical)
	}
	if !wait {
		return nil, 0, nil
	}

	visible, bytes := len(records), 0
	physical := int64(0)
	for i := range records {
		bytes += len(records[i].Key) + len(records[i].Value)
	}
	if visible > 0 {
		last := records[visible-1].Offset
		if last == 1<<63-1 {
			return nil, 0, fmt.Errorf("proceedings offset %d has no following cursor", last)
		}
		physical = last + 1
	}
	for int64(visible) <= pc {
		record, err := k.Fetch(ctx, topic, physical, true)
		if err != nil || record == nil {
			return nil, 0, err
		}
		nextBytes, err := proceedingSnapshotGrowth(visible, bytes, record)
		if err != nil {
			return nil, 0, fmt.Errorf("topic %q: %w", topic, err)
		}
		bytes = nextBytes
		logical := int64(visible)
		if err := k.cacheProceeding(topic, logical, record.Offset); err != nil {
			return nil, 0, err
		}
		if logical == pc {
			physical := record.Offset
			record.Offset = pc
			return record, physical, nil
		}
		if record.Offset == 1<<63-1 {
			return nil, 0, fmt.Errorf("proceedings offset %d has no following cursor", record.Offset)
		}
		physical = record.Offset + 1
		visible++
	}
	panic("unreachable")
}

// FetchProceeding resolves a logical instruction address through visible
// committed records without exposing Kafka control-record gaps.
func (k *KafkaLog) FetchProceeding(ctx context.Context, c Case, pc int64, wait bool) (*Record, error) {
	record, _, err := k.fetchProceeding(ctx, c, pc, wait)
	return record, err
}

// proceedingsPhysicalCursor converts a logical program counter to the raw
// Kafka cursor used only for the nonauthoritative consumer-group mirror.
func (k *KafkaLog) proceedingsPhysicalCursor(ctx context.Context, c Case, pc int64) (int64, error) {
	if pc < 0 {
		return 0, fmt.Errorf("program counter must be nonnegative: %d", pc)
	}
	if pc == 0 {
		return 0, nil
	}
	record, physical, err := k.fetchProceeding(ctx, c, pc-1, false)
	if err != nil {
		return 0, err
	}
	if record == nil {
		return 0, fmt.Errorf("program counter %d is past the visible proceedings", pc)
	}
	if physical == 1<<63-1 {
		return 0, fmt.Errorf("proceedings offset %d has no following cursor", physical)
	}
	return physical + 1, nil
}

// attentionNote records the authoritative execution cursors in the same
// transaction as an instruction's effects.
type attentionNote struct {
	PC      int64   `json:"pc"`
	Summons int64   `json:"summons"`
	Ledger  int64   `json:"ledger,omitempty"`
	Gazette int64   `json:"gazette,omitempty"`
	Heard   []int64 `json:"heard,omitempty"`
}

func (k *KafkaLog) newTransactionClient(c Case) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(k.brokers...),
		kgo.TransactionalID("the-court."+c.ID),
		kgo.TransactionTimeout(60*time.Second),
	)
}

// acquireTransaction pins one per-case producer until releaseTransaction. Its
// own mutex serializes the complete authoritative transaction and the
// nonauthoritative offset mirrors so concurrent commits for one case cannot
// reorder either view.
func (k *KafkaLog) acquireTransaction(c Case) (*transactionProducer, error) {
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return nil, errKafkaLogClosed
	}
	if transaction, ok := k.transactions[c.ID]; ok {
		transaction.refs++
		if transaction.lru != nil {
			k.transactionLRU.MoveToFront(transaction.lru)
		}
		k.mu.Unlock()
		return transaction, nil
	}
	k.mu.Unlock()

	client, err := k.newTransactionClient(c)
	if err != nil {
		return nil, err
	}

	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		client.Close()
		return nil, errKafkaLogClosed
	}
	if transaction, ok := k.transactions[c.ID]; ok {
		transaction.refs++
		if transaction.lru != nil {
			k.transactionLRU.MoveToFront(transaction.lru)
		}
		k.mu.Unlock()
		client.Close()
		return transaction, nil
	}
	if k.transactions == nil {
		k.transactions = make(map[string]*transactionProducer)
	}
	if k.transactionLRU == nil {
		k.transactionLRU = list.New()
	}
	transaction := &transactionProducer{caseID: c.ID, client: client, refs: 1}
	transaction.lru = k.transactionLRU.PushFront(transaction)
	k.transactions[c.ID] = transaction
	evicted := k.trimTransactionsLocked()
	k.mu.Unlock()
	closeKafkaClients(evicted)
	return transaction, nil
}

func (k *KafkaLog) releaseTransaction(transaction *transactionProducer) {
	k.mu.Lock()
	if transaction.refs > 0 {
		transaction.refs--
	}
	evicted := k.trimTransactionsLocked()
	k.mu.Unlock()
	closeKafkaClients(evicted)
}

func (k *KafkaLog) trimTransactionsLocked() []*kgo.Client {
	var clients []*kgo.Client
	for len(k.transactions) > maxCachedTransactions {
		var victim *list.Element
		for element := k.transactionLRU.Back(); element != nil; element = element.Prev() {
			if element.Value.(*transactionProducer).refs == 0 {
				victim = element
				break
			}
		}
		if victim == nil {
			break
		}
		transaction := victim.Value.(*transactionProducer)
		delete(k.transactions, transaction.caseID)
		k.transactionLRU.Remove(victim)
		transaction.lru = nil
		clients = append(clients, transaction.client)
	}
	return clients
}

func (k *KafkaLog) Commit(ctx context.Context, c Case, step Step) error {
	if err := validateStep(step); err != nil {
		return err
	}
	transaction, err := k.acquireTransaction(c)
	if err != nil {
		return err
	}
	defer k.releaseTransaction(transaction)
	var diagnostics []error
	defer func() {
		for _, diagnostic := range diagnostics {
			k.diagnose(diagnostic)
		}
	}()
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	cl := transaction.client
	if err := cl.BeginTransaction(); err != nil {
		return fmt.Errorf("begin execution transaction: %w", err)
	}

	recs := make([]*kgo.Record, 0, len(step.Appends)+1)
	for _, a := range step.Appends {
		recs = append(recs, &kgo.Record{Topic: a.Topic, Key: a.Key, Value: a.Value, Partition: 0})
	}
	note, err := json.Marshal(attentionNote{PC: step.PC, Summons: step.Summons, Ledger: step.Ledger, Gazette: step.Gazette, Heard: step.Heard})
	if err != nil {
		return fmt.Errorf("encode attention: %w", err)
	}
	recs = append(recs, &kgo.Record{Topic: c.AttentionTopic(), Key: []byte("attention"), Value: note, Partition: 0})

	if err := cl.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		abortBufferedErr := cl.AbortBufferedRecords(cleanupCtx)
		abortTransactionErr := cl.EndTransaction(cleanupCtx, kgo.TryAbort)
		return errors.Join(fmt.Errorf("produce execution step: %w", err), abortBufferedErr, abortTransactionErr)
	}
	if err := cl.EndTransaction(ctx, kgo.TryCommit); err != nil {
		return &AmbiguousCommitError{Err: err}
	}

	// Mirror the authoritative attention in consumer offsets for standard
	// Kafka tooling. A mirror failure cannot roll back the committed step.
	if physicalPC, err := k.proceedingsPhysicalCursor(ctx, c, step.PC); err != nil {
		diagnostics = append(diagnostics, fmt.Errorf("map proceedings offset after committed step: %w", err))
	} else {
		offs := make(kadm.Offsets)
		offs.AddOffset(c.Proceedings(), 0, physicalPC, -1)
		if responses, err := k.adm.CommitOffsets(ctx, c.Group(), offs); err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("mirror proceedings offset after committed step: %w", err))
		} else if err := responses.Error(); err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("mirror proceedings offset after committed step: %w", err))
		}
	}
	soffs := make(kadm.Offsets)
	soffs.AddOffset(c.Summons(), 0, step.Summons, -1)
	if responses, err := k.adm.CommitOffsets(ctx, c.SummonsGroup(), soffs); err != nil {
		diagnostics = append(diagnostics, fmt.Errorf("mirror summons offset after committed step: %w", err))
	} else if err := responses.Error(); err != nil {
		diagnostics = append(diagnostics, fmt.Errorf("mirror summons offset after committed step: %w", err))
	}
	return nil
}

func (k *KafkaLog) Attention(ctx context.Context, c Case) (Attention, error) {
	// Compaction may retain older notes, so use the last visible record.
	recs, err := k.ReadAll(ctx, c.AttentionTopic())
	if err != nil {
		return Attention{}, err
	}
	if len(recs) == 0 {
		return Attention{}, nil
	}
	var note attentionNote
	if err := json.Unmarshal(recs[len(recs)-1].Value, &note); err != nil {
		return Attention{}, fmt.Errorf("decode attention: %w", err)
	}
	return Attention{PC: note.PC, Summons: note.Summons, Ledger: note.Ledger, Gazette: note.Gazette, Heard: note.Heard, Started: true}, nil
}

func (k *KafkaLog) CreateCaseTopics(ctx context.Context, c Case) error {
	allTopics := c.AllTopics()
	details, err := k.adm.ListTopics(ctx)
	if err != nil {
		return fmt.Errorf("inspect existing case topics: %w", err)
	}
	for _, topic := range allTopics {
		if _, exists := details[topic]; exists {
			return fmt.Errorf("case %s already has a file at %s", c.ID, topic)
		}
	}
	retainForever := map[string]*string{
		"retention.ms": new("-1"), // required for replay
	}
	compacted := map[string]*string{
		"cleanup.policy": new("compact"), // superseded values may be compacted
		"retention.ms":   new("-1"),
	}
	var plain, compact []string
	for _, t := range allTopics {
		if t == c.Records() || t == c.AttentionTopic() || t == c.Catalog() {
			compact = append(compact, t)
		} else {
			plain = append(plain, t)
		}
	}
	rollback := func(cause error, targets []string) error {
		if len(targets) == 0 {
			return cause
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		responses, deleteErr := k.adm.DeleteTopics(cleanupCtx, targets...)
		var cleanupErrors []error
		if deleteErr != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("roll back partial case file: %w", deleteErr))
		}
		for _, response := range responses {
			if response.Err != nil && !errors.Is(response.Err, kerr.UnknownTopicOrPartition) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("roll back %s: %w", response.Topic, response.Err))
			}
		}
		return errors.Join(append([]error{cause}, cleanupErrors...)...)
	}
	for _, batch := range []struct {
		configs map[string]*string
		topics  []string
	}{{retainForever, plain}, {compacted, compact}} {
		resp, err := k.adm.CreateTopics(ctx, 1, 1, batch.configs, batch.topics...)
		if err != nil {
			return rollback(fmt.Errorf("create case topics: %w", err), caseCreationRollbackTargets(allTopics))
		}
		for _, r := range resp {
			if r.Err != nil {
				return rollback(fmt.Errorf("create case topic %s: %w", r.Topic, r.Err), caseCreationRollbackTargets(allTopics))
			}
		}
	}
	return nil
}

// caseCreationRollbackTargets returns every preflight-absent case topic. A
// failed CreateTopics response can be ambiguous about which topics landed, so
// limiting rollback to definite-success responses can leave a partial case.
func caseCreationRollbackTargets(allTopics []string) []string {
	return slices.Clone(allTopics)
}

func (k *KafkaLog) DeleteCaseTopics(ctx context.Context, c Case) error {
	resp, err := k.adm.DeleteTopics(ctx, c.AllTopics()...)
	if err != nil {
		return err
	}
	for _, r := range resp {
		if r.Err != nil && !errors.Is(r.Err, kerr.UnknownTopicOrPartition) {
			return fmt.Errorf("delete topic %s: %w", r.Topic, r.Err)
		}
	}
	k.dropProceedingCache(c.Proceedings())
	return nil
}

func (k *KafkaLog) ListCases(ctx context.Context) ([]Case, error) {
	details, err := k.adm.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	var out []Case
	for name := range details {
		id, ok := strings.CutSuffix(name, ".filing")
		if !ok {
			continue
		}
		caseFile, err := ParseCase(id)
		if err != nil {
			continue
		}
		out = append(out, caseFile)
		if len(out) > MaxCases {
			return nil, fmt.Errorf("%w: docket has more than %d cases", ErrResourceLimit, MaxCases)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (k *KafkaLog) EnsureTopic(ctx context.Context, topic string) error {
	resp, err := k.adm.CreateTopics(ctx, 1, 1, map[string]*string{
		"retention.ms": new("-1"),
	}, topic)
	if err != nil {
		return err
	}
	for _, r := range resp {
		if r.Err != nil && !errors.Is(r.Err, kerr.TopicAlreadyExists) {
			return fmt.Errorf("create topic %s: %w", r.Topic, r.Err)
		}
	}
	return nil
}

func (k *KafkaLog) ListStatutes(ctx context.Context) ([]string, error) {
	details, err := k.adm.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for name := range details {
		if rest, ok := strings.CutPrefix(name, "statute-"); ok {
			if s, ok := strings.CutSuffix(rest, ".filing"); ok && validStatuteName(s) {
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (k *KafkaLog) Close() {
	k.closeOnce.Do(func() {
		k.mu.Lock()
		k.closed = true
		var clients []*kgo.Client
		for _, cur := range k.consumers {
			clients = append(clients, cur.client)
		}
		for _, transaction := range k.transactions {
			clients = append(clients, transaction.client)
		}
		k.consumers = nil
		k.consumerLRU = nil
		k.transactions = nil
		k.transactionLRU = nil
		k.proceedings = nil
		k.proceedingsLRU = nil
		producer := k.producer
		k.mu.Unlock()

		closeKafkaClients(clients)
		if producer != nil {
			producer.Close()
		}
	})
}

func firstFetchErr(fetches kgo.Fetches) error {
	for _, fe := range fetches.Errors() {
		if fe.Err != nil && !errors.Is(fe.Err, context.Canceled) && !errors.Is(fe.Err, context.DeadlineExceeded) {
			return fmt.Errorf("fetching from %s: %w", fe.Topic, fe.Err)
		}
	}
	return nil
}
