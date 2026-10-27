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

package launcher

import (
	cryptare "github.com/jabbott-iii/Cryptare/pkg"
	munus "github.com/jabbott-iii/Munus/pkg"
	salus "github.com/jabbott-iii/Salus/pkg"
)

// The release tags the tool folders were imported at, which each tool reports for --version.
// /tool-sync sets them after every git subtree add or pull; "unreleased" means that the
// folder has not been imported from a release yet.
const (
	munusVersion    = "unreleased"
	cryptareVersion = "unreleased"
	salusVersion    = "unreleased"
)

// Bundled returns the registry: the bundled tools in menu order, with the descriptions of the
// tool table in README.md.
func Bundled() []Tool {
	return []Tool{
		{
			Name:        "Munus",
			Command:     "munus",
			Description: "Terminal task manager: tasks with deadlines, statuses and tags in a local SQLite database",
			HasTUI:      true,
			Vim:         true,
			Version:     munusVersion,
			Main:        munus.Main,
		},
		{
			Name:        "Cryptare",
			Command:     "cryptare",
			Description: "Encrypt, decrypt, compress and extract files and folders; manage password-protected keys",
			HasTUI:      true,
			Vim:         true,
			Version:     cryptareVersion,
			Main:        cryptare.Main,
		},
		{
			Name:        "Salus",
			Command:     "salus",
			Description: "Environment health checker: disk, memory, CPU, Docker, Kubernetes, services and certificates, with PASS/WARN/FAIL results",
			Version:     salusVersion,
			Main:        salus.Main,
		},
	}
}
