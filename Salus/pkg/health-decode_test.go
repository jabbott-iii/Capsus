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
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestDarwinSysctlValue(t *testing.T) {
	full := []byte{1, 2, 3, 0}
	tests := []struct {
		name    string
		raw     string
		want    []byte
		wantErr string
	}{
		{"complete", string(full), full, ""},
		{"trailing NUL dropped by syscall.Sysctl", string(full[:3]), full, ""},
		{"too short", string(full[:2]), nil, "got 2 bytes, want 4"},
		{"too long", string(append(full, 9)), nil, "got 5 bytes, want 4"},
	}
	for _, tt := range tests {
		got, err := darwinSysctlValue(tt.raw, 4)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: error = %v, want %q", tt.name, err, tt.wantErr)
			}
			continue
		}
		if err != nil || string(got) != string(tt.want) {
			t.Errorf("%s: got %v, %v, want %v", tt.name, got, err, tt.want)
		}
	}
}

func TestDarwinSysctlUint(t *testing.T) {
	le32 := func(v uint32) string { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return string(b) }
	le64 := func(v uint64) string { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, v); return string(b) }
	for _, tt := range []struct {
		name string
		raw  string
		want uint64
	}{
		{"int", le32(16384), 16384},
		{"int with its zero high byte dropped", le32(16384)[:3], 16384},
		{"zero int", le32(0)[:3], 0},
		{"quad", le64(16 << 30), 16 << 30},
		{"quad with its zero high byte dropped", le64(16 << 30)[:7], 16 << 30},
	} {
		if got, err := darwinSysctlUint(tt.raw); err != nil || got != tt.want {
			t.Errorf("%s: darwinSysctlUint() = %d, %v, want %d", tt.name, got, err, tt.want)
		}
	}
	if _, err := darwinSysctlUint(strings.Repeat("x", 9)); err == nil {
		t.Error("darwinSysctlUint() with 9 bytes returned no error")
	}
}

func TestDarwinMemoryUsedPercent(t *testing.T) {
	const page = 16384
	memsize := uint64(16 << 30) // 1048576 pages
	// 1 GiB free, 0.5 GiB speculative, 2 GiB file cache, 0.5 GiB purgeable:
	// 4 of 16 GiB available.
	got, err := darwinMemoryUsedPercent(memsize, page, (1<<30)/page, (1<<29)/page, (2<<30)/page, (1<<29)/page)
	if err != nil || got != 75 {
		t.Errorf("darwinMemoryUsedPercent() = %v, %v, want 75", got, err)
	}
	if got, _ := darwinMemoryUsedPercent(memsize, page, memsize, 0, 0, 0); got != 0 {
		t.Errorf("more available pages than exist = %v, want 0", got)
	}
	if _, err := darwinMemoryUsedPercent(memsize, 0, 1, 1, 1, 1); err == nil {
		t.Error("page size 0 returned no error")
	}
	if _, err := darwinMemoryUsedPercent(100, page, 1, 1, 1, 1); err == nil {
		t.Error("memory smaller than a page returned no error")
	}
}

func TestDarwinLoadAvg(t *testing.T) {
	b := make([]byte, darwinLoadAvgSize)
	binary.LittleEndian.PutUint32(b[0:4], 3072) // 1.5 at fscale 2048
	binary.LittleEndian.PutUint32(b[4:8], 2048)
	binary.LittleEndian.PutUint32(b[8:12], 1024)
	binary.LittleEndian.PutUint64(b[16:24], 2048)
	if got, err := darwinLoadAvg(b); err != nil || got != 1.5 {
		t.Errorf("darwinLoadAvg() = %v, %v, want 1.5", got, err)
	}

	// The fscale's high byte is zero, so syscall.Sysctl drops it.
	b2, err := darwinSysctlValue(string(b[:darwinLoadAvgSize-1]), darwinLoadAvgSize)
	if err != nil {
		t.Fatalf("darwinSysctlValue() error = %v", err)
	}
	if got, err := darwinLoadAvg(b2); err != nil || got != 1.5 {
		t.Errorf("darwinLoadAvg(after NUL restore) = %v, %v, want 1.5", got, err)
	}

	if _, err := darwinLoadAvg(make([]byte, darwinLoadAvgSize)); err == nil {
		t.Error("darwinLoadAvg() with fscale 0 returned no error")
	}
	if _, err := darwinLoadAvg(b[:20]); err == nil {
		t.Error("darwinLoadAvg() with a short value returned no error")
	}
}

func TestDarwinSwapUsage(t *testing.T) {
	b := make([]byte, darwinSwapUsageSize)
	binary.LittleEndian.PutUint64(b[0:8], 2<<30)   // total
	binary.LittleEndian.PutUint64(b[8:16], 3<<29)  // avail
	binary.LittleEndian.PutUint64(b[16:24], 1<<29) // used
	binary.LittleEndian.PutUint32(b[24:28], 16384) // page size
	total, used, err := darwinSwapUsage(b)
	if err != nil || total != 2<<30 || used != 1<<29 {
		t.Errorf("darwinSwapUsage() = %d, %d, %v", total, used, err)
	}
	if _, _, err := darwinSwapUsage(b[:31]); err == nil {
		t.Error("darwinSwapUsage() with a short value returned no error")
	}
}

func TestDarwinBootTime(t *testing.T) {
	b := make([]byte, darwinTimevalSize)
	binary.LittleEndian.PutUint64(b[0:8], 1790000000)
	binary.LittleEndian.PutUint32(b[8:12], 250000)
	got, err := darwinBootTime(b)
	if want := time.Unix(1790000000, 250*int64(time.Millisecond)); err != nil || !got.Equal(want) {
		t.Errorf("darwinBootTime() = %v, %v, want %v", got, err, want)
	}
	if _, err := darwinBootTime(make([]byte, darwinTimevalSize)); err == nil {
		t.Error("darwinBootTime() with zero seconds returned no error")
	}
	if _, err := darwinBootTime(b[:8]); err == nil {
		t.Error("darwinBootTime() with a short value returned no error")
	}
}

func TestUsedPercent(t *testing.T) {
	for _, tt := range []struct {
		total, avail uint64
		want         float64
		ok           bool
	}{
		{100, 25, 75, true},
		{100, 100, 0, true},
		{100, 150, 0, true},
		{0, 0, 0, false},
	} {
		if got, ok := usedPercent(tt.total, tt.avail); got != tt.want || ok != tt.ok {
			t.Errorf("usedPercent(%d, %d) = %v, %v, want %v, %v", tt.total, tt.avail, got, ok, tt.want, tt.ok)
		}
	}
}

func TestCPUBusyPercent(t *testing.T) {
	for _, tt := range []struct {
		idle, kernel, user uint64
		want               float64
		ok                 bool
	}{
		{idle: 750, kernel: 800, user: 200, want: 25, ok: true}, // kernel time includes idle
		{idle: 1000, kernel: 1000, user: 0, want: 0, ok: true},
		{idle: 0, kernel: 400, user: 600, want: 100, ok: true},
		{idle: 0, kernel: 0, user: 0, ok: false},
		{idle: 2000, kernel: 1000, user: 0, ok: false},
	} {
		if got, ok := cpuBusyPercent(tt.idle, tt.kernel, tt.user); got != tt.want || ok != tt.ok {
			t.Errorf("cpuBusyPercent(%d, %d, %d) = %v, %v, want %v, %v", tt.idle, tt.kernel, tt.user, got, ok, tt.want, tt.ok)
		}
	}
}
