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
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestMemoryStatusExLayout(t *testing.T) {
	// GlobalMemoryStatusEx checks dwLength against sizeof(MEMORYSTATUSEX).
	if size := unsafe.Sizeof(memoryStatusEx{}); size != 64 {
		t.Errorf("sizeof(memoryStatusEx) = %d, want 64", size)
	}
}

// TestWindowsKernel32Readable calls the real kernel32 functions on the
// Windows CI runner and checks only their shape and plausibility.
func TestWindowsKernel32Readable(t *testing.T) {
	total, avail, err := diskFreeSpace("/")
	if err != nil || total == 0 || avail > total {
		t.Errorf(`diskFreeSpace("/") = %d, %d, %v`, total, avail, err)
	}
	dir := t.TempDir()
	if _, _, err := diskFreeSpace(dir); err != nil {
		t.Errorf("diskFreeSpace(temp dir) error = %v", err)
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := diskFreeSpace(file); err != nil {
		t.Errorf("diskFreeSpace(file) error = %v, want the file's directory to be used", err)
	}
	if uptime, err := readSystemUptime(); err != nil || uptime <= 0 {
		t.Errorf("readSystemUptime() = %v, %v", uptime, err)
	}
	idle, kernel, user, err := systemTimes()
	if err != nil || kernel+user == 0 || idle > kernel {
		t.Errorf("systemTimes() = %d, %d, %d, %v", idle, kernel, user, err)
	}
	got := checkDiskInodes(CheckOptions{})
	assertOutcome(t, got, keyDiskInodes, StatusPass, "/: filesystem does not report an inode count")
}
