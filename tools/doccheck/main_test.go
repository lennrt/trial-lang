package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMarkdownTargets(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"asset.png", "guide.md", "file name.md", "nested(a).md", "amp&name.md"} {
		writeFixture(t, root, path, "")
	}
	valid := []string{
		"[guide](guide.md#heading)", "[query](guide.md?download=1#heading)",
		"![image](asset.png)", "[![image](asset.png)](https://example.com)",
		"[angle](<file name.md>)", "[encoded](file%20name.md)",
		"[balanced](nested(a).md)", `[escaped](nested\(a\).md)`,
		`[title](guide.md "Title with a ) character")`, "[title](guide.md 'Title')",
		"[title](guide.md (Title))", "[label\non two lines](guide.md)",
		`[title](guide.md "An [example](not-a-link)")`,
		"[entity](amp&amp;name.md)", `[escaped](amp\&name.md)`, "[empty]()", "[anchor](#heading)",
		"[web](https://example.com/missing)", "[mail](mailto:hello@example.com)",
		"[reference][guide]\n[guide]: guide.md#heading \"Title\"",
		"[reference][guide]\n[guide]: guide.md \"An [example](not-a-link)\"",
		"[shortcut]\n[shortcut]:\n  <file name.md>",
	}
	for _, markdown := range valid {
		t.Run(markdown, func(t *testing.T) {
			wantTargets := 1
			if strings.HasPrefix(markdown, "[![") {
				wantTargets = 2
			}
			if got := len(links(markdown)); got != wantTargets {
				t.Fatalf("found %d targets, want %d", got, wantTargets)
			}
			writeFixture(t, root, "README.md", markdown)
			issues, _, err := check(root)
			if err != nil || len(issues) != 0 {
				t.Fatalf("issues = %v, error = %v", issues, err)
			}
		})
	}
}

func TestMissingTargetsAndLocations(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "docs/guide.md", "# Guide\n\n[missing](no.md)\n![missing image](none.png)\n\n[reference]: absent.md\n")
	issues, count, err := check(root)
	if err != nil || count != 1 || len(issues) != 3 {
		t.Fatalf("count = %d, issues = %v, error = %v", count, issues, err)
	}
	for _, location := range []string{"docs/guide.md:3:", "docs/guide.md:4:", "docs/guide.md:6:"} {
		if !strings.Contains(strings.Join(issues, "\n"), location) {
			t.Errorf("missing diagnostic %q: %v", location, issues)
		}
	}
}

func TestNonportablePaths(t *testing.T) {
	root := t.TempDir()
	for _, target := range []string{"/README.md", "C:/README.md", "C:\\README.md", "file:///README.md", "FILE:///README.md", "%2FREADME.md", "C%3A/README.md", "../outside.md", "//server/share.md"} {
		t.Run(target, func(t *testing.T) {
			if err := checkTarget(root, root, target); err == nil {
				t.Fatalf("accepted nonportable target %q", target)
			}
		})
	}
}

func TestCodeAndCommentsAreIgnored(t *testing.T) {
	for _, markdown := range []string{
		"```md\n[example](missing.md)\n```\n", "~~~md\n[example](missing.md)\n~~~\n",
		"    ```md\n    [example](missing.md)\n    ```\n", "> ```md\n> [example](missing.md)\n> ```\n",
		"````md\n```\n[example](missing.md)\n````\n",
		"```md\n``` not a closing fence\n[example](missing.md)\n",
		"~~~\n[example](missing.md)", "`[example](missing.md)`",
		"``a ` tick [example](missing.md)``", "`line one\n[example](missing.md)`",
		"<!-- [example](missing.md) -->", "<!--\n[example](missing.md)",
		`\[example](missing.md)`,
	} {
		t.Run(markdown, func(t *testing.T) {
			if targets := links(markdown); len(targets) != 0 {
				t.Fatalf("code or comment produced targets: %v", targets)
			}
		})
	}
	for _, markdown := range []string{"`unclosed [real](file.md)", "```\nignored\n```\n[real](file.md)", "<!-- comment --> [real](file.md)", "[real `code`](file.md)"} {
		if targets := links(markdown); len(targets) != 1 || targets[0].path != "file.md" {
			t.Fatalf("real link disappeared: %q: %v", markdown, targets)
		}
	}
	for _, markdown := range []string{`[unfinished](file.md "title`, `[bad](file name.md)`, "[unclosed", "] unmatched", `[bad](nested(unclosed.md)`} {
		if targets := links(markdown); len(targets) != 0 {
			t.Fatalf("malformed non-link produced targets: %q: %v", markdown, targets)
		}
	}
}

func TestIgnoredDirectoriesAndCLI(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "README.md", "[local](README.md)")
	for _, dir := range []string{".git", "node_modules", "vendor", ".agents", ".codex"} {
		writeFixture(t, root, dir+"/bad.md", "[missing](absent.md)")
	}
	var out bytes.Buffer
	if code := run([]string{root}, &out); code != 0 || !strings.Contains(out.String(), "Checked 1 Markdown files: 0") {
		t.Fatalf("exit = %d, output = %q", code, out.String())
	}
	if code := run([]string{"-root", root}, &out); code != 0 {
		t.Fatalf("-root exited %d: %s", code, out.String())
	}
	if code := run([]string{"-root", root, root}, &out); code != 2 {
		t.Fatalf("duplicate root exited %d", code)
	}
	out.Reset()
	if code := run([]string{"--help"}, &out); code != 0 || !strings.Contains(out.String(), "not heading anchors") {
		t.Fatalf("help exit = %d, output = %q", code, out.String())
	}
	if code := run([]string{root, root}, &out); code != 2 {
		t.Fatalf("extra arguments exit = %d", code)
	}
	if code := run([]string{filepath.Join(root, "missing")}, &out); code != 1 {
		t.Fatalf("missing root exit = %d", code)
	}
}

func TestRepositoryMarkdownLinks(t *testing.T) {
	issues, count, err := check("../..")
	if err != nil || count == 0 {
		t.Fatalf("checked %d files: %v", count, err)
	}
	for _, issue := range issues {
		t.Error(issue)
	}
}

type brokenReportWriter struct{ short bool }

func (w brokenReportWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, io.ErrClosedPipe
}

func TestOutputFailureReturnsFailure(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "README.md", "No links.")
	for _, short := range []bool{false, true} {
		for _, args := range [][]string{{root}, {"--help"}} {
			if code := run(args, brokenReportWriter{short: short}); code != 1 {
				t.Fatalf("args %v, short=%t: exit = %d, want 1", args, short, code)
			}
		}
	}
}
