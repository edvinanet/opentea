-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Server-side implementation of the draft standard TEA Publisher API
-- (design/publisher-openapi.yaml, design/publisher-service.md), mounted at
-- /publisher/v1 (internal/publisher). Deliberately separate from
-- 0005_authz.sql (read-side authorization) and 0006_trust.sql (evidence
-- bundles, reused here as-is -- a collection's evidence bundle created by
-- commitCollectionDraft is a normal evidence_bundle row, no new table
-- needed for it).

-- Layer B/D bearer credentials (design/publisher-service.md §10.2/§10.4):
-- a service-level credential a publisher platform holds per target, not a
-- human user. Mirrors api_token's shape (0002_users_auth.sql) -- a
-- long-lived, explicitly revoked token, not a short TTL session -- rather
-- than session's, since there's no "login" here, just credential issuance.
-- scope is the credential-scoping decision design/publisher-service.md §11
-- open question #10 left open and this implementation resolves narrowly:
-- 'full' can call every /publisher/v1 operation; 'cicd' is deliberately
-- unable to call approveCollectionDraft/rejectCollectionDraft or any
-- product/component/release/CLE creation (design/publisher-service.md
-- §10.4, §14.3) -- enforced in internal/publisher/auth_middleware.go, not
-- here; this table only records which scope a credential was issued with.
CREATE TABLE publisher_credential (
    uuid       TEXT PRIMARY KEY,
    label      TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    scope      TEXT NOT NULL CHECK (scope IN ('full','cicd')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    revoked_at TEXT
);

-- One collection draft per (owner_type, owner_uuid) -- at most one open
-- draft per release by construction, not a separately-enforced constraint,
-- since a collection's own UUID always equals its owning release's UUID
-- (0001_init.sql's collection.uuid comment) and there is no separately
-- issued draft id (design/publisher-openapi.yaml's putCollectionDraft
-- summary). Column-for-column match of pkg/teapublisher.CollectionDraft +
-- CollectionDraftApproval (pkg/teapublisher/types.go) -- see
-- internal/repo/collectiondraft.go for how each field is populated/read.
CREATE TABLE collection_draft (
    owner_type                    TEXT NOT NULL CHECK (owner_type IN ('PRODUCT_RELEASE','COMPONENT_RELEASE')),
    owner_uuid                    TEXT NOT NULL,
    revision                      INTEGER NOT NULL DEFAULT 1,
    drafted_by                    TEXT NOT NULL,
    update_reason_type            TEXT,
    update_reason_comment         TEXT,
    created_at                    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    expires_at                    TEXT NOT NULL,
    lock_expires_at               TEXT,
    -- Fixed the moment prepareCollectionCommit sets lock_expires_at, and
    -- reused as-is by every later digest recomputation (PeekCollectionDraftCommit,
    -- commitCollectionDraft) while the lock is held -- the would-be
    -- collection's `date` field must stay identical between prepare and
    -- commit, since both are RFC 8785-canonicalized to a digest that must
    -- match exactly (see internal/repo/collectiondraft.go's
    -- buildWouldBeCollectionTx). Cleared alongside lock_expires_at.
    lock_date                     TEXT,
    approval_status               TEXT NOT NULL CHECK (approval_status IN ('none','approved','rejected')) DEFAULT 'none',
    approval_decided_by           TEXT,
    approval_decided_at           TEXT,
    approval_decided_at_revision  INTEGER,
    approval_expires_at           TEXT,
    approval_comment              TEXT,
    PRIMARY KEY (owner_type, owner_uuid)
);

-- A draft's staged artifact list -- same shape as collection_artifact
-- (0001_init.sql), ordered by rowid, but referencing a draft instead of a
-- committed collection. Replaced wholesale on every putCollectionDraft
-- (delete + re-insert -- see PutCollectionDraft), never partially updated.
CREATE TABLE collection_draft_artifact (
    owner_type       TEXT NOT NULL,
    owner_uuid       TEXT NOT NULL,
    artifact_uuid    TEXT NOT NULL,
    artifact_version INTEGER NOT NULL,
    FOREIGN KEY (owner_type, owner_uuid) REFERENCES collection_draft(owner_type, owner_uuid) ON DELETE CASCADE
);
CREATE INDEX idx_collection_draft_artifact_owner ON collection_draft_artifact(owner_type, owner_uuid);

-- Widen admin_audit_log.target_type's CHECK (0006_trust.sql) to also
-- accept 'publisher_credential' -- internal/admin/publishercredential.go
-- reuses the same generic admin-mutation audit-log plumbing (auditWrite)
-- as every other /admin/v1 handler. Same SQLite rebuild-the-table
-- approach 0006_trust.sql already used for the same reason (no
-- ALTER TABLE ... ALTER CHECK in SQLite).
ALTER TABLE admin_audit_log RENAME TO admin_audit_log_pre0007;

CREATE TABLE admin_audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    actor_uuid      TEXT REFERENCES user(uuid) ON DELETE SET NULL,
    request_id      TEXT,
    operation       TEXT NOT NULL,
    target_type     TEXT NOT NULL CHECK (target_type IN ('template','entitlement','product_group','release_group','evidence_bundle','publisher_credential')),
    target_id       TEXT NOT NULL,
    previous_state  TEXT,
    resulting_state TEXT NOT NULL,
    reason          TEXT
);

INSERT INTO admin_audit_log (id, at, actor_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state, reason)
SELECT id, at, actor_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state, reason
FROM admin_audit_log_pre0007;

DROP TABLE admin_audit_log_pre0007;

CREATE INDEX idx_admin_audit_log_target ON admin_audit_log(target_type, target_id);
CREATE INDEX idx_admin_audit_log_at ON admin_audit_log(at);
