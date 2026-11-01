# Cryptare — instructions for Claude Code

Cryptare is a single-binary Go CLI/TUI that encrypts, decrypts, compresses and extracts
files and folders, with a local SQLite store for password-protected keys. It has no
network surface. Module `github.com/jabbott-iii/Cryptare`; `main.go` only stamps the
version and calls `pkg.Run`; all logic is in the flat package `pkg` (`pkg/`), which
another project imports, so its exported identifiers are public API. Code owner:
@jabbott-iii (`CODEOWNERS`).

Path-scoped detail lives in `.claude/rules/` and loads when you touch matching files.
Repeatable procedures are skills: `/doc-sync`, `/doc-audit`, `/format-change`,
`/toolchain-bump`, `/release-prep`. Independent reviews: the `change-reviewer` and
`docs-verifier` agents.

## Working agreement

- Make the smallest complete change for the task. No unrelated refactoring, renames,
  formatting churn or dependency upgrades.
- Never commit, amend, tag, push or rewrite history. Leave changes uncommitted for the
  owner to review. Releases (`make release`/`tag`/`push-tag`) are the owner's.
- Read the code, tests and docs involved before changing them. Don't assume a command,
  flag, file, setting or behaviour exists; find it.
- Don't say a command ran or passed unless you ran it and saw the result. If a check
  can't run here (no CGO toolchain, no network for `go run` tools, no Docker), say so.
- If a request conflicts with a rule below, don't make the conflicting part. Say what
  conflicts and propose a compliant alternative.
- Finish with: what changed and why, files changed, validation run and its results,
  checks not run, assumptions, open questions and residual risk.

## Commands

From the repository root (GNU Make and bash; on Windows, Git Bash). `make help` lists
the targets (all but the internal `check-version`).

| Command | Use |
|---|---|
| `make build` | `bin/cryptare`, CGO on, stamped with `git describe` |
| `make test` | `go test ./...` (cached; quick loop) |
| `make fmt` / `make fmt-check` | `gofmt -s -w .` / fail if anything needs it |
| `make vet`, `make lint` | `go vet`; golangci-lint at CI's version via `go run` |
| `make golden` | only the golden-fixture tests (`^TestGolden`) |
| `make fuzz FUZZ=<target> FUZZTIME=1m` | one fuzz target from `pkg/fuzz_test.go` |
| `make check` | CI's checks (`ci.yml`) plus a gofmt check CI lacks: fmt-check, tidy-check, keys-check, vet, lint, test-race, smoke |
| `make check-all` | `check` + gosec + govulncheck + actionlint: the pre-PR gate |

- CGO and a C compiler are required: the SQLite driver needs them. A `CGO_ENABLED=0`
  build compiles, but the key store fails at runtime.
- `go.mod` pins `toolchain go1.26.9`; an older `go` downloads it unless
  `GOTOOLCHAIN=local`.
- When you run the binary by hand, point `CRYPTARE_DB_PATH` at a throwaway file
  (for example `"$(mktemp -d)/dev.db"`) so you never open the user's real key store.
- Minimum before reporting a Go change done: `make fmt-check vet test`. Run `make check`
  when the toolchain allows it; format, crypto, key-store or extraction changes also need
  `make golden` and the relevant fuzz targets.

## Architecture

| Layer | Files | Responsibility |
|---|---|---|
| Entry | `main.go`, `pkg/run.go`, `pkg/database_path.go` | version, `Main` (the entry point; `Run` passes it `os.Args[1:]`), root command with a lazy key-store opener, exit status, the key store's path and `keys path` |
| Interfaces | `pkg/logic-cli.go` (Cobra); `pkg/ui-dashboard.go`, `pkg/logic-tui.go` (Bubble Tea) | parse input, prompt, call core/storage, render |
| Core | `pkg/crypto.go`, `pkg/format_v2.go`, `pkg/compress.go`, `pkg/keys.go` | formats, key derivation, key blobs and exports, archives, shared key-store flows |
| Storage | `pkg/database.go`, `pkg/fileowner_unix.go`, `pkg/fileowner_other.go` | GORM/SQLite `KeyModel`, key CRUD, and the key store's mode and owner checks |

1. Core is UI-agnostic: no printing, prompting, Cobra or Bubble Tea. It takes paths,
   passwords or bytes and returns errors.
2. Checks that protect data (password policy, same-file and containment checks,
   extraction limits, credential kind) belong in core, so both interfaces get them.
   The existing-output check, `CheckOutputPath`, is defined in core and called by each
   interface (the CLI honours `--force`; the TUI never overwrites).
3. The CLI and TUI stay in step. Shared key-store flows live in `keys.go`. A behaviour
   change lands in both `logic-cli.go` and `logic-tui.go`, with tests for each.
4. Only commands that use stored keys open the key store, through the lazy
   `DatabaseOpener`: the `keys` commands (except `keys path`), the TUI,
   `encrypt --key`, and `decrypt` of a file whose header names a stored key. File
   commands must not touch it.
5. `*Database` is the only persistence type; core sees it through the `Storage`
   interface.

## Non-negotiables

- **Compatibility.** Everything any release wrote must stay readable: legacy (v1) and
  v2 files and folders, stored keys and `.ckey` exports. A format change needs a new
  version, content type, key source or KDF identifier, a read path for existing data,
  and tests for both. The v2 layout is documented in the header comment of
  `pkg/format_v2.go`. Use `/format-change` before touching anything that is read or
  written on disk.
- **Golden fixtures.** `pkg/testdata/golden/` holds files written by released binaries
  (and two stored-key fixtures from the build that introduced key source 2; see its
  `README.md`). Never edit, regenerate or delete one, and never change a golden test's
  expectations to make it pass. A new format version adds new fixtures.
- **Password normalisation is part of the format.** If
  `TestNormalizePasswordKnownAnswers` fails after a `golang.org/x/text` or Go update,
  that is a format change for the owner to decide, not a test to update.
- **Password policy.** `CheckPasswordPolicy` (15+ code points, not one repeated
  character) applies to anything that protects new data. Decrypt and import accept any
  password, so old data stays readable.
- **Secrets.** Never commit, log or print passwords, keys, `*.db`, `*.ckey` or `.env`.
  `make keys-check` (and CI) refuses tracked key material. The golden-fixture
  passwords are published test vectors, not secrets.
- **No weakened controls.** Don't disable or skip tests, linters or scanners, lower
  file modes, relax extraction limits or validation to get a build through. A gosec
  finding that can't be fixed is annotated on its line as
  `// #nosec <rule> -- <reason>`, naming only that rule.
- **Public API.** Don't rename, remove or change the signature of an exported `pkg`
  identifier without the owner's approval; another project imports the package.
- **CI and release config.** Change workflows, `Dockerfile`, `go.mod`/`go.sum` or
  `scripts/` only when the task needs it. Actions stay pinned to commit SHAs and base
  images to digests.

## Documentation

| Document | Holds | Keep in step with |
|---|---|---|
| `README.md` | user-facing behaviour: features, commands and flags, defaults and limits, configuration, install and release assets, Docker, upgrade notes, make targets, project structure | `logic-cli.go`, `logic-tui.go`, core constants, `cd.yml`, `Dockerfile`, `Makefile` |
| `CONTRIBUTING.md` | contributor setup, workflow, validation table, coding and PR expectations | `Makefile`, workflows, this file |
| `SECURITY.md` | supported versions, private reporting | latest release tag |
| `pkg/testdata/golden/README.md` | fixture inventory and passwords | the fixtures |
| `pkg/format_v2.go` header comment | the v2 on-disk layout | the code below it |
| `CLAUDE.md`, `.claude/` | these instructions | the commands, layout and rules they describe |

- After any change, run `/doc-sync` before you report the task done. A doc must
  describe the code as it is; never document planned work as done.
- A user-visible change since the latest tag gets a line in the README's
  "⚠️ Upgrading from …" notes. Find the tag with `git describe --tags --abbrev=0`
  after `git fetch --tags` (clones made by tools are often shallow and tagless).
- Match the existing style: British spelling (behaviour, licence, normalise), plain
  sentences, copyable commands run from the repository root.
- Security-related text stays at summary level: what changed and how it is tested.
  Never write exploit detail, reproduction steps or proof-of-concept input in the
  repository; vulnerability details belong in a private GitHub advisory (`SECURITY.md`).
- There is no in-repo changelog. Significant changes are described in the PR, the
  release notes and, when users are affected, the README upgrade notes.

## Known documentation debt (2026-10-18)

<!-- doc-audit:off -->
Commit `8049027` deleted `AGENTS.md` and `intel/` (`maint.md`, `cybersec.md`, `map.md`,
`notes.md`, `history.md`, `plan.md`, `golang.md`). The documents and comments that
referred to them now point to this file, `.claude/rules/` or the `pkg/format_v2.go`
header comment, or no longer need the reference. Apart from `git show 40c3464:…`
pointers into history (here and in `/format-change`), only an `intel` entry in
`.dockerignore` remains (harmless).

- This file and `.claude/rules/` hold the maintained rules. Don't recreate `AGENTS.md`
  or `intel/`.
- IDs in comments and docs (`SEC-016`, `BUG-011`, `Q-009`, "plan 3.4") point into the
  removed registers. Keep them as they are and don't invent new ones. For background,
  read the last version: `git show 40c3464:intel/cybersec.md` (also `notes.md`,
  `history.md`, `maint.md`); in a shallow clone, `git fetch --depth=50 origin main`
  first.
<!-- doc-audit:on -->
