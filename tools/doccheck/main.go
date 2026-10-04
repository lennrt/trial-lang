// Doccheck checks local Markdown targets without network access.
package main

import (
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

func run(args []string, out io.Writer) int {
	flags := flag.NewFlagSet("doccheck", flag.ContinueOnError)
	flags.SetOutput(out)
	root := flags.String("root", ".", "repository root")
	var usageErr error
	flags.Usage = func() {
		usageErr = writeLine(out, "Usage: doccheck [-root repository-root]\n       doccheck [repository-root]\nThe repository root defaults to the current directory. Supply it only once.\nChecks Markdown inline links/images and reference definitions against local files.\nSupports escaped punctuation, angle destinations, URL-encoded paths, and titles.\nIgnores fenced code, inline code, HTML comments, external URLs, and fragments.\nChecks target existence only, not heading anchors or reference-label resolution.\nAbsolute paths and paths outside the repository are errors. No network requests.\nSkips .git, node_modules, vendor, .agents, and .codex directories.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if usageErr != nil {
				return 1
			}
			return 0
		}
		return 2
	}
	rootFlag := false
	flags.Visit(func(option *flag.Flag) { rootFlag = rootFlag || option.Name == "root" })
	if flags.NArg() > 1 || rootFlag && flags.NArg() == 1 {
		flags.Usage()
		return 2
	}
	if flags.NArg() == 1 {
		*root = flags.Arg(0)
	}
	issues, count, err := check(*root)
	if err != nil {
		// The command already fails; report its cause as far as the writer allows.
		_ = writeLine(out, err.Error())
		return 1
	}
	for _, issue := range issues {
		if err := writeLine(out, issue); err != nil {
			return 1
		}
	}
	if err := writeLine(out, fmt.Sprintf("Checked %d Markdown files: %d local link errors.", count, len(issues))); err != nil {
		return 1
	}
	if len(issues) > 0 {
		return 1
	}
	return 0
}

func writeLine(out io.Writer, line string) error {
	line += "\n"
	n, err := io.WriteString(out, line)
	if err == nil && n != len(line) {
		return io.ErrShortWrite
	}
	return err
}

func check(root string) ([]string, int, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, 0, err
	}
	var issues []string
	count := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", ".agents", ".codex":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		count++
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, link := range links(string(source)) {
			if err := checkTarget(root, filepath.Dir(path), link.path); err != nil {
				line := 1 + strings.Count(string(source[:link.position]), "\n")
				issues = append(issues, fmt.Sprintf("%s:%d: %q: %v", filepath.ToSlash(relative), line, link.path, err))
			}
		}
		return nil
	})
	return issues, count, err
}

func checkTarget(root, base, target string) error {
	parsed, err := url.Parse(html.UnescapeString(target))
	if err != nil {
		return err
	}
	if len(parsed.Scheme) == 1 || strings.EqualFold(parsed.Scheme, "file") {
		return errors.New("use a repository-relative path")
	}
	if parsed.Scheme != "" {
		return nil
	}
	if strings.HasPrefix(target, "/") || strings.HasPrefix(parsed.Path, "\\") || strings.HasPrefix(parsed.Path, "/") || len(parsed.Path) >= 2 && parsed.Path[1] == ':' {
		return errors.New("use a repository-relative path")
	}
	if parsed.Path == "" {
		return nil
	}
	path := filepath.Join(base, filepath.FromSlash(parsed.Path))
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("target leaves the repository")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("target unavailable: %w", err)
	}
	return nil
}

type link struct {
	path     string
	position int
}

var reference = regexp.MustCompile(`(?m)^ {0,3}\[[^\]\n]+\]:[ \t]*(?:\n[ \t]*)?`)

func links(source string) []link {
	text := maskCode(source)
	var found []link
	referenceLines := make(map[int]int)
	for _, match := range reference.FindAllStringIndex(text, -1) {
		if target, end, ok := destination(text, match[1]); ok {
			found = append(found, link{target, match[0]})
			lineEnd := strings.IndexByte(text[end:], '\n')
			if lineEnd < 0 {
				referenceLines[match[0]] = len(text)
			} else {
				referenceLines[match[0]] = end + lineEnd
			}
		}
	}
	var brackets []int
	for i := 0; i < len(text); i++ {
		if end, ok := referenceLines[i]; ok {
			i = end
			continue
		}
		if (text[i] != '[' && text[i] != ']') || escaped(text, i) {
			continue
		}
		if text[i] == '[' {
			brackets = append(brackets, i)
			continue
		}
		if len(brackets) == 0 {
			continue
		}
		start := brackets[len(brackets)-1]
		brackets = brackets[:len(brackets)-1]
		end := i + 1
		if end >= len(text) || text[end] != '(' {
			continue
		}
		target, end, ok := destination(text, skipSpace(text, end+1))
		if !ok {
			continue
		}
		end = skipSpace(text, end)
		if end < len(text) && (text[end] == '"' || text[end] == '\'' || text[end] == '(') {
			quote := text[end]
			if quote == '(' {
				quote = ')'
			}
			end++
			for end < len(text) && (text[end] != quote || escaped(text, end)) {
				end++
			}
			end = skipSpace(text, end+1)
		}
		if end < len(text) && text[end] == ')' {
			found = append(found, link{target, start})
			i = end
		}
	}
	return found
}

func skipSpace(text string, i int) int {
	for i < len(text) && strings.ContainsRune(" \t\r\n", rune(text[i])) {
		i++
	}
	return i
}

func destination(text string, start int) (string, int, bool) {
	if start >= len(text) {
		return "", start, false
	}
	angle := text[start] == '<'
	if angle {
		start++
	}
	var value strings.Builder
	depth, i := 0, start
	for ; i < len(text); i++ {
		c := text[i]
		if c == '\\' && i+1 < len(text) && punctuation(text[i+1]) {
			i++
			value.WriteByte(text[i])
			continue
		}
		if angle {
			if c == '>' {
				return value.String(), i + 1, true
			}
			if c == '\n' || c == '\r' {
				return "", i, false
			}
		} else {
			if strings.ContainsRune(" \t\r\n", rune(c)) || c == ')' && depth == 0 {
				break
			}
			switch c {
			case '(':
				depth++
			case ')':
				depth--
			}
		}
		value.WriteByte(c)
	}
	return value.String(), i, !angle && depth == 0
}

func punctuation(c byte) bool {
	return c > ' ' && c < '0' || c > '9' && c < 'A' || c > 'Z' && c < 'a' || c > 'z' && c < 127
}
