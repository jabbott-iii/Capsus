---
paths:
  - "**/*_test.go"
---

# Tests

- Tests sit beside the code in `pkg/` and write only under `t.TempDir()`. Use
  `t.Helper()` in helpers and `t.Cleanup()` for teardown.
- A behaviour change gets a test; a bug fix gets a regression test that fails before
  the fix. Security-sensitive fixes always do.
- Never weaken an assertion, skip a test or change expected values to get green.
  Golden and known-answer tests in particular pin data users already have.
- CLI tests run `NewRootCmd(db)` with `SetArgs`, `SetIn` and `SetOut`
  (`logic_cli_test.go`). TUI tests drive `DashboardModel.Update` and `updateForm` with
  synthetic `tea.KeyMsg` values (`logic_tui_test.go`). A user-facing change needs both.
- Database tests use `newTestDatabase(t, useFile)` and close their handles (Windows).
- `TestMain` (`legacy_fixtures_test.go`) sets a cheap Argon2id setting for the whole
  package so the suite stays fast; `TestDefaultPasswordKDFIsWritten` checks the real
  default. Don't change either to speed up a test.
- Behaviour only a real process shows (exit status after a signal, stdout written by a
  library) is tested as a subprocess through `TestRunMain` (`interrupt_test.go`).
- `Main`'s own tests (`run_test.go`; their `TestMain…` names refer to `Main`, not to the
  package's `TestMain`) call it in-process and read what it writes with `captureOutput`,
  which swaps `os.Stdout` and `os.Stderr` while it runs, so never use it in a parallel
  test.
- Cancellation logic is tested with `cancelAfter`, not timing. Signal tests may pace
  their input with short sleeps, but their outcome must not depend on timing. Unix-only
  tests go in `*_unix_test.go`.
- CI runs `go test -race` on Linux and macOS and plain `go test` on Windows; keep tests
  deterministic and portable (paths, line endings, permissions).
