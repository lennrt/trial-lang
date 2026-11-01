package docket

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func kafkaRecordTest(t *testing.T, batchLimit int) (context.Context, *KafkaLog, Case) {
	t.Helper()
	broker := os.Getenv("TRIAL_E2E_BROKER")
	if broker == "" {
		t.Skip("TRIAL_E2E_BROKER is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	log, err := OpenKafkaLog(ctx, broker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	c, err := NewCase()
	if err != nil {
		t.Fatal(err)
	}
	topics := []string{c.Proclamations(), c.AttentionTopic(), c.Proceedings(), c.Summons()}
	responses, err := log.adm.CreateTopics(ctx, 1, 1, map[string]*string{
		"max.message.bytes": new(strconv.Itoa(batchLimit)),
	}, topics...)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := log.adm.DeleteTopics(cleanup, topics...); err != nil {
			t.Errorf("delete record-bound topics: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, response := range responses {
		if response.Err != nil {
			t.Fatal(response.Err)
		}
	}
	return ctx, log, c
}

func TestE2E_LargeRecords(t *testing.T) {
	ctx, log, c := kafkaRecordTest(t, maxKafkaRecordBatchBytes)
	topic := c.Proclamations()
	value := kafkaRecordPayload((2<<20)+1, 8)
	if _, err := log.Append(ctx, topic, []byte("append"), value); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := log.AppendBatch(ctx, []StepAppend{{Topic: topic, Key: []byte("batch"), Value: value}}); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	if err := log.Commit(ctx, c, Step{Appends: []StepAppend{{Topic: topic, Key: []byte("commit"), Value: value}}}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	records, err := log.ReadAll(ctx, topic)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("read %d records, want 3", len(records))
	}
	for i, key := range []string{"append", "batch", "commit"} {
		if string(records[i].Key) != key || !bytes.Equal(records[i].Value, value) {
			t.Fatalf("record %d did not roundtrip", i)
		}
	}
	att, err := log.Attention(ctx, c)
	if err != nil || !att.Started {
		t.Fatalf("committed attention: %+v, %v", att, err)
	}
}

func TestE2E_SmallRecordBatches(t *testing.T) {
	// Each record fits this ordinary topic; the two-record aggregate does not.
	// Keep the topic limit explicit so broker-wide overrides cannot mask a
	// regression in client-side batching.
	ctx, log, c := kafkaRecordTest(t, 1_048_588)
	topic := c.Proclamations()
	value := kafkaRecordPayload(600_000, 9)
	if _, err := log.AppendBatch(ctx, []StepAppend{
		{Topic: topic, Key: []byte("batch-a"), Value: value},
		{Topic: topic, Key: []byte("batch-b"), Value: value},
	}); err != nil {
		t.Fatalf("AppendBatch of small records: %v", err)
	}
	if err := log.Commit(ctx, c, Step{Appends: []StepAppend{
		{Topic: topic, Key: []byte("commit-a"), Value: value},
		{Topic: topic, Key: []byte("commit-b"), Value: value},
	}}); err != nil {
		t.Fatalf("Commit of small records: %v", err)
	}
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"append-a", "append-b"} {
		workers.Go(func() {
			_, err := log.Append(ctx, topic, []byte(key), value)
			results <- err
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent Append: %v", err)
		}
	}

	// Failure after an earlier chunk was accepted must abort the entire batch
	// or step, including its attention note.
	invalid := []StepAppend{
		{Topic: topic, Key: []byte("abort-prefix"), Value: value},
		{Topic: topic, Key: []byte("too-large"), Value: kafkaRecordPayload(1_100_000, 10)},
	}
	if _, err := log.AppendBatch(ctx, invalid); err == nil {
		t.Fatal("AppendBatch accepted an individually oversized record")
	}
	if err := log.Commit(ctx, c, Step{Appends: invalid, Ledger: 1}); err == nil {
		t.Fatal("Commit accepted an individually oversized record")
	}
	records, err := log.ReadAll(ctx, topic)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 6 {
		t.Fatalf("read %d records, want six committed records with no aborted prefix", len(records))
	}
	want := map[string]bool{"batch-a": true, "batch-b": true, "commit-a": true, "commit-b": true, "append-a": true, "append-b": true}
	for _, record := range records {
		key := string(record.Key)
		if !want[key] || !bytes.Equal(record.Value, value) {
			t.Fatalf("unexpected or damaged record at offset %d", record.Offset)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing keys: %v", want)
	}
	att, err := log.Attention(ctx, c)
	if err != nil || !att.Started || att.Ledger != 0 {
		t.Fatalf("attention changed with aborted step: %+v, %v", att, err)
	}
}
