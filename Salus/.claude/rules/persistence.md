---
paths:
  - "pkg/database*.go"
  - "pkg/scan-*.go"
  - "pkg/seed.go"
---

# Persistence (SQLite through GORM)

Users keep years of job history in these databases, and older and newer
Salus versions may open the same file. Treat every schema change as a
migration of someone else's data.

## Schema

- The schema comes from the GORM model structs and `AutoMigrate`, which
  only adds tables, columns, and indexes. Changes must be additive. A new
  column needs a default or must be nullable so `ALTER TABLE ADD COLUMN`
  works on existing rows.
- Dropping, renaming, or retyping a column, or rewriting stored data, is a
  destructive migration: stop and get the maintainer's approval for a
  written plan first.
- Every model change bumps `schemaVersion` in `pkg/database.go` (stored as
  `PRAGMA user_version`) and updates `wantSchema` in
  `pkg/database_test.go`; `TestSchemaMatchesSchemaVersion` fails until both
  match. A database already at `schemaVersion` is opened without migrating,
  so a model change without a bump never reaches existing databases.
- A schema or catalog change needs one write to an existing database after
  upgrading. Note that in the README upgrade section.

## Opening and writing

- `prepareDatabase` only reads a current database, so read-only databases
  keep working. Migration, seeding, and the version update run in one
  transaction; the connection defaults from `withConnectionDefaults`
  (`_busy_timeout=5000`, `_txlock=immediate`) make concurrent first opens
  take turns. Keep both properties.
- Seeding is insert-only (`firstOrInsert`). Never update existing catalog
  rows on open.
- Multi-row writes go in one `Transaction`, and queries inside it use `tx`,
  not `db.Conn()`.
- Keep GORM's logger at `logger.Silent`: the default logger writes to
  stdout and would corrupt `--json` output.
- A new database file is created with mode `0600` and missing parent
  directories with `0700`. Never change the mode of an existing file.
  `databaseFile` resolves the file behind a path with go-sqlite3 parameters;
  `misconfig` uses the same helper.
- Commands receive a `DatabaseOpener` and call it only when they need
  storage, so `--help`, `--version`, `completion`, and `check run --no-save`
  never create a database. Every `NewDatabase` is paired with `Close`
  (Windows cannot delete an open SQLite file).
- Stored messages and targets are capped by `truncateMessage` at
  `maxStoredMessageLen`.

## Queries

- Compare stored times with `julianday(...)`: rows keep the UTC offset they
  were written with, so text comparison is wrong across offsets.
- The schema has no foreign key from results to jobs. Pruning deletes a
  job's results explicitly, in the same transaction, selecting jobs with a
  subquery to stay under SQLite's bound-variable limit.
- Results recorded before targets existed have no target; `jobs diff` and
  `jobs stats` match them through `legacyMatch`. Keep that path working.

## Tests

Use a file-backed database in `t.TempDir()` through `newTestDatabase` or
`newSeededTestDatabase`. Never use `:memory:`: each pooled connection would
see a different database.
