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
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// typeString simulates typing each rune of s into the currently focused field.
func typeString(m DashboardModel, s string) DashboardModel {
	for _, r := range s {
		next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(DashboardModel)
	}
	return m
}

func newVimDashboardModel(db *Database) DashboardModel {
	return NewDashboardModelWithOptions(db, dashboardOptions{vimEnabled: true})
}

// TestDashboardNavigation verifies that the cursor moves between the main
// menu items and that "enter" on "Manage keys" switches to the key screen.
func TestDashboardNavigation(t *testing.T) {
	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(DashboardModel)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}

	for m.cursor < 4 {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(DashboardModel)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	if m.screen != screenKeys {
		t.Fatalf("screen = %v, want screenKeys", m.screen)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(DashboardModel)
	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain after Esc", m.screen)
	}
}

func TestDashboardVimMenuNavigation(t *testing.T) {
	db := newTestDatabase(t, false)

	m := newVimDashboardModel(db)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(DashboardModel)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after j", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(DashboardModel)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after k", m.cursor)
	}

	for m.cursor < 4 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = next.(DashboardModel)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(DashboardModel)
	if m.screen != screenKeys {
		t.Fatalf("screen = %v, want screenKeys after l", m.screen)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = next.(DashboardModel)
	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain after h", m.screen)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(DashboardModel)
	for m.cursor < 4 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = next.(DashboardModel)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(DashboardModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(DashboardModel)
	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain after b", m.screen)
	}
}

func TestDashboardVimFormModeTransitions(t *testing.T) {
	db := newTestDatabase(t, false)

	m := newVimDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)

	if m.formMode != formModeInsert {
		t.Fatalf("formMode = %v, want insert", m.formMode)
	}

	m = typeString(m, "ab")
	if got := m.fieldValue(labelFilePath); got != "ab" {
		t.Fatalf("fieldValue = %q, want %q", got, "ab")
	}

	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(DashboardModel)
	if m.screen != screenForm {
		t.Fatalf("screen = %v, want screenForm after leaving insert mode", m.screen)
	}
	if m.formMode != formModeNormal {
		t.Fatalf("formMode = %v, want normal after Esc", m.formMode)
	}

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(DashboardModel)
	if m.fieldIdx != 1 {
		t.Fatalf("fieldIdx = %d, want 1 after j", m.fieldIdx)
	}
	if got := m.fieldValue(labelFilePath); got != "ab" {
		t.Fatalf("fieldValue changed in normal mode: got %q, want %q", got, "ab")
	}

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = next.(DashboardModel)
	if m.fieldIdx != 2 {
		t.Fatalf("fieldIdx = %d, want 2 after o", m.fieldIdx)
	}
	if m.formMode != formModeInsert {
		t.Fatalf("formMode = %v, want insert after o", m.formMode)
	}

	m = typeString(m, "pw")
	if got := m.fieldValue(labelPasswordOrKey); got != "pw" {
		t.Fatalf("password field = %q, want %q", got, "pw")
	}
}

func TestDashboardVimNormalModeDoesNotEditFields(t *testing.T) {
	db := newTestDatabase(t, false)

	m := newVimDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	m = typeString(m, "path")

	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(DashboardModel)

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(DashboardModel)
	if got := m.fieldValue(labelFilePath); got != "path" {
		t.Fatalf("fieldValue = %q, want %q after x in normal mode", got, "path")
	}

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(DashboardModel)
	if got := m.fieldValue(labelFilePath); got != "path" {
		t.Fatalf("fieldValue = %q, want %q after space in normal mode", got, "path")
	}

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(DashboardModel)
	if got := m.fieldValue(labelFilePath); got != "path" {
		t.Fatalf("fieldValue = %q, want %q after backspace in normal mode", got, "path")
	}

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = next.(DashboardModel)
	m = typeString(m, "!")
	if got := m.fieldValue(labelFilePath); got != "path!" {
		t.Fatalf("fieldValue = %q, want %q after returning to insert mode", got, "path!")
	}
}

func TestDashboardVimNormalModeLSubmitsLastField(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "submit.txt")
	if err := os.WriteFile(srcFile, []byte("vim submit"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	db := newTestDatabase(t, false)

	m := newVimDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	m = typeString(m, srcFile)

	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	m = typeString(m, testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	m = typeString(m, testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // to the optional stored key field
	m = next.(DashboardModel)

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(DashboardModel)
	if m.formMode != formModeNormal {
		t.Fatalf("formMode = %v, want normal", m.formMode)
	}

	next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(DashboardModel)
	if cmd == nil {
		t.Fatal("expected a command to be returned for vim l submission")
	}
	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain after submission", m.screen)
	}
	if !m.busy {
		t.Fatal("expected busy=true after submission")
	}

	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("encrypt action failed: %v", result.err)
	}

	if _, err := os.Stat(srcFile + encExt); err != nil {
		t.Fatalf("encrypted file not created: %v", err)
	}
}

func TestDashboardStandardFormBindingsUnaffectedWhenVimDisabled(t *testing.T) {
	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)

	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(DashboardModel)
	if got := m.fieldValue(labelFilePath); got != "j" {
		t.Fatalf("fieldValue = %q, want %q with vim disabled", got, "j")
	}

	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(DashboardModel)
	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain after Esc with vim disabled", m.screen)
	}
}

// TestDashboardEncryptDecryptRoundTrip drives the encrypting, then decrypt forms
// exactly as a user typing into the TUI would, and verifies the resulting
// files match the CLI's behavior.
func TestDashboardEncryptDecryptRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("secret tui data")
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)

	m = typeString(m, srcFile)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to output
	m = next.(DashboardModel)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // leave output empty, move to password
	m = next.(DashboardModel)
	m = typeString(m, testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to password confirmation
	m = next.(DashboardModel)
	m = typeString(m, testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // leave the stored key empty
	m = next.(DashboardModel)

	next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // submit
	m = next.(DashboardModel)
	if cmd == nil {
		t.Fatal("expected a command to be returned for encrypt submission")
	}

	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("encrypt action failed: %v", result.err)
	}

	encFile := srcFile + encExt
	if _, err := os.Stat(encFile); err != nil {
		t.Fatalf("encrypted file not created: %v", err)
	}

	next, _ = m.Update(result)
	m = next.(DashboardModel)
	if m.isError || m.status == "" {
		t.Fatalf("expected success status after encrypt, got isError=%v status=%q", m.isError, m.status)
	}

	// Now decrypt using the form.
	m.startForm(actionDecrypt, screenMain)
	m = typeString(m, encFile)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to output
	m = next.(DashboardModel)
	decFile := filepath.Join(tmpDir, "decrypted.txt")
	m = typeString(m, decFile)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to password
	m = next.(DashboardModel)
	m = typeString(m, testPassword)

	next, cmd = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // submit
	m = next.(DashboardModel)
	if cmd == nil {
		t.Fatal("expected a command to be returned for decrypt submission")
	}

	msg = cmd()
	result, ok = msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("decrypt action failed: %v", result.err)
	}

	decData, err := os.ReadFile(decFile)
	if err != nil {
		t.Fatalf("read decrypted file: %v", err)
	}
	if string(decData) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", decData, content)
	}
}

// TestDashboardEncryptDecryptDirectoryRoundTrip verifies the TUI accepts a
// directory path for encrypt/decrypt actions and restores the directory tree.
func TestDashboardEncryptDecryptDirectoryRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "empty"), 0o755); err != nil {
		t.Fatalf("create empty source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "file.txt"), []byte("secret tui directory data"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	m = typeString(m, srcDir)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	m = typeString(m, testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	m = typeString(m, testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // leave the stored key empty
	m = next.(DashboardModel)

	next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	if cmd == nil {
		t.Fatal("expected a command to be returned for encrypt submission")
	}

	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("encrypt action failed: %v", result.err)
	}

	encFile := srcDir + encExt
	if _, err := os.Stat(encFile); err != nil {
		t.Fatalf("encrypted artifact not created: %v", err)
	}
	// Deliver the result, as Bubble Tea would, so the dashboard is no longer busy.
	next, _ = m.Update(result)
	m = next.(DashboardModel)

	m.startForm(actionDecrypt, screenMain)
	m = typeString(m, encFile)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	restoreDir := filepath.Join(tmpDir, "restored")
	m = typeString(m, restoreDir)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	m = typeString(m, testPassword)

	_, cmd = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command to be returned for decrypt submission")
	}

	msg = cmd()
	result, ok = msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("decrypt action failed: %v", result.err)
	}

	data, err := os.ReadFile(filepath.Join(restoreDir, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "secret tui directory data" {
		t.Fatalf("content mismatch: got %q", string(data))
	}
	if info, err := os.Stat(filepath.Join(restoreDir, "empty")); err != nil {
		t.Fatalf("expected empty directory not restored: %v", err)
	} else if !info.IsDir() {
		t.Fatal("restored empty path is not a directory")
	}
}

func TestDashboardCompressDecompressZipRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "zip-bundle")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "empty"), 0o755); err != nil {
		t.Fatalf("create empty source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "file.txt"), []byte("zip tui data"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionCompress, screenMain)
	m = typeString(m, srcDir)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // output
	m = next.(DashboardModel)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // format
	m = next.(DashboardModel)
	m = typeString(m, "zip")
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // level
	m = next.(DashboardModel)
	next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // submit
	m = next.(DashboardModel)
	if cmd == nil {
		t.Fatal("expected a command to be returned for compress submission")
	}

	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("compress action failed: %v", result.err)
	}

	archive := srcDir + zipExt
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("zip archive not created: %v", err)
	}
	if err := os.RemoveAll(srcDir); err != nil {
		t.Fatalf("remove source directory: %v", err)
	}
	// Deliver the result, as Bubble Tea would, so the dashboard is no longer busy.
	next, _ = m.Update(result)
	m = next.(DashboardModel)

	m.startForm(actionDecompress, screenMain)
	m = typeString(m, archive)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // output
	m = next.(DashboardModel)
	_, cmd = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // submit
	if cmd == nil {
		t.Fatal("expected a command to be returned for decompress submission")
	}

	msg = cmd()
	result, ok = msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("decompress action failed: %v", result.err)
	}

	data, err := os.ReadFile(filepath.Join(srcDir, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "zip tui data" {
		t.Fatalf("content mismatch: got %q", string(data))
	}
	if info, err := os.Stat(filepath.Join(srcDir, "empty")); err != nil {
		t.Fatalf("expected empty directory not restored: %v", err)
	} else if !info.IsDir() {
		t.Fatal("restored empty path is not a directory")
	}
}

// TestDashboardKeysGenerateAndExport drives the "Generate a new key" and
// "Export a key" forms and verifies the key is stored and exported.
func TestDashboardKeysGenerateAndExport(t *testing.T) {
	tmpDir := t.TempDir()
	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionKeysGenerate, screenKeys)
	m = typeString(m, testPassword)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to password confirmation
	m = next.(DashboardModel)
	m = typeString(m, testPassword)

	_, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command for keys generate submission")
	}
	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("generate key failed: %v", result.err)
	}
	if !result.reload {
		t.Fatal("expected reload=true after generating a key")
	}

	keys, err := db.ListKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected 1 stored key, got %d (err=%v)", len(keys), err)
	}

	// Export the generated key.
	m.startForm(actionKeysExport, screenKeys)
	m = typeString(m, keys[0].KeyID)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to output
	m = next.(DashboardModel)
	outFile := filepath.Join(tmpDir, "exported.ckey")
	m = typeString(m, outFile)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to the key's master password
	m = next.(DashboardModel)
	m = typeString(m, testPassword) // checked against the key, so not confirmed (BUG-011)

	_, cmd = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // submit
	if cmd == nil {
		t.Fatal("expected a command for keys export submission")
	}
	msg = cmd()
	result, ok = msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("export key failed: %v", result.err)
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("exported key file not created: %v", err)
	}
}

// TestDashboardFormBackspace verifies that backspace removes the last
// character typed into the focused field.
func TestDashboardFormBackspace(t *testing.T) {
	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	m = typeString(m, "abc")

	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(DashboardModel)

	if got := m.fieldValue(labelFilePath); got != "ab" {
		t.Fatalf("fieldValue = %q, want %q", got, "ab")
	}
}

func TestDashboardKeysDelete(t *testing.T) {
	db := newTestDatabase(t, false)

	km := &KeyModel{
		KeyID:         "tui-delete",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}
	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	m := NewDashboardModel(db)
	m.startForm(actionKeysDelete, screenKeys)
	m = typeString(m, km.KeyID)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to confirmation field
	m = next.(DashboardModel)
	m = typeString(m, "DELETE")

	_, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // submit
	if cmd == nil {
		t.Fatal("expected a command for keys delete submission")
	}
	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("delete key failed: %v", result.err)
	}
	if !result.reload {
		t.Fatal("expected reload=true after deleting a key")
	}

	if _, err := db.GetKey(km.KeyID); err == nil {
		t.Fatal("expected key to be deleted")
	}
}

func TestDashboardKeysDeleteRequiresConfirmationPhrase(t *testing.T) {
	db := newTestDatabase(t, false)

	km := &KeyModel{
		KeyID:         "tui-delete-confirmation",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}
	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	m := NewDashboardModel(db)
	m.startForm(actionKeysDelete, screenKeys)
	m = typeString(m, km.KeyID)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(DashboardModel)
	m = typeString(m, "no")

	_, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command for keys delete submission")
	}
	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}
	if result.err == nil {
		t.Fatal("expected confirmation error")
	}
	if !strings.Contains(result.err.Error(), "type \"DELETE\"") {
		t.Fatalf("unexpected error: %v", result.err)
	}
	if _, err := db.GetKey(km.KeyID); err != nil {
		t.Fatalf("expected key to remain after failed confirmation, got: %v", err)
	}
}

// TestDashboardFormIgnoresUnhandledKeys is a regression test for BUG-002: keys a
// form doesn't use (arrows, Delete, Home/End, …) must be ignored, not crash the TUI.
func TestDashboardFormIgnoresUnhandledKeys(t *testing.T) {
	keys := []tea.KeyType{
		tea.KeyLeft, tea.KeyRight, tea.KeyDelete, tea.KeyHome, tea.KeyEnd,
		tea.KeyPgUp, tea.KeyPgDown, tea.KeyCtrlU, tea.KeyF1,
	}

	for _, vim := range []bool{false, true} {
		for _, key := range keys {
			name := tea.Key{Type: key}.String()
			if vim {
				name = "vim/" + name
			}
			t.Run(name, func(t *testing.T) {
				db := newTestDatabase(t, false)
				m := NewDashboardModelWithOptions(db, dashboardOptions{vimEnabled: vim})
				m.startForm(actionEncrypt, screenMain)
				m = typeString(m, "abc")

				next, cmd := m.Update(tea.KeyMsg{Type: key})
				m = next.(DashboardModel)

				if cmd != nil {
					t.Fatalf("Update(%s) returned a command, want none", name)
				}
				if m.screen != screenForm {
					t.Fatalf("screen = %v, want screenForm", m.screen)
				}
				if got := m.fieldValue(labelFilePath); got != "abc" {
					t.Fatalf("fieldValue = %q, want %q", got, "abc")
				}
			})
		}
	}
}

// TestDashboardUnknownActionReportsError checks that submitting a form with no
// valid action reports an error instead of panicking.
func TestDashboardUnknownActionReportsError(t *testing.T) {
	db := newTestDatabase(t, false)
	m := NewDashboardModel(db)
	m.action = actionNone

	msg, ok := m.buildActionCmd()().(actionResultMsg)
	if !ok {
		t.Fatalf("buildActionCmd() message type = %T, want actionResultMsg", msg)
	}
	if msg.err == nil {
		t.Fatal("buildActionCmd() error = nil, want an unsupported-action error")
	}
}

// TestDashboardEnterOnUnknownScreenIsIgnored checks that Enter on a screen with no
// menu is a no-op instead of a panic.
func TestDashboardEnterOnUnknownScreenIsIgnored(t *testing.T) {
	db := newTestDatabase(t, false)
	m := NewDashboardModel(db)
	m.screen = dashboardScreen(99)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Update(enter) returned a command, want none")
	}
	if got := next.(DashboardModel).screen; got != dashboardScreen(99) {
		t.Fatalf("screen = %v, want unchanged", got)
	}
}

// TestDashboardRejectsEmptyPassword is a regression test for SEC-001: submitting a
// TUI form with the password left blank must fail with an error and change nothing.
func TestDashboardRejectsEmptyPassword(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(srcFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	db := newTestDatabase(t, false)

	// submit presses Enter through every field and runs the resulting command.
	// It then delivers the result, as Bubble Tea would, so the next form can be submitted.
	submit := func(m DashboardModel) (DashboardModel, actionResultMsg) {
		t.Helper()
		for range len(m.fields) + 1 {
			next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(DashboardModel)
			if cmd != nil {
				result, ok := cmd().(actionResultMsg)
				if !ok {
					t.Fatalf("expected actionResultMsg")
				}
				next, _ = m.Update(result)
				return next.(DashboardModel), result
			}
		}
		t.Fatal("form never submitted")
		return m, actionResultMsg{}
	}

	// Encrypt: file path typed, output and password left blank.
	m := NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	m = typeString(m, srcFile)
	m, result := submit(m)
	if !errors.Is(result.err, ErrEmptyPassword) {
		t.Fatalf("encrypt error = %v, want ErrEmptyPassword", result.err)
	}
	if _, err := os.Stat(srcFile + encExt); !os.IsNotExist(err) {
		t.Fatalf("encrypted file created despite the empty password (stat err: %v)", err)
	}
	next, _ := m.Update(result)
	if got := next.(DashboardModel); !got.isError || !strings.Contains(got.status, "password must not be empty") {
		t.Fatalf("status = %q (isError %v), want the empty-password error", got.status, got.isError)
	}

	// Generate a key with a blank master password.
	m.startForm(actionKeysGenerate, screenKeys)
	_, result = submit(m)
	if !errors.Is(result.err, ErrEmptyPassword) {
		t.Fatalf("keys generate error = %v, want ErrEmptyPassword", result.err)
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 0 {
		t.Fatalf("stored keys = %d (err %v), want 0", len(keys), err)
	}

	// Export a key whose master password, set by a version before SEC-001, is empty:
	// the export would be protected by that empty password, so it is refused (BUG-011).
	blobBytes, err := encryptBytes(make([]byte, keyLen), "")
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	blob := base64.StdEncoding.EncodeToString(blobBytes)
	if err := db.SaveKey(&KeyModel{KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", EncryptedBlob: blob, CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	exportPath := filepath.Join(tmpDir, "exported.ckey")
	m.startForm(actionKeysExport, screenKeys)
	m = typeString(m, "0123456789abcdef")
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // move to output
	m = typeString(next.(DashboardModel), exportPath)
	_, result = submit(m)
	if !errors.Is(result.err, ErrEmptyPassword) {
		t.Fatalf("keys export error = %v, want ErrEmptyPassword", result.err)
	}
	if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
		t.Fatalf("export file created despite the empty password (stat err: %v)", err)
	}
}

// TestDashboardRefusesExistingOutput checks that TUI file actions refuse to replace
// an existing output; the TUI has no overwrite option (BUG-004).
func TestDashboardRefusesExistingOutput(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(src, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	taken := filepath.Join(tmpDir, "taken.enc")
	if err := os.WriteFile(taken, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	m := NewDashboardModel(newTestDatabase(t, false))
	m.startForm(actionEncrypt, screenMain)
	m = typeString(m, src)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // output
	m = typeString(next.(DashboardModel), taken)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // password
	m = typeString(next.(DashboardModel), testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // password confirmation
	m = typeString(next.(DashboardModel), testPassword)
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // stored key, left empty
	m = next.(DashboardModel)
	_, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command for the encrypt submission")
	}

	result, ok := cmd().(actionResultMsg)
	if !ok {
		t.Fatal("expected actionResultMsg")
	}
	if !errors.Is(result.err, ErrOutputExists) {
		t.Fatalf("error = %v, want ErrOutputExists", result.err)
	}
	if got, _ := os.ReadFile(taken); string(got) != "existing" {
		t.Fatalf("existing output was modified to %q", got)
	}
}

// submitForm types values into the form's fields in order, pressing Enter after each
// (fields without a value are left blank), and returns the result of the action the
// form submits.
func submitForm(t *testing.T, m DashboardModel, values ...string) actionResultMsg {
	t.Helper()
	for i := 0; ; i++ {
		if i < len(values) {
			m = typeString(m, values[i])
		}
		next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(DashboardModel)
		if cmd != nil {
			result, ok := cmd().(actionResultMsg)
			if !ok {
				t.Fatal("expected actionResultMsg")
			}
			return result
		}
		if i > len(m.fields) {
			t.Fatal("form never submitted")
		}
	}
}

// TestDashboardNewPasswordFormsHaveConfirmation checks that the forms that set a new
// password ask for it twice, and the forms that use an existing password don't.
func TestDashboardNewPasswordFormsHaveConfirmation(t *testing.T) {
	hasConfirm := func(action actionKind) bool {
		for _, f := range fieldsFor(action) {
			if f.label == labelConfirmPassword || f.label == labelConfirmOrKey {
				if !f.password {
					t.Errorf("action %d: confirmation field is not masked", action)
				}
				return true
			}
		}
		return false
	}
	for _, action := range []actionKind{actionEncrypt, actionKeysGenerate} {
		if !hasConfirm(action) {
			t.Errorf("action %d: form has no password confirmation field", action)
		}
	}
	// keys export uses the key's existing master password, checked against the key
	// (BUG-011).
	for _, action := range []actionKind{actionDecrypt, actionKeysExport, actionKeysImport} {
		if hasConfirm(action) {
			t.Errorf("action %d: form asks to confirm an existing password", action)
		}
	}
}

// TestDashboardRejectsWeakOrMismatchedPassword is a regression test for SEC-001 and
// SEC-002 in the TUI: encrypt, keys generate and keys export refuse a password that
// fails the policy or doesn't match its confirmation, and change nothing.
func TestDashboardRejectsWeakOrMismatchedPassword(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(src, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	db := newTestDatabase(t, false)
	blob, err := EncryptKeyBlob(make([]byte, keyLen), testPassword)
	if err != nil {
		t.Fatalf("EncryptKeyBlob: %v", err)
	}
	const keyID = "0123456789abcdef"
	if err := db.SaveKey(&KeyModel{KeyID: keyID, Algorithm: "AES-256-GCM", EncryptedBlob: blob, CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	exportPath := filepath.Join(tmpDir, "key.ckey")

	const other = "correct horse battery stapler"
	cases := []struct {
		name   string
		action actionKind
		values []string
		want   error
	}{
		{"encrypt weak", actionEncrypt, []string{src, "", "hunter2", "hunter2"}, ErrWeakPassword},
		{"encrypt mismatch", actionEncrypt, []string{src, "", testPassword, other}, ErrPasswordMismatch},
		{"encrypt confirmation blank", actionEncrypt, []string{src, "", testPassword}, ErrPasswordMismatch},
		{"keys generate weak", actionKeysGenerate, []string{"hunter2", "hunter2"}, ErrWeakPassword},
		{"keys generate mismatch", actionKeysGenerate, []string{testPassword, other}, ErrPasswordMismatch},
		// The export reuses the key's master password, which must be the right one
		// (BUG-011); a weak legacy master password is covered by
		// TestDashboardExportRefusesWeakMasterPassword.
		{"keys export wrong password", actionKeysExport, []string{keyID, exportPath, other}, ErrWrongMasterPassword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewDashboardModel(db)
			m.startForm(tc.action, screenMain)
			result := submitForm(t, m, tc.values...)
			if !errors.Is(result.err, tc.want) {
				t.Fatalf("error = %v, want %v", result.err, tc.want)
			}
			next, _ := m.Update(result)
			if got := next.(DashboardModel); !got.isError {
				t.Fatalf("status = %q, want an error status", got.status)
			}
			if _, err := os.Stat(src + encExt); !os.IsNotExist(err) {
				t.Fatalf("encrypted file created (stat err: %v)", err)
			}
			if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
				t.Fatalf("export file created (stat err: %v)", err)
			}
			if keys, err := db.ListKeys(); err != nil || len(keys) != 1 {
				t.Fatalf("stored keys = %d (err %v), want only the fixture key", len(keys), err)
			}
		})
	}
}

// TestTUILimitHint checks that an extraction-limit error in the TUI explains that the
// TUI uses the default limits and names the CLI flags that change them.
func TestTUILimitHint(t *testing.T) {
	limitErr := fmt.Errorf("%w: the output is larger than 10 GiB", ErrExtractLimit)
	got := withTUILimitHint(limitErr)
	if !errors.Is(got, ErrExtractLimit) || !strings.Contains(got.Error(), "--max-size") {
		t.Fatalf("withTUILimitHint() = %v, want ErrExtractLimit with a hint about the CLI flags", got)
	}
	other := errors.New("other failure")
	if got := withTUILimitHint(other); got != other {
		t.Fatalf("withTUILimitHint(other) = %v, want it unchanged", got)
	}
}

// TestDashboardImportRejectsInvalidExport is a regression test for SEC-011 in the TUI:
// the Import form refuses an export with a crafted algorithm and stores nothing.
func TestDashboardImportRejectsInvalidExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evil.ckey")
	writeTestExport(t, path, KeyExport{Version: 1, KeyID: "0123456789abcdef", Algorithm: "\x1b[31mAES", CreatedAt: 1, EncryptedBlob: validStoredBlob(t)})
	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionKeysImport, screenKeys)
	result := submitForm(t, m, path, testPassword)
	if !errors.Is(result.err, ErrInvalidKeyExport) {
		t.Fatalf("error = %v, want ErrInvalidKeyExport", result.err)
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 0 {
		t.Fatalf("stored keys = %d (err %v), want 0", len(keys), err)
	}
}

// TestDashboardExportReportsWrittenPath is the TUI side of BUG-006: the Export form with
// no output path reports the file it actually wrote.
func TestDashboardExportReportsWrittenPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clock := time.Unix(1_800_000_000, 0)
	previous := timeNow
	timeNow = func() time.Time { clock = clock.Add(time.Second); return clock }
	t.Cleanup(func() { timeNow = previous })

	db := newTestDatabase(t, false)
	keyID := "0123456789abcdef"
	if err := db.SaveKey(&KeyModel{KeyID: keyID, Algorithm: "AES-256-GCM", EncryptedBlob: validStoredBlob(t), CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}

	m := NewDashboardModel(db)
	m.startForm(actionKeysExport, screenKeys)
	result := submitForm(t, m, keyID, "", testPassword)
	if result.err != nil {
		t.Fatalf("export: %v", result.err)
	}
	_, reported, found := strings.Cut(result.message, " → ")
	if !found {
		t.Fatalf("message %q has no reported path", result.message)
	}
	if _, err := os.Stat(filepath.Join(dir, reported)); err != nil {
		t.Fatalf("reported path %q was not written: %v", reported, err)
	}
}

// TestDashboardRefusesSecondActionWhileBusy is a regression test for BUG-008: while one
// action is running, submitting another form starts nothing and says why; once the
// first result arrives, the form can be submitted.
func TestDashboardRefusesSecondActionWhileBusy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	m := NewDashboardModel(newTestDatabase(t, false))

	// First action: submitted, still running (its command hasn't reported back).
	m.startForm(actionCompress, screenMain)
	m = typeString(m, src)
	var first tea.Cmd
	for first == nil {
		next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
		m, first = next.(DashboardModel), cmd
	}
	if !m.busy {
		t.Fatal("busy = false after submitting an action")
	}

	// Second action while busy: every Enter on the last field must be refused.
	m.startForm(actionCompress, screenMain)
	m = typeString(m, src)
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter}) // output
	m = typeString(next.(DashboardModel), filepath.Join(dir, "second.gz"))
	for range 3 {
		next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(DashboardModel)
		if cmd != nil {
			t.Fatal("a second action was started while the first was still running")
		}
	}
	if m.screen != screenForm || !m.isError || !strings.Contains(m.status, "still running") {
		t.Fatalf("screen %v, status %q (isError %v); want the form kept open with a busy message", m.screen, m.status, m.isError)
	}

	// The first action finishes; now the second form can be submitted.
	next, _ = m.Update(first())
	m = next.(DashboardModel)
	if m.busy {
		t.Fatal("busy = true after the first action reported back")
	}
	next, cmd := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("the form couldn't be submitted after the first action finished")
	}
	if result, ok := cmd().(actionResultMsg); !ok || result.err != nil {
		t.Fatalf("second action result = %+v", result)
	}
	_ = next
}

// TestKeyScreenEscapesControlCharacters is SEC-016's display check for the TUI: the key
// table shows a stored key ID and algorithm holding escape sequences quoted, not raw.
func TestKeyScreenEscapesControlCharacters(t *testing.T) {
	db := newTestDatabase(t, false)
	m := NewDashboardModel(db)
	next, _ := m.Update(keysLoadedMsg{keys: []KeyModel{{
		KeyID:      "\x1b]0;PWNED\a",
		Algorithm:  "\x1b[2J",
		CreatedAt_: 1,
	}}})
	m = next.(DashboardModel)
	m.screen = screenKeys

	view := m.View()
	for _, raw := range []string{"\x1b]0;PWNED", "\a", "\x1b[2J"} {
		if strings.Contains(view, raw) {
			t.Fatalf("key screen shows raw %q: %q", raw, view)
		}
	}
	for _, want := range []string{`"\x1b]0;PWNED\a"`, `"\x1b[2J"`} {
		if !strings.Contains(view, want) {
			t.Errorf("key screen lacks %s: %q", want, view)
		}
	}
}

// TestDashboardExportRefusesExistingOutput mirrors the CLI check for BUG-017: the TUI's
// export form never writes over an existing file or the key database.
func TestDashboardExportRefusesExistingOutput(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "keys.db")
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { closeTestDatabase(t, db) })
	const keyID = "0123456789abcdef"
	if err := db.SaveKey(&KeyModel{KeyID: keyID, Algorithm: "AES-256-GCM", EncryptedBlob: validStoredBlob(t), CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	precious := filepath.Join(t.TempDir(), "precious.txt")
	if err := os.WriteFile(precious, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, tc := range []struct {
		output string
		want   error
	}{
		{precious, ErrOutputExists},
		{dbPath, ErrOutputIsKeyDatabase},
	} {
		m := NewDashboardModel(db)
		m.startForm(actionKeysExport, screenKeys)
		result := submitForm(t, m, keyID, tc.output, testPassword)
		if !errors.Is(result.err, tc.want) {
			t.Fatalf("export to %s: err = %v, want %v", tc.output, result.err, tc.want)
		}
	}
	if got, _ := os.ReadFile(precious); string(got) != "keep me" {
		t.Fatalf("existing file changed to %q", got)
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 1 {
		t.Fatalf("key database after refused exports: %d keys (err %v), want 1", len(keys), err)
	}
}

// TestDashboardEncryptFolderOutputStaysOutside mirrors the CLI check for BUG-016 in the
// TUI's encrypt form.
func TestDashboardEncryptFolderOutputStaysOutside(t *testing.T) {
	parent := t.TempDir()
	folder := filepath.Join(parent, "secret")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "a.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	db := newTestDatabase(t, false)

	m := NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	result := submitForm(t, m, folder+string(filepath.Separator), "", testPassword, testPassword)
	if result.err != nil {
		t.Fatalf("encrypt secret/: %v", result.err)
	}
	if !strings.HasSuffix(result.message, "→ "+folder+encExt) {
		t.Fatalf("message %q doesn't report %s", result.message, folder+encExt)
	}

	m = NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	result = submitForm(t, m, folder, filepath.Join(folder, "x.enc"), testPassword, testPassword)
	if !errors.Is(result.err, ErrOutputInsideInput) {
		t.Fatalf("encrypt into the folder: err = %v, want ErrOutputInsideInput", result.err)
	}
}

// TestDashboardQuitWhileBusyCancelsAction checks SEC-015 in the TUI: quitting while an
// action runs, from the menu or from a form, cancels the action and ends the program
// only once the action has reported back, so its clean-up isn't cut short.
func TestDashboardQuitWhileBusyCancelsAction(t *testing.T) {
	quitKeys := []struct {
		name string
		form bool
		key  tea.KeyMsg
	}{
		{"q on a menu", false, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}},
		{"ctrl+c on a menu", false, tea.KeyMsg{Type: tea.KeyCtrlC}},
		{"ctrl+c in a form", true, tea.KeyMsg{Type: tea.KeyCtrlC}},
	}
	for _, tc := range quitKeys {
		t.Run(tc.name, func(t *testing.T) {
			m := NewDashboardModel(newTestDatabase(t, false))
			started := make(chan struct{})
			cmd := m.runner.cmd(func(ctx context.Context) tea.Msg {
				close(started)
				<-ctx.Done()
				return actionResultMsg{err: ctx.Err()}
			})
			m.busy = true
			if tc.form {
				m.startForm(actionDecrypt, screenMain)
			}
			results := make(chan tea.Msg, 1)
			go func() { results <- cmd() }()
			<-started

			next, quitCmd := m.Update(tc.key)
			m = next.(DashboardModel)
			if quitCmd != nil {
				t.Fatal("quitting while busy ended the program at once; want it to wait for the action")
			}
			if !m.quitting || m.status != "Cancelling…" {
				t.Fatalf("quitting = %v, status = %q; want true and \"Cancelling…\"", m.quitting, m.status)
			}

			var msg tea.Msg
			select {
			case msg = <-results:
			case <-time.After(10 * time.Second):
				t.Fatal("the action wasn't cancelled")
			}
			if result, ok := msg.(actionResultMsg); !ok || !errors.Is(result.err, context.Canceled) {
				t.Fatalf("action result = %#v, want context.Canceled", msg)
			}
			next, finalCmd := m.Update(msg)
			m = next.(DashboardModel)
			if finalCmd == nil {
				t.Fatal("no command after the cancelled action reported back; want tea.Quit")
			}
			if _, ok := finalCmd().(tea.QuitMsg); !ok {
				t.Fatal("the program didn't quit after the cancelled action reported back")
			}
		})
	}
}

// TestDashboardQuitWhenIdle checks that quitting without a running action still ends
// the program at once.
func TestDashboardQuitWhenIdle(t *testing.T) {
	m := NewDashboardModel(newTestDatabase(t, false))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q did nothing")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q didn't quit")
	}
}

// TestActionRunnerShutdownWaitsForAction checks what the TUI launcher relies on when
// the program ends some other way, such as on a signal (SEC-015): shutdown cancels the
// running action and returns only after it has returned, and later actions don't run.
func TestActionRunnerShutdownWaitsForAction(t *testing.T) {
	r := &actionRunner{}
	started := make(chan struct{})
	var cleanedUp atomic.Bool
	cmd := r.cmd(func(ctx context.Context) tea.Msg {
		close(started)
		<-ctx.Done()
		cleanedUp.Store(true)
		return actionResultMsg{err: ctx.Err()}
	})
	go cmd()
	<-started

	r.shutdown()
	if !cleanedUp.Load() {
		t.Fatal("shutdown returned before the running action had finished")
	}

	ran := false
	msg := r.cmd(func(context.Context) tea.Msg { ran = true; return nil })()
	if ran {
		t.Fatal("an action ran after shutdown")
	}
	if result, ok := msg.(actionResultMsg); !ok || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("result after shutdown = %#v, want context.Canceled", msg)
	}
}

// TestDashboardCompressRefusesContradictoryOptions mirrors the CLI check for BUG-020 in
// the TUI's compress form.
func TestDashboardCompressRefusesContradictoryOptions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	db := newTestDatabase(t, false)
	for _, tc := range []struct {
		output, format, level string
		want                  error
	}{
		{filepath.Join(dir, "x.zip"), "gzip", "", ErrFormatMismatch},
		{"", "", "42", ErrInvalidLevel},
		{"", "", "0", ErrInvalidLevel},
	} {
		m := NewDashboardModel(db)
		m.startForm(actionCompress, screenMain)
		result := submitForm(t, m, src, tc.output, tc.format, tc.level)
		if !errors.Is(result.err, tc.want) {
			t.Fatalf("compress (output %q, format %q, level %q): err = %v, want %v", tc.output, tc.format, tc.level, result.err, tc.want)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("folder holds %d entries (err %v), want only a.txt", len(entries), err)
	}
}
