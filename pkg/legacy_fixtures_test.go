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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"os"
	"testing"
)

// Writers for the legacy (version 1) formats. Cryptare no longer writes them, but it
// must keep reading them, so tests build legacy data with these.

// encryptBytes seals plaintext in the legacy single-file layout: salt ‖ nonce ‖
// AES-256-GCM ciphertext, with a PBKDF2 key.
func encryptBytes(plaintext []byte, password string) ([]byte, error) {
	return encryptBytesWithAAD(plaintext, password, nil)
}

func encryptBytesWithAAD(plaintext []byte, password string, aad []byte) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	iv := make([]byte, ivLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := append(append(salt, iv...), gcm.Seal(nil, iv, plaintext, aad)...)
	return out, nil
}

// encryptLegacyDirectory writes the directory tree at src to dst as a legacy folder
// artifact: the magic, then the tar.gz sealed with the magic as additional data.
func encryptLegacyDirectory(t *testing.T, src, dst, password string) {
	t.Helper()
	var archive bytes.Buffer
	if err := writeDirectoryArchive(context.Background(), &archive, src); err != nil {
		t.Fatalf("build legacy archive: %v", err)
	}
	sealed, err := encryptBytesWithAAD(archive.Bytes(), password, []byte(directoryArtifactMagicV1))
	if err != nil {
		t.Fatalf("seal legacy archive: %v", err)
	}
	if err := os.WriteFile(dst, append([]byte(directoryArtifactMagicV1), sealed...), 0o600); err != nil {
		t.Fatalf("write legacy artifact: %v", err)
	}
}

// TestMain lowers the Argon2id setting for new data so the suite stays fast; the setting
// is recorded in each header, so decryption doesn't depend on it.
// TestDefaultPasswordKDFIsWritten checks the real default.
func TestMain(m *testing.M) {
	passwordKDF = argon2Params{memoryKiB: 64, iterations: 1, threads: 1}
	os.Exit(m.Run())
}
