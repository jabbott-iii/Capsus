---
name: docs-sync
description: Bring the Salus documentation in line with the code. Use after a change to flags, defaults, checks, misconfig rules, output formats, exit codes, environment variables, dependencies, the Go or golangci-lint version, workflows, or the file layout, or when asked to audit the docs.
argument-hint: "[audit]"
allowed-tools:
  - Bash(bash .claude/skills/docs-sync/docs-drift.sh)
  - Bash(git status *)
  - Bash(git diff *)
  - Bash(git log *)
  - Read
  - Grep
  - Glob
---

# Documentation sync

Mode: `$ARGUMENTS`

- Empty: update the documentation affected by the current change.
- `audit`: review all documentation against the code and report; edit
  nothing unless the maintainer then asks.

Follow `.claude/rules/docs.md` and the Documentation table in `CLAUDE.md`.

## 1. Run the mechanical check

From the repository root:

```bash
bash .claude/skills/docs-sync/docs-drift.sh
```

It is read-only. Exit `1` lists problems, one per line, tagged by kind:
`retired-ref` (links to the removed `intel/` documents or `AGENTS.md`),
`link`, `anchor`, `version` (Go or golangci-lint), `notice`, `check-key`,
`misconfig`, `env`, `flag`, and `structure` (README project tree). Record
the output; step 5 compares against it.

## 2. Find the scope

- Change mode: read `git status --short`, `git diff HEAD`, and, on a
  branch, `git diff main...HEAD`. List every user-visible effect: new or
  changed flags and defaults, check results, output fields, exit codes,
  messages that docs quote, platforms, files, dependencies, versions.
- Audit mode: every file in the Documentation table.

## 3. Check what the script cannot

Read the source of truth for each item in scope:

- Flag defaults in the README against the `default*` constants in
  `pkg/health.go` and the flag declarations in `pkg/logic-cli.go`.
- The "What each check reports" table against the check functions
  (`pkg/health*.go`), and the platform table against the per-OS files.
- JSON field lists against the `json:"..."` struct tags in `pkg/health.go`,
  `pkg/report.go`, `pkg/logic-cli.go`, and `pkg/scan-history.go`.
- Nagios, Prometheus, and JUnit examples against `pkg/report-formats.go`
  and the golden tests in `pkg/report-formats_test.go`.
- Exit code statements against `pkg/report.go` and `pkg/main.go`.
- Install, release, attestation, and image-tag instructions against
  `.github/workflows/cd.yml`; container notes against `Dockerfile` and
  `.github/workflows/docker.yml`.
- Validation commands in `README.md`, `CONTRIBUTING.md`, and the pull
  request template against `Makefile` and `.github/workflows/ci.yml`.
- Whether the change needs an "Upgrading from" note (it can raise exit
  codes on hosts that passed before, or changes parsed output).

Where an example's output matters, build and run it with `SALUS_DB_PATH`
set to a temporary file. If you cannot build (no matching Go toolchain or
no C compiler), say so instead of presenting output as verified.

## 4. Edit or report

- Change mode: update the docs this change affects, in the same change.
  Fix script findings in the lines you are already editing. List other
  pre-existing findings in the report instead of fixing them, unless the
  maintainer asked for a cleanup.
- Audit mode: report a table with file and line, the problem, the source
  of truth, and the proposed fix. Group `retired-ref` findings by the file
  that should replace the reference (see the Documentation table).

## 5. Verify and report

Re-run `docs-drift.sh`. Report which findings you fixed, which remain and
whether they predate this change, which manual checks you did, and which
examples you ran or could not run.
