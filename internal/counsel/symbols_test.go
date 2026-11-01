package counsel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDocumentSymbolsProtocol(t *testing.T) {
	src := "FORM K-1.\r\nIN THE MATTER OF: outline.\r\nHEREINAFTER, greeting SHALL MEAN \"😀\".\r\nARTICLE 1.\r\nPROCLAIM greeting.\r\nTHE OFFICE OF answer.\r\nSECTION 1.\r\nREMAND WITH 42.\r\n"
	var input bytes.Buffer
	send := func(id int, method string, params any) {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		fmt.Fprintf(&input, "Content-Length: %d\r\n\r\n%s", len(b), b)
	}
	doc := map[string]any{"uri": "untitled:outline", "text": src, "version": 1}
	send(1, "initialize", map[string]any{})
	send(0, "textDocument/didOpen", map[string]any{"textDocument": doc})
	send(2, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": "untitled:outline"}})
	send(0, "textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": "untitled:outline", "version": 2}, "contentChanges": []any{map[string]any{"text": "FORM K-1.\nIN THE MATTER OF: changed.\nARTICLE 9.\nPROCLAIM 1.\n"}}})
	send(3, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": "untitled:outline"}})
	send(0, "textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": "untitled:outline"}})
	send(4, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": "untitled:outline"}})
	var output bytes.Buffer
	if err := (&Server{In: &input, Out: &output}).Serve(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Parse real framed responses, ignoring diagnostics.
	reader := strings.NewReader(output.String())
	results := map[int]json.RawMessage{}
	for reader.Len() > 0 {
		var n int
		if _, err := fmt.Fscanf(reader, "Content-Length: %d\r\n\r\n", &n); err != nil {
			t.Fatal(err)
		}
		b := make([]byte, n)
		if _, err := reader.Read(b); err != nil {
			t.Fatal(err)
		}
		var r struct {
			ID     int
			Result json.RawMessage
			Error  any
		}
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		if r.Error != nil {
			t.Fatalf("protocol error: %s", b)
		}
		results[r.ID] = r.Result
	}
	var init struct {
		Capabilities struct {
			Symbols bool `json:"documentSymbolProvider"`
		}
	}
	if err := json.Unmarshal(results[1], &init); err != nil || !init.Capabilities.Symbols {
		t.Fatal("document symbols not advertised")
	}
	var symbols []struct {
		Name           string
		Range          struct{ Start, End position }
		SelectionRange struct{ Start, End position }
		Children       []struct {
			Name  string
			Range struct{ Start, End position }
		}
	}
	if err := json.Unmarshal(results[2], &symbols); err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 3 || symbols[0].Name != "greeting" || symbols[1].Name != "ARTICLE 1" || symbols[2].Name != "answer" {
		t.Fatalf("outline: %s", results[2])
	}
	if symbols[0].SelectionRange.End.Character != 38 {
		t.Fatalf("UTF16 declaration end: %+v", symbols[0].SelectionRange)
	}
	if len(symbols[2].Children) != 1 || symbols[2].Children[0].Name != "SECTION 1" || symbols[2].Children[0].Range.Start.Line != 6 {
		t.Fatalf("office sections: %s", results[2])
	}
	if !strings.Contains(string(results[3]), "ARTICLE 9") {
		t.Fatalf("stale outline: %s", results[3])
	}
	if string(results[4]) != "[]" {
		t.Fatalf("closed outline: %s", results[4])
	}
}

func TestDocumentSymbolsInvalidAndRanges(t *testing.T) {
	for _, source := range []string{"", "FORM K-1.\nARTICLE", "not a filing"} {
		if symbols := symbolsForTest(t, source); len(symbols) != 0 {
			t.Fatalf("invalid source has symbols: %+v", symbols)
		}
	}
	source := "FORM S-1.\r\nIN THE MATTER OF: library.\r\nTHE EXHIBIT OF pair, COMPRISING left AND right.\r\n  THE OFFICE OF answer.\r\n  SECTION 2.\r\n  REMAND WITH \"😀\"."
	symbols := symbolsForTest(t, source)
	if len(symbols) != 2 || symbols[0].Name != "pair" || symbols[0].Kind != 23 || symbols[1].Kind != 12 {
		t.Fatalf("statute symbols: %+v", symbols)
	}
	office := symbols[1]
	if office.Range.Start != (position{Line: 3, Character: 2}) || office.Range.End != (position{Line: 5, Character: 19}) {
		t.Fatalf("office range %+v", office.Range)
	}
	if symbols[0].Range.End != (position{Line: 3, Character: 2}) {
		t.Fatalf("exhibit range %+v", symbols[0].Range)
	}
	if len(office.Children) != 1 || office.Children[0].Range.End != office.Range.End {
		t.Fatalf("section containment %+v", office)
	}
}

func symbolsForTest(t *testing.T, source string) []documentSymbol {
	t.Helper()
	symbols, err := documentSymbols(t.Context(), source, maxMessageBytes)
	if err != nil {
		t.Fatal(err)
	}
	return symbols
}

func TestDocumentSymbolsSameLineProtocol(t *testing.T) {
	cases := []struct {
		source string
		names  []string
		starts []int
		line   int
	}{
		{`FORM K-1. IN THE MATTER OF: outline. ARTICLE 1. PROCLAIM "😀". ARTICLE 2. ADJOURN INDEFINITELY.`, []string{"ARTICLE 1", "ARTICLE 2"}, []int{37, 63}, 0},
		{`FORM S-1. IN THE MATTER OF: outline. THE OFFICE OF alpha. SECTION 1. REMAND WITH "😀". SECTION 2. REMAND WITH 1. HEREINAFTER, late SHALL MEAN 2.`, []string{"alpha", "late"}, []int{37, 113}, 0},
		{"FORM K-1.\r\nIN THE MATTER OF: outline. HEREINAFTER, greeting SHALL MEAN \"😀\". ARTICLE 3. PROCLAIM greeting.", []string{"greeting", "ARTICLE 3"}, []int{27, 66}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.names[0], func(t *testing.T) {
			var output bytes.Buffer
			input := frame(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": "untitled:compact", "text": tc.source}}}) + frame(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "textDocument/documentSymbol", "params": map[string]any{"textDocument": map[string]any{"uri": "untitled:compact"}}})
			if err := (&Server{In: strings.NewReader(input), Out: &output}).Serve(t.Context()); err != nil {
				t.Fatal(err)
			}
			reader := bufio.NewReader(&output)
			for {
				b, err := readMessage(reader)
				if err != nil {
					t.Fatal(err)
				}
				var response struct {
					ID     int
					Result []documentSymbol
				}
				if err := json.Unmarshal(b, &response); err != nil {
					t.Fatal(err)
				}
				if response.ID != 1 {
					continue
				}
				if len(response.Result) != len(tc.names) {
					t.Fatalf("symbols: %s", b)
				}
				for i, symbol := range response.Result {
					if symbol.Name != tc.names[i] || symbol.Range.Start != (position{Line: tc.line, Character: tc.starts[i]}) {
						t.Fatalf("symbol %d: %+v", i, symbol)
					}
					if i+1 < len(response.Result) && symbol.Range.End != response.Result[i+1].Range.Start {
						t.Fatalf("sibling boundary: %+v", symbol)
					}
					if symbol.Name == "alpha" {
						if len(symbol.Children) != 2 || symbol.Children[0].Range.Start != (position{Character: 58}) || symbol.Children[0].Range.End != (position{Character: 87}) || symbol.Children[1].Range.Start != (position{Character: 87}) {
							t.Fatalf("same-line section positions: %+v", symbol.Children)
						}
					}
					assertSymbolContainment(t, symbol)
				}
				break
			}
		})
	}
}

func assertSymbolContainment(t *testing.T, symbol documentSymbol) {
	t.Helper()
	before := func(a, b position) bool { return a.Line < b.Line || (a.Line == b.Line && a.Character <= b.Character) }
	if !before(symbol.Range.Start, symbol.SelectionRange.Start) || !before(symbol.SelectionRange.End, symbol.Range.End) || !before(symbol.SelectionRange.Start, symbol.SelectionRange.End) {
		t.Fatalf("selection outside range: %+v", symbol)
	}
	for _, child := range symbol.Children {
		if !before(symbol.Range.Start, child.Range.Start) || !before(child.Range.End, symbol.Range.End) {
			t.Fatalf("child outside office: %+v", symbol)
		}
		assertSymbolContainment(t, child)
	}
}
