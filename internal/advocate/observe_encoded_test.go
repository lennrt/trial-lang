package advocate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestObserveEncodedPrefix(t *testing.T) {
	for _, character := range []string{"<", "\"", "\\", "\x00"} {
		t.Run(fmt.Sprintf("%q", character), func(t *testing.T) {
			log := docket.NewMemoryLog()
			defer log.Close()
			c := docket.Case{ID: "case-000000000000000000000003"}
			if err := log.EnsureTopic(t.Context(), c.Proclamations()); err != nil {
				t.Fatal(err)
			}
			value := strings.Repeat(character, 350000)
			for range 2 {
				if _, err := log.Append(t.Context(), c.Proclamations(), nil, []byte(value)); err != nil {
					t.Fatal(err)
				}
			}
			srv := &Server{Log: log}
			offset := int64(0)
			count := 0
			for count < 2 {
				result := srv.call(t.Context(), "trial_observe", json.RawMessage(fmt.Sprintf(`{"case":%q,"from_offset":%d}`, c.ID, offset)))
				if result.IsError {
					t.Fatalf("observe lost fitting prefix: %s", result.Content[0].Text)
				}
				encoded, _ := json.Marshal(result)
				if len(encoded) > maxToolOutputBytes {
					t.Fatalf("encoded result exceeds bound: %d", len(encoded))
				}
				var payload struct {
					Proclamations []struct {
						Offset int64
						Text   string
					}
					Next int64 `json:"next_offset"`
				}
				if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload.Proclamations) == 0 {
					t.Fatal("empty page before end")
				}
				for _, r := range payload.Proclamations {
					if r.Offset != int64(count) || r.Text != value {
						t.Fatal("record changed or skipped")
					}
					count++
				}
				if payload.Next != int64(count) {
					t.Fatalf("next offset %d want %d", payload.Next, count)
				}
				offset = payload.Next
			}
		})
	}
}

func TestObserveSingleEncodedOversize(t *testing.T) {
	log := docket.NewMemoryLog()
	defer log.Close()
	c := docket.Case{ID: "case-000000000000000000000003"}
	if err := log.EnsureTopic(t.Context(), c.Proclamations()); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(t.Context(), c.Proclamations(), nil, []byte(strings.Repeat("<", 700000))); err != nil {
		t.Fatal(err)
	}
	result := (&Server{Log: log}).call(t.Context(), "trial_observe", json.RawMessage(fmt.Sprintf(`{"case":%q}`, c.ID)))
	if !result.IsError || !strings.Contains(result.Content[0].Text, "offset 0") {
		t.Fatalf("want explicit record error: %+v", result)
	}
}

func TestObserveEncodedInclusiveBoundary(t *testing.T) {
	c := docket.Case{ID: "case-000000000000000000000003"}
	// Measure the complete public tool result with an empty record. ASCII adds
	// exactly one encoded byte per character at both JSON layers.
	payload, err := json.MarshalIndent(map[string]any{
		"case": c.ID, "next_offset": int64(1),
		"proclamations": []map[string]any{{"offset": int64(0), "text": ""}},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(toolResult{Content: []toolContent{{Type: "text", Text: string(payload)}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			log := docket.NewMemoryLog()
			defer log.Close()
			if err := log.EnsureTopic(t.Context(), c.Proclamations()); err != nil {
				t.Fatal(err)
			}
			value := strings.Repeat("a", maxToolOutputBytes-len(envelope)+extra)
			if _, err := log.Append(t.Context(), c.Proclamations(), nil, []byte(value)); err != nil {
				t.Fatal(err)
			}
			result := (&Server{Log: log}).call(t.Context(), "trial_observe", json.RawMessage(fmt.Sprintf(`{"case":%q}`, c.ID)))
			if extra == 1 {
				if !result.IsError || !strings.Contains(result.Content[0].Text, "offset 0") {
					t.Fatalf("over-limit record accepted: %+v", result.IsError)
				}
				return
			}
			if result.IsError {
				t.Fatalf("exact-limit record rejected: %s", result.Content[0].Text)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if len(encoded) != maxToolOutputBytes {
				t.Fatalf("encoded bytes %d want exact limit %d", len(encoded), maxToolOutputBytes)
			}
		})
	}
}
