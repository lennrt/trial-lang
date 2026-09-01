// Package docket defines case identifiers and durable execution storage.
package docket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// MaxRecordBytes bounds each record key and value independently.
	MaxRecordBytes = 16 << 20
	// MaxReadRecords bounds one recovery snapshot.
	MaxReadRecords = 1_000_000
	// MaxReadBytes bounds the sum of keys and values in one recovery snapshot.
	MaxReadBytes = 256 << 20
	// MaxStepAppends bounds one atomic execution step.
	MaxStepAppends = 10_000
	// MaxBatchAppends bounds one atomic paperwork batch.
	MaxBatchAppends = 100_000
	// MaxBatchBytes bounds all keys and values in one paperwork batch.
	MaxBatchBytes = 64 << 20
	// MaxCases bounds one docket listing.
	MaxCases = 100_000
	// MaxHeardOffsets bounds selective-receive state retained in attention.
	MaxHeardOffsets = 100_000
)

var (
	// ErrInvalidCase reports a noncanonical case identifier.
	ErrInvalidCase = errors.New("invalid case identifier")
	// ErrTopicNotFound reports a topic that does not exist.
	ErrTopicNotFound = errors.New("topic not found")
	// ErrResourceLimit reports input or persisted state above a documented bound.
	ErrResourceLimit = errors.New("resource limit exceeded")
)

// AmbiguousCommitError reports that Kafka did not confirm whether a commit
// completed. Recovery must read Attention before retrying the step.
type AmbiguousCommitError struct {
	Err error
}

func (e *AmbiguousCommitError) Error() string {
	return "commit outcome is ambiguous; recover attention before retrying: " + e.Err.Error()
}

func (e *AmbiguousCommitError) Unwrap() error { return e.Err }

// Case identifies one program run and names its family of topics.
type Case struct {
	ID string
}

// NewCase returns a case identifier containing 96 random bits.
func NewCase() (Case, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return Case{}, fmt.Errorf("generate case identifier: %w", err)
	}
	return Case{ID: "case-" + hex.EncodeToString(b)}, nil
}

// ParseCase accepts only identifiers produced by NewCase.
func ParseCase(id string) (Case, error) {
	const prefix = "case-"
	if len(id) != len(prefix)+24 || id[:len(prefix)] != prefix {
		return Case{}, fmt.Errorf("%w: %q", ErrInvalidCase, id)
	}
	for _, c := range id[len(prefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return Case{}, fmt.Errorf("%w: %q", ErrInvalidCase, id)
		}
	}
	return Case{ID: id}, nil
}

func (c Case) topic(kind string) string { return c.ID + "." + kind }

func (c Case) Filing() string        { return c.topic("filing") }        // the .trial source, as filed
func (c Case) Proceedings() string   { return c.topic("proceedings") }   // bytecode; offset = instruction address
func (c Case) Dossier() string       { return c.topic("dossier") }       // operand-stack event log
func (c Case) Appeals() string       { return c.topic("appeals") }       // call-stack event log
func (c Case) Records() string       { return c.topic("records") }       // variables (compacted)
func (c Case) Summons() string       { return c.topic("summons") }       // stdin
func (c Case) Proclamations() string { return c.topic("proclamations") } // stdout
func (c Case) Verdicts() string      { return c.topic("verdicts") }      // verdict records
func (c Case) Ledger() string        { return c.topic("ledger") }        // every draw of the discretion, every reading of the clock
func (c Case) Archive() string       { return c.topic("archive") }       // documents, immutable; the offset is the handle
func (c Case) Catalog() string       { return c.topic("catalog") }       // document name -> current archive offset (compacted)
func (c Case) AttentionTopic() string {
	return c.topic("attention") // authoritative program counter
}

// Group is the consumer group whose committed offset on the proceedings
// topic mirrors the program counter for standard Kafka tooling.
func (c Case) Group() string { return "the-court." + c.ID }

// SummonsGroup tracks how much of the summons topic has been served.
func (c Case) SummonsGroup() string { return "the-court." + c.ID + ".summons" }

// StatuteTopic names the filing topic of an enacted statute.
func StatuteTopic(name string) string { return "statute-" + name + ".filing" }

// AllTopics lists every topic in the case file.
func (c Case) AllTopics() []string {
	return []string{
		c.Filing(), c.Proceedings(), c.Dossier(), c.Appeals(),
		c.Records(), c.Summons(), c.Proclamations(), c.Verdicts(),
		c.AttentionTopic(), c.Ledger(), c.Archive(), c.Catalog(),
	}
}

// Record is one entry in a topic. Offsets are assigned by the Log. The
// proceedings topic is written outside transactions and therefore has dense
// offsets that serve as instruction addresses; transactional topics may have
// gaps occupied by Kafka control records.
type Record struct {
	Offset int64
	Key    []byte
	Value  []byte
}

// StepAppend is one record an instruction wishes to enter into a topic.
type StepAppend struct {
	Topic string
	Key   []byte
	Value []byte
}

// Step is one atomic execution effect and its resulting cursors.
type Step struct {
	Appends []StepAppend
	PC      int64
	Summons int64
	Ledger  int64
	Gazette int64
	// Heard lists summons offsets consumed out of turn (AWAIT SUMMONS FROM).
	// They are strictly past Summons and sorted ascending.
	Heard []int64
}

// Attention is the authoritative execution position of a case.
type Attention struct {
	PC      int64
	Summons int64
	Ledger  int64   // how many ledger entries the current timeline has consumed
	Gazette int64   // how much of the gazette this case has read
	Heard   []int64 // summons offsets past Summons already consumed out of turn
	Started bool    // false if the proceedings have never once been convened
}

// Log stores execution state. Every topic has exactly one partition.
// Implementations must copy retained input and returned byte slices.
type Log interface {
	// Append writes one record outside an execution step and returns its offset.
	Append(ctx context.Context, topic string, key, value []byte) (int64, error)

	// AppendBatch writes all records atomically and returns their offsets in
	// input order. An empty batch succeeds without changing state.
	AppendBatch(ctx context.Context, appends []StepAppend) ([]int64, error)

	// ReadAll returns an ordered snapshot. It fails with ErrResourceLimit
	// instead of returning more than MaxReadRecords or MaxReadBytes.
	ReadAll(ctx context.Context, topic string) ([]Record, error)

	// End reports the topic's next offset.
	End(ctx context.Context, topic string) (int64, error)

	// Fetch returns the first visible record at or after the given offset.
	// (Kafka transaction markers and compaction can leave invisible gaps.) If
	// no such record exists yet and wait is true, Fetch blocks until one does.
	// If wait is false, it returns nil.
	Fetch(ctx context.Context, topic string, offset int64, wait bool) (*Record, error)

	// Commit applies one execution step atomically: all appends land
	// and the attention advances, or none of it happened. On Kafka
	// this is a transaction; the attention topic is authoritative, and the
	// consumer group the-court.<case> is updated afterward as a mirror.
	Commit(ctx context.Context, c Case, step Step) error

	// Attention reads back where the case stands.
	Attention(ctx context.Context, c Case) (Attention, error)

	// CreateCaseTopics opens all single-partition case topics with infinite
	// retention. Records, attention, and catalog topics are compacted.
	CreateCaseTopics(ctx context.Context, c Case) error

	// DeleteCaseTopics permanently deletes the case topics.
	DeleteCaseTopics(ctx context.Context, c Case) error

	// ListCases returns at most MaxCases cases or ErrResourceLimit.
	ListCases(ctx context.Context) ([]Case, error)

	// EnsureTopic opens a retained, single-partition topic if it does not exist.
	EnsureTopic(ctx context.Context, topic string) error

	// ListStatutes returns enacted statute names.
	ListStatutes(ctx context.Context) ([]string, error)

	// Close releases resources. It is safe to call more than once. Callers
	// must cancel and join operations before calling Close.
	Close()
}

func validateRecord(key, value []byte) error {
	if len(key) > MaxRecordBytes {
		return fmt.Errorf("%w: record key is %d bytes; limit is %d", ErrResourceLimit, len(key), MaxRecordBytes)
	}
	if len(value) > MaxRecordBytes {
		return fmt.Errorf("%w: record value is %d bytes; limit is %d", ErrResourceLimit, len(value), MaxRecordBytes)
	}
	return nil
}

func validateStep(step Step) error {
	if len(step.Appends) > MaxStepAppends {
		return fmt.Errorf("%w: step has %d appends; limit is %d", ErrResourceLimit, len(step.Appends), MaxStepAppends)
	}
	if len(step.Heard) > MaxHeardOffsets {
		return fmt.Errorf("%w: attention retains %d heard offsets; limit is %d", ErrResourceLimit, len(step.Heard), MaxHeardOffsets)
	}
	for i, appendRecord := range step.Appends {
		if err := validateRecord(appendRecord.Key, appendRecord.Value); err != nil {
			return fmt.Errorf("append %d: %w", i, err)
		}
	}
	return nil
}

func validateBatch(appends []StepAppend) error {
	if len(appends) > MaxBatchAppends {
		return fmt.Errorf("%w: batch has %d appends; limit is %d", ErrResourceLimit, len(appends), MaxBatchAppends)
	}
	bytes := 0
	for i, appendRecord := range appends {
		if err := validateRecord(appendRecord.Key, appendRecord.Value); err != nil {
			return fmt.Errorf("append %d: %w", i, err)
		}
		bytes += len(appendRecord.Key) + len(appendRecord.Value)
		if bytes > MaxBatchBytes {
			return fmt.Errorf("%w: batch exceeds %d bytes", ErrResourceLimit, MaxBatchBytes)
		}
	}
	return nil
}

func checkSnapshotSize(records, bytes int) error {
	if records > MaxReadRecords {
		return fmt.Errorf("%w: snapshot has more than %d records", ErrResourceLimit, MaxReadRecords)
	}
	if bytes > MaxReadBytes {
		return fmt.Errorf("%w: snapshot exceeds %d bytes", ErrResourceLimit, MaxReadBytes)
	}
	return nil
}
