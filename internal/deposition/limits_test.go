package deposition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestDepositionParseListLimits(t *testing.T) {
	for _, tc := range []struct {
		name, statement string
		limit           int
		count           func(*Deposition) int
	}{
		{"enactments", "ENACT: statute.trial.\n", maxEnactments, func(d *Deposition) int { return len(d.Enacts) }},
		{"summonses", "SERVE: 1.\n", maxDepositionItems, func(d *Deposition) int { return len(d.Serves) }},
		{"proclamations", "EXPECT PROCLAMATION: 1.\n", maxDepositionItems, func(d *Deposition) int { return len(d.Proclamations) }},
		{"records", "EXPECT RECORD n: 1.\n", maxDepositionItems, func(d *Deposition) int { return len(d.Records) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "DEPOSITION OF: sample.trial.\n" + strings.Repeat(tc.statement, tc.limit)
			d, err := Parse(source)
			if err != nil || d == nil || tc.count(d) != tc.limit {
				t.Fatalf("exact limit: deposition=%+v, error=%v", d, err)
			}
			if d, err := Parse(source + tc.statement); err == nil || d != nil {
				t.Fatal("accepted one item beyond the limit")
			}
		})
	}
}

func TestDepositionParseByteLimit(t *testing.T) {
	const header = "DEPOSITION OF: sample.trial.\nOFF THE RECORD: "
	source := header + strings.Repeat("x", maxDepositionBytes-len(header))
	if _, err := Parse(source); err != nil {
		t.Fatalf("exact byte limit was rejected: %v", err)
	}
	if _, err := Parse(source + "x"); err == nil {
		t.Fatal("accepted one byte beyond the limit, even in a comment")
	}
}

func TestLoadEnactmentsByteLimitPreservesPriorSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statute.trial")
	contents := strings.Repeat("a", maxDepositionBytes)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &Deposition{Enacts: []string{"statute.trial"}, EnactSources: []string{"previous source"}}
	if err := LoadEnactments(d, dir); err != nil {
		t.Fatalf("exact byte limit was rejected: %v", err)
	}
	if len(d.EnactSources) != 1 || d.EnactSources[0] != contents {
		t.Fatal("loader did not preserve the entire source at the byte limit")
	}
	if err := os.WriteFile(path, []byte(contents+"b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnactments(d, dir); err == nil {
		t.Fatal("accepted a statute one byte beyond the limit")
	}
	if len(d.EnactSources) != 1 || d.EnactSources[0] != contents {
		t.Fatal("oversized replacement changed the previously loaded sources")
	}
}

func TestEnactmentAggregateSourceLimit(t *testing.T) {
	// Reuse one string: this checks aggregate accounting without allocating
	// hundreds of MiB or reading that much data from the test filesystem.
	source := strings.Repeat("a", maxDepositionBytes)
	d := &Deposition{AllowDays: 1}
	for range docket.MaxReadBytes / maxDepositionBytes {
		d.EnactSources = append(d.EnactSources, source)
	}
	if err := validateRunInputs("", d); err != nil {
		t.Fatalf("exact aggregate byte limit was rejected: %v", err)
	}
	d.EnactSources = append(d.EnactSources, "a")
	if err := validateRunInputs("", d); err == nil {
		t.Fatal("accepted one byte beyond the aggregate source limit")
	}
}
