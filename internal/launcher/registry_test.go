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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBundled(t *testing.T) {
	type entry struct {
		name, command string
		hasTUI, vim   bool
		version       string
	}
	var got []entry
	// capsus' own subcommands; a tool must not shadow them.
	taken := map[string]bool{"list": true, "help": true}
	for _, tool := range Bundled() {
		got = append(got, entry{tool.Name, tool.Command, tool.HasTUI, tool.Vim, tool.Version})
		if tool.Main == nil {
			t.Errorf("%s has no Main", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
		if taken[tool.Command] {
			t.Errorf("command %q is taken", tool.Command)
		}
		taken[tool.Command] = true
	}
	want := []entry{
		{"Munus", "munus", true, true, munusVersion},
		{"Cryptare", "cryptare", true, true, cryptareVersion},
		{"Salus", "salus", false, false, salusVersion},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Bundled() = %+v, want %+v", got, want)
	}
}

// TestDescriptionsMatchTheREADME pins the registry's descriptions to the tool table in
// README.md ("| <Name> | <Description> |").
func TestDescriptionsMatchTheREADME(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	for _, tool := range Bundled() {
		if row := "| " + tool.Name + " | " + tool.Description + " |"; !strings.Contains(string(readme), row) {
			t.Errorf("README.md has no tool table row starting %q", row)
		}
	}
}

func TestBundledReturnsAFreshRegistry(t *testing.T) {
	first := Bundled()
	first[0].Name = "changed"
	if Bundled()[0].Name != "Munus" {
		t.Errorf("a change to one Bundled() result reached the next one")
	}
}
