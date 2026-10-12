#!/usr/bin/env bash
#
# Copyright 2026 Joseph Anthony Abbott III
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Smoke test for a built capsus binary: the Capsus CLI contract (intel/plan.md
# D-5) and one non-interactive round trip through each bundled tool, with every
# database in a temporary folder. Used by `make smoke`, ci.yml and cd.yml.
#
# Usage: scripts/smoke.sh <capsus-binary> [expected-version]

set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
	echo "usage: $0 <capsus-binary> [expected-version]" >&2
	exit 2
fi
bin="$1"
expected_version="${2:-}"
if [[ "$bin" != /* && "$bin" != [A-Za-z]:* ]]; then
	bin="$PWD/$bin"
fi
if [[ ! -x "$bin" ]]; then
	echo "not an executable file: $bin" >&2
	exit 2
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
# On Windows (Git Bash), hand native programs a Windows path.
if command -v cygpath >/dev/null 2>&1; then
	tmp="$(cygpath -m "$tmp")"
fi
export MUNUS_DB_PATH="$tmp/munus.db"
export CRYPTARE_DB_PATH="$tmp/cryptare.db"
export SALUS_DB_PATH="$tmp/salus.db"

# Output is captured before grep runs: with pipefail, capsus could otherwise
# fail with SIGPIPE when grep -q exits before capsus has written everything.
expect() { # expect <description> <extended-regex> <text>
	if ! grep -Eq -- "$2" <<<"$3"; then
		printf 'smoke: %s: no line matches %s in:\n%s\n' "$1" "$2" "$3" >&2
		exit 1
	fi
}

# Capsus itself.
out="$("$bin" --version)"
echo "$out"
first="$(head -n 1 <<<"$out")"
if [[ "$first" != "capsus version "* ]]; then
	echo "smoke: capsus --version: first line is not 'capsus version <version>': $first" >&2
	exit 1
fi
if [[ -n "$expected_version" && "$first" != "capsus version $expected_version" ]]; then
	echo "smoke: capsus --version reports '$first', want 'capsus version $expected_version'" >&2
	exit 1
fi
out="$("$bin" list)"
expect "capsus list" '^[Mm]unus[[:space:]].*[^[:alnum:]]TUI([^[:alnum:]]|$)' "$out"
expect "capsus list" '^[Cc]ryptare[[:space:]].*[^[:alnum:]]TUI([^[:alnum:]]|$)' "$out"
expect "capsus list" '^[Ss]alus[[:space:]].*CLI only' "$out"
status=0
"$bin" no-such-tool >/dev/null 2>&1 || status=$?
if ((status == 0)); then
	echo "smoke: capsus accepted an unknown tool" >&2
	exit 1
fi

# Munus: add a task and list it.
out="$("$bin" munus --version)"
expect "capsus munus --version" '^munus version ' "$out"
"$bin" munus add --title smoke --description "capsus smoke test" >/dev/null
out="$("$bin" munus list)"
expect "capsus munus list" 'smoke' "$out"

# Cryptare: password round trip and a stored key. Throwaway passphrase, passed by
# file so it stays out of the arguments.
printf 'capsus-smoke-passphrase\n' >"$tmp/pw.txt"
printf 'capsus smoke test\n' >"$tmp/plain.txt"
"$bin" cryptare encrypt "$tmp/plain.txt" --password-file "$tmp/pw.txt" >/dev/null
"$bin" cryptare decrypt "$tmp/plain.txt.enc" --output "$tmp/plain.out" --password-file "$tmp/pw.txt" >/dev/null
cmp "$tmp/plain.txt" "$tmp/plain.out"
"$bin" cryptare keys generate --password-file "$tmp/pw.txt" >/dev/null
out="$("$bin" cryptare keys list)"
expect "capsus cryptare keys list" 'AES-256-GCM' "$out"

# Salus: list checks, run one, read the stored result, and check that its exit
# codes pass through unchanged. check run exits 0/1/2 for PASS/WARN/FAIL; host
# state varies, so only a code above 1 fails here.
out="$("$bin" salus check list)"
expect "capsus salus check list" 'misconfig' "$out"
status=0
"$bin" salus check run --only misconfig >/dev/null || status=$?
if ((status > 1)); then
	echo "smoke: capsus salus check run exited with $status" >&2
	exit 1
fi
out="$("$bin" salus jobs show 1)"
expect "capsus salus jobs show 1" 'misconfig' "$out"
status=0
"$bin" salus check run --only no-such-check >/dev/null 2>&1 || status=$?
if ((status != 3)); then
	echo "smoke: an unknown Salus check exited with $status, want 3 (Salus' error code)" >&2
	exit 1
fi

echo "Smoke test passed."
