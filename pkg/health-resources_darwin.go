//go:build darwin

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
	"fmt"
	"runtime"
	"syscall"
	"time"
)

// macOS resource checks read sysctl values through the standard library
// (P3-7). Disk space and inodes use statfs, shared with Linux in
// health-disk_unix.go.

// sysctlValue reads a binary sysctl value of exactly size bytes.
func sysctlValue(name string, size int) ([]byte, error) {
	raw, err := syscall.Sysctl(name)
	if err != nil {
		return nil, fmt.Errorf("sysctl %s: %w", name, err)
	}
	b, err := darwinSysctlValue(raw, size)
	if err != nil {
		return nil, fmt.Errorf("sysctl %s: %w", name, err)
	}
	return b, nil
}

// sysctlUint reads an unsigned integer sysctl value.
func sysctlUint(name string) (uint64, error) {
	raw, err := syscall.Sysctl(name)
	if err != nil {
		return 0, fmt.Errorf("sysctl %s: %w", name, err)
	}
	v, err := darwinSysctlUint(raw)
	if err != nil {
		return 0, fmt.Errorf("sysctl %s: %w", name, err)
	}
	return v, nil
}

// checkMemory reports the share of memory in use from the VM page counts
// (darwinMemoryUsedPercent). kern.memorystatus_level, what memory_pressure(1)
// prints, is not used: on macOS it counts active pages as available, so it
// measures pressure (wired and compressed memory) rather than use. The page
// size is the kernel's (vm.pagesize); hw.pagesize reports 4096 to an amd64
// binary under Rosetta. Swap comes from vm.swapusage.
func checkMemory(opts CheckOptions) CheckOutcome {
	start := time.Now()
	values := map[string]uint64{}
	for _, name := range []string{"hw.memsize", "vm.pagesize", "vm.page_free_count", "vm.page_speculative_count", "vm.page_pageable_external_count", "vm.page_purgeable_count"} {
		v, err := sysctlUint(name)
		if err != nil {
			return CheckOutcome{Key: keyMemory, Status: StatusFail, Message: err.Error(), Duration: time.Since(start)}
		}
		values[name] = v
	}
	used, err := darwinMemoryUsedPercent(values["hw.memsize"], values["vm.pagesize"], values["vm.page_free_count"],
		values["vm.page_speculative_count"], values["vm.page_pageable_external_count"], values["vm.page_purgeable_count"])
	if err != nil {
		return CheckOutcome{Key: keyMemory, Status: StatusFail, Message: "memory: " + err.Error(), Duration: time.Since(start)}
	}

	swap := "swap unknown"
	if b, err := sysctlValue("vm.swapusage", darwinSwapUsageSize); err == nil {
		if total, swapUsed, err := darwinSwapUsage(b); err == nil {
			percent := 0.0 // macOS allocates swap on demand; none may exist yet
			if total > 0 {
				percent = float64(min(swapUsed, total)) / float64(total) * 100
			}
			swap = fmt.Sprintf("swap %.1f%% used", percent)
		}
	}

	status, msg := thresholdStatus(used, opts.memWarnPercent(), opts.memFailPercent(),
		fmt.Sprintf("memory %.1f%% used, %s", used, swap))
	return CheckOutcome{Key: keyMemory, Status: status, Message: msg, Duration: time.Since(start)}.withValue(used, unitPercent)
}

// checkCPULoad compares the 1-minute load average with the number of CPUs,
// as on Linux.
func checkCPULoad(opts CheckOptions) CheckOutcome {
	start := time.Now()
	b, err := sysctlValue("vm.loadavg", darwinLoadAvgSize)
	if err != nil {
		return CheckOutcome{Key: keyCPULoad, Status: StatusFail, Message: err.Error(), Duration: time.Since(start)}
	}
	load1, err := darwinLoadAvg(b)
	if err != nil {
		return CheckOutcome{Key: keyCPULoad, Status: StatusFail, Message: err.Error(), Duration: time.Since(start)}
	}

	cpus := runtime.NumCPU()
	perCPU := load1
	if cpus > 0 {
		perCPU = load1 / float64(cpus)
	}
	status, msg := thresholdStatus(perCPU*100, opts.loadWarnPercent(), opts.loadFailPercent(),
		fmt.Sprintf("load average %.2f across %d CPU(s) (%.0f%% per-core)", load1, cpus, perCPU*100))
	return CheckOutcome{Key: keyCPULoad, Status: status, Message: msg, Duration: time.Since(start)}.withValue(perCPU*100, unitPercent)
}

// readSystemUptime returns the time since kern.boottime.
func readSystemUptime() (time.Duration, error) {
	b, err := sysctlValue("kern.boottime", darwinTimevalSize)
	if err != nil {
		return 0, err
	}
	boot, err := darwinBootTime(b)
	if err != nil {
		return 0, err
	}
	return time.Since(boot), nil
}
