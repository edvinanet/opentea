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
      (Rekor/Sigsum/SCITT), staged/commit publisher workflow. The design repo has since grown
      to 34 docs/schemas including a concrete `evidence-bundle-schema.json` (found 2026-08-21;
      superseded the "no fixed schema exists yet" note this entry used to carry). A phased
      implementation plan now exists (six phases); **Phase 1 shipped 2026-08-21**: Ed25519
      signing/verification, ephemeral-key lifecycle, fingerprint identity, and the full
      evidence-bundle data model, in `internal/trust` (pure crypto logic, no DB/HTTP imports,
      mirrors `internal/authz`'s shape but is a deliberately separate subsystem -- trust
      answers "is this evidence valid," authz answers "can this caller read this," and neither
      imports the other) + `internal/repo/evidencebundle.go` + `internal/db/migrations/
      0006_trust.sql` + `pkg/tea/trust.go`'s wire types + minimal attach-only admin endpoints
      (`internal/admin/evidencebundle.go`: `POST /admin/v1/artifacts/{uuid}/{version}/
      evidenceBundle`, `POST /admin/v1/collections/{uuid}/{version}/evidenceBundle`,
      `GET /admin/v1/evidenceBundles/{uuid}`, gated by plain `requireRole`, not `authz.Decide`).
      Server-side verify-before-store is enforced (spec Sec 5.4/15.3): a bad signature is
      rejected before any DB write, tested by `cmd/opentea/trust_evidencebundle_test.go`.
      Bundles stay `status: "draft"` (not spec-conformant -- the schema's `timestamps`/
      `transparency` arrays are `minItems: 1` and required) until Phase 2 (RFC 3161 timestamps)
      and Phase 3 (Rekor/Sigsum transparency-log entries) land; `MarkEvidenceBundleComplete`
      enforces this. Phases 2-6 (timestamps, transparency log, MFA-approved publish/commit
      workflow, CI/CD OIDC/mTLS auth, discovery v2 + DNS/TAPS, conformance docs) remain
      unbuilt, each with open decisions for the user (default TSA, Rekor-vs-Sigsum priority,
      what "MFA-approved" means in this deployment, OIDC-vs-mTLS default, whether TAPS is ever
      claimed) — see the phased plan for detail.
      **Known gap for the bundle format, still open**: `ReleaseDistribution`/
      `ArtifactFormat.SignatureURL` (a legacy/simple detached-signature pointer, unrelated to
      the trust-architecture `EvidenceBundle` fields added alongside it) still isn't wired up
      by any admin handler, and even once it is, the current export/import logic won't handle
      it correctly -- `export.go`'s hash collection only looks at the main file's `Checksums`,
      so a detached signature file has no checksum slot of its own and wouldn't be captured
      into the bundle's `files/`; and import only rewrites `URL` against the destination
      server, not `SignatureURL`, which would leak the source server's address after a
      migration. Separately, `internal/bundle` (product import/export ZIP) doesn't carry
      `EvidenceBundle`/`EvidenceBundleRef` data at all yet either. Revisit the checksum/
      signature model and `internal/bundle`'s export/import together once more of the
      trust-architecture phases land.
- [ ] **Authorization** via OpenIDConnect/Oauth2
- [ ] **Multitenant** — **on hold (2026-08-10), per explicit user decision.** No tenant concept
      exists anywhere in the schema or request path today (confirmed 2026-08-06 while scoping
      ETag/conditional-request support for `/tea/v1`, added the same day). That work's
      per-resource-family list "dataset watermarks" and its `Cache-Control: public` policy are
      both deliberately untenanted, per the user's explicit decision to start simple rather than
      build tenant-shaped plumbing now for a feature that isn't scheduled. If multi-tenancy is
      ever implemented, the watermark tracking needs a tenant dimension added at the same time,
      `Cache-Control` needs to switch from `public` to `private`/`Vary`-based isolation (or a
      trusted, scope-keyed application cache, per the original ETag proposal's own guidance), and
      any CDN/reverse-proxy cache layer added later needs tenant-aware cache-key partitioning,
      not just per-resource ones. Consumer-API authorization work (below, and the
      `TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md` design) should proceed single-tenant
      (one organization = this deployment) for now rather than waiting on this.
      **Per-tenant TEA Trust Architecture support** (found 2026-08-26): the `TEA_TRUST_ARCHITECTURE`
      config flag added that day (`internal/config/config.go`, surfaced as a "Default TEA"/
      "Trusted TEA" badge in the admin GUI nav, see `internal/webadmin/templates/layout.html`) is
      necessarily deployment-wide, the same single-tenant simplification as everything else in
      this entry -- one opentea instance can only declare one profile today. If multi-tenancy is
      ever implemented, whether a vendor operates under the trust-architecture overlay needs to
      become a per-tenant setting (not a single process-wide flag), which means: a tenant dimension
      on the config/setting itself (likely moving it out of `internal/config` and into a per-tenant
      row once a tenant table exists, rather than an env var), the nav-bar badge and the
      per-artifact/collection evidence badges (`internal/webadmin/evidence.go`) scoped to the
      logged-in user's own tenant rather than the whole server, and `internal/trust`/
      `internal/admin/evidencebundle.go`'s evidence-bundle endpoints scoped (or at least
      fingerprint-reuse-checked, see `used_fingerprint` in `0006_trust.sql`) per tenant rather than
      globally -- a fingerprint reused across two different tenants' artifacts is a different
      question than reused within one tenant's own signing history. Revisit together with the rest
      of this Multitenant entry once tenancy is actually scheduled.
      **Roles need a third tier** (found 2026-08-26, per explicit user decision): today's two
      admin-domain roles (`model.RoleAdmin`/`model.RoleConsumer`, `internal/model/model.go`) are
      flat and server-wide -- every admin/consumer account already sees every product/component in
      the one implicit organization, since there's nothing to scope to. Multi-tenancy needs a
      **SuperUser** role, authorized to see and manage every tenant (for the vendor/operator running
      the shared instance), distinct from ordinary **tenant-scoped GUI accounts**, each restricted
      to a single tenant or an explicit subset of tenants -- not the same axis as `admin`/`consumer`
      (which is a *within-a-tenant* read/write distinction and stays meaningful once tenancy exists:
      a tenant will still want its own admins and read-only consumers). Concretely, this touches:
      `internal/authn`'s identity/session model (a user row needs a tenant-membership set, plus a
      superuser flag/role independent of it), `internal/webadmin`'s every list/detail handler (each
      would need to filter by the caller's tenant scope unless they're a superuser, not just role-gate
      via `requireRole` as today), and `internal/admin`'s equivalent JSON API handlers. `internal/authz`
      (the *consumer*-facing `/tea/v1` entitlement system) is a separate, already-tenant-agnostic
      concern per its own file header and is not what this note is about -- this is specifically
      about `/admin/v1`+`/admin/ui` operator/vendor-staff access, which currently has no tenant
      concept to restrict at all. Revisit together with the rest of this Multitenant entry once
      tenancy is actually scheduled.
- [ ] **Versioning of collections**
- [ ] **Promotheus API endpoint for metrics**
- [x] ~~**Consumer API (`/tea/v1`) authorization**~~ — Phase 1 shipped (2026-08-11), per
      `~/TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md` (v0.1 draft, 2026-08-06; not in this
      repo) and its Sec 27 "Implementation Profile for OpenTEA", single-tenant per the user's
      2026-08-10 decision. `/tea/v1` is no longer unconditionally public: every route now runs
      `authz.Decide` (`internal/authz`, a pure policy-evaluation engine implementing the spec's
      Sec 15.1-15.3 ranking algorithm -- resource specificity, then subject specificity, deny
      wins at equal specificity, default deny) against entitlements/templates managed via new
      `/admin/v1` endpoints (`internal/admin/{template,entitlement,productgroup,releasegroup}.go`).
      Schema: `internal/db/migrations/0005_authz.sql`. A migration-seeded bootstrap entitlement
      (everyone/all_products, fully permissive) preserves today's anonymous-read-everything
      behavior on upgrade until an admin narrows it -- a real, visible, revocable row, not a
      bypass in the engine. Capability independence (spec Sec 12.2: product access doesn't imply
      collection/artifact access) and per-caller list-pagination filtering (spec Sec 18) are both
      enforced and integration-tested (`cmd/opentea/authz_tea_test.go`). ETags/Cache-Control are
      now principal-scoped (`internal/api/cachepolicy.go`'s `conditional`/`authzCacheSuffix`) --
      anonymous responses stay public/shared-cacheable, authenticated ones become private and
      partition by the caller's effective entitlement revision.

      **Explicit Phase 1 deferrals** (not built, by design): organization/multitenant subject
      (still on hold), audience/principal groups, capability overrides (single-capability grants
      layered on a template), mTLS, external decision-service profile, signed-collection
      filtering, events/notifications, emergency restrictions, `/admin/ui` GUI for
      templates/entitlements (JSON API only), `insight.query`, and fine-grained SBOM/VEX/VDR
      artifact classification (reuses the existing `artifact.type` enum, which can't distinguish
      a VEX document from a generic SBOM -- real fix is a future `artifact.classification`
      column). Component/component-release scoping exists but is deliberately simpler than the
      product side: no component-group/component-release-group tables, just all_products plus a
      direct component/component_release grant.

      **Known gap, not addressed this pass**: `GET /files/{sha256}` (`internal/files`) -- the raw
      blob download endpoint referenced by artifact/collection URLs -- has no authorization check
      at all, matching its pre-existing behavior. Even where `/tea/v1` now denies
      `artifact.metadata.read`/`artifact.discover` for a given artifact, its underlying blob
      remains fetchable directly if the caller already knows (or brute-forces) its sha256. Low
      practical severity (256-bit hash preimage space) but a real gap between "the API says no"
      and "the bytes are actually inaccessible" -- worth wiring `authz.Decide` into
      `internal/files` in a follow-up, or issuing short-lived signed download URLs instead of
      bare content-addressed ones.

      **Not yet done**: bundle export/import doesn't carry entitlements/templates yet (the
      schema doesn't preclude it -- a future `manifest.entitlements[]` section would follow the
      same `WithTx` + `ErrImportIdentityConflict` pattern as everything else in
      `internal/bundle`), and `/admin/ui` has no template/entitlement management pages.

      A 2026-08-06 external scan flagged that `/tea/v1`'s anonymous-by-default behavior
      (now `internal/api/auth_middleware.go`'s `resolvePrincipal`, formerly `optionalBearerAuth`)
      diverges from the upstream TEA OpenAPI document's global bearer-or-Basic security
      declaration, and suggested either enforcing that contract strictly or publishing an
      implementation-specific OpenAPI doc. Per the user: this isn't a conformance bug --
      authentication behavior is a per-server policy decision, and the OpenAPI's global security
      block doesn't obligate every deployment to require it.

## Reference client (this feature)
- [ ] **Browsing consumer GUI** — a web GUI for the *client* that lets you browse *any* TEA
      server's `/tea/v1` data read-only, for rapid testing/exploration without the CLI, pointed
      at whichever server you choose. Still deferred — client is CLI-only (`cmd/teaclient`) for
      now. Distinct from `/admin/ui`'s own browsing (added 2026-08-04, see below) — that only
      ever shows *this* server's own data via direct repo access, not an arbitrary remote server
      over HTTP like this item would.
- [x] ~~Add data browsing to /admin/ui~~ — done (2026-08-04): `/admin/ui/products` and
      `/admin/ui/components` list pages plus detail pages (with linked releases, CLE events,
      linked components, distributions, and collections/artifacts inline) —
      `internal/webadmin/{products,productreleases,components,componentreleases}.go` +
      matching templates. Read-only, gated at the same `consumer` role as the dashboard. The
      dashboard's Product/Component Release cards link to the `/tea/v1` JSON query endpoints
      instead of getting dedicated pages, since those entities have no owner-less list in the
      repo layer to browse.
- [ ] `teaclient check` is a deliberately modest first cut (pagination re-verification +
      bounded artifact-checksum sampling). Deeper spec-conformance checks belong here:
      required-field presence, enum validity (`pkg/tea/enums.go` now has the canonical value
      lists to check against), cross-reference integrity (e.g. a product release's linked
      component/release actually exist), and CLE ordering/validity rules.
- [x] ~~`teaclient`'s `Discover` only queries the single server it's pointed at (self-authoritative
      lookup) — it doesn't implement the full TEI-authority `.well-known` bootstrap discovery
      flow~~ — done (2026-08-27): `teaclient.BootstrapDiscover` (`pkg/teaclient/wellknown.go`)
      implements the full flow -- `tea.ExtractTEIAuthority` (`pkg/tea/tei.go`) extracts the
      authority, fetches `.well-known/tea`, sorts candidate endpoints by priority, and tries each
      via the existing `Discover` until one succeeds. `cmd/teaclient discover <tei>` now makes
      `-server` optional: given, unchanged direct-query behavior; omitted, triggers this flow.
      Built against the user's own **unmerged** upstream PR
      (`CycloneDX/transparency-exchange-api#261`, head `be64bc7` at the time) -- the TEI format
      moves from `urn:tei:<type>:<domain>:<id>` to `tei://<domain>/<type>/<id>` (identifier is
      plain URL/percent-encoding, not BASE64URL despite that PR's diff having base64url-looking
      examples -- confirmed directly with the user). **Revisit if that PR's syntax changes before
      merging.**

      Also implements two things the spec text turned out to require beyond the bootstrap flow
      itself, found by reading past the initially-excerpted "Port resolution" section into the
      unchanged "Connecting to the API" section: (1) **version negotiation** -- `teaclient` now
      has its own `SupportedVersions` (`pkg/teaclient/semver.go`, hand-rolled SemVer 2.0.0
      precedence comparator, no new dependency) and picks the highest version mutually supported
      with each candidate endpoint, constructing `<endpoint.url>/v<version>/discovery?tei=...`
      per the spec's MUST-level wording -- this originally surfaced a real, separate gap (opentea's
      own server mounted its API at a fixed `/tea/v1`, not `/v{exact-semver}/`), **fixed
      2026-08-27**: `config.Config.APIBasePath` (`TEA_API_BASE_PATH`, default `/tea/v1`,
      `internal/config/config.go`) now controls the one path prefix `internal/api` mounts under
      (`internal/api/router.go` builds every route pattern from it; `cmd/opentea/main.go` mounts
      the router at `cfg.APIBasePath+"/"` instead of the old hardcoded literal). A standalone
      deployment (no fronting proxy) that wants to be literally reachable at the bootstrap flow's
      constructed path sets `TEA_API_BASE_PATH=/v0.4.0` (matching its single `TEA_VERSIONS` entry);
      the default stays `/tea/v1` for existing deployments. Deliberately a single-mount replacement,
      not dual-serving both paths at once, and deliberately doesn't attempt to serve more than one
      `TEA_VERSIONS` entry at its own literal `/v{version}` path simultaneously -- a deployment
      needing that needs a version-aware fronting proxy instead (see the config's own doc comment
      and `README.md`'s `TEA_API_BASE_PATH` row for the split with `TEA_ROOT_URL`, which stays the
      separate, already-existing knob for what's advertised externally and is untouched by this).
      opentea still doesn't serve `.well-known/tea` for itself, though -- **a distinct, still-open
      gap**: even with `APIBasePath` pointed at a literal version path, nothing publishes the
      bootstrap document a TEI-authority lookup would need to find that path in the first place;
      revisit once/if opentea needs to be TEI-authority-discoverable, not just directly queryable.
      (2) **HTTPS/SVCB DNS record support** (`pkg/teaclient/svcb.go`) for the "Port resolution"
      section's optional failover/load-balancing guidance -- added `github.com/miekg/dns` as a
      new dependency (stdlib has no SVCB/RFC 9460 parsing at all; evaluated via the
      `dependency-review` skill, zero new CVEs, SBOMs regenerated). Deliberately out of scope
      within that: ECH config, ipv4hint/ipv6hint, ALPN enforcement beyond Go's TLS stack, and
      AliasForm chains longer than one hop. No native Windows/macOS system-resolver-config
      integration -- `/etc/resolv.conf` or a fixed public-resolver fallback only.
- [x] ~~External security review of `docs/discovery-test-rig.md`
      (`docs/security-review-disc-tests-260827.md`, 2026-08-27) flagged TEST-14 as testing a
      scenario the spec forbids~~ -- verified against the live spec text at the exact PR #261
      commit (`be64bc7`) this work was built against: "Currently, the port number is not part of
      the TEI but it is needed to connect to the API... the server that is part of the first step
      of discovery will by default be running on the default HTTPS port 443" -- a non-default port
      can only come from an HTTPS/SVCB record or `endpoints[].url`, never the TEI itself. TEST-14
      previously asserted the opposite (a port-bearing TEI, fallback must preserve it). **Fixed
      2026-08-27**: TEST-14 now asserts a port-bearing TEI is rejected before any resolution
      attempt; added TEST-22 as the positive case (non-default port via `endpoints[].url`).
      **Fixed 2026-08-27**: `pkg/tea.ExtractTEIAuthority` now rejects a port-bearing TEI outright
      (`u.Port() != ""` check, returning a `*TEIError`), matching TEST-14 and the spec text.
      `pkg/teaclient/svcb.go`'s `fallbackHost`/`fallbackPort`-preserving logic was deliberately
      left in place rather than deleted -- `resolveWellKnownTargetWithConfig` is a general-purpose
      resolver that shouldn't assume its caller already validated a TEI, and it's directly
      exercised with a port-bearing authority by `svcb_test.go`'s own tests; only its doc comment
      was updated to note production callers never actually feed it one anymore. The 7
      `TestBootstrapDiscover*` flow-logic tests in `wellknown_test.go` (priority, version
      negotiation, failover, retry, 401/403) still need a directly-dialable `host:port` to reach a
      local `httptest` server without standing up a fake DNS server just to redirect a
      spec-conformant, port-free authority there -- they now call a new unexported
      `bootstrapDiscoverWithAuthority(ctx, authority, tei, opts...)` directly (authority already
      extracted, bypassing the new check) rather than the public `BootstrapDiscover`.
      `TestBootstrapDiscoverRejectsPortInTEI` (new) exercises the full public entry point instead,
      asserting a port-bearing TEI is rejected before any network I/O. `ExtractTEIAuthority`'s own
      port rejection is covered directly in `pkg/tea/tei_test.go` (plain host, IPv6-literal
      variants).
      The same review also confirmed a real credential-reuse behavior in `BootstrapDiscover`
      (`pkg/teaclient/wellknown.go`): a caller-supplied `WithBearerToken` is threaded through the
      same `opts` to every candidate endpoint's `NewClient` call in the failover loop, so the same
      token is resent to endpoint B after endpoint A fails or is skipped -- not a leak to
      `.well-known/tea` itself (that fetch uses the raw `http.Client`, not `do()`, so no
      `Authorization` header goes there), but real reuse across different candidate servers within
      one `BootstrapDiscover` call. **Documented, not changed, 2026-08-27**: the spec itself
      defines no credential-scoping policy for multi-endpoint failover, and `BootstrapDiscover`'s
      `opts ...Option` signature has no way to express "different credentials per candidate" even
      if it wanted to reject reuse by default -- doing so would break the common case (one
      operator's own redundant/mirrored servers behind one TEI) to guard a rarer one (candidates
      run by mutually-untrusting parties). `BootstrapDiscover`'s doc comment now states this
      explicitly and tells a caller who can't assume mutual trust across candidates to call
      `Discover` directly per server instead of relying on this. Revisit if a real caller needs
      per-endpoint credential scoping badly enough to justify a signature change.
      Remaining findings in the review (caching/freshness tests, full OpenAPI response-body
      validation, SVCB edge cases, TEI encoding edge cases, additional candidate transport-failure
      cases, a missing 401 companion to TEST-09) are plausible test-coverage gaps for a rig
      "meant to become real infrastructure" but weren't independently re-verified one by one, and
      aren't addressed here -- revisit as a batch if/when this rig is actually stood up.
- [ ] BLAKE3 checksum verification isn't implemented in `pkg/teaclient` (no stdlib or
      `golang.org/x/crypto` implementation without adding a new dependency) — reported as an
      explicit "unsupported algorithm" error rather than silently skipped.
- [ ] **`teaclient` has no TEA Trust Architecture evidence-bundle verification** (found
      2026-08-27, confirmed via code search: zero references to `EvidenceBundle`/Ed25519/
      certificates anywhere in `pkg/teaclient`/`cmd/teaclient`). Bearer-token authentication is
      already supported (`teaclient.WithBearerToken`, `cmd/teaclient`'s `-token` flag) and is a
      separate, unrelated concern -- this gap is specifically about the trust overlay added
      2026-08-21 (`internal/trust`, `pkg/tea/trust.go`). The client's existing "verify"
      functionality (`DownloadAndVerify`/`DownloadAndVerifyTo`, `pkg/teaclient/artifacts.go`)
      only checks basic checksums (`ArtifactFormat.Checksums`, MD5/SHA1/SHA256) against
      downloaded bytes -- it never fetches, parses, or verifies an `Artifact`/`Collection`'s
      `EvidenceBundle`/`EvidenceBundleRef` fields at all. A conformant client-side verifier would
      need to: (1) parse the embedded `EvidenceBundle` or dereference an `EvidenceBundleRef` and
      fetch+digest-check the external bundle; (2) verify the Ed25519 signature against the
      embedded certificate (mirrors `internal/trust.Verify`/`ParseCertificatePublicKey`, but
      client-side, so probably a shared/duplicated helper rather than importing the server's
      internal package -- `internal/...` isn't importable outside this module); (3) once Phase
      2/3 land server-side, also verify the RFC 3161 timestamp and transparency-log inclusion
      proof, not just the signature; (4) decide what "verified" should mean for a bundle still in
      `status: "draft"` (no timestamp/transparency yet) vs `"complete"` -- note `status` itself is
      `json:"-"` (internal-only, see `pkg/tea/trust.go`), so the client can't even observe that
      distinction from the wire today, only whether `evidenceBundle`/`evidenceBundleRef` is
      present. Not scoped to any phase of the server-side trust-architecture plan yet -- revisit
      once client-side verification priority is decided.
- [ ] The fixtures replay tool's `{{name.field}}` templating only substitutes into JSON string
      values — an int-typed field (artifact `version`, CLE `eventId`) can't be filled from a
      placeholder. Worked around in `testdata/fixtures/edge-cases.json` by relying on
      deterministic values (artifacts are always created at `version: 1`; CLE `id`s are
      assigned in strict per-owner creation order) instead of fixing the substitution engine.

## Product import/export bundle (this feature)
- [x] ~~`bundle.Import` silently aliases content from two different, unrelated source servers
      that happen to reuse the same UUID~~ -- fixed (2026-08-06): a source server's UUID is only
      meaningful *within* that source server, but every `repo.Import*` method (and
      `LinkComponent`, when reached via bundle import) did a bare check-then-insert on the key --
      if it already existed locally, the call silently no-opped with no comparison of the
      incoming content, so a second bundle from an unrelated server reusing the same UUID/
      `(uuid, version)` would be treated as "already imported" and its real content dropped with
      no trace. All 9 `Import*` methods (`ImportProduct`, `ImportComponent`,
      `ImportProductRelease`, `ImportComponentRelease`, `ImportDistribution`, `ImportArtifact`,
      `ImportCollection`, `ImportCLEEvent`, `ImportCLESupportDefinition`) now fetch the existing
      row and compare content before treating a same-key match as idempotent, returning the new
      `repo.ErrImportIdentityConflict` sentinel on a genuine mismatch instead of silently
      succeeding (which the existing `WithTx`-based atomicity fix above then rolls back for
      free). Added `ImportComponentLink` (`internal/repo/productrelease.go`) as a stricter
      sibling to `LinkComponent` for the same reason -- `LinkComponent` itself is unchanged and
      keeps its permissive UPSERT-on-conflict re-pin behavior for the regular admin API, where
      re-pinning a component to a different release is a deliberate, correct action; bundle
      import now calls `ImportComponentLink` instead. Found along the way and fixed as a
      prerequisite: most `Get*` methods queried via `r.db` directly instead of `r.conn()`, which
      is a latent deadlock hazard (not just staleness) given `internal/db/db.go` pins the
      connection pool to a single connection -- code running inside `Repo.WithTx`/`runInTx`
      holding that one connection would self-deadlock calling a `r.db`-routed `Get*`, not get a
      stale read. Found a second, subtler layer of the same hazard while implementing the fix
      (not caught by review beforehand): even `r.conn()`-based `Get*` methods aren't safe to call
      *from inside* a `runInTx` closure when `runInTx` opens its own standalone transaction (the
      common case when an `Import*` method is called directly, e.g. from a unit test, rather than
      composed into an outer `Repo.WithTx` as `bundle.Import` always does) -- `r.conn()` evaluated
      from inside such a closure still resolves to `r.db`, not the closure's own open `tx`. Fixed
      by extracting every `Get*`'s query logic into a `dbtx`-parameterized twin (`getProductTx`,
      `getComponentTx`, etc., mirroring the pre-existing `getDistributionTx` pattern) that the
      conflict checks call directly with the closure's own `tx`, while the public `Get*` methods
      become thin `r.conn()`-based wrappers. Regression test:
      `TestImportArtifactConflictFailsFast` (`internal/repo/import_test.go`) calls
      `ImportArtifact` directly on a fresh `*Repo` (forcing the standalone-transaction path) with
      a short `context.WithTimeout`, and was verified to actually hang until timeout (not fail
      fast) when the dbtx-twin fix is reverted -- the equivalent scenario exercised through
      `bundle.Import` (`internal/bundle/import_test.go`'s `TestImportConflictDetection/artifact`)
      does *not* reproduce this, since `bundle.Import` always calls through an outer `WithTx`
      where `r.tx` is already set, which is exactly why the bug needed its own repo-level test.
- [ ] Deliberately out of scope for the fix above (larger initiatives, not designed yet): (1)
      full content-based deduplication across *different* source UUIDs -- e.g. reusing an
      existing local artifact when its checksums match even though it arrived under a different
      source UUID, which would need a new source-to-local identity mapping table extended across
      all 7 entity types, plus a reuse-vs-import policy; a future version of this would run
      *before* falling through to the collision check added above, not replace it. (2)
      Signed-collection provenance preservation -- collection signing isn't implemented in this
      project yet (see **Trust architecture overlay** above), so there's nothing to preserve
      through import yet either.
- [ ] CLE `EventID`/`SupersededByVersion` cross-references aren't validated for dangling
      references on import either way (pre-existing gap, not touched or made worse by the
      identity-conflict fix above).
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
- [x] ~~`bundle.Import` isn't atomic~~ -- fixed (2026-08-05): found by external review; each
      entity's own `repo.Import*` call was individually transactional, but nothing spanned the
      whole import, so a failure partway through left earlier entities committed. Added
      `Repo.WithTx(ctx, func(txRepo *Repo) error) error` plus a `conn()`/`runInTx` helper pair
      (`internal/repo/repo.go`) so every `Import*` method, `LinkComponent`, and `UpsertBlob`
      composes into one outer transaction when called through a `WithTx`-scoped `Repo`;
      `bundle.Import` now wraps its whole write sequence (including `importBlobs`'s DB
      bookkeeping, not its file write -- see the item below) in one `WithTx` call. Regression
      test: `TestImportIsAtomicOnFailure` (`internal/bundle/import_test.go`) injects a late FK
      failure and asserts every earlier-written entity (product, release, component, component
      release) is also gone after rollback.
- [ ] No test for context cancellation mid-`Import` (caller's `ctx` canceled/timed out while
      writes are in flight). `TestImportIsAtomicOnFailure` only proves rollback for an
      *application* error (an FK violation); a canceled context should hit the same rollback
      path in `Repo.WithTx` (`ExecContext` returns `context.Canceled`, which propagates through
      the same error return), but that's inference, not a test. Add a test that cancels `ctx`
      partway through a multi-entity import (e.g. via a `context.Context` wrapper that cancels
      after N queries) and asserts the same all-or-nothing rollback `TestImportIsAtomicOnFailure`
      checks for.
- [ ] No concurrency limit on large uploads (found 2026-08-05, external review): both
      `importProduct` (`internal/admin/bundle.go`) and `receiveFile` (`internal/admin/upload.go`)
      cap a single upload at 1 GiB, but nothing caps how many of those can run at once. The
      memory-exhaustion angle is fixed -- `importProduct` no longer buffers the whole bundle into
      a `[]byte` (reads via `multipart.File`'s `io.ReaderAt` instead), and `receiveFile` already
      streamed straight into blob storage -- so concurrent large uploads are now bounded by disk
      (temp files + blob writes) rather than RAM, but many concurrent 1 GiB imports/uploads from
      authenticated admins could still exhaust disk space or degrade the single SQLite connection.
      Consider a bounded semaphore (e.g. `internal/admin`-wide, gating both handlers) if this
      becomes a real concern -- admin-only surface, so the practical exposure is lower than a
      public endpoint.
- [ ] Related to the above: `importBlobs` writes blob files to disk (via `storage.Put`) before
      any DB error partway through the rest of the import could occur, and those writes can't
      be rolled back by a DB transaction (filesystem isn't transactional). Low risk in practice
      -- blobs are content-addressed, so a leftover file from a failed import is either picked
      up harmlessly by a later successful retry, or just sits as an orphan at its own hash path
      -- but there's currently no cleanup/garbage-collection for blobs that end up orphaned
      this way (or orphaned by other means, e.g. a deleted product/artifact leaving its blob
      behind). Needs a GC pass (e.g. sweep `internal/storage` for blobs with no referencing row
      in `blob`/checksum tables) rather than being handled at import time.
      `internal/admin/upload.go`'s `receiveFile` had the same root problem, called out separately
      by external review (2026-08-05): it persisted a blob (file + `UpsertBlob` DB row) *before*
      `uploadDistributionFile`/`uploadArtifactFormatFile` confirmed the target distribution/
      artifact/formatIndex even existed, so an upload to a bad id always orphaned a blob, not just
      on rare failures. Fixed the deterministic case: both handlers now check the target exists
      (`GetDistribution`/`GetArtifactByVersion`, plus a `formatIndex` range check) *before* calling
      `receiveFile` -- see `TestUploadToInvalidTargetDoesNotOrphanBlob`
      (`cmd/opentea/integration_test.go`), which proves via `GET /files/{sha256}` that a rejected
      upload never gets stored at all. Not fully closed: a real error, disconnect, or context
      cancellation between `receiveFile` (blob written) and `SetDistributionFile`/
      `SetArtifactFormatFile` (attach) can still orphan a blob -- that residual window, like
      `importBlobs`'s, needs the same GC pass rather than more upfront checks.
- [ ] Deleting a product/component release doesn't clean up its collections (found 2026-08-06
      while adding ETag support -- see below): `collection.uuid` is polymorphic (either a
      product_release or component_release uuid depending on `belongs_to`), so it can't carry a
      `FOREIGN KEY` to either table, and there's no `DeleteCollection` at all. Deleting the
      "owning" release leaves its collections (and their `collection_artifact` rows) in place as
      orphaned rows, still fetchable by `(uuid, version, belongsTo)` via
      `GET /*Release/{uuid}/collection/{version}` even though the release itself now 404s. Low
      practical impact (collections are rarely deleted independently of their release, and the
      orphaned data isn't wrong, just unreachable through the normal parent-scoped browsing
      paths), but worth an explicit cleanup pass (e.g. `DeleteProductRelease`/
      `DeleteComponentRelease` sweeping `collection`/`collection_artifact` by `uuid` the same way
      `deleteOwnerScoped` already sweeps `identifier`/`cle_event`) rather than leaving it implicit.
- [ ] Bundle-level signing/hashing (noted 2026-08-05, not yet designed): today only individual
      `files/<sha256>` entries are checksum-verified (see the two done items above) -- there's
      no signature or hash covering the *bundle zip as a whole* (manifest + files together), so
      nothing currently proves a bundle wasn't tampered with after export beyond its individual
      file contents matching their own claimed hashes (e.g. the manifest itself, or which files
      are included/excluded, isn't provable as untampered). Distinct from the per-entity
      `SignatureURL` gap already tracked under **Trust architecture overlay** above -- that's
      about signing individual distributions/artifacts; this is about signing/hashing the
      bundle file itself. Revisit alongside that trust-architecture design work once it's
      formalized, rather than inventing an ad hoc bundle-signing scheme now.
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

## ETag / conditional requests (this feature)
- [ ] Check the `revision`/`cle_revision`/`dataset_watermark` scheme (added 2026-08-06) against
      the actual TEA OpenAPI spec, not just the external design proposal it was built from.
      These are purely internal, server-side counters -- never exposed in any response body or
      schema, only folded into the opaque `ETag` string -- so they can't violate the spec's wire
      format by construction, but worth double-checking there isn't spec text about caching,
      `ETag`/`Last-Modified` semantics, or a spec-defined revision/version concept for these
      resources that this implementation should align to (or explicitly diverge from with a
      documented reason) rather than having invented its own scheme unchecked against the spec.
      Also re-verify the immutability assumptions the whole design leans on (`product`/
      `component`/`collection` having no update path today) still hold if the spec is ever
      extended with an update/rename operation for any of them.
- [ ] `pkg/teaclient` doesn't support conditional polling yet -- server-side `ETag`/
      `If-None-Match` handling is done (see above), but the client has no way to send back a
      previously-seen `ETag` or act on a `304`. This is Phase 4 (item 1) of the original ETag
      proposal ("Proposal: ETag and Conditional Request Support", 2026-08-06, see below for
      other unresolved parts of the same document), deliberately deferred when the server-side
      work (Phases 1-3) was scoped and built (2026-08-06). Needs: persisting the last `ETag` per
      URL (+ auth scope, once one exists) so repeated polls can send `If-None-Match`; treating
      `304` as success-with-no-new-data rather than an error; and probably a small wrapper type
      (the original proposal sketched `ConditionalResult[T]{Value, ETag, NotModified}`) so
      callers doing continuous polling don't have to manage raw `*http.Response`s themselves.
      This is the actual point of the whole server-side feature -- it only reduces real polling
      load once a client uses it.
- [ ] `GET /files/{sha256}` (`internal/files/handler.go`) still has no `ETag`/`If-None-Match`
      support -- the document's Phase 1, step 1 explicitly calls this out first ("Add ETag
      support to `/files/{sha256}`"), and it's the easiest case in the whole proposal (blobs are
      already content-addressed and immutable, so `ETag: "sha256:<digest>"` needs no revision
      tracking at all -- the handler already sends the right `Cache-Control:
      public, max-age=31536000, immutable`, just no `ETag` header or conditional check). Missed
      in this pass because `internal/files` sits outside `internal/api` and wasn't in the
      approved implementation scope, which was written against `/tea/v1` routes specifically.
- [ ] `GET /tea/v1/discovery` has no ETag support. The document has a dedicated "Discovery"
      section recommending an ETag built from the normalized TEI, TEI-to-release mapping
      revision, configured root URL, advertised API versions, and (if discovery ever becomes
      restricted) authorization scope. Explicitly out of scope for this pass, same as it's out
      of scope in `## Phase 1 (base server) follow-ups` above (self-authoritative only, no
      federation) -- revisit together.
- [ ] No observability was added for this feature. The document's "Observability" section asks
      for `200` vs `304` counts by endpoint, conditional-request outcomes (missing/matching/stale
      validator), ETag lookup latency vs. full-representation latency, and bytes avoided via
      `304`; its Acceptance Criteria sets a concrete target ("more than 95% of routine no-change
      polls return `304`"). None of this is instrumented today, so there's currently no way to
      confirm the feature is actually reducing load in production the way it was designed to.
- [ ] CDN/reverse-proxy caching (document's Phase 4, items 3-4: "Configure CDN or reverse-proxy
      caching" and "Validate authorization-aware cache isolation") is unaddressed beyond emitting
      correct `Cache-Control` headers server-side -- no actual edge/CDN config, and no test
      proving a real intermediary cache respects `ETag`/`Cache-Control` the way this
      implementation assumes. Overlaps with **Multitenant** above (cache-key partitioning once
      tenancy exists) but is a distinct, narrower gap even for the current untenanted, all-public
      state: nothing has verified an actual proxy/CDN in front of this server behaves correctly.
      Related to the new front-ingress-proxy TODO under **Config / deployment** below, though
      that one is about trusting inbound `X-Forwarded-*` request headers, not outbound response
      cacheability.
- [ ] Compression-variant ETag correctness (document's "Compression variants" section) is
      currently moot -- this server does no gzip/Brotli content negotiation today -- but is a
      landmine if that's ever added: a strong ETag must then either vary by encoding, be
      generated per-variant by the CDN, or fall back to a weak validator, none of which this
      implementation currently has any provision for.
- [ ] Change-event delivery (an `outbox_event` table, sketched in the document's "Import and
      transaction behavior" section) was an explicit non-goal for this pass ("Initial
      implementation does not require webhooks, SSE, or a change-feed endpoint") and remains
      fully undesigned. Distinct from the polling-reduction goal this feature actually
      implements -- would let clients avoid polling entirely rather than just making unchanged
      polls cheap.

## Config / deployment (this feature)
- [x] ~~Separate the webadmin/admin-API surface from the consumer API onto different
      ports/addresses~~ -- done (2026-08-27), per explicit user request ("those are two
      different things and may in some installations bind to different ports and IP
      addresses"). `config.Config.AdminListenAddr` (`TEA_ADMIN_LISTEN_ADDR`, empty by default)
      splits `/admin/v1` + `/admin/ui` (operator-facing, session-cookie auth) onto their own
      `http.Server`, separately from `TEA_LISTEN_ADDR`'s `/tea/v1` + `/files` (consumer-facing).
      Empty (default) keeps today's single-listener behavior exactly (`cmd/opentea/main.go`'s
      `newMux`, unchanged wiring); set, `main()` runs two `http.Server`s concurrently
      (`newAPIMux`/`newAdminMux`, `buildServer`), shut down together on SIGINT/SIGTERM. Both
      listeners share one TLS cert/key pair when TLS is on -- no per-listener TLS config in this
      pass (a deliberate scope cut, see `internal/config/config.go`'s `AdminListenAddr` doc
      comment; revisit if a deployment needs different TLS termination per surface). Also fixed
      a real consequence of the split: `internal/webadmin/templates/dashboard.html`'s
      "Product/Component Release (API)" cards linked to `{{.APIBasePath}}/...` as a
      same-origin-relative path, which only worked because the GUI and the API happened to share
      an origin before this -- now `{{.RootURL}}{{.APIBasePath}}/...`, correct whether or not the
      two are actually the same origin. Regression test:
      `TestSplitListenersIsolateRoutes` (`cmd/opentea/integration_test.go`), proving each
      listener's mux serves only its own routes (404, not just "unauthenticated", for the other
      surface's routes).
- [ ] Add HTTPS proxy settings to the config file (requested 2026-08-11, not yet scoped) --
      needs a decision on what this covers: outbound HTTPS-proxy support (e.g. `HTTPS_PROXY`/
      `NO_PROXY`-style config for any outbound calls this server or `teaclient` makes), vs.
      additional inbound reverse-proxy/TLS-termination settings alongside the existing
      `TEA_TRUST_PROXY_HEADERS` (see the front-ingress-proxy item directly below, which is the
      current state of inbound proxy trust).
- [ ] Front-ingress-proxy support is narrower and less verified than it should be. Today
      `TrustProxyHeaders` (`internal/config/config.go`, added 2026-08-06 fixing a scan finding)
      only gates two call sites: `internal/webadmin/loginlimiter.go`'s `clientIP` (right-most
      `X-Forwarded-For` entry, used for login rate-limit keying) and `internal/httpx/security.go`'s
      `IsSecure` (`X-Forwarded-Proto: https`, used for the `Secure` cookie flag). Gaps: (1) no
      other code path uses `X-Forwarded-For` for client-IP-dependent logic (e.g. if IP-based
      logic is ever added to `/tea/v1` or `/admin/v1`, it would need to thread the same flag
      through, not reimplement its own header parsing); (2) `X-Forwarded-Host` isn't read or
      validated anywhere, so a proxy that rewrites the host isn't accounted for; (3) only tested
      against synthetic headers set directly on an `httptest.Request` (`loginlimiter_test.go`,
      `security_test.go`) -- never against a real reverse proxy actually rewriting/appending
      headers over the wire, so header-format edge cases a real proxy produces (multiple `X-
      Forwarded-For` header lines vs. one comma-joined line, IPv6 literals, a proxy that doesn't
      strip a client-supplied spoofed header before appending its own) are unverified; (4)
      `README-deploy.md` currently states "no reverse proxy -- the server listens directly on
      `TEA_LISTEN_ADDR`" and never mentions `TEA_TRUST_PROXY_HEADERS` at all, so a deployer
      actually putting this behind nginx/Traefik/an ingress controller has no documented guidance
      on when or how to turn proxy trust on safely. Needs: (a) a `README-deploy.md` section
      documenting `TEA_TRUST_PROXY_HEADERS`, when to enable it (only when the proxy is the sole
      network path to the server and itself strips/overwrites client-supplied `X-Forwarded-*`
      headers before appending its own -- the existing doc comment on `config.TrustProxyHeaders`
      already explains the threat model, it just isn't surfaced in deployer-facing docs), and a
      worked example for at least one well-known proxy; (b) an integration test that actually
      runs a real reverse proxy (e.g. nginx or Traefik in a Docker container, alongside this
      project's existing `Dockerfile`/`README-docker.md` conventions) in front of a real running
      `opentea` server and asserts the server sees the correct client IP/scheme through it, not
      just through hand-constructed headers.
- [ ] TLS cert/key rotation isn't automatic (no SIGHUP reload or filesystem watch) — a
      certificate renewal (e.g. via certbot) requires a `systemctl restart opentea.service` to
      pick up the new files. No ACME/Let's Encrypt integration either.
- [x] ~~Dockerfile to run the server~~ — done (2026-08-04): `Dockerfile` (multi-stage,
      `golang:1.26-alpine` builder + `alpine:3.20` runtime, static `CGO_ENABLED=0` build since
      `modernc.org/sqlite` is pure Go, non-root `opentea-server` user, built-in `HEALTHCHECK`),
      `.dockerignore`, `README-docker.md`. Reuses the same `TEA_*` config surface as the
      systemd deployment rather than inventing a new one. Only `cmd/opentea` is containerized
      (not the client/fixtures/bundlecheck dev tools). Build-tested and runtime-verified
      end-to-end by the user: image builds clean, `createadmin` + named-volume persistence
      work, server starts with correct config, `/tea/v1/products` responds, runs as non-root,
      healthcheck reports healthy. (`docker build` prints a harmless BuildKit provenance
      warning about not finding git commit info -- expected since `.git/` is excluded from the
      build context on purpose; documented in `README-docker.md`'s Caveats section.)
- [ ] No image publishing/registry/CI pipeline for the Docker image yet -- local `docker build`
      only.

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
