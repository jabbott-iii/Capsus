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
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	saltLen                  = 16
	ivLen                    = 12
	keyLen                   = 32
	pbkdf2Iter               = 100_000
	encExt                   = ".enc"
	directoryArtifactMagicV1 = "CRYPTARE-DIR-ENC\x00"
)

// MinPasswordLength is the minimum length, in Unicode code points, of a password that
// protects new data. It follows NIST SP 800-63B-4 section 3.1.1.2, which requires at
// least 15 characters for a password used on its own (single-factor).
const MinPasswordLength = 15

var (
	// ErrEmptyPassword is returned when an encryption operation is given an empty
	// password. Decryption still accepts one so that files, archives and key blobs
	// created before this check was added remain readable.
	ErrEmptyPassword = errors.New("password must not be empty")

	// ErrWeakPassword is returned when a password that would protect new data fails
	// the password policy (see CheckPasswordPolicy).
	ErrWeakPassword = errors.New("password is too weak")

	// ErrPasswordMismatch is returned by the CLI and TUI when a new password and its
	// confirmation differ.
	ErrPasswordMismatch = errors.New("passwords do not match")

	// ErrInvalidKeyExport is returned by ImportKeyFromFile when an export's contents
	// aren't what Cryptare writes (see validateKeyExport).
	ErrInvalidKeyExport = errors.New("invalid key export")
)

//--------------------------------------------------core-------------------------------------------------------------------------------------------------//

// deriveKey derives the 32-byte AES key of the legacy (version 1) formats from a
// password and salt with PBKDF2-HMAC-SHA256. New data uses Argon2id (format_v2.go).
func deriveKey(password string, salt []byte) ([]byte, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iter, keyLen)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}

// byteOrderMark is U+FEFF. Editors such as Windows Notepad can start a UTF-8 text file
// with it, so a password read from a password file may begin with one the user never
// typed.
const byteOrderMark = "\uFEFF"

// normalizePassword returns the form of password that protects new data (Q-009): without
// a leading byte order mark, and in Unicode Normalization Form KC, as NIST SP 800-63B
// recommends. The same characters entered on different keyboards, systems or input
// methods, such as a precomposed "é" or an "e" followed by a combining accent, or a
// full-width "Ａ" and an "A", then give the same key. Bytes that aren't valid UTF-8 are
// kept as they are.
func normalizePassword(password string) string {
	return norm.NFKC.String(strings.TrimPrefix(password, byteOrderMark))
}

// passwordCandidates returns the forms of password to try, in order, when decrypting.
// Data whose header records the normalised form (kdfArgon2idNFKC) takes only that.
// Other data, written by an earlier version, in the legacy formats, or by this version
// with a password that normalising doesn't change, takes the password without a
// leading byte order mark, then the password as given if it had one, then its
// normalised form if that differs. So data protected through a password file with a
// byte order mark opens with the password typed, and data protected with a password
// in normalised form opens however the password is entered now.
func passwordCandidates(password string, normalized bool) []string {
	stripped := strings.TrimPrefix(password, byteOrderMark)
	nfkc := norm.NFKC.String(stripped)
	if normalized {
		return []string{nfkc}
	}
	candidates := []string{stripped}
	if stripped != password {
		candidates = append(candidates, password)
	}
	if nfkc != stripped {
		candidates = append(candidates, nfkc)
	}
	return candidates
}

// CheckPasswordPolicy reports whether password may protect new data: encrypted files
// and directories, stored keys and key exports. It returns ErrEmptyPassword for an
// empty password and ErrWeakPassword for one that is shorter than MinPasswordLength
// Unicode code points or is a single character repeated. There are no composition
// rules and no maximum length. It checks the normalised password (normalizePassword),
// which is what protects the data.
//
// Passwords used to decrypt or import are not checked, so data protected before the
// policy existed stays readable.
func CheckPasswordPolicy(password string) error {
	password = normalizePassword(password)
	if password == "" {
		return ErrEmptyPassword
	}
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("%w: use at least %d characters", ErrWeakPassword, MinPasswordLength)
	}
	// Compare bytes so that invalid UTF-8 (which decodes to RuneError) isn't mistaken
	// for a repeated character.
	_, size := utf8.DecodeRuneInString(password)
	if len(password)%size == 0 && strings.Repeat(password[:size], len(password)/size) == password {
		return fmt.Errorf("%w: it is one character repeated", ErrWeakPassword)
	}
	return nil
}

// EncryptFile encrypts src with AES-256-GCM using password, writing to dst in the
// version 2 format (format_v2.go): a folder is written as a tar.gz, and either is
// streamed in authenticated chunks, so its size isn't limited by memory. If dst is
// empty, the output path is defaultEncryptOutput(src). An output inside a folder being
// encrypted is refused with ErrOutputInsideInput (BUG-016). The password must meet the
// password policy (CheckPasswordPolicy).
func EncryptFile(src, dst, password string) error {
	return EncryptFileContext(context.Background(), src, dst, password)
}

// EncryptFileContext is EncryptFile that stops once ctx is done, checked between reads
// and archive entries. The partial output is then removed and an existing dst is left
// as it was (SEC-015).
func EncryptFileContext(ctx context.Context, src, dst, password string) error {
	return EncryptFileWithCredentialContext(ctx, src, dst, PasswordCredential(password))
}

// EncryptFileWithCredentialContext is EncryptFileContext with the data protected by
// cred: a password, which must meet the password policy, or a stored key unlocked for
// new data with StoredKeyCredential (plan 3.4), whose ID the output's header records.
// A stored key unlocked only to decrypt is refused, so the policy on its master
// password can't be skipped (SEC-001).
func EncryptFileWithCredentialContext(ctx context.Context, src, dst string, cred Credential) (err error) {
	switch {
	case cred.isStoredKey() && !cred.canEncrypt:
		return fmt.Errorf("stored key %s was unlocked to decrypt, not to encrypt new files", cred.keyID)
	case !cred.isStoredKey():
		if err := CheckPasswordPolicy(cred.password); err != nil {
			return err
		}
	}

	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("lstat source path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("encrypt input: symlinks are not supported (%s)", src)
	}

	statInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source path: %w", err)
	}

	if dst == "" {
		dst = defaultEncryptOutput(src)
	}
	if err := checkNotSameFile(src, dst); err != nil {
		return err
	}
	if err := checkOutputOutsideFolder(src, dst); err != nil {
		return err
	}

	out, err := createAtomicFile(dst)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer out.Abort()

	if statInfo.IsDir() {
		err = encryptDirectory(ctx, out, src, cred)
	} else {
		err = encryptSingleFile(ctx, out, src, cred)
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

// encryptSingleFile streams the file at src, encrypted, to w.
func encryptSingleFile(ctx context.Context, w io.Writer, src string, cred Credential) (err error) {
	in, err := os.Open(src) // #nosec G304 -- src is the file the user chose to encrypt
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer closeWithError(&err, in, "close source file")

	enc, err := newEncryptingWriter(w, cred, contentFile)
	if err != nil {
		return err
	}
	if _, err := copyContext(ctx, enc, in); err != nil {
		return fmt.Errorf("encrypt data: %w", err)
	}
	return enc.Close()
}

// encryptDirectory streams a tar.gz of the directory tree at src, encrypted, to w. The
// archive is never held in memory or written anywhere in plaintext (SEC-006).
func encryptDirectory(ctx context.Context, w io.Writer, src string, cred Credential) error {
	enc, err := newEncryptingWriter(w, cred, contentFolder)
	if err != nil {
		return err
	}
	if err := writeDirectoryArchive(ctx, enc, src); err != nil {
		return err
	}
	return enc.Close()
}

// writeDirectoryArchive writes a tar.gz of the directory tree at src to w.
func writeDirectoryArchive(ctx context.Context, w io.Writer, src string) error {
	gz, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	gz.Name = gzipFolderName(src)

	if _, err := writeTarGz(ctx, gz, src); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("finalise gzip: %w", err)
	}
	return nil
}

// DecryptFile decrypts an AES-256-GCM encrypted file at src using password,
// writing plaintext to dst.  If dst is empty, the ".enc" suffix is stripped.
// An encrypted directory is restored under DefaultExtractLimits.
func DecryptFile(src, dst, password string) error {
	return DecryptFileWithLimits(src, dst, password, DefaultExtractLimits())
}

// DecryptFileWithLimits is DecryptFile with explicit limits for restoring an
// encrypted directory, which is extracted like an archive (see
// DecompressFileWithLimits). Single files are not affected: their plaintext is
// never larger than the encrypted file.
//
// Version 2 files are decrypted as a stream. Files in the legacy (version 1) formats,
// which have no header, are still read, whole, as before.
func DecryptFileWithLimits(src, dst, password string, limits ExtractLimits) error {
	return DecryptFileWithLimitsContext(context.Background(), src, dst, password, limits)
}

// DecryptFileWithLimitsContext is DecryptFileWithLimits that stops once ctx is done,
// checked between reads and archive entries. The partial plaintext is then removed and
// an existing dst is left as it was (SEC-015).
func DecryptFileWithLimitsContext(ctx context.Context, src, dst, password string, limits ExtractLimits) error {
	return DecryptFileWithCredentialContext(ctx, src, dst, PasswordCredential(password), limits)
}

// DecryptFileWithCredentialContext is DecryptFileWithLimitsContext with cred opening the
// data: a password, or, for data encrypted with a stored key, that key unlocked with
// StoredKeyCredential (plan 3.4). Use EncryptedWithStoredKey to find out which one src
// needs; the wrong kind is refused with ErrStoredKeyRequired or ErrPasswordRequired.
func DecryptFileWithCredentialContext(ctx context.Context, src, dst string, cred Credential, limits ExtractLimits) (err error) {
	f, err := os.Open(src) // #nosec G304 -- src is the file the user chose to decrypt
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only: a close error can't lose data

	if dst == "" {
		dst = defaultDecryptOutput(src)
	}
	if err := checkNotSameFile(src, dst); err != nil {
		return err
	}

	r := bufio.NewReader(f)
	prefix, _ := r.Peek(len(directoryArtifactMagicV1)) // shorter at the end of a small file
	if isV2(prefix) {
		h, plain, err := newDecryptingReader(r, cred, contentFile, contentFolder)
		if err != nil {
			return err
		}
		if h.content == contentFolder {
			return restoreDirectoryArchive(ctx, plain, src, dst, limits)
		}
		return writeStreamAtomic(ctx, dst, plain)
	}
	if cred.isStoredKey() {
		return ErrPasswordRequired // the legacy formats predate stored keys
	}
	password := cred.password

	// The legacy formats are read whole, so their size is bounded by the output limit
	// before reading (BUG-024): a large file that isn't Cryptare's fails fast instead of
	// filling memory, and a raised --max-size still reads any legacy file.
	if limits.MaxBytes > 0 {
		if info, err := f.Stat(); err == nil && info.Mode().IsRegular() && info.Size() > limits.MaxBytes {
			return legacyTooLargeError(limits.MaxBytes)
		}
	}
	var data []byte
	if limits.MaxBytes > 0 {
		data, err = readAllLimit(r, limits.MaxBytes)
		if errors.Is(err, ErrInputTooLarge) {
			return legacyTooLargeError(limits.MaxBytes)
		}
	} else {
		data, err = io.ReadAll(r)
	}
	if err != nil {
		return fmt.Errorf("read source file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if bytes.HasPrefix(data, []byte(directoryArtifactMagicV1)) {
		archive, err := decryptBytesWithAAD(data[len(directoryArtifactMagicV1):], password, []byte(directoryArtifactMagicV1))
		if err != nil {
			return err
		}
		return restoreDirectoryArchive(ctx, bytes.NewReader(archive), src, dst, limits)
	}

	plaintext, err := decryptBytes(data, password)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeFileAtomic(dst, plaintext); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

// EncryptedWithStoredKey reports whether the file at path is version 2 data encrypted
// with a stored key, and if so that key's ID, read from its header (plan 3.4). The CLI
// and the TUI call it before decrypting, to unlock the key instead of asking for a
// password. Only a regular file is read: reading a pipe here would consume the data
// that decrypting it needs. A file that isn't regular, can't be read, or whose header
// isn't valid, is reported as not encrypted with a stored key: decrypting it then
// reports the problem (ErrStoredKeyRequired for a stored-key file read from a pipe).
func EncryptedWithStoredKey(path string) (keyID string, ok bool) {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	f, err := os.Open(path) // #nosec G304 -- path is the file the user chose to decrypt
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }() // read-only: a close error can't lose data
	h, _, err := readV2Header(f)
	if err != nil || h.keySource != keySourceStoredKey {
		return "", false
	}
	return h.storedKeyID(), true
}

// legacyTooLargeError reports a legacy-format input larger than the output limit.
func legacyTooLargeError(limit int64) error {
	return fmt.Errorf("%w: a file without Cryptare's format header is read whole, and this one is larger than %s", ErrExtractLimit, formatSize(limit))
}

// writeStreamAtomic copies r to dst through a temporary file, so dst appears only if
// the whole stream was read without error.
func writeStreamAtomic(ctx context.Context, dst string, r io.Reader) error {
	out, err := createAtomicFile(dst)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer out.Abort()
	if _, err := copyContext(ctx, out, r); err != nil {
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

func decryptBytes(data []byte, password string) ([]byte, error) {
	return decryptBytesWithAAD(data, password, nil)
}

// decryptBytesWithAAD decrypts the legacy (version 1) layout: salt ‖ nonce ‖ ciphertext,
// with a PBKDF2 key derived from each form of password that may have protected it
// (passwordCandidates).
func decryptBytesWithAAD(data []byte, password string, aad []byte) ([]byte, error) {
	if len(data) < saltLen+ivLen {
		return nil, errors.New("file too small to be a valid encrypted file")
	}

	salt := data[:saltLen]
	iv := data[saltLen : saltLen+ivLen]
	ciphertext := data[saltLen+ivLen:]

	for _, candidate := range passwordCandidates(password, false) {
		key, err := deriveKey(candidate, salt)
		if err != nil {
			return nil, err
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("create cipher: %w", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("create GCM: %w", err)
		}
		if plaintext, err := gcm.Open(nil, iv, ciphertext, aad); err == nil {
			return plaintext, nil
		}
	}
	return nil, errDecrypt
}

// restoreDirectoryArchive extracts the decrypted tar.gz of an encrypted directory
// (read from src) into dst, through extractToDir and within limits. Whatever follows
// the archive in r is read too before the output is kept, so that a version 2 stream
// is authenticated to its final chunk.
func restoreDirectoryArchive(ctx context.Context, r io.Reader, src, dst string, limits ExtractLimits) (err error) {
	gz, err := gzip.NewReader(r)
	if errors.Is(err, errDecrypt) {
		return err // a wrong password shows at the first chunk
	}
	if err != nil {
		return fmt.Errorf("read decrypted directory archive: %w", err)
	}
	defer closeWithError(&err, gz, "close directory archive reader")

	budget := &extractBudget{limits: limits}
	return extractToDir(ctx, src, dst, func(dir string) error {
		if err := extractTarGz(ctx, gz, dir, budget); err != nil {
			return err
		}
		if _, err := copyContext(ctx, io.Discard, r); err != nil {
			return err
		}
		return nil
	})
}

//--------------------------------------------------key management---------------------------------------------------------------------------------------//

// GenerateKey generates a random 32-byte AES key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return key, nil
}

// EncryptKeyBlob encrypts rawKey with masterPassword and returns a base64 blob in the
// version 2 format (content type stored key). masterPassword must meet the password
// policy (CheckPasswordPolicy).
func EncryptKeyBlob(rawKey []byte, masterPassword string) (string, error) {
	if err := CheckPasswordPolicy(masterPassword); err != nil {
		return "", err
	}
	sealed, err := sealV2(rawKey, masterPassword, contentStoredKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptKeyBlob decrypts a base64 blob produced by EncryptKeyBlob, in the version 2 or
// the legacy format.
func DecryptKeyBlob(blob, masterPassword string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}
	return openKeyData(data, masterPassword, contentStoredKey)
}

// openKeyData decrypts a decoded stored key or key export with masterPassword: version
// 2 data must hold content, and data without a version 2 header is read in the legacy
// format.
func openKeyData(data []byte, masterPassword string, content byte) ([]byte, error) {
	var plaintext []byte
	var err error
	switch {
	case isV2(data):
		plaintext, err = openV2(data, masterPassword, content)
	case len(data) < saltLen+ivLen:
		return nil, errors.New("blob too small")
	default:
		plaintext, err = decryptBytes(data, masterPassword)
	}
	if errors.Is(err, errDecrypt) {
		if content == contentKeyExport {
			return nil, ErrWrongExportPassword
		}
		return nil, ErrWrongMasterPassword
	}
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

//--------------------------------------------------key export/import------------------------------------------------------------------------------------//

// KeyExport is the JSON-serializable export envelope written to disk.
type KeyExport struct {
	Version       int    `json:"version"`
	KeyID         string `json:"key_id"`
	Algorithm     string `json:"algorithm"`
	CreatedAt     int64  `json:"created_at"`
	EncryptedBlob string `json:"encrypted_blob"` // base64 AES-256-GCM ciphertext
}

// ExportKeyToFile writes an encrypted key export to the path. The export is protected
// by the key's own master password (BUG-011), so a key has one password wherever it is:
// masterPassword must unlock the key (ErrWrongMasterPassword otherwise), and, because
// it protects a new file, meet the password policy (CheckPasswordPolicy). A key stored
// by an earlier version under a shorter password therefore can't be exported. The file
// is the base64 text of version 2 data (content type key export).
func ExportKeyToFile(km *KeyModel, masterPassword, path string) error {
	if _, err := DecryptKeyBlob(km.EncryptedBlob, masterPassword); err != nil {
		return fmt.Errorf("export key %s: %w", displayText(km.KeyID), err)
	}
	if err := CheckPasswordPolicy(masterPassword); err != nil {
		return fmt.Errorf("export key %s: the export is protected by the key's master password, which can't protect a new file: %w", displayText(km.KeyID), err)
	}
	export := KeyExport{
		Version:       1,
		KeyID:         km.KeyID,
		Algorithm:     km.Algorithm,
		CreatedAt:     km.CreatedAt_,
		EncryptedBlob: km.EncryptedBlob,
	}

	raw, err := json.Marshal(export)
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}

	sealed, err := sealV2(raw, masterPassword, contentKeyExport)
	if err != nil {
		return fmt.Errorf("encrypt export: %w", err)
	}
	blob := base64.StdEncoding.EncodeToString(sealed)

	if path == "" {
		path = defaultExportPath(km.KeyID)
	}

	if err := writeFileAtomic(path, []byte(blob)); err != nil {
		return fmt.Errorf("write export file: %w", err)
	}
	return nil
}

// timeNow is the clock used for default export file names; tests replace it.
var timeNow = time.Now

// defaultExportPath returns the default export file name for keyID,
// <key-id>-<unix time>.ckey in the current directory. Callers compute it once and pass
// it on, so the name they report is the file written (BUG-006).
func defaultExportPath(keyID string) string {
	return fmt.Sprintf("%s-%d.ckey", keyID, timeNow().Unix())
}

// defaultEncryptOutput returns the output EncryptFile writes when none is given: src +
// ".enc" for a file, and for a folder a file next to it rather than inside it
// (BUG-016), so "dir/" gives "dir.enc" and "." gives "<parent>/<name>.enc". The CLI
// and the TUI use it too, so the name they report is the file written.
func defaultEncryptOutput(src string) string {
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return src + encExt // EncryptFile reports a missing source
	}
	clean := filepath.Clean(src)
	switch filepath.Base(clean) {
	case ".", "..", string(filepath.Separator):
		abs, err := filepath.Abs(clean)
		if err != nil {
			return clean + encExt // refused as inside the folder, which is safe
		}
		return filepath.Join(filepath.Dir(abs), filepath.Base(abs)+encExt)
	}
	return clean + encExt
}

// checkOutputOutsideFolder returns ErrOutputInsideInput when src is a folder and dst
// lies inside it (BUG-016): the archive would then hold its own partial output, and
// deleting the folder afterwards would delete the encrypted copy too.
func checkOutputOutsideFolder(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return nil // EncryptFile reports a missing source
	}
	return checkOutputOutsideDir(src, dst)
}

// defaultDecryptOutput strips the ".enc" extension from src, in any letter case
// (BUG-007), or appends ".dec" when there is none.
func defaultDecryptOutput(src string) string {
	if len(src) > len(encExt) && hasSuffixFold(src, encExt) {
		return src[:len(src)-len(encExt)]
	}
	return src + ".dec"
}

// ImportKeyFromFile reads an export file, in the version 2 or the legacy format, and
// returns a KeyModel (not yet persisted).
func ImportKeyFromFile(path, masterPassword string) (*KeyModel, error) {
	f, err := os.Open(path) // #nosec G304 -- path is the key export the user chose to import
	if err != nil {
		return nil, fmt.Errorf("read export file: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only: a close error can't lose data
	raw, err := readAllLimit(f, maxKeyExportSize)
	if err != nil {
		return nil, fmt.Errorf("read export file: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, fmt.Errorf("decrypt export file: decode blob: %w", err)
	}

	plaintext, err := openKeyData(data, masterPassword, contentKeyExport)
	if err != nil {
		return nil, fmt.Errorf("decrypt export file: %w", err)
	}

	// Parse minimal JSON manually to avoid import cycle (encoding/json is fine here).
	var export KeyExport
	if err := unmarshalKeyExport(plaintext, &export); err != nil {
		return nil, fmt.Errorf("parse export: %w", err)
	}
	if err := validateKeyExport(export); err != nil {
		return nil, err
	}

	return &KeyModel{
		KeyID:         export.KeyID,
		Algorithm:     export.Algorithm,
		EncryptedBlob: export.EncryptedBlob,
		CreatedAt_:    export.CreatedAt,
	}, nil
}

const (
	// maxKeyExportSize is the most ImportKeyFromFile reads (BUG-024); a real export is
	// under 1 KiB.
	maxKeyExportSize = 1 << 20
	keyExportVersion = 1
	keyAlgorithm     = "AES-256-GCM"
	keyIDLen         = 16 // hex characters, from newKeyID
	// legacyStoredKeyBlobLen is the decoded size of a legacy stored key blob: salt,
	// nonce, and the 32-byte key sealed with a GCM tag.
	legacyStoredKeyBlobLen = saltLen + ivLen + keyLen + gcmTagLen
	// v2StoredKeyBlobLen is the decoded size of a version 2 stored key blob: the header
	// and one final chunk holding the 32-byte key.
	v2StoredKeyBlobLen = v2HeaderLen + keyLen + gcmTagLen
)

// validateKeyExport checks an imported export's fields before they are stored and
// shown by keys list and the TUI (SEC-011): the version must be 1, the key ID 16
// lower-case hex characters as newKeyID makes, the algorithm AES-256-GCM, and the
// encrypted key a base64 blob shaped like one EncryptKeyBlob produces, in the legacy
// or the version 2 format (validStoredKeyBlob). Rejected values
// are quoted with %q, so control characters in them are escaped, not printed.
func validateKeyExport(e KeyExport) error {
	if e.Version != keyExportVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidKeyExport, e.Version)
	}
	if !isKeyID(e.KeyID) {
		return fmt.Errorf("%w: key ID %q is not %d lower-case hexadecimal characters", ErrInvalidKeyExport, e.KeyID, keyIDLen)
	}
	if e.Algorithm != keyAlgorithm {
		return fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidKeyExport, e.Algorithm)
	}
	blob, err := base64.StdEncoding.DecodeString(e.EncryptedBlob)
	if err != nil || !validStoredKeyBlob(blob) {
		return fmt.Errorf("%w: the encrypted key is malformed", ErrInvalidKeyExport)
	}
	return nil
}

// validStoredKeyBlob reports whether blob has the shape of a stored key: a legacy blob
// of the legacy size, or a version 2 stored-key blob, whose header passes
// parseV2Header's checks, with exactly one chunk holding a 32-byte key.
func validStoredKeyBlob(blob []byte) bool {
	if !isV2(blob) {
		return len(blob) == legacyStoredKeyBlobLen
	}
	h, err := parseV2Header(blob)
	return err == nil && h.content == contentStoredKey && len(blob) == v2StoredKeyBlobLen
}

// isKeyID reports whether s has the format of a generated key ID.
func isKeyID(s string) bool {
	if len(s) != keyIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// unmarshalKeyExport parses a JSON export envelope.
func unmarshalKeyExport(data []byte, out *KeyExport) error {
	return json.Unmarshal(data, out)
}

// newKeyID generates a random hex key ID.
func newKeyID() (string, error) {
	b := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
