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

// TestFileCommandsDoNotCreateDatabase is a regression test for SEC-010 and BUG-005:
// the root command is built the way Run builds it, and only the keys commands open
// the key database. --help, --version and the file commands no longer create
// cryptare.db, and a database the keys commands create is private (0600).
func TestFileCommandsDoNotCreateDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cryptare.db")
	t.Setenv(databasePathEnv, dbPath)
	src := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	const password = "correct horse battery staple"

	// Close any database the keys commands open, before the temporary folder is
	// removed: Windows can't delete a file that is still open.
	var opened []*Database
	t.Cleanup(func() {
		for _, db := range opened {
			if sqlDB, err := db.Conn().DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})
	open := databaseOpener(io.Discard)
	run := func(args ...string) error {
		cmd := newRootCmd(func() (*Database, error) {
			db, err := open()
			if db != nil {
				opened = append(opened, db)
			}
			return db, err
		}, testVersion)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetIn(bytes.NewReader(nil))
		cmd.SetArgs(args)
		return cmd.Execute()
	}

	for _, args := range [][]string{
		{"--help"},
		{"--version"},
		{"encrypt", src, "--password", password},
		{"decrypt", src + ".enc", "--output", filepath.Join(dir, "plain.txt"), "--password", password},
		{"compress", src},
		{"decompress", src + ".gz", "--output", filepath.Join(dir, "unpacked.txt")},
	} {
		if err := run(args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
			t.Fatalf("%v created the key database (stat err: %v)", args, err)
		}
	}

	if err := run("keys", "list"); err != nil {
		t.Fatalf("keys list: %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("keys list did not create the key database: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("new key database mode = %04o, want 0600", info.Mode().Perm())
	}
}

// TestDatabaseOpenerRefusesUntrustedDatabase checks SEC-016 from the entry point: a key
// database that others can write is refused, and the error points to CRYPTARE_DB_PATH.
func TestDatabaseOpenerRefusesUntrustedDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply on Windows")
	}
	path := filepath.Join(t.TempDir(), "cryptare.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write database: %v", err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatalf("chmod database: %v", err)
	}
	t.Setenv(databasePathEnv, path)

	db, err := databaseOpener(io.Discard)()
	if db != nil {
		if sqlDB, err := db.Conn().DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	if !errors.Is(err, ErrUntrustedDatabase) {
		t.Fatalf("open err = %v, want ErrUntrustedDatabase", err)
	}
	if !strings.Contains(err.Error(), databasePathEnv) {
		t.Fatalf("error %q doesn't mention %s", err, databasePathEnv)
	}
}
