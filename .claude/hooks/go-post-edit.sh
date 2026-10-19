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
# Claude Code PostToolUse hook (Edit|Write|MultiEdit, .claude/settings.json): after
# one of the launcher's Go files changes, format it with gofmt -s (as `make fmt`
# does) and check that it starts with the Apache-2.0 license header. Files in the
# tool folders (top-level folders with their own go.mod) are skipped: they are
# formatted upstream, and the settings deny edits there.
#
# Exit 2 shows stderr to Claude (the edit has already happened); 0 is silent.
set -u

input="$(cat)"
if command -v jq >/dev/null 2>&1; then
	path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
elif command -v python3 >/dev/null 2>&1; then
	path="$(printf '%s' "$input" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))' 2>/dev/null)"
else
	exit 0
fi
[[ -n "${path:-}" ]] || exit 0
path="${path//\\//}" # Windows separators
[[ "$path" == *.go && -f "$path" ]] || exit 0

root="${CLAUDE_PROJECT_DIR:-$PWD}"
root="${root//\\//}"
rel="${path#"$root"/}"
[[ "$rel" != "$path" ]] || exit 0 # outside the project
if [[ "$rel" == */* && -f "$root/${rel%%/*}/go.mod" ]]; then
	exit 0 # inside an imported tool's module
fi

problems=""
if command -v gofmt >/dev/null 2>&1; then
	if ! out="$(gofmt -s -w "$path" 2>&1)"; then
		problems+="gofmt -s could not format $rel:"$'\n'"$out"$'\n'
	fi
else
	problems+="gofmt not found: run 'make fmt' before reporting this change done."$'\n'
fi

# The header follows a `//go:build` line and a blank line when the file has one.
if ! head -n 8 "$path" | grep -q 'Licensed under the Apache License, Version 2.0'; then
	problems+="$rel does not start with the Apache-2.0 license header; copy the block from a tool's main.go (.claude/rules/go.md)."$'\n'
fi

if [[ -n "$problems" ]]; then
	printf '%s' "$problems" >&2
	exit 2
fi
exit 0
