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
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// podHealthJSONPath prints one line per pod: name, phase, Ready condition
// status, and the waiting reasons of its containers and init containers
// (space-separated), with tabs between the four columns.
const podHealthJSONPath = `jsonpath={range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\t"}{.status.conditions[?(@.type=="Ready")].status}{"\t"}{.status.containerStatuses[*].state.waiting.reason}{" "}{.status.initContainerStatuses[*].state.waiting.reason}{"\n"}{end}`

// namespacePattern matches a Kubernetes namespace name (an RFC 1123 label).
var namespacePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// podNamePattern matches a pod name (an RFC 1123 subdomain); other lines in
// kubectl output, such as warnings, are ignored.
var podNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

// validNamespace reports whether name is an acceptable --kube-namespace
// value. It is passed as the single argument --namespace=<name>.
func validNamespace(name string) bool {
	return len(name) <= 63 && namespacePattern.MatchString(name)
}

// checkKubernetesPods reports pods that are crash-looping, failed, or not
// Ready in one namespace. Those are WARN (Q-014): the cluster works, a
// workload does not. Completed pods (phase Succeeded) are ignored.
func checkKubernetesPods(ctx context.Context, opts CheckOptions) CheckOutcome {
	start := time.Now()
	namespace := strings.TrimSpace(opts.KubeNamespace)
	if namespace != "" && !validNamespace(namespace) {
		return CheckOutcome{Key: keyKubePods, Status: StatusFail, Message: fmt.Sprintf("invalid namespace %q: use a Kubernetes namespace name such as default or kube-system", namespace), Duration: time.Since(start)}
	}

	kubectl, _, stop := kubectlFor(ctx, opts, keyKubePods, start)
	if stop != nil {
		return *stop
	}

	scope := "current namespace"
	args := []string{"get", "pods", "-o", podHealthJSONPath}
	if namespace != "" {
		scope = "namespace " + namespace
		args = append([]string{"--namespace=" + namespace}, args...)
	}
	if kubeContext := strings.TrimSpace(opts.KubeContext); kubeContext != "" {
		scope += fmt.Sprintf(" (context %s)", kubeContext)
	}

	out, err := kubectl(args...)
	if err != nil {
		if forbidden(string(out)) {
			return CheckOutcome{Key: keyKubePods, Status: StatusPass, Message: fmt.Sprintf("%s: pod health not checked (listing pods is forbidden)", scope), Duration: time.Since(start)}
		}
		return CheckOutcome{Key: keyKubePods, Status: StatusWarn, Message: fmt.Sprintf("%s: pod health unknown: %s", scope, errorLine(string(out), err)), Duration: time.Since(start)}
	}

	pods := parsePodHealth(string(out))
	problems := len(pods.crashLooping) + len(pods.failed) + len(pods.notReady)
	switch {
	case pods.total == 0:
		return CheckOutcome{Key: keyKubePods, Status: StatusPass, Message: fmt.Sprintf("%s: no pods to check (completed pods are ignored)", scope), Duration: time.Since(start)}.withValue(0, unitCount)
	case problems == 0:
		return CheckOutcome{Key: keyKubePods, Status: StatusPass, Message: fmt.Sprintf("%s: %d/%d pods Ready", scope, pods.total, pods.total), Duration: time.Since(start)}.withValue(0, unitCount)
	}

	var details []string
	for _, group := range []struct {
		label string
		names []string
	}{
		{"CrashLoopBackOff", pods.crashLooping},
		{"Failed", pods.failed},
		{"not Ready", pods.notReady},
	} {
		if len(group.names) > 0 {
			details = append(details, group.label+": "+nameList(group.names))
		}
	}
	return CheckOutcome{Key: keyKubePods, Status: StatusWarn, Message: fmt.Sprintf("%s: %d/%d pods Ready; %s", scope, pods.total-problems, pods.total, strings.Join(details, "; ")), Duration: time.Since(start)}.
		withValue(float64(problems), unitCount)
}

// podHealth summarizes kubectl get pods output. Each problem pod is in
// exactly one list.
type podHealth struct {
	total        int // pods considered, excluding completed ones
	crashLooping []string
	failed       []string
	notReady     []string // with the first waiting reason, if any: "web-1 (ImagePullBackOff)"
}

// parsePodHealth classifies pods in output produced with podHealthJSONPath.
func parsePodHealth(out string) podHealth {
	var pods podHealth
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) < 3 || !podNamePattern.MatchString(fields[0]) {
			continue
		}
		name, phase, ready := fields[0], fields[1], fields[2]
		var reasons []string
		if len(fields) > 3 {
			reasons = strings.Fields(fields[3])
		}

		if phase == "Succeeded" {
			continue
		}
		pods.total++
		switch {
		case slices.Contains(reasons, "CrashLoopBackOff"):
			pods.crashLooping = append(pods.crashLooping, name)
		case phase == "Failed":
			pods.failed = append(pods.failed, name)
		case ready != "True":
			if len(reasons) > 0 {
				name = fmt.Sprintf("%s (%s)", name, reasons[0])
			}
			pods.notReady = append(pods.notReady, name)
		}
	}
	return pods
}
