// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func createTestArtifactForEvidence(t *testing.T, ctx context.Context, r *Repo) (uuid string, version int) {
	t.Helper()
	a, err := r.CreateArtifact(ctx, ArtifactInput{
		Name:    "sbom.json",
		Type:    "BOM",
		Formats: []ArtifactFormatInput{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	return a.UUID, a.Version
}

func testEvidenceBundleInput(ownerUUID string, ownerVersion int, fingerprint string) EvidenceBundleInput {
	return EvidenceBundleInput{
		OwnerType:              "ARTIFACT",
		OwnerUUID:              ownerUUID,
		OwnerVersion:           ownerVersion,
		ObjectType:             "artifact",
		ObjectDigestValue:      "deadbeef",
		SignatureFormat:        "jws-detached",
		SignatureValue:         "c2lnbmF0dXJl",
		CertificateFormat:      "x509-pem",
		CertificateValue:       "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
		CertificateFingerprint: fingerprint,
		CertificateTrustDomain: "trust.example.com",
	}
}

func TestCreateEvidenceBundleRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid, version := createTestArtifactForEvidence(t, ctx, r)

	created, err := r.CreateEvidenceBundle(ctx, testEvidenceBundleInput(uuid, version, "fp-roundtrip"))
	if err != nil {
		t.Fatalf("CreateEvidenceBundle: %v", err)
	}
	if created.Status != "draft" {
		t.Fatalf("Status = %q, want draft", created.Status)
	}
	if created.Object.Digest.Value != "deadbeef" {
		t.Fatalf("Object.Digest.Value = %q", created.Object.Digest.Value)
	}

	fetched, err := r.GetEvidenceBundle(ctx, created.UUID)
	if err != nil {
		t.Fatalf("GetEvidenceBundle: %v", err)
	}
	if fetched.Certificate.Certificate != created.Certificate.Certificate {
		t.Fatalf("round-tripped certificate mismatch")
	}

	byOwner, err := r.GetEvidenceBundleForOwner(ctx, "ARTIFACT", uuid, version)
	if err != nil {
		t.Fatalf("GetEvidenceBundleForOwner: %v", err)
	}
	if byOwner.UUID != created.UUID {
		t.Fatalf("GetEvidenceBundleForOwner returned %s, want %s", byOwner.UUID, created.UUID)
	}
}

func TestCreateEvidenceBundleUnknownOwnerNotFound(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	_, err := r.CreateEvidenceBundle(ctx, testEvidenceBundleInput("00000000-0000-0000-0000-000000000000", 1, "fp-no-owner"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestCreateEvidenceBundleRejectsFingerprintReuseStandalone forces the
// standalone-transaction path (a fresh *Repo, r.tx == nil) so
// CreateEvidenceBundle's own runInTx call opens and manages its own
// transaction -- mirrors TestImportArtifactConflictFailsFast's rationale
// (internal/repo/import_test.go): the fingerprint-reuse check is exactly
// the kind of conflict check that historically hit the r.conn()/dbtx-twin
// self-deadlock hazard when a Get*-style helper inside the transaction
// wrongly routed through r.conn() instead of the tx it's already running
// in. A short timeout turns a regression into a fast failure instead of a
// hang.
func TestCreateEvidenceBundleRejectsFingerprintReuseStandalone(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid1, version1 := createTestArtifactForEvidence(t, ctx, r)
	uuid2, version2 := createTestArtifactForEvidence(t, ctx, r)

	if _, err := r.CreateEvidenceBundle(ctx, testEvidenceBundleInput(uuid1, version1, "fp-reused")); err != nil {
		t.Fatalf("first CreateEvidenceBundle: %v", err)
	}

	shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := r.CreateEvidenceBundle(shortCtx, testEvidenceBundleInput(uuid2, version2, "fp-reused"))
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("CreateEvidenceBundle hung until context deadline instead of failing fast -- likely a dbtx self-deadlock regression")
	}
	if !errors.Is(err, ErrFingerprintReused) {
		t.Fatalf("second CreateEvidenceBundle: err = %v, want ErrFingerprintReused", err)
	}
}

// TestCreateEvidenceBundleRejectsFingerprintReuseInsideWithTx exercises the
// same conflict check composed inside an outer Repo.WithTx, the other half
// of the dbtx hazard coverage (see the standalone-path test above and
// TestImportArtifactConflictFailsFast's doc comment for why both call
// shapes need their own test).
func TestCreateEvidenceBundleRejectsFingerprintReuseInsideWithTx(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid1, version1 := createTestArtifactForEvidence(t, ctx, r)
	uuid2, version2 := createTestArtifactForEvidence(t, ctx, r)

	if _, err := r.CreateEvidenceBundle(ctx, testEvidenceBundleInput(uuid1, version1, "fp-reused-tx")); err != nil {
		t.Fatalf("first CreateEvidenceBundle: %v", err)
	}

	err := r.WithTx(ctx, func(tx *Repo) error {
		_, err := tx.CreateEvidenceBundle(ctx, testEvidenceBundleInput(uuid2, version2, "fp-reused-tx"))
		return err
	})
	if !errors.Is(err, ErrFingerprintReused) {
		t.Fatalf("err = %v, want ErrFingerprintReused", err)
	}
}

func TestMarkEvidenceBundleCompleteRequiresTimestampAndTransparency(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	uuid, version := createTestArtifactForEvidence(t, ctx, r)

	created, err := r.CreateEvidenceBundle(ctx, testEvidenceBundleInput(uuid, version, "fp-complete"))
	if err != nil {
		t.Fatalf("CreateEvidenceBundle: %v", err)
	}

	if _, err := r.MarkEvidenceBundleComplete(ctx, created.UUID); !errors.Is(err, ErrEvidenceBundleIncomplete) {
		t.Fatalf("with no timestamp/transparency: err = %v, want ErrEvidenceBundleIncomplete", err)
	}

	if err := r.AddEvidenceBundleTimestamp(ctx, created.UUID, EvidenceBundleTimestampInput{Token: "dGltZXN0YW1w"}); err != nil {
		t.Fatalf("AddEvidenceBundleTimestamp: %v", err)
	}
	if _, err := r.MarkEvidenceBundleComplete(ctx, created.UUID); !errors.Is(err, ErrEvidenceBundleIncomplete) {
		t.Fatalf("with only a timestamp: err = %v, want ErrEvidenceBundleIncomplete", err)
	}

	if err := r.AddEvidenceBundleTransparencyEntry(ctx, created.UUID, EvidenceBundleTransparencyInput{
		System: "scitt", EvidenceType: "receipt", BindingType: "signature", BindingDigestValue: "abc123", VerificationJSON: `{}`,
	}); err != nil {
		t.Fatalf("AddEvidenceBundleTransparencyEntry: %v", err)
	}
	if _, err := r.MarkEvidenceBundleComplete(ctx, created.UUID); !errors.Is(err, ErrEvidenceBundleIncomplete) {
		t.Fatalf("with only an scitt transparency entry: err = %v, want ErrEvidenceBundleIncomplete", err)
	}

	if err := r.AddEvidenceBundleTransparencyEntry(ctx, created.UUID, EvidenceBundleTransparencyInput{
		System: "rekor", EvidenceType: "inclusion-proof", BindingType: "signature", BindingDigestValue: "abc123", VerificationJSON: `{"logIndex":1}`,
	}); err != nil {
		t.Fatalf("AddEvidenceBundleTransparencyEntry: %v", err)
	}

	completed, err := r.MarkEvidenceBundleComplete(ctx, created.UUID)
	if err != nil {
		t.Fatalf("MarkEvidenceBundleComplete: %v", err)
	}
	if completed.Status != "complete" {
		t.Fatalf("Status = %q, want complete", completed.Status)
	}
	if len(completed.Timestamps) != 1 || len(completed.Transparency) != 2 {
		t.Fatalf("Timestamps/Transparency = %d/%d, want 1/2", len(completed.Timestamps), len(completed.Transparency))
	}
}
