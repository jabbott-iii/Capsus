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
	"fmt"
	"os/exec"
	"time"
)

const (
	// maxCommandOutput bounds the combined output kept from an external tool.
	// The checks read short listings (one line per container, node, or pod),
	// so more than this means something is wrong, possibly a hostile daemon
	// or API server. Output past the limit is read and discarded, so the tool
	// never blocks on a full pipe, and the command fails instead of returning
	// a truncated listing that could hide problems.
	maxCommandOutput = 8 << 20

	// commandWaitDelay is how long Salus waits for a tool's output pipes to
	// close after the tool exits or is killed. Processes the tool started,
	// such as a kubectl credential plugin, inherit the pipes and can hold
	// them open long after the tool is gone.
	commandWaitDelay = time.Second

	// stopSignalGrace is how long Salus waits, after a tool was ended by
	// SIGINT or SIGTERM, for the same signal to cancel the run (see
	// runExternal).
	stopSignalGrace = 250 * time.Millisecond
)

// runExternal runs an external tool and returns its combined stdout and
// stderr. When ctx ends, the tool is killed, on Unix together with every
// process in its process group (see stopProcessGroup), and runExternal
// returns at most commandWaitDelay later.
func runExternal(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out := &limitedBuffer{limit: maxCommandOutput}
	cmd.Stdout = out // one writer for both streams, like CombinedOutput
	cmd.Stderr = out
	cmd.WaitDelay = commandWaitDelay
	stopProcessGroup(cmd)

	err := cmd.Run()
	if ctx.Err() == nil && endedByStopSignal(err) {
		// systemd signals a whole control group, so the signal that ended
		// the tool most likely reached Salus too, and its handler cancels
		// the run any moment now. Waiting for that keeps the run reported as
		// interrupted, rather than as a check that failed with "signal:
		// terminated".
		select {
		case <-ctx.Done():
		case <-time.After(stopSignalGrace):
		}
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// The tool exited successfully, but a process it started kept the
		// output pipes open past commandWaitDelay. The tool's own output is
		// complete.
		err = nil
	}
	if err == nil && out.truncated {
		return nil, fmt.Errorf("%s printed more than %d MiB of output", name, maxCommandOutput>>20)
	}
	return out.buf, err
}

// limitedBuffer keeps the first limit bytes written to it and discards the
// rest, recording that it did. Writes always succeed, so the copying
// goroutine keeps draining the pipe.
type limitedBuffer struct {
	buf       []byte
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - len(b.buf); room < len(p) {
		b.truncated = true
		b.buf = append(b.buf, p[:max(room, 0)]...)
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}
