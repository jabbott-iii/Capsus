# Cryptare TUI demo, recorded with VHS (https://github.com/charmbracelet/vhs).
#
# Regenerate from the repository root, with `cryptare` and `vhs` on your PATH:
#
#   vhs demo/cryptare-tui.tape
#
# Everything runs in a throwaway temporary folder with its own key store
# (CRYPTARE_DB_PATH), so your files and your real key store are never touched.
# The passphrase below is a demo value for that throwaway data only.

Output demo/cryptare-tui.gif

Require cryptare

Set Shell "bash"
Set FontFamily "DejaVu Sans Mono"
Set FontSize 16
Set Width 1200
Set Height 640
Set Padding 24
Set Margin 0
Set WindowBar Colorful
Set BorderRadius 8
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Framerate 24
Set PlaybackSpeed 1.0

Env LANG "C.UTF-8"

# --- setup (not recorded): sample files in a temporary folder -------------------
Hide
Type `export DEMO="$(mktemp -d)" && export CRYPTARE_DB_PATH="$DEMO/.keys/cryptare.db" && mkdir -m 700 "$DEMO/.keys" && cd "$DEMO"`
Enter
Type `printf 'Meet at the usual place at 0600.\nBring coffee.\n' > secret-plans.txt`
Enter
Type `mkdir -p project/src && printf '# project\n' > project/README.md && printf 'package main\n\nfunc main() {}\n' > project/src/main.go && seq 1 20000 > project/data.csv`
Enter
Type "clear"
Enter
Sleep 500ms
Show

# --- the starting point -----------------------------------------------------------
Type "ls -F"
Sleep 300ms
Enter
Sleep 1s
Type "cat secret-plans.txt"
Sleep 300ms
Enter
Sleep 1.5s
Type "cryptare"
Sleep 400ms
Enter
Wait+Screen /Manage keys/
Sleep 1.5s

# --- encrypt a file with a passphrase -------------------------------------------
Enter
Sleep 800ms
Type "secret-plans.txt"
Sleep 500ms
Enter
Sleep 400ms
Enter
Sleep 300ms
Type "orbit lantern velvet sky"
Sleep 300ms
Enter
Type "orbit lantern velvet sky"
Sleep 300ms
Enter
Sleep 700ms
Enter
Wait+Screen /Encrypted:/
Sleep 2s

# --- decrypt it to a new file ---------------------------------------------------
Down
Sleep 500ms
Enter
Sleep 800ms
Type "secret-plans.txt.enc"
Sleep 400ms
Enter
Type "restored-plans.txt"
Sleep 400ms
Enter
Type "orbit lantern velvet sky"
Sleep 500ms
Enter
Wait+Screen /Decrypted:/
Sleep 2s

# --- compress a folder as a zip archive ------------------------------------------
Down
Sleep 500ms
Enter
Sleep 800ms
Type "project"
Sleep 400ms
Enter
Sleep 300ms
Enter
Type "zip"
Sleep 400ms
Enter
Sleep 500ms
Enter
Wait+Screen /Compressed:/
Sleep 2s

# --- generate a stored key ------------------------------------------------------
Down
Sleep 300ms
Down
Sleep 500ms
Enter
Sleep 1.5s
Enter
Sleep 800ms
Type "harbor quartz meadow lamp"
Sleep 300ms
Enter
Type "harbor quartz meadow lamp"
Sleep 500ms
Enter
Wait+Screen /Generated key:/
Sleep 2.5s

# --- back out and quit ----------------------------------------------------------
Escape
Sleep 800ms
Type "q"
Sleep 800ms

# --- check the results in the shell ---------------------------------------------
Type "ls -F"
Sleep 300ms
Enter
Sleep 1.2s
Type "diff -s secret-plans.txt restored-plans.txt"
Sleep 300ms
Enter
Sleep 1.5s
Type "od -A x -t x1z -N 48 secret-plans.txt.enc"
Sleep 300ms
Enter
Sleep 1.5s
Type "unzip -l project.zip"
Sleep 300ms
Enter
Sleep 4s

# --- clean up (not recorded) ----------------------------------------------------
Hide
Type `cd / && rm -rf "$DEMO"`
Enter
