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
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// endedByStopSignal reports whether err says the tool was ended by SIGINT or
// SIGTERM. Salus itself stops tools with SIGKILL.
func endedByStopSignal(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && (status.Signal() == syscall.SIGINT || status.Signal() == syscall.SIGTERM)
}

// stopProcessGroup starts the tool as the leader of a new process group and
// makes cancellation kill the whole group, so processes the tool started
// (credential plugins, wrapper scripts) do not outlive it. Being in its own
// group, the tool also does not receive a terminal's Ctrl-C directly; check
// run stops it through its context instead.
func stopProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone // the whole group has already exited
		}
		return err
	}
}
