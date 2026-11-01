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

# Writes the licence texts of the third-party code in Cryptare's release
# binaries: the Go standard library, every module that `go list -deps` reports
# for a release target, and the C libraries the release toolchains link in
# (c_libraries below, Q-013). The release workflow (cd.yml) puts the result into
# each archive next to LICENSE and NOTICE (Q-006).
#
# Usage: scripts/third-party-licenses.sh [output [goos/goarch ...]]
#   output   defaults to THIRD_PARTY_LICENSES.txt
#   targets  default to the release matrix in .github/workflows/cd.yml;
#            keep the two lists in step
#
# Run it from the repository root with the release toolchain, since the
# standard library's licence comes from `go env GOROOT`. It needs network
# access the first time, as `go list` downloads modules it hasn't seen.
#
# It fails when a module ships no licence file and isn't listed in
# stated_licence below, so a new dependency is looked at before it is released.

set -euo pipefail

out="${1:-THIRD_PARTY_LICENSES.txt}"
if [ "$#" -gt 0 ]; then shift; fi
targets=("$@")
if [ "${#targets[@]}" -eq 0 ]; then
  targets=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64)
fi

# Modules that ship no licence file, and where their licence is stated.
stated_licence() {
  case "$1" in
    github.com/mattn/go-localereader)
      echo "MIT, as stated under \"License\" in the module's README.md. The module ships no licence text." ;;
    *) return 1 ;;
  esac
}

# Notes on code that a module bundles under different terms.
module_note() {
  case "$1" in
    github.com/mattn/go-sqlite3)
      echo "This module includes the SQLite library (sqlite3-binding.c), which is in the public domain: https://www.sqlite.org/copyright.html" ;;
  esac
}

# C code that the release toolchains link into the binaries (Q-013), one line each:
# the GOOS it is linked into, its name, and its licence file under licenses/.
#   - Linux binaries are linked statically against musl in cd.yml's Alpine builder.
#     Licence text from musl 1.2.5 (the kraj/musl mirror of git.musl-libc.org).
#   - Windows binaries contain parts of the MinGW-w64 runtime (libmingwex,
#     libmingw32 and start-up code). Licence text from mingw-w64 v12.0.0.
# libgcc, linked into both, is under the GCC Runtime Library Exception, which asks
# for no notice. macOS binaries use Apple's system libraries, which they don't
# contain. Update this list, and the files, when the toolchains change.
c_libraries() {
  printf '%s\t%s\t%s\n' \
    linux "musl libc" musl-COPYRIGHT.txt \
    windows "MinGW-w64 runtime" mingw-w64-runtime.txt
}
licenses_dir="$(cd "$(dirname "$0")" && pwd)/licenses"

rule() { printf '%80s\n' '' | tr ' ' "$1"; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# One line per module and target: path, version, source folder, target.
# go list doesn't run the C compiler, so every target can be listed from one machine.
for target in "${targets[@]}"; do
  GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=1 go list -deps \
    -f '{{with .Module}}{{if not .Main}}{{.Path}}{{"\t"}}{{.Version}}{{"\t"}}{{if .Replace}}{{.Replace.Dir}}{{else}}{{.Dir}}{{end}}{{end}}{{end}}' . |
    awk -F '\t' -v t="$target" 'NF {
      if ($2 == "" || $3 == "") { print "third-party-licenses: no version or source folder for " $1 > "/dev/stderr"; exit 1 }
      print $0 "\t" t }' >> "$work/deps"
done

# One line per module: path, version, folder, and the targets that link it
# ("all" when every target does).
LC_ALL=C sort -u "$work/deps" |
  awk -F '\t' -v n="${#targets[@]}" '
    { k = $1 FS $2 FS $3
      if (!(k in where)) { order[++m] = k; where[k] = $4 } else { where[k] = where[k] ", " $4 }
      count[k]++ }
    END { for (i = 1; i <= m; i++) { k = order[i]; print k FS (count[k] == n ? "all" : where[k]) } }' |
  LC_ALL=C sort > "$work/modules"

# One line per C library that a target links: name, licence file, and the targets.
c_libraries | while IFS=$'\t' read -r goos name file; do
  where=""
  for target in "${targets[@]}"; do
    if [ "${target%/*}" = "$goos" ]; then where="${where:+$where, }$target"; fi
  done
  if [ -n "$where" ]; then
    if [ ! -f "$licenses_dir/$file" ]; then
      echo "third-party-licenses: missing $licenses_dir/$file" >&2
      exit 1
    fi
    printf '%s\t%s\t%s\n' "$name" "$file" "$where"
  fi
done > "$work/clibs"

goroot="$(go env GOROOT)"
goversion="$(go env GOVERSION)"
if [ ! -f "$goroot/LICENSE" ]; then
  echo "third-party-licenses: no LICENSE in GOROOT ($goroot)" >&2
  exit 1
fi

missing=0
{
  echo "Third-party software in Cryptare"
  echo
  echo "Cryptare is licensed under the Apache License, Version 2.0; see LICENSE and"
  echo "NOTICE. Its release binaries also contain the Go standard library, the Go"
  echo "modules and the C libraries listed below, each under its own licence, whose text"
  echo "follows the list. libgcc, also linked in, is under the GCC Runtime Library"
  echo "Exception, which needs no notice; macOS binaries use Apple's system libraries"
  echo "without containing them."
  echo
  echo "Generated by scripts/third-party-licenses.sh with ${goversion} for"
  echo "${targets[*]}."
  echo "\"Linked into\" names the binaries that contain a component, when not all do."
  echo
  printf '  %-50s %s\n' "Go standard library" "$goversion"
  while IFS=$'\t' read -r path version _ where; do
    if [ "$where" = all ]; then
      printf '  %-50s %s\n' "$path" "$version"
    else
      printf '  %-50s %s (%s)\n' "$path" "$version" "$where"
    fi
  done < "$work/modules"
  while IFS=$'\t' read -r name _ where; do
    printf '  %-50s (%s)\n' "$name (C)" "$where"
  done < "$work/clibs"
  echo

  rule '='
  echo "Go standard library ${goversion}"
  rule '-'
  cat "$goroot/LICENSE"
  echo

  while IFS=$'\t' read -r path version dir where; do
    rule '='
    echo "$path $version"
    if [ "$where" != all ]; then echo "Linked into: $where"; fi
    note="$(module_note "$path")"
    if [ -n "$note" ]; then echo "$note"; fi
    rule '-'
    files="$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'LICENCE*' \
      -o -iname 'COPYING*' -o -iname 'NOTICE*' \) | LC_ALL=C sort)"
    if [ -n "$files" ]; then
      while IFS= read -r f; do
        echo "--- $(basename "$f")"
        cat "$f"
        # Some licence files lack a final newline.
        if [ -n "$(tail -c 1 "$f")" ]; then echo; fi
      done <<< "$files"
    elif stated="$(stated_licence "$path")"; then
      echo "$stated"
    else
      echo "third-party-licenses: $path $version has no licence file in $dir;" \
        "check its licence and add it to stated_licence" >&2
      missing=1
    fi
    echo
  done < "$work/modules"

  while IFS=$'\t' read -r name file where; do
    rule '='
    echo "$name (C library)"
    echo "Linked into: $where"
    rule '-'
    cat "$licenses_dir/$file"
    if [ -n "$(tail -c 1 "$licenses_dir/$file")" ]; then echo; fi
    echo
  done < "$work/clibs"
} > "$out"

if [ "$missing" -ne 0 ]; then
  rm -f "$out"
  exit 1
fi
echo "third-party-licenses: wrote $out ($(wc -l < "$work/modules" | tr -d ' ') modules," \
  "$(wc -l < "$work/clibs" | tr -d ' ') C libraries)" >&2
