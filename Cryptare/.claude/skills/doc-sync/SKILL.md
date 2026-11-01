---
name: doc-sync
description: Bring Cryptare's documentation in line with a change to code, flags, behaviour, build, CI or release config. Use after changing anything under pkg/, main.go, Makefile, Dockerfile, go.mod, scripts/ or .github/, before reporting the task done.
argument-hint: "[base ref; default: compare the working tree with HEAD]"
---

# Sync the docs with a change

Base to compare against: `$ARGUMENTS` (empty means the working tree against `HEAD`).

1. **Collect the change.** `git status --short`, then `git diff <base>` (include
   untracked files). List each behaviour, flag, default, limit, file, target, version
   or workflow that changed.

2. **Map each change to the docs it affects.**

   | Change | Check |
   |---|---|
   | command, flag, default, limit, exit status, message users act on | README "Core CLI capabilities" (list and examples), "Quick start" notes, Configuration table; the command's `--help` text in `logic-cli.go` |
   | TUI behaviour or key bindings | README "Interactive TUI" |
   | behaviour users of the latest release will notice | README "⚠️ Upgrading from …" notes (latest tag: `git fetch --tags`, then `git describe --tags --abbrev=0`) |
   | on-disk format, KDF, password handling | README "File format" paragraph and upgrade note, `format_v2.go` header comment, golden `README.md`; run `/format-change` |
   | key store location, permissions, `CRYPTARE_DB_PATH` | README Configuration table and Quick start permissions note; CONTRIBUTING "Build and run" |
   | Makefile target or tool version | README "Testing and quality checks"; CONTRIBUTING prerequisites and validation table; `CLAUDE.md` Commands |
   | file added, moved or removed | README "Project structure"; `CLAUDE.md` Architecture; `paths:` in `.claude/rules/*.md` |
   | dependency | CONTRIBUTING prerequisites; `scripts/third-party-licenses.sh` `stated_licence` if needed |
   | Go toolchain | README and CONTRIBUTING prerequisites (use `/toolchain-bump`) |
   | release assets, CD, Docker image | README "Release archives", "Docker"; CONTRIBUTING CI paragraph |
   | security fix | summary-level wording only (`.claude/rules/docs.md`) |
   | supported release line | `SECURITY.md` table |

3. **Edit only what the change made inaccurate or incomplete.** Keep structure, order
   and voice (`.claude/rules/docs.md`). Don't fix unrelated drift you notice; list it.

4. **Verify every sentence you wrote or touched** against the source: grep the flag,
   constant, default or file; read the function. For more than a handful of claims,
   hand the list to the `docs-verifier` agent.

5. **Report**: docs changed (file and section), each claim verified and where, drift
   noticed but not fixed, and anything that needs the owner's wording or decision.
