---
paths:
  - "pkg/health*.go"
  - "pkg/seed.go"
---

# Health checks

## How a check behaves

- A check is a `checkFunc` (`func(context.Context, CheckOptions) CheckOutcome`).
  A check that runs no external tool is written as
  `func(CheckOptions) CheckOutcome` and registered through `withoutContext`.
- A check never panics and never returns an error. It reports through its
  status and message:
  - `StatusWarn` when it cannot run on this host (tool not installed,
    unsupported OS, systemd not running) or found a degraded state.
  - `StatusFail` when the thing checked is unhealthy, unreachable, or the
    user's input for it is invalid.
- Messages are one line and name the target, because text output prints
  only the key. Use `firstLine` when a tool's first line carries the answer,
  `errorLine` for tool errors (it skips Docker `WARNING` lines and kubectl
  klog noise), and `nameList` for lists (at most five names, then a count).
  `RunChecks` passes messages through `sanitizeMessage`; do not bypass it.
- When one number decides the status, attach it with `withValue` and a
  `unit*` constant (`percent`, `seconds`, `days`, `count`). Outcomes that
  measure nothing carry no value.
- The README table "What each check reports" must match the code. Changing
  which conditions give PASS, WARN, or FAIL is a public contract change.

## External tools

- Run tools only through `opts.hasTool` and `opts.command(ctx, ...)`, which
  apply `--timeout`, kill the processes the tool started, and cap output at
  `maxCommandOutput`. Never call `os/exec` directly and never invoke a
  shell.
- Pass arguments as separate strings. Validate every user-supplied value
  before it reaches a tool (`validUnitName`, `validKubeContext`,
  `validNamespace`) and fail the check without running the tool when
  validation fails. Bind values so they cannot become options: put `--`
  before positional values (`systemctl is-active -- <name>`) or use the
  single-argument `--flag=<value>` form (`--context=`, `--namespace=`).
- The runner returns stdout and stderr combined. Parse defensively and skip
  lines that do not match the expected format.

## Files read on the user's behalf

`--cert` files and sshd configuration are opened read-only, only when they
are regular files (a FIFO would block), with a size bound, and their
contents are never echoed: messages carry only paths, subject names, dates,
and fixed keyword values. The `sshd-*` rules report nothing unless they
could read the whole effective configuration.

## Adding a check

1. Add a `key...` constant and append it to the end of `AllCheckKeys` in
   `pkg/health.go`. Never insert it earlier: positions are part of the JSON
   output.
2. Implement it (per-OS files if needed) and register it in `checkRegistry`.
   If it runs once per mount path, service, or file, add a `checkTargets`
   entry with de-duplicated, ordered targets and a `bind` function.
3. Add a `defaultFeatures` entry (and a `defaultCategories` entry if the
   category is new) in `pkg/seed.go`. Without it, `RecordScan` fails with
   `ErrNotFound` for the new key. Seeding is insert-only, so editing an
   existing entry's name or description does not change existing databases.
4. Add tests that use `fakeToolOptions` or fixtures, never the host's tools
   or resource levels.
5. Update `README.md`: features, the "What each check reports" table,
   prerequisites if it needs a tool, platform notes, and an upgrade note,
   because a plain `check run` now runs it and may exit higher than before.

## `misconfig` rules

- A rule is a `misconfigRule` with a stable ID. Append new rules to the end
  of `misconfigRules`; never rename or reuse an ID. Add the rule to the
  README rules table.
- Rules read their inputs from the environment (`HOME`, `PATH`,
  `KUBECONFIG`, `DOCKER_HOST`, `SALUS_DB_PATH`, `SALUS_SSHD_CONFIG`) so tests
  can isolate them with `isolateMisconfigEnv`.
- Rules based on POSIX modes skip Windows and paths where `syntheticModes`
  reports made-up permission bits (WSL drvfs).

## Thresholds and timeouts

Defaults are the `default*` constants in `pkg/health.go` because they are
also flag defaults. Accessors and `orDefault` live in
`pkg/health-thresholds.go`; zero or negative option values fall back to the
defaults. Changing a default is a public contract change.
