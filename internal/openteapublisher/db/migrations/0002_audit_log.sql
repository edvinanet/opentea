-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Audit log for every mutation opentea-publisher makes to its own database
-- -- found missing while writing design/publisher-service.md §18 (GUI
-- requirements): §17.3's own storage plan named "its own audit log" as
-- part of the scope, but 0001_init.sql never actually added one. Mirrors
-- opentea's own admin_audit_log (internal/db/migrations/0005_authz.sql)
-- shape and reasoning closely: an audit record must commit atomically
-- with the mutation it describes (repo.go's WithTx), not be written best-
-- effort after the fact.
--
-- target_type is scoped to 'target' only -- the one resource with real
-- write actions today. Widen the CHECK the same way 0006_trust.sql widened
-- opentea's own admin_audit_log when a new mutable resource is added here
-- (staff-facing collection drafts, business-approval records, etc.).
CREATE TABLE audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    staff_uuid      TEXT REFERENCES staff(uuid) ON DELETE SET NULL,
    request_id      TEXT,
    operation       TEXT NOT NULL,
    target_type     TEXT NOT NULL CHECK (target_type IN ('target')),
    target_id       TEXT NOT NULL,
    previous_state  TEXT,
    resulting_state TEXT NOT NULL
);
CREATE INDEX idx_audit_log_target ON audit_log(target_type, target_id);
CREATE INDEX idx_audit_log_at ON audit_log(at);
