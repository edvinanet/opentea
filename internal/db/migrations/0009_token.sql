-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- TEA 1.0 (spec/openapi.yaml's /token, upstream's auth/readme.md) client_credentials
-- token exchange. api_token (0002_users_auth.sql) was a single opaque string presented
-- directly as the /tea/v1 bearer token -- exactly the pattern auth/readme.md forbids
-- ("a server shall not accept an API key directly on the resource endpoints"). Replaced
-- by api_key, a real identifier+secret pair ("An API key consists of two parts issued
-- together... an identifier and a secret") presented via HTTP Basic to /token, and
-- access_token, the short-lived token that exchange returns -- the only thing /tea/v1
-- accepts as a bearer credential from here on. access_token mirrors session's own shape/
-- indices (0002_users_auth.sql) exactly, since both are opaque, hash-stored,
-- TTL-expiring, user-scoped tokens; kept as a separate table rather than reusing session
-- itself since a browser session cookie and an OAuth access token are different
-- credentials with potentially different future lifecycles/revocation semantics.
DROP TABLE api_token;

CREATE TABLE api_key (
    user_uuid   TEXT PRIMARY KEY REFERENCES user(uuid) ON DELETE CASCADE,
    key_id      TEXT NOT NULL UNIQUE,
    secret_hash TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE access_token (
    token_hash  TEXT PRIMARY KEY,
    user_uuid   TEXT NOT NULL REFERENCES user(uuid) ON DELETE CASCADE,
    expires_at  TEXT NOT NULL
);
CREATE INDEX idx_access_token_expires ON access_token(expires_at);
CREATE INDEX idx_access_token_user ON access_token(user_uuid);
