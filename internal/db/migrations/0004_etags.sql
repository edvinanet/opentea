-- Support for HTTP ETag / conditional-request handling on /tea/v1 GET
-- endpoints: a monotonic revision per mutable row (not updated_at -- a
-- timestamp can collide or lose precision, a counter can't), plus two
-- lookaside tables for entities that have no single owning row to carry a
-- revision column. product/component/collection are deliberately excluded:
-- they have no update path in this codebase today (collection is
-- insert-only; product/component have no Update* at all), so their
-- representations are provably immutable and their ETags are built from
-- identity alone (type+uuid[+version]), no revision needed.
ALTER TABLE artifact ADD COLUMN revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE product_release ADD COLUMN revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE component_release ADD COLUMN revision BIGINT NOT NULL DEFAULT 1;

-- CLE data (product/productRelease/component/componentRelease lifecycle
-- events) has no single owning row -- it's derived by querying cle_event/
-- cle_support_definition filtered by (owner_type, owner_uuid). One row per
-- owner is created on that owner's first CLE write.
CREATE TABLE cle_revision (
    owner_type TEXT NOT NULL,
    owner_uuid TEXT NOT NULL,
    revision   BIGINT NOT NULL DEFAULT 1,
    PRIMARY KEY (owner_type, owner_uuid)
);

-- Dataset watermarks back list-endpoint ETags (GET /products, /components,
-- etc.): one global counter per resource family, bumped on any write that
-- could change that family's list membership or listed content. Global
-- rather than tenant-scoped since this project has no tenant concept yet
-- (see TODO.md's "Multitenant" entry) -- a row is created on that family's
-- first bump.
CREATE TABLE dataset_watermark (
    resource_family TEXT PRIMARY KEY,
    watermark BIGINT NOT NULL DEFAULT 0
);
