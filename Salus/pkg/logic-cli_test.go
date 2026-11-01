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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// openerFor returns a DatabaseOpener that always yields db.
func openerFor(db *Database) DatabaseOpener {
	return func() (*Database, error) { return db, nil }
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestCheckListCmdReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	expectedErr := errors.New("write failed")

	cmd := newCheckListCmd(openerFor(db))
	cmd.SilenceUsage = true
	cmd.SetOut(failingWriter{err: expectedErr})
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
	}
}

func TestJobsListCmdReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	_, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass, Duration: time.Millisecond}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	expectedErr := errors.New("write failed")

	cmd := newJobsListCmd(openerFor(db))
	cmd.SilenceUsage = true
	cmd.SetOut(failingWriter{err: expectedErr})
	cmd.SetErr(io.Discard)

	err = cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
	}
}

func TestJobsShowCmdReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	job, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass, Duration: time.Millisecond}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	expectedErr := errors.New("write failed")

	cmd := newJobsShowCmd(openerFor(db))
	cmd.SilenceUsage = true
	cmd.SetOut(failingWriter{err: expectedErr})
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{strconv.Itoa(int(job.ID))})

	err = cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
	}
}

// executeCheckRun runs `check run` with args and returns stdout, stderr, and the error.
func executeCheckRun(t *testing.T, db *Database, args ...string) (string, string, error) {
	t.Helper()

	cmd := newCheckRunCmd(openerFor(db))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func countScanJobs(t *testing.T, db *Database) int {
	t.Helper()

	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	return len(jobs)
}

func TestCheckRunPassRecordsJob(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	stdout, _, err := executeCheckRun(t, db, "--only", keyMisconfig)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if !strings.Contains(stdout, "[PASS] misconfig") {
		t.Errorf("output = %q, want a PASS line for misconfig", stdout)
	}

	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("recorded %d jobs, want 1", len(jobs))
	}
	if jobs[0].FinishedAt == nil || jobs[0].FinishedAt.Before(jobs[0].StartedAt) {
		t.Errorf("job times = started %v, finished %v; want finished at or after started", jobs[0].StartedAt, jobs[0].FinishedAt)
	}
}

func TestCheckRunNoSaveSkipsPersistence(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	if _, _, err := executeCheckRun(t, db, "--only", keyMisconfig, "--no-save"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if n := countScanJobs(t, db); n != 0 {
		t.Errorf("recorded %d jobs with --no-save, want 0", n)
	}
}

func TestCheckRunJSONOutput(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	stdout, _, err := executeCheckRun(t, db, "--only", keyMisconfig, "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var outcomes []CheckOutcome
	if err := json.Unmarshal([]byte(stdout), &outcomes); err != nil {
		t.Fatalf("output is not a JSON outcome array: %v\n%s", err, stdout)
	}
	if len(outcomes) != 1 || outcomes[0].Key != keyMisconfig || outcomes[0].Status != StatusPass {
		t.Errorf("outcomes = %+v, want one PASS misconfig outcome", outcomes)
	}
}

func TestCheckRunQuietSuppressesAllOutput(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	for _, args := range [][]string{{"--quiet"}, {"--quiet", "--json"}} {
		stdout, stderr, err := executeCheckRun(t, db, append([]string{"--only", keyMisconfig}, args...)...)
		if err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		if stdout != "" || stderr != "" {
			t.Errorf("Execute(%v) wrote stdout %q, stderr %q; want no output", args, stdout, stderr)
		}
	}
}

func TestCheckRunFailOnlyHidesPassingChecks(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	stdout, _, err := executeCheckRun(t, db, "--only", keyMisconfig, "--fail-only")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(stdout, "[PASS]") {
		t.Errorf("--fail-only output contains a PASS line: %q", stdout)
	}
}

func TestCheckRunWarnReturnsExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the misconfig permission warning does not apply on Windows")
	}
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	dbFile := filepath.Join(t.TempDir(), "writable.db")
	if err := os.WriteFile(dbFile, nil, 0o600); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := os.Chmod(dbFile, 0o666); err != nil {
		t.Fatalf("chmod file: %v", err)
	}
	t.Setenv(DatabasePathEnv, dbFile)

	stdout, stderr, err := executeCheckRun(t, db, "--only", keyMisconfig)
	var status *ExitStatusError
	if !errors.As(err, &status) || status.Code != ExitCodeWarn {
		t.Fatalf("Execute() error = %v, want ExitStatusError with code %d", err, ExitCodeWarn)
	}
	if !strings.Contains(stdout, "[WARN] misconfig") {
		t.Errorf("output = %q, want a WARN line for misconfig", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing (no error or usage text for a WARN result)", stderr)
	}
	if n := countScanJobs(t, db); n != 1 {
		t.Errorf("recorded %d jobs, want 1", n)
	}
}

func TestCheckRunFailReturnsExitStatus(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skip("the disk-space check is a stub on this platform")
	}
	db := newSeededTestDatabase(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, _, err := executeCheckRun(t, db, "--only", keyDiskSpace, "--disk-path", missing)
	if got := ExitCode(err); got != ExitCodeFail {
		t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitCodeFail)
	}
}

func TestCheckRunUnknownCheckIsOperationalError(t *testing.T) {
	db := newSeededTestDatabase(t)

	_, _, err := executeCheckRun(t, db, "--only", "does-not-exist")
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown check error")
	}
	if got := ExitCode(err); got != ExitCodeError {
		t.Errorf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
	}
	if n := countScanJobs(t, db); n != 0 {
		t.Errorf("recorded %d jobs for a failed run, want 0", n)
	}
}

func TestRecordScanKeepsStartTime(t *testing.T) {
	db := newSeededTestDatabase(t)
	startedAt := time.Now().Add(-2 * time.Second)

	job, err := RecordScan(db, startedAt, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}

	// Read the job back so the check covers what SQLite actually stores.
	stored, _, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}
	if diff := stored.StartedAt.Sub(startedAt); diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("stored StartedAt = %v, want %v", stored.StartedAt, startedAt)
	}
	if stored.FinishedAt == nil || stored.FinishedAt.Sub(stored.StartedAt) < 2*time.Second {
		t.Errorf("stored FinishedAt = %v, want at least 2s after StartedAt %v", stored.FinishedAt, stored.StartedAt)
	}
}

func TestGroupCommandsRejectUnknownSubcommands(t *testing.T) {
	db := newSeededTestDatabase(t)

	for _, group := range []*cobra.Command{newCheckCmd(openerFor(db)), newJobsCmd(openerFor(db))} {
		t.Run(group.Name(), func(t *testing.T) {
			// Attach to a parent: a command executed as the root gets Cobra's
			// own subcommand handling instead of runGroup.
			root := &cobra.Command{Use: "salus", SilenceErrors: true, SilenceUsage: true}
			root.AddCommand(group)
			var stdout bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(io.Discard)

			root.SetArgs([]string{group.Name(), "lst"})
			err := root.Execute()
			want := `unknown command "lst" for "salus ` + group.Name() + `"; did you mean list?`
			if err == nil || err.Error() != want {
				t.Fatalf("Execute(%s lst) error = %v, want %q", group.Name(), err, want)
			}
			if got := ExitCode(err); got != ExitCodeError {
				t.Errorf("ExitCode = %d, want %d", got, ExitCodeError)
			}

			stdout.Reset()
			root.SetArgs([]string{group.Name()})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute(%s) error = %v, want help", group.Name(), err)
			}
			if !strings.Contains(stdout.String(), "Available Commands:") {
				t.Errorf("help output = %q, want the subcommand list", stdout.String())
			}
		})
	}
}

func TestCheckRunNoSaveNeverOpensDatabase(t *testing.T) {
	isolateMisconfigEnv(t)
	openDB := func() (*Database, error) {
		t.Error("check run --no-save opened the database")
		return nil, errors.New("unexpected open")
	}

	cmd := newCheckRunCmd(openDB)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--only", keyMisconfig, "--no-save"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestCommandsReportDatabaseOpenErrors(t *testing.T) {
	openErr := errors.New("disk on fire")
	openDB := func() (*Database, error) { return nil, openErr }

	commands := map[string]*cobra.Command{
		"check list": newCheckListCmd(openDB),
		"check run":  newCheckRunCmd(openDB),
		"jobs list":  newJobsListCmd(openDB),
		"jobs show":  newJobsShowCmd(openDB),
	}
	args := map[string][]string{"check run": {"--only", keyMisconfig}, "jobs show": {"1"}}

	isolateMisconfigEnv(t)
	for name, cmd := range commands {
		t.Run(name, func(t *testing.T) {
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append([]string{}, args[name]...))
			if err := cmd.Execute(); !errors.Is(err, openErr) {
				t.Fatalf("Execute() error = %v, want %v", err, openErr)
			}
		})
	}
}

func TestCheckRunValidatesChecksBeforeOpeningDatabase(t *testing.T) {
	openDB := func() (*Database, error) {
		t.Error("check run opened the database for an invalid --only value")
		return nil, errors.New("unexpected open")
	}

	cmd := newCheckRunCmd(openDB)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--only", "does-not-exist"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), `unknown check "does-not-exist"`) {
		t.Fatalf("Execute() error = %v, want unknown check", err)
	}
}

// neverOpen returns a DatabaseOpener that fails the test if it is called.
func neverOpen(t *testing.T) DatabaseOpener {
	t.Helper()
	return func() (*Database, error) {
		t.Error("check run opened the database")
		return nil, errors.New("unexpected open")
	}
}

func TestCheckRunPassesFlagValuesToChecks(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want CheckOptions
	}{
		{
			name: "defaults",
			want: CheckOptions{
				DiskPaths:       []string{"/"},
				DiskWarnPercent: 80, DiskFailPercent: 90,
				InodeWarnPercent: 80, InodeFailPercent: 90,
				MemWarnPercent: 80, MemFailPercent: 90,
				LoadWarnPercent: 80, LoadFailPercent: 100,
				CertWarnDays:   30,
				CommandTimeout: 3 * time.Second,
			},
		},
		{
			name: "overrides",
			args: []string{
				"--disk-path", "/data", "--service", "nginx", "--kube-context", "prod", "--kube-namespace", "web",
				"--disk-warn", "70", "--disk-fail", "85.5",
				"--inode-warn", "50", "--inode-fail", "75",
				"--mem-warn", "60", "--mem-fail", "95",
				"--load-warn", "150", "--load-fail", "300",
				"--cert", "/etc/ssl/a.pem", "--cert-warn-days", "14",
				"--timeout", "10s", "--check-timeout", "45s", "--run-timeout", "5m",
			},
			want: CheckOptions{
				DiskPaths: []string{"/data"}, ServiceNames: []string{"nginx"}, KubeContext: "prod", KubeNamespace: "web",
				DiskWarnPercent: 70, DiskFailPercent: 85.5,
				InodeWarnPercent: 50, InodeFailPercent: 75,
				MemWarnPercent: 60, MemFailPercent: 95,
				LoadWarnPercent: 150, LoadFailPercent: 300,
				CertPaths: []string{"/etc/ssl/a.pem"}, CertWarnDays: 14,
				CommandTimeout: 10 * time.Second, CheckTimeout: 45 * time.Second, RunTimeout: 5 * time.Minute,
			},
		},
		{
			name: "repeated targets",
			args: []string{
				"--disk-path", "/", "--disk-path", "/var,with,commas",
				"--service", "nginx,sshd", "--service", "cron",
				"--cert", "a.pem", "--cert", "b,c.pem",
			},
			want: CheckOptions{
				DiskPaths:       []string{"/", "/var,with,commas"},
				ServiceNames:    []string{"nginx", "sshd", "cron"},
				CertPaths:       []string{"a.pem", "b,c.pem"},
				DiskWarnPercent: 80, DiskFailPercent: 90,
				InodeWarnPercent: 80, InodeFailPercent: 90,
				MemWarnPercent: 80, MemFailPercent: 90,
				LoadWarnPercent: 80, LoadFailPercent: 100,
				CertWarnDays:   30,
				CommandTimeout: 3 * time.Second,
			},
		},
		{
			name: "percentages at the 100 limit",
			args: []string{"--disk-fail", "100", "--mem-fail", "100", "--inode-fail", "100"},
			want: CheckOptions{
				DiskPaths:       []string{"/"},
				DiskWarnPercent: 80, DiskFailPercent: 100,
				InodeWarnPercent: 80, InodeFailPercent: 100,
				MemWarnPercent: 80, MemFailPercent: 100,
				LoadWarnPercent: 80, LoadFailPercent: 100,
				CertWarnDays:   30,
				CommandTimeout: 3 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got CheckOptions
			runChecks := func(_ context.Context, keys []string, opts CheckOptions) ([]CheckOutcome, error) {
				got = opts
				return []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}}, nil
			}

			cmd := newCheckRunCmdWith(neverOpen(t), runChecks)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append([]string{"--no-save"}, tt.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			// The test seams are nil in both, and DeepEqual treats nil funcs as equal.
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("options passed to the checks = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCheckRunRejectsInvalidLimits(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"warn above default fail", []string{"--disk-warn", "95"}, "--disk-warn (95) must be less than --disk-fail (90)"},
		{"warn equals fail", []string{"--mem-warn", "90", "--mem-fail", "90"}, "--mem-warn (90) must be less than --mem-fail (90)"},
		{"load warn above fail", []string{"--load-warn", "200", "--load-fail", "150"}, "--load-warn (200) must be less than --load-fail (150)"},
		{"disk above 100", []string{"--disk-fail", "101"}, "invalid --disk-fail value 101: must be at most 100"},
		{"memory above 100", []string{"--mem-fail", "100.5"}, "invalid --mem-fail value 100.5: must be at most 100"},
		{"zero", []string{"--mem-warn", "0"}, "invalid --mem-warn value 0: must be a finite number greater than 0"},
		{"negative", []string{"--load-warn=-1"}, "invalid --load-warn value -1: must be a finite number greater than 0"},
		{"not a number", []string{"--load-fail", "NaN"}, "invalid --load-fail value NaN: must be a finite number greater than 0"},
		{"infinite", []string{"--load-fail", "+Inf"}, "invalid --load-fail value +Inf: must be a finite number greater than 0"},
		{"zero timeout", []string{"--timeout", "0s"}, "invalid --timeout value 0s: must be greater than 0"},
		{"negative timeout", []string{"--timeout=-1s"}, "invalid --timeout value -1s: must be greater than 0"},
		{"negative check timeout", []string{"--check-timeout=-1s"}, "invalid --check-timeout value -1s: must not be negative"},
		{"check timeout below command timeout", []string{"--check-timeout", "2s"}, "--check-timeout (2s) must not be less than --timeout (3s)"},
		{"check timeout below raised command timeout", []string{"--timeout", "1m", "--check-timeout", "30s"}, "--check-timeout (30s) must not be less than --timeout (1m0s)"},
		{"negative run timeout", []string{"--run-timeout=-1m"}, "invalid --run-timeout value -1m0s: must not be negative"},
		{"malformed value", []string{"--disk-warn", "high"}, `invalid argument "high" for "--disk-warn" flag`},
		{"inode warn above fail", []string{"--inode-warn", "95"}, "--inode-warn (95) must be less than --inode-fail (90)"},
		{"inode above 100", []string{"--inode-fail", "101"}, "invalid --inode-fail value 101: must be at most 100"},
		{"zero certificate warning days", []string{"--cert-warn-days", "0"}, "invalid --cert-warn-days value 0: must be greater than 0"},
		{"unknown format", []string{"--format", "xml"}, `invalid --format value "xml": use text, json, nagios, prometheus, junit`},
		{"json conflicts with format", []string{"--json", "--format", "nagios"}, "--json conflicts with --format nagios"},
		{"json conflicts with explicit text", []string{"--json", "--format", "text"}, "--json conflicts with --format text"},
		{"unknown fail-on", []string{"--fail-on", "never"}, `invalid --fail-on value "never": use warn or fail`},
		{"fail-on is case-sensitive", []string{"--fail-on", "WARN"}, `invalid --fail-on value "WARN": use warn or fail`},
		{"invalid retain", []string{"--retain", "soon"}, `invalid --retain value "soon"`},
		{"zero retain", []string{"--retain", "0d"}, `invalid --retain value "0d": must be greater than 0`},
		{"retain without saving", []string{"--retain", "30d", "--no-save"}, "--retain prunes saved runs, so it cannot be used with --no-save"},
		{"cert-expiry without a file", []string{"--only", "cert-expiry"}, "cert-expiry needs at least one --cert file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runChecks := func(context.Context, []string, CheckOptions) ([]CheckOutcome, error) {
				t.Error("checks ran despite an invalid flag value")
				return nil, nil
			}

			cmd := newCheckRunCmdWith(neverOpen(t), runChecks)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Execute(%q) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
			}
			if got := ExitCode(err); got != ExitCodeError {
				t.Errorf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
			}
		})
	}
}

// executeJobs runs cmd with args and returns stdout and the error.
func executeJobs(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

// jsonKeys returns the sorted keys of a JSON object.
func jsonKeys(t *testing.T, raw json.RawMessage) string {
	t.Helper()

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal object %s: %v", raw, err)
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func TestJobsListJSON(t *testing.T) {
	db := newSeededTestDatabase(t)

	stdout, err := executeJobs(t, newJobsListCmd(openerFor(db)), "--json")
	if err != nil || stdout != "[]\n" {
		t.Fatalf("jobs list --json on an empty database = %q, %v, want \"[]\\n\"", stdout, err)
	}

	for i := 0; i < 2; i++ {
		if _, err := RecordScan(db, time.Now(), []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}}); err != nil {
			t.Fatalf("RecordScan() error = %v", err)
		}
	}
	stdout, err = executeJobs(t, newJobsListCmd(openerFor(db)), "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, stdout)
	}
	if len(raw) != 2 {
		t.Fatalf("got %d jobs, want 2", len(raw))
	}
	if keys := jsonKeys(t, raw[0]); keys != "finished_at,id,started_at,status,summary" {
		t.Errorf("job keys = %s", keys)
	}
	var jobs []struct {
		ID         uint       `json:"id"`
		Status     string     `json:"status"`
		FinishedAt *time.Time `json:"finished_at"`
		Summary    string     `json:"summary"`
	}
	if err := json.Unmarshal([]byte(stdout), &jobs); err != nil {
		t.Fatalf("unmarshal jobs: %v", err)
	}
	if jobs[0].ID != 2 || jobs[1].ID != 1 {
		t.Errorf("job order = %d, %d, want newest first (2, 1)", jobs[0].ID, jobs[1].ID)
	}
	if jobs[0].Status != "completed" || jobs[0].FinishedAt == nil || jobs[0].Summary != "1 pass, 0 warn, 0 fail" {
		t.Errorf("job = %+v, want a completed job with a finish time and summary", jobs[0])
	}
}

func TestJobsShowJSON(t *testing.T) {
	db := newSeededTestDatabase(t)
	outcomes := []CheckOutcome{
		{Key: keyMisconfig, Status: StatusPass, Message: "ok", Duration: 1500 * time.Millisecond},
		{Key: keyDiskSpace, Status: StatusWarn, Message: "/: 85.0% used (15.0% free)", Duration: 2 * time.Millisecond},
	}
	job, err := RecordScan(db, time.Now(), outcomes)
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}

	stdout, err := executeJobs(t, newJobsShowCmd(openerFor(db)), strconv.Itoa(int(job.ID)), "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if keys := jsonKeys(t, json.RawMessage(stdout)); keys != "finished_at,id,results,started_at,status,summary" {
		t.Errorf("job keys = %s", keys)
	}

	var got struct {
		ID      uint              `json:"id"`
		Summary string            `json:"summary"`
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal job: %v\n%s", err, stdout)
	}
	if got.ID != job.ID || got.Summary != "1 pass, 1 warn, 0 fail" || len(got.Results) != 2 {
		t.Fatalf("job = %+v, want id %d with 2 results", got, job.ID)
	}
	// Results use the check run --json object shape.
	if keys := jsonKeys(t, got.Results[0]); keys != "duration_ns,key,message,status" {
		t.Errorf("result keys = %s", keys)
	}
	var first CheckOutcome
	if err := json.Unmarshal(got.Results[0], &first); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if first != outcomes[0] {
		t.Errorf("first result = %+v, want %+v", first, outcomes[0])
	}
}

func TestJobsJSONReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	job, err := RecordScan(db, time.Now(), []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	expectedErr := errors.New("write failed")

	for name, cmd := range map[string]*cobra.Command{
		"list": newJobsListCmd(openerFor(db)),
		"show": newJobsShowCmd(openerFor(db)),
	} {
		t.Run(name, func(t *testing.T) {
			args := []string{"--json"}
			if name == "show" {
				args = append(args, strconv.Itoa(int(job.ID)))
			}
			cmd.SetOut(failingWriter{err: expectedErr})
			cmd.SetErr(io.Discard)
			cmd.SetArgs(args)
			if err := cmd.Execute(); !errors.Is(err, expectedErr) {
				t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
			}
		})
	}
}

func TestJobsPrune(t *testing.T) {
	db := newSeededTestDatabase(t)
	for _, age := range []time.Duration{40 * 24 * time.Hour, time.Minute} {
		if _, err := RecordScan(db, time.Now().Add(-age), []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}}); err != nil {
			t.Fatalf("RecordScan() error = %v", err)
		}
	}

	stdout, err := executeJobs(t, newJobsPruneCmd(openerFor(db)), "--older-than", "30d", "--dry-run")
	if err != nil || !strings.HasPrefix(stdout, "Would delete 1 job started before ") {
		t.Fatalf("jobs prune --dry-run = %q, %v, want a count of 1", stdout, err)
	}
	if n := countScanJobs(t, db); n != 2 {
		t.Fatalf("--dry-run left %d jobs, want 2", n)
	}

	stdout, err = executeJobs(t, newJobsPruneCmd(openerFor(db)), "--older-than", "30d")
	if err != nil || !strings.HasPrefix(stdout, "Deleted 1 job started before ") {
		t.Fatalf("jobs prune = %q, %v, want 1 deleted", stdout, err)
	}
	if n := countScanJobs(t, db); n != 1 {
		t.Fatalf("%d jobs remain, want 1", n)
	}

	stdout, err = executeJobs(t, newJobsPruneCmd(openerFor(db)), "--older-than", "30d")
	if err != nil || !strings.HasPrefix(stdout, "Deleted 0 jobs started before ") {
		t.Errorf("second jobs prune = %q, %v, want 0 deleted", stdout, err)
	}
}

func TestJobsPruneRejectsInvalidAges(t *testing.T) {
	tests := []struct {
		args    []string
		wantErr string
	}{
		{nil, "--older-than is required"},
		{[]string{"--older-than", "30x"}, `invalid --older-than value "30x": use a whole number of days`},
		{[]string{"--older-than", "0d"}, `invalid --older-than value "0d": must be greater than 0`},
		{[]string{"--older-than=-2h"}, `invalid --older-than value "-2h": must be greater than 0`},
		{[]string{"--older-than", "30d", "extra"}, `unknown command "extra"`},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			_, err := executeJobs(t, newJobsPruneCmd(neverOpen(t)), tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Execute(%q) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
			}
			if got := ExitCode(err); got != ExitCodeError {
				t.Errorf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
			}
		})
	}
}

func TestParseAge(t *testing.T) {
	valid := map[string]time.Duration{
		"30d":   30 * 24 * time.Hour,
		"+2d":   48 * time.Hour,
		"12h":   12 * time.Hour,
		"90m":   90 * time.Minute,
		"1h30m": 90 * time.Minute,
		"1s":    time.Second,
	}
	for in, want := range valid {
		if got, err := parseAge(in); err != nil || got != want {
			t.Errorf("parseAge(%q) = %v, %v, want %v", in, got, err, want)
		}
	}

	maxDays := strconv.FormatInt(maxAgeDays, 10) + "d"
	if _, err := parseAge(maxDays); err != nil {
		t.Errorf("parseAge(%q) error = %v, want the largest day count accepted", maxDays, err)
	}
	for _, in := range []string{"", "d", "0d", "-1d", "-9223372036854775807d", strconv.FormatInt(maxAgeDays+1, 10) + "d", "1.5d", "30", "0", "-5m", "30D", "thirty days"} {
		if got, err := parseAge(in); err == nil {
			t.Errorf("parseAge(%q) = %v, want an error", in, got)
		}
	}
}

func TestJobsShowSanitizesStoredMessages(t *testing.T) {
	db := newSeededTestDatabase(t)
	// Versions before SEC-009 stored tool output unfiltered.
	job, err := RecordScan(db, time.Now(), []CheckOutcome{{Key: keyDocker, Status: StatusFail, Message: "\x1b[2J\x1b[Hdocker daemon unreachable"}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}

	stdout, err := executeJobs(t, newJobsShowCmd(openerFor(db)), strconv.Itoa(int(job.ID)))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.ContainsRune(stdout, '\x1b') || !strings.Contains(stdout, "?[2J?[Hdocker daemon unreachable") {
		t.Errorf("jobs show output = %q, want control characters replaced", stdout)
	}

	// JSON escapes C0 characters, but not DEL or C1 ones.
	job, err = RecordScan(db, time.Now(), []CheckOutcome{{Key: keyDocker, Status: StatusFail, Message: "del\x7f c1\u009b[2J"}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	stdout, err = executeJobs(t, newJobsShowCmd(openerFor(db)), strconv.Itoa(int(job.ID)), "--json")
	if err != nil {
		t.Fatalf("Execute(--json) error = %v", err)
	}
	if strings.ContainsAny(stdout, "\x7f\u009b") || !strings.Contains(stdout, `"message": "del? c1?[2J"`) {
		t.Errorf("jobs show --json output = %q, want control characters replaced", stdout)
	}
}

// executeCheckRunWith runs check run with a stub that returns outcomes, and
// returns stdout, stderr, and the error. A nil db must never be opened.
func executeCheckRunWith(t *testing.T, db *Database, outcomes []CheckOutcome, args ...string) (string, string, error) {
	t.Helper()

	opener := neverOpen(t)
	if db != nil {
		opener = openerFor(db)
	}
	runChecks := func(context.Context, []string, CheckOptions) ([]CheckOutcome, error) { return outcomes, nil }
	cmd := newCheckRunCmdWith(opener, runChecks)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

var (
	passOutcome = CheckOutcome{Key: keyMisconfig, Status: StatusPass, Message: "no common misconfigurations detected"}
	warnOutcome = CheckOutcome{Key: keyMemory, Status: StatusWarn, Message: "memory 85.0% used, swap 0.0% used"}
	failOutcome = CheckOutcome{Key: keyDiskSpace, Target: "/", Status: StatusFail, Message: "/: 95.0% used (5.0% free)"}
)

func TestCheckRunFailOn(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []CheckOutcome
		args     []string
		want     int
	}{
		{"pass", []CheckOutcome{passOutcome}, nil, ExitCodePass},
		{"warn by default", []CheckOutcome{passOutcome, warnOutcome}, nil, ExitCodeWarn},
		{"warn with fail-on warn", []CheckOutcome{warnOutcome}, []string{"--fail-on", "warn"}, ExitCodeWarn},
		{"warn with fail-on fail", []CheckOutcome{passOutcome, warnOutcome}, []string{"--fail-on", "fail"}, ExitCodePass},
		{"fail with fail-on fail", []CheckOutcome{warnOutcome, failOutcome}, []string{"--fail-on", "fail"}, ExitCodeFail},
		{"fail by default", []CheckOutcome{failOutcome}, nil, ExitCodeFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, err := executeCheckRunWith(t, nil, tt.outcomes, append([]string{"--no-save"}, tt.args...)...)
			if got := ExitCode(err); got != tt.want {
				t.Errorf("ExitCode(%v) = %d, want %d", err, got, tt.want)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
		})
	}
}

func TestCheckRunFormats(t *testing.T) {
	outcomes := []CheckOutcome{passOutcome.withValue(0, unitCount), warnOutcome.withValue(85, unitPercent)}
	tests := []struct {
		args      []string
		wantStart string
	}{
		{nil, "Environment Health Check\n"},
		{[]string{"--format", "text"}, "Environment Health Check\n"},
		{[]string{"--format", "json"}, "[\n"},
		{[]string{"--json", "--format", "json"}, "[\n"},
		{[]string{"--format", "nagios"}, "SALUS WARNING - 2 checks: 1 pass, 1 warn, 0 fail | 'misconfig'=0 'memory'=85%\n"},
		{[]string{"--format", "nagios", "--fail-on", "fail"}, "SALUS OK - 2 checks: 1 pass, 1 warn, 0 fail"},
		{[]string{"--format", "prometheus"}, "# HELP salus_check_status "},
		{[]string{"--format", "junit"}, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<testsuites name=\"salus\" tests=\"2\" failures=\"1\""},
		{[]string{"--format", "junit", "--fail-on", "fail"}, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<testsuites name=\"salus\" tests=\"2\" failures=\"0\""},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			stdout, _, _ := executeCheckRunWith(t, nil, outcomes, append([]string{"--no-save"}, tt.args...)...)
			if !strings.HasPrefix(stdout, tt.wantStart) {
				t.Errorf("output starts %q, want %q", firstLines(stdout, 2), tt.wantStart)
			}
		})
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func TestCheckRunOutputFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "salus.prom")

	for _, args := range [][]string{{}, {"--quiet"}} {
		stdout, _, err := executeCheckRunWith(t, nil, []CheckOutcome{passOutcome}, append([]string{"--no-save", "--format", "prometheus", "--output", path}, args...)...)
		if err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		if stdout != "" {
			t.Errorf("Execute(%v) wrote %q to stdout, want the report only in the file", args, stdout)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if !strings.Contains(string(data), `salus_check_status{key="misconfig",target=""} 0`) {
			t.Errorf("output file = %q, want the misconfig status", data)
		}
	}
	if runtime.GOOS != "windows" {
		assertMode(t, path, 0o600)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the output file (no temp files left)", len(entries))
	}
}

func TestCheckRunRejectsUnusableOutputBeforeRunning(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.prom")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	paths := map[string]string{
		"missing directory": filepath.Join(dir, "missing-dir", "salus.prom"),
		"directory":         dir,
		"parent is a file":  filepath.Join(target, "salus.prom"),
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.prom")
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		paths["symbolic link"] = link
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			runChecks := func(context.Context, []string, CheckOptions) ([]CheckOutcome, error) {
				t.Error("checks ran despite an unusable --output")
				return nil, nil
			}
			cmd := newCheckRunCmdWith(neverOpen(t), runChecks)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--output", path})
			err := cmd.Execute()
			if got := ExitCode(err); got != ExitCodeError || !strings.Contains(err.Error(), "invalid --output") {
				t.Errorf("ExitCode(%v) = %d, want %d with an --output error", err, got, ExitCodeError)
			}
		})
	}
}

func TestCheckRunRetain(t *testing.T) {
	db := newSeededTestDatabase(t)
	old := time.Now().Add(-40 * 24 * time.Hour)
	for i := 0; i < 2; i++ {
		if _, err := RecordScan(db, old, []CheckOutcome{passOutcome}); err != nil {
			t.Fatalf("RecordScan() error = %v", err)
		}
	}
	recent, err := RecordScan(db, time.Now().Add(-24*time.Hour), []CheckOutcome{passOutcome})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}

	if _, _, err := executeCheckRunWith(t, db, []CheckOutcome{passOutcome}, "--retain", "30d"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	if len(jobs) != 2 || jobs[1].ID != recent.ID {
		t.Fatalf("jobs after --retain 30d = %+v, want the recent job and the new run", jobs)
	}

	// Even an age shorter than the run keeps the run just recorded.
	if _, _, err := executeCheckRunWith(t, db, []CheckOutcome{passOutcome}, "--retain", "1ns"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if jobs, err = ListScanJobs(db, 0); err != nil || len(jobs) != 1 {
		t.Fatalf("jobs after --retain 1ns = %+v, %v, want only the new run", jobs, err)
	}
}

func TestCheckRunJSONIncludesTargetsAndValues(t *testing.T) {
	stdout, _, err := executeCheckRunWith(t, nil, []CheckOutcome{failOutcome.withValue(95, unitPercent), passOutcome}, "--no-save", "--json")
	if got := ExitCode(err); got != ExitCodeFail {
		t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitCodeFail)
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("output is not a JSON array: %v", err)
	}
	if keys := jsonKeys(t, raw[0]); keys != "duration_ns,key,message,status,target,unit,value" {
		t.Errorf("targeted outcome keys = %s", keys)
	}
	// Outcomes without a target or value keep the v1.0.2 shape.
	if keys := jsonKeys(t, raw[1]); keys != "duration_ns,key,message,status" {
		t.Errorf("plain outcome keys = %s", keys)
	}
}

func TestJobsShowJSONIncludesTargetsAndValues(t *testing.T) {
	db := newSeededTestDatabase(t)
	job, err := RecordScan(db, time.Now(), []CheckOutcome{failOutcome.withValue(95, unitPercent), passOutcome})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	stdout, err := executeJobs(t, newJobsShowCmd(openerFor(db)), strconv.Itoa(int(job.ID)), "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got struct {
		Results []CheckOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Results) != 2 || got.Results[0].Target != "/" || got.Results[0].Value == nil || *got.Results[0].Value != 95 ||
		got.Results[0].Unit != unitPercent || got.Results[1].Value != nil {
		t.Errorf("results = %+v, want the stored target, value, and unit", got.Results)
	}
}

func TestCheckRunWritesReportWhenSaveFails(t *testing.T) {
	// An unseeded database opens, but RecordScan cannot find the check's
	// feature, standing in for a lock held past the busy timeout or a full
	// disk. The finished report must still be written, and the run exits 3.
	for _, toFile := range []bool{false, true} {
		t.Run(fmt.Sprintf("output file %v", toFile), func(t *testing.T) {
			db := newTestDatabase(t)
			args := []string{}
			output := filepath.Join(t.TempDir(), "report.txt")
			if toFile {
				args = append(args, "--output", output)
			}

			stdout, _, err := executeCheckRunWith(t, db, []CheckOutcome{passOutcome}, args...)
			if got := ExitCode(err); got != ExitCodeError {
				t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
			}
			if !strings.Contains(err.Error(), "the report was written") {
				t.Errorf("error = %q, want it to say the report was written", err)
			}
			report := stdout
			if toFile {
				data, readErr := os.ReadFile(output)
				if readErr != nil {
					t.Fatalf("read report: %v", readErr)
				}
				report = string(data)
			}
			if !strings.Contains(report, "[PASS] misconfig") {
				t.Errorf("report = %q, want the check result", report)
			}
		})
	}
}

func TestCheckRunInterruptedWritesAndSavesNothing(t *testing.T) {
	db := newSeededTestDatabase(t)
	output := filepath.Join(t.TempDir(), "report.txt")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)

	runChecks := func(ctx context.Context, _ []string, _ CheckOptions) ([]CheckOutcome, error) {
		// What RunChecks does when a signal cancels the run's context.
		cancel(errors.New("terminated signal received"))
		<-ctx.Done()
		return nil, context.Cause(ctx)
	}
	cmd := newCheckRunCmdWith(openerFor(db), runChecks)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--output", output})
	err := cmd.ExecuteContext(ctx)

	if got := ExitCode(err); got != ExitCodeError {
		t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
	}
	if want := "interrupted (terminated signal received)"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want it to contain %q", err, want)
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("report file exists (stat error %v), want none", statErr)
	}
	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("%d runs saved, want none", len(jobs))
	}
}
