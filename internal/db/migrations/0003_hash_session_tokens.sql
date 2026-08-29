-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Session tokens were stored raw; api_token already stored only a hash for
-- the same reason (a DB leak shouldn't hand out directly-usable tokens).
-- Renaming (not just changing application code) matches api_token's
-- token_hash column name, so the schema itself makes clear this column
-- never holds a usable token. Existing rows are carried over unchanged --
-- their now-stale raw values will never match a hashed lookup again, which
-- simply invalidates any session that existed before this migration
-- (sessions are short-lived and self-service via login, so that's fine).
ALTER TABLE session RENAME COLUMN token TO token_hash;
