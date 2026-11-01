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
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"
)

// maxCertFileSize bounds how much of a --cert file is read. Certificate
// chains are a few kilobytes; a larger file is not a certificate file.
const maxCertFileSize = 1 << 20

// errNoCertificate means a file holds no parseable certificate.
var errNoCertificate = errors.New("no certificate found (expected PEM CERTIFICATE blocks or one DER certificate)")

func (o CheckOptions) certWarnDays() int {
	if o.CertWarnDays <= 0 {
		return defaultCertWarnDays
	}
	return o.CertWarnDays
}

// checkCertExpiry reports when the certificate in opts.CertPath that expires
// first stops being valid. In a chain file that is usually the leaf, but an
// intermediate that expires sooner breaks the chain just the same. Only
// certificate metadata reaches the message; the file contents (which may
// include a private key) never do.
func checkCertExpiry(opts CheckOptions) CheckOutcome {
	start := time.Now()
	path := opts.CertPath

	certs, err := readCertificates(path)
	if err != nil {
		return CheckOutcome{Key: keyCertExpiry, Status: StatusFail, Message: fmt.Sprintf("%s: %v", path, err), Duration: time.Since(start)}
	}

	first := certs[0]
	for _, cert := range certs[1:] {
		if cert.NotAfter.Before(first.NotAfter) {
			first = cert
		}
	}

	outcome := certOutcome(path, first, len(certs), opts.certWarnDays(), time.Now())
	outcome.Duration = time.Since(start)
	return outcome
}

// certOutcome classifies cert, the first of count certificates in path to
// expire, at time now.
func certOutcome(path string, cert *x509.Certificate, count, warnDays int, now time.Time) CheckOutcome {
	name := cert.Subject.CommonName
	if name == "" {
		name = cert.Subject.String()
	}
	if count > 1 {
		name = fmt.Sprintf("%s (first to expire of %d)", name, count)
	}
	// Unix seconds, because time.Time.Sub saturates at about 292 years and
	// 9999-12-31 is the RFC 5280 "no expiry" date.
	days := float64(cert.NotAfter.Unix()-now.Unix()) / (24 * 60 * 60)
	expiry := cert.NotAfter.UTC().Format(time.DateOnly)

	switch {
	case now.Before(cert.NotBefore):
		return CheckOutcome{Key: keyCertExpiry, Status: StatusFail, Message: fmt.Sprintf("%s: %s is not valid until %s", path, name, cert.NotBefore.UTC().Format(time.RFC3339))}.
			withValue(days, unitDays)
	case !now.Before(cert.NotAfter):
		return CheckOutcome{Key: keyCertExpiry, Status: StatusFail, Message: fmt.Sprintf("%s: %s expired %s (%d days ago)", path, name, expiry, int(-days))}.
			withValue(days, unitDays)
	case days < float64(warnDays):
		return CheckOutcome{Key: keyCertExpiry, Status: StatusWarn, Message: fmt.Sprintf("%s: %s expires %s (in %d days)", path, name, expiry, int(days))}.
			withValue(days, unitDays)
	default:
		return CheckOutcome{Key: keyCertExpiry, Status: StatusPass, Message: fmt.Sprintf("%s: %s expires %s (in %d days)", path, name, expiry, int(days))}.
			withValue(days, unitDays)
	}
}

// readCertificates parses every certificate in a PEM file, or the single
// certificate in a DER file. Non-certificate PEM blocks, such as a private
// key in a combined file, are skipped.
func readCertificates(path string) ([]*x509.Certificate, error) {
	// Opening a FIFO would block until a writer appears.
	info, err := os.Stat(path)
	if err != nil {
		return nil, withoutPath(err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, withoutPath(err)
	}
	defer func() { _ = f.Close() }() // read-only; a close error cannot lose data

	data, err := io.ReadAll(io.LimitReader(f, maxCertFileSize+1))
	if err != nil {
		return nil, withoutPath(err)
	}
	if len(data) > maxCertFileSize {
		return nil, fmt.Errorf("file is larger than %d bytes, which is too large for a certificate file", maxCertFileSize)
	}

	var certs []*x509.Certificate
	sawPEM := false
	for rest := data; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		sawPEM = true
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		certs = append(certs, cert)
	}
	if !sawPEM {
		if cert, err := x509.ParseCertificate(data); err == nil {
			certs = append(certs, cert)
		}
	}
	if len(certs) == 0 {
		return nil, errNoCertificate
	}
	return certs, nil
}

// withoutPath drops the path from a file error, because messages already
// start with it.
func withoutPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
