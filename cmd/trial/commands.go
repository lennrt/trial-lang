package main

import "context"

// command describes one trial subcommand.
//
// The table returned by commandTable is the single source of truth for
// dispatch in run, for the command list used by help and near-miss
// suggestions, and for whether a command documents --broker. Adding a
// subcommand means adding one entry here and one section to usage; the tests
// in commands_test.go check that the two agree.
type command struct {
	// name is the literal subcommand as typed on the command line.
	name string
	// run executes the command with the arguments that followed its name.
	// It returns the process exit status.
	run func(ctx context.Context, args []string) int
	// broker reports whether the command connects to Kafka and therefore
	// accepts --broker.
	broker bool
}

// commandTable lists every subcommand. The order is the suggestion order
// used by nearest, so it is stable rather than alphabetical.
//
// It is a function rather than a package variable because the help command
// reaches back into the table through helpFor and acceptsBroker; a variable
// would form an initialization cycle.
func commandTable() []command {
	return []command{
		{name: "summon", run: noArgs("summon", summon)},
		{name: "dismiss", run: noArgs("dismiss", dismiss)},
		{name: "file", run: fileCase, broker: true},
		{name: "proceed", run: proceedCase, broker: true},
		{name: "observe", run: observe, broker: true},
		{name: "serve", run: serve, broker: true},
		{name: "amend", run: amend, broker: true},
		{name: "enact", run: enact, broker: true},
		{name: "statutes", run: statutes, broker: true},
		{name: "hearing", run: hearing, broker: true},
		{name: "test", run: testCmd},
		{name: "verdict", run: verdict, broker: true},
		{name: "status", run: status, broker: true},
		{name: "docket", run: docketCmd, broker: true},
		{name: "transcript", run: transcript, broker: true},
		{name: "reenact", run: reenact, broker: true},
		{name: "audit", run: audit, broker: true},
		{name: "appeal", run: appeal, broker: true},
		{name: "profile", run: profileCmd, broker: true},
		{name: "burn", run: burn, broker: true},
		{name: "mcp", run: mcpCmd, broker: true},
		{name: "counsel", run: counselCmd},
		{name: "watch", run: watch, broker: true},
		{name: "help", run: helpDispatch},
		{name: "version", run: noArgs("version", func(context.Context) int { return versionCmd() })},
	}
}

// lookupCommand finds the command registered under name.
func lookupCommand(name string) (command, bool) {
	for _, c := range commandTable() {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// noArgs adapts a command that takes no arguments, rejecting any that were
// supplied with the usual "unexpected argument" diagnostic.
func noArgs(name string, fn func(context.Context) int) func(context.Context, []string) int {
	return func(ctx context.Context, args []string) int {
		if unexpectedArgs(name, args) {
			return 2
		}
		return fn(ctx)
	}
}
