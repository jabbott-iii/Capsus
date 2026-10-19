---
name: pre-pr
description: Runs the Capsus checks that the current uncommitted changes need and writes the Validation section of the pull request template. Use before declaring a change done, before opening a pull request, or when asked whether work is ready.
argument-hint: "[base-ref, default HEAD]"
allowed-tools: Bash(git status *) Bash(git diff *) Bash(git ls-files *) Bash(git log *) Bash(go version) Bash(go env *) Bash(make layout) Bash(make keys-check) Bash(make fmt-check) Bash(make check) Bash(make check-all) Bash(make deps-drift) Bash(make actionlint) Bash(make build) Bash(make smoke)
---

# Pre-PR validation

Compare against `$ARGUMENTS` if given, otherwise against `HEAD` (uncommitted work, untracked
files included). After a tool sync, the base is the commit before the maintainer's subtree merge
(`HEAD^1` right after it), so the import is in range. Don't commit, push or open a pull request.
Report problems; fix them only if the user asks.

## 1. Scope

Run `git status --short` and `git diff --stat <base>`, then classify the changed paths:

- **launcher**: root `*.go`, `internal/**`
- **tool folder**: anything under `Munus/`, `Cryptare/` or `Salus/`. Unless it comes from the
  maintainer's `git subtree` merge, stop and report it: those folders change only upstream
  (`AGENTS.md`, Tool folders).
- **modules**: `go.mod`, `go.sum`
- **build**: `Makefile`, `scripts/**`, `Dockerfile`, `.dockerignore`, `tools/**`
- **workflows**: `.github/workflows/*`, `.github/dependabot.yml`
- **docs**: `*.md`, `.github/*.md`, `NOTICE.txt`
- **instructions**: `AGENTS.md`, `CLAUDE.md`, `.claude/**`

## 2. Environment

Record `go version`, `go env CGO_ENABLED` and `go env CC`. If Go is older than 1.26, CGO is off
or there is no C compiler, skip the Go checks, say so, and continue with the rest.

Run `make layout`. Until the tool folders and the root `go.mod` exist (Plan 1 Phases 1 and 2)
it fails and every module target stops there: report the module checks as "not runnable yet
(layout)", not as a failure of this change.

## 3. Checks (only what applies, in this order)

| Condition | Run |
|---|---|
| always | `make keys-check` |
| launcher | Every changed `.go` file starts with the Apache-2.0 header; `make check` |
| modules | `make check-all`; `go.sum` changed only for the modules involved; `make deps-drift` (paste its output) |
| tool sync (a subtree merge is in range) | `make check-all`, `make deps-drift`, `make smoke` |
| build, workflows | `make check-all`; every new or changed `uses:` pinned to a full commit SHA with a `# vX.Y.Z` comment; no `permissions` widened |
| docs | Every command, flag, path and version in the changed lines exists in the repository |
| instructions | Frontmatter parses; `paths:` globs match the intended files (planned files may not exist yet); every named make target exists in the `Makefile` |

Don't count a skipped or partial run as passed. `make lint`, `make sec`, `make vuln` and
`make actionlint` use `go run …@<version>` and need network access to the module proxy.

## 4. Review the diff

Read the full diff against `AGENTS.md` and the `.claude/rules/` files whose `paths:` match:

- Behaviour changes have deterministic tests; databases live under `t.TempDir()`.
- The launcher calls tools only through `Main`, passes arguments and exit codes unchanged, and
  changes no process-wide state to steer a tool.
- Nothing weakened: tests, scans, pins, permissions, `make tool`'s allow-list.
- Docs updated for user-visible changes, or "no doc impact" justified.
- No databases, key exports, coverage files, binaries (`bin/`, `dist/`, a `capsus` in the root)
  or secrets in the change.

## 5. Report

Write it in this shape so it can be pasted into the PR template. Tick a box only for a check
that ran and passed, and paste the decisive output lines.

```markdown
## Validation

- [ ] `make check` (layout, keys-check, fmt-check, tidy-check, vet, lint, race, smoke) — <result>
- [ ] `make check-all` (adds gosec, govulncheck, actionlint) — <result, or "not required">
- [ ] `make deps-drift` — <output, or "not required">
- [ ] Manual check of the menu and of launching each tool with a binary built by `make build` — <what was exercised, or "not done">

**Tested on:** <OS> / <go version output>

**Not run:** <each check and why>
**Findings:** <problems from step 4, or "none">
**Doc impact:** <files updated, or "none" with the reason>
```
