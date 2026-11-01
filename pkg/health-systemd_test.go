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
	"errors"
	"runtime"
	"testing"
)

const notBooted = "System has not been booted with systemd as init system (PID 1). Can't operate.\nFailed to connect to bus: Host is down\n"

func TestCheckSystemdFailed(t *testing.T) {
	if runtime.GOOS != "linux" {
		got := checkSystemdFailed(t.Context(), CheckOptions{})
		assertOutcome(t, got, keySystemdFailed, StatusWarn, "failed units check requires systemd (Linux only)")
		return
	}

	listFailed := "systemctl list-units --state=failed --plain --no-legend --no-pager"
	tests := []struct {
		name        string
		installed   []string
		result      fakeResult
		wantStatus  CheckStatus
		wantMessage string
		wantCount   float64 // -1: no value
	}{
		{"systemctl missing", nil, fakeResult{}, StatusWarn, "systemctl not found in PATH", -1},
		{"none failed", []string{"systemctl"}, fakeResult{out: ""}, StatusPass, "no failed systemd units", 0},
		{
			"some failed", []string{"systemctl"},
			fakeResult{out: "nginx.service loaded failed failed A high performance web server\nlogrotate.timer loaded failed failed Daily rotation\n"},
			StatusWarn, "2 failed systemd unit(s): nginx.service, logrotate.timer", 2,
		},
		{
			"many failed are capped", []string{"systemctl"},
			fakeResult{out: "a.service x\nb.service x\nc.service x\nd.service x\ne.service x\nf.service x\ng.service x\n"},
			StatusWarn, "7 failed systemd unit(s): a.service, b.service, c.service, d.service, e.service and 2 more", 7,
		},
		{
			"bullets and noise", []string{"systemctl"},
			fakeResult{out: "● user@1000.service loaded failed failed User Manager\nWarning: some journal files were not opened\n\n●backup.mount loaded failed failed Backup\n"},
			StatusWarn, "2 failed systemd unit(s): user@1000.service, backup.mount", 2,
		},
		{"systemd not running", []string{"systemctl"}, fakeResult{out: notBooted, err: errors.New("exit status 1")}, StatusWarn, "systemd is not running on this host", -1},
		{"other error", []string{"systemctl"}, fakeResult{out: "Failed to list units: Access denied\n", err: errors.New("exit status 1")}, StatusWarn, "failed units unknown: Failed to list units: Access denied", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _ := fakeToolOptions(t, tt.installed, map[string]fakeResult{listFailed: tt.result})
			got := checkSystemdFailed(t.Context(), opts)
			assertOutcome(t, got, keySystemdFailed, tt.wantStatus, tt.wantMessage)
			assertCount(t, got, tt.wantCount)
		})
	}
}

// assertCount checks a count value; want -1 means no value.
func assertCount(t *testing.T, got CheckOutcome, want float64) {
	t.Helper()
	switch {
	case want < 0 && got.Value != nil:
		t.Errorf("value = %v, want none", *got.Value)
	case want >= 0 && (got.Value == nil || *got.Value != want || got.Unit != unitCount):
		t.Errorf("value = %v %q, want %v count", got.Value, got.Unit, want)
	}
}

func TestCheckTimeSync(t *testing.T) {
	if runtime.GOOS != "linux" {
		got := checkTimeSync(t.Context(), CheckOptions{})
		assertOutcome(t, got, keyTimeSync, StatusWarn, "time sync check requires systemd (Linux only)")
		return
	}

	show := "timedatectl show --property=NTP --property=NTPSynchronized"
	tests := []struct {
		name        string
		installed   []string
		result      fakeResult
		wantStatus  CheckStatus
		wantMessage string
	}{
		{"timedatectl missing", nil, fakeResult{}, StatusWarn, "timedatectl not found in PATH"},
		{"synchronized", []string{"timedatectl"}, fakeResult{out: "NTP=yes\nNTPSynchronized=yes\n"}, StatusPass, "system clock is synchronized"},
		{"synchronized by another daemon", []string{"timedatectl"}, fakeResult{out: "NTP=no\nNTPSynchronized=yes\n"}, StatusPass, "system clock is synchronized"},
		{"not synchronized", []string{"timedatectl"}, fakeResult{out: "NTP=yes\nNTPSynchronized=no\n"}, StatusWarn, "system clock is not synchronized"},
		{"ntp disabled", []string{"timedatectl"}, fakeResult{out: "NTP=no\nNTPSynchronized=no\n"}, StatusWarn, "system clock is not synchronized and network time sync is disabled (timedatectl set-ntp true)"},
		{"property missing", []string{"timedatectl"}, fakeResult{out: "NTP=yes\n"}, StatusWarn, "time sync unknown: timedatectl did not report NTPSynchronized"},
		{"older systemd without show", []string{"timedatectl"}, fakeResult{out: "Unknown operation show\n", err: errors.New("exit status 1")}, StatusWarn, "time sync unknown: Unknown operation show"},
		{"systemd not running", []string{"timedatectl"}, fakeResult{out: notBooted, err: errors.New("exit status 1")}, StatusWarn, "systemd is not running on this host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _ := fakeToolOptions(t, tt.installed, map[string]fakeResult{show: tt.result})
			assertOutcome(t, checkTimeSync(t.Context(), opts), keyTimeSync, tt.wantStatus, tt.wantMessage)
		})
	}
}
