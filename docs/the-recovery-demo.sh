#!/usr/bin/env bash
# Demonstrate process-independent recovery against the real Kafka docket.
# The video driver sets TRIAL_RECOVERY_DELAY to pace the otherwise quick steps.
# Set TRIAL_RECOVERY_EXISTING_BROKER=1 and TRIAL_BROKER=host:port to use an
# already-running broker without starting or stopping Docker Compose.
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
recovery_verified=0
existing_broker="${TRIAL_RECOVERY_EXISTING_BROKER:-0}"
interrupt_signal=INT
case "$(uname -s)" in
  MSYS*|MINGW*|CYGWIN*) interrupt_signal=KILL ;;
esac

# Run only in a background shell. Cancel and reap its current sleep as well as
# the watchdog itself, so no timer can keep a caller's capture pipes open.
watch_process() {
  local child="$1" limit="$2" marker="${3:-}"
  watchdog_timer=""
  watchdog_canceled=0
  trap 'if [[ -n "$watchdog_timer" ]]; then
    kill -KILL "$watchdog_timer" 2>/dev/null || true
    wait "$watchdog_timer" 2>/dev/null || true
  fi' EXIT
  # Do not exit between starting sleep and saving $!: finish that assignment,
  # then let EXIT reap the child. During wait, killing it wakes the wait too.
  trap 'watchdog_canceled=1
    if [[ -n "$watchdog_timer" ]]; then
      kill -KILL "$watchdog_timer" 2>/dev/null || true
    fi' INT TERM

  sleep "$limit" &
  watchdog_timer=$!
  if [[ "$watchdog_canceled" -eq 1 ]]; then return 0; fi
  wait "$watchdog_timer" || true
  if [[ "$watchdog_canceled" -eq 1 ]]; then return 0; fi
  watchdog_timer=""
  if kill -0 "$child" 2>/dev/null; then
    if [[ -n "$marker" ]]; then touch "$marker"; fi
    kill -TERM "$child" 2>/dev/null || true
    sleep 2 &
    watchdog_timer=$!
    if [[ "$watchdog_canceled" -eq 1 ]]; then return 0; fi
    wait "$watchdog_timer" || true
    if [[ "$watchdog_canceled" -eq 1 ]]; then return 0; fi
    watchdog_timer=""
    kill -KILL "$child" 2>/dev/null || true
  fi
}

run_quietly_with_timeout() {
  local limit="$1"
  shift

  "$@" >/dev/null 2>&1 &
  local child=$!
  watch_process "$child" "$limit" </dev/null >/dev/null 2>&1 &
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
    kill -"$interrupt_signal" "$active_pid" 2>/dev/null || true
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
    printf '\n\033[2mCleaning up this demo case...\033[0m\n'
    if run_quietly_with_timeout 15 "$trial_bin" burn "$case_id" --with-prejudice; then
      printf 'Cleanup: the demo case was removed.\n'
    else
      printf '\033[33mCleanup: the demo case may remain on the broker.\033[0m\n'
      printf 'Deletion was refused, failed, or exceeded its deadline.\n'
    fi
  fi
  if [[ -n "$trial_bin" && "$broker_was_running" -eq 0 ]]; then
    if run_quietly_with_timeout 20 "$trial_bin" dismiss; then
      printf 'Cleanup: the broker started here was stopped.\n'
    else
      printf '\033[33mCleanup: the broker started here may still be running.\033[0m\n'
    fi
  elif [[ "$existing_broker" == 1 ]]; then
    printf 'Cleanup: the existing broker was left running.\n'
  fi
  if [[ "$temp_dir" == "$temp_prefix"?????? && "$temp_dir" != "/" && -d "$temp_dir" ]]; then
    # These are the only files this script owns. Keep unexpected contents.
    rm -f -- "$temp_dir/trial" "$temp_dir/observe.log" "$temp_dir/audit.log" "$temp_dir"/runner-*.timeout
    if ! rmdir -- "$temp_dir"; then
      printf 'demo: temporary directory was not empty: %s\n' "$temp_dir" >&2
      result=1
    fi
  else
    printf 'demo: refusing to remove unexpected temporary path: %s\n' "$temp_dir" >&2
    result=1
  fi
  printf '\033[0m\033[?25h'
  if [[ "$result" -eq 0 && "$recovery_verified" -eq 1 ]]; then
    printf '\n\033[1;32mRecovery demo complete.\033[0m\n'
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
  printf '\n\033[38;5;109m$ %s\033[0m\n' "$*"
}

scene() {
  printf '\033[?25l\033[2J\033[H\033[1;36mTHE RECOVERY\033[0m\n'
  printf '\033[1;95m%s\033[0m\n' "$1"
  printf '\033[2m%s\033[0m\n' "$2"
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
  # A timely exit is success here even when the requested signal determines it.
  # Callers that require status zero inspect runner_status separately.
  local runner="$1"
  local limit="$2"
  local timeout_marker="$temp_dir/runner-$runner.timeout"
  watch_process "$runner" "$limit" "$timeout_marker" </dev/null >/dev/null 2>&1 &
  local watchdog=$!

  local result
  # The explicit stop message above explains expected signal termination.
  # Suppress Bash's redundant "Killed" notice while retaining the exit status.
  if wait "$runner" 2>/dev/null; then
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

scene 'REAL KAFKA | INDEPENDENT PROCESSES' 'Committed state survives the process that wrote it.'
case "$existing_broker" in
  1)
    [[ -n "${TRIAL_BROKER:-}" ]] || fail "set TRIAL_BROKER when using an existing broker"
    printf '\nUsing the existing Kafka broker at %s.\n' "$TRIAL_BROKER"
    ;;
  0)
    command -v docker >/dev/null 2>&1 || fail "Docker is required unless TRIAL_RECOVERY_EXISTING_BROKER=1"
    docker compose version >/dev/null 2>&1 || fail "Docker Compose is required for the Kafka recovery demo"
    if ! running_services="$(docker compose ps --status running --services)"; then
      fail "could not inspect existing Compose services; refusing to assume the broker is stopped"
    fi
    if ! grep -Fxq "the-court" <<<"$running_services"; then
      broker_was_running=0
    fi
    prompt "trial summon"
    "$trial_bin" summon
    ;;
  *) fail "TRIAL_RECOVERY_EXISTING_BROKER must be 0 or 1" ;;
esac
wait_for_broker || fail "Kafka did not become ready within 60 seconds"
pause

scene '1 / 4 | FILE A DURABLE CASE' 'The program pauses after its first committed line.'
prompt "sed -n '1,80p' $source_file"
sed -n '1,80p' "$source_file"
pause

prompt "trial file $source_file --quiet"
case_id="$("$trial_bin" file "$source_file" --quiet)"
[[ "$case_id" =~ ^case-[0-9a-f]{24}$ ]] || fail "filing returned an invalid case number: $case_id"
printf '%s\n' "$case_id"
pause

scene '2 / 4 | STOP THE FIRST PROCESS' 'Wait for the continuance to be committed to Kafka.'
prompt "trial proceed $case_id"
"$trial_bin" proceed "$case_id" &
first_runner=$!
active_pid="$first_runner"
if ! wait_for_continuance; then
  # Keep the PID for bounded EXIT cleanup, including native Windows processes.
  fail "the case did not record its continuance within 20 seconds"
fi
pause

if [[ "$interrupt_signal" == KILL ]]; then
  printf '\nForce-stop process %s after the continuance is on file.\n' "$first_runner"
  printf 'Native Windows process termination; no graceful shutdown.\n'
else
  printf '\n^C  interrupt process %s after the continuance is on file\n' "$first_runner"
fi
kill -"$interrupt_signal" "$first_runner"
if ! wait_for_runner "$first_runner" 5; then
  active_pid=""
  fail "the first process did not stop within five seconds"
fi
active_pid=""
pause

scene '3 / 4 | RESUME IN A NEW PROCESS' 'The new official reconstructs this same case from Kafka.'
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
scene '4 / 4 | VERIFY THE COMMITTED RECORD' 'Read the output, then replay the stored history for audit.'
prompt "trial observe $case_id --from-the-beginning"
"$trial_bin" observe "$case_id" --from-the-beginning >"$observe_log" 2>&1 &
observer=$!
active_pid="$observer"
if ! wait_for_text "$observe_log" "$after"; then
  # Do not wait without a deadline for a process that may ignore TERM.
  fail "the committed output was not observable within 10 seconds"
fi
kill -"$interrupt_signal" "$observer"
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
printf '\n\033[1;32mVerified: both committed lines appear exactly once.\033[0m\n'
pause

prompt "trial audit $case_id"
audit_log="$temp_dir/audit.log"
"$trial_bin" audit "$case_id" >"$audit_log" 2>&1 &
audit_runner=$!
active_pid="$audit_runner"
audit_deadline=$((SECONDS + 70))
spinner=('|' '/' '-' '\')
spinner_index=0
while kill -0 "$audit_runner" 2>/dev/null; do
  if ((SECONDS >= audit_deadline)); then
    fail "the audit did not finish within 70 seconds"
  fi
  printf '\r  Replaying committed topics %s' "${spinner[spinner_index % ${#spinner[@]}]}"
  spinner_index=$((spinner_index + 1))
  sleep 1
done
printf '\r%*s\r' 36 ''
if wait "$audit_runner"; then
  audit_status=0
else
  audit_status=$?
fi
active_pid=""
cat "$audit_log"
if [[ "$audit_status" -ne 0 ]]; then
  fail "the audit exited with status $audit_status"
fi
pause

recovery_verified=1
# The EXIT trap performs bounded cleanup before publishing the completion marker.
