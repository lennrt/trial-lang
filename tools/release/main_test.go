package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

const testSHA = "1111111111111111111111111111111111111111"
const otherSHA = "2222222222222222222222222222222222222222"
const tagObject = "3333333333333333333333333333333333333333"
const testTag = "v0.2.0"

type fixture struct {
	t                                                                    *testing.T
	commands                                                             []string
	dirty, head, main, changelog                                         string
	write                                                                bool
	local, remote                                                        tagState
	localRaw, remoteRaw                                                  string
	release, draft                                                       bool
	ci, security                                                         workflowRun
	kafkaStatus, kafkaConclusion                                         string
	jobCount                                                             int
	compare                                                              string
	environmentMissing, environmentUnprotected, environmentOtherReviewer bool
	before                                                               func(string) error
	failPush, failDispatch                                               bool
	output                                                               bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	run := workflowRun{ID: 1, Attempt: 1, SHA: testSHA, Branch: "main", Event: "push", Status: "completed", Conclusion: "success", URL: "https://github.com/lennrt/trial-lang/actions/runs/1"}
	return &fixture{t: t, head: testSHA, main: testSHA, changelog: "# Changelog\n\n## v0.2.0 - 2026-10-18\n\n- Improve releases.\n", write: true, ci: run, security: run, kafkaStatus: "completed", kafkaConclusion: "success", jobCount: 1, compare: "identical"}
}

func response(status int, value any) ([]byte, error) {
	body, _ := json.Marshal(value)
	out := []byte(fmt.Sprintf("HTTP/2.0 %d Status\r\nContent-Type: application/json\r\n\r\n%s", status, body))
	if status >= 400 {
		return out, errors.New("gh failed")
	}
	return out, nil
}

func (f *fixture) run(_ context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	f.t.Helper()
	if timeout <= 0 {
		f.t.Fatal("command has no finite timeout")
	}
	cmd := name + " " + strings.Join(args, " ")
	f.commands = append(f.commands, cmd)
	if f.before != nil {
		if err := f.before(cmd); err != nil {
			return nil, err
		}
	}
	text := func(s string) ([]byte, error) { return []byte(s), nil }
	switch {
	case cmd == "git status --porcelain --untracked-files=normal":
		return text(f.dirty)
	case cmd == "git rev-parse HEAD":
		return text(f.head)
	case cmd == "git show "+testSHA+":CHANGELOG.md":
		return text(f.changelog)
	case strings.HasPrefix(cmd, "git for-each-ref "):
		if f.localRaw != "" {
			return text(f.localRaw)
		}
		if f.local.object == "" {
			return text("")
		}
		return text("tag " + f.local.object + " commit " + f.local.commit)
	case cmd == "git ls-remote --tags "+remoteURL+" refs/tags/"+testTag+" refs/tags/"+testTag+"^{}":
		if f.remoteRaw != "" {
			return text(f.remoteRaw)
		}
		if f.remote.object == "" {
			return text("")
		}
		return text(f.remote.object + "\trefs/tags/" + testTag + "\n" + f.remote.commit + "\trefs/tags/" + testTag + "^{}\n")
	case cmd == "git tag -a "+testTag+" "+testSHA+" -m "+testTag:
		f.local = tagState{object: tagObject, commit: testSHA}
		return text("")
	case cmd == "git -c credential.helper= -c credential.helper=!gh auth git-credential push --porcelain "+remoteURL+" "+tagObject+":refs/tags/"+testTag:
		f.remote = tagState{object: tagObject, commit: testSHA}
		if f.failPush {
			return nil, errors.New("connection lost after push")
		}
		return text("")
	case cmd == "make verify" || cmd == "make vuln":
		return text("gate passed\n")
	case cmd == "gh workflow run release.yml --repo github.com/"+repository+" --ref main --raw-field tag="+testTag+" --raw-field expected_sha="+testSHA:
		if f.failDispatch {
			return nil, errors.New("connection lost after dispatch")
		}
		return text("https://github.com/lennrt/trial-lang/actions/runs/2")
	case strings.HasPrefix(cmd, "gh api --hostname github.com --include --method GET repos/"+repository):
		endpoint := strings.TrimPrefix(args[len(args)-1], "repos/"+repository)
		switch {
		case endpoint == "":
			return response(200, map[string]any{"full_name": repository, "permissions": map[string]bool{"push": f.write}})
		case endpoint == "/branches/main":
			return response(200, map[string]any{"commit": map[string]string{"sha": f.main}})
		case endpoint == "/environments/release":
			if f.environmentMissing {
				return response(404, map[string]string{"message": "Not Found"})
			}
			rules := []map[string]any{}
			if !f.environmentUnprotected {
				reviewers := []map[string]any{{"type": "User", "reviewer": map[string]string{"login": "lennrt"}}}
				if f.environmentOtherReviewer {
					reviewers = append(reviewers, map[string]any{"type": "User", "reviewer": map[string]string{"login": "someone-else"}})
				}
				rules = append(rules, map[string]any{"type": "required_reviewers", "reviewers": reviewers})
			}
			return response(200, map[string]any{"protection_rules": rules})
		case endpoint == "/releases/tags/"+testTag:
			if !f.release {
				return response(404, map[string]string{"message": "Not Found"})
			}
			return response(200, map[string]any{"draft": f.draft, "html_url": "https://github.com/lennrt/trial-lang/releases/tag/" + testTag})
		case endpoint == "/compare/"+testSHA+"...main":
			return response(200, map[string]string{"status": f.compare})
		case strings.HasPrefix(endpoint, "/actions/workflows/"):
			if strings.Contains(endpoint, "status=success") {
				f.t.Fatal("successful runs filter can hide a newer failure")
			}
			if !strings.HasSuffix(endpoint, "?branch=main&event=push&head_sha="+testSHA+"&per_page=1") {
				f.t.Fatalf("query not pinned: %s", endpoint)
			}
			run := f.ci
			if strings.Contains(endpoint, "security.yml") {
				run = f.security
			}
			if run.ID == 0 {
				return response(200, map[string]any{"workflow_runs": []workflowRun{}})
			}
			return response(200, map[string]any{"workflow_runs": []workflowRun{run}})
		case endpoint == "/actions/runs/1/attempts/1/jobs?per_page=100":
			return response(200, map[string]any{"total_count": f.jobCount, "jobs": []map[string]string{{"name": "Kafka integration", "status": f.kafkaStatus, "conclusion": f.kafkaConclusion}}})
		}
	}
	f.t.Fatalf("unexpected command: %s", cmd)
	return nil, errors.New("unexpected command")
}

func (f *fixture) execute(mode string) error {
	return execute(f.t.Context(), []string{"-mode", mode}, testTag, &tool{run: f.run, out: &f.output})
}

func (f *fixture) mutations() []string {
	var result []string
	for _, cmd := range f.commands {
		if strings.HasPrefix(cmd, "git tag ") || strings.Contains(cmd, " push --porcelain ") || strings.HasPrefix(cmd, "gh workflow ") {
			result = append(result, cmd)
		}
	}
	return result
}

func TestCanonicalReleaseTags(t *testing.T) {
	for _, tag := range []string{"v0.0.0", "v1.2.3", "v1.2.3-rc.1", "v1.2.3-0", "v1.2.3-x-1.2"} {
		if err := validateTag(tag); err != nil {
			t.Errorf("valid %q: %v", tag, err)
		}
	}
	for _, tag := range []string{"", "1.2.3", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-01", "v1.2.3-rc.01", "v1.2.3-", "v1.2.3-rc..1", "v1.2.3+meta", "v1.2.3\n", "v1.2.3;touch nope", "$(echo bad)"} {
		if err := validateTag(tag); err == nil {
			t.Errorf("invalid tag accepted: %q", tag)
		}
	}
}

func TestReleaseCheckHasNoMutations(t *testing.T) {
	f := newFixture(t)
	if err := f.execute("check"); err != nil {
		t.Fatal(err)
	}
	if len(f.mutations()) != 0 {
		t.Fatalf("read-only check mutated: %v", f.mutations())
	}
	for _, cmd := range f.commands {
		if strings.HasPrefix(cmd, "make ") {
			t.Fatalf("preflight unexpectedly ran local gates: %s", cmd)
		}
	}
	if !strings.Contains(f.output.String(), testSHA) || !strings.Contains(f.output.String(), "No tag, push, or workflow dispatch") {
		t.Fatalf("unclear outcome: %s", &f.output)
	}
}

func TestReleasePreconditionsBlockEveryMutation(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*fixture)
	}{
		{"dirty", "clean", func(f *fixture) { f.dirty = "?? private.txt" }},
		{"wrong main", "current upstream main", func(f *fixture) { f.main = otherSHA }},
		{"no write", "write access", func(f *fixture) { f.write = false }},
		{"missing environment", "absent or inaccessible", func(f *fixture) { f.environmentMissing = true }},
		{"unprotected environment", "sole required reviewer", func(f *fixture) { f.environmentUnprotected = true }},
		{"nonowner can approve", "sole required reviewer", func(f *fixture) { f.environmentOtherReviewer = true }},
		{"undated", "dated", func(f *fixture) { f.changelog = "## v0.2.0 - Unreleased" }},
		{"wrong date", "dated", func(f *fixture) { f.changelog = "## v0.2.0 - 2026-02-30" }},
		{"missing heading", "dated", func(f *fixture) { f.changelog = "## v0.1.0 - 2026-01-01" }},
		{"published", "published release", func(f *fixture) { f.release = true }},
		{"draft", "draft release", func(f *fixture) { f.release = true; f.draft = true }},
		{"local lightweight", "annotated", func(f *fixture) { f.localRaw = "commit " + testSHA }},
		{"remote lightweight", "annotated", func(f *fixture) { f.remoteRaw = testSHA + "\trefs/tags/" + testTag }},
		{"local wrong SHA", "never moved", func(f *fixture) { f.local = tagState{object: tagObject, commit: otherSHA} }},
		{"remote wrong SHA", "never moved", func(f *fixture) { f.remote = tagState{object: tagObject, commit: otherSHA} }},
		{"different annotations", "different objects", func(f *fixture) {
			f.local = tagState{object: tagObject, commit: testSHA}
			f.remote = tagState{object: otherSHA, commit: testSHA}
		}},
		{"missing CI", "no exact-revision", func(f *fixture) { f.ci.ID = 0 }},
		{"wrong CI SHA", "latest ci.yml", func(f *fixture) { f.ci.SHA = otherSHA }},
		{"PR instead of main", "latest ci.yml", func(f *fixture) { f.ci.Event = "pull_request" }},
		{"wrong branch", "latest ci.yml", func(f *fixture) { f.ci.Branch = "feature" }},
		{"running CI", "latest ci.yml", func(f *fixture) { f.ci.Status = "in_progress" }},
		{"new failed CI", "latest ci.yml", func(f *fixture) { f.ci.Conclusion = "failure" }},
		{"failed Security", "latest security.yml", func(f *fixture) { f.security.Conclusion = "failure" }},
		{"skipped Kafka", "successful Kafka", func(f *fixture) { f.kafkaConclusion = "skipped" }},
		{"failed Kafka", "successful Kafka", func(f *fixture) { f.kafkaConclusion = "failure" }},
		{"missing jobs page", "incomplete", func(f *fixture) { f.jobCount = 101 }},
		{"network error", "network unavailable", func(f *fixture) {
			f.before = func(cmd string) error {
				if strings.HasPrefix(cmd, "gh api") {
					return errors.New("network unavailable")
				}
				return nil
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.change(f)
			err := f.execute("release")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
			if len(f.mutations()) != 0 {
				t.Fatalf("failed preflight mutated: %v", f.mutations())
			}
		})
	}
}

func TestReleasePreparesAndPinsTheWorkflow(t *testing.T) {
	f := newFixture(t)
	if err := f.execute("release"); err != nil {
		t.Fatal(err)
	}
	mutations := f.mutations()
	if len(mutations) != 3 {
		t.Fatalf("mutations=%v", mutations)
	}
	firstMutation := -1
	gates := 0
	preflights := 0
	for i, cmd := range f.commands {
		if cmd == "make verify" || cmd == "make vuln" {
			gates++
		}
		if strings.HasPrefix(cmd, "git status") {
			preflights++
		}
		if strings.HasPrefix(cmd, "git tag ") {
			firstMutation = i
			break
		}
	}
	if gates != 2 || preflights != 2 || firstMutation < 0 {
		t.Fatalf("mutations preceded final verification: %v", f.commands)
	}
	if !strings.Contains(mutations[2], "--ref main") || !strings.Contains(mutations[2], "expected_sha="+testSHA) {
		t.Fatalf("dispatch not pinned: %s", mutations[2])
	}
	if !strings.Contains(f.output.String(), "No release has been confirmed published") {
		t.Fatalf("dispatch claimed publication: %s", &f.output)
	}
}

func TestReleaseResumesImmutableTags(t *testing.T) {
	for _, mode := range []string{"local only", "remote only", "both"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			state := tagState{object: tagObject, commit: testSHA}
			if mode != "remote only" {
				f.local = state
			}
			if mode != "local only" {
				f.remote = state
			}
			if err := f.execute("release"); err != nil {
				t.Fatal(err)
			}
			for _, cmd := range f.mutations() {
				if strings.HasPrefix(cmd, "git tag ") {
					t.Fatalf("existing tag recreated: %s", cmd)
				}
				if mode != "local only" && strings.Contains(cmd, " push --porcelain ") {
					t.Fatalf("remote tag pushed again: %s", cmd)
				}
			}
		})
	}
}

func TestReleasePushUsesPinnedObjectAndActiveGHCredentials(t *testing.T) {
	f := newFixture(t)
	f.local = tagState{object: tagObject, commit: testSHA}
	f.before = func(cmd string) error {
		if strings.Contains(cmd, " push --porcelain ") {
			// Simulate an unrelated local process retargeting the tag after
			// preflight. The push must still name the validated object SHA.
			f.local = tagState{object: otherSHA, commit: otherSHA}
			if !strings.Contains(cmd, tagObject+":refs/tags/"+testTag) || !strings.Contains(cmd, "credential.helper=!gh auth git-credential") {
				t.Fatalf("push uses mutable source or wrong identity: %s", cmd)
			}
		}
		return nil
	}
	if err := f.execute("release"); err != nil {
		t.Fatal(err)
	}
	if f.remote.commit != testSHA || f.remote.object != tagObject {
		t.Fatalf("wrong upstream tag: %+v", f.remote)
	}
}

func TestReleaseFailuresPreserveRecoveryState(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*fixture)
		mutations  int
	}{
		{"verify failed", "gate failed", func(f *fixture) {
			f.before = func(cmd string) error {
				if cmd == "make verify" {
					return errors.New("gate failed")
				}
				return nil
			}
		}, 0},
		{"vuln failed", "scan failed", func(f *fixture) {
			f.before = func(cmd string) error {
				if cmd == "make vuln" {
					return errors.New("scan failed")
				}
				return nil
			}
		}, 0},
		{"main advanced", "current upstream main", func(f *fixture) {
			f.before = func(cmd string) error {
				if cmd == "make vuln" {
					f.main = otherSHA
				}
				return nil
			}
		}, 0},
		{"checkout changed", "clean", func(f *fixture) {
			f.before = func(cmd string) error {
				if cmd == "make vuln" {
					f.dirty = " M source.go"
				}
				return nil
			}
		}, 0},
		{"HEAD changed", "current upstream main", func(f *fixture) {
			f.before = func(cmd string) error {
				if cmd == "make vuln" {
					f.head = otherSHA
				}
				return nil
			}
		}, 0},
		{"remote collision during gates", "never moved", func(f *fixture) {
			f.before = func(cmd string) error {
				if cmd == "make vuln" {
					f.remote = tagState{object: tagObject, commit: otherSHA}
				}
				return nil
			}
		}, 0},
		{"push uncertain", "push may have reached", func(f *fixture) { f.failPush = true }, 2},
		{"dispatch uncertain", "dispatch outcome may be uncertain", func(f *fixture) { f.failDispatch = true }, 3},
		{"tag moved after push", "expected commit", func(f *fixture) {
			f.before = func(cmd string) error {
				if strings.HasPrefix(cmd, "git ls-remote") && f.remote.object != "" {
					f.remote.commit = otherSHA
				}
				return nil
			}
		}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.change(f)
			err := f.execute("release")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v", err)
			}
			if got := len(f.mutations()); got != tc.mutations {
				t.Fatalf("mutations=%v, want %d", f.mutations(), tc.mutations)
			}
			for _, cmd := range f.commands {
				if strings.Contains(cmd, "--delete") || strings.Contains(cmd, "--force") {
					t.Fatalf("destructive recovery: %s", cmd)
				}
			}
		})
	}
}

func TestWorkflowGuardRejectsWrongRevisionAndChecks(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*fixture)
	}{
		{"absent tag", "must still identify", func(*fixture) {}},
		{"tag moved", "must still identify", func(f *fixture) { f.remote = tagState{object: tagObject, commit: otherSHA} }},
		{"outside main", "not in upstream main", func(f *fixture) { f.remote = tagState{object: tagObject, commit: testSHA}; f.compare = "diverged" }},
		{"checks changed", "latest security.yml", func(f *fixture) {
			f.remote = tagState{object: tagObject, commit: testSHA}
			f.security.Conclusion = "failure"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.change(f)
			err := execute(t.Context(), []string{"-mode", "verify", "-tag", testTag, "-sha", testSHA}, "", &tool{run: f.run, out: &f.output})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v", err)
			}
			if len(f.mutations()) != 0 {
				t.Fatal("workflow guard mutated")
			}
		})
	}
	f := newFixture(t)
	f.remote = tagState{object: tagObject, commit: testSHA}
	f.compare = "ahead"
	f.write = false
	f.dirty = " M build-output"
	if err := execute(t.Context(), []string{"-mode", "verify", "-tag", testTag, "-sha", testSHA}, "", &tool{run: f.run, out: &f.output}); err != nil {
		t.Fatalf("read-only guard incorrectly required clean checkout or user permissions: %v", err)
	}
}

func TestAPIOnlyTreatsVerified404AsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		commandError error
		absent       bool
	}{
		{"404", "HTTP/2.0 404 Not Found\n\n{}", errors.New("gh status"), true},
		{"403", "HTTP/2.0 403 Forbidden\n\n{}", errors.New("forbidden"), false},
		{"401", "HTTP/2.0 401 Unauthorized\n\n{}", errors.New("unauthorized"), false},
		{"500", "HTTP/2.0 500 Server Error\n\n{}", errors.New("server failed"), false},
		{"network", "", errors.New("offline"), false},
		{"malformed status", "HTTP/2.0 bad Status\n\n{}", nil, false},
		{"invalid body", "HTTP/2.0 200 OK\n\nnot JSON", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := &tool{run: func(context.Context, time.Duration, string, ...string) ([]byte, error) {
				return []byte(tc.body), tc.commandError
			}}
			var result any
			found, err := tool.api(t.Context(), "/example", &result)
			if tc.absent {
				if found || err != nil {
					t.Fatalf("absence=%t/%v", found, err)
				}
			} else if err == nil {
				t.Fatal("API failure became absence")
			}
		})
	}
}

func TestReleaseEntryPointsRejectUnsafeArguments(t *testing.T) {
	for _, args := range [][]string{{"-mode", "nope"}, {"-mode", "verify", "-sha", "main"}, {"-mode", "check", "-sha", testSHA}, {"extra"}} {
		f := newFixture(t)
		if err := execute(t.Context(), args, testTag, &tool{run: f.run, out: &f.output}); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if len(f.commands) != 0 {
			t.Fatalf("invalid args invoked external command: %v", f.commands)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := runCommand(ctx, time.Second, os.Args[0], "-test.run=^$"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled command=%v", err)
	}
}

func TestWorkflowRetainsProtectedExactRevisionBoundary(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{"environment: release", "expected_sha:", "ref: ${{ github.workflow_sha }}", "ref: ${{ inputs.expected_sha }}", `test "$GITHUB_REF" = refs/heads/main`, `test "$(git rev-parse HEAD)" = "$EXPECTED_SHA"`, "run: make verify", "run: make vuln", "--draft"} {
		if !strings.Contains(workflow, required) {
			t.Errorf("workflow lost required boundary %q", required)
		}
	}
	if count := strings.Count(workflow, `"$RUNNER_TEMP/trial-release" -mode verify -tag "$RELEASE_TAG" -sha "$EXPECTED_SHA"`); count != 2 {
		t.Fatalf("need checks before build and publication, got %d", count)
	}
	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(makefile), "release release-check: export TRIAL_RELEASE_TAG = $(TAG)") {
		t.Fatal("Make targets must pass TAG as environment data")
	}
}

type failingOutput struct {
	match     string
	short     bool
	onFailure func()
}

func (w failingOutput) Write(data []byte) (int, error) {
	if strings.Contains(string(data), w.match) {
		if w.onFailure != nil {
			w.onFailure()
		}
		if w.short {
			return len(data) - 1, nil
		}
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}

func TestReleaseOutputFailuresStopLaterActions(t *testing.T) {
	for _, tc := range []struct {
		name, match, want string
		mutations         int
		existing, short   bool
	}{
		{"initial status", "Validated", "write release output", 0, false, false},
		{"gate announcement", "Running make verify", "write release output", 0, false, false},
		{"gate log", "gate passed", "write release output", 0, false, false},
		{"local tag result", "Created local annotated tag", "local tag v0.2.0 created; no upstream push or dispatch", 1, false, false},
		{"push result", "Pushed annotated tag", "upstream tag v0.2.0 pushed; workflow not dispatched", 2, false, false},
		{"existing tag result", "Using existing upstream", "upstream tag v0.2.0 retained; workflow not dispatched", 0, true, false},
		{"dispatch URL", "https://github.com/lennrt/trial-lang/actions/runs/2", "workflow dispatch completed", 3, false, false},
		{"dispatch summary", "Release workflow dispatched;", "workflow dispatch completed", 3, false, false},
		{"short write before tag", "Running make verify", "write release output", 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if tc.existing {
				f.remote = tagState{object: tagObject, commit: testSHA}
			}
			commandsAtFailure := -1
			writer := failingOutput{match: tc.match, short: tc.short, onFailure: func() { commandsAtFailure = len(f.commands) }}
			err := execute(t.Context(), []string{"-mode", "release"}, testTag, &tool{run: f.run, out: writer})
			wantError := io.ErrClosedPipe
			if tc.short {
				wantError = io.ErrShortWrite
			}
			if !errors.Is(err, wantError) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q wrapping %v", err, tc.want, wantError)
			}
			if len(f.mutations()) != tc.mutations {
				t.Fatalf("mutations after failure=%v", f.mutations())
			}
			if commandsAtFailure < 0 || len(f.commands) != commandsAtFailure {
				t.Fatalf("commands continued after output failure: failed at %d, final %v", commandsAtFailure, f.commands)
			}
		})
	}
}

func TestReadOnlyModesReportOutputFailure(t *testing.T) {
	for _, mode := range []string{"check", "verify"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			args := []string{"-mode", mode}
			match := "No tag, push, or workflow dispatch"
			if mode == "verify" {
				args = append(args, "-sha", testSHA)
				f.remote = tagState{object: tagObject, commit: testSHA}
				match = "Verified remote tag"
			}
			err := execute(t.Context(), args, testTag, &tool{run: f.run, out: failingOutput{match: match}})
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("error=%v", err)
			}
			if len(f.mutations()) != 0 {
				t.Fatalf("read-only output failure mutated: %v", f.mutations())
			}
		})
	}
}

func TestGateAndOutputFailuresBothRemainVisible(t *testing.T) {
	f := newFixture(t)
	gateError := errors.New("verification failed")
	runner := func(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
		if name == "make" {
			return []byte("gate failure details\n"), gateError
		}
		return f.run(ctx, timeout, name, args...)
	}
	err := execute(t.Context(), []string{"-mode", "release"}, testTag, &tool{run: runner, out: failingOutput{match: "gate failure details"}})
	if !errors.Is(err, gateError) || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("lost command or output failure: %v", err)
	}
	if len(f.mutations()) != 0 {
		t.Fatalf("failed gate mutated: %v", f.mutations())
	}
}
