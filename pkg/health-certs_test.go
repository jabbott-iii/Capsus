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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testCertDER creates a self-signed certificate valid from notBefore to
// notAfter.
func testCertDER(t *testing.T, name string, notBefore, notAfter time.Time) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return der
}

func pemBlock(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

func writeTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestCheckCertExpiry(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	valid := testCertDER(t, "valid.example", now.Add(-day), now.Add(90*day+time.Hour))
	expiring := testCertDER(t, "expiring.example", now.Add(-day), now.Add(10*day+time.Hour))
	expired := testCertDER(t, "expired.example", now.Add(-30*day), now.Add(-5*day-time.Hour))
	future := testCertDER(t, "future.example", now.Add(2*day), now.Add(90*day+time.Hour))

	tests := []struct {
		name       string
		data       []byte
		warnDays   int
		wantStatus CheckStatus
		wantText   string
		wantDays   int // whole days of the value, truncated
	}{
		{"valid", pemBlock("CERTIFICATE", valid), 0, StatusPass, "valid.example expires " + now.Add(90*day+time.Hour).UTC().Format(time.DateOnly) + " (in 90 days)", 90},
		{"expiring", pemBlock("CERTIFICATE", expiring), 0, StatusWarn, "expiring.example expires " + now.Add(10*day+time.Hour).UTC().Format(time.DateOnly) + " (in 10 days)", 10},
		{"expiring outside a shorter window", pemBlock("CERTIFICATE", expiring), 7, StatusPass, "(in 10 days)", 10},
		{"expired", pemBlock("CERTIFICATE", expired), 0, StatusFail, "expired.example expired " + now.Add(-5*day-time.Hour).UTC().Format(time.DateOnly) + " (5 days ago)", -5},
		{"not yet valid", pemBlock("CERTIFICATE", future), 0, StatusFail, "future.example is not valid until ", 90},
		{"DER file", valid, 0, StatusPass, "valid.example expires", 90},
		{
			"chain reports the first to expire",
			append(append(pemBlock("CERTIFICATE", valid), pemBlock("CERTIFICATE", expiring)...), pemBlock("CERTIFICATE", valid)...),
			0, StatusWarn, "expiring.example (first to expire of 3) expires", 10,
		},
		{
			"private key in the same file is skipped",
			append(pemBlock("EC PRIVATE KEY", []byte("secret key material")), pemBlock("CERTIFICATE", valid)...),
			0, StatusPass, "valid.example expires", 90,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestFile(t, "cert.pem", tt.data)
			got := checkCertExpiry(CheckOptions{CertPath: path, CertWarnDays: tt.warnDays})
			if got.Key != keyCertExpiry || got.Status != tt.wantStatus || !strings.HasPrefix(got.Message, path+": ") || !strings.Contains(got.Message, tt.wantText) {
				t.Errorf("checkCertExpiry() = {%s %q}, want %s containing %q", got.Status, got.Message, tt.wantStatus, tt.wantText)
			}
			if got.Value == nil || int(*got.Value) != tt.wantDays || got.Unit != unitDays {
				t.Errorf("value = %v %q, want about %d days", got.Value, got.Unit, tt.wantDays)
			}
			if strings.Contains(got.Message, "secret") {
				t.Errorf("message %q leaks file contents", got.Message)
			}
		})
	}
}

func TestCheckCertExpiryFileProblems(t *testing.T) {
	dir := t.TempDir()
	keyOnly := writeTestFile(t, "key.pem", pemBlock("PRIVATE KEY", []byte("secret key material")))
	garbage := writeTestFile(t, "garbage.pem", []byte("not a certificate"))
	badCert := writeTestFile(t, "bad.pem", pemBlock("CERTIFICATE", []byte("not DER")))
	huge := writeTestFile(t, "huge.pem", make([]byte, maxCertFileSize+1))
	missing := filepath.Join(dir, "missing.pem")

	tests := []struct {
		path, want string
	}{
		{keyOnly, keyOnly + ": " + errNoCertificate.Error()},
		{garbage, garbage + ": " + errNoCertificate.Error()},
		{badCert, badCert + ": parse certificate: "},
		{huge, huge + fmt.Sprintf(": file is larger than %d bytes", maxCertFileSize)},
		{missing, missing + ": "},
	}
	for _, tt := range tests {
		got := checkCertExpiry(CheckOptions{CertPath: tt.path})
		if got.Status != StatusFail || !strings.HasPrefix(got.Message, tt.want) {
			t.Errorf("checkCertExpiry(%s) = {%s %q}, want FAIL starting with %q", filepath.Base(tt.path), got.Status, got.Message, tt.want)
		}
		if strings.Contains(got.Message, "secret") || strings.Count(got.Message, tt.path) != 1 {
			t.Errorf("message %q leaks contents or repeats the path", got.Message)
		}
	}
}

func TestCertOutcomeFarFutureExpiry(t *testing.T) {
	// 9999-12-31 is RFC 5280's "no well-defined expiration"; time.Time.Sub
	// would saturate at about 292 years.
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "forever"}, NotBefore: now.Add(-time.Hour), NotAfter: notAfter}
	got := certOutcome("c.pem", cert, 1, 30, now)
	want := float64(notAfter.Unix()-now.Unix()) / 86400
	if got.Status != StatusPass || got.Value == nil || *got.Value != want || !strings.Contains(got.Message, "expires 9999-12-31") {
		t.Errorf("certOutcome() = {%s %q %v}, want PASS with %v days", got.Status, got.Message, got.Value, want)
	}
}

func TestCertOutcomeBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	cert := &x509.Certificate{Subject: pkix.Name{Organization: []string{"No CN"}}, NotBefore: now.Add(-time.Hour)}

	cert.NotAfter = now
	if got := certOutcome("c.pem", cert, 1, 30, now); got.Status != StatusFail {
		t.Errorf("expiring exactly now = %s, want FAIL", got.Status)
	}
	cert.NotAfter = now.Add(30 * 24 * time.Hour)
	if got := certOutcome("c.pem", cert, 1, 30, now); got.Status != StatusPass || !strings.Contains(got.Message, "O=No CN") {
		t.Errorf("exactly at the warning window = {%s %q}, want PASS naming the subject", got.Status, got.Message)
	}
	cert.NotAfter = now.Add(30*24*time.Hour - time.Second)
	if got := certOutcome("c.pem", cert, 1, 30, now); got.Status != StatusWarn {
		t.Errorf("just inside the warning window = %s, want WARN", got.Status)
	}
}
