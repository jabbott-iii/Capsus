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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests call Main as a program that embeds cryptare does: in the test process,
// several times, with the arguments as a parameter. Main writes to the process's
// standard output and error, as the standalone binary does, so captureOutput redirects
// them while it runs.

// captureOutput runs f with os.Stdout and os.Stderr redirected to files, and returns
// what f wrote to each.
func captureOutput(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper()
	dir := t.TempDir()
	outFile, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatalf("create stdout file: %v", err)
	}
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		_ = outFile.Close()
		t.Fatalf("create stderr file: %v", err)
	}

	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outFile, errFile
	func() {
		defer func() { os.Stdout, os.Stderr = savedOut, savedErr }()
		f()
	}()

	var written [2]string
	for i, file := range []*os.File{outFile, errFile} {
		if err := file.Close(); err != nil {
			t.Fatalf("close %s: %v", file.Name(), err)
		}
		data, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatalf("read %s: %v", file.Name(), err)
		}
		written[i] = string(data)
	}
	return written[0], written[1]
}

// runMain calls Main with a background context and args, and returns its exit status
// and what it wrote to stdout and stderr.
func runMain(t *testing.T, version string, args []string) (code int, stdout, stderr string) {
	t.Helper()
	stdout, stderr = captureOutput(t, func() { code = Main(context.Background(), version, args) })
	return code, stdout, stderr
}

// writeTestFile writes data to name in dir and returns its path.
func writeTestFile(t *testing.T, dir, name, data string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestMainReportsVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "keys.db")
	t.Setenv(databasePathEnv, dbPath)
	const want = "cryptare version v9.9.9-test\n"

	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			code, stdout, stderr := runMain(t, "v9.9.9-test", []string{flag})
			if code != 0 || stdout != want || stderr != "" {
				t.Fatalf("Main(%s) = %d, stdout %q, stderr %q; want 0, %q, nothing", flag, code, stdout, stderr, want)
			}
		})
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("--version created the key database (stat err: %v)", err)
	}
}

// TestMainExitCodes checks Main's exit status: 0 on success, and 1 for a command-line
// mistake or a failed command, which report on stderr only. Status 130 after Ctrl+C
// needs a signal, so TestDecryptInterruptedBySignal checks it in a subprocess, through
// Run and Main.
func TestMainExitCodes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(databasePathEnv, filepath.Join(dir, "keys.db"))
	src := writeTestFile(t, dir, "data.txt", "payload")
	pwFile := writePasswordFile(t, "correct horse battery staple")
	missing := filepath.Join(dir, "missing.enc")

	tests := []struct {
		name       string
		args       []string
		want       int
		wantStdout string // contained in stdout; empty means stdout must be empty
		wantStderr string // contained in stderr; empty means stderr must be empty
	}{
		{"success", []string{"compress", src}, 0, "Compressed: " + src, ""},
		{"unknown command", []string{"no-such-command"}, 1, "", `unknown command "no-such-command"`},
		{"unknown flag", []string{"compress", src, "--no-such-flag"}, 1, "", "unknown flag: --no-such-flag"},
		{"failed command", []string{"decrypt", missing, "--password-file", pwFile}, 1, "", missing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runMain(t, testVersion, tc.args)
			if code != tc.want {
				t.Fatalf("Main(%q) = %d, want %d; stderr: %s", tc.args, code, tc.want, stderr)
			}
			for _, out := range []struct{ name, got, want string }{
				{"stdout", stdout, tc.wantStdout},
				{"stderr", stderr, tc.wantStderr},
			} {
				if (out.want == "" && out.got != "") || !strings.Contains(out.got, out.want) {
					t.Errorf("Main(%q) %s = %q, want it to contain %q (empty: nothing)", tc.args, out.name, out.got, out.want)
				}
			}
		})
	}
}

// TestMainTakesArgumentsFromParameter checks that Main reads its arguments only from
// args, never from os.Args, even when args is nil (Cobra uses os.Args[1:] for nil
// arguments). The key database path is one cryptare refuses before creating anything,
// so no arguments, which open the TUI, stop at opening the key database instead.
func TestMainTakesArgumentsFromParameter(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	t.Setenv(databasePathEnv, filepath.Join(t.TempDir(), "unsupported?.db"))

	os.Args = []string{"cryptare", "--version"} // would exit 0 if Main read it
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"nil", nil},
		{"empty", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runMain(t, "v9.9.9-test", tc.args)
			if code != 1 || stdout != "" || !strings.Contains(stderr, "open key database: unsupported key database path") {
				t.Fatalf("Main(%#v) = %d, stdout %q, stderr %q; want 1 and the TUI's key database error", tc.args, code, stdout, stderr)
			}
		})
	}

	os.Args = []string{"cryptare", "no-such-command"} // would exit 1 if Main read it
	if code, stdout, stderr := runMain(t, "v9.9.9-test", []string{"--version"}); code != 0 || stdout != "cryptare version v9.9.9-test\n" {
		t.Fatalf("Main(--version) = %d, stdout %q, stderr %q; want 0 and the version", code, stdout, stderr)
	}
}

// TestMainRunsTwiceInOneProcess calls Main several times in one process, as a program
// that embeds cryptare does, and checks that nothing carries over from one call to the
// next: each call opens the key database CRYPTARE_DB_PATH names at that time, and starts
// from the default flag values.
func TestMainRunsTwiceInOneProcess(t *testing.T) {
	dir := t.TempDir()
	pwFile := writePasswordFile(t, "correct horse battery staple")
	src := writeTestFile(t, dir, "data.txt", "payload")

	first, second := filepath.Join(dir, "first.db"), filepath.Join(dir, "second.db")
	for _, dbPath := range []string{first, second} {
		t.Setenv(databasePathEnv, dbPath)
		code, stdout, stderr := runMain(t, testVersion, []string{"keys", "generate", "--password-file", pwFile})
		if code != 0 || !strings.HasPrefix(stdout, "Generated key: ") {
			t.Fatalf("keys generate in %s = %d, stdout %q, stderr %q", filepath.Base(dbPath), code, stdout, stderr)
		}
	}
	for _, dbPath := range []string{first, second} {
		db, err := NewDatabase(dbPath)
		if err != nil {
			t.Fatalf("open %s: %v", filepath.Base(dbPath), err)
		}
		keys, err := db.ListKeys()
		closeTestDatabase(t, db)
		if err != nil || len(keys) != 1 {
			t.Fatalf("%s holds %d keys (err %v), want 1", filepath.Base(dbPath), len(keys), err)
		}
	}

	// --format zip and --force apply only to the call that has them.
	steps := []struct {
		args       []string
		want       int
		wantStdout string // contained in stdout
	}{
		{[]string{"compress", src, "--format", "zip"}, 0, "→ " + src + ".zip"},
		{[]string{"compress", src}, 0, "→ " + src + ".gz"},
		{[]string{"compress", src, "--force"}, 0, "→ " + src + ".gz"},
		{[]string{"compress", src}, 1, ""},
	}
	for i, step := range steps {
		code, stdout, stderr := runMain(t, testVersion, step.args)
		if code != step.want || !strings.Contains(stdout, step.wantStdout) {
			t.Fatalf("call %d, Main(%q) = %d, stdout %q, stderr %q; want %d, stdout containing %q",
				i+1, step.args, code, stdout, stderr, step.want, step.wantStdout)
		}
	}
}

// TestMainClosesKeyDatabase checks that Main closes the key database a command opened
// before it returns, so calls in one process don't leave database handles open, and
// that a command that doesn't use the key database doesn't open it.
func TestMainClosesKeyDatabase(t *testing.T) {
	t.Setenv(databasePathEnv, filepath.Join(t.TempDir(), "keys.db"))
	open := databaseOpener(io.Discard)

	for _, tc := range []struct {
		args  []string
		opens bool
	}{
		{[]string{"keys", "list"}, true},
		{[]string{"--version"}, false},
	} {
		var opened *Database
		var code int
		_, stderr := captureOutput(t, func() {
			code = mainWith(context.Background(), func() (*Database, error) {
				db, err := open()
				opened = db
				return db, err
			}, testVersion, tc.args)
		})
		if code != 0 {
			t.Fatalf("Main(%q) = %d, want 0; stderr: %s", tc.args, code, stderr)
		}
		if !tc.opens {
			if opened != nil {
				t.Fatalf("Main(%q) opened the key database", tc.args)
			}
			continue
		}
		if opened == nil {
			t.Fatalf("Main(%q) didn't open the key database", tc.args)
		}
		sqlDB, err := opened.Conn().DB()
		if err != nil {
			t.Fatalf("get the SQL handle: %v", err)
		}
		if err := sqlDB.Ping(); err == nil {
			_ = sqlDB.Close()
			t.Fatalf("Main(%q) left the key database open", tc.args)
		}
	}
}

// TestMainPassesContextToCommands checks that Main's context reaches the commands: a
// file command with a cancelled context stops, leaves nothing behind and exits 1.
func TestMainPassesContextToCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(databasePathEnv, filepath.Join(dir, "keys.db"))
	src := writeTestFile(t, dir, "data.txt", "payload")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var code int
	stdout, stderr := captureOutput(t, func() { code = Main(ctx, testVersion, []string{"compress", src}) })
	if code != 1 || stdout != "" || !strings.Contains(stderr, context.Canceled.Error()) {
		t.Fatalf("Main(compress) with a cancelled context = %d, stdout %q, stderr %q; want 1 and %q",
			code, stdout, stderr, context.Canceled)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 1 || entries[0].Name() != "data.txt" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("folder holds %v after a cancelled compress, want only data.txt", names)
	}
}
