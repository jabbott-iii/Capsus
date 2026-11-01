//go:build !linux && !darwin && !windows

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
	"errors"
	"time"
)

// The resource checks are implemented for Linux, macOS, and Windows. On other
// platforms they report as unsupported rather than failing the whole scan.

func checkDiskSpace(opts CheckOptions) CheckOutcome {
	return CheckOutcome{Key: keyDiskSpace, Status: StatusWarn, Message: "disk space check is only supported on Linux, macOS, and Windows"}
}

func checkDiskInodes(opts CheckOptions) CheckOutcome {
	return CheckOutcome{Key: keyDiskInodes, Status: StatusWarn, Message: "inode usage check is only supported on Linux, macOS, and Windows"}
}

func checkMemory(opts CheckOptions) CheckOutcome {
	return CheckOutcome{Key: keyMemory, Status: StatusWarn, Message: "memory check is only supported on Linux, macOS, and Windows"}
}

func checkCPULoad(opts CheckOptions) CheckOutcome {
	return CheckOutcome{Key: keyCPULoad, Status: StatusWarn, Message: "CPU load check is only supported on Linux, macOS, and Windows"}
}

func readSystemUptime() (time.Duration, error) {
	return 0, errors.New("host uptime is only supported on Linux, macOS, and Windows")
}
