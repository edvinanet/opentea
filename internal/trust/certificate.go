package trust

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// BuildCertificate wraps kp's public key in a short-lived, self-signed X.509
// certificate per x509-profile.md: SAN is a DNS name of
// "<fingerprint>.<trustDomain>", NotBefore/NotAfter mirror kp's validity
// window. Self-signed because this profile treats the certificate as "a
// validity wrapper around an ephemeral key," not a long-term CA-issued
// identity (x509-profile.md Sec 3) -- there is no CA-issuance flow in this
// phase; that only matters once a consumer chain-validates against a
// published trust anchor, a later phase (DNS/TAPS).
func BuildCertificate(kp KeyPair, trustDomain TrustDomain) (string, error) {
	fp := FingerprintOf(kp.Public)
	san := fmt.Sprintf("%s.%s", fp, trustDomain)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", fmt.Errorf("trust: generate certificate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: san},
		DNSNames:     []string{san},
		NotBefore:    kp.NotBefore,
		NotAfter:     kp.NotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, kp.Public, kp.Private)
	if err != nil {
		return "", fmt.Errorf("trust: create certificate: %w", err)
	}

	block := &pem.Block{Type: "CERTIFICATE", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

func parseCertificate(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("trust: not a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("trust: parse certificate: %w", err)
	}
	return cert, nil
}

// ParseCertificatePublicKey extracts and validates an Ed25519 public key
// from a PEM certificate, requiring at to fall within the certificate's
// notBefore/notAfter window. Returns an error if certPEM doesn't decode, its
// key isn't Ed25519, or at is outside the validity window -- callers use
// this to check a timestamp's instant falls within the signing cert's
// validity (a later phase wires the timestamp check in; this phase exposes
// the primitive).
func ParseCertificatePublicKey(certPEM string, at time.Time) (ed25519.PublicKey, error) {
	cert, err := parseCertificate(certPEM)
	if err != nil {
		return nil, err
	}
	pub, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("trust: certificate public key is not Ed25519")
	}
	if at.Before(cert.NotBefore) || at.After(cert.NotAfter) {
		return nil, fmt.Errorf("trust: %s is outside certificate validity [%s, %s]", at, cert.NotBefore, cert.NotAfter)
	}
	return pub, nil
}

// ParseCertificateSubject extracts the (fingerprint, trustDomain) pair
// encoded in a certificate's SAN, the "<fingerprint>.<trustDomain>" shape
// BuildCertificate produces. Used by internal/admin's evidence-bundle
// handler so a caller submitting signing material doesn't separately
// declare a trust domain that could disagree with what the certificate
// itself asserts -- the certificate's SAN is the one source of truth.
func ParseCertificateSubject(certPEM string) (Fingerprint, TrustDomain, error) {
	cert, err := parseCertificate(certPEM)
	if err != nil {
		return "", "", err
	}
	if len(cert.DNSNames) != 1 {
		return "", "", fmt.Errorf("trust: certificate SAN must contain exactly one DNS name, got %d", len(cert.DNSNames))
	}
	san := cert.DNSNames[0]
	idx := strings.IndexByte(san, '.')
	if idx <= 0 || idx == len(san)-1 {
		return "", "", fmt.Errorf("trust: SAN %q is not of the form <fingerprint>.<trustDomain>", san)
	}
	return Fingerprint(san[:idx]), TrustDomain(san[idx+1:]), nil
}
