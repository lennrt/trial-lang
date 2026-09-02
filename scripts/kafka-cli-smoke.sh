#!/usr/bin/env bash

set -Eeuo pipefail

readonly trial_bin="${TRIAL_BIN:-./trial}"
readonly broker="${TRIAL_E2E_BROKER:-localhost:9092}"
readonly command_timeout=90s

die() {
	printf 'kafka CLI smoke test: %s\n' "$*" >&2
	exit 1
}

expect_contains() {
	local output="$1"
	local expected="$2"
	local label="$3"

	if [[ "$output" != *"$expected"* ]]; then
		printf 'kafka CLI smoke test: %s did not contain %q\n' "$label" "$expected" >&2
		printf '%s\n' "$output" >&2
		exit 1
	fi
}

run_trial() {
	timeout --kill-after=5s "$command_timeout" "$trial_bin" "$@"
}

[[ -x "$trial_bin" ]] || die "trial binary is not executable: $trial_bin"
command -v timeout >/dev/null 2>&1 || die "GNU timeout is required"
[[ -n "$broker" ]] || die "TRIAL_E2E_BROKER must not be empty"

export TRIAL_BROKER="$broker"

work_dir="$(mktemp -d)"
program_path="$work_dir/cli-smoke.trial"
case_id=""

cleanup() {
	local status=$?
	local cleanup_output
	local cleanup_status

	trap - EXIT INT TERM
	set +e
	if [[ "$case_id" =~ ^case-[0-9a-f]{24}$ ]]; then
		cleanup_output="$(run_trial burn "$case_id" --with-prejudice 2>&1)"
		cleanup_status=$?
		if ((cleanup_status != 0)); then
			printf 'kafka CLI smoke test: cleanup failed for %s\n%s\n' \
				"$case_id" "$cleanup_output" >&2
			if ((status == 0)); then
				status=$cleanup_status
			fi
		fi
	fi
	rm -f -- "$program_path"
	rmdir -- "$work_dir" 2>/dev/null
	exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cat >"$program_path" <<'TRIAL'
FORM K-1.
IN THE MATTER OF: cli-smoke.

ARTICLE 1.
    LET IT BE RECORDED THAT answer IS 40 PLUS 2.
    PROCLAIM "cli-smoke-output".
    PROCLAIM answer.
    ADJOURN INDEFINITELY.
TRIAL

printf 'Filing through the compiled CLI...\n'
case_id="$(run_trial file "$program_path" --quiet)"
[[ "$case_id" =~ ^case-[0-9a-f]{24}$ ]] || die "file returned an invalid case number: $case_id"

printf 'Proceeding with %s through the compiled CLI...\n' "$case_id"
proceed_output="$(run_trial proceed "$case_id")"
expect_contains "$proceed_output" "Proceeding with $case_id." "proceed output"
expect_contains "$proceed_output" "The case is adjourned indefinitely." "proceed output"
expect_contains "$proceed_output" "The case is adjourned." "proceed output"

printf 'Reading status through the compiled CLI...\n'
status_output="$(run_trial status "$case_id" --json)"
expect_contains "$status_output" "\"case\": \"$case_id\"" "status output"
expect_contains "$status_output" '"started": true' "status output"
expect_contains "$status_output" '"answer": "42"' "status output"
expect_contains "$status_output" '"guilty": false' "status output"

printf 'Auditing through the compiled CLI...\n'
audit_output="$(run_trial audit "$case_id" --json)"
expect_contains "$audit_output" "\"case\": \"$case_id\"" "audit output"
expect_contains "$audit_output" '"consistent": true' "audit output"
expect_contains "$audit_output" '"findings": []' "audit output"

printf 'Burning %s through the compiled CLI...\n' "$case_id"
burned_case="$case_id"
burn_output="$(run_trial burn "$burned_case" --with-prejudice)"
case_id=""
expect_contains "$burn_output" "Deleted case $burned_case. This cannot be undone." "burn output"

if post_burn_output="$(run_trial status "$burned_case" --json 2>&1)"; then
	die "status unexpectedly found burned case $burned_case"
fi
expect_contains "$post_burn_output" "could not be examined" "post-burn status output"

printf 'Kafka CLI smoke test passed for %s.\n' "$burned_case"
