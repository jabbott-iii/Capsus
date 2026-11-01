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
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func result(key, target string, status CheckStatus, message string) ScanResult {
	return ScanResult{Key: key, Target: target, Status: string(status), Message: message}
}

func TestDiffResults(t *testing.T) {
	from := []ScanResult{
		result(keyDiskSpace, "/", StatusPass, "/: 10% used"),
		result(keyDiskSpace, "/var", StatusPass, "/var: 50% used"),
		result(keyMemory, "", StatusWarn, "memory 85% used"),
		result(keyDocker, "", StatusFail, "docker daemon unreachable"),
		result(keyTimeSync, "", StatusPass, "system clock is synchronized"),
	}
	to := []ScanResult{
		result(keyDiskSpace, "/", StatusPass, "/: 11% used"),
		result(keyDiskSpace, "/var", StatusFail, "/var: 95% used"),
		result(keyMemory, "", StatusPass, "memory 40% used"),
		result(keyDocker, "", StatusWarn, "docker daemon reachable; unhealthy: web"),
		result(keyCertExpiry, "/etc/ssl/a.pem", StatusWarn, "/etc/ssl/a.pem: expires soon"),
	}

	changes, unchanged := diffResults(from, to)
	want := []resultChange{
		{Key: keyDiskSpace, Target: "/var", Change: changeWorse, From: StatusPass, To: StatusFail, Message: "/var: 95% used"},
		{Key: keyMemory, Change: changeBetter, From: StatusWarn, To: StatusPass, Message: "memory 40% used"},
		{Key: keyDocker, Change: changeBetter, From: StatusFail, To: StatusWarn, Message: "docker daemon reachable; unhealthy: web"},
		{Key: keyCertExpiry, Target: "/etc/ssl/a.pem", Change: changeAdded, To: StatusWarn, Message: "/etc/ssl/a.pem: expires soon"},
		{Key: keyTimeSync, Change: changeRemoved, From: StatusPass, Message: "system clock is synchronized"},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Errorf("changes =\n%+v\nwant\n%+v", changes, want)
	}
	if unchanged != 1 {
		t.Errorf("unchanged = %d, want 1", unchanged)
	}

	if changes, unchanged := diffResults(from, from); len(changes) != 0 || unchanged != len(from) {
		t.Errorf("diff of a run with itself = %+v, %d unchanged", changes, unchanged)
	}
}

// recordAt records a run with the given outcomes and start time.
func recordAt(t *testing.T, db *Database, startedAt time.Time, outcomes ...CheckOutcome) ScanJob {
	t.Helper()
	job, err := RecordScan(db, startedAt, outcomes)
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	return job
}

func TestJobsDiffCmd(t *testing.T) {
	db := newSeededTestDatabase(t)

	_, err := executeJobs(t, newJobsDiffCmd(openerFor(db)))
	if err == nil || !strings.Contains(err.Error(), "jobs diff needs two recorded runs; found 0") || ExitCode(err) != ExitCodeError {
		t.Fatalf("diff on an empty database error = %v, want an operational error", err)
	}

	now := time.Now()
	first := recordAt(t, db, now.Add(-3*time.Hour), passOutcome, warnOutcome)
	second := recordAt(t, db, now.Add(-2*time.Hour), passOutcome, CheckOutcome{Key: keyMemory, Status: StatusPass, Message: "memory 40.0% used"})
	third := recordAt(t, db, now.Add(-time.Hour), passOutcome, CheckOutcome{Key: keyMemory, Status: StatusPass, Message: "memory 41.0% used"})

	stdout, err := executeJobs(t, newJobsDiffCmd(openerFor(db)))
	if err != nil {
		t.Fatalf("diff of the latest two error = %v", err)
	}
	if !strings.HasPrefix(stdout, "Job "+strconv.Itoa(int(second.ID))+" (") || !strings.Contains(stdout, "job "+strconv.Itoa(int(third.ID))+" (") ||
		!strings.HasSuffix(stdout, ": 0 changed, 2 unchanged\n") {
		t.Errorf("diff of the latest two = %q", stdout)
	}

	stdout, err = executeJobs(t, newJobsDiffCmd(openerFor(db)), strconv.Itoa(int(first.ID)))
	if err != nil {
		t.Fatalf("diff with one id error = %v", err)
	}
	if !strings.Contains(stdout, ": 1 changed, 1 unchanged\n") || !strings.Contains(stdout, "[WARN -> PASS] memory            memory 41.0% used\n") {
		t.Errorf("diff of job %d and the latest = %q", first.ID, stdout)
	}

	// --exit-code reports changes as exit status 1, and no changes as 0.
	_, err = executeJobs(t, newJobsDiffCmd(openerFor(db)), "--exit-code", strconv.Itoa(int(first.ID)), strconv.Itoa(int(second.ID)))
	var status *ExitStatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Errorf("--exit-code with changes error = %v, want exit status 1", err)
	}
	if _, err := executeJobs(t, newJobsDiffCmd(openerFor(db)), "--exit-code", strconv.Itoa(int(second.ID)), strconv.Itoa(int(third.ID))); err != nil {
		t.Errorf("--exit-code without changes error = %v, want nil", err)
	}

	stdout, err = executeJobs(t, newJobsDiffCmd(openerFor(db)), "--json", strconv.Itoa(int(first.ID)), strconv.Itoa(int(third.ID)))
	if err != nil {
		t.Fatalf("diff --json error = %v", err)
	}
	if keys := jsonKeys(t, json.RawMessage(stdout)); keys != "changes,from,to,unchanged" {
		t.Errorf("diff --json keys = %s", keys)
	}
	var got struct {
		From    jobJSON        `json:"from"`
		To      jobJSON        `json:"to"`
		Changes []resultChange `json:"changes"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.From.ID != first.ID || got.To.ID != third.ID || len(got.Changes) != 1 || got.Changes[0].Change != changeBetter {
		t.Errorf("diff --json = %+v", got)
	}

	stdout, err = executeJobs(t, newJobsDiffCmd(openerFor(db)), "--json", strconv.Itoa(int(second.ID)), strconv.Itoa(int(third.ID)))
	if err != nil || !strings.Contains(stdout, `"changes": []`) {
		t.Errorf("diff --json without changes = %q, %v, want an empty changes array", stdout, err)
	}
}

func TestJobsDiffCmdErrors(t *testing.T) {
	db := newSeededTestDatabase(t)
	job := recordAt(t, db, time.Now(), passOutcome)

	tests := []struct {
		args []string
		want string
	}{
		{nil, "jobs diff needs two recorded runs; found 1"},
		{[]string{"x"}, `invalid job id "x"`},
		{[]string{strconv.Itoa(int(job.ID)), "999"}, "scan job 999: record not found"},
		{[]string{"1", "2", "3"}, "accepts at most 2 arg(s), received 3"},
		{[]string{strconv.Itoa(int(job.ID))}, "is the most recent run; give an older job id, or two ids"},
	}
	for _, tt := range tests {
		_, err := executeJobs(t, newJobsDiffCmd(openerFor(db)), tt.args...)
		if err == nil || !strings.Contains(err.Error(), tt.want) || ExitCode(err) != ExitCodeError {
			t.Errorf("jobs diff %q error = %v, want an operational error containing %q", tt.args, err, tt.want)
		}
	}
}

func TestWriteDiffTextLabels(t *testing.T) {
	var b strings.Builder
	changes := []resultChange{
		{Key: keyCertExpiry, Target: "/a.pem", Change: changeAdded, To: StatusWarn, Message: "soon"},
		{Key: keyTimeSync, Change: changeRemoved, From: StatusPass, Message: "synced"},
	}
	if err := writeDiffText(&b, ScanJob{ID: 1}, ScanJob{ID: 2}, changes, 0); err != nil {
		t.Fatalf("writeDiffText() error = %v", err)
	}
	for _, want := range []string{"[added WARN] cert-expiry       soon\n", "[removed PASS] time-sync         synced\n"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("output %q lacks %q", b.String(), want)
		}
	}
	if err := writeDiffText(failingWriter{err: errors.New("disk full")}, ScanJob{}, ScanJob{}, changes, 0); err == nil {
		t.Error("writeDiffText() to a failing writer returned nil")
	}
}

func TestScanStats(t *testing.T) {
	db := newSeededTestDatabase(t)
	now := time.Now()
	disk := func(status CheckStatus) CheckOutcome {
		return CheckOutcome{Key: keyDiskSpace, Target: "/", Status: status, Message: "disk"}
	}
	mem := func(status CheckStatus) CheckOutcome {
		return CheckOutcome{Key: keyMemory, Status: status, Message: "memory"}
	}

	// Outside the window: must not count.
	recordAt(t, db, now.Add(-10*24*time.Hour), disk(StatusFail), mem(StatusFail))
	// Inside, oldest first: memory flaps PASS→WARN→PASS→WARN; disk changes once.
	recordAt(t, db, now.Add(-5*time.Hour), disk(StatusPass), mem(StatusPass))
	recordAt(t, db, now.Add(-4*time.Hour), disk(StatusPass), mem(StatusWarn))
	recordAt(t, db, now.Add(-3*time.Hour), disk(StatusPass), mem(StatusPass))
	recordAt(t, db, now.Add(-2*time.Hour), mem(StatusWarn)) // disk not run
	recordAt(t, db, now.Add(-time.Hour), disk(StatusFail), mem(StatusWarn))

	runs, stats, err := ScanStats(db, now.Add(-7*24*time.Hour), 3)
	if err != nil {
		t.Fatalf("ScanStats() error = %v", err)
	}
	if runs != 5 {
		t.Errorf("runs = %d, want 5", runs)
	}
	want := []checkStats{
		{Key: keyDiskSpace, Target: "/", Runs: 4, Pass: 3, Fail: 1, Changes: 1, Last: StatusFail},
		{Key: keyMemory, Runs: 5, Pass: 2, Warn: 3, Changes: 3, Last: StatusWarn, Flapping: true},
	}
	if !reflect.DeepEqual(stats, want) {
		t.Errorf("stats =\n%+v\nwant\n%+v", stats, want)
	}

	if _, stats, _ := ScanStats(db, now.Add(-7*24*time.Hour), 4); stats[1].Flapping {
		t.Error("memory flagged as flapping below the threshold")
	}
	if runs, stats, err := ScanStats(db, now.Add(time.Hour), 3); err != nil || runs != 0 || stats == nil || len(stats) != 0 {
		t.Errorf("ScanStats() for an empty window = %d, %v, %v, want 0 runs and an empty slice", runs, stats, err)
	}
}

func TestJobsStatsCmd(t *testing.T) {
	db := newSeededTestDatabase(t)

	stdout, err := executeJobs(t, newJobsStatsCmd(openerFor(db)))
	if err != nil || !strings.HasPrefix(stdout, "Runs since ") || !strings.HasSuffix(stdout, ": 0\n") {
		t.Fatalf("stats on an empty database = %q, %v", stdout, err)
	}

	recordAt(t, db, time.Now().Add(-time.Hour), passOutcome, failOutcome)
	stdout, err = executeJobs(t, newJobsStatsCmd(openerFor(db)), "--since", "1d")
	if err != nil {
		t.Fatalf("stats error = %v", err)
	}
	for _, want := range []string{
		"CHECK       TARGET  RUNS  PASS  WARN  FAIL  CHANGES  LAST  FLAPPING\n",
		"disk-space  /       1     0     0     1     0        FAIL  no\n",
		"misconfig   -       1     1     0     0     0        PASS  no\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stats output lacks %q:\n%s", want, stdout)
		}
	}

	stdout, err = executeJobs(t, newJobsStatsCmd(openerFor(db)), "--json")
	if err != nil {
		t.Fatalf("stats --json error = %v", err)
	}
	if keys := jsonKeys(t, json.RawMessage(stdout)); keys != "checks,runs,since" {
		t.Errorf("stats --json keys = %s", keys)
	}
	var got struct {
		Checks []json.RawMessage `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || len(got.Checks) != 2 {
		t.Fatalf("stats --json checks = %v, %v", got.Checks, err)
	}
	if keys := jsonKeys(t, got.Checks[0]); keys != "changes,fail,flapping,key,last_status,pass,runs,target,warn" {
		t.Errorf("stats --json check keys = %s", keys)
	}
}

func TestJobsStatsCmdRejectsInvalidFlags(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--since", "soon"}, `invalid --since value "soon"`},
		{[]string{"--flap-threshold", "0"}, "invalid --flap-threshold value 0: must be greater than 0"},
	} {
		_, err := executeJobs(t, newJobsStatsCmd(neverOpen(t)), tt.args...)
		if err == nil || !strings.Contains(err.Error(), tt.want) || ExitCode(err) != ExitCodeError {
			t.Errorf("jobs stats %q error = %v, want %q", tt.args, err, tt.want)
		}
	}
}

func TestWriteStatsTextReturnsWriteErrors(t *testing.T) {
	stats := []checkStats{{Key: keyMemory, Runs: 1, Pass: 1, Last: StatusPass}}
	if err := writeStatsText(failingWriter{err: errors.New("disk full")}, time.Now(), 1, stats); err == nil {
		t.Error("writeStatsText() to a failing writer returned nil")
	}
}

func TestHistoryOrdersRunsByInstantAcrossUTCOffsets(t *testing.T) {
	// Stored times keep the offset they were written with, as across a DST
	// change: 01:00-05:00 sorts before 05:30+00:00 as text, but is later.
	db := newSeededTestDatabase(t)
	base := time.Now().UTC().Truncate(time.Hour).Add(-6 * time.Hour)
	first := recordAt(t, db, base, passOutcome)
	second := recordAt(t, db, base.Add(30*time.Minute), CheckOutcome{Key: keyMisconfig, Status: StatusWarn, Message: "problem"})
	third := recordAt(t, db, base.Add(time.Hour).In(time.FixedZone("EST", -5*60*60)), passOutcome)

	jobs, err := ListScanJobs(db, 2)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	if len(jobs) != 2 || jobs[0].ID != third.ID || jobs[1].ID != second.ID {
		t.Errorf("latest two jobs = %+v, want %d then %d", jobs, third.ID, second.ID)
	}

	_, stats, err := ScanStats(db, base.Add(-time.Minute), 3)
	if err != nil {
		t.Fatalf("ScanStats() error = %v", err)
	}
	if len(stats) != 1 || stats[0].Changes != 2 || stats[0].Last != StatusPass {
		t.Errorf("stats = %+v, want 2 changes ending in PASS (runs %d, %d, %d in order)", stats, first.ID, second.ID, third.ID)
	}
}

func TestDiffResultsAcrossUpgrade(t *testing.T) {
	// v1.0.2 stored no targets; its messages named them.
	from := []ScanResult{
		result(keyDiskSpace, "", StatusPass, "/data: 40.0% used (60.0% free)"),
		result(keyServiceUptime, "", StatusPass, `service "nginx" is active`),
		result(keyMemory, "", StatusPass, "memory 40.0% used, swap 0.0% used"),
	}
	to := []ScanResult{
		result(keyDiskSpace, "/data", StatusWarn, "/data: 85.0% used (15.0% free)"),
		result(keyServiceUptime, "nginx", StatusPass, `service "nginx" is active`),
		result(keyMemory, "", StatusPass, "memory 41.0% used, swap 0.0% used"),
	}
	changes, unchanged := diffResults(from, to)
	want := []resultChange{{Key: keyDiskSpace, Target: "/data", Change: changeWorse, From: StatusPass, To: StatusWarn, Message: "/data: 85.0% used (15.0% free)"}}
	if !reflect.DeepEqual(changes, want) || unchanged != 2 {
		t.Errorf("diff across the upgrade = %+v, %d unchanged; want only the disk change and 2 unchanged", changes, unchanged)
	}
	if from[0].Target != "" {
		t.Error("diffResults modified its input")
	}

	// Host uptime and a service are different results, even with one each.
	changes, _ = diffResults(
		[]ScanResult{result(keyServiceUptime, "", StatusPass, "host has been up for 3h0m0s")},
		[]ScanResult{result(keyServiceUptime, "nginx", StatusPass, `service "nginx" is active`)},
	)
	if len(changes) != 2 || changes[0].Change != changeAdded || changes[1].Change != changeRemoved {
		t.Errorf("host uptime vs service = %+v, want added and removed", changes)
	}

	// Several targets on the new side cannot be matched to one old row.
	changes, _ = diffResults(
		[]ScanResult{result(keyDiskSpace, "", StatusPass, "/: 1.0% used")},
		[]ScanResult{result(keyDiskSpace, "/", StatusPass, "/: 1.0% used"), result(keyDiskSpace, "/var", StatusPass, "/var: 1.0% used")},
	)
	if len(changes) != 3 {
		t.Errorf("one old row vs two targets = %+v, want 2 added and 1 removed", changes)
	}
}

func TestScanStatsFoldsLegacyRowsIntoTheirTarget(t *testing.T) {
	db := newSeededTestDatabase(t)
	now := time.Now()
	recordAt(t, db, now.Add(-3*time.Hour), CheckOutcome{Key: keyDiskSpace, Status: StatusPass, Message: "/: 50.0% used (50.0% free)"})
	recordAt(t, db, now.Add(-2*time.Hour), CheckOutcome{Key: keyDiskSpace, Target: "/", Status: StatusWarn, Message: "/: 85.0% used (15.0% free)"})

	_, stats, err := ScanStats(db, now.Add(-24*time.Hour), 3)
	if err != nil {
		t.Fatalf("ScanStats() error = %v", err)
	}
	want := []checkStats{{Key: keyDiskSpace, Target: "/", Runs: 2, Pass: 1, Warn: 1, Changes: 1, Last: StatusWarn}}
	if !reflect.DeepEqual(stats, want) {
		t.Errorf("stats = %+v, want %+v", stats, want)
	}
}
