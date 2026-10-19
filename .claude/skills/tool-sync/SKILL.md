---
name: tool-sync
description: Prepares a git subtree import or sync of a tool into Capsus (Munus, Cryptare, Salus, or a new tool being added), hands the subtree command to the maintainer, then does the follow-up (bundled version, root go.mod, dependency drift, licenses, docs, checks). Use for the first import (Plan 1 Phase 1), when a tool has a new release to bring in, or from /add-tool.
argument-hint: "<Tool> <tag> [add|pull]"
disable-model-invocation: true
allowed-tools: Bash(git status *) Bash(git log *) Bash(git diff *) Bash(git ls-files *) Bash(git ls-remote *) Bash(make layout) Bash(make keys-check) Bash(make deps-drift) Bash(make check) Bash(make check-all) Bash(make smoke) Bash(go mod tidy -diff)
---

# Tool sync: `$ARGUMENTS`

The arguments are the tool (its exact folder and repository name, such as `Munus`), the upstream
release tag, and optionally `add` (first import) or `pull` (a later sync; the default when the
folder exists). Below, `<Tool>` and `<tag>` stand for them, and `<url>` for the tool's
repository: `https://github.com/jabbott-iii/<Tool>.git` for Munus, Cryptare and Salus, or the
URL given to `/add-tool` for a new tool.

You prepare and follow up; the maintainer runs the subtree command, because it creates commits.
Never run `git subtree`, commit or push, and never edit files under the tool folders.

## 1. Before

- `git status --short` is clean (`git subtree` refuses a dirty working tree); if not, report it
  and stop.
- For `pull`, `<Tool>` is listed in `TOOLS` in the `Makefile`. For `add`, the folder doesn't
  exist yet; a tool that isn't Munus, Cryptare or Salus comes through `/add-tool`, which wires it
  in after the import.
- The tag exists upstream: `git ls-remote --tags <url>` (for an annotated tag, the `^{}` line is
  its commit). Ask before importing anything that isn't a
  release tag. Tags made before 2026-10-13 carry the tool's retired `AGENTS.md` and `intel/`
  instead of its `CLAUDE.md` and `.claude/`; say so if the tag is one of them.
- The tool exports `pkg.Main` at that tag (D-4, Plan 1 Phase 0). If it doesn't, say that the
  launcher can't use the import yet and ask whether to continue.
- For `pull`: find the bundled tag. The registry in `internal/launcher` records it; otherwise the
  last squash commit's `git-subtree-split:` line gives the upstream commit
  (`git log --grep='git-subtree-dir: <Tool>' -1`), which the `^{}` lines of `git ls-remote --tags`
  map to a tag. Summarise the upstream changes between the two tags from the tool's release notes
  and README upgrade notes, and call out changes to `pkg.Main`, exit codes, flags, environment
  variables or the database schema: they can break the launcher or shared data (R-2).

## 2. Hand-off

Print the command for the maintainer to run from a branch:

```sh
git subtree add  --prefix=<Tool> <url> <tag> --squash   # first import
git subtree pull --prefix=<Tool> <url> <tag> --squash   # later syncs
```

Then wait until the maintainer says it has run.

## 3. Follow-up (leave everything uncommitted)

1. HEAD is now a merge commit and `HEAD^2` the squash commit (`git log -1 HEAD^2` shows its
   `git-subtree-dir:` and `git-subtree-split:` lines). `git diff --stat HEAD^1 HEAD` must list
   only paths under `<Tool>/`.
2. Set the tool's bundled version in the launcher registry to `<tag>` (P-011). Before Phase 2
   there is no registry yet; note the tag in the report instead.
3. Root module (after Phase 2): keep the `require`/`replace` pair; raise the root `go` or
   `toolchain` line if the tool's is now higher (D-9); run `go mod tidy` at the root only, after
   asking, since it rewrites `go.mod` and `go.sum`.
4. `make deps-drift` and report every line it prints (R-1). Drift is fixed upstream; a root-level
   bump is a stopgap only for a reachable vulnerability (Q-8).
5. Licenses: if modules were added or removed, regenerate `THIRD_PARTY_LICENSES` and update the
   `NOTICE.txt` module list (P-040, P-041). Until the generator exists, list this as open.
6. Docs: the README tool table and configuration section if the tool's description, TUI status
   or variables changed; the tool table in `AGENTS.md` likewise.
7. `make keys-check`, `make check-all` and `make smoke`, then `/pre-pr` with the commit before
   the subtree merge as its base (`HEAD^1` while the merge is still `HEAD`), so the import is in
   range.

## 4. Report

Tool, old and new tag, the upstream changes that matter to Capsus, files changed, the
`make deps-drift` output, checks run and their results, and what is left for the maintainer.
