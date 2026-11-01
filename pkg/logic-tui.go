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
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//--------------------------------------------------styles---------------------------------------------------------------------------------------//

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f88e02")).
			Bold(true).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9333EA")).
			Bold(true).
			PaddingBottom(1)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f88e02")).
			PaddingBottom(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f10c0c")).
			MarginTop(1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f88e02"))
)

//--------------------------------------------------field labels---------------------------------------------------------------------------------//

const (
	labelFilePath = "File or directory path"
	labelOutput   = "Output path (optional)"
	labelFormat   = "Compression format gzip|zip (optional)"
	labelPassword = "Password"
	labelLevel    = "Compression level 1-9 (optional)"
	labelKeyID    = "Key ID"
	labelConfirm  = "Type DELETE to confirm"

	labelConfirmPassword = "Confirm password"

	// Stored keys (plans 3.3 and 3.4, BUG-011). The encrypt form's key field comes last,
	// so the fields before it keep their order.
	labelPasswordOrKey    = "Password (or the stored key's master password)"
	labelConfirmOrKey     = "Confirm password (not needed with a stored key)"
	labelStoredKeyID      = "Stored key ID (optional, instead of a password)"
	labelDecryptPassword  = "Password (or, for a file encrypted with a stored key, its master password)"
	labelKeyPassword      = "The key's master password"
	labelOlderExportField = "The export's own password, if different (exports from v1.3.1 or earlier)"
)

//--------------------------------------------------form field sets------------------------------------------------------------------------------//

func fieldsFor(action actionKind) []formField {
	switch action {
	case actionEncrypt:
		return []formField{
			{label: labelFilePath},
			{label: labelOutput},
			{label: labelPasswordOrKey, password: true},
			{label: labelConfirmOrKey, password: true},
			{label: labelStoredKeyID},
		}
	case actionDecrypt:
		return []formField{
			{label: labelFilePath},
			{label: labelOutput},
			{label: labelDecryptPassword, password: true},
		}
	case actionCompress:
		return []formField{
			{label: labelFilePath},
			{label: labelOutput},
			{label: labelFormat},
			{label: labelLevel},
		}
	case actionDecompress:
		return []formField{
			{label: labelFilePath},
			{label: labelOutput},
		}
	case actionKeysGenerate:
		return []formField{
			{label: labelPassword, password: true},
			{label: labelConfirmPassword, password: true},
		}
	case actionKeysExport:
		return []formField{
			{label: labelKeyID},
			{label: labelOutput},
			{label: labelKeyPassword, password: true},
		}
	case actionKeysImport:
		return []formField{
			{label: labelFilePath},
			{label: labelKeyPassword, password: true},
			{label: labelOlderExportField, password: true},
		}
	case actionKeysDelete:
		return []formField{
			{label: labelKeyID},
			{label: labelConfirm},
		}
	default:
		return nil
	}
}

func actionTitle(action actionKind) string {
	switch action {
	case actionEncrypt:
		return "Encrypt a file or directory"
	case actionDecrypt:
		return "Decrypt a file or directory archive"
	case actionCompress:
		return "Compress a file or directory"
	case actionDecompress:
		return "Decompress a file or archive"
	case actionKeysGenerate:
		return "Generate a new key"
	case actionKeysExport:
		return "Export a key"
	case actionKeysImport:
		return "Import a key"
	case actionKeysDelete:
		return "Delete a key (files encrypted with it can't be decrypted without it or an export of it)"
	default:
		return ""
	}
}

// fieldValue returns the current text of the field with the given label, or
// "" if no such field exists on the form.
func (m DashboardModel) fieldValue(label string) string {
	for _, f := range m.fields {
		if f.label == label {
			return string(f.value)
		}
	}
	return ""
}

func (m DashboardModel) menuKey(msg tea.KeyMsg) string {
	key := msg.String()
	if !m.vimEnabled {
		return key
	}

	switch key {
	case "j":
		return "down"
	case "k":
		return "up"
	case "l":
		return "enter"
	case "h", "b":
		return "esc"
	default:
		return key
	}
}

func (m DashboardModel) handleVimFormKey(msg tea.KeyMsg) (DashboardModel, tea.Cmd, bool) {
	if !m.vimEnabled {
		return m, nil, false
	}

	if m.formMode == formModeInsert {
		if msg.Type == tea.KeyEsc {
			m.formMode = formModeNormal
			return m, nil, true
		}
		return m, nil, false
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		next, cmd := m.quit()
		return next, cmd, true
	case tea.KeyEsc:
		m.screen = m.formOrigin
		m.status = ""
		return m, nil, true
	case tea.KeyTab, tea.KeyDown:
		if m.fieldIdx < len(m.fields)-1 {
			m.fieldIdx++
		}
		return m, nil, true
	case tea.KeyShiftTab, tea.KeyUp:
		if m.fieldIdx > 0 {
			m.fieldIdx--
		}
		return m, nil, true
	case tea.KeyEnter:
		next, cmd := m.advanceOrSubmitForm()
		return next.(DashboardModel), cmd, true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "i", "a":
			m.formMode = formModeInsert
			return m, nil, true
		case "o":
			if m.fieldIdx < len(m.fields)-1 {
				m.fieldIdx++
			}
			m.formMode = formModeInsert
			return m, nil, true
		case "j":
			if m.fieldIdx < len(m.fields)-1 {
				m.fieldIdx++
			}
			return m, nil, true
		case "k":
			if m.fieldIdx > 0 {
				m.fieldIdx--
			}
			return m, nil, true
		case "h":
			m.screen = m.formOrigin
			m.status = ""
			return m, nil, true
		case "l":
			next, cmd := m.advanceOrSubmitForm()
			return next.(DashboardModel), cmd, true
		}
		return m, nil, false
	case tea.KeyBackspace, tea.KeySpace:
		return m, nil, true
	default:
		return m, nil, false
	}
}

//--------------------------------------------------bubbletea update-----------------------------------------------------------------------------//

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case keysLoadedMsg:
		m.keys = msg.keys
		return m, nil

	case errMsg:
		m.status = msg.err.Error()
		m.isError = true
		return m, nil

	case actionResultMsg:
		m.busy = false
		if m.quitting {
			return m, tea.Quit
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			m.isError = true
			return m, nil
		}
		m.status = msg.message
		m.isError = false
		if msg.reload {
			return m, loadKeysCmd(m.db)
		}
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenForm {
			return m.updateForm(msg)
		}

		switch m.menuKey(msg) {
		case "q", "ctrl+c":
			next, cmd := m.quit()
			return next, cmd

		case "up", "shift+tab":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "tab":
			if m.screen == screenMain && m.cursor < len(mainMenuItems)-1 {
				m.cursor++
			} else if m.screen == screenKeys && m.cursor < len(keysMenuItems)-1 {
				m.cursor++
			}

		case "enter":
			switch m.screen {
			case screenMain:
				switch m.cursor {
				case 0:
					m.startForm(actionEncrypt, screenMain)
				case 1:
					m.startForm(actionDecrypt, screenMain)
				case 2:
					m.startForm(actionCompress, screenMain)
				case 3:
					m.startForm(actionDecompress, screenMain)
				case 4: // Manage keys
					m.screen = screenKeys
					m.cursor = 0
				}
			case screenKeys:
				switch m.cursor {
				case 0:
					m.startForm(actionKeysGenerate, screenKeys)
				case 1:
					m.startForm(actionKeysExport, screenKeys)
				case 2:
					m.startForm(actionKeysImport, screenKeys)
				case 3:
					m.startForm(actionKeysDelete, screenKeys)
				}
			}

		case "esc", "b":
			if m.screen != screenMain {
				m.screen = screenMain
				m.cursor = 0
			}
		}
	}

	return m, nil
}

// quit ends the program, or, while an action runs, cancels the action and ends the
// program once it has reported back, so its clean-up isn't cut short (SEC-015).
func (m DashboardModel) quit() (DashboardModel, tea.Cmd) {
	if !m.busy {
		return m, tea.Quit
	}
	m.runner.cancelRunning()
	m.quitting = true
	m.status = "Cancelling…"
	m.isError = false
	return m, nil
}

// startForm switches the model into the form screen for the given action.
func (m *DashboardModel) startForm(action actionKind, origin dashboardScreen) {
	m.action = action
	m.fields = fieldsFor(action)
	m.fieldIdx = 0
	m.formOrigin = origin
	m.formMode = formModeInsert
	m.screen = screenForm
	m.status = ""
}

func (m DashboardModel) advanceOrSubmitForm() (tea.Model, tea.Cmd) {
	if m.fieldIdx < len(m.fields)-1 {
		m.fieldIdx++
		return m, nil
	}
	// Only one action runs at a time (BUG-008); the form stays open so it can be
	// submitted once the running action reports back.
	if m.busy {
		m.status = "Another action is still running; wait for it to finish, then press Enter again."
		m.isError = true
		return m, nil
	}

	cmd := m.buildActionCmd()
	m.busy = true
	m.screen = m.formOrigin
	m.status = "Working…"
	m.isError = false
	return m, cmd
}

// updateForm handles key input while the form screen is active.
func (m DashboardModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleVimFormKey(msg); handled {
		return next, cmd
	}
	if m.vimEnabled && m.formMode == formModeNormal {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		next, cmd := m.quit()
		return next, cmd

	case tea.KeyEsc:
		m.screen = m.formOrigin
		m.status = ""
		return m, nil

	case tea.KeyTab, tea.KeyDown:
		if m.fieldIdx < len(m.fields)-1 {
			m.fieldIdx++
		}
		return m, nil

	case tea.KeyShiftTab, tea.KeyUp:
		if m.fieldIdx > 0 {
			m.fieldIdx--
		}
		return m, nil

	case tea.KeyBackspace:
		if len(m.fields) > 0 && len(m.fields[m.fieldIdx].value) > 0 {
			f := &m.fields[m.fieldIdx]
			f.value = f.value[:len(f.value)-1]
		}
		return m, nil

	case tea.KeyEnter:
		return m.advanceOrSubmitForm()

	case tea.KeySpace:
		if len(m.fields) > 0 {
			f := &m.fields[m.fieldIdx]
			f.value = append(f.value, ' ')
		}
		return m, nil

	case tea.KeyRunes:
		if m.vimEnabled && m.formMode == formModeNormal {
			return m, nil
		}
		if len(m.fields) > 0 {
			f := &m.fields[m.fieldIdx]
			f.value = append(f.value, msg.Runes...)
		}
		return m, nil
	default:
		// Keys the form doesn't use (arrows, Delete, Home/End, function keys, …) are ignored.
		return m, nil
	}
}

//--------------------------------------------------bubbletea view-------------------------------------------------------------------------------//

func (m DashboardModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("🔒 Cryptare"))
	sb.WriteString("\n\n")

	switch m.screen {
	case screenMain:
		for i, item := range mainMenuItems {
			if i == m.cursor {
				sb.WriteString(selectedStyle.Render("▶ " + item))
			} else {
				sb.WriteString(itemStyle.Render("  " + item))
			}
			sb.WriteString("\n")
		}

	case screenKeys:
		sb.WriteString(statusStyle.Render("  Manage Keys  (press Esc to go back)\n\n"))
		for i, item := range keysMenuItems {
			if i == m.cursor {
				sb.WriteString(selectedStyle.Render("▶ " + item))
			} else {
				sb.WriteString(itemStyle.Render("  " + item))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		if len(m.keys) == 0 {
			sb.WriteString(statusStyle.Render("No keys stored. Use \"Generate a new key\" to create one."))
			sb.WriteString("\n")
		} else {
			sb.WriteString(itemStyle.Render(fmt.Sprintf("%-20s  %-12s  %s", "KEY ID", "ALGORITHM", "CREATED")))
			sb.WriteString("\n")
			for _, k := range m.keys {
				created := time.Unix(k.CreatedAt_, 0).Format("2006-01-02 15:04")
				line := fmt.Sprintf("%-20s  %-12s  %s", displayText(k.KeyID), displayText(k.Algorithm), created)
				sb.WriteString(itemStyle.Render("  " + line))
				sb.WriteString("\n")
			}
		}

	case screenForm:
		sb.WriteString(statusStyle.Render(actionTitle(m.action) + "\n\n"))
		if m.vimEnabled {
			mode := "-- INSERT --"
			if m.formMode == formModeNormal {
				mode = "-- NORMAL --"
			}
			sb.WriteString(statusStyle.Render(mode))
			sb.WriteString("\n\n")
		}
		for i, f := range m.fields {
			display := string(f.value)
			if f.password {
				display = strings.Repeat("*", len(f.value))
			}
			cursor := "  "
			style := itemStyle
			if i == m.fieldIdx {
				cursor = "▶ "
				style = selectedStyle
				display += "█"
			}
			sb.WriteString(style.Render(fmt.Sprintf("%s%s: %s", cursor, f.label, display)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	if m.status != "" {
		if m.isError {
			sb.WriteString(errorStyle.Render("✗ " + m.status))
			sb.WriteString("\n")
		} else {
			sb.WriteString(statusStyle.Render("• " + m.status))
			sb.WriteString("\n")
		}
	}

	if m.screen == screenForm {
		if m.vimEnabled {
			if m.formMode == formModeInsert {
				sb.WriteString(statusStyle.Render("Vim insert • Esc: normal • cancel from normal with Esc/h • Tab/Enter: next field • Shift+Tab: prev • ctrl+c: quit"))
			} else {
				sb.WriteString(statusStyle.Render("Vim normal • j/k: fields • h: cancel • l: next • i/a/o: insert • Enter: next/submit"))
			}
		} else {
			sb.WriteString(statusStyle.Render("Tab/Enter: next field • Shift+Tab: prev • Esc: cancel • ctrl+c: quit"))
		}
	} else {
		if m.vimEnabled {
			sb.WriteString(statusStyle.Render("↑/shift+tab or k | ↓/tab or j: navigate • Enter/l: select • Esc/h/b: back when available • q: quit"))
		} else {
			sb.WriteString(statusStyle.Render("↑/shift+tab | ↓/tab: navigate • Enter: select • q: quit"))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

//--------------------------------------------------action commands------------------------------------------------------------------------------//

// buildActionCmd returns a tea.Cmd that performs the currently selected
// action using the values entered into the form fields, mirroring the
// behavior of the equivalent commands in logic-cli.go. It runs through the model's
// actionRunner, so quitting can cancel it (SEC-015).
func (m DashboardModel) buildActionCmd() tea.Cmd {
	return m.runner.cmd(m.actionFunc())
}

// actionFunc returns the selected action as a function of a context that is
// cancelled when the user quits while it runs.
func (m DashboardModel) actionFunc() func(ctx context.Context) tea.Msg {
	db := m.db
	action := m.action

	file := m.fieldValue(labelFilePath)
	output := m.fieldValue(labelOutput)
	password := m.fieldValue(labelPassword)
	keyID := m.fieldValue(labelKeyID)
	levelStr := m.fieldValue(labelLevel)
	format := m.fieldValue(labelFormat)
	confirm := m.fieldValue(labelConfirm)
	passwordAgain := m.fieldValue(labelConfirmPassword)
	passwordOrKey := m.fieldValue(labelPasswordOrKey)
	passwordOrKeyAgain := m.fieldValue(labelConfirmOrKey)
	storedKeyID := strings.TrimSpace(m.fieldValue(labelStoredKeyID))
	decryptPassword := m.fieldValue(labelDecryptPassword)
	keyPassword := m.fieldValue(labelKeyPassword)
	separateKeyPassword := m.fieldValue(labelOlderExportField)

	switch action {
	case actionEncrypt:
		return func(ctx context.Context) tea.Msg {
			dst := output
			if dst == "" {
				dst = defaultEncryptOutput(file)
			}
			if err := checkTUIOutput(file, dst); err != nil {
				return actionResultMsg{err: err}
			}
			if err := checkOutputOutsideFolder(file, dst); err != nil {
				return actionResultMsg{err: err}
			}
			var cred Credential
			if storedKeyID != "" {
				// The key's master password was set when the key was made; it is checked
				// against the key, so it isn't confirmed (plan 3.4).
				c, err := StoredKeyCredential(db, storedKeyID, passwordOrKey, true)
				if err != nil {
					return actionResultMsg{err: err}
				}
				cred = c
			} else {
				if err := checkTUINewPassword(passwordOrKey, passwordOrKeyAgain); err != nil {
					return actionResultMsg{err: err}
				}
				cred = PasswordCredential(passwordOrKey)
			}
			if err := EncryptFileWithCredentialContext(ctx, file, dst, cred); err != nil {
				return actionResultMsg{err: err}
			}
			return actionResultMsg{message: fmt.Sprintf("Encrypted: %s → %s", file, dst)}
		}

	case actionDecrypt:
		return func(ctx context.Context) tea.Msg {
			dst := output
			if dst == "" {
				dst = deriveDecryptOutput(file)
			}
			if err := checkTUIOutput(file, dst); err != nil {
				return actionResultMsg{err: err}
			}
			cred := PasswordCredential(decryptPassword)
			if keyID, ok := EncryptedWithStoredKey(file); ok {
				c, err := StoredKeyCredential(db, keyID, decryptPassword, false)
				if err != nil {
					return actionResultMsg{err: withMissingKeyHint(err, file, keyID)}
				}
				cred = c
			}
			if err := DecryptFileWithCredentialContext(ctx, file, dst, cred, DefaultExtractLimits()); err != nil {
				return actionResultMsg{err: withStoredKeyHint(withTUILimitHint(err))}
			}
			return actionResultMsg{message: fmt.Sprintf("Decrypted: %s → %s", file, dst)}
		}

	case actionCompress:
		return func(ctx context.Context) tea.Msg {
			level := -1
			if levelStr != "" {
				lv, err := strconv.Atoi(levelStr)
				if err != nil {
					return actionResultMsg{err: fmt.Errorf("invalid compression level %q: %w", levelStr, err)}
				}
				level = lv
			}
			if err := checkCompressLevel(level); err != nil {
				return actionResultMsg{err: err}
			}
			if _, err := resolveCompressFormat(format, output); err != nil {
				return actionResultMsg{err: err}
			}
			dst := output
			if dst == "" {
				dst = deriveCompressOutput(file, format)
			}
			if err := checkTUIOutput(file, dst); err != nil {
				return actionResultMsg{err: err}
			}
			if err := CompressFileWithFormatContext(ctx, file, dst, format, level); err != nil {
				return actionResultMsg{err: err}
			}
			return actionResultMsg{message: fmt.Sprintf("Compressed: %s → %s", file, dst)}
		}

	case actionDecompress:
		return func(ctx context.Context) tea.Msg {
			dst := output
			if dst == "" {
				dst = deriveDecompressOutput(file)
			}
			if err := checkTUIOutput(file, dst); err != nil {
				return actionResultMsg{err: err}
			}
			if err := DecompressFileWithLimitsContext(ctx, file, dst, DefaultExtractLimits()); err != nil {
				return actionResultMsg{err: withTUILimitHint(err)}
			}
			return actionResultMsg{message: fmt.Sprintf("Decompressed: %s → %s", file, dst)}
		}

	case actionKeysGenerate:
		return func(ctx context.Context) tea.Msg {
			if err := checkTUINewPassword(password, passwordAgain); err != nil {
				return actionResultMsg{err: err}
			}

			km, err := GenerateStoredKey(db, password)
			if err != nil {
				return actionResultMsg{err: err}
			}

			return actionResultMsg{message: fmt.Sprintf("Generated key: %s", km.KeyID), reload: true}
		}

	case actionKeysExport:
		return func(ctx context.Context) tea.Msg {
			km, err := db.GetKey(keyID)
			if err != nil {
				return actionResultMsg{err: keyLookupError(keyID, err)}
			}

			dst := output
			if dst == "" {
				dst = defaultExportPath(keyID)
			}
			if err := withTUIOutputHint(checkExportOutput(db, dst, false)); err != nil {
				return actionResultMsg{err: err}
			}
			// The export is protected by the key's master password, which
			// ExportKeyToFile checks against the key (BUG-011), so it isn't confirmed.
			if err := ExportKeyToFile(km, keyPassword, dst); err != nil {
				return actionResultMsg{err: err}
			}
			return actionResultMsg{message: fmt.Sprintf("Exported key %s → %s", keyID, dst)}
		}

	case actionKeysImport:
		return func(ctx context.Context) tea.Msg {
			// Either password may be in either field: ImportStoredKey tries both on the
			// export and on the key inside. An empty last field means it wasn't given.
			var others []string
			if separateKeyPassword != "" {
				others = append(others, separateKeyPassword)
			}
			km, err := ImportStoredKey(db, file, keyPassword, others...)
			if len(others) == 0 && (errors.Is(err, ErrSeparateKeyPassword) || errors.Is(err, ErrWrongExportPassword)) {
				return actionResultMsg{err: fmt.Errorf("%w; if v1.3.1 or earlier made this export with a password of its own, enter both passwords", err)}
			}
			if err != nil {
				return actionResultMsg{err: err}
			}

			return actionResultMsg{message: fmt.Sprintf("Imported key: %s", km.KeyID), reload: true}
		}

	case actionKeysDelete:
		return func(ctx context.Context) tea.Msg {
			if strings.TrimSpace(confirm) != "DELETE" {
				return actionResultMsg{err: errors.New(`confirmation required: type "DELETE" to delete the key`)}
			}

			if err := db.DeleteKey(keyID); err != nil {
				if errors.Is(err, ErrKeyNotFound) {
					return actionResultMsg{err: fmt.Errorf("key %q not found", keyID)}
				}
				return actionResultMsg{err: fmt.Errorf("delete key: %w", err)}
			}

			return actionResultMsg{message: fmt.Sprintf("Deleted key: %s", keyID), reload: true}
		}
	default:
		return func(ctx context.Context) tea.Msg {
			return actionResultMsg{err: fmt.Errorf("unsupported action %d", action)}
		}
	}
}

// actionRunner runs the TUI's form actions so that a running one can be cancelled, and
// waited for before the program exits (SEC-015). Every copy of a DashboardModel shares
// one. Actions run one at a time (BUG-008).
type actionRunner struct {
	mu       sync.Mutex
	cancel   context.CancelFunc // cancels the running action; nil when none runs
	finished chan struct{}      // closed when the running action has returned
	closed   bool               // set by shutdown: actions that start later don't run
}

// cmd returns a tea.Cmd that runs action with a context the runner can cancel.
func (r *actionRunner) cmd(action func(ctx context.Context) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		finished := make(chan struct{})
		defer close(finished)

		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return actionResultMsg{err: context.Canceled}
		}
		r.cancel, r.finished = cancel, finished
		r.mu.Unlock()
		defer func() {
			r.mu.Lock()
			r.cancel, r.finished = nil, nil
			r.mu.Unlock()
		}()

		return action(ctx)
	}
}

// cancelRunning cancels the running action, if there is one. It reports back as usual.
func (r *actionRunner) cancelRunning() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// shutdown stops actions that haven't started from running, and cancels a running one
// and waits until it has returned, so its clean-up has run.
func (r *actionRunner) shutdown() {
	r.mu.Lock()
	r.closed = true
	cancel, finished := r.cancel, r.finished
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if finished != nil {
		<-finished
	}
}

// checkTUINewPassword checks a password that will protect new data against the
// password policy, then checks that the confirmation field matches it.
func checkTUINewPassword(password, confirmation string) error {
	if err := CheckPasswordPolicy(password); err != nil {
		return err
	}
	if confirmation != password {
		return ErrPasswordMismatch
	}
	return nil
}

// withTUILimitHint explains an extraction-limit error: the TUI always uses the
// default limits, and the CLI's flags change them.
func withTUILimitHint(err error) error {
	if errors.Is(err, ErrExtractLimit) {
		return fmt.Errorf("%w; the TUI uses the default limits (use the CLI's --max-size or --max-entries to change them)", err)
	}
	return err
}

// checkTUIOutput applies CheckOutputPath for form actions. The TUI has no overwrite
// option, so an existing output is refused with a hint to choose another path.
func checkTUIOutput(src, dst string) error {
	return withTUIOutputHint(CheckOutputPath(src, dst, false))
}

// withTUIOutputHint adds the TUI's hint to an existing-output error.
func withTUIOutputHint(err error) error {
	if errors.Is(err, ErrOutputExists) {
		return fmt.Errorf("%w; choose a different output path", err)
	}
	return err
}
