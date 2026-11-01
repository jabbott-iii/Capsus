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
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func newTestDatabase(t *testing.T) *Database {
	t.Helper()

	db, err := NewDatabase(filepath.Join(t.TempDir(), "salus_test.db"))
	if err != nil {
		t.Fatalf("NewDatabase() error = %v", err)
	}
	// Registered after t.TempDir, so it runs first: Windows cannot remove an open file.
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return db
}

func TestNewDatabaseCreatesSchema(t *testing.T) {
	db := newTestDatabase(t)

	if err := db.Conn().AutoMigrate(&FeatureCategory{}, &Feature{}, &ScanJob{}, &ScanResult{}); err != nil {
		t.Fatalf("expected schema already migrated, got error: %v", err)
	}
}

func TestEnsureDefaultFeaturesSeedsCatalog(t *testing.T) {
	db := newTestDatabase(t)

	if err := EnsureDefaultFeatures(db); err != nil {
		t.Fatalf("EnsureDefaultFeatures() error = %v", err)
	}

	features, err := ListFeatures(db)
	if err != nil {
		t.Fatalf("ListFeatures() error = %v", err)
	}
	if len(features) != len(defaultFeatures) {
		t.Fatalf("ListFeatures() returned %d features, want %d", len(features), len(defaultFeatures))
	}

	// Seeding twice must be idempotent.
	if err := EnsureDefaultFeatures(db); err != nil {
		t.Fatalf("EnsureDefaultFeatures() second call error = %v", err)
	}
	features, err = ListFeatures(db)
	if err != nil {
		t.Fatalf("ListFeatures() error = %v", err)
	}
	if len(features) != len(defaultFeatures) {
		t.Fatalf("ListFeatures() after re-seed returned %d features, want %d", len(features), len(defaultFeatures))
	}
}

func TestWithConnectionDefaults(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "salus.db", want: "salus.db?_busy_timeout=5000&_txlock=immediate"},
		{path: "/data/salus.db?", want: "/data/salus.db?_busy_timeout=5000&_txlock=immediate"},
		{path: "/data/salus.db?cache=shared", want: "/data/salus.db?cache=shared&_busy_timeout=5000&_txlock=immediate"},
		{path: "/data/salus.db?_busy_timeout=1000", want: "/data/salus.db?_busy_timeout=1000&_txlock=immediate"},
		{path: "/data/salus.db?_timeout=1000", want: "/data/salus.db?_timeout=1000&_txlock=immediate"},
		{path: "/data/salus.db?_txlock=deferred", want: "/data/salus.db?_txlock=deferred&_busy_timeout=5000"},
		{path: "/data/salus.db?_txlock=exclusive&_busy_timeout=0", want: "/data/salus.db?_txlock=exclusive&_busy_timeout=0"},
		{path: "file:salus.db?mode=ro", want: "file:salus.db?mode=ro&_busy_timeout=5000&_txlock=immediate"},
		{path: ":memory:", want: ":memory:?_busy_timeout=5000&_txlock=immediate"},
		{path: "?odd", want: "?odd"},                 // go-sqlite3 splits only at index 1 or later
		{path: "salus.db?%zz", want: "salus.db?%zz"}, // malformed; go-sqlite3 reports it
	}

	for _, tt := range tests {
		if got := withConnectionDefaults(tt.path); got != tt.want {
			t.Errorf("withConnectionDefaults(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestOpenDatabaseConcurrentFirstOpen(t *testing.T) {
	// Several processes opening a new database at once (two cron entries in
	// the same minute) must all succeed. Each OpenDatabase has its own
	// connection pool, so goroutines contend for SQLite locks like processes.
	const openers = 8
	for round := range 3 {
		path := filepath.Join(t.TempDir(), "salus.db")
		dbs := make([]*Database, openers)
		errs := make([]error, openers)
		var wg sync.WaitGroup
		for i := range openers {
			wg.Go(func() { dbs[i], errs[i] = OpenDatabase(path) })
		}
		wg.Wait()
		for i := range openers {
			if errs[i] != nil {
				t.Errorf("round %d: OpenDatabase() #%d error = %v", round, i, errs[i])
				continue
			}
			if err := dbs[i].Close(); err != nil {
				t.Errorf("round %d: Close() #%d error = %v", round, i, err)
			}
		}

		db := openTestDatabaseAt(t, path)
		features, err := ListFeatures(db)
		if err != nil {
			t.Fatalf("ListFeatures() error = %v", err)
		}
		if len(features) != len(defaultFeatures) {
			t.Errorf("round %d: %d features stored, want %d", round, len(features), len(defaultFeatures))
		}
	}
}

func TestNewDatabaseRecordsSchemaVersion(t *testing.T) {
	db := newTestDatabase(t)

	got, err := userVersion(db.Conn())
	if err != nil {
		t.Fatalf("userVersion() error = %v", err)
	}
	if got != schemaVersion {
		t.Errorf("user_version = %d, want %d", got, schemaVersion)
	}
}

func TestOpenDatabaseMigratesUnversionedDatabase(t *testing.T) {
	// Databases from before schema versioning have user_version 0.
	path := filepath.Join(t.TempDir(), "salus.db")
	db := openTestDatabaseAt(t, path)
	setUserVersion(t, db.Conn(), 0)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db = openTestDatabaseAt(t, path)
	if got, err := userVersion(db.Conn()); err != nil || got != schemaVersion {
		t.Errorf("user_version = %d, %v; want %d", got, err, schemaVersion)
	}
}

func TestOpenDatabaseLeavesNewerSchemaVersion(t *testing.T) {
	// After a downgrade, an older Salus opens a newer database as it is.
	path := filepath.Join(t.TempDir(), "salus.db")
	db := openTestDatabaseAt(t, path)
	setUserVersion(t, db.Conn(), schemaVersion+1)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db = openTestDatabaseAt(t, path)
	if got, err := userVersion(db.Conn()); err != nil || got != schemaVersion+1 {
		t.Errorf("user_version = %d, %v; want %d", got, err, schemaVersion+1)
	}
	if _, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}}); err != nil {
		t.Errorf("RecordScan() error = %v", err)
	}
}

func TestOpenDatabaseReadOnlyUnversionedDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only files block t.TempDir cleanup on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write to read-only files")
	}
	// A current but unversioned database (written by a v1.1.0 development
	// build) cannot record its version when read-only, and must still open.
	path := filepath.Join(t.TempDir(), "salus.db")
	db := openTestDatabaseAt(t, path)
	setUserVersion(t, db.Conn(), 0)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	db = openTestDatabaseAt(t, path)
	if got, err := userVersion(db.Conn()); err != nil || got != 0 {
		t.Errorf("user_version = %d, %v; want 0 (unchanged)", got, err)
	}
}

func TestSeedCatalogToleratesConcurrentInsert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salus.db")
	db := newTestDatabaseAt(t, path)
	other := newTestDatabaseAt(t, path)

	// Just before Salus inserts the first category, another connection
	// inserts it, as a concurrent process would between the lookup and the
	// insert.
	first := defaultCategories[0]
	inserted := false
	err := db.Conn().Callback().Create().Before("gorm:begin_transaction").Register("test:concurrent_insert", func(tx *gorm.DB) {
		if inserted {
			return
		}
		inserted = true
		if err := other.Conn().Create(&FeatureCategory{Name: first.Name, Description: "inserted concurrently"}).Error; err != nil {
			t.Errorf("concurrent insert error = %v", err)
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}

	if err := EnsureDefaultFeatures(db); err != nil {
		t.Fatalf("EnsureDefaultFeatures() error = %v", err)
	}
	if !inserted {
		t.Fatal("concurrent insert did not run")
	}

	var category FeatureCategory
	if err := db.Conn().Where("name = ?", first.Name).First(&category).Error; err != nil {
		t.Fatalf("load category: %v", err)
	}
	if category.Description != "inserted concurrently" {
		t.Errorf("category description = %q, want the concurrently inserted row", category.Description)
	}
	features, err := ListFeatures(db)
	if err != nil {
		t.Fatalf("ListFeatures() error = %v", err)
	}
	if len(features) != len(defaultFeatures) {
		t.Fatalf("%d features stored, want %d", len(features), len(defaultFeatures))
	}
	for _, f := range features {
		if f.Category.Name == first.Name && f.CategoryID != category.ID {
			t.Errorf("feature %q has category %d, want %d", f.Key, f.CategoryID, category.ID)
		}
	}
}

// openTestDatabaseAt opens (and seeds) the database at path and closes it
// when the test ends.
func openTestDatabaseAt(t *testing.T, path string) *Database {
	t.Helper()

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newTestDatabaseAt opens the unseeded database at path and closes it when
// the test ends.
func newTestDatabaseAt(t *testing.T, path string) *Database {
	t.Helper()

	db, err := NewDatabase(path)
	if err != nil {
		t.Fatalf("NewDatabase() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func setUserVersion(t *testing.T, conn *gorm.DB, version int) {
	t.Helper()

	if err := conn.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)).Error; err != nil {
		t.Fatalf("set user_version: %v", err)
	}
}

// wantSchema is the schema that schemaVersion describes: each table and index
// with its columns, from PRAGMA table_info and index_info. A database at
// schemaVersion is not migrated again (prepareDatabase), so when a model
// change makes this test fail, bump schemaVersion and then update wantSchema.
var wantSchema = []string{
	"feature_categories: id name description",
	"features: id category_id key name description",
	"idx_feature_categories_name: name",
	"idx_features_category_id: category_id",
	"idx_features_key: key",
	"idx_scan_results_feature_id: feature_id",
	"idx_scan_results_scan_job_id: scan_job_id",
	"scan_jobs: id started_at finished_at status summary",
	"scan_results: id scan_job_id feature_id key target status message value unit duration_ms created_at",
}

func TestSchemaMatchesSchemaVersion(t *testing.T) {
	db := newTestDatabase(t)
	conn := db.Conn()

	var objects []struct{ Type, Name string }
	if err := conn.Raw("SELECT type, name FROM sqlite_master WHERE type IN ('table', 'index') AND name NOT LIKE 'sqlite_%' ORDER BY name").Scan(&objects).Error; err != nil {
		t.Fatalf("list schema objects: %v", err)
	}
	var got []string
	for _, obj := range objects {
		pragma := "table_info"
		if obj.Type == "index" {
			pragma = "index_info"
		}
		var columns []string
		if err := conn.Raw(fmt.Sprintf("SELECT name FROM pragma_%s(?)", pragma), obj.Name).Scan(&columns).Error; err != nil {
			t.Fatalf("columns of %s: %v", obj.Name, err)
		}
		got = append(got, obj.Name+": "+strings.Join(columns, " "))
	}
	if !slices.Equal(got, wantSchema) {
		t.Errorf("schema = %q\nwant %q\n(after a model change, bump schemaVersion, then update wantSchema)", got, wantSchema)
	}
}
