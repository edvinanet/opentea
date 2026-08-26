package webadmin

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

// evidenceBadge summarizes whether a trust-architecture evidence bundle is
// attached to one artifact or collection, for template rendering (see
// templates/layout.html's "evidenceBadge" partial). Deliberately not read
// from tea.Collection/tea.Artifact's own EvidenceBundle fields -- those stay
// unpopulated by internal/repo's Get/List calls in this phase (which also
// back /tea/v1), so evidence presence is looked up separately here instead
// of wiring it into that shared code path ahead of the authz decision that
// gates exposing it over /tea/v1.
type evidenceBadge struct {
	Present bool
	Status  string // "draft" or "complete"; only meaningful when Present
}

// evidenceBadgeFor looks up the evidence bundle attached to
// (ownerType, ownerUUID, ownerVersion), if any. A lookup error (anything
// other than "no bundle") is logged and treated as absent rather than
// failing the whole page render -- there's no per-row error slot to surface
// it in, and a transient DB error on one row's badge shouldn't take down an
// otherwise-working release detail page.
func (s *Server) evidenceBadgeFor(ctx context.Context, ownerType, ownerUUID string, ownerVersion int) evidenceBadge {
	bundle, err := s.repo.GetEvidenceBundleForOwner(ctx, ownerType, ownerUUID, ownerVersion)
	if errors.Is(err, repo.ErrNotFound) {
		return evidenceBadge{}
	}
	if err != nil {
		slog.Error("evidence bundle lookup failed", "ownerType", ownerType, "ownerUUID", ownerUUID, "ownerVersion", ownerVersion, "error", err)
		return evidenceBadge{}
	}
	return evidenceBadge{Present: true, Status: bundle.Status}
}

// artifactView is tea.Artifact plus its resolved evidence badge, for the
// productRelease/componentRelease detail page templates.
type artifactView struct {
	UUID     string
	Type     string
	Name     string
	Formats  []tea.ArtifactFormat
	Evidence evidenceBadge
}

// collectionView is tea.Collection plus its resolved evidence badge and its
// artifacts as artifactViews.
type collectionView struct {
	UUID         string
	Version      int
	Date         time.Time
	UpdateReason *tea.UpdateReason
	Artifacts    []artifactView
	Evidence     evidenceBadge
}

// buildCollectionViews resolves each collection's and each of its
// artifacts' evidence-bundle presence. One extra repo call per
// collection/artifact (N+1) -- accepted, matching the same tradeoff
// productreleases.go's per-linked-component GetComponent call already makes
// on this same page: admin-GUI traffic is low-volume, and correctness/
// simplicity wins over a batched query here.
func (s *Server) buildCollectionViews(ctx context.Context, collections []tea.Collection) []collectionView {
	out := make([]collectionView, 0, len(collections))
	for _, c := range collections {
		artifacts := make([]artifactView, 0, len(c.Artifacts))
		for _, a := range c.Artifacts {
			artifacts = append(artifacts, artifactView{
				UUID: a.UUID, Type: a.Type, Name: a.Name, Formats: a.Formats,
				Evidence: s.evidenceBadgeFor(ctx, "ARTIFACT", a.UUID, a.Version),
			})
		}
		out = append(out, collectionView{
			UUID: c.UUID, Version: c.Version, Date: c.Date, UpdateReason: c.UpdateReason,
			Artifacts: artifacts,
			Evidence:  s.evidenceBadgeFor(ctx, "COLLECTION", c.UUID, c.Version),
		})
	}
	return out
}
