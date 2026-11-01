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
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCheckRunStopsOnSIGTERM(t *testing.T) {
	runChecks := func(ctx context.Context, _ []string, _ CheckOptions) ([]CheckOutcome, error) {
		// check run handles SIGTERM by now; without that, this would end the
		// test binary.
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatalf("send SIGTERM: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-time.After(10 * time.Second):
			t.Error("SIGTERM did not cancel the run's context")
			return nil, nil
		}
	}
	cmd := newCheckRunCmdWith(neverOpen(t), runChecks)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--no-save"})
	err := cmd.Execute()

	if got := ExitCode(err); got != ExitCodeError {
		t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
	}
	if want := "interrupted (terminated signal received)"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

func TestStopSignalContextKeepsIgnoredSignalIgnored(t *testing.T) {
	// A shell starts background jobs with SIGINT ignored; check run must not
	// start handling it.
	signal.Ignore(os.Interrupt)
	t.Cleanup(func() { signal.Reset(os.Interrupt) })

	ctx, stop := stopSignalContext(t.Context())
	defer stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	select {
	case <-ctx.Done():
		t.Errorf("an ignored SIGINT cancelled the context (%v)", context.Cause(ctx))
	case <-time.After(200 * time.Millisecond):
	}
}
