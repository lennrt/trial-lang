#!/usr/bin/env bash
# Plays back the-castle deposition at a watchable pace, for the README
# GIF (docs/the-castle.tape). The frames are real: trial test
# --transcript prints exactly what the witness proclaimed. Only the
# pacing is added, because in chambers the whole speedrun takes under
# a second and against a live broker it would take an hour. This
# script is the middle ground the Court declined to provide.
set -euo pipefail
cd "$(dirname "$0")/.."
trial=(./trial)
[ -x "${trial[0]}" ] || trial=(go run ./cmd/trial)
"${trial[@]}" test --transcript examples/the-castle.deposition |
  awk '{print; fflush(); if ($0 == "") system("sleep 0.5"); else system("sleep 0.02")}'
