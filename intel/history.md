# History (append-only)

Do not edit or remove existing entries. Append new entries at the bottom.

## 2026-10-11 — Import plan; CI/CD, Makefile and documentation for the toolbox layout
- Created the `intel/` documents required by `AGENTS.md` (`plan.md`, `maint.md`, `map.md`,
  `cybersec.md`, `notes.md`, this file); `golang.md` already existed.
- `intel/plan.md` Plan 1: import Munus, Cryptare and Salus as `git subtree`s in folders named after
  them, link them into one `capsus` binary through root-module `replace` directives, and launch
  each tool through a `pkg.Main` entry point added upstream. Status: proposed.
- Replaced the CI/CD, `Makefile` and pull-request template copied from Munus (they built a `munus`
  binary from a Dockerfile and Go module that Capsus does not have) with versions for the
  multi-module layout: per-module checks, a combined-binary smoke test (`scripts/smoke.sh`), gosec
  per module with repository-relative SARIF paths, CodeQL over every module, govulncheck on the
  root build list, static Linux binaries from a planned Dockerfile stage, and release archives with
  provenance but no container image.
- `dependabot.yml`: Go updates limited to the root module, with the tool modules ignored (their
  dependencies are updated upstream); Docker updates apply once the planned Dockerfile exists.
- Security items SEC-001 … SEC-003 opened (In Progress).
- `README.md` and `CONTRIBUTING.md` rewritten for the planned layout; `.gitignore` now ignores
  `bin/`, `dist/`, SQLite databases and key exports.
- Validation: `make` targets exercised against stand-in modules with Go 1.24; workflows linted with
  actionlint 1.7.12 and shellcheck 0.11.0. Not run: builds and tests of the real tools (Go 1.26 and
  the module proxy were unavailable), and the workflows themselves.
