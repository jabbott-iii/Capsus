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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/jabbott-iii/Capsus/internal/launcher"
)

// version is reported by --version. Builds set it with -ldflags "-X main.version=<version>";
// it is only written by the linker.
var version = "dev"

// Exit codes of capsus itself. A tool's exit code is passed on unchanged.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	a := app{
		tools:      launcher.Bundled(),
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		isTerminal: func() bool { return isTerminal(os.Stdout) },
		menu:       launcher.RunMenu,
	}
	// No signal handling here: Munus relies on Ctrl+C's default behaviour, and Cryptare and
	// Salus install their own handlers around the operations that need them.
	os.Exit(a.run(context.Background(), os.Args[1:]))
}

// app is the capsus command line with its surroundings, which tests replace.
type app struct {
	tools      []launcher.Tool
	stdout     io.Writer
	stderr     io.Writer
	isTerminal func() bool
	menu       func(ctx context.Context, tools []launcher.Tool, vim bool) error
}

// failure marks an error in capsus' own work, as opposed to a usage error.
type failure struct{ err error }

func (f failure) Error() string { return f.err.Error() }
func (f failure) Unwrap() error { return f.err }

// run runs the capsus command line with args, which exclude the program name, and returns the
// exit code: the dispatched tool's, or one of capsus' own.
func (a app) run(ctx context.Context, args []string) int {
	if args == nil {
		args = []string{} // nil would make Cobra read os.Args
	}
	code := exitOK
	root := a.rootCmd(&code, args)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		var f failure
		if errors.As(err, &f) {
			_, _ = fmt.Fprintf(a.stderr, "capsus: %v\n", err)
			return exitError
		}
		_, _ = fmt.Fprintf(a.stderr, "capsus: %v\nRun 'capsus --help' for usage.\n", err)
		return exitUsage
	}
	return code
}

// rootCmd builds the command tree for the command line args. A tool subcommand stores the
// tool's exit code in code.
func (a app) rootCmd(code *int, args []string) *cobra.Command {
	var vim, showVersion bool
	root := &cobra.Command{
		Use:   "capsus",
		Short: "Munus, Cryptare and Salus in one binary",
		Long: "Capsus bundles the terminal tools Munus, Cryptare and Salus. Run without arguments\n" +
			"in a terminal, it opens a menu of the tools; 'capsus <tool> [args...]' runs a tool's\n" +
			"own command line, and 'capsus list' lists the tools.",
		SilenceErrors: true,
		SilenceUsage:  true,
		// The command line is D-5's: no completion command, which could not complete the tools'
		// arguments anyway, since Capsus doesn't parse them.
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cobra checks the arguments for unknown commands only before a "--".
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			if showVersion {
				if err := launcher.WriteVersions(a.stdout, version, a.tools); err != nil {
					return failure{err}
				}
				return nil
			}
			if !a.isTerminal() {
				return errors.New("the menu needs a terminal; run 'capsus list' or 'capsus <tool> [args...]'")
			}
			if err := a.menu(cmd.Context(), a.tools, vim); err != nil {
				return failure{err}
			}
			return nil
		},
	}
	root.Flags().BoolVar(&vim, "vim", false, "pass --vim (Vim key bindings) to the TUI tools the menu opens")
	root.Flags().BoolVarP(&showVersion, "version", "v", false, "print the Capsus version and the bundled tool versions")
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)

	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the bundled tools: name, description and TUI or CLI only",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := launcher.WriteList(a.stdout, a.tools); err != nil {
				return failure{err}
			}
			return nil
		},
	})
	for _, t := range a.tools {
		root.AddCommand(&cobra.Command{
			Use:   t.Command + " [args...]",
			Short: t.Description,
			Long: fmt.Sprintf("%s\n\nRuns %s's own command line with the arguments unchanged. "+
				"Run 'capsus %s --help' for its commands.", t.Description, t.Name, t.Command),
			// The tool parses its own arguments, --help and --version included.
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, toolArgs []string) error {
				// Cobra also runs this command when flags or other words come before the tool's
				// name, and passes them on with the rest. A tool gets only what follows its name.
				if len(args) == 0 || args[0] != t.Command {
					return fmt.Errorf("arguments go after the tool name: capsus %s [args...]", t.Command)
				}
				*code = t.Run(cmd.Context(), toolArgs)
				return nil
			},
		})
	}
	return root
}

// isTerminal reports whether f is a character device, such as a terminal, and outside Windows
// not the null device. Only standard output is checked: when standard input is redirected,
// Bubble Tea reads the keys from the terminal itself.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	// Windows gives consoles and NUL alike no file identity, so os.SameFile can't tell them
	// apart there, and NUL counts as a terminal.
	if runtime.GOOS == "windows" {
		return true
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(fi, null)
}
