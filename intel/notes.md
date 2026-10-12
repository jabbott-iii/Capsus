# Engineering Notes & Open Questions

## 1. Multi-module behaviour (checked 2026-10-11)

Checked with dependency-free stand-in modules laid out as Plan 1 proposes, using Go 1.24 (the
Go 1.26 toolchain and the module proxy were not reachable from the environment used, so the real
tools were not built):

- A root module with `require … v0.0.0` and `replace … => ./<Tool>` builds from the local folders,
  and `go mod tidy` adds the tools' dependencies to the root `go.mod` as indirect requirements.
- `./...` at the root matches only root packages, in module mode and in workspace mode (a
  `go.work` listing all four modules, which also coexists with the `replace` directives). Hence the
  per-module loops in the `Makefile` and the module matrix in the workflows.
- `go test github.com/jabbott-iii/<Tool>/...` from the root runs a tool's tests with the root build
  list (stand-ins had no test-only dependencies). Not adopted yet: a tool's test-only dependencies
  may need checksums the root `go.sum` does not record (untested). Worth trying after Phase 2 as a
  direct check of `intel/plan.md` R-1.
- Minimal version selection: with one tool on `example.com/dep v1.0.0` and another on `v1.1.0`, the
  combined binary used `v1.1.0` for both; `make deps-drift` reports exactly that case.
- gosec v2.29.0 run at the root scans only the root module and reports no error for the skipped
  nested modules, even with explicit `./<Tool>/...` arguments or a workspace file; run inside a
  module, its SARIF paths are relative to that module (`intel/cybersec.md` SEC-003).

## 2. Carried-over files that still describe Munus

- `NOTICE.txt` lists Munus' direct dependencies; replace it with the combined list (plan P-041).
- `CODEOWNERS.txt`: GitHub reads only a file named exactly `CODEOWNERS` (repository root,
  `.github/` or `docs/`), so this file has no effect. Renaming it was not part of this change; see
  Q-6.

## 3. CI notes

- `1.26.x` with `check-latest` is used instead of `go-version-file`, because the three `go.mod`
  files declare different minimums (1.26.0, 1.26.0 with toolchain go1.26.9, 1.26.8); the latest
  patch release satisfies all of them and carries the security fixes.
- Only the launcher's coverage goes to Codecov: the tools' Go coverage profiles name files by import
  path, and all three tools have files such as `pkg/database.go`, which Codecov's path matching
  could attribute to the wrong folder.
- The Windows test rows run without the race detector, as in Cryptare.
- `dependabot.yml` has no `allow` restriction: Dependabot's default version updates already cover
  only direct dependencies, and the documentation does not say whether `allow` also filters security
  updates, which must keep reaching the indirect requirements (the tools' code) in the root
  `go.mod`.

## 4. Open questions for the maintainer

- Q-1 Accept decisions D-1 … D-9 in `intel/plan.md`, or change them before Phase 0?
- Q-2 Name of the embedding entry point (`pkg.Main` proposed) and the tool versions to release
  with it.
- Q-3 Import with `--squash` (one commit per import or sync) or with full upstream history?
- Q-4 Should Capsus publish a container image later (D-8 says no for the first release)?
- Q-5 Is a Codecov token (`CODECOV_TOKEN`) configured for the Capsus repository?
- Q-6 Rename `CODEOWNERS.txt` to `CODEOWNERS` so GitHub applies it (and pair it with branch
  protection requiring code-owner review for `.github/`, `Makefile` and `scripts/`)?
- Q-7 Munus' only tag is `v1.0.0`, but its README and `intel/history.md` describe v3.0.x releases.
  Which version line do Munus releases, and the version Capsus reports for Munus, follow?
- Q-8 Dependabot: is a root-level bump of a tool's dependency acceptable as a security stopgap
  (`intel/maint.md` §7), or should Capsus always wait for the upstream release?
