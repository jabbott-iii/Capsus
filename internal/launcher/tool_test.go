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
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
)

// call records one call of a fake tool's Main.
type call struct {
	tool    string
	version string
	args    []string
}

// recorder collects the calls of fake tools in order.
type recorder struct{ calls []call }

// tool returns a fake tool whose Main records its call and returns code.
func (r *recorder) tool(name string, hasTUI, vim bool, code int) Tool {
	return Tool{
		Name:        name,
		Command:     "cmd-" + name,
		Description: "description of " + name,
		HasTUI:      hasTUI,
		Vim:         vim,
		Version:     "v1.2.3-" + name,
		Main: func(_ context.Context, version string, args []string) int {
			r.calls = append(r.calls, call{tool: name, version: version, args: args})
			return code
		},
	}
}

func TestRunPassesVersionArgsAndExitCodeUnchanged(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"no arguments", []string{}, 0},
		{"help flag", []string{"--help"}, 0},
		{"version flag", []string{"--version"}, 0},
		{"flags and values in order", []string{"add", "--title", "two words", "-x", "--k=v", "--", "--vim"}, 1},
		{"Salus WARN", []string{"check", "run"}, 1},
		{"Salus FAIL", []string{"check", "run"}, 2},
		{"Salus error", []string{"check", "run", "--only", "no-such-check"}, 3},
		{"Cryptare interrupted", []string{"encrypt", "file"}, 130},
		{"out-of-range code", []string{"x"}, 255},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r recorder
			tool := r.tool("t", true, true, tt.code)
			args := append([]string(nil), tt.args...)
			if got := tool.Run(context.Background(), args); got != tt.code {
				t.Errorf("Run returned %d, want %d", got, tt.code)
			}
			want := []call{{tool: "t", version: "v1.2.3-t", args: tt.args}}
			if !reflect.DeepEqual(r.calls, want) {
				t.Errorf("calls = %#v, want %#v", r.calls, want)
			}
		})
	}
}

func TestRunPassesNilArgsAsEmpty(t *testing.T) {
	// A Cobra command whose arguments are nil reads os.Args, so a tool must never get nil.
	var r recorder
	tool := r.tool("t", true, false, 0)
	tool.Run(context.Background(), nil)
	if len(r.calls) != 1 || r.calls[0].args == nil || len(r.calls[0].args) != 0 {
		t.Fatalf("calls = %#v, want one call with empty, non-nil args", r.calls)
	}
}

func TestRunPassesContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "value")
	var got any
	tool := Tool{Main: func(ctx context.Context, _ string, _ []string) int {
		got = ctx.Value(key{})
		return 0
	}}
	tool.Run(ctx, nil)
	if got != "value" {
		t.Errorf("Main got a context without the caller's value (got %v)", got)
	}
}

func TestWriteList(t *testing.T) {
	var r recorder
	tools := []Tool{r.tool("Alpha", true, true, 0), r.tool("Be", false, false, 0)}
	tools[1].Description = "short"
	var out bytes.Buffer
	if err := WriteList(&out, tools); err != nil {
		t.Fatalf("WriteList: %v", err)
	}
	want := "" +
		"Alpha  description of Alpha  TUI\n" +
		"Be     short                 CLI only\n"
	if out.String() != want {
		t.Errorf("WriteList wrote\n%q\nwant\n%q", out.String(), want)
	}
	if len(r.calls) != 0 {
		t.Errorf("WriteList called a tool: %#v", r.calls)
	}
}

func TestWriteVersions(t *testing.T) {
	var r recorder
	tools := []Tool{r.tool("Alpha", true, true, 0), r.tool("Be", false, false, 0)}
	var out bytes.Buffer
	if err := WriteVersions(&out, "v9.9.9", tools); err != nil {
		t.Fatalf("WriteVersions: %v", err)
	}
	want := "capsus version v9.9.9\ncmd-Alpha version v1.2.3-Alpha\ncmd-Be version v1.2.3-Be\n"
	if out.String() != want {
		t.Errorf("WriteVersions wrote %q, want %q", out.String(), want)
	}
	if len(r.calls) != 0 {
		t.Errorf("WriteVersions called a tool: %#v", r.calls)
	}
}

// failingWriter fails every write.
type failingWriter struct{}

var errWrite = errors.New("write failed")

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestWritersReportWriteErrors(t *testing.T) {
	var r recorder
	tools := []Tool{r.tool("Alpha", true, true, 0)}
	if err := WriteList(failingWriter{}, tools); !errors.Is(err, errWrite) {
		t.Errorf("WriteList error = %v, want %v", err, errWrite)
	}
	if err := WriteVersions(failingWriter{}, "v1", tools); !errors.Is(err, errWrite) {
		t.Errorf("WriteVersions error = %v, want %v", err, errWrite)
	}
}

// failOn fails only its nth write.
type failOn struct{ n, writes int }

func (f *failOn) Write(p []byte) (int, error) {
	f.writes++
	if f.writes == f.n {
		return 0, errWrite
	}
	return len(p), nil
}

func TestWriteVersionsReportsEveryFailedLine(t *testing.T) {
	var r recorder
	tools := []Tool{r.tool("Alpha", true, true, 0), r.tool("Be", false, false, 0)}
	for n := 1; n <= 1+len(tools); n++ {
		if err := WriteVersions(&failOn{n: n}, "v1", tools); !errors.Is(err, errWrite) {
			t.Errorf("write %d failing: WriteVersions error = %v, want %v", n, err, errWrite)
		}
	}
}
