---
name: add-tool
description: Plans and wires a new CLI or TUI tool into the Capsus toolbox, from the upstream entry point and subtree import to go.mod, the launcher registry, the Makefile, CI, security scans, the smoke test, the docs and these instructions. Use when the maintainer wants another tool bundled in Capsus.
argument-hint: "<ToolName> <repository-url>"
disable-model-invocation: true
---

# Add a tool: `$ARGUMENTS`

The arguments are the tool's name, which becomes its folder name exactly as written, and its
repository URL. Leave every change uncommitted. Ask before editing the `Makefile`, `scripts/`,
the workflows or `go.mod`: those changes are security-sensitive.

## 1. Facts first

Read the tool's repository without putting it in the Capsus tree (for example, a shallow clone
in a temporary folder) and report a table of:

- module path, and its `go` and `toolchain` lines;
- whether it has a TUI, and whether it accepts `--vim`;
- exit codes and what each means;
- data location and its environment variable;
- license (it must be compatible with Apache-2.0) and whether it needs CGO;
- whether it exports `func Main(ctx context.Context, version string, args []string) int` from
  package `pkg` (D-4);
- a one-line description taken from its README;
- the latest release tag.

Stop and ask if the name clashes with an existing folder, command or registry entry, or if the
license is incompatible.

## 2. Upstream prerequisites

Without `pkg.Main`, the change belongs in the tool's repository first: describe what `Main` must
do there (keep its own flag parsing, data location, error output and exit codes; take arguments
from the parameter, not `os.Args`), but don't write it in Capsus. The tool then needs a release
tag.

## 3. Import (maintainer)

Ask the maintainer to run `/tool-sync <ToolName> <tag> add` (only the maintainer can start it),
or print the command it hands over:

```sh
git subtree add --prefix=<ToolName> <repository-url> <tag> --squash
```

Nothing below starts until that subtree merge exists, so the working tree is clean for it.

## 4. Wiring

Use the module path from step 1 everywhere a module path appears.

- `go.mod`: `require <module-path> v0.0.0` and `replace <module-path> => ./<ToolName>`; raise
  the root `go` or `toolchain` line if the tool's is higher (D-9); `go mod tidy` at the root
  (ask first).
- Launcher: a registry entry (name, description, has-TUI, bundled tag, `Main`), the
  `capsus <toolname>` subcommand with `DisableFlagParsing`, `--vim` pass-through if supported,
  and tests for dispatch, the menu row, `capsus list` and `--version`.
- `Makefile`: add it to `TOOLS`.
- `ci.yml`: a row in the `module` matrix.
- `security.yml`: a row in the gosec matrix and the folder in the CodeQL build loop (SEC-003).
- `docker.yml` and `cd.yml`: its data variable in the container smoke steps.
- `scripts/smoke.sh`: its data variable pointed into the temporary folder, its `capsus list`
  expectation, and one non-interactive round trip through `capsus <toolname>`.
- `.github/dependabot.yml`: an `ignore` entry if the module path is not under
  `github.com/jabbott-iii/`.
- `README.md`, `CONTRIBUTING.md` and `.github/pull_request_template.md`: the tool table,
  configuration variables, project structure and every list of tool folders.
- `AGENTS.md` (tool table, layout, tool-folder rules), `CLAUDE.md`, the `paths:` of
  `.claude/rules/tool-folders.md`, an `Edit(/<ToolName>/**)` deny rule in
  `.claude/settings.json`, and the tool-folder lists in the `pre-pr` and `tool-sync` skills and
  the `change-reviewer` agent.
- `NOTICE.txt` and `THIRD_PARTY_LICENSES` for its modules (P-040, P-041).

## 5. Verify and report

`make check-all`, `make smoke`, then `/pre-pr` with the commit before the subtree merge as its
base (`HEAD^1` while the merge is still `HEAD`). Report the facts table, every file changed, the
checks run and their results, and anything left for the maintainer, including the upstream
release the tool still needs.
