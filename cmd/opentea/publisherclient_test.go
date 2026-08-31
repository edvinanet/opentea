// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/pkg/teapublisher"
	"github.com/oej/opentea/pkg/teapublisherclient"
)

// TestPublisherClientFullWorkflow drives the same end-to-end workflow
// TestPublisherFullWorkflow (publisher_test.go) exercises over raw HTTP,
// but this time through pkg/teapublisherclient itself -- the whole point
// of moving the client library into this repo (design/publisher-service.md
// §17): it can be tested directly against the real internal/publisher
// server, not just against a fake one, catching any drift between what
// the client assumes and what the server actually does.
func TestPublisherClientFullWorkflow(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)
	ctx := context.Background()

	fullClient := teapublisherclient.NewClient(srv.URL+"/publisher/v1", full)
	cicdClient := teapublisherclient.NewClient(srv.URL+"/publisher/v1", cicd)

	product, err := fullClient.CreateProduct(ctx, teapublisher.ProductCreate{Name: "Acme Widget"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	release, err := fullClient.CreateProductRelease(ctx, product.UUID, teapublisher.ProductReleaseCreate{
		Version:     "1.0.0",
		CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	artifact, err := cicdClient.CreateArtifact(ctx, teapublisher.ArtifactCreate{
		Type:    "BOM",
		Formats: []teapublisher.ArtifactFormatCreate{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	prepared, err := cicdClient.PrepareArtifactEvidence(ctx, artifact.UUID, 1)
	if err != nil {
		t.Fatalf("PrepareArtifactEvidence: %v", err)
	}
	sigValue, certPEM := signDigest(t, prepared.DigestToSign)
	if _, err := cicdClient.SubmitArtifactEvidence(ctx, artifact.UUID, 1, teapublisher.EvidenceSubmission{
		ObjectDigestValue: prepared.DigestToSign,
		SignatureFormat:   "jws-detached",
		SignatureValue:    sigValue,
		CertificatePEM:    certPEM,
	}); err != nil {
		t.Fatalf("SubmitArtifactEvidence: %v", err)
	}

	if _, err := cicdClient.PutProductReleaseCollectionDraft(ctx, release.UUID, teapublisher.CollectionDraftArtifactList{
		Actor:     "ci-pipeline",
		Artifacts: []teapublisher.ArtifactVersionRef{{UUID: artifact.UUID, Version: 1}},
	}); err != nil {
		t.Fatalf("PutProductReleaseCollectionDraft: %v", err)
	}

	if _, err := cicdClient.ApproveProductReleaseCollectionDraft(ctx, release.UUID, teapublisher.ApprovalDecision{Actor: "reviewer"}); !teapublisherclient.IsForbidden(err) {
		t.Fatalf("cicd approve: err = %v, want IsForbidden", err)
	}
	if _, err := fullClient.ApproveProductReleaseCollectionDraft(ctx, release.UUID, teapublisher.ApprovalDecision{Actor: "reviewer"}); err != nil {
		t.Fatalf("ApproveProductReleaseCollectionDraft: %v", err)
	}

	preparedCommit, err := cicdClient.PrepareProductReleaseCollectionCommit(ctx, release.UUID)
	if err != nil {
		t.Fatalf("PrepareProductReleaseCollectionCommit: %v", err)
	}
	commitSig, commitCertPEM := signDigest(t, preparedCommit.DigestToSign)
	collection, err := cicdClient.CommitProductReleaseCollectionDraft(ctx, release.UUID, teapublisher.EvidenceSubmission{
		ObjectDigestValue: preparedCommit.DigestToSign,
		SignatureFormat:   "jws-detached",
		SignatureValue:    commitSig,
		CertificatePEM:    commitCertPEM,
	})
	if err != nil {
		t.Fatalf("CommitProductReleaseCollectionDraft: %v", err)
	}
	if collection.Version != 1 || len(collection.Artifacts) != 1 || collection.Artifacts[0].UUID != artifact.UUID {
		t.Fatalf("collection = %+v", collection)
	}

	if _, err := cicdClient.GetProductReleaseCollectionDraft(ctx, release.UUID); !teapublisherclient.IsNotFound(err) {
		t.Fatalf("draft after commit: err = %v, want IsNotFound", err)
	}
}
