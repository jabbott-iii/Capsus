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
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Built-in health check keys.
const (
	keyDiskSpace     = "disk-space"
	keyMemory        = "memory"
	keyCPULoad       = "cpu-load"
	keyDocker        = "docker-status"
	keyKubernetes    = "kubernetes-status"
	keyServiceUptime = "service-uptime"
	keyMisconfig     = "misconfig"
	keyDiskInodes    = "disk-inodes"
	keyKubePods      = "kubernetes-pods"
	keySystemdFailed = "systemd-failed"
	keyTimeSync      = "time-sync"
	keyCertExpiry    = "cert-expiry"
)

// AllCheckKeys lists every built-in check key in default run order. Checks
// added after v1.0.2 are appended, so the existing checks keep their
// positions in --json output.
var AllCheckKeys = []string{
	keyDiskSpace,
	keyMemory,
	keyCPULoad,
	keyDocker,
	keyKubernetes,
	keyServiceUptime,
	keyMisconfig,
	keyDiskInodes,
	keyKubePods,
	keySystemdFailed,
	keyTimeSync,
	keyCertExpiry,
}

// CheckStatus represents the severity of a single health check outcome.
type CheckStatus string

// Supported check statuses, ordered from healthiest to least healthy.
const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckOutcome captures the result of running a single health check.
//
// Target names what one run of a check examined (a mount path, a service, a
// certificate file) when the check can run once per target; it is empty
// otherwise. Value and Unit carry the number the status was decided from,
// when there is one, for monitoring outputs.
type CheckOutcome struct {
	Key      string        `json:"key"`
	Target   string        `json:"target,omitempty"`
	Status   CheckStatus   `json:"status"`
	Message  string        `json:"message"`
	Value    *float64      `json:"value,omitempty"`
	Unit     string        `json:"unit,omitempty"`
	Duration time.Duration `json:"duration_ns"`
}

// Units of CheckOutcome.Value. They are part of the JSON and Prometheus
// output contract.
const (
	unitPercent = "percent"
	unitSeconds = "seconds"
	unitDays    = "days"
	unitCount   = "count"
)

// withValue returns o with its measured value and unit set.
func (o CheckOutcome) withValue(value float64, unit string) CheckOutcome {
	o.Value = &value
	o.Unit = unit
	return o
}

// CheckOptions configures thresholds and targets used by the built-in health checks.
//
// The plural target fields (DiskPaths, ServiceNames, CertPaths) are what the
// CLI sets. RunChecks runs a targeted check once per entry, with the matching
// singular field (DiskPath, ServiceName, CertPath) set to that entry; when a
// plural field is empty, the singular field is the only target.
type CheckOptions struct {
	DiskPaths       []string
	DiskPath        string
	DiskWarnPercent float64
	DiskFailPercent float64

	InodeWarnPercent float64
	InodeFailPercent float64

	MemWarnPercent float64
	MemFailPercent float64

	LoadWarnPercent float64
	LoadFailPercent float64

	ServiceNames []string
	ServiceName  string

	// KubeContext selects a kubeconfig context for kubectl; empty means the
	// current context. KubeNamespace selects the namespace for
	// kubernetes-pods; empty means the context's namespace.
	KubeContext   string
	KubeNamespace string

	// CertPaths are certificate files for cert-expiry. Without any,
	// cert-expiry does not run.
	CertPaths    []string
	CertPath     string
	CertWarnDays int

	CommandTimeout time.Duration

	// CheckTimeout limits how long one check (one target of a targeted
	// check) may run; zero means defaultCheckTimeoutFactor times the command
	// timeout. RunTimeout limits the whole run; zero means no limit.
	// RunChecks reports a check stopped by either limit as FAIL.
	CheckTimeout time.Duration
	RunTimeout   time.Duration

	// Test seams for external tools; nil means runExternal and exec.LookPath.
	// Tests set them so no real docker/kubectl/systemctl runs.
	runCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
	lookPath   func(file string) (string, error)
}

// Defaults applied when the corresponding CheckOptions field is unset (<= 0).
// check run also uses them as its flag defaults, on every platform. The
// resource checks read thresholds through the accessors in health-thresholds.go.
const (
	defaultDiskWarnPercent  = 80.0
	defaultDiskFailPercent  = 90.0
	defaultInodeWarnPercent = 80.0
	defaultInodeFailPercent = 90.0
	defaultMemWarnPercent   = 80.0
	defaultMemFailPercent   = 90.0
	defaultLoadWarnPercent  = 80.0
	defaultLoadFailPercent  = 100.0
	defaultCertWarnDays     = 30
	defaultCommandTimeout   = 3 * time.Second
	defaultDiskPath         = "/"

	// defaultCheckTimeoutFactor sets the default --check-timeout as a multiple
	// of --timeout (30s with the default 3s), so a check has room for each of
	// its external commands (docker-status runs up to three) and raising
	// --timeout never makes checks time out sooner than their commands.
	defaultCheckTimeoutFactor = 10
)

func (o CheckOptions) commandTimeout() time.Duration {
	if o.CommandTimeout <= 0 {
		return defaultCommandTimeout
	}
	return o.CommandTimeout
}

func (o CheckOptions) checkTimeout() time.Duration {
	if o.CheckTimeout > 0 {
		return o.CheckTimeout
	}
	if timeout := o.commandTimeout(); timeout <= math.MaxInt64/defaultCheckTimeoutFactor {
		return defaultCheckTimeoutFactor * timeout
	}
	return math.MaxInt64 // the product would overflow; no limit in practice
}

// command runs an external tool with the command timeout and returns its
// combined output. When the timeout ends the tool, the error says so and no
// output is returned, so a partial line is never mistaken for the tool's
// answer. ctx is the check's context: when the check's or the run's time
// limit passes, or a signal stops the run, the tool is stopped too.
func (o CheckOptions) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	timeout := o.commandTimeout()
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	run := runExternal
	if o.runCommand != nil {
		run = o.runCommand
	}
	out, err := run(cmdCtx, name, args...)
	if err != nil && cmdCtx.Err() != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s stopped: %w", name, context.Cause(ctx))
		}
		return nil, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	return out, err
}

// hasTool reports whether an executable is available on PATH.
func (o CheckOptions) hasTool(file string) bool {
	lookPath := exec.LookPath
	if o.lookPath != nil {
		lookPath = o.lookPath
	}
	_, err := lookPath(file)
	return err == nil
}

// thresholdStatus classifies value against warn/fail thresholds, attaching detail as the message.
func thresholdStatus(value, warn, fail float64, detail string) (CheckStatus, string) {
	switch {
	case value >= fail:
		return StatusFail, detail
	case value >= warn:
		return StatusWarn, detail
	default:
		return StatusPass, detail
	}
}

// firstLine extracts a short, human-readable message from command output or an error.
func firstLine(output string, err error) string {
	output = strings.TrimSpace(output)
	if output != "" {
		return strings.SplitN(output, "\n", 2)[0]
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

// klogLine matches the log lines kubectl writes to stderr, such as
// "E0928 12:00:00.000000   12345 memcache.go:265] ...".
var klogLine = regexp.MustCompile(`^[IWEF]\d{4} \d{2}:\d{2}:\d{2}\.\d+\s`)

// errorLine picks the line of a failed tool's combined output that states the
// error. It skips Docker CLI warnings, kubectl log lines, and the hint that
// kubectl cluster-info prints on every run, and otherwise behaves like
// firstLine.
func errorLine(output string, err error) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "WARNING") || klogLine.MatchString(line) ||
			strings.HasPrefix(line, "To further debug and diagnose cluster problems") {
			continue
		}
		return line
	}
	return firstLine(output, err)
}

// checkFunc runs one check. ctx ends when the check's or the run's time
// limit passes, or when a signal stops the run; checks pass it to
// opts.command so their external tools stop too.
type checkFunc func(ctx context.Context, opts CheckOptions) CheckOutcome

var checkRegistry = map[string]checkFunc{
	keyDiskSpace:     withoutContext(checkDiskSpace),
	keyMemory:        withoutContext(checkMemory),
	keyCPULoad:       withoutContext(checkCPULoad),
	keyDocker:        checkDockerStatus,
	keyKubernetes:    checkKubernetesStatus,
	keyServiceUptime: checkServiceUptime,
	keyMisconfig:     withoutContext(checkMisconfiguration),
	keyDiskInodes:    withoutContext(checkDiskInodes),
	keyKubePods:      checkKubernetesPods,
	keySystemdFailed: checkSystemdFailed,
	keyTimeSync:      checkTimeSync,
	keyCertExpiry:    withoutContext(checkCertExpiry),
}

// withoutContext adapts a check that runs no external tool, and so has
// nothing to stop, to checkFunc. RunChecks still enforces its time limit.
func withoutContext(check func(CheckOptions) CheckOutcome) checkFunc {
	return func(_ context.Context, opts CheckOptions) CheckOutcome { return check(opts) }
}

// checkTarget makes a check run once per target. targets lists them in run
// order without repeats; bind sets one of them for a single run. A check
// whose targets function returns none is skipped.
type checkTarget struct {
	targets func(CheckOptions) []string
	bind    func(*CheckOptions, string)
}

var checkTargets = map[string]checkTarget{
	keyDiskSpace:     {diskTargets, bindDiskPath},
	keyDiskInodes:    {diskTargets, bindDiskPath},
	keyServiceUptime: {serviceTargets, func(o *CheckOptions, name string) { o.ServiceName = name }},
	keyCertExpiry:    {certTargets, func(o *CheckOptions, path string) { o.CertPath = path }},
}

func diskTargets(o CheckOptions) []string {
	single := o.DiskPath
	if single == "" {
		single = defaultDiskPath
	}
	return uniqueTargets(o.DiskPaths, single)
}

func bindDiskPath(o *CheckOptions, path string) { o.DiskPath = path }

// serviceTargets returns the services to check; a single empty name means
// host uptime.
func serviceTargets(o CheckOptions) []string {
	return uniqueTargets(o.ServiceNames, o.ServiceName)
}

// certTargets returns the certificate files to check, or none, which skips
// cert-expiry.
func certTargets(o CheckOptions) []string {
	if len(o.CertPaths) == 0 && o.CertPath == "" {
		return nil
	}
	return uniqueTargets(o.CertPaths, o.CertPath)
}

// uniqueTargets returns list without repeats, in order, or only single when
// list is empty.
func uniqueTargets(list []string, single string) []string {
	if len(list) == 0 {
		return []string{single}
	}
	seen := make(map[string]bool, len(list))
	targets := make([]string, 0, len(list))
	for _, target := range list {
		if !seen[target] {
			seen[target] = true
			targets = append(targets, target)
		}
	}
	return targets
}

// ValidateCheckKeys returns an error for the first key that is not a built-in check.
func ValidateCheckKeys(keys []string) error {
	for _, key := range keys {
		if _, ok := checkRegistry[key]; !ok {
			return fmt.Errorf("unknown check %q", key)
		}
	}
	return nil
}

// Causes recorded when a time limit cancels a check's context.
var (
	errCheckTimeout = errors.New("check time limit reached")
	errRunTimeout   = errors.New("run time limit reached")
)

// RunChecks executes the given check keys (or all built-in checks when keys is empty)
// and returns their outcomes in the order requested. A targeted check (see
// checkTargets) yields one outcome per target, in target order, with Target
// set.
//
// A check that exceeds opts.CheckTimeout, and every check that the run's
// opts.RunTimeout stops or prevents from starting, is reported as FAIL. When
// ctx is cancelled for any other reason, such as a signal, RunChecks stops
// and returns the cancellation cause instead of outcomes.
func RunChecks(ctx context.Context, keys []string, opts CheckOptions) ([]CheckOutcome, error) {
	if len(keys) == 0 {
		keys = AllCheckKeys
	}
	if err := ValidateCheckKeys(keys); err != nil {
		return nil, err
	}

	// A repeated key runs once, so no result (or Prometheus series) repeats.
	seen := make(map[string]bool, len(keys))
	keys = slices.DeleteFunc(slices.Clone(keys), func(key string) bool {
		duplicate := seen[key]
		seen[key] = true
		return duplicate
	})

	if opts.RunTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, opts.RunTimeout, errRunTimeout)
		defer cancel()
	}

	outcomes := make([]CheckOutcome, 0, len(keys))
	run := func(key string, o CheckOptions, target string) error {
		outcome, err := runCheck(ctx, key, o, target)
		if err != nil {
			return err
		}
		outcome.Target = sanitizeMessage(target)
		outcome.Message = sanitizeMessage(outcome.Message)
		outcomes = append(outcomes, outcome)
		return nil
	}
	for _, key := range keys {
		spec, targeted := checkTargets[key]
		if !targeted {
			if err := run(key, opts, ""); err != nil {
				return nil, err
			}
			continue
		}
		for _, target := range spec.targets(opts) {
			o := opts
			spec.bind(&o, target)
			if err := run(key, o, target); err != nil {
				return nil, err
			}
		}
	}
	return outcomes, nil
}

// checkResult carries a check's outcome, or the panic it raised, out of the
// goroutine that runs it.
type checkResult struct {
	outcome  CheckOutcome
	panicked any
	stack    []byte
}

// runCheck runs one check under its time limit (CheckOptions.checkTimeout).
// The check runs in its own goroutine, so a check blocked in a system call,
// such as statfs on an unresponsive NFS mount, cannot stall the run: when
// the limit passes, runCheck reports FAIL and returns. Go cannot stop that
// goroutine. The check's external commands are stopped through its
// context, and anything still blocked ends when the process exits. A panic
// in the check is raised again here, so Main reports it as an internal
// error (exit 3) rather than crashing from another goroutine.
func runCheck(ctx context.Context, key string, o CheckOptions, target string) (CheckOutcome, error) {
	start := time.Now()
	if ctx.Err() != nil {
		return runEnded(ctx, key, target, start, "not run")
	}

	limit := o.checkTimeout()
	checkCtx, cancel := context.WithTimeoutCause(ctx, limit, errCheckTimeout)
	defer cancel()

	check := checkRegistry[key]
	done := make(chan checkResult, 1) // buffered, so an abandoned check can still finish
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- checkResult{panicked: r, stack: debug.Stack()}
			}
		}()
		done <- checkResult{outcome: check(checkCtx, o)}
	}()

	select {
	case r := <-done:
		if r.panicked != nil {
			panic(fmt.Sprintf("check %s panicked: %v\n\n%s", key, r.panicked, r.stack))
		}
		return r.outcome, nil
	case <-checkCtx.Done():
		// exec kills the check's tools from its own goroutine. Give the check
		// a moment to return, so the kill has happened before Salus goes on
		// (or exits, after a signal); a tool left running would have no time
		// limit. A check blocked in a system call is abandoned after this.
		select {
		case <-done:
		case <-time.After(commandWaitDelay + 100*time.Millisecond):
		}
	}
	if errors.Is(context.Cause(checkCtx), errCheckTimeout) {
		return stoppedOutcome(key, target, start, fmt.Sprintf("did not finish within %s (--check-timeout)", limit)), nil
	}
	return runEnded(ctx, key, target, start, "stopped")
}

// runEnded handles a check whose run context has ended. The run's time limit
// yields a FAIL outcome (verb says whether the check was stopped or never
// started); any other cause, such as a signal, is returned as the error.
func runEnded(ctx context.Context, key, target string, start time.Time, verb string) (CheckOutcome, error) {
	cause := context.Cause(ctx)
	if !errors.Is(cause, errRunTimeout) {
		return CheckOutcome{}, cause
	}
	return stoppedOutcome(key, target, start, verb+": the run time limit was reached (--run-timeout)"), nil
}

// stoppedOutcome is the FAIL outcome of a check that a time limit stopped.
// Text output prints only the key, so the message names the target.
func stoppedOutcome(key, target string, start time.Time, msg string) CheckOutcome {
	if target != "" {
		msg = target + ": " + msg
	}
	return CheckOutcome{Key: key, Status: StatusFail, Message: msg, Duration: time.Since(start)}
}

// sanitizeMessage replaces control characters with '?'. Messages embed
// external tool output, which a hostile Docker daemon or Kubernetes API server
// could fill with terminal escape sequences; after this, a message can neither
// drive the terminal nor span lines (SEC-009).
func sanitizeMessage(msg string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, msg)
}

//--------------------------------------------------container & orchestration checks-------------------------------------------------------------------//

func checkDockerStatus(ctx context.Context, opts CheckOptions) CheckOutcome {
	start := time.Now()

	if !opts.hasTool("docker") {
		return CheckOutcome{Key: keyDocker, Status: StatusWarn, Message: "docker CLI not found in PATH", Duration: time.Since(start)}
	}

	out, err := opts.command(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		return CheckOutcome{Key: keyDocker, Status: StatusFail, Message: fmt.Sprintf("docker daemon unreachable: %s", errorLine(string(out), err)), Duration: time.Since(start)}
	}
	msg := fmt.Sprintf("docker daemon reachable (server version %s)", dockerServerVersion(string(out)))

	// A reachable daemon can still run broken workloads. Containers failing
	// their HEALTHCHECK or restarting are a WARN: the runtime itself works.
	var problems []string
	count := 0 // containers listed as unhealthy or restarting (one can be in both)
	for _, query := range []struct {
		label string
		args  []string
	}{
		{"unhealthy", []string{"ps", "--filter", "health=unhealthy", "--format", "{{.Names}}"}},
		{"restarting", []string{"ps", "--all", "--filter", "status=restarting", "--format", "{{.Names}}"}},
	} {
		out, err := opts.command(ctx, "docker", query.args...)
		if err != nil {
			return CheckOutcome{Key: keyDocker, Status: StatusWarn, Message: fmt.Sprintf("%s; container health unknown: %s", msg, errorLine(string(out), err)), Duration: time.Since(start)}
		}
		if names := containerNames(string(out)); len(names) > 0 {
			problems = append(problems, query.label+": "+nameList(names))
			count += len(names)
		}
	}

	if len(problems) > 0 {
		return CheckOutcome{Key: keyDocker, Status: StatusWarn, Message: msg + "; " + strings.Join(problems, "; "), Duration: time.Since(start)}.
			withValue(float64(count), unitCount)
	}
	return CheckOutcome{Key: keyDocker, Status: StatusPass, Message: msg, Duration: time.Since(start)}.withValue(0, unitCount)
}

// dockerServerVersion returns the version from docker info output. The output
// is combined with stderr, so blank and WARNING lines are skipped.
func dockerServerVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "WARNING") {
			return line
		}
	}
	return "unknown"
}

// containerNamePattern matches one line of docker ps --format {{.Names}}
// output; legacy links add comma-separated names with slashes.
var containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.,/-]*$`)

// containerNames returns the container names in docker ps output, ignoring
// warnings the docker CLI may print to stderr.
func containerNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); containerNamePattern.MatchString(line) {
			names = append(names, line)
		}
	}
	return names
}

// maxListedNames bounds how many names one message lists.
const maxListedNames = 5

// nameList joins names for a single-line message, listing at most
// maxListedNames and counting the rest.
func nameList(names []string) string {
	if len(names) <= maxListedNames {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListedNames], ", "), len(names)-maxListedNames)
}

// nodeReadinessJSONPath prints one line per node: its name, then the status
// of its Ready, MemoryPressure, DiskPressure, and PIDPressure conditions,
// separated by tabs.
const nodeReadinessJSONPath = `jsonpath={range .items[*]}{.metadata.name}{"\t"}{.status.conditions[?(@.type=="Ready")].status}{"\t"}{.status.conditions[?(@.type=="MemoryPressure")].status}{"\t"}{.status.conditions[?(@.type=="DiskPressure")].status}{"\t"}{.status.conditions[?(@.type=="PIDPressure")].status}{"\n"}{end}`

// nodePressureConditions are the node conditions, in nodeReadinessJSONPath
// column order after Ready, that report resource pressure when True.
var nodePressureConditions = []string{"MemoryPressure", "DiskPressure", "PIDPressure"}

// kubectlFor validates the --kube-context option and returns a kubectl runner
// bound to it, plus a description of the cluster for messages. A non-nil
// outcome means the check stops with that result.
func kubectlFor(ctx context.Context, opts CheckOptions, key string, start time.Time) (func(args ...string) ([]byte, error), string, *CheckOutcome) {
	kubeContext := strings.TrimSpace(opts.KubeContext)

	if kubeContext != "" && !validKubeContext(kubeContext) {
		return nil, "", &CheckOutcome{Key: key, Status: StatusFail, Message: fmt.Sprintf("invalid kubeconfig context %q: it must not start with - or contain control characters", kubeContext), Duration: time.Since(start)}
	}

	if !opts.hasTool("kubectl") {
		return nil, "", &CheckOutcome{Key: key, Status: StatusWarn, Message: "kubectl CLI not found in PATH", Duration: time.Since(start)}
	}

	// A single "--context=<name>" argument binds the value to the flag, so the
	// name can never be read as another kubectl option.
	kubectl := func(args ...string) ([]byte, error) {
		if kubeContext != "" {
			args = append([]string{"--context=" + kubeContext}, args...)
		}
		return opts.command(ctx, "kubectl", args...)
	}
	cluster := "kubernetes cluster"
	if kubeContext != "" {
		cluster = fmt.Sprintf("kubernetes cluster (context %s)", kubeContext)
	}
	return kubectl, cluster, nil
}

func checkKubernetesStatus(ctx context.Context, opts CheckOptions) CheckOutcome {
	start := time.Now()
	kubectl, cluster, stop := kubectlFor(ctx, opts, keyKubernetes, start)
	if stop != nil {
		return *stop
	}

	// cluster-info lists Services in kube-system, which namespace-scoped users
	// may not do. A Forbidden answer still proves the API server is reachable.
	out, err := kubectl("cluster-info")
	if err != nil && !forbidden(string(out)) {
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("%s unreachable: %s", cluster, errorLine(string(out), err)), Duration: time.Since(start)}
	}

	out, err = kubectl("get", "nodes", "-o", nodeReadinessJSONPath)
	if err != nil {
		if forbidden(string(out)) {
			return CheckOutcome{Key: keyKubernetes, Status: StatusPass, Message: fmt.Sprintf("%s reachable; node readiness not checked (listing nodes is forbidden)", cluster), Duration: time.Since(start)}
		}
		return CheckOutcome{Key: keyKubernetes, Status: StatusWarn, Message: fmt.Sprintf("%s reachable; node readiness unknown: %s", cluster, errorLine(string(out), err)), Duration: time.Since(start)}
	}

	nodes := parseNodeReadiness(string(out))
	total := nodes.ready + len(nodes.notReady)
	// Pressure is reported in addition to readiness; it is a WARN (Q-014).
	pressure := ""
	if len(nodes.pressure) > 0 {
		pressure = "; under pressure: " + nameList(nodes.pressure)
	}
	switch {
	case total == 0:
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("%s reachable, but it has no nodes", cluster), Duration: time.Since(start)}
	case nodes.ready == 0:
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("%s reachable, but no nodes are Ready (0/%d); NotReady: %s%s", cluster, total, nameList(nodes.notReady), pressure), Duration: time.Since(start)}
	case len(nodes.notReady) > 0:
		return CheckOutcome{Key: keyKubernetes, Status: StatusWarn, Message: fmt.Sprintf("%s reachable; %d/%d nodes Ready; NotReady: %s%s", cluster, nodes.ready, total, nameList(nodes.notReady), pressure), Duration: time.Since(start)}
	case len(nodes.pressure) > 0:
		return CheckOutcome{Key: keyKubernetes, Status: StatusWarn, Message: fmt.Sprintf("%s reachable; %d/%d nodes Ready%s", cluster, nodes.ready, total, pressure), Duration: time.Since(start)}
	default:
		return CheckOutcome{Key: keyKubernetes, Status: StatusPass, Message: fmt.Sprintf("%s reachable; %d/%d nodes Ready", cluster, nodes.ready, total), Duration: time.Since(start)}
	}
}

// nodeHealth summarizes kubectl get nodes output.
type nodeHealth struct {
	ready    int
	notReady []string
	// pressure lists nodes with a True pressure condition, as
	// "name (MemoryPressure, DiskPressure)".
	pressure []string
}

// parseNodeReadiness counts Ready nodes and names the others and those under
// pressure, in output produced with nodeReadinessJSONPath. Lines without a
// tab, such as kubectl warnings on stderr, are ignored. Missing pressure
// columns count as no pressure.
func parseNodeReadiness(out string) nodeHealth {
	var nodes nodeHealth
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		name := fields[0]
		if len(fields) < 2 || name == "" || strings.ContainsAny(name, " ") {
			continue
		}
		if fields[1] == "True" {
			nodes.ready++
		} else {
			nodes.notReady = append(nodes.notReady, name)
		}
		var conditions []string
		for i, condition := range nodePressureConditions {
			if len(fields) > i+2 && fields[i+2] == "True" {
				conditions = append(conditions, condition)
			}
		}
		if len(conditions) > 0 {
			nodes.pressure = append(nodes.pressure, fmt.Sprintf("%s (%s)", name, strings.Join(conditions, ", ")))
		}
	}
	return nodes
}

// forbidden reports whether kubectl output carries an HTTP 403 from the API
// server, as in `Error from server (Forbidden): nodes is forbidden: ...`.
func forbidden(output string) bool {
	return strings.Contains(output, "(Forbidden)") || strings.Contains(output, " is forbidden:")
}

// validKubeContext reports whether name is an acceptable --kube-context value:
// valid UTF-8 of at most 253 bytes, with no control characters, and not
// starting with "-". Kubeconfig context names have no fixed character set,
// such as arn:aws:eks:region:account:cluster/name or "prod (eu)". The name is
// passed as the single argument --context=<name>, so it cannot become
// another kubectl option.
func validKubeContext(name string) bool {
	return len(name) <= 253 && utf8.ValidString(name) && !strings.HasPrefix(name, "-") &&
		!strings.ContainsFunc(name, unicode.IsControl)
}

//--------------------------------------------------service & configuration checks---------------------------------------------------------------------//

func checkServiceUptime(ctx context.Context, opts CheckOptions) CheckOutcome {
	start := time.Now()
	name := strings.TrimSpace(opts.ServiceName)

	if name == "" {
		return checkProcessUptime(start)
	}

	if runtime.GOOS != "linux" {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "service uptime check requires systemd (Linux only)", Duration: time.Since(start)}
	}

	if !validUnitName(name) {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusFail, Message: fmt.Sprintf("invalid service name %q: use a systemd unit name such as nginx or nginx.service", name), Duration: time.Since(start)}
	}

	if !opts.hasTool("systemctl") {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "systemctl not found in PATH", Duration: time.Since(start)}
	}

	// "--" ends option parsing, so the name can never be read as a systemctl flag.
	out, err := opts.command(ctx, "systemctl", "is-active", "--", name)
	// Only the first line is used so messages stay single-line in reports.
	state := firstLine(string(out), err)

	switch {
	case err == nil && state == "active":
		return CheckOutcome{Key: keyServiceUptime, Status: StatusPass, Message: fmt.Sprintf("service %q is active", name), Duration: time.Since(start)}
	case state == "activating" || state == "reloading":
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: fmt.Sprintf("service %q is %s", name, state), Duration: time.Since(start)}
	default:
		return CheckOutcome{Key: keyServiceUptime, Status: StatusFail, Message: fmt.Sprintf("service %q is not active (%s)", name, state), Duration: time.Since(start)}
	}
}

// unitNamePattern matches systemd unit names: letters, digits, and ":-_.\@",
// not starting with "-" (which systemctl would parse as an option; SEC-001).
var unitNamePattern = regexp.MustCompile(`^[A-Za-z0-9:_.\\@][A-Za-z0-9:_.\\@-]*$`)

// validUnitName reports whether name is an acceptable --service value.
func validUnitName(name string) bool {
	return len(name) <= 255 && unitNamePattern.MatchString(name)
}

func checkProcessUptime(start time.Time) CheckOutcome {
	uptime, err := readSystemUptime()
	if err != nil {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "no --service provided and host uptime unavailable: " + err.Error(), Duration: time.Since(start)}
	}
	return CheckOutcome{Key: keyServiceUptime, Status: StatusPass, Message: fmt.Sprintf("host has been up for %s", uptime.Round(time.Second)), Duration: time.Since(start)}.
		withValue(uptime.Seconds(), unitSeconds)
}

// misconfigRule is one misconfiguration test. Its id prefixes every problem
// it reports, so scripts can match problems by a stable identifier.
type misconfigRule struct {
	id    string
	check func() []string
}

// misconfigRules run in this order. The ids are a documented contract. The
// permission rules use POSIX mode bits and skip Windows, where Go reports 0666
// for every writable file (ACLs are not mode bits).
var misconfigRules = []misconfigRule{
	{"home-unset", homeUnset},
	{"db-permissions", databasePermissions},
	{"kubeconfig-permissions", kubeconfigPermissions},
	{"docker-socket-permissions", dockerSocketPermissions},
	{"path-world-writable", worldWritablePath},
	{"docker-tcp-insecure", dockerTCPInsecure},
	{"sshd-root-login", sshdRootLogin},
	{"sshd-password-auth", sshdPasswordAuth},
}

func checkMisconfiguration(opts CheckOptions) CheckOutcome {
	start := time.Now()
	var warnings []string

	for _, rule := range misconfigRules {
		for _, problem := range rule.check() {
			warnings = append(warnings, rule.id+": "+problem)
		}
	}

	if len(warnings) == 0 {
		return CheckOutcome{Key: keyMisconfig, Status: StatusPass, Message: "no common misconfigurations detected", Duration: time.Since(start)}.
			withValue(0, unitCount)
	}

	return CheckOutcome{Key: keyMisconfig, Status: StatusWarn, Message: strings.Join(warnings, "; "), Duration: time.Since(start)}.
		withValue(float64(len(warnings)), unitCount)
}

func homeUnset() []string {
	if _, ok := os.LookupEnv("HOME"); !ok && runtime.GOOS != "windows" {
		return []string{"HOME environment variable is not set"}
	}
	return nil
}

// databasePermissions checks the database Salus actually uses (SALUS_DB_PATH
// or the per-user default).
func databasePermissions() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	dbPath, err := DatabasePath()
	if err != nil {
		return nil
	}
	file, ok := databaseFile(dbPath)
	if !ok {
		return nil
	}
	return ownerOnly(file)
}

// kubeconfigPermissions checks the files kubectl reads, which hold cluster
// credentials: the KUBECONFIG list, or ~/.kube/config.
func kubeconfigPermissions() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	paths := filepath.SplitList(os.Getenv("KUBECONFIG"))
	if len(paths) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		paths = []string{filepath.Join(home, ".kube", "config")}
	}

	var problems []string
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		problems = append(problems, ownerOnly(path)...)
	}
	return problems
}

// ownerOnly reports an existing file that grants any group/other permission,
// unless its mode bits are made up (WSL drvfs; see syntheticModes).
func ownerOnly(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 && !syntheticModes(path) {
		return []string{fmt.Sprintf("%s is accessible by group/other (mode %04o, want 0600; run chmod 600 on it)", path, perm)}
	}
	return nil
}

// dockerSocketPermissions warns when every local user can write to the Docker
// socket the CLI uses, which gives them root-equivalent control of the host.
// Access for the docker group is normal.
func dockerSocketPermissions() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	path, ok := dockerSocketPath()
	if !ok {
		return nil // a remote daemon has no local socket
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if perm := info.Mode().Perm(); perm&0o002 != 0 {
		return []string{fmt.Sprintf("%s is writable by all users (mode %04o), which gives every local user control of Docker", path, perm)}
	}
	return nil
}

// dockerSocketPath returns the local socket named by DOCKER_HOST (unix://),
// or the default socket. It reports false for a remote daemon. Docker
// contexts, which can select another endpoint, are not consulted.
func dockerSocketPath() (string, bool) {
	const defaultSocket = "/var/run/docker.sock"
	host := os.Getenv("DOCKER_HOST")
	path, ok := strings.CutPrefix(host, "unix://")
	switch {
	case host == "" || (ok && path == ""):
		return defaultSocket, true
	case !ok:
		return "", false
	default:
		return path, true
	}
}

// dockerTCPInsecure warns when the Docker CLI is pointed at a TCP endpoint
// without TLS verification. Without verification, anyone on the path to the
// daemon can impersonate it, and a daemon that accepts such connections lets
// anyone who can reach it control the host as root. Like the Docker CLI, any
// non-empty DOCKER_TLS_VERIFY counts as enabled. Docker contexts and the
// --tlsverify flag are not consulted.
func dockerTCPInsecure() []string {
	host := os.Getenv("DOCKER_HOST")
	if !strings.HasPrefix(strings.ToLower(host), "tcp://") || os.Getenv("DOCKER_TLS_VERIFY") != "" {
		return nil
	}
	endpoint := host[len("tcp://"):]
	// Only host and port are reported; anything after them is dropped.
	if i := strings.IndexAny(endpoint, "/?#"); i >= 0 {
		endpoint = endpoint[:i]
	}
	if i := strings.LastIndex(endpoint, "@"); i >= 0 {
		endpoint = endpoint[i+1:]
	}
	return []string{fmt.Sprintf("DOCKER_HOST uses tcp://%s without TLS verification (set DOCKER_TLS_VERIFY=1 and DOCKER_CERT_PATH, or use ssh:// or a unix socket)", endpoint)}
}

// worldWritablePath warns about PATH directories that every user can write
// to: anyone could add a program there that shadows a trusted one. An empty
// entry means the current directory. WSL drvfs directories (the Windows PATH
// that WSL appends) report a made-up 0777 and are skipped.
func worldWritablePath() []string {
	if runtime.GOOS == "windows" {
		return nil
	}

	var dirs []string
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if perm := info.Mode().Perm(); perm&0o002 != 0 && !syntheticModes(dir) {
			dirs = append(dirs, fmt.Sprintf("%s (mode %04o)", dir, perm))
		}
	}

	if len(dirs) == 0 {
		return nil
	}
	return []string{"PATH directories writable by all users: " + nameList(dirs)}
}
