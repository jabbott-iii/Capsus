# Architecture & Maintainability Guidance

Authoritative architecture and maintainability guidance for Capsus. `CONTRIBUTING.md` must stay
consistent with this file. The import of the tools is planned, not done: see `intel/plan.md`
(Plan 1) for the decisions referenced here (D-n) and their status.

## 1. Purpose

Capsus is a toolbox: one binary that bundles Munus, Cryptare and Salus. Running `capsus` opens a
menu TUI listing the tools; selecting a tool launches that tool's own TUI, and a tool without a TUI
says so. `capsus <tool> [args...]` runs a tool's CLI unchanged.

## 2. Layout and module boundaries

- The root module `github.com/jabbott-iii/Capsus` holds only the launcher: `main.go` and
  `internal/launcher` (tool registry and menu TUI).
- Each tool lives in a folder named after it (`Munus/`, `Cryptare/`, `Salus/`), imported as a
  `git subtree` (D-2) with its own `go.mod`. The root `go.mod` requires the three modules at
  `v0.0.0` and replaces them with the local folders (D-3).
- No `go.work` is committed (`.gitignore` excludes it); the `replace` directives make a plain
  checkout build.
- `./...` never enters a nested module. Every repository-wide check loops over the modules
  (`MODULES` in the `Makefile`, the `module` matrix in `ci.yml`).
- Package names collide (every tool uses package `pkg`), so imports use aliases:
  `munus "github.com/jabbott-iii/Munus/pkg"`.

## 3. Upstream first

- Do not edit files under `Munus/`, `Cryptare/` or `Salus/` in Capsus. Change the tool's own
  repository, release it, then sync (D-7). Local edits conflict with or are overwritten by the next
  `git subtree pull`.
- The tool folders keep their own `AGENTS.md`, `intel/`, tests and `Makefile`. A tool's `AGENTS.md`
  and `intel/` documents govern that tool's code; this file governs the repository around them.
- The tools' nested `.github/` folders are inert: GitHub reads workflows, Dependabot settings,
  templates and code owners only from the repository root.
- Never run a tool's release targets (`tag`, `push-tag`, `release`) inside its folder: they act on
  the Capsus repository. `make tool` allows only non-release targets (`intel/cybersec.md` SEC-001).

Sync procedure for a new tool release:

```sh
git subtree pull --prefix=<Tool> https://github.com/jabbott-iii/<Tool>.git <tag> --squash
```

then update the tool's bundled version in the launcher registry, run `go mod tidy` at the root,
review `make deps-drift`, regenerate the license files, and run `make check` and `make smoke`.

## 4. Embedding contract

- Capsus calls each tool only through `pkg.Main(ctx, version, args) int` (D-4) plus the registry
  metadata it owns (name, description, has-TUI, bundled version). It does not reach into a tool's
  other exported API.
- `Main` keeps the tool's standalone behaviour: its own flag parsing, database location and
  environment variable, error output and exit codes. Capsus passes arguments unchanged and returns
  the tool's exit code unchanged.
- One Bubble Tea program runs at a time: the menu exits before a tool's TUI starts and is shown again
  when the tool returns.
- Adding a tool: get `pkg.Main` into the tool upstream, `git subtree add` it into a folder named
  after it, add its `require`/`replace` lines, add one registry entry, and add it to: `TOOLS` in the
  `Makefile`; the `module` matrix in `ci.yml`; the gosec matrix and the CodeQL build loop in
  `security.yml` (otherwise its code is not scanned, `intel/cybersec.md` SEC-003); the database
  environment variables in the container smoke steps of `docker.yml` and `cd.yml`;
  `scripts/smoke.sh`; `intel/map.md`; and the README.

## 5. Build

- CGO is required: every tool stores data with `mattn/go-sqlite3`. A `CGO_ENABLED=0` build compiles
  but cannot open a database.
- Release Linux binaries are fully static musl builds from the `Dockerfile` `static` stage (planned,
  P-030), linked with an 8 MiB thread-stack request (`-Wl,-z,stack-size=8388608`): musl gives new
  threads 128 KiB, and SQLite calls run on those threads. CI checks the `PT_GNU_STACK` size and that
  the binary has no program interpreter.
- macOS and Windows binaries are built natively on matching runners; windows/arm64 uses a pinned,
  checksum-verified llvm-mingw toolchain.
- Go: the root `go.mod` carries the highest of the tools' `go` and `toolchain` lines (the go
  command ignores `toolchain` lines in dependencies); CI and release builds use the latest Go 1.26
  patch release (D-9).
- The version is stamped with `-ldflags "-X main.version=<tag>"`.

## 6. Quality gates

- `make check` runs what `ci.yml` runs, in every module: layout and key-material checks, gofmt,
  `go mod tidy -diff`, `go vet`, golangci-lint v2.13.2, tests with the race detector, and the
  combined-binary smoke test (`scripts/smoke.sh`). `make check-all` adds gosec, govulncheck and
  actionlint (`security.yml`).
- `make fmt` formats only the launcher's own files; tool folders are checked, never rewritten.
- Each module's own golangci-lint configuration applies inside that module (Salus has one).
- CI runs on Linux, macOS and Windows. The race detector runs on Linux and macOS only.

## 7. Dependencies

- The launcher uses only modules the tools already compile in (Bubble Tea, Lip Gloss, Cobra). A new
  third-party module needs a recorded decision in `intel/notes.md`.
- Minimal version selection makes the binary use the highest version of each shared module, which
  can be newer than a tool's own tests used (`intel/plan.md` R-1). Run `make deps-drift` before
  each sync and resolve drift upstream.
- Dependabot version updates cover the root module's direct dependencies; tool dependencies are
  updated upstream and arrive with the next sync. Dependabot security updates can also propose bumps
  of the root's indirect requirements and of the tools' own `go.mod` files. Fix a tool's manifest
  upstream and close pull requests against tool folders; a root-level bump is an acceptable stopgap
  for a vulnerability govulncheck reports as reachable, and `make deps-drift` will show it until the
  tools catch up.

## 8. Licensing and release

- Every source file keeps the Apache-2.0 header. Release archives contain `LICENSE`, `NOTICE.txt`
  and `THIRD_PARTY_LICENSES` (the latter generated for the combined binary, P-040); regenerate both
  after any dependency change.
- Releases are `vX.Y.Z` tags on Capsus (`make release VERSION=vX.Y.Z`), built by `cd.yml` with
  build-provenance attestations. Capsus publishes no container image (D-8).
