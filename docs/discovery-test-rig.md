# TEA discovery bootstrap: DNS & `.well-known` test rig

A reference matrix of DNS zone configurations and `.well-known/tea` documents for testing the TEA
discovery bootstrap flow — TEI → authority → DNS (optionally SVCB/HTTPS-aware) → `.well-known/tea`
→ candidate server(s) → `/v<version>/discovery?tei=...`. This is **implementation- and
language-agnostic**: every test is defined purely in terms of DNS records, HTTP responses, and
expected behavior per the TEA discovery specification, so it can validate *any* conformant TEA
client — not tied to a specific codebase or library. Each test is numbered (`TEST-01`, `TEST-02`,
...) so it can be referred to by number in discussion, an issue, or a commit message, without
restating the whole scenario.

This is meant to become **real infrastructure**: actual DNS records under a real, controlled
domain, real certificates from a working PKI (a public CA or an internal one the test clients
trust), and real servers answering `.well-known/tea` and `/discovery`. Every hostname below uses
`example.com` as a **placeholder domain** — substitute the real domain once it's chosen, keeping
the `teaNN`/subdomain structure. IP addresses are placeholders too (drawn from the
documentation-only ranges RFC 5737 defines for IPv4 and RFC 3849 for IPv6) — substitute real
addresses once the test servers exist. DNS records are shown in standard zone-file syntax;
`.well-known/tea` documents are shown as the literal JSON body served at
`https://<authority>/.well-known/tea`.

Each test's "Expected" section describes required behavior in spec terms (what any conformant
client must do), not the internals of a particular implementation.

## Summary

| # | Domain | DNS shape | `.well-known` shape | What it exercises |
|---|---|---|---|---|
| [TEST-01](#test-01-ipv4-only-no-svcb-single-endpoint) | `tea01.example.com` | `A` only | 1 endpoint, 1 version | Baseline: no SVCB, plain IPv4 resolution |
| [TEST-02](#test-02-ipv6-only-no-svcb-single-endpoint) | `tea02.example.com` | `AAAA` only | 1 endpoint, 1 version | Baseline: no SVCB, plain IPv6 resolution |
| [TEST-03](#test-03-svcb-serviceform-redirect-to-a-different-host) | `tea03.example.com` | `A` + `HTTPS` (ServiceForm) | 1 endpoint | SVCB redirects the `.well-known` fetch to a different host:port |
| [TEST-04](#test-04-svcb-aliasform-one-hop) | `tea04.example.com` | `HTTPS` (AliasForm) → `tea04-real.example.com` | 1 endpoint | SVCB AliasForm, one-hop chase to the real target |
| [TEST-05](#test-05-single-endpoint-baseline) | `tea05.example.com` | `A` only | 1 endpoint, 1 version | Baseline: single candidate, nothing to choose between |
| [TEST-06](#test-06-multiple-endpoints-priority-failover) | `tea06.example.com` | `A` only | 2 endpoints, different priority | Highest-priority endpoint fails (5xx) → client fails over to the next |
| [TEST-07](#test-07-multiple-endpoints-different-api-versions) | `tea07.example.com` | `A` only | 2 endpoints, disjoint version sets | Client picks the endpoint sharing a version with it, skips the one that doesn't |
| [TEST-08](#test-08-one-endpoint-multiple-versions-version-negotiation) | `tea08.example.com` | `A` only | 1 endpoint, 3 versions | Client picks the *highest* mutually-supported version, not just any match |
| [TEST-09](#test-09-403-must-not-fail-over) | `tea09.example.com` | `A` only | 2 endpoints, priority-ordered | Highest-priority endpoint returns 403 → client stops, does **not** try the next |
| [TEST-10](#test-10-no-svcb-record-falls-back-cleanly) | `tea10.example.com` | `A` only, no `HTTPS` record | 1 endpoint | SVCB query returns nothing → clean fallback to `authority:443`, not an error |
| [TEST-11](#test-11-nxdomain-no-well-known-document-at-all) | `tea11.example.com` | no records at all | — | Authority doesn't resolve / has nothing → clean, permanent failure |
| [TEST-12](#test-12-tls-certificate-untrusted) | `tea12.example.com` | `A` only | 1 endpoint, but untrusted cert | Certificate chain doesn't validate → permanent, no retry |
| [TEST-13](#test-13-malformed-well-known-document) | `tea13.example.com` | `A` only | invalid JSON body | Schema-invalid response (bad `schemaVersion`, empty `endpoints`) → permanent, no retry |
| [TEST-14](#test-14-port-in-the-tei-authority-is-rejected) | `tea14.example.com:8443` | n/a — rejected before resolution | n/a | TEI authority carries a port (never allowed per spec) → client rejects it, no DNS/HTTP attempted |
| [TEST-15](#test-15-cname-on-the-tei-authority) | `tea15.example.com` | `CNAME` → `tea15-canonical.example.com`, which has `HTTPS` (ServiceForm) | 1 endpoint | Authority itself is a CNAME; `Target: "."` must resolve against the record's real owner, not the alias |
| [TEST-16](#test-16-cname-on-the-api-server) | `tea16.example.com` | `A` only | 1 endpoint whose `url` hostname is itself a `CNAME` | The *resolved API server's* hostname (not the TEI authority) is a CNAME |
| [TEST-17](#test-17-trusted-certificate-wrong-hostname-in-san) | `tea17.example.com` | `A` only | 1 endpoint | Certificate chain is trusted, but its SAN doesn't cover the requested hostname → permanent, no retry |
| [TEST-18](#test-18-well-known-endpoint-returns-a-redirect) | `tea18.example.com` | `A` only | `.well-known/tea` returns a `3xx` redirect | Redirect from the well-known endpoint → must not be followed |
| [TEST-19](#test-19-well-known-endpoint-returns-unparseable-json) | `tea19.example.com` | `A` only | `.well-known/tea` returns `200` with a body that isn't valid JSON at all | Distinct from TEST-13 (valid JSON, wrong content) → permanent, no retry |
| [TEST-20](#test-20-no-well-known-endpoint-on-the-resolved-server) | `tea20.example.com` | `A` only | `.well-known/tea` returns `404` | Authority resolves and responds, but has no discovery endpoint at all → permanent, no retry |
| [TEST-21](#test-21-well-formed-well-known-document-first-server-503-second-server-works) | `tea21.example.com` | `A` only | 2 endpoints, well-formed document | First (highest-priority) server returns exactly `503` (the spec's own named example), second works |
| [TEST-22](#test-22-non-default-port-learned-from-endpointsurl) | `tea22.example.com` | `A` only, no port | 1 endpoint, `url` has a non-default port | The spec-sanctioned way to reach a non-default port: `endpoints[].url`, not the TEI |

## TEST-01: IPv4-only, no SVCB, single endpoint

The simplest possible case: a TEI authority with a plain `A` record, no `HTTPS`/SVCB record at
all, one well-known endpoint offering one version.

**TEI**: `tei://tea01.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea01.example.com.       300  IN  A      192.0.2.10
```

**`.well-known/tea`** (served from `https://tea01.example.com/.well-known/tea`, resolved via the
`A` record above on port 443):
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea01.example.com", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: the client fetches the well-known document directly from `192.0.2.10:443`, finds
one endpoint offering one version the client itself supports, and calls
`https://api.tea01.example.com/v0.4.0/discovery?tei=...`.

## TEST-02: IPv6-only, no SVCB, single endpoint

Same as TEST-01 but with an `AAAA` record instead of `A` — confirms nothing about the bootstrap
flow assumes IPv4.

**TEI**: `tei://tea02.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea02.example.com.       300  IN  AAAA   2001:db8::10
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea02.example.com", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: identical outcome to TEST-01, over IPv6.

## TEST-03: SVCB ServiceForm, redirect to a different host

The authority publishes an `HTTPS` record (RFC 9460) redirecting the `.well-known/tea` fetch
itself to a different hostname and a non-default port — e.g. a CDN or dedicated discovery-serving
host, decoupled from wherever `tea03.example.com` itself points. Support for this is optional per
the TEA discovery spec ("if clients are known to support HTTPS/SVCB DNS records... these may be
used"); a client that doesn't implement it is expected to behave as in TEST-10 instead.

**TEI**: `tei://tea03.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea03.example.com.       300  IN  A      192.0.2.30
tea03.example.com.       300  IN  HTTPS  1 discovery.tea03.example.com. port=8443
discovery.tea03.example.com. 300 IN A    198.51.100.30
```

**`.well-known/tea`** (fetched from `https://discovery.tea03.example.com:8443/.well-known/tea` —
note the request's `Host`/TLS SNI stay `tea03.example.com`, per RFC 9460's target-name handling;
only the TCP connection target moves):
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea03.example.com", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: a client implementing SVCB/HTTPS support connects to
`discovery.tea03.example.com:8443` for the `.well-known/tea` fetch, while still presenting
`tea03.example.com` as the request `Host` and TLS SNI (so certificate validation is against the
original authority, not the SVCB target).

## TEST-04: SVCB AliasForm, one hop

The authority's `HTTPS` record is AliasForm (`SvcPriority 0`), pointing at a different owner name
that itself carries the real ServiceForm record. A well-behaved client follows exactly one hop,
not an indefinite chain (to avoid a pathological redirect loop from a misconfigured or hostile
zone).

**TEI**: `tei://tea04.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea04.example.com.        300  IN  HTTPS  0 tea04-real.example.com.
tea04-real.example.com.   300  IN  A      192.0.2.40
tea04-real.example.com.   300  IN  HTTPS  1 . port=9443
```

**`.well-known/tea`** (fetched from `https://tea04-real.example.com:9443/.well-known/tea`; the
second record's `Target` of `.` means "same name as this record's own owner", i.e.
`tea04-real.example.com` itself — not the original `tea04.example.com`):
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea04.example.com", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: the client re-queries `HTTPS` for `tea04-real.example.com` after seeing the
AliasForm record, and uses the resulting ServiceForm record. A **variant** of this test worth
keeping in the rig: point the second record's `HTTPS` at *another* AliasForm (an alias of an
alias) rather than a ServiceForm — a well-behaved client must fall back to the original authority
on plain port 443 rather than chase the chain indefinitely.

## TEST-05: Single endpoint (baseline)

A well-known document listing exactly one endpoint — nothing to prioritize or fail over between.
Included as the simplest possible well-known *document* shape, as distinct from TEST-01 (simplest
possible *DNS* shape); the two are often exercised together.

**TEI**: `tei://tea05.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea05.example.com.       300  IN  A      192.0.2.50
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea05.example.com", "versions": ["0.4.0"]}
  ]
}
```
(`priority` omitted entirely — defaults to `1` per schema; confirms the client doesn't require it.)

**Expected**: the one endpoint is used directly.

## TEST-06: Multiple endpoints, priority failover

Two endpoints at different priorities. The higher-priority one is tried first; when it fails
(modeled here as a `5xx` from its `/discovery` call, a transient failure), the client fails over
to the lower-priority one.

**TEI**: `tei://tea06.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea06.example.com.       300  IN  A      192.0.2.60
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api-primary.tea06.example.com", "versions": ["0.4.0"], "priority": 0.9},
    {"url": "https://api-secondary.tea06.example.com", "versions": ["0.4.0"], "priority": 0.2}
  ]
}
```

**Scenario**: `api-primary.tea06.example.com`'s `/v0.4.0/discovery` returns `503` (a transient
failure, reasonably retried a small bounded number of times with backoff before being given up
on); client moves to `api-secondary.tea06.example.com`, which succeeds.

**Expected**: the final result comes from `api-secondary`; `api-primary` is genuinely attempted
first (highest priority), not skipped.

## TEST-07: Multiple endpoints, different API versions

Two endpoints where only one shares a version with the client — the incompatible one must be
skipped without ever being queried, not merely deprioritized.

**TEI**: `tei://tea07.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea07.example.com.       300  IN  A      192.0.2.70
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api-legacy.tea07.example.com", "versions": ["9.9.9"], "priority": 1},
    {"url": "https://api-current.tea07.example.com", "versions": ["0.4.0"], "priority": 0.5}
  ]
}
```

(Assumes the client under test supports version `0.4.0` but not `9.9.9`.)

**Expected**: `api-legacy` is never contacted at all; `api-current` is used directly, even though
it's lower priority — version compatibility is a hard filter applied before priority ordering
matters.

## TEST-08: One endpoint, multiple versions (version negotiation)

A single endpoint advertising several versions — confirms the client picks the *highest*
mutually-supported one by SemVer 2.0.0 precedence, not merely "any" match.

**TEI**: `tei://tea08.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea08.example.com.       300  IN  A      192.0.2.80
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea08.example.com", "versions": ["0.2.0-beta.2", "0.4.0", "1.0.0"], "priority": 1}
  ]
}
```

(Assumes the client under test supports both `0.4.0` and `1.0.0`.)

**Expected**: the client picks `1.0.0` (higher SemVer precedence than `0.4.0`), so the request
goes to `.../v1.0.0/discovery?tei=...` — not `.../v0.4.0/...`, even though that's also a valid
mutual match.

## TEST-09: 403 must not fail over

The highest-priority endpoint answers with `403 Forbidden` (an authorization error, e.g. a
required bearer token was missing or rejected). Per the TEA discovery spec ("Authentication error
codes (401, 403) should not lead to failover to the next endpoint in the list"), the client must
stop immediately rather than silently trying a lower-priority endpoint.

**TEI**: `tei://tea09.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea09.example.com.       300  IN  A      192.0.2.90
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api-restricted.tea09.example.com", "versions": ["0.4.0"], "priority": 1},
    {"url": "https://api-open.tea09.example.com", "versions": ["0.4.0"], "priority": 0.1}
  ]
}
```

**Scenario**: `api-restricted`'s `/v0.4.0/discovery` returns `403`.

**Expected**: the overall discovery attempt fails, reporting an authentication/authorization
error; `api-open` is never contacted, even though it's a valid, lower-priority fallback.

## TEST-10: No SVCB record — falls back cleanly

The authority has ordinary `A`/`AAAA` records but no `HTTPS` record at all — the common case for
most deployments, since SVCB support is explicitly optional per spec. Confirms the absence of an
SVCB record is not an error, just "nothing to enhance the plain path with." This is also the
expected outcome for a client that doesn't implement SVCB/HTTPS support at all, even when a
server *does* publish one (as in TEST-03) — such a client should just proceed with plain
resolution and never notice the record exists.

**TEI**: `tei://tea10.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea10.example.com.       300  IN  A      192.0.2.100
```
(No `HTTPS` record.)

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea10.example.com", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: the client fetches `.well-known/tea` from `tea10.example.com:443` directly (no
SVCB-driven redirection), without treating the missing `HTTPS` record as an error.

## TEST-11: NXDOMAIN — no well-known document at all

The authority doesn't resolve at all (no `A`/`AAAA`/`HTTPS` records, `NXDOMAIN` for every query
type) — e.g. a mistyped TEI, or a vendor that has stopped operating the domain.

**TEI**: `tei://tea11.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**: *(no records — the zone returns `NXDOMAIN` for `tea11.example.com`)*

**`.well-known/tea`**: n/a — never reachable.

**Expected**: the client reports a clear, permanent failure to resolve/reach the authority at
all — not an indefinite retry loop, and not a confusing error attributing the failure to
something else (e.g. a TLS or content error).

## TEST-12: TLS certificate untrusted

The `.well-known/tea` endpoint is reachable and responds, but its TLS certificate's issuer isn't
trusted (e.g. self-signed, or issued by a CA the client doesn't trust). This must be treated as a
**permanent** failure — retrying doesn't fix an untrusted certificate — unlike a `5xx`, which is
transient.

**TEI**: `tei://tea12.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea12.example.com.       300  IN  A      192.0.2.120
```

**`.well-known/tea`**: served correctly, but over a connection whose certificate chain fails
validation against the client's trust store.

**Expected**: the client fails immediately, with no retries.

## TEST-13: Malformed well-known document

The `.well-known/tea` endpoint answers `200 OK` with a body that fails validation — either the
wrong `schemaVersion` or an empty `endpoints` list. A successful HTTP response with bad *content*
is a different failure class from a `5xx`: the server answered fine, so retrying the same request
won't change what it sends back.

**TEI**: `tei://tea13.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea13.example.com.       300  IN  A      192.0.2.130
```

**`.well-known/tea`** (variant A — wrong schema version):
```json
{"schemaVersion": 2, "endpoints": []}
```

**`.well-known/tea`** (variant B — no endpoints):
```json
{"schemaVersion": 1, "endpoints": []}
```

**Expected**: both variants are rejected immediately, with no retries.

## TEST-14: Port in the TEI authority is rejected

The TEI itself must never carry a port. Per the TEA discovery specification's "Port resolution"
section: "Currently, the port number is not part of the TEI but it is needed to connect to the
API. The TEA API server may be hosted on any port, but the server that is part of the first step
of discovery will by default be running on the default HTTPS port 443." A non-default port is
learned exactly two spec-sanctioned ways — an HTTPS/SVCB DNS record redirecting the initial
`.well-known/tea` fetch (TEST-03), or an `endpoints[].url` in the well-known document itself
(TEST-22) — never from the TEI string. A TEI that embeds a port anyway is malformed input to
reject, not a capability for the client to honor.

**TEI**: `tei://tea14.example.com:8443/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**: n/a — a conformant client must reject the TEI before attempting any resolution.

**`.well-known/tea`**: n/a — never reached.

**Expected**: the client rejects the TEI as malformed, with a clear parsing error, before issuing
any DNS query or HTTP request. It must not silently strip the port and proceed as if it had been
absent, and must not connect to port 8443 (or any port) on the strength of the TEI string alone.

## TEST-15: CNAME on the TEI authority

The TEI authority itself is a `CNAME`, not an owner of any records directly (per DNS rules, a
name with a `CNAME` can't also hold other record types at that same name — a resolver answering
an `HTTPS` query for it returns the `CNAME` plus whatever `HTTPS` record(s) actually exist at the
canonical target name). This is a **DNS-level** indirection, distinct from SVCB's own
**application-level** AliasForm indirection (TEST-04) — both can be present in the same lookup,
independently of each other.

**TEI**: `tei://tea15.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea15.example.com.             300  IN  CNAME  tea15-canonical.example.com.
tea15-canonical.example.com.   300  IN  A      192.0.2.150
tea15-canonical.example.com.   300  IN  HTTPS  1 . port=7443
```

A resolver answering `HTTPS? tea15.example.com` returns both records above in one answer set: the
`CNAME`, and the `HTTPS` record — whose own owner name is `tea15-canonical.example.com.`, **not**
`tea15.example.com.` — with `Target: "."`.

**`.well-known/tea`** (fetched from `https://tea15-canonical.example.com:7443/.well-known/tea`):
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea15.example.com", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: `Target: "."` resolves against the record's *own owner name*
(`tea15-canonical.example.com`, taken from the DNS response itself), not the originally-queried
authority (`tea15.example.com`).

**Implementation warning worth testing for explicitly**: it's easy to get this subtly wrong by
tracking "the name I queried" as a local variable and resolving `Target: "."` against *that*,
rather than against the actual record's owner name as returned by the DNS response. Those two are
identical in the non-CNAME case (TEST-03, TEST-04), which can hide the bug until a real CNAME'd
authority is tested against — exactly this scenario. A client with that bug would silently connect
to the *original alias* name instead of the canonical target, which may not even resolve, or
worse, may resolve to something unrelated.

## TEST-16: CNAME on the API server

The TEI authority resolves normally (no CNAME, no SVCB record — the simple case), but the
`.well-known/tea` document's `endpoints[].url` names a host that is itself a `CNAME` — e.g.
pointing at a load balancer or CDN front door for the actual API backend. This is resolved at a
different point in the flow than TEST-15: not while fetching `.well-known/tea`, but afterward,
when the client makes its final `/v<version>/discovery` call against the *resolved endpoint's*
own hostname.

**TEI**: `tei://tea16.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea16.example.com.             300  IN  A      192.0.2.160
api.tea16.example.com.         300  IN  CNAME  api-backend.tea16.example.com.
api-backend.tea16.example.com. 300  IN  A      198.51.100.160
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea16.example.com", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: the client's final call to `https://api.tea16.example.com/v0.4.0/discovery?tei=...`
resolves `api.tea16.example.com` through its `CNAME` to `api-backend.tea16.example.com`
transparently — ordinary DNS resolution, no special handling required or expected. Useful mainly
as a sanity check that nothing in a client's endpoint-connection logic bypasses normal name
resolution (e.g. by caching or pinning an IP address from an earlier, unrelated lookup).

## TEST-17: Trusted certificate, wrong hostname in SAN

The `.well-known/tea` endpoint's certificate chains to a trusted issuer (unlike TEST-12), but the
certificate's Subject Alternative Name doesn't cover the hostname actually being requested. This
is a **different failure inside certificate verification** than TEST-12: chain trust and hostname
matching are separate checks, and a real client bug could pass one while failing to properly
enforce the other. Both failures must ultimately be treated the same way by the client —
permanent, no retry — but a test rig that only ever exercises the untrusted-chain case could miss
a client that (incorrectly) skips hostname verification.

**TEI**: `tei://tea17.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea17.example.com.       300  IN  A      192.0.2.170
```

**`.well-known/tea`**: served over a connection whose certificate is issued by a trusted CA, but
whose only SAN is a *different* hostname — e.g. `wrong-host.example.com` — not
`tea17.example.com`.

**Expected**: the client fails immediately, with no retries, the same as TEST-12.

**Setup note for whoever provisions this test against real infrastructure**: build this with a
real DNS hostname as the TEI authority, not a bare IP literal. TLS's Server Name Indication (SNI,
RFC 6066) is only meaningful for DNS names — a client connecting to a literal IP address
typically sends no SNI at all, which can cause an SNI-routing test server to select the *wrong*
certificate (or its default one) rather than deliberately serving the mismatched one this test
needs, and produces a different underlying error ("no IP SANs") than the DNS-hostname-mismatch
this test is meant to exercise. Using a real hostname (as every other test in this rig already
does) avoids the ambiguity entirely.

## TEST-18: Well-known endpoint returns a redirect

The `.well-known/tea` endpoint itself responds with an HTTP redirect (`3xx`, e.g. `302 Found`)
pointing at a second server — e.g. a vendor consolidating discovery hosting behind a different
domain after the fact, or a simple `http`→`https` upgrade redirect left in place by mistake on an
endpoint that must already be HTTPS-only.

**TEI**: `tei://tea18.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea18.example.com.       300  IN  A      192.0.2.180
```

**`.well-known/tea` response**: `302 Found`, `Location: https://tea18-alt.example.com/.well-known/tea`.

**Expected**: **redirects must not be followed for this fetch.** The client fails with a clear,
immediate, permanent error — not a silent (and, if naively implemented, potentially broken)
attempt to reach the redirect's target. This combines badly with SVCB/HTTPS-aware clients in
particular: if a client resolves the *original* authority to a specific address (via SVCB or
plain DNS) and reuses that same connection/address for a followed redirect without re-resolving
the *new* target host, it will never actually reach the redirect's real destination — it will
just keep re-requesting the original server, which keeps re-issuing the same redirect, until
hitting whatever redirect-count limit the HTTP stack enforces, and then fail with a generic,
unhelpful "too many redirects"-style error. Whether or not a given client's HTTP stack has that
specific bug, treating any redirect from this endpoint as suspicious and refusing to follow it is
the safer default for a security-sensitive trust-bootstrap step: a `.well-known/tea` response
redirecting to a different origin should not be trusted transparently.

## TEST-19: Well-known endpoint returns unparseable JSON

The `.well-known/tea` endpoint answers `200 OK`, but the response body isn't valid JSON at
all — malformed syntax, truncated output, or an HTML error page served with a `200` status by
mistake. Distinct from TEST-13 (valid JSON that fails schema validation): this is a body that
doesn't even parse as JSON in the first place.

**TEI**: `tei://tea19.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea19.example.com.       300  IN  A      192.0.2.190
```

**`.well-known/tea` response**: `200 OK`, body `this is not json at all {{{`.

**Expected**: the client fails immediately, with no retries — the server answered successfully,
so retrying the identical request won't produce different content.

## TEST-20: No well-known endpoint on the resolved server

The TEI authority resolves fine and the server responds, but there is no `.well-known/tea`
resource configured at all — `404 Not Found` for that specific path. Distinct from TEST-11
(authority doesn't resolve/respond at all, a DNS/connection-level failure): here the server is
reachable and answering, it simply has no discovery endpoint. This is explicitly named in the TEA
discovery spec's "Common errors" section ("404 Not Found — discovery endpoint not present").

**TEI**: `tei://tea20.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea20.example.com.       300  IN  A      192.0.2.200
```

**`.well-known/tea` response**: `404 Not Found`.

**Expected**: the client fails immediately, with no retries — a `404` here means "this vendor
doesn't support TEA discovery bootstrap for this authority," not a transient condition.

## TEST-21: Well-formed well-known document, first server 503, second server works

The `.well-known/tea` document itself is entirely well-formed and reachable without incident —
the failure is downstream, at one of the *listed candidate servers'* own `/discovery` endpoints.
The highest-priority endpoint answers with exactly `503 Service Unavailable` — the spec's own
named example of a transient failure — and the client is expected to fail over to the
next-priority endpoint, which works. (See also TEST-06, which covers the same failover mechanism
with a generic `5xx`; this test exists to check the spec's literal `503` example specifically,
not just "some server error.")

**TEI**: `tei://tea21.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea21.example.com.       300  IN  A      192.0.2.210
```

**`.well-known/tea`**:
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api-primary.tea21.example.com", "versions": ["0.4.0"], "priority": 1},
    {"url": "https://api-secondary.tea21.example.com", "versions": ["0.4.0"], "priority": 0.5}
  ]
}
```

**Scenario**: `api-primary.tea21.example.com`'s `/v0.4.0/discovery` returns exactly `503`, every
time it's queried; `api-secondary.tea21.example.com`'s answers normally.

**Expected**: `api-primary` is genuinely attempted first (highest priority) — reasonably retried a
small bounded number of times with backoff, since `503` is explicitly transient — then the client
fails over to `api-secondary`, and the final result comes from there.

## TEST-22: Non-default port learned from `endpoints[].url`

The TEI authority carries no port at all (as required — see TEST-14) and publishes no
`HTTPS`/SVCB record, so the initial `.well-known/tea` fetch uses the plain default of port 443.
The well-known document's `endpoints[].url` itself names a non-default port — the spec-sanctioned
way a client ends up talking to a port other than 443, distinct from both TEST-03 (SVCB
redirecting the *`.well-known/tea` fetch itself* to a different port) and TEST-14 (rejected: a
port can never come from the TEI).

**TEI**: `tei://tea22.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1`

**DNS zone**:
```
tea22.example.com.       300  IN  A      192.0.2.220
```
(No `HTTPS` record.)

**`.well-known/tea`** (fetched from `tea22.example.com:443`, the plain default):
```json
{
  "schemaVersion": 1,
  "endpoints": [
    {"url": "https://api.tea22.example.com:9443", "versions": ["0.4.0"], "priority": 1}
  ]
}
```

**Expected**: the client fetches `.well-known/tea` from the default port 443 (no SVCB involved,
no port in the TEI), then makes its `/discovery` call to
`https://api.tea22.example.com:9443/v0.4.0/discovery?tei=...` — the non-default port comes
entirely from `endpoints[].url`, never from the TEI or a special-cased fallback.
