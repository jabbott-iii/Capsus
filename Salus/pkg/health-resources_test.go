//go:build linux || darwin || windows

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
	"path/filepath"
	"strings"
	"testing"
)

// Tests for the resource checks on every platform that implements them.

func TestDiskSpaceOutcome(t *testing.T) {
	const gib = 1 << 30
	tests := []struct {
		name        string
		total       uint64
		avail       uint64
		opts        CheckOptions
		wantStatus  CheckStatus
		wantMessage string
		wantValue   float64
	}{
		{"plenty free", 100 * gib, 75 * gib, CheckOptions{}, StatusPass, "/data: 25.0% used (75.0% free)", 25},
		{"warn", 100 * gib, 20 * gib, CheckOptions{}, StatusWarn, "/data: 80.0% used (20.0% free)", 80},
		{"fail", 100 * gib, 10 * gib, CheckOptions{}, StatusFail, "/data: 90.0% used (10.0% free)", 90},
		{"custom thresholds", 100 * gib, 50 * gib, CheckOptions{DiskWarnPercent: 40, DiskFailPercent: 60}, StatusWarn, "/data: 50.0% used (50.0% free)", 50},
		{"available above total is clamped", gib, 2 * gib, CheckOptions{}, StatusPass, "/data: 0.0% used (100.0% free)", 0},
		{"empty volume size", 0, 0, CheckOptions{}, StatusPass, "/data: 0.0% used (100.0% free)", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diskSpaceOutcome("/data", tt.total, tt.avail, tt.opts)
			assertOutcome(t, got, keyDiskSpace, tt.wantStatus, tt.wantMessage)
			if got.Value == nil || *got.Value != tt.wantValue || got.Unit != unitPercent {
				t.Errorf("value = %v %q, want %v percent", got.Value, got.Unit, tt.wantValue)
			}
		})
	}
}

func TestCheckDiskSpaceMissingPathFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	got := checkDiskSpace(CheckOptions{DiskPath: missing})
	if got.Status != StatusFail || !strings.Contains(got.Message, missing) {
		t.Fatalf("checkDiskSpace() = {%s %q}, want FAIL naming %q", got.Status, got.Message, missing)
	}
}

func TestInodeOutcome(t *testing.T) {
	tests := []struct {
		name        string
		files       uint64
		ffree       uint64
		opts        CheckOptions
		wantStatus  CheckStatus
		wantMessage string
		wantValue   float64
	}{
		{"plenty free", 1000, 900, CheckOptions{}, StatusPass, "/: 10.0% of inodes used (900 free)", 10},
		{"warn at default threshold", 1000, 200, CheckOptions{}, StatusWarn, "/: 80.0% of inodes used (200 free)", 80},
		{"fail at default threshold", 1000, 100, CheckOptions{}, StatusFail, "/: 90.0% of inodes used (100 free)", 90},
		{"custom thresholds", 1000, 600, CheckOptions{InodeWarnPercent: 30, InodeFailPercent: 50}, StatusWarn, "/: 40.0% of inodes used (600 free)", 40},
		{"none free", 1000, 0, CheckOptions{}, StatusFail, "/: 100.0% of inodes used (0 free)", 100},
		{"free above total is clamped", 10, 20, CheckOptions{}, StatusPass, "/: 0.0% of inodes used (10 free)", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inodeOutcome("/", tt.files, tt.ffree, tt.opts)
			assertOutcome(t, got, keyDiskInodes, tt.wantStatus, tt.wantMessage)
			if got.Value == nil || *got.Value != tt.wantValue || got.Unit != unitPercent {
				t.Errorf("value = %v %s, want %v percent", got.Value, got.Unit, tt.wantValue)
			}
		})
	}

	got := inodeOutcome("/data", 0, 0, CheckOptions{})
	assertOutcome(t, got, keyDiskInodes, StatusPass, "/data: filesystem does not report an inode count")
	if got.Value != nil {
		t.Errorf("value = %v, want none when the filesystem reports no inodes", *got.Value)
	}
}

func TestCheckDiskInodes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	got := checkDiskInodes(CheckOptions{DiskPath: missing})
	if got.Status != StatusFail || !strings.Contains(got.Message, missing) {
		t.Errorf("checkDiskInodes(missing) = {%s %q}, want FAIL naming %q", got.Status, got.Message, missing)
	}
}

// TestResourceChecksReadThisHost runs the real checks against this machine
// (/proc and statfs, sysctl, or kernel32) and asserts only what holds at any
// resource level: they read their data, attach a value, and the default
// disk path works. On macOS and Windows this is how CI exercises the system
// calls.
func TestResourceChecksReadThisHost(t *testing.T) {
	for _, check := range []func(CheckOptions) CheckOutcome{checkDiskSpace, checkDiskInodes, checkMemory, checkCPULoad} {
		got := check(CheckOptions{})
		if strings.Contains(got.Message, "only supported") || strings.Contains(got.Message, "unknown") {
			t.Errorf("%s = {%s %q}, want a reading", got.Key, got.Status, got.Message)
			continue
		}
		if got.Key == keyDiskInodes && got.Value == nil {
			continue // filesystems without an inode count, and Windows
		}
		if got.Value == nil || got.Unit != unitPercent || *got.Value < 0 {
			t.Errorf("%s = %+v, want a non-negative percent value", got.Key, got)
		}
		if got.Key != keyCPULoad && *got.Value > 100 {
			t.Errorf("%s value %v is above 100 percent", got.Key, *got.Value)
		}
	}

	uptime, err := readSystemUptime()
	if err != nil || uptime <= 0 {
		t.Errorf("readSystemUptime() = %v, %v, want a positive uptime", uptime, err)
	}
	got := checkServiceUptime(t.Context(), CheckOptions{})
	if got.Status != StatusPass || got.Value == nil || got.Unit != unitSeconds {
		t.Errorf("host uptime = %+v, want PASS with seconds", got)
	}
}
