// Package counsel implements the Content-Length-framed LSP server.
package counsel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lennrt/trial-lang/internal/gregor"
)

// Server processes one editor connection. The caller owns In and Out.
type Server struct {
	In      io.Reader
	Out     io.Writer
	Version string

	docs map[string]string // uri -> current text
}

// --- JSON-RPC / LSP plumbing -------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"` // notifications from us
	Params  any             `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	maxMessageBytes     = 16 << 20
	maxHeaderLineBytes  = 8 << 10
	maxHeaderBytes      = 64 << 10
	maxHeaders          = 64
	maxDocumentBytes    = 4 << 20
	maxDocuments        = 128
	maxDocumentURIBytes = 4096
)

func (s *Server) write(v rpcResponse) error {
	v.JSONRPC = "2.0"
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > maxMessageBytes {
		return fmt.Errorf("response is %d bytes; limit is %d", len(b), maxMessageBytes)
	}
	if _, err := fmt.Fprintf(s.Out, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	_, err = s.Out.Write(b)
	return err
}

func (s *Server) respond(req *rpcRequest, response rpcResponse) error {
	if len(req.ID) == 0 {
		return nil
	}
	response.ID = req.ID
	return s.write(response)
}

// readMessage reads one Content-Length framed message.
func readMessage(r *bufio.Reader) ([]byte, error) {
	length := -1
	headerBytes := 0
	for headerCount := 0; ; headerCount++ {
		if headerCount >= maxHeaders {
			return nil, fmt.Errorf("message has more than %d headers", maxHeaders)
		}
		lineBytes, err := r.ReadSlice('\n')
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				return nil, fmt.Errorf("header line exceeds %d bytes", maxHeaderLineBytes)
			}
			return nil, err
		}
		headerBytes += len(lineBytes)
		if headerBytes > maxHeaderBytes {
			return nil, fmt.Errorf("headers exceed %d bytes", maxHeaderBytes)
		}
		line := strings.TrimRight(string(lineBytes), "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("header has no colon")
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "content-length":
			if length >= 0 {
				return nil, fmt.Errorf("message has more than one Content-Length header")
			}
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("the Content-Length header cannot be read: %w", err)
			}
			length = parsed
		case "content-type":
			// LSP permits this optional legacy header.
		default:
			return nil, fmt.Errorf("unsupported header %q", strings.TrimSpace(name))
		}
	}
	if length < 0 {
		return nil, errors.New("message has no Content-Length header")
	}
	if length > maxMessageBytes {
		return nil, fmt.Errorf("Content-Length %d exceeds the %d-byte limit", length, maxMessageBytes)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// Serve reads requests until In closes, the client says exit, or ctx
// is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	if s.docs == nil {
		s.docs = make(map[string]string)
	}
	r := bufio.NewReaderSize(s.In, maxHeaderLineBytes)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := readMessage(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var req rpcRequest
		if err := decodeStrict(msg, &req); err != nil {
			if writeErr := s.write(rpcResponse{ID: json.RawMessage("null"), Error: &rpcError{
				Code: -32700, Message: "request could not be read: " + err.Error(),
			}}); writeErr != nil {
				return writeErr
			}
			continue
		}
		if req.Method == "exit" {
			return nil
		}
		if err := s.handle(&req); err != nil {
			return err
		}
	}
}

func (s *Server) handle(req *rpcRequest) error {
	switch req.Method {
	case "initialize":
		return s.respond(req, rpcResponse{Result: map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync":   1, // full synchronization
				"hoverProvider":      true,
				"completionProvider": map[string]any{},
			},
			"serverInfo": map[string]any{"name": "trial counsel", "version": s.Version},
		}})

	case "shutdown":
		return s.respond(req, rpcResponse{Result: json.RawMessage("null")})

	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI        string `json:"uri"`
				LanguageID string `json:"languageId"`
				Version    int64  `json:"version"`
				Text       string `json:"text"`
			} `json:"textDocument"`
		}
		if err := decodeStrict(req.Params, &p); err != nil {
			return s.invalidParams(req, err)
		}
		if err := s.storeDocument(p.TextDocument.URI, p.TextDocument.Text, false); err != nil {
			return s.publishInputError(p.TextDocument.URI, err)
		}
		return s.publishDiagnostics(p.TextDocument.URI)

	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int64  `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if err := decodeStrict(req.Params, &p); err != nil {
			return s.invalidParams(req, err)
		}
		if len(p.ContentChanges) != 1 {
			return s.publishInputError(p.TextDocument.URI, fmt.Errorf("full synchronization requires exactly one content change"))
		}
		if err := s.storeDocument(p.TextDocument.URI, p.ContentChanges[0].Text, true); err != nil {
			return s.publishInputError(p.TextDocument.URI, err)
		}
		return s.publishDiagnostics(p.TextDocument.URI)

	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if err := decodeStrict(req.Params, &p); err != nil {
			return s.invalidParams(req, err)
		}
		delete(s.docs, p.TextDocument.URI)
		return s.write(rpcResponse{Method: "textDocument/publishDiagnostics", Params: map[string]any{
			"uri": p.TextDocument.URI, "diagnostics": []any{},
		}})

	case "textDocument/hover":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Position position `json:"position"`
		}
		if err := decodeStrict(req.Params, &p); err != nil {
			return s.invalidParams(req, err)
		}
		word := wordAt(s.docs[p.TextDocument.URI], p.Position)
		if text, ok := hoverText[word]; ok {
			return s.respond(req, rpcResponse{Result: map[string]any{
				"contents": map[string]any{"kind": "markdown", "value": text},
			}})
		}
		return s.respond(req, rpcResponse{Result: json.RawMessage("null")})

	case "textDocument/completion":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Position position        `json:"position"`
			Context  json.RawMessage `json:"context"`
		}
		if err := decodeStrict(req.Params, &p); err != nil {
			return s.invalidParams(req, err)
		}
		items := make([]map[string]any, 0, len(completions))
		for _, c := range completions {
			items = append(items, map[string]any{
				"label":  c.label,
				"kind":   14, // keyword
				"detail": c.detail,
			})
		}
		return s.respond(req, rpcResponse{Result: items})
	}

	// Unknown notifications receive no reply, as JSON-RPC requires.
	if len(req.ID) > 0 {
		return s.write(rpcResponse{ID: req.ID, Error: &rpcError{
			Code: -32601, Message: fmt.Sprintf("method %q is not supported", req.Method),
		}})
	}
	return nil
}

func decodeStrict(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("more than one JSON value was supplied")
		}
		return err
	}
	return nil
}

func (s *Server) storeDocument(uri, text string, mustExist bool) error {
	if uri == "" {
		return errors.New("document URI is empty")
	}
	if len(uri) > maxDocumentURIBytes {
		return fmt.Errorf("document URI exceeds %d bytes", maxDocumentURIBytes)
	}
	if len(text) > maxDocumentBytes {
		return fmt.Errorf("document exceeds %d bytes", maxDocumentBytes)
	}
	_, exists := s.docs[uri]
	if mustExist && !exists {
		return errors.New("document is not open")
	}
	if !exists && len(s.docs) >= maxDocuments {
		return fmt.Errorf("at most %d documents may be open", maxDocuments)
	}
	s.docs[uri] = text
	return nil
}

func (s *Server) publishInputError(uri string, cause error) error {
	return s.write(rpcResponse{Method: "textDocument/publishDiagnostics", Params: map[string]any{
		"uri": uri,
		"diagnostics": []map[string]any{{
			"range": map[string]any{
				"start": position{},
				"end":   position{},
			},
			"severity": 1,
			"source":   "trial-counsel",
			"message":  cause.Error(),
		}},
	}})
}

func (s *Server) invalidParams(req *rpcRequest, cause error) error {
	if len(req.ID) == 0 {
		return nil
	}
	return s.write(rpcResponse{ID: req.ID, Error: &rpcError{
		Code: -32602, Message: "request parameters could not be read: " + cause.Error(),
	}})
}

// --- diagnostics --------------------------------------------------------

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Diagnose parses and compiles source with Gregor. If the filing incorporates
// statutes, Diagnose checks syntax only because it has no statute source.
func Diagnose(src string) []map[string]any {
	prog, err := gregor.Parse(src)
	if err == nil && len(prog.Incorporations) == 0 {
		_, err = gregor.Compile(prog)
	}
	if err == nil {
		return []map[string]any{}
	}
	line, col := 1, 1
	msg := err.Error()
	if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
		line, col, msg = rej.Line, rej.Col, rej.Particulars
	}
	start, end := diagnosticRange(src, line, col)
	return []map[string]any{{
		"range": map[string]any{
			"start": start,
			"end":   end,
		},
		"severity": 1,
		"source":   "gregor",
		"message":  msg + " (Filing would be rejected pursuant to Article §4.2.)",
	}}
}

// Gregor reports one-based byte columns; LSP uses zero-based UTF-16 columns.
func diagnosticRange(src string, line, column int) (position, position) {
	lines := strings.Split(src, "\n")
	line--
	if line < 0 || line >= len(lines) {
		return position{}, position{}
	}
	text := lines[line]
	byteColumn := min(max(column-1, 0), len(text))
	units := 0
	for _, r := range text[:byteColumn] {
		units++
		if r > 0xffff {
			units++
		}
	}
	start := position{Line: line, Character: units}
	end := start
	if byteColumn < len(text) {
		end.Character++
		r, _ := utf8.DecodeRuneInString(text[byteColumn:])
		if r > 0xffff {
			end.Character++
		}
	}
	return start, end
}

func (s *Server) publishDiagnostics(uri string) error {
	return s.write(rpcResponse{Method: "textDocument/publishDiagnostics", Params: map[string]any{
		"uri":         uri,
		"diagnostics": Diagnose(s.docs[uri]),
	}})
}

// wordAt extracts the upper-case keyword under the cursor, if any.
func wordAt(text string, pos position) string {
	lines := strings.Split(text, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	byteOffset, ok := utf16ColumnToByteOffset(line, pos.Character)
	if !ok {
		return ""
	}
	isWord := func(c byte) bool {
		return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-'
	}
	start, end := byteOffset, byteOffset
	for start > 0 && isWord(line[start-1]) {
		start--
	}
	for end < len(line) && isWord(line[end]) {
		end++
	}
	return line[start:end]
}

// LSP positions count UTF-16 code units, while Go strings are UTF-8.
func utf16ColumnToByteOffset(line string, column int) (int, bool) {
	if column < 0 {
		return 0, false
	}
	units := 0
	for i, r := range line {
		if units == column {
			return i, true
		}
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > column {
			return 0, false
		}
		units += width
	}
	if units == column {
		return len(line), true
	}
	return 0, false
}

// --- language reference -------------------------------------------------

// hoverText returns reference text for a keyword.
var hoverText = map[string]string{
	"FORM":            "**Form declaration.** `FORM K-1.` opens a case; `FORM K-2.` is a supplemental filing for a live case; `FORM S-1.` defines a statute. — spec §13.2",
	"PROCLAIM":        "**Output.** `PROCLAIM expr.` appends the value's transcript to the proclamations topic (stdout). — spec §11.3",
	"AWAIT":           "**Input.** `AWAIT SUMMONS, FILED UNDER x.` blocks on the summons topic. `AWAIT SUMMONS FOR AT MOST n DAYS, …. FAILING WHICH, ….` adds a deadline. `AWAIT SUMMONS FROM c, ….` selectively receives the first input from case c; skipped records keep their order. `AWAIT THE GAZETTE, FILED UNDER x.` reads the court-wide gazette at this case's cursor. — spec §11.4, §11.4a, §11.4b, §11.14",
	"SUMMONS":         "**Input.** The summons topic stores case input. `AWAIT SUMMONS` consumes according to the case attention. — spec §11.4",
	"GAZETTE":         "**Court-wide broadcast.** `PUBLISH v IN THE GAZETTE.` writes within the execution-step transaction. `AWAIT THE GAZETTE, FILED UNDER x.` uses a per-case cursor. — spec §11.14",
	"PUBLISH":         "**Broadcast.** `PUBLISH v IN THE GAZETTE.` appends the transcript to the one court-wide gazette topic, inside this step's transaction. — spec §11.14",
	"SHOULD":          "**The conditional.** `SHOULD cond, stmt.` and optionally `FAILING WHICH, stmt.` Connectives: `AND ALSO`, `OR IN THE ALTERNATIVE`; every clause is heard (no short-circuit). — spec §11.5",
	"FAILING":         "**Else branch.** `FAILING WHICH, stmt.` follows the consequence's period and attaches to the nearest SHOULD or timed AWAIT. — spec §11.5, §11.4a",
	"REFER":           "**The jump.** `REFER TO ARTICLE n.` (case in chief) or `REFER TO SECTION n.` (within an office) sets the logical program counter to the target instruction. — spec §11.6",
	"ARTICLE":         "**Case label.** Execution begins at the first article and falls through. Compilation resolves article labels to logical instruction addresses. — spec §13.3",
	"SECTION":         "**Office label.** `REFER TO SECTION` can reach a section only from the same office. — spec §12",
	"PETITION":        "**The call.** `PETITION THE OFFICE OF name WITH args.` opens a frame on the appeals topic. As an expression: `THE FINDING OF name REGARDING args`. Dynamic, through a power of attorney: `PETITION UNDER p WITH args.` / `THE FINDING UNDER p REGARDING args`. — spec §11.7, §12, §12.5",
	"ATTORNEY":        "**The office as a value.** `A POWER OF ATTORNEY OVER THE OFFICE OF f`: the right to petition f, wherever the instrument travels within the case. Exercised with PETITION UNDER / THE FINDING UNDER; enforceable only in the case that executed it. — spec §12.5",
	"REMAND":          "**Return.** `REMAND.` or `REMAND WITH expr.` Outside an office, REMAND produces a verdict. — spec §11.7",
	"ADJOURN":         "**Stop or delay execution.** `ADJOURN INDEFINITELY.` stops until amendment. `ADJOURN FOR n DAYS.` records a deadline. One court day is one second. — spec §11.8",
	"HOLD":            "**Explicit verdict.** `HOLD expr IN CONTEMPT.` records a verdict with the value as its sealed details. — spec §11.9",
	"CONTEMPT":        "**See HOLD.** The expression is evaluated, displayed, and stored as verdict details. — spec §11.9",
	"STRIKE":          "**Delete a record value.** `STRIKE x FROM THE RECORD.` writes a Kafka tombstone. — spec §11.10",
	"SERVE":           "**Cross-case output.** `SERVE NOTICE OF v UPON w.` appends to case w's summons topic within the execution-step transaction. Self-service is allowed. — spec §11.11",
	"COMMENCE":        "**Spawn.** `COMMENCE PROCEEDINGS UPON src, FILED UNDER c.` files a new case from a Form K-1 string; the number is ledgered, so replay opens nothing twice. — spec §11.12",
	"JUDGMENT":        "**External verdict.** `ENTER JUDGMENT AGAINST c, ON THE GROUNDS OF g.` writes a verdict within the current step. The current case must have created c. Case c stops before its next step. — spec §11.12a",
	"MOTION":          "**One-time verdict interception.** `FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE n[, THE GROUNDS FILED UNDER g].` Filing clears the operand stack. — spec §11.13",
	"RECONSIDER":      "**See MOTION.** A case can intercept its first eligible verdict. The operation clears the dossier. — spec §11.13",
	"INCORPORATE":     "**Import.** `INCORPORATE BY REFERENCE statute.` compiles the latest statute version into the filing. Imports are transitive. — spec §13.2a",
	"HEREINAFTER":     "**Defined term.** `HEREINAFTER, k SHALL MEAN literal.` The compiler substitutes the literal at each use. — spec §5",
	"EXHIBIT":         "**A struct.** Declare `THE EXHIBIT OF name, COMPRISING a AND b.`; offer `AN EXHIBIT OF name WHEREIN a IS 1 AND b IS 2`; inspect `THE a ENTERED IN x`. Value semantics, deep equality. — spec §8",
	"SCHEDULE":        "**List.** `A SCHEDULE COMPRISING …` / `AN EMPTY SCHEDULE`; `THE ITEM AT i IN s`, `ANNEX e TO s.`, `SUBSTITUTE e FOR ITEM i OF s.` Indices start at one. — spec §8.1",
	"ANNEX":           "**Append to a schedule.** `ANNEX expr TO s.` retrieves a copy, extends it, files it back. — spec §8.1",
	"SUBSTITUTE":      "**Replace in a schedule.** `SUBSTITUTE expr FOR ITEM i OF s.` — spec §8.1",
	"REGISTER":        "**A map.** `A REGISTER COMPRISING v UNDER k AND …` / `AN EMPTY REGISTER`; `THE ENTRY UNDER k IN r`, `INSCRIBE v UNDER k IN r.`, `EXPUNGE THE ENTRY UNDER k IN r.`, `THE ROSTER OF r` (the keys, alphabetically). Keys are strings; an absent entry is a verdict. — spec §8.2",
	"INSCRIBE":        "**Enter in a register.** `INSCRIBE v UNDER k IN r.` retrieves a copy, amends it, files it back. — spec §8.2",
	"EXPUNGE":         "**Remove from a register.** `EXPUNGE THE ENTRY UNDER k IN r.` Missing keys are ignored. — spec §8.2",
	"ROSTER":          "**Register keys.** Returns a schedule of keys in alphabetical order. — spec §8.2",
	"ENTRY":           "**One entry of a register.** `THE ENTRY UNDER k IN r`; an absent entry is a verdict — consult THE ROSTER OF r first. — spec §8.2",
	"DISCRETION":      "**Randomness.** `THE DISCRETION OF THE COURT BETWEEN a AND b` returns an integer in the inclusive range. The draw is recorded so replay repeats it. — spec §10.8",
	"PRESENTS":        "**The clock.** `THE DATE OF THESE PRESENTS`: now, in court days since the epoch; the reading is ledgered. — spec §10.8",
	"STANDING":        "**Supervision.** `THE STANDING OF c`: `GUILTY`, `IN GOOD STANDING`, or `NO MATTER ON FILE`, read through the ledger so replay holds. — spec §10.11",
	"RECORD":          "**Discovery (in expressions).** `THE RECORD name IN THE MATTER OF c` reads another case's record, read-only, through the ledger. Absence is a verdict; ask THE STANDING OF first. — spec §10.12",
	"ARCHIVE":         "**Files.** `COMMIT v TO THE ARCHIVE AS \"name\".` and `THE DOCUMENT \"name\" FROM THE ARCHIVE`: immutable versions, catalog points at the current one. — spec §10.9",
	"DOCUMENT":        "**Retrieval from the archive.** `THE DOCUMENT expr FROM THE ARCHIVE`: the cataloged version. — spec §10.9",
	"LETTERS":         "**Patent grant.** `LET LETTERS PATENT ISSUE FOR name, DISCLOSING v, FOR A TERM OF n DAYS.` Filing order is determined by topic offsets. — spec §10.10",
	"PATENT":          "**See LETTERS.** An existing in-force filing causes a verdict. Expired filings allow a new grant. — spec §10.10",
	"GRANT":           "**License.** `GRANT A LICENSE UNDER x TO c, FOR A TERM OF n DAYS.` Only the holder can grant it; the licensee gets read-only practice, and the license cannot outlive the patent. — spec §10.10a",
	"LICENSE":         "**See GRANT.** Licenses allow read-only practice until their recorded expiry. — spec §10.10a",
	"ASSIGN":          "**Transfer.** `ASSIGN THE LETTERS FOR x TO c.` transfers a patent and is refused while licenses are outstanding. The previous holder can no longer practice it. — spec §10.10a",
	"PRACTICE":        "**Use of an invention.** `THE PRACTICE OF name`: the disclosure to the holder, infringement to everyone else while the term runs. — spec §10.10",
	"TRANSCRIPT":      "**To string.** `THE TRANSCRIPT OF v`: any value, rendered as PROCLAIM would publish it. — spec §10.5",
	"LENGTH":          "**Measure.** `THE LENGTH OF v`: characters of a string, entries of an exhibit, items of a schedule. — spec §10.5",
	"EXCERPT":         "**Substring.** `AN EXCERPT OF s FROM i TO j`: 1-indexed, both ends inclusive, in characters. — spec §10.5",
	"SUM":             "**Parse a number, or money.** `THE SUM CERTAIN OF v`: the integer or sum a string denotes, exactly and entirely, or a verdict. Sums are stated to the penny. — spec §10.5, §7",
	"SUSTAINED":       "**The affirmative finding** (true). Findings come from comparisons and return from offices. — spec §7",
	"OVERRULED":       "**The negative finding** (false). — spec §7",
	"OFF":             "**Comment.** `OFF THE RECORD: …` continues to the end of the line. The filing topic still stores the source text. — spec §4.2",
	"CASE":            "**Self-reference.** `THE CASE AT BAR` returns this case's number as a string. — spec §10.8",
	"NOTWITHSTANDING": "**Remainder.** `a NOTWITHSTANDING b` computes the integer remainder. A zero divisor produces a verdict. — spec §10.2",
	"APPORTIONED":     "**Division.** `a APPORTIONED AMONG b` performs integer division toward zero. A zero divisor produces a verdict. — spec §10.2",
}

type completion struct{ label, detail string }

var completions = buildCompletions()

func buildCompletions() []completion {
	out := []completion{
		{"LET IT BE RECORDED THAT", "recording (assignment): LET IT BE RECORDED THAT x IS expr."},
		{"LET IT BE ENTERED IN", "entry amendment: LET IT BE ENTERED IN x THAT field IS expr."},
		{"LET LETTERS PATENT ISSUE FOR", "patent grant: …, DISCLOSING expr, FOR A TERM OF n DAYS."},
		{"GRANT A LICENSE UNDER", "shared borrow: … x TO c, FOR A TERM OF n DAYS."},
		{"ASSIGN THE LETTERS FOR", "move: … x TO c. (refused while licenses run)"},
		{"PROCLAIM", "output: PROCLAIM expr."},
		{"AWAIT SUMMONS, FILED UNDER", "input: blocks until served"},
		{"AWAIT SUMMONS FOR AT MOST", "timed input: … n DAYS, FILED UNDER x. FAILING WHICH, stmt."},
		{"AWAIT SUMMONS FROM", "selective input: the first notice bearing the named case's seal, out of turn"},
		{"AWAIT THE GAZETTE, FILED UNDER", "broadcast input: the next edition at this case's cursor"},
		{"PUBLISH", "broadcast output: PUBLISH expr IN THE GAZETTE."},
		{"REFER TO ARTICLE", "jump (case in chief)"},
		{"REFER TO SECTION", "jump (within an office)"},
		{"SHOULD", "conditional: SHOULD cond, stmt."},
		{"FAILING WHICH,", "else-branch, after the consequence's period"},
		{"PETITION THE OFFICE OF", "call: … name WITH args."},
		{"PETITION UNDER", "dynamic call: … power WITH args."},
		{"THE FINDING UNDER", "dynamic call in expression position: … power REGARDING args"},
		{"A POWER OF ATTORNEY OVER THE OFFICE OF", "the office as a value"},
		{"REMAND WITH", "return a value from an office"},
		{"ADJOURN INDEFINITELY.", "stop until amendment"},
		{"ADJOURN FOR", "durable timer: ADJOURN FOR n DAYS."},
		{"HOLD", "deliberate verdict: HOLD expr IN CONTEMPT."},
		{"STRIKE", "deletion: STRIKE x FROM THE RECORD."},
		{"SERVE NOTICE OF", "cross-case send: … expr UPON case-number."},
		{"COMMENCE PROCEEDINGS UPON", "spawn: … src, FILED UNDER x."},
		{"ENTER JUDGMENT AGAINST", "sentence a commenced case: … c, ON THE GROUNDS OF g. (parent only)"},
		{"FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE", "verdict interception, once per case"},
		{"COMMIT", "archive: COMMIT expr TO THE ARCHIVE AS \"name\"."},
		{"INCORPORATE BY REFERENCE", "import a statute (transitive)"},
		{"HEREINAFTER,", "defined term: HEREINAFTER, k SHALL MEAN literal."},
		{"THE EXHIBIT OF", "declare a struct shape: …, COMPRISING a AND b."},
		{"THE OFFICE OF", "declare a procedure: … name, CONCERNING a AND b."},
		{"THE FINDING OF", "call in expression position: … office REGARDING args"},
		{"THE STANDING OF", "another case's status, through the ledger"},
		{"THE RECORD", "discovery: THE RECORD name IN THE MATTER OF c"},
		{"THE DISCRETION OF THE COURT BETWEEN", "a random integer, ledgered"},
		{"THE DATE OF THESE PRESENTS", "the clock, in court days, ledgered"},
		{"THE CASE AT BAR", "this case's own number"},
		{"THE LENGTH OF", "characters / entries / items"},
		{"THE TRANSCRIPT OF", "any value, as a string"},
		{"THE SUM CERTAIN OF", "the number a string denotes, or a verdict"},
		{"THE ITEM AT", "schedule indexing: THE ITEM AT i IN s"},
		{"THE DOCUMENT", "from the archive: THE DOCUMENT \"name\" FROM THE ARCHIVE"},
		{"THE PRACTICE OF", "the disclosed invention, if you may"},
		{"A SCHEDULE COMPRISING", "a list literal"},
		{"AN EMPTY SCHEDULE", "the empty list"},
		{"AN EXHIBIT OF", "struct literal: … name WHEREIN a IS 1 AND b IS 2"},
		{"AN EXCERPT OF", "substring: … s FROM i TO j"},
		{"ANNEX", "append: ANNEX expr TO s."},
		{"SUBSTITUTE", "replace: SUBSTITUTE expr FOR ITEM i OF s."},
		{"INSCRIBE", "map entry: INSCRIBE v UNDER k IN r."},
		{"EXPUNGE THE ENTRY UNDER", "map removal: … k IN r."},
		{"THE ENTRY UNDER", "map lookup: … k IN r"},
		{"THE ROSTER OF", "map keys, alphabetically, as a schedule"},
		{"A REGISTER COMPRISING", "map literal: v UNDER k AND v UNDER k"},
		{"AN EMPTY REGISTER", "the map with nothing to declare"},
		{"OFF THE RECORD:", "a comment (retained in the filing topic regardless)"},
	}
	sort.Slice(out, func(i, j int) bool { return out[i].label < out[j].label })
	return out
}
