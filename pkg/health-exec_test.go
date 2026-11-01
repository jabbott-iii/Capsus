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
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCommandReportsTimeout(t *testing.T) {
	opts := CheckOptions{
		CommandTimeout: 20 * time.Millisecond,
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			<-ctx.Done()
			return []byte("partial line"), ctx.Err()
		},
	}

	out, err := opts.command(t.Context(), "kubectl", "get", "nodes")
	if err == nil || err.Error() != "kubectl timed out after 20ms" {
		t.Errorf("command() error = %v, want %q", err, "kubectl timed out after 20ms")
	}
	if out != nil {
		t.Errorf("command() output = %q, want none after a timeout", out)
	}
}

func TestCommandReportsWhyTheCheckStopped(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errCheckTimeout)
	opts := CheckOptions{
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	_, err := opts.command(ctx, "docker", "info")
	if want := "docker stopped: check time limit reached"; err == nil || err.Error() != want {
		t.Errorf("command() error = %v, want %q", err, want)
	}
}

func TestCommandKeepsToolErrorAndOutput(t *testing.T) {
	toolErr := errors.New("exit status 1")
	opts := CheckOptions{
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("Cannot connect to the Docker daemon"), toolErr
		},
	}

	out, err := opts.command(t.Context(), "docker", "info")
	if !errors.Is(err, toolErr) || string(out) != "Cannot connect to the Docker daemon" {
		t.Errorf("command() = %q, %v; want the tool's output and error", out, err)
	}
}

func TestLimitedBuffer(t *testing.T) {
	b := &limitedBuffer{limit: 8}
	for _, chunk := range []string{"abc", "defgh", "ij", "k"} {
		if n, err := b.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v; want %d, nil", chunk, n, err, len(chunk))
		}
	}
	if !bytes.Equal(b.buf, []byte("abcdefgh")) || !b.truncated {
		t.Errorf("buffer = %q, truncated %v; want %q, true", b.buf, b.truncated, "abcdefgh")
	}

	exact := &limitedBuffer{limit: 3}
	if _, err := exact.Write([]byte("abc")); err != nil || exact.truncated {
		t.Errorf("Write at the limit: truncated %v, error %v; want false, nil", exact.truncated, err)
	}
}

func TestCheckTimeoutDefault(t *testing.T) {
	tests := []struct {
		opts CheckOptions
		want time.Duration
	}{
		{CheckOptions{}, 30 * time.Second},
		{CheckOptions{CommandTimeout: 10 * time.Second}, 100 * time.Second},
		{CheckOptions{CommandTimeout: 10 * time.Second, CheckTimeout: 45 * time.Second}, 45 * time.Second},
		{CheckOptions{CommandTimeout: 300000 * time.Hour}, math.MaxInt64}, // 10x would overflow
	}
	for _, tt := range tests {
		if got := tt.opts.checkTimeout(); got != tt.want {
			t.Errorf("checkTimeout() with %+v = %s, want %s", tt.opts, got, tt.want)
		}
	}
}

// registerTestCheck adds a check to the registry for one test. It changes
// package state, so tests that use it must not call t.Parallel.
func registerTestCheck(t *testing.T, key string, check checkFunc) {
	t.Helper()

	checkRegistry[key] = check
	t.Cleanup(func() { delete(checkRegistry, key) })
}

// hangingCheck returns a check that ignores its context and blocks until the
// test ends, like a statfs call on an unresponsive NFS mount.
func hangingCheck(t *testing.T) checkFunc {
	t.Helper()

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return func(context.Context, CheckOptions) CheckOutcome {
		<-release
		return CheckOutcome{Key: "test-hang", Status: StatusPass}
	}
}

func passingCheck(key string) checkFunc {
	return func(context.Context, CheckOptions) CheckOutcome {
		return CheckOutcome{Key: key, Status: StatusPass, Message: "fine"}
	}
}

func TestRunChecksStopsCheckAtItsTimeLimit(t *testing.T) {
	registerTestCheck(t, "test-hang", hangingCheck(t))
	registerTestCheck(t, "test-pass", passingCheck("test-pass"))

	start := time.Now()
	outcomes, err := RunChecks(t.Context(), []string{"test-hang", "test-pass"}, CheckOptions{CheckTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("RunChecks() took %s, want about 50ms", elapsed)
	}
	if len(outcomes) != 2 {
		t.Fatalf("RunChecks() = %+v, want two outcomes", outcomes)
	}
	assertOutcome(t, outcomes[0], "test-hang", StatusFail, "did not finish within 50ms (--check-timeout)")
	assertOutcome(t, outcomes[1], "test-pass", StatusPass, "fine")
}

func TestRunChecksRunTimeLimit(t *testing.T) {
	registerTestCheck(t, "test-hang", hangingCheck(t))
	registerTestCheck(t, "test-hang-2", hangingCheck(t))

	outcomes, err := RunChecks(t.Context(), []string{"test-hang", "test-hang-2"},
		CheckOptions{CheckTimeout: time.Minute, RunTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("RunChecks() = %+v, want two outcomes", outcomes)
	}
	assertOutcome(t, outcomes[0], "test-hang", StatusFail, "stopped: the run time limit was reached (--run-timeout)")
	assertOutcome(t, outcomes[1], "test-hang-2", StatusFail, "not run: the run time limit was reached (--run-timeout)")
}

func TestRunChecksReturnsCancellationCause(t *testing.T) {
	registerTestCheck(t, "test-hang", hangingCheck(t))
	signalErr := errors.New("interrupt signal received")

	// Cancelled before the run starts.
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(signalErr)
	if outcomes, err := RunChecks(ctx, []string{"test-hang"}, CheckOptions{}); !errors.Is(err, signalErr) || outcomes != nil {
		t.Errorf("RunChecks() with a cancelled context = %+v, %v; want nil, %v", outcomes, err, signalErr)
	}

	// Cancelled while a check runs.
	ctx, cancel = context.WithCancelCause(t.Context())
	time.AfterFunc(20*time.Millisecond, func() { cancel(signalErr) })
	if outcomes, err := RunChecks(ctx, []string{"test-hang"}, CheckOptions{}); !errors.Is(err, signalErr) || outcomes != nil {
		t.Errorf("RunChecks() cancelled during a check = %+v, %v; want nil, %v", outcomes, err, signalErr)
	}
}

func TestRunChecksRaisesCheckPanic(t *testing.T) {
	registerTestCheck(t, "test-panic", func(context.Context, CheckOptions) CheckOutcome { panic("boom") })

	defer func() {
		r := recover()
		msg, _ := r.(string)
		if !strings.HasPrefix(msg, "check test-panic panicked: boom") {
			t.Errorf("recovered %v, want the check's panic", r)
		}
	}()
	_, _ = RunChecks(t.Context(), []string{"test-panic"}, CheckOptions{})
	t.Error("RunChecks() returned, want a panic")
}

func TestStoppedOutcomeNamesTarget(t *testing.T) {
	got := stoppedOutcome(keyDiskSpace, "/mnt/nfs", time.Now(), "did not finish within 30s (--check-timeout)")
	assertOutcome(t, got, keyDiskSpace, StatusFail, "/mnt/nfs: did not finish within 30s (--check-timeout)")
}
