package repo

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

func insertChecksums(ctx context.Context, q dbtx, ownerType, ownerID string, checksums []tea.Checksum) error {
	for _, c := range checksums {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO checksum (owner_type, owner_id, alg_type, alg_value) VALUES (?, ?, ?, ?)`,
			ownerType, ownerID, c.AlgType, c.AlgValue,
		); err != nil {
			return err
		}
	}
	return nil
}

func listChecksums(ctx context.Context, q dbtx, ownerType, ownerID string) ([]tea.Checksum, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT alg_type, alg_value FROM checksum WHERE owner_type = ? AND owner_id = ? ORDER BY id`,
		ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []tea.Checksum
	for rows.Next() {
		var c tea.Checksum
		if err := rows.Scan(&c.AlgType, &c.AlgValue); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
