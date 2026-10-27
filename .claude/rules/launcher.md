---
paths:
  - "*.go"
  - "internal/**"
  - "scripts/smoke.sh"
---

# The launcher: root Go files, `internal/launcher`, smoke test

The launcher is written (Plan 1 P-020 to P-024 in `AGENTS.md`) but builds only once the tool
folders exist, imported at releases that export `pkg.Main`. Keep it to this contract;
`scripts/smoke.sh` checks the command-line side of it.

## Structure

- `main.go`: `var version = "dev"`, set only by `-ldflags "-X main.version=…"` (never
  hard-coded); the Cobra root command `capsus` with `list`, `munus`, `cryptare` and `salus` and
  the `--vim` flag; no arguments in a terminal opens the menu. `main` is the only place that
  calls `os.Exit`, with the code the dispatched tool returned.
- `internal/launcher`: the registry and the Bubble Tea menu model. A registry entry is data plus
  the tool's `Main` function: name, command, short description, has-TUI, accepts `--vim`,
  bundled version (the imported tag, P-011). The registry holds no UI code, so dispatch is
  testable without a terminal. `Bundled` in `registry.go` builds it, with each bundled version
  as a constant there; everything else takes the tools as a parameter, so tests use fakes.
- Tool packages are imported with aliases (`munus "github.com/jabbott-iii/Munus/pkg"`): every
  tool's package is named `pkg`.
- Use only Bubble Tea, Lip Gloss and Cobra, at the versions the tools already compile in
  (today `bubbletea v1.3.10`, `lipgloss v1.1.0`, `cobra v1.10.2`).

## Dispatch

- `capsus <tool> [args...]` calls `Main(ctx, bundledVersion, args)` with `args` exactly as
  received after the tool name. The tool subcommands set `DisableFlagParsing`, so Capsus never
  parses, reorders or drops a tool's flags, `--help` and `--version` included. Anything before
  the tool name (`capsus --vim munus`) is a usage error, because Cobra would pass it on to the
  tool: a tool's subcommand runs only when the tool name is the first argument. `-h` or `--help`
  directly before the name shows Capsus' help.
- The exit code comes back unchanged: Munus 0 or 1; Cryptare 0, 1 or 128 + signal (130 after
  Ctrl+C); Salus 0/1/2 for PASS/WARN/FAIL and 3 for errors. Never map, clamp or reinterpret it.
- Capsus never touches a tool's database, flags or environment. Each tool resolves its own
  `*_DB_PATH` variable and default location, so Capsus and the standalone binaries share data.
- Tools run in-process: `Main` is called directly, never through a child process (D-3).
- Only `Main` and the registry metadata cross the boundary: no calls into other exported tool
  API, no copies of a tool's `main` logic, and no writes to `os.Args`, `os.Stdout`, `os.Stderr`
  or other process-wide state to steer a tool (D-4).
- An unknown tool name is a usage error with a non-zero exit code. Capsus' own codes, outside
  Cobra's built-in help: 2 for a usage error (unknown command or flag, a flag before the tool
  name, or no arguments outside a terminal), 1 for its own failures (the menu cannot start,
  output cannot be written).
- "Outside a terminal" means standard output is not a character device. On Unix the null device
  doesn't count as a terminal; on Windows `NUL` does, because Go can't tell it from a console.
- `main` installs no signal handlers: Munus relies on Ctrl+C's default behaviour, and Cryptare
  and Salus install their own only around the operations that need them (Cryptare's file
  commands, TUI and password prompt; Salus' `check run`) and stop them on return.
- A tool never gets a nil argument slice: a Cobra command with nil arguments reads `os.Args`.

## Output

- `capsus list`: one plain-text line per tool: the name, whitespace, then the description and
  `TUI` or `CLI only`. `scripts/smoke.sh` expects lines that start with `Munus` or `munus` and
  with `Cryptare` or `cryptare`, each followed by whitespace and containing the word `TUI`, and
  one that starts with `Salus` or `salus`, followed by whitespace, containing `CLI only`. A line
  such as `Munus: …` fails.
- `capsus --version`: first line `capsus version <version>`, then one line per bundled tool in
  the tools' own format, `<command> version <bundled tag>`. `capsus munus --version` is the
  tool's own output (`munus version <tag>`).
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
- `q` and Ctrl+C quit with exit code 0. When a tool started from the menu exits non-zero, the
  menu shows "<Name> exited with status <code>." The menu renders inline, not on the alternate
  screen, and clears itself when it quits, so a tool's last output stays visible.

## Process-wide state (R-4)

The tools share the `capsus` process. Cryptare and Salus register signal handlers around single
operations and stop them on return, and Munus writes errors to `os.Stderr` directly. Cryptare's
password prompt (`readTerminalPassword` in `Cryptare/pkg/logic-cli.go`) calls `os.Exit` on
Ctrl+C, ending `capsus` and skipping its deferred clean-up until P-002 changes that upstream.
Don't rely on deferred clean-up while a tool runs, and test menu → tool → menu in one process.
Accepted consequences, none fixable in Capsus without steering the tools:

- Bubble Tea's `init()` queries the terminal's background colour once per `capsus` process when
  stdout is a terminal, Salus commands included (standalone Salus has no Bubble Tea). A terminal
  that answers neither that query nor the cursor-position one delays start-up by up to 5 s.
- A SIGINT or SIGTERM signal, as opposed to the Ctrl+C key, while a tool's TUI runs ends only
  that tool, and the menu comes back. At the menu itself, SIGINT exits 1 ("program was
  interrupted") and SIGTERM exits 0, leaving the last menu frame on screen.
- Cryptare's TUI can deadlock in Bubble Tea's shutdown when a SIGINT or SIGTERM signal arrives
  (its handler and Bubble Tea's both subscribe); standalone Cryptare has the same bug. The fix
  belongs upstream (`tea.WithoutSignalHandler()` in Cryptare's TUI program).
- After a tool's Bubble Tea program is killed (a signal or a cancelled context), Bubble Tea
  v1.3.10 can leave its input reader running, and it may swallow one key at the menu. Keys typed
  while the menu hands over to a tool can be lost the same way.

## Tests (P-023)

- Table-driven dispatch tests with fake `Main` functions: arguments passed unchanged, exit codes
  propagated, unknown tool rejected.
- Menu `Update`/`View` tests: movement keys, Enter on a TUI tool returns it, Enter on Salus shows
  the message, the quit keys.
- `capsus list` and `--version` output; menu → tool → menu in one process.
- Deterministic; databases under `t.TempDir()` through the three `*_DB_PATH` variables.
- A change to the command-line contract updates `scripts/smoke.sh` and the README in the same
  change.
