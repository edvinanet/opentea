// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// Sentinel errors specific to the collection-draft workflow
// (design/publisher-service.md §7.7-7.10, design/publisher-openapi.yaml's
// collectionDraft operations). internal/publisher maps these to the HTTP
// status codes the OpenAPI draft documents for each operation.
var (
	// ErrDraftLocked is returned by any operation that mutates a draft's
	// content or approval while an outstanding prepareCollectionCommit lock
	// is held (409 -- design/publisher-openapi.yaml's putCollectionDraft/
	// approveCollectionDraft/rejectCollectionDraft responses).
	ErrDraftLocked = errors.New("repo: collection draft is locked by an outstanding prepareCollectionCommit")
	// ErrSelfApproval is returned by DecideCollectionDraft when actor
	// equals the draft's own draftedBy (403 -- maker-checker,
	// design/publisher-service.md §7.9).
	ErrSelfApproval = errors.New("repo: actor must differ from the draft's drafted-by actor")
	// ErrApprovalRequired is returned when prepareCollectionCommit/commit
	// is attempted without a current, matching, unexpired approval on
	// record (409).
	ErrApprovalRequired = errors.New("repo: no current approval on record for this draft")
	// ErrLockNotHeld is returned by cancelPrepareCollectionCommit (no lock
	// to release) and by PeekCollectionDraftCommit (commit attempted
	// without a preceding, still-valid prepareCollectionCommit) -- both
	// map to 404 per design/publisher-openapi.yaml's cancelPrepare
	// response ("No such draft, or no lock currently held").
	ErrLockNotHeld = errors.New("repo: no outstanding prepareCollectionCommit lock is held for this draft")
)

// collectionDraftRow is collection_draft's raw column set, before parsing
// into the wire-shaped teapublisher.CollectionDraft (getCollectionDraftTx)
// or checking against it (lockHeld, buildWouldBeCollectionTx).
type collectionDraftRow struct {
	revision                  int
	draftedBy                 string
	updateReasonType          sql.NullString
	updateReasonComment       sql.NullString
	createdAt                 string
	expiresAt                 string
	lockExpiresAt             sql.NullString
	lockDate                  sql.NullString
	approvalStatus            string
	approvalDecidedBy         sql.NullString
	approvalDecidedAt         sql.NullString
	approvalDecidedAtRevision sql.NullInt64
	approvalExpiresAt         sql.NullString
	approvalComment           sql.NullString
}

func fetchCollectionDraftRowTx(ctx context.Context, q dbtx, ownerType, ownerUUID string) (collectionDraftRow, error) {
	var row collectionDraftRow
	err := q.QueryRowContext(ctx,
		`SELECT revision, drafted_by, update_reason_type, update_reason_comment, created_at, expires_at, lock_expires_at, lock_date,
		        approval_status, approval_decided_by, approval_decided_at, approval_decided_at_revision, approval_expires_at, approval_comment
		 FROM collection_draft WHERE owner_type = ? AND owner_uuid = ?`,
		ownerType, ownerUUID,
	).Scan(&row.revision, &row.draftedBy, &row.updateReasonType, &row.updateReasonComment, &row.createdAt, &row.expiresAt, &row.lockExpiresAt, &row.lockDate,
		&row.approvalStatus, &row.approvalDecidedBy, &row.approvalDecidedAt, &row.approvalDecidedAtRevision, &row.approvalExpiresAt, &row.approvalComment)
	if errors.Is(err, sql.ErrNoRows) {
		return collectionDraftRow{}, ErrNotFound
	}
	if err != nil {
		return collectionDraftRow{}, err
	}
	return row, nil
}

func listCollectionDraftArtifactsTx(ctx context.Context, q dbtx, ownerType, ownerUUID string) ([]ArtifactRef, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT artifact_uuid, artifact_version FROM collection_draft_artifact WHERE owner_type = ? AND owner_uuid = ? ORDER BY rowid`,
		ownerType, ownerUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []ArtifactRef
	for rows.Next() {
		var a ArtifactRef
		if err := rows.Scan(&a.UUID, &a.Version); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func lockHeld(row collectionDraftRow) bool {
	if !row.lockExpiresAt.Valid {
		return false
	}
	t, err := parseTime(row.lockExpiresAt.String)
	if err != nil {
		return false
	}
	return t.After(time.Now())
}

// requireReleaseOwnerExistsTx confirms ownerUUID exists as ownerType's
// underlying release ("PRODUCT_RELEASE" -> product_release,
// "COMPONENT_RELEASE" -> component_release -- the same vocabulary as
// collection.belongs_to/BelongsToProductRelease/BelongsToComponentRelease).
func requireReleaseOwnerExistsTx(ctx context.Context, q dbtx, ownerType, ownerUUID string) error {
	var table string
	switch ownerType {
	case BelongsToProductRelease:
		table = "product_release"
	case BelongsToComponentRelease:
		table = "component_release"
	default:
		return fmt.Errorf("repo: unknown collection draft owner type %q", ownerType)
	}
	var exists int
	err := q.QueryRowContext(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE uuid = ?`, table), ownerUUID).Scan(&exists) //nolint:gosec // table is one of two hardcoded literals selected by a switch above, never user input
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// PutCollectionDraft creates or replaces (ownerType, ownerUUID)'s draft
// artifact list (design/publisher-service.md §7.7): each artifact must
// already exist (ErrNotFound otherwise), the draft must not be locked by
// an outstanding prepareCollectionCommit (ErrDraftLocked otherwise).
// Bumps revision and resets approval to "none" -- editing invalidates
// whatever approval was on record for the previous content. ttl sets the
// refreshed expiresAt (design/publisher-service.md §9.6,
// config.PublisherDraftTTL).
func (r *Repo) PutCollectionDraft(ctx context.Context, ownerType, ownerUUID, actor string, artifacts []ArtifactRef, updateReason *tea.UpdateReason, ttl time.Duration) (teapublisher.CollectionDraft, error) {
	return runInTx(ctx, r, func(tx dbtx) (teapublisher.CollectionDraft, error) {
		if err := requireReleaseOwnerExistsTx(ctx, tx, ownerType, ownerUUID); err != nil {
			return teapublisher.CollectionDraft{}, err
		}
		for _, a := range artifacts {
			var exists int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM artifact WHERE uuid = ? AND version = ?`, a.UUID, a.Version).Scan(&exists)
			if errors.Is(err, sql.ErrNoRows) {
				return teapublisher.CollectionDraft{}, ErrNotFound
			}
			if err != nil {
				return teapublisher.CollectionDraft{}, err
			}
		}

		revision := 1
		existing, err := fetchCollectionDraftRowTx(ctx, tx, ownerType, ownerUUID)
		switch {
		case err == nil:
			if lockHeld(existing) {
				return teapublisher.CollectionDraft{}, ErrDraftLocked
			}
			revision = existing.revision + 1
		case errors.Is(err, ErrNotFound):
			// No existing draft -- creating one at revision 1.
		default:
			return teapublisher.CollectionDraft{}, err
		}

		var reasonType, reasonComment any
		if updateReason != nil {
			reasonType = updateReason.Type
			if updateReason.Comment != "" {
				reasonComment = updateReason.Comment
			}
		}
		expiresAt := formatTime(time.Now().Add(ttl))

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO collection_draft (owner_type, owner_uuid, revision, drafted_by, update_reason_type, update_reason_comment, expires_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (owner_type, owner_uuid) DO UPDATE SET
			   revision = excluded.revision, drafted_by = excluded.drafted_by,
			   update_reason_type = excluded.update_reason_type, update_reason_comment = excluded.update_reason_comment,
			   expires_at = excluded.expires_at, lock_expires_at = NULL, lock_date = NULL,
			   approval_status = 'none', approval_decided_by = NULL, approval_decided_at = NULL,
			   approval_decided_at_revision = NULL, approval_expires_at = NULL, approval_comment = NULL`,
			ownerType, ownerUUID, revision, actor, reasonType, reasonComment, expiresAt,
		); err != nil {
			return teapublisher.CollectionDraft{}, err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM collection_draft_artifact WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID); err != nil {
			return teapublisher.CollectionDraft{}, err
		}
		for _, a := range artifacts {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO collection_draft_artifact (owner_type, owner_uuid, artifact_uuid, artifact_version) VALUES (?, ?, ?, ?)`,
				ownerType, ownerUUID, a.UUID, a.Version,
			); err != nil {
				return teapublisher.CollectionDraft{}, err
			}
		}

		return getCollectionDraftTx(ctx, tx, ownerType, ownerUUID)
	})
}

// GetCollectionDraft fetches (ownerType, ownerUUID)'s current draft state,
// including a diff against the owner's current live collection (added/
// removed artifact UUIDs). Returns ErrNotFound if no draft exists.
func (r *Repo) GetCollectionDraft(ctx context.Context, ownerType, ownerUUID string) (teapublisher.CollectionDraft, error) {
	return getCollectionDraftTx(ctx, r.conn(), ownerType, ownerUUID)
}

func getCollectionDraftTx(ctx context.Context, q dbtx, ownerType, ownerUUID string) (teapublisher.CollectionDraft, error) {
	row, err := fetchCollectionDraftRowTx(ctx, q, ownerType, ownerUUID)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}
	refs, err := listCollectionDraftArtifactsTx(ctx, q, ownerType, ownerUUID)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}

	draftUUIDs := make(map[string]bool, len(refs))
	artifactRefs := make([]teapublisher.ArtifactVersionRef, len(refs))
	for i, a := range refs {
		artifactRefs[i] = teapublisher.ArtifactVersionRef{UUID: a.UUID, Version: a.Version}
		draftUUIDs[a.UUID] = true
	}

	diff := &teapublisher.CollectionDraftDiff{}
	currentVersion, ok, err := latestCollectionVersionTx(ctx, q, ownerUUID, ownerType)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}
	if ok {
		current, err := listCollectionArtifacts(ctx, q, ownerUUID, currentVersion)
		if err != nil {
			return teapublisher.CollectionDraft{}, err
		}
		currentUUIDs := make(map[string]bool, len(current))
		for _, a := range current {
			currentUUIDs[a.UUID] = true
			if !draftUUIDs[a.UUID] {
				diff.Removed = append(diff.Removed, a.UUID)
			}
		}
		for uuid := range draftUUIDs {
			if !currentUUIDs[uuid] {
				diff.Added = append(diff.Added, uuid)
			}
		}
	} else {
		for uuid := range draftUUIDs {
			diff.Added = append(diff.Added, uuid)
		}
	}

	createdAt, err := parseTime(row.createdAt)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}
	expiresAt, err := parseTime(row.expiresAt)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}
	lockExpiresAt, err := parseNullTime(row.lockExpiresAt)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}

	var updateReason *tea.UpdateReason
	if row.updateReasonType.Valid {
		updateReason = &tea.UpdateReason{Type: row.updateReasonType.String, Comment: row.updateReasonComment.String}
	}

	decidedAt, err := parseNullTime(row.approvalDecidedAt)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}
	approvalExpiresAt, err := parseNullTime(row.approvalExpiresAt)
	if err != nil {
		return teapublisher.CollectionDraft{}, err
	}
	approval := &teapublisher.CollectionDraftApproval{
		Status:    row.approvalStatus,
		DecidedBy: row.approvalDecidedBy.String,
		DecidedAt: decidedAt,
		ExpiresAt: approvalExpiresAt,
		Comment:   row.approvalComment.String,
	}
	if row.approvalDecidedAtRevision.Valid {
		approval.DecidedAtRevision = int(row.approvalDecidedAtRevision.Int64)
	}

	return teapublisher.CollectionDraft{
		OwnerType:          ownerType,
		OwnerUUID:          ownerUUID,
		CreatedAt:          &createdAt,
		ExpiresAt:          &expiresAt,
		LockExpiresAt:      lockExpiresAt,
		Artifacts:          artifactRefs,
		UpdateReason:       updateReason,
		DiffAgainstCurrent: diff,
		DraftedBy:          row.draftedBy,
		Revision:           row.revision,
		Approval:           approval,
	}, nil
}

// DeleteCollectionDraft abandons (ownerType, ownerUUID)'s draft without
// publishing it (cascades to its collection_draft_artifact rows). Returns
// ErrNotFound if no draft exists.
func (r *Repo) DeleteCollectionDraft(ctx context.Context, ownerType, ownerUUID string) error {
	res, err := r.conn().ExecContext(ctx, `DELETE FROM collection_draft WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DecideCollectionDraft records an approve/reject decision against the
// draft's *current* revision (design/publisher-service.md §7.9). Returns
// ErrNotFound if no draft exists, ErrDraftLocked if a
// prepareCollectionCommit lock is held, ErrSelfApproval if actor equals
// the draft's own draftedBy (maker-checker, checked for both approve and
// reject). approvalTTL sets ExpiresAt on approval only (config.PublisherApprovalTTL);
// a rejection has no expiry.
func (r *Repo) DecideCollectionDraft(ctx context.Context, ownerType, ownerUUID, actor, comment string, approve bool, approvalTTL time.Duration) (teapublisher.CollectionDraft, error) {
	return runInTx(ctx, r, func(tx dbtx) (teapublisher.CollectionDraft, error) {
		row, err := fetchCollectionDraftRowTx(ctx, tx, ownerType, ownerUUID)
		if err != nil {
			return teapublisher.CollectionDraft{}, err
		}
		if lockHeld(row) {
			return teapublisher.CollectionDraft{}, ErrDraftLocked
		}
		if actor == row.draftedBy {
			return teapublisher.CollectionDraft{}, ErrSelfApproval
		}

		status := "rejected"
		var expiresAt any
		if approve {
			status = "approved"
			expiresAt = formatTime(time.Now().Add(approvalTTL))
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE collection_draft SET approval_status = ?, approval_decided_by = ?, approval_decided_at = ?,
			   approval_decided_at_revision = ?, approval_expires_at = ?, approval_comment = ?
			 WHERE owner_type = ? AND owner_uuid = ?`,
			status, actor, formatTime(time.Now()), row.revision, expiresAt, nullIfEmpty(comment),
			ownerType, ownerUUID,
		); err != nil {
			return teapublisher.CollectionDraft{}, err
		}

		return getCollectionDraftTx(ctx, tx, ownerType, ownerUUID)
	})
}

// buildWouldBeCollectionTx builds the collection (ownerType, ownerUUID)'s
// draft would become if committed right now: the next version number
// (fresh MAX(version)+1, same as createCollection -- never trusts a value
// computed by an earlier call, so it's correct even called from inside
// CommitCollectionDraft's own transaction), the draft's artifact refs
// resolved to full tea.Artifact objects, and its update reason. Requires
// approval.status == "approved" at the draft's *current* revision, with
// approval.expiresAt still in the future -- ErrApprovalRequired otherwise;
// this is the one place approval is actually enforced (design/publisher-service.md
// §10.3). If requireLock, also requires an unexpired prepareCollectionCommit
// lock -- ErrLockNotHeld otherwise (used by PeekCollectionDraftCommit and
// CommitCollectionDraft, not by PrepareCollectionCommit itself, which is
// what acquires the lock in the first place).
func buildWouldBeCollectionTx(ctx context.Context, q dbtx, ownerType, ownerUUID string, requireLock bool) (tea.Collection, error) {
	row, err := fetchCollectionDraftRowTx(ctx, q, ownerType, ownerUUID)
	if err != nil {
		return tea.Collection{}, err
	}
	if requireLock && !lockHeld(row) {
		return tea.Collection{}, ErrLockNotHeld
	}
	if row.approvalStatus != "approved" ||
		!row.approvalDecidedAtRevision.Valid || int(row.approvalDecidedAtRevision.Int64) != row.revision ||
		!row.approvalExpiresAt.Valid {
		return tea.Collection{}, ErrApprovalRequired
	}
	approvalExpiresAt, err := parseTime(row.approvalExpiresAt.String)
	if err != nil {
		return tea.Collection{}, err
	}
	if !approvalExpiresAt.After(time.Now()) {
		return tea.Collection{}, ErrApprovalRequired
	}

	refs, err := listCollectionDraftArtifactsTx(ctx, q, ownerType, ownerUUID)
	if err != nil {
		return tea.Collection{}, err
	}
	artifacts := make([]tea.Artifact, 0, len(refs))
	for _, ref := range refs {
		a, err := getArtifactByVersionTx(ctx, q, ref.UUID, ref.Version)
		if err != nil {
			return tea.Collection{}, err
		}
		artifacts = append(artifacts, a)
	}

	var maxVersion sql.NullInt64
	if err := q.QueryRowContext(ctx, `SELECT MAX(version) FROM collection WHERE uuid = ?`, ownerUUID).Scan(&maxVersion); err != nil {
		return tea.Collection{}, err
	}
	nextVersion := 1
	if maxVersion.Valid {
		nextVersion = int(maxVersion.Int64) + 1
	}

	var updateReason *tea.UpdateReason
	if row.updateReasonType.Valid {
		updateReason = &tea.UpdateReason{Type: row.updateReasonType.String, Comment: row.updateReasonComment.String}
	}

	// The would-be collection's Date must stay fixed across every
	// recomputation while a lock is held -- PrepareCollectionCommit's
	// caller signs a digest computed over this exact value, and
	// commitCollectionDraft must re-derive the identical digest moments
	// later (design/publisher-openapi.yaml's commit summary). requireLock
	// callers (PeekCollectionDraftCommit, CommitCollectionDraft) reuse the
	// lock_date PrepareCollectionCommit recorded; PrepareCollectionCommit
	// itself (requireLock=false, since it's what sets the lock) mints a
	// fresh one and its caller persists it alongside lock_expires_at.
	var date time.Time
	if requireLock {
		if !row.lockDate.Valid {
			return tea.Collection{}, fmt.Errorf("repo: collection draft lock held without a recorded lock date")
		}
		date, err = parseTime(row.lockDate.String)
		if err != nil {
			return tea.Collection{}, err
		}
	} else {
		// Round-tripped through formatTime/parseTime immediately (rather
		// than the raw, nanosecond-precision time.Now()) so this call's
		// returned Date already exactly equals what a later requireLock
		// call will reconstruct from the second-precision lock_date column
		// -- the digest computed over this value must match bit-for-bit.
		date, err = parseTime(formatTime(time.Now()))
		if err != nil {
			return tea.Collection{}, err
		}
	}

	return tea.Collection{
		UUID:         ownerUUID,
		Version:      nextVersion,
		Date:         date,
		BelongsTo:    ownerType,
		Artifacts:    artifacts,
		UpdateReason: updateReason,
	}, nil
}

// PrepareCollectionCommit is Collection Signing part 1
// (design/publisher-service.md §7.8): builds and returns the would-be
// collection (ErrApprovalRequired if there's no current, unexpired
// approval on record), and locks the draft against further
// PutCollectionDraft calls until CommitCollectionDraft succeeds or
// CancelPrepareCollectionCommit releases it. lockTTL sets the lock's
// expiry (config.PublisherLockTTL) -- past that time the lock releases
// automatically.
func (r *Repo) PrepareCollectionCommit(ctx context.Context, ownerType, ownerUUID string, lockTTL time.Duration) (tea.Collection, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.Collection, error) {
		wouldBe, err := buildWouldBeCollectionTx(ctx, tx, ownerType, ownerUUID, false)
		if err != nil {
			return tea.Collection{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE collection_draft SET lock_expires_at = ?, lock_date = ? WHERE owner_type = ? AND owner_uuid = ?`,
			formatTime(time.Now().Add(lockTTL)), formatTime(wouldBe.Date), ownerType, ownerUUID,
		); err != nil {
			return tea.Collection{}, err
		}
		return wouldBe, nil
	})
}

// PeekCollectionDraftCommit re-derives the exact same would-be collection
// PrepareCollectionCommit last returned, read-only, requiring the lock it
// set to still be held (ErrLockNotHeld otherwise). internal/publisher's
// commitCollectionDraft handler calls this immediately before verifying
// the caller's submitted signature -- never trusts a digest cached from an
// earlier prepare call (design/publisher-openapi.yaml's commit summary:
// "The server re-derives the same digest prepareCommit returned").
func (r *Repo) PeekCollectionDraftCommit(ctx context.Context, ownerType, ownerUUID string) (tea.Collection, error) {
	return buildWouldBeCollectionTx(ctx, r.conn(), ownerType, ownerUUID, true)
}

// CancelPrepareCollectionCommit releases the lock PrepareCollectionCommit
// placed, without deleting the draft. Returns ErrNotFound if no draft
// exists, ErrLockNotHeld if no lock is currently held.
func (r *Repo) CancelPrepareCollectionCommit(ctx context.Context, ownerType, ownerUUID string) error {
	row, err := fetchCollectionDraftRowTx(ctx, r.conn(), ownerType, ownerUUID)
	if err != nil {
		return err
	}
	if !lockHeld(row) {
		return ErrLockNotHeld
	}
	_, err = r.conn().ExecContext(ctx, `UPDATE collection_draft SET lock_expires_at = NULL, lock_date = NULL WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID)
	return err
}

// CommitEvidenceInput carries the already-verified evidence package
// (internal/publisher's handler verifies it against PeekCollectionDraftCommit's
// digest before calling this -- CommitCollectionDraft persists, it does
// not verify, mirroring CreateEvidenceBundle's own contract).
type CommitEvidenceInput struct {
	ObjectMediaType        string
	ObjectLocation         string
	ObjectDigestValue      string
	SignatureFormat        string
	SignatureValue         string
	CertificateFormat      string
	CertificateValue       string
	CertificateFingerprint string
	CertificateTrustDomain string
}

// CommitCollectionDraft is Collection Signing part 2 + Commit, atomic
// (design/publisher-service.md §7.8, §7.10, §8): in one transaction,
// creates the real collection (via the now-composable
// CreateCollectionForProductRelease/ComponentRelease), creates its
// evidence bundle, and deletes the draft. If any step fails, none of it
// happens. Re-checks the lock and approval (defense in depth --
// internal/publisher's handler already checked both via
// PeekCollectionDraftCommit moments earlier) inside this same transaction.
func (r *Repo) CommitCollectionDraft(ctx context.Context, ownerType, ownerUUID string, evidence CommitEvidenceInput) (tea.Collection, error) {
	var result tea.Collection
	err := r.WithTx(ctx, func(tx *Repo) error {
		wouldBe, err := buildWouldBeCollectionTx(ctx, tx.conn(), ownerType, ownerUUID, true)
		if err != nil {
			return err
		}

		var createFn func(context.Context, string, CollectionInput) (tea.Collection, error)
		switch ownerType {
		case BelongsToProductRelease:
			createFn = tx.CreateCollectionForProductRelease
		case BelongsToComponentRelease:
			createFn = tx.CreateCollectionForComponentRelease
		default:
			return fmt.Errorf("repo: unknown collection draft owner type %q", ownerType)
		}

		refs := make([]ArtifactRef, len(wouldBe.Artifacts))
		for i, a := range wouldBe.Artifacts {
			refs[i] = ArtifactRef{UUID: a.UUID, Version: a.Version}
		}
		collection, err := createFn(ctx, ownerUUID, CollectionInput{UpdateReason: wouldBe.UpdateReason, Artifacts: refs})
		if err != nil {
			return err
		}
		if collection.Version != wouldBe.Version {
			// Can only happen if something else created a competing
			// collection version for this owner while the lock was held,
			// which nothing in this codebase does -- a defensive check,
			// not an expected path.
			return fmt.Errorf("repo: collection draft commit race: expected version %d, got %d", wouldBe.Version, collection.Version)
		}

		if _, err := tx.CreateEvidenceBundle(ctx, EvidenceBundleInput{
			OwnerType:              "COLLECTION",
			OwnerUUID:              ownerUUID,
			OwnerVersion:           collection.Version,
			ObjectType:             "tea-collection",
			ObjectMediaType:        evidence.ObjectMediaType,
			ObjectLocation:         evidence.ObjectLocation,
			ObjectDigestValue:      evidence.ObjectDigestValue,
			SignatureFormat:        evidence.SignatureFormat,
			SignatureValue:         evidence.SignatureValue,
			CertificateFormat:      evidence.CertificateFormat,
			CertificateValue:       evidence.CertificateValue,
			CertificateFingerprint: evidence.CertificateFingerprint,
			CertificateTrustDomain: evidence.CertificateTrustDomain,
		}); err != nil {
			return err
		}

		if _, err := tx.conn().ExecContext(ctx, `DELETE FROM collection_draft WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID); err != nil {
			return err
		}

		result = collection
		return nil
	})
	return result, err
}
