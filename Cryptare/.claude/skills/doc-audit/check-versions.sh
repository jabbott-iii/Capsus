#!/usr/bin/env bash
# Print every place the Go toolchain and the CI tool versions are named, and the
# latest release tag next to SECURITY.md's supported line, so drift is easy to spot.
# Usage, from the repository root: bash .claude/skills/doc-audit/check-versions.sh
set -uo pipefail

root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$root" || exit 2

section() { printf '\n== %s\n' "$1"; }
show() { grep -rnHE "$1" "${@:2}" 2>/dev/null || true; }

section "Go toolchain (go.mod is the source of truth)"
show '^toolchain ' go.mod
show 'golang:[0-9.]+-alpine' Dockerfile .github/workflows
show 'go1\.[0-9]+\.[0-9]+|Go 1\.[0-9]+(\.[0-9]+)?' README.md CONTRIBUTING.md

section "golangci-lint"
show 'GOLANGCI_LINT_VERSION :=' Makefile
show 'version: v[0-9]' .github/workflows/ci.yml
show 'golangci-lint v[0-9]' CONTRIBUTING.md

section "gosec"
show 'GOSEC_VERSION :=' Makefile
show 'securego/gosec@' .github/workflows/security.yml
show 'gosec v[0-9]' CONTRIBUTING.md

section "govulncheck"
show 'GOVULNCHECK_VERSION :=' Makefile
show 'govulncheck@v' .github/workflows/security.yml
show 'govulncheck v[0-9]' CONTRIBUTING.md

section "actionlint"
show 'ACTIONLINT_VERSION :=' Makefile
show 'actionlint v[0-9]' CONTRIBUTING.md

section "Release targets (cd.yml matrix vs licence script)"
show 'goos: [a-z]+, goarch: [a-z0-9]+' .github/workflows/cd.yml
show '^\s+targets=\(' scripts/third-party-licenses.sh

section "Releases"
latest="$(git describe --tags --abbrev=0 2>/dev/null || git ls-remote --tags --refs origin 'v*' 2>/dev/null | sed 's#.*refs/tags/##' | sort -V | tail -n 1)"
echo "latest tag: ${latest:-unknown (shallow clone without tags? try: git fetch --tags)}"
show '^\| *[0-9]+\.[0-9]+\.x' SECURITY.md
show 'changes on .main. since v[0-9.]+' README.md
