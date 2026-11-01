---
paths:
  - "*.md"
  - "NOTICE"
  - ".github/ISSUE_TEMPLATE/**"
  - ".github/pull_request_template.md"
  - ".claude/**/*.md"
---

# Documentation

Salus users script against what the README promises, so an inaccurate
sentence is a bug. Which file owns what is in the Documentation table in
`CLAUDE.md`.

## Accuracy

- Confirm every fact in the source before writing it:

  | Fact | Source of truth |
  |---|---|
  | Commands, flags, help text | `pkg/logic-cli.go` |
  | Flag defaults | `default*` constants in `pkg/health.go` |
  | Check keys and order | `AllCheckKeys` in `pkg/health.go` |
  | PASS/WARN/FAIL conditions | the check functions in `pkg/health*.go` |
  | `misconfig` rule IDs | `misconfigRules` in `pkg/health.go` |
  | Exit codes | `ExitCode*` in `pkg/report.go`, `pkg/main.go` |
  | Output formats | `pkg/report.go`, `pkg/report-formats.go`, golden tests |
  | Environment variables, database paths | `pkg/database-path.go`, `pkg/health-sshd.go` |
  | Go and golangci-lint versions | `go.mod`, `.github/workflows/ci.yml` |
  | Release archives, image tags, attestations | `.github/workflows/cd.yml` |
  | Container user, volume, environment | `Dockerfile` |
  | Third-party modules | `go.mod` |

- Run an example command against a build (with `SALUS_DB_PATH` set to a
  temporary file) before documenting its output. If you cannot run it, say
  so in the completion report; never present unverified output as real.
- Do not document planned, partial, or inferred behavior as available.
- Relative links must resolve. Check that every linked path and heading
  anchor exists.

## README

- Keep the existing section order: description; Features; Use cases;
  Prerequisites; Installation; Usage (Quick start, Commands, Output
  formats, jobs, Exit codes); Configuration (with "Upgrading from"
  sections); Containerization; Testing and quality checks; Project
  structure; Contributing and license.
- When behavior changes in a way that can raise the exit code on a host
  that passed before, or changes output that scripts parse, add it to an
  "Upgrading from <previous release>" section, newest first, with those
  changes listed before other changes.
- Update the "What each check reports" table, the `misconfig` rules table,
  the flag list, and the platform table whenever the code they describe
  changes.
- Update the "Project structure" tree when files or directories that it
  lists are added, moved, or removed.
- Keep it accurate first, then concise. Do not add badges, screenshots,
  diagrams, or external links unless they materially help a user.

## Style

- Plain, direct sentences in American English. Wrap prose near 80
  columns, matching the existing files.
- Fenced code blocks with a language (`bash`, `powershell`, `text`, `json`),
  copyable from the repository root. Use placeholders such as `vX.Y.Z` and
  `/path/to/salus.db`, never real secrets or hostnames.
- Tables for flag, value, and status matrices, as the README already does.
- Mark optional steps and platform-specific instructions as such.

## Other files

- `CONTRIBUTING.md` summarizes the contributor workflow; it must agree with
  `CLAUDE.md` and the README "Testing and quality checks" section. When a
  validation command changes, update it, the README, and the pull request
  template together.
- `SECURITY.md`: never describe an undisclosed vulnerability in any public
  file, commit, issue, or pull request.
- `NOTICE` lists every module in `go.mod` with its version, authors, and
  license, and uses CRLF line endings.
- `CLAUDE.md` and `.claude/`: durable rules only, no status or plans. Keep
  `CLAUDE.md` under about 200 lines and move path-specific detail into a
  `.claude/rules/` file with a `paths:` list.
