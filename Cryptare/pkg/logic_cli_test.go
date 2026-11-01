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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestNewRootCmd tests the NewRootCmd function to ensure it returns a valid root command.
// It verifies that the command is not nil and has the expected use string.
func TestNewRootCmd(t *testing.T) {
	db := newTestDatabase(t, false)

	cmd := NewRootCmd(db)
	if cmd.Use != "cryptare" {
		t.Errorf("Root command Use = %q, want cryptare", cmd.Use)
	}

	flag := cmd.Flags().Lookup("vim")
	if flag == nil {
		t.Fatal("expected --vim flag to be registered on root command")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--vim default = %q, want false", flag.DefValue)
	}
}

// TestDeriveDecryptOutput tests the deriveDecryptOutput function to ensure it correctly derives the output file name for decryption.
// It covers cases with .enc extension, no extension, and other extensions.
func TestDeriveDecryptOutput(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "with .enc extension",
			src:  "file.txt.enc",
			want: "file.txt",
		},
		{
			name: "without extension",
			src:  "file",
			want: "file.dec",
		},
		{
			name: "with other extension",
			src:  "file.txt.encrypted",
			want: "file.txt.encrypted.dec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveDecryptOutput(tt.src)
			if got != tt.want {
				t.Errorf("deriveDecryptOutput(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

// TestDeriveDecompressOutput tests the deriveDecompressOutput function to ensure it correctly derives the output file name for decompression.
// It covers cases with .gz extension, no extension, and other extensions.
func TestDeriveDecompressOutput(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "with .gz extension",
			src:  "file.txt.gz",
			want: "file.txt",
		},
		{
			name: "without extension",
			src:  "file",
			want: "file.dec",
		},
		{
			name: "with other extension",
			src:  "file.compressed",
			want: "file.compressed.dec",
		},
		{
			name: "with .tar.gz extension",
			src:  "folder.tar.gz",
			want: "folder",
		},
		{
			name: "with .tgz extension",
			src:  "folder.tgz",
			want: "folder",
		},
		{
			name: "with .zip extension",
			src:  "folder.zip",
			want: "folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveDecompressOutput(tt.src)
			if got != tt.want {
				t.Errorf("deriveDecompressOutput(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

// TestDeriveCompressOutput tests the deriveCompressOutput helper for files and directories.
func TestDeriveCompressOutput(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	dirPath := filepath.Join(tmpDir, "folder")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	tests := []struct {
		name   string
		src    string
		format string
		want   string
	}{
		{
			name:   "file path",
			src:    filePath,
			format: "",
			want:   filePath + gzExt,
		},
		{
			name:   "directory path",
			src:    dirPath,
			format: "",
			want:   dirPath + tarGzExt,
		},
		{
			name:   "zip file path",
			src:    filePath,
			format: "zip",
			want:   filePath + zipExt,
		},
		{
			name:   "zip directory path",
			src:    dirPath,
			format: "zip",
			want:   dirPath + zipExt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveCompressOutput(tt.src, tt.format); got != tt.want {
				t.Errorf("deriveCompressOutput(%q, %q) = %q, want %q", tt.src, tt.format, got, tt.want)
			}
		})
	}
}

// TestEncryptCmdWithPassword tests the encrypt command with a provided password.
// It ensures that the command successfully creates an encrypted file.
func TestEncryptCmdWithPassword(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	db := newTestDatabase(t, false)

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"encrypt", srcFile, "-p", testPassword})

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	encFile := srcFile + encExt
	if _, err := os.Stat(encFile); err != nil {
		t.Fatalf("Encrypted file not created: %v", err)
	}
}

// TestEncryptDecryptCmdRoundTrip tests the full round-trip of encrypting and then decrypting a file.
// It verifies that the decrypted content matches the original content.
func TestEncryptDecryptCmdRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("secret data")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	db := newTestDatabase(t, false)

	password := testPassword

	// Encrypt
	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"encrypt", srcFile, "-p", password})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	encFile := srcFile + encExt

	// Decrypt
	rootCmd = NewRootCmd(db)
	decFile := filepath.Join(tmpDir, "decrypted.txt")
	rootCmd.SetArgs([]string{"decrypt", encFile, "-o", decFile, "-p", password})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	// Verify
	decData, err := os.ReadFile(decFile)
	if err != nil {
		t.Fatalf("Failed to read decrypted file: %v", err)
	}

	if string(decData) != string(originalContent) {
		t.Errorf("Content mismatch: got %q, want %q", string(decData), string(originalContent))
	}
}

// TestEncryptDecryptDirectoryCmdRoundTrip verifies the CLI can encrypt a
// directory into a single artifact and restore its tree on decrypt.
func TestEncryptDecryptDirectoryCmdRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("Failed to create source directories: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "empty"), 0o755); err != nil {
		t.Fatalf("Failed to create empty source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root secret"), 0o644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "child.txt"), []byte("nested secret"), 0o644); err != nil {
		t.Fatalf("Failed to write nested source file: %v", err)
	}

	db := newTestDatabase(t, false)

	password := testPassword

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"encrypt", srcDir, "-p", password})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	encFile := srcDir + encExt
	if info, err := os.Stat(encFile); err != nil {
		t.Fatalf("Encrypted artifact not created: %v", err)
	} else if info.IsDir() {
		t.Fatal("Encrypted artifact should be a file")
	}

	restoreDir := filepath.Join(tmpDir, "restored-project")
	rootCmd = NewRootCmd(db)
	rootCmd.SetArgs([]string{"decrypt", encFile, "-o", restoreDir, "-p", password})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	rootData, err := os.ReadFile(filepath.Join(restoreDir, "root.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored root file: %v", err)
	}
	if string(rootData) != "root secret" {
		t.Fatalf("Content mismatch: got %q", string(rootData))
	}

	childData, err := os.ReadFile(filepath.Join(restoreDir, "nested", "child.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored nested file: %v", err)
	}
	if string(childData) != "nested secret" {
		t.Fatalf("Content mismatch: got %q", string(childData))
	}
	if info, err := os.Stat(filepath.Join(restoreDir, "empty")); err != nil {
		t.Fatalf("Expected empty directory not restored: %v", err)
	} else if !info.IsDir() {
		t.Fatal("Restored empty path is not a directory")
	}
}

// TestCompressDecompressCmdRoundTrip tests the full round-trip of compressing and then decompressing a file.
// It verifies that the decompressed content matches the original content.
func TestCompressDecompressCmdRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("test data to compress")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	db := newTestDatabase(t, false)

	// Compress
	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"compress", srcFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	compFile := srcFile + gzExt

	// Decompress
	rootCmd = NewRootCmd(db)
	decFile := filepath.Join(tmpDir, "decompressed.txt")
	rootCmd.SetArgs([]string{"decompress", compFile, "-o", decFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	// Verify
	decData, err := os.ReadFile(decFile)
	if err != nil {
		t.Fatalf("Failed to read decompressed file: %v", err)
	}

	if string(decData) != string(originalContent) {
		t.Errorf("Content mismatch: got %q, want %q", string(decData), string(originalContent))
	}
}

// TestCompressDecompressDirectoryCmdRoundTrip tests the CLI round-trip for a
// compressed directory archive using default input and output paths.
func TestCompressDecompressDirectoryCmdRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "folder")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("Failed to create source directories: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "file.txt"), []byte("directory data"), 0o644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	db := newTestDatabase(t, false)

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"compress", srcDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	archive := srcDir + tarGzExt
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("Archive not created: %v", err)
	}

	if err := os.RemoveAll(srcDir); err != nil {
		t.Fatalf("Failed to remove source directory: %v", err)
	}

	rootCmd = NewRootCmd(db)
	rootCmd.SetArgs([]string{"decompress", archive})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(srcDir, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(data) != "directory data" {
		t.Fatalf("Content mismatch: got %q", string(data))
	}
}

func TestCompressDecompressZipCmdRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "artifact.txt")
	originalContent := []byte("zip file content")
	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	db := newTestDatabase(t, false)

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"compress", srcFile, "--format", "zip"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	archive := srcFile + zipExt
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("Zip archive not created: %v", err)
	}

	if err := os.Remove(srcFile); err != nil {
		t.Fatalf("Failed to remove source file: %v", err)
	}

	rootCmd = NewRootCmd(db)
	rootCmd.SetArgs([]string{"decompress", archive})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	restored, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(restored) != string(originalContent) {
		t.Fatalf("Content mismatch: got %q", string(restored))
	}
}

func TestCompressDecompressZipDirectoryCmdRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "folder")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("Failed to create source directories: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "file.txt"), []byte("zip directory data"), 0o644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "empty"), 0o755); err != nil {
		t.Fatalf("Failed to create empty source directory: %v", err)
	}

	db := newTestDatabase(t, false)

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"compress", srcDir, "--format", "zip"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	archive := srcDir + zipExt
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("Zip archive not created: %v", err)
	}

	if err := os.RemoveAll(srcDir); err != nil {
		t.Fatalf("Failed to remove source directory: %v", err)
	}

	rootCmd = NewRootCmd(db)
	rootCmd.SetArgs([]string{"decompress", archive})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(srcDir, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(data) != "zip directory data" {
		t.Fatalf("Content mismatch: got %q", string(data))
	}
	if info, err := os.Stat(filepath.Join(srcDir, "empty")); err != nil {
		t.Fatalf("Expected empty directory not restored: %v", err)
	} else if !info.IsDir() {
		t.Fatal("Restored empty path is not a directory")
	}
}

// TestKeysListCmd tests the "keys list" command to ensure it correctly lists all keys in the database.
// It verifies that the output contains the expected key IDs.
func TestKeysListCmd(t *testing.T) {
	db := newTestDatabase(t, false)

	// Add a key
	km := &KeyModel{
		KeyID:         "test-key-1",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}
	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "list"})

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	output := out.String()
	if output == "" {
		t.Error("Command produced no output")
	}
}

func TestKeysDeleteCmdConfirmed(t *testing.T) {
	db := newTestDatabase(t, false)

	km := &KeyModel{
		KeyID:         "delete-me",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}
	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "delete", km.KeyID})
	rootCmd.SetIn(bytes.NewBufferString("y\n"))

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if _, err := db.GetKey(km.KeyID); err == nil {
		t.Fatal("expected key to be deleted")
	}
	if !strings.Contains(out.String(), "Deleted key: "+km.KeyID) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestKeysDeleteCmdRequiresConfirmation(t *testing.T) {
	db := newTestDatabase(t, false)

	km := &KeyModel{
		KeyID:         "do-not-delete",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}
	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "delete", km.KeyID})
	rootCmd.SetIn(bytes.NewBufferString("n\n"))

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation rejection error")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := db.GetKey(km.KeyID); err != nil {
		t.Fatalf("expected key to remain after abort, got: %v", err)
	}
}

func TestKeysDeleteCmdForceBypass(t *testing.T) {
	db := newTestDatabase(t, false)

	km := &KeyModel{
		KeyID:         "force-delete",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}
	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "delete", km.KeyID, "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if _, err := db.GetKey(km.KeyID); err == nil {
		t.Fatal("expected key to be deleted")
	}
}

func TestKeysDeleteCmdYesBypass(t *testing.T) {
	db := newTestDatabase(t, false)

	km := &KeyModel{
		KeyID:         "yes-delete",
		Algorithm:     "AES-256-GCM",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}
	if err := db.SaveKey(km); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "delete", km.KeyID, "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if _, err := db.GetKey(km.KeyID); err == nil {
		t.Fatal("expected key to be deleted")
	}
}

func TestKeysDeleteCmdNotFound(t *testing.T) {
	db := newTestDatabase(t, false)

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "delete", "missing-key", "--yes"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestEncryptCmdRejectsEmptyInteractivePassword guards SEC-001 on the CLI: an empty
// answer at the password prompt must not produce an encrypted file.
func TestEncryptCmdRejectsEmptyInteractivePassword(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("test content"), 0o600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	db := newTestDatabase(t, false)
	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"encrypt", srcFile})
	rootCmd.SetIn(strings.NewReader("\n"))
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("encrypt with an empty password succeeded, want an error")
	}
	if _, err := os.Stat(srcFile + encExt); !os.IsNotExist(err) {
		t.Fatalf("encrypted file created despite the empty password (stat err: %v)", err)
	}
}

// TestReadPassword is a regression test for SEC-002: when input is not a terminal,
// the prompt must keep the whole line, including spaces, and strip only the line ending.
func TestReadPassword(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "multi-word passphrase", input: "correct horse battery staple\n", want: "correct horse battery staple"},
		{name: "windows line ending", input: "pass word\r\n", want: "pass word"},
		{name: "leading and trailing spaces kept", input: "  spaced out  \n", want: "  spaced out  "},
		{name: "no trailing newline", input: "secret", want: "secret"},
		{name: "only the first line is read", input: "first line\nsecond line\n", want: "first line"},
		{name: "empty line", input: "\n", want: ""},
		{name: "no input", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader(tt.input))
			var prompt bytes.Buffer
			cmd.SetErr(&prompt)

			got, err := readPassword(cmd, "Enter password: ")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readPassword() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readPassword() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("readPassword() = %q, want %q", got, tt.want)
			}
			if prompt.String() != "Enter password: " {
				t.Fatalf("prompt written = %q, want %q", prompt.String(), "Enter password: ")
			}
		})
	}
}

// TestEncryptDecryptCmdMultiWordPromptPassword is a regression test for SEC-002: a
// passphrase typed at the prompt must be used in full, not cut at the first space.
func TestEncryptDecryptCmdMultiWordPromptPassword(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("multi-word passphrase content")
	if err := os.WriteFile(srcFile, content, 0o600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	const passphrase = "correct horse battery staple"
	db := newTestDatabase(t, false)

	run := func(stdin string, args ...string) error {
		rootCmd := NewRootCmd(db)
		rootCmd.SetArgs(args)
		rootCmd.SetIn(strings.NewReader(stdin))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}

	encFile := srcFile + encExt
	if err := run(passphrase+"\n", "encrypt", srcFile); err != nil {
		t.Fatalf("encrypt via prompt failed: %v", err)
	}
	if err := run("", "decrypt", encFile, "--output", filepath.Join(tmpDir, "first-word.txt"), "--password", "correct"); err == nil {
		t.Fatal("decrypting with only the first word succeeded; the prompt truncated the passphrase")
	}
	decFile := filepath.Join(tmpDir, "decrypted.txt")
	if err := run(passphrase+"\n", "decrypt", encFile, "--output", decFile); err != nil {
		t.Fatalf("decrypt via prompt with the full passphrase failed: %v", err)
	}
	if got, err := os.ReadFile(decFile); err != nil || string(got) != string(content) {
		t.Fatalf("decrypted content = %q (err %v), want %q", got, err, content)
	}
}

// TestDecryptCmdPromptAcceptsEmptyPasswordForLegacyFiles checks that a file encrypted
// with an empty password before SEC-001 was fixed can be decrypted from the CLI prompt.
func TestDecryptCmdPromptAcceptsEmptyPasswordForLegacyFiles(t *testing.T) {
	tmpDir := t.TempDir()
	content := []byte("legacy content")
	ciphertext, err := encryptBytes(content, "")
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	encFile := filepath.Join(tmpDir, "legacy.txt.enc")
	if err := os.WriteFile(encFile, ciphertext, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	decFile := filepath.Join(tmpDir, "legacy.txt")
	rootCmd := NewRootCmd(newTestDatabase(t, false))
	rootCmd.SetArgs([]string{"decrypt", encFile, "--output", decFile})
	rootCmd.SetIn(strings.NewReader("\n"))
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("decrypt with an empty prompt password failed: %v", err)
	}
	if got, err := os.ReadFile(decFile); err != nil || string(got) != string(content) {
		t.Fatalf("decrypted content = %q (err %v), want %q", got, err, content)
	}
}

// TestReadPasswordFromNonTerminalFile checks that input from a real file that isn't a
// terminal (as with shell redirection) is read as a plain line, not through the
// hidden terminal prompt.
func TestReadPasswordFromNonTerminalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password.txt")
	if err := os.WriteFile(path, []byte("piped pass phrase\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open password file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	cmd := &cobra.Command{}
	cmd.SetIn(f)
	cmd.SetErr(&bytes.Buffer{})
	got, err := readPassword(cmd, "Enter password: ")
	if err != nil {
		t.Fatalf("readPassword() error = %v", err)
	}
	if got != "piped pass phrase" {
		t.Fatalf("readPassword() = %q, want %q", got, "piped pass phrase")
	}
}

// TestReadTerminalPasswordRejectsNonTerminal checks that the hidden-input helper
// fails cleanly, before installing its interrupt handler, when fd is not a terminal.
func TestReadTerminalPasswordRejectsNonTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-terminal")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if _, err := readTerminalPassword(f.Fd(), &bytes.Buffer{}); err == nil {
		t.Fatal("readTerminalPassword() on a regular file succeeded, want an error")
	}
}

// TestFileCmdsRefuseExistingOutput is a regression test for BUG-004: encrypt, decrypt,
// compress and decompress must not replace an existing output unless --force is given.
func TestFileCmdsRefuseExistingOutput(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "data.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	enc := src + encExt
	if err := EncryptFile(src, enc, testPassword); err != nil {
		t.Fatalf("prepare encrypted file: %v", err)
	}
	gz := src + gzExt
	if err := CompressFile(src, gz, -1); err != nil {
		t.Fatalf("prepare gzip file: %v", err)
	}
	db := newTestDatabase(t, false)

	run := func(args ...string) error {
		rootCmd := NewRootCmd(db)
		rootCmd.SetArgs(args)
		rootCmd.SetIn(strings.NewReader(""))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}

	cases := []struct {
		name string
		args func(out string) []string
	}{
		{"encrypt", func(out string) []string {
			return []string{"encrypt", src, "--output", out, "--password", testPassword}
		}},
		{"decrypt", func(out string) []string {
			return []string{"decrypt", enc, "--output", out, "--password", testPassword}
		}},
		{"compress", func(out string) []string { return []string{"compress", src, "--output", out} }},
		{"decompress", func(out string) []string { return []string{"decompress", gz, "--output", out} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(tmpDir, tc.name+".out")
			if err := os.WriteFile(out, []byte("existing"), 0o600); err != nil {
				t.Fatalf("write existing output: %v", err)
			}

			err := run(tc.args(out)...)
			if !errors.Is(err, ErrOutputExists) || !strings.Contains(err.Error(), "--force") {
				t.Fatalf("error = %v, want ErrOutputExists with a --force hint", err)
			}
			if got, _ := os.ReadFile(out); string(got) != "existing" {
				t.Fatalf("existing output was modified to %q", got)
			}

			if err := run(append(tc.args(out), "--force")...); err != nil {
				t.Fatalf("with --force: %v", err)
			}
			if got, _ := os.ReadFile(out); string(got) == "existing" {
				t.Fatal("with --force the output was not replaced")
			}
		})
	}

	// Default output: decrypting data.txt.enc would write data.txt, which still exists.
	if err := run("decrypt", enc, "--password", testPassword); !errors.Is(err, ErrOutputExists) {
		t.Fatalf("decrypt to existing default output: error = %v, want ErrOutputExists", err)
	}
	if got, _ := os.ReadFile(src); string(got) != "payload" {
		t.Fatalf("default output was modified to %q", got)
	}
}

// TestCompressCmdRefusesSameInputOutputEvenWithForce is a regression test for
// BUG-003: --force must not allow an output that is the input itself.
func TestCompressCmdRefusesSameInputOutputEvenWithForce(t *testing.T) {
	src := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(src, []byte("irreplaceable"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	rootCmd := NewRootCmd(newTestDatabase(t, false))
	rootCmd.SetArgs([]string{"compress", src, "--output", src, "--force"})
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)

	if err := rootCmd.Execute(); !errors.Is(err, ErrSameInputOutput) {
		t.Fatalf("error = %v, want ErrSameInputOutput", err)
	}
	if got, _ := os.ReadFile(src); string(got) != "irreplaceable" {
		t.Fatalf("source was modified to %q", got)
	}
}

// TestReadNewPasswordWith is a regression test for SEC-001 and SEC-002: a password that
// will protect new data is checked against the policy before it is confirmed, and when
// confirmation is on (terminal input) a mismatched second entry is refused.
func TestReadNewPasswordWith(t *testing.T) {
	const other = "correct horse battery stapler"
	tests := []struct {
		name        string
		answers     []string
		confirm     bool
		want        string
		wantErr     error
		wantPrompts []string
	}{
		{
			name:        "confirmed",
			answers:     []string{testPassword, testPassword},
			confirm:     true,
			want:        testPassword,
			wantPrompts: []string{"Enter password: ", "Confirm password: "},
		},
		{
			name:        "confirmation differs",
			answers:     []string{testPassword, other},
			confirm:     true,
			wantErr:     ErrPasswordMismatch,
			wantPrompts: []string{"Enter password: ", "Confirm password: "},
		},
		{
			name:        "weak password is refused before confirmation",
			answers:     []string{"hunter2"},
			confirm:     true,
			wantErr:     ErrWeakPassword,
			wantPrompts: []string{"Enter password: "},
		},
		{
			name:        "empty password is refused before confirmation",
			answers:     []string{""},
			confirm:     true,
			wantErr:     ErrEmptyPassword,
			wantPrompts: []string{"Enter password: "},
		},
		{
			name:        "piped input is read once",
			answers:     []string{testPassword},
			want:        testPassword,
			wantPrompts: []string{"Enter password: "},
		},
		{
			name:        "piped weak password is refused",
			answers:     []string{"hunter2"},
			wantErr:     ErrWeakPassword,
			wantPrompts: []string{"Enter password: "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var prompts []string
			read := func(prompt string) (string, error) {
				prompts = append(prompts, prompt)
				if len(prompts) > len(tt.answers) {
					return "", errors.New("unexpected extra prompt")
				}
				return tt.answers[len(prompts)-1], nil
			}

			got, err := readNewPasswordWith(read, "Enter password: ", tt.confirm)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("readNewPasswordWith() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil || got != tt.want {
				t.Fatalf("readNewPasswordWith() = %q, %v; want %q", got, err, tt.want)
			}
			if strings.Join(prompts, "|") != strings.Join(tt.wantPrompts, "|") {
				t.Fatalf("prompts = %q, want %q", prompts, tt.wantPrompts)
			}
		})
	}

	readErr := errors.New("read failed")
	_, err := readNewPasswordWith(func(prompt string) (string, error) {
		if prompt == "Confirm password: " {
			return "", readErr
		}
		return testPassword, nil
	}, "Enter password: ", true)
	if !errors.Is(err, readErr) {
		t.Fatalf("confirmation read error = %v, want %v", err, readErr)
	}
}

// TestNewPasswordCmdsRejectWeakPassword is a regression test for SEC-001 on the CLI:
// encrypt, keys generate and keys export refuse a password that fails the policy,
// whether it comes from --password or the prompt, and change nothing.
func TestNewPasswordCmdsRejectWeakPassword(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "data.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	db := newTestDatabase(t, false)

	blob, err := EncryptKeyBlob(make([]byte, keyLen), testPassword)
	if err != nil {
		t.Fatalf("EncryptKeyBlob: %v", err)
	}
	const keyID = "0123456789abcdef"
	if err := db.SaveKey(&KeyModel{KeyID: keyID, Algorithm: "AES-256-GCM", EncryptedBlob: blob, CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	exportPath := filepath.Join(tmpDir, "key.ckey")

	run := func(stdin string, args ...string) error {
		rootCmd := NewRootCmd(db)
		rootCmd.SetArgs(args)
		rootCmd.SetIn(strings.NewReader(stdin))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}

	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"encrypt flag", "", []string{"encrypt", src, "--password", "hunter2"}},
		{"encrypt prompt", "hunter2\n", []string{"encrypt", src}},
		{"keys generate flag", "", []string{"keys", "generate", "--password", "hunter2"}},
		{"keys generate prompt", "hunter2\n", []string{"keys", "generate"}},
		// keys export reuses the key's master password (BUG-011); a weak one is
		// refused in TestLegacyShortPasswordCmds.
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(tc.stdin, tc.args...); !errors.Is(err, ErrWeakPassword) {
				t.Fatalf("error = %v, want ErrWeakPassword", err)
			}
			if _, err := os.Stat(src + encExt); !os.IsNotExist(err) {
				t.Fatalf("encrypted file created despite the weak password (stat err: %v)", err)
			}
			if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
				t.Fatalf("export file created despite the weak password (stat err: %v)", err)
			}
			if keys, err := db.ListKeys(); err != nil || len(keys) != 1 {
				t.Fatalf("stored keys = %d (err %v), want only the fixture key", len(keys), err)
			}
		})
	}
}

// TestLegacyShortPasswordCmds checks that the CLI still decrypts files and imports key
// exports protected with a password shorter than the policy minimum, and that a key
// stored under a short master password isn't exported: the export would be protected
// by that same password (BUG-011), which can't protect a new file (SEC-001).
func TestLegacyShortPasswordCmds(t *testing.T) {
	tmpDir := t.TempDir()
	const legacyPassword = "hunter2"
	db := newTestDatabase(t, false)

	run := func(stdin string, args ...string) error {
		rootCmd := NewRootCmd(db)
		rootCmd.SetArgs(args)
		rootCmd.SetIn(strings.NewReader(stdin))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}

	ciphertext, err := encryptBytes([]byte("legacy content"), legacyPassword)
	if err != nil {
		t.Fatalf("encryptBytes: %v", err)
	}
	encFile := filepath.Join(tmpDir, "legacy.txt.enc")
	if err := os.WriteFile(encFile, ciphertext, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	if err := run(legacyPassword+"\n", "decrypt", encFile); err != nil {
		t.Fatalf("decrypt legacy file via prompt: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(tmpDir, "legacy.txt")); err != nil || string(got) != "legacy content" {
		t.Fatalf("decrypted content = %q (err %v), want %q", got, err, "legacy content")
	}

	// A key stored with a short master password, exported the way pre-policy builds did.
	blobBytes, err := encryptBytes(make([]byte, keyLen), legacyPassword)
	if err != nil {
		t.Fatalf("encryptBytes(key): %v", err)
	}
	storedBlob := base64.StdEncoding.EncodeToString(blobBytes)
	envelope := []byte(`{"version":1,"key_id":"0123456789abcdef","algorithm":"AES-256-GCM","created_at":1,"encrypted_blob":"` + storedBlob + `"}`)
	exportBytes, err := encryptBytes(envelope, legacyPassword)
	if err != nil {
		t.Fatalf("encryptBytes(export): %v", err)
	}
	legacyExport := filepath.Join(tmpDir, "legacy.ckey")
	if err := os.WriteFile(legacyExport, []byte(base64.StdEncoding.EncodeToString(exportBytes)), 0o600); err != nil {
		t.Fatalf("write legacy export: %v", err)
	}
	if err := run("", "keys", "import", legacyExport, "--password", legacyPassword); err != nil {
		t.Fatalf("import legacy export: %v", err)
	}

	newExport := filepath.Join(tmpDir, "renewed.ckey")
	if err := run("", "keys", "export", "0123456789abcdef", "--output", newExport, "--password", testPassword); !errors.Is(err, ErrWrongMasterPassword) {
		t.Fatalf("export with a password other than the key's: err = %v, want ErrWrongMasterPassword", err)
	}
	if err := run("", "keys", "export", "0123456789abcdef", "--output", newExport, "--password", legacyPassword); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("export of a key with a short master password: err = %v, want ErrWeakPassword", err)
	}
	if _, err := os.Stat(newExport); !os.IsNotExist(err) {
		t.Fatalf("export file created (stat err: %v)", err)
	}
}

// TestParseSize checks the --max-size values the CLI accepts.
func TestParseSize(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{in: "0", want: 0},
		{in: "1024", want: 1024},
		{in: "1B", want: 1},
		{in: "10GiB", want: 10 << 30},
		{in: "10 GiB", want: 10 << 30},
		{in: "500MB", want: 500_000_000},
		{in: "64kib", want: 64 << 10},
		{in: "2TB", want: 2_000_000_000_000},
		{in: "", wantErr: true},
		{in: "ten", wantErr: true},
		{in: "-1", wantErr: true},
		{in: "1.5GB", wantErr: true},
		{in: "10XB", wantErr: true},
		{in: "99999999999TiB", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseSize(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseSize(%q) = %d, want an error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("parseSize(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
}

// TestExtractLimitFlags is a regression test for SEC-007 on the CLI: decompress and
// decrypt stop at the limits set by --max-size and --max-entries (defaults 10 GiB and
// 100,000), say how to change them, and accept 0 for no limit.
func TestExtractLimitFlags(t *testing.T) {
	tmpDir := t.TempDir()
	zeros := filepath.Join(tmpDir, "zeros.bin")
	if err := os.WriteFile(zeros, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatalf("write zeros: %v", err)
	}
	bomb := zeros + gzExt
	if err := CompressFile(zeros, bomb, -1); err != nil {
		t.Fatalf("prepare gzip: %v", err)
	}
	tree := filepath.Join(tmpDir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatalf("create tree: %v", err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(tree, name), make([]byte, 1<<20), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	archive := filepath.Join(tmpDir, "tree.tar.gz")
	if err := CompressFileWithFormat(tree, archive, "", -1); err != nil {
		t.Fatalf("prepare tar.gz: %v", err)
	}
	enc := filepath.Join(tmpDir, "tree.enc")
	if err := EncryptFile(tree, enc, testPassword); err != nil {
		t.Fatalf("prepare encrypted folder: %v", err)
	}
	db := newTestDatabase(t, false)

	run := func(args ...string) error {
		rootCmd := NewRootCmd(db)
		rootCmd.SetArgs(args)
		rootCmd.SetIn(strings.NewReader(""))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}
	out := func(name string) string { return filepath.Join(tmpDir, name) }

	limited := []struct {
		name string
		args []string
		dst  string
	}{
		{"gzip size", []string{"decompress", bomb, "--output", out("a.bin"), "--max-size", "64KiB"}, out("a.bin")},
		{"tar.gz size", []string{"decompress", archive, "--output", out("b"), "--max-size", "1MiB"}, out("b")},
		{"tar.gz entries", []string{"decompress", archive, "--output", out("c"), "--max-entries", "1"}, out("c")},
		{"decrypt size", []string{"decrypt", enc, "--output", out("d"), "--password", testPassword, "--max-size", "64KiB"}, out("d")},
		{"decrypt entries", []string{"decrypt", enc, "--output", out("e"), "--password", testPassword, "--max-entries", "1"}, out("e")},
	}
	for _, tc := range limited {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.args...)
			if !errors.Is(err, ErrExtractLimit) || !strings.Contains(err.Error(), "--max-size") {
				t.Fatalf("error = %v, want ErrExtractLimit with a hint about the flags", err)
			}
			if _, err := os.Stat(tc.dst); !os.IsNotExist(err) {
				t.Fatalf("output left behind (stat err: %v)", err)
			}
		})
	}

	if err := run("decompress", bomb, "--output", out("f.bin"), "--max-size", "0"); err != nil {
		t.Fatalf("decompress with --max-size 0: %v", err)
	}
	if err := run("decrypt", enc, "--output", out("g"), "--password", testPassword, "--max-size", "0", "--max-entries", "0"); err != nil {
		t.Fatalf("decrypt with no limits: %v", err)
	}
	if err := run("decompress", archive, "--output", out("h")); err != nil {
		t.Fatalf("decompress with the default limits: %v", err)
	}

	for _, args := range [][]string{
		{"decompress", bomb, "--output", out("i.bin"), "--max-size", "ten"},
		{"decompress", bomb, "--output", out("i.bin"), "--max-entries", "-1"},
		{"decrypt", enc, "--output", out("j"), "--password", testPassword, "--max-size", "1.5GB"},
	} {
		if err := run(args...); err == nil {
			t.Errorf("%v succeeded, want an invalid-limit error", args)
		}
	}
	if _, err := os.Stat(out("i.bin")); !os.IsNotExist(err) {
		t.Fatalf("output written despite an invalid limit (stat err: %v)", err)
	}

	for _, name := range []string{"decompress", "decrypt"} {
		cmd, _, err := NewRootCmd(db).Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if got := cmd.Flags().Lookup("max-size").DefValue; got != "10 GiB" {
			t.Errorf("%s --max-size default = %q, want %q", name, got, "10 GiB")
		}
		if got := cmd.Flags().Lookup("max-entries").DefValue; got != "100000" {
			t.Errorf("%s --max-entries default = %q, want %q", name, got, "100000")
		}
	}
}

// TestRootCmdOpensDatabaseOnlyForKeys is a regression test for BUG-005 and SEC-010: the
// database opener passed to NewRootCmdLazy is called by the keys commands only.
func TestRootCmdOpensDatabaseOnlyForKeys(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	db := newTestDatabase(t, false)
	opened := 0
	open := func() (*Database, error) {
		opened++
		return db, nil
	}
	run := func(args ...string) error {
		rootCmd := NewRootCmdLazy(open)
		rootCmd.SetArgs(args)
		rootCmd.SetIn(strings.NewReader(""))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}

	for _, args := range [][]string{
		{"--help"},
		{"encrypt", src, "--password", testPassword},
		{"decrypt", src + encExt, "--output", filepath.Join(dir, "plain.txt"), "--password", testPassword},
		{"compress", src},
		{"decompress", src + gzExt, "--output", filepath.Join(dir, "unpacked.txt")},
		{"keys", "--help"},
	} {
		if err := run(args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if opened != 0 {
		t.Fatalf("file commands opened the database %d times, want 0", opened)
	}

	if err := run("keys", "generate", "--password", testPassword); err != nil {
		t.Fatalf("keys generate: %v", err)
	}
	if err := run("keys", "list"); err != nil {
		t.Fatalf("keys list: %v", err)
	}
	if opened != 2 {
		t.Fatalf("keys commands opened the database %d times, want 2", opened)
	}
}

// TestKeysCmdReportsDatabaseOpenError checks that a keys command fails cleanly, with
// context, when the key database can't be opened.
func TestKeysCmdReportsDatabaseOpenError(t *testing.T) {
	openErr := errors.New("disk on fire")
	for _, args := range [][]string{
		{"keys", "list"},
		{"keys", "generate", "--password", testPassword},
		{"keys", "export", "0123456789abcdef", "--password", testPassword},
		{"keys", "import", "missing.ckey", "--password", testPassword},
		{"keys", "delete", "0123456789abcdef", "--yes"},
	} {
		rootCmd := NewRootCmdLazy(func() (*Database, error) { return nil, openErr })
		rootCmd.SetArgs(args)
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		err := rootCmd.Execute()
		if !errors.Is(err, openErr) || !strings.Contains(err.Error(), "open key database") {
			t.Errorf("%v: error = %v, want the open error with context", args, err)
		}
	}
}

// TestReadPasswordFile checks how --password-file reads its file: the first line, with
// spaces kept and only the line ending removed, as for a password piped to the prompt.
func TestReadPasswordFile(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content *string // nil: the file doesn't exist
		want    string
		wantErr string
	}{
		{name: "passphrase with spaces", content: ptr("correct horse battery staple\n"), want: "correct horse battery staple"},
		{name: "windows line ending", content: ptr("pass word here\r\n"), want: "pass word here"},
		{name: "only the first line", content: ptr("first line\nsecond line\n"), want: "first line"},
		{name: "no trailing newline", content: ptr("  spaced  "), want: "  spaced  "},
		{name: "empty first line", content: ptr("\nsecond\n"), want: ""},
		{name: "empty file", content: ptr(""), wantErr: "is empty"},
		{name: "first line too long", content: ptr(strings.Repeat("x", maxPasswordFileSize+1)), wantErr: "too long"},
		{name: "missing file", wantErr: "open password file"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, fmt.Sprintf("pw%d.txt", i))
			if tt.content != nil {
				if err := os.WriteFile(path, []byte(*tt.content), 0o600); err != nil {
					t.Fatalf("write password file: %v", err)
				}
			}
			got, err := readPasswordFile(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("readPasswordFile() = %q, %v; want an error containing %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("readPasswordFile() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// TestPasswordFileFlag is a regression test for SEC-004: every command that takes
// --password also takes --password-file, which keeps the secret out of the process
// arguments and prints no warning.
func TestPasswordFileFlag(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	pwFile := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(pwFile, []byte(testPassword+"\r\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	db := newTestDatabase(t, false)

	run := func(args ...string) (string, error) {
		rootCmd := NewRootCmd(db)
		rootCmd.SetArgs(args)
		rootCmd.SetIn(strings.NewReader(""))
		var out, errOut bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&errOut)
		err := rootCmd.Execute()
		return errOut.String(), err
	}
	check := func(args ...string) {
		t.Helper()
		stderr, err := run(args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(stderr, "warning") {
			t.Fatalf("%v printed a warning: %q", args, stderr)
		}
	}

	check("encrypt", src, "--password-file", pwFile)
	check("decrypt", src+encExt, "--output", filepath.Join(dir, "plain.txt"), "--password-file", pwFile)
	if got, err := os.ReadFile(filepath.Join(dir, "plain.txt")); err != nil || string(got) != "payload" {
		t.Fatalf("decrypted content = %q (err %v), want %q", got, err, "payload")
	}
	// The file's password is the one used: the same passphrase via --password decrypts.
	if _, err := run("decrypt", src+encExt, "--output", filepath.Join(dir, "plain2.txt"), "--password", testPassword); err != nil {
		t.Fatalf("decrypt with the same passphrase via --password: %v", err)
	}

	check("keys", "generate", "--password-file", pwFile)
	keys, err := db.ListKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("stored keys = %d (err %v), want 1", len(keys), err)
	}
	exportPath := filepath.Join(dir, "key.ckey")
	check("keys", "export", keys[0].KeyID, "--output", exportPath, "--password-file", pwFile)
	if err := db.DeleteKey(keys[0].KeyID); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
	check("keys", "import", exportPath, "--password-file", pwFile)
	if keys, err := db.ListKeys(); err != nil || len(keys) != 1 {
		t.Fatalf("keys after import = %d (err %v), want 1", len(keys), err)
	}

	// The policy still applies to a password from a file.
	weakFile := filepath.Join(dir, "weak.txt")
	if err := os.WriteFile(weakFile, []byte("hunter2\n"), 0o600); err != nil {
		t.Fatalf("write weak password file: %v", err)
	}
	if _, err := run("encrypt", src, "--output", filepath.Join(dir, "weak.enc"), "--password-file", weakFile); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("encrypt with a weak password file: error = %v, want ErrWeakPassword", err)
	}

	// --password and --password-file can't be combined.
	if _, err := run("encrypt", src, "--output", filepath.Join(dir, "both.enc"), "--password", testPassword, "--password-file", pwFile); err == nil {
		t.Fatal("encrypt with both --password and --password-file succeeded, want an error")
	}
	// A missing password file is an error, not a fallback to the prompt.
	if _, err := run("encrypt", src, "--output", filepath.Join(dir, "missing.enc"), "--password-file", filepath.Join(dir, "nope.txt")); err == nil {
		t.Fatal("encrypt with a missing password file succeeded, want an error")
	}
}

// TestPasswordFlagWarns is a regression test for SEC-004 step 3: --password still works
// but prints a warning on stderr that suggests the alternatives.
func TestPasswordFlagWarns(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	rootCmd := NewRootCmd(newTestDatabase(t, false))
	rootCmd.SetArgs([]string{"encrypt", src, "--password", testPassword})
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("encrypt with --password: %v", err)
	}
	if !strings.Contains(errOut.String(), "warning: --password") || !strings.Contains(errOut.String(), "--password-file") {
		t.Fatalf("stderr = %q, want a warning that suggests --password-file", errOut.String())
	}
	if strings.Contains(out.String(), "warning") {
		t.Fatalf("warning written to stdout: %q", out.String())
	}
	if strings.Contains(errOut.String(), testPassword) {
		t.Fatal("the warning repeats the password")
	}
}

// TestKeysImportCmdRejectsInvalidExport is a regression test for SEC-011 on the CLI: an
// export with a crafted key ID is refused and nothing is stored.
func TestKeysImportCmdRejectsInvalidExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evil.ckey")
	writeTestExport(t, path, KeyExport{Version: 1, KeyID: "\x1b[2Jevil", Algorithm: "AES-256-GCM", CreatedAt: 1, EncryptedBlob: validStoredBlob(t)})
	db := newTestDatabase(t, false)

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "import", path, "--password-file", writePasswordFile(t, testPassword)})
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	if err := rootCmd.Execute(); !errors.Is(err, ErrInvalidKeyExport) {
		t.Fatalf("error = %v, want ErrInvalidKeyExport", err)
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Fatalf("output contains a raw escape character: %q", out.String())
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 0 {
		t.Fatalf("stored keys = %d (err %v), want 0", len(keys), err)
	}
}

// writePasswordFile writes password to a new file and returns its path.
func writePasswordFile(t *testing.T, password string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pw.txt")
	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	return path
}

// TestKeysExportReportsWrittenPath is a regression test for BUG-006: without --output,
// the file name keys export reports is the file it wrote, even when the clock moves on
// between computing the name and printing it.
func TestKeysExportReportsWrittenPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clock := time.Unix(1_800_000_000, 0)
	previous := timeNow
	timeNow = func() time.Time { clock = clock.Add(time.Second); return clock }
	t.Cleanup(func() { timeNow = previous })

	db := newTestDatabase(t, false)
	keyID := "0123456789abcdef"
	if err := db.SaveKey(&KeyModel{KeyID: keyID, Algorithm: "AES-256-GCM", EncryptedBlob: validStoredBlob(t), CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "export", keyID, "--password-file", writePasswordFile(t, testPassword)})
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("keys export: %v", err)
	}

	_, reported, found := strings.Cut(strings.TrimSpace(out.String()), " → ")
	if !found {
		t.Fatalf("output %q has no reported path", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, reported)); err != nil {
		t.Fatalf("reported path %q was not written: %v", reported, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.ckey")); len(matches) != 1 {
		t.Fatalf("export files = %v, want exactly one", matches)
	}
}

// TestDisplayText checks that only values holding non-printable characters or invalid
// UTF-8 are quoted for display (SEC-016).
func TestDisplayText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"AES-256-GCM", "AES-256-GCM"},
		{"0123456789abcdef", "0123456789abcdef"},
		{"clé für 鍵", "clé für 鍵"},
		{"", ""},
		{"\x1b[2J", `"\x1b[2J"`},
		{"id\twith tab", `"id\twith tab"`},
		{"\u009b31m", `"\u009b31m"`},       // C1 control (CSI)
		{"abc\u202edef", `"abc\u202edef"`}, // right-to-left override
		{"bad\xffutf8", `"bad\xffutf8"`},   // invalid UTF-8
	}
	for _, tc := range tests {
		if got := displayText(tc.in); got != tc.want {
			t.Errorf("displayText(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// TestKeysListEscapesControlCharacters is SEC-016's display check for the CLI: a
// stored key ID and algorithm holding terminal escape sequences are listed quoted,
// not raw.
func TestKeysListEscapesControlCharacters(t *testing.T) {
	db := newTestDatabase(t, false)
	if err := db.SaveKey(&KeyModel{
		KeyID:         "\x1b]0;PWNED\a\x1b[31m",
		Algorithm:     "\x1b[2J",
		EncryptedBlob: "blob",
		CreatedAt_:    time.Now().Unix(),
	}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}

	rootCmd := NewRootCmd(db)
	rootCmd.SetArgs([]string{"keys", "list"})
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("keys list: %v", err)
	}

	got := out.String()
	if strings.ContainsAny(got, "\x1b\a") {
		t.Fatalf("keys list printed raw control characters: %q", got)
	}
	for _, want := range []string{`"\x1b]0;PWNED\a\x1b[31m"`, `"\x1b[2J"`} {
		if !strings.Contains(got, want) {
			t.Errorf("keys list output %q lacks %s", got, want)
		}
	}
}

// TestKeysExportRefusesExistingOutput is the regression test for BUG-017: keys export
// leaves an existing file alone unless --force is given, and never writes over the
// key database, even with --force.
func TestKeysExportRefusesExistingOutput(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "keys.db")
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { closeTestDatabase(t, db) })
	const keyID = "0123456789abcdef"
	if err := db.SaveKey(&KeyModel{KeyID: keyID, Algorithm: "AES-256-GCM", EncryptedBlob: validStoredBlob(t), CreatedAt_: 1}); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	pwFile := writePasswordFile(t, testPassword)
	export := func(args ...string) error {
		rootCmd := NewRootCmd(db)
		rootCmd.SetArgs(append([]string{"keys", "export", keyID, "--password-file", pwFile}, args...))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}

	precious := filepath.Join(t.TempDir(), "precious.txt")
	if err := os.WriteFile(precious, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := export("--output", precious); !errors.Is(err, ErrOutputExists) {
		t.Fatalf("export onto an existing file: err = %v, want ErrOutputExists", err)
	}
	if got, _ := os.ReadFile(precious); string(got) != "keep me" {
		t.Fatalf("existing file changed to %q", got)
	}
	if err := export("--output", precious, "--force"); err != nil {
		t.Fatalf("export --force: %v", err)
	}
	if _, err := ImportKeyFromFile(precious, testPassword); err != nil {
		t.Fatalf("forced export isn't a valid export: %v", err)
	}

	for _, extra := range [][]string{nil, {"--force"}} {
		args := append([]string{"--output", dbPath}, extra...)
		if err := export(args...); !errors.Is(err, ErrOutputIsKeyDatabase) {
			t.Fatalf("export %v: err = %v, want ErrOutputIsKeyDatabase", args, err)
		}
	}
	if keys, err := db.ListKeys(); err != nil || len(keys) != 1 {
		t.Fatalf("key database after refused exports: %d keys (err %v), want 1", len(keys), err)
	}
}

// TestEncryptCmdFolderOutputStaysOutside checks BUG-016 through the CLI: "encrypt dir/"
// writes and reports dir.enc next to the folder, and an --output inside it is refused.
func TestEncryptCmdFolderOutputStaysOutside(t *testing.T) {
	parent := t.TempDir()
	folder := filepath.Join(parent, "secret")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "a.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	pwFile := writePasswordFile(t, testPassword)
	run := func(args ...string) (string, error) {
		rootCmd := NewRootCmd(newTestDatabase(t, false))
		rootCmd.SetArgs(args)
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		err := rootCmd.Execute()
		return out.String(), err
	}

	out, err := run("encrypt", folder+string(filepath.Separator), "--password-file", pwFile)
	if err != nil {
		t.Fatalf("encrypt secret/: %v", err)
	}
	if !strings.Contains(out, "→ "+folder+encExt) {
		t.Fatalf("output %q doesn't report %s", out, folder+encExt)
	}
	if _, err := os.Stat(folder + encExt); err != nil {
		t.Fatalf("secret.enc wasn't written: %v", err)
	}

	inside := filepath.Join(folder, "x.enc")
	if _, err := run("encrypt", folder, "--output", inside, "--password-file", pwFile); !errors.Is(err, ErrOutputInsideInput) {
		t.Fatalf("encrypt into the folder: err = %v, want ErrOutputInsideInput", err)
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatalf("output inside the folder exists (stat err %v)", err)
	}
}

// TestCompressCmdRefusesContradictoryOptions is the regression test for BUG-020: an
// explicit --format that contradicts the output's extension, and a --level outside
// 1–9 (other than -1), are refused before anything is written.
func TestCompressCmdRefusesContradictoryOptions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	run := func(args ...string) error {
		rootCmd := NewRootCmd(newTestDatabase(t, false))
		rootCmd.SetArgs(append([]string{"compress", src}, args...))
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		return rootCmd.Execute()
	}
	for _, tc := range []struct {
		args []string
		want error
	}{
		{[]string{"--format", "gzip", "--output", filepath.Join(dir, "x.zip")}, ErrFormatMismatch},
		{[]string{"--format", "zip", "--output", filepath.Join(dir, "x.tar.gz")}, ErrFormatMismatch},
		{[]string{"--format", "zip", "--output", filepath.Join(dir, "x.gz")}, ErrFormatMismatch},
		{[]string{"--level", "0"}, ErrInvalidLevel},
		{[]string{"--level", "10"}, ErrInvalidLevel},
		{[]string{"--level", "-7"}, ErrInvalidLevel},
	} {
		if err := run(tc.args...); !errors.Is(err, tc.want) {
			t.Fatalf("compress %v: err = %v, want %v", tc.args, err, tc.want)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("folder holds %d entries (err %v), want only a.txt", len(entries), err)
	}
	for _, level := range []string{"-1", "1", "9"} {
		out := filepath.Join(dir, "level"+level+".gz")
		if err := run("--level", level, "--output", out); err != nil {
			t.Fatalf("compress --level %s: %v", level, err)
		}
	}
}

// TestDecryptCmdExplicitEmptyPassword is the regression test for BUG-023: --password ""
// counts as given, so a legacy file encrypted with an empty password decrypts in a
// script without reading stdin.
func TestDecryptCmdExplicitEmptyPassword(t *testing.T) {
	dir := t.TempDir()
	data, err := encryptBytes([]byte("legacy, no password"), "")
	if err != nil {
		t.Fatalf("legacy fixture: %v", err)
	}
	src := filepath.Join(dir, "old.enc")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	dst := filepath.Join(dir, "old.txt")
	rootCmd := NewRootCmd(newTestDatabase(t, false))
	rootCmd.SetArgs([]string{"decrypt", src, "--output", dst, "--password", ""})
	rootCmd.SetIn(bytes.NewReader(nil))
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("decrypt --password \"\": %v", err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "legacy, no password" {
		t.Fatalf("decrypted %q (err %v)", got, err)
	}
}

// TestDecompressCmdRaw checks decompress --raw from the CLI (BUG-022): it writes and
// reports backup.tar for backup.tar.gz, and refuses a zip archive.
func TestDecompressCmdRaw(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "backup.tar")
	if err := os.WriteFile(src, []byte("not really a tarball"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := CompressFile(src, "", -1); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatalf("remove: %v", err)
	}
	run := func(args ...string) (string, error) {
		rootCmd := NewRootCmd(newTestDatabase(t, false))
		rootCmd.SetArgs(args)
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		err := rootCmd.Execute()
		return out.String(), err
	}
	out, err := run("decompress", src+gzExt, "--raw")
	if err != nil {
		t.Fatalf("decompress --raw: %v", err)
	}
	if !strings.Contains(out, "→ "+src) {
		t.Fatalf("output %q doesn't report %s", out, src)
	}
	if got, err := os.ReadFile(src); err != nil || string(got) != "not really a tarball" {
		t.Fatalf("backup.tar = %q (err %v)", got, err)
	}
	zipFile := filepath.Join(dir, "a.zip")
	if err := CompressFileWithFormat(src, zipFile, "zip", -1); err != nil {
		t.Fatalf("zip: %v", err)
	}
	if _, err := run("decompress", zipFile, "--raw", "--output", filepath.Join(dir, "x")); !errors.Is(err, ErrNotGzip) {
		t.Fatalf("decompress a zip with --raw: err = %v, want ErrNotGzip", err)
	}
}
