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
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newTestDatabase creates a test database with proper cleanup.
// It uses in-memory SQLite by default to avoid file-locking issues on Windows.
// If useFile is true, it creates a temp file-based database instead.
func newTestDatabase(t *testing.T, useFile bool) *Database {
	t.Helper()

	var path string
	if useFile {
		tmpDir := t.TempDir()
		path = filepath.Join(tmpDir, "test.db")
	} else {
		// Use in-memory database for most tests
		path = ":memory:"
	}

	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}

	// Ensure database connection is closed after test
	t.Cleanup(func() {
		if db != nil && db.Conn() != nil {
			sqlDB, err := db.Conn().DB()
			if err == nil {
				err := sqlDB.Close()
				if err != nil {
					return
				}
			}
		}
	})

	return db
}

// TestNewDatabase tests the NewDatabase function to ensure it creates a new database and returns a valid connection.
// It verifies that the database file is created and that the connection is not nil.
func TestNewDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}

	defer func() {
		if db != nil && db.Conn() != nil {
			sqlDB, err := db.Conn().DB()
			if err == nil {
				err := sqlDB.Close()
				if err != nil {
					return
				}
			}
		}
	}()

	if db.Conn() == nil {
		t.Error("Database connection is nil")
	}

	// Verify database file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("Database file not created: %v", err)
	}
}

// TestNewDatabaseDefaultPath tests the NewDatabase function when no path is provided.
// It ensures that the database is created at the default location.
func TestNewDatabaseDefaultPath(t *testing.T) {
	tmpDir := t.TempDir()
	cwd, _ := os.Getwd()
	defer func(dir string) {
		err := os.Chdir(dir)
		if err != nil {
			t.Fatalf("Failed to change directory back to original: %v", err)
		}
	}(cwd)
	err := os.Chdir(tmpDir)
	if err != nil {
		return
	}

	db, err := NewDatabase("")
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}

	defer func() {
		if db != nil && db.Conn() != nil {
			sqlDB, err := db.Conn().DB()
			if err == nil {
				err := sqlDB.Close()
				if err != nil {
					return
				}
			}
		}
	}()

	if db == nil {
		t.Error("NewDatabase returned nil")
	}
}

// TestSaveKey tests the SaveKey function to ensure that a key can be saved to the database.
// It verifies that no error is returned when saving a valid key.
func TestSaveKey(t *testing.T) {
	db := newTestDatabase(t, false)

	km := &KeyModel{
		KeyID:         "test-key-1",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "base64encodedblob",
		CreatedAt_:    time.Now().Unix(),
	}

	err := db.SaveKey(km)
	if err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}
}

// TestListKeys tests the ListKeys function to ensure it returns all saved keys.
// It verifies that the initial list is empty and that added keys are correctly listed.
func TestListKeys(t *testing.T) {
	db := newTestDatabase(t, false)

	// Initially, no keys
	keys, err := db.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys initially, got %d", len(keys))
	}

	// Add some keys
	for i := 1; i <= 3; i++ {
		km := &KeyModel{
			KeyID:         "test-key-" + string(rune('0'+i)),
			Algorithm:     "AES-256-GCM",
			EncryptedBlob: "blob" + string(rune('0'+i)),
			CreatedAt_:    time.Now().Unix(),
		}
		if err := db.SaveKey(km); err != nil {
			t.Fatalf("SaveKey failed: %v", err)
		}
	}

	// List keys
	keys, err = db.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}
}

// TestGetKey tests the GetKey function to ensure it retrieves a key by its ID.
// It verifies that the retrieved key matches the saved key.
func TestGetKey(t *testing.T) {
	db := newTestDatabase(t, false)

	keyID := "test-key-1"
	km := &KeyModel{
		KeyID:         keyID,
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "myblob",
		CreatedAt_:    time.Now().Unix(),
	}

	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	retrieved, err := db.GetKey(keyID)
	if err != nil {
		t.Fatalf("GetKey failed: %v", err)
	}

	if retrieved.KeyID != keyID {
		t.Errorf("KeyID mismatch: got %q, want %q", retrieved.KeyID, keyID)
	}
	if retrieved.Algorithm != km.Algorithm {
		t.Errorf("Algorithm mismatch: got %q, want %q", retrieved.Algorithm, km.Algorithm)
	}
	if retrieved.EncryptedBlob != km.EncryptedBlob {
		t.Errorf("EncryptedBlob mismatch: got %q, want %q", retrieved.EncryptedBlob, km.EncryptedBlob)
	}
}

// TestGetKeyNotFound tests the GetKey function when the requested key does not exist.
// It ensures that an error is returned for a nonexistent key.
func TestGetKeyNotFound(t *testing.T) {
	db := newTestDatabase(t, false)

	_, err := db.GetKey("nonexistent")
	if err == nil {
		t.Error("GetKey should fail for nonexistent key")
	}
}

// TestDeleteKey tests the DeleteKey function to ensure it removes a key from the database.
// It verifies that the key exists before deletion and that it cannot be retrieved afterward.
func TestDeleteKey(t *testing.T) {
	db := newTestDatabase(t, false)

	keyID := "test-key-to-delete"
	km := &KeyModel{
		KeyID:         keyID,
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "myblob",
		CreatedAt_:    time.Now().Unix(),
	}

	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	// Verify key exists
	_, err := db.GetKey(keyID)
	if err != nil {
		t.Fatalf("GetKey failed before delete: %v", err)
	}

	// Delete key
	if err := db.DeleteKey(keyID); err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	// Verify key no longer exists
	_, err = db.GetKey(keyID)
	if err == nil {
		t.Error("GetKey should fail after delete")
	}
}

// TestKeyModelUniqueConstraint tests the unique constraint on the KeyModel's KeyID field.
// It ensures that attempting to save a key with a duplicate KeyID either fails or updates the existing key, depending on the database behavior.
func TestKeyModelUniqueConstraint(t *testing.T) {
	db := newTestDatabase(t, false)

	keyID := "unique-key"
	km1 := &KeyModel{
		KeyID:         keyID,
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob1",
		CreatedAt_:    time.Now().Unix(),
	}

	km2 := &KeyModel{
		KeyID:         keyID, // Same KeyID
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob2",
		CreatedAt_:    time.Now().Unix(),
	}

	if err := db.SaveKey(km1); err != nil {
		t.Fatalf("SaveKey km1 failed: %v", err)
	}

	// Second save should fail due to unique constraint or succeed as update
	// Both behaviors are acceptable depending on GORM's safe semantics
	_ = db.SaveKey(km2)

	// Verify only one key with this ID exists
	keys, err := db.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}

	count := 0
	for _, k := range keys {
		if k.KeyID == keyID {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected 1 key with ID %q, got %d", keyID, count)
	}
}

// TestDeleteKeyWipesBlobFromFile is a regression test for SEC-009: once a key is
// deleted, its encrypted blob must no longer be readable from the database file or
// its journal.
func TestDeleteKeyWipesBlobFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.db")
	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	sqlDB, err := db.Conn().DB()
	if err != nil {
		t.Fatalf("sql handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	marker := []byte(strings.Repeat("SEC009-deleted-blob-marker-", 8))
	for _, km := range []*KeyModel{
		{KeyID: "keep", Algorithm: "AES-256-GCM", EncryptedBlob: "still-stored", CreatedAt_: 1},
		{KeyID: "gone", Algorithm: "AES-256-GCM", EncryptedBlob: string(marker), CreatedAt_: 1},
	} {
		if err := db.SaveKey(km); err != nil {
			t.Fatalf("SaveKey(%s): %v", km.KeyID, err)
		}
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Contains(data, marker) {
		t.Fatalf("test setup: blob not found in the database file (err %v)", err)
	}

	if err := db.DeleteKey("gone"); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
	for _, p := range []string{path, path + "-journal", path + "-wal"} {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if bytes.Contains(data, marker) {
			t.Errorf("deleted key's blob is still readable in %s", filepath.Base(p))
		}
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 1 || keys[0].KeyID != "keep" {
		t.Fatalf("remaining keys = %+v (err %v), want only %q", keys, err, "keep")
	}

	// secure_delete is a per-connection setting, so check several pooled connections,
	// held open at the same time so that each is a separate connection.
	for i := range 3 {
		conn, err := sqlDB.Conn(context.Background())
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		var on int
		if err := conn.QueryRowContext(context.Background(), "PRAGMA secure_delete").Scan(&on); err != nil || on != 1 {
			t.Fatalf("connection %d: PRAGMA secure_delete = %d (err %v), want 1", i, on, err)
		}
	}
}

// TestWithSecureDelete checks how the secure-delete parameter is added to a database
// path, including one that already carries SQLite URI parameters.
func TestWithSecureDelete(t *testing.T) {
	tests := map[string]string{
		"cryptare.db":           "cryptare.db?_secure_delete=on",
		":memory:":              ":memory:?_secure_delete=on",
		"file:keys.db?mode=rwc": "file:keys.db?mode=rwc&_secure_delete=on",
	}
	for in, want := range tests {
		if got := withSecureDelete(in); got != want {
			t.Errorf("withSecureDelete(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNewDatabaseCreatesPrivateFile is a regression test for SEC-010: a new key
// database file is created with mode 0600.
func TestNewDatabaseCreatesPrivateFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply on Windows")
	}
	path := filepath.Join(t.TempDir(), "new.db")
	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	closeTestDatabase(t, db)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("new database mode = %04o, want 0600", got)
	}
}

// TestNewDatabaseTightensExistingFile checks the owner's decision for SEC-010: opening
// an existing database (and a leftover journal) that others can read sets it to 0600.
func TestNewDatabaseTightensExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply on Windows")
	}
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	if err := db.SaveKey(&KeyModel{KeyID: "kept", Algorithm: "AES-256-GCM", EncryptedBlob: "blob", CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	closeTestDatabase(t, db)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod database: %v", err)
	}
	// An empty journal is treated as not hot, so SQLite ignores it.
	journal := path + "-journal"
	if err := os.WriteFile(journal, nil, 0o644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	if err := os.Chmod(journal, 0o644); err != nil {
		t.Fatalf("chmod journal: %v", err)
	}

	db, err = NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase (existing): %v", err)
	}
	defer closeTestDatabase(t, db)
	for _, p := range []string{path, journal} {
		info, err := os.Stat(p)
		if os.IsNotExist(err) && p == journal {
			continue // SQLite may remove an empty journal
		}
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %04o after opening, want 0600", filepath.Base(p), got)
		}
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 1 {
		t.Fatalf("keys after reopening = %d (err %v), want 1", len(keys), err)
	}
}

// closeTestDatabase closes the database's SQL handle.
func closeTestDatabase(t *testing.T, db *Database) {
	t.Helper()
	sqlDB, err := db.Conn().DB()
	if err != nil {
		t.Fatalf("sql handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
}

// TestCheckDatabaseFileTrust checks SEC-016's ownership and permission rule with
// synthetic owners and modes, so no second user account is needed.
func TestCheckDatabaseFileTrust(t *testing.T) {
	const me, other = 1000, 1001
	tests := []struct {
		name    string
		owner   int
		perm    os.FileMode
		refused bool
	}{
		{"own private file", me, 0o600, false},
		{"own file others can read", me, 0o644, false}, // then set to 0600 (SEC-010)
		{"own group-writable file", me, 0o620, true},
		{"own world-writable file", me, 0o666, true},
		{"another user's private file", other, 0o600, true},
		{"another user's world-writable file", other, 0o666, true},
		{"root's file opened by another user", 0, 0o600, true},
		{"owner not reported, private", -1, 0o600, false},
		{"owner not reported, world-writable", -1, 0o602, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkDatabaseFileTrust("keys.db", tc.owner, tc.perm, me)
			if got := errors.Is(err, ErrUntrustedDatabase); got != tc.refused {
				t.Fatalf("refused = %v (err %v), want %v", got, err, tc.refused)
			}
		})
	}
}

// TestNewDatabaseRefusesFilesOthersCanWrite checks SEC-016 end to end: an existing
// database that others can write, and a planted journal next to a new database, are
// refused, and the refused database is left as it was.
func TestNewDatabaseRefusesFilesOthersCanWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply on Windows")
	}
	t.Run("world-writable database", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "shared.db")
		db, err := NewDatabase(path)
		if err != nil {
			t.Fatalf("NewDatabase: %v", err)
		}
		closeTestDatabase(t, db)
		if err := os.Chmod(path, 0o666); err != nil {
			t.Fatalf("chmod database: %v", err)
		}

		if _, err := NewDatabase(path); !errors.Is(err, ErrUntrustedDatabase) {
			t.Fatalf("NewDatabase err = %v, want ErrUntrustedDatabase", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat database: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o666 {
			t.Fatalf("refused database mode = %04o, want it left at 0666", got)
		}
	})
	t.Run("planted journal next to a new database", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "new.db")
		journal := path + "-journal"
		if err := os.WriteFile(journal, []byte("planted"), 0o600); err != nil {
			t.Fatalf("write journal: %v", err)
		}
		if err := os.Chmod(journal, 0o666); err != nil {
			t.Fatalf("chmod journal: %v", err)
		}
		if _, err := NewDatabase(path); !errors.Is(err, ErrUntrustedDatabase) {
			t.Fatalf("NewDatabase err = %v, want ErrUntrustedDatabase", err)
		}
	})
}

// TestDatabaseErrorsAreTyped checks the errors the CLI and TUI explain since GORM's log
// is silenced (SEC-017): a missing key is ErrKeyNotFound, and a duplicate key ID is
// ErrKeyExists.
func TestDatabaseErrorsAreTyped(t *testing.T) {
	db := newTestDatabase(t, false)
	if _, err := db.GetKey("ffffffffffffffff"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("GetKey(missing) err = %v, want ErrKeyNotFound", err)
	}
	key := func() *KeyModel {
		return &KeyModel{KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", EncryptedBlob: "blob", CreatedAt_: 1}
	}
	if err := db.SaveKey(key()); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	if err := db.SaveKey(key()); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("SaveKey(duplicate) err = %v, want ErrKeyExists", err)
	}
}

// TestDatabasePathsOpenThePreparedFile is the regression test for SEC-010's path gap
// (plan 5.8): the file that holds the keys is the one Cryptare prepared (0600, trust
// checked), for a file: URI as for a plain path, and a plain path that SQLite would
// cut at "?" is refused without creating anything.
func TestDatabasePathsOpenThePreparedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(`Unix permission bits don't apply, and "?" can't appear in Windows names`)
	}
	listing := func(dir string) []string {
		t.Helper()
		var names []string
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p != dir {
				rel, _ := filepath.Rel(dir, p)
				names = append(names, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
		return names
	}
	uri := func(path, query string) string {
		return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: query}).String()
	}

	t.Run("plain path with ? is refused", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "a?b"), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		for _, path := range []string{filepath.Join(dir, "a?b", "keys.db"), filepath.Join(dir, "keys.db?_journal_mode=WAL")} {
			if _, err := NewDatabase(path); !errors.Is(err, ErrUnsupportedDatabasePath) {
				t.Fatalf("NewDatabase(%s) err = %v, want ErrUnsupportedDatabasePath", path, err)
			}
		}
		if got := listing(dir); len(got) != 1 || got[0] != "a?b" {
			t.Fatalf("folder holds %v, want only the empty a?b", got)
		}
	})

	t.Run("file: URI", func(t *testing.T) {
		dir := t.TempDir()
		folder := filepath.Join(dir, "a?b") // reachable through %3F
		if err := os.Mkdir(folder, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		file := filepath.Join(folder, "keys.db")
		dsn := uri(file, "")
		if !strings.Contains(dsn, "%3F") {
			t.Fatalf("URI %s doesn't escape the ?", dsn)
		}
		db, err := NewDatabase(dsn)
		if err != nil {
			t.Fatalf("NewDatabase(%s): %v", dsn, err)
		}
		if err := db.SaveKey(&KeyModel{KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", EncryptedBlob: "blob", CreatedAt_: 1}); err != nil {
			t.Fatalf("SaveKey: %v", err)
		}
		closeTestDatabase(t, db)

		info, err := os.Stat(file)
		if err != nil {
			t.Fatalf("the keys aren't in %s: %v", file, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("database behind the URI has mode %04o, want 0600", got)
		}
		if got := listing(dir); len(got) != 2 {
			t.Fatalf("folder holds %v, want only a?b and a?b/keys.db", got)
		}
		db, err = NewDatabase(dsn)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		keys, err := db.ListKeys()
		closeTestDatabase(t, db)
		if err != nil || len(keys) != 1 {
			t.Fatalf("keys after reopening = %d (err %v), want 1", len(keys), err)
		}

		// The trust check covers a file behind a URI too (SEC-016).
		if err := os.Chmod(file, 0o666); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		if _, err := NewDatabase(dsn); !errors.Is(err, ErrUntrustedDatabase) {
			t.Fatalf("NewDatabase(world-writable file behind a URI) err = %v, want ErrUntrustedDatabase", err)
		}
	})

	t.Run("in-memory URI creates nothing", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		db, err := NewDatabase("file:scratch?mode=memory")
		if err != nil {
			t.Fatalf("NewDatabase: %v", err)
		}
		closeTestDatabase(t, db)
		if got := listing(dir); len(got) != 0 {
			t.Fatalf("folder holds %v, want nothing", got)
		}
	})
}

// TestDatabaseFilePath checks how key database paths map to files (plan 5.8).
func TestDatabaseFilePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file: URIs are left to SQLite on Windows")
	}
	tests := []struct {
		dsn      string
		path     string
		readOnly bool
		ok       bool
		err      error
	}{
		{":memory:", "", false, false, nil},
		{"keys.db", "keys.db", false, true, nil},
		{"dir/keys.db", "dir/keys.db", false, true, nil},
		{"keys.db?_journal_mode=WAL", "", false, false, ErrUnsupportedDatabasePath},
		{"file:keys.db", "keys.db", false, true, nil},
		{"file:/tmp/a%3Fb/keys.db?_journal_mode=WAL", "/tmp/a?b/keys.db", false, true, nil},
		{"file:///tmp/keys.db", "/tmp/keys.db", false, true, nil},
		{"file://localhost/tmp/keys.db?mode=ro", "/tmp/keys.db", true, true, nil},
		{"file://elsewhere/tmp/keys.db", "", false, false, ErrUnsupportedDatabasePath},
		{"file::memory:", "", false, false, nil},
		{"file:scratch?mode=memory&cache=shared", "", false, false, nil},
	}
	for _, tc := range tests {
		path, readOnly, ok, err := databaseFilePath(tc.dsn)
		if !errors.Is(err, tc.err) || (tc.err == nil && err != nil) {
			t.Errorf("%s: err = %v, want %v", tc.dsn, err, tc.err)
			continue
		}
		if path != filepath.FromSlash(tc.path) || readOnly != tc.readOnly || ok != tc.ok {
			t.Errorf("%s = (%q, readOnly %v, ok %v), want (%q, %v, %v)", tc.dsn, path, readOnly, ok, tc.path, tc.readOnly, tc.ok)
		}
	}
}
