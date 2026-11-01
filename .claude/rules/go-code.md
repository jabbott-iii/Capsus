---
paths:
  - "**/*.go"
---

# Go conventions in this repository

- Every `.go` file starts (after any `//go:build` line) with the Apache-2.0 header
  block used in `main.go`. Copy it into any new file. The PostToolUse hook reports a
  file without it.
- Format with `gofmt -s` (the hook does this after each edit; `make fmt` for the tree).
  Code must pass `go vet` and golangci-lint v2.13.2 with its default linters; the
  repository has no golangci-lint config file.
- Wrap errors with context and lower-case messages:
  `fmt.Errorf("read source file: %w", err)`. Compare with `errors.Is`/`errors.As` and
  the package's sentinels (`ErrEmptyPassword`, `ErrSameInputOutput`, …), never on
  message text. Check errors from output writes (`fmt.Fprintf` to a command's output).
- Deferred `Close` on a writer goes through `closeWithError` (`compress.go`), so a
  close failure isn't lost.
- Files the tool writes are mode `0o600` (extracted files marked executable `0o700`);
  folders it creates are `0o700`.
- Operations that read or write data in a loop take a `context.Context` and check it
  (see `copyContext` and the `…Context` functions). The plain function wraps the
  `…Context` one with `context.Background()`.
- Group code under the existing banner comments
  (`//------…------core------…------//`, `extraction limits and output`, `source trees`,
  `output paths`, `key management`, `key export/import`).
- File names are mixed (`logic-cli.go`, `logic_cli_test.go`). Don't rename existing
  files. Name new ones in snake_case, as Go convention has it. Platform-specific code
  uses build-tagged pairs like `fileowner_unix.go` / `fileowner_other.go`.
- Prefer the standard library and the existing dependencies (Cobra, Bubble Tea, Lip
  Gloss, `charmbracelet/x/term`, GORM with the SQLite driver, `x/crypto` for Argon2id,
  `x/text` for NFKC). Use only established cryptographic primitives; never hand-roll
  one. A new dependency needs the owner's agreement; see `build-release.md`.
- Comments explain non-obvious intent, constraints or trade-offs, not what the code
  says. Existing references such as `(SEC-016)` or `(plan 3.4)` stay; don't add new IDs.
- Exported identifiers in `pkg` are public API (another project imports the package).
  Don't export something that isn't needed outside the package, and don't change or
  remove an existing export without approval.
