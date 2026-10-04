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

transcript=
output=
cleanup() {
  [ -z "$transcript" ] || rm -f -- "$transcript"
  [ -z "$output" ] || rm -f -- "$output"
  if [ "${TRIAL_DEMO_PLAIN:-0}" != 1 ]; then
    printf '\033[0m\033[?25h'
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

header() {
  printf '\033[?25l\033[2J\033[H\033[1;36mTHE ORRERY\033[0m\n'
  printf '%s\n\n' "$1"
  printf '\033[38;5;109m$ %s\033[0m\n\n' "$2"
}

paint() {
  awk -v delay="$1" '
    function color(code, text) { return sprintf("\033[%sm%s\033[0m", code, text) }
    function paint_sphere(line,    out, i, ch) {
      out = ""
      for (i = 1; i <= length(line); i++) {
        ch = substr(line, i, 1)
        if (ch == "+") out = out color("1;96", ch)
        else if (ch == "|") out = out color("1;95", ch)
        else if (ch ~ /[@$]/) out = out color("1;95", ch)
        else if (ch ~ /[#*]/) out = out color("1;96", ch)
        else if (ch ~ /[!=;]/) out = out color("36", ch)
        else if (ch ~ /[:,~.-]/) out = out color("38;5;110", ch)
        else out = out ch
      }
      return out
    }
    function draw(    i, line) {
      if (drawn++) printf "\033[31A"
      for (i = 1; i <= count; i++) {
        line = frame[i]
        if (i == 1 || i == 31) print color("38;5;60", line)
        else if (i == 2) print color("1;36", line)
        else if (i == 3) print color("1;95", line)
        else if (i >= 5 && i <= 9) print color("36", line)
        else if (i >= 11 && i <= 28) print paint_sphere(line)
        else if (i == 30) print color("38;5;109", line)
        else print line
      }
      fflush()
      if (delay > 0 && system("sleep " delay) != 0) exit 1
      delete frame
      count = 0
    }
    {
      if ($0 == "") next
      frame[++count] = $0
      if (count == 31) draw()
    }
    END {
      if (count != 0) {
        print "demo: incomplete Orrery frame" > "/dev/stderr"
        exit 1
      }
    }
  '
}

if [ "$#" -eq 1 ]; then
  if [ "${TRIAL_DEMO_PLAIN:-0}" = 1 ]; then
    exec "${trial[@]}" observe "$1" --from-the-beginning
  fi
  header 'Stored frames | live Kafka case | replay from the beginning' "trial observe $1 --from-the-beginning"
  "${trial[@]}" observe "$1" --from-the-beginning | paint 0
  exit
fi

# Run the whole deposition first; a failed case must never look like a success.
transcript=$(mktemp)
output=$(mktemp)
if [ "${TRIAL_DEMO_PLAIN:-0}" != 1 ]; then
  header '24 stored frames | verified deposition | paced playback' 'trial test --transcript examples/the-orrery.deposition'
  printf '\033[2mVerifying playback in triallang...\033[0m\n'
fi
status=0
"${trial[@]}" test --transcript examples/the-orrery.deposition > "$transcript" || status=$?
if [ "$status" -ne 0 ]; then
  cat "$transcript" >&2
  exit "$status"
fi
awk '
    /^[[:space:]]+OUTPUT \([0-9]+ entries\):[[:space:]]*$/ { live = 1; next }
    live && /^          / {
      print substr($0, 11)
    }
  ' "$transcript" > "$output"
awk '
  length($0) != 80 { bad = 1 }
  END { if (NR != 24 * 31 || bad) exit 1 }
' "$output" || { echo 'demo: expected 24 complete 80-column Orrery frames' >&2; exit 1; }
if [ "${TRIAL_DEMO_PLAIN:-0}" = 1 ]; then
  cat "$output"
  exit
fi
printf '\033[1A\033[2K'
paint 0.22 < "$output"
printf '\n\033[1;32mDemo complete.\033[0m\n'
