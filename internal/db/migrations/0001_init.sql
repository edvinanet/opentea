-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

PRAGMA foreign_keys = ON;

CREATE TABLE product (
    uuid        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE component (
    uuid        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE product_release (
    uuid          TEXT PRIMARY KEY,
    product_uuid  TEXT NOT NULL REFERENCES product(uuid) ON DELETE CASCADE,
    product_name  TEXT,
    version       TEXT NOT NULL,
    created_date  TEXT NOT NULL,
    release_date  TEXT,
    pre_release   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_product_release_product ON product_release(product_uuid);

CREATE TABLE component_release (
    uuid            TEXT PRIMARY KEY,
    component_uuid  TEXT NOT NULL REFERENCES component(uuid) ON DELETE CASCADE,
    component_name  TEXT,
    version         TEXT NOT NULL,
    created_date    TEXT NOT NULL,
    release_date    TEXT,
    pre_release     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_component_release_component ON component_release(component_uuid);

-- component-ref: productRelease.components[] = {uuid, release?}
CREATE TABLE product_release_component (
    product_release_uuid   TEXT NOT NULL REFERENCES product_release(uuid) ON DELETE CASCADE,
    component_uuid          TEXT NOT NULL REFERENCES component(uuid) ON DELETE CASCADE,
    component_release_uuid  TEXT REFERENCES component_release(uuid) ON DELETE SET NULL,
    PRIMARY KEY (product_release_uuid, component_uuid)
);

CREATE TABLE release_distribution (
    distribution_id          TEXT PRIMARY KEY,
    component_release_uuid   TEXT NOT NULL REFERENCES component_release(uuid) ON DELETE CASCADE,
    description               TEXT,
    url                        TEXT,
    signature_url               TEXT
);
CREATE INDEX idx_distribution_release ON release_distribution(component_release_uuid);

-- polymorphic identifiers: PRODUCT | PRODUCT_RELEASE | COMPONENT | COMPONENT_RELEASE | DISTRIBUTION
CREATE TABLE identifier (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_type  TEXT NOT NULL CHECK (owner_type IN ('PRODUCT','PRODUCT_RELEASE','COMPONENT','COMPONENT_RELEASE','DISTRIBUTION')),
    owner_uuid  TEXT NOT NULL,
    id_type     TEXT NOT NULL CHECK (id_type IN ('CPE','TEI','PURL','COMPLIANCE_DOCUMENT')),
    id_value    TEXT NOT NULL
);
CREATE INDEX idx_identifier_owner ON identifier(owner_type, owner_uuid);
CREATE INDEX idx_identifier_lookup ON identifier(owner_type, id_type, id_value);

CREATE TABLE artifact (
    uuid          TEXT NOT NULL,
    version       INTEGER NOT NULL DEFAULT 1,
    name          TEXT,
    type          TEXT NOT NULL CHECK (type IN ('ATTESTATION','BOM','BUILD_META','CERTIFICATION','FORMULATION','LICENSE','RELEASE_NOTES','SECURITY_TXT','THREAT_MODEL','VULNERABILITIES','OTHER')),
    created_date  TEXT,
    PRIMARY KEY (uuid, version)
);

-- artifact-format surrogate key (spec object has no id of its own)
CREATE TABLE artifact_format (
    id                TEXT PRIMARY KEY,
    artifact_uuid     TEXT NOT NULL,
    artifact_version  INTEGER NOT NULL,
    media_type        TEXT NOT NULL,
    description       TEXT,
    url               TEXT,
    signature_url     TEXT,
    FOREIGN KEY (artifact_uuid, artifact_version) REFERENCES artifact(uuid, version) ON DELETE CASCADE
);
CREATE INDEX idx_artifact_format_artifact ON artifact_format(artifact_uuid, artifact_version);

-- polymorphic checksums: DISTRIBUTION | ARTIFACT_FORMAT
CREATE TABLE checksum (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_type  TEXT NOT NULL CHECK (owner_type IN ('DISTRIBUTION','ARTIFACT_FORMAT')),
    owner_id    TEXT NOT NULL,
    alg_type    TEXT NOT NULL CHECK (alg_type IN ('MD5','SHA-1','SHA-256','SHA-384','SHA-512','SHA3-256','SHA3-384','SHA3-512','BLAKE2b-256','BLAKE2b-384','BLAKE2b-512','BLAKE3')),
    alg_value   TEXT NOT NULL
);
CREATE INDEX idx_checksum_owner ON checksum(owner_type, owner_id);

-- artifact.distributionIds[]
CREATE TABLE artifact_distribution (
    artifact_uuid     TEXT NOT NULL,
    artifact_version  INTEGER NOT NULL,
    distribution_id   TEXT NOT NULL REFERENCES release_distribution(distribution_id) ON DELETE CASCADE,
    PRIMARY KEY (artifact_uuid, artifact_version, distribution_id),
    FOREIGN KEY (artifact_uuid, artifact_version) REFERENCES artifact(uuid, version) ON DELETE CASCADE
);

CREATE TABLE collection (
    uuid          TEXT NOT NULL,   -- == owning product_release.uuid or component_release.uuid
    version       INTEGER NOT NULL,
    date          TEXT NOT NULL,
    belongs_to    TEXT NOT NULL CHECK (belongs_to IN ('COMPONENT_RELEASE','PRODUCT_RELEASE')),
    update_reason_type     TEXT CHECK (update_reason_type IN ('INITIAL_RELEASE','VEX_UPDATED','ARTIFACT_UPDATED','ARTIFACT_ADDED','ARTIFACT_REMOVED')),
    update_reason_comment  TEXT,
    PRIMARY KEY (uuid, version)
);
CREATE INDEX idx_collection_uuid ON collection(uuid);

-- many-to-many: a collection references N artifacts; an artifact may be reused by many collections
CREATE TABLE collection_artifact (
    collection_uuid     TEXT NOT NULL,
    collection_version  INTEGER NOT NULL,
    artifact_uuid       TEXT NOT NULL,
    artifact_version    INTEGER NOT NULL,
    PRIMARY KEY (collection_uuid, collection_version, artifact_uuid, artifact_version),
    FOREIGN KEY (collection_uuid, collection_version) REFERENCES collection(uuid, version) ON DELETE CASCADE,
    FOREIGN KEY (artifact_uuid, artifact_version) REFERENCES artifact(uuid, version) ON DELETE RESTRICT
);

-- CLE: owner_type in PRODUCT | PRODUCT_RELEASE | COMPONENT | COMPONENT_RELEASE
CREATE TABLE cle_event (
    owner_type    TEXT NOT NULL CHECK (owner_type IN ('PRODUCT','PRODUCT_RELEASE','COMPONENT','COMPONENT_RELEASE')),
    owner_uuid    TEXT NOT NULL,
    id            INTEGER NOT NULL,
    type          TEXT NOT NULL CHECK (type IN ('released','endOfDevelopment','endOfSupport','endOfLife','endOfDistribution','endOfMarketing','supersededBy','componentRenamed','withdrawn')),
    effective     TEXT NOT NULL,
    published     TEXT NOT NULL,
    version       TEXT,
    support_id    TEXT,
    license       TEXT,
    superseded_by_version  TEXT,
    event_id_ref  INTEGER,
    reason        TEXT,
    description   TEXT,
    PRIMARY KEY (owner_type, owner_uuid, id)
);

CREATE TABLE cle_event_version (
    owner_type     TEXT NOT NULL,
    owner_uuid     TEXT NOT NULL,
    event_id       INTEGER NOT NULL,
    version        TEXT,
    version_range  TEXT,
    FOREIGN KEY (owner_type, owner_uuid, event_id) REFERENCES cle_event(owner_type, owner_uuid, id) ON DELETE CASCADE
);

CREATE TABLE cle_event_identifier (
    owner_type  TEXT NOT NULL,
    owner_uuid  TEXT NOT NULL,
    event_id    INTEGER NOT NULL,
    id_type     TEXT NOT NULL CHECK (id_type IN ('CPE','TEI','PURL','COMPLIANCE_DOCUMENT')),
    id_value    TEXT NOT NULL,
    FOREIGN KEY (owner_type, owner_uuid, event_id) REFERENCES cle_event(owner_type, owner_uuid, id) ON DELETE CASCADE
);

CREATE TABLE cle_event_reference (
    owner_type  TEXT NOT NULL,
    owner_uuid  TEXT NOT NULL,
    event_id    INTEGER NOT NULL,
    uri         TEXT NOT NULL,
    FOREIGN KEY (owner_type, owner_uuid, event_id) REFERENCES cle_event(owner_type, owner_uuid, id) ON DELETE CASCADE
);

CREATE TABLE cle_support_definition (
    owner_type   TEXT NOT NULL CHECK (owner_type IN ('PRODUCT','PRODUCT_RELEASE','COMPONENT','COMPONENT_RELEASE')),
    owner_uuid   TEXT NOT NULL,
    id           TEXT NOT NULL,
    description  TEXT NOT NULL,
    url          TEXT,
    PRIMARY KEY (owner_type, owner_uuid, id)
);

-- blob storage bookkeeping (not part of the spec model; used by admin+files handler)
CREATE TABLE blob (
    sha256      TEXT PRIMARY KEY,
    size_bytes  INTEGER NOT NULL,
    media_type  TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
