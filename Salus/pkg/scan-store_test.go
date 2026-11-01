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
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newSeededTestDatabase(t *testing.T) *Database {
	t.Helper()

	db := newTestDatabase(t)
	if err := EnsureDefaultFeatures(db); err != nil {
		t.Fatalf("EnsureDefaultFeatures() error = %v", err)
	}
	return db
}

func TestRecordScanPersistsJobAndResults(t *testing.T) {
	db := newSeededTestDatabase(t)

	outcomes := []CheckOutcome{
		{Key: keyDiskSpace, Status: StatusPass, Message: "fine", Duration: 5 * time.Millisecond},
		{Key: keyMemory, Status: StatusWarn, Message: "getting full", Duration: 10 * time.Millisecond},
	}

	job, err := RecordScan(db, time.Time{}, outcomes)
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	if job.ID == 0 {
		t.Fatal("RecordScan() returned job with zero ID")
	}
	if job.Status != "completed" {
		t.Errorf("job.Status = %q, want %q", job.Status, "completed")
	}
	if job.Summary != "1 pass, 1 warn, 0 fail" {
		t.Errorf("job.Summary = %q, want %q", job.Summary, "1 pass, 1 warn, 0 fail")
	}

	gotJob, results, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}
	if gotJob.ID != job.ID {
		t.Errorf("GetScanJob() ID = %d, want %d", gotJob.ID, job.ID)
	}
	if len(results) != len(outcomes) {
		t.Fatalf("GetScanJob() returned %d results, want %d", len(results), len(outcomes))
	}
	if results[0].Key != keyDiskSpace || results[0].Status != string(StatusPass) {
		t.Errorf("results[0] = %+v, want key %q status %q", results[0], keyDiskSpace, StatusPass)
	}
}

func TestRecordScanUnknownFeatureFails(t *testing.T) {
	db := newSeededTestDatabase(t)

	outcomes := []CheckOutcome{{Key: "not-a-real-check", Status: StatusPass}}
	if _, err := RecordScan(db, time.Time{}, outcomes); err == nil {
		t.Fatal("RecordScan() expected error for unknown feature key, got nil")
	}
}

func TestGetScanJobNotFound(t *testing.T) {
	db := newSeededTestDatabase(t)

	if _, _, err := GetScanJob(db, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetScanJob() error = %v, want ErrNotFound", err)
	}
}

func TestListScanJobsOrdersNewestFirst(t *testing.T) {
	db := newSeededTestDatabase(t)

	first, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	second, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}

	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("ListScanJobs() returned %d jobs, want 2", len(jobs))
	}
	if jobs[0].ID != second.ID || jobs[1].ID != first.ID {
		t.Errorf("ListScanJobs() order = [%d, %d], want [%d, %d]", jobs[0].ID, jobs[1].ID, second.ID, first.ID)
	}

	limited, err := ListScanJobs(db, 1)
	if err != nil {
		t.Fatalf("ListScanJobs(limit=1) error = %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("ListScanJobs(limit=1) returned %d jobs, want 1", len(limited))
	}
}

func TestRecordScanBoundsStoredMessages(t *testing.T) {
	db := newSeededTestDatabase(t)
	long := strings.Repeat("é", maxStoredMessageLen) // 2 bytes per rune

	job, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusWarn, Message: long}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	_, results, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}

	got := results[0].Message
	if len(got) > maxStoredMessageLen || !utf8.ValidString(got) || !strings.HasSuffix(got, "...") {
		t.Errorf("stored message: %d bytes, valid UTF-8 %v, suffix %q; want <= %d bytes, valid, ending in ...",
			len(got), utf8.ValidString(got), got[max(0, len(got)-3):], maxStoredMessageLen)
	}
}

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		msg  string
		max  int
		want string
	}{
		{msg: "short", max: 10, want: "short"},
		{msg: "exactly10!", max: 10, want: "exactly10!"},
		{msg: "0123456789abc", max: 10, want: "0123456..."},
		{msg: "ééééé", max: 8, want: "éé..."}, // never splits a multi-byte rune
	}

	for _, tt := range tests {
		if got := truncateMessage(tt.msg, tt.max); got != tt.want {
			t.Errorf("truncateMessage(%q, %d) = %q, want %q", tt.msg, tt.max, got, tt.want)
		}
	}
}

func TestPruneScanJobs(t *testing.T) {
	db := newSeededTestDatabase(t)
	cutoff := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	record := func(startedAt time.Time) uint {
		t.Helper()
		job, err := RecordScan(db, startedAt, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
		if err != nil {
			t.Fatalf("RecordScan() error = %v", err)
		}
		return job.ID
	}

	old := record(cutoff.Add(-30 * 24 * time.Hour))
	// Stored times keep their UTC offset, and the text sorts by wall clock, so
	// a plain string comparison would get these two wrong: 20:00+14:00 is
	// 06:00 UTC (before the cutoff), and 08:00-08:00 is 16:00 UTC (after it).
	east := record(time.Date(2026, 9, 1, 20, 0, 0, 0, time.FixedZone("UTC+14", 14*3600)))
	west := record(time.Date(2026, 9, 1, 8, 0, 0, 0, time.FixedZone("UTC-8", -8*3600)))
	recent := record(cutoff.Add(time.Hour))

	countResults := func(ids ...uint) int64 {
		t.Helper()
		var n int64
		if err := db.Conn().Model(&ScanResult{}).Where("scan_job_id IN ?", ids).Count(&n).Error; err != nil {
			t.Fatalf("count results: %v", err)
		}
		return n
	}
	jobIDs := func() []uint {
		t.Helper()
		jobs, err := ListScanJobs(db, 0)
		if err != nil {
			t.Fatalf("ListScanJobs() error = %v", err)
		}
		var ids []uint
		for _, job := range jobs {
			ids = append(ids, job.ID)
		}
		return ids
	}

	n, err := PruneScanJobs(db, cutoff, true)
	if err != nil || n != 2 {
		t.Fatalf("PruneScanJobs(dry run) = %d, %v, want 2, nil", n, err)
	}
	if ids := jobIDs(); len(ids) != 4 {
		t.Fatalf("dry run left jobs %v, want all 4", ids)
	}

	n, err = PruneScanJobs(db, cutoff, false)
	if err != nil || n != 2 {
		t.Fatalf("PruneScanJobs() = %d, %v, want 2, nil", n, err)
	}
	remaining := map[uint]bool{}
	for _, id := range jobIDs() {
		remaining[id] = true
	}
	if len(remaining) != 2 || !remaining[west] || !remaining[recent] {
		t.Errorf("remaining jobs = %v, want only %d and %d", remaining, west, recent)
	}
	if n := countResults(old, east); n != 0 {
		t.Errorf("pruned jobs still have %d results", n)
	}
	if n := countResults(west, recent); n != 2 {
		t.Errorf("kept jobs have %d results, want 2", n)
	}

	if n, err := PruneScanJobs(db, cutoff, false); err != nil || n != 0 {
		t.Errorf("second PruneScanJobs() = %d, %v, want 0, nil", n, err)
	}
}

func TestRecordScanPersistsTargetsAndValues(t *testing.T) {
	db := newSeededTestDatabase(t)
	outcomes := []CheckOutcome{
		CheckOutcome{Key: keyDiskSpace, Target: "/", Status: StatusPass, Message: "/: 10.0% used"}.withValue(10, unitPercent),
		CheckOutcome{Key: keyDiskSpace, Target: "/var", Status: StatusWarn, Message: "/var: 85.5% used"}.withValue(85.5, unitPercent),
		{Key: keyTimeSync, Status: StatusPass, Message: "system clock is synchronized"},
		CheckOutcome{Key: keySystemdFailed, Status: StatusPass, Message: "no failed systemd units"}.withValue(0, unitCount),
	}
	job, err := RecordScan(db, time.Now(), outcomes)
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	if job.Summary != "3 pass, 1 warn, 0 fail" {
		t.Errorf("job.Summary = %q", job.Summary)
	}

	_, results, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}
	if len(results) != len(outcomes) {
		t.Fatalf("got %d results, want %d", len(results), len(outcomes))
	}
	for i, r := range results {
		got := resultOutcome(r)
		want := outcomes[i]
		if got.Key != want.Key || got.Target != want.Target || got.Unit != want.Unit || !sameValue(got.Value, want.Value) {
			t.Errorf("result %d = %+v (value %v), want %+v (value %v)", i, got, deref(got.Value), want, deref(want.Value))
		}
	}
}

func sameValue(a, b *float64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// The v1.0.2 schema, before Target, Value, and Unit existed. The Feature
// relation is part of it: it gives the table its foreign key, so the upgrade
// migrates with ALTER TABLE ADD COLUMN, as a real v1.0.2 database does.
type scanResultV102 struct {
	ID         uint `gorm:"primaryKey"`
	ScanJobID  uint `gorm:"not null;index"`
	FeatureID  uint `gorm:"not null;index"`
	Feature    Feature
	Key        string `gorm:"not null"`
	Status     string `gorm:"not null"`
	Message    string
	DurationMs int64
	CreatedAt  time.Time
}

func (scanResultV102) TableName() string { return "scan_results" }

func TestOpenDatabaseMigratesV102Results(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salus.db")
	old, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := old.AutoMigrate(&FeatureCategory{}, &Feature{}, &ScanJob{}, &scanResultV102{}); err != nil {
		t.Fatalf("create v1.0.2 schema: %v", err)
	}
	job := ScanJob{StartedAt: time.Now(), Status: "completed", Summary: "1 pass, 0 warn, 0 fail"}
	if err := old.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := old.Create(&scanResultV102{ScanJobID: job.ID, FeatureID: 1, Key: keyDiskSpace, Status: "PASS", Message: "/: 1.0% used"}).Error; err != nil {
		t.Fatalf("create result: %v", err)
	}
	sqlDB, err := old.DB()
	if err != nil {
		t.Fatalf("DB(): %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() on a v1.0.2 database error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	_, results, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}
	if len(results) != 1 || results[0].Target != "" || results[0].Value != nil || results[0].Unit != "" || results[0].Message != "/: 1.0% used" {
		t.Errorf("migrated result = %+v, want the old row with an empty target and unit and no value", results)
	}

	// New rows, including new check keys, are recorded after the migration.
	outcome := CheckOutcome{Key: keyDiskInodes, Target: "/", Status: StatusPass, Message: "ok"}.withValue(3, unitPercent)
	if _, err := RecordScan(db, time.Now(), []CheckOutcome{outcome}); err != nil {
		t.Fatalf("RecordScan() after migration error = %v", err)
	}
}
