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
	"io"
	"strings"
	"testing"
)

func TestRootCmdVersionFlagReportsBuildVersion(t *testing.T) {
	// --version is handled by Cobra before any command touches the database.
	root := NewVersionedRootCmd(nil, "v1.2.3-test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--version) error = %v", err)
	}

	want := "salus version v1.2.3-test"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("--version output = %q, want %q", got, want)
	}
}
