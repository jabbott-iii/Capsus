---
paths:
  - "pkg/database.go"
  - "pkg/database_test.go"
  - "pkg/database_path.go"
  - "pkg/database_path_test.go"
  - "pkg/keys.go"
  - "pkg/keys_test.go"
  - "pkg/keys_output_test.go"
  - "pkg/stored_keys_test.go"
  - "pkg/lazy_database_test.go"
  - "pkg/fileowner_*.go"
---

# Key store

- **Location.** Without `CRYPTARE_DB_PATH`, the store is `cryptare/cryptare.db` in the
  user data folder (`%LocalAppData%`, `~/Library/Application Support`, or
  `$XDG_DATA_HOME` if absolute, else `~/.local/share`). The CLI's opener never falls back
  to the current folder (an unusable data folder is an error that names
  `CRYPTARE_DB_PATH`); only the exported `NewDatabase("")` defaults to `./cryptare.db`,
  for API callers. A legacy `cryptare.db` in the current folder only gets a notice
  (`noticeLegacyDatabase`); it is never opened, moved or copied.
- **Creation and trust.** The opener in `run.go` creates the default folder 0700
  (`prepareDefaultDatabaseFolder`). `NewDatabase` creates the file 0600 and, except on
  Windows, tightens an existing one. On Unix it refuses a store or SQLite side file owned by
  another user or writable by group/others (`ErrUntrustedDatabase`). A plain path with
  `?` is refused (`ErrUnsupportedDatabasePath`).
- **Deletion.** Every connection enables SQLite `secure_delete`, so a deleted key's blob
  is overwritten in the file. Keep it.
- **Logging.** GORM's logger stays silent; errors are typed and name the key, never the
  SQL or bound values (`TestKeysCommandsKeepStdoutClean`, `TestDatabaseErrorsAreTyped`).
- **Key material** is stored only encrypted. Values read back are shown through
  `displayText`.
- **Shared flows.** `GenerateStoredKey`, `StoredKeyCredential` and `ImportStoredKey`
  (`keys.go`) take a `Storage`, so core doesn't depend on GORM. A credential unlocked
  for decryption only can't encrypt (`EncryptFileWithCredentialContext` refuses it).
  Import validates the export (`validateKeyExport`, `validStoredKeyBlob`) and checks
  the key inside opens before storing it.
- **Tests** use `newTestDatabase(t, useFile)` and close their SQL handles, or Windows CI
  can't delete the temporary files.
- Docs: `CRYPTARE_DB_PATH` and the store's behaviour are described in README's
  Configuration table and Quick start notes; keep them in step.
