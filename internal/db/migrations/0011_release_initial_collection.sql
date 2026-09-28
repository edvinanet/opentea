-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- TEA 1.0's collection schema (spec/openapi.yaml): "Every component release
-- and product release has a collection. If no artifacts have been published
-- when the release first becomes retrievable, the server serves version 1
-- with an empty artifacts list and updateReason.type: INITIAL_RELEASE." --
-- GET productRelease/componentRelease responses require latestCollection
-- unconditionally, which this server could not previously guarantee for a
-- release created with no artifacts published yet
-- (docs/security-review-260923.md findings #5 and #7). CreateProductRelease/
-- CreateComponentRelease now create this version-1 collection atomically
-- going forward; this migration backfills it for every release that
-- predates that fix. The backfilled collection's date is the release's own
-- created_date, the most meaningful timestamp available for data that
-- predates this column's real creation moment.
INSERT INTO collection (uuid, version, date, belongs_to, update_reason_type)
SELECT pr.uuid, 1, pr.created_date, 'PRODUCT_RELEASE', 'INITIAL_RELEASE'
FROM product_release pr
WHERE NOT EXISTS (SELECT 1 FROM collection c WHERE c.uuid = pr.uuid);

INSERT INTO collection (uuid, version, date, belongs_to, update_reason_type)
SELECT cr.uuid, 1, cr.created_date, 'COMPONENT_RELEASE', 'INITIAL_RELEASE'
FROM component_release cr
WHERE NOT EXISTS (SELECT 1 FROM collection c WHERE c.uuid = cr.uuid);

-- Bump unconditionally (harmless even if nothing above actually inserted a
-- row) so any collection-list ETag a client cached before this migration
-- invalidates rather than appearing falsely still fresh.
INSERT INTO dataset_watermark (resource_family, watermark) VALUES ('collections', 1)
ON CONFLICT (resource_family) DO UPDATE SET watermark = watermark + 1;
