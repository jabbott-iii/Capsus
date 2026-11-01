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

import "fmt"

// Classification shared by the platform implementations of the resource
// checks (Linux and macOS use statfs, Windows uses kernel32). Platforms
// without an implementation use the stubs in health-resources_other.go.

// diskSpaceOutcome classifies space usage on path from its total size and
// the bytes available to an unprivileged user, both in bytes.
func diskSpaceOutcome(path string, total, avail uint64, opts CheckOptions) CheckOutcome {
	usedPercent := 0.0
	if total > 0 {
		if avail > total {
			avail = total
		}
		usedPercent = (1 - float64(avail)/float64(total)) * 100
	}
	status, msg := thresholdStatus(usedPercent, opts.diskWarnPercent(), opts.diskFailPercent(),
		fmt.Sprintf("%s: %.1f%% used (%.1f%% free)", path, usedPercent, 100-usedPercent))
	return CheckOutcome{Key: keyDiskSpace, Status: status, Message: msg}.withValue(usedPercent, unitPercent)
}

// inodeOutcome classifies inode usage from statfs's total (files) and free
// (ffree) inode counts.
func inodeOutcome(path string, files, ffree uint64, opts CheckOptions) CheckOutcome {
	if files == 0 {
		// Filesystems that allocate inodes dynamically (for example btrfs, or
		// NTFS on Windows) report no inode count, so they cannot run out in
		// this sense.
		return CheckOutcome{Key: keyDiskInodes, Status: StatusPass, Message: fmt.Sprintf("%s: filesystem does not report an inode count", path)}
	}
	if ffree > files {
		ffree = files
	}
	usedPercent := float64(files-ffree) / float64(files) * 100
	status, msg := thresholdStatus(usedPercent, opts.inodeWarnPercent(), opts.inodeFailPercent(),
		fmt.Sprintf("%s: %.1f%% of inodes used (%d free)", path, usedPercent, ffree))
	return CheckOutcome{Key: keyDiskInodes, Status: status, Message: msg}.withValue(usedPercent, unitPercent)
}
