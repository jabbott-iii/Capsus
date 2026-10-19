@AGENTS.md

## Claude Code

Everything above is shared with other agents; this section is for Claude Code only.

- **Rules.** `.claude/rules/*.md` load when you read or edit a file that matches their `paths:`.
  Read the matching rule before you change an area, even if it hasn't loaded yet.
- **Skills.** `/pre-pr [base]` runs the checks the diff needs and writes the PR template's
  Validation section. `/docs-drift [scope]` audits the docs and these instructions against the
  repository. The maintainer invokes `/tool-sync <Tool> <tag> [add|pull]`,
  `/add-tool <Name> <repository-url>` and `/release-prep vX.Y.Z`; none of them commits, tags or
  pushes.
- **Agent.** `change-reviewer` gives an independent, read-only review. Use it before you report
  a change to the launcher, `go.mod`, the `Makefile`, `scripts/` or the workflows as done.
- **Settings.** `.claude/settings.json` denies edits under `Munus/`, `Cryptare/` and `Salus/`
  and to `LICENSE` and `THIRD_PARTY_LICENSES`; pushes, tags, history
  rewrites, release targets and `make -C`; and reads of `.env` files, databases and key exports.
  It asks before commits, `git subtree`, module changes, and edits to `go.mod`, `go.sum`, the
  `Makefile`, `scripts/`, the `Dockerfile`, `.github/`, `NOTICE.txt` and the Claude settings and
  hook. These are backstops: `AGENTS.md` applies even where a setting would allow more.
- **Hook.** After you edit one of the launcher's `.go` files, `.claude/hooks/go-post-edit.sh`
  runs `gofmt -s -w` on it and reports a missing license header. It needs `jq` or `python3`;
  without either it does nothing, so run `make fmt` yourself.
- **Nested instructions.** A tool folder imported from a release made after 2026-10-13 brings
  the tool's own `CLAUDE.md`, `.claude/rules/` and skills, which load when you work with files
  there. Use them to understand the tool; for anything that writes files or runs commands,
  `AGENTS.md` and `.claude/rules/tool-folders.md` win. Don't run a tool's skills in Capsus; one
  whose name clashes with a Capsus skill appears with the folder prefix, such as
  `/Munus:release-prep`.
- **Personal overrides** go in `CLAUDE.local.md` and `.claude/settings.local.json`; both are
  git-ignored.
