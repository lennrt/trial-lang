package docket

// The in-memory log claims to observe the same contract as Kafka.
// These tests hold it to that claim; the court tests upstairs assume
// every clause of it.

import (
	"container/list"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAmbiguousCommitErrorNamesTheGenericRecoveryContract(t *testing.T) {
	cause := errors.New("acknowledgement lost")
	err := &AmbiguousCommitError{Err: cause}
	if !errors.Is(err, cause) {
		t.Fatal("AmbiguousCommitError does not unwrap its cause")
	}
	if got := err.Error(); !strings.Contains(got, "reread authoritative state before retrying") || strings.Contains(got, "attention") {
		t.Fatalf("AmbiguousCommitError = %q, want operation-independent recovery guidance", got)
	}
}

func TestNewCaseHasEnoughEntropy(t *testing.T) {
	c, err := NewCase()
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "case-"
	if len(c.ID) != len(prefix)+24 || c.ID[:len(prefix)] != prefix {
		t.Fatalf("case number %q does not contain 96 bits of hexadecimal entropy", c.ID)
	}
	if _, err := hex.DecodeString(c.ID[len(prefix):]); err != nil {
		t.Fatalf("case number %q is not hexadecimal: %v", c.ID, err)
	}
}

func TestMemoryCommitPersistsTheFullAttention(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-attention"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	step := Step{PC: 7, Summons: 2, Ledger: 3}
	if err := m.Commit(ctx, c, step); err != nil {
		t.Fatal(err)
	}
	att, err := m.Attention(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !att.Started || att.PC != 7 || att.Summons != 2 || att.Ledger != 3 {
		t.Fatalf("attention = %+v, want PC 7, Summons 2, Ledger 3, Started", att)
	}
}

func TestMemoryListCasesExcludesStatutes(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-000000000000000000000001"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureTopic(ctx, StatuteTopic("arithmetic")); err != nil {
		t.Fatal(err)
	}
	cases, err := m.ListCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].ID != c.ID {
		t.Fatalf("the docket lists %v; statutes are paperwork, not matters", cases)
	}
	statutes, err := m.ListStatutes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statutes) != 1 || statutes[0] != "arithmetic" {
		t.Fatalf("the statutes on the books = %v, want [arithmetic]", statutes)
	}
}

func TestMemoryListingsIgnoreMalformedTopicNames(t *testing.T) {
	m := NewMemoryLog()
	ctx := t.Context()
	for _, topic := range []string{
		"case-not-a-canonical-id.filing",
		"case-00000000000000000000000G.filing",
		"statute-.filing",
		"statute-Invalid.filing",
		"statute-valid-name.filing",
	} {
		if err := m.EnsureTopic(ctx, topic); err != nil {
			t.Fatal(err)
		}
	}
	cases, err := m.ListCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Fatalf("ListCases returned malformed topic names: %v", cases)
	}
	statutes, err := m.ListStatutes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(statutes, []string{"valid-name"}) {
		t.Fatalf("ListStatutes = %v, want [valid-name]", statutes)
	}
}

func TestMemoryCommitIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-atomic"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	// One append is destined for a real topic, one for a topic that
	// does not exist. The step must fail whole: nothing lands, and the
	// attention does not move.
	step := Step{
		Appends: []StepAppend{
			{Topic: c.Proclamations(), Value: []byte("half")},
			{Topic: "no-such-topic", Value: []byte("said")},
		},
		PC: 5,
	}
	if err := m.Commit(ctx, c, step); err == nil {
		t.Fatal("a step touching a nonexistent topic should fail")
	}
	recs, err := m.ReadAll(ctx, c.Proclamations())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("a failed step half-said something: %q", recs[0].Value)
	}
	att, err := m.Attention(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if att.Started || att.PC != 0 {
		t.Fatalf("a failed step advanced the attention to %+v", att)
	}
}

func TestMemoryAppendRefusesUnopenedTopics(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	if _, err := m.Append(ctx, "case-nowhere.summons", nil, []byte("1")); err == nil {
		t.Fatal("appending to an unopened topic should fail; the case file was never opened")
	}
}

func TestMemoryFetchWithoutWaiting(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-fetch"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Append(ctx, c.Proclamations(), nil, []byte("present")); err != nil {
		t.Fatal(err)
	}
	rec, err := m.Fetch(ctx, c.Proclamations(), 0, false)
	if err != nil || rec == nil || string(rec.Value) != "present" {
		t.Fatalf("fetch at 0 = %v, %v", rec, err)
	}
	rec, err = m.Fetch(ctx, c.Proclamations(), 1, false)
	if err != nil || rec != nil {
		t.Fatalf("fetch past the end without waiting should be nil, nil; got %v, %v", rec, err)
	}
}

func TestMemoryRecordsDoNotAliasCallers(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-copies"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	key, value := []byte("key"), []byte("original")
	if _, err := m.Append(ctx, c.Proclamations(), key, value); err != nil {
		t.Fatal(err)
	}
	key[0], value[0] = 'X', 'X'
	recs, err := m.ReadAll(ctx, c.Proclamations())
	if err != nil {
		t.Fatal(err)
	}
	if string(recs[0].Key) != "key" || string(recs[0].Value) != "original" {
		t.Fatalf("append retained caller-owned storage: key=%q value=%q", recs[0].Key, recs[0].Value)
	}
	recs[0].Key[0], recs[0].Value[0] = 'Y', 'Y'
	again, err := m.ReadAll(ctx, c.Proclamations())
	if err != nil {
		t.Fatal(err)
	}
	if string(again[0].Key) != "key" || string(again[0].Value) != "original" {
		t.Fatalf("ReadAll exposed internal storage: key=%q value=%q", again[0].Key, again[0].Value)
	}
}

func TestMemoryFetchCancellationCannotMissWakeup(t *testing.T) {
	m := NewMemoryLog()
	c := Case{ID: "case-cancel"}
	if err := m.CreateCaseTopics(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := m.Fetch(ctx, c.Proclamations(), 0, true)
			done <- err
		}()
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Fetch returned %v after cancellation", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Fetch missed context cancellation and remained blocked")
		}
	}
}

func TestMemoryDenseOffsets(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-offsets"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		off, err := m.Append(ctx, c.Proceedings(), nil, []byte{byte('a' + i)})
		if err != nil {
			t.Fatal(err)
		}
		if off != int64(i) {
			t.Fatalf("append %d landed at offset %d; offsets are addresses and must be dense", i, off)
		}
	}
}

func TestMemoryDeleteCaseTopics(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-burned"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := m.Commit(ctx, c, Step{PC: 1}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	cases, err := m.ListCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Fatalf("the docket still lists %v after the burning", cases)
	}
	if att, _ := m.Attention(ctx, c); att.Started {
		t.Fatalf("the burned case still has attention on file: %+v", att)
	}
}

func TestMemoryCreateCaseTopicsIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLog()
	c := Case{ID: "case-collision"}
	if err := m.EnsureTopic(ctx, c.Records()); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateCaseTopics(ctx, c); err == nil {
		t.Fatal("opening a case over an existing topic should fail")
	}
	if _, err := m.ReadAll(ctx, c.Filing()); err == nil {
		t.Fatal("a failed opening left a partial case file behind")
	}
}

func TestMemoryAppendBatchIsAtomicAndCopiesInputs(t *testing.T) {
	ctx := t.Context()
	m := NewMemoryLog()
	c := Case{ID: "case-batch-copy"}
	if err := m.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	value := []byte("first")
	offsets, err := m.AppendBatch(ctx, []StepAppend{
		{Topic: c.Summons(), Value: value},
		{Topic: c.Summons(), Value: []byte("second")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 2 || offsets[0] != 0 || offsets[1] != 1 {
		t.Fatalf("offsets = %v, want [0 1]", offsets)
	}
	value[0] = 'X'
	records, err := m.ReadAll(ctx, c.Summons())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(records[0].Value); got != "first" {
		t.Fatalf("retained caller buffer mutation: got %q", got)
	}

	_, err = m.AppendBatch(ctx, []StepAppend{
		{Topic: c.Summons(), Value: []byte("third")},
		{Topic: "missing", Value: []byte("never")},
	})
	if !errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("missing topic error = %v", err)
	}
	end, err := m.End(ctx, c.Summons())
	if err != nil {
		t.Fatal(err)
	}
	if end != 2 {
		t.Fatalf("failed batch changed the topic end to %d", end)
	}
}

func TestResourceLimitBoundaries(t *testing.T) {
	if err := checkSnapshotSize(MaxReadRecords, MaxReadBytes); err != nil {
		t.Fatalf("exact snapshot bounds failed: %v", err)
	}
	if err := checkSnapshotSize(MaxReadRecords+1, 0); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("record overflow error = %v", err)
	}
	if err := checkSnapshotSize(0, MaxReadBytes+1); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("byte overflow error = %v", err)
	}
	if err := validateBatch(make([]StepAppend, MaxBatchAppends+1)); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("batch count overflow error = %v", err)
	}
}

func TestProceedingsCacheHasAnAggregateLRUBound(t *testing.T) {
	log := &KafkaLog{}
	for pc := range int64(maxProceedingCacheEntries + 17) {
		topic := "case-x.proceedings"
		if pc%2 != 0 {
			topic = "case-y.proceedings"
		}
		if err := log.cacheProceeding(topic, pc, pc*2); err != nil {
			t.Fatal(err)
		}
	}
	log.mu.Lock()
	entries, lruEntries := len(log.proceedings), log.proceedingsLRU.Len()
	log.mu.Unlock()
	if entries != maxProceedingCacheEntries || lruEntries != maxProceedingCacheEntries {
		t.Fatalf("cache sizes = map %d, LRU %d; want %d", entries, lruEntries, maxProceedingCacheEntries)
	}
	if _, ok := log.cachedProceeding("case-x.proceedings", 0); ok {
		t.Fatal("least-recently-used mapping was not evicted")
	}
	newest := int64(maxProceedingCacheEntries + 16)
	if physical, ok := log.cachedProceeding("case-x.proceedings", newest); !ok || physical != newest*2 {
		t.Fatalf("newest mapping = %d, %v", physical, ok)
	}
	if err := log.cacheProceeding("case-x.proceedings", newest, 7); err == nil {
		t.Fatal("cache accepted a changed physical mapping")
	}
}

func TestProceedingsCacheIsSafeForConcurrentCases(t *testing.T) {
	log := &KafkaLog{}
	errCh := make(chan error, 8)
	var wg sync.WaitGroup
	for worker := range int64(8) {
		wg.Go(func() {
			topic := "case-even.proceedings"
			if worker%2 != 0 {
				topic = "case-odd.proceedings"
			}
			for i := range int64(1_000) {
				pc := worker*1_000 + i
				if err := log.cacheProceeding(topic, pc, pc+10); err != nil {
					errCh <- err
					return
				}
				if physical, ok := log.cachedProceeding(topic, pc); !ok || physical != pc+10 {
					errCh <- fmt.Errorf("mapping %s/%d = %d, %v", topic, pc, physical, ok)
					return
				}
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.proceedings) > maxProceedingCacheEntries || log.proceedingsLRU.Len() != len(log.proceedings) {
		t.Fatalf("concurrent cache sizes = map %d, LRU %d", len(log.proceedings), log.proceedingsLRU.Len())
	}
}

func TestProceedingsWaitGrowthHonorsSnapshotByteLimit(t *testing.T) {
	record := &Record{Offset: 42, Value: []byte("x")}
	if _, err := proceedingSnapshotGrowth(1, MaxReadBytes, record); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("growth error = %v, want ErrResourceLimit", err)
	}
	if bytes, err := proceedingSnapshotGrowth(1, MaxReadBytes-1, record); err != nil || bytes != MaxReadBytes {
		t.Fatalf("growth at exact limit = %d, %v", bytes, err)
	}
}

func TestKafkaClientRegistriesBoundIdleClientsAndPinActiveOnes(t *testing.T) {
	log := &KafkaLog{
		consumers:      make(map[string]*cursor),
		consumerLRU:    list.New(),
		transactions:   make(map[string]*transactionProducer),
		transactionLRU: list.New(),
	}

	activeCursor := &cursor{topic: "active-topic", refs: 1}
	activeCursor.lru = log.consumerLRU.PushFront(activeCursor)
	log.consumers[activeCursor.topic] = activeCursor
	for i := range maxCachedConsumers + 3 {
		cur := &cursor{topic: fmt.Sprintf("idle-topic-%02d", i)}
		cur.lru = log.consumerLRU.PushFront(cur)
		log.consumers[cur.topic] = cur
	}

	activeTransaction := &transactionProducer{caseID: "active-case", refs: 1}
	activeTransaction.lru = log.transactionLRU.PushFront(activeTransaction)
	log.transactions[activeTransaction.caseID] = activeTransaction
	for i := range maxCachedTransactions + 3 {
		transaction := &transactionProducer{caseID: fmt.Sprintf("idle-case-%02d", i)}
		transaction.lru = log.transactionLRU.PushFront(transaction)
		log.transactions[transaction.caseID] = transaction
	}

	log.mu.Lock()
	consumerEvictions := log.trimConsumersLocked()
	transactionEvictions := log.trimTransactionsLocked()
	_, cursorPinned := log.consumers[activeCursor.topic]
	_, transactionPinned := log.transactions[activeTransaction.caseID]
	consumerCount := len(log.consumers)
	transactionCount := len(log.transactions)
	log.mu.Unlock()

	if !cursorPinned || !transactionPinned {
		t.Fatal("LRU trimming evicted an active Kafka client")
	}
	if consumerCount != maxCachedConsumers || transactionCount != maxCachedTransactions {
		t.Fatalf("registry sizes = consumers %d, transactions %d; want %d and %d", consumerCount, transactionCount, maxCachedConsumers, maxCachedTransactions)
	}
	if len(consumerEvictions) != 4 || len(transactionEvictions) != 4 {
		t.Fatalf("evictions = consumers %d, transactions %d; want four each", len(consumerEvictions), len(transactionEvictions))
	}
}

func TestKafkaClientRegistriesTrimAfterPinnedOperationsFinish(t *testing.T) {
	log := &KafkaLog{
		consumers:      make(map[string]*cursor),
		consumerLRU:    list.New(),
		transactions:   make(map[string]*transactionProducer),
		transactionLRU: list.New(),
	}
	var cursors []*cursor
	var transactions []*transactionProducer
	for i := range maxCachedConsumers + 5 {
		cur := &cursor{topic: fmt.Sprintf("topic-%02d", i), refs: 1}
		cur.lru = log.consumerLRU.PushFront(cur)
		log.consumers[cur.topic] = cur
		cursors = append(cursors, cur)
	}
	for i := range maxCachedTransactions + 5 {
		transaction := &transactionProducer{caseID: fmt.Sprintf("case-%02d", i), refs: 1}
		transaction.lru = log.transactionLRU.PushFront(transaction)
		log.transactions[transaction.caseID] = transaction
		transactions = append(transactions, transaction)
	}

	log.mu.Lock()
	if got := len(log.trimConsumersLocked()); got != 0 {
		t.Fatalf("trimmed %d pinned consumers", got)
	}
	if got := len(log.trimTransactionsLocked()); got != 0 {
		t.Fatalf("trimmed %d pinned transactions", got)
	}
	log.mu.Unlock()

	for _, cur := range cursors {
		log.releaseCursor(cur)
	}
	for _, transaction := range transactions {
		log.releaseTransaction(transaction)
	}
	log.mu.Lock()
	consumerCount := len(log.consumers)
	transactionCount := len(log.transactions)
	log.mu.Unlock()
	if consumerCount != maxCachedConsumers || transactionCount != maxCachedTransactions {
		t.Fatalf("post-release registry sizes = consumers %d, transactions %d", consumerCount, transactionCount)
	}
}

func TestKafkaClientRegistryPinsAreConcurrentSafe(t *testing.T) {
	log := &KafkaLog{
		consumers:      make(map[string]*cursor),
		consumerLRU:    list.New(),
		transactions:   make(map[string]*transactionProducer),
		transactionLRU: list.New(),
	}
	cur := &cursor{topic: "shared-topic"}
	cur.lru = log.consumerLRU.PushFront(cur)
	log.consumers[cur.topic] = cur
	transaction := &transactionProducer{caseID: "shared-case"}
	transaction.lru = log.transactionLRU.PushFront(transaction)
	log.transactions[transaction.caseID] = transaction

	errCh := make(chan error, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for i := range 500 {
				gotCursor, err := log.acquireCursor(cur.topic, int64(i))
				if err != nil {
					errCh <- err
					return
				}
				if gotCursor != cur {
					errCh <- errors.New("cursor acquisition replaced the retained cursor")
					return
				}
				log.releaseCursor(gotCursor)

				gotTransaction, err := log.acquireTransaction(Case{ID: transaction.caseID})
				if err != nil {
					errCh <- err
					return
				}
				if gotTransaction != transaction {
					errCh <- errors.New("transaction acquisition replaced the retained producer")
					return
				}
				log.releaseTransaction(gotTransaction)
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if cur.refs != 0 || transaction.refs != 0 {
		t.Fatalf("leaked pins = cursor %d, transaction %d", cur.refs, transaction.refs)
	}
}

func TestKafkaLogCloseClearsRetainedRegistriesAndRejectsNewPins(t *testing.T) {
	log := &KafkaLog{
		consumers:      make(map[string]*cursor),
		consumerLRU:    list.New(),
		proceedings:    make(map[proceedingAddress]*list.Element),
		proceedingsLRU: list.New(),
		transactions:   make(map[string]*transactionProducer),
		transactionLRU: list.New(),
	}
	cur := &cursor{topic: "retained-topic"}
	cur.lru = log.consumerLRU.PushFront(cur)
	log.consumers[cur.topic] = cur
	transaction := &transactionProducer{caseID: "retained-case"}
	transaction.lru = log.transactionLRU.PushFront(transaction)
	log.transactions[transaction.caseID] = transaction
	if err := log.cacheProceeding("retained.proceedings", 0, 4); err != nil {
		t.Fatal(err)
	}

	log.Close()
	log.Close()
	if _, err := log.acquireCursor("new-topic", 0); !errors.Is(err, errKafkaLogClosed) {
		t.Fatalf("cursor acquisition after Close = %v, want errKafkaLogClosed", err)
	}
	if _, err := log.acquireTransaction(Case{ID: "new-case"}); !errors.Is(err, errKafkaLogClosed) {
		t.Fatalf("transaction acquisition after Close = %v, want errKafkaLogClosed", err)
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.consumers != nil || log.transactions != nil || log.proceedings != nil {
		t.Fatal("Close retained a Kafka registry")
	}
}

func TestCaseCreationRollbackTargetsEveryPreflightAbsentTopic(t *testing.T) {
	c := Case{ID: "case-000000000000000000000001"}
	all := c.AllTopics()
	targets := caseCreationRollbackTargets(all)
	if !slices.Equal(targets, all) {
		t.Fatalf("rollback targets = %v, want all case topics %v", targets, all)
	}
	targets[0] = "changed"
	if all[0] == "changed" {
		t.Fatal("rollback target selection aliases its input")
	}
}

func TestParseCaseRequiresCanonicalIdentifier(t *testing.T) {
	valid := "case-0123456789abcdef01234567"
	if parsed, err := ParseCase(valid); err != nil || parsed.ID != valid {
		t.Fatalf("ParseCase(%q) = %+v, %v", valid, parsed, err)
	}
	for _, invalid := range []string{
		"", "case-0", "CASE-0123456789abcdef01234567",
		"case-0123456789ABCDEF01234567", "case-0123456789abcdef0123456g",
		"other-0123456789abcdef01234567",
	} {
		if _, err := ParseCase(invalid); !errors.Is(err, ErrInvalidCase) {
			t.Errorf("ParseCase(%q) error = %v", invalid, err)
		}
	}
}
