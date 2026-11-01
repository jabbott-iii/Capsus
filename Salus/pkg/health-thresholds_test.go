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

import "testing"

func TestThresholdAccessorsUseDefaultsAndOverrides(t *testing.T) {
	defaults := CheckOptions{}
	overrides := CheckOptions{DiskWarnPercent: 50, DiskFailPercent: 60, InodeWarnPercent: 40, InodeFailPercent: 45, MemWarnPercent: 55, MemFailPercent: 65, LoadWarnPercent: 70, LoadFailPercent: 150}

	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"disk warn default", defaults.diskWarnPercent(), defaultDiskWarnPercent},
		{"disk fail default", defaults.diskFailPercent(), defaultDiskFailPercent},
		{"mem warn default", defaults.memWarnPercent(), defaultMemWarnPercent},
		{"mem fail default", defaults.memFailPercent(), defaultMemFailPercent},
		{"load warn default", defaults.loadWarnPercent(), defaultLoadWarnPercent},
		{"load fail default", defaults.loadFailPercent(), defaultLoadFailPercent},
		{"disk warn override", overrides.diskWarnPercent(), 50},
		{"disk fail override", overrides.diskFailPercent(), 60},
		{"mem warn override", overrides.memWarnPercent(), 55},
		{"mem fail override", overrides.memFailPercent(), 65},
		{"load warn override", overrides.loadWarnPercent(), 70},
		{"load fail override", overrides.loadFailPercent(), 150},
		{"inode warn default", defaults.inodeWarnPercent(), defaultInodeWarnPercent},
		{"inode fail default", defaults.inodeFailPercent(), defaultInodeFailPercent},
		{"inode warn override", overrides.inodeWarnPercent(), 40},
		{"inode fail override", overrides.inodeFailPercent(), 45},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}
