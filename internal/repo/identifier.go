package repo

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

func insertIdentifiers(ctx context.Context, q dbtx, ownerType, ownerUUID string, ids []tea.Identifier) error {
	for _, id := range ids {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO identifier (owner_type, owner_uuid, id_type, id_value) VALUES (?, ?, ?, ?)`,
			ownerType, ownerUUID, id.IDType, id.IDValue,
		); err != nil {
			return err
		}
	}
	return nil
}

func listIdentifiers(ctx context.Context, q dbtx, ownerType, ownerUUID string) ([]tea.Identifier, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id_type, id_value FROM identifier WHERE owner_type = ? AND owner_uuid = ? ORDER BY id`,
		ownerType, ownerUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []tea.Identifier{}
	for rows.Next() {
		var id tea.Identifier
		if err := rows.Scan(&id.IDType, &id.IDValue); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// appendIdentifierFilter appends `AND EXISTS (...)` fragments to query for
// idType/idValue filtering of rows of ownerType, matched against uuidExpr
// (a SQL expression for the candidate row's owner uuid, e.g. "product.uuid").
// idType/idValue must already be validated against the identifier-type enum
// upstream (see httpx.IDFilter) -- this only ever binds them as parameters,
// never interpolates them into the query text.
func appendIdentifierFilter(query *string, args *[]any, ownerType, uuidExpr, idType, idValue string) {
	if idType != "" {
		*query += ` AND EXISTS (SELECT 1 FROM identifier i WHERE i.owner_type = ? AND i.owner_uuid = ` + uuidExpr + ` AND i.id_type = ?)`
		*args = append(*args, ownerType, idType)
	}
	if idValue != "" {
		*query += ` AND EXISTS (SELECT 1 FROM identifier i WHERE i.owner_type = ? AND i.owner_uuid = ` + uuidExpr + ` AND i.id_value = ?)`
		*args = append(*args, ownerType, idValue)
	}
}
