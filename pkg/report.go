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
	"fmt"
	"io"
	"time"
)

// WriteOutcomesText renders check outcomes as a human-readable, aligned report.
// When failOnly is true, only WARN and FAIL outcomes are printed.
func WriteOutcomesText(w io.Writer, title string, outcomes []CheckOutcome, failOnly bool) error {
	if title != "" {
		if _, err := fmt.Fprintln(w, title); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	for _, o := range outcomes {
		if failOnly && o.Status == StatusPass {
			continue
		}
		if _, err := fmt.Fprintf(w, "[%s] %-17s %s\n", o.Status, o.Key, o.Message); err != nil {
			return err
		}
	}
	return nil
}

// WriteOutcomesJSON renders check outcomes as JSON.
func WriteOutcomesJSON(w io.Writer, outcomes []CheckOutcome) error {
	return writeJSON(w, outcomes)
}

// jobJSON is one recorded check run in the output of jobs list --json and
// jobs show --json. finished_at is null for a run that never completed.
type jobJSON struct {
	ID         uint       `json:"id"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Summary    string     `json:"summary"`
}

func newJobJSON(job ScanJob) jobJSON {
	return jobJSON{ID: job.ID, Status: job.Status, StartedAt: job.StartedAt, FinishedAt: job.FinishedAt, Summary: job.Summary}
}

// WriteJobsJSON renders recorded check runs as a JSON array.
func WriteJobsJSON(w io.Writer, jobs []ScanJob) error {
	out := make([]jobJSON, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, newJobJSON(job))
	}
	return writeJSON(w, out)
}

// WriteJobJSON renders one recorded check run and its results. Each result
// has the check run --json object shape; stored durations have millisecond
// precision. Messages are sanitized like in RunChecks, because rows stored
// before SEC-009 may hold DEL or C1 control characters, which JSON leaves
// unescaped.
func WriteJobJSON(w io.Writer, job ScanJob, results []ScanResult) error {
	outcomes := make([]CheckOutcome, 0, len(results))
	for _, r := range results {
		outcomes = append(outcomes, resultOutcome(r))
	}
	return writeJSON(w, struct {
		jobJSON
		Results []CheckOutcome `json:"results"`
	}{newJobJSON(job), outcomes})
}

// resultOutcome converts a stored result back into the outcome it recorded.
// Messages and targets are sanitized like in RunChecks, because rows stored
// before SEC-009 may hold control characters.
func resultOutcome(r ScanResult) CheckOutcome {
	return CheckOutcome{
		Key:      r.Key,
		Target:   sanitizeMessage(r.Target),
		Status:   CheckStatus(r.Status),
		Message:  sanitizeMessage(r.Message),
		Value:    r.Value,
		Unit:     r.Unit,
		Duration: time.Duration(r.DurationMs) * time.Millisecond,
	}
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

// WorstStatus returns the most severe status among the given outcomes,
// or StatusPass if outcomes is empty.
func WorstStatus(outcomes []CheckOutcome) CheckStatus {
	worst := StatusPass
	for _, o := range outcomes {
		switch o.Status {
		case StatusFail:
			return StatusFail
		case StatusWarn:
			worst = StatusWarn
		}
	}
	return worst
}

// Process exit codes. ExitCodeError covers anything that prevents a complete
// run: invalid flags or arguments, unknown check keys, and database failures.
const (
	ExitCodePass  = 0
	ExitCodeWarn  = 1
	ExitCodeFail  = 2
	ExitCodeError = 3
)

// ExitCodeFor maps a worst-case status to a process exit code:
// 0 for PASS, 1 for WARN, and 2 for FAIL.
func ExitCodeFor(status CheckStatus) int {
	switch status {
	case StatusFail:
		return ExitCodeFail
	case StatusWarn:
		return ExitCodeWarn
	default:
		return ExitCodePass
	}
}

// ExitStatusError reports a completed check run whose worst status maps to a
// non-zero exit code. It is returned from a command so that the exit code
// reaches Main without os.Exit inside the command; it is not an operational
// error.
type ExitStatusError struct {
	Code int
}

func (e *ExitStatusError) Error() string {
	return fmt.Sprintf("health checks finished with exit status %d", e.Code)
}

// ExitCode maps the error returned by the root command to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitCodePass
	}
	var status *ExitStatusError
	if errors.As(err, &status) {
		return status.Code
	}
	return ExitCodeError
}
