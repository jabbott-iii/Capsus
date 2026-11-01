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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// sshdFixture writes files (relative name → contents) into a temp directory,
// points SALUS_SSHD_CONFIG at its sshd_config, and returns that path.
func sshdFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	isolateMisconfigEnv(t)
	dir := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	config := filepath.Join(dir, "sshd_config")
	t.Setenv(SSHDConfigEnv, config)
	return config
}

func TestSSHDRules(t *testing.T) {
	const unsetWarning = "does not set PasswordAuthentication, which OpenSSH enables by default (set PasswordAuthentication no)"
	tests := []struct {
		name         string
		files        map[string]string
		wantRoot     bool
		wantPassword string // "" none, "unset", or "yes"
	}{
		{"hardened", map[string]string{"sshd_config": "PermitRootLogin no\nPasswordAuthentication no\n"}, false, ""},
		{"stock install", map[string]string{"sshd_config": "#PermitRootLogin prohibit-password\n#PasswordAuthentication yes\nUsePAM yes\n"}, false, "unset"},
		{"explicit yes", map[string]string{"sshd_config": "PermitRootLogin yes\nPasswordAuthentication yes\n"}, true, "yes"},
		{"case and equals sign", map[string]string{"sshd_config": "permitrootlogin=YES\n  PASSWORDAUTHENTICATION = No\n"}, true, ""},
		{"quoted value and trailing comment", map[string]string{"sshd_config": "PermitRootLogin \"yes\" # legacy\nPasswordAuthentication no #ok\n"}, true, ""},
		{"first value wins", map[string]string{"sshd_config": "PasswordAuthentication no\nPasswordAuthentication yes\nPermitRootLogin prohibit-password\nPermitRootLogin yes\n"}, false, ""},
		{"settings after Match are not global", map[string]string{"sshd_config": "PermitRootLogin no\nMatch User deploy\n  PasswordAuthentication no\n  PermitRootLogin yes\n"}, false, "unset"},
		{
			"include before the main settings wins (Debian and Ubuntu layout)",
			map[string]string{
				"sshd_config":                  "Include sshd_config.d/*.conf\nPasswordAuthentication yes\n",
				"sshd_config.d/50-cloud.conf":  "PasswordAuthentication no\n",
				"sshd_config.d/README":         "PasswordAuthentication yes\n",
				"sshd_config.d/10-first.conf":  "PermitRootLogin yes\n",
				"sshd_config.d/90-ignore.conf": "PermitRootLogin no\n",
			},
			true, "",
		},
		{
			"Match inside an include ends with that file",
			map[string]string{
				"sshd_config":           "Include conf.d/a.conf\nPasswordAuthentication no\n",
				"conf.d/a.conf":         "Match Address 10.0.0.0/8\n  PermitRootLogin yes\n",
				"conf.d/unrelated.conf": "PermitRootLogin yes\n",
			},
			false, "",
		},
		{"include matching nothing", map[string]string{"sshd_config": "Include missing.d/*.conf missing.conf\nPasswordAuthentication no\n"}, false, ""},
		{
			"Match all makes settings global again",
			map[string]string{"sshd_config": "Match User backup\n  ForceCommand internal-sftp\n  PasswordAuthentication yes\nMatch all\nPasswordAuthentication no\nPermitRootLogin yes\n"},
			true, "",
		},
		{
			"Match All with a comment",
			map[string]string{"sshd_config": "Match Group admins\n PermitRootLogin no\nMatch All # back to global\nPermitRootLogin yes\nPasswordAuthentication no\n"},
			true, "",
		},
		{
			"includes inside a Match block are conditional",
			map[string]string{
				"sshd_config": "Match User deploy\n  Include deploy.conf\nMatch all\nPasswordAuthentication no\n",
				"deploy.conf": "PermitRootLogin yes\n",
			},
			false, "",
		},
		{
			"single-quoted value",
			map[string]string{"sshd_config": "PermitRootLogin 'yes'\nPasswordAuthentication 'no'\n"},
			true, "",
		},
		{
			"wildcards skip dotfiles, as glob(3) does",
			map[string]string{
				"sshd_config":                    "Include sshd_config.d/*.conf\nPasswordAuthentication no\n",
				"sshd_config.d/.editor-tmp.conf": "PermitRootLogin yes\n",
			},
			false, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := sshdFixture(t, tt.files)
			var want []string
			if tt.wantRoot {
				want = append(want, "sshd-root-login: "+config+" permits root login with any authentication method (PermitRootLogin yes; use prohibit-password or no)")
			}
			switch tt.wantPassword {
			case "unset":
				want = append(want, "sshd-password-auth: "+config+" "+unsetWarning)
			case "yes":
				want = append(want, "sshd-password-auth: "+config+" enables password authentication (PasswordAuthentication yes; set it to no)")
			}

			got := checkMisconfiguration(CheckOptions{})
			if len(want) == 0 {
				assertOutcome(t, got, keyMisconfig, StatusPass, "no common misconfigurations detected")
				return
			}
			assertOutcome(t, got, keyMisconfig, StatusWarn, strings.Join(want, "; "))
		})
	}
}

func TestSSHDRulesStaySilentWhenConfigCannotBeRead(t *testing.T) {
	t.Run("not installed", func(t *testing.T) {
		isolateMisconfigEnv(t) // SALUS_SSHD_CONFIG points at a missing file
		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
	})

	t.Run("include depth limit (cycle)", func(t *testing.T) {
		sshdFixture(t, map[string]string{"sshd_config": "Include sshd_config\n"})
		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
	})

	t.Run("relocated configuration with an unresolvable absolute include", func(t *testing.T) {
		sshdFixture(t, map[string]string{"sshd_config": "Include /usr/local/etc/ssh/extra.conf\nPermitRootLogin yes\n"})
		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
	})

	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions cannot make a file unreadable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read files without read permission")
	}
	t.Run("unreadable include", func(t *testing.T) {
		config := sshdFixture(t, map[string]string{
			"sshd_config":           "Include sshd_config.d/*.conf\n",
			"sshd_config.d/50.conf": "PasswordAuthentication no\n",
		})
		unreadable := filepath.Join(filepath.Dir(config), "sshd_config.d", "50.conf")
		if err := os.Chmod(unreadable, 0); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		// Without the include, PasswordAuthentication would look unset.
		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
	})
	t.Run("unlistable include directory", func(t *testing.T) {
		config := sshdFixture(t, map[string]string{
			"sshd_config":           "Include sshd_config.d/*.conf\n",
			"sshd_config.d/50.conf": "PasswordAuthentication no\n",
		})
		dir := filepath.Join(filepath.Dir(config), "sshd_config.d")
		if err := os.Chmod(dir, 0o300); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
	})
}

func TestSSHDConfigPathDefault(t *testing.T) {
	t.Setenv(SSHDConfigEnv, "")
	t.Setenv("ProgramData", `C:\ProgramData`)
	want := "/etc/ssh/sshd_config"
	if runtime.GOOS == "windows" {
		want = `C:\ProgramData\ssh\sshd_config`
	}
	if got := sshdConfigPath(); got != want {
		t.Errorf("sshdConfigPath() = %q, want %q", got, want)
	}
	t.Setenv(SSHDConfigEnv, "/host/etc/ssh/sshd_config")
	if got := sshdConfigPath(); got != "/host/etc/ssh/sshd_config" {
		t.Errorf("sshdConfigPath() with %s = %q", SSHDConfigEnv, got)
	}
}

func TestSplitSSHDLine(t *testing.T) {
	tests := []struct {
		line        string
		wantKeyword string
		wantArgs    []string
	}{
		{"", "", nil},
		{"   # comment", "", nil},
		{"PermitRootLogin no", "permitrootlogin", []string{"no"}},
		{"PermitRootLogin=no", "permitrootlogin", []string{"no"}},
		{"PermitRootLogin = no", "permitrootlogin", []string{"no"}},
		{"\tPermitRootLogin\tno\t", "permitrootlogin", []string{"no"}},
		{`Include "/etc/ssh/my dir/*.conf" other.conf`, "include", []string{"/etc/ssh/my dir/*.conf", "other.conf"}},
		{"Banner none # trailing", "banner", []string{"none"}},
		{`Banner '/etc/my banner' "x y"`, "banner", []string{"/etc/my banner", "x y"}},
		{`AuthorizedKeysFile "unterminated`, "authorizedkeysfile", []string{"unterminated"}},
		{"UsePAM", "usepam", nil},
	}
	for _, tt := range tests {
		keyword, args := splitSSHDLine(tt.line)
		if keyword != tt.wantKeyword || !reflect.DeepEqual(args, tt.wantArgs) {
			t.Errorf("splitSSHDLine(%q) = %q, %q; want %q, %q", tt.line, keyword, args, tt.wantKeyword, tt.wantArgs)
		}
	}
}

func TestDockerTCPInsecure(t *testing.T) {
	tests := []struct {
		host, verify string
		want         string
	}{
		{"", "", ""},
		{"unix:///var/run/docker.sock", "", ""},
		{"ssh://admin@host", "", ""},
		{"tcp://10.0.0.5:2376", "1", ""},
		{"tcp://10.0.0.5:2376", "0", ""}, // like the Docker CLI, any value enables verification
		{"tcp://10.0.0.5:2375", "", "10.0.0.5:2375"},
		{"TCP://docker.internal:2375/path?x=1", "", "docker.internal:2375"},
		{"tcp://user:secret@docker.internal:2375", "", "docker.internal:2375"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s verify=%q", tt.host, tt.verify), func(t *testing.T) {
			isolateMisconfigEnv(t)
			t.Setenv("DOCKER_HOST", tt.host)
			t.Setenv("DOCKER_TLS_VERIFY", tt.verify)
			got := checkMisconfiguration(CheckOptions{})
			if tt.want == "" {
				assertOutcome(t, got, keyMisconfig, StatusPass, "no common misconfigurations detected")
				return
			}
			want := "docker-tcp-insecure: DOCKER_HOST uses tcp://" + tt.want + " without TLS verification (set DOCKER_TLS_VERIFY=1 and DOCKER_CERT_PATH, or use ssh:// or a unix socket)"
			assertOutcome(t, got, keyMisconfig, StatusWarn, want)
			if strings.Contains(got.Message, "secret") {
				t.Errorf("message %q leaks credentials from DOCKER_HOST", got.Message)
			}
		})
	}
}

func TestSSHDRelocatedConfigReadsAbsoluteIncludesFromItsDirectory(t *testing.T) {
	// A host's /etc/ssh mounted at <root>/etc/ssh, with the stock Debian and
	// Ubuntu absolute Include line.
	root := t.TempDir()
	isolateMisconfigEnv(t)
	sshDir := filepath.Join(root, "etc", "ssh")
	for name, contents := range map[string]string{
		"sshd_config":                     "Include /etc/ssh/sshd_config.d/*.conf\nPermitRootLogin yes\n",
		"sshd_config.d/99-hardening.conf": "PasswordAuthentication no\n",
	} {
		path := filepath.Join(sshDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	config := filepath.Join(sshDir, "sshd_config")
	t.Setenv(SSHDConfigEnv, config)

	want := "sshd-root-login: " + config + " permits root login with any authentication method (PermitRootLogin yes; use prohibit-password or no)"
	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, want)
}
