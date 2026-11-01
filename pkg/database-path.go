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
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// DatabasePathEnv is the environment variable that overrides the database path.
	DatabasePathEnv = "SALUS_DB_PATH"

	databaseDirName  = "salus"
	databaseFileName = "salus.db"
)

// DatabasePath returns the database path: DatabasePathEnv when it is set,
// otherwise DefaultDatabasePath.
func DatabasePath() (string, error) {
	if path := os.Getenv(DatabasePathEnv); path != "" {
		return path, nil
	}
	return DefaultDatabasePath()
}

// DefaultDatabasePath returns the per-user database location:
//
//	Linux and other Unix: $XDG_DATA_HOME/salus/salus.db, or ~/.local/share/salus/salus.db
//	macOS:                ~/Library/Application Support/salus/salus.db
//	Windows:              %LOCALAPPDATA%\salus\salus.db
func DefaultDatabasePath() (string, error) {
	return defaultDatabasePath(runtime.GOOS, os.Getenv, userHomeDir)
}

// userHomeDir returns $HOME (or the platform equivalent), falling back to the
// account's home directory from the user database, for example for a systemd
// service that runs without HOME set.
func userHomeDir() (string, error) {
	if home, err := os.UserHomeDir(); err == nil {
		return home, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	if u.HomeDir == "" {
		return "", errors.New("home directory is unknown")
	}
	return u.HomeDir, nil
}

// databaseFile returns the file SQLite opens for path. Like go-sqlite3, it
// treats text after the first "?" of a plain path as connection parameters.
// ok is false for in-memory databases and "file:" URIs, whose files Salus
// does not manage.
func databaseFile(path string) (file string, ok bool) {
	if strings.HasPrefix(path, "file:") {
		return "", false
	}
	if i := strings.IndexByte(path, '?'); i >= 1 {
		path = path[:i]
	}
	if path == "" || path == ":memory:" {
		return "", false
	}
	return path, true
}

func defaultDatabasePath(goos string, getenv func(string) string, homeDir func() (string, error)) (string, error) {
	var base string
	switch goos {
	case "windows":
		base = getenv("LOCALAPPDATA")
		if base == "" || !filepath.IsAbs(base) {
			return "", errors.New("LOCALAPPDATA is not set to an absolute path; set " + DatabasePathEnv + " to choose a database path")
		}
	case "darwin":
		home, err := homeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory (set %s to choose a database path): %w", DatabasePathEnv, err)
		}
		base = filepath.Join(home, "Library", "Application Support")
	default:
		// The XDG Base Directory spec says relative values must be ignored.
		if xdg := getenv("XDG_DATA_HOME"); xdg != "" && filepath.IsAbs(xdg) {
			base = xdg
			break
		}
		home, err := homeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory (set %s to choose a database path): %w", DatabasePathEnv, err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, databaseDirName, databaseFileName), nil
}
