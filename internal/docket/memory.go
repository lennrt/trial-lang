package docket

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// MemoryLog implements Log without external services.
type MemoryLog struct {
	mu        sync.Mutex
	cond      *sync.Cond
	topics    map[string][]Record
	attention map[string]Attention // case ID -> position
}

func NewMemoryLog() *MemoryLog {
	m := &MemoryLog{
		topics:    make(map[string][]Record),
		attention: make(map[string]Attention),
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

func (m *MemoryLog) Append(ctx context.Context, topic string, key, value []byte) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := validateRecord(key, value); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.topics[topic]; !ok {
		return 0, fmt.Errorf("%w: %q", ErrTopicNotFound, topic)
	}
	off := int64(len(m.topics[topic]))
	m.topics[topic] = append(m.topics[topic], Record{Offset: off, Key: cloneBytes(key), Value: cloneBytes(value)})
	m.cond.Broadcast()
	return off, nil
}

func (m *MemoryLog) AppendBatch(ctx context.Context, appends []StepAppend) ([]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateBatch(appends); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, appendRecord := range appends {
		if _, ok := m.topics[appendRecord.Topic]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrTopicNotFound, appendRecord.Topic)
		}
	}
	offsets := make([]int64, len(appends))
	for i, appendRecord := range appends {
		records := m.topics[appendRecord.Topic]
		offsets[i] = int64(len(records))
		m.topics[appendRecord.Topic] = append(records, Record{
			Offset: offsets[i],
			Key:    cloneBytes(appendRecord.Key),
			Value:  cloneBytes(appendRecord.Value),
		})
	}
	if len(appends) > 0 {
		m.cond.Broadcast()
	}
	return offsets, nil
}

func (m *MemoryLog) ReadAll(ctx context.Context, topic string) ([]Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, ok := m.topics[topic]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrTopicNotFound, topic)
	}
	bytes := 0
	for _, r := range recs {
		bytes += len(r.Key) + len(r.Value)
		if err := checkSnapshotSize(len(recs), bytes); err != nil {
			return nil, err
		}
	}
	out := make([]Record, len(recs))
	for i, r := range recs {
		out[i] = cloneRecord(r)
	}
	return out, nil
}

func (m *MemoryLog) End(ctx context.Context, topic string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, ok := m.topics[topic]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrTopicNotFound, topic)
	}
	return int64(len(recs)), nil
}

func (m *MemoryLog) Fetch(ctx context.Context, topic string, offset int64, wait bool) (*Record, error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset must be nonnegative: %d", offset)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		recs, ok := m.topics[topic]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrTopicNotFound, topic)
		}
		if offset < int64(len(recs)) {
			r := cloneRecord(recs[offset])
			return &r, nil
		}
		if !wait {
			return nil, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Wake on append; ctx cancellation is checked each round. A
		// watcher goroutine broadcasts when ctx dies so waiters notice.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				// Taking the mutex ensures Broadcast cannot race ahead of
				// Wait and disappear before this goroutine is registered.
				m.mu.Lock()
				m.cond.Broadcast()
				m.mu.Unlock()
			case <-done:
			}
		}()
		m.cond.Wait()
		close(done)
	}
}

// Commit validates the complete step before changing state.
func (m *MemoryLog) Commit(ctx context.Context, c Case, step Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateStep(step); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range step.Appends {
		if _, ok := m.topics[a.Topic]; !ok {
			return fmt.Errorf("%w: %q", ErrTopicNotFound, a.Topic)
		}
	}
	for _, a := range step.Appends {
		recs := m.topics[a.Topic]
		m.topics[a.Topic] = append(recs, Record{Offset: int64(len(recs)), Key: cloneBytes(a.Key), Value: cloneBytes(a.Value)})
	}
	// Copy Heard so committed attention cannot alias the caller.
	var heard []int64
	if len(step.Heard) > 0 {
		heard = append(heard, step.Heard...)
	}
	m.attention[c.ID] = Attention{PC: step.PC, Summons: step.Summons, Ledger: step.Ledger, Gazette: step.Gazette, Heard: heard, Started: true}
	m.cond.Broadcast()
	return nil
}

func (m *MemoryLog) Attention(ctx context.Context, c Case) (Attention, error) {
	if err := ctx.Err(); err != nil {
		return Attention{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	att := m.attention[c.ID]
	if len(att.Heard) > 0 {
		att.Heard = append([]int64(nil), att.Heard...)
	}
	return att, nil
}

func (m *MemoryLog) ListCases(ctx context.Context) ([]Case, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Case
	for t := range m.topics {
		// Statute filing topics do not represent cases.
		if id, ok := strings.CutSuffix(t, ".filing"); ok && strings.HasPrefix(id, "case-") {
			out = append(out, Case{ID: id})
			if len(out) > MaxCases {
				return nil, fmt.Errorf("%w: docket has more than %d cases", ErrResourceLimit, MaxCases)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MemoryLog) EnsureTopic(ctx context.Context, topic string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.topics[topic]; !ok {
		m.topics[topic] = []Record{}
	}
	return nil
}

func (m *MemoryLog) ListStatutes(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for t := range m.topics {
		if rest, ok := strings.CutPrefix(t, "statute-"); ok {
			if s, ok := strings.CutSuffix(rest, ".filing"); ok {
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *MemoryLog) CreateCaseTopics(ctx context.Context, c Case) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range c.AllTopics() {
		if _, ok := m.topics[t]; ok {
			return fmt.Errorf("case %s already has a file", c.ID)
		}
	}
	for _, t := range c.AllTopics() {
		m.topics[t] = []Record{}
	}
	return nil
}

func (m *MemoryLog) DeleteCaseTopics(ctx context.Context, c Case) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range c.AllTopics() {
		delete(m.topics, t)
	}
	delete(m.attention, c.ID)
	return nil
}

func (m *MemoryLog) Close() {}

func cloneBytes(b []byte) []byte {
	return append([]byte(nil), b...)
}

func cloneRecord(r Record) Record {
	return Record{Offset: r.Offset, Key: cloneBytes(r.Key), Value: cloneBytes(r.Value)}
}
