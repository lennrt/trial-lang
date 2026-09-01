package docket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaLog stores execution state in single-partition Kafka topics.
type KafkaLog struct {
	producer *kgo.Client // plain paperwork: filings, verdicts, summonses, markers
	adm      *kadm.Client

	brokers []string

	mu        sync.Mutex
	consumers map[string]*cursor     // one fetch cursor per topic
	officials map[string]*kgo.Client // one transactional producer per case
	diagnose  func(error)
	closeOnce sync.Once
}

// cursor is a direct (group-less) consumer pinned to partition 0 of
// one topic, plus the offset it will look for next. Jumps move it with
// SetOffsets. Transactional commit markers leave silent gaps in the
// offsets, so the cursor yields the first record at or after the
// requested position.
type cursor struct {
	mu     sync.Mutex
	client *kgo.Client
	next   int64
	buf    []*kgo.Record
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
		producer:  producer,
		adm:       kadm.NewClient(producer),
		brokers:   seeds,
		consumers: make(map[string]*cursor),
		officials: make(map[string]*kgo.Client),
		diagnose:  config.diagnose,
	}, nil
}

func (k *KafkaLog) Append(ctx context.Context, topic string, key, value []byte) (int64, error) {
	if err := validateRecord(key, value); err != nil {
		return 0, err
	}
	rec := &kgo.Record{Topic: topic, Key: cloneBytes(key), Value: cloneBytes(value), Partition: 0}
	res := k.producer.ProduceSync(ctx, rec)
	if err := res.FirstErr(); err != nil {
		return 0, fmt.Errorf("the record was refused at the counter: %w", err)
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
			reason = fmt.Errorf("topic %q has no partition 0; the case file is missing", topic)
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

	k.mu.Lock()
	cur, ok := k.consumers[topic]
	if !ok {
		cl, err := kgo.NewClient(
			kgo.SeedBrokers(k.brokers...),
			kgo.FetchIsolationLevel(kgo.ReadCommitted()),
			kgo.FetchMaxWait(100*time.Millisecond),
			kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{
				topic: {0: kgo.NewOffset().At(offset)},
			}),
		)
		if err != nil {
			k.mu.Unlock()
			return nil, err
		}
		cur = &cursor{client: cl, next: offset}
		k.consumers[topic] = cur
	}
	k.mu.Unlock()
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

// attentionNote records the authoritative execution cursors in the same
// transaction as an instruction's effects.
type attentionNote struct {
	PC      int64   `json:"pc"`
	Summons int64   `json:"summons"`
	Ledger  int64   `json:"ledger,omitempty"`
	Gazette int64   `json:"gazette,omitempty"`
	Heard   []int64 `json:"heard,omitempty"`
}

// official returns the transactional producer for a case. The per-case
// transactional ID ensures a second producer fences the first.
func (k *KafkaLog) official(c Case) (*kgo.Client, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if cl, ok := k.officials[c.ID]; ok {
		return cl, nil
	}
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(k.brokers...),
		kgo.TransactionalID("the-court."+c.ID),
		kgo.TransactionTimeout(60*time.Second),
	)
	if err != nil {
		return nil, err
	}
	k.officials[c.ID] = cl
	return cl, nil
}

func (k *KafkaLog) Commit(ctx context.Context, c Case, step Step) error {
	if err := validateStep(step); err != nil {
		return err
	}
	cl, err := k.official(c)
	if err != nil {
		return err
	}
	if err := cl.BeginTransaction(); err != nil {
		return fmt.Errorf("the session could not be opened: %w", err)
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
	offs := make(kadm.Offsets)
	offs.AddOffset(c.Proceedings(), 0, step.PC, -1)
	if responses, err := k.adm.CommitOffsets(ctx, c.Group(), offs); err != nil {
		k.diagnose(fmt.Errorf("mirror proceedings offset after committed step: %w", err))
	} else if err := responses.Error(); err != nil {
		k.diagnose(fmt.Errorf("mirror proceedings offset after committed step: %w", err))
	}
	soffs := make(kadm.Offsets)
	soffs.AddOffset(c.Summons(), 0, step.Summons, -1)
	if responses, err := k.adm.CommitOffsets(ctx, c.SummonsGroup(), soffs); err != nil {
		k.diagnose(fmt.Errorf("mirror summons offset after committed step: %w", err))
	} else if err := responses.Error(); err != nil {
		k.diagnose(fmt.Errorf("mirror summons offset after committed step: %w", err))
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
		return Attention{}, fmt.Errorf("the Court's attention is recorded in a hand no one can read: %w", err)
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
	created := make([]string, 0, len(allTopics))
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
			return rollback(fmt.Errorf("create case topics: %w", err), allTopics)
		}
		for _, r := range resp {
			if r.Err != nil {
				return rollback(fmt.Errorf("the case file could not be opened (%s): %w", r.Topic, r.Err), created)
			}
			created = append(created, r.Topic)
		}
	}
	return nil
}

func (k *KafkaLog) DeleteCaseTopics(ctx context.Context, c Case) error {
	resp, err := k.adm.DeleteTopics(ctx, c.AllTopics()...)
	if err != nil {
		return err
	}
	for _, r := range resp {
		if r.Err != nil && !errors.Is(r.Err, kerr.UnknownTopicOrPartition) {
			return fmt.Errorf("burning %s failed: %w", r.Topic, r.Err)
		}
	}
	return nil
}

func (k *KafkaLog) ListCases(ctx context.Context) ([]Case, error) {
	details, err := k.adm.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	var out []Case
	for name := range details {
		if id, ok := strings.CutSuffix(name, ".filing"); ok && strings.HasPrefix(id, "case-") {
			out = append(out, Case{ID: id})
			if len(out) > MaxCases {
				return nil, fmt.Errorf("%w: docket has more than %d cases", ErrResourceLimit, MaxCases)
			}
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
			return fmt.Errorf("the topic %s could not be opened: %w", r.Topic, r.Err)
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
			if s, ok := strings.CutSuffix(rest, ".filing"); ok {
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
		defer k.mu.Unlock()
		for _, cur := range k.consumers {
			cur.client.Close()
		}
		for _, cl := range k.officials {
			cl.Close()
		}
		k.producer.Close()
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
