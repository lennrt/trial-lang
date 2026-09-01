// Command trial compiles and executes triallang cases.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	fs2 "io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/lennrt/trial-lang/canon"
	"github.com/lennrt/trial-lang/internal/advocate"
	"github.com/lennrt/trial-lang/internal/counsel"
	"github.com/lennrt/trial-lang/internal/court"
	"github.com/lennrt/trial-lang/internal/deposition"
	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// Release builds set version with -ldflags "-X main.version=<tag>".
var version string

func resolveVersion() string {
	if version != "" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "(of no recorded edition)"
	}
	if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = ", amended without leave"
			}
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		return fmt.Sprintf("(devel, at revision %s%s)", rev, dirty)
	}
	return "(devel)"
}

func versionCmd() int {
	fmt.Printf("trial %s\n", resolveVersion())
	fmt.Println("triallang: the language is the log. This binary is merely an official,")
	fmt.Println("and officials are replaceable; the edition above is his commission.")
	return 0
}

func brokerDefault() string {
	if b := os.Getenv("TRIAL_BROKER"); b != "" {
		return b
	}
	return "localhost:9092"
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, shortUsage)
		return 2
	}
	cmd, rest := args[0], args[1:]

	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		if len(rest) > 0 {
			return helpCmd(rest[0])
		}
		fmt.Println(usage)
		return 0
	}
	// Help takes precedence over other command options.
	if wantsHelp(rest) {
		return helpCmd(cmd)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	switch cmd {
	case "summon":
		return summon(ctx)
	case "dismiss":
		return dismiss(ctx)
	case "file":
		return fileCase(ctx, rest)
	case "proceed":
		return proceedCase(ctx, rest)
	case "observe":
		return observe(ctx, rest)
	case "serve":
		return serve(ctx, rest)
	case "amend":
		return amend(ctx, rest)
	case "enact":
		return enact(ctx, rest)
	case "statutes":
		return statutes(ctx, rest)
	case "hearing":
		return hearing(ctx, rest)
	case "test":
		return testCmd(ctx, rest)
	case "verdict":
		return verdict(ctx, rest)
	case "status":
		return status(ctx, rest)
	case "docket":
		return docketCmd(ctx, rest)
	case "transcript":
		return transcript(ctx, rest)
	case "reenact":
		return reenact(ctx, rest)
	case "audit":
		return audit(ctx, rest)
	case "appeal":
		return appeal(ctx, rest)
	case "profile":
		return profileCmd(ctx, rest)
	case "burn":
		return burn(ctx, rest)
	case "mcp":
		return mcpCmd(ctx, rest)
	case "counsel":
		return counselCmd(ctx, rest)
	case "watch":
		return watch(ctx, rest)
	case "version", "--version", "-v":
		return versionCmd()
	}
	fmt.Fprintf(os.Stderr, "trial: %q is not a motion this court entertains.", cmd)
	if s := nearest(cmd); s != "" {
		fmt.Fprintf(os.Stderr, " Perhaps 'trial %s' was intended.", s)
	}
	fmt.Fprintln(os.Stderr, " See 'trial help'.")
	return 2
}

// helpCmd prints help for one command.
func helpCmd(name string) int {
	name = strings.TrimLeft(name, "-")
	if text, ok := helpFor(name); ok {
		fmt.Println(text)
		return 0
	}
	fmt.Fprintf(os.Stderr, "trial help: %q is not a motion this court entertains. See 'trial help'.\n", name)
	return 2
}

func compose(ctx context.Context, verb string, extra ...string) int {
	args := append([]string{"compose", verb}, extra...)
	c := exec.CommandContext(ctx, "docker", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "The courthouse could not be reached: %v\n(Is Docker installed and running?)\n", err)
		return 1
	}
	return 0
}

func summon(ctx context.Context) int {
	if code := compose(ctx, "up", "-d"); code != 0 {
		return code
	}
	fmt.Println()
	fmt.Println("The court is in session at localhost:9092.")
	fmt.Println("File something: trial file examples/hello.trial")
	return 0
}

func dismiss(ctx context.Context) int {
	if code := compose(ctx, "down"); code != 0 {
		return code
	}
	fmt.Println()
	fmt.Println("The court stands in recess. The archive is retained;")
	fmt.Println("your cases will be waiting when the court reconvenes.")
	return 0
}

// openLog connects to Kafka. The caller owns the returned log.
func openLog(ctx context.Context, broker string) (*docket.KafkaLog, int) {
	log, err := docket.OpenKafkaLog(ctx, broker, docket.WithDiagnostic(func(err error) {
		fmt.Fprintf(os.Stderr, "Kafka maintenance warning: %v\n", err)
	}))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, 1
	}
	return log, 0
}

// isTTY reports whether f is a terminal. Pipes never receive prompts or ANSI
// screen controls.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// readSource reads at most maxSourceBytes from path. A path of "-" reads
// standard input. The function rejects extra input before allocation grows.
func readSource(path string) ([]byte, error) {
	if path == "-" {
		return readBounded(os.Stdin, "standard input", 4<<20)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := readBounded(file, path, 4<<20)
	closeErr := file.Close()
	return data, errors.Join(readErr, closeErr)
}

func readBounded(reader io.Reader, name string, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds the %d-byte source limit", name, limit)
	}
	return data, nil
}

func commandFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// printJSON writes one indented JSON document to standard output.
func printJSON(v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "the report could not be rendered: %v\n", err)
		return 1
	}
	fmt.Println(string(b))
	return 0
}

// parseCase parses options on either side of one case identifier.
func caseFlags(name string, rest []string, extra func(*flag.FlagSet)) (*flag.FlagSet, string, docket.Case, bool) {
	fs := commandFlags(name)
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	if extra != nil {
		extra(fs)
	}
	var caseID string
	if err := fs.Parse(rest); err != nil {
		return nil, "", docket.Case{}, false
	}
	if fs.NArg() >= 1 {
		caseID = fs.Arg(0)
		// flag stops at the first positional argument. Parse the suffix so
		// callers may put options on either side of the case identifier.
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return nil, "", docket.Case{}, false
		}
	}
	if caseID == "" {
		fmt.Fprintf(os.Stderr, "trial %s: a case number is required. You were given one. It is your only receipt. See 'trial help %s'.\n", name, name)
		return nil, "", docket.Case{}, false
	}
	c, err := docket.ParseCase(caseID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trial %s: %v\n", name, err)
		return nil, "", docket.Case{}, false
	}
	return fs, *broker, c, true
}

func fileCase(ctx context.Context, rest []string) int {
	fs := commandFlags("file")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	counsel := fs.Bool("counsel", false, "reveal the particulars of a rejection")
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "print only the case number")
	fs.BoolVar(&quiet, "q", false, "print only the case number")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "trial file: exactly one .trial filing is accepted per visit. See 'trial help file'.")
		return 2
	}
	src, err := readSource(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "The filing could not be read: %v\n", err)
		return 1
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	c, err := court.File(ctx, log, string(src))
	if err != nil {
		if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
			fmt.Fprintln(os.Stderr, "Your filing has been rejected pursuant to Article §4.2.")
			fmt.Fprintln(os.Stderr, "The text of Article §4.2 is not available at this time.")
			if *counsel {
				fmt.Fprintf(os.Stderr, "\n[counsel] %s\n", rej.Error())
			} else {
				fmt.Fprintln(os.Stderr, "\n(Retain counsel: rerun with --counsel for the particulars.)")
			}
			return 1
		}
		fmt.Fprintf(os.Stderr, "The filing failed for reasons unrelated to its content: %v\n", err)
		return 1
	}
	if quiet {
		// Quiet output is safe to use in command substitution.
		fmt.Println(c.ID)
		return 0
	}
	fmt.Println("The matter has been accepted for filing.")
	fmt.Println()
	fmt.Printf("    Case number: %s\n", c.ID)
	fmt.Println()
	fmt.Println("This number is your only receipt. The proceedings begin when")
	fmt.Printf("the Court convenes:  trial proceed %s\n", c.ID)
	fmt.Printf("Attend them with:    trial observe %s\n", c.ID)
	return 0
}

func proceedCase(ctx context.Context, rest []string) int {
	fs := commandFlags("proceed")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	docketMode := fs.Bool("docket", false, "serve every matter on the docket, present and future")
	expedited := fs.Int("expedited", 1, "instructions per committed step; above 1, the attention is recorded at the pace of the batch")
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "the outcome is the exit code; the ceremony is dispensed with")
	fs.BoolVar(&quiet, "q", false, "the outcome is the exit code; the ceremony is dispensed with")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	var caseID string
	if fs.NArg() >= 1 {
		caseID = fs.Arg(0)
		if fs.NArg() > 1 {
			if err := fs.Parse(fs.Args()[1:]); err != nil {
				return 2
			}
		}
	}
	if *docketMode {
		return proceedDocket(ctx, *broker)
	}
	if *expedited < 1 || *expedited > 10_000 {
		fmt.Fprintln(os.Stderr, "trial proceed: --expedited must be between 1 and 10000 instructions per step.")
		return 2
	}
	if caseID == "" {
		fmt.Fprintln(os.Stderr, "trial proceed: a case number is required (or --docket, for all of them). You were given one. It is your only receipt.")
		return 2
	}
	c, err := docket.ParseCase(caseID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trial proceed: %v\n", err)
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	say := func(format string, args ...any) {
		if !quiet {
			fmt.Printf(format, args...)
		}
	}
	say("The Court convenes in the matter of %s.\n", c.ID)
	say("(Interrupting the official is permitted; the case survives him.)\n\n")

	if *expedited > 1 {
		say("The docket is expedited: %d instructions to the step. Between steps, the Court's position is its own secret.\n\n", *expedited)
	}
	ct := &court.Court{
		Log:                log,
		Case:               c,
		WaitForProceedings: true,
		Expedite:           *expedited,
		Observer: func(line string) {
			say("  %s\n", line)
		},
	}
	outcome, err := ct.Proceed(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%v\n", err)
		return 1
	}
	say("\n")
	switch outcome {
	case court.OutcomeAdjourned:
		say("The case stands adjourned indefinitely. It may be reopened\n")
		say("at any time (trial proceed %s). It is never over.\n", c.ID)
	case court.OutcomeGuilty:
		say("A verdict has been reached:  trial verdict %s\n", c.ID)
		return 1
	case court.OutcomeApparentAcquittal:
		say("The proceedings have run out. This is apparent acquittal,\n")
		say("which is not the same as innocence.\n")
	}
	return 0
}

// proceedDocket processes current and future docket entries until cancellation.
func proceedDocket(ctx context.Context, broker string) int {
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	fmt.Println("The Court convenes for the general docket. Every matter, present")
	fmt.Println("and future, will be served: new filings and commenced cases are")
	fmt.Println("taken up as they appear, and adjourned cases are watched for")
	fmt.Println("amendment. Interrupting the official is permitted; the docket")
	fmt.Println("survives him.")
	fmt.Println()

	err := court.ServeDocket(ctx, log, court.DocketOptions{
		Dial: func(dialCtx context.Context) (docket.Log, error) {
			l, err := docket.OpenKafkaLog(dialCtx, broker)
			if err != nil {
				return nil, err
			}
			return l, nil
		},
		Poll: time.Second,
		Note: func(c docket.Case, line string) {
			fmt.Printf("  [%s] %s\n", c.ID, line)
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "The docket could not be served: %v\n", err)
		return 1
	}
	fmt.Println()
	fmt.Println("The court stands in recess. The docket is retained.")
	return 0
}

func observe(ctx context.Context, rest []string) int {
	var fromBeginning *bool
	_, broker, c, ok := caseFlags("observe", rest, func(fs *flag.FlagSet) {
		fromBeginning = fs.Bool("from-the-beginning", false, "read the full record")
	})
	if !ok {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	recs, err := log.ReadAll(ctx, c.Proclamations())
	if err != nil {
		fmt.Fprintf(os.Stderr, "The gallery is closed: %v\n", err)
		return 1
	}
	var next int64
	if len(recs) > 0 {
		next = recs[len(recs)-1].Offset + 1
	}
	if *fromBeginning {
		for _, r := range recs {
			fmt.Println(string(r.Value))
		}
	}
	fmt.Fprintf(os.Stderr, "(Attending the proceedings of %s. Ctrl+C to slip out.)\n", c.ID)
	for {
		rec, err := log.Fetch(ctx, c.Proclamations(), next, true)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Fprintln(os.Stderr, "\n(You slip out of the gallery. The proceedings continue without you.)")
				return 0
			}
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(string(rec.Value))
		next = rec.Offset + 1
	}
}

func serve(ctx context.Context, rest []string) int {
	var quiet *bool
	fs, broker, c, ok := caseFlags("serve", rest, func(fs *flag.FlagSet) {
		quiet = fs.Bool("quiet", false, "serve without comment")
		fs.BoolVar(quiet, "q", false, "serve without comment")
	})
	if !ok {
		return 2
	}
	values := fs.Args()
	if len(values) == 0 {
		fmt.Fprintln(os.Stderr, "trial serve: nothing to serve. The Court does not deliver empty envelopes. See 'trial help serve'.")
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	for _, v := range values {
		if _, err := log.Append(ctx, c.Summons(), nil, []byte(v)); err != nil {
			fmt.Fprintf(os.Stderr, "The summons could not be served: %v\n", err)
			return 1
		}
	}
	if *quiet {
		return 0
	}
	if len(values) == 1 {
		fmt.Println("The summons has been served. Compliance is assumed.")
	} else {
		fmt.Printf("%d summonses have been served. Compliance is assumed.\n", len(values))
	}
	return 0
}

func amend(ctx context.Context, rest []string) int {
	var counsel *bool
	fs, broker, c, ok := caseFlags("amend", rest, func(fs *flag.FlagSet) {
		counsel = fs.Bool("counsel", false, "reveal the particulars of a rejection")
	})
	if !ok {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "trial amend: a supplemental filing (Form K-2) is required. See 'trial help amend'.")
		return 2
	}
	src, err := readSource(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "The supplement could not be read: %v\n", err)
		return 1
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	n, err := court.Amend(ctx, log, c, string(src))
	if err != nil {
		if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
			fmt.Fprintln(os.Stderr, "Your supplemental filing has been rejected pursuant to Article §4.2.")
			if *counsel {
				fmt.Fprintf(os.Stderr, "\n[counsel] %s\n", rej.Error())
			} else {
				fmt.Fprintln(os.Stderr, "(Retain counsel: rerun with --counsel for the particulars.)")
			}
			return 1
		}
		fmt.Fprintf(os.Stderr, "The supplement was refused: %v\n", err)
		return 1
	}
	fmt.Printf("New evidence has come to light: %d further instruction(s)\n", n)
	fmt.Printf("have been entered against %s. The proceedings resume when\n", c.ID)
	fmt.Printf("the Court next convenes:  trial proceed %s\n", c.ID)
	return 0
}

func enact(ctx context.Context, rest []string) int {
	fs := commandFlags("enact")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	counsel := fs.Bool("counsel", false, "reveal the particulars of a rejection")
	enactCanon := fs.Bool("canon", false, "enact the standard statutes shipped with the binary, in dependency order")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *enactCanon {
		if fs.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "trial enact --canon: the canon is enacted whole; name no other statutes.")
			return 2
		}
		log, code := openLog(ctx, *broker)
		if log == nil {
			return code
		}
		defer log.Close()
		fmt.Println("THE CANON IS NOW ENACTED, piecemeal, like the wall.")
		fmt.Println()
		for _, file := range canon.Files() {
			src, err := canon.FS.ReadFile(file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "The canon is missing %s; the binary was built carelessly: %v\n", file, err)
				return 1
			}
			name, n, err := court.Enact(ctx, log, string(src))
			if err != nil {
				fmt.Fprintf(os.Stderr, "The statute %s was not enacted: %v\n", file, err)
				return 1
			}
			fmt.Printf("    %s (enactment %d)\n", name, n)
		}
		fmt.Println()
		fmt.Println("Any case may now claim their offices at filing time:")
		fmt.Println("    INCORPORATE BY REFERENCE statutes-of-schedules.")
		fmt.Println("(Incorporating a statute incorporates, transitively, whatever it stands on.)")
		return 0
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "trial enact: exactly one statute (Form S-1) is enacted per session (or the whole canon, with --canon). See 'trial help enact'.")
		return 2
	}
	src, err := readSource(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "The statute could not be read: %v\n", err)
		return 1
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	name, n, err := court.Enact(ctx, log, string(src))
	if err != nil {
		if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
			fmt.Fprintln(os.Stderr, "The proposed statute has been rejected pursuant to Article §4.2.")
			if *counsel {
				fmt.Fprintf(os.Stderr, "\n[counsel] %s\n", rej.Error())
			} else {
				fmt.Fprintln(os.Stderr, "(Retain counsel: rerun with --counsel for the particulars.)")
			}
			return 1
		}
		fmt.Fprintf(os.Stderr, "The statute was not enacted: %v\n", err)
		return 1
	}
	fmt.Println("The statute has been enacted and is binding immediately.")
	fmt.Println()
	fmt.Printf("    Statute:   %s (enactment %d)\n", name, n)
	fmt.Println()
	fmt.Println("Any case may now claim its offices at filing time:")
	fmt.Printf("    INCORPORATE BY REFERENCE %s.\n", name)
	fmt.Println("Cases already filed keep the enactment they incorporated;")
	fmt.Println("the law changes, the past does not. Not here, anyway.")
	return 0
}

func statutes(ctx context.Context, rest []string) int {
	fs := commandFlags("statutes")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	names, err := log.ListStatutes(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The statute books could not be consulted: %v\n", err)
		return 1
	}
	if len(names) == 0 {
		fmt.Println("No statutes have been enacted. The court is governing by improvisation.")
		return 0
	}
	sort.Strings(names)
	fmt.Printf("THE STATUTE BOOKS — %d statute(s) in force\n\n", len(names))
	for _, n := range names {
		fmt.Printf("  %s\n", n)
	}
	fmt.Println()
	fmt.Println("Incorporate any of them: INCORPORATE BY REFERENCE <statute>.")
	return 0
}

func hearing(ctx context.Context, rest []string) int {
	fs := commandFlags("hearing")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	counsel := fs.Bool("counsel", false, "unseal any verdict's particulars")
	if err := fs.Parse(rest); err != nil {
		return 2
	}

	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	var h *court.Hearing
	var err error
	if fs.NArg() >= 1 {
		c, parseErr := docket.ParseCase(fs.Arg(0))
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "trial hearing: %v\n", parseErr)
			return 2
		}
		h, err = court.ResumeHearing(ctx, log, c)
	} else {
		h, err = court.OpenHearing(ctx, log)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "The hearing could not be convened: %v\n", err)
		return 1
	}

	// Pipes receive statements and proclamations without prompts.
	interactive := isTTY(os.Stdin)
	if interactive {
		fmt.Printf("You are before the Court in the matter of %s.\n", h.Case.ID)
		fmt.Println("Speak. End each statement with a period. An empty line is")
		fmt.Println("noted. Leave with Ctrl+C or Ctrl+Z; the case remains.")
		fmt.Println()
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for {
		if interactive {
			fmt.Print("K.> ")
		}
		if !scanner.Scan() {
			if interactive {
				fmt.Println()
				fmt.Println("You have left the hearing. The case remains open;")
				fmt.Printf("return to it at any time:  trial hearing %s\n", h.Case.ID)
			}
			return 0
		}
		line := scanner.Text()
		if len(line) == 0 || len(strings.TrimSpace(line)) == 0 {
			if interactive {
				fmt.Println("(Your silence has been noted.)")
			}
			continue
		}
		proclaimed, verdict, err := h.Submit(ctx, line)
		if err != nil {
			if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
				fmt.Println("The submission has been rejected pursuant to Article §4.2.")
				if *counsel {
					fmt.Printf("[counsel] %s\n", rej.Error())
				}
				continue
			}
			fmt.Fprintf(os.Stderr, "The hearing is disturbed: %v\n", err)
			return 1
		}
		for _, p := range proclaimed {
			fmt.Println(p)
		}
		if verdict != nil {
			fmt.Println()
			fmt.Println("GUILTY.")
			if *counsel {
				fmt.Printf("[counsel] %s\n", verdict.Sealed)
			} else {
				fmt.Println("The particulars are sealed. The hearing is concluded,")
				fmt.Println("as is the case, as, in a sense, are you.")
			}
			return 1
		}
	}
}

// testCmd runs each selected deposition against a new in-memory log.
func testCmd(ctx context.Context, rest []string) int {
	fs := commandFlags("test")
	transcript := fs.Bool("transcript", false, "print everything each witness proclaimed, verbatim")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "trial test: %v\n", err)
			return 2
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs2.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(path, ".deposition") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "trial test: walking %s: %v\n", p, err)
			return 2
		}
	}
	if len(files) == 0 {
		fmt.Println("No depositions were found. The witnesses remain unexamined,")
		fmt.Println("which they should not mistake for being uncharged.")
		return 0
	}

	fmt.Println("THE COURT WILL NOW HEAR THE DEPOSITIONS.")
	fmt.Println()
	contradicted := 0
	for _, f := range files {
		src, err := readSource(f)
		if err != nil {
			fmt.Printf("  FAIL    %s\n          the deposition could not be read: %v\n", f, err)
			contradicted++
			continue
		}
		dep, err := deposition.Parse(string(src))
		if err != nil {
			fmt.Printf("  FAIL    %s\n          %v\n", f, err)
			contradicted++
			continue
		}
		if err := deposition.LoadEnactments(dep, filepath.Dir(f)); err != nil {
			fmt.Printf("  FAIL    %s\n          %v\n", f, err)
			contradicted++
			continue
		}
		prog, err := readSource(filepath.Join(filepath.Dir(f), dep.Program))
		if err != nil {
			fmt.Printf("  FAIL    %s\n          the deposed could not be located: %v\n", f, err)
			contradicted++
			continue
		}
		res := deposition.Run(ctx, string(prog), dep)
		if res.OK() {
			fmt.Printf("  ok      %-40s (%.2fs)\n", f, res.Elapsed.Seconds())
		} else {
			contradicted++
			fmt.Printf("  FAIL    %s\n", f)
			for _, contradiction := range res.Contradictions {
				fmt.Printf("          %s\n", contradiction)
			}
		}
		if *transcript && len(res.Said) > 0 {
			fmt.Println()
			fmt.Printf("          THE WITNESS PROCLAIMED, in %d entr(ies):\n", len(res.Said))
			for _, said := range res.Said {
				fmt.Println()
				for line := range strings.SplitSeq(said, "\n") {
					fmt.Printf("          %s\n", line)
				}
			}
			fmt.Println()
		}
	}
	fmt.Println()
	if contradicted == 0 {
		fmt.Printf("%d witness(es) were deposed. The testimony is consistent with the record.\n", len(files))
		return 0
	}
	fmt.Printf("%d witness(es) were deposed; %d stand(s) in contradiction.\n", len(files), contradicted)
	fmt.Println("Perjury proceedings are being considered. Correct the filings, or the depositions,")
	fmt.Println("whichever was lying.")
	return 1
}

func docketCmd(ctx context.Context, rest []string) int {
	fs := commandFlags("docket")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	asJSON := fs.Bool("json", false, "the docket, as JSON")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	cases, err := log.ListCases(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The docket could not be consulted: %v\n", err)
		return 1
	}
	if *asJSON {
		type entry struct {
			Case   string `json:"case"`
			Status string `json:"status"` // guilty | in-proceedings | filed | unexamined
			PC     *int64 `json:"pc,omitempty"`
		}
		entries := make([]entry, 0, len(cases))
		for _, c := range cases {
			st, err := court.Examine(ctx, log, c)
			switch {
			case err != nil:
				entries = append(entries, entry{Case: c.ID, Status: "unexamined"})
			case st.Verdict != nil:
				entries = append(entries, entry{Case: c.ID, Status: "guilty"})
			case st.Started:
				pc := st.PC
				entries = append(entries, entry{Case: c.ID, Status: "in-proceedings", PC: &pc})
			default:
				entries = append(entries, entry{Case: c.ID, Status: "filed"})
			}
		}
		return printJSON(entries)
	}
	if len(cases) == 0 {
		fmt.Println("The docket is empty. This will be corrected.")
		return 0
	}
	fmt.Printf("THE DOCKET — %d matter(s) before this court\n\n", len(cases))
	for _, c := range cases {
		st, err := court.Examine(ctx, log, c)
		switch {
		case err != nil:
			fmt.Printf("  %-16s (the file could not be examined)\n", c.ID)
		case st.Verdict != nil:
			fmt.Printf("  %-16s GUILTY — the verdict is final\n", c.ID)
		case st.Started:
			fmt.Printf("  %-16s in proceedings; attention at instruction %d\n", c.ID, st.PC)
		default:
			fmt.Printf("  %-16s filed; the proceedings may begin at any moment\n", c.ID)
		}
	}
	return 0
}

func transcript(ctx context.Context, rest []string) int {
	_, broker, c, ok := caseFlags("transcript", rest, nil)
	if !ok {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	recs, err := log.ReadAll(ctx, c.Filing())
	if err != nil {
		fmt.Fprintf(os.Stderr, "The filing could not be read back: %v\n", err)
		return 1
	}
	for _, r := range recs {
		fmt.Println(string(r.Value))
	}
	return 0
}

func verdict(ctx context.Context, rest []string) int {
	var counsel, asJSON *bool
	_, broker, c, ok := caseFlags("verdict", rest, func(fs *flag.FlagSet) {
		counsel = fs.Bool("counsel", false, "unseal the particulars")
		asJSON = fs.Bool("json", false, "the verdict, as JSON")
	})
	if !ok {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	st, err := court.Examine(ctx, log, c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The case file could not be examined: %v\n", err)
		return 1
	}
	if *asJSON {
		view := struct {
			Case    string `json:"case"`
			Guilty  bool   `json:"guilty"`
			PC      *int64 `json:"pc,omitempty"`
			Pos     string `json:"pos,omitempty"`
			Sealed  string `json:"sealed,omitempty"` // only with --counsel; sealed means sealed
			Counsel bool   `json:"counsel_retained"`
		}{Case: c.ID, Counsel: *counsel}
		code := 0
		if st.Verdict != nil {
			view.Guilty = true
			pc := st.Verdict.PC
			view.PC = &pc
			view.Pos = st.Verdict.Pos
			if *counsel {
				view.Sealed = st.Verdict.Sealed
			}
			code = 1
		}
		if r := printJSON(view); r != 0 {
			return r
		}
		return code
	}
	if st.Verdict == nil {
		fmt.Printf("No verdict has been reached in %s.\n", c.ID)
		fmt.Println("This is not the same as innocence.")
		return 0
	}
	fmt.Println("GUILTY.")
	if *counsel {
		fmt.Printf("\n[counsel] %s\n", st.Verdict.Sealed)
		if st.Verdict.Pos != "" {
			fmt.Printf("[counsel] the offense occurred at %s (instruction %d)\n", st.Verdict.Pos, st.Verdict.PC)
		} else {
			fmt.Printf("[counsel] the offense occurred at instruction %d\n", st.Verdict.PC)
		}
	} else {
		fmt.Println("The particulars are sealed. (Retain counsel: --counsel.)")
	}
	return 1
}

// statusView is the JSON representation of one case file.
type statusView struct {
	Case           string            `json:"case"`
	Started        bool              `json:"started"`
	PC             int64             `json:"pc"`
	StackDepth     int               `json:"stack_depth"`
	AppealsDepth   int               `json:"appeals_depth"`
	ContinuedUntil *time.Time        `json:"continued_until,omitempty"`
	AwaitingUntil  *time.Time        `json:"awaiting_until,omitempty"`
	AwaitingVoice  string            `json:"awaiting_voice,omitempty"`
	HeardOutOfTurn int               `json:"heard_out_of_turn,omitempty"`
	MotionFiled    bool              `json:"motion_filed"`
	MotionSpent    bool              `json:"motion_spent"`
	Records        map[string]string `json:"records"`
	Guilty         bool              `json:"guilty"`
}

func buildStatusView(c docket.Case, st *court.Status) statusView {
	records := make(map[string]string, len(st.Records))
	for n, v := range st.Records {
		records[n] = v.Display()
	}
	return statusView{
		Case:           c.ID,
		Started:        st.Started,
		PC:             st.PC,
		StackDepth:     st.StackDepth,
		AppealsDepth:   st.AppealsDepth,
		ContinuedUntil: st.ContinuedUntil,
		AwaitingUntil:  st.AwaitingUntil,
		AwaitingVoice:  st.AwaitingVoice,
		HeardOutOfTurn: st.HeardOutOfTurn,
		MotionFiled:    st.MotionFiled,
		MotionSpent:    st.MotionSpent,
		Records:        records,
		Guilty:         st.Verdict != nil,
	}
}

func status(ctx context.Context, rest []string) int {
	var asJSON *bool
	_, broker, c, ok := caseFlags("status", rest, func(fs *flag.FlagSet) {
		asJSON = fs.Bool("json", false, "the case file, as JSON")
	})
	if !ok {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	st, err := court.Examine(ctx, log, c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The case file could not be examined: %v\n", err)
		return 1
	}
	if *asJSON {
		return printJSON(buildStatusView(c, st))
	}
	fmt.Printf("IN THE MATTER OF %s — a status report, unsolicited\n\n", c.ID)
	if st.Started {
		fmt.Printf("  The Court's attention rests at instruction %d.\n", st.PC)
	} else {
		fmt.Println("  The proceedings have not yet begun. They may begin at any moment.")
	}
	fmt.Printf("  The dossier holds %d item(s) in evidence.\n", st.StackDepth)
	fmt.Printf("  %d appeal(s) are pending.\n", st.AppealsDepth)
	if st.ContinuedUntil != nil {
		fmt.Printf("  A continuance is in effect until %s. The Court is aware of the time.\n",
			st.ContinuedUntil.Format("2006-01-02 15:04:05.000"))
	}
	if st.AwaitingUntil != nil {
		if st.AwaitingVoice != "" {
			fmt.Printf("  The voice of %s is awaited until %s. After that, the contingency.\n",
				st.AwaitingVoice, st.AwaitingUntil.Format("2006-01-02 15:04:05.000"))
		} else {
			fmt.Printf("  A summons is awaited until %s. After that, the contingency.\n",
				st.AwaitingUntil.Format("2006-01-02 15:04:05.000"))
		}
	}
	if st.HeardOutOfTurn > 0 {
		fmt.Printf("  %d summons(es) were heard out of turn; the records passed over await their own.\n",
			st.HeardOutOfTurn)
	}
	if st.MotionFiled && !st.MotionSpent {
		fmt.Println("  A motion to reconsider is on file. It will be granted once, if ever.")
	}
	if st.MotionSpent {
		fmt.Println("  A motion to reconsider was granted. The Court will not do that again.")
	}
	if len(st.Records) == 0 {
		fmt.Println("  No records are on file.")
	} else {
		fmt.Println("  The records read as follows:")
		names := make([]string, 0, len(st.Records))
		for n := range st.Records {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Printf("    %-20s %s\n", n, st.Records[n].Display())
		}
	}
	if st.Verdict != nil {
		fmt.Println("\n  A VERDICT HAS BEEN REACHED. The verdict is final.")
	}
	return 0
}

func reenact(ctx context.Context, rest []string) int {
	_, broker, c, ok := caseFlags("reenact", rest, nil)
	if !ok {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	if err := court.Reenact(ctx, log, c); err != nil {
		fmt.Fprintf(os.Stderr, "The reenactment could not be arranged: %v\n", err)
		return 1
	}
	fmt.Println("The case will be reenacted in full. Every summons will be")
	fmt.Println("re-served; every proclamation will be re-proclaimed. Nothing")
	fmt.Println("has been deleted — nothing is ever deleted — the case simply")
	fmt.Println("begins again, with its entire history watching.")
	fmt.Println()
	fmt.Printf("Convene:  trial proceed %s\n", c.ID)
	return 0
}

func audit(ctx context.Context, rest []string) int {
	fs := commandFlags("audit")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	docketAll := fs.Bool("docket", false, "audit every matter on the docket, and survey the walls")
	asJSON := fs.Bool("json", false, "the report, as JSON")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	var caseID string
	if fs.NArg() >= 1 {
		caseID = fs.Arg(0)
		if fs.NArg() > 1 {
			if err := fs.Parse(fs.Args()[1:]); err != nil {
				return 2
			}
		}
	}
	if *docketAll {
		log, code := openLog(ctx, *broker)
		if log == nil {
			return code
		}
		defer log.Close()
		return burrow(ctx, log)
	}
	if caseID == "" {
		fmt.Fprintln(os.Stderr, "trial audit: a case number is required (or --docket, to listen at every wall).")
		return 2
	}
	c, err := docket.ParseCase(caseID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trial audit: %v\n", err)
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	report, err := court.Audit(ctx, log, c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The audit could not be conducted: %v\n", err)
		return 1
	}
	if *asJSON {
		view := struct {
			Case       string   `json:"case"`
			Timelines  int      `json:"timelines"`
			Steps      int64    `json:"steps"`
			Consistent bool     `json:"consistent"`
			Findings   []string `json:"findings"`
			Notes      []string `json:"notes"`
		}{c.ID, report.Timelines, report.Steps, report.Consistent(), report.Findings, report.Notes}
		if view.Findings == nil {
			view.Findings = []string{}
		}
		if view.Notes == nil {
			view.Notes = []string{}
		}
		if r := printJSON(view); r != 0 {
			return r
		}
		if report.Consistent() {
			return 0
		}
		return 1
	}
	fmt.Printf("IN THE MATTER OF %s — the warden's report\n\n", c.ID)
	fmt.Printf("  The case was reenacted in chambers, against a copy: %d timeline(s),\n", report.Timelines)
	fmt.Printf("  %d committed step(s). Nothing was disturbed.\n", report.Steps)
	for _, n := range report.Notes {
		fmt.Printf("\n  NOTED: %s.\n", n)
	}
	if report.Consistent() {
		fmt.Println("\n  The record is consistent with itself. The verdict, the records,")
		fmt.Println("  and the proclamations agree with their reenactments. The tomb")
		fmt.Println("  is undisturbed, and its occupant is exactly who the stone says.")
		return 0
	}
	fmt.Printf("\n  THE RECORD DOES NOT AGREE WITH ITSELF. %d finding(s):\n", len(report.Findings))
	for i, f := range report.Findings {
		fmt.Printf("\n  %d. %s.\n", i+1, f)
	}
	fmt.Println("\n  Someone has been in the files.")
	return 1
}

func appeal(ctx context.Context, rest []string) int {
	var atStep *int64
	_, broker, c, ok := caseFlags("appeal", rest, func(fs *flag.FlagSet) {
		atStep = fs.Int64("at-step", court.AppealAsItStands, "take the case as it stood after n committed steps")
	})
	if !ok {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	n, err := court.Appeal(ctx, log, c, *atStep)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The appeal could not be taken: %v\n", err)
		return 1
	}
	fmt.Println("The appeal is taken. The legend now comes down in a further")
	fmt.Println("version; the original is not touched, and each ends as it ends.")
	fmt.Println()
	if *atStep == court.AppealAsItStands {
		fmt.Printf("On appeal from:  %s, as it stands\n", c.ID)
	} else {
		fmt.Printf("On appeal from:  %s, as it stood after step %d\n", c.ID, *atStep)
	}
	fmt.Printf("The new matter:  %s\n", n.ID)
	fmt.Println()
	fmt.Printf("Convene:  trial proceed %s\n", n.ID)
	fmt.Printf("Amend:    trial amend %s <k2.trial>\n", n.ID)
	return 0
}

func profileCmd(ctx context.Context, rest []string) int {
	var top *int
	var asJSON *bool
	_, broker, c, ok := caseFlags("profile", rest, func(fs *flag.FlagSet) {
		top = fs.Int("top", 20, "lines to print")
		asJSON = fs.Bool("json", false, "the meter, as JSON")
	})
	if !ok {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	report, err := court.Profile(ctx, log, c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The profile could not be taken: %v\n", err)
		return 1
	}
	if *asJSON {
		type line struct {
			Count int64  `json:"count"`
			PC    int64  `json:"pc"`
			Op    string `json:"op"`
			Pos   string `json:"pos,omitempty"`
		}
		n := *top
		if n <= 0 || n > len(report.Lines) {
			n = len(report.Lines)
		}
		lines := make([]line, 0, n)
		for _, l := range report.Lines[:n] {
			lines = append(lines, line{l.Count, l.PC, l.Op, l.Pos})
		}
		return printJSON(struct {
			Case       string `json:"case"`
			Timelines  int    `json:"timelines"`
			Steps      int64  `json:"steps"`
			Executed   int64  `json:"executed"`
			Consistent bool   `json:"consistent"`
			Lines      []line `json:"lines"`
		}{c.ID, report.Timelines, report.Steps, report.Executed, report.Consistent, lines})
	}
	fmt.Printf("IN THE MATTER OF %s — where the time went\n\n", c.ID)
	fmt.Printf("  %d timeline(s), %d committed step(s), %d instruction execution(s),\n", report.Timelines, report.Steps, report.Executed)
	fmt.Println("  metered in chambers. The philosopher had to catch the top to study")
	fmt.Println("  it; the record was never spinning to begin with.")
	if !report.Consistent {
		fmt.Println("\n  CAUTION: the record did not agree with its own reenactment; this")
		fmt.Println("  is a profile of the reenactment. Audit before believing anything.")
	}
	fmt.Println()
	fmt.Printf("  %10s  %8s  %-16s %s\n", "EXECUTIONS", "ADDRESS", "SEAL", "POSITION")
	n := *top
	if n <= 0 || n > len(report.Lines) {
		n = len(report.Lines)
	}
	for _, l := range report.Lines[:n] {
		fmt.Printf("  %10d  %8d  %-16s %s\n", l.Count, l.PC, l.Op, l.Pos)
	}
	if n < len(report.Lines) {
		fmt.Printf("\n  ... and %d cooler instruction(s), which also served.\n", len(report.Lines)-n)
	}
	return 0
}

// burrow audits every case and court-wide record.
func burrow(ctx context.Context, log *docket.KafkaLog) int {
	b, err := court.SurveyBurrow(ctx, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The burrow could not be surveyed: %v\n", err)
		return 1
	}
	fmt.Printf("THE BURROW — the courthouse, listened at from inside\n\n")
	if len(b.Audits) == 0 {
		fmt.Println("  The docket is empty. The stillness is complete, and a little")
		fmt.Println("  suspicious.")
		return 0
	}
	consistent := 0
	for _, a := range b.Audits {
		if a.Consistent() {
			consistent++
		}
	}
	fmt.Printf("  %d matter(s) on the docket; %d audited consistent.\n\n", len(b.Audits), consistent)
	for _, a := range b.Audits {
		if a.Consistent() {
			fmt.Printf("  %-16s consistent; %d timeline(s), %d step(s) replayed\n", a.Case, a.Timelines, a.Steps)
		} else {
			fmt.Printf("  %-16s THE RECORD DOES NOT AGREE WITH ITSELF; %d finding(s):\n", a.Case, len(a.Findings))
			for i, f := range a.Findings {
				fmt.Printf("      %d. %s.\n", i+1, f)
			}
		}
		for _, n := range a.Notes {
			fmt.Printf("      noted: %s.\n", n)
		}
	}
	if len(b.Drafts) > 0 {
		fmt.Println()
		drafted := make([]string, 0, len(b.Drafts))
		for c := range b.Drafts {
			drafted = append(drafted, c)
		}
		sort.Strings(drafted)
		for _, c := range drafted {
			offs := b.Drafts[c]
			fmt.Printf("  The archive of %s holds %d draft(s), entered at the counter\n", c, len(offs))
			fmt.Printf("  and never cataloged (offset(s) %v). The archive accumulates\n", offs)
			fmt.Println("  drafts; that is its nature; here is where they are.")
		}
	}
	if len(b.Unconvened) > 0 {
		fmt.Printf("\n  %d matter(s) stand unconvened, and no ledger records commencing\n", len(b.Unconvened))
		fmt.Println("  them: filed and waiting, or the remainder of an official who")
		fmt.Println("  perished between counter and commitment. From inside the burrow")
		fmt.Printf("  the two are indistinguishable: %s.\n", strings.Join(b.Unconvened, ", "))
	}
	if len(b.SpentMotions) > 0 {
		fmt.Printf("\n  %d motion(s) to reconsider stand spent: %s. The Court will not\n", len(b.SpentMotions), strings.Join(b.SpentMotions, ", "))
		fmt.Println("  do that again, and here is everyone it will not do it again for.")
	}
	if b.Consistent() {
		fmt.Println("\n  But the most beautiful thing about my burrow is the stillness.")
		return 0
	}
	fmt.Println("\n  There is a faint hissing in the walls. Someone has been in the files.")
	return 1
}

// mcpCmd runs the Advocate MCP server on standard input and output.
func mcpCmd(ctx context.Context, rest []string) int {
	fs := commandFlags("mcp")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	fmt.Fprintln(os.Stderr, "(The Advocate is retained and listening on stdio. He is very good; he is also the only one.)")
	srv := &advocate.Server{Log: log, In: os.Stdin, Out: os.Stdout}
	if err := srv.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "The Advocate has withdrawn from the matter: %v\n", err)
		return 1
	}
	return 0
}

func counselCmd(ctx context.Context, rest []string) int {
	fs := commandFlags("counsel")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	fmt.Fprintln(os.Stderr, "(Counsel is retained and listening on stdio, LSP 3.x. Diagnostics are Gregor's own; the editor and the Court will never disagree.)")
	srv := &counsel.Server{In: os.Stdin, Out: os.Stdout, Version: resolveVersion()}
	if err := srv.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Counsel has withdrawn from the matter: %v\n", err)
		return 1
	}
	return 0
}

func watch(ctx context.Context, rest []string) int {
	fs := commandFlags("watch")
	broker := fs.String("broker", brokerDefault(), "the courthouse")
	interval := fs.Duration("interval", 2*time.Second, "how often the docket is swept")
	once := fs.Bool("once", false, "print the docket once and leave the gallery")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "trial watch: --interval must be greater than zero.")
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	for {
		reports, err := court.ReportDocket(ctx, log)
		if err != nil {
			if ctx.Err() != nil {
				return 0
			}
			fmt.Fprintf(os.Stderr, "The docket could not be read: %v\n", err)
			return 1
		}
		if !*once && isTTY(os.Stdout) {
			// Clear only an interactive terminal. Pipes receive plain tables.
			fmt.Print("\033[2J\033[H")
		}
		fmt.Printf("THE DOCKET, observed %s. %d matter(s) before the court.\n\n",
			time.Now().Format("15:04:05"), len(reports))
		fmt.Printf("  %-14s %8s %8s %8s %8s %8s  %s\n", "CASE", "PC", "END", "BEHIND", "DOSSIER", "APPEALS", "STATUS")
		for _, r := range reports {
			fmt.Printf("  %-14s %8d %8d %8d %8d %8d  %s\n",
				r.Case.ID, r.PC, r.End, r.Lag, r.StackDepth, r.AppealsDepth, r.Status)
		}
		if len(reports) == 0 {
			fmt.Println("  (No matters. This will not last.)")
		}
		fmt.Println()
		fmt.Println("BEHIND is how far the Court's attention has fallen behind the")
		fmt.Println("proceedings: consumer lag, rendered as what it is here.")
		if *once {
			return 0
		}
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(*interval):
		}
	}
}

func burn(ctx context.Context, rest []string) int {
	var insist *bool
	_, broker, c, ok := caseFlags("burn", rest, func(fs *flag.FlagSet) {
		insist = fs.Bool("with-prejudice", false, "insist upon the burning; the dismissal is final")
	})
	if !ok {
		return 2
	}
	if !*insist {
		fmt.Printf("The request to burn %s has been received and considered.\n\n", c.ID)
		fmt.Println("Refused. The Court does not destroy its own records.")
		fmt.Println()
		fmt.Printf("(If you insist:  trial burn %s --with-prejudice)\n", c.ID)
		return 1
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	if err := log.DeleteCaseTopics(ctx, c); err != nil {
		fmt.Fprintf(os.Stderr, "The fire went out: %v\n", err)
		return 1
	}
	fmt.Printf("The file of %s has been burned, with prejudice.\n", c.ID)
	fmt.Println("The Court notes for the record that the record no longer")
	fmt.Println("exists; it notes this in the record.")
	return 0
}
