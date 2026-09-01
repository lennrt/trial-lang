package main

import "strings"

const usage = `trial compiles and executes triallang cases.

USAGE
  trial <command> [flags]

CASE COMMANDS
  trial file <program.trial>      Create a case and print its case number
    --counsel                     Print filing rejection details
    -q, --quiet                   Print only the case number
  trial proceed <case>            Start or resume a case
    --docket                      Process current and future docket entries
    --expedited n                 Commit at most n instructions per step
    -q, --quiet                   Suppress progress output; use the exit status
  trial observe <case>            Follow case output
    --from-the-beginning          Read all committed output
  trial serve <case> <value>...   Send input values to a case
    -q, --quiet                   Print no confirmation
  trial amend <case> <k2.trial>   Append a Form K-2 filing
    --counsel                     Print filing rejection details
  trial hearing [case]            Enter and execute statements interactively
  trial verdict <case>            Read the final verdict
    --counsel                     Print sealed details
    --json                        Print JSON
  trial status <case>             Read case status
    --json                        Print JSON
  trial docket                    List cases
    --json                        Print JSON
  trial transcript <case>         Print the original filing
  trial reenact <case>            Replay a case from its recorded inputs
  trial audit <case>              Replay a copy and compare its record
    --docket                      Audit every case and court-wide record
    --json                        Print JSON
  trial appeal <case>             Copy a case to a new case number
    --at-step n                   Copy state after committed step n
  trial profile <case>            Replay a case and count instruction cost
    --top n                       Print at most n entries; default 20
    --json                        Print JSON
  trial burn <case>               Delete a case; refused without confirmation
    --with-prejudice              Confirm permanent deletion

COURT COMMANDS
  trial summon                    Start the local Docker Compose broker
  trial dismiss                   Stop the local Docker Compose broker
  trial enact <statute.trial>     Add a Form S-1 statute
    --canon                       Add the bundled canon
  trial statutes                  List statutes
  trial test [path ...]           Run deposition files without Kafka
    --transcript                  Print all witness output
  trial watch                     Show the live docket
    --interval 2s                 Set the refresh interval
    --once                        Print one snapshot and exit

PROTOCOL COMMANDS
  trial mcp                       Run the Advocate MCP server on standard I/O
  trial counsel                   Run the Counsel LSP server on standard I/O

OTHER COMMANDS
  trial help [command]            Show all help or one command
  trial version                   Print the build version

FLAGS
  --broker <addr>                 Kafka address; default localhost:9092 or TRIAL_BROKER
  -h, --help                      Show command help

Use "-" as the file name to read a filing from standard input.

EXIT STATUS
  0  The command succeeded.
  1  The command or case failed.
  2  The arguments were invalid.`

const shortUsage = `trial compiles and executes triallang cases.

USAGE
  trial <command> [flags]

COMMON COMMANDS
  summon                Start the local broker
  file <program.trial>  Create a case
  proceed <case>        Start or resume a case
  observe <case>        Follow case output
  serve <case> <v>...   Send input to a case
  test [path ...]       Run depositions without Kafka
  docket                List cases

Run "trial help" for all commands. Run "trial help <command>" for one command.`

var helpExamples = map[string]string{
	"file":    "  trial file examples/hello.trial\n  trial proceed \"$(trial file examples/hello.trial --quiet)\"",
	"proceed": "  trial proceed case-7f3a1c8e2d4b609af137c5e9\n  trial proceed --docket",
	"observe": "  trial observe case-7f3a1c8e2d4b609af137c5e9 --from-the-beginning",
	"serve":   "  trial serve case-7f3a1c8e2d4b609af137c5e9 3",
	"test":    "  trial test examples\n  trial test --transcript examples/hello.deposition",
	"status":  "  trial status case-7f3a1c8e2d4b609af137c5e9 --json",
	"docket":  "  trial docket --json",
	"enact":   "  trial enact --canon",
	"amend":   "  trial amend case-7f3a1c8e2d4b609af137c5e9 new-evidence-k2.trial",
	"audit":   "  trial audit case-7f3a1c8e2d4b609af137c5e9\n  trial audit --docket",
	"appeal":  "  trial appeal case-7f3a1c8e2d4b609af137c5e9 --at-step 40",
	"burn":    "  trial burn case-7f3a1c8e2d4b609af137c5e9 --with-prejudice",
}

// helpFor returns the matching command section from usage.
func helpFor(name string) (string, bool) {
	lines := strings.Split(usage, "\n")
	var out []string
	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, "  trial "+name+" ") || line == "  trial "+name {
			found = true
			out = append(out, line)
			continue
		}
		if found {
			if strings.HasPrefix(line, "  trial ") || (!strings.HasPrefix(line, " ") && line != "") {
				break
			}
			out = append(out, line)
		}
	}
	if !found {
		return "", false
	}
	var b strings.Builder
	if example, ok := helpExamples[name]; ok {
		b.WriteString("EXAMPLE\n")
		b.WriteString(example)
		b.WriteString("\n\n")
	}
	b.WriteString("USAGE\n")
	b.WriteString(strings.Join(out, "\n"))
	b.WriteString("\n\nFLAGS\n")
	if acceptsBroker(name) {
		b.WriteString("  --broker <addr>  Kafka address; default localhost:9092 or TRIAL_BROKER\n")
	}
	b.WriteString("  -h, --help       Show this help\n\nRun \"trial help\" for all commands.")
	return b.String(), true
}

func acceptsBroker(name string) bool {
	switch name {
	case "summon", "dismiss", "test", "counsel", "help", "version":
		return false
	default:
		return true
	}
}

// wantsHelp reports whether an option before "--" requests help.
func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "-help" || arg == "--help" {
			return true
		}
	}
	return false
}

var commandNames = []string{
	"summon", "dismiss", "file", "proceed", "observe", "serve", "amend",
	"enact", "statutes", "hearing", "test", "verdict", "status", "docket",
	"transcript", "reenact", "audit", "appeal", "profile", "burn", "mcp",
	"counsel", "watch", "help", "version",
}

// nearest returns a command within two edits of cmd.
func nearest(cmd string) string {
	best, bestDistance := "", 3
	for _, name := range commandNames {
		if distance := editDistance(cmd, name); distance < bestDistance {
			best, bestDistance = name, distance
		}
	}
	return best
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for index := range previous {
		previous[index] = index
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}
