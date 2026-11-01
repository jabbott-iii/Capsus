//go:build !windows

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
	"path/filepath"
	"syscall"
	"testing"
)

func TestCheckCertExpiryRejectsFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cert.pem")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	// Opening a FIFO would block until a writer appears.
	got := checkCertExpiry(CheckOptions{CertPath: path})
	assertOutcome(t, got, keyCertExpiry, StatusFail, path+": not a regular file")
}

func TestSSHDRulesSkipFIFOIncludes(t *testing.T) {
	config := sshdFixture(t, map[string]string{"sshd_config": "Include pipe.conf\nPermitRootLogin yes\n"})
	if err := syscall.Mkfifo(filepath.Join(filepath.Dir(config), "pipe.conf"), 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	// Opening a FIFO would block until a writer appears; the rules stay
	// silent instead.
	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
}
