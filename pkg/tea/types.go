// Package tea holds the JSON-facing wire types matching the CycloneDX
// Transparency Exchange API OpenAPI spec (spec/openapi.yaml v0.4.0). This
// package is importable from outside this module (unlike internal/...) so
// it can be shared by the server, the reference client (pkg/teaclient), and
// any future publisher without duplicating the wire format.
package tea

import "time"

type Identifier struct {
	IDType  string `json:"idType"`
	IDValue string `json:"idValue"`
}

type Product struct {
	UUID        string       `json:"uuid"`
	Name        string       `json:"name"`
	Identifiers []Identifier `json:"identifiers"`
}

type ComponentRef struct {
	UUID    string  `json:"uuid"`
	Release *string `json:"release,omitempty"`
}

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

type Component struct {
	UUID        string       `json:"uuid"`
	Name        string       `json:"name"`
	Identifiers []Identifier `json:"identifiers"`
}

type Checksum struct {
	AlgType  string `json:"algType"`
	AlgValue string `json:"algValue"`
}

type ReleaseDistribution struct {
	DistributionID string       `json:"distributionId"`
	Description    string       `json:"description,omitempty"`
	Identifiers    []Identifier `json:"identifiers,omitempty"`
	URL            string       `json:"url,omitempty"`
	SignatureURL   string       `json:"signatureUrl,omitempty"`
	Checksums      []Checksum   `json:"checksums,omitempty"`
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

type ComponentReleaseWithCollection struct {
	Release          ComponentRelease `json:"release"`
	LatestCollection Collection       `json:"latestCollection"`
}

type UpdateReason struct {
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
}

type Collection struct {
	UUID         string        `json:"uuid"`
	Version      int           `json:"version"`
	Date         time.Time     `json:"date"`
	BelongsTo    string        `json:"belongsTo"`
	UpdateReason *UpdateReason `json:"updateReason,omitempty"`
	Artifacts    []Artifact    `json:"artifacts"`
}

type ArtifactFormat struct {
	MediaType    string     `json:"mediaType"`
	Description  string     `json:"description,omitempty"`
	URL          string     `json:"url,omitempty"`
	SignatureURL string     `json:"signatureUrl,omitempty"`
	Checksums    []Checksum `json:"checksums,omitempty"`
}

type Artifact struct {
	UUID            string           `json:"uuid"`
	Version         int              `json:"version"`
	Name            string           `json:"name,omitempty"`
	Type            string           `json:"type"`
	CreatedDate     *time.Time       `json:"createdDate,omitempty"`
	DistributionIDs []string         `json:"distributionIds,omitempty"`
	Formats         []ArtifactFormat `json:"formats"`
}

type ErrorResponse struct {
	Error string `json:"error"` // "OBJECT_UNKNOWN" | "OBJECT_NOT_SHAREABLE"
}

const (
	ErrorObjectUnknown      = "OBJECT_UNKNOWN"
	ErrorObjectNotShareable = "OBJECT_NOT_SHAREABLE"
)

type TeaServerInfo struct {
	RootURL  string   `json:"rootUrl"`
	Versions []string `json:"versions"`
	Priority *float64 `json:"priority,omitempty"`
}

type DiscoveryInfo struct {
	ProductReleaseUUID string          `json:"productReleaseUuid"`
	Servers            []TeaServerInfo `json:"servers"`
}

type CLEVersionSpecifier struct {
	Version string `json:"version,omitempty"`
	Range   string `json:"range,omitempty"`
}

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

type CLESupportDefinition struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	URL         string `json:"url,omitempty"`
}

type CLEDefinitions struct {
	Support []CLESupportDefinition `json:"support,omitempty"`
}

type CLE struct {
	Events      []CLEEvent      `json:"events"`
	Definitions *CLEDefinitions `json:"definitions,omitempty"`
}

type PaginationDetails struct {
	HasNext       bool   `json:"hasNext"`
	NextPageToken string `json:"nextPageToken"`
}

type PaginatedProducts struct {
	PaginationDetails
	Results []Product `json:"results"`
}

type PaginatedProductReleases struct {
	PaginationDetails
	Results []ProductRelease `json:"results"`
}

type PaginatedComponents struct {
	PaginationDetails
	Results []Component `json:"results"`
}

type PaginatedComponentReleases struct {
	PaginationDetails
	Results []ComponentRelease `json:"results"`
}

type PaginatedCollections struct {
	PaginationDetails
	Results []Collection `json:"results"`
}
