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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// newStoredKey generates a key protected by testPassword in db and returns its ID.
func newStoredKey(t *testing.T, db *Database) string {
	t.Helper()
	km, err := GenerateStoredKey(db, testPassword)
	if err != nil {
		t.Fatalf("GenerateStoredKey: %v", err)
	}
	return km.KeyID
}

// writeStoredKeyFixtures writes a file and a folder to encrypt under dir.
func writeStoredKeyFixtures(t *testing.T, dir string) (file, folder string) {
	t.Helper()
	file = filepath.Join(dir, "notes.txt")
	folder = filepath.Join(dir, "tree")
	if err := os.WriteFile(file, []byte("stored-key secret"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(folder, "sub"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "sub", "b.txt"), []byte("nested"), 0o600); err != nil {
		t.Fatalf("write folder file: %v", err)
	}
	return file, folder
}

// TestStoredKeyEncryptDecrypt checks plan 3.4 end to end in the core: a file and a
// folder encrypted with a stored key record the key in a 54-byte header and decrypt
// with that key only.
func TestStoredKeyEncryptDecrypt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := newTestDatabase(t, false)
	keyID := newStoredKey(t, db)
	file, folder := writeStoredKeyFixtures(t, dir)

	enc, err := StoredKeyCredential(db, keyID, testPassword, true)
	if err != nil {
		t.Fatalf("StoredKeyCredential: %v", err)
	}
	for _, src := range []string{file, folder} {
		dst := src + encExt
		if err := EncryptFileWithCredentialContext(ctx, src, dst, enc); err != nil {
			t.Fatalf("encrypt %s: %v", src, err)
		}
		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read %s: %v", dst, err)
		}
		h, err := parseV2Header(data)
		if err != nil {
			t.Fatalf("parse header: %v", err)
		}
		if h.keySource != keySourceStoredKey || h.kdfID != kdfHKDFStoredKey || h.kdf != (argon2Params{}) || h.storedKeyID() != keyID {
			t.Fatalf("header = %+v, want key source 2, KDF 3, no Argon2id settings and key %s", h, keyID)
		}
		if !bytes.Equal(h.marshal(), data[:v2StoredKeyHeaderLen]) {
			t.Fatalf("header isn't %d bytes as written", v2StoredKeyHeaderLen)
		}
		if got, ok := EncryptedWithStoredKey(dst); !ok || got != keyID {
			t.Fatalf("EncryptedWithStoredKey = %q, %v; want %s", got, ok, keyID)
		}
	}

	dec, err := StoredKeyCredential(db, keyID, testPassword, false)
	if err != nil {
		t.Fatalf("StoredKeyCredential: %v", err)
	}
	out := filepath.Join(dir, "out.txt")
	if err := DecryptFileWithCredentialContext(ctx, file+encExt, out, dec, DefaultExtractLimits()); err != nil {
		t.Fatalf("decrypt file: %v", err)
	}
	if got, _ := os.ReadFile(out); string(got) != "stored-key secret" {
		t.Fatalf("decrypted file = %q", got)
	}
	outDir := filepath.Join(dir, "out-tree")
	if err := DecryptFileWithCredentialContext(ctx, folder+encExt, outDir, dec, DefaultExtractLimits()); err != nil {
		t.Fatalf("decrypt folder: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(outDir, "sub", "b.txt")); string(got) != "nested" {
		t.Fatalf("decrypted folder file = %q", got)
	}

	// The wrong kind of credential, or another key, is refused before any output.
	other, err := StoredKeyCredential(db, newStoredKey(t, db), testPassword, false)
	if err != nil {
		t.Fatalf("StoredKeyCredential(other): %v", err)
	}
	refused := filepath.Join(dir, "refused.txt")
	for _, tc := range []struct {
		name string
		src  string
		cred Credential
		want error
	}{
		{"password for a stored-key file", file + encExt, PasswordCredential(testPassword), ErrStoredKeyRequired},
		{"another stored key", file + encExt, other, ErrWrongStoredKey},
	} {
		if err := DecryptFileWithCredentialContext(ctx, tc.src, refused, tc.cred, DefaultExtractLimits()); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if _, err := os.Stat(refused); !os.IsNotExist(err) {
			t.Fatalf("%s: output created (stat err %v)", tc.name, err)
		}
	}

	// Password-protected data, version 2 or legacy, doesn't open with a stored key.
	passwordFile := filepath.Join(dir, "password.enc")
	if err := EncryptFile(file, passwordFile, testPassword); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}
	legacy, err := encryptBytes([]byte("legacy"), testPassword)
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	legacyFile := filepath.Join(dir, "legacy.enc")
	if err := os.WriteFile(legacyFile, legacy, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	for _, src := range []string{passwordFile, legacyFile} {
		if err := DecryptFileWithCredentialContext(ctx, src, refused, dec, DefaultExtractLimits()); !errors.Is(err, ErrPasswordRequired) {
			t.Fatalf("decrypt %s with a stored key: err = %v, want ErrPasswordRequired", src, err)
		}
		if _, ok := EncryptedWithStoredKey(src); ok {
			t.Fatalf("EncryptedWithStoredKey(%s) = true for password-protected data", src)
		}
	}
}

// TestStoredKeyHeaderIsAuthenticated checks that the key ID is covered by
// authentication: data relabelled with another key ID doesn't decrypt, even with the
// same key material under that ID.
func TestStoredKeyHeaderIsAuthenticated(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{9}, keyLen)
	cred, err := newStoredKeyCredential("0123456789abcdef", key, true)
	if err != nil {
		t.Fatalf("newStoredKeyCredential: %v", err)
	}
	file, _ := writeStoredKeyFixtures(t, dir)
	enc := file + encExt
	if err := EncryptFileWithCredentialContext(context.Background(), file, enc, cred); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	data, err := os.ReadFile(enc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	data[v2StoredKeyHeaderLen-1] ^= 0x01 // the key ID's last byte: ...ef becomes ...ee
	relabelled := filepath.Join(dir, "relabelled.enc")
	if err := os.WriteFile(relabelled, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	sameKey, err := newStoredKeyCredential("0123456789abcdee", key, false)
	if err != nil {
		t.Fatalf("newStoredKeyCredential: %v", err)
	}
	if err := DecryptFileWithCredentialContext(context.Background(), relabelled, filepath.Join(dir, "out"), sameKey, DefaultExtractLimits()); !errors.Is(err, errDecrypt) {
		t.Fatalf("decrypt relabelled data: err = %v, want errDecrypt", err)
	}
}

// TestStoredKeyHeaderChecks checks that parseV2Header refuses key source 2 headers this
// version doesn't write.
func TestStoredKeyHeaderChecks(t *testing.T) {
	h, err := newV2Header(contentFile)
	if err != nil {
		t.Fatalf("newV2Header: %v", err)
	}
	if err := h.useStoredKey("0123456789abcdef"); err != nil {
		t.Fatalf("useStoredKey: %v", err)
	}
	good := h.marshal()
	if len(good) != v2StoredKeyHeaderLen {
		t.Fatalf("stored-key header is %d bytes, want %d", len(good), v2StoredKeyHeaderLen)
	}
	if _, err := parseV2Header(good); err != nil {
		t.Fatalf("a fresh stored-key header doesn't parse: %v", err)
	}
	set := func(off int, v byte) func(b []byte) []byte { return func(b []byte) []byte { b[off] = v; return b } }
	for _, tc := range []struct {
		name   string
		change func(b []byte) []byte
	}{
		{"stored key content type", set(10, contentStoredKey)},
		{"key export content type", set(10, contentKeyExport)},
		{"Argon2id KDF", set(12, kdfArgon2id)},
		{"Argon2id memory", func(b []byte) []byte { binary.BigEndian.PutUint32(b[13:], 65536); return b }},
		{"Argon2id lanes", set(21, 4)},
		{"truncated key ID", func(b []byte) []byte { return b[:v2StoredKeyHeaderLen-1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.change(append([]byte{}, good...))
			if _, err := parseV2Header(b); !errors.Is(err, errUnsupportedFormat) {
				t.Fatalf("parseV2Header() error = %v, want errUnsupportedFormat", err)
			}
		})
	}

	// Stored keys and key exports are never written with a stored key.
	cred, err := newStoredKeyCredential("0123456789abcdef", make([]byte, keyLen), true)
	if err != nil {
		t.Fatalf("newStoredKeyCredential: %v", err)
	}
	for _, content := range []byte{contentStoredKey, contentKeyExport} {
		if _, err := newEncryptingWriter(&bytes.Buffer{}, cred, content); err == nil {
			t.Fatalf("newEncryptingWriter(%s) with a stored key succeeded", contentName(content))
		}
	}
}

// TestStoredKeyCredentialChecks covers unlocking: an unknown key, a wrong master
// password, and a key stored by an earlier version under a short master password, which
// still decrypts but can't encrypt new files (SEC-001).
func TestStoredKeyCredentialChecks(t *testing.T) {
	db := newTestDatabase(t, false)
	keyID := newStoredKey(t, db)
	if _, err := StoredKeyCredential(db, "ffffffffffffffff", testPassword, false); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("unknown key: err = %v, want ErrKeyNotFound", err)
	}
	if _, err := StoredKeyCredential(db, keyID, "a different password", false); !errors.Is(err, ErrWrongMasterPassword) {
		t.Fatalf("wrong master password: err = %v, want ErrWrongMasterPassword", err)
	}

	const short = "hunter2"
	blob, err := encryptBytes(make([]byte, keyLen), short)
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	const legacyID = "00000000000000aa"
	if err := db.SaveKey(&KeyModel{KeyID: legacyID, Algorithm: keyAlgorithm, EncryptedBlob: base64.StdEncoding.EncodeToString(blob), CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	if _, err := StoredKeyCredential(db, legacyID, short, true); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("encrypting with a short-password key: err = %v, want ErrWeakPassword", err)
	}
	if _, err := StoredKeyCredential(db, legacyID, short, false); err != nil {
		t.Fatalf("decrypting with a short-password key: %v", err)
	}
}

// TestExportChecksMasterPassword is a regression test for BUG-011's export side: the
// export is protected by the key's own master password, so another password is refused
// and nothing is written.
func TestExportChecksMasterPassword(t *testing.T) {
	dir := t.TempDir()
	db := newTestDatabase(t, false)
	keyID := newStoredKey(t, db)
	km, err := db.GetKey(keyID)
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	path := filepath.Join(dir, "key.ckey")
	if err := ExportKeyToFile(km, "correct horse battery stapler", path); !errors.Is(err, ErrWrongMasterPassword) {
		t.Fatalf("export with another password: err = %v, want ErrWrongMasterPassword", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("export written despite the wrong password (stat err %v)", err)
	}
	if err := ExportKeyToFile(km, testPassword, path); err != nil {
		t.Fatalf("export with the master password: %v", err)
	}

	// The export opens with the key's password, and imports as the same key.
	other := newTestDatabase(t, false)
	imported, err := ImportStoredKey(other, path, testPassword)
	if err != nil || imported.KeyID != keyID || imported.EncryptedBlob != km.EncryptedBlob {
		t.Fatalf("ImportStoredKey = %+v, %v; want key %s", imported, err, keyID)
	}
}

// TestImportChecksInnerKey is a regression test for BUG-011's import side: an export
// whose key doesn't open with any password given isn't stored.
func TestImportChecksInnerKey(t *testing.T) {
	dir := t.TempDir()
	const keyPassword = "the key's own master password"
	const exportPassword = "a separate export password"
	blob, err := EncryptKeyBlob(make([]byte, keyLen), keyPassword)
	if err != nil {
		t.Fatalf("EncryptKeyBlob: %v", err)
	}
	// An export the way v1.3.1 could write it: the outer layer under its own password.
	raw, err := json.Marshal(KeyExport{Version: 1, KeyID: "0123456789abcdef", Algorithm: keyAlgorithm, CreatedAt: 1, EncryptedBlob: blob})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sealed, err := sealV2(raw, exportPassword, contentKeyExport)
	if err != nil {
		t.Fatalf("sealV2: %v", err)
	}
	path := filepath.Join(dir, "two-passwords.ckey")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(sealed)), 0o600); err != nil {
		t.Fatalf("write export: %v", err)
	}

	db := newTestDatabase(t, false)
	if _, err := ImportStoredKey(db, path, keyPassword); !errors.Is(err, ErrWrongExportPassword) {
		t.Fatalf("wrong export password: err = %v, want ErrWrongExportPassword", err)
	}
	if _, err := ImportStoredKey(db, path, exportPassword); !errors.Is(err, ErrSeparateKeyPassword) {
		t.Fatalf("export password only: err = %v, want ErrSeparateKeyPassword", err)
	}
	if _, err := ImportStoredKey(db, path, exportPassword, "not the key's password"); !errors.Is(err, ErrWrongMasterPassword) {
		t.Fatalf("wrong key password: err = %v, want ErrWrongMasterPassword", err)
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 0 {
		t.Fatalf("stored keys after refused imports = %d (err %v), want 0", len(keys), err)
	}
	// Either password may come first.
	if _, err := ImportStoredKey(db, path, keyPassword, exportPassword); err != nil {
		t.Fatalf("import with both passwords: %v", err)
	}
	if _, err := ImportStoredKey(db, path, exportPassword, keyPassword); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("second import: err = %v, want ErrKeyExists", err)
	}

	// A key stored before SEC-001 under an empty master password, exported with a
	// password of its own: an empty other password is tried too.
	legacy, err := encryptBytes(make([]byte, keyLen), "")
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	raw, err = json.Marshal(KeyExport{Version: 1, KeyID: "00000000000000bb", Algorithm: keyAlgorithm, CreatedAt: 1, EncryptedBlob: base64.StdEncoding.EncodeToString(legacy)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if sealed, err = sealV2(raw, exportPassword, contentKeyExport); err != nil {
		t.Fatalf("sealV2: %v", err)
	}
	emptyKey := filepath.Join(dir, "empty-key.ckey")
	if err := os.WriteFile(emptyKey, []byte(base64.StdEncoding.EncodeToString(sealed)), 0o600); err != nil {
		t.Fatalf("write export: %v", err)
	}
	if _, err := ImportStoredKey(db, emptyKey, exportPassword, ""); err != nil {
		t.Fatalf("import of a key with an empty master password: %v", err)
	}
}

// TestDecryptOnlyCredentialDoesntEncrypt checks that the core, not only the
// interfaces, keeps a stored key whose master password wasn't policy-checked from
// encrypting new files (SEC-001).
func TestDecryptOnlyCredentialDoesntEncrypt(t *testing.T) {
	dir := t.TempDir()
	db := newTestDatabase(t, false)
	keyID := newStoredKey(t, db)
	file, _ := writeStoredKeyFixtures(t, dir)
	dec, err := StoredKeyCredential(db, keyID, testPassword, false)
	if err != nil {
		t.Fatalf("StoredKeyCredential: %v", err)
	}
	dst := filepath.Join(dir, "out.enc")
	if err := EncryptFileWithCredentialContext(context.Background(), file, dst, dec); err == nil {
		t.Fatal("encrypting with a credential unlocked to decrypt succeeded")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("output written (stat err %v)", err)
	}
}
