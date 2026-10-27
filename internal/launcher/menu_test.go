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

package launcher

import (
	"context"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// keyMsg returns the key message Bubble Tea sends for key, in the notation of
// tea.KeyMsg.String.
func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

// press sends keys to m in order and returns the resulting model and the command of the last
// key.
func press(t *testing.T, m menuModel, keys ...string) (menuModel, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		next, cmd = m.Update(keyMsg(k))
		m = next.(menuModel)
	}
	return m, cmd
}

// ansiCodes matches the SGR escape sequences that lipgloss styles add when the environment
// forces colours.
var ansiCodes = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain is the text of a rendered view without its styling.
func plain(s string) string {
	return ansiCodes.ReplaceAllString(s, "")
}

// isQuit reports whether cmd is tea.Quit.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// threeTools returns fake tools shaped like the bundled ones: two TUI tools that accept
// --vim, then a CLI-only tool.
func threeTools(r *recorder) []Tool {
	return []Tool{
		r.tool("Munus", true, true, 0),
		r.tool("Cryptare", true, true, 0),
		r.tool("Salus", false, false, 0),
	}
}

func TestMenuMovement(t *testing.T) {
	tests := []struct {
		keys []string
		want int
	}{
		{nil, 0},
		{[]string{"down"}, 1},
		{[]string{"j"}, 1},
		{[]string{"j", "j"}, 2},
		{[]string{"j", "j", "j", "down"}, 2},
		{[]string{"j", "j", "up"}, 1},
		{[]string{"j", "j", "k"}, 1},
		{[]string{"k", "up"}, 0},
		{[]string{"j", "x", "?"}, 1},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, ","), func(t *testing.T) {
			var r recorder
			m, cmd := press(t, newMenuModel(threeTools(&r)), tt.keys...)
			if m.cursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.want)
			}
			if cmd != nil || m.chosen != -1 {
				t.Errorf("moving returned a command or chose a tool (chosen %d)", m.chosen)
			}
			if len(r.calls) != 0 {
				t.Errorf("moving called a tool: %#v", r.calls)
			}
		})
	}
}

func TestMenuEnterOnTUIToolQuitsWithIt(t *testing.T) {
	var r recorder
	m, cmd := press(t, newMenuModel(threeTools(&r)), "j", "enter")
	if m.chosen != 1 {
		t.Errorf("chosen = %d, want 1 (Cryptare)", m.chosen)
	}
	if !isQuit(cmd) {
		t.Errorf("Enter on a TUI tool did not quit the menu program")
	}
	if v := m.View(); v != "" {
		t.Errorf("View after choosing a tool = %q, want it cleared", v)
	}
	if len(r.calls) != 0 {
		t.Errorf("Update called a tool: %#v", r.calls)
	}
}

func TestMenuEnterOnCLIOnlyToolShowsHint(t *testing.T) {
	var r recorder
	tools := threeTools(&r)
	tools[2].Command = "salus"
	m, cmd := press(t, newMenuModel(tools), "j", "j", "enter")
	if cmd != nil || m.chosen != -1 {
		t.Fatalf("Enter on a CLI-only tool returned a command or chose it (chosen %d)", m.chosen)
	}
	want := "Salus has no TUI. Run `capsus salus --help` for its commands."
	if m.message != want {
		t.Errorf("message = %q, want %q", m.message, want)
	}
	if !strings.Contains(plain(m.View()), want) {
		t.Errorf("View does not show the hint:\n%s", m.View())
	}
	if m, _ = press(t, m, "k"); m.message != "" {
		t.Errorf("moving kept the hint: %q", m.message)
	}
	if len(r.calls) != 0 {
		t.Errorf("Update called a tool: %#v", r.calls)
	}
}

func TestMenuMovingDownClearsTheMessage(t *testing.T) {
	var r recorder
	m := newMenuModel(threeTools(&r))
	m.message = "Munus exited with status 1."
	if m, _ = press(t, m, "j"); m.cursor != 1 || m.message != "" {
		t.Errorf("after j: cursor %d, message %q; want 1 and no message", m.cursor, m.message)
	}
}

func TestMenuQuitKeys(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			var r recorder
			m, cmd := press(t, newMenuModel(threeTools(&r)), "j", key)
			if !isQuit(cmd) {
				t.Errorf("%s did not quit", key)
			}
			if m.chosen != -1 {
				t.Errorf("%s chose tool %d", key, m.chosen)
			}
			if v := m.View(); v != "" {
				t.Errorf("View after quitting = %q, want it cleared", v)
			}
		})
	}
}

func TestMenuWithoutTools(t *testing.T) {
	m, cmd := press(t, newMenuModel(nil), "j", "k", "enter")
	if cmd != nil || m.chosen != -1 || m.cursor != 0 {
		t.Errorf("empty menu: cursor %d, chosen %d, command %v", m.cursor, m.chosen, cmd != nil)
	}
	if _, cmd := press(t, m, "q"); !isQuit(cmd) {
		t.Errorf("q did not quit the empty menu")
	}
}

func TestMenuView(t *testing.T) {
	var r recorder
	m, _ := press(t, newMenuModel(threeTools(&r)), "j")
	view := plain(m.View())
	lines := strings.Split(view, "\n")
	var rows []string
	for _, l := range lines {
		if strings.Contains(l, "description of") {
			rows = append(rows, l)
		}
	}
	want := []string{
		"  Munus     description of Munus     TUI",
		"> Cryptare  description of Cryptare  TUI",
		"  Salus     description of Salus     CLI only",
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows =\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	for _, s := range []string{"choose a tool", "j/k", "enter", "q: quit"} {
		if !strings.Contains(view, s) {
			t.Errorf("View lacks %q:\n%s", s, view)
		}
	}
}

func TestMenuViewFitsTheTerminalWidth(t *testing.T) {
	var r recorder
	tools := threeTools(&r)
	tools[0].Description = strings.Repeat("long description ", 10)
	m := newMenuModel(tools)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(menuModel)
	view := plain(m.View())
	widest := 0
	for _, l := range strings.Split(view, "\n") {
		widest = max(widest, lipgloss.Width(l))
	}
	if widest != 60 {
		t.Errorf("the widest line is %d cells, want the terminal's 60:\n%s", widest, view)
	}
	if !strings.Contains(view, "…  TUI") {
		t.Errorf("the long description was not cut with an ellipsis:\n%s", view)
	}
	if !strings.Contains(view, "Salus") || !strings.Contains(view, "CLI only") {
		t.Errorf("a row lost its name or kind:\n%s", view)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"abc", 5, "abc  "},
		{"abc", 3, "abc"},
		{"abcdef", 4, "abc…"},
		{"abcdef", 1, "…"},
		{"abcdef", 0, ""},
		{"日本語テキスト", 5, "日本…"},
		{"日本語", 4, "日… "},
	}
	for _, tt := range tests {
		if got := truncate(tt.s, tt.width); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

// script returns a show function for runMenu that plays one key sequence per menu program,
// in order, as Bubble Tea would deliver them, and fails the test when a tool runs while a
// menu program is running or when the menu is shown more often than there are sequences.
func script(t *testing.T, running *bool, sequences ...[]string) (show func(menuModel) (menuModel, error), shown *int) {
	t.Helper()
	n := 0
	return func(m menuModel) (menuModel, error) {
		*running = true
		defer func() { *running = false }()
		if n >= len(sequences) {
			t.Fatalf("menu shown %d times, want %d", n+1, len(sequences))
		}
		keys := sequences[n]
		n++
		for i, k := range keys {
			next, cmd := m.Update(keyMsg(k))
			m = next.(menuModel)
			if isQuit(cmd) {
				if i != len(keys)-1 {
					t.Fatalf("menu quit at key %d of %v", i+1, keys)
				}
				return m, nil
			}
		}
		t.Fatalf("key sequence %v does not quit the menu", keys)
		return m, nil
	}, &n
}

// guarded wraps each tool's Main so that the test fails when it runs during a menu program.
func guarded(t *testing.T, tools []Tool, running *bool) []Tool {
	t.Helper()
	out := make([]Tool, len(tools))
	for i, tool := range tools {
		inner := tool.Main
		tool.Main = func(ctx context.Context, version string, args []string) int {
			if *running {
				t.Errorf("%s ran while the menu program was running", tool.Name)
			}
			return inner(ctx, version, args)
		}
		out[i] = tool
	}
	return out
}

func TestRunMenuLaunchesToolsBetweenMenus(t *testing.T) {
	tests := []struct {
		name string
		vim  bool
		want [][]string
	}{
		{"without --vim", false, [][]string{{}, {}}},
		{"with --vim", true, [][]string{{"--vim"}, {"--vim"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r recorder
			var running bool
			tools := guarded(t, threeTools(&r), &running)
			show, shown := script(t, &running,
				[]string{"enter"},           // Munus
				[]string{"j", "enter"},      // Cryptare
				[]string{"j", "enter", "q"}, // Salus: hint only, then quit
			)
			if err := runMenu(context.Background(), tools, tt.vim, show); err != nil {
				t.Fatalf("runMenu: %v", err)
			}
			if *shown != 3 {
				t.Errorf("menu shown %d times, want 3", *shown)
			}
			want := []call{
				{tool: "Munus", version: "v1.2.3-Munus", args: tt.want[0]},
				{tool: "Cryptare", version: "v1.2.3-Cryptare", args: tt.want[1]},
			}
			if !reflect.DeepEqual(r.calls, want) {
				t.Errorf("calls = %#v, want %#v", r.calls, want)
			}
		})
	}
}

func TestRunMenuPassesVimOnlyToToolsThatAcceptIt(t *testing.T) {
	var r recorder
	var running bool
	tools := guarded(t, []Tool{r.tool("Plain", true, false, 0)}, &running)
	show, _ := script(t, &running, []string{"enter"}, []string{"q"})
	if err := runMenu(context.Background(), tools, true, show); err != nil {
		t.Fatalf("runMenu: %v", err)
	}
	if len(r.calls) != 1 || r.calls[0].args == nil || len(r.calls[0].args) != 0 {
		t.Errorf("calls = %#v, want one call with no arguments", r.calls)
	}
}

func TestRunMenuReportsAToolsExitStatus(t *testing.T) {
	var r recorder
	tools := []Tool{r.tool("Munus", true, true, 0), r.tool("Cryptare", true, true, 130)}
	var states []menuModel
	sequences := [][]string{{"j", "enter"}, {"q"}}
	n := 0
	show := func(m menuModel) (menuModel, error) {
		states = append(states, m)
		m, _ = press(t, m, sequences[n]...)
		n++
		return m, nil
	}
	if err := runMenu(context.Background(), tools, false, show); err != nil {
		t.Fatalf("runMenu: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("menu shown %d times, want 2", len(states))
	}
	again := states[1]
	if again.cursor != 1 || again.chosen != -1 || again.done {
		t.Errorf("menu after the tool: cursor %d, chosen %d, done %v; want 1, -1, false", again.cursor, again.chosen, again.done)
	}
	if want := "Cryptare exited with status 130."; again.message != want {
		t.Errorf("message = %q, want %q", again.message, want)
	}
}

func TestRunMenuClearsTheStatusAfterASuccessfulRun(t *testing.T) {
	var r recorder
	codes := []int{130, 0}
	tool := r.tool("Munus", true, true, 0)
	tool.Main = func(context.Context, string, []string) int {
		code := codes[0]
		codes = codes[1:]
		return code
	}
	var states []menuModel
	show := func(m menuModel) (menuModel, error) {
		states = append(states, m)
		key := "enter"
		if len(states) == 3 {
			key = "q"
		}
		m, _ = press(t, m, key)
		return m, nil
	}
	if err := runMenu(context.Background(), []Tool{tool}, false, show); err != nil {
		t.Fatalf("runMenu: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("menu shown %d times, want 3", len(states))
	}
	if states[1].message != "Munus exited with status 130." || states[2].message != "" {
		t.Errorf("messages after exit 130 and then 0: %q, %q; want the status, then none", states[1].message, states[2].message)
	}
}

func TestRunMenuPassesItsContextToTools(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "from main")
	var got any
	tool := Tool{Name: "Munus", HasTUI: true, Main: func(ctx context.Context, _ string, _ []string) int {
		got = ctx.Value(key{})
		return 0
	}}
	n := 0
	show := func(m menuModel) (menuModel, error) {
		n++
		if n == 1 {
			m, _ = press(t, m, "enter")
		} else {
			m, _ = press(t, m, "q")
		}
		return m, nil
	}
	if err := runMenu(ctx, []Tool{tool}, false, show); err != nil {
		t.Fatalf("runMenu: %v", err)
	}
	if got != "from main" {
		t.Errorf("the tool got a context without runMenu's value (got %v)", got)
	}
}

func TestRunMenuReturnsTheMenuError(t *testing.T) {
	var r recorder
	errMenu := errors.New("no terminal")
	show := func(m menuModel) (menuModel, error) { return m, errMenu }
	if err := runMenu(context.Background(), threeTools(&r), false, show); !errors.Is(err, errMenu) {
		t.Errorf("runMenu error = %v, want %v", err, errMenu)
	}
	if len(r.calls) != 0 {
		t.Errorf("a tool ran after the menu failed: %#v", r.calls)
	}
}

// TestRunMenuWithBubbleTea runs the menu loop with real Bubble Tea programs that read keys
// from strings, menu -> tool -> menu in one process.
func TestRunMenuWithBubbleTea(t *testing.T) {
	var r recorder
	var running bool
	tools := guarded(t, threeTools(&r), &running)
	// The deadline only ends a menu that never quits, so a broken quit key fails the test
	// instead of hanging it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Enter on Munus; then ↓ ↓, Enter on Salus (hint only) and q. Arrow keys rather than "jj":
	// Bubble Tea delivers runes read together as one key message.
	inputs := []string{"\r", "\x1b[B\x1b[B\rq"}
	n := 0
	show := func(m menuModel) (menuModel, error) {
		if n >= len(inputs) {
			t.Fatalf("menu shown %d times, want %d", n+1, len(inputs))
		}
		in := inputs[n]
		n++
		running = true
		defer func() { running = false }()
		return program(ctx,
			tea.WithInput(strings.NewReader(in)),
			tea.WithOutput(io.Discard),
			tea.WithoutSignalHandler(),
		)(m)
	}
	if err := runMenu(ctx, tools, false, show); err != nil {
		t.Fatalf("runMenu: %v", err)
	}
	want := []call{{tool: "Munus", version: "v1.2.3-Munus", args: []string{}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %#v, want %#v", r.calls, want)
	}
	if n != 2 {
		t.Errorf("menu shown %d times, want 2", n)
	}
}

func TestProgramReportsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var r recorder
	done := make(chan error, 1)
	go func() {
		_, err := program(ctx,
			tea.WithInput(strings.NewReader("")),
			tea.WithOutput(io.Discard),
			tea.WithoutSignalHandler(),
		)(newMenuModel(threeTools(&r)))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, tea.ErrProgramKilled) || !strings.HasPrefix(err.Error(), "menu: ") {
			t.Errorf("program error = %v, want %q wrapping %v", err, "menu: ", tea.ErrProgramKilled)
		}
	case <-time.After(30 * time.Second):
		// Fires only on failure: the program ignored its cancelled context.
		t.Fatal("the menu program did not stop for its cancelled context")
	}
}
