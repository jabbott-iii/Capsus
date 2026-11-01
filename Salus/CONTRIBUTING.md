# Contributing to Salus

Thanks for your interest in improving Salus. This guide covers how to propose
changes, set up a development environment, validate your work, and open a pull
request. Architecture and maintainability rules are defined in
[`CLAUDE.md`](CLAUDE.md) and the path-scoped rules in
[`.claude/rules/`](.claude/rules/), which take precedence if this guide and
those files disagree. Go-specific rules are in
[`.claude/rules/go.md`](.claude/rules/go.md).

Please follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Contribution workflow

1. **Open an issue first.** Create an issue to pitch an addition or change,
   using the "Feature or change proposal" form (or "Bug report" for a
   defect). Pull requests with no corresponding issue will be declined.
2. **Comment on existing issues.** If an issue for the item already exists,
   comment on it before opening a pull request that addresses it.
3. **Wait for confirmation.** A maintainer must confirm the pitched concept on
   the issue before you open a pull request that modifies the code base.
4. **Branch and implement.** Keep the change scoped to the confirmed issue.
5. **Validate locally** (see [Validation](#validation)), then open a pull
   request that links the issue.

## Development setup

### Prerequisites

- **Go 1.26.8** or newer, matching the `go` directive in `go.mod`. With the
  default `GOTOOLCHAIN=auto`, an older Go 1.21+ toolchain downloads the
  required version automatically.
- **A C toolchain** (for example `gcc` or `clang`) with `CGO_ENABLED=1`.
  Salus uses `gorm.io/driver/sqlite`, which depends on the CGO driver
  `github.com/mattn/go-sqlite3`. A build with CGO disabled compiles but
  cannot open its database at runtime.
- **Git.**
- Optional: **GNU Make**, for the targets in the `Makefile`.
- Optional: **golangci-lint v2.13.2**, the version CI uses.
- Optional: **Docker**, to build the container image.

A dev container definition is provided in `.devcontainer/devcontainer.json`
(Ubuntu base with the Go and Docker-outside-of-Docker features). Confirm that a
C compiler is available inside it (`gcc --version`) before building.

### Build and run

From the repository root:

```bash
go mod download
CGO_ENABLED=1 go build -o salus .   # or: make build
./salus check list
./salus check run
```

By default Salus stores its database in a per-user data directory (see
Configuration in `README.md`). To keep development data separate, set
`SALUS_DB_PATH`:

```bash
SALUS_DB_PATH="$(mktemp -d)/salus.db" ./salus check run --only misconfig
```

## Validation

Run these from the repository root before opening a pull request. They include
the checks in `.github/workflows/ci.yml`.

```bash
gofmt -s -w .                      # required before every pull request
go vet ./...
go test ./...
go test -race ./...                # required for concurrency-related changes
GOOS=linux golangci-lint run ./...    # CI: golangci-lint v2.13.2, .golangci.yml
GOOS=darwin golangci-lint run ./...   # CI also lints on macOS
GOOS=windows golangci-lint run ./...  # ...and on Windows
go mod tidy && git diff --exit-code   # CI fails if go.mod/go.sum drift
```

`make fmt`, `make vet`, `make test`, and `make lint` (all three `GOOS`
values) run the same commands. `make cover` writes `coverage.out` and prints
coverage per function.

CI also runs tests on Linux, macOS, and Windows, and CodeQL, gosec, and
govulncheck on every push and pull request. Make sure CI passes on your pull
request.

## Coding expectations

- **License header.** Maintain the Apache-2.0 license header on every source
  file. Copy it from an existing `.go` file. Build constraints
  (`//go:build ...`) go above the header.
- **Formatting.** Run `gofmt -s -w .` at the repository root before opening a
  pull request.
- **Scope.** Keep changes narrowly scoped. Avoid unrelated refactoring,
  formatting churn, renames, or dependency upgrades.
- **Compatibility.** Command names, flags and their defaults, exit codes (`0`
  PASS, `1` WARN, `2` FAIL, `3` operational error), check keys and their
  order, `misconfig` rule identifiers, JSON output fields, the Nagios,
  Prometheus, and JUnit output layouts (including metric names and labels),
  `SALUS_DB_PATH`, `SALUS_SSHD_CONFIG`, the database schema, and the exported
  identifiers of package `pkg`, which other projects can import, are public
  contracts. Do not change them without explicit approval on the issue. See
  [Public contracts](CLAUDE.md#public-contracts) in `CLAUDE.md` for the full
  list.
- **Errors.** Wrap errors with context (`fmt.Errorf("...: %w", err)`), and
  check and return every write to a command's output writer.
- **External commands.** Run tools only through `opts.command(ctx, ...)`, which
  applies `--timeout`, kills the processes a tool started, and bounds its
  output. Pass arguments separately, never invoke a shell, and validate
  user-supplied arguments.
- **Dependencies.** Prefer the standard library and existing dependencies.
  Explain any new dependency in the issue, and update `NOTICE` when the module
  graph changes.
- **New checks** follow the checklist in
  [`.claude/rules/checks.md`](.claude/rules/checks.md#adding-a-check): key
  constant appended to `AllCheckKeys`, registry entry (plus a `checkTargets`
  entry if it runs once per target), seeded catalog entry, tests, and
  documentation.

## Tests

- Add or update tests for every behavior change. Use table-driven tests where
  inputs share a pattern.
- Tests must be deterministic. New tests must not depend on Docker,
  Kubernetes, systemd, or the host's current resource levels. Use fixtures or
  injected fakes instead.
- Use `t.TempDir()` for files and databases (see `newTestDatabase` in
  `pkg/database_test.go`) and `t.Cleanup()` for teardown.
- Run the tests as a normal user, not root: tests that make files unreadable
  skip themselves under root.

## Documentation

Update documentation in the same pull request when your change affects it:

- `README.md` for user-visible commands, flags, behavior, output, platforms,
  installation, testing commands, or the project structure. A change that can
  raise the exit code on a host that passed before, or that changes output
  scripts parse, also needs an "Upgrading from" note.
- `CONTRIBUTING.md` when the toolchain, validation commands, workflow, or
  conventions change.
- `SECURITY.md` when the reporting or release verification process changes.
  Never describe an undisclosed vulnerability in a public file; report it
  privately first (see `SECURITY.md`).
- `NOTICE` for any `go.mod` change.
- `.github/pull_request_template.md` and `.github/ISSUE_TEMPLATE/` when the
  validation steps or the list of public contracts change.
- `CLAUDE.md` and `.claude/` when a maintainer rule proves wrong or a
  convention changes.

See the Documentation section of `CLAUDE.md` and `.claude/rules/docs.md` for
the full documentation rules.

## Pull request expectations

The pull request template (`.github/pull_request_template.md`) covers these
points:

- Link the confirmed issue and describe what changed and why.
- List the validation commands you ran and their results.
- Include tests for the change, and note any checks you could not run.
- Call out security-sensitive changes explicitly for review.
- Do not include secrets, credentials, tokens, local databases (`*.db`), or
  `.env` files.
- Keep the pull request focused. Split unrelated work into separate issues.

## Security issues

Do not report vulnerabilities in public issues or pull requests. Report them
privately through GitHub's private vulnerability reporting, as described in
[`SECURITY.md`](SECURITY.md), which also lists the supported versions and
what is in scope. Reported vulnerabilities are disclosed as GitHub security
advisories once fixed (see [Known issues](SECURITY.md#known-issues)).

## Releases

Releases are cut by maintainers. `make release VERSION=vX.Y.Z` creates and
pushes an annotated tag, which triggers the CD workflow. When a change touches
`.github/workflows/cd.yml` or the actions it uses, run CD manually first
(`workflow_dispatch`): it builds, smoke-tests, packages, and attests the
archives, and builds and smoke-tests the container images. It skips the
`Create GitHub Release` and `Publish pre-release` steps and the
`image-publish` job (registry push and image attestation), which run only
for tags; a pre-release tag such as `vX.Y.Z-rc.1` exercises them before a
stable release.

## License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0 (see [`LICENSE`](LICENSE), section 5).
