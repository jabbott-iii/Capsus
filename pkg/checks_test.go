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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeResult is the canned combined output and error for one simulated command.
type fakeResult struct {
	out string
	err error
}

// fakeToolOptions returns CheckOptions whose external tools are simulated.
// installed lists the tools present on PATH; results maps "name arg..." to the
// command's result. It also returns the commands that were run, in order.
func fakeToolOptions(t *testing.T, installed []string, results map[string]fakeResult) (CheckOptions, *[]string) {
	t.Helper()

	var calls []string
	opts := CheckOptions{
		lookPath: func(file string) (string, error) {
			for _, name := range installed {
				if name == file {
					return filepath.Join("fakebin", file), nil
				}
			}
			return "", exec.ErrNotFound
		},
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			command := strings.Join(append([]string{name}, args...), " ")
			calls = append(calls, command)
			result, ok := results[command]
			if !ok {
				t.Errorf("unexpected command %q", command)
				return nil, errors.New("unexpected command")
			}
			return []byte(result.out), result.err
		},
	}
	return opts, &calls
}

// unsetEnv removes key for the duration of the test and restores it afterwards.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	previous, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, previous)
		}
	})
}

// isolateMisconfigEnv gives the misconfig check a clean, passing environment:
// no SALUS_DB_PATH, a per-user default database location in a temp dir that
// does not exist yet, no kubeconfig or Docker socket, a PATH holding only an
// owner-only directory, and no sshd_config.
func isolateMisconfigEnv(t *testing.T) {
	t.Helper()

	t.Setenv(DatabasePathEnv, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", t.TempDir())
	}
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing-kubeconfig"))
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "missing-docker.sock"))
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv(SSHDConfigEnv, filepath.Join(t.TempDir(), "missing-sshd_config"))
}

// fileWithMode creates path (with its directory) and sets its permission bits
// exactly, regardless of the umask.
func fileWithMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func assertOutcome(t *testing.T, got CheckOutcome, wantKey string, wantStatus CheckStatus, wantMessage string) {
	t.Helper()

	if got.Key != wantKey || got.Status != wantStatus || got.Message != wantMessage {
		t.Errorf("outcome = {%s %s %q}, want {%s %s %q}", got.Key, got.Status, got.Message, wantKey, wantStatus, wantMessage)
	}
	if strings.Contains(got.Message, "\n") {
		t.Errorf("outcome message %q spans multiple lines", got.Message)
	}
}

func TestCheckDockerStatus(t *testing.T) {
	const (
		dockerInfo = "docker info --format {{.ServerVersion}}"
		unhealthy  = "docker ps --filter health=unhealthy --format {{.Names}}"
		restarting = "docker ps --all --filter status=restarting --format {{.Names}}"
		reachable  = "docker daemon reachable (server version 27.3.1)"
	)
	tests := []struct {
		name        string
		installed   []string
		results     map[string]fakeResult
		wantStatus  CheckStatus
		wantMessage string
	}{
		{
			name:        "CLI missing",
			wantStatus:  StatusWarn,
			wantMessage: "docker CLI not found in PATH",
		},
		{
			name:        "daemon unreachable",
			installed:   []string{"docker"},
			results:     map[string]fakeResult{dockerInfo: {out: "Cannot connect to the Docker daemon\nIs the docker daemon running?\n", err: errors.New("exit status 1")}},
			wantStatus:  StatusFail,
			wantMessage: "docker daemon unreachable: Cannot connect to the Docker daemon",
		},
		{
			name:        "daemon reachable and containers healthy",
			installed:   []string{"docker"},
			results:     map[string]fakeResult{dockerInfo: {out: "27.3.1\n"}, unhealthy: {}, restarting: {}},
			wantStatus:  StatusPass,
			wantMessage: reachable,
		},
		{
			name:      "unhealthy and restarting containers",
			installed: []string{"docker"},
			results: map[string]fakeResult{
				dockerInfo: {out: "27.3.1\n"},
				unhealthy:  {out: "web\ndb\n"},
				restarting: {out: "worker\n"},
			},
			wantStatus:  StatusWarn,
			wantMessage: reachable + "; unhealthy: web, db; restarting: worker",
		},
		{
			name:      "long lists are summarized",
			installed: []string{"docker"},
			results: map[string]fakeResult{
				dockerInfo: {out: "27.3.1\n"},
				unhealthy:  {},
				restarting: {out: "c1\nc2\nc3\nc4\nc5\nc6\nc7\n"},
			},
			wantStatus:  StatusWarn,
			wantMessage: reachable + "; restarting: c1, c2, c3, c4, c5 and 2 more",
		},
		{
			name:      "stderr warnings are not versions or names",
			installed: []string{"docker"},
			results: map[string]fakeResult{
				dockerInfo: {out: "WARNING: No swap limit support\n27.3.1\n"},
				unhealthy:  {out: "WARNING: Error loading config file: /root/.docker/config.json: permission denied\nweb\n"},
				restarting: {out: "WARNING: Error loading config file: /root/.docker/config.json: permission denied\n"},
			},
			wantStatus:  StatusWarn,
			wantMessage: reachable + "; unhealthy: web",
		},
		{
			name:      "container listing fails",
			installed: []string{"docker"},
			results: map[string]fakeResult{
				dockerInfo: {out: "27.3.1\n"},
				unhealthy:  {out: "Error response from daemon: context deadline exceeded\n", err: errors.New("exit status 1")},
			},
			wantStatus:  StatusWarn,
			wantMessage: reachable + "; container health unknown: Error response from daemon: context deadline exceeded",
		},
		{
			name:      "container listing fails behind a CLI warning",
			installed: []string{"docker"},
			results: map[string]fakeResult{
				dockerInfo: {out: "27.3.1\n"},
				unhealthy: {
					out: "WARNING: Error loading config file: /root/.docker/config.json: permission denied\npermission denied while trying to connect to the docker API\n",
					err: errors.New("exit status 1"),
				},
			},
			wantStatus:  StatusWarn,
			wantMessage: reachable + "; container health unknown: permission denied while trying to connect to the docker API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, calls := fakeToolOptions(t, tt.installed, tt.results)
			assertOutcome(t, checkDockerStatus(t.Context(), opts), keyDocker, tt.wantStatus, tt.wantMessage)
			if len(tt.installed) == 0 && len(*calls) != 0 {
				t.Errorf("commands run without docker installed: %q", *calls)
			}
		})
	}
}

func TestCheckKubernetesStatus(t *testing.T) {
	const clusterInfoHint = "\nTo further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.\n"
	clusterInfo := "kubectl cluster-info"
	getNodes := "kubectl get nodes -o " + nodeReadinessJSONPath
	reachable := fakeResult{out: "Kubernetes control plane is running\n"}
	tests := []struct {
		name        string
		installed   []string
		results     map[string]fakeResult
		wantStatus  CheckStatus
		wantMessage string
	}{
		{
			name:        "CLI missing",
			wantStatus:  StatusWarn,
			wantMessage: "kubectl CLI not found in PATH",
		},
		{
			name:        "cluster unreachable with no output",
			installed:   []string{"kubectl"},
			results:     map[string]fakeResult{clusterInfo: {err: errors.New("exit status 1")}},
			wantStatus:  StatusFail,
			wantMessage: "kubernetes cluster unreachable: exit status 1",
		},
		{
			name:        "all nodes Ready",
			installed:   []string{"kubectl"},
			results:     map[string]fakeResult{clusterInfo: reachable, getNodes: {out: "node-1\tTrue\nnode-2\tTrue\nnode-3\tTrue\n"}},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; 3/3 nodes Ready",
		},
		{
			name:        "some nodes NotReady",
			installed:   []string{"kubectl"},
			results:     map[string]fakeResult{clusterInfo: reachable, getNodes: {out: "node-1\tTrue\nnode-2\tFalse\nnode-3\tUnknown\n"}},
			wantStatus:  StatusWarn,
			wantMessage: "kubernetes cluster reachable; 1/3 nodes Ready; NotReady: node-2, node-3",
		},
		{
			name:        "no node Ready",
			installed:   []string{"kubectl"},
			results:     map[string]fakeResult{clusterInfo: reachable, getNodes: {out: "node-1\tFalse\nnode-2\t\n"}},
			wantStatus:  StatusFail,
			wantMessage: "kubernetes cluster reachable, but no nodes are Ready (0/2); NotReady: node-1, node-2",
		},
		{
			name:        "no nodes at all",
			installed:   []string{"kubectl"},
			results:     map[string]fakeResult{clusterInfo: reachable, getNodes: {}},
			wantStatus:  StatusFail,
			wantMessage: "kubernetes cluster reachable, but it has no nodes",
		},
		{
			name:      "kubectl warnings are not nodes",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: "W0928 12:00:00.000000   12345 warnings.go:70] v1 ComponentStatus is deprecated\nnode-1\tTrue\n",
			}},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; 1/1 nodes Ready",
		},
		{
			name:      "listing nodes is forbidden",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: `Error from server (Forbidden): nodes is forbidden: User "dev" cannot list resource "nodes" in API group "" at the cluster scope` + "\n",
				err: errors.New("exit status 1"),
			}},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; node readiness not checked (listing nodes is forbidden)",
		},
		{
			name:      "listing nodes fails otherwise",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: "Unable to connect to the server: net/http: TLS handshake timeout\n",
				err: errors.New("exit status 1"),
			}},
			wantStatus:  StatusWarn,
			wantMessage: "kubernetes cluster reachable; node readiness unknown: Unable to connect to the server: net/http: TLS handshake timeout",
		},
		{
			// kubectl cluster-info prints its hint on stdout before the error.
			name:      "namespace-scoped user",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{
				clusterInfo: {out: clusterInfoHint + `Error from server (Forbidden): services is forbidden: User "dev" cannot list resource "services" in API group "" in the namespace "kube-system"` + "\n", err: errors.New("exit status 1")},
				getNodes:    {out: `Error from server (Forbidden): nodes is forbidden: User "dev" cannot list resource "nodes" in API group "" at the cluster scope` + "\n", err: errors.New("exit status 1")},
			},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; node readiness not checked (listing nodes is forbidden)",
		},
		{
			name:      "cluster-info forbidden, nodes readable",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{
				clusterInfo: {out: clusterInfoHint + "Error from server (Forbidden)\n", err: errors.New("exit status 1")},
				getNodes:    {out: "node-1\tTrue\n"},
			},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; 1/1 nodes Ready",
		},
		{
			name:      "unreachable behind log lines and the hint",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: {
				out: "E0928 12:00:00.000000   12345 memcache.go:265] couldn't get current server API group list: dial tcp 127.0.0.1:8080: connect: connection refused\n" +
					clusterInfoHint + "The connection to the server localhost:8080 was refused - did you specify the right host or port?\n",
				err: errors.New("exit status 1"),
			}},
			wantStatus:  StatusFail,
			wantMessage: "kubernetes cluster unreachable: The connection to the server localhost:8080 was refused - did you specify the right host or port?",
		},
		{
			name:      "forbidden spelled only in lower case",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: `nodes is forbidden: User "dev" cannot list resource "nodes"` + "\n",
				err: errors.New("exit status 1"),
			}},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; node readiness not checked (listing nodes is forbidden)",
		},
		{
			name:      "nodes under pressure",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: "node-1\tTrue\tFalse\tFalse\tFalse\nnode-2\tTrue\tTrue\tFalse\tFalse\nnode-3\tTrue\tFalse\tTrue\tTrue\n",
			}},
			wantStatus:  StatusWarn,
			wantMessage: "kubernetes cluster reachable; 3/3 nodes Ready; under pressure: node-2 (MemoryPressure), node-3 (DiskPressure, PIDPressure)",
		},
		{
			name:      "NotReady and under pressure",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: "node-1\tTrue\tFalse\tFalse\tFalse\nnode-2\tFalse\tFalse\tTrue\tFalse\n",
			}},
			wantStatus:  StatusWarn,
			wantMessage: "kubernetes cluster reachable; 1/2 nodes Ready; NotReady: node-2; under pressure: node-2 (DiskPressure)",
		},
		{
			name:      "no node Ready and under pressure",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: "node-1\tFalse\tTrue\tFalse\tFalse\n",
			}},
			wantStatus:  StatusFail,
			wantMessage: "kubernetes cluster reachable, but no nodes are Ready (0/1); NotReady: node-1; under pressure: node-1 (MemoryPressure)",
		},
		{
			name:      "pressure conditions False or missing",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: "node-1\tTrue\tFalse\tFalse\tFalse\nnode-2\tTrue\t\t\t\nnode-3\tTrue\tUnknown\n",
			}},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; 3/3 nodes Ready",
		},
		{
			name:      "log line containing a tab is not a node",
			installed: []string{"kubectl"},
			results: map[string]fakeResult{clusterInfo: reachable, getNodes: {
				out: "W0928 12:00:00.000000\t12345 warnings.go:70] deprecated\nnode-1\tTrue\n",
			}},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable; 1/1 nodes Ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _ := fakeToolOptions(t, tt.installed, tt.results)
			assertOutcome(t, checkKubernetesStatus(t.Context(), opts), keyKubernetes, tt.wantStatus, tt.wantMessage)
		})
	}
}

func TestCheckKubernetesStatusWithContext(t *testing.T) {
	results := map[string]fakeResult{
		"kubectl --context=arn:aws:eks:us-east-1:123456789012:cluster/prod cluster-info":                          {out: "Kubernetes control plane is running\n"},
		"kubectl --context=arn:aws:eks:us-east-1:123456789012:cluster/prod get nodes -o " + nodeReadinessJSONPath: {out: "node-1\tTrue\n"},
	}
	opts, calls := fakeToolOptions(t, []string{"kubectl"}, results)
	opts.KubeContext = " arn:aws:eks:us-east-1:123456789012:cluster/prod "

	want := "kubernetes cluster (context arn:aws:eks:us-east-1:123456789012:cluster/prod) reachable; 1/1 nodes Ready"
	assertOutcome(t, checkKubernetesStatus(t.Context(), opts), keyKubernetes, StatusPass, want)
	if len(*calls) != 2 {
		t.Errorf("ran %q, want cluster-info and get nodes", *calls)
	}
}

func TestCheckKubernetesStatusRejectsInvalidContexts(t *testing.T) {
	for _, name := range []string{"--kubeconfig=/tmp/evil", "-x", "ctx\nnewline", "tab\there", "\x1b[31mred", "bad\xffutf8", strings.Repeat("a", 254)} {
		t.Run(name, func(t *testing.T) {
			opts, calls := fakeToolOptions(t, []string{"kubectl"}, nil)
			opts.KubeContext = name

			got := checkKubernetesStatus(t.Context(), opts)
			if got.Status != StatusFail || !strings.HasPrefix(got.Message, "invalid kubeconfig context ") {
				t.Errorf("outcome = {%s %q}, want FAIL for an invalid context", got.Status, got.Message)
			}
			if strings.Contains(got.Message, "\n") {
				t.Errorf("message %q spans multiple lines", got.Message)
			}
			if len(*calls) != 0 {
				t.Errorf("ran %q for an invalid context", *calls)
			}
		})
	}
}

func TestValidKubeContext(t *testing.T) {
	for _, name := range []string{
		"kind-kind",
		"minikube",
		"gke_my-project_us-central1-a_my-cluster",
		"arn:aws:eks:us-west-2:123456789012:cluster/my-cluster",
		"default/api-cluster-example-com:6443/kube:admin",
		"user@cluster",
		"prod (eu)",
		".hidden",
		"@x",
		"prod;reboot",
		strings.Repeat("a", 253),
	} {
		if !validKubeContext(name) {
			t.Errorf("validKubeContext(%q) = false, want true", name)
		}
	}
}

func TestCheckServiceUptimeWithService(t *testing.T) {
	isActive := "systemctl is-active -- nginx"

	if runtime.GOOS != "linux" {
		opts, calls := fakeToolOptions(t, []string{"systemctl"}, nil)
		opts.ServiceName = "nginx"
		assertOutcome(t, checkServiceUptime(t.Context(), opts), keyServiceUptime, StatusWarn, "service uptime check requires systemd (Linux only)")
		if len(*calls) != 0 {
			t.Errorf("commands run on %s: %q", runtime.GOOS, *calls)
		}
		return
	}

	tests := []struct {
		name        string
		installed   []string
		results     map[string]fakeResult
		wantStatus  CheckStatus
		wantMessage string
	}{
		{
			name:        "systemctl missing",
			wantStatus:  StatusWarn,
			wantMessage: "systemctl not found in PATH",
		},
		{
			name:        "active",
			installed:   []string{"systemctl"},
			results:     map[string]fakeResult{isActive: {out: "active\n"}},
			wantStatus:  StatusPass,
			wantMessage: `service "nginx" is active`,
		},
		{
			name:        "activating",
			installed:   []string{"systemctl"},
			results:     map[string]fakeResult{isActive: {out: "activating\n", err: errors.New("exit status 3")}},
			wantStatus:  StatusWarn,
			wantMessage: `service "nginx" is activating`,
		},
		{
			name:        "inactive with multi-line output",
			installed:   []string{"systemctl"},
			results:     map[string]fakeResult{isActive: {out: "inactive\nWarning: unit changed on disk\n", err: errors.New("exit status 3")}},
			wantStatus:  StatusFail,
			wantMessage: `service "nginx" is not active (inactive)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _ := fakeToolOptions(t, tt.installed, tt.results)
			opts.ServiceName = " nginx "
			assertOutcome(t, checkServiceUptime(t.Context(), opts), keyServiceUptime, tt.wantStatus, tt.wantMessage)
		})
	}
}

func TestCommandAppliesTimeout(t *testing.T) {
	var deadline time.Time
	opts := CheckOptions{
		CommandTimeout: 250 * time.Millisecond,
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			var ok bool
			if deadline, ok = ctx.Deadline(); !ok {
				t.Error("command context has no deadline")
			}
			return nil, nil
		},
	}

	before := time.Now()
	if _, err := opts.command(t.Context(), "docker", "info"); err != nil {
		t.Fatalf("command() error = %v", err)
	}
	if remaining := deadline.Sub(before); remaining <= 0 || remaining > time.Second {
		t.Errorf("command deadline is %v after start, want about 250ms", remaining)
	}
}

func TestCheckMisconfigurationPassesInCleanEnvironment(t *testing.T) {
	isolateMisconfigEnv(t)

	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
}

func TestCheckMisconfigurationWarnsWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not required on Windows")
	}
	isolateMisconfigEnv(t)
	unsetEnv(t, "HOME")

	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, "home-unset: HOME environment variable is not set")
}

func TestCheckMisconfigurationDatabasePermissions(t *testing.T) {
	isolateMisconfigEnv(t)

	tests := []struct {
		name string
		mode os.FileMode
		warn bool
	}{
		{name: "owner only", mode: 0o600, warn: false},
		{name: "group readable", mode: 0o640, warn: true},
		{name: "group and other writable", mode: 0o666, warn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "salus.db")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatalf("create database file: %v", err)
			}
			if err := os.Chmod(path, tt.mode); err != nil {
				t.Fatalf("chmod database file: %v", err)
			}
			t.Setenv(DatabasePathEnv, path)

			got := checkMisconfiguration(CheckOptions{})

			// Windows has no POSIX mode bits (Go reports 0666 for any writable
			// file), so the permission test is skipped there and never warns.
			wantWarn := tt.warn && runtime.GOOS != "windows"
			if wantWarn {
				want := fmt.Sprintf("db-permissions: %s is accessible by group/other (mode %04o, want 0600; run chmod 600 on it)", path, tt.mode)
				assertOutcome(t, got, keyMisconfig, StatusWarn, want)
				return
			}
			assertOutcome(t, got, keyMisconfig, StatusPass, "no common misconfigurations detected")
		})
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{name: "first of several lines", output: "  one\ntwo\n", want: "one"},
		{name: "error when output empty", output: " \n", err: errors.New("boom"), want: "boom"},
		{name: "fallback", want: "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstLine(tt.output, tt.err); got != tt.want {
				t.Errorf("firstLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckMisconfigurationChecksDefaultDatabasePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not checked on Windows")
	}
	isolateMisconfigEnv(t)

	path, err := DefaultDatabasePath()
	if err != nil {
		t.Fatalf("DefaultDatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create database file: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod database file: %v", err)
	}

	want := "db-permissions: " + path + " is accessible by group/other (mode 0644, want 0600; run chmod 600 on it)"
	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, want)
}

func TestCheckMisconfigurationKubeconfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not checked on Windows")
	}

	t.Run("KUBECONFIG list", func(t *testing.T) {
		isolateMisconfigEnv(t)
		dir := t.TempDir()
		private := filepath.Join(dir, "private")
		shared := filepath.Join(dir, "shared")
		fileWithMode(t, private, 0o600)
		fileWithMode(t, shared, 0o644)
		missing := filepath.Join(dir, "missing")
		list := strings.Join([]string{private, shared, missing, shared, ""}, string(os.PathListSeparator))
		t.Setenv("KUBECONFIG", list)

		want := "kubeconfig-permissions: " + shared + " is accessible by group/other (mode 0644, want 0600; run chmod 600 on it)"
		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, want)
	})

	t.Run("default ~/.kube/config", func(t *testing.T) {
		isolateMisconfigEnv(t)
		t.Setenv("KUBECONFIG", "")
		config := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		fileWithMode(t, config, 0o640)

		want := "kubeconfig-permissions: " + config + " is accessible by group/other (mode 0640, want 0600; run chmod 600 on it)"
		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, want)
	})
}

func TestCheckMisconfigurationDockerSocketPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not checked on Windows")
	}

	tests := []struct {
		name string
		mode os.FileMode
		warn bool
	}{
		{name: "docker group only", mode: 0o660, warn: false},
		{name: "writable by all users", mode: 0o666, warn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateMisconfigEnv(t)
			socket := filepath.Join(t.TempDir(), "docker.sock")
			fileWithMode(t, socket, tt.mode)
			t.Setenv("DOCKER_HOST", "unix://"+socket)

			got := checkMisconfiguration(CheckOptions{})
			if tt.warn {
				want := fmt.Sprintf("docker-socket-permissions: %s is writable by all users (mode %04o), which gives every local user control of Docker", socket, tt.mode)
				assertOutcome(t, got, keyMisconfig, StatusWarn, want)
				return
			}
			assertOutcome(t, got, keyMisconfig, StatusPass, "no common misconfigurations detected")
		})
	}

	t.Run("remote daemon", func(t *testing.T) {
		isolateMisconfigEnv(t)
		t.Setenv("DOCKER_HOST", "tcp://docker.example.invalid:2376")
		t.Setenv("DOCKER_TLS_VERIFY", "1") // docker-tcp-insecure is tested separately

		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
	})
}

func TestDockerSocketPath(t *testing.T) {
	tests := []struct {
		host   string
		want   string
		wantOK bool
	}{
		{host: "", want: "/var/run/docker.sock", wantOK: true},
		{host: "unix:///run/user/1000/docker.sock", want: "/run/user/1000/docker.sock", wantOK: true},
		{host: "unix://", want: "/var/run/docker.sock", wantOK: true},
		{host: "tcp://docker.example.invalid:2376", wantOK: false},
		{host: "ssh://user@docker.example.invalid", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Setenv("DOCKER_HOST", tt.host)
			got, ok := dockerSocketPath()
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Errorf("dockerSocketPath() = %q, %v, want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCheckMisconfigurationWorldWritablePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not checked on Windows")
	}
	isolateMisconfigEnv(t)

	dirWithMode := func(mode os.FileMode) string {
		dir := t.TempDir()
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatalf("chmod %s: %v", dir, err)
		}
		return dir
	}
	open := dirWithMode(0o777)
	sticky := dirWithMode(0o777 | os.ModeSticky) // like /tmp: still lets anyone add files
	private := dirWithMode(0o755)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	fileWithMode(t, file, 0o777)
	missing := filepath.Join(t.TempDir(), "missing")
	entries := []string{private, open, missing, file, open, sticky}
	t.Setenv("PATH", strings.Join(entries, string(os.PathListSeparator)))

	want := fmt.Sprintf("path-world-writable: PATH directories writable by all users: %s (mode 0777), %s (mode 0777)", open, sticky)
	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, want)

	t.Run("empty entry is the current directory", func(t *testing.T) {
		t.Chdir(open)
		t.Setenv("PATH", private+string(os.PathListSeparator))

		assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn,
			"path-world-writable: PATH directories writable by all users: . (mode 0777)")
	})
}

func TestCheckMisconfigurationReportsEveryRuleInOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME and POSIX permission bits are not checked on Windows")
	}
	isolateMisconfigEnv(t)
	unsetEnv(t, "HOME")
	db := filepath.Join(t.TempDir(), "salus.db")
	fileWithMode(t, db, 0o644)
	t.Setenv(DatabasePathEnv, db)
	open := t.TempDir()
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatalf("chmod %s: %v", open, err)
	}
	t.Setenv("PATH", open)
	t.Setenv("DOCKER_HOST", "tcp://docker.internal:2375")
	sshdConfig := filepath.Join(t.TempDir(), "sshd_config")
	if err := os.WriteFile(sshdConfig, []byte("PermitRootLogin yes\n"), 0o600); err != nil {
		t.Fatalf("write sshd_config: %v", err)
	}
	t.Setenv(SSHDConfigEnv, sshdConfig)

	want := "home-unset: HOME environment variable is not set; " +
		"db-permissions: " + db + " is accessible by group/other (mode 0644, want 0600; run chmod 600 on it); " +
		"path-world-writable: PATH directories writable by all users: " + open + " (mode 0777); " +
		"docker-tcp-insecure: DOCKER_HOST uses tcp://docker.internal:2375 without TLS verification (set DOCKER_TLS_VERIFY=1 and DOCKER_CERT_PATH, or use ssh:// or a unix socket); " +
		"sshd-root-login: " + sshdConfig + " permits root login with any authentication method (PermitRootLogin yes; use prohibit-password or no); " +
		"sshd-password-auth: " + sshdConfig + " does not set PasswordAuthentication, which OpenSSH enables by default (set PasswordAuthentication no)"
	got := checkMisconfiguration(CheckOptions{})
	assertOutcome(t, got, keyMisconfig, StatusWarn, want)
	assertCount(t, got, 6)
}

func TestValidUnitName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"nginx", true},
		{"nginx.service", true},
		{"getty@tty1.service", true},
		{`systemd-fsck@dev-disk-by\x2duuid.service`, true},
		{"--host=user@example.invalid", false},
		{"-H", false},
		{"", false},
		{"nginx;reboot", false},
		{"two words", false},
		{"$(id)", false},
		{strings.Repeat("a", 256), false},
	}

	for _, tt := range tests {
		if got := validUnitName(tt.name); got != tt.valid {
			t.Errorf("validUnitName(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}

func TestCheckServiceUptimeRejectsInvalidNames(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("service names are only used on Linux")
	}

	for _, name := range []string{"--host=user@example.invalid", "-H", "nginx;reboot"} {
		t.Run(name, func(t *testing.T) {
			opts, calls := fakeToolOptions(t, []string{"systemctl"}, nil)
			opts.ServiceName = name

			want := fmt.Sprintf("invalid service name %q: use a systemd unit name such as nginx or nginx.service", name)
			assertOutcome(t, checkServiceUptime(t.Context(), opts), keyServiceUptime, StatusFail, want)
			if len(*calls) != 0 {
				t.Errorf("ran %q for an invalid service name", *calls)
			}
		})
	}
}

func TestSanitizeMessage(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"\x1b[31mred\x1b[0m", "?[31mred?[0m"},
		{"title\x1b]0;pwned\x07", "title?]0;pwned?"},
		{"tab\tand\nnewline\r", "tab?and?newline?"},
		{"c1 \u009b csi", "c1 ? csi"},
		{"unicode é ✓ stays", "unicode é ✓ stays"},
	}

	for _, tt := range tests {
		if got := sanitizeMessage(tt.in); got != tt.want {
			t.Errorf("sanitizeMessage(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRunChecksSanitizesToolOutput(t *testing.T) {
	// A hostile daemon (reached through DOCKER_HOST) controls the error text.
	opts, _ := fakeToolOptions(t, []string{"docker"}, map[string]fakeResult{
		"docker info --format {{.ServerVersion}}": {out: "\x1b]0;pwned\x07\x1b[2KCannot connect\n", err: errors.New("exit status 1")},
	})

	outcomes, err := RunChecks(t.Context(), []string{keyDocker}, opts)
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	want := "docker daemon unreachable: ?]0;pwned??[2KCannot connect"
	if got := outcomes[0].Message; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestErrorLine(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{name: "plain error", output: "Cannot connect\nIs it running?\n", want: "Cannot connect"},
		{name: "docker warning first", output: "WARNING: Error loading config file\nreal error\n", want: "real error"},
		{name: "kubectl log line and hint", output: "E0928 12:00:00.000000   1 x.go:1] noise\n\nTo further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.\nthe error\n", want: "the error"},
		{name: "only noise falls back to the first line", output: "WARNING: only a warning\n", want: "WARNING: only a warning"},
		{name: "no output uses the error", output: "\n", err: errors.New("exit status 1"), want: "exit status 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorLine(tt.output, tt.err); got != tt.want {
				t.Errorf("errorLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
