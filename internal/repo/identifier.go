// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"slices"

	"github.com/oej/opentea/pkg/tea"
)

// ErrComplianceDocumentWrongOwner is returned when an identifier with
// idType COMPLIANCE_DOCUMENT is attached to anything other than a
// Component or ComponentRelease -- upstream TEA 1.0's identifier-type
// description: "It shall not be used on products, product releases,
// distributions, or CLE events."
var ErrComplianceDocumentWrongOwner = errors.New("repo: a COMPLIANCE_DOCUMENT identifier may only be attached to a component or component release")

// ErrInvalidComplianceDocumentType is returned when an identifier with
// idType COMPLIANCE_DOCUMENT has an idValue outside the spec's
// compliance-document-type enum (TEA 1.0, spec/openapi.yaml): "When
// idType is COMPLIANCE_DOCUMENT, the idValue shall be one of these
// values."
var ErrInvalidComplianceDocumentType = errors.New("repo: idValue must be a valid compliance-document-type when idType is COMPLIANCE_DOCUMENT")

// validComplianceDocumentTypes mirrors internal/publisher/artifact.go's
// own local validArtifactTypes convention -- validation lives with the
// one place it's enforced (this package), not exported from pkg/tea,
// which is a source of valid values, not a validator.
var validComplianceDocumentTypes = []string{
	tea.ComplianceDocumentTypeSOC2TypeI, tea.ComplianceDocumentTypeSOC2TypeII, tea.ComplianceDocumentTypeSOC3,
	tea.ComplianceDocumentTypeISO27001, tea.ComplianceDocumentTypeISO27017, tea.ComplianceDocumentTypeISO27018,
	tea.ComplianceDocumentTypeISO27701, tea.ComplianceDocumentTypeISO42001, tea.ComplianceDocumentTypePCIDSS,
	tea.ComplianceDocumentTypeHIPAA, tea.ComplianceDocumentTypeFedRAMP, tea.ComplianceDocumentTypeGDPR,
	tea.ComplianceDocumentTypeCSAStar, tea.ComplianceDocumentTypeNIST80053, tea.ComplianceDocumentTypeNIST800171,
	tea.ComplianceDocumentTypeCMMC, tea.ComplianceDocumentTypeHITRUST, tea.ComplianceDocumentTypeTISAX,
	tea.ComplianceDocumentTypeCyberEssentials, tea.ComplianceDocumentTypeCyberEssentialsPlus,
	tea.ComplianceDocumentTypeEUDeclarationOfConformity,
}

// validateComplianceDocumentIdentifier enforces both TEA 1.0 rules for a
// COMPLIANCE_DOCUMENT identifier; a no-op for any other idType. Shared by
// insertIdentifiers (below) and insertCLEEventIdentifiers (cle.go) -- the
// two owner-type rules differ (component/component-release only, vs.
// unconditionally forbidden on a CLE event), so callers pass whether their
// ownerType is allowed at all.
func validateComplianceDocumentIdentifier(id tea.Identifier, ownerTypeAllowed bool) error {
	if id.IDType != tea.IdentifierTypeComplianceDocument {
		return nil
	}
	if !ownerTypeAllowed {
		return ErrComplianceDocumentWrongOwner
	}
	if !slices.Contains(validComplianceDocumentTypes, id.IDValue) {
		return ErrInvalidComplianceDocumentType
	}
	return nil
}

func insertIdentifiers(ctx context.Context, q dbtx, ownerType, ownerUUID string, ids []tea.Identifier) error {
	for _, id := range ids {
		if err := validateComplianceDocumentIdentifier(id, ownerType == OwnerComponent || ownerType == OwnerComponentRelease); err != nil {
			return err
		}
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
