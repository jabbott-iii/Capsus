# Cryptare CLI demo, recorded with VHS (https://github.com/charmbracelet/vhs).
#
# Regenerate from the repository root, with `cryptare`, `vhs` and `ffmpeg` on your
# PATH. The recording is about two minutes long, and VHS's own GIF encoder keeps
# every frame in memory (more than 6 GB here), so VHS writes an MP4 and ffmpeg
# turns it into the GIF in two passes (palette first, then the frames):
#
#   vhs demo/cryptare-cli.tape
#   ffmpeg -y -i demo/cryptare-cli.mp4 \
#     -vf "fps=20,palettegen=max_colors=256:stats_mode=full" /tmp/cryptare-cli-palette.png
#   ffmpeg -y -i demo/cryptare-cli.mp4 -i /tmp/cryptare-cli-palette.png \
#     -lavfi "fps=20[x];[x][1:v]paletteuse=dither=none:diff_mode=rectangle" demo/cryptare-cli.gif
#   rm demo/cryptare-cli.mp4 /tmp/cryptare-cli-palette.png
#
# Everything runs in a throwaway temporary folder with its own key store
# (CRYPTARE_DB_PATH), so your files and your real key store are never touched.
# The passphrases below are demo values for that throwaway data only.

Output demo/cryptare-cli.mp4

Require cryptare

Set Shell "bash"
Set FontFamily "DejaVu Sans Mono"
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 24
Set Margin 0
Set WindowBar Colorful
Set BorderRadius 8
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms
Set Framerate 24
Set PlaybackSpeed 1.0

Env LANG "C.UTF-8"

# --- setup (not recorded): sample files in a temporary folder -------------------
# banner clears the screen and prints a section title in Cryptare's orange.
Hide
Type `export DEMO="$(mktemp -d)" && export CRYPTARE_DB_PATH="$DEMO/.keys/cryptare.db" && mkdir -m 700 "$DEMO/.keys" && cd "$DEMO"`
Enter
Type `banner() { clear; printf '\033[1;38;2;248;142;2m%s\033[0m\n\n' "$*"; }`
Enter
Type `printf 'Meet at the usual place at 0600.\nBring coffee.\n' > notes.txt && printf 'Q3 numbers: up and to the right.\n' > report.txt`
Enter
Type `mkdir -p project/src && printf '# project\n' > project/README.md && printf 'package main\n\nfunc main() {}\n' > project/src/main.go && seq 1 20000 > project/data.csv && seq 1 200000 > server.log`
Enter
Type `printf 'orbit lantern velvet sky\n' > pw.txt && chmod 600 pw.txt`
Enter
Type `banner "Cryptare CLI: encrypt, compress and manage keys from the terminal"`
Enter
Sleep 500ms
Show

# --- overview -------------------------------------------------------------------
Type "cryptare --version"
Enter
Wait
Sleep 800ms
Type "cryptare --help"
Enter
Wait
Sleep 4s

# --- 1: encrypt and decrypt a file at the password prompt ------------------------
Hide
Type `banner "1/6  Encrypt and decrypt a file (hidden password prompt)"`
Enter
Sleep 300ms
Show
Type "ls -F"
Enter
Wait
Sleep 1s
Type "cryptare encrypt notes.txt"
Enter
Wait /Enter password:/
Sleep 400ms
Type@25ms "orbit lantern velvet sky"
Enter
Wait /Confirm password:/
Sleep 300ms
Type@25ms "orbit lantern velvet sky"
Enter
Wait
Sleep 1.5s
Type "cryptare decrypt notes.txt.enc"
Enter
Wait
Sleep 2.5s
Type "cryptare decrypt notes.txt.enc -o restored.txt"
Enter
Wait /Enter password:/
Sleep 400ms
Type@25ms "not the right password"
Enter
Wait
Sleep 2.5s
Type "cryptare decrypt notes.txt.enc -o restored.txt"
Enter
Wait /Enter password:/
Sleep 400ms
Type@25ms "orbit lantern velvet sky"
Enter
Wait
Sleep 1s
Type "cat restored.txt"
Enter
Wait
Sleep 3s

# --- 2: folders, scripting and --force ------------------------------------------
Hide
Type `banner "2/6  Encrypt a whole folder, script it with --password-file, overwrite with --force"`
Enter
Sleep 300ms
Show
Type "cryptare encrypt project --password-file pw.txt"
Enter
Wait
Sleep 1.2s
Type "cryptare decrypt project.enc -o project-restored --password-file pw.txt"
Enter
Wait
Sleep 1.2s
Type `diff -r project project-restored && echo "folders identical"`
Enter
Wait
Sleep 1.5s
Type "cryptare encrypt notes.txt --password-file pw.txt"
Enter
Wait
Sleep 2s
Type "cryptare encrypt notes.txt --password-file pw.txt --force"
Enter
Wait
Sleep 3s

# --- 3: compress ----------------------------------------------------------------
Hide
Type `banner "3/6  Compress: gzip a file, tar.gz or zip a folder, pick a level"`
Enter
Sleep 300ms
Show
Type "cryptare compress server.log"
Enter
Wait
Sleep 1s
Type "cryptare compress project"
Enter
Wait
Sleep 1s
Type "cryptare compress project --format zip --level 9"
Enter
Wait
Sleep 1s
Type "ls -gGh server.log server.log.gz project.tar.gz project.zip"
Enter
Wait
Sleep 3.5s

# --- 4: decompress --------------------------------------------------------------
Hide
Type `banner "4/6  Decompress and extract, with limits against decompression bombs"`
Enter
Sleep 300ms
Show
Type "cryptare decompress server.log.gz -o server-copy.log"
Enter
Wait
Sleep 1s
Type "cryptare decompress project.tar.gz -o from-tar"
Enter
Wait
Sleep 1s
Type "cryptare decompress project.zip -o from-zip"
Enter
Wait
Sleep 1s
Type "cryptare decompress project.tar.gz --raw"
Enter
Wait
Sleep 1.2s
Type "cryptare decompress project.tar.gz -o too-big --max-size 100KB"
Enter
Wait
Sleep 3s
Type `cmp server.log server-copy.log && diff -r project from-tar && diff -r project from-zip && echo "all identical"`
Enter
Wait
Sleep 3s

# --- 5: stored keys -------------------------------------------------------------
Hide
Type `banner "5/6  Stored keys: generate one, then encrypt with it"`
Enter
Sleep 300ms
Show
Type "cryptare keys path"
Enter
Wait
Sleep 1s
Type "cryptare keys generate"
Enter
Wait /Enter master password to protect key:/
Sleep 400ms
Type@25ms "harbor quartz meadow lamp"
Enter
Wait /Confirm password:/
Sleep 300ms
Type@25ms "harbor quartz meadow lamp"
Enter
Wait
Sleep 1.5s
Type "cryptare keys list"
Enter
Wait
Sleep 1.5s
Type `KEY=$(cryptare keys list | awk 'NR==2 {print $1}')`
Enter
Wait
Sleep 500ms
Type `cryptare encrypt report.txt --key "$KEY"`
Enter
Wait /Enter master password for key/
Sleep 400ms
Type@25ms "harbor quartz meadow lamp"
Enter
Wait
Sleep 3s

# --- 6: back up and restore a key -----------------------------------------------
Hide
Type `banner "6/6  Back up a key, delete it, import it back, decrypt again"`
Enter
Sleep 300ms
Show
Type `cryptare keys export "$KEY" -o backup.ckey`
Enter
Wait /Enter master password for key/
Sleep 400ms
Type@25ms "harbor quartz meadow lamp"
Enter
Wait
Sleep 1.2s
Type `cryptare keys delete "$KEY"`
Enter
Wait+Screen /\[y.N\]:/
Sleep 1.5s
Type "y"
Enter
Wait
Sleep 1.2s
Type "cryptare decrypt report.txt.enc -o report-restored.txt"
Enter
Wait
Sleep 3s
Type "cryptare keys import backup.ckey"
Enter
Wait /Enter the key's master password:/
Sleep 400ms
Type@25ms "harbor quartz meadow lamp"
Enter
Wait
Sleep 1.2s
Type "cryptare decrypt report.txt.enc -o report-restored.txt"
Enter
Wait /Enter master password for key/
Sleep 400ms
Type@25ms "harbor quartz meadow lamp"
Enter
Wait
Sleep 1s
Type "cat report-restored.txt"
Enter
Wait
Sleep 4.5s

# --- clean up (not recorded) ----------------------------------------------------
Hide
Type `cd / && rm -rf "$DEMO"`
Enter
