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
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	gzExt    = ".gz"
	tarGzExt = ".tar.gz"
	tgzExt   = ".tgz"
	zipExt   = ".zip"
)

type compressFormat string

const (
	formatGzip compressFormat = "gzip"
	formatZip  compressFormat = "zip"
)

var (
	// ErrOutputExists is returned by CheckOutputPath when the output already exists
	// and overwriting was not requested.
	ErrOutputExists = errors.New("output already exists")
	// ErrSameInputOutput is returned when an operation's output is its own input;
	// writing it would destroy the input before it has been read.
	ErrSameInputOutput = errors.New("output is the same file as the input")
	// ErrOutputInsideInput is returned when a directory would be archived into a file
	// inside itself, which would archive its own partial output.
	ErrOutputInsideInput = errors.New("output is inside the input directory")
	// ErrInputInsideOutput is returned when extraction would replace an existing
	// output directory that contains the archive being extracted.
	ErrInputInsideOutput = errors.New("input is inside the output directory")
	// ErrExtractLimit is returned when decompression or extraction would exceed its
	// ExtractLimits. The partial output is removed.
	ErrExtractLimit = errors.New("extraction limit exceeded")
	// ErrFormatMismatch is returned when an explicit compression format contradicts the
	// output's extension, such as gzip into "x.zip" (BUG-020).
	ErrFormatMismatch = errors.New("compression format doesn't match the output name")
	// ErrInvalidLevel is returned for a compression level other than 1–9 or -1, the
	// default (BUG-020).
	ErrInvalidLevel = errors.New("invalid compression level")
	// ErrNotGzip is returned when GunzipFileContext is given a zip archive.
	ErrNotGzip = errors.New("not a gzip file")
	// ErrInputTooLarge is returned when an input that is read whole is larger than the
	// most Cryptare reads of that kind (BUG-024).
	ErrInputTooLarge = errors.New("input is too large")
)

// Default extraction limits (SEC-007): the most one decompression or extraction may
// write, as a guard against decompression bombs.
const (
	DefaultMaxExtractBytes   int64 = 10 << 30 // 10 GiB
	DefaultMaxExtractEntries       = 100_000
)

// ExtractLimits bounds what one decompression or extraction may write. A zero field
// means no limit.
type ExtractLimits struct {
	MaxBytes   int64 // total bytes of file content written
	MaxEntries int   // archive entries (files and directories) extracted
}

// DefaultExtractLimits returns the limits used when the caller doesn't set its own.
func DefaultExtractLimits() ExtractLimits {
	return ExtractLimits{MaxBytes: DefaultMaxExtractBytes, MaxEntries: DefaultMaxExtractEntries}
}

//--------------------------------------------------core-------------------------------------------------------------------------------------------------//

// CompressFile compresses src at the given level (1–9), writing to dst.
// It defaults to gzip/tar.gz unless dst implies zip.
func CompressFile(src, dst string, level int) (err error) {
	return CompressFileWithFormat(src, dst, "", level)
}

// CompressFileWithFormat compresses src with the selected format (gzip or zip)
// at the given level (1–9), writing to dst.
func CompressFileWithFormat(src, dst, format string, level int) error {
	return CompressFileWithFormatContext(context.Background(), src, dst, format, level)
}

// CompressFileWithFormatContext is CompressFileWithFormat that stops once ctx is done,
// checked between reads and archive entries. The partial output is then removed and an
// existing dst is left as it was (SEC-015).
func CompressFileWithFormatContext(ctx context.Context, src, dst, format string, level int) (err error) {
	selectedFormat, err := resolveCompressFormat(format, dst)
	if err != nil {
		return err
	}

	if level < gzip.BestSpeed || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source path: %w", err)
	}

	if dst == "" {
		dst = defaultCompressOutput(src, info.IsDir(), selectedFormat)
	}
	if err := checkNotSameFile(src, dst); err != nil {
		return err
	}
	if info.IsDir() {
		if err := checkOutputOutsideDir(src, dst); err != nil {
			return err
		}
	}

	out, err := createAtomicFile(dst)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer out.Abort()

	if selectedFormat == formatZip {
		err = writeZip(ctx, out, src, info, level)
	} else {
		err = writeGzip(ctx, out, src, info, level)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := out.Commit(); err != nil {
		return fmt.Errorf("finalise output file: %w", err)
	}
	return nil
}

// writeGzip writes src to w as gzip: a tar.gz stream for a directory, plain gzip for
// a file. The gzip stream is finalised before it returns.
func writeGzip(ctx context.Context, w io.Writer, src string, info fs.FileInfo, level int) (err error) {
	gz, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	defer closeWithError(&err, gz, "finalise gzip")

	if info.IsDir() {
		gz.Name = gzipFolderName(src)
		if _, err := writeTarGz(ctx, gz, src); err != nil {
			return err
		}
	} else {
		in, err := os.Open(src) // #nosec G304 -- src is the file the user chose to compress
		if err != nil {
			return fmt.Errorf("open source file: %w", err)
		}
		defer closeWithError(&err, in, "close source file")

		gz.Name = gzipHeaderName(filepath.Base(src), "")

		if _, err := copyContext(ctx, gz, in); err != nil {
			return fmt.Errorf("compress data: %w", err)
		}
	}
	return nil
}

// fallbackTarName is stored in the gzip header of a folder's tar.gz when the folder's
// name can't be (BUG-015). Ending in ".tar" keeps the archive recognised as a tarball
// by isTarGzArchive whatever the archive file is called.
const fallbackTarName = "archive.tar"

// gzipFolderName returns the name stored in the gzip header of the tar.gz of the
// folder src: the folder's name with ".tar" added, or fallbackTarName.
func gzipFolderName(src string) string {
	return gzipHeaderName(filepath.Base(filepath.Clean(src))+".tar", fallbackTarName)
}

// gzipHeaderName returns name if a gzip header can store it, and fallback otherwise.
// The header holds Latin-1 text only, and Go's gzip writer fails on any other
// character (BUG-015), so a name in, say, Chinese, Cyrillic or with an emoji, or one
// that isn't valid UTF-8, is replaced. The name is informational: Cryptare names its
// output after the archive file, not the header.
func gzipHeaderName(name, fallback string) string {
	for _, r := range name {
		if r == 0 || r > unicode.MaxLatin1 { // invalid UTF-8 decodes to U+FFFD
			return fallback
		}
	}
	return name
}

// DecompressFile decompresses a gzip/zip file at src, writing to dst.
// Tar-based gzip archives and zip archives are extracted into directories.
func DecompressFile(src, dst string) error {
	return DecompressFileWithLimits(src, dst, DefaultExtractLimits())
}

// DecompressFileWithLimits is DecompressFile with explicit extraction limits. When a
// limit is hit it returns ErrExtractLimit and leaves no output behind. Archives are
// extracted into a new hidden directory next to dst, which is then renamed to dst,
// so an existing dst is replaced as a whole rather than merged into.
func DecompressFileWithLimits(src, dst string, limits ExtractLimits) error {
	return DecompressFileWithLimitsContext(context.Background(), src, dst, limits)
}

// DecompressFileWithLimitsContext is DecompressFileWithLimits that stops once ctx is
// done, checked between reads and archive entries. The partial output is then removed
// and an existing dst is left as it was (SEC-015).
func DecompressFileWithLimitsContext(ctx context.Context, src, dst string, limits ExtractLimits) (err error) {
	budget := &extractBudget{limits: limits}
	if strings.HasSuffix(strings.ToLower(src), zipExt) {
		if dst == "" {
			dst = defaultDecompressOutput(src)
		}
		if err := checkNotSameFile(src, dst); err != nil {
			return err
		}
		return extractZip(ctx, src, dst, budget)
	}

	in, err := os.Open(src) // #nosec G304 -- src is the archive the user chose to decompress
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer closeWithError(&err, in, "close source file")

	gz, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer closeWithError(&err, gz, "close gzip reader")

	if dst == "" {
		dst = defaultDecompressOutput(src)
	}
	if err := checkNotSameFile(src, dst); err != nil {
		return err
	}

	if isTarGzArchive(src, gz.Name) {
		return extractToDir(ctx, src, dst, func(dir string) error {
			return extractTarGz(ctx, gz, dir, budget)
		})
	}
	return gunzipToFile(ctx, gz, dst, budget)
}

// GunzipFileContext decompresses the gzip file src to the single file dst without
// extracting a tar archive it holds (BUG-022, Q-011), so "decompress --raw" gives
// x.tar back from x.tar.gz. If dst is empty, the output is defaultRawOutput(src). It
// stops once ctx is done, limits.MaxBytes bounds the output, and a failure leaves no
// partial output.
func GunzipFileContext(ctx context.Context, src, dst string, limits ExtractLimits) (err error) {
	if hasSuffixFold(src, zipExt) {
		return fmt.Errorf("%w: %s is a zip archive", ErrNotGzip, src)
	}
	in, err := os.Open(src) // #nosec G304 -- src is the archive the user chose to decompress
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer closeWithError(&err, in, "close source file")

	gz, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotGzip, err)
	}
	defer closeWithError(&err, gz, "close gzip reader")

	if dst == "" {
		dst = defaultRawOutput(src)
	}
	if err := checkNotSameFile(src, dst); err != nil {
		return err
	}
	return gunzipToFile(ctx, gz, dst, &extractBudget{limits: limits})
}

// defaultRawOutput is GunzipFileContext's default output: src without ".gz", with
// ".tgz" becoming ".tar", in any letter case, or src + ".dec".
func defaultRawOutput(src string) string {
	switch {
	case hasSuffixFold(src, tgzExt):
		return src[:len(src)-len(tgzExt)] + ".tar"
	case hasSuffixFold(src, gzExt):
		return src[:len(src)-len(gzExt)]
	}
	return src + ".dec"
}

// gunzipToFile writes the decompressed stream gz to the single file dst through a
// temporary file, within budget.
func gunzipToFile(ctx context.Context, gz io.Reader, dst string, budget *extractBudget) error {
	out, err := createAtomicFile(dst)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer out.Abort()

	if err := budget.copy(ctx, out, gz); err != nil {
		if errors.Is(err, ErrExtractLimit) {
			return err
		}
		return fmt.Errorf("decompress data: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := out.Commit(); err != nil {
		return fmt.Errorf("finalise output file: %w", err)
	}
	return nil
}

func defaultCompressOutput(src string, isDir bool, format compressFormat) string {
	if format == formatZip {
		return filepath.Clean(src) + zipExt
	}
	if isDir {
		return filepath.Clean(src) + tarGzExt
	}
	return src + gzExt
}

// defaultDecompressOutput strips a known archive extension from src, in any letter
// case (BUG-007), or appends ".dec" when there is none.
func defaultDecompressOutput(src string) string {
	for _, ext := range []string{tarGzExt, tgzExt, gzExt, zipExt} {
		if hasSuffixFold(src, ext) {
			return src[:len(src)-len(ext)]
		}
	}
	return src + ".dec"
}

// isTarGzArchive reports whether a gzip file holds a tar archive, judging by its file
// extension or the name stored in its gzip header, in any letter case.
func isTarGzArchive(src, gzipName string) bool {
	return hasSuffixFold(src, tarGzExt) ||
		hasSuffixFold(src, tgzExt) ||
		hasSuffixFold(gzipName, ".tar")
}

// hasSuffixFold is strings.HasSuffix ignoring letter case.
func hasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix)
}

// resolveCompressFormat picks the format for compressing to dst: format if given,
// otherwise zip for a ".zip" name and gzip for anything else. An explicit format that
// contradicts dst's extension, gzip into ".zip" or zip into ".gz", ".tgz" or
// ".tar.gz", is refused instead of being silently overridden (BUG-020).
func resolveCompressFormat(format, dst string) (compressFormat, error) {
	zipName := hasSuffixFold(dst, zipExt)
	gzipName := hasSuffixFold(dst, gzExt) || hasSuffixFold(dst, tgzExt)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "":
		if zipName {
			return formatZip, nil
		}
		return formatGzip, nil
	case string(formatGzip):
		if zipName {
			return "", fmt.Errorf("%w: gzip into %q (use zip, or a .gz name)", ErrFormatMismatch, dst)
		}
		return formatGzip, nil
	case string(formatZip):
		if gzipName {
			return "", fmt.Errorf("%w: zip into %q (use gzip, or a .zip name)", ErrFormatMismatch, dst)
		}
		return formatZip, nil
	default:
		return "", fmt.Errorf("unsupported compression format %q (supported: gzip, zip)", format)
	}
}

// checkCompressLevel refuses a compression level the CLI or TUI was given other than
// 1–9, or -1 for the default (BUG-020). CompressFileWithFormat itself still treats an
// out-of-range level as the default.
func checkCompressLevel(level int) error {
	if level == gzip.DefaultCompression || (level >= gzip.BestSpeed && level <= gzip.BestCompression) {
		return nil
	}
	return fmt.Errorf("%w: %d (use 1–9, or -1 for the default)", ErrInvalidLevel, level)
}

// writeTarGz writes the directory tree at root to w as a tar stream, reading it through
// walkSourceTree, and returns the number of entries written.
func writeTarGz(ctx context.Context, w io.Writer, root string) (int, error) {
	tw := tar.NewWriter(w)
	entries := 0

	err := walkSourceTree(filepath.Clean(root), func(name string, info fs.FileInfo, file fs.File) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create tar header: %w", err)
		}
		header.Name = name
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header: %w", err)
		}
		if file == nil {
			return nil
		}
		if _, err := copyContext(ctx, tw, file); err != nil {
			return fmt.Errorf("write tar contents: %w", err)
		}
		return nil
	})
	if err != nil {
		closeErr := tw.Close()
		if closeErr != nil {
			return entries, errors.Join(err, fmt.Errorf("finalise tar archive: %w", closeErr))
		}
		return entries, err
	}
	if err := tw.Close(); err != nil {
		return entries, fmt.Errorf("finalise tar archive: %w", err)
	}
	return entries, nil
}

func writeZip(ctx context.Context, w io.Writer, src string, info fs.FileInfo, level int) (err error) {
	zw := zip.NewWriter(w)
	defer closeWithError(&err, zw, "finalise zip")

	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, level)
	})

	if info.IsDir() {
		return writeZipDirectory(ctx, zw, src)
	}
	return writeZipFile(ctx, zw, src, filepath.Base(src))
}

func writeZipDirectory(ctx context.Context, zw *zip.Writer, root string) error {
	return walkSourceTree(filepath.Clean(root), func(name string, info fs.FileInfo, file fs.File) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("create zip header: %w", err)
		}
		if file == nil {
			header.Name = name + "/"
			if _, err := zw.CreateHeader(header); err != nil {
				return fmt.Errorf("write zip header: %w", err)
			}
			return nil
		}

		header.Name = name
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("write zip header: %w", err)
		}
		if _, err := copyContext(ctx, writer, file); err != nil {
			return fmt.Errorf("write zip contents: %w", err)
		}
		return nil
	})
}

func writeZipFile(ctx context.Context, zw *zip.Writer, srcPath, zipName string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("create zip header: %w", err)
	}
	header.Name = filepath.ToSlash(zipName)
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("write zip header: %w", err)
	}

	file, err := os.Open(srcPath) // #nosec G304 -- srcPath is the file the user chose to compress
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	if _, err := copyContext(ctx, writer, file); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return errors.Join(fmt.Errorf("write zip contents: %w", err), fmt.Errorf("close source file: %w", closeErr))
		}
		return fmt.Errorf("write zip contents: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close source file: %w", err)
	}
	return nil
}

// extractTarGz extracts a tar stream into the existing directory dst, counting its
// entries and bytes against budget. Callers pass a new, empty directory (see
// extractToDir). Writes go through an os.Root on dst, so no entry can land outside
// it, and permissions are set by extractDirMode and extractFileMode.
func extractTarGz(ctx context.Context, r io.Reader, dst string, budget *extractBudget) (err error) {
	cleanDst := filepath.Clean(dst)
	cleanDstWithSep := cleanDst + string(os.PathSeparator)
	root, err := os.OpenRoot(cleanDst)
	if err != nil {
		return fmt.Errorf("open output directory: %w", err)
	}
	defer closeWithError(&err, root, "close output directory")

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}
		if err := budget.addEntry(ctx); err != nil {
			return err
		}

		archivePath := filepath.Clean(filepath.FromSlash(header.Name))
		target := filepath.Join(cleanDst, archivePath)
		if target == cleanDst {
			continue
		}
		if !strings.HasPrefix(target, cleanDstWithSep) {
			return fmt.Errorf("extract archive: invalid path %q", header.Name)
		}
		if archivePath == ".." || strings.HasPrefix(archivePath, ".."+string(filepath.Separator)) {
			return fmt.Errorf("extract archive: invalid path %q", header.Name)
		}
		// Relative to the output; a leading "/" in the entry name was dropped by Join.
		name := strings.TrimPrefix(target, cleanDstWithSep)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, extractDirMode); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
		case tar.TypeReg, 0:
			if err := root.MkdirAll(filepath.Dir(name), extractDirMode); err != nil {
				return fmt.Errorf("create parent directory: %w", err)
			}
			file, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, extractFileMode(header.FileInfo().Mode()))
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			if err := budget.copy(ctx, file, tr); err != nil {
				closeErr := file.Close()
				if closeErr != nil {
					return errors.Join(wrapExtractError(err), fmt.Errorf("close output file: %w", closeErr))
				}
				return wrapExtractError(err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close output file: %w", err)
			}
		default:
			return fmt.Errorf("extract archive: unsupported entry type %q", header.Name)
		}
	}
}

func extractZip(ctx context.Context, src, dst string, budget *extractBudget) (err error) {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	defer closeWithError(&err, r, "close zip reader")

	// The central directory lists every entry, so an archive with too many is refused
	// before anything is written.
	if limit := budget.limits.MaxEntries; limit > 0 && len(r.File) > limit {
		return entryLimitError(limit)
	}

	singleFile := len(r.File) == 1 && !r.File[0].FileInfo().IsDir() && r.File[0].Mode().IsRegular()
	cleanDst := filepath.Clean(dst)
	if singleFile {
		entryName := filepath.Clean(filepath.FromSlash(r.File[0].Name))
		if filepath.Base(entryName) == filepath.Base(cleanDst) {
			return extractZipSingleFile(ctx, r.File[0], cleanDst, budget)
		}
	}

	return extractToDir(ctx, src, cleanDst, func(dir string) error {
		return extractZipEntries(ctx, r.File, dir, budget)
	})
}

// extractZipEntries extracts zip entries into the existing directory dst, like
// extractTarGz: through an os.Root, with permissions from extractDirMode and
// extractFileMode, counting entries and bytes against budget.
func extractZipEntries(ctx context.Context, files []*zip.File, dst string, budget *extractBudget) (err error) {
	cleanDst := filepath.Clean(dst)
	cleanDstWithSep := cleanDst + string(os.PathSeparator)
	root, err := os.OpenRoot(cleanDst)
	if err != nil {
		return fmt.Errorf("open output directory: %w", err)
	}
	defer closeWithError(&err, root, "close output directory")

	for _, file := range files {
		if err := budget.addEntry(ctx); err != nil {
			return err
		}
		archivePath := filepath.Clean(filepath.FromSlash(file.Name))
		target := filepath.Join(cleanDst, archivePath)
		if target == cleanDst {
			continue
		}
		if !strings.HasPrefix(target, cleanDstWithSep) {
			return fmt.Errorf("extract archive: invalid path %q", file.Name)
		}
		if archivePath == ".." || strings.HasPrefix(archivePath, ".."+string(filepath.Separator)) {
			return fmt.Errorf("extract archive: invalid path %q", file.Name)
		}
		// Relative to the output; a leading "/" in the entry name was dropped by Join.
		name := strings.TrimPrefix(target, cleanDstWithSep)

		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("extract archive: unsupported entry type %q", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := root.MkdirAll(name, extractDirMode); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("extract archive: unsupported entry type %q", file.Name)
		}

		if err := root.MkdirAll(filepath.Dir(name), extractDirMode); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}

		out, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, extractFileMode(mode))
		if err != nil {
			closeErr := rc.Close()
			if closeErr != nil {
				return errors.Join(fmt.Errorf("create output file: %w", err), fmt.Errorf("close zip entry: %w", closeErr))
			}
			return fmt.Errorf("create output file: %w", err)
		}
		if err := budget.copy(ctx, out, rc); err != nil {
			closeOutErr := out.Close()
			closeReadErr := rc.Close()
			if closeOutErr != nil && closeReadErr != nil {
				return errors.Join(wrapExtractError(err), fmt.Errorf("close output file: %w", closeOutErr), fmt.Errorf("close zip entry: %w", closeReadErr))
			}
			if closeOutErr != nil {
				return errors.Join(wrapExtractError(err), fmt.Errorf("close output file: %w", closeOutErr))
			}
			if closeReadErr != nil {
				return errors.Join(wrapExtractError(err), fmt.Errorf("close zip entry: %w", closeReadErr))
			}
			return wrapExtractError(err)
		}
		if err := out.Close(); err != nil {
			closeErr := rc.Close()
			if closeErr != nil {
				return errors.Join(fmt.Errorf("close output file: %w", err), fmt.Errorf("close zip entry: %w", closeErr))
			}
			return fmt.Errorf("close output file: %w", err)
		}
		if err := rc.Close(); err != nil {
			return fmt.Errorf("close zip entry: %w", err)
		}
	}

	return nil
}

// extractZipSingleFile extracts a zip holding one file straight to the file dst. Like
// other single-file outputs it is written through a temporary file and a rename, so
// it gets mode 0600 and a failure leaves no partial file.
func extractZipSingleFile(ctx context.Context, file *zip.File, dst string, budget *extractBudget) (err error) {
	mode := file.Mode()
	if mode&os.ModeSymlink != 0 || !mode.IsRegular() {
		return fmt.Errorf("extract archive: unsupported entry type %q", file.Name)
	}
	if err := budget.addEntry(ctx); err != nil {
		return err
	}

	// Missing parent folders are owner-only, like everything else the tool writes.
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("open zip entry: %w", err)
	}
	defer closeWithError(&err, rc, "close zip entry")

	out, err := createAtomicFile(dst)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer out.Abort()

	if err := budget.copy(ctx, out, rc); err != nil {
		return wrapExtractError(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := out.Commit(); err != nil {
		return fmt.Errorf("finalise output file: %w", err)
	}
	return nil
}

//--------------------------------------------------extraction limits and output-------------------------------------------------------------------------//

// extractDirMode is the mode of every folder created by extraction. Extracted files
// and folders are private to the owner whatever the archive stores (SEC-008), and a
// read-only folder in the archive can't block extracting its contents.
const extractDirMode fs.FileMode = 0o700

// extractFileMode returns the mode of an extracted file: 0700 when the archive marks
// it executable for anyone, otherwise 0600.
func extractFileMode(archived fs.FileMode) fs.FileMode {
	if archived&0o111 != 0 {
		return 0o700
	}
	return 0o600
}

// extractBudget tracks one decompression or extraction against its ExtractLimits.
type extractBudget struct {
	limits  ExtractLimits
	written int64
	entries int
}

// addEntry counts one archive entry against the entry limit, and stops the extraction
// once ctx is done (SEC-015).
func (b *extractBudget) addEntry(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.entries++
	if b.limits.MaxEntries > 0 && b.entries > b.limits.MaxEntries {
		return entryLimitError(b.limits.MaxEntries)
	}
	return nil
}

// copy copies r to w, stopping once ctx is done, and counts the bytes against the
// size limit. It reads at most one byte beyond the limit, which is how an overrun is
// detected.
func (b *extractBudget) copy(ctx context.Context, w io.Writer, r io.Reader) error {
	n := int64(math.MaxInt64)
	if b.limits.MaxBytes > 0 {
		n = b.limits.MaxBytes - b.written + 1
	}
	copied, err := copyContext(ctx, w, io.LimitReader(r, n))
	b.written += copied
	if err != nil {
		return err
	}
	if b.limits.MaxBytes > 0 && b.written > b.limits.MaxBytes {
		return fmt.Errorf("%w: the output is larger than %s", ErrExtractLimit, formatSize(b.limits.MaxBytes))
	}
	return nil
}

func entryLimitError(limit int) error {
	return fmt.Errorf("%w: the archive has more than %d entries", ErrExtractLimit, limit)
}

// wrapExtractError adds context to an error from copying an entry's contents. Limit
// errors are returned unchanged so their message stays readable.
func wrapExtractError(err error) error {
	if errors.Is(err, ErrExtractLimit) {
		return err
	}
	return fmt.Errorf("extract file contents: %w", err)
}

// formatSize formats n bytes with the largest unit (binary or decimal) that divides it
// exactly, such as "10 GiB" or "500 MB", and otherwise in bytes.
func formatSize(n int64) string {
	units := []struct {
		name string
		size int64
	}{
		{"TiB", 1 << 40}, {"TB", 1e12}, {"GiB", 1 << 30}, {"GB", 1e9},
		{"MiB", 1 << 20}, {"MB", 1e6}, {"KiB", 1 << 10}, {"KB", 1e3},
	}
	for _, u := range units {
		if n >= u.size && n%u.size == 0 {
			return fmt.Sprintf("%d %s", n/u.size, u.name)
		}
	}
	return fmt.Sprintf("%d bytes", n)
}

// extractToDir runs extract on a new, empty hidden directory next to dst (mode 0700)
// and renames it to dst when extract succeeds. A failure, including a limit being
// hit, leaves no partial output. Because extraction always starts from an empty
// directory, it can't follow symlinks planted in an existing dst (SEC-008). An
// existing dst, which the CLI allows only with --force, is replaced as a whole, not
// merged into; that is refused when dst holds the archive src itself. A ctx that is
// done before the rename counts as a failure (SEC-015).
func extractToDir(ctx context.Context, src, dst string, extract func(dir string) error) (err error) {
	dst = filepath.Clean(dst)
	if err := checkInputOutsideOutput(src, dst); err != nil {
		return err
	}
	parent := filepath.Dir(dst)
	// Missing parent folders are owner-only, like everything else the tool writes.
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, "."+tempNamePart(filepath.Base(dst))+".*.tmp")
	if err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(tmp)
		}
	}()

	if err := extract(tmp); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return replacePath(tmp, dst)
}

// replacePath renames newPath to dst. An existing dst is first moved aside and is
// removed only once newPath is in place, so a failed rename leaves it where it was.
func replacePath(newPath, dst string) error {
	if _, err := os.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(newPath, dst); err != nil {
			return fmt.Errorf("move output into place: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("check output path: %w", err)
	}

	old := newPath + ".old"
	if err := os.Rename(dst, old); err != nil {
		return fmt.Errorf("move existing output aside: %w", err)
	}
	if err := os.Rename(newPath, dst); err != nil {
		if restoreErr := os.Rename(old, dst); restoreErr != nil {
			return errors.Join(fmt.Errorf("move output into place: %w", err),
				fmt.Errorf("restore previous output from %s: %w", old, restoreErr))
		}
		return fmt.Errorf("move output into place: %w", err)
	}
	if err := os.RemoveAll(old); err != nil {
		return fmt.Errorf("output written to %s, but removing the previous version at %s failed: %w", dst, old, err)
	}
	return nil
}

// checkInputOutsideOutput returns ErrInputInsideOutput when dst is an existing
// directory (not a symlink, which would be replaced rather than followed) that
// contains src, because replacing dst would delete src.
func checkInputOutsideOutput(src, dst string) error {
	info, err := os.Lstat(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check output path: %w", err)
	}
	if !info.IsDir() {
		return nil
	}
	inside, err := pathWithin(dst, src)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%w: %s", ErrInputInsideOutput, dst)
	}
	return nil
}

//--------------------------------------------------source trees-----------------------------------------------------------------------------------------//

// testHookBeforeArchiveOpen, when a test sets it, runs just before walkSourceTree opens
// a file, so the test can swap the file for a symlink (SEC-014). It is nil otherwise.
var testHookBeforeArchiveOpen func(path string)

// walkSourceTree calls visit for every entry below the directory root, in lexical
// order, with its slash-separated path relative to root. Folders are visited with a nil
// file. Regular files are visited open, with the opened file's own metadata, so a
// header built from info matches the data read. The walk and the opens go through an
// os.Root on root, so an entry swapped for a symlink during the walk can't lead outside
// the tree (SEC-014). Symlinks and special files found by the walk are rejected.
func walkSourceTree(root string, visit func(name string, info fs.FileInfo, file fs.File) error) (err error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open source directory: %w", err)
	}
	defer closeWithError(&err, r, "close source directory")
	fsys := r.FS()

	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk source directory: %w", walkErr)
		}
		if name == "." {
			return nil
		}
		path := filepath.Join(root, filepath.FromSlash(name))

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("read entry info: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("compress directory: symlinks are not supported (%s)", path)
		}
		if info.IsDir() {
			return visit(name, info, nil)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("compress directory: unsupported file type at %s", path)
		}

		if testHookBeforeArchiveOpen != nil {
			testHookBeforeArchiveOpen(path)
		}
		file, err := fsys.Open(name)
		if err != nil {
			return fmt.Errorf("open source file: %w", err)
		}
		err = visitOpenFile(file, name, path, visit)
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close source file: %w", closeErr)
		}
		return err
	})
}

// visitOpenFile checks that an opened tree entry is still a regular file, then visits
// it with its own metadata.
func visitOpenFile(file fs.File, name, path string, visit func(string, fs.FileInfo, fs.File) error) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("compress directory: %s changed while it was being read", path)
	}
	return visit(name, info, file)
}

//--------------------------------------------------output paths-----------------------------------------------------------------------------------------//

// CheckOutputPath reports whether an operation reading src may write dst. It fails
// with ErrSameInputOutput when dst is src itself, even when overwrite is true, and
// with ErrOutputExists when dst already exists and overwrite is false. The CLI and
// TUI call it before encrypting, decrypting, compressing or decompressing.
func CheckOutputPath(src, dst string, overwrite bool) error {
	if err := checkNotSameFile(src, dst); err != nil {
		return err
	}
	return checkOutputFree(dst, overwrite)
}

// checkOutputFree returns ErrOutputExists when something already exists at dst and
// overwrite is false.
func checkOutputFree(dst string, overwrite bool) error {
	if _, err := os.Lstat(dst); err == nil {
		if !overwrite {
			return fmt.Errorf("%w: %s", ErrOutputExists, dst)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("check output path: %w", err)
	}
	return nil
}

// checkNotSameFile returns ErrSameInputOutput when dst names the same file as src,
// including through a different spelling, a symlink or a hard link.
func checkNotSameFile(src, dst string) error {
	dstInfo, err := os.Stat(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check output path: %w", err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("check input path: %w", err)
	}
	if os.SameFile(srcInfo, dstInfo) {
		return fmt.Errorf("%w: %s", ErrSameInputOutput, dst)
	}
	return nil
}

// checkOutputOutsideDir returns ErrOutputInsideInput when dst lies inside srcDir.
func checkOutputOutsideDir(srcDir, dst string) error {
	inside, err := pathWithin(srcDir, dst)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%w: %s", ErrOutputInsideInput, dst)
	}
	return nil
}

// pathWithin reports whether path is dir itself or lies inside it. It compares the
// absolute paths as text and, failing that, walks up from path comparing each existing
// folder with dir by identity (os.SameFile), so a path reached through a symlink, or
// spelled in another letter case on a case-insensitive file system, is recognised too
// (BUG-019). Parts of path that don't exist yet are skipped.
func pathWithin(dir, path string) (bool, error) {
	inside, err := pathWithinLexically(dir, path)
	if err != nil || inside {
		return inside, err
	}
	dirInfo, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve path %s: %w", dir, err)
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolve path %s: %w", path, err)
	}
	for {
		if info, err := os.Stat(p); err == nil && os.SameFile(info, dirInfo) {
			return true, nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return false, nil
		}
		p = parent
	}
}

// pathWithinLexically reports whether path is dir itself or lies inside it, comparing
// absolute paths as text.
func pathWithinLexically(dir, path string) (bool, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, fmt.Errorf("resolve path %s: %w", dir, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolve path %s: %w", path, err)
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false, nil // e.g. different Windows volumes: path cannot be inside dir
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

// maxTempNamePart is the most of an output's name copied into the name of its
// temporary file or folder (BUG-021).
const maxTempNamePart = 64

// tempNamePart returns name cut to maxTempNamePart bytes at a UTF-8 boundary. The
// temporary name ".<name>.<random>.tmp" adds about 15 bytes, which would otherwise
// push an output name near the 255-byte limit over it.
func tempNamePart(name string) string {
	if len(name) <= maxTempNamePart {
		return name
	}
	cut := maxTempNamePart
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}
	return name[:cut]
}

// readAllLimit reads r to the end, refusing input longer than limit bytes with
// ErrInputTooLarge (BUG-024).
func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: over %s", ErrInputTooLarge, formatSize(limit))
	}
	return data, nil
}

// atomicFile is an output written to a hidden temporary file next to its destination
// and renamed into place by Commit, so the destination is either left as it was or
// fully written, never partially.
type atomicFile struct {
	*os.File
	dst       string
	committed bool
}

// createAtomicFile starts an atomic write of dst. The temporary file has mode 0600,
// the mode the tool uses for all single-file outputs.
func createAtomicFile(dst string) (*atomicFile, error) {
	f, err := os.CreateTemp(filepath.Dir(dst), "."+tempNamePart(filepath.Base(dst))+".*.tmp")
	if err != nil {
		return nil, err
	}
	return &atomicFile{File: f, dst: dst}, nil
}

// Commit flushes the temporary file to disk and renames it to the destination,
// replacing any file already there.
func (a *atomicFile) Commit() error {
	if err := a.Sync(); err != nil {
		return err
	}
	if err := a.Close(); err != nil {
		return err
	}
	if err := os.Rename(a.Name(), a.dst); err != nil {
		return err
	}
	a.committed = true
	return nil
}

// Abort removes the temporary file unless Commit succeeded. It is safe to defer.
func (a *atomicFile) Abort() {
	if a.committed {
		return
	}
	_ = a.Close()
	_ = os.Remove(a.Name())
}

// writeFileAtomic writes data to dst through a temporary file and a rename.
func writeFileAtomic(dst string, data []byte) error {
	f, err := createAtomicFile(dst)
	if err != nil {
		return err
	}
	defer f.Abort()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Commit()
}

// copyContext is io.Copy that checks ctx before each read and returns ctx's error once
// it is done, so a cancelled operation stops within one buffer (SEC-015). The deferred
// clean-up of the caller then removes the partial output.
func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32<<10)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			w, err := dst.Write(buf[:n])
			written += int64(w)
			if err != nil {
				return written, err
			}
			if w != n {
				return written, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func closeWithError(target *error, closer io.Closer, message string) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil && *target == nil {
		*target = fmt.Errorf("%s: %w", message, err)
	}
}
