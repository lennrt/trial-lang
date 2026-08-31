package deposition

// Every deposition shipped in examples/ is heard here, so trial test
// and go test agree about what the witnesses will say.

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
				t.Fatalf("the deposition would not parse: %v", err)
			}
			if err := LoadEnactments(dep, filepath.Dir(path)); err != nil {
				t.Fatalf("the statutes could not be located: %v", err)
			}
			prog, err := os.ReadFile(filepath.Join(filepath.Dir(path), dep.Program))
			if err != nil {
				t.Fatalf("the deposed could not be located: %v", err)
			}
			res := Run(context.Background(), string(prog), dep)
			if !res.OK() {
				t.Fatalf("the testimony contradicts the record:\n  %v", res.Contradictions)
			}
		})
	}
}
