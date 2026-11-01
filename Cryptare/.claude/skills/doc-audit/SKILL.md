---
name: doc-audit
description: Audit all of Cryptare's documentation against the repository and report drift (broken links, stale versions, wrong commands, flags or defaults). Report first; edit only what the user approves.
disable-model-invocation: true
argument-hint: "[file or section to limit the audit to]"
---

# Documentation audit

Scope: `$ARGUMENTS` (empty means every document below).

Documents: `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`,
`pkg/testdata/golden/README.md`, `CLAUDE.md`, `.claude/rules/*.md`,
`.claude/skills/*/SKILL.md`, `.claude/agents/*.md`, and the explanatory comments in
`Makefile`, `Dockerfile`, `.github/dependabot.yml` and `.github/workflows/*.yml`.

## Steps

1. **Mechanical checks.** Run both scripts from the repository root and keep their
   output:
   - `bash .claude/skills/doc-audit/check-links.sh` lists relative Markdown links and
     backticked repository paths that don't exist.
   - `bash .claude/skills/doc-audit/check-versions.sh` prints the Go toolchain and tool
     versions wherever they are named, and the latest tag next to `SECURITY.md`'s
     supported line.
2. **Claims.** For each document, list its checkable claims: commands and make
   targets, flags and defaults, limits and constants, file modes, paths and
   environment variables, versions, release asset names, platforms, test and function
   names. Verify them against the code, Makefile, workflows and Dockerfile. Split the
   work across `docs-verifier` agents, one per document, so each claim is checked
   independently.
3. **Known debt.** Confirm the items in `CLAUDE.md` → "Known documentation debt" are
   still present, and add any new ones.
4. **Report** one table, most severe first:

   | File:line | Claim | Evidence (file:line or command output) | Severity | Proposed fix |
   |---|---|---|---|---|

   Severity: *wrong* (a user following it fails or is misled about safety), *stale*
   (outdated but harmless), *broken link*, *style*.
5. **Stop.** Ask which fixes to make. Then apply only those, as small edits, and
   re-run step 1.

Never "fix" a doc by changing code, and never touch golden fixtures or upgrade notes
for past releases without the owner's say-so.
