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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCryptare runs cryptare in a subprocess (through TestRunMain) with its key database
// at dbPath, and returns what it wrote to stdout and to stderr.
func runCryptare(t *testing.T, dbPath string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestRunMain$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), helperEnv+"=1", databasePathEnv+"="+dbPath)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	return out.String(), errOut.String(), err
}

// TestKeysCommandsKeepStdoutClean is the regression test for SEC-017: a failed key
// lookup and a duplicate import write nothing to stdout, where GORM's logger used to
// print the SQL statement with its values (the encrypted key blob included), and the
// error names the key.
func TestKeysCommandsKeepStdoutClean(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "keys.db")
	pw := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(pw, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	if _, stderr, err := runCryptare(t, dbPath, "keys", "generate", "--password-file", pw); err != nil {
		t.Fatalf("keys generate: %v; stderr: %s", err, stderr)
	}
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("open key database: %v", err)
	}
	keys, err := db.ListKeys()
	if sqlDB, dbErr := db.Conn().DB(); dbErr == nil {
		_ = sqlDB.Close()
	}
	if err != nil || len(keys) != 1 {
		t.Fatalf("stored keys = %d (err %v), want 1", len(keys), err)
	}
	keyID, blob := keys[0].KeyID, keys[0].EncryptedBlob
	export := filepath.Join(dir, "key.ckey")
	if _, stderr, err := runCryptare(t, dbPath, "keys", "export", keyID, "--output", export, "--password-file", pw); err != nil {
		t.Fatalf("keys export: %v; stderr: %s", err, stderr)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string // in the error message
	}{
		{"export of an unknown key", []string{"keys", "export", "ffffffffffffffff", "--output", filepath.Join(dir, "unknown.ckey"), "--password-file", pw}, `"ffffffffffffffff"`},
		{"import of a stored key", []string{"keys", "import", export, "--password-file", pw}, `"` + keyID + `"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCryptare(t, dbPath, tc.args...)
			if err == nil {
				t.Fatal("the command succeeded; want an error")
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want nothing", stdout)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Fatalf("stderr %q doesn't name the key %s", stderr, tc.want)
			}
			if strings.Contains(stderr, blob) {
				t.Fatal("stderr contains the encrypted key blob")
			}
		})
	}
}
