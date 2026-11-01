//go:build !windows

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
	"io/fs"
	"os"
	"syscall"
)

// keepGroup gives tmp the group of the file it replaces. Changing to a group
// the user is not a member of is not permitted (root may); then the new file
// keeps the user's group, and only its permission bits carry over.
func keepGroup(tmp *os.File, existing fs.FileInfo) error {
	st, ok := existing.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := tmp.Chown(-1, int(st.Gid)); err != nil && !errors.Is(err, fs.ErrPermission) {
		return err
	}
	return nil
}
