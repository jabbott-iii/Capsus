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

import "testing"

// wslMounts is a /proc/self/mounts excerpt from WSL: C: through 9p (WSL 2),
// D: through drvfs (WSL 1), and ordinary Linux mounts around them.
const wslMounts = `/dev/sdc / ext4 rw,relatime,discard,errors=remount-ro,data=ordered 0 0
C:\134 /mnt/c 9p rw,noatime,dirsync,aname=drvfs;path=C:\134;uid=1000;gid=1000;symlinkroot=/mnt/,mmap,access=client,msize=65536,trans=fd,rfd=5,wfd=5 0 0
D: /mnt/d drvfs rw,noatime,uid=1000,gid=1000 0 0
tmpfs /mnt/c/overlay tmpfs rw,nosuid,nodev 0 0
server:/export /mnt/nfs nfs rw,relatime 0 0
share /mnt/share 9p rw,relatime,trans=virtio 0 0
/dev/sdd /mnt/with\040space ext4 rw,relatime 0 0
E: /windows\040drives/e drvfs rw,noatime 0 0
none /mnt/d drvfs rw 0 0
/dev/sde /mnt/d ext4 rw 0 0
`

func TestOnDrvfs(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/mnt/c", true},
		{"/mnt/c/Windows/System32", true},
		{"/mnt/c/Windows/../Users", true},
		{"/mnt/c/overlay/bin", false},
		{"/mnt/cdrom/bin", false},
		{"/usr/local/bin", false},
		{"/mnt/nfs/bin", false},
		{"/mnt/share/bin", false},
		{"/mnt/with space/bin", false},
		// wsl.conf can move the automount root to a path with a space.
		{"/windows drives/e/tools", true},
		// The last mount on /mnt/d (ext4) hides the drvfs mounts below it.
		{"/mnt/d/tools", false},
	}

	for _, tt := range tests {
		if got := onDrvfs(tt.path, wslMounts); got != tt.want {
			t.Errorf("onDrvfs(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}

	wsl1 := "/dev/sdb / ext4 rw 0 0\nD: /mnt/d drvfs rw,noatime 0 0\n"
	if !onDrvfs("/mnt/d/tools", wsl1) {
		t.Error("onDrvfs(/mnt/d/tools) = false for a WSL 1 drvfs mount, want true")
	}
}

func TestUnescapeMountPath(t *testing.T) {
	tests := map[string]string{
		`/mnt/with\040space`: "/mnt/with space",
		`C:\134`:             `C:\`,
		`/tab\011end`:        "/tab\tend",
		`/trailing\04`:       `/trailing\04`,
		`/plain`:             "/plain",
	}
	for in, want := range tests {
		if got := unescapeMountPath(in); got != want {
			t.Errorf("unescapeMountPath(%q) = %q, want %q", in, got, want)
		}
	}
}
