//go:build unix

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
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunExternalKillsProcessGroupOnTimeout(t *testing.T) {
	// The shell starts a background process that inherits its output pipe,
	// like a kubectl credential plugin, and both hang.
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	start := time.Now()
	_, err := runExternal(ctx, "/bin/sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("runExternal() took %s, want it to return soon after the 1s timeout", elapsed)
	}
	if err == nil {
		t.Error("runExternal() error = nil, want the kill to be reported")
	}

	if runtime.GOOS != "linux" {
		return // the liveness check below reads /proc
	}
	data, readErr := os.ReadFile(pidFile)
	if errors.Is(readErr, os.ErrNotExist) {
		t.Skip("the shell did not start its background process before the timeout")
	}
	if readErr != nil {
		t.Fatalf("read child pid: %v", readErr)
	}
	pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if convErr != nil {
		t.Fatalf("parse child pid %q: %v", data, convErr)
	}
	// Dead means gone, or a zombie waiting to be reaped by init.
	deadline := time.Now().Add(5 * time.Second)
	for {
		stat, statErr := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if errors.Is(statErr, os.ErrNotExist) || (statErr == nil && strings.Contains(string(stat), ") Z ")) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("background process %d is still running after the timeout", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunExternalStopsWaitingForLeftoverProcess(t *testing.T) {
	// The tool succeeds, but a process it started keeps the pipe open.
	start := time.Now()
	out, err := runExternal(t.Context(), "/bin/sh", "-c", "sleep 5 & echo ok")
	if err != nil {
		t.Fatalf("runExternal() error = %v, want nil", err)
	}
	if string(out) != "ok\n" {
		t.Errorf("runExternal() output = %q, want %q", out, "ok\n")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("runExternal() took %s, want about commandWaitDelay (%s)", elapsed, commandWaitDelay)
	}
}

func TestRunExternalRejectsOversizedOutput(t *testing.T) {
	out, err := runExternal(t.Context(), "/bin/sh", "-c", "head -c 9000000 /dev/zero")
	if err == nil || !strings.Contains(err.Error(), "printed more than 8 MiB of output") {
		t.Errorf("runExternal() error = %v, want the output limit error", err)
	}
	if out != nil {
		t.Errorf("runExternal() returned %d bytes, want none", len(out))
	}
}

func TestRunExternalReturnsCombinedOutput(t *testing.T) {
	out, err := runExternal(t.Context(), "/bin/sh", "-c", "echo out; echo err >&2; exit 3")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Errorf("runExternal() error = %v, want exit status 3", err)
	}
	if !strings.Contains(string(out), "out\n") || !strings.Contains(string(out), "err\n") {
		t.Errorf("runExternal() output = %q, want stdout and stderr", out)
	}
}

func TestEndedByStopSignal(t *testing.T) {
	tests := []struct {
		script string
		want   bool
	}{
		{"kill -TERM $$", true},
		{"kill -INT $$", true},
		{"kill -KILL $$", false},
		{"exit 143", false}, // a status, not a signal
	}
	for _, tt := range tests {
		err := exec.Command("/bin/sh", "-c", tt.script).Run()
		if got := endedByStopSignal(err); got != tt.want {
			t.Errorf("endedByStopSignal(%q: %v) = %v, want %v", tt.script, err, got, tt.want)
		}
	}
	if endedByStopSignal(nil) || endedByStopSignal(errors.New("other")) {
		t.Error("endedByStopSignal() = true for an error that is not an exit status")
	}
}

func TestRunExternalWaitsForRunStopAfterStopSignal(t *testing.T) {
	// The tool and Salus get SIGTERM together; Salus's handler cancels the
	// run shortly after the tool has died.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)

	if _, err := runExternal(ctx, "/bin/sh", "-c", "kill -TERM $$"); !endedByStopSignal(err) {
		t.Fatalf("runExternal() error = %v, want the tool's SIGTERM", err)
	}
	if ctx.Err() == nil {
		t.Error("runExternal() returned before the run was cancelled, want it to wait up to stopSignalGrace")
	}
}
