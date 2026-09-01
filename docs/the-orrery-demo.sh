#!/usr/bin/env bash
# Play the Orrery deposition as a colored terminal animation. With a
# case number, follow the same frames from a live Kafka-backed case.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ -n "${TRIAL_BIN:-}" ]; then
  trial=("$TRIAL_BIN")
  if [ ! -x "${trial[0]}" ]; then
    echo "TRIAL_BIN is not executable: ${trial[0]}" >&2
    exit 2
  fi
else
  trial=(./trial)
  [ -x "${trial[0]}" ] || trial=(go run ./cmd/trial)
fi

if [ "$#" -gt 1 ]; then
  echo "usage: $0 [case-number]" >&2
  exit 2
fi

paint() {
  awk -v delay="${1:-0.12}" '
    function color(code, text) { return sprintf("\033[%sm%s\033[0m", code, text) }
    function paint_sphere(line,    out, i, ch) {
      out = ""
      for (i = 1; i <= length(line); i++) {
        ch = substr(line, i, 1)
        if (ch == "+") out = out color("1;97", ch)
        else if (ch == "|") out = out color("1;95", ch)
        else if (ch ~ /[@$#*]/) out = out color("1;96", ch)
        else if (ch ~ /[!=;]/) out = out color("36", ch)
        else if (ch ~ /[:,~.-]/) out = out color("38;5;33", ch)
        else out = out ch
      }
      return out
    }
    function draw(    i, line) {
      if (count == 0) return
      printf "\033[2J\033[H"
      for (i = 1; i <= count; i++) {
        line = frame[i]
        if (i == 1 || i == 31) print color("38;5;33", line)
        else if (i == 2) print color("1;95", line)
        else if (i == 3) print color("1;36", line)
        else if (i >= 5 && i <= 9) print color("1;38;5;213", line)
        else if (i >= 11 && i <= 28) print paint_sphere(line)
        else if (i == 30) print color("1;92", line)
        else print line
      }
      fflush()
      system("sleep " delay)
      delete frame
      count = 0
    }
    {
      frame[++count] = $0
      if (count == 31) draw()
    }
    END { draw() }
  '
}

if [ "$#" -eq 1 ]; then
  "${trial[@]}" observe "$1" --from-the-beginning 2>/dev/null | paint 0
  exit
fi

# The deposition queues explicit ticks. It tests every byte of every frame.
"${trial[@]}" test --transcript examples/the-orrery.deposition |
  awk '
    /^[[:space:]]+OUTPUT \([0-9]+ entries\):[[:space:]]*$/ { live = 1; next }
    live && /^          / {
      print substr($0, 11)
      lines++
      if (lines == 31) lines = 0
    }
  ' | paint 0.12
