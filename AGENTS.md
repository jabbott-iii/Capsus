# AGENTS.md

Instructions for AI coding agents working in Capsus. They apply to the whole repository; the
tool folders also carry their own instructions (see [Tool folders](#tool-folders)). Claude Code
reads this file through `CLAUDE.md`. The area rules in `.claude/rules/` apply to every agent:
read the matching one before you change an area ([Area rules](#area-rules)).

## Purpose

Capsus is a toolbox: one Go binary, `capsus`, that bundles three terminal tools. The project
instructions require:

1. Bring together the CLI/TUI tools [Munus](https://github.com/jabbott-iii/Munus),
   [Cryptare](https://github.com/jabbott-iii/Cryptare) and
   [Salus](https://github.com/jabbott-iii/Salus).
2. A TUI lists each tool's name with a short description.
3. Selecting a tool launches that tool's TUI; a tool without a TUI says so.
4. Each imported tool lives in a folder named after it: `Munus/`, `Cryptare/`, `Salus/`.

The whole repository is written in Go, like the tools.

| Tool | Module | TUI | Exit codes | Data location variable |
|---|---|---|---|---|
| Munus: task manager (deadlines, statuses, tags) | `github.com/jabbott-iii/Munus` | yes, `--vim` | 0; 1 on error | `MUNUS_DB_PATH` |
| Cryptare: encrypt, decrypt, compress, extract; password-protected keys | `github.com/jabbott-iii/Cryptare` | yes, `--vim` | 0; 1; 128 + signal (130 on Ctrl+C) | `CRYPTARE_DB_PATH` |
| Salus: host health checks with PASS/WARN/FAIL results | `github.com/jabbott-iii/Salus` | no, CLI only | 0/1/2 for PASS/WARN/FAIL; 3 on error | `SALUS_DB_PATH` |

All three use Go 1.26, keep their code in package `pkg` (`pkg/`), use Cobra v1.10.2 and GORM
with `mattn/go-sqlite3` (CGO), and are Apache-2.0. Munus and Cryptare use Bubble Tea v1.3.10
and Lip Gloss v1.1.0.

## Current state (checked 2026-11-01)

- Phases 0 and 1 are done, apart from the optional P-004. Munus `v1.0.1`, Cryptare `v1.4.1` and
  Salus `v1.2.1` export `pkg.Main` (D-4). On 2026-11-01 the maintainer imported them with
  `git subtree add --squash`, directly on `main` (merges `f188212`, `cc4c579`, `4c72f58`), and
  the registry's bundled versions are those tags (P-011). Every module folder now has its
  `go.mod`, so the module targets no longer stop at `make layout`.
- Phase 2's launcher is written; its validation is open:
  - gosec reports findings in `Salus/pkg` that predate the import and are to be fixed or
    annotated upstream in Salus, so `make sec` fails and `make check-all` stops there. The
    Security workflow runs gosec with `-no-fail`.
  - CI on `4c72f58` failed in the four Windows test jobs at "Verify dependencies"
    (`go mod tidy -diff`): Windows runners check out `go.sum` with CRLF line endings. The root
    `.gitattributes` keeps `go.mod` and `go.sum` LF on every checkout; no GitHub run has
    confirmed the fix yet.
- Phase 3's `Dockerfile` (`static` stage) and `.dockerignore` are written (P-030); no Docker
  workflow run has confirmed them yet (P-031).
- Phase 4's `tools/licenses/` generator, `make licenses`, `THIRD_PARTY_LICENSES` and the
  combined `NOTICE.txt` are written (P-040, P-041); no CD run has packaged them yet. P-042 is
  still open, and Phase 5 too: there is no Capsus release or tag.
- The imported releases carry each tool's `CLAUDE.md` and `.claude/`; none carries the retired
  `AGENTS.md` or `intel/`.
- This file and `.claude/` replace the old `AGENTS.md` and `intel/` documents (last version in
  commit `7cb3629`, for example `git show 7cb3629:intel/plan.md`). Their IDs (`D-n`, `P-0nn`,
  `R-n`, `Q-n`, `SEC-00n`) are kept here and in `.claude/rules/` so existing references still
  resolve. IDs inside a tool folder belong to that tool's own retired documents; look them up in
  its repository. Don't recreate `intel/` and don't add new IDs.

## Ground rules

- Never commit, amend, tag, push, merge, rebase, stash or rewrite history, and never run
  `git subtree` (it creates commits). Leave every change uncommitted; the maintainer runs
  imports, syncs and releases.
- Make the smallest complete change: no unrelated refactors, renames, formatting churn,
  dependency upgrades or new abstractions. Preserve the CLI contract, menu keys and how
  arguments and exit codes reach a tool unless a breaking change is approved.
- Inspect the code, tests, configuration and docs involved before changing them, and follow the
  nearest existing pattern. Don't assume a command, flag, file, variable or feature exists:
  find it.
- Never claim a command ran or passed unless you ran it and saw the result; say what you could
  not run and why (no Go 1.26, no C compiler, no Docker, no network for `go run` tools).
- Never weaken a control to get a change through: tests, linters, scanners, SHA and digest pins,
  workflow permissions, file modes, input validation, `make tool`'s allow-list. A suppression
  needs the maintainer's agreement and its reason on the same line (`// #nosec G204 -- reason`).
- Never commit, log or print secrets, `.env` files, databases (`*.db*`) or key exports
  (`*.ckey`).
- If a request conflicts with these rules, a security control or a public contract, don't make
  the conflicting part: say what conflicts and propose a compliant alternative. Ask before any
  broad or irreversible change.
- Flag security-sensitive changes in the completion report: how the launcher calls a tool,
  workflows, the `Makefile`, `scripts/`, the `Dockerfile`, dependencies.

## Commands

Run from the repository root. They need GNU Make, bash, git, Go 1.26 and a C compiler (CGO for
`mattn/go-sqlite3`); Docker only for `make docker-static`. `make help` lists every target.

| Command | Does |
|---|---|
| `make check` | What CI runs, in every module: layout, keys-check, fmt-check, tidy-check, vet, lint, race, smoke |
| `make check-all` | `check` plus gosec, govulncheck and actionlint: the full pre-PR gate |
| `make build` | `bin/capsus` with CGO, `main.version` stamped from `git describe` |
| `make smoke` | Build, then run `scripts/smoke.sh` with every database in a temporary folder |
| `make test`, `race`, `cover`, `vet`, `lint`, `tidy-check` | One check, looped over every module |
| `make fmt` | `gofmt -s -w` on the launcher's files only; tool folders are never rewritten |
| `make deps-drift` | Modules the capsus build uses at other versions than a tool's `go.mod` (R-1) |
| `make licenses` | Regenerate `THIRD_PARTY_LICENSES` and check the `NOTICE.txt` module list (P-040); `go test ./tools/licenses` fails while either is stale |
| `make tool TOOL=<Tool> TARGET=<target>` | One of a tool's own Makefile targets, from an allow-list without release or file-rewriting targets |

- `go … ./...` at the root never enters a nested module: use the make targets, which loop over
  `MODULES`, or run `go` inside one module's folder.
- `make layout` fails only when a module's `go.mod` is missing; since Phase 1 that is a real
  failure, such as a broken subtree pull. `make check-all` stops at `make sec` while gosec
  reports the known findings in `Salus/pkg`, so run `make vuln actionlint` separately. Report
  tool findings upstream and never annotate tool code here (R-5); a gosec finding in the
  launcher is a real failure.
- When you run a binary by hand, point `MUNUS_DB_PATH`, `CRYPTARE_DB_PATH` and `SALUS_DB_PATH` at
  files in a temporary folder. Build into `bin/`; `.gitignore` doesn't cover a root `capsus`.
- `make release`, `make tag` and `make push-tag` are the maintainer's. Never run a tool's `tag`,
  `push-tag` or `release` target, nor `make -C <Tool>`: inside Capsus they tag and push Capsus
  and start `cd.yml` (SEC-001).

## Layout

```text
Capsus/
├── main.go                        # P-021: version, Cobra root command `capsus`
├── internal/launcher/             # P-022: tool registry and Bubble Tea menu
├── go.mod, go.sum                 # P-020: require and replace the three tools
├── .gitattributes                 # LF line endings for go.mod and go.sum on every checkout
├── Munus/ Cryptare/ Salus/        # P-010: git subtrees, one Go module each
├── Dockerfile, .dockerignore      # P-030: `static` stage for the Linux release binaries
├── tools/licenses/                # P-040: THIRD_PARTY_LICENSES generator (`make licenses`)
├── THIRD_PARTY_LICENSES           # P-040: generated license texts for the release archives
├── scripts/smoke.sh               # smoke test of a built capsus (Makefile, ci.yml, cd.yml, docker.yml)
├── Makefile                       # per-module checks, build, smoke, scans, `make tool`, release tags
├── .github/                       # ci, security, docker and cd workflows; dependabot.yml; PR template
├── AGENTS.md, CLAUDE.md, .claude/ # agent instructions; Claude Code rules, skills, settings, hook
└── README.md, CONTRIBUTING.md, SECURITY.md, LICENSE, NOTICE.txt, CODEOWNERS.txt
```

## Architecture: Plan 1 decisions

Plan 1 (2026-10-11) is the design the README, `Makefile`, smoke test and workflows already
target. Its decisions await the maintainer's acceptance (Q-1): build to them, and raise a
disagreement instead of diverging.

- **D-1 Layout.** `Munus/`, `Cryptare/`, `Salus/` at the root, named exactly like the
  repositories and module paths, each with its own `go.mod`.
- **D-2 Import.** `git subtree add --squash` per tool at a release tag; later releases arrive
  with `git subtree pull --squash`. Rejected: submodules (every clone, CI job and release build
  would need `--recurse-submodules`), plain copies (no recorded link to the upstream commit), and
  a module dependency only (no folders, against the project instructions).
- **D-3 One binary, tools in-process.** The root module `github.com/jabbott-iii/Capsus` requires
  each tool at `v0.0.0` and replaces it with its folder (`github.com/jabbott-iii/Munus =>
  ./Munus`, and so on); no `go.work` is committed. So `go install …@latest` is unsupported, and
  `make build` and the release archives are the install paths. Running the tools as child
  processes was rejected: four binaries on `PATH` instead of one.
- **D-4 Entry point, added upstream first (Phase 0).** Each tool's `pkg` exports
  `func Main(ctx context.Context, version string, args []string) int`, which runs the tool's
  command line with `args` (no program name), reports `version` for `--version` and returns the
  exit code. The tool's `main.go` becomes a thin wrapper, so the standalone binary and Capsus
  share one tested path (Cryptare's lazy database opening and exit 130, Salus' panic-to-exit-3
  and database close, Munus' database close). `Main` is a proposed name (Q-2). A fallback
  without upstream changes (other exported tool API, a copy of Salus' `runWith`, `os.Args`
  overwritten for `cryptare.Run`) was considered and rejected: don't use it, or copy a tool's
  `main` logic into Capsus, unless the maintainer decides otherwise.
- **D-5 CLI contract** (`scripts/smoke.sh` tests its non-interactive parts): `capsus` in a
  terminal opens the menu; `capsus list` prints each tool's name, description and `TUI` or
  `CLI only`; `capsus munus|cryptare|salus [args...]` passes `args` unchanged (Cobra
  `DisableFlagParsing`) and exits with the tool's exit code, and without arguments a TUI tool
  opens its TUI while Salus prints its help; an unknown tool exits non-zero; `capsus --version`
  prints `capsus version <version>`, then one line per bundled tool; `capsus munus --version`
  prints `munus version <bundled tag>`. Each tool keeps its own data location and variable, so
  Capsus and the standalone binaries share data (R-2). A tool's help shows the tool's own name
  (`munus add …`); accepted for the first release.
- **D-6 Menu.** One row per tool: name, short description, `TUI` or `CLI only`. Keys ↑/↓ and
  j/k, Enter, q or Ctrl+C; `--vim` is passed on to Munus and Cryptare. Enter on a TUI tool: the
  menu exits, Capsus calls that tool's `Main` with no arguments so its TUI takes the terminal,
  and shows the menu again when it returns; two Bubble Tea programs never run at once. Enter on
  Salus: the menu stays and says "Salus has no TUI. Run `capsus salus --help` for its commands."
- **D-7 Upstream is the source of truth.** See [Tool folders](#tool-folders).
- **D-8 Releases.** Archives for linux/amd64 and linux/arm64 (static, musl), darwin/arm64,
  darwin/amd64, windows/amd64 and windows/arm64, each with `LICENSE`, `NOTICE.txt` and
  `THIRD_PARTY_LICENSES`, plus `checksums.txt` and build provenance. No container image (Q-4):
  Capsus is interactive, Salus in a container would inspect the container rather than the host,
  and each tool publishes its own image.
- **D-9 Go version.** The root `go.mod` carries the highest of the tools' `go` and `toolchain`
  lines (today `go 1.26.8` and `toolchain go1.26.9`), because the go command ignores `toolchain`
  lines in dependencies: Cryptare pins go1.26.9 for the standard library's `os.Root` fix on
  Windows (GO-2026-6604), which Capsus' Windows binaries need too. CI and release builds use the
  latest Go 1.26 patch release; a new Go minor release is a separate, deliberate change.

The launcher's own code is `main.go` (Cobra commands) and `internal/launcher`: a registry (name,
command, description, has-TUI, accepts `--vim`, bundled version, `Main`) that holds no UI code,
and the menu model.
Details: `.claude/rules/launcher.md`.

## Tool folders

- Never change tracked files under `Munus/`, `Cryptare/` or `Salus/`. Change the tool's own
  repository, release it, and the maintainer syncs it; a local edit conflicts with, or is lost at,
  the next `git subtree pull` (R-5). Report problems in tool code so they can be fixed upstream.
- Each folder brings the instructions of the release it was imported at (the tag
  `internal/launcher/registry.go` records). The current imports carry the tool's `CLAUDE.md` and
  `.claude/` (rules, skills and settings; Cryptare also agents and hooks); most tags made before
  2026-10-13 carry a retired `AGENTS.md` and `intel/` instead. They describe the standalone
  repository: use them to understand a tool, but inside Capsus this file wins for anything that
  writes files or runs commands. Nested `.github/` folders are inert.
- Run a tool's own checks with `make tool TOOL=<Tool> TARGET=<target>`.
- Every tool's package is named `pkg`: import with aliases, such as
  `munus "github.com/jabbott-iii/Munus/pkg"`. Capsus uses only `pkg.Main` and the registry
  metadata it owns, never other exported tool API (D-4).
- Adding a tool touches the `Makefile`, the workflows, `scripts/smoke.sh`, the README, the
  contributor docs and these instructions as well as the launcher;
  `.claude/rules/build-and-ci.md` lists every place.

## Security

- Each tool's own rules stay authoritative for its code. Capsus runs a tool only through its
  `Main`, so file permissions, input limits, terminal sanitising, lazy database opening,
  untrusted-database checks, interrupt clean-up and exit codes behave as in the standalone
  binary. Never bypass them; security fixes to tool code are made upstream.
- No `*.db`, `*.db-journal`, `*.db-wal`, `*.db-shm` or `*.ckey` file is tracked anywhere
  (`make keys-check` and CI's "Repository checks" refuse them).
- Scans cover what ships: gosec and CodeQL in every module, govulncheck on the root module's
  build list. Pipeline hardening: actions pinned by commit SHA, downloads by SHA-256, the
  `Dockerfile`'s builder image by digest, least-privilege permissions, provenance for every
  archive, release builds from clean caches.
- Dependabot pull requests against tool folders are closed and fixed upstream; a root-level bump
  is a stopgap only for a vulnerability govulncheck reports as reachable (Q-8).
- Never write exploit details, reproduction steps or proof-of-concept input into files, commits,
  issues or PR text. `SECURITY.md` is still GitHub's template; until it describes a reporting
  process, report a suspected vulnerability to the maintainer privately, and don't add tracked
  items or IDs yourself.
- SEC-001 (tool release targets act on Capsus), SEC-002 (vulnerability scanning must cover what
  ships) and SEC-003 (static analysis must enter nested modules) are In Progress. Their
  remaining validation, and the tools' own open items that affect Capsus, are in
  `.claude/rules/build-and-ci.md`.

## Tests

- Every behaviour change gets a test, every bug fix a regression test. Tests are deterministic:
  no sleeps, no real home directory, no network; files and databases under `t.TempDir()`,
  databases through the three `*_DB_PATH` variables. Never weaken or delete an assertion.
- Launcher tests (P-023): table-driven dispatch (arguments unchanged, exit codes propagated,
  unknown tool rejected), menu `Update`/`View`, `capsus list` and `--version` output, and menu →
  tool → menu in one process (R-4).
- Tool tests run in their own modules (`make test`, `make race`).

## Documentation

| File | Owns | Keep in step with |
|---|---|---|
| `README.md` | Users: bundled tools, commands, prerequisites, install, configuration, checks, structure | The launcher, `Makefile`, workflows |
| `CONTRIBUTING.md`, `.github/pull_request_template.md` | Contributor workflow, validation, coding and PR expectations | Each other, the `Makefile`, `ci.yml`, this file |
| `SECURITY.md` | Supported versions and private vulnerability reporting (still GitHub's template) | The release tags |
| `NOTICE.txt`, `THIRD_PARTY_LICENSES` | Licensing of the combined binary (P-040, P-041) | The root build list |
| `AGENTS.md`, `CLAUDE.md`, `.claude/` | Agent instructions | The commands, files and rules they name |

Update docs in the same change as the code, or say why a change has no doc impact. Never
document planned behaviour as done. Details: `.claude/rules/docs.md`.

## Area rules

Read the matching rule before changing an area (Claude Code loads it automatically):

| Area | Rule |
|---|---|
| Go code, `go.mod`, `go.sum` | `.claude/rules/go.md` |
| Root `*.go`, `internal/`, `scripts/smoke.sh` | `.claude/rules/launcher.md` |
| `Munus/`, `Cryptare/`, `Salus/` | `.claude/rules/tool-folders.md` |
| `Makefile`, `.github/`, `scripts/`, `Dockerfile`, `.dockerignore`, `.gitignore`, `.gitattributes`, `tools/`, `go.mod`, `go.sum`, `NOTICE.txt`, `THIRD_PARTY_LICENSES` | `.claude/rules/build-and-ci.md` |
| Markdown documents | `.claude/rules/docs.md` |

Repeatable procedures are Claude Code skills that other agents can follow as checklists:
`.claude/skills/{pre-pr,tool-sync,add-tool,docs-drift,release-prep}/SKILL.md`.

## Plan 1 work order

Phases 0 and 1 are done (2026-11-01), apart from the optional P-004. Phase 2's code and tests,
Phase 3's `Dockerfile` and Phase 4's license files are written and wait for the validation
below; every later item is open. The maintainer runs every step that commits, tags or pushes.

- **Phase 0, upstream (done, in each tool's repository).** P-001 Munus `v1.0.1`, P-002 Cryptare
  `v1.4.1` and P-003 Salus `v1.2.1` export `pkg.Main`, with tests (version output, exit codes,
  arguments taken from the parameter rather than `os.Args`); they are patch releases, not the
  minor releases planned. Cryptare's `Run(version)` calls
  `Main(context.Background(), version, os.Args[1:])` and keeps exit status 130 and its Ctrl+C
  shutdown; its release includes the Go 1.26.9 fix and keeps both the `os.Exit` at its hidden
  password prompt (`readTerminalPassword` in `Cryptare/pkg/logic-cli.go`) and its TUI
  signal deadlock (no `tea.WithoutSignalHandler()`; R-4). Salus' `run`/`runWith` are in `pkg`
  behind `Main`, keeping panic to exit 3, the database close and errors on stderr only; its
  gosec findings remain. Munus prints `munus version <tag>` (Q-7). P-004 (optional, open)
  aligns shared dependency versions across the tools; `make deps-drift` lists the differences.
- **Phase 1, import (done; maintainer, `/tool-sync` prepares it).** P-010 `git subtree add
  --prefix=<Tool> https://github.com/jabbott-iii/<Tool>.git <tag> --squash` per tool at the
  Phase 0 tags, run directly on `main` rather than on a branch (merges `f188212`, `cc4c579`,
  `4c72f58`); P-011 each tag recorded as the bundled version in `internal/launcher/registry.go`;
  P-012 `make keys-check`.
- **Phase 2, launcher (written; validation open).** P-020 root `go.mod` (D-3, D-9; Bubble Tea,
  Lip Gloss and Cobra at the tools' versions); P-021 `main.go`; P-022 `internal/launcher`;
  P-023 tests; P-024 license headers.
- **Phase 3, build (written; validation open).** P-030 `Dockerfile` `static` stage and
  `.dockerignore`; P-031 `make check` and the CI, Security and Docker workflows green.
- **Phase 4, licensing and docs (P-040 and P-041 written).** P-040 `THIRD_PARTY_LICENSES`
  generator and `make licenses`; P-041 combined `NOTICE.txt`, whose module list
  `go test ./tools/licenses` checks; P-042 README features verified against a built binary (open: on
  Linux a built binary shows them, so the README no longer labels them planned; on macOS and
  Windows only CI's non-interactive smoke jobs cover them); P-043 the layout above.
- **Phase 5, first release (maintainer).** P-050 `make release VERSION=v0.1.0`; then a green
  `cd.yml`, complete archives and `gh attestation verify <archive> --repo jabbott-iii/Capsus`.

Validation: Phase 0 is checked in each tool's repository. From Phase 2 on, `make check-all` and
`make smoke` pass locally and the CI (Linux, macOS, Windows) and Security workflows are green;
from Phase 3 on, the Docker workflow too. Phase 5 also needs a green CD run with verified
attestations. Phase 2's validation waits for a Salus release that fixes or annotates its gosec
findings, synced into `Salus/`, and for a green CI run with the `.gitattributes` fix (see
"Current state").

Risks: R-1 the binary can run newer shared modules than a tool's own tests used (minimal
version selection; `make deps-drift`); R-2 an older bundled tool can open a database written by
a newer standalone binary (sync soon after each tool release; the tools keep schema changes
additive); R-3 release targets inside tool folders (SEC-001); R-4 the tools share one process
(signal handlers, `os.Stderr`, Cryptare's `os.Exit` at a password prompt); R-5 local edits in
tool folders; R-6 CI is red: its Windows test jobs failed on `4c72f58` (CRLF `go.sum`), the
Docker workflow had no `Dockerfile` to build there, and no GitHub run has confirmed the
`.gitattributes` fix or the `Dockerfile` (P-030) yet.

## Open questions for the maintainer

Don't decide these yourself; work around them or ask.

- Q-1 Accept D-1 … D-9 as written, or change them before Phase 0?
- Q-2 The entry point's name (`pkg.Main` proposed) and the tool versions that ship it. In
  practice: Munus `v1.0.1`, Cryptare `v1.4.1` and Salus `v1.2.1` export `Main` and are the
  imported tags.
- Q-3 Import with `--squash`, or with full upstream history? In practice: the Phase 1 imports
  used `--squash`.
- Q-4 Publish a container image later?
- Q-5 Is `CODECOV_TOKEN` configured for Capsus?
- Q-6 Rename `CODEOWNERS.txt` to `CODEOWNERS` (GitHub ignores the current name) and require
  code-owner review for `.github/`, the `Makefile` and `scripts/`?
- Q-7 Munus' `main` README now calls v3.0.x pre-1.0.0 builds and shows `munus version v1.0.0`,
  but the `v1.0.0` tag's README still describes v3.x releases: confirm that Capsus reports
  Munus' v1.x tags. In practice: Capsus bundles Munus `v1.0.1` and reports
  `munus version v1.0.1`, and that release's README calls the v3.x builds pre-1.0.0.
- Q-8 Is a root-level bump of a tool's dependency acceptable as a security stopgap?

## Completion report

End every task with: a short summary; files changed and why; commands run and their results;
checks not run and why; assumptions; doc impact; unresolved issues and residual risks.
