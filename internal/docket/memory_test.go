package docket

// The in-memory log claims to observe the same contract as Kafka.
// These tests hold it to that claim; the court tests upstairs assume
// every clause of it.

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

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
	c := Case{ID: "case-genuine"}
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
