<!--
Thank you for contributing to Capsus. Read CONTRIBUTING.md before opening this pull request.
Pull requests without a linked issue on which a maintainer has confirmed the concept will be denied.
Changes to a tool's code belong in that tool's repository (Munus, Cryptare or Salus), not here.
Replace the comments below and remove sections that do not apply.
-->

## Linked issue

Closes #

- [ ] A maintainer confirmed the concept on the linked issue.

## Summary

<!-- What changed and why. Keep the change focused on the linked issue. -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change (capsus commands or flags, menu keys, how arguments or exit codes are passed to a tool)
- [ ] Tool sync (`git subtree pull` of Munus, Cryptare or Salus; name the tool and tag below)
- [ ] Documentation
- [ ] Build, CI or dependency update
- [ ] Refactor or tests only (no behaviour change)

## Validation

<!-- Tick only what you actually ran, and paste the relevant output below. -->

- [ ] `make check` (layout, keys-check, fmt-check, tidy-check, vet, lint, race, smoke)
- [ ] `make check-all` (adds gosec, govulncheck, actionlint; required for CI, build, dependency or security changes)
- [ ] `make deps-drift` (required for a tool sync; paste its output)
- [ ] Manual check of the menu and of launching each tool with a binary built by `make build` (describe below)

**Tested on:** <!-- OS (Linux / macOS / Windows) and `go version` output -->

<details>
<summary>Command output</summary>

```text
paste output here
```

</details>

## Checklist

- [ ] No edits under `Munus/`, `Cryptare/` or `Salus/` other than a `git subtree pull`
- [ ] No tool release target (`tag`, `push-tag`, `release`) was run inside a tool folder
- [ ] Tests added or updated for every behaviour change; they are deterministic and use `t.TempDir()` for files and databases
- [ ] The Apache-2.0 license header is kept on every source file
- [ ] No new dependency, or the reason is recorded in `intel/notes.md`
- [ ] `README.md` updated for user-visible changes
- [ ] `intel/` documents updated where architecture, security items or plans changed (`intel/history.md` is append-only)
- [ ] No databases (`*.db`), key exports, coverage files, built binaries or secrets are included

## Security and compatibility

<!--
Security: does this touch how the launcher calls a tool, CI/CD, the Makefile, scripts/ or
dependencies? Reference any affected item in intel/cybersec.md (for example SEC-00X), and for a tool
sync any security item that changed in the tool's own intel/cybersec.md.
Compatibility: does this change capsus commands, flags, menu keys, or how arguments and exit codes
reach a tool? Write "None" if neither applies.
-->

## Screenshots or recordings

<!-- For menu or TUI changes, add a screenshot or terminal recording. Remove this section otherwise. -->

## Notes for reviewers

<!-- Areas to focus on, known limitations or follow-up work. -->
