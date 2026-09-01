-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- The CI/CD-facing API and its own capability scoping
-- (design/publisher-service.md §18.11). opentea-publisher stores exactly
-- one bearer credential per target (target.bearer_token), which must be
-- "full"-scoped for the GUI to do everything it does -- but it always
-- presents that "full" credential outbound regardless of who's actually
-- calling opentea-publisher itself, so the target's own full/cicd
-- separation (internal/publisher) provides no protection once CI/CD's
-- access is mediated through opentea-publisher. This table lets
-- opentea-publisher issue its *own*, narrower credentials -- one per
-- target, granting only the cicd-scoped subset of operations (see
-- cicdapi.go) -- reintroducing that scoping at opentea-publisher's own
-- boundary.
--
-- No scope column, unlike internal/repo's own publisher_credential table:
-- this credential type *is* the cicd scope, there is no "full" variant of
-- it -- issuing a "full" credential to CI/CD would defeat the point.
-- Soft-revoke (revoked_at nullable), mirroring publisher_credential's own
-- shape exactly, not a hard delete -- a revoked credential stays visible
-- in the audit trail.
CREATE TABLE cicd_credential (
    uuid        TEXT PRIMARY KEY,
    target_uuid TEXT NOT NULL REFERENCES target(uuid) ON DELETE CASCADE,
    label       TEXT NOT NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    revoked_at  TEXT
);

CREATE INDEX idx_cicd_credential_target ON cicd_credential(target_uuid);

-- Widen audit_log.target_type's CHECK (0004_business_approval.sql) to
-- also accept 'cicd_credential' -- same SQLite rebuild-the-table approach
-- as 0003/0004 (no ALTER TABLE ... ALTER CHECK in SQLite).
ALTER TABLE audit_log RENAME TO audit_log_pre0005;

CREATE TABLE audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    staff_uuid      TEXT REFERENCES staff(uuid) ON DELETE SET NULL,
    request_id      TEXT,
    operation       TEXT NOT NULL,
    target_type     TEXT NOT NULL CHECK (target_type IN ('target','staff','approval_request','cicd_credential')),
    target_id       TEXT NOT NULL,
    previous_state  TEXT,
    resulting_state TEXT NOT NULL
);

INSERT INTO audit_log (id, at, staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state)
SELECT id, at, staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state
FROM audit_log_pre0005;

DROP TABLE audit_log_pre0005;

CREATE INDEX idx_audit_log_target ON audit_log(target_type, target_id);
CREATE INDEX idx_audit_log_at ON audit_log(at);
