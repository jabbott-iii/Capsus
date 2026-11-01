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
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var ErrKeyNotFound = errors.New("key not found")

// ErrKeyExists is returned by SaveKey when a key with the same ID is already stored.
var ErrKeyExists = errors.New("a key with this ID is already stored")

// ErrUnsupportedDatabasePath is returned by NewDatabase for a key database path that
// SQLite would open differently from the file Cryptare prepares (SEC-010).
var ErrUnsupportedDatabasePath = errors.New("unsupported key database path")

// ErrUntrustedDatabase is returned by NewDatabase when the key database, or a SQLite
// file next to it, may have been planted or changed by another user (SEC-016).
var ErrUntrustedDatabase = errors.New("key database is not safe to use")

// ErrOutputIsKeyDatabase is returned when a key export would be written over the key
// database or one of its SQLite files (BUG-017).
var ErrOutputIsKeyDatabase = errors.New("output is the key database")

//--------------------------------------------------core-------------------------------------------------------------------------------------------------//

// Database owns the gorm connection for internal data access.
type Database struct {
	conn *gorm.DB
	path string // as given to NewDatabase
}

// NewDatabase opens (or creates) the sqlite file and runs schema migrations.
// Every connection has SQLite's secure_delete on, so a deleted key's encrypted blob
// is overwritten in the file instead of being left in free space (SEC-009).
func NewDatabase(path string) (*Database, error) {
	if path == "" {
		path = "cryptare.db"
	}
	if err := prepareDatabaseFile(path); err != nil {
		return nil, err
	}

	conn, err := gorm.Open(sqlite.Open(withSecureDelete(path)), &gorm.Config{
		// GORM's default logger prints failed and slow statements, with their values
		// (encrypted key blobs included), to stdout (SEC-017). Errors are returned
		// instead, and the CLI and TUI explain them.
		Logger:         logger.Discard.LogMode(logger.Silent),
		TranslateError: true, // a duplicate key ID becomes gorm.ErrDuplicatedKey
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := conn.AutoMigrate(
		&KeyModel{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate schema: %w", err)
	}

	return &Database{conn: conn, path: path}, nil
}

// prepareDatabaseFile makes the key database private to its owner (SEC-010) and
// refuses one that someone else may have planted or changed (SEC-016). dsn is the
// path as given to NewDatabase; databaseFilePath finds the file SQLite will open,
// including behind a file: URI. A new file is created with mode 0600 before SQLite
// opens it (unless the URI asks for read-only access); SQLite gives its journal files
// the same mode. On Unix, the database and any -journal, -wal or -shm file next to it
// must pass checkDatabaseFileTrust, including when the database itself is new, since
// SQLite would replay a planted journal into it. A file that passes but others can
// read is set to 0600. In-memory databases have no file and are left to SQLite.
func prepareDatabaseFile(dsn string) error {
	path, readOnly, ok, err := databaseFilePath(dsn)
	if err != nil || !ok {
		return err
	}

	if !readOnly {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- path is the user's key database (CRYPTARE_DB_PATH or the default)
		switch {
		case err == nil:
			if err := f.Close(); err != nil {
				return fmt.Errorf("create database file: %w", err)
			}
		case !errors.Is(err, fs.ErrExist):
			return fmt.Errorf("create database file: %w", err)
		}
	}
	if runtime.GOOS == "windows" {
		return nil // Unix owners and permission bits don't apply (SEC-019)
	}

	uid := os.Geteuid()
	for _, p := range databaseFiles(path) {
		info, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("check database file: %w", err)
		}
		if err := checkDatabaseFileTrust(p, fileOwner(info), info.Mode().Perm(), uid); err != nil {
			return err
		}
		if info.Mode().Perm()&0o077 == 0 {
			continue
		}
		if err := os.Chmod(p, 0o600); err != nil && !errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("restrict database file permissions: %w", err)
		}
	}
	return nil
}

// databaseFilePath returns the file SQLite opens for dsn, the key database path as
// given to NewDatabase (SEC-010): a plain path, or the percent-decoded path of a
// file: URI, with readOnly set when the URI asks for mode=ro. ok is false when there
// is no file to prepare: an in-memory database, or a file: URI on Windows, which is
// left to SQLite as before (permissions aren't checked there; SEC-019).
//
// A plain path containing "?" is refused: go-sqlite3 cuts a plain path at its first
// "?", so SQLite would open, and create with the umask's mode, a different file from
// the one checked here. Such a folder can still be used through a file: URI that
// writes the "?" as %3F.
func databaseFilePath(dsn string) (path string, readOnly, ok bool, err error) {
	if dsn == ":memory:" {
		return "", false, false, nil
	}
	if !strings.HasPrefix(dsn, "file:") {
		if strings.Contains(dsn, "?") {
			return "", false, false, fmt.Errorf(`%w: %q contains "?", where SQLite would cut the path; use a file: URI with the "?" written as %%3F`, ErrUnsupportedDatabasePath, dsn)
		}
		return dsn, false, true, nil
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", false, false, fmt.Errorf("%w: %w", ErrUnsupportedDatabasePath, err)
	}
	query := u.Query()
	if query.Get("mode") == "memory" {
		return "", false, false, nil
	}
	if u.Host != "" && u.Host != "localhost" {
		return "", false, false, fmt.Errorf("%w: %q names a remote host", ErrUnsupportedDatabasePath, dsn)
	}
	path = u.Path
	if u.Opaque != "" { // a relative path, as in file:keys.db
		if path, err = url.PathUnescape(u.Opaque); err != nil {
			return "", false, false, fmt.Errorf("%w: %w", ErrUnsupportedDatabasePath, err)
		}
	}
	if path == "" || path == ":memory:" || runtime.GOOS == "windows" {
		return "", false, false, nil
	}
	return filepath.FromSlash(path), query.Get("mode") == "ro", true, nil
}

// databaseFiles returns the database file at path and the SQLite files that can sit
// next to it.
func databaseFiles(path string) []string {
	return []string{path, path + "-journal", path + "-wal", path + "-shm"}
}

// files returns the database's file and SQLite side files, also behind a file: URI;
// an in-memory database has none.
func (d *Database) files() []string {
	path, _, ok, err := databaseFilePath(d.path)
	if err != nil || !ok {
		return nil
	}
	return databaseFiles(path)
}

// checkDatabaseFileTrust refuses a key database file, or one of its SQLite side files,
// that the user uid doesn't own or that group or others can write (SEC-016, refused
// rather than warned about per Q-010, like OpenSSH's StrictModes). Either way another
// user could have planted or changed its rows, and keys stored in a file someone else
// owns can be read by them. owner is -1 when the platform doesn't report it.
func checkDatabaseFileTrust(path string, owner int, perm fs.FileMode, uid int) error {
	if owner >= 0 && owner != uid {
		return fmt.Errorf("%w: %s is owned by another user (uid %d)", ErrUntrustedDatabase, path, owner)
	}
	if perm&0o022 != 0 {
		return fmt.Errorf("%w: other users can change %s (mode %04o); if it is yours, run chmod 600 on it", ErrUntrustedDatabase, path, perm)
	}
	return nil
}

// withSecureDelete adds go-sqlite3's _secure_delete=on parameter to a database path.
// The driver applies it to each new connection, which a one-off PRAGMA would not do
// for a connection pool.
func withSecureDelete(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "_secure_delete=on"
}

// Conn exposes the raw gorm handle for advanced queries/transactions.
func (d *Database) Conn() *gorm.DB {
	return d.conn
}

//-----------------------------------------------------------models and types------------------------------------------------------------------------------------------------//

// KeyModel persists an encryption key (always stored encrypted) together with its metadata.
type KeyModel struct {
	gorm.Model
	KeyID         string `gorm:"uniqueIndex;not null"`
	Algorithm     string `gorm:"not null"`
	EncryptedBlob string `gorm:"not null"` // base64-encoded AES-256-GCM ciphertext
	CreatedAt_    int64  `gorm:"column:created_epoch"`
}

//-----------------------------------------------------------database operations--------------------------------------------------------------------------------------------//

type Storage interface {
	SaveKey(k *KeyModel) error
	ListKeys() ([]KeyModel, error)
	GetKey(keyID string) (*KeyModel, error)
	DeleteKey(keyID string) error
}

// SaveKey persists as a key record. A key whose ID is already stored is refused with
// ErrKeyExists.
func (d *Database) SaveKey(k *KeyModel) error {
	err := d.conn.Save(k).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrKeyExists
	}
	return err
}

// ListKeys returns all stored key records.
func (d *Database) ListKeys() ([]KeyModel, error) {
	var keys []KeyModel
	if err := d.conn.Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetKey returns a key record by its KeyID, or ErrKeyNotFound.
func (d *Database) GetKey(keyID string) (*KeyModel, error) {
	var k KeyModel
	if err := d.conn.Where("key_id = ?", keyID).First(&k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}
	return &k, nil
}

// DeleteKey removes a key record by its KeyID.
func (d *Database) DeleteKey(keyID string) error {
	sqlDB, err := d.conn.DB()
	if err != nil {
		return err
	}
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}

	var blob []byte
	if err := tx.QueryRow("DELETE FROM key_models WHERE key_id = ? RETURNING encrypted_blob", keyID).Scan(&blob); err != nil {
		_ = tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return ErrKeyNotFound
		}
		return err
	}
	defer zeroBytes(blob)

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}

func zeroBytes(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
