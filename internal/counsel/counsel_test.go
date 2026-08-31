package counsel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDiagnoseCleanFiling(t *testing.T) {
	if d := Diagnose("FORM K-1.\nIN THE MATTER OF: clean.\nARTICLE 1.\nPROCLAIM 1.\n"); len(d) != 0 {
		t.Fatalf("a lawful filing drew diagnostics: %v", d)
	}
}

func TestDiagnoseRejectedFiling(t *testing.T) {
	d := Diagnose("FORM K-1.\nIN THE MATTER OF: broken.\nARTICLE 1.\nPROCLAIM\n")
	if len(d) != 1 {
		t.Fatalf("expected one diagnostic, got %v", d)
	}
	msg := d[0]["message"].(string)
	if !strings.Contains(msg, "Article §4.2") {
		t.Fatalf("the diagnostic does not cite the Article: %q", msg)
	}
}

func TestDiagnoseSkipsCompileWhenIncorporating(t *testing.T) {
	// The clerk cannot fetch enactments from inside an editor: a filing
	// that incorporates gets parse-level counsel only, not false
	// convictions about offices it lawfully imported.
	src := `FORM K-1.
IN THE MATTER OF: importer.
INCORPORATE BY REFERENCE statutes-of-arithmetic.
ARTICLE 1.
    PROCLAIM THE FINDING OF maximum REGARDING 1 AND 2.
    ADJOURN INDEFINITELY.
`
	if d := Diagnose(src); len(d) != 0 {
		t.Fatalf("an incorporating filing drew compile diagnostics it cannot answer in an editor: %v", d)
	}
}

func TestWordAt(t *testing.T) {
	text := "    PROCLAIM THE STANDING OF ward.\n"
	if w := wordAt(text, position{Line: 0, Character: 6}); w != "PROCLAIM" {
		t.Fatalf("wordAt = %q", w)
	}
	if w := wordAt(text, position{Line: 0, Character: 30}); w != "" {
		t.Fatalf("a lower-case identifier is not a keyword; wordAt = %q", w)
	}
}

func TestWordAtUsesLSPUTF16Columns(t *testing.T) {
	text := "😀 PROCLAIM 1.\n"
	if w := wordAt(text, position{Line: 0, Character: 4}); w != "PROCLAIM" {
		t.Fatalf("wordAt after a surrogate pair = %q, want PROCLAIM", w)
	}
	if w := wordAt(text, position{Line: 0, Character: 1}); w != "" {
		t.Fatalf("a cursor inside a surrogate pair returned %q", w)
	}
}

func TestReadMessageRejectsOversizedPayload(t *testing.T) {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", maxMessageBytes+1)
	_, err := readMessage(bufio.NewReader(strings.NewReader(header)))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized LSP payload returned %v", err)
	}
}

func TestReadMessageRejectsOversizedHeader(t *testing.T) {
	header := "X-" + strings.Repeat("a", maxHeaderLineBytes) + ": value\r\n\r\n"
	_, err := readMessage(bufio.NewReaderSize(strings.NewReader(header), maxHeaderLineBytes))
	if err == nil || !strings.Contains(err.Error(), "header line exceeds") {
		t.Fatalf("oversized header returned %v", err)
	}
}

func TestDocumentStoreBoundsAndPreservesOldText(t *testing.T) {
	s := &Server{docs: make(map[string]string)}
	if err := s.storeDocument("file:///x.trial", "old", false); err != nil {
		t.Fatal(err)
	}
	if err := s.storeDocument("file:///x.trial", strings.Repeat("x", maxDocumentBytes+1), true); err == nil {
		t.Fatal("oversized document was accepted")
	}
	if got := s.docs["file:///x.trial"]; got != "old" {
		t.Fatalf("failed change replaced old text with %q", got)
	}
	if err := s.storeDocument("file:///missing.trial", "new", true); err == nil {
		t.Fatal("change for a closed document was accepted")
	}
}

func TestDocumentStoreBoundsOpenDocumentCount(t *testing.T) {
	s := &Server{docs: make(map[string]string)}
	for i := range maxDocuments {
		uri := fmt.Sprintf("file:///case-%03d.trial", i)
		if err := s.storeDocument(uri, "", false); err != nil {
			t.Fatalf("open document %d: %v", i, err)
		}
	}
	if err := s.storeDocument("file:///one-too-many.trial", "", false); err == nil {
		t.Fatal("document count above the limit was accepted")
	}
	if len(s.docs) != maxDocuments {
		t.Fatalf("document count = %d, want %d", len(s.docs), maxDocuments)
	}
}

func TestHoverTableCitesSpec(t *testing.T) {
	for kw, text := range hoverText {
		if !strings.Contains(text, "spec §") {
			t.Errorf("the hover for %s does not cite the spec", kw)
		}
	}
}

// frame wraps a payload in LSP Content-Length framing.
func frame(v any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(b), b)
}

func TestServeInitializeOpenHover(t *testing.T) {
	var in bytes.Buffer
	in.WriteString(frame(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}}))
	in.WriteString(frame(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{
		"textDocument": map[string]any{"uri": "file:///x.trial", "text": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM ghost\n"},
	}}))
	in.WriteString(frame(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "textDocument/hover", "params": map[string]any{
		"textDocument": map[string]any{"uri": "file:///x.trial"},
		"position":     map[string]any{"line": 3, "character": 2},
	}}))
	in.WriteString(frame(map[string]any{"jsonrpc": "2.0", "method": "exit"}))

	var out bytes.Buffer
	s := &Server{In: &in, Out: &out, Version: "test"}
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("counsel withdrew: %v", err)
	}

	r := bufio.NewReader(&out)
	var messages []map[string]any
	for {
		b, err := readMessage(r)
		if err != nil {
			break
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, m)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 replies (initialize, diagnostics, hover), got %d: %v", len(messages), messages)
	}
	if _, ok := messages[0]["result"].(map[string]any)["capabilities"]; !ok {
		t.Fatalf("initialize reply lacks capabilities: %v", messages[0])
	}
	if messages[1]["method"] != "textDocument/publishDiagnostics" {
		t.Fatalf("expected diagnostics notification, got %v", messages[1])
	}
	diags := messages[1]["params"].(map[string]any)["diagnostics"].([]any)
	if len(diags) != 1 {
		t.Fatalf("the unterminated statement drew %d diagnostic(s)", len(diags))
	}
	hover := messages[2]["result"].(map[string]any)["contents"].(map[string]any)["value"].(string)
	if !strings.Contains(hover, "Output") {
		t.Fatalf("hover on PROCLAIM = %q", hover)
	}
}
