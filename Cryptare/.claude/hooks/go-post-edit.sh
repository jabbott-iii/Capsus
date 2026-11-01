#!/usr/bin/env bash
# PostToolUse hook (Edit|Write): after a .go file changes, format it with gofmt -s (as
# `make fmt` would) and check that it keeps the Apache-2.0 licence header.
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
[ -n "${path:-}" ] || exit 0
case "$path" in *.go) ;; *) exit 0 ;; esac
[ -f "$path" ] || exit 0

problems=""
if command -v gofmt >/dev/null 2>&1; then
  if ! out="$(gofmt -s -w "$path" 2>&1)"; then
    problems+="gofmt -s could not format $path:"$'\n'"$out"$'\n'
  fi
else
  problems+="gofmt not found: run 'make fmt' before reporting this change done."$'\n'
fi

if ! head -n 6 "$path" | grep -q 'Licensed under the Apache License, Version 2.0'; then
  problems+="$path has no Apache-2.0 licence header at the top; copy the block from main.go."$'\n'
fi

if [ -n "$problems" ]; then
  printf '%s' "$problems" >&2
  exit 2
fi
exit 0
