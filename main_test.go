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
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/jabbott-iii/Capsus/internal/launcher"
)

// call records one call of a fake tool's Main.
type call struct {
	tool    string
	version string
	args    []string
}

// testEnv is the surroundings of an app under test: output buffers, whether standard output
// is a terminal, and the calls of the fake tools and of the menu.
type testEnv struct {
	stdout, stderr bytes.Buffer
	terminal       bool
	menuErr        error
	menus          []bool // the vim argument of each menu call
	menuTools      [][]launcher.Tool
	calls          []call
}

// fake returns a fake tool whose Main records its call and returns code.
func (e *testEnv) fake(name, command string, hasTUI bool, code int) launcher.Tool {
	return launcher.Tool{
		Name:        name,
		Command:     command,
		Description: "fake " + name,
		HasTUI:      hasTUI,
		Vim:         hasTUI,
		Version:     "v0.1.0-" + command,
		Main: func(_ context.Context, version string, args []string) int {
			e.calls = append(e.calls, call{tool: command, version: version, args: args})
			return code
		},
	}
}

// fakes returns fake tools under the bundled tools' commands, each returning code.
func (e *testEnv) fakes(code int) []launcher.Tool {
	return []launcher.Tool{
		e.fake("Munus", "munus", true, code),
		e.fake("Cryptare", "cryptare", true, code),
		e.fake("Salus", "salus", false, code),
	}
}

// app returns an app that runs tools in e.
func (e *testEnv) app(tools []launcher.Tool) app {
	return app{
		tools:      tools,
		stdout:     &e.stdout,
		stderr:     &e.stderr,
		isTerminal: func() bool { return e.terminal },
		menu: func(_ context.Context, tools []launcher.Tool, vim bool) error {
			e.menus = append(e.menus, vim)
			e.menuTools = append(e.menuTools, tools)
			return e.menuErr
		},
	}
}

func TestToolCommandsPassArgumentsUnchanged(t *testing.T) {
	tests := [][]string{
		{"munus"},
		{"munus", "--help"},
		{"munus", "-h"},
		{"munus", "--version"},
		{"munus", "--vim"},
		{"munus", "list"},
		{"munus", "help", "add"},
		{"munus", "add", "--title", "two words", "--deadline=2026-12-01", "-x"},
		{"munus", "--", "--vim"},
		{"cryptare", "--vim"},
		{"cryptare", "encrypt", "file.txt", "--password-file", "pw.txt", "--", "--not-a-flag"},
		{"salus", "check", "run", "--only", "misconfig", "--json"},
		{"salus", "--version", "--help", "-v"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var e testEnv
			e.terminal = true
			code := e.app(e.fakes(0)).run(context.Background(), append([]string(nil), args...))
			if code != 0 {
				t.Errorf("exit code %d, want 0; stderr: %s", code, e.stderr.String())
			}
			want := []call{{tool: args[0], version: "v0.1.0-" + args[0], args: args[1:]}}
			if !reflect.DeepEqual(e.calls, want) {
				t.Errorf("calls = %#v, want %#v", e.calls, want)
			}
			if e.stdout.Len() != 0 || e.stderr.Len() != 0 {
				t.Errorf("capsus wrote output of its own: stdout %q, stderr %q", e.stdout.String(), e.stderr.String())
			}
			if len(e.menus) != 0 {
				t.Errorf("a tool command opened the menu")
			}
		})
	}
}

func TestToolCommandsReturnTheToolsExitCode(t *testing.T) {
	for _, code := range []int{0, 1, 2, 3, 130, 255} {
		for _, tool := range []string{"munus", "cryptare", "salus"} {
			var e testEnv
			if got := e.app(e.fakes(code)).run(context.Background(), []string{tool, "x"}); got != code {
				t.Errorf("capsus %s x exited with %d, want the tool's %d", tool, got, code)
			}
			if e.stderr.Len() != 0 {
				t.Errorf("capsus %s x: capsus wrote to stderr: %q", tool, e.stderr.String())
			}
		}
	}
}

func TestUsageErrorsExitWith2(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"no-such-tool"}, `unknown command "no-such-tool" for "capsus"`},
		{[]string{"no-such-tool", "arg"}, `unknown command "no-such-tool" for "capsus"`},
		{[]string{"Munus"}, `unknown command "Munus" for "capsus"`},
		{[]string{"list", "extra"}, `unknown command "extra" for "capsus list"`},
		{[]string{"--no-such-flag"}, "unknown flag: --no-such-flag"},
		{[]string{"completion", "bash"}, `unknown command "completion" for "capsus"`},
		// Cobra checks for unknown commands only before a "--".
		{[]string{"--", "no-such-tool"}, `unknown command "no-such-tool" for "capsus"`},
		{[]string{"--vim", "--", "munus"}, `unknown command "munus" for "capsus"`},
		// A tool gets only the arguments after its name, so anything before it is refused,
		// including words Cobra takes as the value of an unknown flag.
		{[]string{"--vim", "munus"}, "arguments go after the tool name: capsus munus [args...]"},
		{[]string{"--vim", "salus", "check"}, "arguments go after the tool name: capsus salus [args...]"},
		{[]string{"-v", "munus"}, "arguments go after the tool name: capsus munus [args...]"},
		{[]string{"--version", "--vim", "cryptare", "keys", "list"}, "arguments go after the tool name: capsus cryptare [args...]"},
		{[]string{"--x=1", "munus"}, "arguments go after the tool name: capsus munus [args...]"},
		{[]string{"--foo", "bar", "munus"}, "arguments go after the tool name: capsus munus [args...]"},
		{[]string{"--vim", "", "munus"}, "arguments go after the tool name: capsus munus [args...]"},
		{[]string{"-h", "x", "munus"}, "arguments go after the tool name: capsus munus [args...]"},
		{[]string{"--title", "t", "munus", "add"}, "arguments go after the tool name: capsus munus [args...]"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var e testEnv
			e.terminal = true
			if code := e.app(e.fakes(0)).run(context.Background(), tt.args); code != 2 {
				t.Errorf("exit code %d, want 2", code)
			}
			const hint = "Run 'capsus --help' for usage.\n"
			stderr := e.stderr.String()
			if !strings.HasPrefix(stderr, "capsus: "+tt.want) || strings.Count(stderr, tt.want) != 1 ||
				!strings.HasSuffix(stderr, hint) || strings.Count(stderr, hint) != 1 {
				t.Errorf("stderr = %q, want %q once, then the usage hint once", stderr, "capsus: "+tt.want)
			}
			if e.stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", e.stdout.String())
			}
			if len(e.calls) != 0 || len(e.menus) != 0 {
				t.Errorf("a usage error ran a tool or the menu: calls %#v, menus %v", e.calls, e.menus)
			}
		})
	}
}

func TestList(t *testing.T) {
	var e testEnv
	if code := e.app(e.fakes(0)).run(context.Background(), []string{"list"}); code != 0 {
		t.Fatalf("exit code %d, want 0; stderr: %s", code, e.stderr.String())
	}
	want := "" +
		"Munus     fake Munus     TUI\n" +
		"Cryptare  fake Cryptare  TUI\n" +
		"Salus     fake Salus     CLI only\n"
	if e.stdout.String() != want {
		t.Errorf("capsus list wrote\n%q\nwant\n%q", e.stdout.String(), want)
	}
	if len(e.calls) != 0 || len(e.menus) != 0 {
		t.Errorf("capsus list ran a tool or the menu")
	}
}

// TestListOfBundledToolsMatchesSmokeTest checks the lines scripts/smoke.sh looks for.
func TestListOfBundledToolsMatchesSmokeTest(t *testing.T) {
	var e testEnv
	if code := e.app(launcher.Bundled()).run(context.Background(), []string{"list"}); code != 0 {
		t.Fatalf("exit code %d, want 0; stderr: %s", code, e.stderr.String())
	}
	for _, re := range []string{
		`(?m)^[Mm]unus[[:space:]].*[^[:alnum:]]TUI([^[:alnum:]]|$)`,
		`(?m)^[Cc]ryptare[[:space:]].*[^[:alnum:]]TUI([^[:alnum:]]|$)`,
		`(?m)^[Ss]alus[[:space:]].*CLI only`,
	} {
		if !regexp.MustCompile(re).MatchString(e.stdout.String()) {
			t.Errorf("no line of capsus list matches %s:\n%s", re, e.stdout.String())
		}
	}
	if n := strings.Count(e.stdout.String(), "\n"); n != 3 {
		t.Errorf("capsus list wrote %d lines, want 3", n)
	}
}

func TestVersion(t *testing.T) {
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			var e testEnv
			e.terminal = true
			if code := e.app(e.fakes(0)).run(context.Background(), []string{flag}); code != 0 {
				t.Fatalf("exit code %d, want 0; stderr: %s", code, e.stderr.String())
			}
			want := "capsus version " + version + "\n" +
				"munus version v0.1.0-munus\n" +
				"cryptare version v0.1.0-cryptare\n" +
				"salus version v0.1.0-salus\n"
			if e.stdout.String() != want {
				t.Errorf("capsus %s wrote %q, want %q", flag, e.stdout.String(), want)
			}
			if len(e.calls) != 0 || len(e.menus) != 0 {
				t.Errorf("capsus %s ran a tool or the menu", flag)
			}
		})
	}
}

func TestVersionOfBundledTools(t *testing.T) {
	var e testEnv
	if code := e.app(launcher.Bundled()).run(context.Background(), []string{"--version"}); code != 0 {
		t.Fatalf("exit code %d, want 0; stderr: %s", code, e.stderr.String())
	}
	want := []string{"capsus version " + version}
	for _, tool := range launcher.Bundled() {
		want = append(want, tool.Command+" version "+tool.Version)
	}
	if got := strings.Split(strings.TrimSuffix(e.stdout.String(), "\n"), "\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("capsus --version wrote %q, want %q", got, want)
	}
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestOutputErrorsExitWith1(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var e testEnv
			a := e.app(e.fakes(0))
			a.stdout = failingWriter{}
			if code := a.run(context.Background(), args); code != 1 {
				t.Errorf("exit code %d, want 1", code)
			}
			if want := "capsus: write failed\n"; e.stderr.String() != want {
				t.Errorf("stderr = %q, want %q", e.stderr.String(), want)
			}
		})
	}
}

func TestHelp(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"--help"}, []string{"munus", "fake Munus", "cryptare", "fake Cryptare", "salus", "fake Salus", "list", "--vim", "--version"}},
		{[]string{"-h", "munus"}, []string{"Available Commands:"}},
		{[]string{"help", "munus"}, []string{"fake Munus", "Run 'capsus munus --help' for its commands."}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var e testEnv
			if code := e.app(e.fakes(0)).run(context.Background(), tt.args); code != 0 {
				t.Errorf("exit code %d, want 0; stderr: %s", code, e.stderr.String())
			}
			for _, s := range tt.want {
				if !strings.Contains(e.stdout.String(), s) {
					t.Errorf("help lacks %q:\n%s", s, e.stdout.String())
				}
			}
			if strings.Contains(e.stdout.String(), "completion") {
				t.Errorf("help lists a completion command:\n%s", e.stdout.String())
			}
			if len(e.calls) != 0 || len(e.menus) != 0 {
				t.Errorf("help ran a tool or the menu: calls %#v", e.calls)
			}
		})
	}
}

func TestMenu(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		terminal bool
		menuErr  error
		code     int
		menus    []bool
		stderr   string
	}{
		{"no arguments", []string{}, true, nil, 0, []bool{false}, ""},
		{"vim", []string{"--vim"}, true, nil, 0, []bool{true}, ""},
		{"not a terminal", []string{}, false, nil, 2, nil,
			"capsus: the menu needs a terminal; run 'capsus list' or 'capsus <tool> [args...]'\nRun 'capsus --help' for usage.\n"},
		{"menu fails", []string{}, true, errors.New("could not open a new TTY"), 1, []bool{false}, "capsus: could not open a new TTY\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e testEnv
			e.terminal, e.menuErr = tt.terminal, tt.menuErr
			tools := e.fakes(0)
			if code := e.app(tools).run(context.Background(), tt.args); code != tt.code {
				t.Errorf("exit code %d, want %d; stderr: %s", code, tt.code, e.stderr.String())
			}
			if !reflect.DeepEqual(e.menus, tt.menus) {
				t.Errorf("menu calls (vim) = %v, want %v", e.menus, tt.menus)
			}
			if e.stderr.String() != tt.stderr {
				t.Errorf("stderr = %q, want %q", e.stderr.String(), tt.stderr)
			}
			if e.stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", e.stdout.String())
			}
			for _, got := range e.menuTools {
				if len(got) != len(tools) || got[0].Name != tools[0].Name {
					t.Errorf("the menu got tools %v, want the registry", got)
				}
			}
			if len(e.calls) != 0 {
				t.Errorf("capsus ran a tool itself: %#v", e.calls)
			}
		})
	}
}

// TestNilArgumentsDontReadOSArgs pins that run never lets Cobra fall back to os.Args, which
// it does for nil arguments.
func TestNilArgumentsDontReadOSArgs(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{"capsus", "munus", "from-os-args"}
	var e testEnv
	e.terminal = true
	if code := e.app(e.fakes(0)).run(context.Background(), nil); code != 0 {
		t.Errorf("exit code %d, want 0; stderr: %s", code, e.stderr.String())
	}
	if len(e.calls) != 0 || !reflect.DeepEqual(e.menus, []bool{false}) {
		t.Errorf("nil arguments ran %#v and the menu %v times; want the menu once and no tool", e.calls, len(e.menus))
	}
}

func TestContextReachesToolsAndMenu(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "from main")
	var seen []any
	tool := launcher.Tool{Name: "Munus", Command: "munus", Version: "v1", Main: func(ctx context.Context, _ string, _ []string) int {
		seen = append(seen, ctx.Value(key{}))
		return 0
	}}
	a := app{
		tools:      []launcher.Tool{tool},
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		isTerminal: func() bool { return true },
		menu: func(ctx context.Context, _ []launcher.Tool, _ bool) error {
			seen = append(seen, ctx.Value(key{}))
			return nil
		},
	}
	a.run(ctx, []string{"munus"})
	a.run(ctx, []string{})
	if want := []any{"from main", "from main"}; !reflect.DeepEqual(seen, want) {
		t.Errorf("context values seen by the tool and the menu = %v, want %v", seen, want)
	}
}

// isolate points the tools' databases and the user's directories into a temporary folder,
// and returns it.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MUNUS_DB_PATH", filepath.Join(dir, "munus.db"))
	t.Setenv("CRYPTARE_DB_PATH", filepath.Join(dir, "cryptare.db"))
	t.Setenv("SALUS_DB_PATH", filepath.Join(dir, "salus.db"))
	home := filepath.Join(dir, "home")
	for _, v := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(v, home)
	}
	return dir
}

// TestBundledToolsRunInOneProcess runs the real tools one after another in this process, as
// the menu and repeated commands do, and checks that each call returns the tool's own exit
// code and uses the tool's database variable.
func TestBundledToolsRunInOneProcess(t *testing.T) {
	dir := isolate(t)
	steps := []struct {
		args []string
		code int
	}{
		{[]string{"munus", "--version"}, 0},
		{[]string{"salus", "check", "list"}, 0},
		{[]string{"cryptare", "--version"}, 0},
		{[]string{"munus", "list"}, 0},
		{[]string{"cryptare", "keys", "list"}, 0},
		{[]string{"salus", "check", "run", "--only", "no-such-check"}, 3},
		{[]string{"cryptare", "no-such-command"}, 1},
		{[]string{"munus", "no-such-command"}, 1},
		{[]string{"salus", "--version"}, 0},
		{[]string{"munus", "--version"}, 0},
	}
	for _, s := range steps {
		var e testEnv
		if code := e.app(launcher.Bundled()).run(context.Background(), s.args); code != s.code {
			t.Errorf("capsus %s exited with %d, want %d", strings.Join(s.args, " "), code, s.code)
		}
		if e.stdout.Len() != 0 || e.stderr.Len() != 0 {
			t.Errorf("capsus %s: capsus wrote output of its own: %q %q", strings.Join(s.args, " "), e.stdout.String(), e.stderr.String())
		}
	}
	for _, db := range []string{"munus.db", "cryptare.db", "salus.db"} {
		if _, err := os.Stat(filepath.Join(dir, db)); err != nil {
			t.Errorf("a tool did not use its database variable: %v", err)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if isTerminal(f) {
		t.Errorf("a regular file counts as a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	if isTerminal(w) {
		t.Errorf("a pipe counts as a terminal")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows can't tell NUL from a console, and has no /dev/ptmx")
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = null.Close() })
	if isTerminal(null) {
		t.Errorf("%s counts as a terminal", os.DevNull)
	}
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal to check the positive case: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })
	if !isTerminal(ptmx) {
		t.Errorf("a pseudo-terminal does not count as a terminal")
	}
}
