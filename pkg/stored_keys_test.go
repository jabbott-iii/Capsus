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
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CLI and TUI tests for stored keys (plans 3.3 and 3.4, BUG-011) and for when the CLI
// prints its usage text.

// runStoredKeyCLI runs the CLI against db with stdin, returning its combined output.
func runStoredKeyCLI(db *Database, stdin string, args ...string) (string, error) {
	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs(args)
	rootCmd.SetIn(strings.NewReader(stdin))
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	err := rootCmd.Execute()
	return out.String(), err
}

// TestCLIStoredKeyRoundTrip covers the CLI flow: keys generate, encrypt --key, and a
// decrypt that finds the key from the file's header and asks for its master password.
func TestCLIStoredKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db := newTestDatabase(t, false)
	pw := writePasswordFile(t, testPassword)
	file, folder := writeStoredKeyFixtures(t, dir)

	if _, err := runStoredKeyCLI(db, "", "keys", "generate", "--password-file", pw); err != nil {
		t.Fatalf("keys generate: %v", err)
	}
	keys, err := db.ListKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys = %d (err %v), want 1", len(keys), err)
	}
	keyID := keys[0].KeyID

	if _, err := runStoredKeyCLI(db, "", "encrypt", file, "--key", keyID, "--password-file", pw); err != nil {
		t.Fatalf("encrypt --key: %v", err)
	}
	if _, err := runStoredKeyCLI(db, "", "encrypt", folder, "-k", keyID, "--password-file", pw); err != nil {
		t.Fatalf("encrypt -k (folder): %v", err)
	}
	if got, ok := EncryptedWithStoredKey(file + encExt); !ok || got != keyID {
		t.Fatalf("encrypted file names key %q (%v), want %s", got, ok, keyID)
	}

	// The master password at the prompt (piped), with no flag naming the key.
	out := filepath.Join(dir, "out.txt")
	output, err := runStoredKeyCLI(db, testPassword+"\n", "decrypt", file+encExt, "--output", out)
	if err != nil {
		t.Fatalf("decrypt: %v (output %q)", err, output)
	}
	if !strings.Contains(output, "Enter master password for key "+keyID) {
		t.Fatalf("decrypt prompt = %q, want it to name the key", output)
	}
	if got, _ := os.ReadFile(out); string(got) != "stored-key secret" {
		t.Fatalf("decrypted file = %q", got)
	}
	outDir := filepath.Join(dir, "out-tree")
	if _, err := runStoredKeyCLI(db, "", "decrypt", folder+encExt, "--output", outDir, "--password-file", pw); err != nil {
		t.Fatalf("decrypt folder: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(outDir, "sub", "b.txt")); string(got) != "nested" {
		t.Fatalf("decrypted folder file = %q", got)
	}

	// A wrong master password, and a key database without the key, write nothing.
	refused := filepath.Join(dir, "refused.txt")
	if _, err := runStoredKeyCLI(db, "", "decrypt", file+encExt, "--output", refused, "--password-file", writePasswordFile(t, "not the master password")); !errors.Is(err, ErrWrongMasterPassword) {
		t.Fatalf("wrong master password: err = %v, want ErrWrongMasterPassword", err)
	}
	empty := newTestDatabase(t, false)
	_, err = runStoredKeyCLI(empty, "", "decrypt", file+encExt, "--output", refused, "--password-file", pw)
	if !errors.Is(err, ErrKeyNotFound) || !strings.Contains(err.Error(), "keys import") {
		t.Fatalf("missing key: err = %v, want ErrKeyNotFound with a keys import hint", err)
	}
	if _, err := os.Stat(refused); !os.IsNotExist(err) {
		t.Fatalf("output written by a refused decrypt (stat err %v)", err)
	}
	if _, err := runStoredKeyCLI(db, "", "encrypt", file, "--output", filepath.Join(dir, "x.enc"), "--key", "ffffffffffffffff", "--password-file", pw); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("encrypt with an unknown key: err = %v, want ErrKeyNotFound", err)
	}
	// An empty --key (an unset variable in a script) is refused, not taken as "use the
	// password": the file would otherwise be protected by the master password alone.
	emptyKeyOut := filepath.Join(dir, "empty-key.enc")
	if _, err := runStoredKeyCLI(db, "", "encrypt", file, "--output", emptyKeyOut, "--key", "", "--password-file", pw); err == nil || !strings.Contains(err.Error(), "--key") {
		t.Fatalf("encrypt --key \"\": err = %v, want a refusal naming --key", err)
	}
	if _, err := os.Stat(emptyKeyOut); !os.IsNotExist(err) {
		t.Fatalf("encrypt --key \"\" wrote %s (stat err %v)", emptyKeyOut, err)
	}
}

// TestCLIKeysExportPrompt is a regression test for BUG-011's wording: the export asks
// once for the key's master password, naming the key, and checks it.
func TestCLIKeysExportPrompt(t *testing.T) {
	dir := t.TempDir()
	db := newTestDatabase(t, false)
	keyID := newStoredKey(t, db)
	path := filepath.Join(dir, "key.ckey")

	output, err := runStoredKeyCLI(db, testPassword+"\n", "keys", "export", keyID, "--output", path)
	if err != nil {
		t.Fatalf("keys export: %v", err)
	}
	if !strings.Contains(output, "Enter master password for key "+keyID) || strings.Contains(output, "Confirm") {
		t.Fatalf("export prompts = %q, want one prompt naming the key", output)
	}
	other := filepath.Join(dir, "other.ckey")
	if _, err := runStoredKeyCLI(db, "correct horse battery stapler\n", "keys", "export", keyID, "--output", other); !errors.Is(err, ErrWrongMasterPassword) {
		t.Fatalf("export with another password: err = %v, want ErrWrongMasterPassword", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("export written despite the wrong password (stat err %v)", err)
	}
}

// TestCLIKeysImportSeparatePassword checks the CLI with an export that has a password of
// its own (BUG-011): with one password and no terminal it explains and stores nothing,
// and --export-password-file supplies the other password, in either role.
func TestCLIKeysImportSeparatePassword(t *testing.T) {
	db := newTestDatabase(t, false)
	path := goldenPath("v2-key-export-separate-password.txt")
	for _, one := range []string{goldenExportPassword, goldenPassword} {
		_, err := runStoredKeyCLI(db, "", "keys", "import", path, "--password-file", writePasswordFile(t, one))
		explained := errors.Is(err, ErrSeparateKeyPassword) || errors.Is(err, ErrWrongExportPassword)
		if !explained || !strings.Contains(err.Error(), "--export-password-file") {
			t.Fatalf("err = %v, want a hint to give the other password with --export-password-file", err)
		}
	}
	if keys, _ := db.ListKeys(); len(keys) != 0 {
		t.Fatalf("%d keys stored, want 0", len(keys))
	}
	if _, err := runStoredKeyCLI(db, "", "keys", "import", path, "--password-file", writePasswordFile(t, goldenPassword), "--export-password-file", writePasswordFile(t, goldenExportPassword)); err != nil {
		t.Fatalf("import with --export-password-file: %v", err)
	}
	other := newTestDatabase(t, false)
	if _, err := runStoredKeyCLI(other, "", "keys", "import", path, "--password-file", writePasswordFile(t, goldenExportPassword), "--export-password-file", writePasswordFile(t, goldenPassword)); err != nil {
		t.Fatalf("import with the two passwords swapped: %v", err)
	}
}

// TestCLIUsageOnlyForCommandLineMistakes checks that the usage text follows a mistake in
// the command line, and not a command that fails for another reason.
func TestCLIUsageOnlyForCommandLineMistakes(t *testing.T) {
	dir := t.TempDir()
	db := newTestDatabase(t, false)
	file, _ := writeStoredKeyFixtures(t, dir)
	if err := EncryptFile(file, file+encExt, testPassword); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}
	failures := [][]string{
		{"decrypt", file + encExt, "--output", filepath.Join(dir, "out"), "--password-file", writePasswordFile(t, "the wrong password here")},
		{"encrypt", file, "--password-file", writePasswordFile(t, testPassword)}, // the output exists
		{"keys", "export", "ffffffffffffffff", "--password-file", writePasswordFile(t, testPassword)},
	}
	for _, args := range failures {
		output, err := runStoredKeyCLI(db, "", args...)
		if err == nil {
			t.Fatalf("%v succeeded", args)
		}
		if strings.Contains(output, "Usage:") {
			t.Fatalf("%v printed its usage after a failure that isn't a usage mistake:\n%s", args, output)
		}
	}
	mistakes := [][]string{
		{"decrypt"},                         // missing argument
		{"encrypt", file, "--no-such-flag"}, // unknown flag
	}
	for _, args := range mistakes {
		output, err := runStoredKeyCLI(db, "", args...)
		if err == nil || !strings.Contains(output, "Usage:") {
			t.Fatalf("%v: err = %v, output %q; want the usage text", args, err, output)
		}
	}
}

// TestDashboardStoredKeys covers the TUI: the encrypt form's stored key field, a
// decrypt that finds the key from the header, an export that takes the key's master
// password once, and an import of an export with a password of its own.
func TestDashboardStoredKeys(t *testing.T) {
	dir := t.TempDir()
	db := newTestDatabase(t, false)
	keyID := newStoredKey(t, db)
	file, _ := writeStoredKeyFixtures(t, dir)

	m := NewDashboardModel(db)
	m.startForm(actionEncrypt, screenMain)
	// File, output, the key's master password, no confirmation, the key.
	if result := submitForm(t, m, file, "", testPassword, "", keyID); result.err != nil {
		t.Fatalf("encrypt with a stored key: %v", result.err)
	}
	if got, ok := EncryptedWithStoredKey(file + encExt); !ok || got != keyID {
		t.Fatalf("encrypted file names key %q (%v), want %s", got, ok, keyID)
	}

	out := filepath.Join(dir, "out.txt")
	m.startForm(actionDecrypt, screenMain)
	if result := submitForm(t, m, file+encExt, out, testPassword); result.err != nil {
		t.Fatalf("decrypt: %v", result.err)
	}
	if got, _ := os.ReadFile(out); string(got) != "stored-key secret" {
		t.Fatalf("decrypted file = %q", got)
	}

	m.startForm(actionEncrypt, screenMain)
	if result := submitForm(t, m, file, filepath.Join(dir, "x.enc"), "not the master password", "", keyID); !errors.Is(result.err, ErrWrongMasterPassword) {
		t.Fatalf("encrypt with a wrong master password: err = %v, want ErrWrongMasterPassword", result.err)
	}

	export := filepath.Join(dir, "key.ckey")
	m.startForm(actionKeysExport, screenKeys)
	if result := submitForm(t, m, keyID, export, testPassword); result.err != nil {
		t.Fatalf("export: %v", result.err)
	}

	other := newTestDatabase(t, false)
	m = NewDashboardModel(other)
	separate := goldenPath("v2-key-export-separate-password.txt")
	m.startForm(actionKeysImport, screenKeys)
	if result := submitForm(t, m, separate, goldenExportPassword); !errors.Is(result.err, ErrSeparateKeyPassword) {
		t.Fatalf("import without the key's password: err = %v, want ErrSeparateKeyPassword", result.err)
	}
	// The key's master password first, as the fields are labelled; the other order
	// works too.
	m.startForm(actionKeysImport, screenKeys)
	if result := submitForm(t, m, separate, goldenPassword, goldenExportPassword); result.err != nil {
		t.Fatalf("import with both passwords: %v", result.err)
	}
	m2 := NewDashboardModel(newTestDatabase(t, false))
	m2.startForm(actionKeysImport, screenKeys)
	if result := submitForm(t, m2, separate, goldenExportPassword, goldenPassword); result.err != nil {
		t.Fatalf("import with the passwords swapped: %v", result.err)
	}
	m.startForm(actionKeysImport, screenKeys)
	if result := submitForm(t, m, export, testPassword); result.err != nil {
		t.Fatalf("import of a new export: %v", result.err)
	}
	if keys, err := other.ListKeys(); err != nil || len(keys) != 2 {
		t.Fatalf("imported keys = %d (err %v), want 2", len(keys), err)
	}
}
