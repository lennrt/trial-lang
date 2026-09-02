#!/usr/bin/env bash
# Demonstrate process-independent recovery against the real Kafka docket.
# The video driver sets TRIAL_RECOVERY_DELAY to pace the otherwise quick steps.
set -euo pipefail

export LC_ALL=C

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

source_file="examples/the-recovery.trial"
demo_delay="${TRIAL_RECOVERY_DELAY:-0}"
temp_parent="${TMPDIR:-/tmp}"
if [[ "$temp_parent" != /* || "$temp_parent" == "/" || ! -d "$temp_parent" ]]; then
  temp_parent="/tmp"
fi
temp_prefix="$temp_parent/triallang-recovery."
temp_dir="$(mktemp -d "${temp_prefix}XXXXXX")"
case_id=""
trial_bin=""
broker_was_running=1
active_pid=""
runner_status=0

run_quietly_with_timeout() {
  local limit="$1"
  shift

  "$@" >/dev/null 2>&1 &
  local child=$!
  (
    sleep "$limit"
    if kill -0 "$child" 2>/dev/null; then
      kill -TERM "$child" 2>/dev/null || true
      sleep 2
      kill -KILL "$child" 2>/dev/null || true
    fi
  ) &
  local watchdog=$!

  local result
  if wait "$child"; then
    result=0
  else
    result=$?
  fi
  kill "$watchdog" 2>/dev/null || true
  wait "$watchdog" 2>/dev/null || true
  return "$result"
}

cleanup() {
  local result=$?
  trap - EXIT INT TERM

  if [[ -n "$active_pid" ]]; then
    kill -INT "$active_pid" 2>/dev/null || true
    for _ in {1..50}; do
      if ! kill -0 "$active_pid" 2>/dev/null; then
        break
      fi
      sleep 0.1
    done
    kill -TERM "$active_pid" 2>/dev/null || true
    for _ in {1..20}; do
      if ! kill -0 "$active_pid" 2>/dev/null; then
        break
      fi
      sleep 0.1
    done
    kill -KILL "$active_pid" 2>/dev/null || true
    wait "$active_pid" 2>/dev/null || true
  fi
  if [[ -n "$trial_bin" && -n "$case_id" ]]; then
    run_quietly_with_timeout 15 "$trial_bin" burn "$case_id" --with-prejudice || true
  fi
  if [[ -n "$trial_bin" && "$broker_was_running" -eq 0 ]]; then
    run_quietly_with_timeout 20 "$trial_bin" dismiss || true
  fi
  if [[ "$temp_dir" == "$temp_prefix"?????? && "$temp_dir" != "/" && -d "$temp_dir" ]]; then
    rm -rf -- "$temp_dir"
  else
    printf 'demo: refusing to remove unexpected temporary path: %s\n' "$temp_dir" >&2
    result=1
  fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

pause() {
  if [[ "$demo_delay" != "0" ]]; then
    sleep "$demo_delay"
  fi
}

prompt() {
  printf '\n\033[1;36m$\033[0m %s\n' "$*"
}

fail() {
  printf 'demo: %s\n' "$*" >&2
  exit 1
}

wait_for_broker() {
  local deadline=$((SECONDS + 60))
  while ((SECONDS < deadline)); do
    if "$trial_bin" docket --json >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_continuance() {
  local deadline=$((SECONDS + 20))
  local status
  while ((SECONDS < deadline)); do
    status="$("$trial_bin" status "$case_id" 2>/dev/null || true)"
    if grep -Fq "Continued until" <<<"$status"; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

wait_for_text() {
  local path="$1"
  local text="$2"
  local deadline=$((SECONDS + 10))
  while ((SECONDS < deadline)); do
    if grep -Fq "$text" "$path" 2>/dev/null; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

wait_for_runner() {
  # A timely exit is success here even when SIGINT determines its status.
  # Callers that require status zero inspect runner_status separately.
  local runner="$1"
  local limit="$2"
  local timeout_marker="$temp_dir/runner-$runner.timeout"
  (
    sleep "$limit"
    if kill -0 "$runner" 2>/dev/null; then
      touch "$timeout_marker"
      kill -TERM "$runner" 2>/dev/null || true
      sleep 2
      kill -KILL "$runner" 2>/dev/null || true
    fi
  ) &
  local watchdog=$!

  local result
  if wait "$runner"; then
    result=0
  else
    result=$?
  fi
  kill "$watchdog" 2>/dev/null || true
  wait "$watchdog" 2>/dev/null || true
  if [[ -e "$timeout_marker" ]]; then
    return 1
  fi
  runner_status="$result"
  return 0
}

if [[ -n "${TRIAL_BIN:-}" ]]; then
  trial_bin="$TRIAL_BIN"
  [[ -x "$trial_bin" ]] || fail "TRIAL_BIN is not executable: $trial_bin"
elif [[ -x ./trial ]]; then
  trial_bin="$repo_root/trial"
else
  trial_bin="$temp_dir/trial"
  go build -o "$trial_bin" ./cmd/trial
fi

command -v docker >/dev/null 2>&1 || fail "Docker is required for the Kafka recovery demo"
docker compose version >/dev/null 2>&1 || fail "Docker Compose is required for the Kafka recovery demo"

if docker compose ps --status running --services 2>/dev/null | grep -Fxq "the-court"; then
  broker_was_running=1
else
  broker_was_running=0
fi

printf '\033[1;35mREAL KAFKA RECOVERY\033[0m\n'
printf 'One durable case, resumed by two processes.\n'
pause

prompt "trial summon"
"$trial_bin" summon
wait_for_broker || fail "Kafka did not become ready within 60 seconds"
pause

prompt "sed -n '1,80p' $source_file"
sed -n '1,80p' "$source_file"
pause

prompt "trial file $source_file --quiet"
case_id="$("$trial_bin" file "$source_file" --quiet)"
[[ "$case_id" =~ ^case-[0-9a-f]{24}$ ]] || fail "filing returned an invalid case number: $case_id"
printf '%s\n' "$case_id"
pause

prompt "trial proceed $case_id"
"$trial_bin" proceed "$case_id" &
first_runner=$!
active_pid="$first_runner"
if ! wait_for_continuance; then
  kill -TERM "$first_runner" 2>/dev/null || true
  wait "$first_runner" 2>/dev/null || true
  active_pid=""
  fail "the case did not record its continuance within 20 seconds"
fi
pause

printf '\n^C  interrupt process %s after the continuance is on file\n' "$first_runner"
kill -INT "$first_runner"
if ! wait_for_runner "$first_runner" 5; then
  active_pid=""
  fail "the first process did not stop cleanly"
fi
active_pid=""
pause

prompt "trial status $case_id"
"$trial_bin" status "$case_id"
pause

printf '\nStarting a new process for the same case.\n'
prompt "trial proceed $case_id"
"$trial_bin" proceed "$case_id" &
second_runner=$!
active_pid="$second_runner"
if ! wait_for_runner "$second_runner" 25; then
  active_pid=""
  fail "the replacement process did not finish within 25 seconds"
fi
active_pid=""
if [[ "$runner_status" -ne 0 ]]; then
  fail "the replacement process exited with status $runner_status"
fi
pause

before="committed before the interruption"
after="committed after the restart"
observe_log="$temp_dir/observe.log"
prompt "trial observe $case_id --from-the-beginning"
"$trial_bin" observe "$case_id" --from-the-beginning >"$observe_log" 2>&1 &
observer=$!
active_pid="$observer"
if ! wait_for_text "$observe_log" "$after"; then
  kill -TERM "$observer" 2>/dev/null || true
  wait "$observer" 2>/dev/null || true
  active_pid=""
  fail "the committed output was not observable within 10 seconds"
fi
kill -INT "$observer"
if ! wait_for_runner "$observer" 5; then
  active_pid=""
  fail "the observer did not stop cleanly"
fi
active_pid=""
cat "$observe_log"

before_count="$(grep -Fxc "$before" "$observe_log" || true)"
after_count="$(grep -Fxc "$after" "$observe_log" || true)"
if [[ "$before_count" != "1" || "$after_count" != "1" ]]; then
  fail "expected each committed line once; found $before_count and $after_count"
fi
printf '\nVerified: both committed lines appear exactly once.\n'
pause

prompt "trial audit $case_id"
"$trial_bin" audit "$case_id"
pause

printf '\nDemo complete. Cleanup now attempts to remove the case and stop any broker started here.\n'
