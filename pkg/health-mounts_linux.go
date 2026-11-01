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
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// syntheticModes reports whether path is on a WSL drvfs mount, such as the
// Windows drive at /mnt/c. Its POSIX mode bits are made up (commonly 0777)
// while Windows ACLs decide access, so the misconfig permission rules skip
// such paths as they skip Windows itself.
func syntheticModes(path string) bool {
	mounts, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return false
	}
	return onDrvfs(path, string(mounts))
}

// onDrvfs reports whether path is on a drvfs mount according to mounts, the
// contents of /proc/self/mounts. WSL 1 reports the type drvfs; WSL 2 mounts
// the drives as 9p with aname=drvfs. The longest mount point that contains
// path decides, and a later mount on the same point hides an earlier one.
func onDrvfs(path, mounts string) bool {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	path = filepath.Clean(path)

	longest, drvfs := -1, false
	for _, line := range strings.Split(mounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mountPoint := unescapeMountPath(fields[1])
		if len(mountPoint) < longest || !pathWithin(path, mountPoint) {
			continue
		}
		longest = len(mountPoint)
		drvfs = fields[2] == "drvfs" || (fields[2] == "9p" && strings.Contains(fields[3], "aname=drvfs"))
	}
	return drvfs
}

// pathWithin reports whether path is dir or below it.
func pathWithin(path, dir string) bool {
	return dir == "/" || path == dir || strings.HasPrefix(path, dir+"/")
}

// unescapeMountPath decodes the octal escapes that /proc/self/mounts uses for
// spaces, tabs, newlines, and backslashes in paths, such as \040.
func unescapeMountPath(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if c, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(c))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
