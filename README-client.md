# Reference client, fixtures loader, and shared types

This covers the interoperability-testing tools built alongside the server: the shared wire
types (`pkg/tea`), the reference client (`pkg/teaclient` + `cmd/teaclient`), and the fixtures
replay tool (`cmd/fixtures`) with its reference test data (`testdata/fixtures/`).

`make build` builds all three binaries (server included) into `./bin` in one go; the `go
build` commands below build just one at a time if that's all you need.

## `pkg/tea` — shared wire types

`pkg/tea` holds the JSON-facing structs matching `spec/openapi.yaml` (Product, ProductRelease,
Component, ComponentRelease, Collection, Artifact, CLE, etc.) plus the spec's enum values as
string constants (`tea.ArtifactTypeBOM`, `tea.ChecksumTypeSHA256`, ...). It's importable from
outside this module (unlike `internal/...`) so it can be shared by the server, this client, and
any future publisher without duplicating the wire format. Everything here is kept in this one
repo/module for now, but `pkg/tea` has no dependency on any other package in this repo, so it
(and `pkg/teaclient`) could move to their own repo later without untangling anything.

## `pkg/teaclient` — reference client library

A Go client for the TEA consumer read API. Works against **any** conformant TEA server, not
just this repo's own reference server — that's the point, it's for interoperability testing.

```go
client := teaclient.NewClient("https://example.com/tea/v1",
    teaclient.WithBearerToken("optional-token"), // only if the server supports/requires one
)

product, err := client.GetProduct(ctx, uuid)
page, err := client.QueryProducts(ctx, teaclient.ListParams{PageSize: 50})
all, err := teaclient.ListAll(func(token string) ([]tea.Product, bool, string, error) {
    resp, err := client.QueryProducts(ctx, teaclient.ListParams{PageToken: token})
    return resp.Results, resp.HasNext, resp.NextPageToken, err
})

// Downloads an artifact format's file and verifies it against every checksum
// the server declared for it -- the concrete interoperability check the
// spec's checksum field exists for.
data, err := client.DownloadAndVerify(ctx, artifact.Formats[0])
```

Errors from any method are `*teaclient.APIError` (status code + raw body) on non-2xx HTTP
responses; `teaclient.IsNotFound(err)` / `IsUnauthorized(err)` / `IsBadRequest(err)` check for
the common cases.

Every read endpoint in the spec has a corresponding method: `GetProduct`/`QueryProducts`,
`GetProductRelease`/`QueryProductReleases`, `GetComponent`/`QueryComponents`,
`GetComponentReleaseWithCollection`/`QueryComponentReleases`, the collection getters
(latest/list/by-version, for both product releases and component releases),
`GetLatestArtifact`/`GetArtifactByVersion`, the four `GetCLEBy*` owner-type getters, and
`Discover`.

`Discover` queries a single, already-known server's own `/discovery` endpoint. For the full
TEI-authority bootstrap flow — extract authority from the TEI, fetch its `.well-known/tea`
document, try each listed server in priority order — use `BootstrapDiscover` instead, which
needs no server URL at all, just a TEI:

```go
result, err := teaclient.BootstrapDiscover(ctx, "tei://products.example.com/uuid/d4d9f54a-...",
    teaclient.WithBearerToken("optional-token"),
)
// result.ServerURL is the versioned base URL that answered, e.g.
// "https://api.teaexample.com/v1.0.0"; result.Info is Discover's own result from that server.
```

`BootstrapDiscover` picks the highest TEA API version both this client (`teaclient.SupportedVersions`)
and a candidate endpoint support (SemVer 2.0.0 precedence), tries endpoints in priority order (a
401/403 from one stops the whole attempt rather than trying the next, per spec), and retries
transient failures (5xx, network errors — not 4xx or TLS certificate failures) with backoff. It
also does a best-effort HTTPS/SVCB (RFC 9460) DNS lookup for the `.well-known/tea` fetch itself,
falling back silently to the authority's own host on port 443 if none is found.

`cmd/teaclient discover <tei>` uses this automatically when `-server` is omitted; give `-server`
to keep the older, single-server-only behavior.

Built against the TEI URL syntax (`tei://<domain>/<type>/<id>`) from
`CycloneDX/transparency-exchange-api` PR #261, **unmerged** at the time this was written — the
prior URN syntax (`urn:tei:<type>:<domain>:<id>`) isn't accepted. See `TODO.md` for what's
deliberately out of scope (ECH config, IP hints, alias chains beyond one hop, and — a real,
separate gap this surfaced — opentea's own server doesn't yet serve at the `/v{version}/` path
this flow constructs, so it can't bootstrap-discover opentea itself yet).

**Known limitation**: checksum verification supports MD5, SHA-1/256/384/512, SHA3-256/384/512,
and BLAKE2b-256/384/512 (all available from the stdlib or the already-present
`golang.org/x/crypto` module). BLAKE3 is not implemented (no stdlib or x/crypto
implementation without adding a new dependency) — `DownloadAndVerify` returns an explicit
"unsupported algorithm" error for it rather than silently skipping it.

## `cmd/teaclient` — reference CLI

```bash
go build -o teaclient ./cmd/teaclient
```

Every subcommand needs `-server=<url>` (the TEA server's read-API root, e.g.
`http://localhost:8080/tea/v1`). Flags can appear anywhere on the command line (before or
after positional arguments). `-token=<bearer>` attaches an `Authorization` header; `-json`
prints raw JSON instead of a plain Go-syntax dump.

```bash
teaclient discover urn:tei:uuid:acme.example.com:widget-1.0.0 -server=$BASE

teaclient get product <uuid> -server=$BASE
teaclient get product-release <uuid> -server=$BASE
teaclient get component <uuid> -server=$BASE
teaclient get component-release <uuid> -server=$BASE          # -> component-release-with-collection
teaclient get product-release-collection <uuid> [-version=N] -server=$BASE
teaclient get component-release-collection <uuid> [-version=N] -server=$BASE
teaclient get artifact <uuid> [-version=N] -server=$BASE

teaclient list products -server=$BASE [-all] [-id-type=PURL -id-value=pkg:...] [-page-size=50]
teaclient list product-releases -server=$BASE
teaclient list components -server=$BASE
teaclient list component-releases -server=$BASE

teaclient verify artifact <uuid> [-version=N] -server=$BASE    # downloads + checksum-verifies every format

teaclient check -server=$BASE [-sample=5]                      # see below
```

`check` is a **deliberately modest first cut** at an interoperability/conformance report: it
walks every reachable product → release → component → component release, re-verifies
pagination by paging through with the cursors the server actually returns, and
checksum-verifies a bounded sample of artifacts (`-sample`, default 5). It prints counts plus
any failures and exits non-zero if it found issues. It is **not** a full spec-conformance
validator (no required-field/enum-validity/cross-reference checks yet) — deeper checks are
tracked in `TODO.md`.

## `cmd/fixtures` — generic fixture replay tool

Not the (deferred) reference publisher — a small, generic tool for loading structured JSON
fixture files into a running opentea server via the **existing** `/admin/v1` API. Useful now
for seeding demo/test data, and for loading the shared reference test-data set below regardless
of what the eventual spec-driven publisher looks like.

```bash
go build -o fixtures ./cmd/fixtures
./fixtures -server=http://localhost:8080 -username=admin -password=<password> testdata/fixtures/log4j-tomcat.json
```

It logs into `/admin/ui/login` (same as a human operator) to get a session cookie, then
replays a fixture file's `steps` in order against `/admin/v1`, stopping at the first failure
with the step index and response body.

### Fixture file format

```json
{
  "steps": [
    {"op": "POST", "path": "/admin/v1/products", "body": {"name": "Acme Widget"}, "save": "product"},
    {"op": "POST", "path": "/admin/v1/products/{{product.uuid}}/releases", "body": {"version": "1.0.0", "createdDate": "2026-07-01T00:00:00Z"}, "save": "release"},
    {"op": "UPLOAD", "path": "/admin/v1/distributions/{{dist.distributionId}}/files", "file": "widget.tar.gz", "contentType": "application/gzip"},
    {"op": "DELETE", "path": "/admin/v1/products/{{product.uuid}}"}
  ]
}
```

- `op`: `"POST"`, `"DELETE"`, or `"UPLOAD"` (multipart file upload).
- `path` and any string value inside `body` may contain `{{name.field}}` placeholders,
  resolved against a previously-`save`d response (dotted paths work, e.g. `{{x.y.z}}`).
  **Only string values support placeholders** — an int-typed request field (like an artifact's
  `version` or a CLE event's `eventId`) can't be filled from a placeholder; since freshly
  created artifacts are always `version: 1` in this server, and CLE `id`s are assigned in
  strict creation order per owner, both reference points are deterministic and can just be
  written as literal numbers instead (see `testdata/fixtures/edge-cases.json`'s `withdrawn`
  event referencing `"eventId": 2`). Tracked as a known limitation in `TODO.md`, not fixed.
- `file` (UPLOAD only) is resolved relative to the fixture file's own directory.
- `save`, if set, stores the decoded JSON response under that name for later steps.

### Reference test data — `testdata/fixtures/`

- **`log4j-tomcat.json`**: modeled on the worked examples already embedded in
  `spec/openapi.yaml` (Apache Log4j 2, Apache Tomcat) — products, releases, components,
  component releases with multiple distributions, multi-checksum artifacts, collections, and a
  TEI identifier for discovery. Uses placeholder byte content under `testdata/fixtures/assets/`
  (not real archives/SBOMs — the point is exercising the API shapes, not the archive formats).
- **`edge-cases.json`**: a pre-release product/component release, three collection *versions*
  on the same owner with different `updateReason` values, a multi-format artifact (XML + JSON),
  and a full CLE lifecycle (`released` → `endOfSupport` → `supersededBy` → `withdrawn`,
  including a support-policy definition).

Load both, then explore with `cmd/teaclient` or the admin GUI dashboard:

```bash
go run ./cmd/opentea createadmin -username=admin -password=<password>
go run ./cmd/opentea &
./fixtures -server=http://localhost:8080 -username=admin -password=<password> testdata/fixtures/log4j-tomcat.json
./fixtures -server=http://localhost:8080 -username=admin -password=<password> testdata/fixtures/edge-cases.json
./teaclient check -server=http://localhost:8080/tea/v1
```

## Testing

`pkg/teaclient` has `httptest`-based unit tests (fake server, canned responses/errors).
`cmd/teaclient/integration_test.go` and `cmd/fixtures/integration_test.go` each build a real
in-process opentea server (the same `internal/db`+`internal/repo`+`internal/api`+
`internal/admin`+`internal/webadmin` wiring `cmd/opentea` uses) and drive it with the real
client/fixtures tool — no mocks, the actual HTTP round trip end to end.
