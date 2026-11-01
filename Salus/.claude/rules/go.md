---
paths:
  - "**/*.go"
---

# Go code in Salus

## Every file

- Keep the Apache-2.0 license header at the top of every `.go` file; copy it
  from an existing file for new ones. A `//go:build` line goes above the
  header.
- Format with `gofmt -s`. golangci-lint runs the linter set pinned in
  `.golangci.yml` (with `run.tests: true`); change that set only when the
  maintainer asks.
- Comment only non-obvious intent, constraints, or trade-offs. Exported
  identifiers get doc comments that start with the name.

## Errors and output

- Wrap errors with context: `fmt.Errorf("...: %w", err)`. Missing records
  wrap `ErrNotFound`; callers test with `errors.Is`, never by matching
  strings.
- Commands write output through `cmd.OutOrStdout()`, never `os.Stdout`
  directly, and check and return the error of every write (tests use a
  failing writer to enforce this). Errors are returned, not printed;
  `runWith` in `pkg/main.go` writes them to stderr.
- Commands return `*ExitStatusError` for a non-zero result. Only `main.go`
  calls `os.Exit`; `pkg.Main` returns the code, because programs that embed
  Salus call it more than once in one process. Keep commands free of
  package-level state: flags bind to variables inside the command
  constructors, and signal handlers stop before the command returns. The
  root command sets `SilenceErrors` and `SilenceUsage`; Cobra must never
  print errors or usage itself, or they land on stdout.
- Group commands use `runGroup` so an unknown subcommand is an error (exit
  `3`), and leaf commands declare `Args`.
- `check run` validates every flag (`validateLimits`, `ValidateCheckKeys`,
  `checkOutputPath`, the format, `--retain`) before it opens the database or
  runs a check. Keep new flag validation in that phase.

## Platform-specific code

- Per-OS files use the `_linux.go`, `_darwin.go`, `_windows.go` suffixes and
  `_other.go` with `//go:build !linux && !darwin && !windows`. Process
  handling splits on `unix` / `!unix` (`health-exec_unix.go`,
  `health-exec_other.go`), WSL drvfs detection on `linux` / `!linux`
  (`health-mounts_linux.go`, `health-mounts_other.go`), and `--output` group
  handling on `!windows` / `windows` (`report-files_unix.go`,
  `report-files_windows.go`). Every platform must define every function the
  check registry references.
- Unexported helpers used only from platform-specific files carry the same
  build constraint, or `unused` fails the lint on the other operating
  systems. Run `make lint`, which lints all three `GOOS` values.
- Pure decoding of platform data lives in `pkg/health-decode.go`, which has
  no build constraint, so its tests run on every OS.
- Resource checks use only the standard library `syscall` package. On
  Windows, call kernel32 through the existing `syscall.NewLazyDLL` handle,
  call `Find()` before `Call`, and never use `Filetime.Nanoseconds` for a
  duration (it converts to Unix time). Adding `golang.org/x/sys` needs the
  maintainer's approval and a `NOTICE` update.

## Concurrency and context

- `RunChecks` runs each check in its own goroutine with a deadline and may
  abandon one that overruns, so checks must not write shared state.
- Pass the check's `ctx` to every external command. Do not add goroutines,
  channels, or locks unless the task needs them; each goroutine needs an
  owner, a cancellation path, and an end.
- Run `go test -race ./...` after touching the runner, `health-exec*.go`,
  signal handling in `logic-cli.go`, or the database.
