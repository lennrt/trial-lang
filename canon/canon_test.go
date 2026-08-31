package canon_test

import (
	"testing"

	"github.com/lennrt/trial-lang/canon"
)

func TestFilesReturnsCallerOwnedSlice(t *testing.T) {
	first := canon.Files()
	first[0] = "changed"
	second := canon.Files()
	if second[0] == "changed" {
		t.Fatal("Files retained a caller-owned slice")
	}
}
