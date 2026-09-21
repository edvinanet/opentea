// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package tea holds the JSON-facing wire types matching the CycloneDX
// Transparency Exchange API OpenAPI spec (spec/openapi.yaml v0.4.0). This
// package is importable from outside this module (unlike internal/...) so
// it can be shared by the server, the reference client (pkg/teaclient), and
// any future publisher without duplicating the wire format.
package tea

import "time"

// Identifier is a single external identifier (e.g. a CPE, PURL, or TEI)
// attached to a product, release, component, or distribution.
type Identifier struct {
	IDType  string `json:"idType"`
	IDValue string `json:"idValue"`
}

// Product is a top-level product entity: the thing a vendor ships, tracked
// independently of any specific release.
type Product struct {
	UUID        string       `json:"uuid"`
	Name        string       `json:"name"`
	Identifiers []Identifier `json:"identifiers"`
}

// ComponentRef links a ProductRelease to a Component it depends on,
// optionally pinned to one specific ComponentRelease via Release.
type ComponentRef struct {
	UUID    string  `json:"uuid"`
	Release *string `json:"release,omitempty"`
}

// ProductRelease is one versioned release of a Product, along with the
// components it's built from (see Components).
type ProductRelease struct {
	UUID        string         `json:"uuid"`
	Product     *string        `json:"product,omitempty"`
	ProductName string         `json:"productName,omitempty"`
	Version     string         `json:"version"`
	CreatedDate time.Time      `json:"createdDate"`
	ReleaseDate *time.Time     `json:"releaseDate,omitempty"`
	PreRelease  *bool          `json:"preRelease,omitempty"`
	Identifiers []Identifier   `json:"identifiers,omitempty"`
	Components  []ComponentRef `json:"components"`
}

// Component is a reusable piece of software (e.g. a library or package)
// that can be shared across many products' releases, tracked independently
// of any specific release.
type Component struct {
	UUID        string       `json:"uuid"`
	Name        string       `json:"name"`
	Identifiers []Identifier `json:"identifiers"`
}

// Checksum is a single algorithm/value pair used to verify a downloaded
// file (a ReleaseDistribution or ArtifactFormat).
type Checksum struct {
	AlgType  string `json:"algType"`
	AlgValue string `json:"algValue"`
}

// ReleaseDistribution is one downloadable artifact (a binary, installer,
// etc.) belonging to a ComponentRelease -- distinct from an Artifact, which
// is a security-related document (SBOM, VEX, ...) rather than the software
// itself.
type ReleaseDistribution struct {
	DistributionID string       `json:"distributionId"`
	Description    string       `json:"description,omitempty"`
	Identifiers    []Identifier `json:"identifiers,omitempty"`
	URL            string       `json:"url,omitempty"`
	// SignatureURL: see ArtifactFormat.SignatureURL's doc comment -- a
	// legacy/simple pointer, not part of the TEA Trust Architecture
	// overlay. Distributions are software artifacts, not TEA Artifacts, so
	// they aren't currently a trust-architecture evidence-bundle owner type
	// (see 0006_trust.sql); this field is unaffected either way.
	SignatureURL string     `json:"signatureUrl,omitempty"`
	Checksums    []Checksum `json:"checksums,omitempty"`
}

// ComponentRelease corresponds to the spec's "release" schema (a TEA Component Release).
type ComponentRelease struct {
	UUID          string                `json:"uuid"`
	Component     string                `json:"component,omitempty"`
	ComponentName string                `json:"componentName,omitempty"`
	Version       string                `json:"version"`
	CreatedDate   time.Time             `json:"createdDate"`
	ReleaseDate   *time.Time            `json:"releaseDate,omitempty"`
	PreRelease    *bool                 `json:"preRelease,omitempty"`
	Identifiers   []Identifier          `json:"identifiers,omitempty"`
	Distributions []ReleaseDistribution `json:"distributions,omitempty"`
}

// ComponentReleaseWithCollection bundles a ComponentRelease with its most
// recent Collection, matching the shape GET .../componentRelease/{uuid}
// returns in one call.
type ComponentReleaseWithCollection struct {
	Release          ComponentRelease `json:"release"`
	LatestCollection Collection       `json:"latestCollection"`
}

// UpdateReason explains why a new Collection version was published (e.g.
// an artifact was added or a VEX status changed).
type UpdateReason struct {
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
}

// Collection is one version of the set of Artifacts published for a
// product release or component release (BelongsTo distinguishes which).
// Collections are append-only: a new version is published whenever the
// artifact set changes, never overwritten in place.
type Collection struct {
	UUID         string        `json:"uuid"`
	Version      int           `json:"version"`
	Date         time.Time     `json:"date"`
	BelongsTo    string        `json:"belongsTo"`
	UpdateReason *UpdateReason `json:"updateReason,omitempty"`
	Artifacts    []Artifact    `json:"artifacts"`

	// EvidenceBundle/EvidenceBundleRef are the TEA Trust Architecture
	// overlay's cryptographic evidence of this collection's origin and
	// integrity (see internal/trust and pkg/tea/trust.go). Mutually
	// exclusive; both nil means no trust-architecture evidence exists for
	// this collection, which is the common case for a deployment not
	// claiming that profile. Per the spec, collection evidence MUST NOT be
	// reused across versions of the same collection uuid (enforced by the
	// Phase 4 publish/commit workflow, not yet built).
	EvidenceBundle    *EvidenceBundle    `json:"evidenceBundle,omitempty"`
	EvidenceBundleRef *EvidenceBundleRef `json:"evidenceBundleRef,omitempty"`
}

// ArtifactFormat is one downloadable representation of an Artifact (e.g.
// the same SBOM offered as both CycloneDX XML and JSON).
type ArtifactFormat struct {
	MediaType   string `json:"mediaType"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	// SignatureURL is a legacy/simple detached-signature pointer for
	// deployments that don't claim the TEA Trust Architecture profile --
	// it is not verified or otherwise interpreted by this server. Retained
	// for backward wire compatibility rather than replaced outright.
	// EvidenceBundle/EvidenceBundleRef below are what make a deployment
	// trust-architecture-conformant.
	SignatureURL string     `json:"signatureUrl,omitempty"`
	Checksums    []Checksum `json:"checksums,omitempty"`

	// EvidenceBundle/EvidenceBundleRef: see Collection's fields of the same
	// name. Per the spec, artifact evidence MAY be reused across a
	// document's versions (unlike collection evidence).
	EvidenceBundle    *EvidenceBundle    `json:"evidenceBundle,omitempty"`
	EvidenceBundleRef *EvidenceBundleRef `json:"evidenceBundleRef,omitempty"`
}

// Artifact is a security-related document (SBOM, VEX, attestation, license,
// etc. -- see the ArtifactType* constants in enums.go) attached to a
// Collection. Distinct from a ReleaseDistribution, which is the software
// itself rather than a document about it.
type Artifact struct {
	UUID            string           `json:"uuid"`
	Version         int              `json:"version"`
	Name            string           `json:"name,omitempty"`
	Type            string           `json:"type"`
	CreatedDate     *time.Time       `json:"createdDate,omitempty"`
	DistributionIDs []string         `json:"distributionIds,omitempty"`
	Formats         []ArtifactFormat `json:"formats"`
}

// ErrorResponse is the spec's standard error body (TEA 1.0,
// spec/openapi.yaml's error-response schema): "error is one of the values
// of unknown-error-type, and message is the only other property allowed."
// A body is optional on any 4xx from a resource endpoint ("Clients shall
// not require a body") -- when a server does send one, it must be exactly
// this shape (additionalProperties: false upstream), so Error/Message are
// the only fields, ever.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// unknown-error-type (TEA 1.0, spec/openapi.yaml) -- classification of a
// TEA error response.
const (
	// ErrorObjectUnknown means the requested object doesn't exist on this server.
	ErrorObjectUnknown = "OBJECT_UNKNOWN"
	// ErrorNotImplemented means the server understands the request but
	// doesn't support the requested operation.
	ErrorNotImplemented = "NOT_IMPLEMENTED"
	// ErrorNoAcceptableFormat means none of the artifact's available
	// formats matches what the caller's Accept header/mediaType
	// selection requested.
	ErrorNoAcceptableFormat = "NO_ACCEPTABLE_FORMAT"
	// ErrorSignatureNotFound means the caller asked for a signature that
	// doesn't exist for the requested artifact/format.
	ErrorSignatureNotFound = "SIGNATURE_NOT_FOUND"
	// ErrorInvalidRequest means the request is malformed in a way none
	// of the more specific error types name.
	ErrorInvalidRequest = "INVALID_REQUEST"
	// ErrorInvalidPageToken means a supplied pageToken is malformed or
	// doesn't match the request's current sortField/sortOrder.
	ErrorInvalidPageToken = "INVALID_PAGE_TOKEN"
)

// ServerInfo identifies one server that hosts a given product release,
// as returned by discovery.
type ServerInfo struct {
	RootURL  string   `json:"rootUrl"`
	Versions []string `json:"versions"`
	Priority *float64 `json:"priority,omitempty"`
}

// DiscoveryInfo is one result of GET /tea/v1/discovery: which server(s)
// host the product release identified by a given TEI.
type DiscoveryInfo struct {
	ProductReleaseUUID string       `json:"productReleaseUuid"`
	Servers            []ServerInfo `json:"servers"`
}

// WellKnownDocument is the JSON body served at
// https://<tei-authority>/.well-known/tea, the entry point of TEA's
// TEI-authority bootstrap discovery flow (see
// pkg/teaclient.BootstrapDiscover): the first step in going from a bare
// TEI to a server that can answer GET /discovery for it. Matches
// CycloneDX/transparency-exchange-api's discovery/tea-well-known.schema.json.
type WellKnownDocument struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Endpoints     []WellKnownEndpoint `json:"endpoints"`
}

// WellKnownEndpoint is one candidate TEA server listed in a
// WellKnownDocument -- distinct from ServerInfo (which describes a server
// already known, via /tea/v1/discovery, to host a specific product
// release): a WellKnownEndpoint is a candidate the client hasn't queried
// yet.
type WellKnownEndpoint struct {
	URL      string   `json:"url"`
	Versions []string `json:"versions"`
	// Priority: 0-1, higher tried first. A pointer (matching ServerInfo's
	// own Priority field above) because the schema's default of 1 when
	// absent is different from an explicit priority of 0 (lowest) -- a
	// plain float64 can't distinguish "not present" from "present and
	// zero" on decode; omitempty only affects encoding, not decoding.
	Priority *float64 `json:"priority,omitempty"`
}

// CLEVersionSpecifier names either a single version or a version range a
// CLEEvent applies to.
type CLEVersionSpecifier struct {
	Version string `json:"version,omitempty"`
	Range   string `json:"range,omitempty"`
}

// CLEEvent is one lifecycle event (ECMA-428 Common Lifecycle Enumeration)
// for a product, release, component, or component release -- e.g. a
// release, end-of-support, or end-of-life date. See the CLEEventType*
// constants in enums.go for the possible Type values.
type CLEEvent struct {
	ID                  int                   `json:"id"`
	Type                string                `json:"type"`
	Effective           time.Time             `json:"effective"`
	Published           time.Time             `json:"published"`
	Version             string                `json:"version,omitempty"`
	Versions            []CLEVersionSpecifier `json:"versions,omitempty"`
	SupportID           string                `json:"supportId,omitempty"`
	License             string                `json:"license,omitempty"`
	SupersededByVersion string                `json:"supersededByVersion,omitempty"`
	Identifiers         []Identifier          `json:"identifiers,omitempty"`
	EventID             *int                  `json:"eventId,omitempty"`
	Reason              string                `json:"reason,omitempty"`
	Description         string                `json:"description,omitempty"`
	References          []string              `json:"references,omitempty"`
}

// CLESupportDefinition names a support policy (e.g. "LTS") that CLEEvents
// can reference by ID via their SupportID field.
type CLESupportDefinition struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	URL         string `json:"url,omitempty"`
}

// CLEDefinitions holds the support policy definitions referenced by a CLE's events.
type CLEDefinitions struct {
	Support []CLESupportDefinition `json:"support,omitempty"`
}

// CLE is the full lifecycle history for one owner (product, release,
// component, or component release): every CLEEvent recorded for it, plus
// any support policy definitions those events reference.
type CLE struct {
	Events      []CLEEvent      `json:"events"`
	Definitions *CLEDefinitions `json:"definitions,omitempty"`
}

// PaginationDetails is embedded in every paginated list response, carrying
// the cursor needed to fetch the next page.
type PaginationDetails struct {
	HasNext       bool   `json:"hasNext"`
	NextPageToken string `json:"nextPageToken"`
}

// PaginatedProducts is the response shape for GET /tea/v1/products.
type PaginatedProducts struct {
	PaginationDetails
	Results []Product `json:"results"`
}

// PaginatedProductReleases is the response shape for GET /tea/v1/productReleases.
type PaginatedProductReleases struct {
	PaginationDetails
	Results []ProductRelease `json:"results"`
}

// PaginatedComponents is the response shape for GET /tea/v1/components.
type PaginatedComponents struct {
	PaginationDetails
	Results []Component `json:"results"`
}

// PaginatedComponentReleases is the response shape for GET /tea/v1/componentReleases.
type PaginatedComponentReleases struct {
	PaginationDetails
	Results []ComponentRelease `json:"results"`
}

// PaginatedCollections is the response shape for a collection-list endpoint
// (GET .../productRelease/{uuid}/collections or .../componentRelease/{uuid}/collections).
type PaginatedCollections struct {
	PaginationDetails
	Results []Collection `json:"results"`
}
