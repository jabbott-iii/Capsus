//go:build linux || darwin

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
	"syscall"
	"time"
)

// checkDiskSpace inspects free space on the configured mount path (defaults to "/").
func checkDiskSpace(opts CheckOptions) CheckOutcome {
	start := time.Now()
	path := opts.DiskPath
	if path == "" {
		path = defaultDiskPath
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return CheckOutcome{Key: keyDiskSpace, Status: StatusFail, Message: fmt.Sprintf("statfs %s: %v", path, err), Duration: time.Since(start)}
	}

	// Bavail is what an unprivileged user can still use; blocks reserved for
	// root count as used.
	outcome := diskSpaceOutcome(path, stat.Blocks*uint64(stat.Bsize), stat.Bavail*uint64(stat.Bsize), opts)
	outcome.Duration = time.Since(start)
	return outcome
}

// checkDiskInodes inspects inode usage on the configured mount path. A
// filesystem can run out of inodes (many small files) while it still has
// free space, and then no file can be created on it.
func checkDiskInodes(opts CheckOptions) CheckOutcome {
	start := time.Now()
	path := opts.DiskPath
	if path == "" {
		path = defaultDiskPath
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return CheckOutcome{Key: keyDiskInodes, Status: StatusFail, Message: fmt.Sprintf("statfs %s: %v", path, err), Duration: time.Since(start)}
	}
	outcome := inodeOutcome(path, stat.Files, stat.Ffree, opts)
	outcome.Duration = time.Since(start)
	return outcome
}
