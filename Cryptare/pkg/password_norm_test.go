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
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Passwords for the Unicode normalisation tests (Q-009): the same passphrase with
// precomposed letters (NFC and NFKC form), and with base letters followed by combining
// accents, as some keyboards, input methods and systems produce it.
const (
	composedPassword   = "caf\u00e9 au lait, s'il vous pla\u00eet"
	decomposedPassword = "cafe\u0301 au lait, s'il vous plai\u0302t"
	bom                = "\uFEFF"
)

func TestNormalizePassword(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"ASCII is unchanged", testPassword, testPassword},
		{"precomposed letters are unchanged", composedPassword, composedPassword},
		{"combining accents are composed", decomposedPassword, composedPassword},
		{"a leading byte order mark is removed", bom + testPassword, testPassword},
		{"only one leading byte order mark is removed", bom + bom + "x", bom + "x"},
		{"a byte order mark elsewhere is kept", "a" + bom + "b", "a" + bom + "b"},
		{"full-width letters become ASCII", "\uff21\uff22\uff23", "ABC"},
		{"a ligature is split", "\ufb01", "fi"},
		{"a no-break space becomes a space", "a\u00a0b", "a b"},
		{"invalid UTF-8 is kept", "\x80\xff\xfe", "\x80\xff\xfe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePassword(tt.in); got != tt.want {
				t.Fatalf("normalizePassword(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizePasswordKnownAnswers pins the normalised form of passwords (Q-009).
// Data protected with a password that normalisation changes records kdfArgon2idNFKC
// and is read through this form only, so a golang.org/x/text update (or the Unicode
// tables that a newer Go toolchain selects) that changes it would make that data
// unreadable. If this test fails after such an update, don't just change the expected
// values: that is a format change for the owner to decide. The expected values agree
// with Python's unicodedata.normalize("NFKC"), an independent implementation.
func TestNormalizePasswordKnownAnswers(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"precomposed letter", "caf\u00e9", "caf\u00e9"},
		{"combining accent", "cafe\u0301", "caf\u00e9"},
		{"ligature", "\ufb01", "fi"},
		{"full-width letters", "\uff21\uff22\uff23", "ABC"},
		{"Roman numeral", "\u2163", "IV"},
		{"superscript", "\u00b2", "2"},
		{"angstrom sign", "\u212b", "\u00c5"},
		{"no-break space", "\u00a0", " "},
		{"Hangul jamo", "\u1100\u1161\u11a8", "\uac01"},
		{"Hangul syllable and an accent", "\uac00\u0301", "\uac00\u0301"},
		{"half-width katakana with a voiced mark", "\uff76\uff9e", "\u30ac"},
		{"Greek with two accents", "\u03c9\u0313\u0342", "\u1f66"},
		{"long s with dot above and dot below", "\u1e9b\u0323", "\u1e69"},
		{"Tamil two-part vowel sign", "\u0b95\u0bc6\u0bbe", "\u0b95\u0bca"},
		{"Devanagari composition exclusion", "\u0958", "\u0915\u093c"},
		// golang.org/x/text before v0.42.0 composed the accent with the "o" across
		// the Tamil vowel sign, which blocks it.
		{"accent after a vowel sign", "o\u0bbe\u0317\u0301", "o\u0bbe\u0317\u0301"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePassword(tt.in); got != tt.want {
				t.Fatalf("normalizePassword(%+q) = %+q, want %+q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPasswordCandidates(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		normalized bool
		want       []string
	}{
		{"ASCII", testPassword, false, []string{testPassword}},
		{"ASCII, normalised data", testPassword, true, []string{testPassword}},
		{"byte order mark", bom + testPassword, false, []string{testPassword, bom + testPassword}},
		{"byte order mark, normalised data", bom + testPassword, true, []string{testPassword}},
		{"combining accents", decomposedPassword, false, []string{decomposedPassword, composedPassword}},
		{"combining accents, normalised data", decomposedPassword, true, []string{composedPassword}},
		{"byte order mark and combining accents", bom + decomposedPassword, false,
			[]string{decomposedPassword, bom + decomposedPassword, composedPassword}},
		{"precomposed", composedPassword, false, []string{composedPassword}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passwordCandidates(tt.in, tt.normalized); !slices.Equal(got, tt.want) {
				t.Fatalf("passwordCandidates(%q, %v) = %q, want %q", tt.in, tt.normalized, got, tt.want)
			}
		})
	}
}

// TestCheckPasswordPolicyCountsNormalizedPassword checks that the policy applies to the
// password that protects the data, after normalising (Q-009).
func TestCheckPasswordPolicyCountsNormalizedPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     error
	}{
		{"a byte order mark alone is empty", bom, ErrEmptyPassword},
		{"a byte order mark isn't counted", bom + "abcdefghijklmn", ErrWeakPassword},
		// 15 code points as typed, 14 once the accent is composed.
		{"a combining accent isn't counted", "abcdefghijklme\u0301", ErrWeakPassword},
		// 14 code points as typed, 15 once the ligature is split.
		{"a ligature counts as its letters", "abcdefghijklm\ufb01", nil},
		{"full-width letters repeated", strings.Repeat("\uff41", 15) + "a", ErrWeakPassword},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPasswordPolicy(tt.password)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("CheckPasswordPolicy() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("CheckPasswordPolicy() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestUnicodePasswordFormsOpenTheSameData is a regression test for Q-009: data
// protected with a password opens when the password is entered with its characters
// composed differently, or read from a password file that starts with a byte order
// mark. It covers files, folders, stored keys and key exports.
func TestUnicodePasswordFormsOpenTheSameData(t *testing.T) {
	pairs := []struct {
		name          string
		protect, open []string
	}{
		{"combining accents, then precomposed", []string{decomposedPassword}, []string{composedPassword, decomposedPassword}},
		{"precomposed, then combining accents", []string{composedPassword}, []string{decomposedPassword, composedPassword}},
		{"password file with a byte order mark", []string{bom + testPassword}, []string{testPassword, bom + testPassword}},
		{"typed, then a password file with a byte order mark", []string{testPassword}, []string{bom + testPassword}},
	}
	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			dir := t.TempDir()
			protect := p.protect[0]

			plain := filepath.Join(dir, "plain.txt")
			if err := os.WriteFile(plain, []byte("secret"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			folder := filepath.Join(dir, "folder")
			if err := os.MkdirAll(folder, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(folder, "a.txt"), []byte("folder secret"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			if err := EncryptFile(plain, filepath.Join(dir, "plain.enc"), protect); err != nil {
				t.Fatalf("EncryptFile(file): %v", err)
			}
			if err := EncryptFile(folder, filepath.Join(dir, "folder.enc"), protect); err != nil {
				t.Fatalf("EncryptFile(folder): %v", err)
			}
			rawKey, err := GenerateKey()
			if err != nil {
				t.Fatalf("GenerateKey: %v", err)
			}
			blob, err := EncryptKeyBlob(rawKey, protect)
			if err != nil {
				t.Fatalf("EncryptKeyBlob: %v", err)
			}
			export := filepath.Join(dir, "key.ckey")
			// The export is protected by the key's own master password (BUG-011).
			km := &KeyModel{KeyID: "0123456789abcdef", Algorithm: keyAlgorithm, EncryptedBlob: blob, CreatedAt_: 1}
			if err := ExportKeyToFile(km, protect, export); err != nil {
				t.Fatalf("ExportKeyToFile: %v", err)
			}

			if err := os.MkdirAll(filepath.Join(dir, "out"), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			for i, open := range p.open {
				out := filepath.Join(dir, "out", string(rune('a'+i)))
				if err := DecryptFile(filepath.Join(dir, "plain.enc"), out+".txt", open); err != nil {
					t.Fatalf("DecryptFile(file, %q): %v", open, err)
				}
				if got, _ := os.ReadFile(out + ".txt"); string(got) != "secret" {
					t.Fatalf("decrypted file = %q", got)
				}
				if err := DecryptFile(filepath.Join(dir, "folder.enc"), out, open); err != nil {
					t.Fatalf("DecryptFile(folder, %q): %v", open, err)
				}
				if got, _ := os.ReadFile(filepath.Join(out, "a.txt")); string(got) != "folder secret" {
					t.Fatalf("decrypted folder file = %q", got)
				}
				if got, err := DecryptKeyBlob(blob, open); err != nil || !bytes.Equal(got, rawKey) {
					t.Fatalf("DecryptKeyBlob(%q) = %x, %v; want the key", open, got, err)
				}
				if imported, err := ImportKeyFromFile(export, open); err != nil || imported.KeyID != km.KeyID {
					t.Fatalf("ImportKeyFromFile(%q) = %+v, %v", open, imported, err)
				}
			}

			// Another password still fails, and writes nothing.
			wrong := filepath.Join(dir, "wrong.txt")
			if err := DecryptFile(filepath.Join(dir, "plain.enc"), wrong, "a different password"); err == nil {
				t.Fatal("DecryptFile with another password succeeded")
			}
			if _, err := os.Stat(wrong); !os.IsNotExist(err) {
				t.Fatalf("output written with a wrong password (stat err: %v)", err)
			}
		})
	}
}

// TestNormalizedPasswordHeader checks which key derivation identifier new data records
// (Q-009): kdfArgon2id when normalising doesn't change the password, so earlier versions
// can read the data, and kdfArgon2idNFKC when it does.
func TestNormalizedPasswordHeader(t *testing.T) {
	tests := []struct {
		password string
		want     byte
	}{
		{testPassword, kdfArgon2id},
		{composedPassword, kdfArgon2id},
		{decomposedPassword, kdfArgon2idNFKC},
		{bom + testPassword, kdfArgon2idNFKC},
	}
	for _, tt := range tests {
		for _, content := range []byte{contentFile, contentStoredKey} {
			sealed, err := sealV2([]byte("x"), tt.password, content)
			if err != nil {
				t.Fatalf("sealV2: %v", err)
			}
			if sealed[12] != tt.want {
				t.Errorf("password %q, %s: key derivation %d, want %d", tt.password, contentName(content), sealed[12], tt.want)
			}
		}
	}
	blob, err := EncryptKeyBlob(make([]byte, keyLen), decomposedPassword)
	if err != nil {
		t.Fatalf("EncryptKeyBlob: %v", err)
	}
	if data, _ := base64.StdEncoding.DecodeString(blob); data[12] != kdfArgon2idNFKC {
		t.Errorf("stored key: key derivation %d, want %d", data[12], kdfArgon2idNFKC)
	}
}

// sealAsEarlierVersion encrypts plaintext the way versions before Q-009 did: a
// kdfArgon2id header, and a key derived from the password exactly as given.
func sealAsEarlierVersion(t *testing.T, plaintext []byte, password string, content byte) []byte {
	t.Helper()
	h, err := newV2Header(content)
	if err != nil {
		t.Fatalf("newV2Header: %v", err)
	}
	aead, err := h.passwordAEAD(password)
	if err != nil {
		t.Fatalf("aead: %v", err)
	}
	var buf bytes.Buffer
	aad := h.marshal()
	buf.Write(aad)
	w := &encryptingWriter{
		w: &buf, h: h, aad: aad, aead: aead,
		buf: make([]byte, 0, h.chunkSize()), out: make([]byte, 0, h.chunkSize()+gcmTagLen), nonce: make([]byte, 0, 12),
	}
	if _, err := w.Write(plaintext); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// TestEarlierDataOpensWithPasswordForms checks that data written before Q-009, in the
// version 2 and legacy formats, still opens with the password as it was entered then,
// with a password file's byte order mark, and with the password entered in another
// form when it was protected in normalised form.
func TestEarlierDataOpensWithPasswordForms(t *testing.T) {
	tests := []struct {
		name, protect, open string
	}{
		{"same combining accents", decomposedPassword, decomposedPassword},
		{"precomposed, then combining accents", composedPassword, decomposedPassword},
		{"same password file with a byte order mark", bom + testPassword, bom + testPassword},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			v2 := filepath.Join(dir, "v2.enc")
			if err := os.WriteFile(v2, sealAsEarlierVersion(t, []byte("v2 secret"), tt.protect, contentFile), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			legacyData, err := encryptBytes([]byte("legacy secret"), tt.protect)
			if err != nil {
				t.Fatalf("encryptBytes: %v", err)
			}
			legacy := filepath.Join(dir, "legacy.enc")
			if err := os.WriteFile(legacy, legacyData, 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			for src, want := range map[string]string{v2: "v2 secret", legacy: "legacy secret"} {
				out := src + ".out"
				if err := DecryptFile(src, out, tt.open); err != nil {
					t.Fatalf("DecryptFile(%s, %q): %v", filepath.Base(src), tt.open, err)
				}
				if got, _ := os.ReadFile(out); string(got) != want {
					t.Fatalf("%s decrypted to %q, want %q", filepath.Base(src), got, want)
				}
			}
			blob := base64.StdEncoding.EncodeToString(sealAsEarlierVersion(t, []byte("key"), tt.protect, contentStoredKey))
			if got, err := DecryptKeyBlob(blob, tt.open); err != nil || string(got) != "key" {
				t.Fatalf("DecryptKeyBlob = %q, %v", got, err)
			}
		})
	}
}
