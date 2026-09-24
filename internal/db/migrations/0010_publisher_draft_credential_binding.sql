-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Binds a collection draft's maker/checker steps to the actual authenticated
-- publisher_credential that performed each one, not merely the caller-supplied
-- "actor" JSON string (0007_publisher.sql's drafted_by/approval_decided_by).
-- A holder of one full-scope credential could draft as one actor name and
-- approve as another -- the equality check never established two independent
-- credentials, only two arbitrary strings the same caller typed
-- (docs/security-review-260923.md finding #4). Nullable: a draft already open
-- when this migration runs has no recorded drafting credential and falls back
-- to the pre-existing actor-string check alone until it's re-drafted.
ALTER TABLE collection_draft ADD COLUMN drafted_by_credential_uuid TEXT REFERENCES publisher_credential(uuid) ON DELETE SET NULL;
ALTER TABLE collection_draft ADD COLUMN approval_decided_by_credential_uuid TEXT REFERENCES publisher_credential(uuid) ON DELETE SET NULL;
