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
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestThresholdStatus(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  CheckStatus
	}{
		{name: "below warn", value: 10, want: StatusPass},
		{name: "at warn", value: 80, want: StatusWarn},
		{name: "between warn and fail", value: 85, want: StatusWarn},
		{name: "at fail", value: 90, want: StatusFail},
		{name: "above fail", value: 99, want: StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := thresholdStatus(tt.value, 80, 90, "detail")
			if status != tt.want {
				t.Errorf("thresholdStatus(%v) = %v, want %v", tt.value, status, tt.want)
			}
		})
	}
}

func TestRunChecksDefaultsToAllChecks(t *testing.T) {
	// No external tools are "installed", so docker/kubectl/systemctl/timedatectl
	// are never executed. Without --cert, cert-expiry is skipped.
	opts, calls := fakeToolOptions(t, nil, nil)
	outcomes, err := RunChecks(t.Context(), nil, opts)
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	want := slices.DeleteFunc(slices.Clone(AllCheckKeys), func(key string) bool { return key == keyCertExpiry })
	if got := outcomeKeys(outcomes); !slices.Equal(got, want) {
		t.Errorf("RunChecks() keys = %q, want %q", got, want)
	}
	if len(*calls) != 0 {
		t.Errorf("RunChecks() executed external commands %q, want none", *calls)
	}
}

func outcomeKeys(outcomes []CheckOutcome) []string {
	keys := make([]string, 0, len(outcomes))
	for _, o := range outcomes {
		keys = append(keys, o.Key)
	}
	return keys
}

func TestAllCheckKeysKeepExistingPositions(t *testing.T) {
	// Scripts may index check run --json by position; checks added later go
	// at the end.
	v102 := []string{"disk-space", "memory", "cpu-load", "docker-status", "kubernetes-status", "service-uptime", "misconfig"}
	if !slices.Equal(AllCheckKeys[:len(v102)], v102) {
		t.Errorf("AllCheckKeys starts with %q, want %q", AllCheckKeys[:len(v102)], v102)
	}
	for _, key := range AllCheckKeys {
		if _, ok := checkRegistry[key]; !ok {
			t.Errorf("check %q has no registry entry", key)
		}
	}
	if len(checkRegistry) != len(AllCheckKeys) {
		t.Errorf("checkRegistry has %d entries, AllCheckKeys %d", len(checkRegistry), len(AllCheckKeys))
	}
}

func TestRunChecksRunsTargetedChecksPerTarget(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("service checks need systemd (Linux only)")
	}
	isolateMisconfigEnv(t)
	opts, _ := fakeToolOptions(t, []string{"systemctl"}, map[string]fakeResult{
		"systemctl is-active -- nginx": {out: "active\n"},
		"systemctl is-active -- sshd":  {out: "inactive\n", err: errors.New("exit status 3")},
	})
	opts.ServiceNames = []string{"nginx", "sshd", "nginx"}

	outcomes, err := RunChecks(t.Context(), []string{keyServiceUptime, keyMisconfig}, opts)
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	type row struct {
		key, target string
		status      CheckStatus
	}
	var got []row
	for _, o := range outcomes {
		got = append(got, row{o.Key, o.Target, o.Status})
	}
	want := []row{
		{keyServiceUptime, "nginx", StatusPass},
		{keyServiceUptime, "sshd", StatusFail},
		{keyMisconfig, "", StatusPass},
	}
	if !slices.Equal(got, want) {
		t.Errorf("outcomes = %+v, want %+v (duplicate targets run once)", got, want)
	}
}

func TestRunChecksRunsRepeatedKeysOnce(t *testing.T) {
	isolateMisconfigEnv(t)
	outcomes, err := RunChecks(t.Context(), []string{keyMisconfig, keyTimeSync, keyMisconfig}, CheckOptions{})
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	if got := outcomeKeys(outcomes); !slices.Equal(got, []string{keyMisconfig, keyTimeSync}) {
		t.Errorf("keys = %q, want each check once, in first-seen order", got)
	}
}

func TestCheckTargets(t *testing.T) {
	tests := []struct {
		name string
		key  string
		opts CheckOptions
		want []string
	}{
		{"disk default", keyDiskSpace, CheckOptions{}, []string{"/"}},
		{"disk single field", keyDiskInodes, CheckOptions{DiskPath: "/data"}, []string{"/data"}},
		{"disk list wins", keyDiskSpace, CheckOptions{DiskPath: "/data", DiskPaths: []string{"/", "/var", "/"}}, []string{"/", "/var"}},
		{"host uptime", keyServiceUptime, CheckOptions{}, []string{""}},
		{"services", keyServiceUptime, CheckOptions{ServiceNames: []string{"a", "b"}}, []string{"a", "b"}},
		{"no certificates", keyCertExpiry, CheckOptions{}, nil},
		{"certificate field", keyCertExpiry, CheckOptions{CertPath: "a.pem"}, []string{"a.pem"}},
		{"certificates", keyCertExpiry, CheckOptions{CertPaths: []string{"a.pem", "b.pem", "a.pem"}}, []string{"a.pem", "b.pem"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkTargets[tt.key].targets(tt.opts); !slices.Equal(got, tt.want) {
				t.Errorf("targets = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunChecksSanitizesTargets(t *testing.T) {
	isolateMisconfigEnv(t)
	outcomes, err := RunChecks(t.Context(), []string{keyCertExpiry}, CheckOptions{CertPaths: []string{"bad\x1b[2Jname.pem"}})
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Target != "bad?[2Jname.pem" || strings.ContainsRune(outcomes[0].Message, '\x1b') {
		t.Errorf("outcomes = %+v, want the escape character replaced in target and message", outcomes)
	}
}

func TestRunChecksSubset(t *testing.T) {
	outcomes, err := RunChecks(t.Context(), []string{keyMisconfig}, CheckOptions{})
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Key != keyMisconfig {
		t.Fatalf("RunChecks() = %+v, want single misconfig outcome", outcomes)
	}
}

func TestRunChecksUnknownKey(t *testing.T) {
	if _, err := RunChecks(t.Context(), []string{"does-not-exist"}, CheckOptions{}); err == nil {
		t.Fatal("RunChecks() expected error for unknown check key, got nil")
	}
}

func TestWorstStatus(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []CheckOutcome
		want     CheckStatus
	}{
		{name: "empty", outcomes: nil, want: StatusPass},
		{name: "all pass", outcomes: []CheckOutcome{{Status: StatusPass}, {Status: StatusPass}}, want: StatusPass},
		{name: "warn present", outcomes: []CheckOutcome{{Status: StatusPass}, {Status: StatusWarn}}, want: StatusWarn},
		{name: "fail wins", outcomes: []CheckOutcome{{Status: StatusWarn}, {Status: StatusFail}, {Status: StatusPass}}, want: StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorstStatus(tt.outcomes); got != tt.want {
				t.Errorf("WorstStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "success", err: nil, want: ExitCodePass},
		{name: "warn status", err: &ExitStatusError{Code: ExitCodeWarn}, want: ExitCodeWarn},
		{name: "fail status", err: &ExitStatusError{Code: ExitCodeFail}, want: ExitCodeFail},
		{name: "wrapped status", err: fmt.Errorf("run: %w", &ExitStatusError{Code: ExitCodeFail}), want: ExitCodeFail},
		{name: "operational error", err: errors.New("unknown check"), want: ExitCodeError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		status CheckStatus
		want   int
	}{
		{status: StatusPass, want: 0},
		{status: StatusWarn, want: 1},
		{status: StatusFail, want: 2},
	}

	for _, tt := range tests {
		if got := ExitCodeFor(tt.status); got != tt.want {
			t.Errorf("ExitCodeFor(%v) = %d, want %d", tt.status, got, tt.want)
		}
	}
}

func TestWriteOutcomesTextFailOnly(t *testing.T) {
	outcomes := []CheckOutcome{
		{Key: "a", Status: StatusPass, Message: "ok"},
		{Key: "b", Status: StatusWarn, Message: "careful"},
		{Key: "c", Status: StatusFail, Message: "broken"},
	}

	var buf bytes.Buffer
	if err := WriteOutcomesText(&buf, "", outcomes, true); err != nil {
		t.Fatalf("WriteOutcomesText() error = %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "] a ") {
		t.Errorf("fail-only output unexpectedly contains passing check: %q", out)
	}
	if !strings.Contains(out, "[WARN] b") || !strings.Contains(out, "[FAIL] c") {
		t.Errorf("fail-only output missing WARN/FAIL entries: %q", out)
	}
}

func TestWriteOutcomesJSON(t *testing.T) {
	outcomes := []CheckOutcome{{Key: "a", Status: StatusPass, Message: "ok"}}

	var buf bytes.Buffer
	if err := WriteOutcomesJSON(&buf, outcomes); err != nil {
		t.Fatalf("WriteOutcomesJSON() error = %v", err)
	}
	if !strings.Contains(buf.String(), `"key": "a"`) {
		t.Errorf("WriteOutcomesJSON() output = %q, missing expected key field", buf.String())
	}
}
