// Command demogen generates the Orrery triallang demo and its deposition.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	frameCount   = 24
	panelWidth   = 78
	objectWidth  = 76
	objectHeight = 18
	maxOutputLen = 1 << 20
)

type point3 struct {
	x, y, z float64
}

var wordmarkGlyphs = map[byte][5]string{
	'T': {"#####", "..#..", "..#..", "..#..", "..#.."},
	'R': {"####.", "#...#", "####.", "#..#.", "#...#"},
	'I': {"#####", "..#..", "..#..", "..#..", "#####"},
	'A': {".###.", "#...#", "#####", "#...#", "#...#"},
	'L': {"#....", "#....", "#....", "#....", "#####"},
	'-': {".....", ".....", "#####", ".....", "....."},
	'N': {"#...#", "##..#", "#.#.#", "#..##", "#...#"},
	'G': {".####", "#....", "#.###", "#...#", ".###."},
}

var wordmark = renderWordmark()

type artifact struct {
	path string
	data []byte
}

func main() {
	log.SetFlags(0)
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "fail if generated files are stale")
	write := flag.Bool("write", false, "write generated files")
	flag.Parse()
	if *check == *write {
		return fmt.Errorf("select exactly one of -check or -write")
	}

	frames := make([]string, frameCount)
	for frame := range frameCount {
		frames[frame] = renderFrame(frame)
	}
	artifacts := []artifact{
		{path: filepath.Join(*root, "examples", "the-orrery.trial"), data: []byte(trialSource(frames))},
		{path: filepath.Join(*root, "examples", "the-orrery.deposition"), data: []byte(deposition(frames))},
	}
	for _, item := range artifacts {
		if len(item.data) > maxOutputLen {
			return fmt.Errorf("generated file exceeds %d bytes: %s", maxOutputLen, item.path)
		}
		if *write {
			if err := os.WriteFile(item.path, item.data, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", item.path, err)
			}
			continue
		}
		got, err := os.ReadFile(item.path)
		if err != nil {
			return fmt.Errorf("read %s: %w", item.path, err)
		}
		if !bytes.Equal(got, item.data) {
			return fmt.Errorf("generated file is stale: %s; run make demo-generate", item.path)
		}
	}
	return nil
}

func renderFrame(frame int) string {
	lines := []string{
		"+" + strings.Repeat("-", panelWidth) + "+",
		panelLine(" TRIAL-LANG // TRANSACTIONAL SPHERE"),
		panelLine(fmt.Sprintf(" FRAME %02d/%d  |  OFFSET %04d  |  STATE COMMITTED  |  REPLAY EXACT", frame+1, frameCount, frame+1)),
		panelLine(""),
	}
	for _, line := range wordmark {
		lines = append(lines, panelLine(line))
	}
	lines = append(lines, panelLine(""))
	for _, line := range renderSphere(frame) {
		lines = append(lines, panelLine(" "+line+" "))
	}
	lines = append(lines,
		panelLine(""),
		panelLine(" STOP THE COURT. RESTART IT. THE SPHERE CONTINUES AT THIS OFFSET."),
		"+"+strings.Repeat("-", panelWidth)+"+",
	)
	return strings.Join(lines, "\n")
}

func renderSphere(frame int) []string {
	canvas := make([][]byte, objectHeight)
	depth := make([]float64, objectWidth*objectHeight)
	for row := range canvas {
		canvas[row] = []byte(strings.Repeat(" ", objectWidth))
	}
	for index := range depth {
		depth[index] = math.Inf(-1)
	}

	angle := 2 * math.Pi * float64(frame) / frameCount
	light := normalize(point3{-0.45, 0.35, 1})
	const shades = ".,-~:;=!*#$@"
	for latitudeStep := range 180 {
		latitude := math.Pi * (float64(latitudeStep) + 0.5) / 180
		sinLatitude, cosLatitude := math.Sincos(latitude)
		for longitudeStep := range 360 {
			longitude := 2 * math.Pi * float64(longitudeStep) / 360
			sinLongitude, cosLongitude := math.Sincos(longitude)
			point := point3{
				x: sinLatitude * cosLongitude,
				y: cosLatitude,
				z: sinLatitude * sinLongitude,
			}
			point = rotate(point, -0.32, angle, 0.16*math.Sin(angle))
			perspective := 4.5 / (4.5 - point.z)
			x := int(math.Round(objectWidth/2 + 17*point.x*perspective))
			y := int(math.Round(objectHeight/2 - 7.4*point.y*perspective))
			if x < 0 || x >= objectWidth || y < 0 || y >= objectHeight {
				continue
			}
			index := y*objectWidth + x
			if point.z <= depth[index] {
				continue
			}
			depth[index] = point.z
			brightness := 0.16 + 0.84*max(0, dot(point, light))
			shadeIndex := min(int(brightness*float64(len(shades))), len(shades)-1)
			character := shades[shadeIndex]
			meridian := math.Abs(math.Sin(4*longitude)) < 0.1
			parallel := math.Abs(math.Sin(4*latitude)) < 0.08
			switch {
			case meridian && parallel:
				character = '+'
			case meridian:
				character = '|'
			case parallel:
				character = '-'
			}
			canvas[y][x] = character
		}
	}
	lines := make([]string, objectHeight)
	for row := range canvas {
		lines[row] = string(canvas[row])
	}
	return lines
}

func rotate(point point3, xAngle, yAngle, zAngle float64) point3 {
	sinX, cosX := math.Sincos(xAngle)
	sinY, cosY := math.Sincos(yAngle)
	sinZ, cosZ := math.Sincos(zAngle)
	point.y, point.z = point.y*cosX-point.z*sinX, point.y*sinX+point.z*cosX
	point.x, point.z = point.x*cosY+point.z*sinY, -point.x*sinY+point.z*cosY
	point.x, point.y = point.x*cosZ-point.y*sinZ, point.x*sinZ+point.y*cosZ
	return point
}

func dot(a, b point3) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }

func normalize(point point3) point3 {
	length := math.Sqrt(dot(point, point))
	return point3{point.x / length, point.y / length, point.z / length}
}

func renderWordmark() []string {
	lines := make([]strings.Builder, 5)
	for column, letter := range []byte("TRIAL-LANG") {
		glyph := wordmarkGlyphs[letter]
		for row := range lines {
			if column > 0 {
				lines[row].WriteByte(' ')
			}
			lines[row].WriteString(strings.ReplaceAll(glyph[row], ".", " "))
		}
	}
	result := make([]string, len(lines))
	for row := range lines {
		result[row] = lines[row].String()
	}
	return result
}

func panelLine(text string) string {
	if len(text) > panelWidth {
		panic("demo panel line exceeds its fixed width")
	}
	return "|" + text + strings.Repeat(" ", panelWidth-len(text)) + "|"
}

func trialSource(frames []string) string {
	var out strings.Builder
	out.WriteString(`FORM K-1.
IN THE MATTER OF: the-orrery.
FILED BY: the keeper of the transactional sphere.

OFF THE RECORD: Code generated by go run ./tools/demogen. Do not edit.
OFF THE RECORD: Each shaded sphere frame is evidence in the filing. The Court stores
OFF THE RECORD: the current frame in the case record, commits it, and then
OFF THE RECORD: proclaims it. Stop the process between frames and start it
OFF THE RECORD: again: the next Court resumes from the committed offset.

ARTICLE 1.
    LET IT BE RECORDED THAT frames IS A SCHEDULE COMPRISING
`)
	for index, frame := range frames {
		out.WriteString("        ")
		out.WriteString(strconv.Quote(frame))
		if index == len(frames)-1 {
			out.WriteString(".\n")
		} else {
			out.WriteString(" AND\n")
		}
	}
	out.WriteString(`    LET IT BE RECORDED THAT frame IS 0.

ARTICLE 2.
    LET IT BE RECORDED THAT frame IS frame PLUS 1.
    PROCLAIM THE ITEM AT frame IN frames.
    SHOULD frame FAIL TO FALL SHORT OF 24, REFER TO ARTICLE 5.

ARTICLE 3.
    AWAIT SUMMONS FOR AT MOST 1 DAY, FILED UNDER tick. FAILING WHICH, REFER TO ARTICLE 4.
    SHOULD tick EQUAL "q", REFER TO ARTICLE 5.

ARTICLE 4.
    REFER TO ARTICLE 2.

ARTICLE 5.
    ADJOURN INDEFINITELY.
`)
	return out.String()
}

func deposition(frames []string) string {
	var out strings.Builder
	out.WriteString(`DEPOSITION OF: the-orrery.trial.

OFF THE RECORD: Code generated by go run ./tools/demogen. Do not edit.
OFF THE RECORD: Queue one tick per remaining frame. The deposition checks the
OFF THE RECORD: exact framebuffer without waiting for realtime court days.

`)
	for range len(frames) - 1 {
		out.WriteString("SERVE: tick.\n")
	}
	out.WriteString("\n")
	for _, frame := range frames {
		out.WriteString("EXPECT PROCLAMATION: ")
		out.WriteString(strconv.Quote(frame))
		out.WriteString(".\n")
	}
	out.WriteString("\nEXPECT RECORD frame: ")
	out.WriteString(strconv.Itoa(len(frames)))
	out.WriteString(".\nEXPECT ADJOURNMENT.\n")
	return out.String()
}
