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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDatabasePathUsesConfiguredPath(t *testing.T) {
	want := filepath.Join(t.TempDir(), "keys.db")
	t.Setenv(databasePathEnv, want)

	got, fromEnv, err := databasePath()
	if err != nil || got != want || !fromEnv {
		t.Fatalf("databasePath() = %q, %v, %v; want %q, true, nil", got, fromEnv, err, want)
	}
}

// useDataFolder points the user data folder at a new temporary folder on any system,
// clears CRYPTARE_DB_PATH, and returns the default key database path that results.
func useDataFolder(t *testing.T) string {
	t.Helper()
	t.Setenv(databasePathEnv, "")
	base := t.TempDir()
	dir := base
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", base)
	case "darwin", "ios":
		t.Setenv("HOME", base)
		dir = filepath.Join(base, "Library", "Application Support")
	default:
		t.Setenv("XDG_DATA_HOME", base)
	}
	return filepath.Join(dir, "cryptare", "cryptare.db")
}

// closeDatabase closes db when the test ends, before its temporary folder is removed:
// Windows can't delete a file that is still open.
func closeDatabase(t *testing.T, db *Database) {
	t.Helper()
	if db == nil {
		return
	}
	t.Cleanup(func() {
		if sqlDB, err := db.Conn().DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// TestDatabasePathDefaultsToDataFolder is a regression test for Q-003 (SEC-010 step 3,
// SEC-016 step 3): without CRYPTARE_DB_PATH the key database is in the user's data
// folder, wherever the command runs.
func TestDatabasePathDefaultsToDataFolder(t *testing.T) {
	want := useDataFolder(t)
	t.Chdir(t.TempDir())

	got, fromEnv, err := databasePath()
	if err != nil || got != want || fromEnv {
		t.Fatalf("databasePath() = %q, %v, %v; want %q, false, nil", got, fromEnv, err, want)
	}
}

func TestUserDataDir(t *testing.T) {
	abs := t.TempDir()
	home := filepath.Join(abs, "home")
	tests := []struct {
		name    string
		goos    string
		env     map[string]string
		want    string
		wantErr string
	}{
		{"Linux, XDG_DATA_HOME", "linux", map[string]string{"XDG_DATA_HOME": filepath.Join(abs, "xdg"), "HOME": home}, filepath.Join(abs, "xdg"), ""},
		{"Linux, relative XDG_DATA_HOME is ignored", "linux", map[string]string{"XDG_DATA_HOME": "xdg", "HOME": home}, filepath.Join(home, ".local", "share"), ""},
		{"Linux, HOME", "linux", map[string]string{"HOME": home}, filepath.Join(home, ".local", "share"), ""},
		{"FreeBSD, HOME", "freebsd", map[string]string{"HOME": home}, filepath.Join(home, ".local", "share"), ""},
		{"Linux, no HOME", "linux", nil, "", "$HOME"},
		{"Linux, relative HOME", "linux", map[string]string{"HOME": "home"}, "", "$HOME"},
		{"macOS", "darwin", map[string]string{"HOME": home, "XDG_DATA_HOME": filepath.Join(abs, "xdg")}, filepath.Join(home, "Library", "Application Support"), ""},
		{"macOS, no HOME", "darwin", nil, "", "$HOME"},
		{"Windows", "windows", map[string]string{"LocalAppData": filepath.Join(abs, "local"), "HOME": home}, filepath.Join(abs, "local"), ""},
		{"Windows, no LocalAppData", "windows", map[string]string{"HOME": home}, "", "%LocalAppData%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := userDataDir(tt.goos, func(name string) string { return tt.env[name] })
			if tt.wantErr != "" {
				if !errors.Is(err, errNoDataFolder) || !strings.Contains(err.Error(), tt.wantErr) ||
					!strings.Contains(err.Error(), databasePathEnv) {
					t.Fatalf("userDataDir() = %q, %v; want errNoDataFolder naming %s and %s", got, err, tt.wantErr, databasePathEnv)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("userDataDir() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// TestDatabaseOpenerUsesPrivateDataFolder checks that the keys commands create the
// default key database in a private folder in the user data folder, and nothing in the
// current folder (Q-003).
func TestDatabaseOpenerUsesPrivateDataFolder(t *testing.T) {
	want := useDataFolder(t)
	cwd := t.TempDir()
	t.Chdir(cwd)

	db, err := databaseOpener(io.Discard)()
	closeDatabase(t, db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("key database not at %s: %v", want, err)
	}
	if runtime.GOOS != "windows" {
		folder, err := os.Stat(filepath.Dir(want))
		if err != nil {
			t.Fatalf("stat folder: %v", err)
		}
		if folder.Mode().Perm() != 0o700 || info.Mode().Perm() != 0o600 {
			t.Fatalf("folder mode %04o and database mode %04o, want 0700 and 0600", folder.Mode().Perm(), info.Mode().Perm())
		}
	}
	if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
		t.Fatalf("the current folder holds %d entries, want none", len(entries))
	}
}

// TestLegacyDatabaseNotice checks Q-003's migration: a cryptare.db in the current folder
// gets a notice on stderr saying how to keep using it, and is never opened or moved.
// "keys path" prints the path and creates nothing.
func TestLegacyDatabaseNotice(t *testing.T) {
	want := useDataFolder(t)
	dir := t.TempDir()
	t.Chdir(dir)
	legacy := []byte("not a database; must not be opened")
	if err := os.WriteFile(databaseFileName, legacy, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	keysPath := func() (stdout, stderr string) {
		t.Helper()
		cmd := newRootCmd(func() (*Database, error) {
			t.Fatal("keys path opened the key database")
			return nil, nil
		}, testVersion)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{"keys", "path"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("keys path: %v", err)
		}
		return out.String(), errOut.String()
	}
	move := "mv "
	if runtime.GOOS == "windows" {
		move = "move "
	}

	out, notice := keysPath()
	if out != want+"\n" {
		t.Fatalf("keys path printed %q, want %q", out, want+"\n")
	}
	for _, s := range []string{"notice:", move + databaseFileName + " " + quotePath(want), databasePathEnv} {
		if !strings.Contains(notice, s) {
			t.Fatalf("notice %q doesn't contain %q", notice, s)
		}
	}
	if _, err := os.Stat(filepath.Dir(want)); !os.IsNotExist(err) {
		t.Fatalf("keys path created the data folder (stat err: %v)", err)
	}

	// The keys commands open the new store and leave the old file as it was.
	var opened bytes.Buffer
	db, err := databaseOpener(&opened)()
	closeDatabase(t, db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.Contains(opened.String(), "notice:") {
		t.Fatalf("opening wrote %q, want the notice", opened.String())
	}
	if got, err := os.ReadFile(databaseFileName); err != nil || !bytes.Equal(got, legacy) {
		t.Fatalf("the old file changed: %q, %v", got, err)
	}

	// Once the new store exists, moving would overwrite it, so only CRYPTARE_DB_PATH is
	// suggested.
	if _, notice = keysPath(); !strings.Contains(notice, databasePathEnv) || strings.Contains(notice, move) {
		t.Fatalf("notice with both databases = %q", notice)
	}

	// No notice with CRYPTARE_DB_PATH set, or in the data folder itself.
	configured := filepath.Join(dir, databaseFileName)
	t.Setenv(databasePathEnv, configured)
	if out, notice = keysPath(); out != configured+"\n" || notice != "" {
		t.Fatalf("with %s: keys path printed %q and %q", databasePathEnv, out, notice)
	}
	t.Setenv(databasePathEnv, "")
	t.Chdir(filepath.Dir(want))
	if _, notice = keysPath(); notice != "" {
		t.Fatalf("in the data folder: notice %q", notice)
	}
}

func TestQuotePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		if got := quotePath(`C:\Users\a b\cryptare.db`); got != `"C:\Users\a b\cryptare.db"` {
			t.Fatalf("quotePath() = %s", got)
		}
		return
	}
	if got, want := quotePath("/Users/a/Library/Application Support/it's/cryptare.db"),
		`'/Users/a/Library/Application Support/it'\''s/cryptare.db'`; got != want {
		t.Fatalf("quotePath() = %s, want %s", got, want)
	}
}
