# CLAUDE.md

Instructions for Claude Code in the Salus repository. This file and the
path-scoped rules in `.claude/rules/` are the maintainer guidance.
`README.md` is the user documentation and `CONTRIBUTING.md` the contributor
guide; keep all three consistent.

The `intel/` documents and `AGENTS.md` were retired in commit `8911853`. Do
not recreate them. Comments still cite their tracker IDs (`SEC-nnn` security
items, `Q-nnn` decisions, `Pn-n` plan items); when you need the reasoning
behind one, read it from history, for example
`git show 8911853~1:intel/cybersec.md`.

## What Salus is

A single-binary Go CLI that runs local host health checks (disk space and
inodes, memory, CPU load, Docker, Kubernetes nodes and pods, service uptime,
failed systemd units, time sync, certificate expiry, misconfigurations),
reports PASS/WARN/FAIL as text, JSON, Nagios, Prometheus, or JUnit, and
records each run in a local SQLite database. It has no network listener, no
HTTP API, and no configuration file: every setting is a flag or an
environment variable. Users run it from cron, CI, and monitoring systems, so
its output and exit codes are consumed by scripts.

## Commands

Run from the repository root.

| Task | Command |
|---|---|
| Build | `make build` (`CGO_ENABLED=1 go build -o salus .`) |
| Test | `make test` (`go test ./...`) |
| Race detector | `go test -race ./...` (concurrency, runner, signal, or database changes) |
| Vet | `make vet` |
| Lint | `make lint` (golangci-lint for `GOOS=linux`, `darwin`, and `windows`) |
| Format | `make fmt` (`gofmt -s -w .`) |
| Module drift | `go mod tidy -diff` (CI runs `go mod tidy && git diff --exit-code`) |
| Coverage | `make cover` |
| Try the CLI | `SALUS_DB_PATH="$(mktemp -d)/salus.db" ./salus check run --only misconfig` |

- The Go version is the `go` directive in `go.mod`; the golangci-lint
  version is the `version:` input in `.github/workflows/ci.yml`. Read them
  there rather than trusting a number in prose.
- CGO is required (`github.com/mattn/go-sqlite3`). A `CGO_ENABLED=0` build
  compiles but cannot open a database, so never "fix" a build by disabling
  CGO. Cross-OS linting needs no C toolchain.
- Always set `SALUS_DB_PATH` to a temporary file when running the binary, so
  you never touch the maintainer's real job history.
- `/validate` runs the full pre-PR suite; `/docs-sync` checks documentation
  against the code.

## Layout

- `main.go`: entry point. Holds `var version` (set by
  `-ldflags "-X main.version=..."` in CD and the `Dockerfile`) and exits
  with the code `pkg.Main` returns. The only place that calls `os.Exit`.
- `pkg/`: everything else, one package named `pkg`:
  - `main.go`: `Main`, which runs the CLI for the binary and for programs
    that embed Salus. Opens the database lazily and closes it, maps errors
    and panics to exit codes. `main_test.go` tests the whole CLI through
    `run` and `Main`.
  - `logic-cli.go`: Cobra commands and flag validation. `version.go`: root
    command with the version attached.
  - `health.go`: check keys, registry, per-target expansion, the runner,
    external command execution, `misconfig` rules. `health-*.go`: individual
    checks and per-OS implementations.
  - `report.go`, `report-formats.go`, `report-files_*.go`: output formats,
    exit codes, atomic `--output` files.
  - `database*.go`, `scan-store.go`, `scan-history.go`, `seed.go`:
    persistence, job history, `jobs diff` and `jobs stats`, the check
    catalog.
- Dependency direction is CLI → checks, reporting, persistence. Check and
  report code never touches the database.

## Public contracts

Scripts, dashboards, alert rules, and existing databases depend on these.
Change one only when the maintainer has explicitly approved it for the task
(contributors get that approval on the issue), and then update `README.md`
in the same change:

- Command names, flags, and flag defaults (the `default*` constants in
  `pkg/health.go`).
- Exit codes: `0` PASS, `1` WARN, `2` FAIL, `3` operational error
  (`ExitCode*` in `pkg/report.go`). `jobs diff --exit-code` exits `1` on
  change.
- Check keys and their order in `AllCheckKeys`: append only, never rename
  (keys are stored in the database and accepted by `--only`).
- `misconfig` rule IDs and order in `misconfigRules`: append only, never
  rename or reuse; problems print as `<id>: <details>` joined by `; `.
- JSON output of `check run` and `jobs list|show|diff|stats`: fields may be
  added, never renamed, removed, or retyped; `check run --json` stays a bare
  array.
- Nagios line layout, Prometheus metric names and labels, and JUnit layout
  (golden tests in `pkg/report-formats_test.go`).
- Streams: stdout carries only command output; errors go to stderr.
- `SALUS_DB_PATH`, `SALUS_SSHD_CONFIG`, and the default database locations.
- The database schema (additive changes only; see `.claude/rules/persistence.md`).
- Release archive names, `checksums.txt`, and container image tags (`cd.yml`).
- Exported identifiers in `pkg`: the package moved out of `internal/` so
  another project can import it (commits `08faf9b` and `555b10c`), so treat
  exported names and signatures as API.
- `Main(ctx, version, args) int`, which other programs call to run Salus
  in-process, possibly several times: Salus code never calls `os.Exit`
  (Cobra still can on a few edge paths: the Windows mousetrap and failed
  writes in `help` and `__complete`), `Main` never reads `os.Args`, and no
  flag value, database, or signal handler outlives a call.

A change that can raise the exit code on a host that passed before, or that
changes output scripts parse, also needs an "Upgrading from <version>" note
in `README.md`.

## Working rules

- Inspect the relevant code, tests, and docs before changing anything. Make
  the smallest complete change. No unrelated refactoring, renames (file names
  use hyphens; keep them), formatting churn, or dependency upgrades.
- Match the nearest existing pattern before introducing a helper,
  abstraction, or dependency. Direct dependencies are `spf13/cobra`,
  `gorm.io/gorm`, and `gorm.io/driver/sqlite`; prefer the standard library.
  A new module needs a stated reason and a `NOTICE` update.
- Every behavior change gets tests in the same change
  (`.claude/rules/testing.md`).
- Never weaken a security control to make something pass: input validation,
  file modes, output sanitizing, timeouts, CI permissions, linters, tests,
  or scanners. Do not suppress a lint or gosec finding without a comment
  that explains why it is safe.
- Never put secrets, tokens, local databases (`*.db`), or `.env` files in the
  repository, logs, or output.
- Flag security-sensitive changes (external commands, file handling, the
  database, workflows, the `Dockerfile`) in the completion report.
- If instructions conflict, or a request would break a public contract or a
  security control, stop and say so, and propose a compliant alternative.

## Git and releases

- Leave every change uncommitted for the maintainer to review. Do not
  commit, amend, rebase, reset, stash, tag, push, or merge unless the
  maintainer explicitly asks in the current session.
- Releases are cut only by the maintainer. `make release` and `make push-tag`
  push a tag that starts CD, which publishes immutable releases and
  container images. Never run them, `git push`, or `gh release`.
- `.claude/settings.json` denies pushes and release commands and asks before
  commits, tags, and edits to workflows and `go.mod`. Those are backstops;
  the rules above apply regardless.

## Documentation

Documentation changes ship in the same change as the code they describe.

| File | Owns | Update when |
|---|---|---|
| `README.md` | Everything a user sees: features, flags, check results table, output formats, exit codes, configuration, installation, container, upgrade notes, testing commands, project structure | Any user-visible behavior, flag, output, platform, install, or layout change |
| `CONTRIBUTING.md` | Workflow, setup, validation commands, coding and test expectations, release process | Toolchain, validation, workflow, or convention changes |
| `SECURITY.md` | Supported versions, private reporting, release verification | Reporting or verification process changes |
| `NOTICE` | Every module in `go.mod` with version and license | Any `go.mod` change |
| `.github/pull_request_template.md`, `.github/ISSUE_TEMPLATE/` | Contributor checklists | Validation steps or the contract list change |
| `CLAUDE.md`, `.claude/` | Maintainer rules for Claude | A rule proves wrong or a convention changes |

- Ground every statement in the code, configuration, or workflows. Never
  document a flag, default, platform, or behavior you have not confirmed in
  the source, and never claim a command ran unless it did.
- When a file you edit links to `intel/` or `AGENTS.md`, point the link at
  the current owner from the table above.
- Keep `CLAUDE.md` and `.claude/rules/` to durable rules. Do not record
  plans, status, or history in them; history lives in Git.
- Formatting and README rules: `.claude/rules/docs.md`.

## Completion report

End every task with:

1. What changed and why, with the files affected.
2. Validation run, with results; checks not run and why (for example, no
   macOS or Windows host, or golangci-lint version differs from CI).
3. Assumptions, unresolved questions, and residual risks, including any
   public contract or security-sensitive area touched.
4. Documentation updated, or why none was needed.
