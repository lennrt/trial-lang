#!/usr/bin/env bash
# Plays examples/the-castle.deposition at a watchable pace for
# docs/the-castle.tape. `trial test --transcript` produces the frames; this
# script only adds timing. The in-memory run finishes in under a second, while
# the live-broker run takes about an hour.
set -euo pipefail
cd "$(dirname "$0")/.."
trial=(./trial)
[ -x "${trial[0]}" ] || trial=(go run ./cmd/trial)
"${trial[@]}" test --transcript examples/the-castle.deposition |
  awk '{print; fflush(); if ($0 == "") system("sleep 0.5"); else system("sleep 0.02")}'
