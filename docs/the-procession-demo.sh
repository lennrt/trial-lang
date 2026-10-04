#!/usr/bin/env bash
# Replay a freshly checked deposition, or follow an optional live case number.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ "$#" -gt 1 ]; then
  echo "usage: bash docs/the-procession-demo.sh [case-number]" >&2
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

animate() {
  awk -v delay="$1" -v live="$2" -v case_id="${3:-}" '
    function draw(    i, ch) {
      printf "\033[H\033[J\033[1;36mTHE PROCESSION\033[0m\n"
      if (live) {
        printf "Live case output | replaying history, then following new frames\n\n"
        printf "\033[38;5;109m$ ./trial observe %s --from-the-beginning\033[0m\n\n", case_id
      } else {
        printf "Accelerated playback | fresh deposition | 22 queued ticks\n\n"
        printf "\033[38;5;109m$ ./trial test --transcript examples/the-procession.deposition\033[0m\n\n"
      }
      printf "\033[2m%s\033[0m\n", frame[1]
      printf "\033[1;95m%s\033[0m\n", frame[2]
      for (i = 1; i <= length(frame[3]); i++) {
        ch = substr(frame[3], i, 1)
        printf "\033[%sm%s", ch == "K" ? "1;95" : "36", ch
      }
      printf "\033[0m\n\033[1;96m%s\033[0m\n", frame[4]
      printf "\033[38;5;109m%s\033[0m\n", frame[5]
      fflush()
      if (delay > 0 && system("sleep " delay) != 0) exit 1
      lines = 0
      frames++
    }
    {
      frame[++lines] = $0
      if (lines == 5) draw()
    }
    END { if (lines != 0 || frames == 0) exit 1 }
  '
}

if [ "$#" -eq 1 ]; then
  if [ "${TRIAL_DEMO_PLAIN:-0}" = 1 ]; then
    exec "${trial[@]}" observe "$1" --from-the-beginning
  fi
  trap 'printf "\033[0m\033[?25h"' EXIT
  printf '\033[?25l\033[2J\033[H'
  "${trial[@]}" observe "$1" --from-the-beginning | animate 0 1 "$1"
  exit
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
  printf '\033[?25l\033[2J\033[H\033[1;36mTHE PROCESSION\033[0m\n'
  printf 'Accelerated playback | fresh deposition | 22 queued ticks\n\n'
  printf '\033[38;5;109m$ ./trial test --transcript examples/the-procession.deposition\033[0m\n\n'
  printf '\033[2mComputing and checking every frame in triallang...\033[0m\n'
fi

status=0
"${trial[@]}" test --transcript examples/the-procession.deposition > "$work/transcript" || status=$?
if [ "$status" -ne 0 ]; then
  cat "$work/transcript" >&2
  exit "$status"
fi
if ! awk '
  /^          OUTPUT \(23 entries\):[[:space:]]*$/ { active = 1; headers++; next }
  active && /^          / { print substr($0, 11); lines++; next }
  active && /^[^[:space:]]/ { active = 0 }
  END { if (headers != 1 || lines != 115) exit 1 }
' "$work/transcript" > "$work/frames"; then
  echo 'Procession transcript did not contain twenty-three five-line frames.' >&2
  exit 1
fi
if [ "${TRIAL_DEMO_PLAIN:-0}" = 1 ]; then
  cat "$work/frames"
  exit
fi

animate 0.45 0 < "$work/frames"
printf '\n\033[1;32mDemo complete.\033[0m\n'
