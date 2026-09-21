// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisherclient

import (
	"context"
	"fmt"
	"io"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// CreateArtifact creates an artifact -- metadata only, file content
// (UploadArtifactFile) and evidence (PrepareArtifactEvidence/
// SubmitArtifactEvidence) are attached by separate calls
// (POST /artifacts). createdDate is target-assigned; in never carries one
// (teapublisher.ArtifactCreate has no such field).
func (c *Client) CreateArtifact(ctx context.Context, in teapublisher.ArtifactCreate) (tea.Artifact, error) {
	var a tea.Artifact
	err := c.do(ctx, "POST", "/artifacts", in, &a)
	return a, err
}

// UploadArtifactFile uploads the file content for one format of artifact
// (artifactUUID, version), selected by mediaType -- the same value that
// format was created with (teapublisher.ArtifactFormatCreate.MediaType),
// not a positional index (POST /artifacts/{uuid}/{version}/files,
// design/publisher-openapi.yaml v0.10). r is read to completion, not
// closed by this method. Returns an *APIError with StatusCode 409
// (IsConflict) if evidence has already been submitted for this artifact
// version -- file content is frozen once validated.
func (c *Client) UploadArtifactFile(ctx context.Context, artifactUUID string, version int, mediaType, filename string, r io.Reader) error {
	return c.doUpload(ctx, fmt.Sprintf("/artifacts/%s/%d/files", artifactUUID, version), mediaType, filename, r)
}

// UploadArtifactSignatureFile uploads a detached signature for one format of
// artifact (artifactUUID, version), selected by mediaType the same way as
// UploadArtifactFile (POST /artifacts/{uuid}/{version}/signature/files).
// Unlike UploadArtifactFile, this is never rejected once evidence has been
// submitted -- a signature upload doesn't change the format's content
// checksum evidence attests to. r is read to completion, not closed by this
// method.
func (c *Client) UploadArtifactSignatureFile(ctx context.Context, artifactUUID string, version int, mediaType, filename string, r io.Reader) error {
	return c.doUpload(ctx, fmt.Sprintf("/artifacts/%s/%d/signature/files", artifactUUID, version), mediaType, filename, r)
}

// PrepareArtifactEvidence returns the current artifact and the digest to
// sign over it (POST /artifacts/{uuid}/{version}/evidence/prepare).
// Callable repeatedly -- reflects current state each time, locks nothing.
func (c *Client) PrepareArtifactEvidence(ctx context.Context, artifactUUID string, version int) (teapublisher.PrepareArtifactEvidenceResponse, error) {
	var resp teapublisher.PrepareArtifactEvidenceResponse
	err := c.do(ctx, "POST", fmt.Sprintf("/artifacts/%s/%d/evidence/prepare", artifactUUID, version), nil, &resp)
	return resp, err
}

// SubmitArtifactEvidence submits the evidence package produced by signing
// PrepareArtifactEvidence's digest (POST /artifacts/{uuid}/{version}/evidence).
// The target re-derives the digest itself and rejects (400, IsBadRequest)
// anything that fails verification -- nothing is stored on failure.
func (c *Client) SubmitArtifactEvidence(ctx context.Context, artifactUUID string, version int, in teapublisher.EvidenceSubmission) (tea.EvidenceBundle, error) {
	var bundle tea.EvidenceBundle
	err := c.do(ctx, "POST", fmt.Sprintf("/artifacts/%s/%d/evidence", artifactUUID, version), in, &bundle)
	return bundle, err
}
