package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/pkg/tea"
)

// CreateDistribution creates a new distribution for componentReleaseUUID
// with a fresh generated ID. Returns ErrNotFound if componentReleaseUUID
// doesn't exist.
func (r *Repo) CreateDistribution(ctx context.Context, componentReleaseUUID, description string) (tea.ReleaseDistribution, error) {
	// Ensure the parent exists so we fail with ErrNotFound rather than a raw FK error.
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM component_release WHERE uuid = ?`, componentReleaseUUID).Scan(&exists); err != nil {
		return tea.ReleaseDistribution{}, err
	}
	if exists == 0 {
		return tea.ReleaseDistribution{}, ErrNotFound
	}

	id := idgen.New()
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO release_distribution (distribution_id, component_release_uuid, description) VALUES (?, ?, ?)`,
		id, componentReleaseUUID, description,
	); err != nil {
		return tea.ReleaseDistribution{}, err
	}
	return r.GetDistribution(ctx, id)
}

// ImportDistributionInput carries everything a bundle already knows about a
// distribution -- unlike the normal create-then-upload flow, url/checksums
// are set directly at insert time since the bundle's files/ directory
// already has the bytes.
type ImportDistributionInput struct {
	DistributionID       string
	ComponentReleaseUUID string
	Description          string
	Identifiers          []tea.Identifier
	URL                  string
	SignatureURL         string
	Checksums            []tea.Checksum
}

// ImportDistribution creates the distribution with an explicit
// (caller-supplied) DistributionID if it doesn't already exist -- see
// ImportProduct for the identity/idempotency rationale.
func (r *Repo) ImportDistribution(ctx context.Context, in ImportDistributionInput) (created bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM release_distribution WHERE distribution_id = ?`, in.DistributionID).Scan(&exists); err == nil {
		return false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO release_distribution (distribution_id, component_release_uuid, description, url, signature_url) VALUES (?, ?, ?, ?, ?)`,
		in.DistributionID, in.ComponentReleaseUUID, in.Description, nullIfEmpty(in.URL), nullIfEmpty(in.SignatureURL),
	); err != nil {
		return false, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerDistribution, in.DistributionID, in.Identifiers); err != nil {
		return false, err
	}
	if err := insertChecksums(ctx, tx, OwnerDistribution, in.DistributionID, in.Checksums); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// GetDistribution fetches a distribution by ID. Returns ErrNotFound if id doesn't exist.
func (r *Repo) GetDistribution(ctx context.Context, id string) (tea.ReleaseDistribution, error) {
	var description, url, sigURL sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT description, url, signature_url FROM release_distribution WHERE distribution_id = ?`, id,
	).Scan(&description, &url, &sigURL)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.ReleaseDistribution{}, ErrNotFound
	}
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}

	ids, err := listIdentifiers(ctx, r.db, OwnerDistribution, id)
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}
	checksums, err := listChecksums(ctx, r.db, OwnerDistribution, id)
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}

	return tea.ReleaseDistribution{
		DistributionID: id,
		Description:    description.String,
		Identifiers:    ids,
		URL:            url.String,
		SignatureURL:   sigURL.String,
		Checksums:      checksums,
	}, nil
}

// SetDistributionFile records an uploaded file for a distribution: sets its
// download URL and adds a SHA-256 checksum row.
func (r *Repo) SetDistributionFile(ctx context.Context, id, url, sha256Hex string) (tea.ReleaseDistribution, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `UPDATE release_distribution SET url = ? WHERE distribution_id = ?`, url, id)
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return tea.ReleaseDistribution{}, err
	} else if n == 0 {
		return tea.ReleaseDistribution{}, ErrNotFound
	}
	if err := insertChecksums(ctx, tx, OwnerDistribution, id, []tea.Checksum{{AlgType: "SHA-256", AlgValue: sha256Hex}}); err != nil {
		return tea.ReleaseDistribution{}, err
	}
	if err := tx.Commit(); err != nil {
		return tea.ReleaseDistribution{}, err
	}
	return r.GetDistribution(ctx, id)
}

func listDistributionsForRelease(ctx context.Context, q dbtx, componentReleaseUUID string) ([]tea.ReleaseDistribution, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT distribution_id FROM release_distribution WHERE component_release_uuid = ? ORDER BY rowid`, componentReleaseUUID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := []tea.ReleaseDistribution{}
	for _, id := range ids {
		d, err := getDistributionTx(ctx, q, id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func getDistributionTx(ctx context.Context, q dbtx, id string) (tea.ReleaseDistribution, error) {
	var description, url, sigURL sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT description, url, signature_url FROM release_distribution WHERE distribution_id = ?`, id,
	).Scan(&description, &url, &sigURL)
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}
	ids, err := listIdentifiers(ctx, q, OwnerDistribution, id)
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}
	checksums, err := listChecksums(ctx, q, OwnerDistribution, id)
	if err != nil {
		return tea.ReleaseDistribution{}, err
	}
	return tea.ReleaseDistribution{
		DistributionID: id,
		Description:    description.String,
		Identifiers:    ids,
		URL:            url.String,
		SignatureURL:   sigURL.String,
		Checksums:      checksums,
	}, nil
}
