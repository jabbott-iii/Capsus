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

// Package launcher holds the Capsus tool registry (Bundled), the plain-text listings and the
// menu TUI. Everything but Bundled takes the tools as a parameter, so it can be tested with
// fake tools.
package launcher

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// MainFunc is a bundled tool's entry point, its pkg.Main: it runs the tool's command line
// with args, which exclude the program name, reports version for --version and returns the
// tool's exit code.
type MainFunc func(ctx context.Context, version string, args []string) int

// Tool is one registry entry: what the menu, `capsus list` and `capsus --version` show, and
// the tool's entry point.
type Tool struct {
	// Name is the display name, such as "Munus".
	Name string
	// Command is the capsus subcommand that runs the tool, such as "munus".
	Command string
	// Description is the short description shown by the menu and `capsus list`.
	Description string
	// HasTUI reports whether the tool opens a TUI when it runs without arguments.
	HasTUI bool
	// Vim reports whether the tool accepts --vim, which `capsus --vim` passes on to it.
	Vim bool
	// Version is the release tag the tool's folder was imported at.
	Version string
	// Main is the tool's entry point.
	Main MainFunc
}

// Run calls the tool's Main with its bundled version and args, and returns the tool's exit
// code unchanged. Nil args are passed as an empty slice: a Cobra command whose arguments are
// nil reads os.Args instead.
func (t Tool) Run(ctx context.Context, args []string) int {
	if args == nil {
		args = []string{}
	}
	return t.Main(ctx, t.Version, args)
}

// kind is the label the menu and `capsus list` show for the tool.
func (t Tool) kind() string {
	if t.HasTUI {
		return "TUI"
	}
	return "CLI only"
}

// WriteList writes one line per tool to w: the name, the description, and "TUI" or
// "CLI only", in columns separated by spaces.
func WriteList(w io.Writer, tools []Tool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, t := range tools {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, t.Description, t.kind()); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// WriteVersions writes "capsus version <version>" to w, then one line per tool in the
// format of the tool's own --version: "<command> version <bundled version>".
func WriteVersions(w io.Writer, version string, tools []Tool) error {
	if _, err := fmt.Fprintf(w, "capsus version %s\n", version); err != nil {
		return err
	}
	for _, t := range tools {
		if _, err := fmt.Fprintf(w, "%s version %s\n", t.Command, t.Version); err != nil {
			return err
		}
	}
	return nil
}
