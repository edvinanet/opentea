// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testDraftTTL = time.Hour

func createTestProductReleaseForDraft(t *testing.T, ctx context.Context, r *Repo) (productReleaseUUID string) {
	t.Helper()
	product, err := r.CreateProduct(ctx, "Acme Widget", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	pr, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{
		Version:     "1.0.0",
		CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	return pr.UUID
}

func testCommitEvidence(digest string) CommitEvidenceInput {
	return CommitEvidenceInput{
		ObjectDigestValue:      digest,
		SignatureFormat:        "jws-detached",
		SignatureValue:         "c2lnbmF0dXJl",
		CertificateFormat:      "x509-pem",
		CertificateValue:       "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
		CertificateFingerprint: "fp-" + digest,
		CertificateTrustDomain: "trust.example.com",
	}
}

func TestPutGetDeleteCollectionDraft(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)
	artifactUUID, artifactVersion := createTestArtifactForEvidence(t, ctx, r)

	draft, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline",
		[]ArtifactRef{{UUID: artifactUUID, Version: artifactVersion}}, nil, testDraftTTL)
	if err != nil {
		t.Fatalf("PutCollectionDraft: %v", err)
	}
	if draft.Revision != 1 {
		t.Fatalf("Revision = %d, want 1", draft.Revision)
	}
	if draft.DraftedBy != "ci-pipeline" {
		t.Fatalf("DraftedBy = %q", draft.DraftedBy)
	}
	if draft.Approval == nil || draft.Approval.Status != "none" {
		t.Fatalf("Approval = %+v, want status none", draft.Approval)
	}
	if len(draft.DiffAgainstCurrent.Added) != 1 || draft.DiffAgainstCurrent.Added[0] != artifactUUID {
		t.Fatalf("DiffAgainstCurrent.Added = %v, want [%s]", draft.DiffAgainstCurrent.Added, artifactUUID)
	}

	// Editing bumps revision and leaves approval at none.
	draft2, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline",
		[]ArtifactRef{{UUID: artifactUUID, Version: artifactVersion}}, nil, testDraftTTL)
	if err != nil {
		t.Fatalf("PutCollectionDraft (2nd): %v", err)
	}
	if draft2.Revision != 2 {
		t.Fatalf("Revision = %d, want 2", draft2.Revision)
	}

	got, err := r.GetCollectionDraft(ctx, BelongsToProductRelease, prUUID)
	if err != nil {
		t.Fatalf("GetCollectionDraft: %v", err)
	}
	if got.Revision != 2 {
		t.Fatalf("GetCollectionDraft.Revision = %d, want 2", got.Revision)
	}

	if err := r.DeleteCollectionDraft(ctx, BelongsToProductRelease, prUUID); err != nil {
		t.Fatalf("DeleteCollectionDraft: %v", err)
	}
	if _, err := r.GetCollectionDraft(ctx, BelongsToProductRelease, prUUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCollectionDraft after delete: err = %v, want ErrNotFound", err)
	}
}

func TestPutCollectionDraftUnknownOwnerOrArtifactNotFound(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)

	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, "00000000-0000-0000-0000-000000000000", "ci", nil, nil, testDraftTTL); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown owner: err = %v, want ErrNotFound", err)
	}
	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci",
		[]ArtifactRef{{UUID: "00000000-0000-0000-0000-000000000000", Version: 1}}, nil, testDraftTTL); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown artifact: err = %v, want ErrNotFound", err)
	}
}

func TestDecideCollectionDraftSelfApprovalRejected(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)
	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL); err != nil {
		t.Fatalf("PutCollectionDraft: %v", err)
	}

	if _, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", "", true, time.Hour); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("err = %v, want ErrSelfApproval", err)
	}
}

func TestDecideCollectionDraftThenEditResetsApproval(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)
	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL); err != nil {
		t.Fatalf("PutCollectionDraft: %v", err)
	}
	approved, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "reviewer", "looks good", true, time.Hour)
	if err != nil {
		t.Fatalf("DecideCollectionDraft (approve): %v", err)
	}
	if approved.Approval.Status != "approved" || approved.Approval.DecidedBy != "reviewer" {
		t.Fatalf("Approval = %+v", approved.Approval)
	}

	edited, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL)
	if err != nil {
		t.Fatalf("PutCollectionDraft (edit after approve): %v", err)
	}
	if edited.Approval.Status != "none" {
		t.Fatalf("Approval.Status after edit = %q, want none", edited.Approval.Status)
	}
}

func TestPrepareCollectionCommitRequiresApproval(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)
	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL); err != nil {
		t.Fatalf("PutCollectionDraft: %v", err)
	}

	if _, err := r.PrepareCollectionCommit(ctx, BelongsToProductRelease, prUUID, time.Hour); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("err = %v, want ErrApprovalRequired", err)
	}

	// A rejected decision doesn't satisfy the requirement either.
	if _, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "reviewer", "no", false, time.Hour); err != nil {
		t.Fatalf("DecideCollectionDraft (reject): %v", err)
	}
	if _, err := r.PrepareCollectionCommit(ctx, BelongsToProductRelease, prUUID, time.Hour); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("after reject: err = %v, want ErrApprovalRequired", err)
	}

	// An expired approval doesn't satisfy it either.
	if _, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "reviewer", "ok but late", true, -time.Hour); err != nil {
		t.Fatalf("DecideCollectionDraft (expired approve): %v", err)
	}
	if _, err := r.PrepareCollectionCommit(ctx, BelongsToProductRelease, prUUID, time.Hour); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("after expired approval: err = %v, want ErrApprovalRequired", err)
	}
}

func TestCollectionDraftLockRejectsPutAndDecide(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)
	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL); err != nil {
		t.Fatalf("PutCollectionDraft: %v", err)
	}
	if _, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "reviewer", "", true, time.Hour); err != nil {
		t.Fatalf("DecideCollectionDraft: %v", err)
	}
	if _, err := r.PrepareCollectionCommit(ctx, BelongsToProductRelease, prUUID, time.Hour); err != nil {
		t.Fatalf("PrepareCollectionCommit: %v", err)
	}

	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL); !errors.Is(err, ErrDraftLocked) {
		t.Fatalf("PutCollectionDraft while locked: err = %v, want ErrDraftLocked", err)
	}
	if _, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "reviewer2", "", true, time.Hour); !errors.Is(err, ErrDraftLocked) {
		t.Fatalf("DecideCollectionDraft while locked: err = %v, want ErrDraftLocked", err)
	}

	if err := r.CancelPrepareCollectionCommit(ctx, BelongsToProductRelease, prUUID); err != nil {
		t.Fatalf("CancelPrepareCollectionCommit: %v", err)
	}
	if err := r.CancelPrepareCollectionCommit(ctx, BelongsToProductRelease, prUUID); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("CancelPrepareCollectionCommit (2nd): err = %v, want ErrLockNotHeld", err)
	}

	// Lock released -- put now succeeds again.
	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL); err != nil {
		t.Fatalf("PutCollectionDraft after cancel: %v", err)
	}
}

func TestCommitCollectionDraftWithoutPrepareRejected(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)
	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline", nil, nil, testDraftTTL); err != nil {
		t.Fatalf("PutCollectionDraft: %v", err)
	}
	if _, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "reviewer", "", true, time.Hour); err != nil {
		t.Fatalf("DecideCollectionDraft: %v", err)
	}

	if _, err := r.CommitCollectionDraft(ctx, BelongsToProductRelease, prUUID, testCommitEvidence("deadbeef")); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("commit without prepare: err = %v, want ErrLockNotHeld", err)
	}
}

func TestCollectionDraftFullHappyPath(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	prUUID := createTestProductReleaseForDraft(t, ctx, r)
	artifactUUID, artifactVersion := createTestArtifactForEvidence(t, ctx, r)

	if _, err := r.PutCollectionDraft(ctx, BelongsToProductRelease, prUUID, "ci-pipeline",
		[]ArtifactRef{{UUID: artifactUUID, Version: artifactVersion}}, nil, testDraftTTL); err != nil {
		t.Fatalf("PutCollectionDraft: %v", err)
	}
	if _, err := r.DecideCollectionDraft(ctx, BelongsToProductRelease, prUUID, "reviewer", "lgtm", true, time.Hour); err != nil {
		t.Fatalf("DecideCollectionDraft: %v", err)
	}

	wouldBe, err := r.PrepareCollectionCommit(ctx, BelongsToProductRelease, prUUID, time.Hour)
	if err != nil {
		t.Fatalf("PrepareCollectionCommit: %v", err)
	}
	if wouldBe.Version != 1 {
		t.Fatalf("wouldBe.Version = %d, want 1", wouldBe.Version)
	}
	if len(wouldBe.Artifacts) != 1 || wouldBe.Artifacts[0].UUID != artifactUUID {
		t.Fatalf("wouldBe.Artifacts = %+v", wouldBe.Artifacts)
	}

	peeked, err := r.PeekCollectionDraftCommit(ctx, BelongsToProductRelease, prUUID)
	if err != nil {
		t.Fatalf("PeekCollectionDraftCommit: %v", err)
	}
	if peeked.Version != wouldBe.Version {
		t.Fatalf("peeked.Version = %d, want %d", peeked.Version, wouldBe.Version)
	}

	collection, err := r.CommitCollectionDraft(ctx, BelongsToProductRelease, prUUID, testCommitEvidence("deadbeef"))
	if err != nil {
		t.Fatalf("CommitCollectionDraft: %v", err)
	}
	if collection.UUID != prUUID || collection.Version != 1 {
		t.Fatalf("collection = %+v", collection)
	}

	// The draft is gone.
	if _, err := r.GetCollectionDraft(ctx, BelongsToProductRelease, prUUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCollectionDraft after commit: err = %v, want ErrNotFound", err)
	}

	// The collection is really there, and carries its evidence bundle.
	stored, err := r.GetCollectionByVersion(ctx, prUUID, 1, BelongsToProductRelease)
	if err != nil {
		t.Fatalf("GetCollectionByVersion: %v", err)
	}
	if len(stored.Artifacts) != 1 || stored.Artifacts[0].UUID != artifactUUID {
		t.Fatalf("stored.Artifacts = %+v", stored.Artifacts)
	}
	bundle, err := r.GetEvidenceBundleForOwner(ctx, "COLLECTION", prUUID, 1)
	if err != nil {
		t.Fatalf("GetEvidenceBundleForOwner: %v", err)
	}
	if bundle.Object.Digest.Value != "deadbeef" {
		t.Fatalf("bundle.Object.Digest.Value = %q", bundle.Object.Digest.Value)
	}
}
