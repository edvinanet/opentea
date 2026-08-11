package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// ErrTemplateInUse is returned by DeleteTemplate when at least one
// entitlement still references (any revision of) the template -- the
// schema's ON DELETE RESTRICT on entitlement.template_uuid enforces this;
// this error is the domain-level translation of that constraint violation.
var ErrTemplateInUse = errors.New("repo: template is referenced by at least one entitlement")

// CreateTemplate creates a new template together with its first revision
// (revision 1) and that revision's capability rules, all in one
// transaction. The template is created with no active revision (spec
// Sec 13.1: activation must be a separate, explicit, audited step) -- call
// ActivateTemplateRevision to make it take effect.
func (r *Repo) CreateTemplate(ctx context.Context, name, description string, rules []model.TemplateCapabilityRule, createdBy, comment string) (model.Template, model.TemplateRevision, error) {
	uuid := idgen.New()

	rev, err := runInTx(ctx, r, func(tx dbtx) (model.TemplateRevision, error) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO template (uuid, name, description) VALUES (?, ?, ?)`, uuid, name, nullIfEmpty(description)); err != nil {
			return model.TemplateRevision{}, err
		}
		return insertTemplateRevisionTx(ctx, tx, uuid, 1, rules, createdBy, comment)
	})
	if err != nil {
		return model.Template{}, model.TemplateRevision{}, err
	}

	t, err := r.GetTemplate(ctx, uuid)
	return t, rev, err
}

// insertTemplateRevisionTx inserts template_revision row (templateUUID,
// revision) and its capability rules.
func insertTemplateRevisionTx(ctx context.Context, q dbtx, templateUUID string, revision int, rules []model.TemplateCapabilityRule, createdBy, comment string) (model.TemplateRevision, error) {
	createdAt := time.Now()
	if _, err := q.ExecContext(ctx,
		`INSERT INTO template_revision (template_uuid, revision, created_at, created_by, comment) VALUES (?, ?, ?, ?, ?)`,
		templateUUID, revision, formatTime(createdAt), nullIfEmpty(createdBy), nullIfEmpty(comment),
	); err != nil {
		return model.TemplateRevision{}, err
	}
	for _, rule := range rules {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO template_capability_rule (template_uuid, revision, capability, artifact_type, decision) VALUES (?, ?, ?, ?, ?)`,
			templateUUID, revision, string(rule.Capability), nullIfEmpty(string(rule.ArtifactType)), rule.Decision,
		); err != nil {
			return model.TemplateRevision{}, err
		}
	}
	return model.TemplateRevision{
		TemplateUUID: templateUUID,
		Revision:     revision,
		CreatedAt:    createdAt,
		CreatedBy:    createdBy,
		Comment:      comment,
		Rules:        rules,
	}, nil
}

// CreateTemplateRevision adds a new, immutable revision (current max + 1)
// to an existing template. Does not activate it -- see
// ActivateTemplateRevision. Returns ErrNotFound if templateUUID doesn't
// exist.
func (r *Repo) CreateTemplateRevision(ctx context.Context, templateUUID string, rules []model.TemplateCapabilityRule, createdBy, comment string) (model.TemplateRevision, error) {
	return runInTx(ctx, r, func(tx dbtx) (model.TemplateRevision, error) {
		if err := existsTemplateTx(ctx, tx, templateUUID); err != nil {
			return model.TemplateRevision{}, err
		}
		var maxRevision int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) FROM template_revision WHERE template_uuid = ?`, templateUUID).Scan(&maxRevision); err != nil {
			return model.TemplateRevision{}, err
		}
		return insertTemplateRevisionTx(ctx, tx, templateUUID, maxRevision+1, rules, createdBy, comment)
	})
}

// ActivateTemplateRevision sets templateUUID's active revision to revision.
// Returns ErrNotFound if either the template or that specific revision
// doesn't exist.
func (r *Repo) ActivateTemplateRevision(ctx context.Context, templateUUID string, revision int) error {
	_, err := runInTx(ctx, r, func(tx dbtx) (struct{}, error) {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM template_revision WHERE template_uuid = ? AND revision = ?`, templateUUID, revision).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return struct{}{}, ErrNotFound
		}
		if err != nil {
			return struct{}{}, err
		}
		res, err := tx.ExecContext(ctx, `UPDATE template SET active_revision = ? WHERE uuid = ?`, revision, templateUUID)
		if err != nil {
			return struct{}{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return struct{}{}, err
		} else if n == 0 {
			return struct{}{}, ErrNotFound
		}
		return struct{}{}, nil
	})
	return err
}

// GetTemplate fetches a template's metadata (not its revisions/rules -- see
// GetTemplateRevision) by UUID. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetTemplate(ctx context.Context, uuid string) (model.Template, error) {
	var t model.Template
	var description sql.NullString
	var activeRevision sql.NullInt64
	var createdAt string
	err := r.conn().QueryRowContext(ctx,
		`SELECT uuid, name, description, active_revision, created_at FROM template WHERE uuid = ?`, uuid,
	).Scan(&t.UUID, &t.Name, &description, &activeRevision, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Template{}, ErrNotFound
	}
	if err != nil {
		return model.Template{}, err
	}
	t.Description = description.String
	t.ActiveRevision = int(activeRevision.Int64)
	created, err := parseTime(createdAt)
	if err != nil {
		return model.Template{}, err
	}
	t.CreatedAt = created
	return t, nil
}

func existsTemplateTx(ctx context.Context, q dbtx, uuid string) error {
	var exists int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM template WHERE uuid = ?`, uuid).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// GetTemplateRevision fetches one specific revision of templateUUID,
// including its capability rules. Returns ErrNotFound if the (template,
// revision) pair doesn't exist.
func (r *Repo) GetTemplateRevision(ctx context.Context, templateUUID string, revision int) (model.TemplateRevision, error) {
	var rev model.TemplateRevision
	var createdBy sql.NullString
	var comment sql.NullString
	var createdAt string
	err := r.conn().QueryRowContext(ctx,
		`SELECT created_at, created_by, comment FROM template_revision WHERE template_uuid = ? AND revision = ?`,
		templateUUID, revision,
	).Scan(&createdAt, &createdBy, &comment)
	if errors.Is(err, sql.ErrNoRows) {
		return model.TemplateRevision{}, ErrNotFound
	}
	if err != nil {
		return model.TemplateRevision{}, err
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return model.TemplateRevision{}, err
	}
	rev.TemplateUUID = templateUUID
	rev.Revision = revision
	rev.CreatedAt = created
	rev.CreatedBy = createdBy.String
	rev.Comment = comment.String

	rules, err := listTemplateCapabilityRules(ctx, r.conn(), templateUUID, revision)
	if err != nil {
		return model.TemplateRevision{}, err
	}
	rev.Rules = rules
	return rev, nil
}

func listTemplateCapabilityRules(ctx context.Context, q dbtx, templateUUID string, revision int) ([]model.TemplateCapabilityRule, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT capability, artifact_type, decision FROM template_capability_rule WHERE template_uuid = ? AND revision = ?`,
		templateUUID, revision,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.TemplateCapabilityRule{}
	for rows.Next() {
		var capability, decision string
		var artifactType sql.NullString
		if err := rows.Scan(&capability, &artifactType, &decision); err != nil {
			return nil, err
		}
		out = append(out, model.TemplateCapabilityRule{
			Capability:   authz.Capability(capability),
			ArtifactType: authz.ArtifactType(artifactType.String),
			Decision:     decision,
		})
	}
	return out, rows.Err()
}

// ListTemplates returns every template, ordered by name.
func (r *Repo) ListTemplates(ctx context.Context) ([]model.Template, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT uuid, name, description, active_revision, created_at FROM template ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.Template{}
	for rows.Next() {
		var t model.Template
		var description sql.NullString
		var activeRevision sql.NullInt64
		var createdAt string
		if err := rows.Scan(&t.UUID, &t.Name, &description, &activeRevision, &createdAt); err != nil {
			return nil, err
		}
		t.Description = description.String
		t.ActiveRevision = int(activeRevision.Int64)
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		t.CreatedAt = created
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTemplate deletes templateUUID. Returns ErrNotFound if it doesn't
// exist, or ErrTemplateInUse if any entitlement still references it.
func (r *Repo) DeleteTemplate(ctx context.Context, uuid string) error {
	res, err := r.conn().ExecContext(ctx, `DELETE FROM template WHERE uuid = ?`, uuid)
	if isForeignKeyConstraintError(err) {
		return ErrTemplateInUse
	}
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
