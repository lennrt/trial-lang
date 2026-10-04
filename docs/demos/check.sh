#!/usr/bin/env bash
# Check the inputs to the recordings without starting a browser or encoder.
set -euo pipefail
cd "$(dirname "$0")/../.."
bash -n docs/demos/run.sh
bash -n docs/demos/check.sh
bash -n docs/demos/record.sh
for name in the-orrery the-castle the-procession the-recovery; do
  bash -n "docs/$name-demo.sh"
done
names=(the-julia-set the-wave-chamber the-distance-field
       the-cellular-court the-labyrinth the-stack-clerk
       the-orrery the-castle the-procession)
tapes=()
# Recovery needs Kafka, so parse its tape without running its program here.
for name in "${names[@]}" the-recovery; do
  case "$name" in
    the-orrery|the-castle|the-procession|the-recovery) tape="docs/$name.tape" ;;
    *) tape="docs/demos/$name.tape" ;;
  esac
  # VHS v0.11.0 silently skips files it cannot read, including missing files.
  if [ ! -r "$tape" ] || [ ! -s "$tape" ]; then
    echo "missing, unreadable, or empty tape: $tape" >&2
    exit 1
  fi
  tapes+=("$tape")
done
vhs validate "${tapes[@]}"
for name in "${names[@]}"; do
  ./trial test "examples/$name.deposition"
done
