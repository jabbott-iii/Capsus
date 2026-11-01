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
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// DatabaseOpener returns the database, opening it on first use. Commands call
// it only when they need storage, so help, --version, completion, and
// check run --no-save never create a database file.
type DatabaseOpener func() (*Database, error)

// NewRootCmd builds the Salus CLI command tree.
func NewRootCmd(openDB DatabaseOpener) *cobra.Command {
	root := &cobra.Command{
		Use:   "salus",
		Short: "Salus is an environment health checker",
		Long: "Salus verifies disk space and inodes, memory, CPU load, Docker and Kubernetes status, Kubernetes pods, service uptime, " +
			"failed systemd units, time synchronization, certificate expiry, and common misconfigurations.",
		// Cobra would print errors and usage through the output writer, which
		// mixes them into stdout (and --json). The caller reports errors on
		// stderr instead; see runWith in pkg/main.go.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(newCheckCmd(openDB))
	root.AddCommand(newJobsCmd(openDB))

	return root
}

// runGroup shows help for a command group and rejects unknown subcommands.
// Without it Cobra prints help and exits 0 for a mistyped subcommand such as
// "salus check rn", which would let a broken CI step pass.
func runGroup(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "; did you mean " + strings.Join(suggestions, " or ") + "?"
	}
	return errors.New(msg)
}

func newCheckCmd(openDB DatabaseOpener) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run or list environment health checks",
		RunE:  runGroup,
		// Cobra only defaults this for the root command's suggestions.
		SuggestionsMinimumDistance: 2,
	}

	cmd.AddCommand(newCheckListCmd(openDB))
	cmd.AddCommand(newCheckRunCmd(openDB))

	return cmd
}

func newCheckListCmd(openDB DatabaseOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available health checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			features, err := ListFeatures(db)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			for _, f := range features {
				if _, err := fmt.Fprintf(out, "%-20s %-20s %s\n", f.Key, f.Category.Name, f.Description); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newCheckRunCmd(openDB DatabaseOpener) *cobra.Command {
	return newCheckRunCmdWith(openDB, RunChecks)
}

// newCheckRunCmdWith builds check run around the function that runs the
// checks, so tests can inspect the options produced by the flags.
func newCheckRunCmdWith(openDB DatabaseOpener, runChecks func(context.Context, []string, CheckOptions) ([]CheckOutcome, error)) *cobra.Command {
	var (
		only       []string
		opts       CheckOptions
		jsonOutput bool
		failOnly   bool
		quiet      bool
		noSave     bool
		format     string
		outputPath string
		failOnFlag string
		retain     string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run health checks and report the results",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Every input is validated before the database opens or a check
			// runs, so mistakes surface fast and change nothing (exit 3).
			if err := ValidateCheckKeys(only); err != nil {
				return err
			}
			if jsonOutput {
				if cmd.Flags().Changed("format") && format != formatJSON {
					return fmt.Errorf("--json conflicts with --format %s; use one of them", format)
				}
				format = formatJSON
			}
			if !slices.Contains(reportFormats, format) {
				return fmt.Errorf("invalid --format value %q: use %s", format, strings.Join(reportFormats, ", "))
			}
			failOn, err := parseFailOn(failOnFlag)
			if err != nil {
				return err
			}
			if err := validateLimits(opts); err != nil {
				return err
			}
			if slices.Contains(only, keyCertExpiry) && len(certTargets(opts)) == 0 {
				return errors.New("cert-expiry needs at least one --cert file")
			}
			var retainAge time.Duration
			if retain != "" {
				if noSave {
					return errors.New("--retain prunes saved runs, so it cannot be used with --no-save")
				}
				if retainAge, err = parseAge(retain); err != nil {
					return fmt.Errorf("invalid --retain value %q: %w", retain, err)
				}
			}
			if outputPath != "" {
				if err := checkOutputPath(outputPath); err != nil {
					return err
				}
			}

			// Open storage before running checks, so a database problem is
			// reported before slow checks run rather than after.
			var db *Database
			if !noSave {
				if db, err = openDB(); err != nil {
					return err
				}
			}

			// SIGINT or SIGTERM while checks run stops them, including the
			// external tools they started, and ends the run with exit 3: no
			// report is written and nothing is saved. A second signal stops
			// Salus at once (the default handling is restored after the
			// first). Once the checks have finished, the report and the save
			// complete before the signal takes effect.
			ctx, stop := stopSignalContext(cmd.Context())
			defer stop()
			context.AfterFunc(ctx, stop)

			startedAt := time.Now()
			outcomes, err := runChecks(ctx, only, opts)
			if err == nil && ctx.Err() != nil {
				// The signal can also reach the tools (systemd signals the
				// whole control group) and end them before the context is
				// cancelled; their checks then report the signal as a
				// failure. The run was still interrupted.
				err = context.Cause(ctx)
			}
			if err != nil {
				if ctx.Err() != nil {
					return fmt.Errorf("interrupted (%w); no report was written and the run was not saved", context.Cause(ctx))
				}
				return err
			}
			finishedAt := time.Now()

			code := exitCodeWithFailOn(WorstStatus(outcomes), failOn)
			report := runReport{outcomes: outcomes, startedAt: startedAt, finishedAt: finishedAt, exitCode: code, failOn: failOn, failOnly: failOnly}
			write := func(w io.Writer) error { return writeReport(w, format, report) }
			switch {
			case outputPath != "":
				// --quiet only silences standard output; the file is the
				// point of --output.
				if err := writeFileAtomic(outputPath, write); err != nil {
					return err
				}
			case quiet:
				// --quiet suppresses report output, including --json.
			default:
				if err := write(cmd.OutOrStdout()); err != nil {
					return err
				}
			}

			// Saving and pruning run after the report, so a database failure
			// (for example a lock held past the busy timeout, or a full disk)
			// still leaves the report written. Either failure exits 3.
			if !noSave {
				if _, err := RecordScan(db, startedAt, outcomes); err != nil {
					return fmt.Errorf("record scan: %w (the report was written; this run is not in the history)", err)
				}
			}
			if retainAge > 0 {
				// The cutoff never passes this run's start, so the run just
				// recorded is kept even with a tiny age.
				cutoff := time.Now().Add(-retainAge)
				if cutoff.After(startedAt) {
					cutoff = startedAt
				}
				if _, err := PruneScanJobs(db, cutoff, false); err != nil {
					return fmt.Errorf("--retain: %w", err)
				}
			}

			if code != ExitCodePass {
				// A WARN or FAIL result is not a usage problem, so Cobra must not
				// print the error or the usage text; Main maps it to the exit code.
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				return &ExitStatusError{Code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&only, "only", nil, "comma-separated list of checks to run (default: all)")
	cmd.Flags().StringSliceVar(&opts.ServiceNames, "service", nil, "systemd service to check; repeat or comma-separate for several (default: host uptime)")
	cmd.Flags().StringVar(&opts.KubeContext, "kube-context", "", "kubeconfig context for the Kubernetes checks (defaults to kubectl's current context)")
	cmd.Flags().StringVar(&opts.KubeNamespace, "kube-namespace", "", "namespace for the kubernetes-pods check (defaults to the context's namespace)")
	cmd.Flags().StringArrayVar(&opts.DiskPaths, "disk-path", []string{defaultDiskPath}, "mount path for the disk-space and disk-inodes checks; repeat for several")
	cmd.Flags().Float64Var(&opts.DiskWarnPercent, "disk-warn", defaultDiskWarnPercent, "disk usage percent at which disk-space reports WARN")
	cmd.Flags().Float64Var(&opts.DiskFailPercent, "disk-fail", defaultDiskFailPercent, "disk usage percent at which disk-space reports FAIL")
	cmd.Flags().Float64Var(&opts.InodeWarnPercent, "inode-warn", defaultInodeWarnPercent, "inode usage percent at which disk-inodes reports WARN")
	cmd.Flags().Float64Var(&opts.InodeFailPercent, "inode-fail", defaultInodeFailPercent, "inode usage percent at which disk-inodes reports FAIL")
	cmd.Flags().Float64Var(&opts.MemWarnPercent, "mem-warn", defaultMemWarnPercent, "memory usage percent at which memory reports WARN")
	cmd.Flags().Float64Var(&opts.MemFailPercent, "mem-fail", defaultMemFailPercent, "memory usage percent at which memory reports FAIL")
	cmd.Flags().Float64Var(&opts.LoadWarnPercent, "load-warn", defaultLoadWarnPercent, "1-minute load average per CPU, in percent, at which cpu-load reports WARN")
	cmd.Flags().Float64Var(&opts.LoadFailPercent, "load-fail", defaultLoadFailPercent, "1-minute load average per CPU, in percent, at which cpu-load reports FAIL")
	cmd.Flags().StringArrayVar(&opts.CertPaths, "cert", nil, "certificate file (PEM or DER) for the cert-expiry check; repeat for several")
	cmd.Flags().IntVar(&opts.CertWarnDays, "cert-warn-days", defaultCertWarnDays, "days before expiry at which cert-expiry reports WARN")
	cmd.Flags().DurationVar(&opts.CommandTimeout, "timeout", defaultCommandTimeout, "time limit for each external command (docker, kubectl, systemctl, timedatectl)")
	cmd.Flags().DurationVar(&opts.CheckTimeout, "check-timeout", 0, "time limit for each check and target, after which it reports FAIL (default 10 times --timeout)")
	cmd.Flags().DurationVar(&opts.RunTimeout, "run-timeout", 0, "time limit for the whole run; checks it stops or never starts report FAIL (default no limit)")
	cmd.Flags().StringVar(&format, "format", formatText, "report format: "+strings.Join(reportFormats, ", "))
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output results as JSON (same as --format json)")
	cmd.Flags().StringVar(&outputPath, "output", "", "write the report to this file, replacing it atomically, instead of standard output")
	cmd.Flags().BoolVar(&failOnly, "fail-only", false, "only show WARN and FAIL results in text and nagios output")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress report output on standard output, including --json (still sets the exit code)")
	cmd.Flags().StringVar(&failOnFlag, "fail-on", "warn", "lowest status that makes the exit code non-zero: warn or fail")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "do not persist this run to the database")
	cmd.Flags().StringVar(&retain, "retain", "", "after saving this run, delete saved runs older than this age, such as 30d or 12h")

	return cmd
}

// stopSignalContext returns a context that SIGINT or SIGTERM cancels. A
// signal the process inherited as ignored, such as SIGINT for a background
// job started by a non-interactive shell, stays ignored.
func stopSignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	var signals []os.Signal
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		if !signal.Ignored(sig) {
			signals = append(signals, sig)
		}
	}
	if len(signals) == 0 {
		// signal.NotifyContext with no signals would catch every signal.
		return context.WithCancel(parent)
	}
	return signal.NotifyContext(parent, signals...)
}

// checkOutputPath rejects an --output path that writeFileAtomic cannot
// replace, before any check runs or anything is recorded: its directory must
// exist, and an existing entry must be a regular file, not a directory,
// device, or symbolic link (renaming over a link would replace the link and
// leave its target stale).
func checkOutputPath(path string) error {
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		return fmt.Errorf("invalid --output %s: %w", path, err)
	} else if !info.IsDir() {
		return fmt.Errorf("invalid --output %s: %s is not a directory", path, dir)
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("invalid --output %s: not a regular file (give the path of the file itself, not a link or directory)", path)
	}
	return nil
}

// parseFailOn parses --fail-on: the lowest status that makes check run exit
// non-zero.
func parseFailOn(value string) (CheckStatus, error) {
	switch value {
	case "warn":
		return StatusWarn, nil
	case "fail":
		return StatusFail, nil
	}
	return "", fmt.Errorf("invalid --fail-on value %q: use warn or fail", value)
}

// exitCodeWithFailOn maps the worst status to an exit code, treating WARN as
// passing when failOn is StatusFail.
func exitCodeWithFailOn(worst, failOn CheckStatus) int {
	if failOn == StatusFail && worst == StatusWarn {
		return ExitCodePass
	}
	return ExitCodeFor(worst)
}

// validateLimits rejects threshold and timeout flag values that cannot work:
// every threshold must be a finite number above 0, each warn threshold must be
// below its fail threshold, disk, inode, and memory thresholds are percentages
// of capacity (at most 100), and the timeout and certificate warning days must
// be positive. Load can exceed 100 percent.
func validateLimits(opts CheckOptions) error {
	pairs := []struct {
		name       string
		warn, fail float64
		max        float64 // 0 means no upper bound
	}{
		{"disk", opts.DiskWarnPercent, opts.DiskFailPercent, 100},
		{"inode", opts.InodeWarnPercent, opts.InodeFailPercent, 100},
		{"mem", opts.MemWarnPercent, opts.MemFailPercent, 100},
		{"load", opts.LoadWarnPercent, opts.LoadFailPercent, 0},
	}

	for _, p := range pairs {
		for _, f := range []struct {
			flag  string
			value float64
		}{{"--" + p.name + "-warn", p.warn}, {"--" + p.name + "-fail", p.fail}} {
			// pflag parses "NaN" and "Inf", so they are rejected explicitly.
			if math.IsNaN(f.value) || math.IsInf(f.value, 0) || f.value <= 0 {
				return fmt.Errorf("invalid %s value %g: must be a finite number greater than 0", f.flag, f.value)
			}
			if p.max > 0 && f.value > p.max {
				return fmt.Errorf("invalid %s value %g: must be at most %g", f.flag, f.value, p.max)
			}
		}
		if p.warn >= p.fail {
			return fmt.Errorf("--%s-warn (%g) must be less than --%s-fail (%g)", p.name, p.warn, p.name, p.fail)
		}
	}

	if opts.CommandTimeout <= 0 {
		return fmt.Errorf("invalid --timeout value %s: must be greater than 0", opts.CommandTimeout)
	}
	// Zero means the default for both limits.
	if opts.CheckTimeout < 0 {
		return fmt.Errorf("invalid --check-timeout value %s: must not be negative", opts.CheckTimeout)
	}
	if opts.CheckTimeout > 0 && opts.CheckTimeout < opts.CommandTimeout {
		return fmt.Errorf("--check-timeout (%s) must not be less than --timeout (%s)", opts.CheckTimeout, opts.CommandTimeout)
	}
	if opts.RunTimeout < 0 {
		return fmt.Errorf("invalid --run-timeout value %s: must not be negative", opts.RunTimeout)
	}
	if opts.CertWarnDays <= 0 {
		return fmt.Errorf("invalid --cert-warn-days value %d: must be greater than 0", opts.CertWarnDays)
	}
	return nil
}

func newJobsCmd(openDB DatabaseOpener) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "View past health check runs",
		RunE:  runGroup,
		// Cobra only defaults this for the root command's suggestions.
		SuggestionsMinimumDistance: 2,
	}

	cmd.AddCommand(newJobsListCmd(openDB))
	cmd.AddCommand(newJobsShowCmd(openDB))
	cmd.AddCommand(newJobsPruneCmd(openDB))
	cmd.AddCommand(newJobsDiffCmd(openDB))
	cmd.AddCommand(newJobsStatsCmd(openDB))

	return cmd
}

func newJobsListCmd(openDB DatabaseOpener) *cobra.Command {
	var (
		limit      int
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent health check runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			jobs, err := ListScanJobs(db, limit)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if jsonOutput {
				return WriteJobsJSON(out, jobs)
			}
			for _, j := range jobs {
				finished := "running"
				if j.FinishedAt != nil {
					finished = j.FinishedAt.Format(time.RFC3339)
				}
				if _, err := fmt.Fprintf(out, "%-4d %-9s %-25s %-25s %s\n", j.ID, j.Status, j.StartedAt.Format(time.RFC3339), finished, j.Summary); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of jobs to list")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output jobs as JSON")

	return cmd
}

func newJobsShowCmd(openDB DatabaseOpener) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "show [job-id]",
		Short: "Show details for a specific health check run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseJobID(args[0])
			if err != nil {
				return err
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			job, results, err := GetScanJob(db, id)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if jsonOutput {
				return WriteJobJSON(out, job, results)
			}
			if _, err := fmt.Fprintf(out, "Job %d: %s (%s)\n", job.ID, job.Status, job.Summary); err != nil {
				return err
			}
			for _, r := range results {
				// Rows written before SEC-009 may still hold control characters.
				if _, err := fmt.Fprintf(out, "[%s] %-17s %s\n", r.Status, r.Key, sanitizeMessage(r.Message)); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the job and its results as JSON")

	return cmd
}

// parseJobID parses a job id argument.
func parseJobID(arg string) (uint, error) {
	id, err := strconv.ParseUint(arg, 10, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("invalid job id %q: %w", arg, err)
	}
	return uint(id), nil
}

func newJobsDiffCmd(openDB DatabaseOpener) *cobra.Command {
	var (
		jsonOutput bool
		exitCode   bool
	)

	cmd := &cobra.Command{
		Use:   "diff [from-id] [to-id]",
		Short: "Show which check results changed between two runs",
		Long: "Compare two recorded runs check by check (and target by target). With no ids, compare the two most recent runs; " +
			"with one id, compare that run with the most recent run.",
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := make([]uint, 0, len(args))
			for _, arg := range args {
				id, err := parseJobID(arg)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			if len(ids) < 2 {
				latest, err := ListScanJobs(db, 2)
				if err != nil {
					return err
				}
				switch {
				case len(ids) == 1 && len(latest) > 0 && latest[0].ID == ids[0]:
					return fmt.Errorf("job %d is the most recent run; give an older job id, or two ids", ids[0])
				case len(ids) == 1 && len(latest) > 0:
					ids = append(ids, latest[0].ID)
				case len(ids) == 0 && len(latest) == 2:
					ids = []uint{latest[1].ID, latest[0].ID}
				default:
					return fmt.Errorf("jobs diff needs two recorded runs; found %d", len(latest))
				}
			}

			fromJob, fromResults, err := GetScanJob(db, ids[0])
			if err != nil {
				return err
			}
			toJob, toResults, err := GetScanJob(db, ids[1])
			if err != nil {
				return err
			}
			changes, unchanged := diffResults(fromResults, toResults)

			out := cmd.OutOrStdout()
			if jsonOutput {
				if changes == nil {
					changes = []resultChange{}
				}
				err = writeJSON(out, struct {
					From      jobJSON        `json:"from"`
					To        jobJSON        `json:"to"`
					Changes   []resultChange `json:"changes"`
					Unchanged int            `json:"unchanged"`
				}{newJobJSON(fromJob), newJobJSON(toJob), changes, unchanged})
			} else {
				err = writeDiffText(out, fromJob, toJob, changes, unchanged)
			}
			if err != nil {
				return err
			}

			if exitCode && len(changes) > 0 {
				return &ExitStatusError{Code: 1}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the comparison as JSON")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit with 1 when any result changed, like git diff --exit-code")

	return cmd
}

// writeDiffText prints a jobs diff: a summary line, then one line per change.
func writeDiffText(w io.Writer, from, to ScanJob, changes []resultChange, unchanged int) error {
	if _, err := fmt.Fprintf(w, "Job %d (%s) -> job %d (%s): %d changed, %d unchanged\n",
		from.ID, from.StartedAt.Format(time.RFC3339), to.ID, to.StartedAt.Format(time.RFC3339), len(changes), unchanged); err != nil {
		return err
	}
	for _, c := range changes {
		var label string
		switch c.Change {
		case changeAdded:
			label = "added " + string(c.To)
		case changeRemoved:
			label = "removed " + string(c.From)
		default:
			label = string(c.From) + " -> " + string(c.To)
		}
		// Like jobs show, the line names the check; messages of targeted
		// checks already name their target.
		if _, err := fmt.Fprintf(w, "[%s] %-17s %s\n", label, c.Key, c.Message); err != nil {
			return err
		}
	}
	return nil
}

func newJobsStatsCmd(openDB DatabaseOpener) *cobra.Command {
	var (
		since         string
		flapThreshold int
		jsonOutput    bool
	)

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Summarize check results over recent runs",
		Long: "Count PASS, WARN, and FAIL results per check (and target) over the runs that started within --since, " +
			"count status changes between consecutive runs, and flag checks that change status at least --flap-threshold times.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			age, err := parseAge(since)
			if err != nil {
				return fmt.Errorf("invalid --since value %q: %w", since, err)
			}
			if flapThreshold <= 0 {
				return fmt.Errorf("invalid --flap-threshold value %d: must be greater than 0", flapThreshold)
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			cutoff := time.Now().Add(-age)
			runs, stats, err := ScanStats(db, cutoff, flapThreshold)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if jsonOutput {
				return writeJSON(out, struct {
					Since  time.Time    `json:"since"`
					Runs   int          `json:"runs"`
					Checks []checkStats `json:"checks"`
				}{cutoff, runs, stats})
			}
			return writeStatsText(out, cutoff, runs, stats)
		},
	}

	cmd.Flags().StringVar(&since, "since", "7d", "summarize runs that started within this age, such as 7d or 12h")
	cmd.Flags().IntVar(&flapThreshold, "flap-threshold", 3, "status changes at which a check is flagged as flapping")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the statistics as JSON")

	return cmd
}

// writeStatsText prints jobs stats as an aligned table.
func writeStatsText(w io.Writer, since time.Time, runs int, stats []checkStats) error {
	if _, err := fmt.Fprintf(w, "Runs since %s: %d\n", since.Format(time.RFC3339), runs); err != nil {
		return err
	}
	if len(stats) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "CHECK\tTARGET\tRUNS\tPASS\tWARN\tFAIL\tCHANGES\tLAST\tFLAPPING"); err != nil {
		return err
	}
	for _, s := range stats {
		flapping := "no"
		if s.Flapping {
			flapping = "yes"
		}
		target := s.Target
		if target == "" {
			target = "-"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\t%s\n",
			s.Key, target, s.Runs, s.Pass, s.Warn, s.Fail, s.Changes, s.Last, flapping); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func newJobsPruneCmd(openDB DatabaseOpener) *cobra.Command {
	var (
		olderThan string
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete health check runs older than a given age",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if olderThan == "" {
				return errors.New("--older-than is required, for example --older-than 30d")
			}
			age, err := parseAge(olderThan)
			if err != nil {
				return fmt.Errorf("invalid --older-than value %q: %w", olderThan, err)
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			cutoff := time.Now().Add(-age)
			n, err := PruneScanJobs(db, cutoff, dryRun)
			if err != nil {
				return err
			}

			verb, noun := "Deleted", "jobs"
			if dryRun {
				verb = "Would delete"
			}
			if n == 1 {
				noun = "job"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s started before %s\n", verb, n, noun, cutoff.Format(time.RFC3339))
			return err
		},
	}

	cmd.Flags().StringVar(&olderThan, "older-than", "", "delete runs that started longer ago than this age, such as 30d, 12h, or 90m (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report how many runs would be deleted without deleting them")

	return cmd
}

// maxAgeDays keeps a day count within time.Duration's range (about 292 years).
const maxAgeDays = math.MaxInt64 / int64(24*time.Hour)

var errAgeFormat = errors.New("use a whole number of days such as 30d, or a duration such as 12h")

// parseAge parses a positive age: a whole number of days such as 30d, or a Go
// duration such as 12h or 90m.
func parseAge(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseInt(days, 10, 64)
		switch {
		case err != nil:
			return 0, errAgeFormat
		case n <= 0:
			return 0, errors.New("must be greater than 0")
		case n > maxAgeDays:
			return 0, fmt.Errorf("must be at most %dd", maxAgeDays)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}

	age, err := time.ParseDuration(s)
	if err != nil {
		return 0, errAgeFormat
	}
	if age <= 0 {
		return 0, errors.New("must be greater than 0")
	}
	return age, nil
}
