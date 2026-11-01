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
	"bytes"
	"testing"
)

// testVersion is the version the tests build the root command with: what main reports
// in a build without -ldflags "-X main.version=<tag>".
const testVersion = "dev"

func TestNewRootCmdReportsVersion(t *testing.T) {
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			// The version flag is handled before any command runs, so no database is opened.
			cmd := newRootCmd(func() (*Database, error) {
				t.Fatal("--version opened the key database")
				return nil, nil
			}, "v9.9.9-test")
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{flag})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(%s) error = %v", flag, err)
			}
			if got, want := out.String(), "cryptare version v9.9.9-test\n"; got != want {
				t.Fatalf("Execute(%s) output = %q, want %q", flag, got, want)
			}
		})
	}
}
