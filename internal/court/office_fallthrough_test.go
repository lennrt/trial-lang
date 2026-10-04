package court

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestOfficeBodiesDoNotFallThroughAcrossSessions(t *testing.T) {
	for _, explicitEnd := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit-end-%t", explicitEnd), func(t *testing.T) {
			ending := ""
			if explicitEnd {
				ending = "ADJOURN INDEFINITELY."
			}
			log, c := convene(t, `FORM K-1.
IN THE MATTER OF: office-suspension.
ARTICLE 1.
    LET IT BE RECORDED THAT proxy IS A POWER OF ATTORNEY OVER THE OFFICE OF outer.
    PROCLAIM THE FINDING UNDER proxy REGARDING 6.
    ADJOURN INDEFINITELY.
    PROCLAIM "after main adjournment".
`+ending+`
THE OFFICE OF outer, CONCERNING amount.
    ADJOURN INDEFINITELY.
    REMAND WITH THE FINDING OF doubled REGARDING amount.
THE OFFICE OF doubled, CONCERNING amount.
    REMAND WITH amount TIMES 2.
`)
			t.Cleanup(log.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			adjournments := 3 // inside office, intermediate main, implicit boundary
			if explicitEnd {
				adjournments++
			}
			for session := range adjournments + 3 {
				ct := &Court{Log: log, Case: c}
				outcome, err := ct.Proceed(ctx)
				want := OutcomeAdjourned
				if session >= adjournments {
					want = OutcomeApparentAcquittal
				}
				if err != nil || outcome != want {
					t.Fatalf("session %d = %v, %v; want %v", session, outcome, err, want)
				}
				if session == 0 && len(ct.frames) != 1 {
					t.Fatalf("office suspension lost its call frame: %d", len(ct.frames))
				}
			}
			if got := proclamations(t, log, c); !slices.Equal(got, []string{"12", "after main adjournment"}) {
				t.Fatalf("unexpected office execution: %q", got)
			}
			report, err := Audit(ctx, log, c)
			if err != nil || !report.Consistent() {
				t.Fatalf("resumed office audit: %+v, %v", report, err)
			}
		})
	}
}

const officeAmendmentSource = `FORM K-1.
IN THE MATTER OF: office-amendment.
ARTICLE 1.
    LET IT BE RECORDED THAT proxy IS A POWER OF ATTORNEY OVER THE OFFICE OF doubled.
    PROCLAIM THE FINDING UNDER proxy REGARDING 21.
THE OFFICE OF doubled, CONCERNING amount.
    REMAND WITH amount TIMES 2.
`

func TestOfficeGuardReachesAmendments(t *testing.T) {
	for _, imported := range []bool{false, true} {
		for _, alreadyAtEnd := range []bool{false, true} {
			t.Run(fmt.Sprintf("imported-%t/already-at-end-%t", imported, alreadyAtEnd), func(t *testing.T) {
				log := docket.NewMemoryLog()
				t.Cleanup(log.Close)
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				source := officeAmendmentSource
				if imported {
					if _, _, err := Enact(ctx, log, `FORM S-1. IN THE MATTER OF: office-library.
THE OFFICE OF doubled, CONCERNING amount. REMAND WITH amount TIMES 2.`); err != nil {
						t.Fatal(err)
					}
					source, _, _ = strings.Cut(source, "THE OFFICE OF doubled, CONCERNING")
					source = strings.Replace(source, "ARTICLE 1.", "INCORPORATE BY REFERENCE office-library. ARTICLE 1.", 1)
				}
				c, err := File(ctx, log, source)
				if err != nil {
					t.Fatal(err)
				}
				ct := &Court{Log: log, Case: c, Expedite: 7}
				if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
					t.Fatalf("initial session: %v, %v", out, err)
				}
				if alreadyAtEnd {
					if out, err := ct.Proceed(ctx); err != nil || out != OutcomeApparentAcquittal {
						t.Fatalf("reach end: %v, %v", out, err)
					}
				}
				if _, err := Amend(ctx, log, c, `FORM K-2.
IN THE MATTER OF: office-amendment.
ARTICLE 1.
    REFER TO ARTICLE 2.
ARTICLE 2.
    PROCLAIM THE FINDING UNDER proxy REGARDING 9.
    PROCLAIM "amended".
    ADJOURN INDEFINITELY.
`); err != nil {
					t.Fatal(err)
				}
				ct = &Court{Log: log, Case: c, Expedite: 7}
				if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
					t.Fatalf("amendment session: %v, %v", out, err)
				}
				if out, err := ct.Proceed(ctx); err != nil || out != OutcomeApparentAcquittal {
					t.Fatalf("amendment end: %v, %v", out, err)
				}
				if got := proclamations(t, log, c); !slices.Equal(got, []string{"42", "18", "amended"}) {
					t.Fatalf("amendment output = %q", got)
				}
				report, err := Audit(ctx, log, c)
				if err != nil || !report.Consistent() {
					t.Fatalf("amended office audit: %+v, %v", report, err)
				}
			})
		}
	}
}

func TestDocketChildOfficeRemainsInGoodStanding(t *testing.T) {
	childText := strings.ReplaceAll(strings.TrimSpace(officeAmendmentSource), "\n", " ")
	log, parent := convene(t, `FORM K-1. IN THE MATTER OF: office-parent. ARTICLE 1.
COMMENCE PROCEEDINGS UPON "`+childText+`", FILED UNDER child. ADJOURN INDEFINITELY.`)
	t.Cleanup(log.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ct := &Court{Log: log, Case: parent}
	if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
		t.Fatalf("parent: %v, %v", out, err)
	}
	child, err := docket.ParseCase(ct.globals["child"].S)
	if err != nil {
		t.Fatal(err)
	}
	code, err := log.ReadAll(ctx, child.Proceedings())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- ServeDocket(ctx, log, DocketOptions{Poll: time.Millisecond, Skip: func(c docket.Case) bool { return c.ID == parent.ID }})
	}()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("docket did not stop after cancellation")
		}
	}()
	for {
		state, err := Examine(ctx, log, child)
		if err != nil {
			t.Fatal(err)
		}
		if state.Verdict != nil {
			t.Fatalf("completed child entered its office without a petition: %+v", state.Verdict)
		}
		if state.PC == int64(len(code)) && state.AppealsDepth == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("child did not reach the end of its filing")
		case <-time.After(time.Millisecond):
		}
	}
	if got := proclamations(t, log, child); !slices.Equal(got, []string{"42"}) {
		t.Fatalf("child office output = %q", got)
	}
}

func TestLegacyOfficeBytecodeIsNotReinterpreted(t *testing.T) {
	log := docket.NewMemoryLog()
	t.Cleanup(log.Close)
	c := docket.Case{ID: "case-000000000000000000000017"}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := log.CreateCaseTopics(ctx, c); err != nil {
		t.Fatal(err)
	}
	// This is the old compiler layout: implicit ADJOURN immediately followed
	// by an office. The runtime must continue executing the stored opcodes.
	for _, instruction := range []string{
		`{"op":"SUBMIT","value":{"t":"str","s":"main"}}`,
		`{"op":"PROCLAIM"}`, `{"op":"ADJOURN"}`,
		`{"op":"SUBMIT","value":{"t":"str","s":"legacy office"}}`,
		`{"op":"PROCLAIM"}`, `{"op":"REMAND"}`,
	} {
		if _, err := log.Append(ctx, c.Proceedings(), nil, []byte(instruction)); err != nil {
			t.Fatal(err)
		}
	}
	ct := &Court{Log: log, Case: c}
	if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
		t.Fatalf("legacy first session: %v, %v", out, err)
	}
	if out, err := ct.Proceed(ctx); err != nil || out != OutcomeGuilty {
		t.Fatalf("legacy second session: %v, %v", out, err)
	}
	if got := proclamations(t, log, c); !slices.Equal(got, []string{"main", "legacy office"}) {
		t.Fatalf("stored legacy bytecode changed behavior: %q", got)
	}
}
