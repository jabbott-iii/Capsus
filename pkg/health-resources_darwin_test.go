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
	"testing"
	"time"
)

// TestDarwinSysctlsReadable reads the real sysctl values on the macOS CI
// runner and checks only their shape and plausibility.
func TestDarwinSysctlsReadable(t *testing.T) {
	pageSize, err := sysctlUint("vm.pagesize")
	if err != nil || (pageSize != 4096 && pageSize != 16384) {
		t.Errorf("vm.pagesize = %d, %v, want 4096 or 16384", pageSize, err)
	}
	memsize, err := sysctlUint("hw.memsize")
	if err != nil || memsize < 1<<30 {
		t.Errorf("hw.memsize = %d, %v, want at least 1 GiB", memsize, err)
	}
	for _, name := range []string{"vm.page_free_count", "vm.page_speculative_count", "vm.page_pageable_external_count", "vm.page_purgeable_count"} {
		if pages, err := sysctlUint(name); err != nil || (pageSize > 0 && pages > memsize/pageSize) {
			t.Errorf("%s = %d, %v, want at most %d pages", name, pages, err, memsize/max(pageSize, 1))
		}
	}
	if b, err := sysctlValue("vm.loadavg", darwinLoadAvgSize); err != nil {
		t.Errorf("vm.loadavg: %v", err)
	} else if load, err := darwinLoadAvg(b); err != nil || load < 0 {
		t.Errorf("load average = %v, %v", load, err)
	}
	if b, err := sysctlValue("vm.swapusage", darwinSwapUsageSize); err != nil {
		t.Errorf("vm.swapusage: %v", err)
	} else if total, used, err := darwinSwapUsage(b); err != nil || used > total {
		t.Errorf("swap = %d used of %d, %v", used, total, err)
	}
	if b, err := sysctlValue("kern.boottime", darwinTimevalSize); err != nil {
		t.Errorf("kern.boottime: %v", err)
	} else if boot, err := darwinBootTime(b); err != nil || boot.After(time.Now()) || time.Since(boot) > 10*365*24*time.Hour {
		t.Errorf("boot time = %v, %v", boot, err)
	}
}
