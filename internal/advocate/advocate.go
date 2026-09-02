// Package advocate implements the newline-delimited JSON-RPC MCP server.
package advocate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lennrt/trial-lang/internal/court"
	"github.com/lennrt/trial-lang/internal/deposition"
	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
	"github.com/lennrt/trial-lang/internal/law"
)

// Server processes one request at a time. The caller owns Log, In, and Out.
type Server struct {
	Log     docket.Log
	In      io.Reader
	Out     io.Writer
	Version string // reported in serverInfo
}

const (
	maxRequestBytes    = 16 << 20
	maxSourceBytes     = 4 << 20
	maxToolValues      = 1000
	maxPageSize        = 1000
	defaultPageSize    = 100
	maxToolOutputBytes = 4 << 20
	maxNarrationLines  = 1000
)

// --- JSON-RPC plumbing -------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	Meta    json.RawMessage `json:"_meta,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// toolResult is the result of one tools/call request.
type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(v any) toolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult("tool result could not be encoded: %v", err)
	}
	return boundedToolResult(string(b), false)
}

func errorResult(format string, args ...any) toolResult {
	return boundedToolResult(fmt.Sprintf(format, args...), true)
}

func boundedToolResult(message string, isError bool) toolResult {
	result := toolResult{
		Content: []toolContent{{Type: "text", Text: message}},
		IsError: isError,
	}
	if encoded, err := json.Marshal(result); err == nil && len(encoded) <= maxToolOutputBytes {
		return result
	}
	return toolResult{
		Content: []toolContent{{Type: "text", Text: fmt.Sprintf("tool result exceeds the %d-byte output limit", maxToolOutputBytes)}},
		IsError: true,
	}
}

func ambiguousCommitResult(operation, target string, err error) (toolResult, bool) {
	if _, ambiguous := errors.AsType[*docket.AmbiguousCommitError](err); !ambiguous {
		return toolResult{}, false
	}
	return errorResult("%s result is uncertain for %s: %v. It may already be committed; inspect %s before taking further action", operation, target, err, target), true
}

func recoverableCaseResult(operation string, c docket.Case, err error) (toolResult, bool) {
	if c.ID == "" {
		return toolResult{}, false
	}
	if result, ambiguous := ambiguousCommitResult(operation, "case "+c.ID, err); ambiguous {
		return result, true
	}
	return errorResult("%s failed for case %s: %v. The case may be partial; inspect case %s before retrying", operation, c.ID, err, c.ID), true
}

type boundedNarration struct {
	lines     []string
	bytes     int
	truncated bool
}

func (n *boundedNarration) add(line string) {
	if n.truncated || len(n.lines) >= maxNarrationLines || n.bytes+len(line) > maxToolOutputBytes {
		n.truncated = true
		return
	}
	n.lines = append(n.lines, line)
	n.bytes += len(line)
}

// Serve reads requests until In closes or ctx is canceled. It writes only
// protocol responses to Out.
func (s *Server) Serve(ctx context.Context) error {
	if s.Version == "" {
		s.Version = "(devel)"
	}
	enc := json.NewEncoder(s.Out)
	sc := bufio.NewScanner(s.In)
	sc.Buffer(make([]byte, 0, 64*1024), maxRequestBytes)

	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := decodeStrict(line, &req, false); err != nil {
			if encodeErr := enc.Encode(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &rpcError{Code: -32700, Message: "request could not be read: " + err.Error()}}); encodeErr != nil {
				return encodeErr
			}
			continue
		}
		resp, reply := s.handle(ctx, &req)
		if reply {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func (s *Server) handle(ctx context.Context, req *rpcRequest) (rpcResponse, bool) {
	// JSON-RPC notifications have no ID and receive no response.
	notification := len(req.ID) == 0
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string          `json:"protocolVersion"`
			Capabilities    json.RawMessage `json:"capabilities"`
			ClientInfo      json.RawMessage `json:"clientInfo"`
			Meta            json.RawMessage `json:"_meta"`
		}
		if len(req.Params) > 0 {
			if err := decodeStrict(req.Params, &params, false); err != nil {
				resp.Error = &rpcError{Code: -32602, Message: "initialize parameters could not be read: " + err.Error()}
				return resp, !notification
			}
		}
		version := params.ProtocolVersion
		if version == "" {
			version = "2025-06-18"
		}
		resp.Result = map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "trial",
				"title":   "triallang MCP server",
				"version": s.Version,
			},
			"instructions": advocateInstructions,
		}
		return resp, !notification

	case "ping":
		resp.Result = map[string]any{}
		return resp, !notification

	case "tools/list":
		resp.Result = map[string]any{"tools": toolDefs()}
		return resp, !notification

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Meta      json.RawMessage `json:"_meta"`
		}
		if err := decodeStrict(req.Params, &params, false); err != nil {
			resp.Error = &rpcError{Code: -32602, Message: "tools/call parameters could not be read"}
			return resp, !notification
		}
		resp.Result = s.call(ctx, params.Name, params.Arguments)
		return resp, !notification
	}

	if notification {
		return resp, false // initialized, cancelled, and other notifications
	}
	resp.Error = &rpcError{Code: -32601, Message: fmt.Sprintf("method %q is not supported", req.Method)}
	return resp, true
}

func decodeStrict(data []byte, target any, useNumber bool) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if useNumber {
		dec.UseNumber()
	}
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

const advocateInstructions = `triallang stores Kafka-backed case state in topics. Write programs in
legal English. Use trial_test before filing. Use trial_file to create a
case, trial_serve to add input, trial_proceed to execute a bounded
session, and trial_observe to read output. Retain the case number and
output offset. If a commit result is ambiguous, read case status before
retrying. Broker retention, access control, backup, and recovery belong
to the operator.`

// --- the tools ---------------------------------------------------------

func obj(props map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func toolDefs() []map[string]any {
	caseProp := map[string]any{"type": "string", "description": "A case number, for example case-1a2b3c4d5e6f708192a3b4c5."}
	sourceProp := map[string]any{"type": "string", "description": "Complete triallang source text."}
	return []map[string]any{
		{
			"name":        "trial_file",
			"description": "Compile and store one Form K-1 case. Returns its case number. The case does not execute until trial_proceed runs. Source is limited to 4 MiB. A minimal filing is: FORM K-1. / IN THE MATTER OF: my-plan. / ARTICLE 1. / PROCLAIM \"hello\". / ADJOURN INDEFINITELY.",
			"inputSchema": obj(map[string]any{"source": sourceProp}, "source"),
		},
		{
			"name":        "trial_proceed",
			"description": "Execute from the recorded attention for a bounded session. One court day is one second. The default is 10; the accepted range is 0.001 through 600. The call stops early on adjournment, verdict, or the current end of proceedings. If a commit result is ambiguous, read trial_status before retrying.",
			"inputSchema": obj(map[string]any{
				"case":                   caseProp,
				"for_at_most_court_days": map[string]any{"type": "number", "minimum": 0.001, "maximum": 600, "description": "Session budget in court days (seconds). Default 10."},
			}, "case"),
		},
		{
			"name":        "trial_serve",
			"description": "Append one atomic input batch to a case. Values retain order. The batch is limited to 1,000 values and 4 MiB. Integers and exact two-decimal sums remain numeric; other values are strings.",
			"inputSchema": obj(map[string]any{
				"case":   caseProp,
				"values": map[string]any{"type": "array", "minItems": 1, "maxItems": maxToolValues, "items": map[string]any{"type": []string{"string", "number"}}, "description": "Values to serve, in order."},
			}, "case", "values"),
		},
		{
			"name":        "trial_observe",
			"description": "Read a bounded page of case output from an offset. Returns record offsets and next_offset. Retain next_offset for the next call. Broker retention policy controls how long output remains available.",
			"inputSchema": obj(map[string]any{
				"case":        caseProp,
				"from_offset": map[string]any{"type": "integer", "description": "First offset to read (default 0, the beginning)."},
				"limit":       map[string]any{"type": "integer", "minimum": 1, "maximum": maxPageSize, "description": "Maximum records to return. Default 100."},
			}, "case"),
		},
		{
			"name":        "trial_status",
			"description": "Read a bounded case-status page. It includes attention, stack depth, pending calls, sorted records, continuance, and verdict state.",
			"inputSchema": obj(map[string]any{
				"case":        caseProp,
				"from_offset": map[string]any{"type": "integer", "description": "First sorted record name to return. Default 0."},
				"limit":       map[string]any{"type": "integer", "minimum": 1, "maximum": maxPageSize, "description": "Maximum records to return. Default 100."},
			}, "case"),
		},
		{
			"name":        "trial_verdict",
			"description": "Read a case verdict and its diagnostic fields. A missing verdict means that no verdict record exists.",
			"inputSchema": obj(map[string]any{"case": caseProp}, "case"),
		},
		{
			"name":        "trial_amend",
			"description": "Append one Form K-2 filing and its instructions. Existing state remains. A K-2 cannot define offices or incorporate statutes. Do not amend one case concurrently from separate processes.",
			"inputSchema": obj(map[string]any{"case": caseProp, "source": sourceProp}, "case", "source"),
		},
		{
			"name":        "trial_docket",
			"description": "List one bounded, sorted page of cases and their dispositions.",
			"inputSchema": obj(map[string]any{
				"from_offset": map[string]any{"type": "integer", "description": "First sorted matter to return. Default 0."},
				"limit":       map[string]any{"type": "integer", "minimum": 1, "maximum": maxPageSize, "description": "Maximum matters to return. Default 100."},
			}),
		},
		{
			"name":        "trial_reenact",
			"description": "Append replay-reset markers for a case. A later trial_proceed starts from the first instruction and reuses recorded summons, clock reads, and random draws. Existing history remains. Use trial_observe with the correct offset to distinguish timelines.",
			"inputSchema": obj(map[string]any{"case": caseProp}, "case"),
		},
		{
			"name":        "trial_enact",
			"description": "Compile and append one Form S-1 statute version. Later filings may use INCORPORATE BY REFERENCE <statute-name>. Existing cases keep their compiled instruction range.",
			"inputSchema": obj(map[string]any{"source": sourceProp}, "source"),
		},
		{
			"name":        "trial_statutes",
			"description": "List statute names available to INCORPORATE BY REFERENCE.",
			"inputSchema": obj(map[string]any{}),
		},
		{
			"name":        "trial_test",
			"description": "Run a program and deposition on a new in-memory log. This call does not access Kafka or create a persistent case. The deposition may SERVE values, EXPECT output or records, select an expected outcome, and ALLOW at most 600 court days.",
			"inputSchema": obj(map[string]any{
				"program_source":    map[string]any{"type": "string", "description": "The triallang program to depose."},
				"deposition_source": map[string]any{"type": "string", "description": "The deposition to hear it against."},
			}, "program_source", "deposition_source"),
		},
	}
}

// call dispatches one tool invocation. Invalid tool arguments return tool
// errors; protocol errors are handled before this call.
func (s *Server) call(ctx context.Context, name string, rawArgs json.RawMessage) toolResult {
	if err := validateToolFields(name, rawArgs); err != nil {
		return errorResult("the arguments could not be read: %v", err)
	}
	var args struct {
		Case             string   `json:"case"`
		Source           string   `json:"source"`
		Values           []any    `json:"values"`
		FromOffset       int64    `json:"from_offset"`
		Limit            int      `json:"limit"`
		Days             *float64 `json:"for_at_most_court_days"`
		ProgramSource    string   `json:"program_source"`
		DepositionSource string   `json:"deposition_source"`
	}
	if len(rawArgs) > 0 {
		if err := decodeStrict(rawArgs, &args, true); err != nil {
			return errorResult("the arguments could not be read: %v", err)
		}
	}
	if args.FromOffset < 0 {
		return errorResult("from_offset must be nonnegative")
	}
	if args.Limit < 0 || args.Limit > maxPageSize {
		return errorResult("limit must be between 1 and %d", maxPageSize)
	}
	if args.Limit == 0 {
		args.Limit = defaultPageSize
	}
	if len(args.Source) > maxSourceBytes || len(args.ProgramSource) > maxSourceBytes || len(args.DepositionSource) > maxSourceBytes {
		return errorResult("source text exceeds the %d-byte tool limit", maxSourceBytes)
	}
	var c docket.Case
	switch name {
	case "trial_proceed", "trial_serve", "trial_observe", "trial_status", "trial_verdict", "trial_amend", "trial_reenact":
		var err error
		c, err = docket.ParseCase(args.Case)
		if err != nil {
			return errorResult("%v", err)
		}
	}

	switch name {
	case "trial_file":
		filed, err := court.File(ctx, s.Log, args.Source)
		if err != nil {
			if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
				return errorResult("filing rejected pursuant to Article §4.2: %s", rej.Error())
			}
			if result, recoverable := recoverableCaseResult("filing", filed, err); recoverable {
				return result
			}
			return errorResult("filing failed: %v", err)
		}
		return textResult(map[string]any{
			"case": filed.ID,
			"note": "The case is filed. Call trial_proceed to execute it.",
		})

	case "trial_proceed":
		days := 10.0
		if args.Days != nil {
			days = *args.Days
			if days < 0.001 || days > 600 {
				return errorResult("for_at_most_court_days must be between 0.001 and 600")
			}
		}
		var narration boundedNarration
		ct := &court.Court{
			Log:      s.Log,
			Case:     c,
			Observer: narration.add,
		}
		dctx, cancel := context.WithTimeout(ctx, time.Duration(days*float64(court.CourtDay)))
		defer cancel()
		outcome, err := ct.Proceed(dctx)
		expired := dctx.Err() != nil
		if err != nil {
			if result, ambiguous := ambiguousCommitResult("execution step", "case "+c.ID, err); ambiguous {
				return result
			}
			return errorResult("session failed: %v", err)
		}
		result := map[string]any{
			"case":                c.ID,
			"outcome":             outcome.String(),
			"session_expired":     expired,
			"narration":           narration.lines,
			"narration_truncated": narration.truncated,
		}
		switch {
		case expired:
			result["note"] = "The session time limit expired. Completed steps are committed. If the case is waiting for input, call trial_serve and then trial_proceed again."
		case outcome == court.OutcomeGuilty:
			result["note"] = "The case produced a verdict. Call trial_verdict for details."
		case outcome == court.OutcomeApparentAcquittal:
			result["note"] = "Execution reached the current end of the proceedings. The case can still be amended with trial_amend."
		default:
			result["note"] = "The case is adjourned and can be resumed."
		}
		return textResult(result)

	case "trial_serve":
		if len(args.Values) == 0 {
			return errorResult("values must contain at least one input")
		}
		if len(args.Values) > maxToolValues {
			return errorResult("at most %d summonses may be served in one call", maxToolValues)
		}
		appends := make([]docket.StepAppend, len(args.Values))
		totalBytes := 0
		for i, v := range args.Values {
			text, err := summonsText(v)
			if err != nil {
				return errorResult("Summons %d could not be stated exactly: %v", i+1, err)
			}
			totalBytes += len(text)
			if len(text) > docket.MaxRecordBytes || totalBytes > maxToolOutputBytes {
				return errorResult("summons payload exceeds the %d-byte call limit", maxToolOutputBytes)
			}
			appends[i] = docket.StepAppend{Topic: c.Summons(), Value: []byte(text)}
		}
		if _, err := s.Log.AppendBatch(ctx, appends); err != nil {
			if result, ambiguous := ambiguousCommitResult("input batch", "case "+c.ID, err); ambiguous {
				return result
			}
			return errorResult("inputs could not be appended atomically: %v", err)
		}
		return textResult(map[string]any{
			"case":   c.ID,
			"served": len(args.Values),
			"note":   "The input batch was appended.",
		})

	case "trial_observe":
		next := args.FromOffset
		type procl struct {
			Offset int64  `json:"offset"`
			Text   string `json:"text"`
		}
		out := make([]procl, 0, args.Limit)
		outputBytes := 0
		for len(out) < args.Limit {
			record, err := s.Log.Fetch(ctx, c.Proclamations(), next, false)
			if err != nil {
				return errorResult("case output could not be read: %v", err)
			}
			if record == nil {
				break
			}
			if outputBytes+len(record.Value) > maxToolOutputBytes {
				if len(out) == 0 {
					return errorResult("proclamation at offset %d exceeds the %d-byte tool output limit", record.Offset, maxToolOutputBytes)
				}
				break
			}
			out = append(out, procl{Offset: record.Offset, Text: string(record.Value)})
			outputBytes += len(record.Value)
			next = record.Offset + 1
		}
		return textResult(map[string]any{
			"case":          c.ID,
			"proclamations": out,
			"next_offset":   next,
		})

	case "trial_status":
		st, err := court.Examine(ctx, s.Log, c)
		if err != nil {
			return errorResult("case status could not be read: %v", err)
		}
		names := make([]string, 0, len(st.Records))
		for name := range st.Records {
			names = append(names, name)
		}
		sort.Strings(names)
		start := pageStart(args.FromOffset, len(names))
		end := min(start+args.Limit, len(names))
		records := make(map[string]string, end-start)
		for _, name := range names[start:end] {
			records[name] = st.Records[name].Display()
		}
		result := map[string]any{
			"case":               c.ID,
			"started":            st.Started,
			"pc":                 st.PC,
			"stack_depth":        st.StackDepth,
			"appeals_depth":      st.AppealsDepth,
			"records":            records,
			"next_record_offset": end,
			"records_complete":   end == len(names),
			"guilty":             st.Verdict != nil,
		}
		if st.ContinuedUntil != nil {
			result["continued_until"] = st.ContinuedUntil.Format(time.RFC3339)
			result["note"] = "A continuance is active. trial_proceed will honor the recorded deadline."
		}
		if st.AwaitingUntil != nil {
			result["awaiting_until"] = st.AwaitingUntil.Format(time.RFC3339)
			result["note"] = "A timed input wait is active until the recorded deadline."
		}
		return textResult(result)

	case "trial_verdict":
		st, err := court.Examine(ctx, s.Log, c)
		if err != nil {
			return errorResult("case verdict could not be read: %v", err)
		}
		if st.Verdict == nil {
			return textResult(map[string]any{
				"case":    c.ID,
				"verdict": nil,
				"note":    "No verdict has been recorded.",
			})
		}
		return textResult(map[string]any{
			"case":    c.ID,
			"verdict": "GUILTY",
			"counsel": map[string]any{
				"sealed": st.Verdict.Sealed,
				"pc":     st.Verdict.PC,
				"pos":    st.Verdict.Pos,
			},
			"note": "The verdict is final. The case can still be reenacted.",
		})

	case "trial_amend":
		n, err := court.Amend(ctx, s.Log, c, args.Source)
		if err != nil {
			if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
				return errorResult("supplemental filing rejected pursuant to Article §4.2: %s", rej.Error())
			}
			if result, ambiguous := ambiguousCommitResult("supplemental filing", "case "+c.ID, err); ambiguous {
				return result
			}
			return errorResult("supplemental filing failed: %v", err)
		}
		return textResult(map[string]any{
			"case":                 c.ID,
			"instructions_entered": n,
			"note":                 "The supplemental instructions were appended. Call trial_proceed to resume.",
		})

	case "trial_docket":
		cases, err := s.Log.ListCases(ctx)
		if err != nil {
			return errorResult("docket could not be read: %v", err)
		}
		type entry struct {
			Case        string `json:"case"`
			Disposition string `json:"disposition"`
		}
		start := pageStart(args.FromOffset, len(cases))
		end := min(start+args.Limit, len(cases))
		out := make([]entry, 0, end-start)
		for _, dc := range cases[start:end] {
			st, err := court.Examine(ctx, s.Log, dc)
			switch {
			case err != nil:
				out = append(out, entry{dc.ID, "status unavailable"})
			case st.Verdict != nil:
				out = append(out, entry{dc.ID, "guilty"})
			case st.Started:
				out = append(out, entry{dc.ID, fmt.Sprintf("in proceedings; attention at instruction %d", st.PC)})
			default:
				out = append(out, entry{dc.ID, "filed"})
			}
		}
		return textResult(map[string]any{
			"matters":     out,
			"next_offset": end,
			"complete":    end == len(cases),
		})

	case "trial_reenact":
		if err := court.Reenact(ctx, s.Log, c); err != nil {
			if result, ambiguous := ambiguousCommitResult("replay reset", "case "+c.ID, err); ambiguous {
				return result
			}
			return errorResult("reenactment failed: %v", err)
		}
		return textResult(map[string]any{
			"case": c.ID,
			"note": "The replay markers were appended. Call trial_proceed to replay the case from its recorded inputs, clock readings, and random draws.",
		})

	case "trial_enact":
		statute, n, err := court.Enact(ctx, s.Log, args.Source)
		if err != nil {
			if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
				return errorResult("statute rejected pursuant to Article §4.2: %s", rej.Error())
			}
			if statute != "" {
				target := fmt.Sprintf("statute %s (enactment %d)", statute, n)
				if result, ambiguous := ambiguousCommitResult("enactment", target, err); ambiguous {
					return result
				}
			}
			return errorResult("statute enactment failed: %v", err)
		}
		return textResult(map[string]any{
			"statute":   statute,
			"enactment": n,
			"note":      fmt.Sprintf("New cases can incorporate it with: INCORPORATE BY REFERENCE %s.", statute),
		})

	case "trial_statutes":
		names, err := s.Log.ListStatutes(ctx)
		if err != nil {
			return errorResult("statutes could not be listed: %v", err)
		}
		return textResult(map[string]any{"statutes": names})

	case "trial_test":
		dep, err := deposition.Parse(args.DepositionSource)
		if err != nil {
			return errorResult("deposition could not be parsed: %v", err)
		}
		res := deposition.Run(ctx, args.ProgramSource, dep)
		result := map[string]any{
			"consistent":      res.OK(),
			"contradictions":  res.Contradictions,
			"elapsed_seconds": res.Elapsed.Seconds(),
		}
		if res.OK() {
			result["note"] = "The program matched the deposition."
		}
		return textResult(result)
	}

	return errorResult("unknown tool %q; consult tools/list", name)
}

func validateToolFields(name string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := decodeStrict(raw, &fields, false); err != nil {
		return err
	}
	var allowed []string
	switch name {
	case "trial_amend":
		allowed = []string{"case", "source"}
	case "trial_file", "trial_enact":
		allowed = []string{"source"}
	case "trial_proceed":
		allowed = []string{"case", "for_at_most_court_days"}
	case "trial_serve":
		allowed = []string{"case", "values"}
	case "trial_observe", "trial_status":
		allowed = []string{"case", "from_offset", "limit"}
	case "trial_verdict", "trial_reenact":
		allowed = []string{"case"}
	case "trial_docket":
		allowed = []string{"from_offset", "limit"}
	case "trial_statutes":
	case "trial_test":
		allowed = []string{"program_source", "deposition_source"}
	default:
		return nil
	}
	for field := range fields {
		if !slices.Contains(allowed, field) {
			return fmt.Errorf("unknown field %q for %s", field, name)
		}
	}
	return nil
}

func pageStart(offset int64, length int) int {
	if offset >= int64(length) {
		return length
	}
	return int(offset)
}

// summonsText preserves the spelling and precision of JSON numbers. In
// particular, decoding through float64 would silently change integers beyond
// 2^53 before they ever reached the language. Numeric summonses are integers
// or sums stated to at most the penny; callers can always use a JSON string
// when they intend arbitrary text.
func summonsText(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		s := t.String()
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			return "", fmt.Errorf("%q is not a valid JSON number", s)
		}
		// A decimal point is the caller's declaration that this is a
		// sum, including 12.00. Exponent notation without a point is an
		// integer when its exact value is integral; otherwise it may still
		// be an exact sum (1e-2 is 0.01).
		if !strings.Contains(s, ".") && r.IsInt() && r.Num().IsInt64() {
			return r.Num().String(), nil
		}
		pennies := new(big.Rat).Mul(r, big.NewRat(law.SumScale, 1))
		if !pennies.IsInt() {
			return "", fmt.Errorf("%q is finer than a sum stated to the penny", s)
		}
		if !pennies.Num().IsInt64() {
			return "", fmt.Errorf("%q is outside the range of a sum stated to the penny", t)
		}
		return law.Sum(pennies.Num().Int64()).Display(), nil
	default:
		return "", fmt.Errorf("values must be JSON strings or numbers")
	}
}
