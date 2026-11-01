#!/usr/bin/env bash
# PreToolUse hook (Edit|Write): refuse changes to golden fixtures that are already
# tracked. They were written by released binaries and pin the on-disk formats; editing
# one hides a compatibility break instead of catching it. New fixtures (untracked files)
# and the folder's README.md are allowed; the permission rules still ask before any
# write under pkg/testdata/golden/.
#
# Exit 2 blocks the tool call and shows stderr to Claude. Anything else lets it run.
set -u

input="$(cat)"
if command -v jq >/dev/null 2>&1; then
  path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
elif command -v python3 >/dev/null 2>&1; then
  path="$(printf '%s' "$input" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))' 2>/dev/null)"
else
  echo "guard-golden: neither jq nor python3 found; golden fixtures are not guarded." >&2
  exit 0
fi
[ -n "${path:-}" ] || exit 0

path="${path//\\//}" # Windows separators
case "$path" in
  pkg/testdata/golden/* | */pkg/testdata/golden/*) ;;
  *) exit 0 ;;
esac
rel="pkg/testdata/golden/${path#*pkg/testdata/golden/}"
[ "$rel" = "pkg/testdata/golden/README.md" ] && exit 0

root="${CLAUDE_PROJECT_DIR:-.}"
if git -C "$root" ls-files --error-unmatch -- "$rel" >/dev/null 2>&1; then
  cat >&2 <<EOF
Blocked: $rel is a golden fixture written by a released Cryptare binary.
Never edit, regenerate or replace a fixture to make a test pass: a failing golden test
means files users already have would stop opening. Fix the code instead, or, for a new
format version, add a new fixture file and a README row (see /format-change).
EOF
  exit 2
fi
exit 0
