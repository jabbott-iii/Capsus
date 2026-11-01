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
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultDatabasePath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	xdg := filepath.Join(t.TempDir(), "xdg") // absolute on every host OS
	localAppData := filepath.Join(t.TempDir(), "AppData", "Local")

	tests := []struct {
		name    string
		goos    string
		env     map[string]string
		homeErr error
		want    string
		wantErr bool
	}{
		{name: "linux default", goos: "linux", want: filepath.Join(home, ".local", "share", "salus", "salus.db")},
		{name: "linux XDG_DATA_HOME", goos: "linux", env: map[string]string{"XDG_DATA_HOME": xdg}, want: filepath.Join(xdg, "salus", "salus.db")},
		{name: "linux relative XDG_DATA_HOME is ignored", goos: "linux", env: map[string]string{"XDG_DATA_HOME": "relative/data"}, want: filepath.Join(home, ".local", "share", "salus", "salus.db")},
		{name: "freebsd uses the XDG layout", goos: "freebsd", want: filepath.Join(home, ".local", "share", "salus", "salus.db")},
		{name: "macOS", goos: "darwin", env: map[string]string{"XDG_DATA_HOME": xdg}, want: filepath.Join(home, "Library", "Application Support", "salus", "salus.db")},
		{name: "windows", goos: "windows", env: map[string]string{"LOCALAPPDATA": localAppData}, want: filepath.Join(localAppData, "salus", "salus.db")},
		{name: "windows without LOCALAPPDATA", goos: "windows", wantErr: true},
		{name: "windows with relative LOCALAPPDATA", goos: "windows", env: map[string]string{"LOCALAPPDATA": `relative\dir`}, wantErr: true},
		{name: "linux without a home directory", goos: "linux", homeErr: errors.New("no home"), wantErr: true},
		{name: "macOS without a home directory", goos: "darwin", homeErr: errors.New("no home"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			homeDir := func() (string, error) { return home, tt.homeErr }

			got, err := defaultDatabasePath(tt.goos, getenv, homeDir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("defaultDatabasePath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("defaultDatabasePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDatabasePathPrefersEnvironment(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom.db")
	t.Setenv(DatabasePathEnv, want)

	got, err := DatabasePath()
	if err != nil || got != want {
		t.Fatalf("DatabasePath() = %q, %v; want %q", got, err, want)
	}
}

func TestDatabasePathFallsBackToDefault(t *testing.T) {
	isolateMisconfigEnv(t)

	want, err := DefaultDatabasePath()
	if err != nil {
		t.Fatalf("DefaultDatabasePath() error = %v", err)
	}
	if got, err := DatabasePath(); err != nil || got != want {
		t.Fatalf("DatabasePath() = %q, %v; want %q", got, err, want)
	}
}

func TestNewDatabaseCreatesOwnerOnlyFileAndDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "salus.db")

	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if runtime.GOOS == "windows" {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("database file not created: %v", err)
		}
		return // Windows has no POSIX mode bits to check.
	}
	assertMode(t, path, 0o600)
	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, filepath.Dir(filepath.Dir(path)), 0o700)
}

func TestNewDatabaseKeepsExistingFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX mode bits")
	}
	path := filepath.Join(t.TempDir(), "salus.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod file: %v", err)
	}

	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// Salus does not silently change an existing file; misconfig reports it.
	assertMode(t, path, 0o640)
}

func TestDefaultDatabasePathWithoutHOME(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses LOCALAPPDATA")
	}
	t.Setenv("XDG_DATA_HOME", "")
	unsetEnv(t, "HOME")

	// Falls back to the account's home directory from the user database.
	path, err := DefaultDatabasePath()
	if err != nil {
		t.Fatalf("DefaultDatabasePath() without HOME error = %v", err)
	}
	if filepath.Base(path) != "salus.db" || !filepath.IsAbs(path) {
		t.Errorf("DefaultDatabasePath() = %q, want an absolute path ending in salus.db", path)
	}
}

func TestDatabaseFile(t *testing.T) {
	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{path: "salus.db", want: "salus.db", wantOK: true},
		{path: "/data/salus.db?_busy_timeout=1000", want: "/data/salus.db", wantOK: true},
		{path: "?odd", want: "?odd", wantOK: true}, // go-sqlite3 splits only at index 1 or later
		{path: ":memory:", wantOK: false},
		{path: ":memory:?cache=shared", wantOK: false},
		{path: "file:salus.db?mode=ro", wantOK: false},
		{path: "", wantOK: false},
	}

	for _, tt := range tests {
		got, ok := databaseFile(tt.path)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("databaseFile(%q) = %q, %v; want %q, %v", tt.path, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestNewDatabaseStripsConnectionParameters(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "salus.db")

	db, err := NewDatabase(file + "?_busy_timeout=1000")
	if err != nil {
		t.Fatalf("NewDatabase() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "salus.db" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory contains %q, want only salus.db", names)
	}
	if runtime.GOOS != "windows" {
		assertMode(t, file, 0o600)
	}
}

func TestNewDatabaseOpensReadOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only files block t.TempDir cleanup on Windows")
	}
	path := filepath.Join(t.TempDir(), "salus.db")
	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	// An existing database must open without write access to the file, so
	// read-only commands such as jobs list keep working.
	db, err = NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase() on a read-only file error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestNewDatabaseRejectsEmptyPath(t *testing.T) {
	if _, err := NewDatabase(""); err == nil {
		t.Fatal("NewDatabase(\"\") error = nil, want an error")
	}
}

func TestNewDatabaseInMemoryCreatesNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewDatabase(:memory:) error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("working directory contains %d entries, want none", len(entries))
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %04o, want %04o", path, got, want)
	}
}
