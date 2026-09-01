-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Staff roles/permissions -- found missing while writing
-- design/publisher-service.md §18 (GUI requirements): nothing limited
-- which staff members could do what, including sensitive actions like
-- storing a target's bearer credential (§18.9's own flagged gap, though
-- that section was really about a *later* need -- collection-draft
-- approval -- this migration addresses the more immediate one: target
-- credential management is already a real, sensitive write action today).
--
-- Two roles only, mirroring opentea's own user.role CHECK
-- (internal/db/migrations/0002_users_auth.sql) shape: 'admin' manages
-- targets and other staff accounts; 'member' can do everything else
-- (currently: view the dashboard). Not a capability system -- a flat
-- two-value role, same scope as opentea's own admin/consumer split, wide
-- enough to gate what actually needs gating today, no wider.
ALTER TABLE staff ADD COLUMN role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin','member'));

-- Widen audit_log.target_type's CHECK (0002_audit_log.sql) to also accept
-- 'staff' -- staff account creation/deletion is now itself an audited,
-- admin-only mutation. Same SQLite rebuild-the-table approach opentea's
-- own 0006_trust.sql/0007_publisher.sql used for the identical reason (no
-- ALTER TABLE ... ALTER CHECK in SQLite).
ALTER TABLE audit_log RENAME TO audit_log_pre0003;

CREATE TABLE audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    staff_uuid      TEXT REFERENCES staff(uuid) ON DELETE SET NULL,
    request_id      TEXT,
    operation       TEXT NOT NULL,
    target_type     TEXT NOT NULL CHECK (target_type IN ('target','staff')),
    target_id       TEXT NOT NULL,
    previous_state  TEXT,
    resulting_state TEXT NOT NULL
);

INSERT INTO audit_log (id, at, staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state)
SELECT id, at, staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state
FROM audit_log_pre0003;

DROP TABLE audit_log_pre0003;

CREATE INDEX idx_audit_log_target ON audit_log(target_type, target_id);
CREATE INDEX idx_audit_log_at ON audit_log(at);
