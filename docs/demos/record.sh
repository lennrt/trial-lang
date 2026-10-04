#!/usr/bin/env bash
# Replace a recording only after a fresh VHS output passes a full GIF decode.
set -euo pipefail
cd "$(dirname "$0")/../.."

names=(the-julia-set the-wave-chamber the-distance-field
       the-cellular-court the-labyrinth the-stack-clerk
       the-orrery the-castle the-procession)
if [ "$#" -gt 1 ]; then
  echo 'usage: bash docs/demos/record.sh [the-example-name]' >&2
  exit 2
fi
if [ "$#" -eq 1 ]; then
  case "$1" in
    the-julia-set|the-wave-chamber|the-distance-field|the-cellular-court|the-labyrinth|the-stack-clerk|the-orrery|the-castle|the-procession|the-recovery)
      names=("$1") ;;
    *) echo "unknown demo: $1" >&2; exit 2 ;;
  esac
fi
command -v vhs >/dev/null
command -v ffmpeg >/dev/null

temporary=
cleanup() {
  if [ -n "$temporary" ]; then
    rm -f -- "$temporary/recording.gif"
    rmdir -- "$temporary"
  fi
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

for name in "${names[@]}"; do
  case "$name" in
    the-orrery|the-castle|the-procession|the-recovery) directory=docs ;;
    *) directory=docs/demos ;;
  esac
  tape="$directory/$name.tape"
  if [ ! -r "$tape" ] || [ ! -s "$tape" ]; then
    echo "missing, unreadable, or empty tape: $tape" >&2
    exit 1
  fi
  vhs validate "$tape"
  # Keep the temporary file beside the destination for a same-filesystem move.
  # VHS v0.11.0 can return success after an FFmpeg error, so never let it write
  # directly over the previous recording or mistake that recording for new output.
  temporary=$(mktemp -d "$directory/.record.XXXXXX")
  vhs --output "$temporary/recording.gif" "$tape"
  if [ ! -s "$temporary/recording.gif" ]; then
    echo "VHS produced no recording for $name" >&2
    exit 1
  fi
  if ! ffmpeg -nostdin -v error -xerror -i "$temporary/recording.gif" -f null -; then
    echo "recording failed to decode: $name" >&2
    exit 1
  fi
  mv -f -- "$temporary/recording.gif" "$directory/$name.gif"
  rmdir -- "$temporary"
  temporary=
  printf 'Verified recording: %s/%s.gif\n' "$directory" "$name"
done
