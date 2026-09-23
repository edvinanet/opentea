# TODO / follow-ups

Deferred items identified along the way — not blocking current work, but worth tracking so
they don't get lost.

## TEA spec conformance: upstream is now v1.0.0 (found 2026-09-21)
Upstream `CycloneDX/transparency-exchange-api` (`main`, commit `8635688`, 2026-09-21) has
moved from the v0.4.0 "Beta 2" this project was built against to **`info.version: 1.0.0`**
in `spec/openapi.yaml` — considerably more than a version bump. `README.md:8`,
`internal/config/config.go:144` (`TEA_VERSIONS` default `"0.4.0"`), and
`pkg/tea/types.go:5`'s header comment are all stale on the version number alone; the items
below are the substantive deltas found while diffing. The last commit this repo had any
recorded reference to was `be64bc7` (`TODO.md`'s own **BLAKE3 checksum verification** /
TEI-format entries), now many commits behind. To be worked issue by issue, not as one batch.

- [x] ~~**Artifact content download is now a standard, normative endpoint**~~ — fixed
      2026-09-21: added all four endpoints (`internal/api/artifactdownload.go`,
      `internal/api/router.go`) — `/artifact/{uuid}/latest/download`,
      `/artifact/{uuid}/{artifactVersion}/download`, and their `.../signature/download`
      counterparts. Strong `ETag`/`If-None-Match`/`304` and `Cache-Control` reuse the existing
      per-artifact `revision` counter and `s.conditional` helper (`internal/api/cachepolicy.go`)
      unchanged; `HEAD` returns identical headers with no body; `Content-Location` is the
      absolute versioned URL including `mediaType`; format selection is by `mediaType` query
      param (case-insensitive) falling back to a simple `Accept`-header match (first listed
      candidate wins — a deliberate, flagged simplification, not full RFC 9110 §12 q-value
      conformance), `Vary: Accept` set only when `Accept` actually drove the choice; `406` +
      `NO_ACCEPTABLE_FORMAT` when nothing matches; `404` + `OBJECT_UNKNOWN` for a concealed/
      nonexistent artifact, extending to the signature sub-resource too (existence-hiding);
      `404` + `SIGNATURE_NOT_FOUND` (deliberately *not* existence-hiding, per spec) for a real
      format with no signature published; `302` + `Location` for externally-hosted content,
      unconditional (never gated by `If-None-Match` — redirect sits outside conditional-request
      semantics). Registered without an HTTP-method prefix plus a manual `requireGetOrHead`
      guard (`internal/api/router.go`) — Go's `http.ServeMux` panics at registration when a
      literal path segment (`.../latest/...`) and a wildcard one (`.../{artifactVersion}/...`)
      for the same route collide across `GET`/`HEAD` patterns specifically, a real stdlib
      corner case, not a design choice.

      Alongside this, fixed `url`/`signatureUrl` to match the new external-only spec wording
      (**behavior change**, flagged explicitly): both fields are now populated only for
      genuinely external locations; self-hosted content/signatures leave them empty and are
      retrieved via the new download endpoints instead, resolved server-side from the
      `checksum` table (content, unchanged schema) and a new `artifact_format.signature_sha256`
      column (signature, `internal/db/migrations/0008_artifact_download.sql` — a single direct
      column rather than reusing the polymorphic `checksum` table, since a signature has no
      `checksums[]` array of its own). `SetArtifactFormatFile` no longer accepts/writes a `url`
      value; new `SetArtifactFormatSignatureFile` (`internal/repo/artifact.go`) mirrors it for
      signatures. **Local signature hosting is new capability** (not just external
      `signatureUrl` pass-through) — real signature-byte upload added across all three write
      surfaces: `POST /admin/v1/artifacts/{uuid}/{version}/signature`
      (`internal/admin/upload.go`), `POST /publisher/v1/artifacts/{uuid}/{version}/signature/files`
      (`internal/publisher/artifact.go`, `design/publisher-openapi.yaml` v0.11), and
      `POST /cicdapi/v1/artifacts/{uuid}/{version}/signature/files`
      (`internal/openteapublisher/cicdapi.go`, proxying to the publisher one via a new
      `pkg/teapublisherclient.UploadArtifactSignatureFile`) — unlike content upload, a
      signature upload is never blocked by an already-submitted evidence bundle, since it
      doesn't change the content checksum evidence attests to.

      Deferred, separate question (not addressed here): `release_distribution.url` has no
      TEA-hosted download endpoint at all in the spec (distributions aren't Artifacts), so
      `internal/bundle/import.go`'s existing self-referential-rewrite behavior for
      distributions (`rewriteURL`) was deliberately left untouched — `selfHostedOrURL` (new,
      same file) only replaces it for artifact formats, which do have one.

      Verified: repo-level tests for the full resolution matrix (self-hosted/external/no-
      match/not-yet-uploaded for content, `SIGNATURE_NOT_FOUND` vs `ErrNoMatchingFormat` for
      signatures — `internal/repo/artifact_test.go`); HTTP-level tests for 200/ETag/HEAD/304/
      406/404/302 and all three signature-upload surfaces round-tripped through a real download
      (`cmd/opentea/artifactdownload_test.go`, `publisher_test.go`, `cicdapi_test.go`); a manual
      smoke test against real built binaries (upload content + signature via `curl`, `-I` for
      `HEAD`, `-H 'If-None-Match: ...'` for `304`, byte-for-byte `diff` against the uploaded
      files). `go test ./... -race`/`golangci-lint`/`go vet`/`gofmt` clean on every file
      touched.
- [x] ~~**`artifact-format.signatureUrl` is now a first-class, normative spec field**~~ — fixed
      2026-09-21, together with the item above: `pkg/tea.ArtifactFormat`'s `URL`/`SignatureURL`
      doc comments now state the new external-only meaning explicitly, including that an empty
      `SignatureURL` no longer means "no signature" (it may be self-hosted — `404`
      `SIGNATURE_NOT_FOUND` on the download endpoint is the authoritative "no signature at all"
      signal).
- [ ] **Artifact download responses are missing the `Repr-Digest` header** — found 2026-09-21,
      while researching `/token` (a fresh re-read of `spec/openapi.yaml`'s `artifact-content`/
      `artifact-signature-content` response components turned this up; not caught when the
      download endpoints themselves were built, same day). `artifact-repr-digest` (RFC 9530):
      "Digest of the content... allowing a client to verify the bytes against the `checksums`
      published in the artifact metadata." Optional per its own schema (no `required: true`,
      unlike `ETag`), but not currently set at all by
      `internal/api/artifactdownload.go`'s `downloadArtifactContent`/`downloadArtifactSignature`.
      Separately, the same spec text describes the `Cache-Control` header for these responses
      as `artifact-cache-control-immutable` ("A TEA Artifact revision is immutable... long-lived
      caching with `immutable` is appropriate") — opentea's implementation deliberately uses
      `cacheControlRevalidate` instead, a conscious choice (documented in
      `artifactdownload.go`'s own comments) because opentea's create-then-upload flow means a
      revision isn't *actually* immutable here; worth a second look at whether that reasoning
      still holds or whether the create-then-upload flow itself should be tightened instead,
      but not re-litigated here.
- [x] ~~**Discovery must support `?purl=` alongside `?tei=`**~~ — fixed 2026-09-21:
      `discoveryByTEI` renamed `discovery` (`internal/api/discovery.go`), now reads either
      `tei` or `purl` (400 if neither or both are supplied, per upstream: "Exactly one of the
      tei and purl query parameters shall be provided"), resolving purl the same way tei
      already did — new `Repo.FindProductReleaseUUIDByPURL` (`internal/repo/discovery.go`,
      same shape as `FindProductReleaseUUIDByTEI`, just `id_type = 'PURL'`), same
      404+OBJECT_UNKNOWN no-match/denied-match handling. Client gained a `DiscoverByPURL`
      method (`pkg/teaclient/discovery.go`) — no `BootstrapDiscover`-style counterpart exists
      for it, since there's no `.well-known` bootstrap flow for a bare PURL (caller must
      already know which server to ask); `cmd/teaclient discover` gained a `-purl=` flag
      (requires `-server`). Verified: updated/new tests in
      `cmd/opentea/integration_test.go`/`authz_tea_test.go` (match, no-match, denied-match,
      neither/both params → 400) and `pkg/teaclient/client_test.go`
      (`TestDiscoverByPURL`/`TestDiscoverByPURLNoMatchIsNotFound`), plus a manual end-to-end
      run against the real built binaries (`teaclient discover -purl=... -server=...`
      resolving correctly, encoding round-tripped through `url.Values`/`r.URL.Query()`
      automatically — confirmed, not assumed). `go test ./... -race`/`golangci-lint` clean
      on every file touched.
- [x] ~~**Discovery no-match must return `404`, not `200` with an empty array**~~ — fixed
      2026-09-21: `discoveryByTEI` (`internal/api/discovery.go`) now returns `httpx.NotFound`
      (404 + `OBJECT_UNKNOWN`) for both a genuine no-match and an authz-denied match, aligning
      with the rest of `internal/api`'s existing existence-hiding convention. Caught and fixed
      a real, previously-untested bug in `pkg/teaclient` as a side effect: `Discover` used to
      return `([], nil)` for "this endpoint doesn't have the TEI," which
      `bootstrapDiscoverWithAuthority`'s multi-endpoint failover loop couldn't distinguish
      from a genuine (if empty) match — it silently stopped at the *first* endpoint in a
      well-known document every time, never trying a lower-priority one that actually had the
      TEI. `Discover` now surfaces a no-match as a 404 `*APIError` (`IsNotFound`), which the
      existing non-retryable-4xx failover path already handles correctly. New tests:
      `cmd/opentea/integration_test.go` (updated), `TestTeaV1AuthzDiscoveryDeniedMatchesNoMatch`
      (`authz_tea_test.go`), `TestDiscoverNoMatchIsNotFound` (`pkg/teaclient/client_test.go`),
      `TestBootstrapDiscoverFailoverOnNoMatch` (`pkg/teaclient/wellknown_test.go` — the
      regression test for the failover bug). `go test ./... -race`/`golangci-lint` clean
      (aside from two confirmed pre-existing, unrelated findings: a data race in
      `svcb_test.go`'s DNS test helper, and `cmd/opentea/main.go`'s `gocyclo` complexity —
      neither touched by this fix).
- [x] ~~**A protected object with no valid token shall answer `401`, not a concealing
      `404`**~~ — fixed 2026-09-21 (the 401-vs-404 half of the larger token-exchange item
      below, split out since it's a self-contained fix with no open design question, unlike
      the `/token` endpoint itself). Verbatim from `spec/openapi.yaml`'s `401-unauthorized`
      response component: "On a TEA server where some data is available without
      authentication, but not all, protected endpoints return `401` when no valid token is
      presented... A protected object shall not answer `404` solely because the client is
      unauthenticated." opentea's Phase 1 authz work had deliberately made "denial always
      renders as 404, never 403" (see its own commit message) without distinguishing "no
      token presented at all" from "authenticated but unauthorized" -- turned out
      `authz.Principal.IsAuthenticated()` already existed for exactly this distinction
      (used elsewhere for cache-policy/ETag partitioning), just not wired into the denial
      path yet. New `Server.writeAuthzDenial` (`internal/api/authz.go`) is now the single
      shared point both `authorize` (every single-resource GET) and `discovery.go`'s own
      hand-rolled denial branch call: anonymous + denied → 401 via new
      `httpx.UnauthorizedBearer(errCode, message)`, which also adds the RFC 6750 §3
      `WWW-Authenticate: Bearer` challenge the spec separately requires ("Servers shall
      include a `WWW-Authenticate` header") -- `error="invalid_token"` when a token was
      presented but rejected, no `error` param when none was presented at all (per RFC 6750
      §3's own guidance). Authenticated-but-unauthorized keeps the existing, deliberately
      concealing 404 unchanged -- spec's `403-forbidden` text explicitly allows that:
      "Servers may instead conceal the existence of a resource from an authenticated but
      unauthorized client by answering `404`." This is `internal/api`-only (the literal
      `/tea/v1` surface the spec governs) -- `internal/admin`/`internal/publisher`/
      `internal/webadmin` keep their own, unrelated 401 conventions (cookie-based or
      differently-scoped bearer, not RFC 6750 Bearer challenges). Caught and fixed two
      existing tests that had encoded the *old* semantics as correct
      (`TestTeaV1AuthzCapabilityIndependence`, `TestTeaV1AuthzDiscoveryDeniedMatchesNoMatch`,
      both `cmd/opentea/authz_tea_test.go`) -- both now assert 401 for the anonymous case and
      keep (or add) an authenticated-but-denied companion case proving 404 still applies
      there, so the split itself stays covered by an actual regression test, not just the
      fixed assertion. New `TestInvalidBearerTokenChallenge`
      (`cmd/opentea/integration_test.go`) covers the `error="invalid_token"` challenge shape
      specifically. Manual smoke test against the real built binary confirms all three cases
      side by side (no entitlement narrowing → 200 anonymous; narrowed + anonymous → 401 bare
      challenge; garbage token → 401 `invalid_token` challenge). `go test ./... -race`/
      `golangci-lint` clean.
- [x] ~~**`POST /token`: the actual client_credentials token-exchange endpoint**~~ — fixed
      2026-09-23. Originally deferred as future work (explicit user decision, 2026-09-21);
      history below kept for context on what was researched/decided along the way.

      **Framing from the user, load-bearing for whatever design this eventually gets**:
      authentication and authorization in TEA are optional end to end -- a server may run
      with none at all (opentea already does, by default: the bootstrap entitlement grants
      anonymous read-everything). `/token` is specifically the *recovery/discovery* path: a
      client that hits an endpoint and gets an authentication error (the `401` +
      `WWW-Authenticate: Bearer` challenge this repo's own `writeAuthzDenial`/
      `UnauthorizedBearer` now correctly emit, see the entry above) goes to `/token` next --
      and `/token` itself can trigger a full OpenIDConnect/OAuth2 workflow, not just a bare
      key-id/secret exchange. So this isn't only "add one more credential table"; it's a real
      front door that should be designed with room for a proper external-IdP flow behind it,
      even though that flow itself isn't being built yet.

      **What the spec actually requires vs. allows** (`spec/openapi.yaml`'s `/token`): HTTP
      Basic client authentication (API-key-id as user-id, API-key-secret as password)
      exchanged for a short-lived, opaque Bearer `access_token` (`token_type: Bearer`,
      `expires_in`), RFC 6749 `token-error-response` shape
      (`error`/`error_description`/`error_uri`) on failure -- `client_credentials` is the
      **required baseline** ("shall support"). SAML2-bearer/JWT-bearer **assertion grants**
      (RFC 7522/7523) -- the actual OIDC/OAuth2-fronting mechanism the user's framing points
      at -- are explicitly a "may support" extra, RFC 8693 token exchange is explicitly out of
      scope for TEA 1.0's interoperability profile. "A TEA server that requires authentication
      on any of its endpoints shall implement this endpoint" -- conditionally mandatory, not
      optional, once any entitlement narrows beyond the anonymous bootstrap (which opentea
      already supports and exercises today, so this is a real, not hypothetical, gap once
      anyone actually narrows an entitlement).

      **Distinct from opentea's current model**: today's `api_token` is a single long-lived
      secret presented *directly* as the bearer token (`internal/repo/apitoken.go`) -- no
      key-id/secret pair, no exchange step, no separate short-lived access token, no expiry.

      **Open design questions, still unresolved, to work through when this is picked up**:
      does the existing long-lived-token-as-bearer path stay usable alongside `/token`, or
      does `/tea/v1` access now require having gone through the exchange; does a new
      client-credential pair replace or sit alongside today's `api_token`/"regenerate API
      token" GUI concept; how are issued (short-lived) access tokens stored/expired/cleaned
      up, separately from the long-lived credential that mints them; how `grant_type`
      dispatch should be shaped so a later assertion-grant/OIDC handler can slot in without
      reworking the endpoint, even though building the actual IdP integration is explicitly
      out of scope for the first pass whenever it happens.

      **Additional normative detail found 2026-09-21**, reading `auth/readme.md` (upstream's
      full authentication narrative doc, alongside `spec/`, not previously read at all — see
      the "not yet checked" item below) rather than just `spec/openapi.yaml`'s `/token`
      operation description:
      - An API key is an **identifier + secret pair**, issued together (identifier need not
        be confidential, secret is) — distinct from today's single-string `api_token`.
      - **Hard rule, currently violated by opentea's own `/tea/v1` today, not just a future
        gap to design around**: "A server shall not accept an API key directly on the
        resource endpoints, and a client shall not present one there. The API key is
        exchanged for an access token, and the access token is what the resource endpoints
        see." `internal/authn.BearerUser` (used by `internal/api/auth_middleware.go`, i.e.
        `/tea/v1` itself) resolves an `Authorization: Bearer` header **directly** against
        `Repo.GetUserByAPIToken` (`internal/repo/apitoken.go`) — the long-lived secret *is*
        the bearer token presented to resource endpoints today, exactly the pattern this rule
        forbids. Not currently exploitable in practice (the only entitlement opentea ships is
        the anonymous-read-everything bootstrap — nothing today actually depends on this
        bearer path for a real access decision), but it means `/token`'s design can't just be
        "add an exchange step alongside the existing token" — the existing path itself needs
        to stop being presentable on `/tea/v1` once this is built, not stay as a silent
        parallel bypass.
      - Servers **should not** issue refresh tokens for `client_credentials` (RFC 6749
        §4.4.3) — a client holding its own long-lived API key can just call `/token` again.
      - Token responses require `Cache-Control: no-store`.
      - `401` + `WWW-Authenticate: Bearer ... error="invalid_token"` is the client's signal to
        get a fresh token and retry **once** (RFC 6750 §3.1) — not a general retry policy.
      - Formal behavior for **"servers without authentication"**: need not implement
        `/token`; **shall not** answer any resource request with `401`; **shall ignore**
        (not reject) a stray `Authorization: Bearer` header a client presents anyway. Worth
        re-checking opentea's current default (unrestricted) mode against this specifically
        when `/token` is designed, since today's `BearerUser` already treats an unrecognized
        token as simply "not present" (falls through to anonymous), which is closer to
        "ignore" than "reject" — plausibly already correct, but not yet explicitly verified
        against this exact rule.
      - `RFC 9728` Protected Resource Metadata (external-IdP discovery via
        `WWW-Authenticate`) is an optional extra, not required for the baseline.

      **Fixed 2026-09-23** (explicit user decision, replacing today's `api_token` with a real
      identifier+secret pair rather than reusing the single opaque string — see the AskUserQuestion
      recorded in this session): new `internal/db/migrations/0009_token.sql` drops `api_token`,
      adding `api_key` (`user_uuid`, `key_id`, `secret_hash`, `created_at` — one per user, same
      replace-on-regenerate semantics as before) and `access_token` (mirrors `session`'s own
      shape/indices exactly: `token_hash`, `user_uuid`, `expires_at`). New
      `internal/api/token.go`'s `requestToken` (`POST {APIBasePath}/token`): `r.BasicAuth()` +
      `url.QueryUnescape` per RFC 6749 §2.3.1 for the key-id/secret, `r.ParseForm()` for the
      `application/x-www-form-urlencoded` body, `client_credentials`-only (anything else →
      `400` `unsupported_grant_type`), `Repo.VerifyAPIKey` → `401` `invalid_client` +
      `WWW-Authenticate: Basic realm="tea"` on failure (RFC 6749 §5.2), success mints via
      `Repo.CreateAccessToken` (TTL: new `TEA_ACCESS_TOKEN_TTL` config var, default 1h) and
      responds via new `httpx.TokenIssued` (`Cache-Control: no-store`, TEA 1.0's own
      `token-response` shape — new `pkg/tea/token.go`). **The actual fix for the found gap**:
      `internal/authn.BearerUser`'s one lookup switched from the old `GetUserByAPIToken` to
      `GetAccessTokenUser` — a raw API key secret is no longer ever accepted as a `/tea/v1`
      bearer credential, only a freshly-exchanged access token is (regression-tested,
      `cmd/opentea/token_test.go`'s `TestTokenExchangeAPIKeyNeverWorksAsBearer`). One real
      integration bug found and fixed along the way: `resolvePrincipal`
      (`internal/api/auth_middleware.go`), which wraps the whole `/tea/v1` mux, would have
      rejected every `/token` request outright — it treated any non-`Bearer` `Authorization`
      header (i.e. every legitimate `Basic` credential `/token` itself expects) as a malformed
      bearer attempt and returned `401` before `requestToken` ever ran; fixed by exempting the
      `/token` path from that check specifically, since it authenticates callers by an entirely
      different, self-contained scheme. GUI (`internal/webadmin`): "API Token" renamed "API Key"
      throughout (nav, page copy, `token.html` now shows both the key ID and secret with a
      `/token` exchange example, not a bare bearer-ready value); `README.md`/`README-admin.md`
      updated to the real two-step flow (generate key → exchange at `/token` → use the returned
      access token, noting re-exchange is required after `TEA_ACCESS_TOKEN_TTL` since TEA
      defines no refresh token). Deliberately out of scope, not silently dropped: assertion
      grants/OIDC federation (see the new, separate TODO item below, added per explicit request
      this same session), multiple API keys per user (pre-existing, separate backlog item), RFC
      9728 Protected Resource Metadata, and re-verifying the "servers without authentication"
      ignore-vs-reject nuance noted above (unchanged pre-existing behavior, not part of this
      fix). Verified: repo-level tests (`internal/repo/apitoken_test.go` — key generate/verify/
      regenerate-invalidates-old, wrong secret, unknown key id; new `accesstoken` coverage —
      valid/unknown/expired); HTTP-level tests (`cmd/opentea/token_test.go` — full exchange
      round-tripped through a real `/tea/v1` resource call, the API-key-never-works-as-bearer
      regression, wrong secret, missing Basic header, bad `grant_type`); a manual smoke test
      against the real built binary (generate a key via the live GUI, exchange it via `curl`,
      use the access token against `/tea/v1/products`, confirm the raw secret is rejected there,
      confirm `Cache-Control: no-store` on the token response, confirm regenerating a key
      invalidates the old key/secret pair immediately while an already-issued access token from
      it keeps working until its own natural expiry). `go build`/`go vet`/`gofmt`/full suite/
      `go test ./... -race`/`golangci-lint` clean (one pre-existing, unrelated DNS test race in
      `pkg/teaclient/svcb_test.go`, confirmed unrelated multiple times earlier this session).

      `signatures/signature.md` (the other previously-unread upstream doc) was also read in
      full: it's explicitly non-normative ("the framework for digital signatures will not be
      mandatory for API compliance"), mostly an outline with several empty placeholder
      sections. It proposes a `/trust-anchors/` endpoint for downloading PEM-encoded PKI trust
      anchors, but that's prose only — confirmed zero matches for `trust-anchor` anywhere in
      `spec/openapi.yaml`, so nothing implementable exists yet. Relevant background for, but
      not in conflict with, oej's separately-maintained `tea-trust-architecture`/
      `internal/trust` overlay (already a deliberate non-official-spec extension per its own
      design doc) — no action needed now.
- [ ] **Federated identity via OpenID Connect, once the `client_credentials` baseline exists**
      (user request, 2026-09-22) — `/token`'s `grant_type` dispatch (see above) should leave
      room for this without rework, but the actual integration (delegating identity to an
      external IdP — Keycloak, or something lighter-weight) is separate, larger follow-on
      work, not part of the baseline `/token` build. `auth/readme.md`'s framing: a server
      delegating identity still issues its *own* TEA access token from its *own* `/token`
      endpoint via the `urn:ietf:params:oauth:grant-type:jwt-bearer` assertion grant (RFC
      7523) — the external provider authenticates the user, the TEA server still decides
      what that identity may see (today's `authz.Principal{UserUUID}`-scoped entitlement
      model would need a real mapping from "externally-authenticated identity" to a
      `UserUUID`, or its own subject shape, which doesn't exist yet). Options to weigh when
      this is picked up: run/depend on a full IdP like Keycloak (heavier, but standards-
      compliant and offloads user management entirely) vs. a minimal in-process JWT-bearer
      verifier against a configured issuer/JWKS (lighter, less deployment overhead, but more
      code to get right — token validation, audience/issuer checks, key rotation). No
      decision made yet; picking this up needs its own research pass.
- [x] ~~**`error-response` schema is now strict**~~ — fixed 2026-09-21, with a real
      correction to the original finding: `additionalProperties: false` still applies, but
      re-reading the actual spec text turned up an important nuance the first pass missed --
      "any 4xx from a resource endpoint *may* carry an `error-response` body... clients shall
      not require a body." A typed body is optional everywhere, not mandatory, so this was a
      genuine interoperability improvement to make, not a strict compliance gap to close.
      `unknown-error-type` is a six-value enum: `OBJECT_UNKNOWN`, `NOT_IMPLEMENTED`,
      `NO_ACCEPTABLE_FORMAT`, `SIGNATURE_NOT_FOUND`, `INVALID_REQUEST`, `INVALID_PAGE_TOKEN`
      -- confirmed (also by re-checking) that upstream only ever references `error-response`
      from the 400/401/403/404 shared response components; 409 doesn't appear in
      `spec/openapi.yaml` at all, so opentea's own `httpx.Conflict` (used only by
      `/admin/v1`/`/publisher/v1`, never `/tea/v1`) was never in scope. `pkg/tea/types.go`'s
      `ErrorResponse` gained a `Message` field (optional, matches the schema's own "human-
      readable explanation for diagnostics") and five new `Error*` constants for the missing
      enum values; the locally-invented, never-emitted `ErrorObjectNotShareable` was removed.
      New `httpx.BadRequestTyped(w, errType, message)` writes the real shape --
      **scoped to `internal/api` only** (the literal `/tea/v1` surface `spec/openapi.yaml`
      governs): every one of its ~26 `httpx.BadRequest` call sites now calls this instead,
      mapped to `INVALID_REQUEST` except the two page-token-specific pagination checks
      (`INVALID_PAGE_TOKEN`). `internal/admin`/`internal/publisher`/`internal/webadmin` keep
      the plain, untyped `BadRequest` unchanged -- none of their APIs are bound to this
      schema (admin explicitly isn't part of the spec at all; publisher is its own,
      independently-designed draft). `httpx.Unauthorized` (401) deliberately stays untyped
      too, on purpose, not by oversight: the six-value enum has no entry for "missing/invalid
      credential," and forcing an ill-fitting one would be worse than the optional-body
      allowance the spec itself grants. Verified: extended `TestErrorResponses`
      (`cmd/opentea/integration_test.go`) to assert the actual `error` value on every 400
      case, including a new page-token-specific one, plus a manual smoke test against the
      real built binary confirming `/tea/v1`'s new shape, `/tea/v1`'s unchanged 404/401
      shapes, and `/admin/v1`'s unchanged plain-message convention side by side.
      `go test ./... -race`/`golangci-lint` clean.
- [ ] ~~**`checksum-type` dropped `MD5`**~~ **on hold (2026-09-21), per explicit user
      decision** — upstream now lists `SHA-1, SHA-256, SHA-384, SHA-512, SHA3-256/384/512,
      BLAKE2b-256/384/512, BLAKE3`; opentea's `pkg/tea/enums.go:33-45` still has
      `ChecksumTypeMD5`. Not removing it: MD5's presence in `checksum-type` is itself under
      discussion upstream and may come back in a later spec revision — wait for that to
      settle rather than churn the enum twice.
- [x] ~~**New `compliance-document-type` enum, entirely absent locally**~~ — fixed
      2026-09-21: 21 `ComplianceDocumentType*` constants added to `pkg/tea/enums.go`
      (`SOC_2_TYPE_I`/`II`, `SOC_3`, `ISO_27001`/`27017`/`27018`/`27701`/`42001`, `PCI_DSS`,
      `HIPAA`, `FEDRAMP`, `GDPR`, `CSA_STAR`, `NIST_800_53`/`171`, `CMMC`, `HITRUST`,
      `TISAX`, `CYBER_ESSENTIALS`/`_PLUS`, `EU_DECLARATION_OF_CONFORMITY`). The scoping rule
      ("shall not be used on products, product releases, distributions, or CLE events") is
      now enforced too, in one shared place: `internal/repo/identifier.go`'s
      `insertIdentifiers` (every Product/ProductRelease/Component/ComponentRelease/
      Distribution Create+Import path already funnels through it) gained
      `validateComplianceDocumentIdentifier` -- `ErrComplianceDocumentWrongOwner` unless the
      owner is a component or component release, `ErrInvalidComplianceDocumentType` unless
      `idValue` is a real enum value. `internal/repo/cle.go`'s two identical
      `cle_event_identifier` insert loops got factored into one new
      `insertCLEEventIdentifiers` (a real DRY fix, not just validation plumbing) that rejects
      `COMPLIANCE_DOCUMENT` unconditionally -- CLE events are named directly in the spec
      text, no owner-type carve-out the way the generic identifier table has. Both
      `internal/admin` (opentea's own ingestion API) and `internal/publisher`
      (`/publisher/v1`) inherit correct behavior automatically since both already route
      through the same repo methods -- each handler just needed the new errors mapped to
      400 (new shared `writeIdentifierValidationError` helper, one per package, mirroring
      the existing `ErrComponentIdentifierConflict` → 409 precedent). `internal/bundle`'s
      import needed *no* code changes at all: `Import` already runs a manifest in one
      transaction and `internal/admin/bundle.go` already maps any `Import` error to 400, so
      a bad `COMPLIANCE_DOCUMENT` identifier in an imported bundle was already going to fail
      cleanly once the repo-layer check existed. **Deliberately backend-only** (explicit
      decision): neither `internal/webadmin` (browse-only, no create forms exist at all;
      confirmed its identifier rendering is already fully generic --
      `{{.IDType}}: {{.IDValue}}` -- and needed zero changes, verified against a real running
      server) nor `internal/openteapublisher` (§18.4's Components screen is still unbuilt
      intent) got GUI work this pass; both existing JSON write APIs are already fully usable
      today. Verified: new tests in `internal/repo/identifier_test.go` (valid on
      component/componentRelease, wrong-owner on product/productRelease/distribution via
      both Create and Import, invalid `idValue`, forbidden-on-CLE-event via both Create and
      Import), `cmd/opentea/compliancedocument_test.go` (same behavior through `/admin/v1`
      and `/publisher/v1` HTTP, confirming each handler's new error-mapping actually fired,
      not just the repo logic), `internal/bundle/compliancedocument_test.go` (a bundle
      carrying an invalid identifier fails import cleanly), plus a manual smoke test against
      the real built binary (created via `/admin/v1`, rendered correctly on the existing,
      unmodified `/admin/ui` component detail page, found via the existing, unmodified
      `GET /tea/v1/components?idType=...&idValue=...` `IDFilter` mechanism, wrong-owner
      rejected with a clear 400). `go test ./... -race`/`golangci-lint` clean.
      `design/publisher-service.md` §7.6 corrected to match (v0.26) -- it previously claimed
      `COMPLIANCE_DOCUMENT` was "attachable to a product/release/component/component-release
      the same way" as CPE/PURL/TEI, which predates and is now contradicted by TEA 1.0's
      scoping rule.
- [ ] **`server-info.versions` / `TEA_VERSIONS` needs a version bump** — upstream now requires
      full SemVer 2.0.0 strings, no leading `v` (e.g. `["1.0.0"]`). Do this *last*, once the
      conformance work above actually lands — bumping the advertised version before the server
      behaves like it should would be actively misleading to clients.
- [x] ~~**Not yet checked, flagged rather than assumed clean**~~ — checked 2026-09-21, all
      four sub-areas skipped in the initial diff pass:
      1. `discovery/tea-well-known.schema.json` vs. `pkg/tea.WellKnownDocument` — diffed
         field-by-field against a fresh fetch. Exact match (`schemaVersion`, `endpoints[].url`/
         `versions`/`priority`), and `pkg/teaclient`'s consumption already conforms to every
         behavioral rule the schema states: rejects `schemaVersion != 1`
         (`wellknown.go:194`), rejects an empty `endpoints` list, defaults absent `priority`
         to 1, sorts by priority descending, and builds the versioned URL exactly as
         specified ("append `/v` followed by the exact matched version string"). No changes
         needed. One incidental finding, not a new issue — `pkg/teaclient/semver.go:20`'s
         `SupportedVersions = []string{"0.4.0"}` is the same stale version string as the
         `TEA_VERSIONS` item above; fold it into that bump when it happens, not a separate
         item.
      2. The CLE/ECMA-428 schema family (`cle-event-type`, `cle-version-specifier`,
         `cle-event`, `cle-support-definition`, `cle-definitions`, `cle`) — diffed
         field-by-field against a fresh fetch. Every field present on `pkg/tea`'s
         `CLEEvent`/`CLEVersionSpecifier`/`CLESupportDefinition`/`CLEDefinitions`/`CLE`, all
         nine `cle-event-type` enum values match `enums.go` exactly. Behavioral rules also
         hold: `events` ordered by `id` descending (`internal/repo/cle.go`'s `getCLETx`,
         `ORDER BY id DESC`); ids are never renumbered (no delete/reassign code path exists
         at all — `CreateCLEEvent` only ever assigns `MAX(id)+1`); the withdrawn-event
         consistency rule ("include a withdrawn event if, and only if, they include the event
         it withdraws") holds trivially, since opentea never selectively omits events from
         the response in the first place (the optional per-release version-scoping filter the
         spec allows, but doesn't require, isn't implemented either — legal, a `MAY`). No
         changes needed.
      3. Whether `spec/TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md` needs reconciling
         against `/token` — it doesn't, by the doc's own design: §2.1 explicitly lists "how a
         bearer token is initially acquired" as **out of scope**, which is exactly what
         `/token` defines. The two documents already have a clean boundary (this one governs
         what happens *after* a token is presented — normalization, capabilities,
         entitlements, policy evaluation; `/token` governs how that token is obtained) rather
         than overlapping or conflicting. No reconciliation needed.
      4. Upstream's `auth/readme.md` and `signatures/signature.md` — both read in full (were
         previously not read at all). `auth/readme.md` turned out to be substantially more
         detailed and load-bearing than `spec/openapi.yaml`'s `/token` operation text alone
         suggested, including one rule opentea's *current* behavior already runs against —
         folded into the **`POST /token`** entry above rather than duplicated here.
         `signatures/signature.md` is explicitly non-normative and mostly an empty outline;
         nothing actionable, see the `/token` entry above for the one relevant detail (its
         proposed, not-yet-real `/trust-anchors/` endpoint).
- [x] ~~`spec/publisher/` (the write-API draft `design/publisher-service.md` explicitly
      designed independently of, calling it superseded)~~ — checked, still stale upstream
      (last touched 2026-01-16, cosmetic terminology commits only). No reconciliation needed;
      the decision to treat it as superseded remains sound.
- [ ] **Product Release / Component Release UUIDs must be disjoint within an authoritative
      domain** — found 2026-09-23, diffing a newer local upstream checkout
      (`~/transparency-exchange-api`, `f5817aa`, 2026-09-22) against the `8635688` snapshot
      this session's earlier conformance work was based on. New normative rule (#329,
      `doc/tea-uuid-scope.md`): "within an authoritative domain, a UUID MUST NOT be used as
      both a Product Release UUID and a Component Release UUID" — because a Collection
      inherits its parent release's UUID, a collision there would mean two distinct
      Collections sharing one identity. (The rest of that local checkout's diff against
      `8635688` — dozens of `operationId` renames, path/tag reordering, typo/formatting
      fixes, `#313`/`#312` — is pure OpenAPI metadata with no wire-protocol effect,
      confirmed not actionable for this Go implementation; `auth/readme.md` is unaffected,
      the one upstream-merged "oej/auth" PR in that range is itself just the spelling-fix
      commit already read this session, confirmed byte-identical.)

      Checked against opentea: **already satisfied by construction** on the normal
      create path — `internal/repo/productrelease.go` and `componentrelease.go` both mint
      their UUID via the same random `idgen.New()` (UUIDv4), so the collision probability
      between the two tables is the standard, negligible UUIDv4 birthday bound, exactly
      what the upstream doc's own text expects ("generators that produce globally unique
      UUIDs... satisfies the rule in ordinary practice"). **One real, narrow gap**: bundle
      import (`Repo.ImportProductRelease`/`ImportComponentRelease`) takes an explicit,
      externally-supplied UUID with no check against the *other* release type's table —
      a crafted (or accidentally colliding) bundle could violate this rule today. Low
      practical severity (import is already admin-gated, and this needs either malice or a
      genuine UUIDv4 collision to trigger), but a real gap, not just a theoretical one.
      Not fixed yet — flagged for a future pass, possibly bundled with other import-
      validation hardening.
- [x] ~~**`Collection`'s wire field is `createdDate`, not `date`**~~ — fixed 2026-09-23,
      found during a thorough audit of the import/export bundle format (user request) that
      diffed the core Product/Collection/Artifact schemas field-by-field against the fresh
      local upstream checkout for the first time this session — every earlier conformance
      pass had focused on well-known/CLE/auth/discovery/artifact-download instead. Upstream
      had already renamed this before the `8635688` snapshot this whole effort started from,
      so the bug predates this session's TEA 1.0 work entirely and was simply never caught.
      `pkg/tea.Collection.Date` (`json:"date"`) renamed to `CreatedDate`
      (`json:"createdDate"`) to match; the DB column itself stays named `date`
      (`internal/db/migrations/0001_init.sql`) since that's an internal detail with no wire
      exposure, no migration needed. Updated every call site
      (`internal/repo/collection.go`, `collectiondraft.go`, `internal/bundle/import.go`,
      `internal/webadmin/evidence.go`, plus tests) and `internal/bundle/schema.json`'s
      `collection` `$def` (property name and `required` list both said `date`). Verified:
      `cmd/opentea/integration_test.go`'s new `TestCollectionWireFieldIsCreatedDate` asserts
      against the *raw* JSON body (a decode-based assertion wouldn't have caught the
      original bug either — `json.Unmarshal` silently drops an unrecognized key regardless
      of which side is wrong), plus a manual smoke test against the real built binary
      (fetched a live collection, confirmed `createdDate` present and `date` absent;
      exported a product, confirmed the bundle's `manifest.json` carries `createdDate` and
      passes `bundlecheck`; imported that bundle into a second fresh server, confirmed a
      clean round-trip). `go test ./... -race`/`golangci-lint`/`go vet`/`gofmt` clean.

      **Other findings from the same bundle-format audit, still open** (not part of this
      fix, recorded here so they aren't lost): (1) `artifact.createdDate` is now
      spec-required but `/admin/v1/artifacts` allows creating one without it (the publisher
      path always sets it via `&now`, the admin path doesn't default it); (2)
      `product-release.product` (parent UUID) is spec-required but
      `internal/repo/productrelease.go` has a nullable-DB-column code path that can omit it
      from the wire response — worth checking whether that's ever actually reachable or just
      defensive coding for a column that's always populated in practice; (3)
      `docs/bundle-format.md`'s "File URLs" section is stale — it claims import always
      rewrites content URLs to `<dest>/files/<sha256>`, true only for distributions now;
      artifact-format content instead leaves `url` empty (self-hosted, retrieved via the
      download endpoint) since this session's earlier artifact-download work; (4)
      `internal/bundle/schema.json`'s `checksum.algValue` doesn't enforce the hex
      pattern/length upstream's `checksum` schema now specifies; (5) the bundle schema
      doesn't carry `evidenceBundle`/`evidenceBundleRef` on `collection`/`artifactFormat` --
      **not** an official-spec gap (those fields are oej's own tea-trust-architecture
      extension, not in the upstream standard at all), but a latent self-consistency gap
      that's harmless only because evidence isn't wired into the normal Get path yet (Phase
      1 scope boundary, `internal/trust`).

## Project rename: OpenTEA → OpenTeapot
- [ ] **Rename the project** (decided 2026-08-28) — "OpenTEA" turned out to already be in use
      by another, unrelated project. New name: **OpenTeapot** — the user has already
      registered `openteapot.org`. Checked before settling on it: `openteapot` is free on npm;
      the bare GitHub organization name `OpenTeapot` is already taken
      (`github.com/OpenTeapot`, hosting a repo called `otp`, AGPL-3.0), but that org looks
      abandoned (zero stars/forks/issues, last activity January 2021, no relation to
      TEA/SBOM/supply-chain) and doesn't block using `openteapot` as a repo name under an
      existing namespace (e.g. `github.com/oej/openteapot`) — only the bare top-level org name
      is unavailable.
      **Deliberately not done yet** — explicit decision to track and defer, not execute as a
      side effect of registering the domain. When it happens, it's a large, invasive,
      cross-cutting change, not a quick find-replace:
      - Go module path (`github.com/oej/opentea` in `go.mod` and therefore every single `.go`
        file's import statements across the whole repo — `internal/*`, `pkg/*`, `cmd/*`).
      - Binary names (`opentea`, `teaclient`, `bundlecheck`, `fixtures` — `Makefile`,
        `cmd/*/main.go`, `README*.md`'s build instructions).
      - Docker image name/tags and `Dockerfile`/`README-docker.md`.
      - Every `README*.md`, `docs/*.md`, and now `design/*.md` — many of these reference
        `opentea`/`OpenTEA` by name dozens of times each (e.g. `design/publisher-service.md`
        alone reasons about "opentea" as a concrete example implementation throughout its
        text, not just in code references).
      - `spec/TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md`'s own "Implementation
        Profile for OpenTEA" section (Sec 27) names opentea specifically by name.
      - Config var prefixes are `TEA_*` (e.g. `TEA_LISTEN_ADDR`), not `OPENTEA_*` — these
        don't necessarily need to change (they reference the *spec* name TEA, not the project
        name OpenTEA/OpenTeapot), but worth an explicit decision either way when this is
        scoped, not an assumption.
      - The `TEA_TRUST_ARCHITECTURE` nav-bar badge text ("Default TEA"/"Trusted TEA",
        `internal/webadmin/templates/layout.html`) — same question: does "TEA" there mean the
        spec (stays) or was it ever meant as the project name (wasn't, per the naming above,
        but worth confirming during the actual rename pass).

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
- [ ] **Server telemetry and stats** (found 2026-08-27, while designing the TEA publisher's
      own resource-limit needs — see `design/publisher-service.md` §15.1's used-vs-unused
      storage distinction). `GET /admin/v1/stats` / `internal/model.Stats` only tracks entity
      counts (`Products`/`ProductReleases`/`Components`/`ComponentReleases`/`Collections`/
      `Artifacts`) — confirmed by reading the struct directly. No storage byte totals at all
      (used — referenced by a collection — vs. unused/orphaned would need to be distinguished,
      not just a single number), and no access/request counts anywhere (per-artifact download
      counts, per-endpoint request volume) — confirmed via `internal/httpx/requestid.go`,
      which only does request-id correlation for logging/audit, not counting. A real,
      independent operational need this surfaced, not just a publisher-design dependency:
      capacity planning (how much blob storage is actually in use, and how much of that is
      genuinely orphaned) and basic usage visibility (what's actually being fetched, by
      whom, how often) both need this today and have nothing.
      Related, narrower, already tracked separately: the ETag feature's own observability
      gap (below, "No observability was added for this feature") is scoped to that one
      feature's `200`-vs-`304`/latency metrics specifically, not general server stats — don't
      conflate the two. The bare **Promotheus API endpoint for metrics** entry below (under
      **Deferred phases**) is the likely *delivery* mechanism for whatever gets built here
      (and for the ETag metrics too), not a substitute for designing what the stats actually
      are first — that design work is what this entry is actually about.

## Deferred phases (large, not started)
- [x] **Publisher API** — the official CycloneDX TEA spec has no publisher/write API
      defined yet (post-1.0 per the spec's own roadmap). This project's own draft protocol
      is now fully designed (`design/publisher-openapi.yaml`, `design/publisher-service.md`
      §8) and no longer treats `/admin/v1` as its stand-in — normal manufacturer
      publication should eventually use `/publisher/v1` instead
      (`design/opentea-server.md` §8.3/§8.4). **opentea's own server-side implementation
      shipped 2026-08-30** (`internal/publisher`, mounted at `/publisher/v1` on the
      management-plane listener; `internal/db/migrations/0007_publisher.sql`): the full v1
      protocol surface (product/component/release/CLE creation, artifact create/upload/
      evidence prepare+submit, collection-draft put/get/delete/approve/reject/prepareCommit/
      cancelPrepare/commit, both product-release- and component-release-owned), gated by a
      new `publisher_credential` bearer-token model with two scopes (`full`/`cicd` —
      `design/publisher-service.md` §10.4's split, structurally enforced, not just
      conventional), admin-issued via `/admin/v1/publisherCredentials`
      (`internal/admin/publishercredential.go`). Two pragmatic decisions made during
      implementation, not settled spec facts — worth revisiting: (1) CLE events bucketed
      under `full` scope, not named explicitly in §10.4's cicd/full split; (2) draft/lock/
      approval expiry defaults (168h/1h/24h — §11 open question #14 had none) are this
      implementation's own choice, configurable via `TEA_PUBLISHER_DRAFT_TTL`/
      `TEA_PUBLISHER_LOCK_TTL`/`TEA_PUBLISHER_APPROVAL_TTL`. Known gap: `commitCollectionDraft`
      doesn't yet write an `admin_audit_log` entry the way every other admin mutation does
      (design/publisher-openapi.yaml's commit summary says "records one audit entry") —
      skipped for the same reason `evidence_bundle.created_by` stays NULL for
      publisher-credential-authenticated writes: there's no `user` row to attribute it to
      until the approval-actor-identity item below is resolved. `pkg/teapublisherclient`/
      `cmd/teapublisherclient` remain on hold (separate item below).

      **External security review 2026-08-28** (`docs/security-review-publisher-design-260828.md`,
      reviewed the design docs before this implementation existed) found 16 issues; 3 fixed
      2026-08-31 (findings 1, 12, 14 — file-upload-after-evidence lock, component identifier
      uniqueness, `mediaType`-based format addressing). Findings 2 and 13 were already
      tracked below/in `design/publisher-service.md` §11 before the review. Remaining
      findings are their own entries below, each prefixed **[security review finding N]**
      for cross-reference.
- [ ] **[security review finding 3] Publisher API: explicit prepare→commit transaction
      binding** — `prepareCollectionCommit`/`commitCollectionDraft` bind to each other only
      implicitly, via the draft's own `lock_expires_at`/`lock_date` columns
      (`internal/repo/collectiondraft.go`), not an explicit `prepareId`/nonce the OpenAPI
      documents. Traced through each concrete failure mode the review named (commit after
      lock expiry, after `cancelPrepare`, without a prior prepare, approval expiring between
      prepare and commit) — all are already correctly rejected by the lock mechanism, so
      this is a documentation/explicitness gap, not an open vulnerability. Worth doing
      before calling the protocol stable: add a real `prepareId` to
      `prepare-commit-response`, require it in `commit`, document the binding explicitly
      instead of leaving it implicit in the reference implementation's own choices.
- [ ] **[security review finding 4] Publisher API: create a new version of an existing
      artifact** — `createArtifact` always mints a fresh UUID at version 1
      (`internal/repo/artifact.go`'s `CreateArtifact`); there's no `/publisher/v1` operation
      to add version 2 under an existing artifact identity, even though
      `design/publisher-service.md` §3's own SBOM-correction scenario assumes this is
      possible. Needs a design pass: version allocation/concurrency semantics, which
      metadata carries forward, whether a new format set on an existing version is a new
      version or a pre-finalization update (finding 4's own framing).
- [ ] **[security review finding 5] Publisher API: byte-exact signing format specification**
      — `digestToSign`/`signatureValue` say "sign the digest bytes" but leave real
      ambiguity open: ASCII hex vs. decoded bytes, whether JWS/CMS wrap the digest or the
      canonical JSON, required protected headers/signed attributes, accepted algorithm
      identifiers. opentea's own Phase 1 only ever produces/verifies one concrete case
      (raw Ed25519 over hex-decoded digest bytes, `internal/trust.Sign`/`Verify`), so this
      hasn't bitten anything yet, but a second implementation (or opentea's own Web
      PKI/HSM mode, `design/publisher-service.md` §9.5) would have nothing precise to
      interoperate against. Needs published byte-exact test vectors, not just prose.
- [ ] **[security review finding 6] Publisher API: finer-grained capability model** — the
      shipped `full`/`cicd` credential scopes (`internal/publisher`, `model.PublisherScope*`)
      are a coarse first cut, not the per-operation capability vocabulary the review
      proposes (`artifact.evidence.submit`, `collection.approve`, etc., mirroring
      `internal/authz`'s existing read-side capability naming). Revisit once a real
      deployment needs something between "can do everything" and "can do everything except
      approve/reject and identity-defining creates."
- [ ] **[security review finding 7] Publisher API: evidence storage level doesn't match the
      signing flow** — `prepareArtifactEvidence`/`submitArtifactEvidence` sign the artifact
      as a whole, but the consumer spec's `evidenceBundle`/`evidenceBundleRef` extension
      fields live on `artifact-format`, not `artifact` (`pkg/tea/trust.go`,
      `internal/repo/evidencebundle.go`'s `OwnerType` is `"ARTIFACT"`, an
      opentea-internal choice, not a spec-defined owner type). Unclear for a multi-format
      artifact (e.g. SBOM as both XML and JSON) whether one signature covers every format
      or only one. Resolve by either moving evidence to the `artifact` object at the spec
      level, or scoping prepare/submit per-format with a stable format identifier (also
      needed for finding 14's fix to go further than upload addressing).
- [ ] **[security review finding 9/10] Publisher API OpenAPI draft: completeness and
      schema-reuse accuracy** — `design/publisher-openapi.yaml` still omits
      component/componentRelease CLE endpoints and the componentRelease collection-draft
      path variants (explicitly marked as omitted-for-brevity, not a scope decision —
      `internal/publisher`'s actual implementation already covers all of these). Separately,
      the review found a structural comparison shows 21 of the 24 schemas the document
      calls "reused verbatim" from the consumer spec actually differ (e.g. the publisher
      `uuid`/`date-time` schemas drop the consumer schema's format patterns) — some
      differences are editorial, some weaken validation. Needs a pass reconciling the draft
      against both what's actually implemented and the consumer spec's real schemas.
- [ ] **[security review finding 11] Publisher API: caller-supplied creation timestamps**
      — `productRelease-create`/`release-create`'s `createdDate` and CLE's `published` are
      required request fields a caller can backdate or future-date; the consumer spec
      describes `createdDate` as server-assigned. opentea's own implementation already
      does the right thing for artifacts (`internal/publisher/artifact.go`'s `createArtifact`
      sets `createdDate` to `time.Now()`, ignoring any caller input — the OpenAPI schema
      never exposed it there in the first place) but not for releases, which still take
      the caller's value as-is. Decide whether `createdDate` should become
      server-assigned everywhere (breaking bundle-import's need to set a historical value)
      or stay caller-supplied with the field explicitly redefined as a manufacturer
      assertion, not a target-authoritative fact.
- [ ] **[security review finding 13] Publisher API: idempotency for write operations** —
      already an open question before the review (`design/publisher-service.md` §11 #13,
      §14.4), but the review confirms it's now also a real gap in the shipped
      `internal/publisher`: retried `createProduct`/`createArtifact` calls allocate a new
      identity each time, retried `submitArtifactEvidence` would hit
      `ErrFingerprintReused` (a confusing error for an intended retry), and a lost
      `commitCollectionDraft` response leaves the caller with no way to check whether
      publication actually happened. Needs `Idempotency-Key` support and a defined replay
      window before CI/CD integrations can retry safely.
- [ ] **[security review finding 15] Publisher API: standard error-response schema** —
      `internal/publisher` handlers mostly reuse `httpx`'s existing `{"message": ...}` shape
      (`BadRequest`/`Conflict`/etc.), which doesn't distinguish, say, a stale-digest 400 from
      a bad-signature-format 400 by machine-readable code. Needs a real problem/error schema
      (stable code, correlation id, retryability) before automated clients can branch on
      failure reasons instead of string-matching messages.
- [x] **[security review finding 8]** N/A against the shipped implementation — the review's
      concern was a phased rollout that commits unsigned collections before signing becomes
      mandatory. `internal/publisher`'s `commitCollectionDraft` was built signed-only from
      the start (always verifies evidence before persisting, `repo.CommitCollectionDraft`);
      there was never an unsigned-commit code path to begin with.
- [x] **[security review finding 16]** Considered and resolved differently than the review's
      own recommendation, with reasoning already on record — not a gap. The review suggests
      editable drafts live in the publisher platform, with the target only ever seeing an
      immutable prepare transaction. This project explicitly chose target-owned staging
      instead (`design/publisher-service.md` §4/§11 Q1, `design/opentea-server.md` §8.4),
      grounded in a real workflow the review's alternative can't support: direct CI/CD
      publication with no publisher-platform database in the loop at all, where the target
      is the only state CI/CD and the human approver share.
- [ ] **Reference publisher / `opentea-publisher`** (part of the server/client/publisher
      reference-implementation trio) — explicitly deferred by the user (2026-07-04); design
      work resumed 2026-08-31, implementation not started. The protocol is fully designed
      (`design/publisher-openapi.yaml`, `design/publisher-service.md` §8), and the shared
      library shape is settled (2026-08-29, §8/§14.1 v0.18): `pkg/teapublisher` (wire types
      for the publisher-only delta — `collection-draft`, `approval-decision`,
      `evidence-submission`, etc. — importing `pkg/tea` for the reused base objects).
      **`pkg/teapublisher` is scaffolded and in real use** (imported by opentea's own
      `internal/publisher` server implementation, see the **Publisher API** entry above).

      **Repo-location reversal, 2026-08-31** (`design/publisher-service.md` v0.21, new §17):
      the full GUI publisher platform — previously scoped as "a separate, standalone
      project" (§3/§4, through v0.20) — now lives in **this** repo too, per explicit
      direction ("the publisher code should be in this repository as it reuse a lot of the
      objects in the opentea server and we need to be able to test them together"). Still a
      separate *service* (own binary `cmd/openteapublisher`, own database, own deployment) —
      only the repository changed, not the architecture v0.3 corrected v0.2 to establish.
      Settled alongside the reversal: `pkg/teapublisherclient` (HTTP client for a target's
      `/publisher/v1`) is load-bearing again, no longer "on hold" — it's `opentea-publisher`'s
      own backend dependency now, not just a reference-CLI's library.
      `cmd/teapublisherclient` (a bare CLI for direct-to-target CI/CD use, workflow (a))
      stays on hold — this build targets workflow (b), CI/CD calling `opentea-publisher`'s
      own API endpoints, first. Also settled: signing is ephemeral-key-only for v1 (no
      persistent/HSM key storage needed yet), and the GUI queries a target live via
      `pkg/teaclient` rather than caching/mirroring product data locally. See
      `design/publisher-service.md` §17 for the full package layout, shared-code map, and
      storage scope (including one real remaining design gap: the internal multi-team
      business-approval workflow's own shape isn't designed yet, only scoped as needed).
      **`pkg/teapublisherclient` scaffolded 2026-08-31**: full client coverage of every
      `/publisher/v1` operation `internal/publisher` actually implements (products,
      components, artifacts + upload + evidence, CLE × 4 owner types, collection-draft ×
      8 operations × 2 owner types), built against the real handler code rather than the
      OpenAPI draft (which has known inaccuracies — e.g. `linkComponent`'s real response is
      `200` + the updated product release, not the draft's documented `204`). Verified two
      ways: `pkg/teapublisherclient/client_test.go`'s fake-server unit tests, and a new
      `cmd/opentea/publisherclient_test.go` end-to-end test driving the whole workflow
      (product→release→artifact→evidence→draft→approve→prepare→commit, with real Ed25519
      signatures) through the client against the *real* `internal/publisher` server — this
      is the actual payoff of moving both into one repo, catching client/server drift
      directly rather than by inspection. Zero third-party dependencies.
      **`cmd/openteapublisher`/`internal/openteapublisher` scaffolded 2026-08-31**: the
      `internal/db` migration-runner refactor landed first (new `OpenWithMigrations(path,
      migrations fs.FS, dir string)`, `Open` now a thin wrapper -- zero behavior change,
      confirmed by the existing `db_test.go` passing unchanged plus a new test proving a
      second, independent database works). `clientIP`/`loginLimiter` moved out of
      `internal/webadmin` into `internal/httpx` (`ClientIP`/`LoginLimiter`) and are now
      shared by both GUIs — a concrete instance of "create a shared library when possible."
      `internal/openteapublisher` (one package: data layer + HTTP/GUI, not opentea's
      three-way repo/admin/webadmin split — appropriately scoped for its current size) has
      its own database (`staff`, `session`, `target` tables — Layer A/B, §10.1/§10.5),
      bcrypt login mirroring `internal/repo/user.go`'s own contract, a minimal GUI (login,
      dashboard listing configured targets, add/remove a target), and `cmd/openteapublisher`
      itself (own small config, own `createstaff` bootstrap subcommand mirroring `opentea
      createadmin`). Verified: repo-layer unit tests, an HTTP-level test driving the full
      login→dashboard→add-target→list flow, a manual smoke test against the real built
      binary (`createstaff`, login via `curl`, session cookie confirmed, target added and
      listed), `make check`/`golangci-lint`/`go test -race` all clean, new `make
      build-publisher` Makefile target. **Deliberately not built** (§17.3's own flagged
      gap): the internal multi-team business-approval workflow (not designed yet -- no
      draft assembly, no CI/CD-facing JSON API against it, since there's nothing real yet
      for CI/CD to call), anything that calls `pkg/teapublisherclient` against a stored
      `Target` (plain CRUD only so far), and a Docker image (separate item, still open).
      Known limitation carried from the design: `target.bearer_token` is stored as
      plaintext, no encryption-at-rest -- this app must present the actual usable
      credential on outbound calls, unlike opentea's own `publisher_credential`, which only
      ever stores a verifier-side hash.
- [ ] **`opentea-publisher`: GUI requirements pass** (`design/publisher-service.md` §18,
      v0.22, companion to §7) maps the manufacturer process onto actual screens instead of
      protocol operations, and surfaces three concrete gaps the backend scaffold didn't:
      (1) ~~no audit log table~~ **fixed 2026-08-31** — new
      `internal/openteapublisher/db/migrations/0002_audit_log.sql` (`audit_log`, mirroring
      opentea's own `admin_audit_log` shape), `Repo.RecordAudit`/`ListAuditEntries`, wired
      atomically (new `Repo.WithTx`, matching `internal/repo.Repo`'s own composability —
      every existing method switched from `r.db` to `r.conn()` so it works both standalone
      and nested) into `createTargetForm`/`deleteTargetForm`, the only two write actions
      that exist so far. `bearer_token` is deliberately excluded from what gets written
      (`redact` helper) — the audit trail must not become a second place a live credential
      leaks. `httpx.WithRequestID` wired into the router too, so entries correlate to
      request logs. Verified: repo-level tests, and an HTTP-level test confirming the audit
      write actually happens through the real handler, not just in isolation. No GUI page
      to browse it yet (§18.12 named the requirement, not a screen) — a natural, not yet
      requested, follow-up. (2) ~~no staff role/permission concept~~ **fixed 2026-09-01** —
      new `internal/openteapublisher/db/migrations/0003_staff_roles.sql` adds `staff.role`
      (`admin`/`member`, mirroring opentea's own `user.role` shape), `RoleSatisfies`
      (mirrors `internal/authn.RoleSatisfies`), and a new `requireRole` middleware. Target
      management (a target's `bearer_token` is a real, usable credential) is now
      admin-only, closing the immediate, already-real gap this scaffold had; a new
      admin-only `/staff` page (list/create/delete staff accounts, mirroring
      `internal/webadmin/users.go`, including `DeleteStaff`'s last-admin protection) makes
      the role distinction actually exercisable via the GUI, not just schema. Both actions
      are now audited too (`staff.create`/`staff.delete`, `audit_log.target_type` widened
      to accept `'staff'`). **Does not resolve §18.9's own framing** (who may approve a
      collection draft) — that workflow still doesn't exist in the GUI; this is the schema
      groundwork it will build on, not a solution to it. Caught and fixed a real bug while
      building this: `GetSessionStaff` never selected the new `role` column, so every
      session resolved to an empty role and every admin action 403'd — found because the
      new audit-log assertion in the existing HTTP test failed, which also exposed that the
      *test's own* prior assertion (matching "Acme production" against page text) was a
      false positive the whole time, matching a static form placeholder rather than real
      created data; both are fixed. ~~(3) the
      still-undesigned CI/CD-facing API (see the **Reference publisher** entry above) needs
      its own capability scoping mirroring `/publisher/v1`'s full/cicd split, since
      `opentea-publisher` always presents a "full" credential to the target regardless of
      who's actually calling it (§18.11) — without that, the target-side scope separation
      provides no protection against a compromised `opentea-publisher` process or malicious
      CI/CD submission.~~ **scaffolded 2026-09-01** — see the new **`opentea-publisher`:
      CI/CD-facing API and capability scoping** entry below. ~~The internal multi-team
      business-approval workflow (§18.8) remains explicitly undesigned~~ **scaffolded
      2026-09-01** — see the new **`opentea-publisher`: internal business-approval
      workflow** entry below.
- [ ] **`opentea-publisher`: internal business-approval workflow** (`design/publisher-
      service.md` §18.8, v0.23) resolves the gap §17.3/§18.8 both flagged as "the single
      biggest remaining unknown" -- a scaffolded approval-request workflow, connecting §10.1's
      already-named workflow vocabulary ("release manager, component maintainer,
      security/compliance approver") that hadn't been wired to anything yet. New
      `internal/openteapublisher/db/migrations/0004_business_approval.sql`: `staff` gains a
      nullable `workflow_role` column (a second axis orthogonal to the existing admin/member
      `staff.role` -- that one gates who manages this *tool*, this one gates where a staff
      account sits in the publishing *workflow*; holding `admin` does not imply approval
      capability, deliberate separation of duties), plus `approval_request`/
      `approval_decision` tables. `internal/openteapublisher/approval.go`:
      `CreateApprovalRequest` (any authenticated staff member, no role gate -- matches "no
      drafting UI exists yet to enforce who may request"), `RecordApprovalDecision`
      implementing maker-checker exactly as §10.1 stated it (`ErrSelfApproval` if the decider
      is the requester, `ErrRequestNotPending` if already closed, a single rejection closes
      immediately, an approval closes once enough *distinct* approving staff accounts reach
      `required_approvals`, default 1). New `requireApprovalRole` middleware
      (`auth_middleware.go`) gates `POST /approvals/{uuid}/decide` on
      `workflow_role == "security_compliance_approver"` -- a separate check from `requireRole`,
      not merged into it. New `/approvals` screen (list + create + inline approve/reject),
      `staff.html` gained a workflow-role select. Verified: repo-level tests (self-approval
      rejected, decision-on-closed-request rejected, partial-then-complete approval crossing
      `required_approvals` > 1 with duplicate decisions from the same approver not
      double-counting, immediate rejection), HTTP-level tests (an admin with no approver
      workflow role gets 403 deciding, the requester gets 403/self-approval-rejected even when
      *also* holding the approver role, the approver succeeds, both create and decide are
      audited), a manual smoke test against the real built binary (admin/requester blocked,
      approver succeeds, self-approval blocked, all via `curl`), `make check`/
      `golangci-lint`/`go test -race` all clean. **Deliberately not built**: no screen
      consumes an approved request yet -- there's no "Sign & Publish" action to gate on one
      (§18.7's draft-assembly screen and §18.10 both still unbuilt); `release_uuid` is typed
      in by hand with no live `pkg/teaclient` lookup to resolve it to a human-readable label
      (deferred until there's a real draft to source a reference from, not built
      speculatively); this is a plain per-role approval count, not a true multi-team system --
      no way to require one legal sign-off *and* one security sign-off specifically, §10.1's
      own collapse of "legal, compliance, security engineering" into one
      `security_compliance_approver` role is carried through as-is, not re-litigated; no
      federated-identity role mapping (§10.1's own separately-open OIDC/LDAP claim-to-role
      question) -- `workflow_role` is set by hand today by whichever admin creates the
      account.
- [ ] **`opentea-publisher`: CI/CD-facing API and capability scoping** (`design/publisher-
      service.md` §18.11, v0.24) resolves the gap §18.11 originally surfaced:
      `opentea-publisher` always presents a "full" credential to a target regardless of who
      calls it, so the target's own full/cicd split (`internal/publisher`) gave no
      protection once CI/CD's access was mediated through `opentea-publisher`. New migration
      `internal/openteapublisher/db/migrations/0005_cicd_credential.sql`: `cicd_credential`
      -- one per target, soft-revocable, mirroring `internal/repo/publishercredential.go`'s
      own shape exactly but with no scope column (this credential type *is* cicd, there's no
      "full" variant of it). New `internal/openteapublisher/cicdcredential.go` repo layer
      (create/get-by-token/list/revoke, all mirroring
      `internal/repo/publishercredential.go` method-for-method). New `cicdapi.go` +
      `cicdapi_middleware.go`: a bearer-credential-gated `/cicdapi/v1` mux, structurally
      exposing only the operations `internal/publisher/router.go` itself treats as
      cicd-scoped -- artifact create/upload/evidence-prepare/evidence-submit, and
      collection-draft PUT/GET/DELETE/prepareCommit/cancelPrepare/commit for both owner
      types (16 endpoints total) -- product/component/CLE creation, `linkComponent`, and
      draft approve/reject are simply never registered on this mux, not runtime-scope-
      checked. Each handler builds a `pkg/teapublisherclient.Client` against the calling
      credential's associated `Target` (its real, stored, full-scoped `bearer_token`) and
      proxies straight through, replaying the target's real status/body verbatim on error
      (`teapublisherclient.APIError`, which was already documented with exactly this use in
      mind) -- this is also the first real caller of `pkg/teapublisherclient` against a
      *stored* `Target` (`TODO.md`'s own longstanding "plain CRUD only so far" gap, closed
      in the same motion). `NewRouter` now mounts two separate muxes (`/` for the GUI behind
      the existing `limitBody` 64 KiB cap, `/cicdapi/` unwrapped with its own per-handler
      limits, since `http.MaxBytesReader` can only shrink an already-applied limit, never
      raise one -- matches `internal/publisher/server.go`'s own real convention of no
      blanket body cap). New admin-only `/cicd-credentials` GUI screen to issue/revoke
      credentials -- the raw token is shown exactly once, inline, on the create response
      (never redirected away from, unlike every other create-form in this app, since only
      its hash is ever stored). Verified: repo-level tests (create/list/revoke, revoked
      tokens excluded, unknown-target create fails cleanly), an HTTP-level test proving the
      401 gate (missing/garbage/revoked token) and that full-scoped routes structurally
      don't exist even for a *valid* cicd token, and -- the real payoff proof, in
      `cmd/opentea/cicdapi_test.go` since `internal/publisher` has no test-server helper of
      its own -- a genuine three-party test: a real `internal/publisher` target, a real
      `opentea-publisher` pointed at it, and a proxied artifact-create + collection-draft
      PUT confirmed to have actually landed on the target (read back with a full-scoped
      credential afterward, not just trusting opentea-publisher's own 200). Also confirmed
      via a manual smoke test against both real built binaries with `curl`. `make
      check`/`golangci-lint`/`go test -race` all clean. **Deliberately not built**: no
      per-credential scoping *within* cicd (one credential grants every cicd-scoped
      operation against its one target -- `internal/publisher` itself doesn't sub-scope
      cicd either, so this matches, not falls short of, the thing it mirrors); no credential
      expiry/rotation reminders; ~~Docker packaging (separate, already-tracked item) still
      doesn't exist to actually run any of this in a real CI pipeline.~~ — see the new
      **`opentea-publisher`: Docker image** entry below.
- [ ] **`opentea-publisher`: Docker image** (`design/publisher-service.md` §17.7, v0.25)
      resolves that section's own deferred item: `docker/openteapublisher.Dockerfile`
      builds and runs `cmd/openteapublisher`, mirroring the existing root `Dockerfile`'s
      structure exactly (same `golang:1.26-alpine` builder + `alpine:3.20` runtime,
      `CGO_ENABLED=0`, non-root user, named-volume persistence, `HEALTHCHECK`) — placed
      under `docker/` alongside the existing `docker/testdata.Dockerfile`, both built from
      the repo root via `-f`. New `README-docker.md` "opentea-publisher" section (quick
      start, persistence, config/TLS, `docker-compose`), mirroring the existing sections'
      own shape. One real difference from opentea's own image, flagged rather than silently
      matched: there's no unauthenticated, DB-touching route to healthcheck against (no
      `opentea-publisher` equivalent of `GET /tea/v1/products`), so the healthcheck targets
      `GET /login` instead — confirms the process is serving HTTP, not that the database is
      reachable. **Not build/run-verified in this pass** — the sandboxed environment this
      was scaffolded in has no Docker daemon access (`permission denied ...
      /var/run/docker.sock`, same constraint noted when the original `Dockerfile` shipped:
      that one was "Build-tested and runtime-verified end-to-end by the user," not by the
      agent). Verified instead by a careful line-by-line comparison against that
      already-proven `Dockerfile` and a successful native `go build ./cmd/openteapublisher`.
      **A real `docker build -f docker/openteapublisher.Dockerfile -t openteapublisher .`
      and `docker run` pass from an environment with daemon access is still needed** before
      this can be marked verified.
- [ ] **Publisher API: derive approval actor from authenticated identity, not a
      caller-supplied string** (found 2026-08-28, during `design/opentea-server.md` review —
      see its §11.4). `design/publisher-openapi.yaml`'s `approval-decision.actor` is
      currently a plain client-supplied string; now that collection-draft staging and
      approval are confirmed target-owned (`design/publisher-service.md` §4/§11 Q1,
      `design/opentea-server.md` §8.4), the target genuinely enforces the maker-checker
      decision itself, so it should derive `actor`/`decidedBy` from the authenticated
      Layer B/D bearer credential's identity rather than trust an asserted field. Needs its
      own design pass (does the credential's subject claim always map 1:1 to a human actor,
      or does a shared service-account credential need an additional asserted-but-verified
      sub-identity) before touching the OpenAPI schema.
- [ ] **Publisher platform: domain-ownership verification** (found 2026-08-29, during a DNS
      access-management discussion for the publisher platform) — before letting a
      manufacturer publish under a given domain, the publisher platform needs to verify they
      actually control it. Not designed anywhere yet. The well-trodden shape (ACME DNS-01,
      Google Search Console, SPF/DKIM setup) is: publisher platform generates a random
      challenge value, manufacturer publishes it as a TXT record (or proves control via an
      HTTP file/response) at a well-known name, publisher platform polls and confirms. Needs
      its own design pass: record name/format, challenge lifetime, re-verification policy
      (domains can change hands), and how it relates to (but is distinct from) DNS trust
      anchors (TAPS, next entry) and SVCB/HTTPS discovery records.
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
