#!/usr/bin/env bash
# Execute the deposition, then replay its actual proclamations in place.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ "$#" -ne 0 ]; then
  echo "usage: bash docs/the-castle-demo.sh" >&2
  exit 2
fi
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

work=$(mktemp -d)
styled=0
cleanup() {
  rm -f "$work/transcript" "$work/frames"
  rmdir "$work"
  if [ "$styled" -eq 1 ]; then printf '\033[0m\033[?25h'; fi
}
trap cleanup EXIT
if [ "${TRIAL_DEMO_PLAIN:-0}" != 1 ]; then
  styled=1
  printf '\033[?25l\033[2J\033[H\033[1;36mTHE CASTLE\033[0m\n'
  printf 'First-person raycasting | 15 frames | paced deposition playback\n\n'
  printf '\033[38;5;109m$ ./trial test --transcript examples/the-castle.deposition\033[0m\n\n'
  printf '\033[2mComputing and checking every frame in triallang...\033[0m\n'
fi

# Save a fresh, complete run before playback. A failed deposition must never
# become a successful animation because an output-processing command succeeded.
status=0
"${trial[@]}" test --transcript examples/the-castle.deposition > "$work/transcript" || status=$?
if [ "$status" -ne 0 ]; then
  cat "$work/transcript" >&2
  exit "$status"
fi
if ! awk '
  /^          OUTPUT \(16 entries\):[[:space:]]*$/ { active = 1; headers++; next }
  active && /^          / { print substr($0, 11); lines++; next }
  active && /^[^[:space:]]/ { active = 0 }
  END { if (headers != 1 || lines != 121) exit 1 }
' "$work/transcript" > "$work/frames"; then
  echo 'Castle transcript did not contain fifteen eight-line frames and one closing line.' >&2
  exit 1
fi
if [ "${TRIAL_DEMO_PLAIN:-0}" = 1 ]; then
  cat "$work/frames"
  exit
fi

awk '
  function paint(line,    i, ch, color) {
    for (i = 1; i <= length(line); i++) {
      ch = substr(line, i, 1)
      color = "38;5;109"
      if (ch == "%" || ch == "+") color = "1;95"
      else if (ch ~ /[@#=]/) color = "1;96"
      else if (ch == "-" || ch == ":") color = "36"
      printf "\033[%sm%s", color, ch
    }
    printf "\033[0m\n"
  }
  NR <= 120 {
    row = (NR - 1) % 8
    if (row == 0) {
      frame++
      printf "\033[H\033[J\033[1;36mTHE CASTLE\033[0m\n"
      printf "First-person raycasting | 15 frames | paced deposition playback\n\n"
      printf "\033[38;5;109m$ ./trial test --transcript examples/the-castle.deposition\033[0m\n\n"
      printf "\033[1;95mFrame %d / 15\033[0m\n\n", frame
    }
    if (row < 7) paint($0)
    else {
      printf "\n\033[1;92m%s\033[0m\n", $0
      fflush()
      if (system("sleep 0.7") != 0) exit 1
    }
    next
  }
  { printf "\n%s\n", $0 }
' "$work/frames"
printf '\n\033[1;32mDemo complete.\033[0m\n'
