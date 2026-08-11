-- Single-tenant authorization for /tea/v1, per
-- ~/TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md (v0.1 draft) Sec
-- 10-18, 22, 27's "Implementation Profile for OpenTEA". This deployment is
-- one implicit organization -- no organization/tenant table, no audience or
-- principal groups (spec Sec 10.1/10.2 explicitly allows omitting groups
-- absent corresponding admin functionality; see TODO.md's "Multitenant"
-- entry, on hold). Subjects are: everyone (anonymous), authenticated (any
-- valid bearer token), and individual principal (user.uuid).
--
-- Release groups are product-release-only in Phase 1, not polymorphic
-- across product/component releases like collection.belongs_to -- extending
-- to component releases later is a non-breaking, additive change (a new
-- CHECK value plus a parallel member table), not a redesign.
--
-- Resource scopes below cover the spec Sec 27 minimum five (all-products,
-- product-group, product, release-group, product-release) plus two
-- extensions -- specific-collection and specific-artifact -- needed because
-- this deployment enforces collection/artifact capabilities from day one,
-- and a template-only grant with no way to scope "just this one collection"
-- would be a real gap.

-- Reusable, versioned capability-rule sets (spec Sec 13). A template MUST
-- NOT itself name a subject or resource -- that's supplied by whatever
-- entitlement references it.
CREATE TABLE template (
    uuid            TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    description     TEXT,
    active_revision INTEGER,  -- NULL until a revision is explicitly activated; FK-like reference to template_revision(template_uuid, revision), not schema-enforced since template_revision doesn't exist until after this row does
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- One immutable row per template edit -- editing a template creates a new
-- revision, it never mutates rules of an existing one (spec: "Changing a
-- template MUST create a new revision; activation of a new revision MUST be
-- explicit/audited"). revision is a per-template sequence (1, 2, 3, ...).
CREATE TABLE template_revision (
    template_uuid TEXT NOT NULL REFERENCES template(uuid) ON DELETE CASCADE,
    revision      INTEGER NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    created_by    TEXT REFERENCES user(uuid) ON DELETE SET NULL,
    comment       TEXT,
    PRIMARY KEY (template_uuid, revision)
);

-- One capability rule per (template_uuid, revision, capability[, artifact_type]).
-- artifact_type is NULL for capabilities that aren't artifact-scoped;
-- non-NULL rows constrain the rule to one controlled artifact
-- classification (spec Sec 17.1). Reuses artifact.type's existing enum
-- verbatim rather than inventing a separate SBOM/VEX/VDR vocabulary the
-- data doesn't actually support -- known limitation: this can't distinguish
-- a VEX document from a generic SBOM (both are typically type=BOM), so the
-- spec's example "SBOM and VEX Access" vs "VEX-only" template split can't
-- be expressed precisely yet. A capability with no matching row here means
-- "this template doesn't speak to this capability" (the "inherit" state at
-- evaluation time), not an implicit deny.
CREATE TABLE template_capability_rule (
    template_uuid  TEXT NOT NULL,
    revision       INTEGER NOT NULL,
    capability     TEXT NOT NULL CHECK (capability IN (
                       'product.discover','product.read',
                       'release.discover','release.read',
                       'lifecycle.current.read','lifecycle.history.read',
                       'collection.discover','collection.read','collection.download',
                       'artifact.discover','artifact.metadata.read','artifact.download'
                   )),
    artifact_type  TEXT CHECK (artifact_type IN (
                       'ATTESTATION','BOM','BUILD_META','CERTIFICATION','FORMULATION',
                       'LICENSE','RELEASE_NOTES','SECURITY_TXT','THREAT_MODEL',
                       'VULNERABILITIES','OTHER'
                   )),
    decision       TEXT NOT NULL CHECK (decision IN ('allow','deny')),
    FOREIGN KEY (template_uuid, revision) REFERENCES template_revision(template_uuid, revision) ON DELETE CASCADE,
    -- one rule per (capability, artifact_type) within a revision; SQLite
    -- treats NULLs as distinct for UNIQUE, which is exactly what's wanted:
    -- a capability can have both a type-agnostic default rule AND per-type
    -- overrides within the same revision.
    UNIQUE (template_uuid, revision, capability, artifact_type)
);
CREATE INDEX idx_template_capability_rule_lookup ON template_capability_rule(template_uuid, revision);

-- Product groups: a named, admin-managed set of products, usable as an
-- entitlement resource scope. Flat -- no nesting, matching the "no template
-- inheritance in the minimum profile" simplicity bar applied consistently.
CREATE TABLE product_group (
    uuid        TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE product_group_member (
    product_group_uuid TEXT NOT NULL REFERENCES product_group(uuid) ON DELETE CASCADE,
    product_uuid        TEXT NOT NULL REFERENCES product(uuid) ON DELETE CASCADE,
    PRIMARY KEY (product_group_uuid, product_uuid)
);
CREATE INDEX idx_product_group_member_product ON product_group_member(product_uuid);

-- Release groups: product-release-only in Phase 1 (see file header).
CREATE TABLE release_group (
    uuid        TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE release_group_member (
    release_group_uuid   TEXT NOT NULL REFERENCES release_group(uuid) ON DELETE CASCADE,
    product_release_uuid TEXT NOT NULL REFERENCES product_release(uuid) ON DELETE CASCADE,
    PRIMARY KEY (release_group_uuid, product_release_uuid)
);
CREATE INDEX idx_release_group_member_release ON release_group_member(product_release_uuid);

-- Entitlements (spec Sec 14.1): the assignment of subject + resource scope
-- + template, with status/validity/provenance. One row per grant.
--
-- subject_type/subject_id: 'everyone' and 'authenticated' have subject_id
-- NULL (there is only one of each); 'principal' has subject_id = user.uuid.
--
-- resource_type/resource_id together select the scope:
--   all_products    -> resource_id NULL
--   product_group   -> resource_id = product_group.uuid
--   product         -> resource_id = product.uuid
--   release_group   -> resource_id = release_group.uuid
--   product_release -> resource_id = product_release.uuid
--   collection      -> resource_id = collection.uuid (all versions of that collection identity)
--   artifact        -> resource_id = artifact.uuid (all versions of that artifact identity)
--   component         -> resource_id = component.uuid
--   component_release -> resource_id = component_release.uuid
--
-- component/component_release exist because the TEA wire model has
-- Component/ComponentRelease as top-level resources independent of
-- Product/ProductRelease (reachable directly via /tea/v1/component/...,
-- not only by navigating a product's components[]). Phase 1 gives them a
-- deliberately simpler scope model than the product side: no
-- component-group or component-release-group scope, just all_products plus
-- a direct component/component_release grant -- symmetric to how release
-- groups are product-release-only (see this file's header). If
-- component-side grouping is ever needed, add component_group/
-- component_release_group tables mirroring product_group/release_group.
--
-- Ad hoc inline capability rules on an entitlement (spec Sec 14.2's
-- "overrides") are deferred -- Phase 1 entitlements reference exactly one
-- template revision, no inline rule list.
CREATE TABLE entitlement (
    uuid                TEXT PRIMARY KEY,
    subject_type        TEXT NOT NULL CHECK (subject_type IN ('everyone','authenticated','principal')),
    subject_id          TEXT REFERENCES user(uuid) ON DELETE CASCADE,
    template_uuid       TEXT NOT NULL REFERENCES template(uuid) ON DELETE RESTRICT,
    template_revision   INTEGER NOT NULL,
    resource_type       TEXT NOT NULL CHECK (resource_type IN (
                            'all_products','product_group','product',
                            'release_group','product_release',
                            'collection','artifact',
                            'component','component_release'
                         )),
    resource_id         TEXT,
    status              TEXT NOT NULL CHECK (status IN ('active','suspended','revoked')) DEFAULT 'active',
    valid_from          TEXT,
    valid_until         TEXT,
    granting_authority  TEXT NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    created_by          TEXT REFERENCES user(uuid) ON DELETE SET NULL,
    revision            INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (template_uuid, template_revision) REFERENCES template_revision(template_uuid, revision) ON DELETE RESTRICT,
    CHECK ((subject_type = 'principal') = (subject_id IS NOT NULL)),
    CHECK ((resource_type = 'all_products') = (resource_id IS NULL))
);
CREATE INDEX idx_entitlement_subject ON entitlement(subject_type, subject_id);
CREATE INDEX idx_entitlement_resource ON entitlement(resource_type, resource_id);
CREATE INDEX idx_entitlement_status ON entitlement(status) WHERE status = 'active';

-- Administrative audit (spec Sec 22.5): template/entitlement/group
-- lifecycle changes. One append-only row per mutation, written in the same
-- transaction as the mutation it records (see internal/admin/audit.go) so
-- "mutation and audit record commit atomically, or both fail" holds by
-- construction. Deliberately separate from per-request authorization
-- decision logging (spec Sec 22.4), which is structured slog output, not a
-- DB table -- see internal/authz's decision-logging call sites for why
-- (very different volume/retention/query needs).
CREATE TABLE admin_audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    actor_uuid      TEXT REFERENCES user(uuid) ON DELETE SET NULL,
    request_id      TEXT,
    operation       TEXT NOT NULL,
    target_type     TEXT NOT NULL CHECK (target_type IN ('template','entitlement','product_group','release_group')),
    target_id       TEXT NOT NULL,
    previous_state  TEXT,  -- JSON snapshot, NULL for creates
    resulting_state TEXT NOT NULL,  -- JSON snapshot
    reason          TEXT
);
CREATE INDEX idx_admin_audit_log_target ON admin_audit_log(target_type, target_id);
CREATE INDEX idx_admin_audit_log_at ON admin_audit_log(at);

-- Bootstrap seed: a permissive built-in "Public" template + an
-- everyone/all_products entitlement referencing it, so existing (and
-- freshly created) deployments keep today's "anyone can read anything"
-- behavior the instant this migration runs, rather than flipping to
-- default-deny with no admin having configured anything yet. This is a
-- real, visible, revocable row -- shows up in GET /admin/v1/entitlements
-- like any other -- not a special-cased bypass in the policy engine, which
-- keeps Decide's default-deny semantics honest.
INSERT INTO template (uuid, name, description, active_revision) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Public (bootstrap)',
    'Built-in permissive template seeded by migration 0005 so existing deployments keep pre-authorization read access until an admin narrows it. Grants every consumer capability.',
    1
);
INSERT INTO template_revision (template_uuid, revision, comment) VALUES (
    '00000000-0000-0000-0000-000000000001', 1, 'Initial bootstrap revision: allow every consumer capability, unconstrained by artifact type.'
);
INSERT INTO template_capability_rule (template_uuid, revision, capability, decision) VALUES
    ('00000000-0000-0000-0000-000000000001', 1, 'product.discover', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'product.read', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'release.discover', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'release.read', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'lifecycle.current.read', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'lifecycle.history.read', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'collection.discover', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'collection.read', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'collection.download', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'artifact.discover', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'artifact.metadata.read', 'allow'),
    ('00000000-0000-0000-0000-000000000001', 1, 'artifact.download', 'allow');
INSERT INTO entitlement (uuid, subject_type, template_uuid, template_revision, resource_type, granting_authority) VALUES (
    '00000000-0000-0000-0000-000000000002', 'everyone', '00000000-0000-0000-0000-000000000001', 1, 'all_products', 'system (migration 0005 bootstrap)'
);

-- Backs principal-scoped /tea/v1 ETags (internal/api/cachepolicy.go):
-- bumped by every entitlement write so an authenticated caller's cached
-- representation invalidates the moment ANY entitlement changes, even one
-- that didn't previously apply to them (see
-- Repo.GetPrincipalEntitlementWatermark). The bootstrap INSERT above is
-- itself the first bump.
INSERT INTO dataset_watermark (resource_family, watermark) VALUES ('entitlements', 1);
