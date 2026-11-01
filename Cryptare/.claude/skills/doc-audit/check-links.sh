#!/usr/bin/env bash
# Report references in the repository's Markdown files that point at files which don't
# exist:
#   - relative Markdown links and images, ](target);
#   - backticked repository paths ending in .md, .go, .yml, .yaml, .sh or .json
#     (bare names such as `cd.yml` are checked anywhere in the repository).
# Lines between <!-- doc-audit:off --> and <!-- doc-audit:on --> are skipped, for text
# that names missing files on purpose (such as CLAUDE.md's documentation-debt list).
# Usage, from the repository root: bash .claude/skills/doc-audit/check-links.sh
# Prints "file:line: kind: target" per problem; exits 1 if any were found.
set -uo pipefail

root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$root" || exit 2

tracked="$(git ls-files; git ls-files --others --exclude-standard)"
exists_anywhere() { grep -qE "(^|/)$(printf '%s' "$1" | sed 's/[.[\*^$]/\\&/g')\$" <<<"$tracked"; }

report="$(
  printf '%s\n' "$tracked" | grep -E '\.md$' | sort -u | while IFS= read -r file; do
    [ -f "$file" ] || continue
    dir="$(dirname "$file")"
    skip=" $(awk '/<!-- doc-audit:off -->/ { off = 1 } off { printf "%d ", NR } /<!-- doc-audit:on -->/ { off = 0 }' "$file")"

    grep -noE '\]\([^)[:space:]]+' "$file" | while IFS=: read -r line match; do
      case "$skip" in *" $line "*) continue ;; esac
      target="${match#](}"
      target="${target%%#*}"
      case "$target" in '' | http://* | https://* | mailto:*) continue ;; esac
      [ -e "$dir/$target" ] || [ -e "${target#/}" ] || echo "$file:$line: link: $target"
    done

    grep -noE '`[A-Za-z0-9_./-]+\.(md|go|ya?ml|sh|json)`' "$file" | while IFS=: read -r line match; do
      case "$skip" in *" $line "*) continue ;; esac
      target="${match//\`/}"
      case "$target" in
        */*) [ -e "$target" ] || [ -e "$dir/$target" ] || echo "$file:$line: path: $target" ;;
        *) exists_anywhere "$target" || echo "$file:$line: name: $target" ;;
      esac
    done
  done
)"

if [ -n "$report" ]; then
  printf '%s\n' "$report"
  exit 1
fi
echo "No broken references found."
