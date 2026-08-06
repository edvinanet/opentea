package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// CLEEventInput carries the fields needed to record a new CLE event.
type CLEEventInput struct {
	Type                string
	Effective           time.Time
	Published           time.Time
	Version             string
	Versions            []tea.CLEVersionSpecifier
	SupportID           string
	License             string
	SupersededByVersion string
	Identifiers         []tea.Identifier
	EventID             *int
	Reason              string
	Description         string
	References          []string
}

// CreateCLEEvent records a new lifecycle event for (ownerType, ownerUUID),
// assigning it the next sequential id for that owner.
func (r *Repo) CreateCLEEvent(ctx context.Context, ownerType, ownerUUID string, in CLEEventInput) (tea.CLEEvent, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.CLEEvent, error) {
		var maxID sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(id) FROM cle_event WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID).Scan(&maxID); err != nil {
			return tea.CLEEvent{}, err
		}
		id := 1
		if maxID.Valid {
			id = int(maxID.Int64) + 1
		}

		_, err := tx.ExecContext(ctx,
			`INSERT INTO cle_event (owner_type, owner_uuid, id, type, effective, published, version, support_id, license, superseded_by_version, event_id_ref, reason, description)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ownerType, ownerUUID, id, in.Type, formatTime(in.Effective), formatTime(in.Published),
			nullIfEmpty(in.Version), nullIfEmpty(in.SupportID), nullIfEmpty(in.License), nullIfEmpty(in.SupersededByVersion),
			in.EventID, nullIfEmpty(in.Reason), nullIfEmpty(in.Description),
		)
		if err != nil {
			return tea.CLEEvent{}, err
		}

		for _, v := range in.Versions {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO cle_event_version (owner_type, owner_uuid, event_id, version, version_range) VALUES (?, ?, ?, ?, ?)`,
				ownerType, ownerUUID, id, nullIfEmpty(v.Version), nullIfEmpty(v.Range),
			); err != nil {
				return tea.CLEEvent{}, err
			}
		}
		for _, ident := range in.Identifiers {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO cle_event_identifier (owner_type, owner_uuid, event_id, id_type, id_value) VALUES (?, ?, ?, ?, ?)`,
				ownerType, ownerUUID, id, ident.IDType, ident.IDValue,
			); err != nil {
				return tea.CLEEvent{}, err
			}
		}
		for _, ref := range in.References {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO cle_event_reference (owner_type, owner_uuid, event_id, uri) VALUES (?, ?, ?, ?)`,
				ownerType, ownerUUID, id, ref,
			); err != nil {
				return tea.CLEEvent{}, err
			}
		}

		if err := bumpCLERevisionTx(ctx, tx, ownerType, ownerUUID); err != nil {
			return tea.CLEEvent{}, err
		}

		return getCLEEventTx(ctx, tx, ownerType, ownerUUID, id)
	})
}

// ImportCLEEvent creates the event at its exact source id if it doesn't
// already exist, or leaves an existing one untouched if its content
// matches -- preserving the literal id (rather than assigning the next
// sequential one, as CreateCLEEvent does) is required so eventId
// cross-references within the same bundle keep resolving correctly. See
// ImportProduct for the identity/idempotency rationale. Returns
// ErrImportIdentityConflict if (ownerType, ownerUUID, id) already exists
// with different content.
func (r *Repo) ImportCLEEvent(ctx context.Context, ownerType, ownerUUID string, e tea.CLEEvent) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		existing, err := getCLEEventTx(ctx, tx, ownerType, ownerUUID, e.ID)
		if err == nil {
			if cleEventConflicts(existing, e) {
				return false, fmt.Errorf("%w: CLE event %s/%s/%d", ErrImportIdentityConflict, ownerType, ownerUUID, e.ID)
			}
			return false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return false, err
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO cle_event (owner_type, owner_uuid, id, type, effective, published, version, support_id, license, superseded_by_version, event_id_ref, reason, description)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ownerType, ownerUUID, e.ID, e.Type, formatTime(e.Effective), formatTime(e.Published),
			nullIfEmpty(e.Version), nullIfEmpty(e.SupportID), nullIfEmpty(e.License), nullIfEmpty(e.SupersededByVersion),
			e.EventID, nullIfEmpty(e.Reason), nullIfEmpty(e.Description),
		)
		if err != nil {
			return false, err
		}

		for _, v := range e.Versions {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO cle_event_version (owner_type, owner_uuid, event_id, version, version_range) VALUES (?, ?, ?, ?, ?)`,
				ownerType, ownerUUID, e.ID, nullIfEmpty(v.Version), nullIfEmpty(v.Range),
			); err != nil {
				return false, err
			}
		}
		for _, ident := range e.Identifiers {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO cle_event_identifier (owner_type, owner_uuid, event_id, id_type, id_value) VALUES (?, ?, ?, ?, ?)`,
				ownerType, ownerUUID, e.ID, ident.IDType, ident.IDValue,
			); err != nil {
				return false, err
			}
		}
		for _, ref := range e.References {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO cle_event_reference (owner_type, owner_uuid, event_id, uri) VALUES (?, ?, ?, ?)`,
				ownerType, ownerUUID, e.ID, ref,
			); err != nil {
				return false, err
			}
		}

		if err := bumpCLERevisionTx(ctx, tx, ownerType, ownerUUID); err != nil {
			return false, err
		}

		return true, nil
	})
}

// cleEventConflicts reports whether existing (already stored) differs from
// e (being imported) in a way that means they're not the same event.
// Unlike most other entities' conflict checks, there's no
// denormalized/cached field to exclude here -- everything is the source's
// own assertion.
func cleEventConflicts(existing, e tea.CLEEvent) bool {
	if existing.Type != e.Type {
		return true
	}
	if !existing.Effective.Equal(e.Effective) {
		return true
	}
	if !existing.Published.Equal(e.Published) {
		return true
	}
	if existing.Version != e.Version {
		return true
	}
	if !setEqual(existing.Versions, e.Versions) {
		return true
	}
	if existing.SupportID != e.SupportID {
		return true
	}
	if existing.License != e.License {
		return true
	}
	if existing.SupersededByVersion != e.SupersededByVersion {
		return true
	}
	if !setEqual(existing.Identifiers, e.Identifiers) {
		return true
	}
	if !ptrEqual(existing.EventID, e.EventID) {
		return true
	}
	if existing.Reason != e.Reason {
		return true
	}
	if existing.Description != e.Description {
		return true
	}
	return !setEqual(existing.References, e.References)
}

// ImportCLESupportDefinition creates the support definition if one with this
// (ownerType, ownerUUID, id) doesn't already exist, or leaves an existing
// one untouched if its content matches. Returns ErrImportIdentityConflict
// if a definition with this (ownerType, ownerUUID, id) already exists with
// different content. Unlike artifact/distribution URLs, URL here points to
// an external support-policy document that's never rewritten at import
// time, so it's real identity and is compared.
func (r *Repo) ImportCLESupportDefinition(ctx context.Context, ownerType, ownerUUID string, def tea.CLESupportDefinition) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		var description string
		var url sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT description, url FROM cle_support_definition WHERE owner_type = ? AND owner_uuid = ? AND id = ?`,
			ownerType, ownerUUID, def.ID,
		).Scan(&description, &url)
		switch {
		case err == nil:
			if description != def.Description || url.String != def.URL {
				return false, fmt.Errorf("%w: CLE support definition %s/%s/%s", ErrImportIdentityConflict, ownerType, ownerUUID, def.ID)
			}
			return false, nil
		case !errors.Is(err, sql.ErrNoRows):
			return false, err
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO cle_support_definition (owner_type, owner_uuid, id, description, url) VALUES (?, ?, ?, ?, ?)`,
			ownerType, ownerUUID, def.ID, def.Description, nullIfEmpty(def.URL),
		); err != nil {
			return false, err
		}
		if err := bumpCLERevisionTx(ctx, tx, ownerType, ownerUUID); err != nil {
			return false, err
		}
		return true, nil
	})
}

// CreateCLESupportDefinition records a support policy definition for
// (ownerType, ownerUUID), referenceable by CLE events via their SupportID.
func (r *Repo) CreateCLESupportDefinition(ctx context.Context, ownerType, ownerUUID string, def tea.CLESupportDefinition) (tea.CLESupportDefinition, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.CLESupportDefinition, error) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO cle_support_definition (owner_type, owner_uuid, id, description, url) VALUES (?, ?, ?, ?, ?)`,
			ownerType, ownerUUID, def.ID, def.Description, nullIfEmpty(def.URL),
		); err != nil {
			return tea.CLESupportDefinition{}, err
		}
		if err := bumpCLERevisionTx(ctx, tx, ownerType, ownerUUID); err != nil {
			return tea.CLESupportDefinition{}, err
		}
		return def, nil
	})
}

// GetCLE returns the full CLE object (events ordered newest-first by id, per
// the spec's ordering requirement, plus any support-policy definitions) for
// the given owner. Returns an empty (not-found-free) CLE if the owner simply
// has no events yet, matching "GET .../cle" being a valid call on any
// existing product/release even before any lifecycle events are recorded.
func (r *Repo) GetCLE(ctx context.Context, ownerType, ownerUUID string) (tea.CLE, error) {
	return getCLETx(ctx, r.conn(), ownerType, ownerUUID)
}

// GetCLERevision fetches just the revision counter for one CLE owner -- for
// ETag construction, so a conditional GET can check If-None-Match against a
// single indexed lookup instead of the full CLE fetch (which joins events,
// their versions/identifiers/references, and support definitions).
// Returns 0, nil (not ErrNotFound) if the owner has no CLE data yet,
// matching GetCLE's own "empty CLE is valid" semantics -- unlike every
// other entity here, GetCLE never checks whether ownerUUID itself still
// exists, so this can't distinguish "no CLE data" from "no such owner"
// either; callers needing that distinction must check the owner separately
// (see internal/api/cle.go).
func (r *Repo) GetCLERevision(ctx context.Context, ownerType, ownerUUID string) (int64, error) {
	return getCLERevisionTx(ctx, r.conn(), ownerType, ownerUUID)
}

func getCLERevisionTx(ctx context.Context, q dbtx, ownerType, ownerUUID string) (int64, error) {
	var revision int64
	err := q.QueryRowContext(ctx, `SELECT revision FROM cle_revision WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return revision, nil
}

// bumpCLERevisionTx increments (ownerType, ownerUUID)'s CLE revision,
// creating its row at revision 1 on the first ever bump. Called from every
// CLE write (CreateCLEEvent/ImportCLEEvent/CreateCLESupportDefinition/
// ImportCLESupportDefinition) and also from deleteOwnerScoped -- the latter
// is required, not optional: GetCLE (see its own doc comment) never checks
// owner existence, so without bumping here, deleting a product/release with
// CLE events would leave a client's cached CLE ETag matching forever
// against now-deleted event data instead of the (now-empty) current state.
func bumpCLERevisionTx(ctx context.Context, q dbtx, ownerType, ownerUUID string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO cle_revision (owner_type, owner_uuid, revision) VALUES (?, ?, 1)
		 ON CONFLICT (owner_type, owner_uuid) DO UPDATE SET revision = revision + 1`,
		ownerType, ownerUUID,
	)
	return err
}

// getCLETx is GetCLE's logic parameterized over a dbtx -- see product.go's
// getProductTx doc comment for why this exists.
func getCLETx(ctx context.Context, q dbtx, ownerType, ownerUUID string) (tea.CLE, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id FROM cle_event WHERE owner_type = ? AND owner_uuid = ? ORDER BY id DESC`, ownerType, ownerUUID)
	if err != nil {
		return tea.CLE{}, err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return tea.CLE{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return tea.CLE{}, err
	}
	_ = rows.Close()

	events := []tea.CLEEvent{}
	for _, id := range ids {
		e, err := getCLEEventTx(ctx, q, ownerType, ownerUUID, id)
		if err != nil {
			return tea.CLE{}, err
		}
		events = append(events, e)
	}

	defs, err := listCLESupportDefinitions(ctx, q, ownerType, ownerUUID)
	if err != nil {
		return tea.CLE{}, err
	}

	cle := tea.CLE{Events: events}
	if len(defs) > 0 {
		cle.Definitions = &tea.CLEDefinitions{Support: defs}
	}
	return cle, nil
}

// getCLEEventTx fetches one CLE event by its exact (ownerType, ownerUUID,
// id) key. Returns ErrNotFound if it doesn't exist.
func getCLEEventTx(ctx context.Context, q dbtx, ownerType, ownerUUID string, id int) (tea.CLEEvent, error) {
	var (
		eventType                                                      string
		effective, published                                           string
		version, supportID, license, supersededByVersion, reason, desc sql.NullString
		eventIDRef                                                     sql.NullInt64
	)
	err := q.QueryRowContext(ctx,
		`SELECT type, effective, published, version, support_id, license, superseded_by_version, event_id_ref, reason, description
		 FROM cle_event WHERE owner_type = ? AND owner_uuid = ? AND id = ?`,
		ownerType, ownerUUID, id,
	).Scan(&eventType, &effective, &published, &version, &supportID, &license, &supersededByVersion, &eventIDRef, &reason, &desc)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.CLEEvent{}, ErrNotFound
	}
	if err != nil {
		return tea.CLEEvent{}, err
	}

	eff, err := parseTime(effective)
	if err != nil {
		return tea.CLEEvent{}, err
	}
	pub, err := parseTime(published)
	if err != nil {
		return tea.CLEEvent{}, err
	}

	versions, err := listCLEEventVersions(ctx, q, ownerType, ownerUUID, id)
	if err != nil {
		return tea.CLEEvent{}, err
	}
	identifiers, err := listCLEEventIdentifiers(ctx, q, ownerType, ownerUUID, id)
	if err != nil {
		return tea.CLEEvent{}, err
	}
	references, err := listCLEEventReferences(ctx, q, ownerType, ownerUUID, id)
	if err != nil {
		return tea.CLEEvent{}, err
	}

	e := tea.CLEEvent{
		ID:                  id,
		Type:                eventType,
		Effective:           eff,
		Published:           pub,
		Version:             version.String,
		Versions:            versions,
		SupportID:           supportID.String,
		License:             license.String,
		SupersededByVersion: supersededByVersion.String,
		Identifiers:         identifiers,
		Reason:              reason.String,
		Description:         desc.String,
		References:          references,
	}
	if eventIDRef.Valid {
		v := int(eventIDRef.Int64)
		e.EventID = &v
	}
	return e, nil
}

func listCLEEventVersions(ctx context.Context, q dbtx, ownerType, ownerUUID string, eventID int) ([]tea.CLEVersionSpecifier, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT version, version_range FROM cle_event_version WHERE owner_type = ? AND owner_uuid = ? AND event_id = ?`,
		ownerType, ownerUUID, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []tea.CLEVersionSpecifier
	for rows.Next() {
		var v, rng sql.NullString
		if err := rows.Scan(&v, &rng); err != nil {
			return nil, err
		}
		out = append(out, tea.CLEVersionSpecifier{Version: v.String, Range: rng.String})
	}
	return out, rows.Err()
}

func listCLEEventIdentifiers(ctx context.Context, q dbtx, ownerType, ownerUUID string, eventID int) ([]tea.Identifier, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id_type, id_value FROM cle_event_identifier WHERE owner_type = ? AND owner_uuid = ? AND event_id = ?`,
		ownerType, ownerUUID, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []tea.Identifier
	for rows.Next() {
		var id tea.Identifier
		if err := rows.Scan(&id.IDType, &id.IDValue); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func listCLEEventReferences(ctx context.Context, q dbtx, ownerType, ownerUUID string, eventID int) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT uri FROM cle_event_reference WHERE owner_type = ? AND owner_uuid = ? AND event_id = ?`,
		ownerType, ownerUUID, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return nil, err
		}
		out = append(out, uri)
	}
	return out, rows.Err()
}

func listCLESupportDefinitions(ctx context.Context, q dbtx, ownerType, ownerUUID string) ([]tea.CLESupportDefinition, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, description, url FROM cle_support_definition WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []tea.CLESupportDefinition
	for rows.Next() {
		var d tea.CLESupportDefinition
		var url sql.NullString
		if err := rows.Scan(&d.ID, &d.Description, &url); err != nil {
			return nil, err
		}
		d.URL = url.String
		out = append(out, d)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
