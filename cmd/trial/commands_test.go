package main

import (
	"strings"
	"testing"
)

// TestCommandTableAgreesWithUsage checks the two places a subcommand must be
// registered: the dispatch table and the usage text. Each must mention exactly
// the commands the other does.
func TestCommandTableAgreesWithUsage(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range commandTable() {
		if c.name == "" || c.run == nil {
			t.Fatalf("command table entry %+v is incomplete", c)
		}
		if registered[c.name] {
			t.Fatalf("command %q is registered twice", c.name)
		}
		registered[c.name] = true
		if _, ok := helpFor(c.name); !ok {
			t.Errorf("command %q has no section in the usage text", c.name)
		}
	}
	for line := range strings.SplitSeq(usage, "\n") {
		fields := strings.Fields(line)
		if !strings.HasPrefix(line, "  trial ") || len(fields) < 2 || fields[0] != "trial" || strings.HasPrefix(fields[1], "<") {
			continue
		}
		if !registered[fields[1]] {
			t.Errorf("usage documents %q, which is not in the command table", fields[1])
		}
	}
	if len(commandNames) != len(registered) {
		t.Errorf("commandNames has %d entries, the table has %d", len(commandNames), len(registered))
	}
}

func TestAcceptsBrokerFollowsTable(t *testing.T) {
	for _, c := range commandTable() {
		if got := acceptsBroker(c.name); got != c.broker {
			t.Errorf("acceptsBroker(%q) = %v, table says %v", c.name, got, c.broker)
		}
	}
	if acceptsBroker("no-such-command") {
		t.Error("acceptsBroker accepted an unknown command")
	}
	for _, name := range []string{"summon", "dismiss", "test", "counsel", "help", "version"} {
		if acceptsBroker(name) {
			t.Errorf("%q does not talk to Kafka but documents --broker", name)
		}
	}
}

// TestRunExitStatus covers the argument-handling paths of run that do not
// need a broker. Exit status 2 is the documented "invalid arguments" code.
func TestRunExitStatus(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "no arguments", args: nil, want: 2},
		{name: "unknown command", args: []string{"prosecute"}, want: 2},
		{name: "version with extra argument", args: []string{"version", "extra"}, want: 2},
		{name: "summon with extra argument", args: []string{"summon", "extra"}, want: 2},
		{name: "help with two arguments", args: []string{"help", "file", "proceed"}, want: 2},
		{name: "help for unknown command", args: []string{"help", "prosecute"}, want: 2},
		{name: "version", args: []string{"version"}, want: 0},
		{name: "version alias", args: []string{"--version"}, want: 0},
		{name: "help", args: []string{"help"}, want: 0},
		{name: "help alias", args: []string{"-h"}, want: 0},
		{name: "help for one command", args: []string{"help", "file"}, want: 0},
		{name: "command help flag", args: []string{"file", "--help"}, want: 0},
		{name: "help flag before positional", args: []string{"proceed", "-h", "case-x"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(tc.args); got != tc.want {
				t.Errorf("run(%q) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

func TestServeHelpDocumentsOptionTerminator(t *testing.T) {
	text, ok := helpFor("serve")
	if !ok {
		t.Fatal("serve has no help section")
	}
	if !strings.Contains(text, " -- -5") {
		t.Errorf("serve help does not show how to pass a negative value:\n%s", text)
	}
	if !strings.Contains(usage, "Use \"--\" before") {
		t.Error("usage does not explain the -- terminator")
	}
}

func TestReadBoundedRejectsOversizedInput(t *testing.T) {
	data, err := readBounded(strings.NewReader("abcde"), "x", 5)
	if err != nil || string(data) != "abcde" {
		t.Fatalf("readBounded at the limit = %q, %v", data, err)
	}
	if _, err := readBounded(strings.NewReader("abcdef"), "x", 5); err == nil {
		t.Fatal("readBounded accepted input one byte over the limit")
	}
}

func TestOptionTerminatorDetection(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{args: []string{"--"}, want: true},
		{args: []string{"--quiet", "--"}, want: true},
		{args: []string{"--broker=x", "--"}, want: true},
		{args: []string{"--broker", "--", "case-x"}, want: false},
		{args: []string{"case-x", "--"}, want: false},
		{args: []string{"-", "--"}, want: false},
		{args: nil, want: false},
	}
	for _, tc := range cases {
		fs := commandFlags("t")
		fs.String("broker", "", "")
		fs.Bool("quiet", false, "")
		if got := terminatesOptionsBeforeFirstArg(fs, tc.args); got != tc.want {
			t.Errorf("terminatesOptionsBeforeFirstArg(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
