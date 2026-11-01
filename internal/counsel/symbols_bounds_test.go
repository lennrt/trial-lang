package counsel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/lennrt/trial-lang/internal/gregor"
)

func outlineArticles(count int, separator string) string {
	var source strings.Builder
	source.WriteString("FORM K-1. IN THE MATTER OF: outline. ")
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&source, "ARTICLE %d.%s", i, separator)
	}
	return source.String()
}

func TestDocumentSymbolsOversizeKeepsSession(t *testing.T) {
	const uri = "untitled:large-outline"
	const id = `outline-<"large">`
	input := frame(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "text": outlineArticles(90000, "\n")}}}) +
		frame(map[string]any{"jsonrpc": "2.0", "id": id, "method": "textDocument/documentSymbol", "params": map[string]any{"textDocument": map[string]any{"uri": uri}}}) +
		frame(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"})
	var output bytes.Buffer
	if err := (&Server{In: strings.NewReader(input), Out: &output}).Serve(t.Context()); err != nil {
		t.Fatalf("oversized outline closed the connection: %v", err)
	}
	reader := bufio.NewReader(&output)
	var foundError, foundShutdown bool
	for {
		body, err := readMessage(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		var responseID string
		if json.Unmarshal(response.ID, &responseID) == nil && responseID == id {
			foundError = response.Error != nil && response.Error.Code == -32603 && strings.Contains(response.Error.Message, "response limit") && len(response.Result) == 0
		}
		if string(response.ID) == "2" {
			foundShutdown = response.Error == nil && string(response.Result) == "null"
		}
	}
	if !foundError || !foundShutdown {
		t.Fatalf("outline error=%t subsequent shutdown=%t", foundError, foundShutdown)
	}
}

func BenchmarkDocumentSymbolsCompact(b *testing.B) {
	for _, count := range []int{4000, 8000, 16000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			source := outlineArticles(count, " ")
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if symbols, err := documentSymbols(b.Context(), source, maxMessageBytes); err != nil || len(symbols) != count {
					b.Fatalf("symbols=%d want %d, err=%v", len(symbols), count, err)
				}
			}
		})
	}
}

func TestSourcePositionIndex(t *testing.T) {
	// A long first line with many requested positions must be scanned once;
	// the index stores corrections for its four multibyte runes, not prefixes
	// for every declaration. Later lines must discard earlier corrections.
	source := "😀é" + strings.Repeat(" ARTICLE 1.", 32000) + "界\r\n\t😀 ARTICLE 2.\r\n"
	index := indexSourcePositions(source)
	if len(index.adjustments) != 4 || len(index.lines) != 3 {
		t.Fatalf("index has %d corrections and %d lines", len(index.adjustments), len(index.lines))
	}
	for line, text := range strings.Split(source, "\n") {
		units := 0
		for column, r := range text {
			if got := index.position(line+1, column+1); got != (position{Line: line, Character: units}) {
				t.Fatalf("byte position %d:%d = %+v, want character %d", line, column, got, units)
			}
			units += utf16.RuneLen(r)
		}
		if got := index.position(line+1, len(text)+1); got != (position{Line: line, Character: units}) {
			t.Fatalf("line end = %+v, want character %d", got, units)
		}
	}
	if got := index.end(source); got != (position{Line: 2}) {
		t.Fatalf("EOF = %+v", got)
	}
	if got := indexSourcePositions("😀\r").end("😀\r"); got != (position{Character: 2}) {
		t.Fatalf("trailing CR = %+v", got)
	}
}

func TestOutlineMinimumSize(t *testing.T) {
	encoded, err := json.Marshal(documentSymbol{})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != minimumDocumentSymbolBytes {
		t.Fatalf("minimum symbol encoding changed: got %d want %d", len(encoded), minimumDocumentSymbolBytes)
	}
	program, err := gregor.Parse("FORM K-1. IN THE MATTER OF: outline. ARTICLE 1.")
	if err != nil {
		t.Fatal(err)
	}
	minimum := minimumDocumentSymbolBytes + len("ARTICLE 1") + 2
	if !outlineCanFit(program, minimum) || outlineCanFit(program, minimum-1) {
		t.Fatal("minimum encoded-size boundary is not inclusive")
	}
}

func TestDocumentSymbolsCompleteResponseBoundary(t *testing.T) {
	const source = "FORM K-1. IN THE MATTER OF: outline. ARTICLE 1."
	symbols := symbolsForTest(t, source)
	emptyIDResponse, err := json.Marshal(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage(`""`), Result: symbols})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			// Padding only the ID makes the whole response exactly at or one
			// byte above the limit, while the symbol result stays tiny.
			id, err := json.Marshal(strings.Repeat("a", maxMessageBytes-len(emptyIDResponse)+extra))
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			server := &Server{Out: &output}
			if err := server.respondDocumentSymbols(&rpcRequest{ID: id}, source); err != nil {
				t.Fatal(err)
			}
			body, err := readMessage(bufio.NewReader(&output))
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				ID     json.RawMessage `json:"id"`
				Error  *rpcError       `json:"error"`
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(response.ID, id) {
				t.Fatal("response ID changed")
			}
			if extra == 0 && (len(body) != maxMessageBytes || response.Error != nil || len(response.Result) == 0) {
				t.Fatalf("exact-size response: bytes=%d error=%v", len(body), response.Error)
			}
			if extra == 1 && (response.Error == nil || response.Error.Code != -32603 || len(response.Result) != 0) {
				t.Fatal("oversized complete response did not become a request error")
			}
		})
	}
}

func TestDocumentSymbolsPreservesTransportFailures(t *testing.T) {
	for _, source := range []string{outlineArticles(1, " "), outlineArticles(90000, "\n")} {
		for _, failAt := range []int{1, 2} {
			cause := errors.New("transport failed")
			writer := &framingShortWriter{failAt: failAt, err: cause}
			server := &Server{Out: writer}
			if err := server.respondDocumentSymbols(&rpcRequest{ID: json.RawMessage(`1`)}, source); !errors.Is(err, cause) {
				t.Fatalf("transport error = %v", err)
			}
			if writer.calls != failAt {
				t.Fatalf("continued writing after transport failure: calls=%d want=%d", writer.calls, failAt)
			}
		}
	}
}

func TestDocumentSymbolsMaximumRequestID(t *testing.T) {
	const prefix = `{"jsonrpc":"2.0","id":"`
	const suffix = `","method":"textDocument/documentSymbol","params":{"textDocument":{"uri":"u"}}}`
	for _, character := range []string{"<", "\u2028", "\u2029"} {
		t.Run(fmt.Sprintf("%U", []rune(character)[0]), func(t *testing.T) {
			// HTML and JavaScript separators in a raw ID must not expand.
			// The request fills the input budget, leaving only its method
			// wrapper's space for a same-ID error response.
			budget := maxMessageBytes - len(prefix) - len(suffix)
			id := strings.Repeat(character, budget/len(character)) + strings.Repeat("a", budget%len(character))
			body := prefix + id + suffix
			var input, output bytes.Buffer
			fmt.Fprintf(&input, "Content-Length: %d\r\n\r\n%s", len(body), body)
			input.WriteString(frame(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"}))
			server := &Server{In: &input, Out: &output, docs: map[string]string{"u": outlineArticles(1, " ")}}
			if err := server.Serve(t.Context()); err != nil {
				t.Fatal(err)
			}
			reader := bufio.NewReader(&output)
			first, err := readMessage(reader)
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				ID    string    `json:"id"`
				Error *rpcError `json:"error"`
			}
			if err := json.Unmarshal(first, &response); err != nil {
				t.Fatal(err)
			}
			if response.ID != id || response.Error == nil || response.Error.Code != -32603 {
				t.Fatal("maximum-size request did not receive its same-ID outline error")
			}
			second, err := readMessage(reader)
			if err != nil || !bytes.Contains(second, []byte(`"result":null`)) {
				t.Fatalf("subsequent shutdown: %s, %v", second, err)
			}
		})
	}
}

func TestDocumentSymbolsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var output bytes.Buffer
	server := &Server{Out: &output, ctx: ctx}
	if err := server.respondDocumentSymbols(&rpcRequest{ID: json.RawMessage(`1`)}, outlineArticles(1, " ")); !errors.Is(err, context.Canceled) || output.Len() != 0 {
		t.Fatalf("canceled outline: err=%v output=%d bytes", err, output.Len())
	}
}
