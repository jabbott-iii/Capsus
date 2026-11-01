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
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Report formats accepted by check run --format.
const (
	formatText       = "text"
	formatJSON       = "json"
	formatNagios     = "nagios"
	formatPrometheus = "prometheus"
	formatJUnit      = "junit"
)

var reportFormats = []string{formatText, formatJSON, formatNagios, formatPrometheus, formatJUnit}

// runReport is a completed check run as the report writers see it.
type runReport struct {
	outcomes   []CheckOutcome
	startedAt  time.Time
	finishedAt time.Time
	// exitCode is the run's exit code after --fail-on; Nagios derives its
	// state from it.
	exitCode int
	// failOn is the lowest status that fails the run (--fail-on); JUnit
	// reports WARN as a failure only when it is StatusWarn.
	failOn   CheckStatus
	failOnly bool
}

// writeReport renders r in one of reportFormats.
func writeReport(w io.Writer, format string, r runReport) error {
	switch format {
	case formatJSON:
		return WriteOutcomesJSON(w, r.outcomes)
	case formatNagios:
		return writeNagios(w, r)
	case formatPrometheus:
		return writePrometheus(w, r)
	case formatJUnit:
		return writeJUnit(w, r)
	default:
		return WriteOutcomesText(w, "Environment Health Check", r.outcomes, r.failOnly)
	}
}

// countStatuses counts outcomes by status.
func countStatuses(outcomes []CheckOutcome) (pass, warn, fail int) {
	for _, o := range outcomes {
		switch o.Status {
		case StatusPass:
			pass++
		case StatusWarn:
			warn++
		case StatusFail:
			fail++
		}
	}
	return pass, warn, fail
}

// displayName is the key, followed by the target when there is one.
func displayName(o CheckOutcome) string {
	if o.Target == "" {
		return o.Key
	}
	return o.Key + " " + o.Target
}

//--------------------------------------------------nagios---------------------------------------------------------------------------------------------//

// nagiosStates maps exit codes to Nagios plugin states; Salus uses the same
// numbers, including 3 for UNKNOWN (an operational error).
var nagiosStates = []string{"OK", "WARNING", "CRITICAL", "UNKNOWN"}

// nagiosUnits maps value units to Nagios performance data units of measure.
var nagiosUnits = map[string]string{unitPercent: "%", unitSeconds: "s"}

// writeNagios writes Nagios plugin output: a status line with the counts and
// performance data, then one line per outcome. "|" separates performance data
// in plugin output, so it is replaced in text.
func writeNagios(w io.Writer, r runReport) error {
	pass, warn, fail := countStatuses(r.outcomes)
	state := "UNKNOWN"
	if r.exitCode >= 0 && r.exitCode < len(nagiosStates) {
		state = nagiosStates[r.exitCode]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "SALUS %s - %d checks: %d pass, %d warn, %d fail", state, len(r.outcomes), pass, warn, fail)
	var perf []string
	for _, o := range r.outcomes {
		if o.Value == nil {
			continue
		}
		label := strings.NewReplacer("'", "''", "=", "_", "|", "/").Replace(displayName(o))
		value := strconv.FormatFloat(math.Round(*o.Value*100)/100, 'f', -1, 64)
		perf = append(perf, fmt.Sprintf("'%s'=%s%s", label, value, nagiosUnits[o.Unit]))
	}
	if len(perf) > 0 {
		b.WriteString(" | " + strings.Join(perf, " "))
	}
	b.WriteString("\n")

	// Like the text report, detail lines name the check; messages of
	// targeted checks already start with or name their target.
	noPipes := strings.NewReplacer("|", "/")
	for _, o := range r.outcomes {
		if r.failOnly && o.Status == StatusPass {
			continue
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", o.Status, o.Key, noPipes.Replace(o.Message))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

//--------------------------------------------------prometheus-----------------------------------------------------------------------------------------//

// statusValue is the numeric form of a status in metrics; it matches the
// exit codes without --fail-on.
func statusValue(s CheckStatus) int {
	return ExitCodeFor(s)
}

// promLabel escapes a label value for the Prometheus text format.
var promLabel = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

// writePrometheus writes the Prometheus text exposition format, as read by
// node_exporter's textfile collector. Messages are left out: they are free
// text and would create a new series whenever they change.
func writePrometheus(w io.Writer, r runReport) error {
	var b strings.Builder
	labels := func(o CheckOutcome) string {
		return fmt.Sprintf(`key="%s",target="%s"`, promLabel.Replace(o.Key), promLabel.Replace(o.Target))
	}
	float := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

	b.WriteString("# HELP salus_check_status Result of a Salus health check: 0 PASS, 1 WARN, 2 FAIL.\n")
	b.WriteString("# TYPE salus_check_status gauge\n")
	for _, o := range r.outcomes {
		fmt.Fprintf(&b, "salus_check_status{%s} %d\n", labels(o), statusValue(o.Status))
	}

	header := false
	for _, o := range r.outcomes {
		if o.Value == nil {
			continue
		}
		if !header {
			b.WriteString("# HELP salus_check_value Value a Salus health check measured, in the unit named by the unit label.\n")
			b.WriteString("# TYPE salus_check_value gauge\n")
			header = true
		}
		fmt.Fprintf(&b, "salus_check_value{%s,unit=\"%s\"} %s\n", labels(o), promLabel.Replace(o.Unit), float(*o.Value))
	}

	b.WriteString("# HELP salus_check_duration_seconds Time a Salus health check took.\n")
	b.WriteString("# TYPE salus_check_duration_seconds gauge\n")
	for _, o := range r.outcomes {
		fmt.Fprintf(&b, "salus_check_duration_seconds{%s} %s\n", labels(o), float(o.Duration.Seconds()))
	}

	b.WriteString("# HELP salus_last_run_timestamp_seconds Unix time when the Salus check run finished.\n")
	b.WriteString("# TYPE salus_last_run_timestamp_seconds gauge\n")
	// Milliseconds as a fixed decimal; a float64 of nanoseconds prints noise.
	fmt.Fprintf(&b, "salus_last_run_timestamp_seconds %s\n", strconv.FormatFloat(float64(r.finishedAt.UnixMilli())/1000, 'f', 3, 64))

	_, err := io.WriteString(w, b.String())
	return err
}

//--------------------------------------------------junit----------------------------------------------------------------------------------------------//

type junitTestsuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      string          `xml:"time,attr"`
	Timestamp string          `xml:"timestamp,attr"`
	Cases     []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

// writeJUnit writes JUnit XML with one test case per outcome. FAIL is a
// failure; WARN is a failure only when it fails the run (--fail-on warn), so
// a CI report shows as failed exactly what fails the step.
func writeJUnit(w io.Writer, r runReport) error {
	seconds := func(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', 3, 64) }

	suite := junitTestsuite{
		Name:      "salus check run",
		Tests:     len(r.outcomes),
		Time:      seconds(r.finishedAt.Sub(r.startedAt)),
		Timestamp: r.startedAt.UTC().Format("2006-01-02T15:04:05"),
	}
	for _, o := range r.outcomes {
		tc := junitTestcase{Name: displayName(o), Classname: "salus." + o.Key, Time: seconds(o.Duration)}
		switch {
		case o.Status == StatusFail || (o.Status == StatusWarn && r.failOn == StatusWarn):
			tc.Failure = &junitFailure{Message: o.Message, Type: string(o.Status), Text: o.Message}
			suite.Failures++
		case o.Status == StatusWarn:
			tc.SystemOut = "WARN: " + o.Message
		default:
			tc.SystemOut = o.Message
		}
		suite.Cases = append(suite.Cases, tc)
	}
	report := junitTestsuites{Name: "salus", Tests: suite.Tests, Failures: suite.Failures, Time: suite.Time, Suites: []junitTestsuite{suite}}

	data, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, xml.Header+string(data)+"\n")
	return err
}

//--------------------------------------------------output file----------------------------------------------------------------------------------------//

// writeFileAtomic writes the output of write to path through a temporary file
// in the same directory and a rename, so a reader such as node_exporter's
// textfile collector never sees a partial file. The temporary name starts
// with "." and ends in ".tmp", which the collector ignores. A new file is
// created owner-only (0600); an existing file keeps its permission bits and,
// where the user may set it, its group, so it can be shared with a reader
// (for example mode 0640 and node_exporter's group) once.
func writeFileAtomic(path string, write func(io.Writer) error) (err error) {
	mode := fs.FileMode(0o600)
	existing, statErr := os.Lstat(path)
	if statErr == nil {
		if !existing.Mode().IsRegular() {
			return fmt.Errorf("output %s is not a regular file", path)
		}
		mode = existing.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if err = write(tmp); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	if existing != nil {
		if err = keepGroup(tmp, existing); err != nil {
			return fmt.Errorf("set output file group: %w", err)
		}
	}
	if err = tmp.Chmod(mode); err != nil {
		return fmt.Errorf("set output file mode: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace output file: %w", err)
	}
	return nil
}
