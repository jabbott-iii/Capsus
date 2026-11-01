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
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Main runs the Salus command line with args, which exclude the program name,
// and returns the process exit code: 0/1/2 for a check run's PASS/WARN/FAIL
// result, and ExitCodeError for operational errors. version is reported by
// --version; an empty version leaves the root command without a --version
// flag. Command output goes to os.Stdout and errors to os.Stderr only.
//
// ctx reaches every command; check run stops its checks when ctx ends. Main
// itself never calls os.Exit, and never reads os.Args (nil args means no
// arguments).
// Each call builds a new command tree and opens the database at most once,
// lazily, closing it before returning, so a program that embeds Salus can
// call Main more than once in one process, one call at a time. Only a check
// still blocked in a system call when its time limit passes, which runCheck
// abandons, can outlive the call.
func Main(ctx context.Context, version string, args []string) int {
	return run(ctx, version, args, os.Stdout, os.Stderr)
}

// run is Main with the output streams as parameters, so tests can read them.
func run(ctx context.Context, version string, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, func(openDB DatabaseOpener) *cobra.Command {
		return NewVersionedRootCmd(openDB, version)
	}, args, stdout, stderr)
}

// runWith is run with the command tree's constructor as a parameter, so
// tests can exercise the panic handling and the database close. Returning
// instead of exiting lets the database close first.
func runWith(ctx context.Context, build func(DatabaseOpener) *cobra.Command, args []string, stdout, stderr io.Writer) (code int) {
	// The database is opened only when a command needs it, at most once.
	var db *Database
	openDB := func() (*Database, error) {
		if db != nil {
			return db, nil
		}
		path, err := DatabasePath()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
		opened, err := OpenDatabase(path)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
		db = opened
		return db, nil
	}
	defer func() {
		if db == nil {
			return
		}
		if err := db.Close(); err != nil {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
			if code == ExitCodePass {
				code = ExitCodeError
			}
		}
	}()

	// A panic is a bug in Salus, not a check result. Without this, Go would
	// exit with 2, the code for a FAIL result. Registered after the database
	// close above, so it runs first and the database still closes.
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(stderr, "Error: internal error: %v\n%s", r, debug.Stack())
			code = ExitCodeError
		}
	}()

	if args == nil {
		// Cobra executes os.Args[1:] when the arguments are nil, which would
		// be the embedding program's own command line.
		args = []string{}
	}
	rootCmd := build(openDB)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	cmd, err := rootCmd.ExecuteContextC(ctx)
	code = ExitCode(err)
	if code == ExitCodeError {
		// The command tree silences Cobra's own error printing, so errors are
		// reported here, on stderr only, and never mix into stdout or --json.
		if cmd == nil {
			cmd = rootCmd
		}
		_, _ = fmt.Fprintf(stderr, "Error: %v\nRun '%s --help' for usage.\n", err, cmd.CommandPath())
	}
	return code
}
