-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Internal business-approval workflow (design/publisher-service.md §18.8) --
-- the "single biggest remaining unknown" flagged when the GUI requirements
-- pass was written. Deliberately narrow v1: see the design doc's revision
-- history for what's simplified.
--
-- staff.workflow_role is a second, orthogonal role axis alongside
-- staff.role (0003_staff_roles.sql): that column gates *administrative*
-- capability (who manages targets/staff, this tool); this one records
-- §10.1's own workflow vocabulary ("release manager, component maintainer,
-- security/compliance approver"). Nullable -- not every staff account
-- participates in the publishing workflow (e.g. an infra-only admin).
-- Only 'security_compliance_approver' is actually gated on anywhere in
-- this pass; the other two are recorded because §10.1 already names them,
-- not because anything enforces them yet.
ALTER TABLE staff ADD COLUMN workflow_role TEXT
    CHECK (workflow_role IN ('release_manager','component_maintainer','security_compliance_approver'));

-- One business-approval request against a target's release. release_uuid
-- is stored and displayed as-is -- no live pkg/teaclient lookup to resolve
-- it to a human-readable label in this pass; there's no draft-assembly UI
-- yet (§18.7) to source a real reference from, so a request is typed in by
-- hand today. required_approvals lets a deployment ask for more than one
-- security_compliance_approver decision; a single rejection always fails
-- the request immediately, matching the asymmetry of the protocol-level
-- collection-draft approve/reject.
CREATE TABLE approval_request (
    uuid                TEXT PRIMARY KEY,
    target_uuid         TEXT NOT NULL REFERENCES target(uuid) ON DELETE CASCADE,
    release_kind        TEXT NOT NULL CHECK (release_kind IN ('productRelease','componentRelease')),
    release_uuid        TEXT NOT NULL,
    notes               TEXT,
    requested_by        TEXT REFERENCES staff(uuid) ON DELETE SET NULL,
    required_approvals  INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX idx_approval_request_target ON approval_request(target_uuid);
CREATE INDEX idx_approval_request_status ON approval_request(status);

-- One decision against a request. Maker-checker (approval.go's
-- RecordApprovalDecision) rejects a decision from the same staff_uuid that
-- created the request -- mirrors §10.1's own text: "whoever approves a
-- commit must be a different verified identity than whoever built the
-- draft."
CREATE TABLE approval_decision (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    request_uuid TEXT NOT NULL REFERENCES approval_request(uuid) ON DELETE CASCADE,
    staff_uuid   TEXT REFERENCES staff(uuid) ON DELETE SET NULL,
    decision     TEXT NOT NULL CHECK (decision IN ('approved','rejected')),
    comment      TEXT,
    decided_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX idx_approval_decision_request ON approval_decision(request_uuid);

-- Widen audit_log.target_type's CHECK (0003_staff_roles.sql) to also
-- accept 'approval_request' -- same SQLite rebuild-the-table approach as
-- 0003 itself (no ALTER TABLE ... ALTER CHECK in SQLite).
ALTER TABLE audit_log RENAME TO audit_log_pre0004;

CREATE TABLE audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    staff_uuid      TEXT REFERENCES staff(uuid) ON DELETE SET NULL,
    request_id      TEXT,
    operation       TEXT NOT NULL,
    target_type     TEXT NOT NULL CHECK (target_type IN ('target','staff','approval_request')),
    target_id       TEXT NOT NULL,
    previous_state  TEXT,
    resulting_state TEXT NOT NULL
);

INSERT INTO audit_log (id, at, staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state)
SELECT id, at, staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state
FROM audit_log_pre0004;

DROP TABLE audit_log_pre0004;

CREATE INDEX idx_audit_log_target ON audit_log(target_type, target_id);
CREATE INDEX idx_audit_log_at ON audit_log(at);
