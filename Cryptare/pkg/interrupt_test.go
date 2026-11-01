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
	"fmt"
	"os"
	"syscall"
	"testing"
)

// helperEnv makes TestRunMain run cryptare, for tests that need it in a subprocess.
const helperEnv = "CRYPTARE_TEST_RUN_MAIN"

// TestRunMain isn't a test on its own: a subprocess started with helperEnv set runs
// cryptare here, through Run as main does, with the arguments that follow "--".
func TestRunMain(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("runs only as a helper process")
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	os.Args = append([]string{"cryptare"}, args...)
	os.Exit(Run(testVersion))
}

// TestExitCode checks the exit status for a command stopped by a signal (SEC-015):
// 128 plus the signal's number, as shells report it, and 1 for any other error.
func TestExitCode(t *testing.T) {
	interrupted := &InterruptedError{Signal: syscall.SIGTERM}
	tests := []struct {
		err  error
		want int
	}{
		{&InterruptedError{Signal: os.Interrupt}, 130},
		{interrupted, 143},
		{fmt.Errorf("decrypt: %w", interrupted), 143},
		{errors.New("other failure"), 1},
	}
	for _, tc := range tests {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}
