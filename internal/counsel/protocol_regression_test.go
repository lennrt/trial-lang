package counsel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func serveRawMessages(t *testing.T, messages ...string) []map[string]json.RawMessage {
	t.Helper()
	var in, out bytes.Buffer
	for _, message := range messages {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(message), message)
	}
	s := &Server{In: &in, Out: &out}
	if err := s.Serve(t.Context()); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(&out)
	var replies []map[string]json.RawMessage
	for {
		body, err := readMessage(reader)
		if errors.Is(err, io.EOF) {
			return replies
		}
		if err != nil {
			t.Fatal(err)
		}
		var reply map[string]json.RawMessage
		if err := json.Unmarshal(body, &reply); err != nil {
			t.Fatal(err)
		}
		replies = append(replies, reply)
	}
}

func TestServeDistinguishesInvalidRequestsFromParseErrors(t *testing.T) {
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"jsonrpc":`, -32700},
		{`{} {}`, -32700},
		{`null`, -32600},
		{`[]`, -32600},
		{`true`, -32600},
		{`{}`, -32600},
		{`{"id":1,"method":"initialize"}`, -32600},
		{`{"jsonrpc":"1.0","id":1,"method":"initialize"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":42}`, -32600},
		{`{"jsonrpc":"2.0","id":true,"method":"initialize"}`, -32600},
		{`{"jsonrpc":"2.0","id":{},"method":"initialize"}`, -32600},
		{`{"jsonrpc":"2.0","id":[],"method":"initialize"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":true}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"shutdown","params":42}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"unknown","params":"text"}`, -32600},
	} {
		t.Run(tc.body, func(t *testing.T) {
			replies := serveRawMessages(t, tc.body, `{"jsonrpc":"2.0","id":2,"method":"shutdown"}`)
			if len(replies) != 2 {
				t.Fatalf("received %d replies, want rejection and subsequent shutdown", len(replies))
			}
			var rpcErr rpcError
			if err := json.Unmarshal(replies[0]["error"], &rpcErr); err != nil {
				t.Fatalf("missing error: %s", replies[0])
			}
			if rpcErr.Code != tc.code || string(replies[0]["id"]) != "null" {
				t.Fatalf("rejection = %v, want code %d with null ID", replies[0], tc.code)
			}
			if string(replies[1]["id"]) != "2" || string(replies[1]["result"]) != "null" {
				t.Fatalf("server did not continue after malformed request: %v", replies[1])
			}
		})
	}
}

func TestServePreservesScalarIDsAndNotificationSilence(t *testing.T) {
	for _, id := range []string{`0`, `-1`, `"request-1"`, `null`} {
		replies := serveRawMessages(t,
			`{"jsonrpc":"2.0","method":"initialize","params":{}}`,
			`{"jsonrpc":"2.0","method":"unknown-notification"}`,
			`{"jsonrpc":"2.0","id":`+id+`,"method":"shutdown","params":null}`,
		)
		if len(replies) != 1 || string(replies[0]["id"]) != id || string(replies[0]["result"]) != "null" {
			t.Fatalf("ID %s: replies = %v, want one shutdown response with the original ID", id, replies)
		}
	}
}

func TestDocumentNotificationsRequireText(t *testing.T) {
	const uri = "file:///retained.trial"
	const original = "FORM K-1. IN THE MATTER OF: retained. ARTICLE 1. PROCLAIM 1."
	for _, method := range []string{"textDocument/didOpen", "textDocument/didChange"} {
		for _, missing := range []string{`{}`, `{"text":null}`} {
			t.Run(method+missing, func(t *testing.T) {
				var out bytes.Buffer
				s := &Server{Out: &out, docs: map[string]string{uri: original}}
				params := `{"textDocument":{"uri":"` + uri + `"},"contentChanges":[` + missing + `]}`
				if method == "textDocument/didOpen" {
					fields := strings.TrimPrefix(missing, "{")
					if fields != "}" {
						fields = "," + fields
					}
					params = `{"textDocument":{"uri":"` + uri + `"` + fields + `}`
				}
				if err := s.handle(&rpcRequest{Method: method, Params: json.RawMessage(params)}); err != nil {
					t.Fatal(err)
				}
				if got := s.docs[uri]; got != original {
					t.Fatalf("malformed %s replaced document with %q", method, got)
				}
			})
		}
	}
	// An explicitly empty string is a legitimate full replacement.
	var out bytes.Buffer
	s := &Server{Out: &out, docs: map[string]string{uri: original}}
	if err := s.handle(&rpcRequest{Method: "textDocument/didChange", Params: json.RawMessage(`{"textDocument":{"uri":"` + uri + `"},"contentChanges":[{"text":""}]}`)}); err != nil {
		t.Fatal(err)
	}
	if s.docs[uri] != "" {
		t.Fatal("an explicitly empty document was not applied")
	}
}

func TestLSPColumnsClampBeforeLineEnding(t *testing.T) {
	for _, line := range []string{"PROCLAIM", "PROCLAIM\r\n", "😀PROCLAIM\r\n"} {
		if got := wordAt(line, position{Character: 100}); got != "PROCLAIM" {
			t.Errorf("hover past end of %q = %q, want PROCLAIM", line, got)
		}
	}
	for _, line := range []string{"PROCLAIM\r\n", "PROCLAIM\r"} {
		start, end := diagnosticRange(line, 1, 100)
		if start.Character != 8 || end != start {
			t.Errorf("diagnostic at end of %q = %+v..%+v, want 8..8", line, start, end)
		}
	}
}

// Exercise complete framed sessions, not just JSON decoding: a malformed
// request must leave the connection usable, and every emitted message must
// remain a complete JSON-RPC response or notification.
func FuzzCounselEnvelope(f *testing.F) {
	for _, body := range []string{
		``, `null`, `[]`, `{"jsonrpc":`,
		`{"jsonrpc":"2.0","id":{},"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":true}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///fuzz.trial","text":"FORM K-1. IN THE MATTER OF: fuzz. ARTICLE 1. PROCLAIM 1."}}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"file:///fuzz.trial"},"contentChanges":[{"text":null}]}}`,
		`{"jsonrpc":"2.0","id":"hover","method":"textDocument/hover","params":{"textDocument":{"uri":"file:///fuzz.trial"},"position":{"line":0,"character":2147483647}}}`,
		`{"jsonrpc":"2.0","id":null,"method":"textDocument/completion","params":{}}`,
	} {
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body string) {
		// Keep mutations cheap enough for sustained protocol fuzzing while
		// exercising the compiler through document notifications as well.
		if len(body) > 16<<10 {
			t.Skip()
		}
		replies := serveRawMessages(t, body, `{"jsonrpc":"2.0","id":"after-fuzz","method":"shutdown"}`)
		for _, reply := range replies {
			if string(reply["jsonrpc"]) != `"2.0"` {
				t.Fatalf("invalid response version: %v", reply)
			}
			_, method := reply["method"]
			_, id := reply["id"]
			_, result := reply["result"]
			_, rpcErr := reply["error"]
			if method && (id || result || rpcErr) || !method && (!id || result == rpcErr) {
				t.Fatalf("invalid response or notification shape: %v", reply)
			}
		}
		if !json.Valid([]byte(body)) {
			if len(replies) != 2 || string(replies[1]["id"]) != `"after-fuzz"` {
				t.Fatalf("invalid JSON disrupted the next request: %v", replies)
			}
			var rpcErr rpcError
			if err := json.Unmarshal(replies[0]["error"], &rpcErr); err != nil || rpcErr.Code != -32700 {
				t.Fatalf("invalid JSON did not produce a parse error: %v", replies[0])
			}
		}
	})
}
