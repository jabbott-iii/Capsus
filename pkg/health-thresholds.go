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

// Threshold accessors for the disk, inode, memory, and CPU load checks. The
// defaults are in health.go, because check run uses them as flag defaults
// everywhere. Only the platforms with resource checks (Linux, macOS, and
// Windows; P3-7) use the accessors, so this file is built only there;
// otherwise the unused linter fails on the stubs in
// health-resources_other.go.

func orDefault(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

func (o CheckOptions) diskWarnPercent() float64 {
	return orDefault(o.DiskWarnPercent, defaultDiskWarnPercent)
}
func (o CheckOptions) diskFailPercent() float64 {
	return orDefault(o.DiskFailPercent, defaultDiskFailPercent)
}
func (o CheckOptions) inodeWarnPercent() float64 {
	return orDefault(o.InodeWarnPercent, defaultInodeWarnPercent)
}
func (o CheckOptions) inodeFailPercent() float64 {
	return orDefault(o.InodeFailPercent, defaultInodeFailPercent)
}
func (o CheckOptions) memWarnPercent() float64 {
	return orDefault(o.MemWarnPercent, defaultMemWarnPercent)
}
func (o CheckOptions) memFailPercent() float64 {
	return orDefault(o.MemFailPercent, defaultMemFailPercent)
}
func (o CheckOptions) loadWarnPercent() float64 {
	return orDefault(o.LoadWarnPercent, defaultLoadWarnPercent)
}
func (o CheckOptions) loadFailPercent() float64 {
	return orDefault(o.LoadFailPercent, defaultLoadFailPercent)
}
