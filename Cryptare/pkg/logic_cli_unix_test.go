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
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestRunCancellableStopsOnSignal checks SEC-015's CLI mechanism in-process: a signal
// that arrives while an operation runs cancels its context, and the result is an
// *InterruptedError for that signal (exit status 128 + 1 for SIGHUP).
func TestRunCancellableStopsOnSignal(t *testing.T) {
	cmd := &cobra.Command{}
	err := runCancellable(cmd, func(ctx context.Context) error {
		// runCancellable has registered its handler by now, so the signal doesn't end
		// the test process.
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
			t.Errorf("send SIGHUP: %v", err)
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return errors.New("the context wasn't cancelled")
		}
	})
	var interrupted *InterruptedError
	if !errors.As(err, &interrupted) {
		t.Fatalf("err = %v, want *InterruptedError", err)
	}
	if interrupted.Signal != syscall.SIGHUP || interrupted.ExitCode() != 129 {
		t.Fatalf("signal %v, exit code %d; want SIGHUP and 129", interrupted.Signal, interrupted.ExitCode())
	}
	if !cmd.SilenceUsage {
		t.Fatal("usage would be printed after an interrupt")
	}
}

// TestRunCancellableWithoutSignal checks that an operation's own error, or success, is
// returned unchanged when no signal arrives.
func TestRunCancellableWithoutSignal(t *testing.T) {
	cmd := &cobra.Command{}
	if err := runCancellable(cmd, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	failure := errors.New("failed")
	if err := runCancellable(cmd, func(context.Context) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the operation's error", err)
	}
}
