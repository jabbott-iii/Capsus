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
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//--------------------------------------------------core-------------------------------------------------------------------------------------------------//

// Database owns the gorm connection for internal data access.
type Database struct {
	conn *gorm.DB
}

// schemaVersion is the schema this build migrates to, stored in SQLite's
// PRAGMA user_version. Bump it whenever a model struct below changes, so that
// existing databases are migrated once. Version 0 is every database created
// before schema versioning (v1.1.0); 1 is the v1.1.0 schema (M6 columns).
const schemaVersion = 1

// NewDatabase opens (or creates) the sqlite file and runs schema migrations.
// A new file and any missing parent directories are created owner-only.
func NewDatabase(path string) (*Database, error) {
	return openDatabase(path, false)
}

// OpenDatabase opens the database at path, runs schema migrations, and seeds
// the built-in check catalog.
func OpenDatabase(path string) (*Database, error) {
	return openDatabase(path, true)
}

func openDatabase(path string, seed bool) (*Database, error) {
	if path == "" {
		return nil, errors.New("database path is empty")
	}
	if err := createDatabaseFile(path); err != nil {
		return nil, err
	}

	// GORM's default logger writes to stdout (corrupting --json output) and
	// logs "record not found" for normal lookups. Errors are returned to
	// callers and reported by the CLI instead.
	conn, err := gorm.Open(sqlite.Open(withConnectionDefaults(path)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %s: %w", path, err)
	}
	db := &Database{conn: conn}
	if err := prepareDatabase(conn, seed); err != nil {
		return nil, errors.Join(fmt.Errorf("prepare database %s: %w", path, err), db.Close())
	}
	return db, nil
}

// withConnectionDefaults adds Salus's connection parameters to path unless
// it already sets them (go-sqlite3 reads parameters after the first "?" of
// the path, at index 1 or later). _busy_timeout makes a connection wait up to
// five seconds for another process's lock (go-sqlite3's default, made
// explicit). _txlock=immediate takes the write lock when a transaction
// begins, so concurrent runs queue for it instead of failing when a read lock
// cannot be upgraded. On a read-only database SQLite starts such a
// transaction without the write lock, so commands that only read keep
// working there (jobs prune --dry-run reads inside a transaction).
func withConnectionDefaults(path string) string {
	sep, query := "?", ""
	if i := strings.IndexByte(path, '?'); i == 0 {
		return path // go-sqlite3 treats the whole path as a file name
	} else if i > 0 {
		sep, query = "&", path[i+1:]
		if query == "" {
			sep = ""
		}
	}
	params, err := url.ParseQuery(query)
	if err != nil {
		return path // go-sqlite3 reports the malformed parameters
	}
	var add []string
	if !params.Has("_busy_timeout") && !params.Has("_timeout") {
		add = append(add, "_busy_timeout=5000")
	}
	if !params.Has("_txlock") {
		add = append(add, "_txlock=immediate")
	}
	if len(add) == 0 {
		return path
	}
	return path + sep + strings.Join(add, "&")
}

// prepareDatabase brings the schema to schemaVersion and, when seed is set,
// seeds the check catalog. A database that is already current is only read,
// so concurrent opens do not contend and read-only databases keep working.
//
// Otherwise migration, seeding, and the version update run in one
// transaction, which _txlock=immediate starts under the write lock: two
// processes opening a new database at the same time take turns, and the
// second finds the work done. GORM's AutoMigrate checks for each table and
// column before creating it, so without the lock both could try to create
// the same table.
//
// A database newer than schemaVersion (opened by an older Salus after a
// downgrade) is used as it is: the schema only ever gains columns with
// defaults (intel/maint.md section 4), so older code can still use it.
func prepareDatabase(conn *gorm.DB, seed bool) error {
	if version, err := userVersion(conn); err == nil && version >= schemaVersion {
		if !seed {
			return nil
		}
		if complete, err := catalogComplete(conn); err == nil && complete {
			return nil
		}
	}

	txErr := conn.Transaction(func(tx *gorm.DB) error {
		version, err := userVersion(tx) // re-read under the write lock
		if err != nil {
			return err
		}
		if version < schemaVersion {
			if err := migrateSchema(tx); err != nil {
				return err
			}
		}
		if seed {
			if err := seedCatalog(tx); err != nil {
				return fmt.Errorf("seed default features: %w", err)
			}
		}
		if version < schemaVersion {
			// PRAGMA does not take bound parameters; schemaVersion is a constant.
			if err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)).Error; err != nil {
				return fmt.Errorf("set schema version: %w", err)
			}
		}
		return nil
	})
	if txErr == nil {
		return nil
	}

	// The transaction needs write access. A read-only database (or one in a
	// read-only directory) whose schema and catalog are already current, but
	// whose version was never recorded, must still open: repeat the work
	// without the transaction, which only reads when nothing is missing.
	// If that fails too, its error describes the problem: it is the same work.
	if err := migrateSchema(conn); err != nil {
		return err
	}
	if seed {
		if err := seedCatalog(conn); err != nil {
			return fmt.Errorf("seed default features: %w", err)
		}
	}
	return nil
}

func migrateSchema(conn *gorm.DB) error {
	if err := conn.AutoMigrate(
		&FeatureCategory{},
		&Feature{},
		&ScanJob{},
		&ScanResult{},
	); err != nil {
		return fmt.Errorf("auto-migrate schema: %w", err)
	}
	return nil
}

// userVersion returns the database's PRAGMA user_version (0 for a new file).
func userVersion(conn *gorm.DB) (int, error) {
	var version int
	if err := conn.Raw("PRAGMA user_version").Row().Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// createDatabaseFile creates a missing database file (mode 0600) and its
// missing parent directories (mode 0700) before SQLite opens it, so a new
// database is never readable by other users (SEC-004). An existing file is not
// touched: it keeps its mode (the misconfig check reports on it) and may be
// read-only. In-memory databases and "file:" URIs are left to SQLite.
func createDatabaseFile(path string) error {
	file, ok := databaseFile(path)
	if !ok {
		return nil
	}
	if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
		// It exists, or cannot be inspected; SQLite reports any real problem.
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil // created concurrently by another process
	}
	if err != nil {
		return fmt.Errorf("create database file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("create database file: %w", err)
	}
	return nil
}

// Conn exposes the raw gorm handle for advanced queries/transactions.
func (d *Database) Conn() *gorm.DB {
	return d.conn
}

// Close releases the underlying connection pool. Windows cannot delete the
// database file while it is open, so tests and Main must always call it.
func (d *Database) Close() error {
	sqlDB, err := d.conn.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}
	return nil
}

//-----------------------------------------------------------models and types------------------------------------------------------------------------------------------------//

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("record not found")

// FeatureCategory groups related health checks together (e.g. "System Resources").
type FeatureCategory struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex;not null"`
	Description string
}

// Feature describes a single health check available to run.
type Feature struct {
	ID          uint `gorm:"primaryKey"`
	CategoryID  uint `gorm:"not null;index"`
	Category    FeatureCategory
	Key         string `gorm:"uniqueIndex;not null"`
	Name        string `gorm:"not null"`
	Description string
}

// ScanJob records a single execution of one or more health checks.
type ScanJob struct {
	ID         uint `gorm:"primaryKey"`
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     string // "completed": RecordScan creates and completes a job in one transaction
	Summary    string
}

// ScanResult records the outcome of one health check within a ScanJob.
// Target, Value, and Unit were added in v1.1.0; AutoMigrate adds them to
// older databases, where existing rows get an empty target and unit and no
// value.
type ScanResult struct {
	ID         uint `gorm:"primaryKey"`
	ScanJobID  uint `gorm:"not null;index"`
	FeatureID  uint `gorm:"not null;index"`
	Feature    Feature
	Key        string `gorm:"not null"`
	Target     string `gorm:"not null;default:''"`
	Status     string `gorm:"not null"` // "PASS", "WARN", or "FAIL"
	Message    string
	Value      *float64
	Unit       string `gorm:"not null;default:''"`
	DurationMs int64
	CreatedAt  time.Time
}
