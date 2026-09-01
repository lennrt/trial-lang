package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestSphereFrames(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, frameCount)
	for frame := range frameCount {
		got := renderFrame(frame)
		lines := strings.Split(got, "\n")
		if len(lines) != 31 {
			t.Fatalf("frame %d has %d lines; want 31", frame+1, len(lines))
		}
		for line, text := range lines {
			if len(text) != panelWidth+2 {
				t.Fatalf("frame %d line %d has width %d; want %d", frame+1, line+1, len(text), panelWidth+2)
			}
		}
		if !strings.Contains(got, "TRANSACTIONAL SPHERE") {
			t.Fatalf("frame %d has no sphere title", frame+1)
		}
		if !strings.Contains(got, fmt.Sprintf("FRAME %02d/%d", frame+1, frameCount)) {
			t.Fatalf("frame %d has the wrong frame counter", frame+1)
		}
		if _, ok := seen[got]; ok {
			t.Fatalf("frame %d duplicates an earlier frame", frame+1)
		}
		seen[got] = struct{}{}
	}
}

func TestSphereIsSolidAndShaded(t *testing.T) {
	t.Parallel()

	for frame := range frameCount {
		lines := renderSphere(frame)
		if len(lines) != objectHeight {
			t.Fatalf("frame %d has %d object lines; want %d", frame+1, len(lines), objectHeight)
		}
		pixels := strings.Join(lines, "")
		shaded := 0
		levels := make(map[rune]struct{})
		for _, character := range pixels {
			if strings.ContainsRune(".,-~:;=!*#$@", character) {
				shaded++
				levels[character] = struct{}{}
			}
		}
		if shaded < 250 {
			t.Fatalf("frame %d does not contain enough shaded surface pixels", frame+1)
		}
		if len(levels) < 6 {
			t.Fatalf("frame %d uses only %d shade levels", frame+1, len(levels))
		}
	}
}
