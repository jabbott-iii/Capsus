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
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// Fuzz tests for the code that reads untrusted input (plan 5.21). `go test` runs their
// seed corpora as ordinary tests; run one with, for example,
// `go test -run '^$' -fuzz FuzzParseV2Header -fuzztime 1m ./pkg`.

// fuzzPassword is the password the fuzz seeds are encrypted with.
const fuzzPassword = "fuzzing password"

// fuzzStoredKey returns the stored key that the stored-key fuzz seeds are encrypted
// with (plan 3.4).
func fuzzStoredKey(tb testing.TB) Credential {
	tb.Helper()
	cred, err := newStoredKeyCredential("0123456789abcdef", bytes.Repeat([]byte{7}, keyLen), true)
	if err != nil {
		tb.Fatalf("newStoredKeyCredential: %v", err)
	}
	return cred
}

// cheapToOpen reports whether opening data would derive a key cheaply. parseV2Header
// allows Argon2id settings up to 1 GiB on purpose (Q-008), which would make each fuzz
// run slow; inputs whose header asks for more than the tests' setting are skipped.
func cheapToOpen(data []byte) bool {
	h, err := parseV2Header(data)
	return err != nil || (h.kdf.memoryKiB <= 256 && h.kdf.iterations <= 2)
}

// FuzzParseV2Header checks that header parsing never panics and that an accepted header
// is exactly what marshal writes for it.
func FuzzParseV2Header(f *testing.F) {
	for _, c := range []byte{contentFile, contentFolder, contentStoredKey, contentKeyExport} {
		h, err := newV2Header(c)
		if err != nil {
			f.Fatalf("newV2Header: %v", err)
		}
		f.Add(h.marshal())
		h.kdfID = kdfArgon2idNFKC
		f.Add(h.marshal())
		if err := h.useStoredKey("0123456789abcdef"); err != nil {
			f.Fatalf("useStoredKey: %v", err)
		}
		f.Add(h.marshal()) // refused for stored keys and key exports
	}
	f.Add([]byte(v2Magic))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		h, err := parseV2Header(data)
		if err != nil {
			if !errors.Is(err, errUnsupportedFormat) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		if got := h.marshal(); !bytes.Equal(got, data[:len(got)]) {
			t.Fatalf("header doesn't round-trip:\n read %x\nwrote %x", data[:len(got)], got)
		}
	})
}

// FuzzDecryptingReader feeds damaged and arbitrary streams to the version 2 decryption,
// with the seeds' password and with their stored key (plan 3.4), which must fail
// cleanly: no panic, no hang, and no plaintext from a stream that doesn't authenticate
// to its final chunk.
func FuzzDecryptingReader(f *testing.F) {
	creds := []Credential{PasswordCredential(fuzzPassword), fuzzStoredKey(f)}
	for _, cred := range creds {
		for _, size := range []int{0, 1, 100, 70_000} { // 70,000 bytes span two 64 KiB chunks
			var buf bytes.Buffer
			w, err := newEncryptingWriter(&buf, cred, contentFile)
			if err != nil {
				f.Fatalf("newEncryptingWriter: %v", err)
			}
			if _, err := w.Write(bytes.Repeat([]byte{'x'}, size)); err != nil {
				f.Fatalf("write: %v", err)
			}
			if err := w.Close(); err != nil {
				f.Fatalf("close: %v", err)
			}
			f.Add(buf.Bytes())
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if !cheapToOpen(data) {
			t.Skip("header asks for an expensive key derivation")
		}
		for _, cred := range creds {
			_, plain, err := newDecryptingReader(bufio.NewReader(bytes.NewReader(data)), cred, contentFile, contentFolder)
			if err != nil {
				continue
			}
			got, err := io.ReadAll(plain)
			if err != nil {
				continue
			}
			// Whatever decrypts must be one of the seeds' plaintexts: all 'x'.
			if len(bytes.Trim(got, "x")) != 0 {
				t.Fatalf("decrypted %d bytes that aren't a seed's plaintext", len(got))
			}
		}
	})
}

// FuzzKeyExportValidation checks that an accepted key export has fields that are safe
// to store and print (SEC-011, SEC-016).
func FuzzKeyExportValidation(f *testing.F) {
	rawKey, err := GenerateKey()
	if err != nil {
		f.Fatalf("GenerateKey: %v", err)
	}
	blob, err := EncryptKeyBlob(rawKey, testPassword)
	if err != nil {
		f.Fatalf("EncryptKeyBlob: %v", err)
	}
	valid, err := json.Marshal(KeyExport{Version: 1, KeyID: "0123456789abcdef", Algorithm: keyAlgorithm, CreatedAt: 1, EncryptedBlob: blob})
	if err != nil {
		f.Fatalf("marshal: %v", err)
	}
	f.Add(valid)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"version":1,"key_id":"\u001b]0;x\u0007","algorithm":"AES-256-GCM"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var e KeyExport
		if err := unmarshalKeyExport(data, &e); err != nil {
			return
		}
		if err := validateKeyExport(e); err != nil {
			if !errors.Is(err, ErrInvalidKeyExport) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		if !isKeyID(e.KeyID) || e.Algorithm != keyAlgorithm || displayText(e.KeyID) != e.KeyID {
			t.Fatalf("accepted key ID %q, algorithm %q", e.KeyID, e.Algorithm)
		}
	})
}

// FuzzParseSize checks that --max-size parsing never panics or returns a negative size.
func FuzzParseSize(f *testing.F) {
	for _, s := range []string{"0", "1024", "500MB", "10 GiB", "9223372036854775807", "9223372036854775807KiB", "-1", "1e3", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if n, err := parseSize(s); err == nil && n < 0 {
			t.Fatalf("parseSize(%q) = %d", s, n)
		}
	})
}

// fuzzExtraction runs extract on a new output folder inside a fresh base folder, then
// checks that nothing was written outside the output folder and that no symlink was
// created (SEC-008).
func fuzzExtraction(t *testing.T, extract func(dir string, budget *extractBudget) error) {
	base := t.TempDir()
	out := filepath.Join(base, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	budget := &extractBudget{limits: ExtractLimits{MaxBytes: 1 << 20, MaxEntries: 64}}
	_ = extract(out, budget) // errors are expected for most inputs
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 1 {
		t.Fatalf("base folder holds %d entries (err %v); extraction wrote outside its folder", len(entries), err)
	}
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			t.Fatalf("extraction created a symlink: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// FuzzExtractTar feeds arbitrary tar streams to extractTarGz.
func FuzzExtractTar(f *testing.F) {
	for _, entries := range [][]*tar.Header{
		{{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755}, {Name: "dir/a.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3}},
		{{Name: "../escape.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3}},
		{{Name: "/abs.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3}},
		{{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../etc"}, {Name: "link/x", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3}},
	} {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for _, h := range entries {
			if err := tw.WriteHeader(h); err != nil {
				f.Fatalf("tar header: %v", err)
			}
			if h.Size > 0 {
				if _, err := tw.Write([]byte("abc")); err != nil {
					f.Fatalf("tar data: %v", err)
				}
			}
		}
		if err := tw.Close(); err != nil {
			f.Fatalf("tar close: %v", err)
		}
		f.Add(buf.Bytes())
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzExtraction(t, func(dir string, budget *extractBudget) error {
			return extractTarGz(context.Background(), bytes.NewReader(data), dir, budget)
		})
	})
}

// FuzzExtractZip feeds arbitrary zip archives to extractZipEntries.
func FuzzExtractZip(f *testing.F) {
	for _, names := range [][]string{{"dir/", "dir/a.txt"}, {"../escape.txt"}, {"/abs.txt"}} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, name := range names {
			w, err := zw.Create(name)
			if err != nil {
				f.Fatalf("zip create: %v", err)
			}
			if name[len(name)-1] != '/' {
				if _, err := w.Write([]byte("abc")); err != nil {
					f.Fatalf("zip write: %v", err)
				}
			}
		}
		if err := zw.Close(); err != nil {
			f.Fatalf("zip close: %v", err)
		}
		f.Add(buf.Bytes())
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		fuzzExtraction(t, func(dir string, budget *extractBudget) error {
			return extractZipEntries(context.Background(), r.File, dir, budget)
		})
	})
}
