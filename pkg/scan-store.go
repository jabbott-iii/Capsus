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
	"fmt"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

// maxStoredMessageLen bounds each persisted result message in bytes, so
// verbose tool output cannot grow the database without limit (SEC-004).
const maxStoredMessageLen = 1024

// truncateMessage shortens msg to at most maxLen bytes on a UTF-8 boundary,
// marking the cut with "...".
func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	const marker = "..."
	cut := maxLen - len(marker)
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return msg[:cut] + marker
}

// ListFeatures returns every seeded Feature along with its Category, ordered by category then name.
func ListFeatures(db *Database) ([]Feature, error) {
	var features []Feature
	if err := db.Conn().Joins("Category").Order("features.category_id, features.name").Find(&features).Error; err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	return features, nil
}

// featureByKey looks up a Feature by its check key using conn, which may be a transaction.
func featureByKey(conn *gorm.DB, key string) (Feature, error) {
	var feature Feature
	err := conn.Where(Feature{Key: key}).First(&feature).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Feature{}, fmt.Errorf("feature %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return Feature{}, fmt.Errorf("look up feature %q: %w", key, err)
	}
	return feature, nil
}

// RecordScan persists a ScanJob and its ScanResults for the given check outcomes.
// startedAt is when the checks began running; a zero value means now.
func RecordScan(db *Database, startedAt time.Time, outcomes []CheckOutcome) (ScanJob, error) {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	job := ScanJob{
		StartedAt: startedAt,
		Status:    "running",
	}

	err := db.Conn().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return fmt.Errorf("create scan job: %w", err)
		}

		pass, warn, fail := 0, 0, 0
		features := map[string]Feature{} // targeted checks repeat a key
		for _, outcome := range outcomes {
			feature, ok := features[outcome.Key]
			if !ok {
				var err error
				if feature, err = featureByKey(tx, outcome.Key); err != nil {
					return err
				}
				features[outcome.Key] = feature
			}

			result := ScanResult{
				ScanJobID:  job.ID,
				FeatureID:  feature.ID,
				Key:        outcome.Key,
				Target:     truncateMessage(outcome.Target, maxStoredMessageLen),
				Status:     string(outcome.Status),
				Message:    truncateMessage(outcome.Message, maxStoredMessageLen),
				Value:      outcome.Value,
				Unit:       outcome.Unit,
				DurationMs: outcome.Duration.Milliseconds(),
				CreatedAt:  time.Now(),
			}
			if err := tx.Create(&result).Error; err != nil {
				return fmt.Errorf("create scan result for %q: %w", outcome.Key, err)
			}

			switch outcome.Status {
			case StatusPass:
				pass++
			case StatusWarn:
				warn++
			case StatusFail:
				fail++
			}
		}

		finished := time.Now()
		job.FinishedAt = &finished
		job.Status = "completed"
		job.Summary = fmt.Sprintf("%d pass, %d warn, %d fail", pass, warn, fail)

		return tx.Save(&job).Error
	})
	if err != nil {
		return ScanJob{}, err
	}

	return job, nil
}

// ListScanJobs returns the most recent scan jobs, newest first, limited to limit rows
// (or all rows when limit <= 0). Jobs are ordered by the instant they
// started (see startedBefore for why the stored text does not sort).
func ListScanJobs(db *Database, limit int) ([]ScanJob, error) {
	query := db.Conn().Order("julianday(started_at) DESC, id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var jobs []ScanJob
	if err := query.Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("list scan jobs: %w", err)
	}
	return jobs, nil
}

// startedBefore selects scan jobs that started before the given time.
// julianday compares instants, so rows written under a different UTC offset
// (for example across a daylight saving change) still compare correctly; the
// stored text itself only sorts correctly within one offset.
const startedBefore = "julianday(started_at) < julianday(?)"

// PruneScanJobs deletes the scan jobs that started before cutoff, together
// with their results, in one transaction, and returns how many jobs matched.
// With dryRun it only counts them.
func PruneScanJobs(db *Database, cutoff time.Time, dryRun bool) (int64, error) {
	var pruned int64
	err := db.Conn().Transaction(func(tx *gorm.DB) error {
		if dryRun {
			return tx.Model(&ScanJob{}).Where(startedBefore, cutoff).Count(&pruned).Error
		}

		// Results carry no foreign key constraint, so they are deleted
		// explicitly. A subquery avoids SQLite's limit on bound variables.
		oldJobs := tx.Model(&ScanJob{}).Select("id").Where(startedBefore, cutoff)
		if err := tx.Where("scan_job_id IN (?)", oldJobs).Delete(&ScanResult{}).Error; err != nil {
			return fmt.Errorf("delete scan results: %w", err)
		}
		deleted := tx.Where(startedBefore, cutoff).Delete(&ScanJob{})
		if deleted.Error != nil {
			return fmt.Errorf("delete scan jobs: %w", deleted.Error)
		}
		pruned = deleted.RowsAffected
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("prune scan jobs: %w", err)
	}
	return pruned, nil
}

// GetScanJob returns a single scan job and its results by ID.
func GetScanJob(db *Database, id uint) (ScanJob, []ScanResult, error) {
	var job ScanJob
	if err := db.Conn().First(&job, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ScanJob{}, nil, fmt.Errorf("scan job %d: %w", id, ErrNotFound)
		}
		return ScanJob{}, nil, fmt.Errorf("get scan job %d: %w", id, err)
	}

	var results []ScanResult
	if err := db.Conn().Where(ScanResult{ScanJobID: id}).Order("id").Find(&results).Error; err != nil {
		return ScanJob{}, nil, fmt.Errorf("list results for scan job %d: %w", id, err)
	}

	return job, results, nil
}
