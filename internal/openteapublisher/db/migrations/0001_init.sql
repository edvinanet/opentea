-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- opentea-publisher's own database -- entirely separate from opentea's own
-- (internal/db/migrations), reflecting that this is a genuinely separate
-- service (design/publisher-service.md §17, v0.21): its own staff
-- accounts (Layer A, §10.1) and its own per-target credentials
-- (Layer B, §10.5), not opentea's user/session/api_token tables.

-- Manufacturer staff account. No role column -- every logged-in staff
-- member has equal access in this phase; there's no permission boundary
-- designed yet to enforce one against (the internal multi-team
-- business-approval workflow, §17.3, is explicitly not designed yet).
CREATE TABLE staff (
    uuid          TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Mirrors opentea's own session table exactly (internal/db/migrations/0002_users_auth.sql
-- + 0003's hashed-token follow-up) -- only the hash is ever stored, same
-- reasoning: a DB leak shouldn't hand out directly-usable sessions.
CREATE TABLE session (
    token_hash  TEXT PRIMARY KEY,
    staff_uuid  TEXT NOT NULL REFERENCES staff(uuid) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    expires_at  TEXT NOT NULL
);
CREATE INDEX idx_session_expires ON session(expires_at);
CREATE INDEX idx_session_staff ON session(staff_uuid);

-- One row per target TEA server this deployment publishes to (§10.5,
-- multi-target). bearer_token is stored as plaintext -- unlike opentea's
-- own publisher_credential (which only ever stores a verifier-side hash,
-- since it only ever needs to check a presented token), this app must
-- present the actual usable credential on every outbound call to the
-- target, so it must be recoverable. No encryption-at-rest is designed for
-- this phase -- a known, deliberate v1 limitation (see TODO.md), same
-- honesty standard as ephemeral-only signing (§17.5): revisit if/when a
-- real deployment needs it.
CREATE TABLE target (
    uuid         TEXT PRIMARY KEY,
    label        TEXT NOT NULL,
    base_url     TEXT NOT NULL, -- e.g. "https://tea.example.com/publisher/v1"
    bearer_token TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
