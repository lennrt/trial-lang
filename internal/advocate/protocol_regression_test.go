package advocate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func serveLines(t *testing.T, log docket.Log, lines ...string) []rpcResponse {
	t.Helper()
	var out bytes.Buffer
	s := &Server{Log: log, In: strings.NewReader(strings.Join(lines, "\n") + "\n"), Out: &out}
	if err := s.Serve(t.Context()); err != nil {
		t.Fatal(err)
	}
	var replies []rpcResponse
	dec := json.NewDecoder(&out)
	for {
		var reply rpcResponse
		if err := dec.Decode(&reply); errors.Is(err, io.EOF) {
			return replies
		} else if err != nil {
			t.Fatal(err)
		}
		replies = append(replies, reply)
	}
}

func TestInvalidEnvelopeCannotFileCase(t *testing.T) {
	const params = `"method":"tools/call","params":{"name":"trial_file","arguments":{"source":"FORM K-1. IN THE MATTER OF: invalid-envelope. ARTICLE 1. ADJOURN INDEFINITELY."}}`
	for _, fields := range []string{
		`"id":1`, `"jsonrpc":"1.0","id":1`,
		`"jsonrpc":"2.0","id":true`, `"jsonrpc":"2.0","id":{}`,
		`"jsonrpc":"2.0","id":[]`, `"jsonrpc":"2.0","id":null`,
		`"jsonrpc":"2.0","id":1.5`,
		`"jsonrpc":"2.0","id":1e-1000000000`,
		`"jsonrpc":"2.0","id":1e-9999999999999999999999999999`,
	} {
		t.Run(fields, func(t *testing.T) {
			log := docket.NewMemoryLog()
			t.Cleanup(log.Close)
			replies := serveLines(t, log, "{"+fields+","+params+"}", `{"jsonrpc":"2.0","id":"after","method":"ping"}`)
			cases, err := log.ListCases(t.Context())
			if err != nil || len(cases) != 0 {
				t.Fatalf("invalid envelope created cases %v: %v", cases, err)
			}
			if len(replies) != 2 || replies[0].Error == nil || replies[0].Error.Code != -32600 || string(replies[0].ID) != "null" || string(replies[1].ID) != `"after"` {
				t.Fatalf("invalid envelope or follow-up response: %+v", replies)
			}
		})
	}
}

func TestToolNotificationCannotFileCase(t *testing.T) {
	log := docket.NewMemoryLog()
	t.Cleanup(log.Close)
	replies := serveLines(t, log,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"trial_file","arguments":{"source":"FORM K-1. IN THE MATTER OF: unacknowledged. ARTICLE 1. ADJOURN INDEFINITELY."}}}`,
		`{"jsonrpc":"2.0","id":"after","method":"ping"}`,
	)
	cases, err := log.ListCases(t.Context())
	if err != nil || len(cases) != 0 || len(replies) != 1 || string(replies[0].ID) != `"after"` {
		t.Fatalf("tool notification: cases %v, replies %+v, error %v", cases, replies, err)
	}
}

func TestToolNotificationCannotServeInput(t *testing.T) {
	log := docket.NewMemoryLog()
	t.Cleanup(log.Close)
	c := docket.Case{ID: "case-000000000000000000000009"}
	if err := log.EnsureTopic(t.Context(), c.Summons()); err != nil {
		t.Fatal(err)
	}
	replies := serveLines(t, log,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"trial_serve","arguments":{"case":"`+c.ID+`","values":["unacknowledged"]}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"trial_serve","arguments":{"case":"`+c.ID+`","values":["acknowledged"]}}}`,
	)
	records, err := log.ReadAll(t.Context(), c.Summons())
	if err != nil || len(records) != 1 || string(records[0].Value) != "acknowledged" || len(replies) != 1 || string(replies[0].ID) != "1" {
		t.Fatalf("tool notification: records %v, replies %+v, error %v", records, replies, err)
	}
}

func TestInitializeOnlyAdvertisesSupportedVersion(t *testing.T) {
	for _, requested := range []string{"2025-06-18", "2099-99-99", ""} {
		replies := serveLines(t, nil, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q}}`, requested))
		if len(replies) != 1 || replies[0].Error != nil {
			t.Fatalf("initialize %q: %+v", requested, replies)
		}
		if got := replies[0].Result.(map[string]any)["protocolVersion"]; got != "2025-06-18" {
			t.Fatalf("offered %q; server claimed unsupported version %q", requested, got)
		}
	}
}

func TestAdvocateClassifiesMalformedMessages(t *testing.T) {
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"jsonrpc":`, -32700}, {`{} {}`, -32700},
		{`null`, -32600}, {`[]`, -32600}, {`true`, -32600}, {`{}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":42}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"ping","params":true}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"ping","params":null}`, -32600},
	} {
		t.Run(tc.body, func(t *testing.T) {
			replies := serveLines(t, nil, tc.body, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
			if len(replies) != 2 || replies[0].Error == nil || replies[0].Error.Code != tc.code {
				t.Fatalf("replies = %+v, want error %d then ping", replies, tc.code)
			}
		})
	}
}

func TestAdvocatePreservesIntegerIDsAndNotificationSilence(t *testing.T) {
	for _, id := range []string{`"request"`, `9007199254740993`, `-1`, `1.0`, `1e3`, `1000e-3`, `10.0e-1`, `1000.000e-3`, `0e-1000000000`, `1e1000000000`, `1e9999999999999999999999999999`} {
		replies := serveLines(t, nil, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, `{"jsonrpc":"2.0","id":`+id+`,"method":"ping"}`)
		if len(replies) != 1 || string(replies[0].ID) != id || replies[0].Error != nil {
			t.Fatalf("ID %s: replies %+v", id, replies)
		}
	}
}

func FuzzMCPIntegerID(f *testing.F) {
	for _, seed := range []struct {
		mantissa int64
		exponent int16
		fraction uint16
	}{
		{0, 0, 0}, {1, -1, 0}, {-1000, -3, 0},
		{120, 0, 1200}, {120, -2, 1200}, {9223372036854775807, 0, 9999},
	} {
		f.Add(seed.mantissa, seed.exponent, seed.fraction)
	}
	f.Fuzz(func(t *testing.T, mantissa int64, exponent int16, fraction uint16) {
		// Keep the independent big.Rat oracle small. Extreme exponents are
		// separate regressions and must never be expanded by the implementation.
		exponent %= 100
		text := strconv.FormatInt(mantissa, 10)
		text += fmt.Sprintf(".%04d", fraction%10000)
		text += "e" + strconv.Itoa(int(exponent))
		rational, ok := new(big.Rat).SetString(text)
		if !ok {
			t.Fatalf("bad oracle input %q", text)
		}
		if got := validRequestID(json.RawMessage(text)); got != rational.IsInt() {
			t.Fatalf("integer ID %s = %t, want %t", text, got, rational.IsInt())
		}
	})
}

type shortOutput struct{ calls int }

func (w *shortOutput) Write(p []byte) (int, error) { w.calls++; return len(p) - 1, nil }

func TestAdvocateStopsOnShortOutput(t *testing.T) {
	out := &shortOutput{}
	s := &Server{In: strings.NewReader(strings.Repeat(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n", 2)), Out: out}
	if err := s.Serve(t.Context()); !errors.Is(err, io.ErrShortWrite) || out.calls != 1 {
		t.Fatalf("short output: error %v, writes %d", err, out.calls)
	}
}

type countedInput struct{ reads int }

func (r *countedInput) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }

func TestAdvocateChecksCancellationBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	in := &countedInput{}
	s := &Server{In: in, Out: io.Discard}
	if err := s.Serve(ctx); !errors.Is(err, context.Canceled) || in.reads != 0 {
		t.Fatalf("canceled server: error %v, reads %d", err, in.reads)
	}
}

func TestAdvocateRequestSizeBoundary(t *testing.T) {
	const ping = `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	for _, ending := range []string{"", "\n", "\r\n"} {
		t.Run(fmt.Sprintf("ending-%q", ending), func(t *testing.T) {
			var out bytes.Buffer
			input := ping + strings.Repeat(" ", maxRequestBytes-len(ping)) + ending
			s := &Server{In: strings.NewReader(input), Out: &out}
			if err := s.Serve(t.Context()); err != nil || out.Len() == 0 {
				t.Fatalf("exact-limit request: error %v, output %q", err, out.String())
			}
		})
	}
	var out bytes.Buffer
	s := &Server{In: strings.NewReader(ping + strings.Repeat(" ", maxRequestBytes+1-len(ping)) + "\n"), Out: &out}
	if err := s.Serve(t.Context()); err == nil || out.Len() != 0 {
		t.Fatalf("oversized request: error %v, output %q", err, out.String())
	}
}

func TestToolArgumentsRequireDeclaredFields(t *testing.T) {
	s := &Server{Log: docket.NewMemoryLog()}
	for _, args := range []string{
		`{"deposition_source":"DEPOSITION OF: missing.trial.\nEXPECT REJECTION."}`,
		`{"program_source":null,"deposition_source":"DEPOSITION OF: missing.trial.\nEXPECT REJECTION."}`,
	} {
		if result := s.call(t.Context(), "trial_test", json.RawMessage(args)); !result.IsError {
			t.Fatalf("missing program passed a deposition: %+v", result)
		}
	}
	// Explicitly empty source is still source and can legitimately be rejected.
	args := `{"program_source":"","deposition_source":"DEPOSITION OF: empty.trial.\nEXPECT REJECTION."}`
	if result := s.call(t.Context(), "trial_test", json.RawMessage(args)); result.IsError || !strings.Contains(result.Content[0].Text, `"consistent": true`) {
		t.Fatalf("explicit empty source was refused: %+v", result)
	}
	for _, args := range []string{`{"limit":0}`, `{"limit":null}`, `{"from_offset":null}`} {
		if result := s.call(t.Context(), "trial_docket", json.RawMessage(args)); !result.IsError {
			t.Fatalf("invalid page arguments were accepted: %s", args)
		}
	}
}
