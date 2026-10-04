// Command examplegallery renders verified triallang output as an offline gallery.
package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lennrt/trial-lang/internal/deposition"
)

type exhibit struct {
	Name, Title, Description string
	Input                    string
	Animate                  bool
	Frames                   []string
}

var examples = []exhibit{
	{Name: "the-julia-set", Title: "The Julia Set", Description: "Fixed-point complex orbits, 32 updates per pixel."},
	{Name: "the-wave-chamber", Title: "The Wave Chamber", Description: "Two circular waves, four quarter-period phases.", Animate: true},
	{Name: "the-distance-field", Title: "The Distance Field", Description: "Sphere tracing with surface normals and shadow rays."},
	{Name: "the-cellular-court", Title: "The Cellular Court", Description: "Rule 90, with a separate schedule for each generation."},
	{Name: "the-labyrinth", Title: "The Labyrinth", Description: "Breadth-first search through a maze with a dead end."},
	{Name: "the-stack-clerk", Title: "The Stack Clerk", Description: "A postfix expression, evaluated on a nested stack."},
}

type artifact struct {
	name string
	data []byte
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "examplegallery: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("examplegallery", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	root := flags.String("root", ".", "repository root")
	check := flags.Bool("check", false, "check generated files (the default)")
	write := flags.Bool("write", false, "write the SVG and HTML galleries")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || (*check && *write) {
		return fmt.Errorf("use -root <repository> with either -check or -write")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	items, err := collect(ctx, *root)
	if err != nil {
		return err
	}
	artifacts, err := render(items)
	if err != nil {
		return err
	}
	return store(*root, artifacts, *write)
}

// Only successful executions supply frames. Expected fixture output is never
// substituted for actual output, and a failed deposition stops generation.
func collect(ctx context.Context, root string) ([]exhibit, error) {
	items := make([]exhibit, 0, len(examples))
	for _, example := range examples {
		base := filepath.Join(root, "examples")
		path := filepath.Join(base, example.Name+".deposition")
		fixture, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		dep, err := deposition.Parse(string(fixture))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if dep.Program != example.Name+".trial" {
			return nil, fmt.Errorf("%s names a different program: %s", path, dep.Program)
		}
		if err := deposition.LoadEnactments(dep, base); err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
		source, err := os.ReadFile(filepath.Join(base, dep.Program))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dep.Program, err)
		}
		result := deposition.Run(ctx, string(source), dep)
		if !result.OK() {
			return nil, fmt.Errorf("%s: deposition failed: %s", example.Name, strings.Join(result.Contradictions, "; "))
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(result.Said) == 0 {
			return nil, fmt.Errorf("%s produced no output", example.Name)
		}
		example.Input = strings.Join(dep.Serves, "\n")
		if example.Animate {
			example.Frames = result.Said
		} else {
			example.Frames = []string{strings.Join(result.Said, "\n")}
		}
		items = append(items, example)
	}
	return items, nil
}

func render(items []exhibit) ([]artifact, error) {
	if len(items) == 0 || len(items) > 6 {
		return nil, fmt.Errorf("gallery needs between one and six examples")
	}
	for _, item := range items {
		if len(item.Frames) == 0 {
			return nil, fmt.Errorf("%s has no frames", item.Name)
		}
	}
	var html bytes.Buffer
	tmpl, err := template.New("gallery").Parse(htmlTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&html, items); err != nil {
		return nil, err
	}
	return []artifact{{"gallery.svg", renderSVG(items)}, {"gallery.html", html.Bytes()}}, nil
}

func store(root string, artifacts []artifact, write bool) error {
	for _, item := range artifacts {
		if len(item.data) > 1<<20 {
			return fmt.Errorf("generated %s exceeds one MiB", item.name)
		}
		path := filepath.Join(root, "docs", item.name)
		if write {
			if err := os.WriteFile(path, item.data, 0o644); err != nil {
				return err
			}
			continue
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, item.data) {
			return fmt.Errorf("%s is stale; run %q", path, "go run ./tools/examplegallery -write -root .")
		}
	}
	return nil
}

func escapeXML(text string) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(text)) // bytes.Buffer writes cannot fail.
	return escaped.String()
}

func renderSVG(items []exhibit) []byte {
	var out bytes.Buffer
	out.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="1300" viewBox="0 0 1200 1300" role="img" aria-labelledby="title description">
<title id="title">triallang: six programs, computed in the language</title>
<desc id="description">Verified output from the Julia set, wave interference, distance-field renderer, cellular automaton, maze solver, and postfix calculator. The wave panel shows the first of four frames.</desc>
<rect width="1200" height="1300" fill="#f1f5f9"/>
<g font-family="system-ui, sans-serif">
<text x="36" y="53" font-size="30" font-weight="650" fill="#0f172a">triallang / computed examples</text>
<text x="36" y="83" font-size="16" fill="#475569">Real program output. Integer arithmetic. No graphics library.</text>
`)
	for index, item := range items {
		x, y := 36+(index%2)*578, 126+(index/2)*376
		fmt.Fprintf(&out, "<g>\n<rect x=\"%d\" y=\"%d\" width=\"550\" height=\"352\" rx=\"12\" fill=\"#fff\" stroke=\"#cbd5e1\"/>\n", x, y)
		fmt.Fprintf(&out, "<text x=\"%d\" y=\"%d\" font-size=\"20\" font-weight=\"600\" fill=\"#0f172a\">%s</text>\n", x+20, y+31, escapeXML(item.Title))
		fmt.Fprintf(&out, "<text x=\"%d\" y=\"%d\" font-size=\"12\" fill=\"#475569\">%s</text>\n", x+20, y+53, escapeXML(item.Description))
		fmt.Fprintf(&out, "<rect x=\"%d\" y=\"%d\" width=\"514\" height=\"246\" rx=\"8\" fill=\"#0f172a\"/>\n", x+18, y+70)
		lines := strings.Split(strings.TrimSuffix(item.Frames[0], "\n"), "\n")
		columns := 1
		for _, line := range lines {
			columns = max(columns, utf8.RuneCountInString(line))
		}
		fontSize := min(18.0, 228.0/(float64(len(lines))*1.2), 478.0/(float64(columns)*0.6))
		lineHeight := fontSize * 1.2
		left := float64(x+275) - float64(columns)*fontSize*0.3
		top := float64(y+193) - float64(len(lines))*lineHeight/2 + fontSize
		for row, line := range lines {
			fmt.Fprintf(&out, "<text x=\"%.2f\" y=\"%.2f\" font-family=\"'Courier New', monospace\" font-size=\"%.2f\" fill=\"#d1fae5\" xml:space=\"preserve\">%s</text>\n", left, top+float64(row)*lineHeight, fontSize, escapeXML(line))
		}
		label := "Verified program output"
		if item.Input != "" {
			label = "Input: " + item.Input
		}
		if item.Animate {
			label = fmt.Sprintf("Frame 1 of %d · play all frames in the HTML gallery", len(item.Frames))
		}
		fmt.Fprintf(&out, "<text x=\"%d\" y=\"%d\" font-size=\"12\" fill=\"#475569\">%s</text>\n</g>\n", x+20, y+337, escapeXML(label))
	}
	out.WriteString(`<text x="36" y="1270" font-size="13" fill="#475569">Generated from checked depositions · Open docs/gallery.html for the offline gallery and source links.</text>
</g>
</svg>
`)
	return out.Bytes()
}
