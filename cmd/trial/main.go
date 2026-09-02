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
		return "unknown"
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
				dirty = ", modified"
			}
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		return fmt.Sprintf("devel, revision %s%s", rev, dirty)
	}
	return "(devel)"
}

func versionCmd() int {
	fmt.Printf("trial %s\n", resolveVersion())
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
		if len(rest) > 1 {
			unexpectedArgs("help", rest[1:])
			return 2
		}
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
		if unexpectedArgs(cmd, rest) {
			return 2
		}
		return summon(ctx)
	case "dismiss":
		if unexpectedArgs(cmd, rest) {
			return 2
		}
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
		if unexpectedArgs("version", rest) {
			return 2
		}
		return versionCmd()
	}
	fmt.Fprintf(os.Stderr, "trial: unknown command %q.", cmd)
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
	fmt.Fprintf(os.Stderr, "trial help: unknown command %q. See 'trial help'.\n", name)
	return 2
}

func compose(ctx context.Context, verb string, extra ...string) int {
	args := append([]string{"compose", verb}, extra...)
	c := exec.CommandContext(ctx, "docker", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Docker Compose failed: %v\n(Is Docker installed and running?)\n", err)
		return 1
	}
	return 0
}

func summon(ctx context.Context) int {
	if code := compose(ctx, "up", "-d"); code != 0 {
		return code
	}
	fmt.Println()
	fmt.Println("Kafka is running at localhost:9092.")
	fmt.Println("Try: trial file examples/hello.trial")
	return 0
}

func dismiss(ctx context.Context) int {
	if code := compose(ctx, "down"); code != 0 {
		return code
	}
	fmt.Println()
	fmt.Println("Kafka is stopped. Stored case data was retained.")
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

// parseFirstArg parses flags on either side of the first positional argument.
// Any remaining positional arguments are left in fs.Args.
func parseFirstArg(fs *flag.FlagSet, args []string) (string, bool) {
	terminated := terminatesOptionsBeforeFirstArg(fs, args)
	if err := fs.Parse(args); err != nil {
		return "", false
	}
	if fs.NArg() == 0 {
		return "", true
	}
	first := fs.Arg(0)
	suffix := fs.Args()[1:]
	if terminated {
		// The first parse consumed --. Restore it so the second parse leaves
		// every later token positional too.
		suffix = append([]string{"--"}, suffix...)
	}
	if err := fs.Parse(suffix); err != nil {
		return "", false
	}
	return first, true
}

// terminatesOptionsBeforeFirstArg distinguishes a real -- terminator from a
// token equal to "--" that is consumed as a non-boolean flag's value.
func terminatesOptionsBeforeFirstArg(fs *flag.FlagSet, args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return true
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			return false
		}
		name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
		name, _, hasValue := strings.Cut(name, "=")
		option := fs.Lookup(name)
		if option == nil || hasValue {
			continue
		}
		if boolean, ok := option.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
			continue
		}
		i++ // the next token is this option's value, even when it is "--"
	}
	return false
}

func unexpectedArgs(name string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	fmt.Fprintf(os.Stderr, "trial %s: unexpected argument %q. See 'trial help %s'.\n", name, args[0], name)
	return true
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
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	if extra != nil {
		extra(fs)
	}
	caseID, ok := parseFirstArg(fs, rest)
	if !ok {
		return nil, "", docket.Case{}, false
	}
	if caseID == "" {
		fmt.Fprintf(os.Stderr, "trial %s: a case number is required. See 'trial help %s'.\n", name, name)
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
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	counsel := fs.Bool("counsel", false, "reveal the particulars of a rejection")
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "print only the case number")
	fs.BoolVar(&quiet, "q", false, "print only the case number")
	path, ok := parseFirstArg(fs, rest)
	if !ok {
		return 2
	}
	if path == "" || fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "trial file: exactly one .trial filing is accepted per visit. See 'trial help file'.")
		return 2
	}
	src, err := readSource(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trial file: read source: %v\n", err)
		return 1
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	c, err := court.File(ctx, log, string(src))
	if err != nil {
		return reportFileError(os.Stderr, c, err, *counsel)
	}
	if quiet {
		// Quiet output is safe to use in command substitution.
		fmt.Println(c.ID)
		return 0
	}
	fmt.Printf("Case filed: %s\n\n", c.ID)
	fmt.Printf("Run:     trial proceed %s\n", c.ID)
	fmt.Printf("Observe: trial observe %s\n", c.ID)
	return 0
}

func reportFileError(w io.Writer, c docket.Case, err error, revealRejection bool) int {
	if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
		_, _ = fmt.Fprintln(w, "The filing was rejected pursuant to Article §4.2.")
		if revealRejection {
			_, _ = fmt.Fprintf(w, "\n[counsel] %s\n", rej.Error())
		} else {
			_, _ = fmt.Fprintln(w, "\n(Rerun with --counsel for details.)")
		}
		return 1
	}
	if reportRecoverableCaseError(w, "filing", c, err) {
		return 1
	}
	_, _ = fmt.Fprintf(w, "The filing failed: %v\n", err)
	return 1
}

func reportRecoverableCaseError(w io.Writer, operation string, c docket.Case, err error) bool {
	if c.ID == "" {
		return false
	}
	if reportAmbiguousCommit(w, operation, "case "+c.ID, err) {
		return true
	}
	_, _ = fmt.Fprintf(w, "The %s failed for case %s: %v\nThe case may be partial; inspect case %s before retrying.\n", operation, c.ID, err, c.ID)
	return true
}

func reportAmbiguousCommit(w io.Writer, operation, target string, err error) bool {
	if _, ambiguous := errors.AsType[*docket.AmbiguousCommitError](err); !ambiguous {
		return false
	}
	_, _ = fmt.Fprintf(w, "The %s result is uncertain for %s: %v\nIt may already be committed; inspect %s before taking further action.\n", operation, target, err, target)
	return true
}

func proceedCase(ctx context.Context, rest []string) int {
	fs := commandFlags("proceed")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	docketMode := fs.Bool("docket", false, "process every current and future case")
	expedited := fs.Int("expedited", 1, "instructions per committed step; above 1, the attention is recorded at the pace of the batch")
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "suppress progress output; use the exit status")
	fs.BoolVar(&quiet, "q", false, "suppress progress output; use the exit status")
	caseID, ok := parseFirstArg(fs, rest)
	if !ok {
		return 2
	}
	if *docketMode {
		if caseID != "" || unexpectedArgs("proceed", fs.Args()) {
			fmt.Fprintln(os.Stderr, "trial proceed: --docket does not take a case number.")
			return 2
		}
		return proceedDocket(ctx, *broker)
	}
	if unexpectedArgs("proceed", fs.Args()) {
		return 2
	}
	if *expedited < 1 || *expedited > 10_000 {
		fmt.Fprintln(os.Stderr, "trial proceed: --expedited must be between 1 and 10000 instructions per step.")
		return 2
	}
	if caseID == "" {
		fmt.Fprintln(os.Stderr, "trial proceed: a case number is required (or use --docket).")
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
	say("Proceeding with %s. Press Ctrl+C to stop safely.\n\n", c.ID)

	if *expedited > 1 {
		say("Batch size: up to %d instructions per committed step.\n\n", *expedited)
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
		say("The case is adjourned. Resume with: trial proceed %s\n", c.ID)
	case court.OutcomeGuilty:
		say("A verdict has been reached:  trial verdict %s\n", c.ID)
		return 1
	case court.OutcomeApparentAcquittal:
		say("Execution reached the current end of the proceedings.\n")
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

	fmt.Println("Processing current and future cases. Press Ctrl+C to stop safely.")
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
		fmt.Fprintf(os.Stderr, "Docket processing failed: %v\n", err)
		return 1
	}
	fmt.Println()
	fmt.Println("Docket processing stopped.")
	return 0
}

func observe(ctx context.Context, rest []string) int {
	var fromBeginning *bool
	fs, broker, c, ok := caseFlags("observe", rest, func(fs *flag.FlagSet) {
		fromBeginning = fs.Bool("from-the-beginning", false, "read the full record")
	})
	if !ok {
		return 2
	}
	if unexpectedArgs("observe", fs.Args()) {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	recs, err := log.ReadAll(ctx, c.Proclamations())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Case output could not be read: %v\n", err)
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
	fmt.Fprintf(os.Stderr, "Following output for %s. Press Ctrl+C to stop.\n", c.ID)
	for {
		rec, err := log.Fetch(ctx, c.Proclamations(), next, true)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Fprintln(os.Stderr, "\nStopped following output.")
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
		fmt.Fprintln(os.Stderr, "trial serve: at least one value is required. See 'trial help serve'.")
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	if err := appendSummons(ctx, log, c, values); err != nil {
		if reportAmbiguousCommit(os.Stderr, "input batch", "case "+c.ID, err) {
			return 1
		}
		fmt.Fprintf(os.Stderr, "The summonses could not be served atomically: %v\n", err)
		return 1
	}
	if *quiet {
		return 0
	}
	if len(values) == 1 {
		fmt.Println("1 input appended.")
	} else {
		fmt.Printf("%d inputs appended.\n", len(values))
	}
	return 0
}

func appendSummons(ctx context.Context, log docket.Log, c docket.Case, values []string) error {
	appends := make([]docket.StepAppend, len(values))
	for i, value := range values {
		appends[i] = docket.StepAppend{Topic: c.Summons(), Value: []byte(value)}
	}
	_, err := log.AppendBatch(ctx, appends)
	return err
}

func amend(ctx context.Context, rest []string) int {
	var counsel *bool
	fs, broker, c, ok := caseFlags("amend", rest, func(fs *flag.FlagSet) {
		counsel = fs.Bool("counsel", false, "reveal the particulars of a rejection")
	})
	if !ok {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "trial amend: exactly one supplemental filing (Form K-2) is required. See 'trial help amend'.")
		return 2
	}
	src, err := readSource(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "trial amend: read source: %v\n", err)
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
			fmt.Fprintln(os.Stderr, "The supplemental filing was rejected pursuant to Article §4.2.")
			if *counsel {
				fmt.Fprintf(os.Stderr, "\n[counsel] %s\n", rej.Error())
			} else {
				fmt.Fprintln(os.Stderr, "(Rerun with --counsel for details.)")
			}
			return 1
		}
		if reportAmbiguousCommit(os.Stderr, "supplemental filing", "case "+c.ID, err) {
			return 1
		}
		fmt.Fprintf(os.Stderr, "The supplemental filing failed: %v\n", err)
		return 1
	}
	fmt.Printf("Appended %d instruction(s) to %s.\n", n, c.ID)
	fmt.Printf("Resume with: trial proceed %s\n", c.ID)
	return 0
}

func enact(ctx context.Context, rest []string) int {
	fs := commandFlags("enact")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	counsel := fs.Bool("counsel", false, "reveal the particulars of a rejection")
	enactCanon := fs.Bool("canon", false, "enact the standard statutes shipped with the binary, in dependency order")
	path, ok := parseFirstArg(fs, rest)
	if !ok {
		return 2
	}
	if *enactCanon {
		if path != "" || fs.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "trial enact --canon: no statute path may be supplied.")
			return 2
		}
		log, code := openLog(ctx, *broker)
		if log == nil {
			return code
		}
		defer log.Close()
		fmt.Println("Enacting the bundled canon:")
		fmt.Println()
		for _, file := range canon.Files() {
			src, err := canon.FS.ReadFile(file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "The bundled canon is missing %s: %v\n", file, err)
				return 1
			}
			name, n, err := court.Enact(ctx, log, string(src))
			if err != nil {
				if name != "" && reportAmbiguousCommit(os.Stderr, "enactment", fmt.Sprintf("statute %s (enactment %d)", name, n), err) {
					return 1
				}
				fmt.Fprintf(os.Stderr, "The statute %s was not enacted: %v\n", file, err)
				return 1
			}
			fmt.Printf("    %s (enactment %d)\n", name, n)
		}
		fmt.Println()
		fmt.Println("Cases can now incorporate these offices:")
		fmt.Println("    INCORPORATE BY REFERENCE statutes-of-schedules.")
		fmt.Println("Dependencies are incorporated transitively.")
		return 0
	}
	if path == "" || fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "trial enact: provide one Form S-1 statute, or use --canon. See 'trial help enact'.")
		return 2
	}
	src, err := readSource(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trial enact: read source: %v\n", err)
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
			fmt.Fprintln(os.Stderr, "The statute was rejected pursuant to Article §4.2.")
			if *counsel {
				fmt.Fprintf(os.Stderr, "\n[counsel] %s\n", rej.Error())
			} else {
				fmt.Fprintln(os.Stderr, "(Rerun with --counsel for details.)")
			}
			return 1
		}
		if name != "" && reportAmbiguousCommit(os.Stderr, "enactment", fmt.Sprintf("statute %s (enactment %d)", name, n), err) {
			return 1
		}
		fmt.Fprintf(os.Stderr, "The statute was not enacted: %v\n", err)
		return 1
	}
	fmt.Println("Statute enacted.")
	fmt.Println()
	fmt.Printf("    Statute:   %s (enactment %d)\n", name, n)
	fmt.Println()
	fmt.Println("Cases can incorporate its offices with:")
	fmt.Printf("    INCORPORATE BY REFERENCE %s.\n", name)
	fmt.Println("Existing cases keep the version they incorporated.")
	return 0
}

func statutes(ctx context.Context, rest []string) int {
	fs := commandFlags("statutes")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if unexpectedArgs("statutes", fs.Args()) {
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	names, err := log.ListStatutes(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Statutes could not be listed: %v\n", err)
		return 1
	}
	if len(names) == 0 {
		fmt.Println("No statutes have been enacted.")
		return 0
	}
	fmt.Printf("STATUTES (%d)\n\n", len(names))
	for _, n := range names {
		fmt.Printf("  %s\n", n)
	}
	fmt.Println()
	fmt.Println("Incorporate any of them: INCORPORATE BY REFERENCE <statute>.")
	return 0
}

func hearing(ctx context.Context, rest []string) int {
	fs := commandFlags("hearing")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	counsel := fs.Bool("counsel", false, "unseal any verdict's particulars")
	caseID, ok := parseFirstArg(fs, rest)
	if !ok {
		return 2
	}
	if unexpectedArgs("hearing", fs.Args()) {
		return 2
	}

	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	var h *court.Hearing
	var err error
	if caseID != "" {
		c, parseErr := docket.ParseCase(caseID)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "trial hearing: %v\n", parseErr)
			return 2
		}
		h, err = court.ResumeHearing(ctx, log, c)
	} else {
		h, err = court.OpenHearing(ctx, log)
	}
	if err != nil {
		if h != nil && reportRecoverableCaseError(os.Stderr, "hearing filing", h.Case, err) {
			return 1
		}
		fmt.Fprintf(os.Stderr, "The hearing could not be opened: %v\n", err)
		return 1
	}

	// Pipes receive statements and proclamations without prompts.
	interactive := isTTY(os.Stdin)
	if interactive {
		fmt.Printf("Hearing for %s.\n", h.Case.ID)
		fmt.Println("Enter one statement per line, ending with a period.")
		fmt.Println("Press Ctrl+C or Ctrl+D to leave; the case is retained.")
		fmt.Println()
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for {
		if interactive {
			fmt.Print("K.> ")
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "The hearing input could not be read: %v\n", err)
				return 1
			}
			if interactive {
				fmt.Println()
				fmt.Printf("Hearing closed. Resume with: trial hearing %s\n", h.Case.ID)
			}
			return 0
		}
		line := scanner.Text()
		if len(line) == 0 || len(strings.TrimSpace(line)) == 0 {
			if interactive {
				fmt.Println("(empty input ignored)")
			}
			continue
		}
		proclaimed, verdict, err := h.Submit(ctx, line)
		if err != nil {
			if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
				fmt.Println("The statement was rejected pursuant to Article §4.2.")
				if *counsel {
					fmt.Printf("[counsel] %s\n", rej.Error())
				}
				continue
			}
			if reportAmbiguousCommit(os.Stderr, "hearing submission", "case "+h.Case.ID, err) {
				return 1
			}
			fmt.Fprintf(os.Stderr, "The hearing failed: %v\n", err)
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
				fmt.Println("The details are sealed. Use --counsel to display them.")
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
		fmt.Println("No deposition files were found.")
		return 0
	}

	fmt.Println("RUNNING DEPOSITIONS")
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
			fmt.Printf("          OUTPUT (%d entries):\n", len(res.Said))
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
		fmt.Printf("%d deposition(s) passed.\n", len(files))
		return 0
	}
	fmt.Printf("%d deposition(s) ran; %d failed.\n", len(files), contradicted)
	return 1
}

func docketCmd(ctx context.Context, rest []string) int {
	fs := commandFlags("docket")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	asJSON := fs.Bool("json", false, "the docket, as JSON")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if unexpectedArgs("docket", fs.Args()) {
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	cases, err := log.ListCases(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The docket could not be read: %v\n", err)
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
		fmt.Println("The docket is empty.")
		return 0
	}
	fmt.Printf("DOCKET (%d cases)\n\n", len(cases))
	for _, c := range cases {
		st, err := court.Examine(ctx, log, c)
		switch {
		case err != nil:
			fmt.Printf("  %-16s status unavailable\n", c.ID)
		case st.Verdict != nil:
			fmt.Printf("  %-16s guilty\n", c.ID)
		case st.Started:
			fmt.Printf("  %-16s in proceedings; attention at instruction %d\n", c.ID, st.PC)
		default:
			fmt.Printf("  %-16s filed\n", c.ID)
		}
	}
	return 0
}

func transcript(ctx context.Context, rest []string) int {
	fs, broker, c, ok := caseFlags("transcript", rest, nil)
	if !ok {
		return 2
	}
	if unexpectedArgs("transcript", fs.Args()) {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	recs, err := log.ReadAll(ctx, c.Filing())
	if err != nil {
		fmt.Fprintf(os.Stderr, "The filing could not be read: %v\n", err)
		return 1
	}
	for _, r := range recs {
		fmt.Println(string(r.Value))
	}
	return 0
}

func verdict(ctx context.Context, rest []string) int {
	var counsel, asJSON *bool
	fs, broker, c, ok := caseFlags("verdict", rest, func(fs *flag.FlagSet) {
		counsel = fs.Bool("counsel", false, "unseal the particulars")
		asJSON = fs.Bool("json", false, "the verdict, as JSON")
	})
	if !ok {
		return 2
	}
	if unexpectedArgs("verdict", fs.Args()) {
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
		fmt.Println("The details are sealed. Use --counsel to display them.")
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
	fs, broker, c, ok := caseFlags("status", rest, func(fs *flag.FlagSet) {
		asJSON = fs.Bool("json", false, "the case file, as JSON")
	})
	if !ok {
		return 2
	}
	if unexpectedArgs("status", fs.Args()) {
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
	fmt.Printf("STATUS %s\n\n", c.ID)
	if st.Started {
		fmt.Printf("  Next instruction: %d\n", st.PC)
	} else {
		fmt.Println("  The case has not started.")
	}
	fmt.Printf("  Operand stack depth: %d\n", st.StackDepth)
	fmt.Printf("  Call stack depth: %d\n", st.AppealsDepth)
	if st.ContinuedUntil != nil {
		fmt.Printf("  Continued until %s.\n",
			st.ContinuedUntil.Format("2006-01-02 15:04:05.000"))
	}
	if st.AwaitingUntil != nil {
		if st.AwaitingVoice != "" {
			fmt.Printf("  Waiting for input from %s until %s.\n",
				st.AwaitingVoice, st.AwaitingUntil.Format("2006-01-02 15:04:05.000"))
		} else {
			fmt.Printf("  Waiting for input until %s.\n",
				st.AwaitingUntil.Format("2006-01-02 15:04:05.000"))
		}
	}
	if st.HeardOutOfTurn > 0 {
		fmt.Printf("  Inputs consumed out of turn: %d\n",
			st.HeardOutOfTurn)
	}
	if st.MotionFiled && !st.MotionSpent {
		fmt.Println("  A motion to reconsider is pending.")
	}
	if st.MotionSpent {
		fmt.Println("  The motion to reconsider has been spent.")
	}
	if len(st.Records) == 0 {
		fmt.Println("  Records: none")
	} else {
		fmt.Println("  Records:")
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
		fmt.Println("\n  Verdict: GUILTY")
	}
	return 0
}

func reenact(ctx context.Context, rest []string) int {
	fs, broker, c, ok := caseFlags("reenact", rest, nil)
	if !ok {
		return 2
	}
	if unexpectedArgs("reenact", fs.Args()) {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	if err := court.Reenact(ctx, log, c); err != nil {
		if reportAmbiguousCommit(os.Stderr, "replay reset", "case "+c.ID, err) {
			return 1
		}
		fmt.Fprintf(os.Stderr, "The reenactment could not be arranged: %v\n", err)
		return 1
	}
	fmt.Println("Replay markers appended. Recorded inputs, clock readings, and")
	fmt.Println("random draws will be reused; existing history is retained.")
	fmt.Println()
	fmt.Printf("Convene:  trial proceed %s\n", c.ID)
	return 0
}

func audit(ctx context.Context, rest []string) int {
	fs := commandFlags("audit")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	docketAll := fs.Bool("docket", false, "audit every case and court-wide record")
	asJSON := fs.Bool("json", false, "the report, as JSON")
	caseID, ok := parseFirstArg(fs, rest)
	if !ok {
		return 2
	}
	if *docketAll {
		if caseID != "" || fs.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "trial audit: --docket does not take a case number.")
			return 2
		}
		log, code := openLog(ctx, *broker)
		if log == nil {
			return code
		}
		defer log.Close()
		return auditDocket(ctx, log)
	}
	if unexpectedArgs("audit", fs.Args()) {
		return 2
	}
	if caseID == "" {
		fmt.Fprintln(os.Stderr, "trial audit: a case number is required (or use --docket).")
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
	fmt.Printf("AUDIT %s\n\n", c.ID)
	fmt.Printf("  Replayed %d timeline(s) and %d committed step(s).\n", report.Timelines, report.Steps)
	for _, n := range report.Notes {
		fmt.Printf("\n  Note: %s.\n", n)
	}
	if report.Consistent() {
		fmt.Println("\n  The recorded state and output match the replay.")
		return 0
	}
	fmt.Printf("\n  The replay found %d inconsistency(ies):\n", len(report.Findings))
	for i, f := range report.Findings {
		fmt.Printf("\n  %d. %s.\n", i+1, f)
	}
	return 1
}

func appeal(ctx context.Context, rest []string) int {
	var atStep *int64
	fs, broker, c, ok := caseFlags("appeal", rest, func(fs *flag.FlagSet) {
		atStep = fs.Int64("at-step", court.AppealAsItStands, "take the case as it stood after n committed steps")
	})
	if !ok {
		return 2
	}
	if unexpectedArgs("appeal", fs.Args()) {
		return 2
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	n, err := court.Appeal(ctx, log, c, *atStep)
	if err != nil {
		if reportRecoverableCaseError(os.Stderr, "appeal", n, err) {
			return 1
		}
		fmt.Fprintf(os.Stderr, "The appeal could not be taken: %v\n", err)
		return 1
	}
	fmt.Println("Appeal created. The original case was not changed.")
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
	fs, broker, c, ok := caseFlags("profile", rest, func(fs *flag.FlagSet) {
		top = fs.Int("top", 20, "lines to print")
		asJSON = fs.Bool("json", false, "the meter, as JSON")
	})
	if !ok {
		return 2
	}
	if unexpectedArgs("profile", fs.Args()) {
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
	fmt.Printf("PROFILE %s\n\n", c.ID)
	fmt.Printf("  %d timeline(s), %d committed step(s), %d instruction execution(s),\n", report.Timelines, report.Steps, report.Executed)
	fmt.Println("  measured during replay.")
	if !report.Consistent {
		fmt.Println("\n  Warning: the replay did not match the record. Run trial audit before using this profile.")
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
		fmt.Printf("\n  ... %d additional instruction(s) omitted.\n", len(report.Lines)-n)
	}
	return 0
}

// auditDocket audits every case and court-wide record.
func auditDocket(ctx context.Context, log *docket.KafkaLog) int {
	b, err := court.SurveyBurrow(ctx, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Docket audit failed: %v\n", err)
		return 1
	}
	fmt.Println("DOCKET AUDIT")
	fmt.Println()
	if len(b.Audits) == 0 {
		fmt.Println("  The docket is empty.")
		return 0
	}
	consistent := 0
	for _, a := range b.Audits {
		if a.Consistent() {
			consistent++
		}
	}
	fmt.Printf("  %d case(s); %d replayed consistently.\n\n", len(b.Audits), consistent)
	for _, a := range b.Audits {
		if a.Consistent() {
			fmt.Printf("  %-16s consistent; %d timeline(s), %d step(s) replayed\n", a.Case, a.Timelines, a.Steps)
		} else {
			fmt.Printf("  %-16s inconsistent; %d finding(s):\n", a.Case, len(a.Findings))
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
			fmt.Printf("  %s has %d uncataloged archive record(s) at offset(s) %v.\n", c, len(offs), offs)
		}
	}
	if len(b.Unconvened) > 0 {
		fmt.Printf("\n  %d case(s) have not started and have no commencement record: %s.\n",
			len(b.Unconvened), strings.Join(b.Unconvened, ", "))
	}
	if len(b.SpentMotions) > 0 {
		fmt.Printf("\n  %d spent motion(s) to reconsider: %s.\n",
			len(b.SpentMotions), strings.Join(b.SpentMotions, ", "))
	}
	if b.Consistent() {
		fmt.Println("\n  The docket and court-wide records are consistent.")
		return 0
	}
	fmt.Println("\n  The audit found inconsistencies.")
	return 1
}

// mcpCmd runs the Advocate MCP server on standard input and output.
func mcpCmd(ctx context.Context, rest []string) int {
	fs := commandFlags("mcp")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if unexpectedArgs("mcp", fs.Args()) {
		return 2
	}
	log, code := openLog(ctx, *broker)
	if log == nil {
		return code
	}
	defer log.Close()

	fmt.Fprintln(os.Stderr, "trial MCP server listening on stdio")
	srv := newAdvocateServer(log, os.Stdin, os.Stdout)
	if err := srv.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server stopped: %v\n", err)
		return 1
	}
	return 0
}

func newAdvocateServer(log docket.Log, in io.Reader, out io.Writer) *advocate.Server {
	return &advocate.Server{
		Log:     log,
		In:      in,
		Out:     out,
		Version: resolveVersion(),
	}
}

func counselCmd(ctx context.Context, rest []string) int {
	fs := commandFlags("counsel")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if unexpectedArgs("counsel", fs.Args()) {
		return 2
	}
	fmt.Fprintln(os.Stderr, "trial LSP server listening on stdio")
	srv := &counsel.Server{In: os.Stdin, Out: os.Stdout, Version: resolveVersion()}
	if err := srv.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "LSP server stopped: %v\n", err)
		return 1
	}
	return 0
}

func watch(ctx context.Context, rest []string) int {
	fs := commandFlags("watch")
	broker := fs.String("broker", brokerDefault(), "Kafka broker address")
	interval := fs.Duration("interval", 2*time.Second, "refresh interval")
	once := fs.Bool("once", false, "print one snapshot and exit")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if unexpectedArgs("watch", fs.Args()) {
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
		fmt.Printf("DOCKET %s (%d cases)\n\n",
			time.Now().Format("15:04:05"), len(reports))
		fmt.Printf("  %-14s %8s %8s %8s %8s %8s  %s\n", "CASE", "PC", "END", "BEHIND", "DOSSIER", "APPEALS", "STATUS")
		for _, r := range reports {
			end, lag := docketPositionFields(r)
			fmt.Printf("  %-14s %8d %8s %8s %8d %8d  %s\n",
				r.Case.ID, r.PC, end, lag, r.StackDepth, r.AppealsDepth, r.Status)
		}
		if len(reports) == 0 {
			fmt.Println("  (empty)")
		}
		fmt.Println()
		fmt.Println("BEHIND is the distance between the current instruction and the end.")
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

func docketPositionFields(report court.MatterReport) (end, lag string) {
	if !report.EndKnown {
		return "?", "?"
	}
	return fmt.Sprintf("%d", report.End), fmt.Sprintf("%d", report.Lag)
}

func burn(ctx context.Context, rest []string) int {
	var insist *bool
	fs, broker, c, ok := caseFlags("burn", rest, func(fs *flag.FlagSet) {
		insist = fs.Bool("with-prejudice", false, "confirm permanent deletion")
	})
	if !ok {
		return 2
	}
	if unexpectedArgs("burn", fs.Args()) {
		return 2
	}
	if !*insist {
		fmt.Fprintf(os.Stderr, "Refusing to delete %s without --with-prejudice.\n", c.ID)
		fmt.Fprintf(os.Stderr, "To confirm: trial burn %s --with-prejudice\n", c.ID)
		return 1
	}
	log, code := openLog(ctx, broker)
	if log == nil {
		return code
	}
	defer log.Close()

	if err := log.DeleteCaseTopics(ctx, c); err != nil {
		fmt.Fprintf(os.Stderr, "Case deletion failed: %v\n", err)
		return 1
	}
	fmt.Printf("Deleted case %s. This cannot be undone.\n", c.ID)
	return 0
}
