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
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Run runs cryptare with the arguments in os.Args through Main, with a background
// context, and returns Main's exit status. main calls it, so the tests can run cryptare
// in a subprocess the same way (TestRunMain).
func Run(version string) int {
	return Main(context.Background(), version, os.Args[1:])
}

// Main runs cryptare with args, the command line without the program name, and returns
// its exit status: 0 on success; 128 plus the signal's number when SIGINT, SIGTERM or
// SIGHUP stops encrypt, decrypt, compress or decompress (130 for Ctrl+C); and 1 for any
// other error, which it reports on stderr. --version prints "cryptare version " followed
// by version; an empty version means there is no --version flag. Main reads its
// arguments only from args, never from os.Args: nil or empty args run cryptare without
// arguments, which opens the TUI. ctx reaches the commands, so cancelling it stops a
// running file command or the TUI, with exit status 1.
//
// Main doesn't call os.Exit, with one exception: SIGINT, SIGTERM, SIGQUIT or SIGHUP
// arriving while cryptare reads a hidden password from the terminal (a command run
// without --password or --password-file) restores the terminal and ends the process
// with os.Exit, status 128 plus the signal's number. Each call builds its own commands
// and flags, opens the key database only if a command needs it, and closes it before
// returning, so a program that embeds cryptare can call Main more than once.
func Main(ctx context.Context, version string, args []string) int {
	return mainWith(ctx, databaseOpener(os.Stderr), version, args)
}

// mainWith implements Main with open as the key database opener; tests replace it.
func mainWith(ctx context.Context, open DatabaseOpener, version string, args []string) int {
	var opened *Database
	defer func() {
		// The commands are done with the database by now: the TUI waits for a running
		// action, and a key list it may still be loading only reads (Close lets a
		// started query finish and refuses new ones). SQLite has committed every
		// change, so a failed close loses nothing; the standalone binary exits next.
		if opened != nil {
			if sqlDB, err := opened.Conn().DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	}()
	rootCmd := newRootCmd(func() (*Database, error) {
		db, err := open()
		opened = db
		return db, err
	}, version)
	if args == nil {
		args = []string{} // Cobra would read os.Args[1:] for nil arguments
	}
	rootCmd.SetArgs(args)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		return exitCode(err)
	}
	return 0
}

// exitCode is the exit status for a command that failed with err: 128 plus the
// signal's number for one stopped by a signal (130 for Ctrl+C), as shells report, and
// 1 otherwise.
func exitCode(err error) int {
	var interrupted *InterruptedError
	if errors.As(err, &interrupted) {
		return interrupted.ExitCode()
	}
	return 1
}

// databaseOpener opens the key database at CRYPTARE_DB_PATH or, when that isn't set,
// at the default path in the user's data folder (databasePath), creating that folder
// first and writing noticeLegacyDatabase's notice, if any, to stderr. The CLI calls it
// only for the keys commands, the TUI, encrypt --key and the decryption of a file
// encrypted with a stored key, so other commands never create anything. A database
// refused as untrusted (SEC-016) gets a hint about CRYPTARE_DB_PATH.
func databaseOpener(stderr io.Writer) DatabaseOpener {
	return func() (*Database, error) {
		path, fromEnv, err := databasePath()
		if err != nil {
			return nil, err
		}
		if !fromEnv {
			noticeLegacyDatabase(stderr, path)
			if err := prepareDefaultDatabaseFolder(path); err != nil {
				return nil, err
			}
		}
		db, err := NewDatabase(path)
		if errors.Is(err, ErrUntrustedDatabase) {
			return nil, fmt.Errorf("%w; set %s to use a key database of your own", err, databasePathEnv)
		}
		return db, err
	}
}

// newRootCmd returns the CLI root command with the build version attached (a
// non-empty Version makes Cobra register the --version flag) and "keys path"
// (newKeysPathCmd).
func newRootCmd(open DatabaseOpener, version string) *cobra.Command {
	cmd := NewRootCmdLazy(open)
	cmd.Version = version
	if keys, _, err := cmd.Find([]string{"keys"}); err == nil && keys != cmd {
		keys.AddCommand(newKeysPathCmd())
	}
	return cmd
}
