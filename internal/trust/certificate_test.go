package trust

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
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

	fp, domain, err := ParseCertificateSubject(certPEM)
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
