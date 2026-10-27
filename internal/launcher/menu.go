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
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RunMenu shows the menu of tools until the user quits it. Enter on a tool with a TUI ends the
// menu program, runs the tool's Main with no arguments (--vim when vim is set and the tool
// accepts it) so that the tool's TUI has the terminal to itself, and shows the menu again when
// the tool returns. Enter on a CLI-only tool keeps the menu open with a hint.
func RunMenu(ctx context.Context, tools []Tool, vim bool) error {
	return runMenu(ctx, tools, vim, program(ctx))
}

// runMenu is RunMenu with the menu program passed in as show, which runs the menu until it
// quits and returns its final state. A tool runs only after show has returned, so two Bubble
// Tea programs never run at once and no tool runs inside the menu's Update.
func runMenu(ctx context.Context, tools []Tool, vim bool, show func(menuModel) (menuModel, error)) error {
	m := newMenuModel(tools)
	for {
		var err error
		if m, err = show(m); err != nil {
			return err
		}
		if m.chosen < 0 {
			return nil
		}
		t := m.tools[m.chosen]
		args := []string{}
		if vim && t.Vim {
			args = append(args, "--vim")
		}
		m = m.back(t, t.Run(ctx, args))
	}
}

// program returns a show function for runMenu that runs the menu as a Bubble Tea program with
// opts. The program renders inline rather than on the alternate screen, so what a tool prints
// when it exits stays visible above the menu.
func program(ctx context.Context, opts ...tea.ProgramOption) func(menuModel) (menuModel, error) {
	return func(m menuModel) (menuModel, error) {
		all := append([]tea.ProgramOption{tea.WithContext(ctx)}, opts...)
		final, err := tea.NewProgram(m, all...).Run()
		if err != nil {
			return m, fmt.Errorf("menu: %w", err)
		}
		return final.(menuModel), nil
	}
}

// menuModel is the Bubble Tea model of the menu.
type menuModel struct {
	tools   []Tool
	cursor  int
	chosen  int    // index of the TUI tool picked with Enter; -1 while none is
	done    bool   // the menu is quitting, and View clears it from the screen
	message string // shown under the rows
	width   int    // terminal width from the last tea.WindowSizeMsg; 0 while unknown
}

func newMenuModel(tools []Tool) menuModel {
	return menuModel{tools: tools, chosen: -1}
}

// back is the menu shown again after tool t returned code: the cursor stays on the tool, and a
// non-zero exit code is reported under the rows.
func (m menuModel) back(t Tool, code int) menuModel {
	m.chosen, m.done, m.message = -1, false, ""
	if code != 0 {
		m.message = fmt.Sprintf("%s exited with status %d.", t.Name, code)
	}
	return m
}

func (m menuModel) Init() tea.Cmd {
	return nil
}

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.message = ""
			}
		case "down", "j":
			if m.cursor < len(m.tools)-1 {
				m.cursor++
				m.message = ""
			}
		case "enter":
			if len(m.tools) == 0 {
				break
			}
			t := m.tools[m.cursor]
			if !t.HasTUI {
				m.message = fmt.Sprintf("%s has no TUI. Run `capsus %s --help` for its commands.", t.Name, t.Command)
				break
			}
			m.chosen, m.done = m.cursor, true
			return m, tea.Quit
		case "q", "ctrl+c":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m menuModel) View() string {
	if m.done {
		return ""
	}
	bold := lipgloss.NewStyle().Bold(true)
	nameWidth, descWidth, kindWidth := 0, 0, 0
	for _, t := range m.tools {
		nameWidth = max(nameWidth, lipgloss.Width(t.Name))
		descWidth = max(descWidth, lipgloss.Width(t.Description))
		kindWidth = max(kindWidth, lipgloss.Width(t.kind()))
	}
	// A row is the cursor column, the name, the description and the kind, two spaces apart;
	// descriptions are cut to fit the terminal's width once it is known.
	if m.width > 0 {
		descWidth = max(0, min(descWidth, m.width-2-nameWidth-2-2-kindWidth))
	}

	var b strings.Builder
	b.WriteString(bold.Render("Capsus: choose a tool") + "\n\n")
	for i, t := range m.tools {
		row := pad(t.Name, nameWidth) + "  " + truncate(t.Description, descWidth) + "  " + t.kind()
		if i == m.cursor {
			b.WriteString("> " + bold.Render(row) + "\n")
		} else {
			b.WriteString("  " + row + "\n")
		}
	}
	if m.message != "" {
		b.WriteString("\n" + m.message + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render("↑/↓ or j/k: move · enter: open · q: quit") + "\n")
	return b.String()
}

// pad adds spaces to s up to width terminal cells.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

// truncate shortens s to at most width terminal cells, ending it with "…" when it cuts, and
// pads it to width so that the kind column stays aligned.
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return pad(s, width)
	}
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return pad(string(r)+"…", width)
}
