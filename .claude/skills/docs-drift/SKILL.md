---
name: docs-drift
description: Audits Capsus documentation (README, CONTRIBUTING, SECURITY, PR template, NOTICE.txt, AGENTS.md, CLAUDE.md and .claude/) against the Makefile, workflows, smoke test, tags and code, and reports every mismatch with a proposed fix. Use when asked to check or refresh the docs, after a Plan 1 phase lands, after a tool sync, or before a release.
argument-hint: "[all|readme|contributing|security|instructions|references]"
allowed-tools: Bash(git ls-files *) Bash(git grep *) Bash(git log *) Bash(make help) Bash(make build)
---

# Documentation drift audit

Scope: `$ARGUMENTS` (default `all`). This is an audit first: read, compare, report. Edit files
only after the maintainer approves the findings, or if they asked you to fix drift directly.
Never commit.

The repository is the source of truth: the code, the `Makefile`, the workflows and
`scripts/smoke.sh`. When a doc and the repository disagree, the fix is usually the doc; if the
code looks wrong instead, report it as a possible bug and don't change behaviour.

## Checks

For each finding, cite the doc location and the repository location.

**Status (readme)**: what the README calls planned against what exists (`git ls-files`:
`go.mod`, `main.go`, `internal/`, the tool folders, `Dockerfile`, `THIRD_PARTY_LICENSES`).
Something described as available that doesn't exist is **wrong**; something that exists but is
still marked planned needs P-042's check against a built binary before it moves to the features.

**CLI (readme)**: once `make build` works, run `bin/capsus --help`, `bin/capsus list` and
`bin/capsus --version`, with `MUNUS_DB_PATH`, `CRYPTARE_DB_PATH` and `SALUS_DB_PATH` pointing
into a temporary folder, and compare them with the README and the CLI contract in `AGENTS.md`
(D-5). If it can't build, say so and compare with the code instead.

**Make targets and tool versions (readme, contributing)**: `make help` against README "Testing
and quality checks", `CONTRIBUTING.md` and the PR template; the Go, golangci-lint, gosec,
govulncheck and actionlint versions in the `Makefile`, the workflows and the docs agree.

**Security policy (security)**: `SECURITY.md`'s supported versions against the release tags
(`git for-each-ref --sort=-v:refname --format='%(refname:short)' refs/tags`), and whether it
still holds GitHub's placeholder text instead of a reporting process (**missing**). Propose
changes; never edit the table without the maintainer's confirmation.

**Configuration (readme)**: the data variables in the README against the tools' code once they
are imported (`Munus/pkg/database_path.go`, `Cryptare/pkg/database_path.go`,
`Salus/pkg/database-path.go`).

**References (references)**: every path a doc or comment names exists, for example
`git grep -nE 'intel/|AGENTS\.md|CLAUDE\.md|\.claude/'`. References to the removed `intel/`
documents are known debt (`AGENTS.md`, "Known documentation debt"): report each with where its
content lives now. IDs: `D-n`, `P-0nn`, `R-n` and `Q-n` in `AGENTS.md`; `SEC-00n` in
`.claude/rules/build-and-ci.md`. Sections of `intel/maint.md`: §3 (upstream first, sync) →
`AGENTS.md` "Tool folders" and `/tool-sync`; §4 (embedding contract, adding a tool) →
`.claude/rules/launcher.md` "Dispatch" and `/add-tool`; §5 (build) and §7 (dependencies) → the
"Toolchain" and "Dependencies" sections of `.claude/rules/build-and-ci.md`. Whole documents:
`intel/plan.md` and `intel/map.md` → `AGENTS.md`; `intel/cybersec.md` → `AGENTS.md` "Security"
and the open items in `.claude/rules/build-and-ci.md`; `intel/notes.md` → `AGENTS.md` (Q-n) and
`.claude/rules/build-and-ci.md`; `intel/golang.md` → `.claude/rules/go.md`; `intel/history.md`
→ git history only. IDs that appear inside a tool folder belong to that tool.

**Instructions (instructions)**: `AGENTS.md`, `CLAUDE.md`, `.claude/rules/*.md`, the skills and
the agents. Every command, make target, path and version they name exists; `paths:` globs match
the files they are meant for (globs for planned files match nothing until the files land); the
layout tree and tool table in `AGENTS.md` match the repository; the dated "Current state" in
`AGENTS.md` is still true.

## Report

A table, most severe first:

| # | Severity | Doc location | Says | Repository says (location) | Proposed fix |
|---|---|---|---|---|---|

Severity: **wrong** (a reader following the doc fails or is misled), **missing** (behaviour
exists but is undocumented), **stale** (outdated reference or wording, harmless), **code?** (the
code may be the problem). Then list what could not be checked and why, and offer to apply the
doc fixes as minimal edits in the existing style.
