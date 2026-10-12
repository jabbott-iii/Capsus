# Security Requirements & Remediation Tracking

Security requirements for Capsus, identified issues, and their remediation status. Never delete an
item; closed items are the audit trail. Item format and rules: `AGENTS.md` (Security Issue Tracking).

## Requirements

- **Inherited controls.** Each tool's `intel/cybersec.md` (inside `Munus/`, `Cryptare/`, `Salus/`
  once imported) stays authoritative for that tool's code. Capsus must not bypass or weaken those
  controls: it runs each tool only through the tool's own `pkg.Main` (`intel/maint.md` §4), so file
  permissions, input limits, terminal sanitising, lazy database opening, untrusted-database checks,
  interrupt clean-up and exit codes behave as in the standalone binary.
- **Upstream first.** Security fixes to tool code are made in the tool's repository and arrive with
  `git subtree pull`; never patch a tool folder in Capsus (R-5 in `intel/plan.md`).
- **No key material or databases in git.** No `*.db`, `*.db-journal`, `*.db-wal`, `*.db-shm` or
  `*.ckey` file is tracked anywhere in the repository (`make keys-check`, CI "Repository checks"
  job; Cryptare SEC-003 carried over).
- **Scans cover what ships.** Static analysis covers every module; vulnerability scanning covers the
  root module's build list, which is what the release binary contains (SEC-002, SEC-003).
- **Dependency alerts for tool code** are fixed upstream (`intel/maint.md` §7): close Dependabot pull
  requests against tool folders; a root-level bump is a stopgap only for a vulnerability govulncheck
  reports as reachable.
- **Pipeline hardening.** Workflow actions are pinned by commit SHA; downloads are pinned by SHA-256;
  container base and builder images are pinned by digest (planned Dockerfile, P-030, as in Cryptare
  and Salus);
  workflows default to `contents: read`; only the release job can write contents; build provenance
  is attested for every release archive; release builds use clean module and build caches and the
  latest Go 1.26 patch release.
- Treat changes to workflows, the `Makefile`, `scripts/` and the embedding contract as
  security-sensitive and flag them for review.

## Inherited items still open upstream (2026-10-11)

Tracked in the tool repositories, listed here because they affect what Capsus would ship:

- Cryptare SEC-013 (container base images), SEC-020 (release provenance) and SEC-021 (Go standard
  library `os.Root` flaw on Windows, GO-2026-6604): In Progress. SEC-021 also applies to Capsus'
  Windows binaries: the root `go.mod` must carry `toolchain go1.26.9` or later itself, because the
  go command ignores Cryptare's `toolchain` line (plan D-9); CI and release builds use the latest
  Go 1.26 patch release, and govulncheck runs on the root module.
- Salus SEC-003 (`.dockerignore`) and SEC-005 (dependency vulnerability scanning): In Progress.
- Munus: all 19 items Closed.

## Items

### SEC-001 — Tool release targets act on the Capsus repository

- **Status:** In Progress
- **Affected component:** `Makefile`; the imported tools' `Makefile`s (`tag`, `push-tag`, `release`)
- **Risk:** Inside Capsus, `make -C Munus release VERSION=v1.0.0` (or delegating to it) creates and
  pushes a tag on the Capsus repository. A `v*` tag starts `cd.yml`, which builds and publishes a
  Capsus release from whatever commit is checked out, without review, and the tag name collides
  with the tool's own version line.
- **Required remediation:** (1) `make tool` delegates only an allow-list of non-release targets
  (done 2026-10-11). (2) `intel/maint.md` §3, `CONTRIBUTING.md` and the pull-request template forbid
  release targets inside tool folders (done 2026-10-11). (3) Maintainer adds a GitHub tag ruleset
  restricting who can create `v*` tags.
- **Validation:** `make tool TOOL=Munus TARGET=release` is refused (checked with stand-in modules on
  2026-10-11; repeat after Phase 1 with the real tool folders); the tag ruleset is visible in the
  repository settings.
- **Resolution:** —

### SEC-002 — Vulnerability scanning must cover the dependency set that ships

- **Status:** In Progress
- **Affected component:** `.github/workflows/security.yml` (`govulncheck`), `Makefile` (`vuln`,
  `deps-drift`)
- **Risk:** The release binary is built from the root module, whose build list takes the highest
  version of each shared module across the three tools. Each tool's own govulncheck run checks the
  versions in that tool's `go.mod`, not the combination Capsus ships; running govulncheck only
  inside the tool folders would report on a dependency set that is never released.
- **Required remediation:** Run govulncheck on the root module, whose call graph starts at the
  `capsus` entry point and reaches the tool code it calls, with the versions that ship (workflow and
  `make vuln`, done 2026-10-11). Review `make deps-drift` on every sync.
- **Validation:** A green govulncheck run on the root module after Phase 2 of `intel/plan.md`.
- **Resolution:** —

### SEC-003 — Static analysis does not enter nested modules by default

- **Status:** In Progress
- **Affected component:** `.github/workflows/security.yml` (gosec, CodeQL), `Makefile` (`sec`)
- **Risk:** The tools are nested Go modules. Checked with gosec v2.29.0 on stand-in modules on
  2026-10-11: run from the repository root, gosec scans only the root module and silently skips
  nested modules, even when they are named explicitly or a workspace file lists them. Its SARIF
  paths are relative to the directory it runs in. The workflow copied from Munus ran gosec and
  CodeQL once at the root, so tool code would not have been scanned in Capsus.
- **Required remediation:** Run gosec in every module (matrix job; `make sec` loops), rewrite each
  SARIF file's paths to repository-relative paths and upload it under its own category
  (`gosec-<module>`); build every module explicitly for CodeQL (`build-mode: manual`). Done in the
  workflow on 2026-10-11.
- **Validation:** After Phase 2, Code Scanning lists the gosec categories `gosec-capsus`,
  `gosec-munus`, `gosec-cryptare` and `gosec-salus`, alerts open on files under the tool folders,
  and the CodeQL run reports scanned Go files under `Munus/`, `Cryptare/` and `Salus/`.
- **Resolution:** —
