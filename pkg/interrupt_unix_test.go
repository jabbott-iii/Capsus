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
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestDecryptInterruptedBySignal is SEC-015's end-to-end check: SIGINT while cryptare
// decrypts makes it remove its partial plaintext and exit with status 130
// (128 + SIGINT). The encrypted input comes through a named pipe, so the test controls
// how far decryption has got when the signal is sent.
func TestDecryptInterruptedBySignal(t *testing.T) {
	dir := t.TempDir()
	const password = "correct horse battery staple"
	pwFile := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(pwFile, []byte(password+"\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	plain := filepath.Join(dir, "plain.bin")
	if err := os.WriteFile(plain, bytes.Repeat([]byte("secret plaintext\n"), 1<<18), 0o600); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	encrypted := plain + ".enc"
	if err := EncryptFile(plain, encrypted, password); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	data, err := os.ReadFile(encrypted)
	if err != nil {
		t.Fatalf("read encrypted file: %v", err)
	}
	stream := filepath.Join(dir, "stream.enc")
	if err := syscall.Mkfifo(stream, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	out := filepath.Join(work, "out.bin")

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunMain$", "--",
		"decrypt", stream, "--output", out, "--password-file", pwFile)
	cmd.Env = append(os.Environ(), helperEnv+"=1", databasePathEnv+"="+filepath.Join(dir, "keys.db"))
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cryptare: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-exited
	})

	// Feed half the stream. Decryption is then writing its temporary output and waits
	// for more input.
	pipe, err := os.OpenFile(stream, os.O_WRONLY, 0) // blocks until cryptare opens it
	if err != nil {
		t.Fatalf("open pipe: %v", err)
	}
	defer func() { _ = pipe.Close() }()
	half := len(data) / 2
	if _, err := pipe.Write(data[:half]); err != nil {
		t.Fatalf("write first half: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(hiddenTempFiles(t, work)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no temporary output appeared")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	// Trickle in the rest: each piece lets the blocked read return, and the next check
	// of the context stops decryption. The pauses only pace the input; the outcome
	// doesn't depend on them. Writes fail once cryptare has exited.
	var waitErr error
	done := false
	for rest := data[half:]; len(rest) > 0 && !done; {
		select {
		case waitErr = <-exited:
			done = true
			continue
		default:
		}
		n := min(len(rest), 4096)
		if _, err := pipe.Write(rest[:n]); err != nil {
			break
		}
		rest = rest[n:]
		time.Sleep(5 * time.Millisecond)
	}
	_ = pipe.Close()
	if !done {
		select {
		case waitErr = <-exited:
		case <-time.After(30 * time.Second):
			t.Fatal("cryptare didn't exit after SIGINT")
		}
	}
	exited <- waitErr // for the clean-up

	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 130 {
		t.Fatalf("exit = %v, want status 130; cryptare said: %s", waitErr, output.String())
	}
	if !strings.Contains(output.String(), "interrupted") {
		t.Errorf("output %q doesn't say the command was interrupted", output.String())
	}
	if left := hiddenTempFiles(t, work); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output exists after the interrupt (stat err %v)", err)
	}
}

// hiddenTempFiles lists the hidden temporary files (".<name>.*.tmp") in dir.
func hiddenTempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && strings.HasSuffix(e.Name(), ".tmp") {
			names = append(names, e.Name())
		}
	}
	return names
}
