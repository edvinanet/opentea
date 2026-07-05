// Package bundle implements the product import/export bundle format: a zip
// file (one per product) containing a JSON manifest plus a content-addressed
// files/ directory of the referenced blobs. Used for backup, ownership
// transfer (e.g. one company selling a product to another), and migrating
// between hosted TEA providers -- always admin-authenticated, and entirely
// separate from the /tea/v1 (consumer) and any future publisher API. See
// docs/bundle-format.md for the full specification.
package bundle

import (
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// FormatVersion identifies the manifest shape itself (not the TEA spec
// version) -- bump this if the bundle format's own structure changes in a
// way that breaks older importers.
const FormatVersion = "1.0"

// Manifest is the JSON document at the root of a bundle zip (manifest.json).
type Manifest struct {
	FormatVersion     string                  `json:"formatVersion"`
	ExportedAt        time.Time               `json:"exportedAt"`
	Product           ProductEntry            `json:"product"`
	ProductReleases   []ProductReleaseEntry   `json:"productReleases"`
	Components        []ComponentEntry        `json:"components"`
	ComponentReleases []ComponentReleaseEntry `json:"componentReleases"`
	// Collections already carry their full Artifacts inline (matching the
	// spec's own wire shape for a fetched collection), so there's no
	// separate top-level "artifacts" list -- importing walks
	// Collections[].Artifacts, and the identity-based dedup rule means an
	// artifact referenced by more than one collection is handled for free.
	Collections []tea.Collection `json:"collections"`
}

// CLE data isn't part of the spec's Product/ProductRelease/Component/
// ComponentRelease wire shapes (it's fetched via a separate endpoint per the
// spec), so the *Entry types add it as an additive field rather than
// changing the reused pkg/tea types themselves.

type ProductEntry struct {
	tea.Product
	CLE *tea.CLE `json:"cle,omitempty"`
}

type ProductReleaseEntry struct {
	tea.ProductRelease
	CLE *tea.CLE `json:"cle,omitempty"`
}

type ComponentEntry struct {
	tea.Component
	CLE *tea.CLE `json:"cle,omitempty"`
}

type ComponentReleaseEntry struct {
	tea.ComponentRelease          // already embeds Distributions
	CLE                  *tea.CLE `json:"cle,omitempty"`
}
