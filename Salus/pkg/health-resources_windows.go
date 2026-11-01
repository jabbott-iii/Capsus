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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Windows resource checks call kernel32 directly (P3-7). The standard
// library's syscall package registers kernel32.dll as a system DLL, so
// NewLazyDLL loads it only from System32 (no DLL preloading), and no
// golang.org/x/sys dependency is needed.
var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW  = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
)

// cpuSampleInterval is how long checkCPULoad measures CPU time on Windows,
// which has no load average.
const cpuSampleInterval = time.Second

// callFailed returns the error of a kernel32 call that returned FALSE.
func callFailed(proc *syscall.LazyProc, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno != 0 {
		return fmt.Errorf("%s: %w", proc.Name, errno)
	}
	return fmt.Errorf("%s failed", proc.Name)
}

// diskFreeSpace returns the size of the volume holding path and the bytes
// available to the calling user (which honors disk quotas).
// GetDiskFreeSpaceExW takes only directories, and a UNC share needs a
// trailing backslash, so a file is replaced by its directory and a separator
// is appended, as statfs on Linux and macOS accepts any path.
func diskFreeSpace(path string) (total, avail uint64, err error) {
	if err := procGetDiskFreeSpaceExW.Find(); err != nil {
		return 0, 0, err
	}
	dir := filepath.FromSlash(path)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	if !strings.HasSuffix(dir, `\`) {
		dir += `\`
	}
	name, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, err
	}
	var freeToCaller, totalBytes, totalFree uint64
	r, _, callErr := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(&freeToCaller)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)))
	if r == 0 {
		return 0, 0, callFailed(procGetDiskFreeSpaceExW, callErr)
	}
	return totalBytes, freeToCaller, nil
}

// checkDiskSpace inspects free space on the volume holding the configured
// path. The default "/" is the root of the current drive.
func checkDiskSpace(opts CheckOptions) CheckOutcome {
	start := time.Now()
	path := opts.DiskPath
	if path == "" {
		path = defaultDiskPath
	}
	total, avail, err := diskFreeSpace(path)
	if err != nil {
		return CheckOutcome{Key: keyDiskSpace, Status: StatusFail, Message: fmt.Sprintf("disk space of %s: %v", path, err), Duration: time.Since(start)}
	}
	outcome := diskSpaceOutcome(path, total, avail, opts)
	outcome.Duration = time.Since(start)
	return outcome
}

// checkDiskInodes has nothing to measure on Windows: NTFS and ReFS allocate
// file records as needed. It still reports a missing path, like the other
// platforms.
func checkDiskInodes(opts CheckOptions) CheckOutcome {
	start := time.Now()
	path := opts.DiskPath
	if path == "" {
		path = defaultDiskPath
	}
	if _, _, err := diskFreeSpace(path); err != nil {
		return CheckOutcome{Key: keyDiskInodes, Status: StatusFail, Message: fmt.Sprintf("disk space of %s: %v", path, err), Duration: time.Since(start)}
	}
	outcome := inodeOutcome(path, 0, 0, opts)
	outcome.Duration = time.Since(start)
	return outcome
}

// memoryStatusEx is MEMORYSTATUSEX.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// checkMemory reports physical memory in use, and the commit charge against
// the commit limit (physical memory plus page files), which is the closest
// Windows measure to swap usage.
func checkMemory(opts CheckOptions) CheckOutcome {
	start := time.Now()
	if err := procGlobalMemoryStatusEx.Find(); err != nil {
		return CheckOutcome{Key: keyMemory, Status: StatusFail, Message: err.Error(), Duration: time.Since(start)}
	}
	mem := memoryStatusEx{}
	mem.Length = uint32(unsafe.Sizeof(mem))
	r, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem)))
	if r == 0 {
		return CheckOutcome{Key: keyMemory, Status: StatusFail, Message: callFailed(procGlobalMemoryStatusEx, callErr).Error(), Duration: time.Since(start)}
	}

	used, ok := usedPercent(mem.TotalPhys, mem.AvailPhys)
	if !ok {
		return CheckOutcome{Key: keyMemory, Status: StatusFail, Message: "GlobalMemoryStatusEx reported no physical memory", Duration: time.Since(start)}
	}
	commit, _ := usedPercent(mem.TotalPageFile, mem.AvailPageFile)
	status, msg := thresholdStatus(used, opts.memWarnPercent(), opts.memFailPercent(),
		fmt.Sprintf("memory %.1f%% used, commit charge %.1f%% of limit", used, commit))
	return CheckOutcome{Key: keyMemory, Status: status, Message: msg, Duration: time.Since(start)}.withValue(used, unitPercent)
}

// systemTimes returns GetSystemTimes' idle, kernel, and user time summed over
// all processors, in 100-nanosecond units.
func systemTimes() (idle, kernel, user uint64, err error) {
	if err := procGetSystemTimes.Find(); err != nil {
		return 0, 0, 0, err
	}
	var idleTime, kernelTime, userTime syscall.Filetime
	r, _, callErr := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)))
	if r == 0 {
		return 0, 0, 0, callFailed(procGetSystemTimes, callErr)
	}
	// Filetime.Nanoseconds converts to Unix time and would overflow for
	// durations, so the raw 100-ns counts are combined here.
	ticks := func(ft syscall.Filetime) uint64 { return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime) }
	return ticks(idleTime), ticks(kernelTime), ticks(userTime), nil
}

// checkCPULoad reports how busy the CPUs were over cpuSampleInterval, because
// Windows has no load average. The --load-warn and --load-fail thresholds
// apply to this busy percentage, which cannot exceed 100: with the defaults,
// WARN from 80% busy and FAIL only when every CPU was busy for the whole
// sample.
func checkCPULoad(opts CheckOptions) CheckOutcome {
	start := time.Now()
	idle1, kernel1, user1, err := systemTimes()
	if err != nil {
		return CheckOutcome{Key: keyCPULoad, Status: StatusFail, Message: err.Error(), Duration: time.Since(start)}
	}
	time.Sleep(cpuSampleInterval)
	idle2, kernel2, user2, err := systemTimes()
	if err != nil {
		return CheckOutcome{Key: keyCPULoad, Status: StatusFail, Message: err.Error(), Duration: time.Since(start)}
	}
	if idle2 < idle1 || kernel2 < kernel1 || user2 < user1 {
		return CheckOutcome{Key: keyCPULoad, Status: StatusWarn, Message: "CPU usage unknown: GetSystemTimes counters went backwards", Duration: time.Since(start)}
	}
	busy, ok := cpuBusyPercent(idle2-idle1, kernel2-kernel1, user2-user1)
	if !ok {
		return CheckOutcome{Key: keyCPULoad, Status: StatusWarn, Message: "CPU usage unknown: GetSystemTimes reported no CPU time", Duration: time.Since(start)}
	}

	status, msg := thresholdStatus(busy, opts.loadWarnPercent(), opts.loadFailPercent(),
		fmt.Sprintf("CPUs %.0f%% busy over %s across %d CPU(s) (Windows has no load average)", busy, cpuSampleInterval, runtime.NumCPU()))
	return CheckOutcome{Key: keyCPULoad, Status: status, Message: msg, Duration: time.Since(start)}.withValue(busy, unitPercent)
}

// readSystemUptime returns the time since the system started
// (GetTickCount64, which does not wrap).
func readSystemUptime() (time.Duration, error) {
	if err := procGetTickCount64.Find(); err != nil {
		return 0, err
	}
	low, high, _ := procGetTickCount64.Call()
	ms := uint64(low)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		// On 32-bit Windows the 64-bit result comes back in two registers.
		ms |= uint64(high) << 32
	}
	return time.Duration(ms) * time.Millisecond, nil
}
