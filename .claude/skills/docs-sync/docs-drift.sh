#!/usr/bin/env bash
# Read-only documentation drift check for Salus. It compares the docs with
# facts that can be read mechanically from the code and configuration, and
# prints one line per problem. Exit codes: 0 no drift, 1 drift found, 2 not
# run inside the repository.
#
# It does not judge prose. Defaults, check semantics, and examples still need
# to be read against the code; see SKILL.md.
#
# Compatible with the bash 3.2 that ships with macOS (no associative arrays).

set -u

root="$(git rev-parse --show-toplevel 2>/dev/null)" || {
  echo "docs-drift: run this inside the Salus repository" >&2
  exit 2
}
cd "$root" || exit 2

found="$(mktemp)" || exit 2
trap 'rm -f "$found"' EXIT

# Every problem is one line in $found, so checks that run in pipelines
# (subshells) can still report.
report() { printf '[%s] %s\n' "$1" "$2" >>"$found"; }

# Strip CR so CRLF files (ci.yml, cd.yml, security.yml, NOTICE) compare cleanly.
nocr() { tr -d '\r'; }

# Lines of a Markdown file outside fenced code blocks.
prose() { nocr <"$1" | awk '/^```/ { fence = !fence; next } !fence'; }

# GitHub-style heading anchors of a Markdown file, one per line: lowercase,
# punctuation other than - and _ dropped, spaces turned into hyphens.
anchors() {
  prose "$1" | sed -n -e 's/^#\{1,6\} \{1,\}//p' | sed -e 's/ *#* *$//' |
    tr '[:upper:]' '[:lower:]' | sed -e 's/[^a-z0-9 _-]//g' -e 's/ /-/g'
}

# Tracked and new (untracked, not ignored) files, so new docs are checked too.
files() { git ls-files --cached --others --exclude-standard -- "$@" | sort -u; }

# 1. References to the retired intel/ documents and AGENTS.md.
git grep --untracked -n -I -E 'intel/|AGENTS\.md' -- . ':!CLAUDE.md' ':!.claude/' 2>/dev/null |
  while IFS= read -r hit; do report retired-ref "$hit"; done

# 2. Relative links and heading anchors in Markdown files.
for md in $(files '*.md'); do
  dir="$(dirname "$md")"
  for target in $(prose "$md" | grep -oE '\]\([^) ]+\)' | sed -e 's/^](//' -e 's/)$//'); do
    case "$target" in http://* | https://* | mailto:*) continue ;; esac
    path="${target%%#*}"
    anchor=""
    case "$target" in *'#'*) anchor="${target#*#}" ;; esac
    file="$md"
    if [ -n "$path" ]; then
      file="$dir/$path"
      [ -e "$file" ] || { report link "$md: ($target) points to a missing file"; continue; }
    fi
    case "$file" in *.md) ;; *) continue ;; esac
    if [ -n "$anchor" ] && ! anchors "$file" | grep -qxF -- "$anchor"; then
      report anchor "$md: ($target) has no matching heading in $file"
    fi
  done
done

# 3. Tool versions quoted in the docs.
go_version="$(grep -m1 -E '^go [0-9]' go.mod | awk '{print $2}')"
for doc in README.md CONTRIBUTING.md; do
  for v in $(grep -oE '\*\*Go [0-9]+\.[0-9]+(\.[0-9]+)?\*\*' "$doc" | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?'); do
    [ "$v" = "$go_version" ] || report version "$doc says Go $v; go.mod says $go_version"
  done
done

lint_version="$(nocr <.github/workflows/ci.yml | grep -m1 -E '^[[:space:]]+version: v[0-9]' | awk '{print $2}')"
for doc in README.md CONTRIBUTING.md Makefile .github/pull_request_template.md; do
  [ -f "$doc" ] || continue
  for v in $(grep -oE 'golangci-lint v[0-9]+\.[0-9]+\.[0-9]+' "$doc" | awk '{print $2}'); do
    [ "$v" = "$lint_version" ] || report version "$doc says golangci-lint $v; ci.yml pins $lint_version"
  done
done

# 4. NOTICE lists exactly the modules that go.mod requires.
gomod_modules="$(awk '
  /^require \(/ { block = 1; next }
  block && /^\)/ { block = 0; next }
  block && NF >= 2 { print $1, $2; next }
  /^require [^(]/ { print $2, $3 }
' go.mod)"
notice_modules="$(nocr <NOTICE | sed -n -e 's/^[[:space:]]*- \([^ ]*\) \(v[^ ]*\).*/\1 \2/p')"
printf '%s\n' "$gomod_modules" | while read -r mod ver; do
  [ -n "$mod" ] || continue
  printf '%s\n' "$notice_modules" | grep -qxF -- "$mod $ver" ||
    report notice "go.mod requires $mod $ver; NOTICE does not list it at that version"
done
printf '%s\n' "$notice_modules" | while read -r mod ver; do
  [ -n "$mod" ] || continue
  printf '%s\n' "$gomod_modules" | grep -qxF -- "$mod $ver" ||
    report notice "NOTICE lists $mod $ver; go.mod does not require it at that version"
done

# 5. Public names in the code that README.md must document.
for key in $(sed -n '/^const (/,/^)/p' pkg/health.go |
  grep -oE '^[[:space:]]+key[A-Za-z]+[[:space:]]+= "[a-z-]+"' | grep -oE '"[a-z-]+"' | tr -d '"'); do
  grep -qF "\`$key\`" README.md || report check-key "README.md does not mention check \`$key\`"
done

for rule in $(sed -n '/^var misconfigRules/,/^}/p' pkg/health.go | grep -oE '\{"[a-z-]+"' | tr -d '{"'); do
  grep -qF "\`$rule\`" README.md || report misconfig "README.md does not mention misconfig rule \`$rule\`"
done

for env in $(git grep -h -oE '"SALUS_[A-Z_]+"' -- 'pkg/*.go' ':!pkg/*_test.go' | tr -d '"' | sort -u); do
  grep -qF "$env" README.md || report env "README.md does not mention $env"
done

declared_flags="$(grep -oE 'Flags\(\)\.[A-Za-z0-9]+\(&[^,]+, "[a-z-]+"' pkg/logic-cli.go |
  grep -oE '"[a-z-]+"$' | tr -d '"' | sort -u)"
for flag in $declared_flags; do
  grep -qE -e "--${flag}([^a-z-]|\$)" README.md || report flag "README.md does not document --$flag"
done
# The other direction: flags named at the start of README bullets
# ("- `--only` — ...") must exist.
for flag in $(grep -E '^- `' README.md | sed 's/ — .*//' | grep -oE -e '--[a-z][a-z-]*' | sed 's/^--//' | sort -u); do
  case "$flag" in help | version) continue ;; esac
  printf '%s\n' "$declared_flags" | grep -qxF -- "$flag" ||
    report flag "README.md documents --$flag, which pkg/logic-cli.go does not declare"
done

# 6. Paths in the README "Project structure" tree exist.
nocr <README.md | awk '
  /^## Project structure/ { section = 1; next }
  section && /^## / { exit }
  section && /^```/ { if (inblock) exit; inblock = 1; next }
  inblock { print }
' | while IFS= read -r line; do
  case "$line" in
    "├── "* | "└── "*) depth=0 ;;
    "│   ├── "* | "│   └── "*) depth=1 ;;
    *) continue ;;
  esac
  # The name is the first word, or a comma-separated list of words:
  # "report-formats.go Nagios, ..." -> "report-formats.go",
  # "LICENSE, NOTICE       Apache-2.0 ..." -> "LICENSE, NOTICE".
  names="$(printf '%s' "$line" | sed -E -e 's/^.*── //' -e 's/^(([^ ,]+, )*[^ ]+).*/\1/')"
  set -f
  old_ifs="$IFS"
  IFS=','
  for name in $names; do
    IFS="$old_ifs"
    name="$(printf '%s' "$name" | sed -e 's/^ *//' -e 's/ *$//')"
    [ -n "$name" ] || continue
    if [ "$depth" = 0 ]; then
      path="$name"
      case "$name" in */) parent="$name" ;; *) parent="" ;; esac
    else
      path="${parent:-}$name"
    fi
    # Entries such as pkg/health*.go are globs, so expand them here.
    set +f
    # shellcheck disable=SC2086 # intentional glob expansion
    set -- $path
    set -f
    [ -e "$1" ] || report structure "README.md project structure lists $path, which does not exist"
  done
  IFS="$old_ifs"
  set +f
done

count="$(wc -l <"$found" | tr -d ' ')"
if [ "$count" -eq 0 ]; then
  echo "docs-drift: no drift found"
  exit 0
fi
cat "$found"
echo "docs-drift: $count problem(s) found"
exit 1
