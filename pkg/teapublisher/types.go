// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package teapublisher holds the JSON-facing wire types for the draft
// standard TEA Publisher API (design/publisher-openapi.yaml,
// design/publisher-service.md §8) -- the publisher-only delta with no
// /tea/v1 equivalent (collection-draft staging, evidence submission,
// approval, prepare/commit responses, and the "-create" request shapes).
// Most of design/publisher-openapi.yaml's schemas are pkg/tea's own types
// reused verbatim (product, release, collection, artifact, evidence-bundle,
// etc.) -- this package imports pkg/tea for those rather than redefining
// them, and holds only what's genuinely new.
//
// Deliberately separate from pkg/tea, which stays scoped to "the CycloneDX
// Transparency Exchange API OpenAPI spec" (its own doc comment) -- the
// official spec. The Publisher API is still this project's own draft
// proposal (design/publisher-service.md §11 open question #9); keeping it
// in its own package means pkg/tea's scope stays honest, and only this
// package needs to reconcile if/when a real official TEA Publisher API
// ever lands.
//
// This package is importable from outside this module (unlike internal/...)
// so it can be shared by opentea's own future /publisher/v1 implementation
// and every publisher-platform shape (the GUI service and the reference
// CLI client, pkg/teapublisherclient/cmd/teapublisherclient) without
// duplicating the wire format -- design/publisher-service.md §8/§14.1 v0.18.
package teapublisher

import (
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// ProductCreate is the request body for createProduct: same as tea.Product,
// minus the server-assigned UUID.
type ProductCreate struct {
	Name        string           `json:"name"`
	Identifiers []tea.Identifier `json:"identifiers"`
}

// ComponentCreate is the request body for createComponent: same as
// tea.Component, minus the server-assigned UUID.
type ComponentCreate struct {
	Name        string           `json:"name"`
	Identifiers []tea.Identifier `json:"identifiers"`
}

// ProductReleaseCreate is the request body for createProductRelease: same
// as tea.ProductRelease, minus the server-assigned UUID, the path-implied
// product, and the components list (linked separately via linkComponent).
type ProductReleaseCreate struct {
	Version     string           `json:"version"`
	CreatedDate time.Time        `json:"createdDate"`
	ReleaseDate *time.Time       `json:"releaseDate,omitempty"`
	PreRelease  *bool            `json:"preRelease,omitempty"`
	Identifiers []tea.Identifier `json:"identifiers,omitempty"`
}

// ComponentReleaseCreate is the request body for createComponentRelease
// (the OpenAPI schema is named `release-create`, matching pkg/tea's own
// tea.ComponentRelease naming for the spec's "release" schema): same as
// tea.ComponentRelease, minus the server-assigned UUID and the path-implied
// component.
type ComponentReleaseCreate struct {
	Version     string           `json:"version"`
	CreatedDate time.Time        `json:"createdDate"`
	ReleaseDate *time.Time       `json:"releaseDate,omitempty"`
	PreRelease  *bool            `json:"preRelease,omitempty"`
	Identifiers []tea.Identifier `json:"identifiers,omitempty"`
}

// ArtifactFormatCreate is one entry in ArtifactCreate.Formats: same as
// tea.ArtifactFormat, minus checksums (server-computed from the uploaded
// file, see uploadArtifactFile), url, and the evidence fields (attached by
// later operations -- none exist yet at artifact-creation time).
type ArtifactFormatCreate struct {
	MediaType   string `json:"mediaType"`
	Description string `json:"description,omitempty"`
}

// ArtifactCreate is the request body for createArtifact: same as
// tea.Artifact, minus the server-assigned UUID/version/createdDate.
// Metadata only -- file content (uploadArtifactFile) and evidence
// (prepareArtifactEvidence/submitArtifactEvidence) are attached by
// separate operations.
type ArtifactCreate struct {
	Name            string                 `json:"name,omitempty"`
	Type            string                 `json:"type"`
	DistributionIDs []string               `json:"distributionIds,omitempty"`
	Formats         []ArtifactFormatCreate `json:"formats"`
}

// CLEEventCreate is the request body for the createProductCLEEvent /
// createProductReleaseCLEEvent / component and component-release
// equivalents: same as tea.CLEEvent, minus the server-assigned,
// auto-incrementing ID.
type CLEEventCreate struct {
	Type                string                    `json:"type"`
	Effective           time.Time                 `json:"effective"`
	Published           time.Time                 `json:"published"`
	Version             string                    `json:"version,omitempty"`
	Versions            []tea.CLEVersionSpecifier `json:"versions,omitempty"`
	SupportID           string                    `json:"supportId,omitempty"`
	License             string                    `json:"license,omitempty"`
	SupersededByVersion string                    `json:"supersededByVersion,omitempty"`
	Identifiers         []tea.Identifier          `json:"identifiers,omitempty"`
	EventID             *int                      `json:"eventId,omitempty"`
	Reason              string                    `json:"reason,omitempty"`
	Description         string                    `json:"description,omitempty"`
	References          []string                  `json:"references,omitempty"`
}

// EvidenceSubmission is an **evidence package** (design/publisher-service.md
// §9.3 -- same concept as tea.EvidenceBundle, request-shaped): everything
// needed to verify one signature over one object. The single request shape
// used everywhere a signature is submitted in this API -- submitArtifactEvidence
// and commitCollectionDraft both take this directly.
type EvidenceSubmission struct {
	// ObjectDigestValue is hex SHA-256 of RFC 8785-canonicalized object
	// content, as returned by the matching prepare operation. The server
	// independently re-derives this from the object's actual current state
	// and rejects a mismatch -- this field is the caller's own record of
	// what it believed it was signing, checked against reality, never
	// trusted on its own.
	ObjectDigestValue string `json:"objectDigestValue"`
	ObjectMediaType   string `json:"objectMediaType,omitempty"`
	ObjectLocation    string `json:"objectLocation,omitempty"`
	// SignatureFormat: see the SignatureFormat* constants in enums.go.
	// Which format depends on the signing mode (design/publisher-service.md
	// §9.5): jws-detached fits an ephemeral, self-signed software key (the
	// only format opentea's Phase 1 actually verifies today). cms-detached
	// is the natural fit for a Web PKI certificate, HSM/PKCS#11-backed or
	// air-gapped signing. dsse-envelope/cose-sign1 remain reserved, no
	// concrete use case identified yet.
	SignatureFormat string `json:"signatureFormat"`
	// SignatureValue is base64-encoded signature over the digest bytes, in
	// SignatureFormat's encoding. Algorithm follows from the signing
	// certificate's own key type -- not fixed by this type.
	SignatureValue string `json:"signatureValue"`
	// CertificatePEM is the leaf (signing) certificate.
	CertificatePEM string `json:"certificatePem"`
	// CertificateChain is intermediate CA certificates, leaf-to-root order,
	// needed to validate CertificatePEM against a trust store (§9.5's Mode
	// 2) -- normally empty for opentea Phase 1's self-signed certificates,
	// which chain to nothing.
	CertificateChain []string `json:"certificateChain,omitempty"`
}

// PrepareArtifactEvidenceResponse is the response to prepareArtifactEvidence:
// symmetric with PrepareCommitResponse, for artifacts.
type PrepareArtifactEvidenceResponse struct {
	// DigestToSign is hex SHA-256 of RFC 8785-canonicalized artifact --
	// sign exactly these bytes.
	DigestToSign string       `json:"digestToSign"`
	Artifact     tea.Artifact `json:"artifact"`
}

// ArtifactVersionRef identifies one already-created, already-validated
// artifact version by reference (not a full tea.Artifact) -- used by both
// CollectionDraftArtifactList and CollectionDraft, since a collection
// draft's artifacts already exist and were already validated separately
// (see putCollectionDraft's summary for why embedding a second, possibly
// stale copy here would be a mistake).
type ArtifactVersionRef struct {
	UUID    string `json:"uuid"`
	Version int    `json:"version"`
}

// CollectionDraftArtifactList is the request body for putCollectionDraft:
// create-or-replace the draft's artifact list (design/publisher-service.md
// §7.7). Idempotent; callable repeatedly as artifacts arrive over however
// many sessions assembly takes.
type CollectionDraftArtifactList struct {
	// Actor is the identity of the staff member or CI/CD system making
	// this change, as asserted by the publisher software from its own
	// Layer A session (design/publisher-service.md §10.1) -- the target
	// server has no independent way to verify this beyond trusting the
	// publisher software's assertion. Recorded as the draft's DraftedBy
	// and checked against ApprovalDecision's own Actor for maker-checker
	// (§7.9). See TODO.md's "derive approval actor from authenticated
	// identity" entry -- a known, tracked limitation of this field as a
	// plain client-supplied string.
	Actor        string               `json:"actor"`
	Artifacts    []ArtifactVersionRef `json:"artifacts,omitempty"`
	UpdateReason *tea.UpdateReason    `json:"updateReason,omitempty"`
}

// CollectionDraftDiff is CollectionDraft.DiffAgainstCurrent: added/removed
// artifacts vs. the owner's current live collection, if one exists.
type CollectionDraftDiff struct {
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

// CollectionDraftApproval is CollectionDraft.Approval: absent/Status "none"
// until the first approveCollectionDraft or rejectCollectionDraft call
// (design/publisher-service.md §7.9). See the ApprovalStatus* constants in
// enums.go.
type CollectionDraftApproval struct {
	Status    string     `json:"status"`
	DecidedBy string     `json:"decidedBy,omitempty"`
	DecidedAt *time.Time `json:"decidedAt,omitempty"`
	// DecidedAtRevision is the CollectionDraft.Revision that was actually
	// reviewed -- compared against the current revision to detect
	// staleness (an edit after approval resets Status back to "none").
	DecidedAtRevision int `json:"decidedAtRevision,omitempty"`
	// ExpiresAt is set alongside DecidedAt on approval (§9.6) -- an old
	// approval shouldn't authorize a commit long after the fact.
	// prepareCollectionCommit checks this the same way it checks
	// DecidedAtRevision: expired is treated the same as no approval at
	// all. Not meaningful for a rejected decision.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Comment   string     `json:"comment,omitempty"`
}

// CollectionDraft is the response shape for putCollectionDraft/
// getCollectionDraft: the in-progress state of a collection before it's
// committed (design/publisher-service.md §7.7). No independent ID --
// OwnerUUID is always the eventual collection's own UUID too (a
// collection's UUID always matches its owning release's UUID), so there's
// nothing to allocate and at most one open draft per owner exists by
// construction.
type CollectionDraft struct {
	// OwnerType: see tea.CollectionBelongsTo* constants (pkg/tea/enums.go)
	// -- reused directly, this is the same enum as tea.Collection.BelongsTo.
	OwnerType string     `json:"ownerType"`
	OwnerUUID string     `json:"ownerUuid"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	// ExpiresAt is set on creation, refreshed on every putCollectionDraft
	// (§7.7, §9.6) -- an open draft nobody is actively working isn't kept
	// indefinitely. Distinct from LockExpiresAt: this is about a draft
	// nobody is touching at all, not a signature that's overdue.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// LockExpiresAt is present only while locked (§9.5) -- set by
	// prepareCollectionCommit, cleared by cancelPrepareCollectionCommit or
	// commitCollectionDraft succeeding. Past this time the lock is treated
	// as released automatically -- an abandoned signing ceremony doesn't
	// block the draft forever.
	LockExpiresAt      *time.Time           `json:"lockExpiresAt,omitempty"`
	Artifacts          []ArtifactVersionRef `json:"artifacts,omitempty"`
	UpdateReason       *tea.UpdateReason    `json:"updateReason,omitempty"`
	DiffAgainstCurrent *CollectionDraftDiff `json:"diffAgainstCurrent,omitempty"`
	// DraftedBy is the most recent putCollectionDraft's Actor (§7.9).
	DraftedBy string `json:"draftedBy,omitempty"`
	// Revision starts at 1, incremented on every putCollectionDraft. Ties
	// an approval decision to the exact content it was made against -- an
	// edit after approval invalidates it (Approval.Status resets to
	// "none"), rather than silently letting a stale approval cover
	// different content than what was actually reviewed.
	Revision int                      `json:"revision"`
	Approval *CollectionDraftApproval `json:"approval,omitempty"`
}

// ApprovalDecision is the request body for approveCollectionDraft/
// rejectCollectionDraft (design/publisher-service.md §7.9).
type ApprovalDecision struct {
	// Actor is the identity of the approver, asserted the same way
	// CollectionDraftArtifactList's own Actor is (§10.1). Must differ from
	// the draft's current DraftedBy -- maker-checker (§7.9); rejected with
	// 403 otherwise. See the same TODO.md caveat CollectionDraftArtifactList.Actor
	// carries.
	Actor   string `json:"actor"`
	Comment string `json:"comment,omitempty"`
}

// PrepareCommitResponse is the response to prepareCollectionCommit: the
// collection that would be created if commitCollectionDraft is called now,
// plus the digest to sign over it. Also the to-be-signed (TBS) package for
// offline/air-gapped signing (§9.5) -- save it as-is rather than assuming
// it's only ever consumed synchronously in the same call chain.
type PrepareCommitResponse struct {
	// DigestToSign is hex SHA-256 of RFC 8785-canonicalized
	// WouldBeCollection -- sign exactly these bytes.
	DigestToSign      string         `json:"digestToSign"`
	WouldBeCollection tea.Collection `json:"wouldBeCollection"`
}
