# Product import/export bundle format

A standardized zip format for exporting a product's complete TEA data out of an opentea server
and importing it back in — used for backup, transferring ownership of a product between
organizations (e.g. company A sells a product to company B), and migrating between hosted TEA
providers. This is an **admin-only, `/admin/v1`-only** capability — it has nothing to do with
`/tea/v1` (the consumer read API) or any future publisher API, and it always requires the admin
role in both directions.

The canonical machine-readable schema is `internal/bundle/schema.json` (JSON Schema, draft
2020-12), embedded into the server binary and used to validate every bundle before it's
imported. This document is the human-readable companion to that schema.

## Endpoints

| Method | Path | Role | Notes |
|---|---|---|---|
| GET | `/admin/v1/products/{uuid}/export` | admin | Streams `application/zip` |
| POST | `/admin/v1/products/import` | admin | multipart, field `bundle`; returns a JSON summary |

## Zip layout

```
manifest.json
files/<sha256-hex>      # one entry per distinct blob referenced by any
                         # distribution or artifact-format checksum
```

`manifest.json` is the only required entry; `files/` is only as large as the product's actual
binary content (distributions, SBOM/VEX documents, etc.).

## Manifest shape

The manifest reuses `pkg/tea`'s wire types directly (no separate wire schema to keep in sync) —
`internal/bundle/manifest.go`:

| Field | Type | Notes |
|---|---|---|
| `formatVersion` | string | Currently always `"1.0"`. Identifies the *manifest's own* structure, not the TEA spec version — bump this if the bundle format itself changes in a breaking way. |
| `exportedAt` | date-time | When this bundle was generated. |
| `product` | `Product` + `cle` | The product this bundle is for. |
| `productReleases` | `[]ProductRelease` + `cle` | Every release of this product. |
| `components` | `[]Component` + `cle` | Every component referenced by any release (see scoping rule below). |
| `componentReleases` | `[]ComponentRelease` (with `distributions` embedded) + `cle` | Every *pinned* component release (see scoping rule below). |
| `collections` | `[]Collection` (with `artifacts` embedded) | Every collection version for every product release and pinned component release. |

CLE (lifecycle) data isn't part of the spec's own Product/ProductRelease/Component/
ComponentRelease response shapes (it's a separate endpoint per the spec), so the bundle adds it
as an additive `cle` property alongside the reused type rather than modifying `pkg/tea` itself.

There's no separate top-level `artifacts` list — collections already carry their artifacts
inline (matching how a real `GET .../collection/...` response looks), so importing walks
`collections[].artifacts` directly.

## Scoping rule: "full history", but only for what's actually part of the product

A bundle contains **full version history** — every collection version, every CLE event — not
just current state, since the point is a faithful, complete replica for backup/migration.

That said, scope is bounded to what's actually part of *this* product:

- Every component referenced by any release's `components[]` is included (as a bare
  `Component`), regardless of whether the reference pins a specific release.
- Only if the reference *pins* a specific release (`components[].release` is set) is that
  component release's full data — distributions, collection history, CLE — pulled in too.
- An unpinned component reference brings in only the bare `Component` — no release is actually
  part of this product's data, so no release history is exported for it.

This means bundle size scales with what the product actually depends on, not with every
unrelated release a shared component happens to have.

## Dedup / idempotency

Every shared entity is identity-keyed, and import is **idempotent**: importing the same bundle
twice, or two bundles that reference the same underlying entity, never creates a duplicate.

| Entity | Identity |
|---|---|
| Product, Component | `uuid` |
| Product release, Component release | `uuid` |
| Distribution | `distributionId` |
| Artifact | `(uuid, version)` — artifacts are versioned, so a bundle can introduce a new version of an artifact the target already has earlier versions of |
| Collection | `(uuid, version)` — same versioning behavior as artifacts |
| CLE event | the literal source `id` (not reassigned) — preserving it is what keeps `eventId` cross-references resolving correctly |
| File | SHA-256 content hash |

The rule is exists-or-create at the whole-entity level, not a field-by-field merge: if an
identity already exists in the target, it's left untouched and simply reused/linked — a second
import never overwrites data that's already there.

## Validation on import

1. **Schema validation first.** The raw `manifest.json` bytes are validated against
   `internal/bundle/schema.json` *before* being decoded into Go structs or written anywhere —
   a non-conformant bundle is rejected outright, not partially imported.
2. **File integrity.** Every `files/<sha256>` entry's actual content hash is recomputed and
   compared against the hash in its own filename before it's accepted — a bundle whose file
   bytes don't match its own claimed checksum is rejected as corrupt, before any distribution
   or artifact that references it is imported. (Only SHA-256 is independently verified this
   way, since it's the one algorithm this server itself ever computes — see `TODO.md`.)

## Standalone validation: `bundlecheck`

`cmd/bundlecheck` validates a bundle zip on disk without needing a running server, a database,
or admin authentication — it only reads the given file(s). Useful for checking a bundle before
sending it to another organization, or verifying one received from elsewhere, independent of
any particular opentea server instance.

```bash
bundlecheck product-<uuid>.zip
# product-<uuid>.zip: OK

bundlecheck product-a.zip product-b.zip
# product-a.zip: OK
# product-b.zip: INVALID
#   hash mismatch: files/2e01b874... (content doesn't match its claimed checksum)
```

It runs the same two checks as import's own validation (schema conformance, file-hash
integrity) plus a third: every SHA-256 checksum referenced by a distribution or artifact format
must have a matching `files/` entry actually present in the zip (a "missing file" error, not
just a hash mismatch on what's there). An extra `files/` entry not referenced by anything in the
manifest is reported as a note, not an error — harmless orphan data, not invalid.

**Exit codes are the contract for scripting/CI use**: `0` if every given bundle is valid, `1` if
any bundle is invalid *or* couldn't even be read (missing file, not a zip, etc.) — so
`bundlecheck *.zip && echo "all good"` (or an equivalent CI step) works as expected without
having to parse the human-readable output.

## File URLs

A bundle's manifest still carries each distribution/artifact-format's original `url` field (the
*source* server's URL), but import never reuses it directly — it wouldn't resolve against a
different destination server. Instead, import rebuilds each URL as
`<destination-server-root-url>/files/<sha256>` once the file itself has been stored locally.

## Authorization vs. authentication

Only authorization concepts should ever travel with a bundle; authentication (user accounts,
sessions, API tokens) never does — those are specific to each server instance and shouldn't
cross organizational boundaries in a transfer or migration. As of this format's `1.0` version
there's no per-product authorization model in opentea yet (roles are global admin/consumer, not
product-scoped), so there's nothing concrete to include here today — this is a forward-looking
constraint on the format's design, tracked in `TODO.md` alongside the "Consumer API
authorization" and "Multitenant" follow-ups.
