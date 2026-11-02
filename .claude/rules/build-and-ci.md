---
paths:
  - "Makefile"
  - ".github/**"
  - "scripts/**"
  - "Dockerfile"
  - ".dockerignore"
  - ".gitignore"
  - ".gitattributes"
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
  (Rechecked with the imported tools: `go list ./...` at the root lists only the launcher's
  packages, with or without a `go.work` that uses all four modules.)
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
  on matching runners, windows/arm64 with the pinned, checksum-verified llvm-mingw. The Linux
  binaries get the Go of the digest-pinned builder, not setup-go's: Dependabot's docker updates
  move the digest forward, and `/release-prep` checks that it carries the latest Go 1.26 patch
  release (D-9).
- The version is stamped with `-ldflags "-X main.version=<version>"` (`make build`, `ci.yml`,
  `cd.yml`). `cd.yml` refuses version strings outside `^[0-9A-Za-z._+-]+$`; `make tag` and
  `make release` need `vMAJOR.MINOR.PATCH`.
- `go install …@latest` is unsupported because of the root `replace` directives; never document
  it as an install path.

## Workflows

- Every action is pinned to a full commit SHA with a `# vX.Y.Z` comment. Downloads are pinned by
  version and SHA-256 (llvm-mingw in `cd.yml`: bump `LLVM_MINGW_VERSION` and
  `LLVM_MINGW_SHA256` together). The `Dockerfile` pins its builder image by digest (P-030);
  the smoke-test images `debian:11` and `alpine:3.24` in `docker.yml` and `cd.yml` are pinned by
  tag and move with the Alpine release.
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
- The root `.gitattributes` keeps `go.mod` and `go.sum` LF on every checkout, in every module:
  `go mod tidy -diff` (`make tidy-check`, the "Verify dependencies" step of `ci.yml`) compares
  `go.sum` byte for byte, and Windows runners check it out with CRLF otherwise. Never remove it.
- The launcher uses only modules the tools already compile in (Bubble Tea, Lip Gloss, Cobra). A
  new module needs the maintainer's approval with a stated reason, a license review, a
  `NOTICE.txt` entry and `make licenses`. Every dependency change or tool sync needs
  `make licenses`, since a bundled tool's changed `LICENSE` or `NOTICE` or a module's changed
  license text also changes `THIRD_PARTY_LICENSES`; one that adds or removes a module also needs
  the `NOTICE.txt` lines.
- Minimal version selection gives the binary the highest version of each shared module, which can
  be newer than a tool's own tests used (R-1). Run `make deps-drift` at every sync and fix drift
  upstream (P-004). Running a tool's tests from the root
  (`go test github.com/jabbott-iii/<Tool>/...`) uses the versions capsus ships and so checks R-1
  directly; with the Phase 1 imports the root `go.sum` has the checksums those tests need, but no
  make target or workflow runs them that way.
- Dependabot: Go version updates cover the root module's direct dependencies and ignore
  `github.com/jabbott-iii/*`. There is deliberately no `allow` filter, so security updates still
  reach the indirect requirements. Close pull requests against tool folders and fix upstream; a
  root-level bump is a stopgap only for a vulnerability govulncheck reports as reachable (Q-8).
  Dependabot pull requests get the same review and checks as any other and are never merged
  automatically. A Go update that adds or removes a module or changes a license text fails
  `ci.yml`'s capsus test job until `make licenses` and any `NOTICE.txt` lines are committed on
  its branch.

## Release build pieces

- **`Dockerfile` (P-030, written).** A builder stage and the `static` stage (`FROM scratch`,
  output `/capsus`), mirroring Munus' static build: `golang:1.26-alpine3.24` pinned by digest
  with a pinned `build-base`, `-tags sqlite_omit_load_extension,osusergo,netgo` and
  `-linkmode external -extldflags '-static -Wl,-z,stack-size=8388608'`. The 8 MiB stack request
  matters: musl gives new threads 128 KiB, and the tools' SQLite calls run on those threads.
  `docker.yml` and `cd.yml` build this stage and check `GNU_STACK` and the absence of a program
  interpreter. The builder stays on the Alpine release they test on (`alpine:3.24`), and its Go
  at or above the root `go.mod`'s `go` line, since the image sets `GOTOOLCHAIN=local`: move the
  tag, the digest and the `build-base` pin together, and with a new Alpine release
  `tools/licenses`' musl text and constants (Licenses below). Dependabot refreshes only the
  digest; the Alpine release and the `build-base` pin move by hand. `.dockerignore` keeps VCS,
  agent and CI files, Go workspace files, databases, key material and build output out of the
  context. No runtime stage and no image (D-8).
- **Licenses (P-040, P-041, written).** `THIRD_PARTY_LICENSES` holds the license texts for the
  release binaries: Go, SQLite, musl (Linux), the MinGW-w64 runtime (Windows) and every module
  that `go list -deps` reports for a release platform, the bundled tools with their `LICENSE`
  and `NOTICE` files included. `tools/licenses`, a stdlib-only adaptation of Munus' generator,
  writes it (`make licenses`); its test, which `make test`, `make race` and CI's capsus test
  job run, fails while the file is stale or the `NOTICE.txt` module list doesn't match the
  compiled modules. It runs the go command with `GOPROXY=off`, so the module cache must already
  hold every release platform's modules: CI's "Verify dependencies" step and `make check`'s
  `tidy-check` fill it; otherwise run `go mod download` before `make test` or `make race`.
  Never edit `THIRD_PARTY_LICENSES` by hand, and list modules in `NOTICE.txt`
  in the existing format (module path, copyright holder, license, no version).
  `tools/licenses/musl-COPYRIGHT` (musl 1.2.6) and the generator's `muslVersion` and
  `muslAlpineRelease` follow the `Dockerfile`'s Alpine release, and `linuxReleaseTags` its
  build tags: the test fails until they agree. `mingw-w64-runtime.txt` is mingw-w64 14.0.0's
  runtime license. `cd.yml` packaging fails if `THIRD_PARTY_LICENSES` is missing.

## Open security items

Carried over from the retired `intel/cybersec.md` (last version:
`git show 7cb3629:intel/cybersec.md`); this table is their home now, and all are In Progress.
Close an item only after its validation has passed: mark it Closed with the resolution, keep the
row, and say so in the completion report. Report a newly found issue to the maintainer privately
instead of adding a row.

| Item | Done | Validation still needed |
|---|---|---|
| SEC-001 Tool release targets act on the Capsus repository: `make -C <Tool> release` would tag and push Capsus and start `cd.yml` | `make tool` allow-list; release targets forbidden in `CONTRIBUTING.md`, the PR template and `AGENTS.md`; `make tool TOOL=Munus TARGET=release` stops with "TARGET must be one of" (checked on 2026-11-01 in a clone without remotes) | The maintainer adds a GitHub tag ruleset restricting who creates `v*` tags |
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
- a `COPY <Tool>/go.mod <Tool>/` line in the `Dockerfile`, before `go mod download`;
- `NOTICE.txt` (the tool's module under "Bundled tools", plus any new dependencies), then
  `make licenses`; a C library that `tools/licenses` doesn't cover yet goes into it;
- Dependabot's `ignore` list, when the module path is not under `github.com/jabbott-iii/`;
- the README, `CONTRIBUTING.md` and the PR template, which name the tool folders;
- `AGENTS.md` (tool table, layout, tool-folder rules), `CLAUDE.md`, the `paths:` of
  `.claude/rules/tool-folders.md`, an `Edit(/<Tool>/**)` deny rule in `.claude/settings.json`,
  and the skills and agent that list the tool folders (`pre-pr`, `tool-sync`,
  `change-reviewer`).
