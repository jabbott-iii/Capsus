# Capsus

CLI toolbox containing all CLI tools I have made in one binary.

Capsus bundles three terminal tools, Munus, Cryptare and Salus, into a single `capsus` command for
people who use them together. Running `capsus` opens a menu that lists the tools with a short
description; selecting a tool launches that tool's own terminal UI (TUI), and a tool without a TUI
says so.

> **Status: pre-release.** The tools are imported at their release tags (`capsus --version` lists
> them), and `make build` builds the `capsus` binary from source. There is no release yet, so
> there are no release archives. Not every check passes yet: `make check-all` stops at gosec
> findings in Salus' code, which are to be resolved in Salus, and CI has yet to pass on all three
> platforms. The remaining work is in [Plan 1](AGENTS.md#plan-1-work-order), which is awaiting
> review.

## Bundled tools

| Tool | What it does | TUI | Source |
|---|---|---|---|
| Munus | Terminal task manager: tasks with deadlines, statuses and tags in a local SQLite database | Yes | [jabbott-iii/Munus](https://github.com/jabbott-iii/Munus) |
| Cryptare | Encrypt, decrypt, compress and extract files and folders; manage password-protected keys | Yes | [jabbott-iii/Cryptare](https://github.com/jabbott-iii/Cryptare) |
| Salus | Environment health checker: disk, memory, CPU, Docker, Kubernetes, services and certificates, with PASS/WARN/FAIL results | No (CLI only) | [jabbott-iii/Salus](https://github.com/jabbott-iii/Salus) |

Each tool's own README is the reference for its commands and options.

## Features

The command-line and menu contract is in [AGENTS.md](AGENTS.md#architecture-plan-1-decisions),
decisions D-5 and D-6.

- `capsus`: menu TUI listing each tool, its description and whether it has a TUI. Enter launches the
  tool's TUI and returns to the menu when it exits; for Salus the menu says it has no TUI. Outside a
  terminal it exits with 2 and points to `capsus list` and `capsus <tool>`.
- `capsus list`: the same list as plain text, for scripts.
- `capsus munus|cryptare|salus [args...]`: runs that tool's CLI with the arguments unchanged and
  exits with the tool's exit code. A tool's help shows the tool's own name (for example
  `munus add`). Arguments go after the tool name: `capsus --vim munus` is a usage error.
- `capsus --vim`: opens the menu and passes `--vim` to Munus and Cryptare.
- `capsus --version` (`-v`): the Capsus version and the bundled tool versions.
- Capsus' own errors: exit 2 for an unknown tool, command or flag, and 1 when the menu cannot
  start or output cannot be written.

## Prerequisites

For building from source and running the checks:

- Go 1.26 (CI uses the latest 1.26 patch release)
- A C compiler: every tool stores data with `mattn/go-sqlite3`, which needs CGO
- GNU Make, bash and git
- Docker with BuildKit, only for `make docker-static` (the static Linux build)

## Installation

There is no release yet. Build from source:

```sh
git clone https://github.com/jabbott-iii/Capsus.git
cd Capsus
make build    # writes bin/capsus
```

Release archives will be published on the GitHub Releases page by the CD workflow;
`go install …@latest` is not supported because the root module uses `replace` directives for the
tool folders.

## Configuration

The bundled tools keep their own configuration, so Capsus and the standalone binaries share data.
The main settings are the database locations:

| Variable | Tool | Purpose |
|---|---|---|
| `MUNUS_DB_PATH` | Munus | Path of the task database |
| `CRYPTARE_DB_PATH` | Cryptare | Path of the key database |
| `SALUS_DB_PATH` | Salus | Path of the scan-history database |

When unset, each tool uses its default location in the per-user data directory; see each tool's
README for details and further options.

## Static Linux build

The `Dockerfile` builds the fully static Linux binary (linked with musl) that the release
workflow puts in the Linux archives, for the architecture of the machine that runs Docker.
Capsus publishes no container image.

```sh
make docker-static    # writes bin/static/capsus
```

## Testing and quality checks

Run `make` (or `make help`) for the full list. The main targets run in every module, the launcher
at the root and each tool folder:

```sh
make check        # what CI runs: layout, key-material check, gofmt, go mod tidy, vet, golangci-lint, race tests, smoke test
make check-all    # check plus gosec, govulncheck and actionlint
make test         # tests only
make smoke        # build bin/capsus and run scripts/smoke.sh against it
make deps-drift   # report dependencies the capsus build uses at other versions than a tool's go.mod
make licenses     # regenerate THIRD_PARTY_LICENSES after a dependency change or a tool sync
make tool TOOL=Cryptare TARGET=golden   # run one of a tool's own Makefile targets (release targets refused)
```

## Project structure

```
Capsus/
├── Munus/ Cryptare/ Salus/  # the tools, imported as git subtrees (own go.mod each)
├── main.go, internal/       # the capsus launcher and menu TUI
├── go.mod, go.sum           # root module: requires the tools, replaced by their folders
├── .gitattributes           # go.mod and go.sum stay LF on every checkout (go mod tidy -diff)
├── scripts/smoke.sh         # smoke test for a built capsus binary
├── Makefile                 # checks, build, smoke test, security scans, release tagging
├── .github/                 # CI, security, Docker and CD workflows; Dependabot; PR template
├── Dockerfile               # static Linux release build (musl), with a .dockerignore
├── tools/licenses/          # THIRD_PARTY_LICENSES generator and check (make licenses)
├── THIRD_PARTY_LICENSES     # license texts for the release binaries (generated)
├── NOTICE.txt               # attribution summary: the bundled tools and third-party modules
├── AGENTS.md, CLAUDE.md     # agent instructions: purpose, plan, decisions, ground rules
└── .claude/                 # Claude Code rules, skills, agent, settings and hook
```

See "Layout" in [AGENTS.md](AGENTS.md#layout) for more detail.

## Contributing and license

See [CONTRIBUTING.md](CONTRIBUTING.md). Changes to a tool's code go to that tool's repository
first. Capsus is licensed under the Apache License 2.0 ([LICENSE](LICENSE)). The binaries also
contain third-party software under its own licenses: [NOTICE.txt](NOTICE.txt) lists it, and
[THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES) reproduces the license texts.
