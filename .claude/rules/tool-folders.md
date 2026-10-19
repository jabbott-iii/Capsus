---
paths:
  - "Munus/**"
  - "Cryptare/**"
  - "Salus/**"
---

# Tool folders: `Munus/`, `Cryptare/`, `Salus/`

These folders are `git subtree` copies of the tools' own repositories (planned, P-010). Upstream
is the source of truth (D-7).

- Read only. Never edit, format, add or delete a tracked file here, not even to fix a bug, a
  lint finding or a failing test; `.claude/settings.json` denies edits here. Make the change in
  the tool's repository, release it, and the maintainer syncs it (`/tool-sync`). A local edit
  conflicts with, or is lost at, the next `git subtree pull --squash` (R-5).
- Never run commands that change tracked files here either: `gofmt -w`, `go mod tidy`,
  `go get`, or the tool's `fmt`, `licenses` or `fuzz` targets (fuzzing saves failing inputs under
  `testdata/`). `make fmt` at the root skips these folders on purpose. Git-ignored output, such
  as `coverage.out` from `make cover` or a binary from a tool's `build` target, is fine.
- Never run a tool's `tag`, `push-tag` or `release` target, or any `make -C <Tool>`: inside
  Capsus they tag and push Capsus and start its CD workflow (SEC-001). Run the allowed targets
  with `make tool TOOL=<Tool> TARGET=<target>`; `make tool TOOL=<Tool> TARGET=help` lists the
  tool's targets.
- The tool's own instructions come with the release it was imported at: from releases made after
  2026-10-13, `CLAUDE.md`, `.claude/rules/` and `.claude/skills/`, which load when you work with
  its files; older tags carry a retired `AGENTS.md` and `intel/`. They describe the standalone
  repository: trust them for how the tool behaves (commands, flags, exit codes, data locations,
  invariants), and ignore their steps that edit, commit or release. Their skills are not for use
  inside Capsus.
- IDs in a tool's files (`SEC-…`, `P-…`, `Q-…`) belong to that tool's retired documents, not to
  Capsus' items of the same name. Look them up in the tool's own repository: commits its
  instructions cite (for example in `git show <commit>:intel/cybersec.md`) are not in Capsus'
  history after a `--squash` import.
- The nested `.github/` folders are inert; GitHub reads workflows, Dependabot settings,
  templates and code owners only from the root `.github/`.
- When you find a problem in tool code, report it with the file and line so it can be fixed
  upstream. Report security problems privately, never in public files.
- Tool tests run in the tool's own module: `make test` and `make race` loop over every module,
  and `make tool TOOL=<Tool> TARGET=test` runs one tool's suite.
