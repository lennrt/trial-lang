package court

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

type shaderLog struct {
	docket.Log
	dossierRecords int
}

func (m *shaderLog) Commit(ctx context.Context, c docket.Case, step docket.Step) error {
	if err := m.Log.Commit(ctx, c, step); err != nil {
		return err
	}
	for _, a := range step.Appends {
		if a.Topic == c.Dossier() {
			m.dossierRecords++
		}
	}
	return nil
}

// Run the real example offices with small diagnostic articles, or render the
// full frame when articles is empty. The deposition runner covers unbatched
// execution too. Batching here keeps additional geometric checks inexpensive.
func runShader(t *testing.T, name, articles string) []string {
	t.Helper()
	src := example(t, name)
	if articles != "" {
		start := strings.Index(src, "\nARTICLE 1.")
		end := strings.Index(src, "\nTHE OFFICE OF")
		if start < 0 || end <= start {
			t.Fatal("shader has no replaceable articles or offices")
		}
		src = src[:start] + "\nARTICLE 1.\n" + articles + "\nADJOURN INDEFINITELY.\n" + src[end:]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log := docket.NewMemoryLog()
	t.Cleanup(log.Close)
	if strings.Contains(src, "INCORPORATE BY REFERENCE statutes-of-trigonometry.") {
		statutes, err := os.ReadFile("../../canon/statutes-of-trigonometry.trial")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := Enact(ctx, log, string(statutes)); err != nil {
			t.Fatalf("enact trigonometry: %v", err)
		}
	}
	c, err := File(ctx, log, src)
	if err != nil {
		t.Fatalf("file shader: %v", err)
	}
	metered := &shaderLog{Log: log}
	ct := &Court{Log: metered, Case: c, Expedite: 1000}
	out, err := ct.Proceed(ctx)
	if err != nil {
		t.Fatalf("run shader: %v", err)
	}
	if out != OutcomeAdjourned {
		state, inspectErr := Examine(ctx, log, c)
		t.Fatalf("shader outcome = %v, want adjournment; state=%+v, inspect=%v", out, state, inspectErr)
	}
	t.Logf("dossier records: %d", metered.dossierRecords)
	// The public deposition runner reconstructs state from retained records.
	// Keep headroom under its snapshot cap, even though rendering can finish
	// without reading that history. Batching does not remove dossier events.
	if metered.dossierRecords > docket.MaxReadRecords*9/10 {
		t.Fatalf("shader needs %d dossier records; preserve 10%% headroom under %d", metered.dossierRecords, docket.MaxReadRecords)
	}
	if _, err := Examine(ctx, log, c); err != nil {
		t.Fatalf("completed shader cannot be inspected: %v", err)
	}
	return proclamations(t, log, c)
}

func TestShaderJuliaOrbits(t *testing.T) {
	got := runShader(t, "the-julia-set", `
PROCLAIM THE FINDING OF escape-age REGARDING 0 AND 0 AND 0 AND 0.
PROCLAIM THE FINDING OF escape-age REGARDING 10000 AND 0 AND 0 AND 0.
PROCLAIM THE FINDING OF escape-age REGARDING -10000 AND 0 AND 0 AND 0.
PROCLAIM THE FINDING OF escape-age REGARDING 20000 AND 0 AND 0 AND 0.
PROCLAIM THE FINDING OF escape-age REGARDING 0 AND 20000 AND 0 AND 0.
PROCLAIM THE FINDING OF escape-age REGARDING 20001 AND 0 AND 0 AND 0.
PROCLAIM THE FINDING OF escape-age REGARDING 0 AND 0 AND 10000 AND 0.
PROCLAIM THE FINDING OF escape-age REGARDING 0 AND 0 AND -10000 AND 0.
`)
	// For c=0 the unit circle is bounded, 2 reaches 4 after one update,
	// and points already outside radius 2 escape before any update. Starting
	// at zero with c=1 follows 0,1,2,5. With c=-1 it cycles between 0 and -1.
	want := "32|32|32|1|1|0|3|32"
	if strings.Join(got, "|") != want {
		t.Fatalf("orbit ages = %v, want %s", got, want)
	}
}

func TestShaderDistanceGeometry(t *testing.T) {
	got := runShader(t, "the-distance-field", `
PROCLAIM THE FINDING OF scene-distance REGARDING 0 AND 0 AND 0.
PROCLAIM THE FINDING OF scene-distance REGARDING 750 AND 0 AND 0.
PROCLAIM THE FINDING OF scene-distance REGARDING 0 AND 0 AND -1000.
PROCLAIM THE FINDING OF scene-distance REGARDING 1100 AND -300 AND 700.
PROCLAIM THE FINDING OF scene-distance REGARDING 2000 AND -800 AND 0.
PROCLAIM THE FINDING OF scene-distance REGARDING 2000 AND -900 AND 0.
LET IT BE RECORDED THAT probe IS THE FINDING OF march REGARDING 0 AND 0 AND -3500 AND 0 AND 0 AND 1000.
PROCLAIM THE hit ENTERED IN probe.
PROCLAIM THE z ENTERED IN probe.
LET IT BE RECORDED THAT probe IS THE FINDING OF march REGARDING 2000 AND 0 AND 0 AND 0 AND -1000 AND 0.
PROCLAIM THE hit ENTERED IN probe.
PROCLAIM THE y ENTERED IN probe.
LET IT BE RECORDED THAT probe IS THE FINDING OF march REGARDING -3000 AND 1000 AND -3500 AND 0 AND 1000 AND 0.
PROCLAIM THE hit ENTERED IN probe.
SHOULD THE steps ENTERED IN probe FAIL TO EXCEED march-limit, PROCLAIM SUSTAINED.
PROCLAIM THE FINDING OF shadowed REGARDING 1000 AND -776 AND 0.
PROCLAIM THE FINDING OF shadowed REGARDING -2000 AND -776 AND 0.
LET IT BE RECORDED THAT ramp IS " .:-=+*#%@".
PROCLAIM THE FINDING OF surface-shade REGARDING 1000 AND -800 AND 0.
PROCLAIM THE FINDING OF surface-shade REGARDING -2000 AND -800 AND 0.
`)
	want := "-750|0|250|-500|0|-100|SUSTAINED|-750|SUSTAINED|-800|OVERRULED|SUSTAINED|SUSTAINED|OVERRULED|.|+"
	if strings.Join(got, "|") != want {
		t.Fatalf("geometric probes = %v, want %s", got, want)
	}
}

func TestShaderDistanceMatchesGeometry(t *testing.T) {
	// Compare the optimized field with direct Euclidean sphere distances and
	// a plane. The fixed grid includes interiors, tangent points, below-floor
	// points, and places where each different surface supplies the minimum.
	var articles strings.Builder
	var want []int64
	for _, x := range []int64{-1000, 0, 750, 1100, 1600, 3000} {
		for _, y := range []int64{-900, -800, -796, -750, -300, 0, 750, 1000} {
			for _, z := range []int64{-1000, 0, 700, 2000} {
				fmt.Fprintf(&articles, "PROCLAIM THE FINDING OF scene-distance REGARDING %d AND %d AND %d.\n", x, y, z)
				first := int64(math.Sqrt(float64(x*x+y*y+z*z))) - 750
				a, b, c := x-1100, y+300, z-700
				second := int64(math.Sqrt(float64(a*a+b*b+c*c))) - 500
				want = append(want, min(first, second, y+800))
			}
		}
	}
	got := runShader(t, "the-distance-field", articles.String())
	if len(got) != len(want) {
		t.Fatalf("got %d distances, want %d", len(got), len(want))
	}
	for i, expected := range want {
		if got[i] != strconv.FormatInt(expected, 10) {
			t.Errorf("distance probe %d = %s, want %d", i, got[i], expected)
		}
	}
}

func TestShaderSquareRootBounds(t *testing.T) {
	// Each estimate must remain an upper bound around its branch threshold.
	// Exact square neighbors also catch premature Newton convergence.
	inputs := []int64{0, 1, 2, 3, 4, 15, 16, 17, 255, 256, 257, 1023, 1024, 1025,
		65535, 65536, 65537, 1073741823, 1073741824, 1073741825, math.MaxInt64 - 1, math.MaxInt64}
	for _, tt := range []struct{ example, office string }{
		{"the-distance-field", "field-root"},
		{"the-wave-chamber", "wave-root"},
	} {
		t.Run(tt.example, func(t *testing.T) {
			var articles strings.Builder
			for _, n := range inputs {
				fmt.Fprintf(&articles, "PROCLAIM THE FINDING OF %s REGARDING %d.\n", tt.office, n)
			}
			got := runShader(t, tt.example, articles.String())
			if len(got) != len(inputs) {
				t.Fatalf("got %d roots, want %d", len(got), len(inputs))
			}
			for i, n := range inputs {
				root, err := strconv.ParseInt(got[i], 10, 64)
				// Division tests both square bounds without overflowing, even
				// if a regression returns a value far above the correct root.
				if err != nil || root < 0 || (root > 0 && uint64(root) > uint64(n)/uint64(root)) || uint64(root)+1 <= uint64(n)/(uint64(root)+1) {
					t.Errorf("floor square root of %d = %q, error %v", n, got[i], err)
				}
			}
		})
	}
}

func TestShaderWavePhases(t *testing.T) {
	got := runShader(t, "the-wave-chamber", `
LET IT BE RECORDED THAT sines IS THE FINDING OF the-table-of-sines.
PROCLAIM THE FINDING OF wave-root REGARDING 0.
PROCLAIM THE FINDING OF wave-root REGARDING 1.
PROCLAIM THE FINDING OF wave-root REGARDING 2.
PROCLAIM THE FINDING OF wave-root REGARDING 225.
PROCLAIM THE FINDING OF wave-root REGARDING 224.
LET IT BE RECORDED THAT phase-zero IS THE FINDING OF interference REGARDING 30 AND 40 AND 0.
LET IT BE RECORDED THAT phase-full IS THE FINDING OF interference REGARDING 30 AND 40 AND 120.
LET IT BE RECORDED THAT phase-half IS THE FINDING OF interference REGARDING 30 AND 40 AND 60.
LET IT BE RECORDED THAT reflected IS THE FINDING OF interference REGARDING -30 AND -40 AND 0.
SHOULD phase-zero EQUAL phase-full, PROCLAIM SUSTAINED.
SHOULD phase-zero PLUS phase-half EQUAL 0, PROCLAIM SUSTAINED.
SHOULD phase-zero EQUAL reflected, PROCLAIM SUSTAINED.
`)
	want := "0|1|1|15|14|SUSTAINED|SUSTAINED|SUSTAINED"
	if strings.Join(got, "|") != want {
		t.Fatalf("wave probes = %v, want %s", got, want)
	}
}

func TestShaderFrames(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
		frames        int
	}{
		{"the-julia-set", 49, 17, 1},
		{"the-wave-chamber", 33, 13, 4},
		{"the-distance-field", 20, 10, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := runShader(t, tt.name, "")
			if len(got) != tt.frames {
				t.Fatalf("got %d frames, want %d", len(got), tt.frames)
			}
			seen := make(map[string]bool)
			for i, frame := range got {
				lines := strings.Split(frame, "\n")
				if len(lines) != tt.height+1 || lines[tt.height] != "" {
					t.Fatalf("frame %d: want %d rows and a trailing newline", i, tt.height)
				}
				for row, line := range lines[:tt.height] {
					if len(line) != tt.width {
						t.Fatalf("frame %d row %d: width %d, want %d", i, row, len(line), tt.width)
					}
					if strings.Trim(line, " .:-=+*#%@") != "" {
						t.Fatalf("frame %d row %d uses a character outside the ramp: %q", i, row, line)
					}
					if tt.name == "the-julia-set" {
						for column := range tt.width {
							if line[column] != lines[tt.height-1-row][tt.width-1-column] {
								t.Fatalf("Julia frame lacks half-turn symmetry at (%d,%d)", column, row)
							}
						}
					}
					if tt.name == "the-wave-chamber" {
						for column := range tt.width {
							if line[column] != line[tt.width-1-column] || line[column] != lines[tt.height-1-row][column] {
								t.Fatalf("wave frame lacks reflection symmetry at (%d,%d)", column, row)
							}
						}
					}
				}
				if seen[frame] {
					t.Fatalf("animation repeats frame %d before a full period", i)
				}
				seen[frame] = true
			}
		})
	}
}
