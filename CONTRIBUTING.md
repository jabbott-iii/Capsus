# Contributing to Capsus

## Contributing Instructions:
- Create an issue to pitch an addition or change, pull requests with no corresponding issue will be denied.
- If the issue item is already existing, comment on it before making a pull request to address the issue item.
- Confirmation of a pitched concept is required on the applicable issue before making a pull request to modify the code base.
- License header must be maintained on every source file.
- Must run `make fmt` (`gofmt -s -w` on the launcher's Go files) at the root directory before making a pull request. It leaves the tool folders untouched: they are formatted upstream, and `make fmt-check` verifies them.

## Where a change belongs

Capsus contains the launcher (repository root) and three tools imported as `git subtree`s in
`Munus/`, `Cryptare/` and `Salus/`. Architecture and rules: [AGENTS.md](AGENTS.md) and the area
rules in [.claude/rules/](.claude/rules/).

- **Launcher, build, CI or documentation:** open the pull request here.
- **A tool's code, tests or documentation:** open it in that tool's repository
  ([Munus](https://github.com/jabbott-iii/Munus), [Cryptare](https://github.com/jabbott-iii/Cryptare),
  [Salus](https://github.com/jabbott-iii/Salus)). Do not edit the tool folders in Capsus; the change
  arrives here with the next `git subtree pull`, and local edits would conflict with it.
- **Syncing a tool release** (maintainer): follow the `/tool-sync` procedure in
  [.claude/skills/tool-sync/SKILL.md](.claude/skills/tool-sync/SKILL.md).
- Never run a tool's `tag`, `push-tag` or `release` target inside its folder: it would tag and push
  Capsus. `make tool` refuses them.

## Setup

You need Go 1.26, a C compiler (the tools use `mattn/go-sqlite3`, which needs CGO), GNU Make, bash
and git; Docker only for `make docker-static`, the static Linux build (P-030). Run
`make help` for every target.

## Validation

Before opening a pull request, run and paste the result into the template:

```sh
make check        # what CI runs, in every module, plus the combined-binary smoke test
make check-all    # check plus gosec, govulncheck and actionlint (security-relevant changes)
```

When a pull request syncs a tool, also run `make deps-drift` and report what it prints.

After a dependency change or a tool sync, run `make licenses` to regenerate
`THIRD_PARTY_LICENSES`, and add or remove the `NOTICE.txt` lines it reports, in the existing
format. Never edit `THIRD_PARTY_LICENSES` by hand. `make check` and CI fail while
`THIRD_PARTY_LICENSES` or the `NOTICE.txt` module list is out of date.

`make sec` scans every module and fails if any has findings, so while gosec reports findings in a
tool folder, `make check-all` stops there and skips `vuln` and `actionlint`: run
`make vuln actionlint` as well. Findings in tool code are handled in the tool's repository, never
in Capsus; a finding in the launcher (module `.` in the list `make sec` prints) is a real failure.

## Coding expectations

- Follow [.claude/rules/go.md](.claude/rules/go.md) and [AGENTS.md](AGENTS.md).
- Keep the launcher thin: it calls each tool only through the tool's `pkg.Main` entry point and
  passes arguments and exit codes through unchanged ("Dispatch" in
  [.claude/rules/launcher.md](.claude/rules/launcher.md#dispatch)).
- Add tests for every behaviour change; keep them deterministic and keep databases under
  `t.TempDir()`.
- No new third-party dependency without the maintainer's approval and a reason stated in the pull
  request ("Dependencies" in
  [.claude/rules/build-and-ci.md](.claude/rules/build-and-ci.md#dependencies)).

## Pull requests

- Link the confirmed issue and fill in the pull-request template.
- Update `README.md` for user-visible changes, and [AGENTS.md](AGENTS.md) and
  [.claude/rules/](.claude/rules/) when architecture, plans or security items change; keep their
  existing IDs and add no new ones.
- Do not commit databases, key exports, coverage files, built binaries or secrets.
