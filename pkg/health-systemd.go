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
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// systemdNotRunning reports whether systemctl or timedatectl output says the
// host was not booted with systemd, as in containers and WSL 1.
func systemdNotRunning(output string) bool {
	return strings.Contains(output, "not been booted with systemd") ||
		strings.Contains(output, "Failed to connect to bus")
}

// checkSystemdFailed reports systemd units in the failed state. A failed unit
// is a WARN: systemd itself works, and some units fail harmlessly, but each
// one deserves a look.
func checkSystemdFailed(ctx context.Context, opts CheckOptions) CheckOutcome {
	start := time.Now()

	if runtime.GOOS != "linux" {
		return CheckOutcome{Key: keySystemdFailed, Status: StatusWarn, Message: "failed units check requires systemd (Linux only)", Duration: time.Since(start)}
	}
	if !opts.hasTool("systemctl") {
		return CheckOutcome{Key: keySystemdFailed, Status: StatusWarn, Message: "systemctl not found in PATH", Duration: time.Since(start)}
	}

	out, err := opts.command(ctx, "systemctl", "list-units", "--state=failed", "--plain", "--no-legend", "--no-pager")
	if err != nil {
		if systemdNotRunning(string(out)) {
			return CheckOutcome{Key: keySystemdFailed, Status: StatusWarn, Message: "systemd is not running on this host", Duration: time.Since(start)}
		}
		return CheckOutcome{Key: keySystemdFailed, Status: StatusWarn, Message: "failed units unknown: " + errorLine(string(out), err), Duration: time.Since(start)}
	}

	units := failedUnits(string(out))
	if len(units) == 0 {
		return CheckOutcome{Key: keySystemdFailed, Status: StatusPass, Message: "no failed systemd units", Duration: time.Since(start)}.withValue(0, unitCount)
	}
	return CheckOutcome{Key: keySystemdFailed, Status: StatusWarn, Message: fmt.Sprintf("%d failed systemd unit(s): %s", len(units), nameList(units)), Duration: time.Since(start)}.
		withValue(float64(len(units)), unitCount)
}

// failedUnits returns the unit names in systemctl list-units --plain
// --no-legend output: the first field of each row that is a valid unit name
// with a type suffix. Other lines, such as warnings, are ignored.
func failedUnits(out string) []string {
	var units []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// Skip the status bullet systemctl prints when --plain is not honored.
		name := strings.TrimPrefix(fields[0], "●")
		if name == "" && len(fields) > 1 {
			name = fields[1]
		}
		if validUnitName(name) && strings.Contains(name, ".") {
			units = append(units, name)
		}
	}
	return units
}

// checkTimeSync reports whether the system clock is synchronized, as the
// kernel sees it. That works with systemd-timesyncd, chrony, and ntpd alike.
// An unsynchronized clock is a WARN: it drifts, which breaks TLS validation,
// Kerberos and Active Directory logins, and log correlation over time.
func checkTimeSync(ctx context.Context, opts CheckOptions) CheckOutcome {
	start := time.Now()

	if runtime.GOOS != "linux" {
		return CheckOutcome{Key: keyTimeSync, Status: StatusWarn, Message: "time sync check requires systemd (Linux only)", Duration: time.Since(start)}
	}
	if !opts.hasTool("timedatectl") {
		return CheckOutcome{Key: keyTimeSync, Status: StatusWarn, Message: "timedatectl not found in PATH", Duration: time.Since(start)}
	}

	out, err := opts.command(ctx, "timedatectl", "show", "--property=NTP", "--property=NTPSynchronized")
	if err != nil {
		if systemdNotRunning(string(out)) {
			return CheckOutcome{Key: keyTimeSync, Status: StatusWarn, Message: "systemd is not running on this host", Duration: time.Since(start)}
		}
		return CheckOutcome{Key: keyTimeSync, Status: StatusWarn, Message: "time sync unknown: " + errorLine(string(out), err), Duration: time.Since(start)}
	}

	props := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if name, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[name] = value
		}
	}
	switch synced, ntp := props["NTPSynchronized"], props["NTP"]; {
	case synced == "yes":
		return CheckOutcome{Key: keyTimeSync, Status: StatusPass, Message: "system clock is synchronized", Duration: time.Since(start)}
	case synced == "no" && ntp == "no":
		return CheckOutcome{Key: keyTimeSync, Status: StatusWarn, Message: "system clock is not synchronized and network time sync is disabled (timedatectl set-ntp true)", Duration: time.Since(start)}
	case synced == "no":
		return CheckOutcome{Key: keyTimeSync, Status: StatusWarn, Message: "system clock is not synchronized", Duration: time.Since(start)}
	default:
		return CheckOutcome{Key: keyTimeSync, Status: StatusWarn, Message: "time sync unknown: timedatectl did not report NTPSynchronized", Duration: time.Since(start)}
	}
}
