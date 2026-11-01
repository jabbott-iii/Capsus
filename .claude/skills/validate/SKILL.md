---
name: validate
description: Run the Salus validation suite that CI enforces (gofmt, go vet, tests, race detector, golangci-lint for Linux, macOS, and Windows, module drift, build and smoke test) and report each result. Use before calling a Go, module, or workflow change done.
argument-hint: "[quick|full]"
allowed-tools:
  - Bash(go version)
  - Bash(go env CC)
  - Bash(gofmt -s -l *)
  - Bash(go vet *)
  - Bash(go test *)
  - Bash(go mod tidy -diff)
  - Bash(make build)
  - Bash(make lint)
  - Bash(golangci-lint --version)
  - Bash(git status *)
  - Bash(git diff *)
---

# Validate

Mode: `$ARGUMENTS`

- `quick`: steps 1 to 4.
- Empty: steps 1 to 7, with step 5 only when the change touches the check
  runner (`pkg/health.go`), `pkg/health-exec*.go`, signal handling in
  `pkg/logic-cli.go`, or the database files.
- `full`: every step.

Run from the repository root. Never edit code to make a step pass without
saying so, and never weaken a test, lint rule, or check. Do not commit.

## 1. Preflight

- Read the `go` directive in `go.mod` and compare it with `go version`.
  With `GOTOOLCHAIN=auto`, Go downloads the required toolchain; if that
  fails (offline or blocked), stop and report that the Go steps could not
  run.
- Confirm a C compiler is available (`go env CC`, then that compiler with
  `--version`). Without one, tests that open a database and the build cannot
  run; say which steps that affects.
- Compare `golangci-lint --version` with the `version:` input in
  `.github/workflows/ci.yml`. A different version can report different
  findings; say so in the report.

## 2. Formatting

`gofmt -s -l .` must print nothing. If it lists files this change touched,
format them with `gofmt -s -w <file>`. If it lists files this change did not
touch, report them and leave them alone.

## 3. Vet

`go vet ./...`

## 4. Tests

`go test -count=1 ./...`. Run as a normal user: tests that make files
unreadable skip under root.

## 5. Race detector

`go test -race -count=1 ./...` (needs CGO).

## 6. Lint

`make lint`, which runs golangci-lint with `GOOS=linux`, `GOOS=darwin`, and
`GOOS=windows`. A failure for one `GOOS` only usually means an unexported
helper is missing the build constraint of the files that use it.

## 7. Module drift, build, and smoke test

- `go mod tidy -diff` must print nothing (CI runs `go mod tidy` and fails on
  any diff).
- `make build`, then mirror the CI smoke test with a throwaway database:

  ```bash
  db_dir="$(mktemp -d)"
  ./salus --version
  SALUS_DB_PATH="$db_dir/salus.db" ./salus check list
  SALUS_DB_PATH="$db_dir/salus.db" ./salus check run --only misconfig; echo "exit=$?"
  SALUS_DB_PATH="$db_dir/salus.db" ./salus jobs show 1
  ```

  `check run` may exit `0` or `1` depending on the host; anything above `1`
  is a failure. `jobs show 1` must list `misconfig`.

## Report

A table with each step, the exact command, and `pass`, `fail` (with the
first relevant error lines), or `not run` (with the reason). Then note what
CI covers that this run did not: macOS and Windows test runs, CodeQL,
gosec, govulncheck, the Docker image smoke test, and, for `cd.yml` changes,
a manual CD run.
