---
paths:
  - "*.md"
  - ".github/*.md"
  - ".claude/**/*.md"
---

# Documentation

## Grounding

- Inspect the repository before editing a document. Every command, flag, path, variable, version,
  platform and make target must match the code, the `Makefile`, the workflows or `scripts/`:
  find it before you write it.
- Don't invent features, commands, variables, ports, platforms or integrations. Don't document
  planned or partial behaviour as done: the README keeps its "planned" labels until a built
  binary shows the behaviour (P-042).
- Never write that a command ran or passed. If something can't be verified, leave it out, or add
  a `TODO` only when the maintainer must supply the information.
- Each tool's README documents that tool; Capsus links to it instead of copying it.
- Edit in place: change only the affected sections, and keep accurate existing content, its
  order and its headings.

## README

Keep these sections, in this order, as far as they apply: title and short description (purpose,
intended users); the status note while preparation lasts; bundled tools; features (implemented
only); use cases; prerequisites; installation; quick start or usage; configuration;
containerization (only once container files exist; Capsus publishes no image, D-8); testing and
quality checks; project structure; contributing and license.

- Commands are copyable and run from the repository root. Use placeholders such as `vX.Y.Z`,
  never secrets, real data or personal paths. Label optional steps and platform-specific
  instructions as such.
- No new badges, screenshots, diagrams or external links unless they already exist or
  materially help a reader.
- Update "Project structure" when top-level files or directories change.
- After a README change, the completion report lists the sections added or changed and every
  fact you could not verify.

## Other documents

- `CONTRIBUTING.md` and `.github/pull_request_template.md` describe the same workflow and checks:
  change them together, and keep them consistent with `AGENTS.md`, the `Makefile` and `ci.yml`.
- Never edit `LICENSE`. `THIRD_PARTY_LICENSES` is generated (P-040).
  Change only the module list in `NOTICE.txt`, and only with a dependency change.
- `AGENTS.md`, `CLAUDE.md` and `.claude/`: keep them short, specific and true. Update them when a
  command, file, layout or rule they describe changes, and keep each rule's `paths:` in step with
  the files it covers (globs for planned files match nothing until those files land). Don't
  create `intel/`-style registers or new IDs; existing IDs stay as they are.
- `SECURITY.md` is public: never describe an undisclosed vulnerability in it. Its
  supported-versions table and reporting process change only with the maintainer's confirmation
  (today it is still GitHub's template).
- Security text stays at summary level: what changed and how it is tested.

## Style

British spelling as in the existing documents (behaviour, initialise, sanitising); plain
present-tense sentences; backticks for commands, flags, paths and identifiers; relative links
that resolve. Wrap prose at about 100 columns, as `README.md` does.
