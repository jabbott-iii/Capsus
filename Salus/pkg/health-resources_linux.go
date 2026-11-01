//go:build linux

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
	"runtime"
	"strconv"
	"strings"
	"time"
)

// meminfo holds the subset of /proc/meminfo used by the memory check.
type meminfo struct {
	totalKB     uint64
	availableKB uint64
	swapTotalKB uint64
	swapFreeKB  uint64

	// missing names the fields the usage calculation needs (MemTotal,
	// MemAvailable) that were absent or malformed; empty when both parsed.
	missing string
}

func (m meminfo) swapPercent() float64 {
	if m.swapTotalKB == 0 {
		return 0
	}
	used := m.swapTotalKB - m.swapFreeKB
	return float64(used) / float64(m.swapTotalKB) * 100
}

func readMeminfo() (meminfo, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return meminfo{}, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	return parseMeminfo(data), nil
}

// parseMeminfo extracts the fields used by the memory check from /proc/meminfo
// contents. Missing or malformed lines leave the corresponding field at zero;
// missing names the absent fields that the usage calculation needs.
func parseMeminfo(data []byte) meminfo {
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSuffix(parts[0], ":")
		v, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			continue
		}
		values[name] = v
	}

	var missing []string
	for _, name := range []string{"MemTotal", "MemAvailable"} {
		if _, ok := values[name]; !ok {
			missing = append(missing, name)
		}
	}
	return meminfo{
		totalKB:     values["MemTotal"],
		availableKB: values["MemAvailable"],
		swapTotalKB: values["SwapTotal"],
		swapFreeKB:  values["SwapFree"],
		missing:     strings.Join(missing, " and "),
	}
}

func checkMemory(opts CheckOptions) CheckOutcome {
	start := time.Now()
	info, err := readMeminfo()
	if err != nil {
		return CheckOutcome{Key: keyMemory, Status: StatusFail, Message: err.Error(), Duration: time.Since(start)}
	}
	return memoryOutcome(info, opts, start)
}

// memoryOutcome classifies parsed /proc/meminfo. Usage needs MemTotal and
// MemAvailable (which kernels before 3.14 lack). Without them, or with a zero
// total, usage is unknown: that is a WARN with no value, not a made-up 0%
// (PASS) or 100% (FAIL).
func memoryOutcome(info meminfo, opts CheckOptions, start time.Time) CheckOutcome {
	if info.missing != "" {
		return CheckOutcome{Key: keyMemory, Status: StatusWarn, Message: "memory usage unknown: /proc/meminfo has no " + info.missing, Duration: time.Since(start)}
	}
	if info.totalKB == 0 {
		return CheckOutcome{Key: keyMemory, Status: StatusWarn, Message: "memory usage unknown: /proc/meminfo reports MemTotal 0", Duration: time.Since(start)}
	}

	usedPercent := (1 - float64(info.availableKB)/float64(info.totalKB)) * 100
	status, msg := thresholdStatus(usedPercent, opts.memWarnPercent(), opts.memFailPercent(),
		fmt.Sprintf("memory %.1f%% used, swap %.1f%% used", usedPercent, info.swapPercent()))

	return CheckOutcome{Key: keyMemory, Status: status, Message: msg, Duration: time.Since(start)}.withValue(usedPercent, unitPercent)
}

func readLoadAverage() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	return parseLoadAverage(data)
}

// parseLoadAverage returns the 1-minute load average from /proc/loadavg contents.
func parseLoadAverage(data []byte) (float64, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected /proc/loadavg contents")
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse load average: %w", err)
	}
	return load1, nil
}

func checkCPULoad(opts CheckOptions) CheckOutcome {
	start := time.Now()
	load1, err := readLoadAverage()
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

func readSystemUptime() (time.Duration, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("read /proc/uptime: %w", err)
	}
	return parseUptime(data)
}

// parseUptime returns the system uptime from /proc/uptime contents.
func parseUptime(data []byte) (time.Duration, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected /proc/uptime contents")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse uptime: %w", err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
