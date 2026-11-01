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
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// countdownContext reports itself cancelled from the (n+1)th call to Err on, so a test
// can stop an operation partway through without depending on timing. The file
// operations check Err between reads and archive entries.
type countdownContext struct {
	context.Context
	left atomic.Int64
}

func (c *countdownContext) Err() error {
	if c.left.Add(-1) < 0 {
		return context.Canceled
	}
	return nil
}

func cancelAfter(n int64) context.Context {
	c := &countdownContext{Context: context.Background()}
	c.left.Store(n)
	return c
}

// TestCancelledOperationsLeaveNothingBehind is SEC-015's core check: each file
// operation, cancelled partway through, returns context.Canceled, leaves no output and
// no hidden temporary file or folder, and leaves an existing output as it was.
func TestCancelledOperationsLeaveNothingBehind(t *testing.T) {
	src := t.TempDir()
	// 1 MiB is read 32 KiB at a time, so every operation checks the context dozens of
	// times before it could finish.
	big := bytes.Repeat([]byte("cryptare cancellation test data\n"), 1<<15)
	file := filepath.Join(src, "big.txt")
	folder := filepath.Join(src, "tree")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for path, data := range map[string][]byte{
		file:                           big,
		filepath.Join(folder, "a.txt"): []byte("small"),
		filepath.Join(folder, "b.txt"): big,
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// Inputs for the reverse operations, made without cancellation.
	encFile, encFolder := file+encExt, folder+encExt
	gz, tgz := file+gzExt, folder+tarGzExt
	zipFolder, zipFile := folder+zipExt, file+zipExt
	for _, step := range []error{
		EncryptFile(file, encFile, testPassword),
		EncryptFile(folder, encFolder, testPassword),
		CompressFile(file, gz, -1),
		CompressFile(folder, tgz, -1),
		CompressFileWithFormat(folder, zipFolder, "zip", -1),
		CompressFileWithFormat(file, zipFile, "zip", -1),
	} {
		if step != nil {
			t.Fatalf("prepare inputs: %v", step)
		}
	}
	limits := DefaultExtractLimits()

	ops := []struct {
		name   string
		output string
		run    func(ctx context.Context, dst string) error
	}{
		{"encrypt file", "out.enc", func(ctx context.Context, dst string) error {
			return EncryptFileContext(ctx, file, dst, testPassword)
		}},
		{"encrypt folder", "out.enc", func(ctx context.Context, dst string) error {
			return EncryptFileContext(ctx, folder, dst, testPassword)
		}},
		{"decrypt file", "big.txt", func(ctx context.Context, dst string) error {
			return DecryptFileWithLimitsContext(ctx, encFile, dst, testPassword, limits)
		}},
		{"decrypt folder", "restored", func(ctx context.Context, dst string) error {
			return DecryptFileWithLimitsContext(ctx, encFolder, dst, testPassword, limits)
		}},
		{"gzip a file", "out.gz", func(ctx context.Context, dst string) error {
			return CompressFileWithFormatContext(ctx, file, dst, "gzip", -1)
		}},
		{"tar.gz a folder", "out.tar.gz", func(ctx context.Context, dst string) error {
			return CompressFileWithFormatContext(ctx, folder, dst, "gzip", -1)
		}},
		{"zip a folder", "out.zip", func(ctx context.Context, dst string) error {
			return CompressFileWithFormatContext(ctx, folder, dst, "zip", -1)
		}},
		{"gunzip a file", "big.txt", func(ctx context.Context, dst string) error {
			return DecompressFileWithLimitsContext(ctx, gz, dst, limits)
		}},
		{"extract a tar.gz", "restored", func(ctx context.Context, dst string) error {
			return DecompressFileWithLimitsContext(ctx, tgz, dst, limits)
		}},
		{"extract a zip", "restored", func(ctx context.Context, dst string) error {
			return DecompressFileWithLimitsContext(ctx, zipFolder, dst, limits)
		}},
		{"extract a single-file zip", "big.txt", func(ctx context.Context, dst string) error {
			return DecompressFileWithLimitsContext(ctx, zipFile, dst, limits)
		}},
	}
	for _, op := range ops {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing output %v", op.name, existing), func(t *testing.T) {
				out := t.TempDir()
				dst := filepath.Join(out, op.output)
				if existing {
					if err := os.WriteFile(dst, []byte("keep"), 0o600); err != nil {
						t.Fatalf("write existing output: %v", err)
					}
				}

				if err := op.run(cancelAfter(5), dst); !errors.Is(err, context.Canceled) {
					t.Fatalf("err = %v, want context.Canceled", err)
				}

				entries, err := os.ReadDir(out)
				if err != nil {
					t.Fatalf("read output folder: %v", err)
				}
				var names []string
				for _, e := range entries {
					names = append(names, e.Name())
				}
				if !existing {
					if len(names) != 0 {
						t.Fatalf("output folder holds %v, want nothing", names)
					}
					return
				}
				if len(names) != 1 || names[0] != op.output {
					t.Fatalf("output folder holds %v, want only %s", names, op.output)
				}
				if got, err := os.ReadFile(dst); err != nil || string(got) != "keep" {
					t.Fatalf("existing output = %q (err %v), want it unchanged", got, err)
				}
			})
		}
	}
}

// TestUncancelledContextOperationsComplete checks that the context variants behave
// like the plain functions when the context isn't cancelled.
func TestUncancelledContextOperationsComplete(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ctx := context.Background()
	if err := EncryptFileContext(ctx, src, src+encExt, testPassword); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := DecryptFileWithLimitsContext(ctx, src+encExt, filepath.Join(dir, "b.txt"), testPassword, DefaultExtractLimits()); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if err := CompressFileWithFormatContext(ctx, src, src+gzExt, "gzip", -1); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := DecompressFileWithLimitsContext(ctx, src+gzExt, filepath.Join(dir, "c.txt"), DefaultExtractLimits()); err != nil {
		t.Fatalf("decompress: %v", err)
	}
	for _, name := range []string{"b.txt", "c.txt"} {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != "payload" {
			t.Fatalf("%s = %q (err %v), want payload", name, got, err)
		}
	}
}
