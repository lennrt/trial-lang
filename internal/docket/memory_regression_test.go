package docket

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryPreservesNilAndEmptyRecords(t *testing.T) {
	for _, write := range []string{"append", "batch", "commit"} {
		t.Run(write, func(t *testing.T) {
			ctx := context.Background()
			m := NewMemoryLog()
			c := Case{ID: "case-000000000000000000000001"}
			if err := m.CreateCaseTopics(ctx, c); err != nil {
				t.Fatal(err)
			}
			values := [][]byte{nil, {}, []byte("evidence")}
			appends := make([]StepAppend, len(values))
			for i, value := range values {
				appends[i] = StepAppend{Topic: c.Proclamations(), Key: value, Value: value}
			}
			switch write {
			case "append":
				for _, a := range appends {
					if _, err := m.Append(ctx, a.Topic, a.Key, a.Value); err != nil {
						t.Fatal(err)
					}
				}
			case "batch":
				if _, err := m.AppendBatch(ctx, appends); err != nil {
					t.Fatal(err)
				}
			case "commit":
				if err := m.Commit(ctx, c, Step{Appends: appends, PC: 1}); err != nil {
					t.Fatal(err)
				}
			}
			check := func(label string, records []Record) {
				t.Helper()
				if len(records) != len(values) {
					t.Fatalf("%s: got %d records, want %d", label, len(records), len(values))
				}
				for i, r := range records {
					for name, got := range map[string][]byte{"key": r.Key, "value": r.Value} {
						if !bytes.Equal(got, values[i]) || (got == nil) != (values[i] == nil) {
							t.Errorf("%s record %d %s = %#v, want %#v", label, i, name, got, values[i])
						}
					}
				}
			}
			// Check retained state separately: ReadAll must not hide a bad write.
			check("stored", m.topics[c.Proclamations()])
			records, err := m.ReadAll(ctx, c.Proclamations())
			if err != nil {
				t.Fatal(err)
			}
			check("ReadAll", records)
			fetched := make([]Record, 0, len(values))
			for i := range values {
				r, err := m.Fetch(ctx, c.Proclamations(), int64(i), false)
				if err != nil || r == nil {
					t.Fatalf("Fetch(%d) = %v, %v", i, r, err)
				}
				fetched = append(fetched, *r)
			}
			check("Fetch", fetched)
		})
	}
}

// A Cond queues its waiter before calling Unlock. This barrier makes deletion
// happen after Fetch is waiting, without sleeps or scheduler assumptions.
type fetchWaitLocker struct {
	sync.Locker
	waiting chan struct{}
	once    sync.Once
}

func (l *fetchWaitLocker) Unlock() {
	l.once.Do(func() { close(l.waiting) })
	l.Locker.Unlock()
}

func TestMemoryDeleteWakesWaitingFetch(t *testing.T) {
	m := NewMemoryLog()
	c := Case{ID: "case-000000000000000000000002"}
	if err := m.CreateCaseTopics(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	waiting := make(chan struct{})
	m.cond = sync.NewCond(&fetchWaitLocker{Locker: &m.mu, waiting: waiting})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := m.Fetch(ctx, c.Summons(), 0, true)
		done <- err
	}()
	defer cancel()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
		t.Fatal("Fetch did not start waiting")
	}
	if err := m.DeleteCaseTopics(context.Background(), c); err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrTopicNotFound) {
			t.Fatalf("Fetch after deletion = %v, want ErrTopicNotFound", err)
		}
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("deleting the case left Fetch blocked")
	}
}

func TestMemoryFetchHonorsCanceledContext(t *testing.T) {
	m := NewMemoryLog()
	if err := m.EnsureTopic(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Append(context.Background(), "test", nil, []byte("present")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, wait := range []bool{false, true} {
		for _, offset := range []int64{0, 1} {
			r, err := m.Fetch(ctx, "test", offset, wait)
			if r != nil || !errors.Is(err, context.Canceled) {
				t.Errorf("Fetch(offset=%d, wait=%t) = %v, %v, want nil, context.Canceled", offset, wait, r, err)
			}
		}
	}
}

func TestCloneBytesPreservesOwnership(t *testing.T) {
	original := []byte("evidence")
	copy := cloneBytes(original)
	copy[0] = 'X'
	if string(original) != "evidence" {
		t.Fatal("cloneBytes retained caller-owned storage")
	}
}
