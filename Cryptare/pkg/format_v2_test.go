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
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// v2Prefix is the start of every version 2 artifact: the magic and the version.
const v2Prefix = "CRYPTARE\x00\x02"

// TestEncryptWritesVersion2Format is a regression test for SEC-005: files, folders,
// stored keys and key exports are all written in the version 2 format, with a header
// naming their content type and recording the Argon2id setting used.
func TestEncryptWritesVersion2Format(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	folder := filepath.Join(dir, "tree")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "b.txt"), []byte("world"), 0o600); err != nil {
		t.Fatalf("write folder file: %v", err)
	}

	artifacts := map[string]byte{}
	for src, content := range map[string]byte{file: 1, folder: 2} {
		dst := src + ".enc"
		if err := EncryptFile(src, dst, testPassword); err != nil {
			t.Fatalf("EncryptFile(%s): %v", src, err)
		}
		artifacts[dst] = content
	}
	checkHeader := func(name string, data []byte, content byte) {
		t.Helper()
		if len(data) < 46 || string(data[:10]) != v2Prefix || data[10] != content {
			t.Fatalf("%s doesn't start with a version 2 header for content type %d: % x", name, content, data[:min(len(data), 16)])
		}
		if data[11] != 1 || data[12] != 1 {
			t.Fatalf("%s: key source %d, KDF %d; want password (1) and Argon2id (1)", name, data[11], data[12])
		}
		kdf := argon2Params{memoryKiB: binary.BigEndian.Uint32(data[13:17]), iterations: binary.BigEndian.Uint32(data[17:21]), threads: data[21]}
		if kdf != passwordKDF {
			t.Fatalf("%s: header KDF %+v, want %+v", name, kdf, passwordKDF)
		}
	}
	for path, content := range artifacts {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		checkHeader(filepath.Base(path), data, content)
	}

	rawKey, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	blob, err := EncryptKeyBlob(rawKey, testPassword)
	if err != nil {
		t.Fatalf("EncryptKeyBlob: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	checkHeader("stored key", decoded, 3)

	exportPath := filepath.Join(dir, "key.ckey")
	km := &KeyModel{KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", EncryptedBlob: blob, CreatedAt_: 1}
	if err := ExportKeyToFile(km, testPassword, exportPath); err != nil {
		t.Fatalf("ExportKeyToFile: %v", err)
	}
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	decoded, err = base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		t.Fatalf("decode export: %v", err)
	}
	checkHeader("key export", decoded, 4)

	// And everything reads back.
	if got, err := DecryptKeyBlob(blob, testPassword); err != nil || !bytes.Equal(got, rawKey) {
		t.Fatalf("DecryptKeyBlob = %x, %v; want the key", got, err)
	}
	imported, err := ImportKeyFromFile(exportPath, testPassword)
	if err != nil || imported.EncryptedBlob != blob {
		t.Fatalf("ImportKeyFromFile = %+v, %v; want the exported key", imported, err)
	}
}

// TestDefaultPasswordKDFIsWritten checks the owner's choice for plan 3.1: new data uses
// Argon2id with 64 MiB, 3 passes and 4 lanes, in 64 KiB chunks. (Other tests use a
// cheaper setting; see TestMain.)
func TestDefaultPasswordKDFIsWritten(t *testing.T) {
	previous := passwordKDF
	passwordKDF = defaultPasswordKDF
	t.Cleanup(func() { passwordKDF = previous })

	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("default settings"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := EncryptFile(src, src+".enc", testPassword); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}
	data, err := os.ReadFile(src + ".enc")
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	h, err := parseV2Header(data)
	if err != nil {
		t.Fatalf("parseV2Header: %v", err)
	}
	if want := (argon2Params{memoryKiB: 64 * 1024, iterations: 3, threads: 4}); h.kdf != want {
		t.Fatalf("header KDF = %+v, want %+v", h.kdf, want)
	}
	if h.chunkShift != 16 {
		t.Fatalf("chunk size = 2^%d, want 2^16", h.chunkShift)
	}
	out := filepath.Join(dir, "out.txt")
	if err := DecryptFile(src+".enc", out, testPassword); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}
	if got, _ := os.ReadFile(out); string(got) != "default settings" {
		t.Fatalf("decrypted %q", got)
	}
}

// TestStreamRoundTripSizes is a regression test for BUG-010: files are encrypted in
// 64 KiB chunks, each with its own tag, and round-trip at every size around a chunk
// boundary, including empty files.
func TestStreamRoundTripSizes(t *testing.T) {
	const chunk = 1 << 16
	dir := t.TempDir()
	for _, n := range []int{0, 1, chunk - 1, chunk, chunk + 1, 3 * chunk, 3*chunk + 5} {
		data := make([]byte, n)
		for i := range data {
			data[i] = byte(i*31 + i/chunk)
		}
		src := filepath.Join(dir, "plain.bin")
		if err := os.WriteFile(src, data, 0o600); err != nil {
			t.Fatalf("write %d bytes: %v", n, err)
		}
		enc := src + ".enc"
		_ = os.Remove(enc)
		if err := EncryptFile(src, enc, testPassword); err != nil {
			t.Fatalf("EncryptFile(%d bytes): %v", n, err)
		}
		chunks := n/chunk + 1 // the final chunk holds the rest, possibly nothing...
		if n > 0 && n%chunk == 0 {
			chunks = n / chunk // ...unless the data ends on a chunk boundary
		}
		info, err := os.Stat(enc)
		if err != nil {
			t.Fatalf("stat artifact: %v", err)
		}
		if want := int64(46 + n + chunks*16); info.Size() != want {
			t.Fatalf("%d bytes: artifact is %d bytes, want %d (header + data + %d tags)", n, info.Size(), want, chunks)
		}
		out := filepath.Join(dir, "out.bin")
		_ = os.Remove(out)
		if err := DecryptFile(enc, out, testPassword); err != nil {
			t.Fatalf("DecryptFile(%d bytes): %v", n, err)
		}
		if got, err := os.ReadFile(out); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("%d bytes: round trip changed the data (err %v)", n, err)
		}
	}
}

// TestStreamRejectsTampering checks that the chunked format detects every kind of
// change to an encrypted file or folder: flipped bits, a changed header, dropped,
// reordered or appended chunks, and truncation, including at a chunk boundary.
// Nothing is written when it does.
func TestStreamRejectsTampering(t *testing.T) {
	const chunk = 1 << 16
	const header = 46
	dir := t.TempDir()
	// Random bytes don't compress, so the folder's tar.gz also spans four chunks.
	plain := make([]byte, 3*chunk+100)
	if _, err := rand.Read(plain); err != nil {
		t.Fatalf("random data: %v", err)
	}
	file := filepath.Join(dir, "plain.bin")
	if err := os.WriteFile(file, plain, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	folder := filepath.Join(dir, "tree")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "big.bin"), plain, 0o600); err != nil {
		t.Fatalf("write folder file: %v", err)
	}

	sealed := func(src string) []byte {
		t.Helper()
		enc := filepath.Join(t.TempDir(), "x.enc")
		if err := EncryptFile(src, enc, testPassword); err != nil {
			t.Fatalf("EncryptFile: %v", err)
		}
		data, err := os.ReadFile(enc)
		if err != nil {
			t.Fatalf("read artifact: %v", err)
		}
		return data
	}
	sealedChunk := chunk + 16
	cut := func(data []byte, i int) []byte { return data[header+i*sealedChunk : header+(i+1)*sealedChunk] }

	tampers := []struct {
		name   string
		change func(d []byte) []byte
	}{
		{"flipped bit in a middle chunk", func(d []byte) []byte { d[header+sealedChunk+10] ^= 1; return d }},
		{"flipped bit in the salt", func(d []byte) []byte { d[25] ^= 1; return d }},
		{"flipped bit in the nonce prefix", func(d []byte) []byte { d[40] ^= 1; return d }},
		{"content type changed", func(d []byte) []byte {
			if d[10] == 1 {
				d[10] = 2
			} else {
				d[10] = 1
			}
			return d
		}},
		{"Argon2id memory changed", func(d []byte) []byte { d[16] ^= 1; return d }},
		// 1 and 2 both parse (Q-009), so the change must fail authentication.
		{"key derivation identifier changed", func(d []byte) []byte { d[12] = 3 - d[12]; return d }},
		{"final chunk dropped", func(d []byte) []byte { return d[:header+3*sealedChunk] }},
		{"truncated inside the final chunk", func(d []byte) []byte { return d[:len(d)-5] }},
		{"middle chunk dropped", func(d []byte) []byte {
			return append(append([]byte{}, d[:header+sealedChunk]...), d[header+2*sealedChunk:]...)
		}},
		{"chunks swapped", func(d []byte) []byte {
			c0, c1 := append([]byte{}, cut(d, 0)...), append([]byte{}, cut(d, 1)...)
			copy(d[header:], c1)
			copy(d[header+sealedChunk:], c0)
			return d
		}},
		{"bytes appended", func(d []byte) []byte { return append(d, make([]byte, 16)...) }},
		{"first chunk appended again", func(d []byte) []byte { return append(d, cut(d, 0)...) }},
		{"header only", func(d []byte) []byte { return d[:header] }},
	}

	for _, src := range []string{file, folder} {
		original := sealed(src)
		if len(original) <= header+3*sealedChunk {
			t.Fatalf("%s: artifact too short (%d bytes) for this test", src, len(original))
		}
		for _, tc := range tampers {
			t.Run(filepath.Base(src)+"/"+tc.name, func(t *testing.T) {
				tampered := tc.change(append([]byte{}, original...))
				caseDir := t.TempDir()
				enc := filepath.Join(caseDir, "x.enc")
				if err := os.WriteFile(enc, tampered, 0o600); err != nil {
					t.Fatalf("write tampered artifact: %v", err)
				}
				out := filepath.Join(caseDir, "out")
				err := DecryptFile(enc, out, testPassword)
				if !errors.Is(err, errDecrypt) && !errors.Is(err, errUnsupportedFormat) {
					t.Fatalf("DecryptFile() error = %v, want a decryption or format error", err)
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatalf("output written from a tampered artifact (stat err: %v)", err)
				}
				if left := tempLeftovers(t, caseDir); len(left) != 0 {
					t.Fatalf("temporary files left behind: %v", left)
				}
			})
		}
	}
}

// TestDecryptRejectsUnsupportedHeaders checks that header values this build doesn't
// know, or Argon2id settings beyond its limits, are refused before any key derivation,
// so a crafted file can't make decryption use unbounded memory or time.
func TestDecryptRejectsUnsupportedHeaders(t *testing.T) {
	h, err := newV2Header(contentFile)
	if err != nil {
		t.Fatalf("newV2Header: %v", err)
	}
	good := h.marshal()
	if _, err := parseV2Header(good); err != nil {
		t.Fatalf("a fresh header doesn't parse: %v", err)
	}
	put32 := func(off int, v uint32) func(b []byte) {
		return func(b []byte) { binary.BigEndian.PutUint32(b[off:], v) }
	}
	set := func(off int, v byte) func(b []byte) { return func(b []byte) { b[off] = v } }
	tests := []struct {
		name   string
		change func(b []byte)
	}{
		{"format version 3", set(9, 3)},
		{"unknown content type", set(10, 9)},
		{"stored-key source with Argon2id settings", set(11, 2)},
		{"unknown key source", set(11, 3)},
		{"stored-key KDF with a password", set(12, 3)},
		{"unknown KDF", set(12, 4)},
		{"4 GiB of memory", put32(13, 4<<20)},
		{"memory below 8 KiB per lane", put32(13, 7)},
		{"zero passes", put32(17, 0)},
		{"11 passes", put32(17, 11)},
		{"zero lanes", set(21, 0)},
		{"17 lanes", set(21, 17)},
		{"chunk size 2^9", set(38, 9)},
		{"chunk size 2^25", set(38, 25)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := append([]byte{}, good...)
			tt.change(b)
			if _, err := parseV2Header(b); !errors.Is(err, errUnsupportedFormat) {
				t.Fatalf("parseV2Header() error = %v, want errUnsupportedFormat", err)
			}
		})
	}

	// End to end: a file asking for 4 GiB is refused at once.
	b := append([]byte{}, good...)
	put32(13, 4<<20)(b)
	enc := filepath.Join(t.TempDir(), "greedy.enc")
	if err := os.WriteFile(enc, append(b, make([]byte, 32)...), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	start := time.Now()
	if err := DecryptFile(enc, "", testPassword); !errors.Is(err, errUnsupportedFormat) {
		t.Fatalf("DecryptFile() error = %v, want errUnsupportedFormat", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("refusing the header took %v", elapsed)
	}
}

// TestDecryptRejectsWrongContentType checks that the authenticated content type keeps
// each kind of artifact to its own use: a key export isn't decrypted as a file, and a
// stored key isn't imported as an export.
func TestDecryptRejectsWrongContentType(t *testing.T) {
	dir := t.TempDir()
	rawKey, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	blob, err := EncryptKeyBlob(rawKey, testPassword)
	if err != nil {
		t.Fatalf("EncryptKeyBlob: %v", err)
	}
	exportPath := filepath.Join(dir, "key.ckey")
	km := &KeyModel{KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", EncryptedBlob: blob, CreatedAt_: 1}
	if err := ExportKeyToFile(km, testPassword, exportPath); err != nil {
		t.Fatalf("ExportKeyToFile: %v", err)
	}
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		t.Fatalf("decode export: %v", err)
	}

	asFile := filepath.Join(dir, "export-as-file.enc")
	if err := os.WriteFile(asFile, decoded, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := DecryptFile(asFile, "", testPassword); !errors.Is(err, errUnsupportedFormat) {
		t.Fatalf("DecryptFile(key export) error = %v, want errUnsupportedFormat", err)
	}
	if _, err := DecryptKeyBlob(string(raw), testPassword); !errors.Is(err, errUnsupportedFormat) {
		t.Fatalf("DecryptKeyBlob(key export) error = %v, want errUnsupportedFormat", err)
	}

	storedAsExport := filepath.Join(dir, "stored.ckey")
	if err := os.WriteFile(storedAsExport, []byte(blob), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ImportKeyFromFile(storedAsExport, testPassword); !errors.Is(err, errUnsupportedFormat) {
		t.Fatalf("ImportKeyFromFile(stored key) error = %v, want errUnsupportedFormat", err)
	}
}
