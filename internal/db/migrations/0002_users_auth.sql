CREATE TABLE user (
    uuid          TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin', 'consumer')),
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE session (
    token       TEXT PRIMARY KEY,
    user_uuid   TEXT NOT NULL REFERENCES user(uuid) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    expires_at  TEXT NOT NULL
);
CREATE INDEX idx_session_expires ON session(expires_at);
CREATE INDEX idx_session_user ON session(user_uuid);

-- One active API token per user; regenerating replaces it. Only the hash is
-- stored (like a password) so a DB leak doesn't hand out usable tokens.
CREATE TABLE api_token (
    user_uuid   TEXT PRIMARY KEY REFERENCES user(uuid) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
