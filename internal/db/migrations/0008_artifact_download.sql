-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- TEA 1.0 (spec/openapi.yaml) artifact content/signature download endpoints.
-- Content already has a way to locate its backing blob: SetArtifactFormatFile
-- inserts a SHA-256 row into the polymorphic checksum table (owner_type =
-- 'ARTIFACT_FORMAT') independent of the url column, so no new schema is
-- needed there. Signatures have never had local storage at all -- only the
-- external signature_url pointer -- so local signature hosting (this
-- session's own decision, not upstream-mandated) needs a new column.
-- A single direct column, not a checksum-table row, since a signature has
-- no checksums[] array of its own in the wire schema ("not the detached
-- signature") -- it's one opaque blob per format, not a multi-algorithm
-- checksum set.
ALTER TABLE artifact_format ADD COLUMN signature_sha256 TEXT;
