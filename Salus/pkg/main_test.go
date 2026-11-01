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
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// testVersion is the version the tests pass to Main and run.
const testVersion = "v0.0.0-test"

// isolateHostEnv hides the host's kubeconfig, Docker endpoint, PATH, and
// sshd_config from the misconfig check, so it passes regardless of the
// machine running the tests. With PATH empty, docker-status reports WARN.
func isolateHostEnv(t *testing.T) {
	t.Helper()

	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing-kubeconfig"))
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "missing-docker.sock"))
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv(SSHDConfigEnv, filepath.Join(t.TempDir(), "missing-sshd_config"))
}

// useTempDatabase points SALUS_DB_PATH at an owner-only file in a temp dir so
// the misconfig check passes regardless of the process umask.
func useTempDatabase(t *testing.T) {
	t.Helper()

	isolateHostEnv(t)

	path := filepath.Join(t.TempDir(), "salus.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create database file: %v", err)
	}
	t.Setenv(DatabasePathEnv, path)
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", t.TempDir())
	}
}

// mainOutput calls Main with os.Stdout and os.Stderr pointed at files in a
// temp dir, and returns the exit code and what Main wrote to each stream.
// Tests that use it must not call t.Parallel.
func mainOutput(t *testing.T, ctx context.Context, version string, args []string) (code int, stdout, stderr string) {
	t.Helper()

	dir := t.TempDir()
	outFile, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatalf("create stdout file: %v", err)
	}
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatalf("create stderr file: %v", err)
	}

	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outFile, errFile
	func() {
		defer func() { os.Stdout, os.Stderr = savedOut, savedErr }()
		code = Main(ctx, version, args)
	}()

	// Closed before reading, and before t.TempDir is removed: Windows cannot
	// remove an open file.
	for _, f := range []*os.File{outFile, errFile} {
		if err := f.Close(); err != nil {
			t.Fatalf("close %s: %v", f.Name(), err)
		}
	}
	out, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	errOut, err := os.ReadFile(errFile.Name())
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return code, string(out), string(errOut)
}

func TestRunExitCodes(t *testing.T) {
	// The checks report this missing file as FAIL; it is not a usage error.
	missingCert := filepath.Join(t.TempDir(), "missing.pem")

	tests := []struct {
		name       string
		args       []string
		want       int
		wantStdout string
		wantStderr string
	}{
		{name: "version", args: []string{"--version"}, want: ExitCodePass, wantStdout: "salus version " + testVersion + "\n"},
		{name: "passing check run", args: []string{"check", "run", "--only", "misconfig"}, want: ExitCodePass, wantStdout: "[PASS] misconfig"},
		{name: "warning check run", args: []string{"check", "run", "--only", "docker-status", "--no-save"}, want: ExitCodeWarn, wantStdout: "[WARN] docker-status"},
		{name: "failing check run", args: []string{"check", "run", "--only", "cert-expiry", "--cert", missingCert, "--no-save"}, want: ExitCodeFail, wantStdout: "[FAIL] cert-expiry"},
		{name: "group help", args: []string{"check"}, want: ExitCodePass, wantStdout: "Available Commands:"},
		{name: "unknown check", args: []string{"check", "run", "--only", "does-not-exist", "--json"}, want: ExitCodeError, wantStderr: `unknown check "does-not-exist"`},
		{name: "invalid threshold", args: []string{"check", "run", "--disk-warn", "95", "--json"}, want: ExitCodeError, wantStderr: "--disk-warn (95) must be less than --disk-fail (90)"},
		{name: "unknown flag", args: []string{"--no-such-flag"}, want: ExitCodeError, wantStderr: "unknown flag"},
		{name: "unknown subcommand flag", args: []string{"check", "run", "--no-such-flag"}, want: ExitCodeError, wantStderr: "Run 'salus check run --help' for usage."},
		{name: "unknown command", args: []string{"chek"}, want: ExitCodeError, wantStderr: `unknown command "chek" for "salus"`},
		{name: "unknown subcommand", args: []string{"check", "rn"}, want: ExitCodeError, wantStderr: `unknown command "rn" for "salus check"; did you mean run?`},
		{name: "extra argument", args: []string{"check", "run", "extra"}, want: ExitCodeError, wantStderr: `unknown command "extra" for "salus check run"`},
		{name: "invalid job id", args: []string{"jobs", "show", "abc"}, want: ExitCodeError, wantStderr: `invalid job id "abc"`},
		{name: "missing job", args: []string{"jobs", "show", "999"}, want: ExitCodeError, wantStderr: "scan job 999: record not found"},
		{name: "nagios format", args: []string{"check", "run", "--only", "misconfig", "--format", "nagios"}, want: ExitCodePass, wantStdout: "SALUS OK - 1 checks: 1 pass, 0 warn, 0 fail | 'misconfig'=0\n"},
		{name: "invalid format", args: []string{"check", "run", "--format", "yaml"}, want: ExitCodeError, wantStderr: `invalid --format value "yaml"`},
		{name: "invalid fail-on", args: []string{"check", "run", "--fail-on", "never"}, want: ExitCodeError, wantStderr: `invalid --fail-on value "never"`},
		{name: "cert-expiry without a file", args: []string{"check", "run", "--only", "cert-expiry"}, want: ExitCodeError, wantStderr: "cert-expiry needs at least one --cert file"},
		{name: "jobs diff without runs", args: []string{"jobs", "diff"}, want: ExitCodeError, wantStderr: "jobs diff needs two recorded runs; found 0"},
		{name: "jobs stats", args: []string{"jobs", "stats"}, want: ExitCodePass, wantStdout: "Runs since "},
		{name: "invalid stats age", args: []string{"jobs", "stats", "--since", "later"}, want: ExitCodeError, wantStderr: `invalid --since value "later"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTempDatabase(t)
			var stdout, stderr bytes.Buffer

			if got := run(t.Context(), testVersion, tt.args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run(%q) = %d, want %d\nstdout: %s\nstderr: %s", tt.args, got, tt.want, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
			if tt.want == ExitCodeError {
				// Errors must never reach stdout, where they would corrupt --json output.
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q, want nothing on error", stdout.String())
				}
				if n := strings.Count(stderr.String(), "Error:"); n != 1 {
					t.Errorf("stderr reports %d errors, want exactly 1:\n%s", n, stderr.String())
				}
			} else if stderr.Len() != 0 {
				// A WARN or FAIL result is not an error and prints none.
				t.Errorf("stderr = %q, want nothing", stderr.String())
			}
		})
	}
}

func TestMainWritesToStandardStreams(t *testing.T) {
	useTempDatabase(t)

	code, stdout, stderr := mainOutput(t, t.Context(), "v9.8.7-test", []string{"--version"})
	if code != ExitCodePass || stdout != "salus version v9.8.7-test\n" || stderr != "" {
		t.Errorf("Main(--version) = %d, stdout %q, stderr %q; want 0 and only the version line on stdout", code, stdout, stderr)
	}

	code, stdout, stderr = mainOutput(t, t.Context(), "v9.8.7-test", []string{"chek"})
	if code != ExitCodeError {
		t.Errorf("Main(chek) = %d, want %d", code, ExitCodeError)
	}
	if stdout != "" || !strings.HasPrefix(stderr, "Error: unknown command \"chek\" for \"salus\"\n") ||
		!strings.HasSuffix(stderr, "Run 'salus --help' for usage.\n") || strings.Count(stderr, "Error:") != 1 {
		t.Errorf("Main(chek) stdout %q, stderr %q; want nothing on stdout and one error on stderr", stdout, stderr)
	}
}

func TestMainTakesArgumentsOnlyFromItsParameter(t *testing.T) {
	useTempDatabase(t)
	// Cobra executes os.Args[1:] when given nil arguments. A test binary's
	// own -test.* flags would not show that (pflag skips them), so os.Args
	// holds a command whose output differs from Main without arguments.
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{"salus", "--version"}

	for _, args := range [][]string{nil, {}} {
		code, stdout, stderr := mainOutput(t, t.Context(), testVersion, args)
		if code != ExitCodePass || stderr != "" {
			t.Errorf("Main(%#v) = %d, stderr %q; want 0 and no error", args, code, stderr)
		}
		if strings.Contains(stdout, "salus version") || !strings.Contains(stdout, "Available Commands:") {
			t.Errorf("Main(%#v) stdout = %q, want the root help, not the command in os.Args", args, stdout)
		}
	}
}

func TestMainCalledTwiceKeepsNoStateBetweenCalls(t *testing.T) {
	useTempDatabase(t)

	// Flags of the first call must not carry over: the second call would
	// then run docker-status again, in Nagios format, without saving.
	code, stdout, stderr := mainOutput(t, t.Context(), "v1.0.0-first", []string{"check", "run", "--only", "docker-status", "--format", "nagios", "--no-save"})
	if code != ExitCodeWarn || !strings.HasPrefix(stdout, "SALUS WARNING - 1 checks:") {
		t.Fatalf("first Main = %d, stdout %q, stderr %q; want 1 and a Nagios WARNING line", code, stdout, stderr)
	}

	code, stdout, stderr = mainOutput(t, t.Context(), "v2.0.0-second", []string{"check", "run", "--only", "misconfig"})
	if code != ExitCodePass || !strings.Contains(stdout, "\n[PASS] misconfig") || strings.Contains(stdout, "docker") || strings.Contains(stdout, "SALUS") {
		t.Fatalf("second Main = %d, stdout %q, stderr %q; want 0 and only the misconfig result as text", code, stdout, stderr)
	}

	code, stdout, _ = mainOutput(t, t.Context(), "v3.0.0-third", []string{"--version"})
	if code != ExitCodePass || stdout != "salus version v3.0.0-third\n" {
		t.Errorf("third Main(--version) = %d, stdout %q; want this call's version", code, stdout)
	}

	// The second call closed the database; this call opens it again.
	code, stdout, stderr = mainOutput(t, t.Context(), "v4.0.0-fourth", []string{"jobs", "show", "1"})
	if code != ExitCodePass || !strings.Contains(stdout, "[PASS] misconfig") {
		t.Errorf("fourth Main(jobs show 1) = %d, stdout %q, stderr %q; want the run saved by the second call", code, stdout, stderr)
	}
}

func TestMainPassesContextToCommands(t *testing.T) {
	useTempDatabase(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("stopped by the caller"))

	code, stdout, stderr := mainOutput(t, ctx, testVersion, []string{"check", "run", "--only", "misconfig", "--no-save"})
	if code != ExitCodeError {
		t.Errorf("Main with a cancelled context = %d, want %d", code, ExitCodeError)
	}
	if want := "interrupted (stopped by the caller)"; !strings.Contains(stderr, want) || stdout != "" {
		t.Errorf("stdout %q, stderr %q; want no report and an error containing %q", stdout, stderr, want)
	}
}

func TestRunPersistsJobsAcrossInvocations(t *testing.T) {
	useTempDatabase(t)

	var stdout, stderr bytes.Buffer
	if got := run(t.Context(), testVersion, []string{"check", "run", "--only", "misconfig", "--quiet"}, &stdout, &stderr); got != ExitCodePass {
		t.Fatalf("check run exit code = %d, want 0 (stderr: %s)", got, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("--quiet wrote %q", stdout.String())
	}

	stdout.Reset()
	if got := run(t.Context(), testVersion, []string{"jobs", "show", "1"}, &stdout, &stderr); got != ExitCodePass {
		t.Fatalf("jobs show exit code = %d, want 0 (stderr: %s)", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[PASS] misconfig") {
		t.Errorf("jobs show output = %q, want the recorded misconfig result", stdout.String())
	}
}

func TestRunOpensDatabaseOnlyWhenNeeded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never", "salus.db")
	t.Setenv(DatabasePathEnv, path)
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", t.TempDir())
	}
	isolateHostEnv(t)

	for _, args := range [][]string{
		{"--version"},
		{"--help"},
		{"check"},
		{"completion", "bash"},
		{"check", "run", "--only", "misconfig", "--no-save", "--quiet"},
		{"check", "run", "--only", "misconfig", "--no-save", "--format", "junit"},
		{"jobs", "stats", "--since", "never"},
		{"check", "run", "--retain", "30d", "--no-save"},
	} {
		wantCode := ExitCodePass
		if slices.Contains(args, "stats") || slices.Contains(args, "--retain") {
			wantCode = ExitCodeError // rejected before the database opens
		}
		var stdout, stderr bytes.Buffer
		if got := run(t.Context(), testVersion, args, &stdout, &stderr); got != wantCode {
			t.Fatalf("run(%q) = %d, want %d (stderr: %s)", args, got, wantCode, stderr.String())
		}
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatalf("run(%q) created %s (stat error %v), want no database", args, filepath.Dir(path), err)
		}
	}
}

func TestRunUsesPerUserDefaultDatabase(t *testing.T) {
	t.Setenv(DatabasePathEnv, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	if got := run(t.Context(), testVersion, []string{"check", "list"}, &stdout, &stderr); got != ExitCodePass {
		t.Fatalf("check list exit code = %d, want 0 (stderr: %s)", got, stderr.String())
	}

	want, err := DefaultDatabasePath()
	if err != nil {
		t.Fatalf("DefaultDatabasePath() error = %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("database not created at the per-user default %s: %v", want, err)
	}
	if _, err := os.Stat("salus.db"); !os.IsNotExist(err) {
		t.Errorf("salus.db created in the working directory (stat error %v)", err)
	}
}

func TestRunDatabaseInitFailureIsOperationalError(t *testing.T) {
	// A directory cannot be opened as a sqlite database file.
	t.Setenv(DatabasePathEnv, t.TempDir())

	var stdout, stderr bytes.Buffer
	if got := run(t.Context(), testVersion, []string{"check", "list"}, &stdout, &stderr); got != ExitCodeError {
		t.Fatalf("run() = %d, want %d (stderr: %s)", got, ExitCodeError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "failed to initialize database") {
		t.Errorf("stderr = %q, want a database initialization error", stderr.String())
	}
}

func TestRunJobsDiffExitCode(t *testing.T) {
	useTempDatabase(t)
	var stdout, stderr bytes.Buffer
	for i := 0; i < 2; i++ {
		if got := run(t.Context(), testVersion, []string{"check", "run", "--only", "misconfig", "--quiet"}, &stdout, &stderr); got != ExitCodePass {
			t.Fatalf("check run exit code = %d (stderr: %s)", got, stderr.String())
		}
	}
	if got := run(t.Context(), testVersion, []string{"jobs", "diff", "--exit-code"}, &stdout, &stderr); got != ExitCodePass {
		t.Fatalf("jobs diff --exit-code without changes = %d, want 0 (stderr: %s)", got, stderr.String())
	}

	// A run with another check is a change; --exit-code reports it as 1 and
	// prints no error.
	// The disk-space status depends on the host; only its presence matters.
	if got := run(t.Context(), testVersion, []string{"check", "run", "--only", "misconfig,disk-space", "--disk-path", t.TempDir(), "--quiet"}, &stdout, &stderr); got == ExitCodeError {
		t.Fatalf("check run exit code = %d (stderr: %s)", got, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if got := run(t.Context(), testVersion, []string{"jobs", "diff", "--exit-code"}, &stdout, &stderr); got != 1 {
		t.Fatalf("jobs diff --exit-code with changes = %d, want 1 (stderr: %s)", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[added ") || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q; want the added result and no error", stdout.String(), stderr.String())
	}
}

func TestRunReportsPanicAsOperationalError(t *testing.T) {
	panicking := func(DatabaseOpener) *cobra.Command {
		return &cobra.Command{
			Use:  "salus",
			RunE: func(*cobra.Command, []string) error { panic("boom") },
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runWith(t.Context(), panicking, nil, &stdout, &stderr); code != ExitCodeError {
		t.Errorf("runWith() exit code = %d, want %d (not 2, which means FAIL)", code, ExitCodeError)
	}
	if !strings.HasPrefix(stderr.String(), "Error: internal error: boom\n") {
		t.Errorf("stderr = %q, want the panic reported as an internal error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}

// closeFailingPool is a GORM connection pool whose sql.DB cannot be reached,
// so Database.Close fails. Its other methods are never called.
type closeFailingPool struct{ gorm.ConnPool }

func (closeFailingPool) GetDBConn() (*sql.DB, error) { return nil, errors.New("close failed") }

func TestRunWithReportsDatabaseCloseError(t *testing.T) {
	tests := []struct {
		name string
		// finish ends the command after the database was opened.
		finish     func() error
		want       int
		wantStderr []string
	}{
		{name: "passing command", finish: func() error { return nil }, want: ExitCodeError, wantStderr: []string{"Error: get sql.DB: close failed\n"}},
		// A WARN or FAIL result keeps its code; the close error is still reported.
		{name: "warning result", finish: func() error { return &ExitStatusError{Code: ExitCodeWarn} }, want: ExitCodeWarn, wantStderr: []string{"Error: get sql.DB: close failed\n"}},
		// The database closes after a panic, too.
		{name: "panic", finish: func() error { panic("boom") }, want: ExitCodeError, wantStderr: []string{"Error: internal error: boom\n", "Error: get sql.DB: close failed\n"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTempDatabase(t)
			build := func(openDB DatabaseOpener) *cobra.Command {
				return &cobra.Command{
					Use:  "salus",
					Args: cobra.NoArgs,
					// As in NewRootCmd: runWith reports errors, not Cobra.
					SilenceErrors: true,
					SilenceUsage:  true,
					RunE: func(*cobra.Command, []string) error {
						db, err := openDB()
						if err != nil {
							return err
						}
						if again, err := openDB(); err != nil || again != db {
							t.Errorf("second openDB() = %p, %v; want the database opened first (%p)", again, err, db)
						}
						// Release the real file, then make runWith's Close fail.
						if err := db.Close(); err != nil {
							return err
						}
						db.conn = &gorm.DB{Config: &gorm.Config{ConnPool: closeFailingPool{}}}
						return tt.finish()
					},
				}
			}

			var stdout, stderr bytes.Buffer
			if code := runWith(t.Context(), build, []string{}, &stdout, &stderr); code != tt.want {
				t.Errorf("runWith() exit code = %d, want %d (stderr: %s)", code, tt.want, stderr.String())
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
				}
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
		})
	}
}
