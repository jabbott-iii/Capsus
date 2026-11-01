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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDeriveKey checks the legacy PBKDF2-HMAC-SHA256 key derivation (100,000
// iterations) against known answers computed independently (Python's
// hashlib.pbkdf2_hmac), so that switching to the standard library's crypto/pbkdf2 keeps
// legacy files readable.
func TestDeriveKey(t *testing.T) {
	tests := []struct {
		name     string
		password string
		salt     []byte
		wantHex  string
	}{
		{
			name:     "basic derivation",
			password: "testpassword",
			salt:     []byte("1234567890123456"),
			wantHex:  "31a0d366a7956779e761485eef16ec05c4f89365bea1439eda37f9f8e1a2aa1d",
		},
		{
			name:     "empty password",
			password: "",
			salt:     []byte("1234567890123456"),
			wantHex:  "f24f8a7c580566fa4bf907ecadc7aacb1c48992484ec4e7e3e8b06e359b73bbe",
		},
		{
			name:     "unicode password",
			password: "pässwörd🔐",
			salt:     []byte("1234567890123456"),
			wantHex:  "ca55a6916bf56a474bd97f2b39e49ae2ff84352f410cf331b4a6240b7d001f71",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := deriveKey(tt.password, tt.salt)
			if err != nil {
				t.Fatalf("deriveKey() error = %v", err)
			}
			if got := hex.EncodeToString(key); got != tt.wantHex {
				t.Errorf("deriveKey() = %s, want %s", got, tt.wantHex)
			}
		})
	}
}

// TestEncryptDecryptFile tests the EncryptFile and DecryptFile functions by encrypting and decrypting a file with various passwords and output paths.
// It ensures that the encrypted file differs from the original and that decryption restores the original content accurately.
func TestEncryptDecryptFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("Hello, World! This is a test file.")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	tests := []struct {
		name     string
		password string
		dstPath  string
	}{
		{
			name:     "encrypt with default output",
			password: testPassword,
			dstPath:  "",
		},
		{
			name:     "encrypt with custom output",
			password: "another correct horse battery staple",
			dstPath:  filepath.Join(tmpDir, "custom.enc"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encFile := tt.dstPath
			if encFile == "" {
				encFile = srcFile + encExt
			}

			// Encrypt
			if err := EncryptFile(srcFile, encFile, tt.password); err != nil {
				t.Fatalf("EncryptFile failed: %v", err)
			}

			// Verify encrypted file exists and is different
			encData, err := os.ReadFile(encFile)
			if err != nil {
				t.Fatalf("Failed to read encrypted file: %v", err)
			}
			if string(encData) == string(originalContent) {
				t.Error("Encrypted file matches original content")
			}

			// Decrypt
			decFile := filepath.Join(tmpDir, "decrypted.txt")
			if err := DecryptFile(encFile, decFile, tt.password); err != nil {
				t.Fatalf("DecryptFile failed: %v", err)
			}

			// Verify decrypted content matches original
			decData, err := os.ReadFile(decFile)
			if err != nil {
				t.Fatalf("Failed to read decrypted file: %v", err)
			}
			if string(decData) != string(originalContent) {
				t.Errorf("Decrypted content mismatch: got %q, want %q", string(decData), string(originalContent))
			}
		})
	}
}

// TestEncryptDecryptDirectory verifies directory encryption, produces one
// encrypted artifact, and decrypts back into the original directory structure.
func TestEncryptDecryptDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "empty"), 0o755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root data"), 0o644); err != nil {
		t.Fatalf("Failed to write root file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "child.txt"), []byte("nested data"), 0o640); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	encFile := srcDir + encExt
	if err := EncryptFile(srcDir, "", testPassword); err != nil {
		t.Fatalf("EncryptFile failed: %v", err)
	}

	info, err := os.Stat(encFile)
	if err != nil {
		t.Fatalf("Encrypted artifact not found: %v", err)
	}
	if info.IsDir() {
		t.Fatal("Encrypted artifact should be a single file")
	}

	restoreDir := filepath.Join(tmpDir, "restored")
	if err := DecryptFile(encFile, restoreDir, testPassword); err != nil {
		t.Fatalf("DecryptFile failed: %v", err)
	}

	rootData, err := os.ReadFile(filepath.Join(restoreDir, "root.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored root file: %v", err)
	}
	if string(rootData) != "root data" {
		t.Fatalf("Root file mismatch: got %q", string(rootData))
	}

	nestedData, err := os.ReadFile(filepath.Join(restoreDir, "nested", "child.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored nested file: %v", err)
	}
	if string(nestedData) != "nested data" {
		t.Fatalf("Nested file mismatch: got %q", string(nestedData))
	}
	if info, err := os.Stat(filepath.Join(restoreDir, "nested", "child.txt")); err != nil {
		t.Fatalf("Failed to stat restored nested file: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		// Restored files are owner-only (SEC-008), whatever mode they had (0640 here).
		t.Fatalf("Nested file mode mismatch: got %o, want %o", info.Mode().Perm(), 0o600)
	}

	if info, err := os.Stat(filepath.Join(restoreDir, "empty")); err != nil {
		t.Fatalf("Expected empty directory not restored: %v", err)
	} else if !info.IsDir() {
		t.Fatal("Restored empty path is not a directory")
	}
}

// TestEncryptDecryptEmptyDirectory ensures an empty directory can be restored from a single encrypted artifact.
func TestEncryptDecryptEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	emptyDir := filepath.Join(tmpDir, "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}

	encFile := emptyDir + encExt
	if err := EncryptFile(emptyDir, "", testPassword); err != nil {
		t.Fatalf("EncryptFile failed: %v", err)
	}
	if _, err := os.Stat(encFile); err != nil {
		t.Fatalf("Encrypted artifact not found: %v", err)
	}

	restoreDir := filepath.Join(tmpDir, "restored-empty")
	if err := DecryptFile(encFile, restoreDir, testPassword); err != nil {
		t.Fatalf("DecryptFile failed: %v", err)
	}

	if info, err := os.Stat(restoreDir); err != nil {
		t.Fatalf("Restored directory not found: %v", err)
	} else if !info.IsDir() {
		t.Fatal("Restored empty path is not a directory")
	}
}

// TestEncryptDirectoryWithSymlink ensures nested symlinks are rejected.
func TestEncryptDirectoryWithSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	linkPath := filepath.Join(srcDir, "linked.txt")
	if err := os.Symlink(filepath.Join(srcDir, "root.txt"), linkPath); err != nil {
		t.Skipf("Symlinks are unavailable in this environment: %v", err)
	}

	err := EncryptFile(srcDir, "", testPassword)
	if err == nil {
		t.Fatal("EncryptFile should fail when the directory contains a symlink")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("Expected symlink error, got %v", err)
	}
}

// TestEncryptSymlinkedDirectoryPath ensures symlinked source directories are rejected.
func TestEncryptSymlinkedDirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("Failed to write target file: %v", err)
	}

	linkDir := filepath.Join(tmpDir, "linked-target")
	if err := os.Symlink(targetDir, linkDir); err != nil {
		t.Skipf("Symlinks are unavailable in this environment: %v", err)
	}

	err := EncryptFile(linkDir, "", testPassword)
	if err == nil {
		t.Fatal("EncryptFile should fail when the source directory path is a symlink")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("Expected symlink error, got %v", err)
	}
}

// TestDecryptLegacyEncryptedFile verifies backward compatibility with legacy file ciphertext layout.
func TestDecryptLegacyEncryptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalContent := []byte("legacy encrypted file data")

	encFile := filepath.Join(tmpDir, "legacy.txt.enc")
	legacyCiphertext, err := encryptBytes(originalContent, "testpassword")
	if err != nil {
		t.Fatalf("encryptBytes failed: %v", err)
	}
	if err := os.WriteFile(encFile, legacyCiphertext, 0o600); err != nil {
		t.Fatalf("Failed to write legacy ciphertext: %v", err)
	}

	decFile := filepath.Join(tmpDir, "legacy.txt.dec")
	if err := DecryptFile(encFile, decFile, "testpassword"); err != nil {
		t.Fatalf("DecryptFile failed: %v", err)
	}

	decData, err := os.ReadFile(decFile)
	if err != nil {
		t.Fatalf("Failed to read decrypted file: %v", err)
	}
	if string(decData) != string(originalContent) {
		t.Fatalf("Decrypted content mismatch: got %q, want %q", string(decData), string(originalContent))
	}
}

// TestDecryptWithWrongPassword tests that attempting to decrypt a file with an incorrect password fails as expected.
// It ensures that the decryption process returns an error and does not produce the original content.
func TestDecryptWithWrongPassword(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("Secret data")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	encFile := filepath.Join(tmpDir, "test.enc")
	if err := EncryptFile(srcFile, encFile, testPassword); err != nil {
		t.Fatalf("EncryptFile failed: %v", err)
	}

	decFile := filepath.Join(tmpDir, "decrypted.txt")
	err := DecryptFile(encFile, decFile, "wrongpassword")
	if err == nil {
		t.Error("DecryptFile should have failed with wrong password")
	}
}

// TestGenerateKey tests the GenerateKey function to ensure it produces keys of the expected length and that consecutive calls produce different keys.
// It verifies that the generated keys have the correct length and that multiple calls produce unique keys.
func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name    string
		wantLen int
	}{
		{
			name:    "generate valid key",
			wantLen: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := GenerateKey()
			if err != nil {
				t.Fatalf("GenerateKey failed: %v", err)
			}
			if len(key) != tt.wantLen {
				t.Errorf("GenerateKey() length = %d, want %d", len(key), tt.wantLen)
			}

			// Keys should be random
			key2, err := GenerateKey()
			if err != nil {
				t.Fatalf("GenerateKey failed: %v", err)
			}
			if string(key) == string(key2) {
				t.Error("GenerateKey() produced identical keys")
			}
		})
	}
}

func TestEncryptDecryptKeyBlob(t *testing.T) {
	tests := []struct {
		name       string
		rawKey     []byte
		masterPass string
	}{
		{
			name:       "basic key encryption",
			rawKey:     []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			masterPass: "master passphrase 123",
		},
		{
			name:       "32-byte key",
			rawKey:     make([]byte, 32),
			masterPass: "another master passphrase",
		},
		{
			name:       "unicode master password",
			rawKey:     []byte("some_key_data"),
			masterPass: "pässwörd🔐-mit-Ümlauten",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			blob, err := EncryptKeyBlob(tt.rawKey, tt.masterPass)
			if err != nil {
				t.Fatalf("EncryptKeyBlob failed: %v", err)
			}
			if blob == "" {
				t.Error("EncryptKeyBlob returned empty string")
			}

			// Decrypt
			decrypted, err := DecryptKeyBlob(blob, tt.masterPass)
			if err != nil {
				t.Fatalf("DecryptKeyBlob failed: %v", err)
			}
			if string(decrypted) != string(tt.rawKey) {
				t.Errorf("Decrypted key mismatch: got %v, want %v", decrypted, tt.rawKey)
			}
		})
	}
}

// TestDecryptKeyBlobWithWrongPassword tests that attempting to decrypt an encrypted key blob with an incorrect password fails as expected.
// It ensures that the decryption process returns an error and does not produce the original key.
func TestDecryptKeyBlobWithWrongPassword(t *testing.T) {
	rawKey := []byte{1, 2, 3, 4, 5}
	masterPass := testPassword

	blob, err := EncryptKeyBlob(rawKey, masterPass)
	if err != nil {
		t.Fatalf("EncryptKeyBlob failed: %v", err)
	}

	_, err = DecryptKeyBlob(blob, "wrongpass")
	if err == nil {
		t.Error("DecryptKeyBlob should have failed with wrong password")
	}
}

// TestEncryptDecryptKeyBlobRandomness tests that encrypting the same key blob multiple times with the same password produces different encrypted outputs.
// This ensures that the encryption process uses random IVs and salts to enhance security.
func TestEncryptDecryptKeyBlobRandomness(t *testing.T) {
	rawKey := []byte{1, 2, 3}
	masterPass := testPassword

	blob1, err := EncryptKeyBlob(rawKey, masterPass)
	if err != nil {
		t.Fatalf("EncryptKeyBlob failed: %v", err)
	}

	blob2, err := EncryptKeyBlob(rawKey, masterPass)
	if err != nil {
		t.Fatalf("EncryptKeyBlob failed: %v", err)
	}

	// Blobs should be different due to random IV and salt
	if blob1 == blob2 {
		t.Error("EncryptKeyBlob produced identical blobs for same input")
	}
}

// TestNewKeyID tests the newKeyID function to ensure it generates unique IDs of the expected length.
// It verifies that consecutive calls produce different IDs and that the length of the generated ID matches the expected value.
func TestNewKeyID(t *testing.T) {
	id1, err := newKeyID()
	if err != nil {
		t.Fatalf("newKeyID failed: %v", err)
	}
	if len(id1) != 16 { // 8 bytes in hex
		t.Errorf("newKeyID length = %d, want 16", len(id1))
	}

	id2, err := newKeyID()
	if err != nil {
		t.Fatalf("newKeyID failed: %v", err)
	}

	// IDs should be unique
	if id1 == id2 {
		t.Error("newKeyID generated identical IDs")
	}
}

// TestEncryptRejectsEmptyPassword is a regression test for SEC-001: every encryption
// path must refuse an empty password and write nothing.
func TestEncryptRejectsEmptyPassword(t *testing.T) {
	tmpDir := t.TempDir()

	srcFile := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(srcFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	srcDir := filepath.Join(tmpDir, "secret-dir")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "inner.txt"), []byte("inner"), 0o600); err != nil {
		t.Fatalf("write inner file: %v", err)
	}

	for _, src := range []string{srcFile, srcDir} {
		dst := src + encExt
		if err := EncryptFile(src, dst, ""); !errors.Is(err, ErrEmptyPassword) {
			t.Errorf("EncryptFile(%s) error = %v, want ErrEmptyPassword", src, err)
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Errorf("EncryptFile(%s) created %s despite the empty password", src, dst)
		}
	}

	rawKey, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if _, err := EncryptKeyBlob(rawKey, ""); !errors.Is(err, ErrEmptyPassword) {
		t.Errorf("EncryptKeyBlob error = %v, want ErrEmptyPassword", err)
	}

	// A key stored by an earlier version under this password: the export would be
	// protected by the same password (BUG-011), so it is refused.
	legacyBlob, err := encryptBytes(rawKey, "")
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	exportPath := filepath.Join(tmpDir, "key.ckey")
	km := &KeyModel{KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", EncryptedBlob: base64.StdEncoding.EncodeToString(legacyBlob), CreatedAt_: 1}
	if err := ExportKeyToFile(km, "", exportPath); !errors.Is(err, ErrEmptyPassword) {
		t.Errorf("ExportKeyToFile error = %v, want ErrEmptyPassword", err)
	}
	if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
		t.Errorf("ExportKeyToFile created %s despite the empty password", exportPath)
	}
}

// TestDecryptAcceptsLegacyEmptyPassword checks that files, directory archives and key
// blobs encrypted with an empty password before SEC-001 was fixed still decrypt.
func TestDecryptAcceptsLegacyEmptyPassword(t *testing.T) {
	tmpDir := t.TempDir()

	// Single file, written the way pre-fix builds did.
	plaintext := []byte("legacy empty-password data")
	ciphertext, err := encryptBytes(plaintext, "")
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	encFile := filepath.Join(tmpDir, "legacy.txt.enc")
	if err := os.WriteFile(encFile, ciphertext, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	decFile := filepath.Join(tmpDir, "legacy.txt")
	if err := DecryptFile(encFile, decFile, ""); err != nil {
		t.Fatalf("DecryptFile(legacy file) error = %v", err)
	}
	if got, err := os.ReadFile(decFile); err != nil || string(got) != string(plaintext) {
		t.Fatalf("decrypted content = %q (err %v), want %q", got, err, plaintext)
	}

	// Directory archive.
	srcDir := filepath.Join(tmpDir, "legacy-dir")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("create legacy directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "inner.txt"), []byte("inner"), 0o600); err != nil {
		t.Fatalf("write inner file: %v", err)
	}
	dirArtifact := filepath.Join(tmpDir, "legacy-dir.enc")
	encryptLegacyDirectory(t, srcDir, dirArtifact, "")
	restored := filepath.Join(tmpDir, "restored")
	if err := DecryptFile(dirArtifact, restored, ""); err != nil {
		t.Fatalf("DecryptFile(legacy directory) error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(restored, "inner.txt")); err != nil || string(got) != "inner" {
		t.Fatalf("restored inner.txt = %q (err %v), want %q", got, err, "inner")
	}

	// Key blob (same salt ‖ nonce ‖ ciphertext layout as EncryptKeyBlob).
	rawKey := []byte("0123456789abcdef0123456789abcdef")
	blobBytes, err := encryptBytes(rawKey, "")
	if err != nil {
		t.Fatalf("encryptBytes(key): %v", err)
	}
	gotKey, err := DecryptKeyBlob(base64.StdEncoding.EncodeToString(blobBytes), "")
	if err != nil {
		t.Fatalf("DecryptKeyBlob(legacy blob) error = %v", err)
	}
	if string(gotKey) != string(rawKey) {
		t.Fatalf("DecryptKeyBlob = %q, want %q", gotKey, rawKey)
	}
}

// testPassword meets the password policy; tests use it wherever a password protects
// new data.
const testPassword = "correct horse battery staple"

// TestCheckPasswordPolicy is a regression test for SEC-001: passwords that protect new
// data must be at least MinPasswordLength Unicode code points long and must not be a
// single repeated character. There are no composition rules.
func TestCheckPasswordPolicy(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     error
	}{
		{name: "empty", password: "", want: ErrEmptyPassword},
		{name: "short", password: "hunter2", want: ErrWeakPassword},
		{name: "one character short", password: "abcdefghijklmn", want: ErrWeakPassword},
		{name: "minimum length", password: "abcdefghijklmno"},
		{name: "passphrase with spaces", password: testPassword},
		{name: "lower-case letters only", password: "correcthorsebatterystaple"},
		{name: "one repeated character", password: "aaaaaaaaaaaaaaaaaaaa", want: ErrWeakPassword},
		{name: "only spaces", password: strings.Repeat(" ", 20), want: ErrWeakPassword},
		// 14 code points but 28 bytes: length is counted in code points, not bytes.
		{name: "multi-byte one character short", password: "äöüßéèêëïîôûçñ", want: ErrWeakPassword},
		{name: "multi-byte minimum length", password: "äöüßéèêëïîôûçñå"},
		{name: "repeated multi-byte character", password: strings.Repeat("🔐", 20), want: ErrWeakPassword},
		// Bytes that aren't valid UTF-8 count as one character each and are compared as bytes.
		{name: "distinct invalid UTF-8 bytes", password: "\x80\x81\x82\x83\x84\x85\x86\x87\x88\x89\x8a\x8b\x8c\x8d\x8e"},
		{name: "repeated invalid UTF-8 byte", password: strings.Repeat("\xff", 15), want: ErrWeakPassword},
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

	if err := CheckPasswordPolicy("hunter2"); err == nil || !strings.Contains(err.Error(), "at least 15 characters") {
		t.Fatalf("short password error = %v, want it to state the minimum length", err)
	}
}

// TestEncryptRejectsWeakPassword is a regression test for SEC-001: every operation that
// protects new data enforces the password policy and writes nothing when it fails.
func TestEncryptRejectsWeakPassword(t *testing.T) {
	tmpDir := t.TempDir()

	srcFile := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(srcFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	srcDir := filepath.Join(tmpDir, "secret-dir")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "inner.txt"), []byte("inner"), 0o600); err != nil {
		t.Fatalf("write inner file: %v", err)
	}

	for _, src := range []string{srcFile, srcDir} {
		dst := src + encExt
		if err := EncryptFile(src, dst, "hunter2"); !errors.Is(err, ErrWeakPassword) {
			t.Errorf("EncryptFile(%s) error = %v, want ErrWeakPassword", src, err)
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Errorf("EncryptFile(%s) created %s despite the weak password", src, dst)
		}
	}

	rawKey, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if _, err := EncryptKeyBlob(rawKey, "hunter2"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("EncryptKeyBlob error = %v, want ErrWeakPassword", err)
	}

	// A key stored by an earlier version under this password: the export would be
	// protected by the same password (BUG-011), so it is refused.
	legacyBlob, err := encryptBytes(rawKey, "hunter2")
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	exportPath := filepath.Join(tmpDir, "key.ckey")
	km := &KeyModel{KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", EncryptedBlob: base64.StdEncoding.EncodeToString(legacyBlob), CreatedAt_: 1}
	if err := ExportKeyToFile(km, "hunter2", exportPath); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("ExportKeyToFile error = %v, want ErrWeakPassword", err)
	}
	if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
		t.Errorf("ExportKeyToFile created %s despite the weak password", exportPath)
	}
}

// TestDecryptAcceptsLegacyShortPassword checks that data protected with a password
// shorter than the policy minimum, before the policy existed, stays readable: files,
// stored key blobs and key exports.
func TestDecryptAcceptsLegacyShortPassword(t *testing.T) {
	tmpDir := t.TempDir()
	const legacyPassword = "hunter2"

	ciphertext, err := encryptBytes([]byte("legacy data"), legacyPassword)
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	encFile := filepath.Join(tmpDir, "legacy.txt.enc")
	if err := os.WriteFile(encFile, ciphertext, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	decFile := filepath.Join(tmpDir, "legacy.txt")
	if err := DecryptFile(encFile, decFile, legacyPassword); err != nil {
		t.Fatalf("DecryptFile(legacy file) error = %v", err)
	}
	if got, err := os.ReadFile(decFile); err != nil || string(got) != "legacy data" {
		t.Fatalf("decrypted content = %q (err %v), want %q", got, err, "legacy data")
	}

	// A stored key blob created with a short master password.
	rawKey := []byte("0123456789abcdef0123456789abcdef")
	blobBytes, err := encryptBytes(rawKey, legacyPassword)
	if err != nil {
		t.Fatalf("encryptBytes(key): %v", err)
	}
	storedBlob := base64.StdEncoding.EncodeToString(blobBytes)
	if got, err := DecryptKeyBlob(storedBlob, legacyPassword); err != nil || string(got) != string(rawKey) {
		t.Fatalf("DecryptKeyBlob(legacy blob) = %q (err %v), want the raw key", got, err)
	}

	// A key export created with a short export password.
	envelope := []byte(`{"version":1,"key_id":"0123456789abcdef","algorithm":"AES-256-GCM","created_at":1,"encrypted_blob":"` + storedBlob + `"}`)
	exportBytes, err := encryptBytes(envelope, legacyPassword)
	if err != nil {
		t.Fatalf("encryptBytes(export): %v", err)
	}
	exportPath := filepath.Join(tmpDir, "legacy.ckey")
	if err := os.WriteFile(exportPath, []byte(base64.StdEncoding.EncodeToString(exportBytes)), 0o600); err != nil {
		t.Fatalf("write legacy export: %v", err)
	}
	km, err := ImportKeyFromFile(exportPath, legacyPassword)
	if err != nil {
		t.Fatalf("ImportKeyFromFile(legacy export) error = %v", err)
	}
	if km.KeyID != "0123456789abcdef" || km.EncryptedBlob != storedBlob {
		t.Fatalf("imported key = %+v, want the legacy key", km)
	}
}

// TestEncryptDirectoryWritesNoTempPlaintext is a regression test for SEC-006: encrypting
// a directory must not stage a plaintext archive in the system temp directory. With
// the temp directory unusable, encryption still succeeds and round-trips.
func TestEncryptDirectoryWritesNoTempPlaintext(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "secret-dir")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "secret.txt"), []byte("top secret"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	// Point every platform's temp-directory variable at a path that doesn't exist, so
	// any attempt to create a temporary file there fails.
	missing := filepath.Join(tmpDir, "no-such-temp-dir")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missing)
	}

	enc := filepath.Join(tmpDir, "secret-dir.enc")
	if err := EncryptFile(srcDir, enc, testPassword); err != nil {
		t.Fatalf("EncryptFile(directory) with an unusable temp directory: %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("temp directory was created (stat err: %v)", err)
	}

	restored := filepath.Join(tmpDir, "restored")
	if err := DecryptFile(enc, restored, testPassword); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(restored, "nested", "secret.txt")); err != nil || string(got) != "top secret" {
		t.Fatalf("restored content = %q (err %v), want %q", got, err, "top secret")
	}
}

// validStoredBlob returns an encrypted key blob with the shape keys generate stores.
func validStoredBlob(t *testing.T) string {
	t.Helper()
	rawKey, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	blob, err := EncryptKeyBlob(rawKey, testPassword)
	if err != nil {
		t.Fatalf("EncryptKeyBlob: %v", err)
	}
	return blob
}

// writeTestExport writes export to path as a .ckey file protected with testPassword,
// exactly as given, so tests can craft exports with bad fields.
func writeTestExport(t *testing.T, path string, export KeyExport) {
	t.Helper()
	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	sealed, err := sealV2(raw, testPassword, contentKeyExport)
	if err != nil {
		t.Fatalf("encrypt export: %v", err)
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(sealed)), 0o600); err != nil {
		t.Fatalf("write export: %v", err)
	}
}

// TestImportKeyRejectsInvalidMetadata is a regression test for SEC-011: an export whose
// key ID, algorithm, blob or version isn't what Cryptare writes is refused with
// ErrInvalidKeyExport, and the error never echoes control characters to the terminal.
func TestImportKeyRejectsInvalidMetadata(t *testing.T) {
	dir := t.TempDir()
	good := KeyExport{Version: 1, KeyID: "0123456789abcdef", Algorithm: "AES-256-GCM", CreatedAt: 1_700_000_000, EncryptedBlob: validStoredBlob(t)}

	path := filepath.Join(dir, "good.ckey")
	writeTestExport(t, path, good)
	km, err := ImportKeyFromFile(path, testPassword)
	if err != nil {
		t.Fatalf("valid export refused: %v", err)
	}
	if km.KeyID != good.KeyID || km.Algorithm != good.Algorithm || km.EncryptedBlob != good.EncryptedBlob || km.CreatedAt_ != good.CreatedAt {
		t.Fatalf("imported %+v, want the fields of %+v", km, good)
	}

	shortBlob := base64.StdEncoding.EncodeToString(make([]byte, 20))
	tests := []struct {
		name   string
		modify func(e *KeyExport)
	}{
		{"terminal escape in key ID", func(e *KeyExport) { e.KeyID = "\x1b]0;pwned\x07\x1b[2J0123" }},
		{"key ID too short", func(e *KeyExport) { e.KeyID = "0123abcd" }},
		{"key ID upper-case", func(e *KeyExport) { e.KeyID = "0123456789ABCDEF" }},
		{"key ID not hex", func(e *KeyExport) { e.KeyID = "0123456789abcdeg" }},
		{"empty key ID", func(e *KeyExport) { e.KeyID = "" }},
		{"unknown algorithm", func(e *KeyExport) { e.Algorithm = "AES-128-CBC" }},
		{"terminal escape in algorithm", func(e *KeyExport) { e.Algorithm = "AES-256-GCM\x1b[31m" }},
		{"blob not base64", func(e *KeyExport) { e.EncryptedBlob = "not base64!" }},
		{"blob wrong length", func(e *KeyExport) { e.EncryptedBlob = shortBlob }},
		{"empty blob", func(e *KeyExport) { e.EncryptedBlob = "" }},
		{"unsupported version", func(e *KeyExport) { e.Version = 2 }},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			export := good
			tt.modify(&export)
			path := filepath.Join(dir, fmt.Sprintf("bad%d.ckey", i))
			writeTestExport(t, path, export)

			km, err := ImportKeyFromFile(path, testPassword)
			if !errors.Is(err, ErrInvalidKeyExport) {
				t.Fatalf("ImportKeyFromFile() = %+v, %v; want ErrInvalidKeyExport", km, err)
			}
			if strings.ContainsAny(err.Error(), "\x1b\x07") {
				t.Fatalf("error message contains raw control characters: %q", err.Error())
			}
		})
	}
}

// TestEncryptFolderOutputStaysOutside is the regression test for BUG-016: the default
// output for "dir/" and for "." is written next to the folder, and an explicit output
// inside the folder is refused without writing anything there.
func TestEncryptFolderOutputStaysOutside(t *testing.T) {
	parent := t.TempDir()
	folder := filepath.Join(parent, "secret")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "a.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	onlyOriginal := func(dir string) {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		if len(entries) != 1 || entries[0].Name() != "a.txt" {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Fatalf("%s holds %v, want only a.txt", dir, names)
		}
	}
	encrypted := folder + encExt

	// "secret/", as shell tab completion types it.
	if err := EncryptFile(folder+string(filepath.Separator), "", testPassword); err != nil {
		t.Fatalf("encrypt secret/: %v", err)
	}
	if _, err := os.Stat(encrypted); err != nil {
		t.Fatalf("secret.enc wasn't written next to the folder: %v", err)
	}
	onlyOriginal(folder)
	restored := filepath.Join(parent, "restored")
	if err := DecryptFile(encrypted, restored, testPassword); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	onlyOriginal(restored)

	// ".", from inside the folder.
	if err := os.Remove(encrypted); err != nil {
		t.Fatalf("remove: %v", err)
	}
	t.Chdir(folder)
	if err := EncryptFile(".", "", testPassword); err != nil {
		t.Fatalf("encrypt .: %v", err)
	}
	if _, err := os.Stat(encrypted); err != nil {
		t.Fatalf("<parent>/secret.enc wasn't written: %v", err)
	}
	onlyOriginal(folder)

	// An explicit output inside the folder.
	if err := EncryptFile(folder, filepath.Join(folder, "inside.enc"), testPassword); !errors.Is(err, ErrOutputInsideInput) {
		t.Fatalf("encrypt into the folder: err = %v, want ErrOutputInsideInput", err)
	}
	onlyOriginal(folder)
}
