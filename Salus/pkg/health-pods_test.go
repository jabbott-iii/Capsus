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
	"strings"
	"testing"
)

func TestCheckKubernetesPods(t *testing.T) {
	getPods := "kubectl get pods -o " + podHealthJSONPath
	tests := []struct {
		name        string
		installed   []string
		result      fakeResult
		wantStatus  CheckStatus
		wantMessage string
		wantCount   float64 // -1: no value
	}{
		{"CLI missing", nil, fakeResult{}, StatusWarn, "kubectl CLI not found in PATH", -1},
		{"no pods", []string{"kubectl"}, fakeResult{out: "No resources found in default namespace.\n"}, StatusPass, "current namespace: no pods to check (completed pods are ignored)", 0},
		{
			"all Ready, completed ignored", []string{"kubectl"},
			fakeResult{out: "web-1\tRunning\tTrue\t \nweb-2\tRunning\tTrue\t \nmigrate-x7\tSucceeded\tFalse\t \n"},
			StatusPass, "current namespace: 2/2 pods Ready", 0,
		},
		{
			"problems by kind", []string{"kubectl"},
			fakeResult{out: "web-1\tRunning\tTrue\t \n" +
				"api-1\tRunning\tFalse\tCrashLoopBackOff \n" +
				"init-1\tPending\tFalse\t CrashLoopBackOff\n" +
				"job-1\tFailed\tFalse\t \n" +
				"img-1\tPending\tFalse\tImagePullBackOff \n" +
				"slow-1\tRunning\tFalse\t \n"},
			StatusWarn, "current namespace: 1/6 pods Ready; CrashLoopBackOff: api-1, init-1; Failed: job-1; not Ready: img-1 (ImagePullBackOff), slow-1", 5,
		},
		{
			"kubectl warnings are not pods", []string{"kubectl"},
			fakeResult{out: "Warning: v1 Pod is deprecated\nW1003 12:00:00.000000   1 warnings.go:70] x\nweb-1\tRunning\tTrue\t \n"},
			StatusPass, "current namespace: 1/1 pods Ready", 0,
		},
		{
			"forbidden", []string{"kubectl"},
			fakeResult{out: `Error from server (Forbidden): pods is forbidden: User "dev" cannot list resource "pods"` + "\n", err: errors.New("exit status 1")},
			StatusPass, "current namespace: pod health not checked (listing pods is forbidden)", -1,
		},
		{
			"unreachable", []string{"kubectl"},
			fakeResult{out: "E1003 12:00:00.000000   1 memcache.go:265] couldn't get current server API group list\nThe connection to the server localhost:8080 was refused - did you specify the right host or port?\n", err: errors.New("exit status 1")},
			StatusWarn, "current namespace: pod health unknown: The connection to the server localhost:8080 was refused - did you specify the right host or port?", -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _ := fakeToolOptions(t, tt.installed, map[string]fakeResult{getPods: tt.result})
			got := checkKubernetesPods(t.Context(), opts)
			assertOutcome(t, got, keyKubePods, tt.wantStatus, tt.wantMessage)
			assertCount(t, got, tt.wantCount)
		})
	}
}

func TestCheckKubernetesPodsWithNamespaceAndContext(t *testing.T) {
	command := "kubectl --context=prod --namespace=web get pods -o " + podHealthJSONPath
	opts, calls := fakeToolOptions(t, []string{"kubectl"}, map[string]fakeResult{command: {out: "web-1\tRunning\tTrue\t \n"}})
	opts.KubeContext = "prod"
	opts.KubeNamespace = "web"

	assertOutcome(t, checkKubernetesPods(t.Context(), opts), keyKubePods, StatusPass, "namespace web (context prod): 1/1 pods Ready")
	if len(*calls) != 1 || (*calls)[0] != command {
		t.Errorf("commands = %q, want %q", *calls, command)
	}
}

func TestCheckKubernetesPodsRejectsInvalidNamespaces(t *testing.T) {
	for _, namespace := range []string{"-n", "--all-namespaces", "Web", "web_1", "web.prod", "web-", strings.Repeat("a", 64), "a\x1bb"} {
		opts, calls := fakeToolOptions(t, []string{"kubectl"}, nil)
		opts.KubeNamespace = namespace
		got := checkKubernetesPods(t.Context(), opts)
		if got.Status != StatusFail || !strings.HasPrefix(got.Message, "invalid namespace ") {
			t.Errorf("namespace %q: {%s %q}, want FAIL invalid namespace", namespace, got.Status, got.Message)
		}
		if len(*calls) != 0 {
			t.Errorf("namespace %q ran %q, want no command", namespace, *calls)
		}
	}
	for _, namespace := range []string{"default", "kube-system", "a", "team-1", strings.Repeat("a", 63)} {
		if !validNamespace(namespace) {
			t.Errorf("validNamespace(%q) = false, want true", namespace)
		}
	}
}

func TestCheckKubernetesPodsRejectsInvalidContext(t *testing.T) {
	opts, calls := fakeToolOptions(t, []string{"kubectl"}, nil)
	opts.KubeContext = "--kubeconfig=/tmp/x"
	if got := checkKubernetesPods(t.Context(), opts); got.Status != StatusFail || !strings.HasPrefix(got.Message, "invalid kubeconfig context ") {
		t.Errorf("checkKubernetesPods() = {%s %q}, want FAIL for the context", got.Status, got.Message)
	}
	if len(*calls) != 0 {
		t.Errorf("ran %q, want no command", *calls)
	}
}
