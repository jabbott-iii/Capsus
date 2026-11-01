/*
Copyright 2026 Joseph Anthony Abbott III

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pkg

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	databasePathEnv = "CRYPTARE_DB_PATH"

	// databaseFileName is the key database's name in the per-user data folder, and in
	// the current folder, where versions before Q-003 kept it by default.
	databaseFileName = "cryptare.db"

	// dataFolderName is Cryptare's folder inside the per-user data folder.
	dataFolderName = "cryptare"
)

// errNoDataFolder is returned when the default key database path can't be worked out
// because the system doesn't say where the user's data folder is.
var errNoDataFolder = errors.New("can't find your user data folder for the key database")

// databasePath returns the key database's path, and whether it came from
// CRYPTARE_DB_PATH. When that isn't set, the path is cryptare/cryptare.db in the user's
// data folder (userDataDir), so the key store no longer depends on the current folder
// (Q-003, SEC-010, SEC-016).
func databasePath() (path string, fromEnv bool, err error) {
	if path := os.Getenv(databasePathEnv); path != "" {
		return path, true, nil
	}
	dir, err := userDataDir(runtime.GOOS, os.Getenv)
	if err != nil {
		return "", false, err
	}
	return filepath.Join(dir, dataFolderName, databaseFileName), false, nil
}

// userDataDir returns the folder for the user's application data on goos, reading the
// environment through getenv:
//   - Windows: %LocalAppData%, which isn't copied to other machines with a roaming
//     profile;
//   - macOS: ~/Library/Application Support, as os.UserConfigDir returns;
//   - elsewhere: $XDG_DATA_HOME or ~/.local/share, following the XDG Base Directory
//     specification, which ignores a relative $XDG_DATA_HOME.
//
// A folder that isn't an absolute path is refused, since it would make the key store
// depend on the current folder again.
func userDataDir(goos string, getenv func(string) string) (string, error) {
	var name, dir string
	switch goos {
	case "windows":
		name, dir = "%LocalAppData%", getenv("LocalAppData")
	case "darwin", "ios":
		name, dir = "$HOME", getenv("HOME")
		if filepath.IsAbs(dir) {
			dir = filepath.Join(dir, "Library", "Application Support")
		}
	default:
		if xdg := getenv("XDG_DATA_HOME"); filepath.IsAbs(xdg) {
			return xdg, nil
		}
		name, dir = "$HOME", getenv("HOME")
		if filepath.IsAbs(dir) {
			dir = filepath.Join(dir, ".local", "share")
		}
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%w: %s is not set to an absolute path; set %s to the key database's path",
			errNoDataFolder, name, databasePathEnv)
	}
	return dir, nil
}

// prepareDefaultDatabaseFolder creates the folder of the default key database, private
// to the user (0700), as the XDG specification asks for new data folders.
func prepareDefaultDatabaseFolder(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create key database folder: %w", err)
	}
	return nil
}

// noticeLegacyDatabase tells the user, on w, about a cryptare.db in the current folder,
// where earlier versions kept the key database by default, when the default path path
// is in use (Q-003). It says how to keep using it but never opens or moves it: the file
// may not be the user's (SEC-016), and only the user knows whether it is.
func noticeLegacyDatabase(w io.Writer, path string) {
	legacy, err := os.Stat(databaseFileName)
	if err != nil || !legacy.Mode().IsRegular() {
		return
	}
	current, err := os.Stat(path)
	if err == nil && os.SameFile(legacy, current) {
		return // the current folder is the data folder
	}
	var advice string
	if errors.Is(err, fs.ErrNotExist) {
		move := "mv"
		if runtime.GOOS == "windows" {
			move = "move"
		}
		advice = fmt.Sprintf("To keep using the keys in it, move it there:\n    %s %s %s\nor set %s to its path.",
			move, databaseFileName, quotePath(path), databasePathEnv)
	} else {
		advice = fmt.Sprintf("To use the keys in it, set %s to its path.", databasePathEnv)
	}
	_, _ = fmt.Fprintf(w, "notice: there is a %s in the current folder. Earlier versions of Cryptare kept the key\n"+
		"database there; it is now %s.\n%s\n", databaseFileName, path, advice)
}

// quotePath quotes path for the shell named in noticeLegacyDatabase's advice.
func quotePath(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + path + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// newKeysPathCmd returns "keys path", which prints the key database's path without
// opening or creating anything (Q-003).
func newKeysPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the path of the key database",
		Long: "Print the path of the key database: " + databasePathEnv + " when it is set, and otherwise\n" +
			"cryptare/cryptare.db in your user data folder. Nothing is opened or created.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true // a failure here isn't a command-line mistake
			path, fromEnv, err := databasePath()
			if err != nil {
				return err
			}
			if !fromEnv {
				noticeLegacyDatabase(cmd.ErrOrStderr(), path)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			return nil
		},
	}
}
