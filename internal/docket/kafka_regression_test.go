package docket

import (
	"math"
	"testing"
)

func TestProceedingCacheWindowRejectsInvalidAddresses(t *testing.T) {
	for _, start := range []int64{-1, 4, math.MaxInt32 + 1, math.MaxInt64} {
		log := &KafkaLog{}
		if err := log.cacheProceedingWindow("test", make([]Record, 3), start); err == nil {
			t.Errorf("cache accepted address %d outside its snapshot", start)
		}
		if len(log.proceedings) != 0 {
			t.Fatal("invalid address changed the cache")
		}
	}
}

func TestProceedingCacheWindowPreservesLogicalAndPhysicalOffsets(t *testing.T) {
	records := make([]Record, maxProceedingCacheWindow+3)
	for i := range records {
		records[i].Offset = int64(3*i + 7)
	}
	for _, start := range []int64{0, 1, int64(len(records) - 1), int64(len(records))} {
		log := &KafkaLog{}
		if err := log.cacheProceedingWindow("test", records, start); err != nil {
			t.Fatal(err)
		}
		for i, record := range records {
			physical, cached := log.cachedProceeding("test", int64(i))
			wantCached := int64(i) >= start && int64(i)-start < maxProceedingCacheWindow
			if cached != wantCached || (cached && physical != record.Offset) {
				t.Errorf("start=%d address=%d: offset=%d cached=%t; want offset=%d cached=%t", start, i, physical, cached, record.Offset, wantCached)
			}
		}
	}
}
