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

import "github.com/spf13/cobra"

// NewVersionedRootCmd builds the Salus command tree and attaches the build
// version, which makes Cobra provide the --version flag. Main passes the
// version it is given; the salus binary passes main.version, which release
// builds set with -ldflags "-X main.version=vX.Y.Z" (see
// .github/workflows/cd.yml).
func NewVersionedRootCmd(openDB DatabaseOpener, version string) *cobra.Command {
	root := NewRootCmd(openDB)
	root.Version = version
	return root
}
