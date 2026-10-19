---
paths:
  - "Makefile"
  - ".github/**"
  - "scripts/**"
  - "Dockerfile"
  - ".dockerignore"
  - ".gitignore"
  - "go.mod"
  - "go.sum"
  - "tools/**"
  - "NOTICE.txt"
  - "THIRD_PARTY_LICENSES"
---

# Build, CI/CD, dependencies and licensing

Change these files only when the task needs it, flag every such change as security-sensitive in
the completion report, and say that only a GitHub run proves CI behaviour.

## Several modules

- Four modules: the root (the launcher) and `Munus/`, `Cryptare/`, `Salus/`. `go … ./...` at the
  root never enters a nested module, in module or workspace mode, so every repository-wide check
  loops over them: `TOOLS` and `MODULES` in the `Makefile`, the `module` matrix in `ci.yml`, the
  gosec matrix and the CodeQL build loop in `security.yml`. A new check must loop the same way.
  (Plan 1 verified this with Go 1.24 stand-in modules, not the real tools; recheck after Phase 2.)
- `make check` mirrors `ci.yml`, its smoke job included; `make check-all` adds the scans of
  `security.yml` and actionlint. Keep the `Makefile`, the workflows, the README's "Testing and
  quality checks", `CONTRIBUTING.md` and the PR template in step.
- Pinned tool versions move together wherever they appear: golangci-lint `v2.13.2` (`ci.yml`,
  `Makefile`); gosec `v2.29.0` and govulncheck `v1.8.0` (`security.yml`, `Makefile`; Dependabot
  doesn't update them); actionlint `v1.7.12` (`Makefile` only).
- Each module's own golangci-lint configuration applies inside it (Salus has `.golangci.yml`).
- `make fmt` rewrites only the launcher's files; `make fmt-check` checks every module.
- `TOOL_TARGETS`, the allow-list of `make tool`, must never gain `tag`, `push-tag`, `release` or
  a target that rewrites tracked files (`fmt`, `licenses`, `fuzz`) (SEC-001). `make tool`
  depends on `make layout`, so it only works once every module's `go.mod` exists.

## Toolchain

- CGO everywhere: the tools store data with `mattn/go-sqlite3`. A `CGO_ENABLED=0` build compiles
  but cannot open a database, so never disable CGO for a build that includes a tool. The race
  detector needs CGO too and runs on Linux and macOS only.
- Workflows use `go-version: "1.26.x"` with `check-latest: true`, because the three `go.mod`
  files declare different minimums. The root `go.mod` carries the highest `go` and `toolchain`
  lines of the tools (D-9). A Go minor upgrade is deliberate and touches `go.mod`, every
  workflow, the `Dockerfile` and the docs together.
- Release binaries: Linux from the `Dockerfile` `static` stage; macOS and Windows built natively
  on matching runners, windows/arm64 with the pinned, checksum-verified llvm-mingw.
- The version is stamped with `-ldflags "-X main.version=<version>"` (`make build`, `ci.yml`,
  `cd.yml`). `cd.yml` refuses version strings outside `^[0-9A-Za-z._+-]+$`; `make tag` and
  `make release` need `vMAJOR.MINOR.PATCH`.
- `go install …@latest` is unsupported because of the root `replace` directives; never document
  it as an install path.

## Workflows

- Every action is pinned to a full commit SHA with a `# vX.Y.Z` comment. Downloads are pinned by
  version and SHA-256 (llvm-mingw in `cd.yml`: bump `LLVM_MINGW_VERSION` and
  `LLVM_MINGW_SHA256` together). The planned `Dockerfile` pins its base and builder images by
  digest (P-030); the smoke-test images `debian:11` and `alpine:3.24` in `docker.yml` and
  `cd.yml` are pinned by tag and move with the Alpine release.
- Least privilege: top-level `permissions: contents: read`; in `cd.yml` only the tag-only
  `release` job has `contents: write`, and `package` adds `id-token: write` and
  `attestations: write`; in `security.yml` the CodeQL and gosec jobs add
  `security-events: write`. Checkouts use `persist-credentials: false`. Don't widen any of this
  or add `pull_request_target`.
- Release builds start from clean caches: `cache: false` for Go in `cd.yml`, and
  `docker build --pull` for the Linux binaries.
- Only the launcher's coverage goes to Codecov (`CODECOV_TOKEN`): the tools' profiles name files
  by import path, and all three have files such as `pkg/database.go`, which Codecov's path
  matching could attribute to the wrong folder. The Windows test rows run without `-race`.
- gosec runs in each module with `-no-fail`; SARIF paths are rewritten to repository-relative
  paths and uploaded under `gosec-<module>` (SEC-003). CodeQL builds every module
  (`build-mode: manual`). govulncheck runs on the root module, whose build list is what ships
  (SEC-002). Never remove or weaken a scan.
- Smoke steps capture a command's output before `grep`: with `pipefail`, `capsus … | grep -q`
  can fail with SIGPIPE.
- The only secrets used are `CODECOV_TOKEN` and `GITHUB_TOKEN`; never echo them.

## Dependencies

- The root `go.mod` requires the tools at `v0.0.0` with `replace … => ./<Tool>` (D-3); `go mod
  tidy` adds their dependencies as indirect requirements. No committed `go.work`.
- The launcher uses only modules the tools already compile in (Bubble Tea, Lip Gloss, Cobra). A
  new module needs the maintainer's approval with a stated reason, a license review, and
  `NOTICE.txt` and `THIRD_PARTY_LICENSES` updates.
- Minimal version selection gives the binary the highest version of each shared module, which can
  be newer than a tool's own tests used (R-1). Run `make deps-drift` at every sync and fix drift
  upstream (P-004). After Phase 2, running a tool's tests from the root
  (`go test github.com/jabbott-iii/<Tool>/...`) would check R-1 directly; it is untried, because
  test-only dependencies may lack checksums in the root `go.sum`.
- Dependabot: Go version updates cover the root module's direct dependencies and ignore
  `github.com/jabbott-iii/*`. There is deliberately no `allow` filter, so security updates still
  reach the indirect requirements. Close pull requests against tool folders and fix upstream; a
  root-level bump is a stopgap only for a vulnerability govulncheck reports as reachable (Q-8).
  Dependabot pull requests get the same review and checks as any other and are never merged
  automatically.

## Planned build pieces

- **`Dockerfile` (P-030).** A `static` stage that mirrors Munus' (`golang:1.26-alpine3.24` with
  a pinned `build-base`), with the builder image pinned by digest,
  `-tags sqlite_omit_load_extension,osusergo,netgo`,
  `-linkmode external -extldflags '-static -Wl,-z,stack-size=8388608'` and output `/capsus`. The
  8 MiB stack request matters: musl gives new threads 128 KiB, and the tools' SQLite calls run on
  those threads. `docker.yml` and `cd.yml` already build this stage and check `GNU_STACK` and the
  absence of a program interpreter. Add a `.dockerignore` with it. No image is published (D-8).
- **Licenses (P-040, P-041).** `THIRD_PARTY_LICENSES` for the combined binary (Go, SQLite, musl
  and every compiled module, including Cryptare's `golang.org/x/crypto`), generated by an
  adaptation of Munus' stdlib-only `tools/licenses` with a test that fails when the file is
  stale, run by a new `make licenses`. Never edit it by hand. `cd.yml` packaging fails until it
  exists. `NOTICE.txt` still lists Munus' modules and must become the combined list.

## Open security items

Carried over from `intel/cybersec.md`; all In Progress. Close an item only after its validation
has passed: mark it Closed with the resolution, keep the row, and say so in the completion
report. Report a newly found issue to the maintainer privately instead of adding a row.

| Item | Done | Validation still needed |
|---|---|---|
| SEC-001 Tool release targets act on the Capsus repository: `make -C <Tool> release` would tag and push Capsus and start `cd.yml` | `make tool` allow-list; release targets forbidden in `CONTRIBUTING.md`, the PR template and `AGENTS.md` | After Phase 2 (`make tool` needs every `go.mod`), `make tool TOOL=Munus TARGET=release` stops with "TARGET must be one of"; the maintainer adds a GitHub tag ruleset restricting who creates `v*` tags |
| SEC-002 Vulnerability scanning must cover the dependency set that ships | govulncheck on the root module (`security.yml`, `make vuln`) | A green govulncheck run on the root module after Phase 2; `make deps-drift` reviewed at every sync |
| SEC-003 Static analysis doesn't enter nested modules by default (gosec at the root skips them silently) | gosec in every module with repository-relative SARIF; CodeQL builds every module | After Phase 2, Code Scanning shows `gosec-capsus`, `gosec-munus`, `gosec-cryptare` and `gosec-salus`, alerts open on files under the tool folders, and CodeQL reports scanned files there |

The tools' own items that affect what Capsus ships were still open when the tools retired their
registers on 2026-10-13: Cryptare SEC-013 (container base images), SEC-020 (release provenance)
and SEC-021 (Go `os.Root` flaw on Windows, the reason for D-9's `toolchain` line); Salus SEC-003
(`.dockerignore`) and SEC-005 (dependency scanning). Those IDs belong to the tools, not to the
Capsus items above; they are tracked upstream.

## Adding a tool

`/add-tool` walks through it. Besides the subtree, the `require`/`replace` lines and the registry
entry, a new tool goes into:

- `TOOLS` in the `Makefile`; the `module` matrix in `ci.yml`; the gosec matrix and the CodeQL
  build loop in `security.yml` (otherwise its code is not scanned, SEC-003); the database
  variables in the container smoke steps of `docker.yml` and `cd.yml`; `scripts/smoke.sh`;
- Dependabot's `ignore` list, when the module path is not under `github.com/jabbott-iii/`;
- the README, `CONTRIBUTING.md` and the PR template, which name the tool folders;
- `AGENTS.md` (tool table, layout, tool-folder rules), `CLAUDE.md`, the `paths:` of
  `.claude/rules/tool-folders.md`, an `Edit(/<Tool>/**)` deny rule in `.claude/settings.json`,
  and the skills and agent that list the tool folders (`pre-pr`, `tool-sync`,
  `change-reviewer`).
