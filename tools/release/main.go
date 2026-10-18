// Command release prepares immutable release tags and starts the protected
// GitHub workflow. It never builds release assets or approves publication.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	repository     = "lennrt/trial-lang"
	remoteURL      = "https://github.com/" + repository + ".git"
	commandTimeout = 2 * time.Minute
)

var (
	versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?$`)
	shaPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

type commandRunner func(context.Context, time.Duration, string, ...string) ([]byte, error)

type tool struct {
	run commandRunner
	out io.Writer
}

type tagState struct{ object, commit string }
type plan struct {
	sha           string
	local, remote tagState
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	if err := execute(ctx, os.Args[1:], os.Getenv("TRIAL_RELEASE_TAG"), &tool{run: runCommand, out: os.Stdout}); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func execute(ctx context.Context, args []string, defaultTag string, t *tool) error {
	flags := flag.NewFlagSet("release", flag.ContinueOnError)
	mode := flags.String("mode", "check", "check, release, or workflow verification")
	tag := flags.String("tag", defaultTag, "explicit semantic version, such as v0.2.0")
	sha := flags.String("sha", "", "expected commit for workflow verification")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := validateTag(*tag); err != nil {
		return err
	}
	switch *mode {
	case "check", "release":
		if *sha != "" {
			return errors.New("-sha is only used by workflow verification")
		}
		p, err := t.preflight(ctx, *tag)
		if err != nil {
			return err
		}
		if err := t.writef("Validated %s at %s in %s.\n", *tag, p.sha, repository); err != nil {
			return err
		}
		if *mode == "check" {
			return t.writef("No tag, push, or workflow dispatch performed. Release also runs make verify and make vuln before preparing the tag.\n")
		}
		return t.prepare(ctx, *tag, p)
	case "verify":
		if !shaPattern.MatchString(*sha) {
			return errors.New("workflow verification needs a full 40-character lowercase commit SHA")
		}
		if err := t.verifyRemote(ctx, *tag, *sha); err != nil {
			return err
		}
		return t.writef("Verified remote tag %s at %s and its exact-revision checks.\n", *tag, *sha)
	default:
		return fmt.Errorf("unknown mode %q", *mode)
	}
}

func validateTag(tag string) error {
	if !versionPattern.MatchString(tag) {
		return errors.New("set TAG to an explicit canonical version such as v0.2.0 or v0.2.0-rc.1 (no build metadata)")
	}
	_, prerelease, found := strings.Cut(tag, "-")
	if found {
		for part := range strings.SplitSeq(prerelease, ".") {
			numeric := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) == -1
			if numeric && len(part) > 1 && part[0] == '0' {
				return errors.New("numeric prerelease identifiers must not have leading zeros")
			}
		}
	}
	return nil
}

func (t *tool) writef(format string, args ...any) error {
	text := fmt.Sprintf(format, args...)
	n, err := io.WriteString(t.out, text)
	if err == nil && n != len(text) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("write release output: %w", err)
	}
	return nil
}

func runCommand(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return output, fmt.Errorf("%s: %w", name, ctx.Err())
	}
	if err != nil {
		return output, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func (t *tool) command(ctx context.Context, name string, args ...string) (string, error) {
	out, err := t.run(ctx, commandTimeout, name, args...)
	return strings.TrimSpace(string(out)), err
}

// --include gives a reliable HTTP status even when gh exits nonzero. Only a
// verified 404 means absence; authentication and transport errors remain fatal.
func (t *tool) api(ctx context.Context, endpoint string, target any) (bool, error) {
	out, commandErr := t.run(ctx, commandTimeout, "gh", "api", "--hostname", "github.com", "--include", "--method", "GET", "repos/"+repository+endpoint)
	header, body, ok := strings.Cut(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n\n")
	statusLine, _, _ := strings.Cut(header, "\n")
	fields := strings.Fields(statusLine)
	if !ok || len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		if commandErr != nil {
			return false, commandErr
		}
		return false, fmt.Errorf("GitHub returned an invalid HTTP response for %s", endpoint)
	}
	status, err := strconv.Atoi(fields[1])
	if err != nil {
		return false, fmt.Errorf("GitHub returned an invalid HTTP status for %s", endpoint)
	}
	if status == http.StatusNotFound {
		return false, nil
	}
	if commandErr != nil {
		return false, commandErr
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("GitHub returned HTTP %d for %s", status, endpoint)
	}
	if err := json.Unmarshal([]byte(body), target); err != nil {
		return false, fmt.Errorf("read GitHub response for %s: %w", endpoint, err)
	}
	return true, nil
}

func (t *tool) requireAPI(ctx context.Context, endpoint string, target any) error {
	found, err := t.api(ctx, endpoint, target)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("GitHub resource %s is absent or inaccessible", endpoint)
	}
	return nil
}

func (t *tool) preflight(ctx context.Context, tag string) (plan, error) {
	var p plan
	status, err := t.command(ctx, "git", "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return p, err
	}
	if status != "" {
		return p, errors.New("checkout must be clean, including untracked files")
	}
	p.sha, err = t.command(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return p, err
	}
	if !shaPattern.MatchString(p.sha) {
		return p, errors.New("HEAD is not a full commit SHA")
	}
	var repo struct {
		FullName    string `json:"full_name"`
		Permissions struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if err := t.requireAPI(ctx, "", &repo); err != nil {
		return p, err
	}
	if repo.FullName != repository || !repo.Permissions.Push {
		return p, fmt.Errorf("the active GitHub account needs write access to %s", repository)
	}
	if err := t.checkEnvironment(ctx); err != nil {
		return p, err
	}
	var branch struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := t.requireAPI(ctx, "/branches/main", &branch); err != nil {
		return p, err
	}
	if p.sha != branch.Commit.SHA {
		return p, fmt.Errorf("HEAD %s must equal current upstream main %s", p.sha, branch.Commit.SHA)
	}
	if err := t.checkChangelog(ctx, tag, p.sha); err != nil {
		return p, err
	}
	if err := t.checkReleaseAbsent(ctx, tag); err != nil {
		return p, err
	}
	p.local, err = t.localTag(ctx, tag)
	if err != nil {
		return p, err
	}
	p.remote, err = t.remoteTag(ctx, tag)
	if err != nil {
		return p, err
	}
	for _, existing := range []tagState{p.local, p.remote} {
		if existing.object != "" && existing.commit != p.sha {
			return p, fmt.Errorf("existing tag %s is not an annotated tag for %s; tags are never moved", tag, p.sha)
		}
	}
	if p.local.object != "" && p.remote.object != "" && p.local.object != p.remote.object {
		return p, fmt.Errorf("local and remote tag %s are different objects; resolve the collision without moving the remote tag", tag)
	}
	if err := t.checkRuns(ctx, p.sha); err != nil {
		return p, err
	}
	return p, nil
}

func (t *tool) checkChangelog(ctx context.Context, tag, sha string) error {
	text, err := t.command(ctx, "git", "show", sha+":CHANGELOG.md")
	if err != nil {
		return err
	}
	prefix := "## " + tag + " - "
	for line := range strings.SplitSeq(text, "\n") {
		if date, ok := strings.CutPrefix(line, prefix); ok {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return fmt.Errorf("CHANGELOG.md needs a dated %s heading; Unreleased is not releasable", tag)
			}
			return nil
		}
	}
	return fmt.Errorf("CHANGELOG.md has no dated release heading for %s", tag)
}

func (t *tool) checkReleaseAbsent(ctx context.Context, tag string) error {
	var release struct {
		Draft bool   `json:"draft"`
		URL   string `json:"html_url"`
	}
	found, err := t.api(ctx, "/releases/tags/"+tag, &release)
	if err != nil {
		return err
	}
	if found {
		kind := "published"
		if release.Draft {
			kind = "draft"
		}
		return fmt.Errorf("a %s release already exists for %s (%s); inspect it manually, do not replace its assets", kind, tag, release.URL)
	}
	return nil
}

func (t *tool) checkEnvironment(ctx context.Context) error {
	var environment struct {
		Rules []struct {
			Type      string `json:"type"`
			Reviewers []struct {
				Type     string `json:"type"`
				Reviewer struct {
					Login string `json:"login"`
				} `json:"reviewer"`
			} `json:"reviewers"`
		} `json:"protection_rules"`
	}
	if err := t.requireAPI(ctx, "/environments/release", &environment); err != nil {
		return err
	}
	for _, rule := range environment.Rules {
		if rule.Type != "required_reviewers" {
			continue
		}
		// GitHub accepts any listed reviewer. Requiring the owner as the sole
		// reviewer prevents another account from replacing owner approval.
		if len(rule.Reviewers) == 1 && rule.Reviewers[0].Type == "User" && rule.Reviewers[0].Reviewer.Login == "lennrt" {
			return nil
		}
	}
	return errors.New("the release environment must require owner lennrt as its sole required reviewer")
}

func (t *tool) localTag(ctx context.Context, tag string) (tagState, error) {
	out, err := t.command(ctx, "git", "for-each-ref", "--format=%(objecttype) %(objectname) %(*objecttype) %(*objectname)", "refs/tags/"+tag)
	if err != nil {
		return tagState{}, err
	}
	if out == "" {
		return tagState{}, nil
	}
	parts := strings.Fields(out)
	if len(parts) != 4 || parts[0] != "tag" || parts[2] != "commit" {
		return tagState{}, fmt.Errorf("local tag %s must be annotated and point directly to a commit", tag)
	}
	return tagState{object: parts[1], commit: parts[3]}, nil
}

func (t *tool) remoteTag(ctx context.Context, tag string) (tagState, error) {
	out, err := t.command(ctx, "git", "ls-remote", "--tags", remoteURL, "refs/tags/"+tag, "refs/tags/"+tag+"^{}")
	if err != nil {
		return tagState{}, err
	}
	var state tagState
	for line := range strings.SplitSeq(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		if len(parts) != 2 || !shaPattern.MatchString(parts[0]) {
			return state, errors.New("invalid remote tag response")
		}
		switch parts[1] {
		case "refs/tags/" + tag:
			state.object = parts[0]
		case "refs/tags/" + tag + "^{}":
			state.commit = parts[0]
		default:
			return state, errors.New("unexpected remote tag reference")
		}
	}
	if state.object != "" && state.commit == "" {
		return state, fmt.Errorf("remote tag %s must be annotated", tag)
	}
	if state.object == "" && state.commit != "" {
		return state, errors.New("remote tag has no tag object")
	}
	return state, nil
}

type workflowRun struct {
	ID         int64  `json:"id"`
	Attempt    int    `json:"run_attempt"`
	SHA        string `json:"head_sha"`
	Branch     string `json:"head_branch"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"html_url"`
}

func (t *tool) checkRuns(ctx context.Context, sha string) error {
	for _, workflow := range []string{"ci.yml", "security.yml"} {
		var response struct {
			Runs []workflowRun `json:"workflow_runs"`
		}
		endpoint := "/actions/workflows/" + workflow + "/runs?branch=main&event=push&head_sha=" + sha + "&per_page=1"
		if err := t.requireAPI(ctx, endpoint, &response); err != nil {
			return err
		}
		if len(response.Runs) != 1 {
			return fmt.Errorf("no exact-revision main-push run for %s at %s", workflow, sha)
		}
		run := response.Runs[0]
		if run.SHA != sha || run.Branch != "main" || run.Event != "push" || run.Status != "completed" || run.Conclusion != "success" || run.ID <= 0 || run.Attempt <= 0 {
			return fmt.Errorf("latest %s run for %s must have completed successfully: %s (%s/%s)", workflow, sha, run.URL, run.Status, run.Conclusion)
		}
		if workflow == "ci.yml" {
			var jobs struct {
				Total int `json:"total_count"`
				Jobs  []struct {
					Name       string `json:"name"`
					Status     string `json:"status"`
					Conclusion string `json:"conclusion"`
				} `json:"jobs"`
			}
			endpoint = fmt.Sprintf("/actions/runs/%d/attempts/%d/jobs?per_page=100", run.ID, run.Attempt)
			if err := t.requireAPI(ctx, endpoint, &jobs); err != nil {
				return err
			}
			if jobs.Total > len(jobs.Jobs) {
				return errors.New("CI job response is incomplete; cannot establish Kafka success")
			}
			passed := false
			for _, job := range jobs.Jobs {
				if job.Name == "Kafka integration" && job.Status == "completed" && job.Conclusion == "success" {
					passed = true
				}
			}
			if !passed {
				return errors.New("the exact CI run must include a successful Kafka integration job")
			}
		}
	}
	return nil
}

func (t *tool) verifyRemote(ctx context.Context, tag, sha string) error {
	if err := t.checkEnvironment(ctx); err != nil {
		return err
	}
	state, err := t.remoteTag(ctx, tag)
	if err != nil {
		return err
	}
	if state.object == "" || state.commit != sha {
		return fmt.Errorf("remote tag %s must still identify expected commit %s", tag, sha)
	}
	var comparison struct {
		Status string `json:"status"`
	}
	if err := t.requireAPI(ctx, "/compare/"+sha+"...main", &comparison); err != nil {
		return err
	}
	if comparison.Status != "ahead" && comparison.Status != "identical" {
		return fmt.Errorf("release commit %s is not in upstream main history", sha)
	}
	if err := t.checkChangelog(ctx, tag, sha); err != nil {
		return err
	}
	if err := t.checkReleaseAbsent(ctx, tag); err != nil {
		return err
	}
	return t.checkRuns(ctx, sha)
}

func (t *tool) prepare(ctx context.Context, tag string, p plan) error {
	for _, gate := range []struct {
		name    string
		timeout time.Duration
	}{{"verify", 40 * time.Minute}, {"vuln", 10 * time.Minute}} {
		if err := t.writef("Running make %s before creating or pushing a tag.\n", gate.name); err != nil {
			return err
		}
		output, err := t.run(ctx, gate.timeout, "make", gate.name)
		if len(output) != 0 {
			if writeErr := t.writef("%s", output); writeErr != nil {
				return errors.Join(err, writeErr)
			}
		}
		if err != nil {
			return err
		}
	}
	current, err := t.preflight(ctx, tag)
	if err != nil {
		return err
	}
	if current.sha != p.sha {
		return errors.New("upstream main changed during local verification; restart release preparation")
	}
	p = current
	if p.remote.object == "" {
		if p.local.object == "" {
			if _, err := t.command(ctx, "git", "tag", "-a", tag, p.sha, "-m", tag); err != nil {
				return err
			}
			if err := t.writef("Created local annotated tag %s at %s.\n", tag, p.sha); err != nil {
				return fmt.Errorf("local tag %s created; no upstream push or dispatch performed: %w", tag, err)
			}
			p.local, err = t.localTag(ctx, tag)
			if err != nil {
				return err
			}
			if p.local.object == "" || p.local.commit != p.sha {
				return errors.New("new local tag changed before push; no upstream tag was created")
			}
		}
		if _, err := t.command(ctx, "git", "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "push", "--porcelain", remoteURL, p.local.object+":refs/tags/"+tag); err != nil {
			return fmt.Errorf("tag retained locally; push may have reached upstream. Run release-check and inspect the remote before retrying: %w", err)
		}
		if err := t.writef("Pushed annotated tag %s to %s.\n", tag, repository); err != nil {
			return fmt.Errorf("upstream tag %s pushed; workflow not dispatched: %w", tag, err)
		}
	} else {
		if err := t.writef("Using existing upstream annotated tag %s at %s.\n", tag, p.sha); err != nil {
			return fmt.Errorf("upstream tag %s retained; workflow not dispatched: %w", tag, err)
		}
	}
	if err := t.verifyRemote(ctx, tag, p.sha); err != nil {
		return fmt.Errorf("tag retained; workflow not dispatched: %w", err)
	}
	output, err := t.command(ctx, "gh", "workflow", "run", "release.yml", "--repo", "github.com/"+repository, "--ref", "main", "--raw-field", "tag="+tag, "--raw-field", "expected_sha="+p.sha)
	if err != nil {
		return fmt.Errorf("tag retained; dispatch outcome may be uncertain. Inspect Release runs for %s at %s before retrying: %w", tag, p.sha, err)
	}
	if output != "" {
		if err := t.writef("%s\n", output); err != nil {
			return fmt.Errorf("workflow dispatch completed for %s at %s; inspect Release runs before retrying: %w", tag, p.sha, err)
		}
	}
	if err := t.writef("Release workflow dispatched; publication is pending its checks and owner environment approval. No release has been confirmed published.\n"); err != nil {
		return fmt.Errorf("workflow dispatch completed for %s at %s; inspect Release runs before retrying: %w", tag, p.sha, err)
	}
	return nil
}
