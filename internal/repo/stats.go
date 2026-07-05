package repo

import (
	"context"

	"github.com/oej/opentea/internal/model"
)

// GetStats returns overview counts for the admin dashboard. Collections and
// artifacts have composite (uuid, version) identity, so COUNT(DISTINCT uuid)
// counts distinct collections/artifacts rather than every revision.
func (r *Repo) GetStats(ctx context.Context) (model.Stats, error) {
	var s model.Stats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM product),
			(SELECT COUNT(*) FROM product_release),
			(SELECT COUNT(*) FROM component),
			(SELECT COUNT(*) FROM component_release),
			(SELECT COUNT(DISTINCT uuid) FROM collection),
			(SELECT COUNT(DISTINCT uuid) FROM artifact)
	`).Scan(&s.Products, &s.ProductReleases, &s.Components, &s.ComponentReleases, &s.Collections, &s.Artifacts)
	return s, err
}
