package deposition

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func toySource(t *testing.T, name string) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("../../examples", name+".trial"))
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func replaceToy(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if count := strings.Count(source, old); count != 1 {
		t.Fatalf("fixture contains %d copies of %q, want one", count, old)
	}
	return strings.Replace(source, old, replacement, 1)
}

func runToy(t *testing.T, source string, d *Deposition) {
	t.Helper()
	// Leave room for race instrumentation while keeping changed toys bounded.
	d.AllowDays = 30
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if result := Run(ctx, source, d); !result.OK() {
		t.Fatalf("toy contradicted its deposition: %v", result.Contradictions)
	}
}

func TestStackClerkInputs(t *testing.T) {
	source := toySource(t, "the-stack-clerk")
	for _, tc := range []struct {
		name   string
		input  string
		result string
		tokens int
	}{
		{"signed-add-and-multiply", "12 -5 + 2 *", "14", 5},
		{"subtraction-order", "3 8 -", "-5", 3},
		{"negative-division", "-7 2 /", "-3", 3},
		{"mixed-whitespace", " \t12\n3 + ", "15", 3},
		{"one-integer-summons", "-5", "-5", 1},
		{"explicit-positive-sign", "+7", "7", 1},
		{"sum-times-integer", "1.20 2 *", "2.40", 3},
		{"integer-plus-sum", "2 1.20 +", "3.20", 3},
		{"sum-division-precision", "1.00 3 /", "0.33", 3},
		{"negative-sum-division", "-1.00 3 /", "-0.33", 3},
		{"sum-divided-by-sum", "1.20 0.30 /", "4.00", 3},
		{"wrapping-integer-arithmetic", "9223372036854775807 1 +", "-9223372036854775808", 3},
		{"both-limits", strings.Repeat("1 ", 32) + strings.Repeat("+ ", 31) + "  ", "32", 63},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runToy(t, source, &Deposition{
				Serves:        []string{tc.input},
				Outcome:       "adjournment",
				Proclamations: []string{"Result: " + tc.result, fmt.Sprintf("Tokens executed: %d", tc.tokens)},
				Records: []RecordExpect{
					{Name: "result", Display: tc.result},
					{Name: "depth", Display: "1"},
				},
			})
		})
	}
}

func TestStackClerkRefusesInvalidInputs(t *testing.T) {
	source := toySource(t, "the-stack-clerk")
	for _, tc := range []struct{ name, input, citing string }{
		{"empty", "", "Expression must leave exactly one value."},
		{"only-whitespace", " \t\n ", "Expression must leave exactly one value."},
		{"unused-value", "1 2", "Expression must leave exactly one value."},
		{"empty-stack", "+", "An operator needs two values."},
		{"one-operand", "8 *", "An operator needs two values."},
		{"zero-divisor", "8 0 /", "Division by zero is not permitted."},
		{"sum-zero-divisor", "8 0.00 /", "Division by zero is not permitted."},
		{"sum-numerator-zero-divisor", "8.00 0 /", "Division by zero is not permitted."},
		{"invalid-sum-precision", "1.2 2 *", "1.2"},
		{"invalid-integer", "8 fish +", "fish"},
		{"unknown-operator", "1 2 %", "%"},
		{"integer-out-of-range", "9223372036854775808", "9223372036854775808"},
		{"too-long", "1" + strings.Repeat(" ", 128), "Expression exceeds 128 characters."},
		{"too-deep", strings.Repeat("1 ", 33), "Stack exceeds 32 values."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runToy(t, source, &Deposition{Serves: []string{tc.input}, Outcome: "verdict", Citing: tc.citing})
		})
	}
}

func TestCellularCourtBoundaries(t *testing.T) {
	source := toySource(t, "the-cellular-court")
	for _, tc := range []struct {
		name       string
		width      int
		last       int
		rows       []string
		population string
		citing     string
	}{
		{name: "initial-generation", width: 17, last: 0, rows: []string{"0 |........#........|"}, population: "1"},
		{name: "single-cell-edge", width: 1, last: 2, rows: []string{"0 |#|", "1 |.|", "2 |.|"}, population: "0"},
		{name: "two-cell-edge", width: 2, last: 2, rows: []string{"0 |.#|", "1 |#.|", "2 |.#|"}, population: "1"},
		{name: "empty-board", width: 0, last: 8, citing: "Width must be between 1 and 65."},
		{name: "too-wide", width: 66, last: 8, citing: "Width must be between 1 and 65."},
		{name: "negative-generation", width: 17, last: -1, citing: "Last generation must be between 0 and 32."},
		{name: "too-many-generations", width: 17, last: 33, citing: "Last generation must be between 0 and 32."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			program := replaceToy(t, source, "width SHALL MEAN 17.", fmt.Sprintf("width SHALL MEAN %d.", tc.width))
			program = replaceToy(t, program, "last-generation SHALL MEAN 8.", fmt.Sprintf("last-generation SHALL MEAN %d.", tc.last))
			d := &Deposition{Outcome: "adjournment", Proclamations: tc.rows}
			if tc.citing != "" {
				d.Outcome, d.Citing = "verdict", tc.citing
			} else {
				d.Records = []RecordExpect{{Name: "population", Display: tc.population}, {Name: "generation", Display: fmt.Sprint(tc.last)}}
			}
			runToy(t, program, d)
		})
	}
}

func TestCellularCourtLargestBoard(t *testing.T) {
	source := toySource(t, "the-cellular-court")
	source = replaceToy(t, source, "width SHALL MEAN 17.", "width SHALL MEAN 65.")
	source = replaceToy(t, source, "last-generation SHALL MEAN 8.", "last-generation SHALL MEAN 32.")
	var rows []string
	for generation := 0; generation <= 32; generation++ {
		row := []byte(strings.Repeat(".", 65))
		for column := range row {
			// A Rule 90 row contains binomial coefficients modulo two.
			// Lucas's theorem gives their parity without simulating neighbors.
			distance := column - 32
			k := (generation + distance) / 2
			if distance >= -generation && distance <= generation && (generation+distance)%2 == 0 && k&generation == k {
				row[column] = '#'
			}
		}
		rows = append(rows, fmt.Sprintf("%d |%s|", generation, row))
	}
	runToy(t, source, &Deposition{
		Outcome:       "adjournment",
		Proclamations: rows,
		Records:       []RecordExpect{{Name: "generation", Display: "32"}, {Name: "population", Display: "2"}},
	})
}

func TestLabyrinthBoundaries(t *testing.T) {
	source := toySource(t, "the-labyrinth")
	const maze = `"#######" PLUS
        "#S#...#" PLUS
        "#.#.#.#" PLUS
        "#...#.#" PLUS
        "###.#.#" PLUS
        "#...#E#" PLUS
        "#######"`
	for _, tc := range []struct {
		name   string
		maze   string
		width  int
		start  int
		goal   int
		rows   []string
		steps  string
		citing string
	}{
		{name: "one-vertical-step", maze: "S.E.", width: 2, start: 1, goal: 3, steps: "1", rows: []string{"S.", "E."}},
		{name: "start-is-goal", maze: "S.", width: 2, start: 1, goal: 1, steps: "0", rows: []string{"S."}},
		{name: "no-horizontal-wrap", maze: "#SE#", width: 2, start: 2, goal: 3, citing: "The maze has no route to the exit."},
		{name: "no-reverse-horizontal-wrap", maze: "#ES#", width: 2, start: 3, goal: 2, citing: "The maze has no route to the exit."},
		{name: "start-wall", maze: "#E", width: 2, start: 1, goal: 2, citing: "Start cell is a wall."},
		{name: "goal-wall", maze: "S#", width: 2, start: 1, goal: 2, citing: "Goal cell is a wall."},
		{name: "start-outside", maze: "SE", width: 2, start: 0, goal: 2, citing: "Start cell is outside the maze."},
		{name: "goal-outside", maze: "SE", width: 2, start: 1, goal: 3, citing: "Goal cell is outside the maze."},
		{name: "zero-width", maze: "SE", width: 0, start: 1, goal: 2, citing: "Maze width must be between 2 and 25."},
		{name: "uneven-row", maze: "S.E", width: 2, start: 1, goal: 3, citing: "Maze rows must have equal widths."},
		{name: "empty-maze", maze: "", width: 2, start: 1, goal: 2, citing: "Maze must contain between 1 and 225 cells."},
		{name: "too-many-cells", maze: "S" + strings.Repeat(".", 224) + "E", width: 2, start: 1, goal: 226, citing: "Maze must contain between 1 and 225 cells."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			program := replaceToy(t, source, maze, fmt.Sprintf("%q", tc.maze))
			program = replaceToy(t, program, "width SHALL MEAN 7.", fmt.Sprintf("width SHALL MEAN %d.", tc.width))
			program = replaceToy(t, program, "start-cell SHALL MEAN 9.", fmt.Sprintf("start-cell SHALL MEAN %d.", tc.start))
			program = replaceToy(t, program, "goal-cell SHALL MEAN 41.", fmt.Sprintf("goal-cell SHALL MEAN %d.", tc.goal))
			d := &Deposition{Outcome: "adjournment"}
			if tc.citing != "" {
				d.Outcome, d.Citing = "verdict", tc.citing
			} else {
				d.Proclamations = append([]string{"Shortest route: " + tc.steps + " steps."}, tc.rows...)
				d.Records = []RecordExpect{{Name: "steps", Display: tc.steps}}
			}
			runToy(t, program, d)
		})
	}
}

func TestLabyrinthLargestOpenBoard(t *testing.T) {
	source := toySource(t, "the-labyrinth")
	const oldMaze = `"#######" PLUS
        "#S#...#" PLUS
        "#.#.#.#" PLUS
        "#...#.#" PLUS
        "###.#.#" PLUS
        "#...#E#" PLUS
        "#######"`
	source = replaceToy(t, source, oldMaze, `"S`+strings.Repeat(".", 223)+`E"`)
	source = replaceToy(t, source, "width SHALL MEAN 7.", "width SHALL MEAN 25.")
	source = replaceToy(t, source, "start-cell SHALL MEAN 9.", "start-cell SHALL MEAN 1.")
	source = replaceToy(t, source, "goal-cell SHALL MEAN 41.", "goal-cell SHALL MEAN 225.")
	rows := []string{"Shortest route: 32 steps.", "S" + strings.Repeat("*", 24)}
	for range 7 {
		rows = append(rows, strings.Repeat(".", 24)+"*")
	}
	rows = append(rows, strings.Repeat(".", 24)+"E")
	runToy(t, source, &Deposition{
		Outcome:       "adjournment",
		Proclamations: rows,
		Records:       []RecordExpect{{Name: "steps", Display: "32"}, {Name: "visited", Display: "225"}},
	})
}
