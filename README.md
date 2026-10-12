# Capsus

CLI toolbox containing all CLI tools I have made in one binary.

Capsus bundles three terminal tools, Munus, Cryptare and Salus, into a single `capsus` command for
people who use them together. Running `capsus` will open a menu that lists the tools with a short
description; selecting a tool launches that tool's own terminal UI (TUI), and a tool without a TUI
says so.

> **Status: in preparation.** The tools are not imported yet and the `capsus` binary does not exist.
> Everything under "Planned" below describes [Plan 1](intel/plan.md), which is awaiting review. The
> build, test and release tooling in this repository already targets that layout.

## Bundled tools (planned)

| Tool | What it does | TUI | Source |
|---|---|---|---|
| Munus | Terminal task manager: tasks with deadlines, statuses and tags in a local SQLite database | Yes | [jabbott-iii/Munus](https://github.com/jabbott-iii/Munus) |
| Cryptare | Encrypt, decrypt, compress and extract files and folders; manage password-protected keys | Yes | [jabbott-iii/Cryptare](https://github.com/jabbott-iii/Cryptare) |
| Salus | Environment health checker: disk, memory, CPU, Docker, Kubernetes, services and certificates, with PASS/WARN/FAIL results | No (CLI only) | [jabbott-iii/Salus](https://github.com/jabbott-iii/Salus) |

Each tool's own README is the reference for its commands and options.

## Planned features

Not implemented yet; see [intel/plan.md](intel/plan.md), decisions D-5 and D-6.

- `capsus`: menu TUI listing each tool, its description and whether it has a TUI. Enter launches the
  tool's TUI and returns to the menu when it exits; for Salus the menu says it has no TUI.
- `capsus list`: the same list as plain text, for scripts.
- `capsus munus|cryptare|salus [args...]`: runs that tool's CLI with the arguments unchanged and
  exits with the tool's exit code. A tool's help shows the tool's own name (for example
  `munus add`).
- `capsus --vim`: opens the menu and passes `--vim` to Munus and Cryptare.
- `capsus --version`: the Capsus version and the bundled tool versions.

## Prerequisites

For building from source and running the checks:

- Go 1.26 (CI uses the latest 1.26 patch release)
- A C compiler: every tool stores data with `mattn/go-sqlite3`, which needs CGO
- GNU Make, bash and git
- Docker, only for `make docker-static` (needs the planned Dockerfile)

## Installation

There is no release yet. Once the import lands, build from source:

```sh
git clone https://github.com/jabbott-iii/Capsus.git
cd Capsus
make build    # writes bin/capsus
```

Until then, `make build` stops with a message pointing to [intel/plan.md](intel/plan.md). Release
archives will be published on the GitHub Releases page by the CD workflow; `go install …@latest` is
not supported because the root module uses `replace` directives for the tool folders.

## Configuration

Planned: the bundled tools keep their own configuration, so Capsus and the standalone binaries
share data. The main settings are the database locations:

| Variable | Tool | Purpose |
|---|---|---|
| `MUNUS_DB_PATH` | Munus | Path of the task database |
| `CRYPTARE_DB_PATH` | Cryptare | Path of the key database |
| `SALUS_DB_PATH` | Salus | Path of the scan-history database |

When unset, each tool uses its default location in the per-user data directory; see each tool's
README for details and further options.

## Testing and quality checks

Run `make` (or `make help`) for the full list. The main targets run in every module, the launcher
at the root and each tool folder:

```sh
make check        # what CI runs: layout, key-material check, gofmt, go mod tidy, vet, golangci-lint, race tests, smoke test
make check-all    # check plus gosec, govulncheck and actionlint
make test         # tests only
make smoke        # build bin/capsus and run scripts/smoke.sh against it
make deps-drift   # report dependencies the capsus build uses at other versions than a tool's go.mod
make tool TOOL=Cryptare TARGET=golden   # run one of a tool's own Makefile targets (release targets refused)
```

## Project structure

```
Capsus/
├── Munus/ Cryptare/ Salus/  # planned: the tools, imported as git subtrees (own go.mod each)
├── main.go, internal/       # planned: the capsus launcher and menu TUI
├── scripts/smoke.sh         # smoke test for a built capsus binary
├── Makefile                 # checks, build, smoke test, security scans, release tagging
├── .github/                 # CI, security, Docker and CD workflows; Dependabot; PR template
└── intel/                   # plan, architecture, map, security register, notes, history
```

See [intel/map.md](intel/map.md) for the full map.

## Contributing and license

See [CONTRIBUTING.md](CONTRIBUTING.md). Changes to a tool's code go to that tool's repository
first. Capsus is licensed under the Apache License 2.0 ([LICENSE](LICENSE)).
