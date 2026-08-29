// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package trust

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestBuildCertificateRoundTrip(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	certPEM, err := BuildCertificate(kp, "trust.example.com")
	if err != nil {
		t.Fatalf("BuildCertificate: %v", err)
	}

	pub, err := ParseCertificatePublicKey(certPEM, kp.NotBefore.Add(time.Minute))
	if err != nil {
		t.Fatalf("ParseCertificatePublicKey: %v", err)
	}
	if !bytes.Equal(pub, kp.Public) {
		t.Fatal("recovered public key does not match the original")
	}
}

func TestParseCertificateRejectsExpired(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	certPEM, err := BuildCertificate(kp, "trust.example.com")
	if err != nil {
		t.Fatalf("BuildCertificate: %v", err)
	}

	if _, err := ParseCertificatePublicKey(certPEM, kp.NotAfter.Add(time.Minute)); err == nil {
		t.Fatal("ParseCertificatePublicKey accepted an instant after NotAfter")
	}
	if _, err := ParseCertificatePublicKey(certPEM, kp.NotBefore.Add(-time.Minute)); err == nil {
		t.Fatal("ParseCertificatePublicKey accepted an instant before NotBefore")
	}
}

func TestBuildCertificateSAN(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	certPEM, err := BuildCertificate(kp, "trust.example.com")
	if err != nil {
		t.Fatalf("BuildCertificate: %v", err)
	}

	wantSAN := string(FingerprintOf(kp.Public)) + ".trust.example.com"
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatal("BuildCertificate did not return a decodable PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != wantSAN {
		t.Fatalf("SAN = %v, want [%s]", cert.DNSNames, wantSAN)
	}
}

func TestParseCertificateSubject(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	certPEM, err := BuildCertificate(kp, "trust.example.com")
	if err != nil {
		t.Fatalf("BuildCertificate: %v", err)
	}

	fp, domain, err := ParseCertificateSubject(certPEM, kp.Public)
	if err != nil {
		t.Fatalf("ParseCertificateSubject: %v", err)
	}
	if fp != FingerprintOf(kp.Public) {
		t.Fatalf("fingerprint = %s, want %s", fp, FingerprintOf(kp.Public))
	}
	if domain != "trust.example.com" {
		t.Fatalf("trustDomain = %s, want trust.example.com", domain)
	}
}

// TestParseCertificateSubjectRejectsFingerprintMismatch is the regression
// test for a real vulnerability: ParseCertificateSubject used to return
// whatever fingerprint string appeared in the certificate's SAN without
// checking it was actually derived from the certificate's own key. Since a
// self-signed certificate's SAN is just a string its own subject chose, a
// caller submitting evidence via internal/admin's evidence-bundle handler
// could sign with one key while claiming an arbitrary, unrelated
// fingerprint string -- defeating the used_fingerprint reuse ledger this
// function's result feeds (a signer could reuse the same key under
// many different claimed fingerprints, or collide with someone else's
// claimed fingerprint). This builds a certificate, self-signed by a real
// key, whose SAN claims a fingerprint that does NOT match that key.
func TestParseCertificateSubjectRejectsFingerprintMismatch(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}

	forgedSAN := "0000000000000000000000000000000000000000000000000000000000000000.trust.example.com"
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: forgedSAN},
		DNSNames:     []string{forgedSAN},
		NotBefore:    kp.NotBefore,
		NotAfter:     kp.NotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, kp.Public, kp.Private)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	if _, _, err := ParseCertificateSubject(certPEM, kp.Public); err == nil {
		t.Fatal("ParseCertificateSubject accepted a SAN fingerprint that does not match the certificate's own public key")
	}
}
