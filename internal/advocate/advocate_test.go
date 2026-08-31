package advocate

// These tests run complete MCP sessions over pipes and an in-memory log.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

type session struct {
	t   *testing.T
	enc *json.Encoder
	dec *json.Decoder
	id  int
}

func open(t *testing.T) *session {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := &Server{Log: docket.NewMemoryLog(), In: inR, Out: outW}
	done := make(chan error, 1)
	go func() {
		done <- errors.Join(srv.Serve(t.Context()), outW.Close())
	}()
	t.Cleanup(func() {
		if err := inW.Close(); err != nil {
			t.Errorf("close MCP input: %v", err)
		}
		if err := <-done; err != nil {
			t.Errorf("serve MCP session: %v", err)
		}
		if err := outR.Close(); err != nil {
			t.Errorf("close MCP output: %v", err)
		}
	})
	return &session{t: t, enc: json.NewEncoder(inW), dec: json.NewDecoder(outR)}
}

// rpc sends one request and decodes the response's result.
func (s *session) rpc(method string, params any) map[string]any {
	s.t.Helper()
	s.id++
	req := map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if err := s.enc.Encode(req); err != nil {
		s.t.Fatal(err)
	}
	var resp struct {
		ID     int             `json:"id"`
		Result map[string]any  `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := s.dec.Decode(&resp); err != nil {
		s.t.Fatal(err)
	}
	if resp.ID != s.id {
		s.t.Fatalf("response id %d for request %d; the mail has been misdelivered", resp.ID, s.id)
	}
	if len(resp.Error) > 0 {
		s.t.Fatalf("rpc %s: %s", method, resp.Error)
	}
	return resp.Result
}

// call invokes a tool and returns its decoded JSON payload and the
// isError flag.
func (s *session) call(name string, args any) (map[string]any, bool) {
	s.t.Helper()
	res := s.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	isErr, _ := res["isError"].(bool)
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if isErr {
		return map[string]any{"error": text}, true
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		s.t.Fatalf("tool %s returned unparseable text: %q", name, text)
	}
	return payload, false
}

func TestAdvocateProtocol(t *testing.T) {
	s := open(t)

	init := s.rpc("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})
	info := init["serverInfo"].(map[string]any)
	if info["name"] != "trial" {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}
	// The initialized notification receives, and requires, no reply.
	if err := s.enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		t.Fatal(err)
	}

	if s.rpc("ping", nil) == nil {
		t.Fatal("ping went unanswered")
	}

	tools := s.rpc("tools/list", nil)["tools"].([]any)
	if len(tools) != 12 {
		t.Fatalf("the Advocate offers %d tools; 12 were promised", len(tools))
	}

	// An unknown tool is an error result, not a protocol error.
	res, isErr := s.call("trial_appeal", nil)
	if !isErr || !strings.Contains(res["error"].(string), "not a tool of this court") {
		t.Fatalf("expected a tool error, got %v", res)
	}

	// A rejected filing returns the particulars; agents have counsel.
	res, isErr = s.call("trial_file", map[string]any{"source": "FORM K-9.\n"})
	if !isErr || !strings.Contains(res["error"].(string), "§4.2") {
		t.Fatalf("expected an Article §4.2 rejection, got %v", res)
	}
}

func TestServePreservesExactJSONNumbers(t *testing.T) {
	log := docket.NewMemoryLog()
	caseFile := docket.Case{ID: "case-000000000000000000000001"}
	if err := log.EnsureTopic(context.Background(), caseFile.Summons()); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Log: log}
	result := srv.call(context.Background(), "trial_serve", json.RawMessage(
		`{"case":"case-000000000000000000000001","values":[9007199254740993,12.5,12.00,1e3,1e-2,-92233720368547758.08]}`,
	))
	if result.IsError {
		t.Fatalf("exact service was refused: %+v", result)
	}
	recs, err := log.ReadAll(context.Background(), caseFile.Summons())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"9007199254740993", "12.50", "12.00", "1000", "0.01", "-92233720368547758.08"}
	if len(recs) != len(want) {
		t.Fatalf("served %d summonses, want %d", len(recs), len(want))
	}
	for i := range recs {
		if got := string(recs[i].Value); got != want[i] {
			t.Errorf("summons %d = %q, want %q", i+1, got, want[i])
		}
	}
}

func TestServeRejectsImpreciseBatchBeforeWriting(t *testing.T) {
	log := docket.NewMemoryLog()
	caseFile := docket.Case{ID: "case-000000000000000000000002"}
	if err := log.EnsureTopic(context.Background(), caseFile.Summons()); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Log: log}
	result := srv.call(context.Background(), "trial_serve", json.RawMessage(
		`{"case":"case-000000000000000000000002","values":[1,1.234]}`,
	))
	if !result.IsError {
		t.Fatalf("imprecise service was accepted: %+v", result)
	}
	recs, err := log.ReadAll(context.Background(), caseFile.Summons())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("an invalid batch was partly served: %+v", recs)
	}
}

func TestToolArgumentsFailClosed(t *testing.T) {
	srv := &Server{Log: docket.NewMemoryLog()}
	for name, raw := range map[string]json.RawMessage{
		"unknown field":  json.RawMessage(`{"source":"x","case":"case-000000000000000000000001"}`),
		"trailing value": json.RawMessage(`{"source":"x"} {"source":"y"}`),
	} {
		result := srv.call(t.Context(), "trial_file", raw)
		if !result.IsError {
			t.Errorf("%s was accepted: %+v", name, result)
		}
	}
	result := srv.call(t.Context(), "trial_proceed", json.RawMessage(
		`{"case":"case-000000000000000000000001","for_at_most_court_days":601}`,
	))
	if !result.IsError {
		t.Fatalf("oversized session was accepted: %+v", result)
	}
}

func TestObservePaginates(t *testing.T) {
	log := docket.NewMemoryLog()
	caseFile := docket.Case{ID: "case-000000000000000000000003"}
	if err := log.EnsureTopic(t.Context(), caseFile.Proclamations()); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"one", "two", "three"} {
		if _, err := log.Append(t.Context(), caseFile.Proclamations(), nil, []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	srv := &Server{Log: log}
	result := srv.call(t.Context(), "trial_observe", json.RawMessage(
		`{"case":"case-000000000000000000000003","limit":2}`,
	))
	if result.IsError {
		t.Fatalf("observe failed: %+v", result)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatal(err)
	}
	if got := len(payload["proclamations"].([]any)); got != 2 {
		t.Fatalf("page has %d records, want 2", got)
	}
	if payload["next_offset"] != float64(2) {
		t.Fatalf("next_offset = %v", payload["next_offset"])
	}
}

func TestToolSourceAndBatchBounds(t *testing.T) {
	log := docket.NewMemoryLog()
	srv := &Server{Log: log}

	tooLargeSource, err := json.Marshal(map[string]string{
		"source": strings.Repeat("x", maxSourceBytes+1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result := srv.call(t.Context(), "trial_file", tooLargeSource); !result.IsError {
		t.Fatal("oversized source was accepted")
	}

	caseFile := docket.Case{ID: "case-000000000000000000000004"}
	if err := log.EnsureTopic(t.Context(), caseFile.Summons()); err != nil {
		t.Fatal(err)
	}
	tooMany := make([]string, maxToolValues+1)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	tooManyArgs, err := json.Marshal(map[string]any{"case": caseFile.ID, "values": tooMany})
	if err != nil {
		t.Fatal(err)
	}
	if result := srv.call(t.Context(), "trial_serve", tooManyArgs); !result.IsError {
		t.Fatal("oversized value count was accepted")
	}

	largeValue := strings.Repeat("x", maxToolOutputBytes/2+1)
	tooManyBytes, err := json.Marshal(map[string]any{
		"case":   caseFile.ID,
		"values": []string{largeValue, largeValue},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result := srv.call(t.Context(), "trial_serve", tooManyBytes); !result.IsError {
		t.Fatal("oversized value bytes were accepted")
	}
	records, err := log.ReadAll(t.Context(), caseFile.Summons())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("rejected batches wrote %d summonses", len(records))
	}
}

func TestBoundedNarration(t *testing.T) {
	var byLines boundedNarration
	for range maxNarrationLines {
		byLines.add("x")
	}
	if byLines.truncated || len(byLines.lines) != maxNarrationLines {
		t.Fatalf("narration at line limit = %+v", byLines)
	}
	byLines.add("x")
	if !byLines.truncated || len(byLines.lines) != maxNarrationLines {
		t.Fatalf("narration above line limit = %+v", byLines)
	}

	var byBytes boundedNarration
	byBytes.add(strings.Repeat("x", maxToolOutputBytes+1))
	if !byBytes.truncated || len(byBytes.lines) != 0 {
		t.Fatalf("narration above byte limit = lines %d, truncated %v", len(byBytes.lines), byBytes.truncated)
	}
}

// TestAdvocateAgentBus checks a bounded request-and-reply flow between two
// cases. The client retains only their case numbers between calls.
func TestAdvocateAgentBus(t *testing.T) {
	s := open(t)
	s.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18"})

	// The oracle: a service. It answers any petitioner, forever.
	oracle, isErr := s.call("trial_file", map[string]any{"source": `FORM K-1.
IN THE MATTER OF: the-oracle.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER petitioner.
    AWAIT SUMMONS, FILED UNDER n.
    SERVE NOTICE OF n PLUS 1 UPON petitioner.
    REFER TO ARTICLE 1.
`})
	if isErr {
		t.Fatalf("the oracle was rejected: %v", oracle)
	}
	oracleID := oracle["case"].(string)

	// The petitioner: sends its own case number as a return address,
	// then a question, then waits for the reply.
	petitioner, isErr := s.call("trial_file", map[string]any{"source": `FORM K-1.
IN THE MATTER OF: the-petitioner.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER the-oracle.
    SERVE NOTICE OF THE CASE AT BAR UPON the-oracle.
    SERVE NOTICE OF 20 UPON the-oracle.
    AWAIT SUMMONS, FILED UNDER reply.
    PROCLAIM "The oracle answers: " PLUS THE TRANSCRIPT OF reply.
    ADJOURN INDEFINITELY.
`})
	if isErr {
		t.Fatalf("the petitioner was rejected: %v", petitioner)
	}
	petitionerID := petitioner["case"].(string)

	// Introduce them: serve the petitioner the oracle's case number.
	if res, isErr := s.call("trial_serve", map[string]any{
		"case": petitionerID, "values": []any{oracleID},
	}); isErr {
		t.Fatalf("service failed: %v", res)
	}

	// Session 1: the petitioner runs until it blocks awaiting the
	// reply; the session expires; everything done so far is committed.
	res, _ := s.call("trial_proceed", map[string]any{
		"case": petitionerID, "for_at_most_court_days": 0.5,
	})
	if res["session_expired"] != true {
		t.Fatalf("expected the petitioner to block awaiting the reply: %v", res)
	}

	// Session 2: the oracle consumes both notices, answers, loops, and
	// blocks awaiting the next petitioner.
	res, _ = s.call("trial_proceed", map[string]any{
		"case": oracleID, "for_at_most_court_days": 0.5,
	})
	if res["session_expired"] != true {
		t.Fatalf("expected the oracle to block awaiting further petitioners: %v", res)
	}

	// Session 3: the petitioner finds the reply waiting and adjourns.
	res, _ = s.call("trial_proceed", map[string]any{
		"case": petitionerID, "for_at_most_court_days": 5,
	})
	if res["session_expired"] != false || res["outcome"] != "adjourned indefinitely" {
		t.Fatalf("expected adjournment, got %v", res)
	}

	// The durable stdout.
	obs, _ := s.call("trial_observe", map[string]any{"case": petitionerID})
	procs := obs["proclamations"].([]any)
	if len(procs) != 1 {
		t.Fatalf("proclamations = %v", procs)
	}
	if text := procs[0].(map[string]any)["text"]; text != "The oracle answers: 21" {
		t.Fatalf("the oracle answered %q", text)
	}

	// The subpoenaable memory: the oracle's records show exactly what
	// it was told and by whom it was petitioned.
	st, _ := s.call("trial_status", map[string]any{"case": oracleID})
	records := st["records"].(map[string]any)
	if records["petitioner"] != petitionerID || records["n"] != "20" {
		t.Fatalf("the oracle's memory is not as subpoenaed: %v", records)
	}

	// The docket sees both matters.
	dk, _ := s.call("trial_docket", nil)
	if matters := dk["matters"].([]any); len(matters) != 2 {
		t.Fatalf("the docket holds %d matters, want 2", len(matters))
	}

	// No verdict anywhere, which is not the same as innocence.
	v, _ := s.call("trial_verdict", map[string]any{"case": petitionerID})
	if v["verdict"] != nil {
		t.Fatalf("an unexpected verdict: %v", v)
	}
}

// TestAdvocateCommencement checks that a case can create and coordinate a child.
func TestAdvocateCommencement(t *testing.T) {
	s := open(t)
	s.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18"})

	filed, isErr := s.call("trial_file", map[string]any{"source": `FORM K-1.
IN THE MATTER OF: the-coordinator.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON
        "FORM K-1. IN THE MATTER OF: the-clerk. ARTICLE 1. AWAIT SUMMONS, FILED UNDER employer. AWAIT SUMMONS, FILED UNDER n. SERVE NOTICE OF n TIMES 2 UPON employer. ADJOURN INDEFINITELY.",
        FILED UNDER clerk.
    SERVE NOTICE OF THE CASE AT BAR UPON clerk.
    SERVE NOTICE OF 21 UPON clerk.
    AWAIT SUMMONS, FILED UNDER answer.
    PROCLAIM "The clerk reports: " PLUS THE TRANSCRIPT OF answer.
    ADJOURN INDEFINITELY.
`})
	if isErr {
		t.Fatalf("the coordinator was rejected: %v", filed)
	}
	coordID := filed["case"].(string)

	// Session 1: the coordinator hires, briefs the hire, and blocks
	// awaiting the answer.
	res, _ := s.call("trial_proceed", map[string]any{
		"case": coordID, "for_at_most_court_days": 0.5,
	})
	if res["session_expired"] != true {
		t.Fatalf("expected the coordinator to block awaiting the answer: %v", res)
	}

	// The docket now holds a matter no one filed by hand.
	dk, _ := s.call("trial_docket", nil)
	if matters := dk["matters"].([]any); len(matters) != 2 {
		t.Fatalf("the docket holds %d matters, want 2", len(matters))
	}

	// The agent learns the hire's case number the way it learns
	// everything: by reading the record.
	st, _ := s.call("trial_status", map[string]any{"case": coordID})
	clerkID, _ := st["records"].(map[string]any)["clerk"].(string)
	if clerkID == "" {
		t.Fatalf("the coordinator's records do not name the clerk: %v", st["records"])
	}

	// Session 2: the clerk answers and adjourns.
	res, _ = s.call("trial_proceed", map[string]any{"case": clerkID})
	if res["outcome"] != "adjourned indefinitely" {
		t.Fatalf("the clerk: %v", res)
	}

	// Session 3: the coordinator finds the answer waiting.
	res, _ = s.call("trial_proceed", map[string]any{"case": coordID})
	if res["session_expired"] != false || res["outcome"] != "adjourned indefinitely" {
		t.Fatalf("the coordinator: %v", res)
	}

	obs, _ := s.call("trial_observe", map[string]any{"case": coordID})
	procs := obs["proclamations"].([]any)
	if len(procs) != 1 || procs[0].(map[string]any)["text"] != "The clerk reports: 42" {
		t.Fatalf("proclamations = %v", procs)
	}
}

// TestAdvocateStatutesAndTests checks brokerless tests and statutes.
func TestAdvocateStatutesAndTests(t *testing.T) {
	s := open(t)
	s.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18"})

	// Depose before filing: the dry run touches no case, no broker.
	res, isErr := s.call("trial_test", map[string]any{
		"program_source": `FORM K-1.
IN THE MATTER OF: rehearsal.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM n TIMES 2.
    ADJOURN INDEFINITELY.
`,
		"deposition_source": `DEPOSITION OF: rehearsal.trial.
SERVE: 21.
EXPECT PROCLAMATION: 42.
EXPECT ADJOURNMENT.
`,
	})
	if isErr || res["consistent"] != true {
		t.Fatalf("the rehearsal contradicted itself: %v", res)
	}
	// And nothing was filed.
	dk, _ := s.call("trial_docket", nil)
	if matters := dk["matters"].([]any); len(matters) != 0 {
		t.Fatalf("the dry run left %d matters on the docket", len(matters))
	}

	// Enact a statute, list it, incorporate it.
	res, isErr = s.call("trial_enact", map[string]any{"source": `FORM S-1.
IN THE MATTER OF: the-statutes-of-doubling.
THE OFFICE OF doubling, CONCERNING n.
    REMAND WITH n TIMES 2.
`})
	if isErr || res["statute"] != "the-statutes-of-doubling" {
		t.Fatalf("enactment failed: %v", res)
	}
	sts, _ := s.call("trial_statutes", nil)
	if list := sts["statutes"].([]any); len(list) != 1 || list[0] != "the-statutes-of-doubling" {
		t.Fatalf("statutes = %v", list)
	}

	filed, isErr := s.call("trial_file", map[string]any{"source": `FORM K-1.
IN THE MATTER OF: borrower.
INCORPORATE BY REFERENCE the-statutes-of-doubling.
ARTICLE 1.
    PROCLAIM THE FINDING OF doubling REGARDING 21.
    ADJOURN INDEFINITELY.
`})
	if isErr {
		t.Fatalf("the incorporating filing was rejected: %v", filed)
	}
	caseID := filed["case"].(string)
	if res, _ = s.call("trial_proceed", map[string]any{"case": caseID}); res["outcome"] != "adjourned indefinitely" {
		t.Fatalf("proceed: %v", res)
	}
	obs, _ := s.call("trial_observe", map[string]any{"case": caseID})
	if text := obs["proclamations"].([]any)[0].(map[string]any)["text"]; text != "42" {
		t.Fatalf("the statute did not apply: %v", text)
	}
}

// TestAdvocateAmendAndReenact checks amendment and deterministic replay.
func TestAdvocateAmendAndReenact(t *testing.T) {
	s := open(t)
	s.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18"})

	filed, _ := s.call("trial_file", map[string]any{"source": `FORM K-1.
IN THE MATTER OF: extendable.
ARTICLE 1.
    LET IT BE RECORDED THAT roll IS THE DISCRETION OF THE COURT BETWEEN 1 AND 1000000000.
    PROCLAIM roll.
`})
	caseID := filed["case"].(string)
	if res, _ := s.call("trial_proceed", map[string]any{"case": caseID}); res["outcome"] != "apparently acquitted (do not celebrate)" {
		t.Fatalf("expected apparent acquittal, got %v", res)
	}

	// Extend the plan: proclaim the same roll again.
	if res, isErr := s.call("trial_amend", map[string]any{"case": caseID, "source": `FORM K-2.
IN THE MATTER OF: extendable.
ARTICLE 1.
    PROCLAIM roll.
`}); isErr {
		t.Fatalf("the supplement was refused: %v", res)
	}
	s.call("trial_proceed", map[string]any{"case": caseID})

	obs, _ := s.call("trial_observe", map[string]any{"case": caseID})
	procs := obs["proclamations"].([]any)
	if len(procs) != 2 {
		t.Fatalf("proclamations = %v", procs)
	}
	first := procs[0].(map[string]any)["text"]
	if second := procs[1].(map[string]any)["text"]; first != second {
		t.Fatalf("the record disagrees with itself: %v then %v", first, second)
	}

	// Reenact: the same draw, a third and fourth time.
	s.call("trial_reenact", map[string]any{"case": caseID})
	s.call("trial_proceed", map[string]any{"case": caseID})
	obs, _ = s.call("trial_observe", map[string]any{"case": caseID, "from_offset": 0})
	procs = obs["proclamations"].([]any)
	if len(procs) != 4 {
		t.Fatalf("after reenactment, proclamations = %v", procs)
	}
	for _, p := range procs {
		if p.(map[string]any)["text"] != first {
			t.Fatalf("the reenactment diverged: %v vs %v", p, first)
		}
	}
}
