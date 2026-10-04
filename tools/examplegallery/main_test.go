package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRepositoryGallery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	items, err := collect(ctx, "../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 || len(items[1].Frames) != 4 {
		t.Fatalf("want six examples and four wave frames, got %+v", items)
	}
	first, err := render(items)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(items)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical executed output produced different gallery bytes")
	}
	if err := store("../..", first, false); err != nil {
		t.Fatal(err)
	}
}

func TestGalleryEscapesOutputWithoutChangingIt(t *testing.T) {
	const hostile = "\n</script><img src=x onerror=alert(1)>\n<svg onload=alert(2)> & \"quoted\"  two spaces"
	items := []exhibit{{Name: "escaping", Title: "<A&B> \"quoted\"", Description: "Output is data <not markup>", Input: "<not markup> & input", Frames: []string{hostile}, Animate: true}}
	artifacts, err := render(items)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range artifacts {
		if bytes.Contains(item.data, []byte("<img")) || bytes.Contains(item.data, []byte("<svg onload")) {
			t.Fatalf("%s contains executable output markup", item.name)
		}
	}
	// XML decoding must recover each original line, including spaces, quotes,
	// and angle brackets. Escaping must not alter the actual program output.
	decoder := xml.NewDecoder(bytes.NewReader(artifacts[0].data))
	var svgText []string
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("gallery is not valid standalone SVG: %v", err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "text" {
			var text string
			if err := decoder.DecodeElement(&text, &start); err != nil {
				t.Fatal(err)
			}
			svgText = append(svgText, text)
		}
	}
	if !strings.Contains(strings.Join(svgText, "\n"), hostile) {
		t.Fatal("SVG text does not preserve the program output")
	}
	html := string(artifacts[1].data)
	// The output region remains keyboard-scrollable. Nesting a span inside
	// pre also prevents HTML's initial-newline rule from dropping a blank row.
	_, screen, ok := strings.Cut(html, `<div class="screen"`)
	if !ok {
		t.Fatal("gallery has no output region")
	}
	screen, _, ok = strings.Cut(screen, "</div>")
	if !ok {
		t.Fatal("output region is not closed")
	}
	var region struct {
		Role     string `xml:"role,attr"`
		Tabindex string `xml:"tabindex,attr"`
		Frame    struct {
			ID   string `xml:"id,attr"`
			Text string `xml:"span"`
		} `xml:"pre"`
	}
	if err := xml.Unmarshal([]byte(`<div class="screen"`+screen+"</div>"), &region); err != nil {
		t.Fatal(err)
	}
	if region.Role != "region" || region.Tabindex != "0" || region.Frame.Text != hostile {
		t.Fatalf("output region lost keyboard access or exact text: %+v", region)
	}
	if region.Frame.ID == "" || strings.Count(html, `aria-controls="`+region.Frame.ID+`"`) != 3 {
		t.Fatal("animation controls do not identify their output region")
	}
	const opening = `<script type="application/json" id="gallery-data">`
	_, jsonAndRest, ok := strings.Cut(html, opening)
	if !ok {
		t.Fatal("gallery has no animation data")
	}
	encoded, _, ok := strings.Cut(jsonAndRest, "</script>")
	if !ok {
		t.Fatal("animation data has no closing tag")
	}
	var decoded []exhibit
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("animation data is not JSON: %v", err)
	}
	if !reflect.DeepEqual(decoded, items) {
		t.Fatal("HTML animation data does not preserve the exact program output")
	}
	if strings.Count(html, "<script") != 2 || strings.Count(html, "</script>") != 2 {
		t.Fatal("program output created an extra script element")
	}
}

func TestGalleryRefusesFailedDeposition(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "examples")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "FORM K-1.\nIN THE MATTER OF: the-julia-set.\nARTICLE 1.\nPROCLAIM \"actual execution\".\nADJOURN INDEFINITELY.\n"
	fixture := "DEPOSITION OF: the-julia-set.trial.\nEXPECT PROCLAMATION: \"invented expected output\".\nEXPECT ADJOURNMENT.\n"
	for name, contents := range map[string]string{"the-julia-set.trial": source, "the-julia-set.deposition": fixture} {
		if err := os.WriteFile(filepath.Join(base, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	items, err := collect(ctx, root)
	if err == nil || !strings.Contains(err.Error(), "deposition failed") || items != nil {
		t.Fatalf("mismatched output generated a gallery: items=%v err=%v", items, err)
	}
}

func TestGalleryCheckDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "docs", "gallery.svg")
	if err := os.WriteFile(path, []byte("hand-edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts := []artifact{{"gallery.svg", []byte("computed output")}}
	if err := store(root, artifacts, false); err == nil {
		t.Fatal("check accepted stale output")
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != "hand-edited" {
		t.Fatalf("check modified the file: %q, %v", unchanged, err)
	}
	if err := store(root, artifacts, true); err != nil {
		t.Fatal(err)
	}
	if err := store(root, artifacts, false); err != nil {
		t.Fatalf("fresh output failed its check: %v", err)
	}
}

func TestGalleryArguments(t *testing.T) {
	var help bytes.Buffer
	if err := run([]string{"-help"}, &help); err != nil || !strings.Contains(help.String(), "-write") {
		t.Fatalf("help failed: %v, %s", err, help.String())
	}
	for _, args := range [][]string{{"-check", "-write"}, {"unexpected"}, {"-unknown"}} {
		if err := run(args, io.Discard); err == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
}
