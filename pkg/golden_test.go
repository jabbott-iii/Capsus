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
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden fixtures in testdata/golden were written by released binaries (see its
// README.md). Unlike the round-trip tests, they catch a format change made in the writer
// and the reader alike, which would leave users' existing files unreadable.

const (
	goldenPassword       = "golden-fixture-password-1"
	goldenExportPassword = "golden-fixture-export-only-2"
	// goldenNFKCPassword's NFKC form is "golden fixture password".
	goldenNFKCPassword = "golden ﬁxture ｐａｓｓｗｏｒｄ"
	goldenV1KeyID      = "2e5fe1f18a8a72d2"
	goldenV2KeyID      = "b49800c1a2913f46"
)

func goldenPath(name string) string { return filepath.Join("testdata", "golden", name) }

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// checkGoldenOutput compares a decrypted file or folder with the fixtures' plaintext:
// plain/hello.txt for a file, plain/tree for a folder.
func checkGoldenOutput(t *testing.T, got string, folder bool) {
	t.Helper()
	if !folder {
		want := readGolden(t, filepath.Join("plain", "hello.txt"))
		if data, err := os.ReadFile(got); err != nil || !bytes.Equal(data, want) {
			t.Fatalf("decrypted file = %q (err %v), want %q", data, err, want)
		}
		return
	}
	read := func(root string) map[string]string {
		files := map[string]string{}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			files[filepath.ToSlash(rel)] = string(data)
			return err
		})
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		return files
	}
	want, have := read(goldenPath(filepath.Join("plain", "tree"))), read(got)
	if len(want) != 2 || len(have) != len(want) {
		t.Fatalf("decrypted folder has %d files %v, want the fixture's %d", len(have), have, len(want))
	}
	for name, content := range want {
		if have[name] != content {
			t.Fatalf("decrypted %s = %q, want %q", name, have[name], content)
		}
	}
}

// TestGoldenPasswordFiles opens the files and folders that released versions protected
// with a password: the legacy format (v1.0.1) and version 2 with both password key
// derivations (v1.3.1).
func TestGoldenPasswordFiles(t *testing.T) {
	cases := []struct {
		fixture   string
		folder    bool
		passwords []string
		kdf       byte // the version 2 header's KDF; 0 for the legacy format
	}{
		{"v1-file.enc", false, []string{goldenPassword}, 0},
		{"v1-folder.enc", true, []string{goldenPassword}, 0},
		{"v2-file-kdf1.enc", false, []string{goldenPassword}, kdfArgon2id},
		// KDF 2 data opens with the password as typed and with its normalised form.
		{"v2-file-kdf2.enc", false, []string{goldenNFKCPassword, "golden fixture password"}, kdfArgon2idNFKC},
		{"v2-folder.enc", true, []string{goldenPassword}, kdfArgon2id},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			data := readGolden(t, tc.fixture)
			if isV2(data) != (tc.kdf != 0) || (tc.kdf != 0 && (data[11] != keySourcePassword || data[12] != tc.kdf)) {
				t.Fatalf("fixture header % x isn't the one the test expects; never regenerate golden fixtures", data[:min(len(data), 13)])
			}
			for i, password := range tc.passwords {
				out := filepath.Join(t.TempDir(), "out")
				if err := DecryptFile(goldenPath(tc.fixture), out, password); err != nil {
					t.Fatalf("DecryptFile(password %d): %v", i, err)
				}
				checkGoldenOutput(t, out, tc.folder)
			}
			wrong := filepath.Join(t.TempDir(), "wrong")
			if err := DecryptFile(goldenPath(tc.fixture), wrong, goldenExportPassword); err == nil {
				t.Fatal("DecryptFile with another password succeeded")
			}
		})
	}
}

// TestGoldenStoredKeys opens the stored keys that v1.0.1 (legacy blob) and v1.3.1
// (version 2 blob) wrote to their key databases.
func TestGoldenStoredKeys(t *testing.T) {
	for _, fixture := range []string{"v1-stored-key.txt", "v2-stored-key.txt"} {
		t.Run(fixture, func(t *testing.T) {
			blob := string(readGolden(t, fixture))
			key, err := DecryptKeyBlob(blob, goldenPassword)
			if err != nil || len(key) != keyLen {
				t.Fatalf("DecryptKeyBlob = %d bytes, %v; want a %d-byte key", len(key), err, keyLen)
			}
			if _, err := DecryptKeyBlob(blob, goldenExportPassword); !errors.Is(err, ErrWrongMasterPassword) {
				t.Fatalf("DecryptKeyBlob(another password) error = %v, want ErrWrongMasterPassword", err)
			}
		})
	}
}

// TestGoldenKeyExports imports the key exports that v1.0.1 and v1.3.1 wrote, including
// one with a password of its own (BUG-011), and checks that each holds the key its
// database stored.
func TestGoldenKeyExports(t *testing.T) {
	cases := []struct {
		fixture, keyID, storedKey string
	}{
		{"v1-key-export.txt", goldenV1KeyID, "v1-stored-key.txt"},
		{"v2-key-export.txt", goldenV2KeyID, "v2-stored-key.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			db := newTestDatabase(t, false)
			km, err := ImportStoredKey(db, goldenPath(tc.fixture), goldenPassword)
			if err != nil {
				t.Fatalf("ImportStoredKey: %v", err)
			}
			if km.KeyID != tc.keyID || km.EncryptedBlob != string(readGolden(t, tc.storedKey)) {
				t.Fatalf("imported key %s with blob %q, want %s holding %s", km.KeyID, km.EncryptedBlob, tc.keyID, tc.storedKey)
			}
		})
	}

	t.Run("v2-key-export-separate-password.txt", func(t *testing.T) {
		db := newTestDatabase(t, false)
		path := goldenPath("v2-key-export-separate-password.txt")
		if _, err := ImportStoredKey(db, path, goldenPassword); !errors.Is(err, ErrWrongExportPassword) {
			t.Fatalf("import with the key's password: err = %v, want ErrWrongExportPassword", err)
		}
		if _, err := ImportStoredKey(db, path, goldenExportPassword); !errors.Is(err, ErrSeparateKeyPassword) {
			t.Fatalf("import with the export's password only: err = %v, want ErrSeparateKeyPassword", err)
		}
		if keys, _ := db.ListKeys(); len(keys) != 0 {
			t.Fatalf("%d keys stored after refused imports, want 0", len(keys))
		}
		km, err := ImportStoredKey(db, path, goldenExportPassword, goldenPassword)
		if err != nil || km.KeyID != goldenV2KeyID {
			t.Fatalf("import with both passwords = %+v, %v", km, err)
		}
		// The passwords may come in either order.
		if km, err := ImportStoredKey(newTestDatabase(t, false), path, goldenPassword, goldenExportPassword); err != nil || km.KeyID != goldenV2KeyID {
			t.Fatalf("import with the passwords swapped = %+v, %v", km, err)
		}
	})
}

// TestGoldenStoredKeyFiles opens the file and folder encrypted with v1.3.1's stored key
// (key source 2, plan 3.4), with the key imported from v1.3.1's export.
func TestGoldenStoredKeyFiles(t *testing.T) {
	db := newTestDatabase(t, false)
	if _, err := ImportStoredKey(db, goldenPath("v2-key-export.txt"), goldenPassword); err != nil {
		t.Fatalf("ImportStoredKey: %v", err)
	}
	for _, tc := range []struct {
		fixture string
		folder  bool
	}{{"v2-storedkey-file.enc", false}, {"v2-storedkey-folder.enc", true}} {
		t.Run(tc.fixture, func(t *testing.T) {
			data := readGolden(t, tc.fixture)
			if len(data) < v2StoredKeyHeaderLen || data[11] != keySourceStoredKey || data[12] != kdfHKDFStoredKey ||
				!strings.HasSuffix(string(data[:v2StoredKeyHeaderLen]), string([]byte{0xb4, 0x98, 0x00, 0xc1, 0xa2, 0x91, 0x3f, 0x46})) {
				t.Fatalf("fixture header % x isn't the one the test expects; never regenerate golden fixtures", data[:min(len(data), v2StoredKeyHeaderLen)])
			}
			keyID, ok := EncryptedWithStoredKey(goldenPath(tc.fixture))
			if !ok || keyID != goldenV2KeyID {
				t.Fatalf("EncryptedWithStoredKey = %q, %v; want %s", keyID, ok, goldenV2KeyID)
			}
			cred, err := StoredKeyCredential(db, keyID, goldenPassword, false)
			if err != nil {
				t.Fatalf("StoredKeyCredential: %v", err)
			}
			out := filepath.Join(t.TempDir(), "out")
			if err := DecryptFileWithCredentialContext(context.Background(), goldenPath(tc.fixture), out, cred, DefaultExtractLimits()); err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			checkGoldenOutput(t, out, tc.folder)
		})
	}
}
