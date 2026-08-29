-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Evidence-bundle data model, Phase 1 of oej's TEA Trust Architecture
-- overlay (github.com/oej/tea-trust-architecture), per
-- 08-evidence-bundle.md and its evidence-bundle-schema.json. See TODO.md's
-- "Trust architecture overlay" entry and the phased implementation plan.
--
-- This is a deliberately SEPARATE subsystem from 0005_authz.sql's
-- template/entitlement authorization model: authz answers "can this caller
-- read this resource" for /tea/v1 consumers; trust answers "is this
-- evidence of an artifact/collection's origin and integrity valid" for
-- publishers. Nothing here extends template_capability_rule's capability
-- vocabulary or entitlement's resource-scope model, and nothing in authz
-- references these tables.
--
-- Phase 1 models the full evidence-bundle shape (signature + certificate)
-- but not yet the timestamp/transparency-log evidence that a schema-
-- conformant bundle requires (evidence-bundle-schema.json's "timestamps"
-- and "transparency" arrays are both minItems:1 and required) -- those
-- land in Phase 2/3. A bundle's status stays 'draft' (not spec-conformant)
-- until both are populated; MarkEvidenceBundleComplete enforces this
-- application-side.
--
-- Also unrelated to internal/bundle (the product import/export ZIP
-- format): that's product-metadata portability, this is cryptographic
-- evidence of origin/integrity. Deliberately different vocabulary
-- ("evidence bundle" vs "bundle") to keep the two from ever being confused
-- in code or conversation.

-- Global fingerprint-reuse ledger (01-tea-trust-architecture.md Sec 9:
-- "prevent reuse of known fingerprints; reject reuse attempts"). One row
-- per fingerprint ever used to produce a signature this server created or
-- ingested. The PRIMARY KEY is the enforcement mechanism:
-- repo.CreateEvidenceBundle inserts here before creating the bundle row,
-- and a UNIQUE-constraint failure surfaces as repo.ErrFingerprintReused.
CREATE TABLE used_fingerprint (
    fingerprint          TEXT PRIMARY KEY,
    trust_domain         TEXT NOT NULL,
    evidence_bundle_uuid TEXT,  -- set once that bundle row exists; not FK-enforced at insert time since the bundle doesn't exist yet at the moment this row is first written
    created_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- One evidence bundle: signature + certificate + a status tracking whether
-- Phase 2/3's timestamp/transparency evidence is present yet.
-- owner_type/owner_uuid/owner_version is the polymorphic link to the
-- signed object, mirroring the checksum/identifier tables' owner_type/
-- owner_id pattern from 0001_init.sql -- not FK-enforced for the same
-- reason (SQLite has no conditional FK across a type discriminator), just
-- indexed. Phase 1 scopes owner_type to ARTIFACT and COLLECTION (both are
-- (uuid, version)-keyed, hence owner_version is NOT NULL); DISCOVERY_DOCUMENT
-- and CLE_DOCUMENT are added in a later, additive migration when a later
-- phase needs them.
CREATE TABLE evidence_bundle (
    uuid                    TEXT PRIMARY KEY,
    bundle_version          TEXT NOT NULL DEFAULT '1.0',
    owner_type              TEXT NOT NULL CHECK (owner_type IN ('ARTIFACT','COLLECTION')),
    owner_uuid              TEXT NOT NULL,
    owner_version           INTEGER NOT NULL,
    object_type             TEXT NOT NULL,  -- schema's object.objectType, e.g. "artifact" / "tea-collection"
    object_media_type       TEXT,
    object_location         TEXT,
    object_digest_value     TEXT NOT NULL,  -- SHA-256 hex; algorithm is always sha-256 (spec 08 Sec 9.1), not a separate column
    signature_format        TEXT NOT NULL CHECK (signature_format IN ('cms-detached','dsse-envelope','jws-detached','cose-sign1')),
    signature_value         TEXT NOT NULL,  -- base64
    signature_digest_value  TEXT,           -- optional per schema; SHA-256 hex of the signature bytes
    certificate_format      TEXT NOT NULL CHECK (certificate_format IN ('x509-pem','x509-der')),
    certificate_value       TEXT NOT NULL,
    certificate_fingerprint TEXT NOT NULL REFERENCES used_fingerprint(fingerprint),
    status                  TEXT NOT NULL CHECK (status IN ('draft','complete')) DEFAULT 'draft',
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    created_by              TEXT REFERENCES user(uuid) ON DELETE SET NULL
);
CREATE INDEX idx_evidence_bundle_owner ON evidence_bundle(owner_type, owner_uuid, owner_version);
CREATE INDEX idx_evidence_bundle_fingerprint ON evidence_bundle(certificate_fingerprint);

-- Timestamp evidence (spec 08 Sec 7). Empty for every Phase 1 bundle --
-- table exists now so Phase 2's RFC 3161 client only needs an INSERT, not a
-- migration. The schema's minItems:1 is enforced at the status transition
-- (repo.MarkEvidenceBundleComplete), not here.
CREATE TABLE evidence_bundle_timestamp (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    evidence_bundle_uuid   TEXT NOT NULL REFERENCES evidence_bundle(uuid) ON DELETE CASCADE,
    format                 TEXT NOT NULL CHECK (format = 'rfc3161'),
    tsa_subject_dn         TEXT,
    tsa_pubkey_fingerprint TEXT,
    tsa_uri                TEXT,
    token                  TEXT NOT NULL,  -- base64-encoded RFC 3161 TimeStampToken
    message_imprint_value  TEXT,           -- SHA-256 hex; should equal the bundle's signature_digest_value
    created_at             TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_evidence_bundle_timestamp_bundle ON evidence_bundle_timestamp(evidence_bundle_uuid);

-- Transparency-log evidence (spec 08 Sec 8). Empty for every Phase 1
-- bundle. verification_json stores the schema's transparencyVerification
-- object as opaque JSON text: Rekor's inclusion-proof/signed-tree-head
-- shape and Sigsum's witness-signature shape differ entirely, so modeling
-- every system's fields as columns would be premature -- mirrors
-- admin_audit_log's previous_state/resulting_state JSON-text-column
-- precedent (0005_authz.sql).
CREATE TABLE evidence_bundle_transparency (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    evidence_bundle_uuid  TEXT NOT NULL REFERENCES evidence_bundle(uuid) ON DELETE CASCADE,
    system                TEXT NOT NULL CHECK (system IN ('rekor','sigsum','scitt')),
    evidence_type         TEXT NOT NULL,
    binding_type          TEXT NOT NULL CHECK (binding_type IN ('object','signature','timestamped-signature','dsse-envelope')),
    binding_digest_value  TEXT NOT NULL,
    verification_json     TEXT NOT NULL,
    created_at            TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_evidence_bundle_transparency_bundle ON evidence_bundle_transparency(evidence_bundle_uuid);

-- External-reference form of an evidence bundle (spec 08 Sec 10): the
-- bundle lives elsewhere, only a URI + digest over its canonical JSON is
-- stored here. Mutually exclusive with an evidence_bundle row for the same
-- owner in application logic (not DB-enforced -- same non-enforcement
-- precedent as the collection-evidence-non-reuse rule, both deferred to
-- the Phase 4 publish/commit workflow).
CREATE TABLE evidence_bundle_ref (
    owner_type    TEXT NOT NULL CHECK (owner_type IN ('ARTIFACT','COLLECTION')),
    owner_uuid    TEXT NOT NULL,
    owner_version INTEGER NOT NULL,
    uri           TEXT NOT NULL,
    digest_value  TEXT NOT NULL,  -- SHA-256 hex of RFC 8785 canonical JSON (spec 08 Sec 10.3)
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (owner_type, owner_uuid, owner_version)
);

-- Widen admin_audit_log.target_type's CHECK (0005_authz.sql) to also
-- accept 'evidence_bundle': internal/admin/evidencebundle.go reuses the
-- same generic admin-mutation audit-log plumbing as template/entitlement/
-- group handlers (auditWrite), which is fine -- admin_audit_log is
-- generic infrastructure, not authz-specific -- but its target_type CHECK
-- was scoped to authz's own target kinds only. SQLite has no
-- ALTER TABLE ... ALTER CHECK, so the table is rebuilt: rename, recreate
-- with the widened CHECK, copy rows back by explicit column list
-- (preserves existing ids/autoincrement continuation), drop the old table,
-- recreate its indexes.
ALTER TABLE admin_audit_log RENAME TO admin_audit_log_pre0006;

CREATE TABLE admin_audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    actor_uuid      TEXT REFERENCES user(uuid) ON DELETE SET NULL,
    request_id      TEXT,
    operation       TEXT NOT NULL,
    target_type     TEXT NOT NULL CHECK (target_type IN ('template','entitlement','product_group','release_group','evidence_bundle')),
    target_id       TEXT NOT NULL,
    previous_state  TEXT,
    resulting_state TEXT NOT NULL,
    reason          TEXT
);

INSERT INTO admin_audit_log (id, at, actor_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state, reason)
SELECT id, at, actor_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state, reason
FROM admin_audit_log_pre0006;

DROP TABLE admin_audit_log_pre0006;

CREATE INDEX idx_admin_audit_log_target ON admin_audit_log(target_type, target_id);
CREATE INDEX idx_admin_audit_log_at ON admin_audit_log(at);
