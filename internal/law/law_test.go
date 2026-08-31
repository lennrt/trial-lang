package law

// The value system, deposed directly: display forms, deep equality,
// the promotion of integers in the presence of money, and the sum
// parser's insistence on the penny.

import (
	"encoding/json"
	"testing"
)

func TestCollectionConstructorsOwnCallerStorage(t *testing.T) {
	nested := map[string]Value{"inner": Str("original")}
	entries := map[string]Value{"nested": Register(nested)}
	items := []Value{Exhibit("source", entries)}

	exhibit := Exhibit("copy", entries)
	register := Register(entries)
	schedule := Schedule(items)

	nested["inner"] = Str("changed")
	entries["nested"] = Str("changed")
	items[0] = Str("changed")

	for name, value := range map[string]Value{
		"exhibit":  exhibit.X["nested"].X["inner"],
		"register": register.X["nested"].X["inner"],
		"schedule": schedule.L[0].X["nested"].X["inner"],
	} {
		if value.S != "original" {
			t.Fatalf("%s retained caller-owned collection storage: %+v", name, value)
		}
	}
}

func TestParseSum(t *testing.T) {
	cases := []struct {
		in       string
		mantissa int64
		ok       bool
	}{
		{"12.50", 1250, true},
		{"0.05", 5, true},
		{"0.00", 0, true},
		{"-0.05", -5, true},
		{"-12.50", -1250, true},
		{"9999999.99", 999999999, true},
		{"92233720368547758.07", 1<<63 - 1, true},
		{"-92233720368547758.08", -1 << 63, true},
		{"92233720368547758.08", 0, false},
		{"-92233720368547758.09", 0, false},
		{"92233720368547759.00", 0, false},
		{"-92233720368547759.00", 0, false},
		{"12.5", 0, false},   // one figure short of the penny
		{"12.505", 0, false}, // one figure past it
		{"12", 0, false},     // an integer is not a sum
		{".50", 0, false},    // no whole part, no standing
		{"12.x5", 0, false},
		{"12.5x", 0, false},
		{"twelve.fifty", 0, false},
	}
	for _, c := range cases {
		m, ok := ParseSum(c.in)
		if ok != c.ok || (ok && m != c.mantissa) {
			t.Errorf("ParseSum(%q) = %d, %v; want %d, %v", c.in, m, ok, c.mantissa, c.ok)
		}
	}
}

func TestDisplay(t *testing.T) {
	nested := Exhibit("person", map[string]Value{
		"name": Str("A. Turing"),
		"age":  Int(41),
	})
	cases := []struct {
		v    Value
		want string
	}{
		{Int(42), "42"},
		{Int(-1), "-1"},
		{Sum(1250), "12.50"},
		{Sum(-5), "-0.05"},
		{Sum(-1 << 63), "-92233720368547758.08"},
		{Sum(0), "0.00"},
		{Str("testimony"), "testimony"},
		{Finding(true), "SUSTAINED"},
		{Finding(false), "OVERRULED"},
		// Exhibit entries print in sorted order, so display is stable.
		{nested, "AN EXHIBIT OF person (age: 41; name: A. Turing)"},
		{Schedule([]Value{Int(500), Str("marks")}), "A SCHEDULE (500; marks)"},
		{Schedule(nil), "A SCHEDULE ()"},
		{Value{}, "(a value of no recognized standing)"},
	}
	for _, c := range cases {
		if got := c.v.Display(); got != c.want {
			t.Errorf("Display() = %q, want %q", got, c.want)
		}
	}
}

func TestEqual(t *testing.T) {
	person := func(age int64) Value {
		return Exhibit("person", map[string]Value{"name": Str("k"), "age": Int(age)})
	}
	cases := []struct {
		name string
		l, r Value
		want bool
	}{
		{"ints", Int(5), Int(5), true},
		{"ints differ", Int(5), Int(6), false},
		{"strings", Str("a"), Str("a"), true},
		{"findings", Finding(true), Finding(true), true},
		{"findings differ", Finding(true), Finding(false), false},
		{"kinds differ", Str("5"), Int(5), false},
		// The promotion of integers happens only in the presence of
		// money: 5 and 5.00 are the same amount.
		{"int meets sum", Int(5), Sum(500), true},
		{"sum meets int", Sum(500), Int(5), true},
		{"int misses sum", Int(5), Sum(501), false},
		{"sums", Sum(1250), Sum(1250), true},
		{"exhibits agree", person(30), person(30), true},
		{"exhibits differ", person(30), person(31), false},
		{"different exhibits", person(30), Exhibit("dog", map[string]Value{"name": Str("k"), "age": Int(30)}), false},
		{"schedules agree", Schedule([]Value{Int(1), Str("a")}), Schedule([]Value{Int(1), Str("a")}), true},
		{"schedules differ", Schedule([]Value{Int(1)}), Schedule([]Value{Int(2)}), false},
		{"schedules of different length", Schedule([]Value{Int(1)}), Schedule([]Value{Int(1), Int(2)}), false},
		{"nested schedules", Schedule([]Value{person(30)}), Schedule([]Value{person(30)}), true},
	}
	for _, c := range cases {
		if got := c.l.Equal(c.r); got != c.want {
			t.Errorf("%s: Equal = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAmounts(t *testing.T) {
	// Two integers are not money; nothing is promoted.
	if _, _, ok := Amounts(Int(5), Int(5)); ok {
		t.Error("two integers should not be treated as amounts")
	}
	// A string near money remains a string near money.
	if _, _, ok := Amounts(Str("5.00"), Sum(500)); ok {
		t.Error("a string is not an amount, whatever it says")
	}
	if lm, rm, ok := Amounts(Int(5), Sum(499)); !ok || lm != 500 || rm != 499 {
		t.Errorf("Amounts(5, 4.99) = %d, %d, %v", lm, rm, ok)
	}
	if lm, rm, ok := Amounts(Sum(-5), Sum(5)); !ok || lm != -5 || rm != 5 {
		t.Errorf("Amounts(-0.05, 0.05) = %d, %d, %v", lm, rm, ok)
	}
}

func TestInstrRoundTrip(t *testing.T) {
	v := Str("evidence")
	in := Instr{
		Op:     OpPetition,
		Value:  &v,
		Name:   "actuarial-services",
		Target: 17,
		Params: []string{"n", "m"},
		Wants:  true,
		Count:  2,
		Pos:    "line 4",
	}
	out, err := Unmarshal(in.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if out.Op != in.Op || out.Name != in.Name || out.Target != in.Target ||
		out.Wants != in.Wants || out.Count != in.Count || out.Pos != in.Pos ||
		len(out.Params) != 2 || out.Params[0] != "n" || out.Params[1] != "m" ||
		out.Value == nil || !out.Value.Equal(v) {
		t.Fatalf("the instruction did not survive transcription: %+v", out)
	}
}

func TestInstrUnmarshalGarbage(t *testing.T) {
	if _, err := Unmarshal([]byte("not the law")); err == nil {
		t.Fatal("garbage should not unmarshal into an instruction")
	}
}

func TestValueJSONStability(t *testing.T) {
	// Values travel between topics as JSON; a value must survive the
	// journey byte-for-byte in meaning, including nesting.
	v := Exhibit("cell", map[string]Value{
		"head": Sum(-125),
		"tail": Schedule([]Value{Finding(false), Str("end")}),
	})
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var back Value
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !v.Equal(back) {
		t.Fatalf("the value did not survive its journey: %s vs %s", v.Display(), back.Display())
	}
}
