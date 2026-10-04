package deposition

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadEnactmentsReplacesSources(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{"first.trial": "first", "second.trial": "second"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d := &Deposition{Enacts: []string{"first.trial", "second.trial"}}
	for range 2 {
		if err := LoadEnactments(d, dir); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.EnactSources, []string{"first", "second"}) {
			t.Fatalf("loaded sources = %q", d.EnactSources)
		}
	}
	d.Enacts = nil
	if err := LoadEnactments(d, dir); err != nil {
		t.Fatal(err)
	}
	if len(d.EnactSources) != 0 {
		t.Fatalf("cleared dependency list retained %q", d.EnactSources)
	}
}

func TestLoadEnactmentsFailureDoesNotPartiallyReplaceSources(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "first.trial"), []byte("new first"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &Deposition{
		Enacts:       []string{"first.trial", "missing.trial"},
		EnactSources: []string{"old first", "old second"},
	}
	if err := LoadEnactments(d, dir); err == nil {
		t.Fatal("missing dependency was accepted")
	}
	if !reflect.DeepEqual(d.EnactSources, []string{"old first", "old second"}) {
		t.Fatalf("failed load changed sources: %q", d.EnactSources)
	}
	if err := os.WriteFile(filepath.Join(dir, "missing.trial"), []byte("new second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnactments(d, dir); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.EnactSources, []string{"new first", "new second"}) {
		t.Fatalf("retry sources = %q", d.EnactSources)
	}
}

func TestRejectedFilingCannotIgnoreRuntimeExpectations(t *testing.T) {
	for _, expectation := range []string{`EXPECT PROCLAMATION: "never".`, `EXPECT RECORD value: 42.`} {
		if _, err := Parse("DEPOSITION OF: invalid.trial.\nEXPECT REJECTION.\n" + expectation); err == nil {
			t.Fatalf("rejection accepted a runtime assertion: %s", expectation)
		}
	}
	d := &Deposition{Outcome: "rejection", AllowDays: 1, Proclamations: []string{"never"}}
	result := Run(t.Context(), "not a program", d)
	if result.OK() || !strings.Contains(strings.Join(result.Contradictions, " "), "EXPECT REJECTION") {
		t.Fatalf("runtime assertion was ignored: %+v", result)
	}
}
