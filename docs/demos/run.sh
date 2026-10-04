#!/usr/bin/env bash
# Run real triallang programs, then pace their output for a terminal recording.
set -euo pipefail
cd "$(dirname "$0")/../.."

if [ "$#" -ne 1 ]; then
  echo "usage: bash docs/demos/run.sh {julia|wave|distance|cellular|labyrinth|stack}" >&2
  exit 2
fi

mode=$1
args=(run --timeout 3m)
case "$mode" in
  julia)
    name=the-julia-set
    title='THE JULIA SET'
    subtitle='49 x 17 pixels | fixed-point complex orbits | 32 updates'
    ;;
  wave)
    name=the-wave-chamber
    title='THE WAVE CHAMBER'
    subtitle='Two circular waves | four phases | paced playback'
    args+=(--canon)
    ;;
  distance)
    name=the-distance-field
    title='THE DISTANCE FIELD'
    subtitle='Two spheres and a floor | surface normals | shadow rays'
    ;;
  cellular)
    name=the-cellular-court
    title='THE CELLULAR COURT'
    subtitle='Rule 90 | nine generations | paced playback'
    ;;
  labyrinth)
    name=the-labyrinth
    title='THE LABYRINTH'
    subtitle='Breadth-first search | shortest route around a dead end'
    ;;
  stack)
    name=the-stack-clerk
    title='THE STACK CLERK'
    subtitle='Postfix evaluation | a stack of values | integer arithmetic'
    args+=(--serve '8 3 - 4 * 2 /')
    ;;
  *) echo "unknown demo: $mode" >&2; exit 2 ;;
esac
args+=("examples/$name.trial")
trial=${TRIAL_BIN:-./trial}
if [ ! -x "$trial" ]; then
  echo 'Build the CLI first: go build -o trial ./cmd/trial' >&2
  exit 2
fi

# Plain output is useful for comparing the recording's input with depositions.
if [ "${TRIAL_DEMO_PLAIN:-0}" = 1 ]; then
  exec "$trial" "${args[@]}"
fi

output=$(mktemp)
trap 'rm -f "$output"; printf "\033[0m\033[?25h"' EXIT
printf '\033[?25l\033[2J\033[H\033[1;36m%s\033[0m\n' "$title"
printf '%s\n\n' "$subtitle"
printf '\033[38;5;109m$ ./trial run --timeout 3m'
case "$mode" in
  wave) printf ' --canon' ;;
  stack) printf ' --serve "8 3 - 4 * 2 /" \\\n   ' ;;
esac
printf ' examples/%s.trial\033[0m\n\n' "$name"
printf '\033[2mComputing in triallang...\033[0m\n'

# Keep failure visible. No fixture or stored frame substitutes for this output.
"$trial" "${args[@]}" > "$output"
printf '\033[1A\033[2K'
awk -v mode="$mode" '
  function paint(line,    i, ch, color) {
    for (i = 1; i <= length(line); i++) {
      ch = substr(line, i, 1)
      color = "36"
      if (ch ~ /[@%]/) color = "1;95"
      else if (ch == "#") color = mode == "labyrinth" ? "38;5;60" : "1;96"
      else if (ch == "*") color = "1;92"
      else if (ch == "+") color = "38;5;110"
      else if (ch == "S" || ch == "E") color = "1;95"
      printf "\033[%sm%s", color, ch
    }
    printf "\033[0m\n"
    fflush()
  }
  mode == "wave" {
    # trial run separates these newline-terminated proclamations with a blank.
    if ($0 == "") next
    if (row == 0) {
      if (frame > 0) printf "\033[14A"
      printf "\033[2K\033[1;95mPhase %d / 120\033[0m\n", frame * 30
    }
    paint($0)
    row++
    if (row == 13) {
      row = 0
      frame++
      system("sleep 1.2")
    }
    next
  }
  {
    paint($0)
    if (mode == "cellular") system("sleep 0.3")
  }
' "$output"
if [ "$mode" = stack ]; then
  printf '\n\033[2m(8 - 3) * 4 / 2 = 10\033[0m\n'
fi
printf '\n\033[1;32mDemo complete.\033[0m\n'
