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
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// sampleReport is a run with one outcome of each status, a target that needs
// escaping in every format, and a message with characters each format
// treats specially.
func sampleReport(failOn CheckStatus, exitCode int) runReport {
	started := time.Date(2026, 10, 3, 23, 0, 0, 0, time.UTC)
	return runReport{
		outcomes: []CheckOutcome{
			CheckOutcome{Key: keyDiskSpace, Target: `/mnt/it's "a"=b\c|d`, Status: StatusPass, Message: "fine | <ok> & done", Duration: 1500 * time.Microsecond}.withValue(42.123, unitPercent),
			CheckOutcome{Key: keyServiceUptime, Status: StatusPass, Message: "host has been up for 1h0m0s", Duration: time.Millisecond}.withValue(3600, unitSeconds),
			CheckOutcome{Key: keyCertExpiry, Target: "/etc/ssl/a.pem", Status: StatusWarn, Message: "/etc/ssl/a.pem: a expires 2026-10-20 (in 16 days)", Duration: 2 * time.Millisecond}.withValue(16.5, unitDays),
			{Key: keyTimeSync, Status: StatusFail, Message: "broken", Duration: 0},
		},
		startedAt:  started,
		finishedAt: started.Add(250 * time.Millisecond),
		exitCode:   exitCode,
		failOn:     failOn,
	}
}

func TestWriteNagios(t *testing.T) {
	var b strings.Builder
	if err := writeNagios(&b, sampleReport(StatusWarn, ExitCodeFail)); err != nil {
		t.Fatalf("writeNagios() error = %v", err)
	}
	want := `SALUS CRITICAL - 4 checks: 2 pass, 1 warn, 1 fail | 'disk-space /mnt/it''s "a"_b\c/d'=42.12% 'service-uptime'=3600s 'cert-expiry /etc/ssl/a.pem'=16.5
[PASS] disk-space: fine / <ok> & done
[PASS] service-uptime: host has been up for 1h0m0s
[WARN] cert-expiry: /etc/ssl/a.pem: a expires 2026-10-20 (in 16 days)
[FAIL] time-sync: broken
`
	if b.String() != want {
		t.Errorf("writeNagios() =\n%s\nwant\n%s", b.String(), want)
	}
}

func TestWriteNagiosStatesAndFailOnly(t *testing.T) {
	for code, state := range []string{"OK", "WARNING", "CRITICAL", "UNKNOWN"} {
		r := sampleReport(StatusWarn, code)
		r.failOnly = true
		var b strings.Builder
		if err := writeNagios(&b, r); err != nil {
			t.Fatalf("writeNagios() error = %v", err)
		}
		if !strings.HasPrefix(b.String(), "SALUS "+state+" - ") {
			t.Errorf("exit code %d: status line %q, want state %s", code, firstLines(b.String(), 1), state)
		}
		if strings.Contains(b.String(), "[PASS]") {
			t.Errorf("--fail-only nagios output lists PASS results: %q", b.String())
		}
	}
}

func TestWritePrometheus(t *testing.T) {
	var b strings.Builder
	if err := writePrometheus(&b, sampleReport(StatusWarn, ExitCodeFail)); err != nil {
		t.Fatalf("writePrometheus() error = %v", err)
	}
	want := `# HELP salus_check_status Result of a Salus health check: 0 PASS, 1 WARN, 2 FAIL.
# TYPE salus_check_status gauge
salus_check_status{key="disk-space",target="/mnt/it's \"a\"=b\\c|d"} 0
salus_check_status{key="service-uptime",target=""} 0
salus_check_status{key="cert-expiry",target="/etc/ssl/a.pem"} 1
salus_check_status{key="time-sync",target=""} 2
# HELP salus_check_value Value a Salus health check measured, in the unit named by the unit label.
# TYPE salus_check_value gauge
salus_check_value{key="disk-space",target="/mnt/it's \"a\"=b\\c|d",unit="percent"} 42.123
salus_check_value{key="service-uptime",target="",unit="seconds"} 3600
salus_check_value{key="cert-expiry",target="/etc/ssl/a.pem",unit="days"} 16.5
# HELP salus_check_duration_seconds Time a Salus health check took.
# TYPE salus_check_duration_seconds gauge
salus_check_duration_seconds{key="disk-space",target="/mnt/it's \"a\"=b\\c|d"} 0.0015
salus_check_duration_seconds{key="service-uptime",target=""} 0.001
salus_check_duration_seconds{key="cert-expiry",target="/etc/ssl/a.pem"} 0.002
salus_check_duration_seconds{key="time-sync",target=""} 0
# HELP salus_last_run_timestamp_seconds Unix time when the Salus check run finished.
# TYPE salus_last_run_timestamp_seconds gauge
salus_last_run_timestamp_seconds 1791068400.250
`
	if b.String() != want {
		t.Errorf("writePrometheus() =\n%s\nwant\n%s", b.String(), want)
	}
}

func TestWritePrometheusEscapesNewlinesAndOmitsEmptyValueFamily(t *testing.T) {
	r := runReport{outcomes: []CheckOutcome{{Key: keyTimeSync, Target: "a\nb", Status: StatusPass}}, finishedAt: time.Unix(0, 0)}
	var b strings.Builder
	if err := writePrometheus(&b, r); err != nil {
		t.Fatalf("writePrometheus() error = %v", err)
	}
	if !strings.Contains(b.String(), `target="a\nb"`) {
		t.Errorf("newline in a label is not escaped: %q", b.String())
	}
	if strings.Contains(b.String(), "salus_check_value") {
		t.Errorf("output declares salus_check_value without samples: %q", b.String())
	}
}

func TestWriteJUnit(t *testing.T) {
	var b strings.Builder
	if err := writeJUnit(&b, sampleReport(StatusWarn, ExitCodeFail)); err != nil {
		t.Fatalf("writeJUnit() error = %v", err)
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="salus" tests="4" failures="2" errors="0" time="0.250">
  <testsuite name="salus check run" tests="4" failures="2" errors="0" skipped="0" time="0.250" timestamp="2026-10-03T23:00:00">
    <testcase name="disk-space /mnt/it&#39;s &#34;a&#34;=b\c|d" classname="salus.disk-space" time="0.002">
      <system-out>fine | &lt;ok&gt; &amp; done</system-out>
    </testcase>
    <testcase name="service-uptime" classname="salus.service-uptime" time="0.001">
      <system-out>host has been up for 1h0m0s</system-out>
    </testcase>
    <testcase name="cert-expiry /etc/ssl/a.pem" classname="salus.cert-expiry" time="0.002">
      <failure message="/etc/ssl/a.pem: a expires 2026-10-20 (in 16 days)" type="WARN">/etc/ssl/a.pem: a expires 2026-10-20 (in 16 days)</failure>
    </testcase>
    <testcase name="time-sync" classname="salus.time-sync" time="0.000">
      <failure message="broken" type="FAIL">broken</failure>
    </testcase>
  </testsuite>
</testsuites>
`
	if b.String() != want {
		t.Errorf("writeJUnit() =\n%s\nwant\n%s", b.String(), want)
	}

	var parsed junitTestsuites
	if err := xml.Unmarshal([]byte(b.String()), &parsed); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
	if got := parsed.Suites[0].Cases[0].Name; got != `disk-space /mnt/it's "a"=b\c|d` {
		t.Errorf("round-tripped name = %q", got)
	}
}

func TestWriteJUnitWarnPassesWithFailOnFail(t *testing.T) {
	var b strings.Builder
	if err := writeJUnit(&b, sampleReport(StatusFail, ExitCodeFail)); err != nil {
		t.Fatalf("writeJUnit() error = %v", err)
	}
	if !strings.Contains(b.String(), `failures="1"`) || !strings.Contains(b.String(), "<system-out>WARN: /etc/ssl/a.pem") {
		t.Errorf("with --fail-on fail, WARN should be reported as passing output:\n%s", b.String())
	}
}

func TestWriteReportDispatchesEveryFormat(t *testing.T) {
	for _, format := range reportFormats {
		var b strings.Builder
		if err := writeReport(&b, format, sampleReport(StatusWarn, ExitCodeFail)); err != nil || b.Len() == 0 {
			t.Errorf("writeReport(%s) = %q, %v", format, b.String(), err)
		}
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	write := func(s string) func(io.Writer) error {
		return func(w io.Writer) error {
			_, err := io.WriteString(w, s)
			return err
		}
	}

	if err := writeFileAtomic(path, write("first")); err != nil {
		t.Fatalf("writeFileAtomic() error = %v", err)
	}
	if runtime.GOOS != "windows" {
		assertMode(t, path, 0o600)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("chmod: %v", err)
		}
	}

	if err := writeFileAtomic(path, write("second")); err != nil {
		t.Fatalf("writeFileAtomic() error = %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "second" {
		t.Errorf("file = %q, %v, want %q", data, err, "second")
	}
	if runtime.GOOS != "windows" {
		assertMode(t, path, 0o644) // an existing file keeps its mode
	}

	failing := errors.New("disk full")
	err := writeFileAtomic(path, func(w io.Writer) error { return failing })
	if !errors.Is(err, failing) {
		t.Errorf("writeFileAtomic() error = %v, want %v", err, failing)
	}
	if data, _ := os.ReadFile(path); string(data) != "second" {
		t.Errorf("a failed write changed the file to %q", data)
	}

	if err := writeFileAtomic(dir, write("x")); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("writeFileAtomic(directory) error = %v, want a not-a-regular-file error", err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(t.TempDir(), "link.txt")
		if err := os.Symlink(path, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if err := writeFileAtomic(link, write("x")); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("writeFileAtomic(symlink) error = %v, want a not-a-regular-file error", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the report (temp files removed)", len(entries))
	}
}
