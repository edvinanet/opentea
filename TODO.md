# TODO / follow-ups

Deferred items identified along the way — not blocking current work, but worth tracking so
they don't get lost.

## Admin GUI / auth (this feature)
- [ ] CSRF token protection for GUI form POSTs. Phase 1 relies on `SameSite=Lax` cookies as a
      baseline mitigation only — a real per-form CSRF token is a conscious follow-up, not an
      oversight (see `README-admin.md` security notes once written).
- [ ] Password change / self-service, and editing a user's role after creation. First
      implementation only supports create + delete.
- [ ] Sliding session renewal — sessions currently expire at a fixed 24h from creation, not
      renewed on activity.
- [ ] Multiple API tokens per user (e.g. named tokens, scoped tokens, revocation of one
      without affecting others). Phase 1 is one token per user; regenerating invalidates the
      previous one.

## Phase 1 (base server) follow-ups
- [ ] Blob storage is filesystem-only (`internal/storage/fsblob.go`). The `Storage` interface
      was designed to make an S3-compatible implementation a drop-in later — not built yet.
- [ ] `version` sortField (on product releases / component releases) sorts lexicographically
      as TEXT, not semver-aware (e.g. "10.0.0" sorts before "2.0.0"). Matches a plain SQL
      implementation; documented limitation, not fixed.
- [ ] Discovery (`GET /tea/v1/discovery`) is self-authoritative only — no federation to other
      TEA servers.
- [ ] SBOM for all components

## Deferred phases (large, not started)
- [ ] **Publisher API** — the official TEA spec has no publisher/write API defined yet
      (post-1.0 per the spec's own roadmap). `/admin/v1` is an explicit stand-in.
- [ ] **Reference publisher** (part of the server/client/publisher reference-implementation
      trio) — explicitly deferred by the user (2026-07-04). Blocked on authoring an OpenAPI
      spec that extends TEA with the trust model from oej's `tea-trust-architecture` repo,
      since that spec shapes how the publisher actually works. Do this only after that spec
      exists — don't build a publisher against `/admin/v1` as a stand-in for this without
      checking with the user first, since the trust-model spec may change the shape entirely.
- [ ] **Trust architecture overlay** (from oej's `tea-trust-architecture` repo) — evidence
      bundles, Ed25519 ephemeral certs, DNS trust anchors (TAPS), optional transparency logs
      (Rekor/Sigsum/SCITT), staged/commit publisher workflow. No fixed schema exists yet for
      this, so nothing to build against — revisit once that design is formalized.
      **Known gap for the bundle format once this exists** (found 2026-07-05, not yet fixed):
      `ReleaseDistribution`/`ArtifactFormat.SignatureURL` isn't wired up by any admin handler
      today, and even once it is, the current export/import logic won't handle it correctly --
      `export.go`'s hash collection only looks at the main file's `Checksums`, so a detached
      signature file has no checksum slot of its own and wouldn't be captured into the bundle's
      `files/`; and import only rewrites `URL` against the destination server, not
      `SignatureURL`, which would leak the source server's address after a migration.
      Transparency-log entries don't exist in the schema at all yet, so nothing to
      export/import there either. Revisit the checksum/signature model and
      `internal/bundle`'s export/import together with the trust-architecture design.
- [ ] **Authorization** via OpenIDConnect/Oauth2
- [ ] **Multitenant**
- [ ] **Versioning of collections**
- [ ] **Promotheus API endpoint for metrics**
- [ ] **Consumer API (`/tea/v1`) authorization** — not discussed yet (2026-07-04). Today
      `/tea/v1` has no real authorization model: it's public by default, and a bearer token
      (when supplied) just identifies who's asking without granting/restricting access to
      specific products — anyone can read anything. Need to design what per-product/per-tenant
      access control for consumers should look like, presumably alongside **Multitenant**
      above. Directly relevant to the product import/export design (see next section): once a
      real authorization model exists, decide whether its rules travel with a product's
      export/import bundle (the user's stated intent is that they should — authorization
      travels, authentication/credentials do not).

## Reference client (this feature)
- [ ] **Browsing consumer GUI** — a web GUI for the client that lets you browse any TEA server's
      `/tea/v1` data (products, releases, components, collections, artifacts) read-only, for
      rapid testing/exploration without the CLI. Distinct from the server's own `/admin/ui`
      (which only covers users/stats/tokens, not product data browsing at all — see
      `internal/webadmin`). Deferred — client is CLI-only (`cmd/teaclient`) for now.
- [ ] `teaclient check` is a deliberately modest first cut (pagination re-verification +
      bounded artifact-checksum sampling). Deeper spec-conformance checks belong here:
      required-field presence, enum validity (`pkg/tea/enums.go` now has the canonical value
      lists to check against), cross-reference integrity (e.g. a product release's linked
      component/release actually exist), and CLE ordering/validity rules.
- [ ] `teaclient`'s `Discover` only queries the single server it's pointed at (self-authoritative
      lookup) — it doesn't implement the full TEI-authority `.well-known` bootstrap discovery
      flow (`discovery/readme.md`): extract authority from the TEI → fetch a well-known
      document from that authority → query one of the servers it lists.
- [ ] BLAKE3 checksum verification isn't implemented in `pkg/teaclient` (no stdlib or
      `golang.org/x/crypto` implementation without adding a new dependency) — reported as an
      explicit "unsupported algorithm" error rather than silently skipped.
- [ ] The fixtures replay tool's `{{name.field}}` templating only substitutes into JSON string
      values — an int-typed field (artifact `version`, CLE `eventId`) can't be filled from a
      placeholder. Worked around in `testdata/fixtures/edge-cases.json` by relying on
      deterministic values (artifacts are always created at `version: 1`; CLE `id`s are
      assigned in strict per-owner creation order) instead of fixing the substitution engine.

## Product import/export bundle (this feature)
- [x] ~~Validate manifest.json against the bundle JSON Schema before writing anything to the
      repository~~ -- done: `bundle.Import` calls `ValidateManifest` on the raw bytes before
      `json.Unmarshal`, before any repo writes.
- [x] ~~Verify artifact/distribution file checksums against actual bundle bytes before
      importing~~ -- done: `importBlobs` recomputes each `files/<sha256>` entry's real SHA-256
      via `storage.Put` and rejects the whole import if it doesn't match the claimed name,
      before any entity referencing it is created.
- [ ] Only SHA-256 is actually re-verified against file bytes (the one checksum type this
      server itself ever computes) -- other checksum algorithms in a bundle's checksums lists
      (MD5, SHA-1/384/512, SHA3-*, BLAKE2b-*, BLAKE3) are stored as-is but not independently
      verified, matching `pkg/teaclient`'s existing BLAKE3-unsupported limitation.
- [ ] No dry-run/validate-only mode for import (report what would happen without writing).
- [ ] Export is strictly per-product; no whole-server ("export everything") bundle mode.
- [ ] No `teaclient`/CLI convenience commands for actually running export/import -- that
      remains server-side (`/admin/v1`) only, per the user's explicit scoping. (A *standalone
      validator*, `cmd/bundlecheck`, was added 2026-07-05 -- see below -- but it only checks a
      bundle's validity, it doesn't call the export/import endpoints.)
- [x] ~~Standalone tool to check the validity of an export file~~ -- done (2026-07-05):
      `cmd/bundlecheck` + `internal/bundle/check.go`'s `Check(zr) (*CheckReport, error)`.
      Validates schema conformance, file-hash integrity, and referential completeness (every
      checksum referenced by the manifest has a matching `files/` entry) purely by reading the
      zip -- no server, DB, or admin auth needed. Scriptable: exit 0 if every given bundle is
      valid, exit 1 if any is invalid or unreadable.

## Config / deployment (this feature)
- [ ] TLS cert/key rotation isn't automatic (no SIGHUP reload or filesystem watch) — a
      certificate renewal (e.g. via certbot) requires a `systemctl restart opentea.service` to
      pick up the new files. No ACME/Let's Encrypt integration either.

## Tooling
- [x] ~~Makefile to run the build and test process~~ — done (2026-07-04): `Makefile` with
      `build`/`install`/`uninstall`/`test`/`vet`/`fmt`/`fmt-check`/`check`/`clean`/`help`
      targets, covering all three binaries (`opentea`, `teaclient`, `fixtures`).

## Scalability (second stage, not started)
- [ ] Server needs to scale, possibly by splitting into microservices (e.g. separating
      read-heavy `/tea/v1` traffic from write-heavy `/admin/v1` ingestion into independently
      deployable/scalable services). SQLite's single-writer constraint is the first thing that
      would need to change (e.g. Postgres) for true horizontal scale. Not started — keeping
      current package boundaries clean now so this is a smaller lift later, per user's
      confirmed preference.
