//go:build !unix

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

import "os/exec"

// endedByStopSignal reports false: outside Unix, Salus cannot tell from the
// exit status that a signal ended the tool.
func endedByStopSignal(error) bool { return false }

// stopProcessGroup leaves exec's default cancellation, which kills only the
// tool itself. Processes the tool started can outlive it; commandWaitDelay
// still bounds how long Salus waits for them to close the output pipes.
func stopProcessGroup(*exec.Cmd) {}
