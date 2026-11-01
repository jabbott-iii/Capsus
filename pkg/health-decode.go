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
	"fmt"
	"time"
)

// Pure decoding for the macOS and Windows resource checks
// (health-resources_darwin.go, health-resources_windows.go). This file has no
// build constraint so that its tests run on every platform, including the
// Linux machines most development happens on.

// Sizes of the macOS sysctl values the checks read (LP64; macOS runs only on
// little-endian amd64 and arm64).
const (
	darwinLoadAvgSize   = 24 // struct loadavg: fixpt_t ldavg[3]; (pad); long fscale
	darwinSwapUsageSize = 32 // struct xsw_usage: u_int64_t total, avail, used; u_int32_t pagesize; boolean_t encrypted
	darwinTimevalSize   = 16 // struct timeval: time_t tv_sec; suseconds_t tv_usec; (pad)
)

// darwinSysctlValue returns a binary sysctl value of exactly size bytes from
// what syscall.Sysctl returned. syscall.Sysctl drops one trailing NUL byte,
// meant for strings; for these values that byte is the high byte of a
// little-endian integer or struct padding, and it is restored.
func darwinSysctlValue(raw string, size int) ([]byte, error) {
	b := []byte(raw)
	if len(b) == size-1 {
		b = append(b, 0)
	}
	if len(b) != size {
		return nil, fmt.Errorf("got %d bytes, want %d", len(b), size)
	}
	return b, nil
}

// darwinSysctlUint returns an unsigned integer sysctl value of 4 or 8 bytes
// from what syscall.Sysctl returned (see darwinSysctlValue for the dropped
// NUL byte). Some of these values are int and others quad in XNU, so both
// sizes are accepted.
func darwinSysctlUint(raw string) (uint64, error) {
	switch n := len(raw); {
	case n <= 4:
		b, err := darwinSysctlValue(raw, 4)
		if err != nil {
			return 0, err
		}
		return uint64(binary.LittleEndian.Uint32(b)), nil
	case n <= 8:
		b, err := darwinSysctlValue(raw, 8)
		if err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint64(b), nil
	default:
		return 0, fmt.Errorf("got %d bytes, want 4 or 8", n)
	}
}

// darwinMemoryUsedPercent returns the share of physical memory (memsize bytes,
// in pages of pageSize) that cannot be handed out without compressing or
// swapping: everything except free and speculative pages, file-backed pages
// (the "Cached Files" of Activity Monitor), and purgeable pages. That is close
// to Activity Monitor's "Memory Used" and to Linux's MemTotal - MemAvailable.
func darwinMemoryUsedPercent(memsize, pageSize, free, speculative, fileBacked, purgeable uint64) (float64, error) {
	if pageSize == 0 || memsize < pageSize {
		return 0, fmt.Errorf("invalid memory size %d or page size %d", memsize, pageSize)
	}
	percent, _ := usedPercent(memsize/pageSize, free+speculative+fileBacked+purgeable)
	return percent, nil
}

// darwinLoadAvg returns the 1-minute load average from vm.loadavg.
func darwinLoadAvg(b []byte) (float64, error) {
	if len(b) != darwinLoadAvgSize {
		return 0, fmt.Errorf("vm.loadavg: got %d bytes, want %d", len(b), darwinLoadAvgSize)
	}
	load1 := binary.LittleEndian.Uint32(b[0:4])
	fscale := int64(binary.LittleEndian.Uint64(b[16:24]))
	if fscale <= 0 {
		return 0, fmt.Errorf("vm.loadavg: invalid fscale %d", fscale)
	}
	return float64(load1) / float64(fscale), nil
}

// darwinSwapUsage returns total and used swap bytes from vm.swapusage.
func darwinSwapUsage(b []byte) (total, used uint64, err error) {
	if len(b) != darwinSwapUsageSize {
		return 0, 0, fmt.Errorf("vm.swapusage: got %d bytes, want %d", len(b), darwinSwapUsageSize)
	}
	return binary.LittleEndian.Uint64(b[0:8]), binary.LittleEndian.Uint64(b[16:24]), nil
}

// darwinBootTime returns the boot time from kern.boottime.
func darwinBootTime(b []byte) (time.Time, error) {
	if len(b) != darwinTimevalSize {
		return time.Time{}, fmt.Errorf("kern.boottime: got %d bytes, want %d", len(b), darwinTimevalSize)
	}
	sec := int64(binary.LittleEndian.Uint64(b[0:8]))
	usec := int64(int32(binary.LittleEndian.Uint32(b[8:12])))
	if sec <= 0 {
		return time.Time{}, fmt.Errorf("kern.boottime: invalid seconds %d", sec)
	}
	return time.Unix(sec, usec*int64(time.Microsecond)), nil
}

// usedPercent returns how much of total is not available, in percent, or
// false when total is zero.
func usedPercent(total, avail uint64) (float64, bool) {
	if total == 0 {
		return 0, false
	}
	if avail > total {
		avail = total
	}
	return float64(total-avail) / float64(total) * 100, true
}

// cpuBusyPercent returns the share of CPU time that was not idle, from the
// changes in Windows' GetSystemTimes counters over a sample (in any one unit).
// Windows counts idle time as part of kernel time. It reports false when no
// time passed or the counters went backwards.
func cpuBusyPercent(idle, kernel, user uint64) (float64, bool) {
	total := kernel + user
	if total == 0 || idle > total {
		return 0, false
	}
	return float64(total-idle) / float64(total) * 100, true
}
