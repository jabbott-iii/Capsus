---
paths:
  - "**/*_test.go"
---

# Tests

CI runs the suite on Linux, macOS, and Windows runners whose state varies,
so tests must be deterministic and must not depend on the host.

## Isolation

- Never run Docker, kubectl, systemctl, or timedatectl, and never depend on
  the host's disk, memory, load, or uptime. Use the existing seams:
  - `fakeToolOptions` (`pkg/checks_test.go`) for external tools.
  - `parseMeminfo`, `parseLoadAverage`, and `parseUptime` with fixture data
    for `/proc` contents; `pkg/health-decode.go` functions for macOS and
    Windows data.
  - `isolateMisconfigEnv` (`pkg/checks_test.go`) and `isolateHostEnv`
    (`pkg/main_test.go`) for anything that reads `HOME`, `PATH`,
    `KUBECONFIG`, `DOCKER_HOST`, `SALUS_DB_PATH`, or `SALUS_SSHD_CONFIG`.
  - `testCertDER` to generate certificates and `sshdFixture` to write sshd
    configuration trees.
  - `newTestDatabase` and `newSeededTestDatabase` for a file-backed database
    in `t.TempDir()`.
- `registerTestCheck` changes package state; tests that use it must not call
  `t.Parallel`.
- `runExternal` is tested with real `/bin/sh` processes in Unix-only files,
  and signal handling by signalling the test process
  (`pkg/logic-cli_unix_test.go`). Keep such tests behind a `unix` build
  constraint.

## Platform differences

- OS-specific expectations use `runtime.GOOS` with `t.Skip` and a reason.
  A test must never pass silently on a platform where it checks nothing.
- Tests that make files unreadable skip when running as root. Run the suite
  as a normal user.

## Style

- Table-driven tests where inputs share a pattern; `t.Helper()` in helpers;
  `t.TempDir()` for files; `t.Cleanup()` for teardown.
- Commands are tested through `Execute()` with injected writers and
  arguments. `newCheckRunCmdWith` takes the function that runs the checks,
  so tests can assert the `CheckOptions` built from flags. Exit codes are
  tested through `run` and `Main` in `pkg/main_test.go`; `mainOutput`
  captures what `Main` writes to `os.Stdout` and `os.Stderr`.
- Report formats have golden-output tests in `pkg/report-formats_test.go`.
  Change a golden string only when the output change itself was approved;
  never edit one to make a failing test pass.
- Never weaken an assertion, raise a timeout, or add a skip just to get a
  test passing. Find the cause or report it.
- Avoid sleeps for synchronization where a channel, context, or hook works.
