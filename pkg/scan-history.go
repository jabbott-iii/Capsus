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
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Kinds of resultChange.
const (
	changeWorse   = "worse"
	changeBetter  = "better"
	changeAdded   = "added"
	changeRemoved = "removed"
)

// resultID identifies one check result across runs: a check key and, for
// targeted checks, the target. Rows stored before v1.1.0 have no target.
type resultID struct {
	key, target string
}

// resultChange is one difference between two recorded runs.
type resultChange struct {
	Key    string      `json:"key"`
	Target string      `json:"target,omitempty"`
	Change string      `json:"change"`
	From   CheckStatus `json:"from,omitempty"`
	To     CheckStatus `json:"to,omitempty"`
	// Message is from the newer run, or from the older run for a removed
	// result.
	Message string `json:"message"`
}

// statusRank orders statuses from healthiest to least healthy.
func statusRank(s CheckStatus) int {
	return ExitCodeFor(s)
}

// legacyMatch reports whether r, stored before v1.1.0 without a target, is
// the same result as one with target. Those messages already named their
// target ("/var: 45.0% used", `service "nginx" is active`), unlike a result
// for something else that now has a target (host uptime versus a service).
func legacyMatch(r ScanResult, target string) bool {
	return r.Target == "" && target != "" && strings.Contains(r.Message, target)
}

// alignLegacyTargets gives a result stored without a target the target of
// its counterpart in the other run, when the check has exactly one result in
// each run and legacyMatch holds. Without it, the first comparison across an
// upgrade would report every targeted check as removed and added.
func alignLegacyTargets(from, to []ScanResult) ([]ScanResult, []ScanResult) {
	from, to = slices.Clone(from), slices.Clone(to)
	byKey := func(results []ScanResult) map[string][]int {
		indexes := map[string][]int{}
		for i, r := range results {
			indexes[r.Key] = append(indexes[r.Key], i)
		}
		return indexes
	}
	toKeys := byKey(to)
	for key, fromIndexes := range byKey(from) {
		toIndexes := toKeys[key]
		if len(fromIndexes) != 1 || len(toIndexes) != 1 {
			continue
		}
		f, t := &from[fromIndexes[0]], &to[toIndexes[0]]
		switch {
		case legacyMatch(*f, t.Target):
			f.Target = t.Target
		case legacyMatch(*t, f.Target):
			t.Target = f.Target
		}
	}
	return from, to
}

// diffResults compares the results of two runs by (key, target). Changes
// follow the order of the newer run, then removed results in the order of
// the older run. unchanged counts results with the same status in both.
func diffResults(from, to []ScanResult) (changes []resultChange, unchanged int) {
	from, to = alignLegacyTargets(from, to)
	before := make(map[resultID]CheckOutcome, len(from))
	for _, r := range from {
		before[resultID{r.Key, r.Target}] = resultOutcome(r)
	}

	seen := make(map[resultID]bool, len(to))
	for _, r := range to {
		id := resultID{r.Key, r.Target}
		seen[id] = true
		now := resultOutcome(r)
		old, existed := before[id]
		switch {
		case !existed:
			changes = append(changes, resultChange{Key: now.Key, Target: now.Target, Change: changeAdded, To: now.Status, Message: now.Message})
		case old.Status == now.Status:
			unchanged++
		default:
			kind := changeBetter
			if statusRank(now.Status) > statusRank(old.Status) {
				kind = changeWorse
			}
			changes = append(changes, resultChange{Key: now.Key, Target: now.Target, Change: kind, From: old.Status, To: now.Status, Message: now.Message})
		}
	}
	for _, r := range from {
		id := resultID{r.Key, r.Target}
		if seen[id] {
			continue
		}
		seen[id] = true // a repeated row is reported once
		old := resultOutcome(r)
		changes = append(changes, resultChange{Key: old.Key, Target: old.Target, Change: changeRemoved, From: old.Status, Message: old.Message})
	}
	return changes, unchanged
}

// checkStats summarizes one check result over a period.
type checkStats struct {
	Key    string `json:"key"`
	Target string `json:"target,omitempty"`
	Runs   int    `json:"runs"`
	Pass   int    `json:"pass"`
	Warn   int    `json:"warn"`
	Fail   int    `json:"fail"`
	// Changes counts status changes between consecutive runs that include
	// this result.
	Changes  int         `json:"changes"`
	Last     CheckStatus `json:"last_status"`
	Flapping bool        `json:"flapping"`
}

// startedSince selects scan jobs that started at or after the given time;
// see startedBefore for why julianday is used.
const startedSince = "julianday(started_at) >= julianday(?)"

// ScanStats returns how many runs started at or after since, and statistics
// per (key, target) over those runs, sorted by key and target. A result is
// flapping when its status changed at least flapThreshold times. Results
// stored before v1.1.0 without a target count toward the only target their
// check has in the period, when legacyMatch holds.
func ScanStats(db *Database, since time.Time, flapThreshold int) (int, []checkStats, error) {
	var jobs []ScanJob
	if err := db.Conn().Where(startedSince, since).Order("julianday(started_at), id").Find(&jobs).Error; err != nil {
		return 0, nil, fmt.Errorf("list scan jobs: %w", err)
	}
	if len(jobs) == 0 {
		return 0, []checkStats{}, nil
	}

	var results []ScanResult
	jobIDs := db.Conn().Model(&ScanJob{}).Select("id").Where(startedSince, since)
	if err := db.Conn().Where("scan_job_id IN (?)", jobIDs).Order("id").Find(&results).Error; err != nil {
		return 0, nil, fmt.Errorf("list scan results: %w", err)
	}
	byJob := make(map[uint][]ScanResult, len(jobs))
	targets := map[string]map[string]bool{} // key → targets seen in the period
	for _, r := range results {
		byJob[r.ScanJobID] = append(byJob[r.ScanJobID], r)
		if r.Target != "" {
			if targets[r.Key] == nil {
				targets[r.Key] = map[string]bool{}
			}
			targets[r.Key][r.Target] = true
		}
	}
	targetOf := func(r ScanResult) string {
		if r.Target == "" && len(targets[r.Key]) == 1 {
			for target := range targets[r.Key] {
				if legacyMatch(r, target) {
					return target
				}
			}
		}
		return r.Target
	}

	stats := map[resultID]*checkStats{}
	for _, job := range jobs {
		for _, r := range byJob[job.ID] {
			r.Target = targetOf(r)
			o := resultOutcome(r)
			id := resultID{o.Key, o.Target}
			s, ok := stats[id]
			if !ok {
				s = &checkStats{Key: o.Key, Target: o.Target}
				stats[id] = s
			}
			if s.Runs > 0 && s.Last != o.Status {
				s.Changes++
			}
			s.Runs++
			switch o.Status {
			case StatusPass:
				s.Pass++
			case StatusWarn:
				s.Warn++
			case StatusFail:
				s.Fail++
			}
			s.Last = o.Status
		}
	}

	list := make([]checkStats, 0, len(stats))
	for _, s := range stats {
		s.Flapping = s.Changes >= flapThreshold
		list = append(list, *s)
	}
	slices.SortFunc(list, func(a, b checkStats) int {
		return cmp.Or(cmp.Compare(a.Key, b.Key), cmp.Compare(a.Target, b.Target))
	})
	return len(jobs), list, nil
}
