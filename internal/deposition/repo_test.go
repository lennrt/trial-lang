package deposition

// Repository depositions must pass through the same runner as trial test.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepoDepositions(t *testing.T) {
	var matches []string
	for _, dir := range []string{"../../examples", "../../canon"} {
		found, err := filepath.Glob(filepath.Join(dir, "*.deposition"))
		if err != nil {
			t.Fatal(err)
		}
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		t.Fatal("no depositions found in examples/ or canon/")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			dep, err := Parse(string(src))
			if err != nil {
				t.Fatalf("parse deposition: %v", err)
			}
			if err := LoadEnactments(dep, filepath.Dir(path)); err != nil {
				t.Fatalf("load statutes: %v", err)
			}
			prog, err := os.ReadFile(filepath.Join(filepath.Dir(path), dep.Program))
			if err != nil {
				t.Fatalf("read program: %v", err)
			}
			res := Run(context.Background(), string(prog), dep)
			if !res.OK() {
				t.Fatalf("deposition failed:\n  %v", res.Contradictions)
			}
		})
	}
}
