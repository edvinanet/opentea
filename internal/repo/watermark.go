package repo

import (
	"context"
	"database/sql"
	"errors"
)

// Resource families backing list-endpoint ETags (GET /products,
// /componentReleases, etc.) -- one global watermark per family, bumped on
// any write that could change that family's list membership or listed
// content. Global rather than tenant-scoped since this project has no
// tenant concept yet -- see TODO.md's "Multitenant" entry.
//
// Bump points are deliberately coarse: e.g. WatermarkComponentReleases
// bumps on a distribution-only change even though not every componentRelease
// list variant embeds distributions. Over-invalidating a watermark is
// always safe (one extra cache miss for a list consumer, never staleness),
// so precision here isn't worth the added bookkeeping.
const (
	WatermarkProducts          = "products"
	WatermarkProductReleases   = "productReleases"
	WatermarkComponents        = "components"
	WatermarkComponentReleases = "componentReleases"
	WatermarkCollections       = "collections"
)

// GetWatermark fetches the current watermark for family -- for list-ETag
// construction. Returns 0 (not an error) if family has never been bumped,
// matching bumpWatermarkTx's upsert-from-nothing behavior.
func (r *Repo) GetWatermark(ctx context.Context, family string) (int64, error) {
	return getWatermarkTx(ctx, r.conn(), family)
}

func getWatermarkTx(ctx context.Context, q dbtx, family string) (int64, error) {
	var watermark int64
	err := q.QueryRowContext(ctx, `SELECT watermark FROM dataset_watermark WHERE resource_family = ?`, family).Scan(&watermark)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return watermark, nil
}

// bumpWatermarkTx increments family's watermark, creating its row at 1 on
// the first ever bump.
func bumpWatermarkTx(ctx context.Context, q dbtx, family string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO dataset_watermark (resource_family, watermark) VALUES (?, 1)
		 ON CONFLICT (resource_family) DO UPDATE SET watermark = watermark + 1`,
		family,
	)
	return err
}
