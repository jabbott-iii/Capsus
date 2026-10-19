---
paths:
  - "*.go"
  - "internal/**"
  - "scripts/smoke.sh"
---

# The launcher: root Go files, `internal/launcher`, smoke test

The launcher does not exist yet (Plan 1 P-021 to P-023 in `AGENTS.md`). Build it to this
contract; `scripts/smoke.sh` already checks the command-line side of it.

## Structure

- `main.go`: `var version = "dev"`, set only by `-ldflags "-X main.version=…"` (never
  hard-coded); the Cobra root command `capsus` with `list`, `munus`, `cryptare` and `salus` and
  the `--vim` flag; no arguments in a terminal opens the menu. `main` is the only place that
  calls `os.Exit`, with the code the dispatched tool returned.
- `internal/launcher`: the registry and the Bubble Tea menu model. A registry entry is data plus
  the tool's `Main` function: name, short description, has-TUI, bundled version (the imported
  tag, P-011). The registry holds no UI code, so dispatch is testable without a terminal.
- Tool packages are imported with aliases (`munus "github.com/jabbott-iii/Munus/pkg"`): every
  tool's package is named `pkg`.
- Use only Bubble Tea, Lip Gloss and Cobra, at the versions the tools already compile in
  (today `bubbletea v1.3.10`, `lipgloss v1.1.0`, `cobra v1.10.2`).

## Dispatch

- `capsus <tool> [args...]` calls `Main(ctx, bundledVersion, args)` with `args` exactly as
  received. The tool subcommands set `DisableFlagParsing`, so Capsus never parses, reorders or
  drops a tool's flags, `--help` and `--version` included.
- The exit code comes back unchanged: Munus 0 or 1; Cryptare 0, 1 or 128 + signal (130 after
  Ctrl+C); Salus 0/1/2 for PASS/WARN/FAIL and 3 for errors. Never map, clamp or reinterpret it.
- Capsus never touches a tool's database, flags or environment. Each tool resolves its own
  `*_DB_PATH` variable and default location, so Capsus and the standalone binaries share data.
- Tools run in-process: `Main` is called directly, never through a child process (D-3).
- Only `Main` and the registry metadata cross the boundary: no calls into other exported tool
  API, no copies of a tool's `main` logic, and no writes to `os.Args`, `os.Stdout`, `os.Stderr`
  or other process-wide state to steer a tool (D-4).
- An unknown tool name is a usage error with a non-zero exit code.

## Output

- `capsus list`: one plain-text line per tool: the name, whitespace, then the description and
  `TUI` or `CLI only`. `scripts/smoke.sh` expects lines that start with `Munus` or `munus` and
  with `Cryptare` or `cryptare`, each followed by whitespace and containing the word `TUI`, and
  one that starts with `Salus` or `salus`, followed by whitespace, containing `CLI only`. A line
  such as `Munus: …` fails.
- `capsus --version`: first line `capsus version <version>`, then one line per bundled tool with
  its version. `capsus munus --version` is the tool's own output (`munus version <tag>`).
- Results go to stdout, diagnostics to stderr.

## Menu (D-6)

- Rows show name, short description and `TUI` or `CLI only`. ↑/↓ and `j`/`k` move, Enter
  selects, `q` or Ctrl+C quits. The descriptions live in the registry and match the tool table
  in `README.md`.
- Enter on a TUI tool: the menu program returns the selection and exits; the launcher then calls
  that tool's `Main` with no arguments (plus `--vim` for Munus and Cryptare when Capsus got
  `--vim`), so the tool's TUI takes the terminal, and starts the menu again when `Main` returns.
  Never run two Bubble Tea programs at once, and never call a tool from inside the menu's
  `Update`.
- Enter on a CLI-only tool (Salus): the menu stays and shows "Salus has no TUI. Run
  `capsus salus --help` for its commands."
- No blocking I/O in `Update`; side effects run as Bubble Tea commands.

## Process-wide state (R-4)

The tools share the `capsus` process. Cryptare and Salus register signal handlers per call and
stop them on return, and Munus writes errors to `os.Stderr` directly. Cryptare's password prompt
(`readTerminalPassword` in `Cryptare/pkg/logic-cli.go`) calls `os.Exit` on Ctrl+C, ending
`capsus` and skipping its deferred clean-up until P-002 changes that upstream. Don't rely on
deferred clean-up while a tool runs, and test menu → tool → menu in one process.

## Tests (P-023)

- Table-driven dispatch tests with fake `Main` functions: arguments passed unchanged, exit codes
  propagated, unknown tool rejected.
- Menu `Update`/`View` tests: movement keys, Enter on a TUI tool returns it, Enter on Salus shows
  the message, the quit keys.
- `capsus list` and `--version` output; menu → tool → menu in one process.
- Deterministic; databases under `t.TempDir()` through the three `*_DB_PATH` variables.
- A change to the command-line contract updates `scripts/smoke.sh` and the README in the same
  change.
