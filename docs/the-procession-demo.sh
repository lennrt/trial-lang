#!/usr/bin/env bash
# With no argument, replay the deposition as an accelerated in-place animation.
# With a case number, follow the one-frame-per-day case.
set -euo pipefail
cd "$(dirname "$0")/.."

trial=(./trial)
[ -x "${trial[0]}" ] || trial=(go run ./cmd/trial)

animate() {
  awk -v delay="$1" '
    NF {
      frame[++lines] = $0
      if (lines == 5) {
        printf "\033[2J\033[H"
        for (i = 1; i <= lines; i++) print frame[i]
        fflush()
        if (delay > 0) system("sleep " delay)
        lines = 0
      }
    }
  '
}

if [ "$#" -gt 1 ]; then
  echo "usage: $0 [case-number]" >&2
  exit 2
fi

if [ "$#" -eq 1 ]; then
  "${trial[@]}" observe "$1" --from-the-beginning | animate 0
  exit
fi

# The deposition queues explicit ticks so CI and this recording need not wait.
"${trial[@]}" test --transcript examples/the-procession.deposition |
  awk '
    /^[[:space:]]+OUTPUT \([0-9]+ entries\):[[:space:]]*$/ { live = 1; next }
    live && /^          / {
      line = substr($0, 11)
      if (line != "") print line
      next
    }
    live && /^[^[:space:]]/ { exit }
  ' | animate 0.45
