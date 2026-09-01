#!/usr/bin/env bash
# Plays examples/the-castle.deposition at a watchable pace for
# docs/the-castle.tape. `trial test --transcript` produces the output; this
# script only adds timing.
set -euo pipefail
cd "$(dirname "$0")/.."
trial=(./trial)
[ -x "${trial[0]}" ] || trial=(go run ./cmd/trial)
"${trial[@]}" test --transcript examples/the-castle.deposition |
  awk '{print; fflush(); if ($0 == "") system("sleep 0.5"); else system("sleep 0.02")}'
