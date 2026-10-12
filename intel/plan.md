# Active Plans & Follow-on Work

## Plan 1 — Import Munus, Cryptare and Salus (2026-10-11)

**Status:** Proposed. The decisions (D-1 … D-9) await maintainer review. The CI/CD workflows, the
`Makefile` and the contributor documents were updated in the same change to target the layout this
plan produces (`intel/history.md`, 2026-10-11), so CI stays red until Phases 1–3 land (R-6).

### Goal

One `capsus` binary that contains the three tools. Running `capsus` opens a menu TUI that lists each
tool with a short description. Selecting a tool launches that tool's own TUI; a tool without a TUI
says so instead. Each tool's source lives in a folder named after the tool.

### Source facts

Verified on 2026-10-11 against each repository's default branch (`main`).

| | Munus | Cryptare | Salus |
|---|---|---|---|
| Module path | `github.com/jabbott-iii/Munus` | `github.com/jabbott-iii/Cryptare` | `github.com/jabbott-iii/Salus` |
| `go` directive | 1.26.0 | 1.26.0, `toolchain go1.26.9` | 1.26.8 |
| Latest tag | `v1.0.0` (= `main`); its README and `intel/history.md` refer to v3.0.x releases (`intel/notes.md` Q-7) | `v1.4.0`; `main` is 2 commits ahead (Go 1.26.9 vulnerability fix) | `v1.2.0` (= `main`) |
| TUI | Yes: root command without a subcommand; `--vim` | Yes: dashboard, root command without a subcommand; `--vim` | No: CLI only |
| Entry point today | `main.go`: `LocateDatabase` → `NewDeferredDatabaseAt` → `NewRootCmd` → `ExecuteContext`, then closes the database | `pkg.Run(version)`, which reads `os.Args` | `run`/`runWith` in package `main` |
| Exit codes | 0, or 1 on error | 0, 1, or 128 + signal (130 on Ctrl+C) | 0/1/2 = PASS/WARN/FAIL, 3 = error |
| Database override | `MUNUS_DB_PATH` | `CRYPTARE_DB_PATH` | `SALUS_DB_PATH` |

Common to all three: application code in package `pkg` (directory `pkg/`); Cobra v1.10.2; GORM with
`mattn/go-sqlite3` v1.14.52, so every build needs CGO and a C compiler; Apache-2.0; workflow actions
pinned by commit SHA. Munus and Cryptare both use Bubble Tea v1.3.10 and Lip Gloss v1.1.0, so a Bubble
Tea launcher adds no third-party module that the tools do not already compile in.

### Decisions (proposed)

- **D-1 Layout.** `Munus/`, `Cryptare/` and `Salus/` at the repository root, named exactly like the
  repositories and module paths (project instruction). Each keeps its own `go.mod`.
- **D-2 Import mechanism: `git subtree add --squash`**, one per tool, at a release tag.
  - Subtree: the source is in the repository, so a plain `git clone` builds and CodeQL, gosec and
    Dependabot see it; `git subtree pull` brings later releases; one squashed commit per import or
    sync keeps Capsus history readable while the commit message records the upstream commit.
  - Rejected — submodules: every clone, CI job and release build needs `--recurse-submodules`, and the
    folders are empty pointers until initialised.
  - Rejected — plain copy: no recorded link to the upstream commit; every sync is a manual diff.
  - Rejected — Go module dependency only: no folders, contrary to the project instruction.
- **D-3 One binary; tools run in-process.** The root module `github.com/jabbott-iii/Capsus` requires
  the three modules and replaces them with the local folders:

  ```go
  require (
  	github.com/jabbott-iii/Cryptare v0.0.0
  	github.com/jabbott-iii/Munus v0.0.0
  	github.com/jabbott-iii/Salus v0.0.0
  )

  replace (
  	github.com/jabbott-iii/Cryptare => ./Cryptare
  	github.com/jabbott-iii/Munus => ./Munus
  	github.com/jabbott-iii/Salus => ./Salus
  )
  ```

  Checked with stand-in modules on 2026-10-11 (Go 1.24): the root builds from the local folders, and
  `./...` at the root never enters a nested module, in module or workspace mode, so the `Makefile`
  and CI loop over each module. Consequence: `go install github.com/jabbott-iii/Capsus@latest` is not
  supported (the go command refuses `go install pkg@version` for modules with `replace` directives);
  release archives and `make build` are the install paths. Running the tools as child processes was
  rejected: it needs four binaries on `PATH` and contradicts "one binary" in the README.
- **D-4 Embedding entry point, added upstream (Phase 0).** Each tool exports from `pkg`:

  ```go
  // Main runs the tool's command line with args (without the program name),
  // reporting version for --version, and returns the process exit code.
  func Main(ctx context.Context, version string, args []string) int
  ```

  and its own `main.go` becomes a thin wrapper around it. Reasons for adding it upstream instead of
  re-creating each `main` in Capsus:
  - Cryptare's database opener (lazy open, SEC-010; untrusted-database hint, SEC-016), its
    `keys path` command and its exit status 130 after the Ctrl+C clean-up (SEC-015) are unexported.
    Salus' panic-to-exit-3 handling and its close-the-database-before-exit logic live in package
    `main`. Munus' `main` closes its deferred database. Copies in Capsus would duplicate
    security-relevant code that the upstream tests no longer cover.
  - The standalone binary and Capsus then share one code path, tested upstream.

  The change is additive, so each tool ships it in a minor release. Cryptare's existing
  `Run(version)` stays and calls `Main`. The name `Main` is a proposal.
  Fallback without upstream changes (not recommended): call Munus' exported API and Salus'
  `NewVersionedRootCmd` directly (re-implementing Salus' `runWith`), and overwrite `os.Args` before
  calling `cryptare.Run` (global state).
- **D-5 Capsus CLI contract.** `scripts/smoke.sh` already tests this contract:
  - `capsus` in a terminal opens the menu TUI.
  - `capsus list` prints each tool's name, description and whether it has a TUI (`TUI` or
    `CLI only`), for scripts and non-terminal use.
  - `capsus munus|cryptare|salus [args...]` runs that tool with `args` unchanged (Cobra
    `DisableFlagParsing`, so the tool parses its own flags) and exits with the tool's exit code. With
    no arguments, a TUI tool opens its TUI and Salus prints its help, as the standalone binaries do.
  - `capsus --version` prints `capsus version <version>` on its first line, then one line per
    bundled tool with its version; `capsus munus --version` prints `munus version <bundled tag>`.
  - Each tool keeps its own database location and environment variable, so Capsus and the
    standalone binaries share data (R-2).
  - A tool's help shows the tool's own name (`munus add …`) because the tool builds its own command
    tree. Accepted for the first release; the README says so.
- **D-6 Menu TUI.**
  - One row per tool: name, short description, and `TUI` or `CLI only`. Keys: ↑/↓ (and j/k), Enter,
    q or Ctrl+C. `--vim` is passed on to Munus and Cryptare, which both accept it.
  - Enter on a TUI tool: the menu program exits, Capsus calls that tool's `Main` with no arguments
    (its TUI takes over the terminal) and shows the menu again when the tool exits. Two Bubble Tea
    programs never run at the same time.
  - Enter on Salus: the menu stays and shows "Salus has no TUI. Run `capsus salus --help` for its
    commands."
  - Descriptions, from the tools' READMEs:
    - Munus — terminal task manager: tasks with deadlines, statuses and tags in a local SQLite
      database.
    - Cryptare — encrypt, decrypt, compress and extract files and folders; manage
      password-protected keys.
    - Salus — environment health checker: disk, memory, CPU, Docker, Kubernetes, services and
      certificates, with PASS/WARN/FAIL results.
- **D-7 Upstream is the source of truth.** Changes to tool code go to the tool's repository first and
  arrive with `git subtree pull`. The tool folders keep their own `AGENTS.md`, `intel/` and tests;
  their nested `.github/` folders are inert (GitHub reads only the root `.github/`).
- **D-8 Release artifacts.** Binaries for linux/amd64 and linux/arm64 (static, musl), darwin/arm64,
  darwin/amd64, windows/amd64 and windows/arm64, each archived with `LICENSE`, `NOTICE.txt` and
  `THIRD_PARTY_LICENSES`, plus `checksums.txt` and build provenance. No container image in the first
  release: Capsus is interactive, Salus in a container inspects the container rather than the host,
  and each tool already publishes its own image. The image jobs can be restored from Munus'
  `cd.yml` if wanted.
- **D-9 Go version.** The root `go.mod` takes the highest of the tools' `go` and `toolchain` lines:
  `go 1.26.8` and `toolchain go1.26.9` today. The go command ignores `toolchain` lines in
  dependencies, so without its own line the root could build with Go 1.26.8 and lose the
  standard-library fix Cryptare pins for SEC-021 (GO-2026-6604). CI and release builds use the
  latest Go 1.26 patch release, as Munus does (Cryptare and Salus pin Go from `go.mod`); `toolchain`
  sets the floor for local builds. Moving to a new Go minor release is a separate, deliberate change.

### Work items

Status of every item: Open.

#### Phase 0 — Upstream preparation (in each tool repository)

- P-001 Munus: add `pkg.Main` (D-4) with tests (version output, exit codes, arguments taken from the
  parameter rather than `os.Args`); `main.go` calls it. Release a minor version; settle the version
  line first (Q-7).
- P-002 Cryptare: add `pkg.Main`; `Run(version)` calls `Main(context.Background(), version,
  os.Args[1:])`; keep exit status 130 and the SEC-015 shutdown. Decide whether Ctrl+C at a password
  prompt keeps calling `os.Exit` (R-4) or returns an error through `Main`. Release a version that
  also contains the Go 1.26.9 fix (`main` is ahead of `v1.4.0`).
- P-003 Salus: move `run`/`runWith` into `pkg` behind `Main`, keeping panic → exit 3, the database
  close, and errors on stderr only; `main.go` calls it. Release a minor version.
- P-004 Optional: align shared dependency versions across the three repositories (today
  `golang.org/x/text` v0.41.0 in Munus and Salus, v0.42.0 in Cryptare; `golang.org/x/sys` v0.47.0 in
  Munus, v0.48.0 in Cryptare), so each tool's tests run the versions Capsus ships (R-1).

#### Phase 1 — Import (run by the maintainer)

`git subtree add` creates commits; under `AGENTS.md` agents do not create commits, so the maintainer
runs these steps.

- P-010 From a clean working tree, on a branch, at the tags released in Phase 0:

  ```sh
  git switch -c import-tools
  git subtree add --prefix=Munus    https://github.com/jabbott-iii/Munus.git    <munus-tag>    --squash
  git subtree add --prefix=Cryptare https://github.com/jabbott-iii/Cryptare.git <cryptare-tag> --squash
  git subtree add --prefix=Salus    https://github.com/jabbott-iii/Salus.git    <salus-tag>    --squash
  ```

- P-011 Record each imported tag as that tool's bundled version in the launcher registry (P-022);
  update it at every sync.
- P-012 `make keys-check`: no database or key file is tracked (none is tracked upstream today).

#### Phase 2 — Root module and launcher

- P-020 Root `go.mod` per D-3 and D-9 (including the `toolchain` line), with Bubble Tea, Lip Gloss
  and Cobra at the versions the tools use; `go mod tidy` writes `go.sum`.
- P-021 `main.go`: `var version = "dev"`, set by `-ldflags "-X main.version=…"`; Cobra root command
  `capsus` with `list`, `munus`, `cryptare` and `salus` (D-5) and `--vim`; no arguments → menu.
- P-022 `internal/launcher`: the tool registry (name, description, has-TUI, bundled version, `Main`)
  and the Bubble Tea menu model (D-6). The registry holds no UI code, so dispatch is unit-testable.
- P-023 Tests: table-driven dispatch tests (arguments passed unchanged, exit codes propagated,
  unknown tool rejected); menu `Update`/`View` tests (Enter on a TUI tool returns it, Enter on Salus
  shows the message, quit keys); `capsus list` and `--version` output; menu → tool → menu in one
  process (R-4). Deterministic; databases under `t.TempDir()` through the environment variables.
- P-024 Apache-2.0 header on every new source file (`CONTRIBUTING.md`).

#### Phase 3 — Build support

- P-030 `Dockerfile` and `.dockerignore`. The `static` stage mirrors Munus' (`golang:1.26-alpine3.24`
  with pinned `build-base`, but with the builder image pinned by digest as Cryptare (SEC-013) and
  Salus (SEC-002) pin theirs, `-tags sqlite_omit_load_extension,osusergo,netgo`,
  `-linkmode external -extldflags '-static -Wl,-z,stack-size=8388608'`, output `/capsus`). The
  8 MiB stack request matters: musl gives new threads 128 KiB, and the tools' SQLite calls run on
  those threads. `docker.yml` and `cd.yml` already expect this stage.
- P-031 `make check` and the CI, Security and Docker workflows green on Linux, macOS and Windows;
  `make smoke` passes.

#### Phase 4 — Licensing and documentation

- P-040 `THIRD_PARTY_LICENSES` for the combined binary: Go, SQLite, musl and every compiled module,
  including Cryptare's `golang.org/x/crypto`. Adapt Munus' `tools/licenses`
  (stdlib-only generator with a test that fails when the file is stale) into the root module and add
  `make licenses`. The CD packaging step fails until this file exists.
- P-041 `NOTICE.txt`: replace the module list carried over from Munus with the combined list.
- P-042 `README.md`: move the launcher and tools from "planned" to features, with usage verified
  against a built binary.
- P-043 `intel/map.md`: replace the planned layout with the actual one.

#### Phase 5 — First release

- P-050 `make release VERSION=v0.1.0`; confirm `cd.yml` is green, every archive holds the binary,
  `LICENSE`, `NOTICE.txt` and `THIRD_PARTY_LICENSES`, and
  `gh attestation verify <archive> --repo jabbott-iii/Capsus` passes.

#### Ongoing — syncing a tool release

```sh
git subtree pull --prefix=Munus https://github.com/jabbott-iii/Munus.git <new-tag> --squash
```

Then update the bundled version (P-011), run `go mod tidy` at the root, review `make deps-drift`,
regenerate licenses (P-040), and run `make check` and `make smoke`.

### Risks

- **R-1 Dependency version skew.** The root build selects the highest required version of each
  shared module (minimal version selection), so the binary can run a tool with newer dependencies
  than its own tests used. Reproduced with stand-in modules on 2026-10-11. Mitigation:
  `make deps-drift` before each sync, P-004, the combined-binary smoke test, and govulncheck on the
  root module (`intel/cybersec.md` SEC-002).
- **R-2 Shared databases.** An older tool version inside Capsus can open a database written by a
  newer standalone binary. Mitigation: sync Capsus soon after each tool release; the tools keep
  schema changes additive (Munus' pull-request template requires it).
- **R-3 Release targets inside tool folders.** `make -C Munus release VERSION=…` would tag and push
  Capsus and trigger the Capsus CD workflow. `make tool` refuses release targets
  (`intel/cybersec.md` SEC-001).
- **R-4 Process-wide state.** The tools now share one process. Cryptare and Salus register signal
  handlers per call and stop them on return (checked on 2026-10-11); Munus writes errors to
  `os.Stderr` directly. Cryptare's password prompt calls `os.Exit` on Ctrl+C
  (`pkg/logic-cli.go`, `readTerminalPassword`), which ends the whole `capsus` process and skips the
  launcher's deferred clean-up (P-002). P-023 covers running two tools in one process.
- **R-5 Local edits in tool folders** are lost or conflict on the next `git subtree pull` (D-7).
- **R-6 CI is red until Phases 1–3 land.** The workflows target the post-import layout; the CI
  "Repository checks" job (`make layout`) names what is missing. The workflows copied from Munus failed before this change as
  well, because Capsus had no Go module.

### Validation

A phase is done when `make check-all` passes locally, the CI, Security and Docker workflows are green
on every operating system, and `make smoke` passes; Phase 5 also needs a green CD run with verified
attestations.
