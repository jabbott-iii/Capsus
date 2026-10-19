---
paths:
  - "**/*.go"
  - "go.mod"
  - "go.sum"
---

# Go conventions

These cover Capsus' own Go code: the root package `main`, `internal/` and `tools/`. Go files in
the tool folders follow their own repository's rules and are never edited here.

## Every file

- Starts with the Apache-2.0 header block the tools use: a `/* … */` comment that begins
  `Copyright 2026 Joseph Anthony Abbott III`, followed by the standard Apache License 2.0
  notice (copy it from any tool's `main.go`). Test files too. Then a blank line and `package`.
  A file with a build constraint puts `//go:build …` and a blank line before the header.
- `gofmt -s` (`make fmt` for the launcher's files). `goimports` is not part of the tooling.
- golangci-lint v2 runs with its defaults at the root; there is no root `.golangci.yml`, and you
  don't add one without asking. A suppression (`//nolint:<linter>` or
  `// #nosec <rule> -- <reason>`) is a last resort, needs the maintainer's agreement and names
  its reason on the same line.

## Design

- Keep the launcher thin: registry, dispatch, menu and Cobra commands. Tool behaviour belongs to
  the tools.
- Export nothing from `internal/` that `main` doesn't need, and keep the package cohesive; split
  it only when that clearly helps.
- No package-level mutable state. Build the registry in a function or as an immutable value, and
  inject what varies (output writers, environment lookups, the tools' `Main` functions) so tests
  can use fakes.
- Define small interfaces at the consumer, and only when a test or a second implementation needs
  one.
- No new goroutines, channels or mutexes unless the task needs them; menu background work runs as
  Bubble Tea commands. Every goroutine has an owner, a cancellation path and an end.
- Prefer the standard library and the modules the tools already use. Generics only when they
  remove real duplication. No logging framework: results go to stdout, diagnostics to stderr.
- Tools run in-process through their `Main`; don't start them with `os/exec` (D-3).
- Comments explain non-obvious intent, constraints or trade-offs. Exported identifiers get doc
  comments that start with their name.

## Errors and context

- Return errors with context; wrap with `%w` when callers inspect them; compare with `errors.Is`
  or `errors.As`, never by string. No `panic` for user input or I/O failures.
- `context.Context` comes first in anything that blocks or calls a tool. Create it in `main`,
  pass it down to `Main(ctx, …)`, and never store it in a struct.
- Discard an error only deliberately (`_ =`), with a comment when the reason isn't obvious.
- A tool's exit code is a result, not an error: return it unchanged.

## Tests

- Next to the code. Deterministic: no sleeps, no real home directory, no network, no dependence on
  the machine's time zone. Use `t.TempDir()`, `t.Helper()`, `t.Cleanup()`, `t.Setenv` for the
  `*_DB_PATH` variables, and `t.Fatalf` on setup errors.
- Table-driven where inputs share a pattern; a regression test names the behaviour it pins.
- `make race` for anything concurrent (it needs CGO).

## Modules

- `go.mod` and `go.sum` change only when a dependency change requires it. Run `go mod tidy` at
  the root only, never inside a tool folder; review the diff for unrelated churn; then
  `make tidy-check`. Never edit `go.sum` by hand.
- Keep the `require … v0.0.0` and `replace … => ./<Tool>` pairs and the root `toolchain` line
  (D-3, D-9).
