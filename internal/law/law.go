// Package law defines triallang instructions and runtime values.
package law

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Opcode strings are stable JSON wire values in the proceedings topic.
const (
	OpSubmit          = "SUBMIT"           // push a literal onto the dossier
	OpRetrieve        = "RETRIEVE"         // push the record filed under Name
	OpFile            = "FILE"             // pop and file under Name
	OpCombine         = "COMBINE"          // + (joinder, for strings)
	OpDeduct          = "DEDUCT"           // -
	OpCompound        = "COMPOUND"         // *
	OpApportion       = "APPORTION"        // integer division, toward zero
	OpNotwithstanding = "NOTWITHSTANDING"  // remainder
	OpExceeds         = "EXCEEDS"          // pop R, L; push finding L > R
	OpFallsShort      = "FALLS-SHORT"      // pop R, L; push finding L < R
	OpEquals          = "EQUALS"           // pop R, L; push finding L = R
	OpDiffers         = "DIFFERS"          // pop R, L; push finding L ≠ R
	OpOverturn        = "OVERTURN"         // negate the finding atop the dossier
	OpRefer           = "REFER"            // jump to Target
	OpReferOverruled  = "REFER-OVERRULED"  // pop finding; jump to Target if OVERRULED
	OpProclaim        = "PROCLAIM"         // pop; append to proclamations
	OpAwait           = "AWAIT"            // block on summons; push what is served
	OpPetition        = "PETITION"         // call the office at Target
	OpRemand          = "REMAND"           // return; With says whether a finding accompanies it
	OpAdjourn         = "ADJOURN"          // indefinite postponement
	OpExhibit         = "EXHIBIT"          // pop len(Params) values; push an exhibit of Name
	OpInspect         = "INSPECT"          // pop an exhibit; push its entry Name
	OpEnter           = "ENTER"            // pop value, pop exhibit; push the exhibit with entry Name replaced
	OpConsolidate     = "CONSOLIDATE"      // pop two findings; push their conjunction (AND ALSO)
	OpAlternative     = "ALTERNATIVE"      // pop two findings; push their disjunction (OR IN THE ALTERNATIVE)
	OpMeasure         = "MEASURE"          // pop a string or exhibit; push its length (THE LENGTH OF)
	OpExcerpt         = "EXCERPT"          // pop j, i, s; push s[i..j], 1-indexed, inclusive (AN EXCERPT OF)
	OpTranscribe      = "TRANSCRIBE"       // pop any value; push its display string (THE TRANSCRIPT OF)
	OpSumCertain      = "SUM-CERTAIN"      // pop a string or int; push the integer it denotes (THE SUM CERTAIN OF)
	OpContempt        = "CONTEMPT"         // pop a value and produce a verdict with it as the particulars
	OpStrike          = "STRIKE"           // strike the record Name: a tombstone in the records topic
	OpServe           = "SERVE"            // pop respondent case number, pop notice; append the notice to the respondent's summons topic
	OpCaseAtBar       = "CASE-AT-BAR"      // push this case's own case number, as a string
	OpContinuance     = "CONTINUANCE"      // pop a number of days; the matter is continued (a durable timer)
	OpDiscretion      = "DISCRETION"       // pop upper, lower; push an integer selected at the Court's discretion, inclusive
	OpPresents        = "DATE-OF-PRESENTS" // push the current court day since the epoch
	OpSchedule        = "SCHEDULE"         // pop Count values; push a schedule of them, in filing order
	OpItem            = "ITEM"             // pop index, pop schedule; push the item at that 1-based position
	OpAnnex           = "ANNEX"            // pop value, pop schedule; push the schedule with the value annexed at the end
	OpSubstitute      = "SUBSTITUTE"       // pop value, pop index, pop schedule; push the schedule with the item at index replaced
	OpArchive         = "ARCHIVE"          // pop name, pop value; enter the value in the archive and repoint the catalog
	OpDocument        = "DOCUMENT"         // pop name; push the current version of the document so cataloged
	OpPatent          = "PATENT"           // pop term and disclosure; issue the patent named by Name or produce a verdict
	OpPractice        = "PRACTICE"         // push the disclosure named by Name when the case may practice it
	OpCommence        = "COMMENCE"         // pop a Form K-1 source string; open a new case upon it; push the new case number
	OpStanding        = "STANDING"         // pop a case number; push its standing: GUILTY, IN GOOD STANDING, or NO MATTER ON FILE
	OpMotion          = "MOTION"           // file a motion; Target resumes execution and Name optionally receives the grounds
	OpAwaitFor        = "AWAIT-FOR"        // pop a term in days; await a summons for at most that long. Served: push it and fall through. Expired: refer to Target. The outcome is entered in the ledger
	OpDiscovery       = "DISCOVERY"        // pop a case number; push its record Name and record the read in the ledger
	OpPublish         = "PUBLISH"          // pop a value; publish its transcript in the court-wide gazette, within the step's transaction
	OpAwaitGazette    = "AWAIT-GAZETTE"    // block at this case's gazette cursor; push the next edition; the cursor advances with the step
	OpLicense         = "LICENSE"          // pop term and licensee; grant read-only practice, capped by the patent term
	OpAssign          = "ASSIGN"           // pop assignee; transfer Name when no licenses remain outstanding
	OpAwaitFrom       = "AWAIT-FROM"       // pop a case number; await the next summons bearing that case's seal, out of turn; push what is served. The records passed over await their own turn
	OpAwaitFromFor    = "AWAIT-FROM-FOR"   // pop a term in days, pop a case number; the selective receive with a deadline. Served: push it and fall through. Expired: refer to Target. The outcome is entered in the ledger
	OpInscribe        = "INSCRIBE"         // pop value, pop key, pop register; push the register with the value inscribed under the key
	OpEntry           = "ENTRY"            // pop key and register; push the entry or produce a verdict when absent
	OpExpunge         = "EXPUNGE"          // pop key and register; push a copy without that key
	OpRoster          = "ROSTER"           // pop a register; push a schedule of its keys, in alphabetical order
	OpPower           = "POWER"            // push a power of attorney over the office at Target: Name, Params, and the executing case travel with it
	OpPetitionUnder   = "PETITION-UNDER"   // pop Count args, pop a power of attorney; open a frame and jump to the office it confers. Wants says whether a finding is expected back
	OpJudgment        = "JUDGMENT"         // pop case number and grounds; record a verdict when this case commenced the target
)

// Value kinds.
const (
	KindInt      = "int"
	KindString   = "str"
	KindFinding  = "finding"
	KindExhibit  = "exhibit"
	KindSchedule = "sched"
	KindSum      = "sum"
	KindRegister = "reg"
	KindPower    = "poa"
)

// SumScale is the number of minor units in one whole sum unit.
const (
	SumScale    = 100
	maxSumWhole = int64(1<<63-1) / SumScale
)

// Value is a runtime value: integer, string, finding, exhibit, schedule, sum,
// register, or power of attorney.
type Value struct {
	T  string           `json:"t"`
	I  int64            `json:"i,omitempty"` // INT: the integer; SUM: the mantissa, in pennies
	S  string           `json:"s,omitempty"`
	B  bool             `json:"b,omitempty"`
	Of string           `json:"of,omitempty"` // EXHIBIT: which exhibit it is an exhibit of
	X  map[string]Value `json:"x,omitempty"`  // EXHIBIT: the entries
	L  []Value          `json:"l,omitempty"`  // SCHEDULE: the items, in order
}

func Int(i int64) Value    { return Value{T: KindInt, I: i} }
func Str(s string) Value   { return Value{T: KindString, S: s} }
func Finding(b bool) Value { return Value{T: KindFinding, B: b} }
func Exhibit(of string, entries map[string]Value) Value {
	return Value{T: KindExhibit, Of: of, X: cloneValues(entries)}
}
func Schedule(items []Value) Value { return Value{T: KindSchedule, L: cloneValueSlice(items)} }

// Register is an unordered string-to-value map with value semantics. Entries
// reuse the X field.
func Register(entries map[string]Value) Value { return Value{T: KindRegister, X: cloneValues(entries)} }

// Power stores an office name, instruction address, concerns, and executing
// case. It is enforceable only in the case whose proceedings contain that
// address.
func Power(name string, target int64, caseID string, params []string) Value {
	l := make([]Value, len(params))
	for i, p := range params {
		l[i] = Str(p)
	}
	return Value{T: KindPower, S: name, I: target, Of: caseID, L: l}
}

// cloneValue copies collection storage retained by a Value. Values built by
// the parser and JSON decoder are acyclic.
func cloneValue(value Value) Value {
	value.X = cloneValues(value.X)
	value.L = cloneValueSlice(value.L)
	return value
}

func cloneValues(values map[string]Value) map[string]Value {
	if values == nil {
		return nil
	}
	cloned := make(map[string]Value, len(values))
	for key, value := range values {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValueSlice(values []Value) []Value {
	if values == nil {
		return nil
	}
	cloned := make([]Value, len(values))
	for i, value := range values {
		cloned[i] = cloneValue(value)
	}
	return cloned
}

// Sum is a fixed-point decimal: the mantissa counts pennies. Sum(1250) is
// 12.50. It does not use IEEE floating point, preserving deterministic results
// across platforms.
func Sum(mantissa int64) Value { return Value{T: KindSum, I: mantissa} }

// ParseSum reads a decimal literal with exactly two decimal places (for
// example, "12.50" or "-0.05") into a mantissa.
func ParseSum(s string) (int64, bool) {
	whole, frac, ok := strings.Cut(s, ".")
	if !ok || len(frac) != 2 || frac[0] < '0' || frac[0] > '9' || frac[1] < '0' || frac[1] > '9' {
		return 0, false
	}
	neg := strings.HasPrefix(whole, "-")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	pennies := int64(frac[0]-'0')*10 + int64(frac[1]-'0')
	if neg {
		// MinInt64 has one more unit of magnitude than MaxInt64. Check
		// before scaling so the parser never silently wraps a literal.
		if w < -maxSumWhole || w == -maxSumWhole && pennies > 8 {
			return 0, false
		}
		return w*SumScale - pennies, true
	}
	if w > maxSumWhole || w == maxSumWhole && pennies > 7 {
		return 0, false
	}
	return w*SumScale + pennies, true
}

// Display renders a value the way PROCLAIM publishes it.
func (v Value) Display() string {
	switch v.T {
	case KindInt:
		return fmt.Sprintf("%d", v.I)
	case KindSum:
		m := uint64(v.I)
		sign := ""
		if v.I < 0 {
			sign = "-"
			// Avoid negating MinInt64 in signed arithmetic: its absolute
			// value exists as uint64 even though it does not as int64.
			m = uint64(-(v.I + 1)) + 1
		}
		return fmt.Sprintf("%s%d.%02d", sign, m/SumScale, m%SumScale)
	case KindString:
		return v.S
	case KindFinding:
		if v.B {
			return "SUSTAINED"
		}
		return "OVERRULED"
	case KindExhibit, KindRegister:
		var sb strings.Builder
		if v.T == KindExhibit {
			fmt.Fprintf(&sb, "AN EXHIBIT OF %s (", v.Of)
		} else {
			sb.WriteString("A REGISTER (")
		}
		for i, name := range slices.Sorted(maps.Keys(v.X)) {
			if i > 0 {
				sb.WriteString("; ")
			}
			fmt.Fprintf(&sb, "%s: %s", name, v.X[name].Display())
		}
		sb.WriteString(")")
		return sb.String()
	case KindSchedule:
		var sb strings.Builder
		sb.WriteString("A SCHEDULE (")
		for i, item := range v.L {
			if i > 0 {
				sb.WriteString("; ")
			}
			sb.WriteString(item.Display())
		}
		sb.WriteString(")")
		return sb.String()
	case KindPower:
		return fmt.Sprintf("A POWER OF ATTORNEY OVER THE OFFICE OF %s (%d concern(s), executed in the matter of %s)", v.S, len(v.L), v.Of)
	}
	return "(a value of no recognized standing)"
}

// Equal reports deep equality. Exhibits include their subject and entries;
// integers and sums compare by monetary amount.
func (v Value) Equal(o Value) bool {
	if v.T != o.T {
		if lm, rm, ok := Amounts(v, o); ok {
			return lm == rm
		}
		return false
	}
	switch v.T {
	case KindInt, KindSum:
		return v.I == o.I
	case KindString:
		return v.S == o.S
	case KindFinding:
		return v.B == o.B
	case KindExhibit, KindRegister:
		return (v.T != KindExhibit || v.Of == o.Of) && maps.EqualFunc(v.X, o.X, Value.Equal)
	case KindSchedule:
		return slices.EqualFunc(v.L, o.L, Value.Equal)
	case KindPower:
		// The office address and executing case determine the concerns.
		return v.S == o.S && v.I == o.I && v.Of == o.Of
	}
	return false
}

// Amounts converts two values to a common penny scale, when both are
// numbers (integers or sums) and at least one is a sum. It reports
// false otherwise; the promotion of integers to money is performed
// only in the presence of money.
func Amounts(l, r Value) (lm, rm int64, ok bool) {
	if (l.T != KindSum && l.T != KindInt) || (r.T != KindSum && r.T != KindInt) {
		return 0, 0, false
	}
	if l.T != KindSum && r.T != KindSum {
		return 0, 0, false
	}
	lm, rm = l.I, r.I
	if l.T == KindInt {
		lm *= SumScale
	}
	if r.T == KindInt {
		rm *= SumScale
	}
	return lm, rm, true
}

// Instr is one record in the proceedings topic. Its zero-based position among
// visible instruction records is its address; Target fields refer to those
// logical positions rather than physical Kafka offsets.
type Instr struct {
	Op     string   `json:"op"`
	Value  *Value   `json:"value,omitempty"`  // SUBMIT
	Name   string   `json:"name,omitempty"`   // RETRIEVE, FILE; office name on PETITION (decorative); exhibit name on EXHIBIT; entry name on INSPECT, ENTER
	Target int64    `json:"target,omitempty"` // REFER, REFER-OVERRULED, PETITION
	Params []string `json:"params,omitempty"` // PETITION: names bound in the office's frame; EXHIBIT: the entries, in filing order
	Wants  bool     `json:"wants,omitempty"`  // PETITION: caller awaits a finding
	With   bool     `json:"with,omitempty"`   // REMAND: a finding accompanies the remand
	Count  int      `json:"count,omitempty"`  // SCHEDULE: how many items to pop
	Pos    string   `json:"pos,omitempty"`    // source position, for counsel
}

func (i Instr) Marshal() []byte {
	b, err := json.Marshal(i)
	if err != nil {
		panic(fmt.Sprintf("an instruction refused transcription: %v", err))
	}
	return b
}

func Unmarshal(b []byte) (Instr, error) {
	var i Instr
	err := json.Unmarshal(b, &i)
	return i, err
}
