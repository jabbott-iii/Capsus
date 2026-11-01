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
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestCompressFile tests the CompressFile function by compressing a file with various compression levels and output paths.
// It ensures that the compressed file is created and that its size is smaller than the original for repetitive content.
func TestCompressFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("This is a test file that should be compressed. " +
		"Repeated content makes compression more effective. " +
		"This is a test file that should be compressed.")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	tests := []struct {
		name    string
		dstPath string
		level   int
	}{
		{
			name:    "compress with default output",
			dstPath: "",
			level:   -1,
		},
		{
			name:    "compress with custom output",
			dstPath: filepath.Join(tmpDir, "custom.gz"),
			level:   gzip.DefaultCompression,
		},
		{
			name:    "compress with max compression",
			dstPath: filepath.Join(tmpDir, "max.gz"),
			level:   gzip.BestCompression,
		},
		{
			name:    "compress with best speed",
			dstPath: filepath.Join(tmpDir, "speed.gz"),
			level:   gzip.BestSpeed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compFile := tt.dstPath
			if compFile == "" {
				compFile = srcFile + gzExt
			}

			if err := CompressFile(srcFile, compFile, tt.level); err != nil {
				t.Fatalf("CompressFile failed: %v", err)
			}

			// Verify compressed file exists
			if _, err := os.Stat(compFile); err != nil {
				t.Fatalf("Compressed file not found: %v", err)
			}

			// Verify file is smaller (due to repetitive content)
			origInfo, _ := os.Stat(srcFile)
			compInfo, _ := os.Stat(compFile)
			if compInfo.Size() >= origInfo.Size() {
				t.Logf("Compressed file size (%d) >= original (%d), expected smaller",
					compInfo.Size(), origInfo.Size())
			}
		})
	}
}

// TestCompressDecompressFile tests the CompressFile and DecompressFile functions by compressing and then decompressing a file.
// It ensures that the decompressed content matches the original content.
func TestCompressDecompressFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("Hello, World! This is test content for compression and decompression.")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	tests := []struct {
		name  string
		level int
	}{
		{
			name:  "default compression level",
			level: -1,
		},
		{
			name:  "best speed",
			level: gzip.BestSpeed,
		},
		{
			name:  "best compression",
			level: gzip.BestCompression,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compFile := filepath.Join(tmpDir, "test-"+tt.name+".gz")
			decFile := filepath.Join(tmpDir, "test-"+tt.name+".dec")

			// Compress
			if err := CompressFile(srcFile, compFile, tt.level); err != nil {
				t.Fatalf("CompressFile failed: %v", err)
			}

			// Decompress
			if err := DecompressFile(compFile, decFile); err != nil {
				t.Fatalf("DecompressFile failed: %v", err)
			}

			// Verify decompressed content matches original
			decData, err := os.ReadFile(decFile)
			if err != nil {
				t.Fatalf("Failed to read decompressed file: %v", err)
			}
			if string(decData) != string(originalContent) {
				t.Errorf("Decompressed content mismatch: got %q, want %q",
					string(decData), string(originalContent))
			}
		})
	}
}

// TestCompressDirectoryRoundTrip tests directory compression by writing a
// tar.gz archive and extracting it into a destination directory.
func TestCompressDirectoryRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	nestedDir := filepath.Join(srcDir, "nested")
	emptyDir := filepath.Join(srcDir, "empty")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root data"), 0o644); err != nil {
		t.Fatalf("Failed to write root file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "child.txt"), []byte("nested data"), 0o644); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	archive := srcDir + tarGzExt
	if err := CompressFile(srcDir, "", -1); err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("Expected archive not found: %v", err)
	}

	outDir := filepath.Join(tmpDir, "restored")
	if err := DecompressFile(archive, outDir); err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}

	rootData, err := os.ReadFile(filepath.Join(outDir, "root.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored root file: %v", err)
	}
	if string(rootData) != "root data" {
		t.Fatalf("Root file mismatch: got %q", string(rootData))
	}

	nestedData, err := os.ReadFile(filepath.Join(outDir, "nested", "child.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored nested file: %v", err)
	}
	if string(nestedData) != "nested data" {
		t.Fatalf("Nested file mismatch: got %q", string(nestedData))
	}

	if info, err := os.Stat(filepath.Join(outDir, "empty")); err != nil {
		t.Fatalf("Expected empty directory not restored: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("Restored empty path is not a directory")
	}
}

// TestDecompressFileWithDefaultOutput tests the DecompressFile function when the output path is not specified.
// It ensures that the decompressed file is created with the default output path derived from the compressed file name.
func TestDecompressFileWithDefaultOutput(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("Test data")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	compFile := filepath.Join(tmpDir, "test.txt.gz")
	if err := CompressFile(srcFile, compFile, -1); err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	// Decompress with empty destination (should strip .gz)
	if err := DecompressFile(compFile, ""); err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "test.txt")
	decData, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Expected decompressed file not found: %v", err)
	}

	if string(decData) != string(originalContent) {
		t.Errorf("Content mismatch: got %q, want %q", string(decData), string(originalContent))
	}
}

// TestDecompressTarGzWithDefaultOutput tests that tar.gz archives restore to a
// directory path derived from the archive name when no destination is given.
func TestDecompressTarGzWithDefaultOutput(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("bundle data"), 0o644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	archive := srcDir + tarGzExt
	if err := CompressFile(srcDir, "", -1); err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}
	if err := os.RemoveAll(srcDir); err != nil {
		t.Fatalf("Failed to remove source directory before restore: %v", err)
	}

	if err := DecompressFile(archive, ""); err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(srcDir, "file.txt"))
	if err != nil {
		t.Fatalf("Expected restored file not found: %v", err)
	}
	if string(data) != "bundle data" {
		t.Fatalf("Content mismatch: got %q", string(data))
	}
}

// TestDecompressFileWithoutGzExtension tests the DecompressFile function when the compressed file does not have a .gz extension.
// It ensures that the decompressed file is created with a .dec suffix appended to the original file name.
func TestDecompressFileWithoutGzExtension(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("Test data")

	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	compFile := filepath.Join(tmpDir, "test.compressed")
	if err := CompressFile(srcFile, compFile, -1); err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	// Decompress file without .gz extension
	if err := DecompressFile(compFile, ""); err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "test.compressed.dec")
	decData, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Expected decompressed file not found: %v", err)
	}

	if string(decData) != string(originalContent) {
		t.Errorf("Content mismatch: got %q, want %q", string(decData), string(originalContent))
	}
}

// TestCompressFileInvalidLevel tests the CompressFile function with an invalid compression level.
// It ensures that the function defaults to DefaultCompression when an invalid level is provided.
func TestCompressFileInvalidLevel(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(srcFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Invalid level should default to DefaultCompression
	dstFile := filepath.Join(tmpDir, "test.gz")
	if err := CompressFile(srcFile, dstFile, 99); err != nil {
		t.Fatalf("CompressFile should handle invalid level: %v", err)
	}

	if _, err := os.Stat(dstFile); err != nil {
		t.Fatalf("Output file should be created: %v", err)
	}
}

// TestDecompressInvalidFile tests the DecompressFile function with an invalid gzip file.
// It ensures that the function returns an error when attempting to decompress a non-gzip file.
func TestDecompressInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	fakeGzFile := filepath.Join(tmpDir, "fake.gz")

	if err := os.WriteFile(fakeGzFile, []byte("not a gzip file"), 0o644); err != nil {
		t.Fatalf("Failed to write fake file: %v", err)
	}

	err := DecompressFile(fakeGzFile, filepath.Join(tmpDir, "out.txt"))
	if err == nil {
		t.Error("DecompressFile should fail on invalid gzip file")
	}
}

func TestCompressZipFileRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "single.txt")
	originalContent := []byte("zip single file data")
	if err := os.WriteFile(srcFile, originalContent, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	archive := srcFile + zipExt
	if err := CompressFileWithFormat(srcFile, "", "zip", gzip.BestCompression); err != nil {
		t.Fatalf("CompressFileWithFormat failed: %v", err)
	}

	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatalf("Failed to open zip archive: %v", err)
	}
	if len(reader.File) != 1 {
		_ = reader.Close()
		t.Fatalf("Expected single zip entry, got %d", len(reader.File))
	}
	if reader.File[0].Name != "single.txt" {
		_ = reader.Close()
		t.Fatalf("Unexpected zip entry name: %q", reader.File[0].Name)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Failed to close zip reader: %v", err)
	}

	if err := os.Remove(srcFile); err != nil {
		t.Fatalf("Failed to remove source file: %v", err)
	}
	if err := DecompressFile(archive, ""); err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}

	restored, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(restored) != string(originalContent) {
		t.Fatalf("Content mismatch: got %q", string(restored))
	}
}

func TestCompressZipDirectoryRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "empty"), 0o755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("zip root"), 0o644); err != nil {
		t.Fatalf("Failed to write root file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "child.txt"), []byte("zip nested"), 0o644); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	archive := srcDir + zipExt
	if err := CompressFileWithFormat(srcDir, "", "zip", gzip.BestSpeed); err != nil {
		t.Fatalf("CompressFileWithFormat failed: %v", err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("Expected archive not found: %v", err)
	}

	if err := os.RemoveAll(srcDir); err != nil {
		t.Fatalf("Failed to remove source directory: %v", err)
	}
	if err := DecompressFile(archive, ""); err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}

	rootData, err := os.ReadFile(filepath.Join(srcDir, "root.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored root file: %v", err)
	}
	if string(rootData) != "zip root" {
		t.Fatalf("Root file mismatch: got %q", string(rootData))
	}

	nestedData, err := os.ReadFile(filepath.Join(srcDir, "nested", "child.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored nested file: %v", err)
	}
	if string(nestedData) != "zip nested" {
		t.Fatalf("Nested file mismatch: got %q", string(nestedData))
	}
	if info, err := os.Stat(filepath.Join(srcDir, "empty")); err != nil {
		t.Fatalf("Expected empty directory not restored: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("Restored empty path is not a directory")
	}
}

// TestCheckOutputPath covers the output-path policy shared by the CLI and TUI.
func TestCheckOutputPath(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(src, []byte("input"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	existing := filepath.Join(tmpDir, "existing.out")
	if err := os.WriteFile(existing, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	hardLink := filepath.Join(tmpDir, "hardlink.txt")
	if err := os.Link(src, hardLink); err != nil {
		t.Fatalf("create hard link: %v", err)
	}

	tests := []struct {
		name      string
		dst       string
		overwrite bool
		wantErr   error
	}{
		{name: "new output", dst: filepath.Join(tmpDir, "new.out")},
		{name: "existing output refused", dst: existing, wantErr: ErrOutputExists},
		{name: "existing output with overwrite", dst: existing, overwrite: true},
		{name: "same path", dst: src, wantErr: ErrSameInputOutput},
		{name: "same path even with overwrite", dst: src, overwrite: true, wantErr: ErrSameInputOutput},
		{name: "same file, different spelling", dst: filepath.Join(tmpDir, ".", "input.txt"), overwrite: true, wantErr: ErrSameInputOutput},
		{name: "hard link to the input", dst: hardLink, overwrite: true, wantErr: ErrSameInputOutput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckOutputPath(src, tt.dst, tt.overwrite)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("CheckOutputPath() error = %v, want nil", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("CheckOutputPath() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	symlink := filepath.Join(tmpDir, "symlink.txt")
	if err := os.Symlink(src, symlink); err != nil {
		t.Logf("symlinks unavailable, skipping symlink case: %v", err)
		return
	}
	if err := CheckOutputPath(src, symlink, true); !errors.Is(err, ErrSameInputOutput) {
		t.Fatalf("CheckOutputPath(symlink to input) error = %v, want ErrSameInputOutput", err)
	}
}

// TestOperationsRefuseToOverwriteTheirInput is a regression test for BUG-003: an
// operation whose output is its own input must fail and leave the input untouched.
func TestOperationsRefuseToOverwriteTheirInput(t *testing.T) {
	tmpDir := t.TempDir()
	content := []byte(strings.Repeat("do not destroy me ", 256))

	plain := filepath.Join(tmpDir, "plain.bin")
	gz := filepath.Join(tmpDir, "data.gz")
	enc := filepath.Join(tmpDir, "data.enc")
	zipFile := filepath.Join(tmpDir, "data.zip")
	if err := os.WriteFile(plain, content, 0o600); err != nil {
		t.Fatalf("write plain: %v", err)
	}
	if err := CompressFile(plain, gz, -1); err != nil {
		t.Fatalf("prepare gzip: %v", err)
	}
	if err := EncryptFile(plain, enc, testPassword); err != nil {
		t.Fatalf("prepare encrypted file: %v", err)
	}
	if err := CompressFileWithFormat(plain, zipFile, "zip", -1); err != nil {
		t.Fatalf("prepare zip: %v", err)
	}

	cases := []struct {
		name string
		path string
		run  func(p string) error
	}{
		{"compress", plain, func(p string) error { return CompressFileWithFormat(p, p, "", -1) }},
		{"compress zip", plain, func(p string) error { return CompressFileWithFormat(p, p, "zip", -1) }},
		{"decompress gzip", gz, func(p string) error { return DecompressFile(p, p) }},
		{"decompress zip", zipFile, func(p string) error { return DecompressFile(p, p) }},
		{"encrypt", plain, func(p string) error { return EncryptFile(p, p, testPassword) }},
		{"decrypt", enc, func(p string) error { return DecryptFile(p, p, testPassword) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			if err := tc.run(tc.path); !errors.Is(err, ErrSameInputOutput) {
				t.Errorf("error = %v, want ErrSameInputOutput", err)
			}
			after, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s after: %v", tc.path, err)
			}
			if string(after) != string(before) {
				t.Errorf("%s was modified (%d bytes before, %d after)", tc.path, len(before), len(after))
			}
		})
	}
}

// TestCompressDirectoryRejectsOutputInsideInput checks that a directory can't be
// compressed into an archive inside itself, which would archive its own partial output.
func TestCompressDirectoryRejectsOutputInsideInput(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	for _, format := range []string{"gzip", "zip"} {
		dst := filepath.Join(srcDir, "self."+format)
		if err := CompressFileWithFormat(srcDir, dst, format, -1); !errors.Is(err, ErrOutputInsideInput) {
			t.Errorf("%s: error = %v, want ErrOutputInsideInput", format, err)
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Errorf("%s: archive written inside the input folder (stat err: %v)", format, err)
		}
	}
}

// tempLeftovers lists hidden temporary files (".<name>.*.tmp") left in dir.
func tempLeftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && strings.HasSuffix(e.Name(), ".tmp") {
			names = append(names, e.Name())
		}
	}
	return names
}

// TestFailedDecompressLeavesNoPartialOutput is a regression test for BUG-004: a
// decompression that fails part-way must not leave a partial output, and must not
// damage an existing output it was allowed to replace.
func TestFailedDecompressLeavesNoPartialOutput(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "random.bin")
	data := make([]byte, 64*1024)
	for i := range data {
		data[i] = byte(i*7919 + i/13) // varied, poorly compressible content
	}
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	gz := src + gzExt
	if err := CompressFile(src, gz, -1); err != nil {
		t.Fatalf("compress: %v", err)
	}
	info, err := os.Stat(gz)
	if err != nil {
		t.Fatalf("stat gzip: %v", err)
	}
	if err := os.Truncate(gz, info.Size()/2); err != nil {
		t.Fatalf("truncate gzip: %v", err)
	}

	newOut := filepath.Join(tmpDir, "new-output.bin")
	if err := DecompressFile(gz, newOut); err == nil {
		t.Fatal("DecompressFile of a truncated gzip succeeded, want an error")
	}
	if _, err := os.Stat(newOut); !os.IsNotExist(err) {
		t.Fatalf("partial output left behind (stat err: %v)", err)
	}

	existing := filepath.Join(tmpDir, "existing.bin")
	if err := os.WriteFile(existing, []byte("previous contents"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	if err := DecompressFile(gz, existing); err == nil {
		t.Fatal("DecompressFile of a truncated gzip succeeded, want an error")
	}
	if got, _ := os.ReadFile(existing); string(got) != "previous contents" {
		t.Fatalf("existing output damaged by a failed run: %d bytes", len(got))
	}
	if left := tempLeftovers(t, tmpDir); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}

// TestFailedCompressLeavesNoPartialOutput is a regression test for BUG-004: when
// compressing a folder fails part-way (here on a symlink, which is refused), no
// partial archive may be left and an existing archive must survive.
func TestFailedCompressLeavesNoPartialOutput(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "folder")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte(strings.Repeat("data ", 1000)), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Symlink(filepath.Join(srcDir, "a.txt"), filepath.Join(srcDir, "z-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	for _, format := range []string{"gzip", "zip"} {
		t.Run(format, func(t *testing.T) {
			newOut := filepath.Join(tmpDir, "new."+format)
			if err := CompressFileWithFormat(srcDir, newOut, format, -1); err == nil {
				t.Fatal("compressing a folder with a symlink succeeded, want an error")
			}
			if _, err := os.Stat(newOut); !os.IsNotExist(err) {
				t.Fatalf("partial archive left behind (stat err: %v)", err)
			}

			existing := filepath.Join(tmpDir, "existing."+format)
			if err := os.WriteFile(existing, []byte("previous archive"), 0o600); err != nil {
				t.Fatalf("write existing output: %v", err)
			}
			if err := CompressFileWithFormat(srcDir, existing, format, -1); err == nil {
				t.Fatal("compressing a folder with a symlink succeeded, want an error")
			}
			if got, _ := os.ReadFile(existing); string(got) != "previous archive" {
				t.Fatalf("existing archive damaged by a failed run: %d bytes", len(got))
			}
			if left := tempLeftovers(t, tmpDir); len(left) != 0 {
				t.Fatalf("temporary files left behind: %v", left)
			}
		})
	}
}

// TestWriteFileAtomic checks the write-then-rename helper used for single-file outputs.
func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "out.bin")

	if err := writeFileAtomic(dst, []byte("first")); err != nil {
		t.Fatalf("writeFileAtomic() error = %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "first" {
		t.Fatalf("content = %q, want %q", got, "first")
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(dst); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v (err %v), want 0600", info.Mode().Perm(), err)
		}
	}

	if err := writeFileAtomic(dst, []byte("second")); err != nil {
		t.Fatalf("writeFileAtomic() replace error = %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "second" {
		t.Fatalf("content after replace = %q, want %q", got, "second")
	}

	// An aborted write leaves the existing file as it was and removes the temp file.
	f, err := createAtomicFile(dst)
	if err != nil {
		t.Fatalf("createAtomicFile() error = %v", err)
	}
	if _, err := f.Write([]byte("never committed")); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	f.Abort()
	if got, _ := os.ReadFile(dst); string(got) != "second" {
		t.Fatalf("content after abort = %q, want %q", got, "second")
	}
	if left := tempLeftovers(t, tmpDir); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}

	if err := writeFileAtomic(filepath.Join(tmpDir, "missing-dir", "out.bin"), []byte("x")); err == nil {
		t.Fatal("writeFileAtomic() into a missing folder succeeded, want an error")
	}
}

// bombFixtures builds small, highly compressible archives in dir that expand to
// payload bytes of zeros: a .gz, a .tar.gz and a .zip of a folder, and a single-file
// .zip. It returns their paths keyed by kind, plus the output path to use for each.
func bombFixtures(t *testing.T, dir string, payload int) map[string][2]string {
	t.Helper()
	zeros := make([]byte, payload)
	src := filepath.Join(dir, "zeros.bin")
	if err := os.WriteFile(src, zeros, 0o600); err != nil {
		t.Fatalf("write zeros: %v", err)
	}
	tree := filepath.Join(dir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatalf("create tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "zeros.bin"), zeros, 0o600); err != nil {
		t.Fatalf("write tree file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatalf("create output folder: %v", err)
	}

	fixtures := map[string][2]string{
		"gzip":            {filepath.Join(dir, "zeros.bin.gz"), filepath.Join(dir, "out", "zeros.bin")},
		"tar.gz":          {filepath.Join(dir, "tree.tar.gz"), filepath.Join(dir, "out", "tree")},
		"zip":             {filepath.Join(dir, "tree.zip"), filepath.Join(dir, "out", "tree")},
		"single-file zip": {filepath.Join(dir, "zeros.zip"), filepath.Join(dir, "out", "zeros.bin")},
	}
	if err := CompressFile(src, fixtures["gzip"][0], -1); err != nil {
		t.Fatalf("prepare gzip: %v", err)
	}
	if err := CompressFileWithFormat(tree, fixtures["tar.gz"][0], "", -1); err != nil {
		t.Fatalf("prepare tar.gz: %v", err)
	}
	if err := CompressFileWithFormat(tree, fixtures["zip"][0], "zip", -1); err != nil {
		t.Fatalf("prepare zip: %v", err)
	}
	if err := CompressFileWithFormat(src, fixtures["single-file zip"][0], "zip", -1); err != nil {
		t.Fatalf("prepare single-file zip: %v", err)
	}
	return fixtures
}

// TestDecompressEnforcesSizeLimit is a regression test for SEC-007: decompression and
// extraction stop once the output passes ExtractLimits.MaxBytes, report
// ErrExtractLimit and leave no output behind. Output up to the limit is allowed.
func TestDecompressEnforcesSizeLimit(t *testing.T) {
	const payload = 1 << 20 // 1 MiB of zeros compresses to about 1 KiB
	for kind := range bombFixtures(t, t.TempDir(), 1) {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			f := bombFixtures(t, dir, payload)[kind]
			archive, out := f[0], f[1]

			err := DecompressFileWithLimits(archive, out, ExtractLimits{MaxBytes: 64 << 10})
			if !errors.Is(err, ErrExtractLimit) {
				t.Fatalf("error = %v, want ErrExtractLimit", err)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("output left behind after the limit was hit (stat err: %v)", err)
			}
			if left := tempLeftovers(t, filepath.Dir(out)); len(left) != 0 {
				t.Fatalf("temporary files left behind: %v", left)
			}

			if err := DecompressFileWithLimits(archive, out, ExtractLimits{MaxBytes: payload}); err != nil {
				t.Fatalf("output exactly at the limit: %v", err)
			}
		})
	}
}

// TestExtractEnforcesEntryLimit is a regression test for SEC-007: extraction stops when
// an archive has more entries than ExtractLimits.MaxEntries. Zero means no limit.
func TestExtractEnforcesEntryLimit(t *testing.T) {
	tmpDir := t.TempDir()
	tree := filepath.Join(tmpDir, "many")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatalf("create tree: %v", err)
	}
	for i := range 20 {
		if err := os.WriteFile(filepath.Join(tree, fmt.Sprintf("f%02d.txt", i)), []byte("x"), 0o600); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	for _, format := range []string{"", "zip"} {
		name := "tar.gz"
		archive := filepath.Join(tmpDir, "many.tar.gz")
		if format == "zip" {
			name, archive = "zip", filepath.Join(tmpDir, "many.zip")
		}
		t.Run(name, func(t *testing.T) {
			if err := CompressFileWithFormat(tree, archive, format, -1); err != nil {
				t.Fatalf("prepare archive: %v", err)
			}
			out := filepath.Join(t.TempDir(), "restored")
			err := DecompressFileWithLimits(archive, out, ExtractLimits{MaxEntries: 5})
			if !errors.Is(err, ErrExtractLimit) {
				t.Fatalf("error = %v, want ErrExtractLimit", err)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("output left behind after the limit was hit (stat err: %v)", err)
			}
			if err := DecompressFileWithLimits(archive, out, ExtractLimits{}); err != nil {
				t.Fatalf("without limits: %v", err)
			}
			if entries, err := os.ReadDir(out); err != nil || len(entries) != 20 {
				t.Fatalf("restored %d entries (err %v), want 20", len(entries), err)
			}
		})
	}
}

// TestDefaultExtractLimits pins the default limits (plan 2.1): 10 GiB and 100,000
// entries per run.
func TestDefaultExtractLimits(t *testing.T) {
	got := DefaultExtractLimits()
	if got.MaxBytes != 10<<30 || got.MaxEntries != 100_000 {
		t.Fatalf("DefaultExtractLimits() = %+v, want 10 GiB and 100000 entries", got)
	}
}

// TestFailedExtractionLeavesNoPartialOutput checks that a corrupt archive leaves no
// partial folder behind, and that a failed run with an existing output folder (the
// --force case) leaves that folder as it was.
func TestFailedExtractionLeavesNoPartialOutput(t *testing.T) {
	tmpDir := t.TempDir()
	tree := filepath.Join(tmpDir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatalf("create tree: %v", err)
	}
	data := make([]byte, 256*1024)
	for i := range data {
		data[i] = byte(i*7919 + i/13) // poorly compressible, so truncation cuts file data
	}
	for _, name := range []string{"a.bin", "b.bin"} {
		if err := os.WriteFile(filepath.Join(tree, name), data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	for _, ext := range []string{tarGzExt, zipExt} {
		t.Run(ext, func(t *testing.T) {
			archive := filepath.Join(tmpDir, "tree"+ext)
			if err := CompressFileWithFormat(tree, archive, "", -1); err != nil {
				t.Fatalf("prepare archive: %v", err)
			}
			if ext == tarGzExt {
				// Truncating a tar.gz corrupts the stream after the first file.
				info, err := os.Stat(archive)
				if err != nil {
					t.Fatalf("stat archive: %v", err)
				}
				if err := os.Truncate(archive, info.Size()*3/4); err != nil {
					t.Fatalf("truncate archive: %v", err)
				}
			} else {
				// Corrupt the second entry's compressed data; the central directory stays valid.
				raw, err := os.ReadFile(archive)
				if err != nil {
					t.Fatalf("read archive: %v", err)
				}
				for i := len(raw) * 3 / 4; i < len(raw)*3/4+64; i++ {
					raw[i] ^= 0xff
				}
				if err := os.WriteFile(archive, raw, 0o600); err != nil {
					t.Fatalf("write archive: %v", err)
				}
			}

			outDir := t.TempDir()
			newOut := filepath.Join(outDir, "new")
			if err := DecompressFile(archive, newOut); err == nil {
				t.Fatal("DecompressFile of a corrupt archive succeeded, want an error")
			}
			if _, err := os.Stat(newOut); !os.IsNotExist(err) {
				t.Fatalf("partial output left behind (stat err: %v)", err)
			}

			existing := filepath.Join(outDir, "existing")
			if err := os.MkdirAll(existing, 0o755); err != nil {
				t.Fatalf("create existing output: %v", err)
			}
			if err := os.WriteFile(filepath.Join(existing, "keep.txt"), []byte("keep"), 0o600); err != nil {
				t.Fatalf("write existing file: %v", err)
			}
			if err := DecompressFile(archive, existing); err == nil {
				t.Fatal("DecompressFile of a corrupt archive succeeded, want an error")
			}
			if entries, _ := os.ReadDir(existing); len(entries) != 1 {
				t.Fatalf("existing output changed by a failed run: %d entries", len(entries))
			}
			if left := tempLeftovers(t, outDir); len(left) != 0 {
				t.Fatalf("temporary folders left behind: %v", left)
			}
		})
	}
}

// TestExtractReplacesExistingOutput checks the --force behaviour of extraction: an
// existing output folder is replaced as a whole rather than merged into. This is also
// a regression test for SEC-008: a symlink planted in that folder is not followed.
func TestExtractReplacesExistingOutput(t *testing.T) {
	tmpDir := t.TempDir()
	tree := filepath.Join(tmpDir, "tree")
	if err := os.MkdirAll(filepath.Join(tree, "link"), 0o755); err != nil {
		t.Fatalf("create tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "link", "escaped.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("write tree file: %v", err)
	}

	for _, ext := range []string{tarGzExt, zipExt} {
		t.Run(ext, func(t *testing.T) {
			archive := filepath.Join(tmpDir, "tree"+ext)
			if err := CompressFileWithFormat(tree, archive, "", -1); err != nil {
				t.Fatalf("prepare archive: %v", err)
			}
			base := t.TempDir()
			outside := filepath.Join(base, "outside")
			if err := os.MkdirAll(outside, 0o755); err != nil {
				t.Fatalf("create outside dir: %v", err)
			}
			out := filepath.Join(base, "out")
			if err := os.MkdirAll(out, 0o755); err != nil {
				t.Fatalf("create output dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(out, "stale.txt"), []byte("old"), 0o600); err != nil {
				t.Fatalf("write stale file: %v", err)
			}
			if err := os.Symlink(outside, filepath.Join(out, "link")); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlinks unavailable: %v", err)
				}
				t.Fatalf("create symlink: %v", err)
			}

			if err := DecompressFile(archive, out); err != nil {
				t.Fatalf("DecompressFile into an existing folder: %v", err)
			}
			if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !os.IsNotExist(err) {
				t.Fatalf("extraction followed a symlink out of the output folder (stat err: %v)", err)
			}
			if got, err := os.ReadFile(filepath.Join(out, "link", "escaped.txt")); err != nil || string(got) != "payload" {
				t.Fatalf("extracted file = %q (err %v), want %q", got, err, "payload")
			}
			if info, err := os.Lstat(filepath.Join(out, "link")); err != nil || !info.IsDir() {
				t.Fatalf("link is not a real directory after extraction (err %v)", err)
			}
			if _, err := os.Stat(filepath.Join(out, "stale.txt")); !os.IsNotExist(err) {
				t.Fatalf("stale file kept: the output was merged into, not replaced (stat err: %v)", err)
			}
			if left := tempLeftovers(t, base); len(left) != 0 {
				t.Fatalf("temporary folders left behind: %v", left)
			}
		})
	}
}

// TestExtractRefusesToReplaceFolderHoldingInput checks that replacing an existing
// output folder is refused when the archive being extracted is inside it, since
// replacing the folder would delete the archive.
func TestExtractRefusesToReplaceFolderHoldingInput(t *testing.T) {
	tmpDir := t.TempDir()
	tree := filepath.Join(tmpDir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatalf("create tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write tree file: %v", err)
	}
	out := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}
	archive := filepath.Join(out, "tree.tar.gz")
	if err := CompressFileWithFormat(tree, archive, "", -1); err != nil {
		t.Fatalf("prepare archive: %v", err)
	}

	if err := DecompressFile(archive, out); !errors.Is(err, ErrInputInsideOutput) {
		t.Fatalf("error = %v, want ErrInputInsideOutput", err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archive lost: %v", err)
	}
}

// TestDecryptDirectoryEnforcesExtractLimits is a regression test for SEC-007 on the
// decrypt path: an encrypted directory is extracted under the same limits.
func TestDecryptDirectoryEnforcesExtractLimits(t *testing.T) {
	tmpDir := t.TempDir()
	tree := filepath.Join(tmpDir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatalf("create tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "zeros.bin"), make([]byte, 1<<20), 0o600); err != nil {
		t.Fatalf("write tree file: %v", err)
	}
	enc := filepath.Join(tmpDir, "tree.enc")
	if err := EncryptFile(tree, enc, testPassword); err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	out := filepath.Join(tmpDir, "restored")
	err := DecryptFileWithLimits(enc, out, testPassword, ExtractLimits{MaxBytes: 64 << 10})
	if !errors.Is(err, ErrExtractLimit) {
		t.Fatalf("error = %v, want ErrExtractLimit", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output left behind after the limit was hit (stat err: %v)", err)
	}
	if err := DecryptFileWithLimits(enc, out, testPassword, DefaultExtractLimits()); err != nil {
		t.Fatalf("decrypt with default limits: %v", err)
	}
}

// TestFormatSize checks the sizes shown in limit errors.
func TestFormatSize(t *testing.T) {
	tests := map[int64]string{
		10 << 30:    "10 GiB",
		500_000_000: "500 MB",
		64 << 10:    "64 KiB",
		1 << 40:     "1 TiB",
		1000:        "1 KB",
		1023:        "1023 bytes",
	}
	for n, want := range tests {
		if got := formatSize(n); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// archiveEntry describes one entry of a hand-built test archive.
type archiveEntry struct {
	name string
	mode os.FileMode // permission bits stored in the archive
	dir  bool
	body string
}

// writeTestTarGz writes entries to a new tar.gz at path, exactly as given.
func writeTestTarGz(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	gz := gzip.NewWriter(f)
	gz.Name = strings.TrimSuffix(filepath.Base(path), ".gz")
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: int64(e.mode), Typeflag: tar.TypeReg, Size: int64(len(e.body))}
		if e.dir {
			h.Typeflag, h.Size = tar.TypeDir, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("write tar header %s: %v", e.name, err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatalf("write tar entry %s: %v", e.name, err)
		}
	}
	for _, c := range []interface{ Close() error }{tw, gz, f} {
		if err := c.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
	}
}

// writeTestZip writes entries to a new zip at path, exactly as given.
func writeTestZip(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		if e.dir {
			mode |= os.ModeDir
		}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatalf("write zip header %s: %v", e.name, err)
		}
		if !e.dir {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatalf("write zip entry %s: %v", e.name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

// TestExtractMasksArchivePermissions is a regression test for SEC-008 step 3: extracted
// folders are 0700 and files 0600, or 0700 when the archive marks them executable,
// whatever permissions the archive stores. A read-only folder in the archive doesn't
// stop its contents from being extracted.
func TestExtractMasksArchivePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply on Windows")
	}
	entries := []archiveEntry{
		{name: "open/", mode: 0o777, dir: true},
		{name: "open/shared.txt", mode: 0o666, body: "shared"},
		{name: "open/tool.sh", mode: 0o775, body: "#!/bin/sh\n"},
		{name: "locked/", mode: 0o500, dir: true},
		{name: "locked/inner.txt", mode: 0o444, body: "inner"},
	}
	want := map[string]os.FileMode{
		"open":             0o700,
		"open/shared.txt":  0o600,
		"open/tool.sh":     0o700,
		"locked":           0o700,
		"locked/inner.txt": 0o600,
	}

	for _, ext := range []string{tarGzExt, zipExt} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "perms"+ext)
			if ext == tarGzExt {
				writeTestTarGz(t, archive, entries)
			} else {
				writeTestZip(t, archive, entries)
			}
			out := filepath.Join(dir, "out")
			if err := DecompressFile(archive, out); err != nil {
				t.Fatalf("DecompressFile: %v", err)
			}
			for name, mode := range want {
				info, err := os.Stat(filepath.Join(out, filepath.FromSlash(name)))
				if err != nil {
					t.Fatalf("stat %s: %v", name, err)
				}
				if got := info.Mode().Perm(); got != mode {
					t.Errorf("%s mode = %04o, want %04o", name, got, mode)
				}
			}
		})
	}
}

// TestExtractCreatesPrivateParentFolders checks that missing parent folders of the
// output are created owner-only (0700), for folder archives (extractToDir) and for a
// single-file zip (extractZipSingleFile).
func TestExtractCreatesPrivateParentFolders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply on Windows")
	}
	folder := []archiveEntry{
		{name: "docs/", mode: 0o755, dir: true},
		{name: "docs/a.txt", mode: 0o644, body: "a"},
	}
	cases := []struct {
		name    string
		archive string
		entries []archiveEntry
		output  string
	}{
		{"tar.gz folder", "docs" + tarGzExt, folder, "docs"},
		{"zip folder", "docs" + zipExt, folder, "docs"},
		{"zip single file", "note.txt" + zipExt, []archiveEntry{{name: "note.txt", mode: 0o644, body: "note"}}, "note.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, tc.archive)
			if strings.HasSuffix(tc.archive, zipExt) {
				writeTestZip(t, archive, tc.entries)
			} else {
				writeTestTarGz(t, archive, tc.entries)
			}
			out := filepath.Join(dir, "new", "nested", tc.output)
			if err := DecompressFile(archive, out); err != nil {
				t.Fatalf("DecompressFile: %v", err)
			}
			for _, parent := range []string{filepath.Join(dir, "new"), filepath.Join(dir, "new", "nested")} {
				info, err := os.Stat(parent)
				if err != nil {
					t.Fatalf("stat %s: %v", parent, err)
				}
				if got := info.Mode().Perm(); got != 0o700 {
					t.Errorf("%s mode = %04o, want 0700", parent, got)
				}
			}
		})
	}
}

// TestExtractRejectsPathTraversal checks that an archive entry naming a path outside
// the output folder is refused, nothing is written outside it, and no partial
// output is left behind.
func TestExtractRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../escaped.txt", "a/../../escaped.txt"} {
		for _, ext := range []string{tarGzExt, zipExt} {
			t.Run(ext+" "+name, func(t *testing.T) {
				dir := t.TempDir()
				archive := filepath.Join(dir, "evil"+ext)
				entries := []archiveEntry{
					{name: "ok.txt", mode: 0o644, body: "fine"},
					{name: name, mode: 0o644, body: "escaped"},
				}
				if ext == tarGzExt {
					writeTestTarGz(t, archive, entries)
				} else {
					writeTestZip(t, archive, entries)
				}
				out := filepath.Join(dir, "sub", "out")
				err := DecompressFile(archive, out)
				if err == nil || !strings.Contains(err.Error(), "invalid path") {
					t.Fatalf("error = %v, want an invalid-path error", err)
				}
				for _, p := range []string{filepath.Join(dir, "escaped.txt"), filepath.Join(dir, "sub", "escaped.txt")} {
					if _, err := os.Stat(p); !os.IsNotExist(err) {
						t.Fatalf("%s written outside the output folder (stat err: %v)", p, err)
					}
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatalf("partial output left behind (stat err: %v)", err)
				}
			})
		}
	}
}

// TestExtractAbsoluteEntryStaysInside checks that a tar entry with an absolute name is
// extracted inside the output folder, as before extraction went through os.Root.
func TestExtractAbsoluteEntryStaysInside(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "abs.tar.gz")
	writeTestTarGz(t, archive, []archiveEntry{{name: "/etc/cryptare-test.txt", mode: 0o644, body: "inside"}})
	out := filepath.Join(dir, "out")
	if err := DecompressFile(archive, out); err != nil {
		t.Fatalf("DecompressFile: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(out, "etc", "cryptare-test.txt")); err != nil || string(got) != "inside" {
		t.Fatalf("extracted file = %q (err %v), want %q inside the output folder", got, err, "inside")
	}
}

// TestArchivingIgnoresFileSwappedForSymlink is a regression test for SEC-014: if a file
// in the tree is swapped for a symlink to a file outside the tree after it was walked
// but before it is opened, compressing or encrypting the tree must not read the
// outside file.
func TestArchivingIgnoresFileSwappedForSymlink(t *testing.T) {
	const secret = "TOP-SECRET-OUTSIDE-THE-TREE"
	operations := []struct {
		name string
		run  func(tree, out string) error
		ext  string
	}{
		{"tar.gz", func(tree, out string) error { return CompressFileWithFormat(tree, out, "", -1) }, tarGzExt},
		{"zip", func(tree, out string) error { return CompressFileWithFormat(tree, out, "zip", -1) }, zipExt},
		{"encrypt", func(tree, out string) error { return EncryptFile(tree, out, testPassword) }, encExt},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			dir := t.TempDir()
			outside := filepath.Join(dir, "outside.txt")
			if err := os.WriteFile(outside, []byte(secret), 0o600); err != nil {
				t.Fatalf("write outside file: %v", err)
			}
			tree := filepath.Join(dir, "tree")
			if err := os.MkdirAll(tree, 0o755); err != nil {
				t.Fatalf("create tree: %v", err)
			}
			victim := filepath.Join(tree, "a.txt")
			// Same length as the secret, so a tar header written from the walked size
			// still matches after the swap.
			if err := os.WriteFile(victim, bytes.Repeat([]byte("h"), len(secret)), 0o600); err != nil {
				t.Fatalf("write tree file: %v", err)
			}
			// Symlink support is checked up front so the hook can't fail half-way.
			if err := os.Symlink(outside, filepath.Join(dir, "probe")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			swapped := false
			testHookBeforeArchiveOpen = func(path string) {
				if filepath.Clean(path) != victim || swapped {
					return
				}
				swapped = true
				if err := os.Remove(victim); err != nil {
					t.Errorf("swap: remove: %v", err)
				}
				if err := os.Symlink(outside, victim); err != nil {
					t.Errorf("swap: symlink: %v", err)
				}
			}
			t.Cleanup(func() { testHookBeforeArchiveOpen = nil })

			out := filepath.Join(dir, "out"+op.ext)
			err := op.run(tree, out)
			if !swapped {
				t.Fatal("the swap hook never ran")
			}
			if err == nil {
				t.Fatal("archiving succeeded after the file was swapped for a symlink out of the tree, want an error")
			}
			if data, readErr := os.ReadFile(out); readErr == nil && bytes.Contains(data, []byte(secret)) {
				t.Fatal("the outside file's contents reached the output")
			}
			if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
				t.Fatalf("output written despite the error (stat err: %v)", statErr)
			}
		})
	}
}

// TestExtensionsIgnoreCase is a regression test for BUG-007: archive and encrypted-file
// extensions are recognised in any letter case, both for default output names and for
// deciding that a .tar.gz is a folder archive.
func TestExtensionsIgnoreCase(t *testing.T) {
	names := map[string]string{
		"FOO.ZIP":         "FOO",
		"data.TAR.GZ":     "data",
		"data.Tar.Gz":     "data",
		"backup.TGZ":      "backup",
		"notes.txt.GZ":    "notes.txt",
		"plain.tar.gz":    "plain",
		"no-extension":    "no-extension.dec",
		"archive.tar.gzx": "archive.tar.gzx.dec",
	}
	for in, want := range names {
		if got := defaultDecompressOutput(in); got != want {
			t.Errorf("defaultDecompressOutput(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"SECRET.ENC", "secret.Enc", "secret.enc"} {
		if got, want := deriveDecryptOutput(in), in[:len(in)-len(encExt)]; got != want {
			t.Errorf("deriveDecryptOutput(%q) = %q, want %q", in, got, want)
		}
	}

	// An upper-case .TAR.GZ whose gzip header names "ARCHIVE.TAR" is a folder archive.
	dir := t.TempDir()
	archive := filepath.Join(dir, "ARCHIVE.TAR.GZ")
	writeTestTarGz(t, archive, []archiveEntry{{name: "inner.txt", mode: 0o644, body: "inside"}})
	if err := DecompressFile(archive, ""); err != nil {
		t.Fatalf("DecompressFile: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "ARCHIVE", "inner.txt")); err != nil || string(got) != "inside" {
		t.Fatalf("extracted file = %q (err %v), want the archive extracted into ARCHIVE/", got, err)
	}

	// DecryptFile's own default output strips an upper-case .ENC too.
	plain := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(plain, []byte("note"), 0o600); err != nil {
		t.Fatalf("write note: %v", err)
	}
	enc := filepath.Join(dir, "NOTE.ENC")
	if err := EncryptFile(plain, enc, testPassword); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}
	if err := DecryptFile(enc, "", testPassword); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "NOTE")); err != nil || string(got) != "note" {
		t.Fatalf("decrypted file = %q (err %v), want NOTE", got, err)
	}
}

// TestNonLatin1NamesRoundTrip is the regression test for BUG-015: a file or folder
// whose name Latin-1 can't hold is gzip-compressed, archived as tar.gz and encrypted,
// and each comes back intact.
func TestNonLatin1NamesRoundTrip(t *testing.T) {
	const password = "correct horse battery staple"
	for _, name := range []string{"日本語", "данные", "lock-🔒"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite := func(path, content string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}
			mustRead := func(path, want string) {
				t.Helper()
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				if string(got) != want {
					t.Fatalf("%s = %q, want %q", path, got, want)
				}
			}

			file := filepath.Join(dir, name+".txt")
			mustWrite(file, "file "+name)
			if err := CompressFile(file, "", gzip.DefaultCompression); err != nil {
				t.Fatalf("gzip a file: %v", err)
			}
			unzipped := filepath.Join(dir, "unzipped-"+name+".txt")
			if err := DecompressFile(file+gzExt, unzipped); err != nil {
				t.Fatalf("gunzip a file: %v", err)
			}
			mustRead(unzipped, "file "+name)

			folder := filepath.Join(dir, name)
			if err := os.Mkdir(folder, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			mustWrite(filepath.Join(folder, name+"-inner.txt"), "inner "+name)
			inner := name + "-inner.txt"

			// A plain ".gz" name relies on the gzip header to mark the tarball.
			for i, archive := range []string{folder + tarGzExt, filepath.Join(dir, "plain"+gzExt)} {
				if err := CompressFile(folder, archive, gzip.DefaultCompression); err != nil {
					t.Fatalf("tar.gz a folder as %s: %v", filepath.Base(archive), err)
				}
				out := filepath.Join(dir, fmt.Sprintf("extracted-%d", i))
				if err := DecompressFile(archive, out); err != nil {
					t.Fatalf("extract %s: %v", filepath.Base(archive), err)
				}
				mustRead(filepath.Join(out, inner), "inner "+name)
			}

			encrypted := filepath.Join(dir, "folder.enc")
			if err := EncryptFile(folder, encrypted, password); err != nil {
				t.Fatalf("encrypt a folder: %v", err)
			}
			restored := filepath.Join(dir, "restored")
			if err := DecryptFile(encrypted, restored, password); err != nil {
				t.Fatalf("decrypt a folder: %v", err)
			}
			mustRead(filepath.Join(restored, inner), "inner "+name)
		})
	}
}

// TestGzipHeaderName checks which names a gzip header keeps (BUG-015).
func TestGzipHeaderName(t *testing.T) {
	tests := []struct{ name, want string }{
		{"notes.txt", "notes.txt"},
		{"résumé.txt", "résumé.txt"}, // Latin-1
		{"日本語.txt", "fallback"},
		{"lock-🔒.txt", "fallback"},
		{"bad\xff.txt", "fallback"}, // invalid UTF-8
	}
	for _, tc := range tests {
		if got := gzipHeaderName(tc.name, "fallback"); got != tc.want {
			t.Errorf("gzipHeaderName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := gzipFolderName("/tmp/данные/"); got != fallbackTarName {
		t.Errorf("gzipFolderName = %q, want %q", got, fallbackTarName)
	}
	if got := gzipFolderName("/tmp/photos/"); got != "photos.tar" {
		t.Errorf("gzipFolderName = %q, want photos.tar", got)
	}
}

// caseInsensitiveFS reports whether dir is on a file system that ignores letter case,
// as macOS and Windows file systems do by default.
func caseInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	defer func() { _ = os.Remove(probe) }()
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// TestContainmentChecksUseFileIdentity is the regression test for BUG-019: an output
// inside the folder being compressed or encrypted is refused when it is spelled
// through a symlink, or in another letter case on a case-insensitive file system, and
// so is replacing a folder that holds the archive under another spelling.
func TestContainmentChecksUseFileIdentity(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	if err := os.Mkdir(tree, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	onlyOriginal := func() {
		t.Helper()
		entries, err := os.ReadDir(tree)
		if err != nil {
			t.Fatalf("read tree: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("tree holds %d entries, want only a.txt", len(entries))
		}
	}
	refusesOutputsUnder := func(t *testing.T, spelling string) {
		t.Helper()
		if err := CompressFile(tree, filepath.Join(spelling, "self.tar.gz"), -1); !errors.Is(err, ErrOutputInsideInput) {
			t.Fatalf("compress into %s: err = %v, want ErrOutputInsideInput", spelling, err)
		}
		if err := EncryptFile(tree, filepath.Join(spelling, "self.enc"), testPassword); !errors.Is(err, ErrOutputInsideInput) {
			t.Fatalf("encrypt into %s: err = %v, want ErrOutputInsideInput", spelling, err)
		}
		onlyOriginal()
	}

	t.Run("symlinked spelling", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("creating symlinks needs extra privileges on Windows")
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(tree, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		refusesOutputsUnder(t, link)
	})

	t.Run("letter case", func(t *testing.T) {
		if !caseInsensitiveFS(t, dir) {
			t.Skip("the file system is case-sensitive")
		}
		refusesOutputsUnder(t, filepath.Join(dir, "TREE"))

		// Replacing a folder that holds the archive being extracted.
		out := filepath.Join(dir, "out")
		if err := os.Mkdir(out, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		archive := filepath.Join(out, "tree.tar.gz")
		if err := CompressFile(tree, archive, -1); err != nil {
			t.Fatalf("compress: %v", err)
		}
		if err := DecompressFile(archive, filepath.Join(dir, "OUT")); !errors.Is(err, ErrInputInsideOutput) {
			t.Fatalf("extract over the folder holding the archive: err = %v, want ErrInputInsideOutput", err)
		}
		if _, err := os.Stat(archive); err != nil {
			t.Fatalf("the archive is gone: %v", err)
		}
	})
}

// TestLongNamesFitTemporaryFiles is the regression test for BUG-021: outputs whose
// names are close to the 255-byte limit are written, because their temporary files and
// folders use a shortened name.
func TestLongNamesFitTemporaryFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows counts name length in UTF-16 units and limits paths differently")
	}
	name := strings.Repeat("n", 247) // outputs reach 250–254 bytes
	newDir := func() string { return t.TempDir() }

	src := newDir()
	file := filepath.Join(src, name)
	if err := os.WriteFile(file, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := EncryptFile(file, "", testPassword); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := DecryptFile(file+encExt, filepath.Join(newDir(), name), testPassword); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if err := CompressFile(file, "", -1); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	if err := DecompressFile(file+gzExt, filepath.Join(newDir(), name)); err != nil {
		t.Fatalf("gunzip: %v", err)
	}

	folder := filepath.Join(newDir(), name)
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "inner.txt"), []byte("inner"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := CompressFile(folder, "", -1); err != nil {
		t.Fatalf("tar.gz: %v", err)
	}
	out := filepath.Join(newDir(), name)
	if err := DecompressFile(folder+tarGzExt, out); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(out, "inner.txt")); err != nil || string(got) != "inner" {
		t.Fatalf("extracted inner.txt = %q (err %v)", got, err)
	}
}

// TestTempNamePart checks that a long name is cut at a UTF-8 boundary (BUG-021).
func TestTempNamePart(t *testing.T) {
	if got := tempNamePart("short.txt"); got != "short.txt" {
		t.Fatalf("short name changed to %q", got)
	}
	long := strings.Repeat("é", 40) // 80 bytes
	got := tempNamePart(long)
	if len(got) > maxTempNamePart || !utf8.ValidString(got) || !strings.HasPrefix(long, got) {
		t.Fatalf("tempNamePart = %q (%d bytes), want a valid prefix of at most %d bytes", got, len(got), maxTempNamePart)
	}
}

// TestLegacyReadsAreBounded is the regression test for BUG-024 (plan 5.15): an input
// without Cryptare's format header, which is read whole, is refused before reading
// when it is larger than the output limit, and a key export larger than 1 MiB is
// refused, while a small legacy file still decrypts under the same limit.
func TestLegacyReadsAreBounded(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "video.bin")
	if err := os.WriteFile(big, bytes.Repeat([]byte{0xA5}, 2<<20), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	limits := ExtractLimits{MaxBytes: 1 << 20}
	out := filepath.Join(dir, "out")
	if err := DecryptFileWithLimits(big, out, testPassword, limits); !errors.Is(err, ErrExtractLimit) {
		t.Fatalf("decrypt a 2 MiB non-Cryptare file under a 1 MiB limit: err = %v, want ErrExtractLimit", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output exists (stat err %v)", err)
	}

	legacy, err := encryptBytes([]byte("legacy plaintext"), testPassword)
	if err != nil {
		t.Fatalf("legacy fixture: %v", err)
	}
	small := filepath.Join(dir, "small.enc")
	if err := os.WriteFile(small, legacy, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := DecryptFileWithLimits(small, filepath.Join(dir, "small.txt"), testPassword, limits); err != nil {
		t.Fatalf("decrypt a small legacy file: %v", err)
	}

	huge := filepath.Join(dir, "huge.ckey")
	if err := os.WriteFile(huge, bytes.Repeat([]byte("A"), maxKeyExportSize+1), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ImportKeyFromFile(huge, testPassword); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("import a %d-byte file: err = %v, want ErrInputTooLarge", maxKeyExportSize+1, err)
	}
}

// TestGunzipRawRoundTrip is the regression test for BUG-022 (Q-011): a single .tar file
// that was gzip-compressed comes back as the same .tar through GunzipFileContext
// ("decompress --raw"), even when it holds a symlink, which ordinary extraction
// refuses.
func TestGunzipRawRoundTrip(t *testing.T) {
	dir := t.TempDir()
	var tarball bytes.Buffer
	tw := tar.NewWriter(&tarball)
	for _, h := range []*tar.Header{
		{Name: "data.txt", Mode: 0o644, Size: 5, Typeflag: tar.TypeReg},
		{Name: "latest", Linkname: "data.txt", Typeflag: tar.TypeSymlink},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte("hello")); err != nil {
				t.Fatalf("tar data: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	original := filepath.Join(dir, "backup.tar")
	if err := os.WriteFile(original, tarball.Bytes(), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := CompressFile(original, "", -1); err != nil {
		t.Fatalf("compress: %v", err)
	}
	gz := original + gzExt
	if err := DecompressFile(gz, filepath.Join(dir, "extracted")); err == nil {
		t.Fatal("extracting a tarball with a symlink succeeded; the test assumes it is refused")
	}

	if err := os.Remove(original); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := GunzipFileContext(context.Background(), gz, "", DefaultExtractLimits()); err != nil {
		t.Fatalf("gunzip --raw: %v", err)
	}
	got, err := os.ReadFile(original)
	if err != nil || !bytes.Equal(got, tarball.Bytes()) {
		t.Fatalf("backup.tar after --raw: %d bytes (err %v), want the original %d bytes", len(got), err, tarball.Len())
	}

	zipFile := filepath.Join(dir, "a.zip")
	if err := os.WriteFile(zipFile, []byte("PK"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := GunzipFileContext(context.Background(), zipFile, "", DefaultExtractLimits()); !errors.Is(err, ErrNotGzip) {
		t.Fatalf("gunzip a zip: err = %v, want ErrNotGzip", err)
	}
}

// TestDefaultRawOutput checks decompress --raw's default output names (BUG-022).
func TestDefaultRawOutput(t *testing.T) {
	for in, want := range map[string]string{
		"backup.tar.gz": "backup.tar",
		"backup.TGZ":    "backup.tar",
		"notes.txt.gz":  "notes.txt",
		"data":          "data.dec",
	} {
		if got := defaultRawOutput(in); got != want {
			t.Errorf("defaultRawOutput(%q) = %q, want %q", in, got, want)
		}
	}
}
