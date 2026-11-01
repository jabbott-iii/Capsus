---
paths:
  - "pkg/logic-cli.go"
  - "pkg/logic-tui.go"
  - "pkg/ui-dashboard.go"
  - "pkg/run.go"
  - "pkg/run_test.go"
  - "pkg/database_path.go"
  - "pkg/logic_cli_test.go"
  - "pkg/logic_cli_unix_test.go"
  - "pkg/logic_tui_test.go"
---

# CLI and TUI

- **Parity.** The CLI (`logic-cli.go`) and TUI (`logic-tui.go`, `ui-dashboard.go`) call
  the same core functions. Prompting, output-path checks and the TUI's form fields stay
  in each interface and must stay equivalent. A behaviour change lands in both, with a
  CLI test and a TUI test. Intended differences: the extraction-limit flags
  (`--max-size`, `--max-entries`), `decompress --raw` and `keys path` are CLI-only, and
  the TUI never overwrites an existing output.
- **Passwords.** Commands that take a password register it with
  `passwordFlags.register` and read it with `passwordFlags.get` (handles
  `--password-file` and the `--password` warning). With neither flag they prompt:
  `readNewPassword` (confirms at a terminal) when the user chooses a new password
  (`encrypt` without `--key`, `keys generate`), otherwise `readPassword`. A stored key's
  master password goes through `unlockStoredKey` for `encrypt --key` and `decrypt`;
  `keys export` reads it once with `readPassword` and checks it against the key, and
  `keys import` reads `--export-password-file` separately. The TUI confirms new
  passwords with its "Confirm password" field and `checkTUINewPassword`. New commands
  and forms that set a password do the same.
- **Outputs.** Call `CheckOutputPath` before writing. The CLI overwrites only with
  `--force`; the TUI never does. `keys export` uses `checkExportOutput`, which also
  refuses the key store itself. Compression levels go through `checkCompressLevel` in
  both interfaces.
- **Cancellation.** CLI file operations run under `runCancellable` (SIGINT, SIGTERM,
  SIGHUP cancel; exit status 128 + signal via `InterruptedError`). TUI actions run
  through `actionRunner`; quitting cancels the running action first.
- **Key store access.** `Main` builds the root command with `NewRootCmdLazy` and a
  `DatabaseOpener`. Only stored-key commands may call it, at most once per run. `keys
  path` is added by `newRootCmd` and opens nothing.
- **Entry point.** `Main(ctx, version, args)` is public API: another program calls it
  in-process, more than once. It takes the arguments only from `args` (nil means none,
  never `os.Args`), passes `ctx` to the commands, returns the exit status without
  calling `os.Exit` (only `readTerminalPassword` does), closes the key database it
  opened, and keeps nothing between calls. `main.go` calls `Run`, which only adds
  `os.Args[1:]` and a background context. Its tests are in `run_test.go`.
- **Usage text.** `NewRootCmdLazy` wraps each `RunE` with `silenceUsageOnRun`, so usage
  prints only for command-line mistakes. A command added outside it sets
  `SilenceUsage` itself.
- **Display.** Show values read from the key store through `displayText`, so stored
  control characters print escaped.
- **Docs.** Any change to commands, flags, defaults, messages users act on, exit
  statuses or TUI behaviour is user-visible: update README's "Core CLI capabilities",
  "Quick start" and "Interactive TUI" sections as needed (`/doc-sync`).
