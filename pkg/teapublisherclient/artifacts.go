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
// (teapublisher.ArtifactCreate has no such field). The response's
// FormatIDs (docs/security-review-publisher-design-260828.md finding 14)
// are each format's stable, server-assigned id, in the same order as
// Formats -- the only way to learn one; pass one to
// UploadArtifactFileByFormatID/UploadArtifactSignatureFileByFormatID
// instead of a mediaType when two formats might share a media type.
func (c *Client) CreateArtifact(ctx context.Context, in teapublisher.ArtifactCreate) (teapublisher.ArtifactCreated, error) {
	var created teapublisher.ArtifactCreated
	err := c.do(ctx, "POST", "/artifacts", in, &created)
	return created, err
}

// UploadArtifactFile uploads the file content for one format of artifact
// (artifactUUID, version), selected by mediaType -- the same value that
// format was created with (teapublisher.ArtifactFormatCreate.MediaType).
// Rejected (400, IsBadRequest) if more than one format shares that media
// type -- use UploadArtifactFileByFormatID instead in that case
// (POST /artifacts/{uuid}/{version}/files, design/publisher-openapi.yaml
// v0.10/v0.14). r is read to completion, not closed by this method.
// Returns an *APIError with StatusCode 409 (IsConflict) if evidence has
// already been submitted for this artifact version -- file content is
// frozen once validated.
func (c *Client) UploadArtifactFile(ctx context.Context, artifactUUID string, version int, mediaType, filename string, r io.Reader) (teapublisher.ArtifactFileUploaded, error) {
	return c.doUpload(ctx, fmt.Sprintf("/artifacts/%s/%d/files", artifactUUID, version), "mediaType", mediaType, filename, r)
}

// UploadArtifactFileByFormatID is UploadArtifactFile addressed by a
// format's stable id (CreateArtifact's own ArtifactCreated.FormatIDs)
// instead of its mediaType -- unambiguous even when two formats of the
// same artifact share a media type (docs/security-review-publisher-design-260828.md
// finding 14).
func (c *Client) UploadArtifactFileByFormatID(ctx context.Context, artifactUUID string, version int, formatID, filename string, r io.Reader) (teapublisher.ArtifactFileUploaded, error) {
	return c.doUpload(ctx, fmt.Sprintf("/artifacts/%s/%d/files", artifactUUID, version), "formatId", formatID, filename, r)
}

// UploadArtifactSignatureFile uploads a detached signature for one format of
// artifact (artifactUUID, version), selected by mediaType the same way as
// UploadArtifactFile (POST /artifacts/{uuid}/{version}/signature/files).
// Unlike UploadArtifactFile, this is never rejected once evidence has been
// submitted -- a signature upload doesn't change the format's content
// checksum evidence attests to. r is read to completion, not closed by this
// method.
func (c *Client) UploadArtifactSignatureFile(ctx context.Context, artifactUUID string, version int, mediaType, filename string, r io.Reader) (teapublisher.ArtifactFileUploaded, error) {
	return c.doUpload(ctx, fmt.Sprintf("/artifacts/%s/%d/signature/files", artifactUUID, version), "mediaType", mediaType, filename, r)
}

// UploadArtifactSignatureFileByFormatID is UploadArtifactSignatureFile
// addressed by a format's stable id instead of its mediaType -- see
// UploadArtifactFileByFormatID's own doc comment.
func (c *Client) UploadArtifactSignatureFileByFormatID(ctx context.Context, artifactUUID string, version int, formatID, filename string, r io.Reader) (teapublisher.ArtifactFileUploaded, error) {
	return c.doUpload(ctx, fmt.Sprintf("/artifacts/%s/%d/signature/files", artifactUUID, version), "formatId", formatID, filename, r)
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
