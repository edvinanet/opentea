# opentea

Note: This is at this point a lab for OEJ learning vibe coding. Play with it, but remember it's at this point
not anything useful for any type of production. /OEJ

---

OpenTEApot is a reference implementation set for the [CycloneDX Transparency Exchange API (TEA)](https://github.com/cyclonedx/transparency-exchange-api) (spec v0.4.0, Beta 2):
 - a server, OpenTEA Server. An implementation of the TEA consumer API with an admin interface and a proposed publisher interface. The server
   also includes extensions from Olle's TEA Trust Architecture for testing
 - a client, OpenTEA consumer, for downloading software transparency data using the CLI
 - a publisher, OpenTEA Publisher for manufacturers and Open Source projects. A publication workflow support server
   with a GUI. Includes the proposed publisher API client to communicate with the OpenTEA server and extensions
   from Olle's TEA Trust Architecture
 - and shared test data, built for interoperability testing — the server should work with any conformant client, and the client should work against any conformant server, not just each other.

## Scope for OpenTEA Server (Phase 1)

- The spec-conformant, read-only consumer API at `/tea/v1/...` — Products, Product Releases, Components, Component Releases, Collections, Artifacts, CLE (lifecycle) data, and TEI discovery.
- An unofficial internal ingestion API at `/admin/v1/...` for loading data (there is no publisher/write API in the official TEA spec yet), plus a browser admin GUI at `/admin/ui/...` — user accounts, roles, a stats dashboard, and API tokens. Both are authenticated; see [README-admin.md](README-admin.md).
- A blob server at `/files/{sha256}` serving uploaded artifact/distribution files.

Metadata is stored in SQLite; uploaded files are stored content-addressed on the local filesystem, behind a small `Storage` interface (`internal/storage`) so a different backend (e.g. S3) could be swapped in later.


## OpenTEA consumer

Alongside the server: a reference client library + CLI (`pkg/teaclient`, `cmd/teaclient`) that
works against any conformant TEA server, and a fixtures replay tool (`cmd/fixtures`) with
reference test data (`testdata/fixtures/`) for interoperability testing — see
[README-client.md](README-client.md). Also a product import/export bundle format for backup,
ownership transfer, and provider migration (`internal/bundle`, admin-only `/admin/v1`
endpoints), with a standalone validator CLI (`cmd/bundlecheck`) — see
[docs/bundle-format.md](docs/bundle-format.md). A `Dockerfile` builds and runs just the server
for quick local testing — see [README-docker.md](README-docker.md).

## OpenTEA Publisher

A separate, standalone service also lives in this repo: **opentea-publisher**
(`cmd/openteapublisher`, `internal/openteapublisher`) — a GUI publisher platform that signs
locally and calls a target TEA server's `/publisher/v1` (opentea's own implementation:
`internal/publisher`) as a credentialed client. Its own binary, database, and deployment; not
part of the opentea server process. See `design/publisher-service.md` §17 for why it lives here
(shared code, integrated testing) despite being architecturally separate. Currently scaffolded:
staff login and per-target credential storage only — see `TODO.md`'s **Reference publisher**
entry for what's not built yet.

## Building

```bash
make all           # or just `make build` -- builds everything: server + consumer + tools + publisher
make build-server   # just bin/opentea
make build-consumer # just bin/teaclient
make build-tools    # just bin/fixtures, bin/bundlecheck
make build-publisher # just bin/openteapublisher (a separate service, see below)
sudo make install   # installs everything to /usr/local/bin (override with PREFIX=... or BINDIR=...)
make help           # list all Makefile targets
```

## Running OpenTEA Server

```bash
go run ./cmd/opentea createadmin -username=admin -password=<a-real-password>  # once, to bootstrap
go run ./cmd/opentea
```

See [README-admin.md](README-admin.md) for the admin GUI, roles, and the full `/admin/v1` API reference.

Configuration is layered: built-in defaults < config file < environment variables (env vars
always win). All of the following are optional and can be set either way — via a `KEY=VALUE`
config file (default path `/etc/opentea/opentea.conf`, override with `TEA_CONFIG_FILE`; see
`packaging/opentea.conf.example`) or as environment variables directly:

| Variable | Default | Purpose |
|---|---|---|
| `TEA_LISTEN_ADDR` | `:8080` | HTTP(S) listen address for the consumer-facing surface (`/tea/v1` + `/files`) — or for everything, if `TEA_ADMIN_LISTEN_ADDR` is unset |
| `TEA_ADMIN_LISTEN_ADDR` | *(none)* | If set, binds a second, independent listener for the operator-facing surface (`/admin/v1` + `/admin/ui`), separately from `TEA_LISTEN_ADDR` — e.g. an internal-only interface/port for admin access while the consumer API stays on a public one. Both listeners share the same TLS cert/key pair below when TLS is on. Leave unset for today's default: one listener serving everything |
| `TEA_DB_PATH` | `data/opentea.db` | SQLite database file |
| `TEA_BLOB_DIR` | `data/blobs` | Uploaded file storage directory |
| `TEA_ROOT_URL` | `http://localhost:8080` | This server's own root URL, used in discovery responses and generated file URLs. Independent of `TEA_API_BASE_PATH` below — set this to whatever origin/path is actually externally reachable (e.g. a fronting reverse proxy's public URL), regardless of where this process itself listens |
| `TEA_VERSIONS` | `0.4.0` | Comma-separated list of supported TEA API versions, reported by discovery |
| `TEA_API_BASE_PATH` | `/tea/v1` | Path prefix this process serves the consumer read API under. Change to e.g. `/v0.4.0` for a standalone deployment (no fronting proxy) that needs to answer literally at the path TEA's discovery spec has clients construct (`<url>/v<negotiated-version>/...`); leave at the default if a reverse proxy in front of this server handles that path rewrite instead. Replaces the previous path entirely — this server does not serve both at once |
| `TEA_ORG_NAME` | *(none)* | Organisation name; shown in the admin GUI and `GET /admin/v1/stats` when set |
| `TEA_TRUST_ARCHITECTURE` | `false` | Declares this deployment's profile as "Trusted TEA" (oej's TEA Trust Architecture overlay) rather than plain TEA. Display-only in the admin GUI nav bar — does not enforce evidence-bundle requirements on any write path |
| `TEA_ACCESS_TOKEN_TTL` | `1h` | How long a `POST {TEA_API_BASE_PATH}/token`-issued access token stays valid before a client must re-exchange its API key for a new one (TEA does not define a refresh token) — see [README-admin.md](README-admin.md) §5 |
| `TEA_TLS_CERT_FILE` / `TEA_TLS_KEY_FILE` | *(none)* | If both are set, the server listens with HTTPS instead of plain HTTP. Setting only one is a startup error |
| `TEA_CONFIG_FILE` | `/etc/opentea/opentea.conf` | Path to the config file itself. A missing *default* path is fine (skipped); an explicitly-set path that's missing/malformed is a startup error |

- See [SMOKE_TEST.md](SMOKE_TEST.md) for a full walkthrough of creating and reading back data.
- See [README-deploy.md](README-deploy.md) for the config file format and systemd deployment.
- See [README-deploy.md](README-deploy.md) for deploying on Debian with systemd (unit files under
`packaging/systemd/`).

## OpenTEApot Project layout

```
cmd/
  opentea/           server entrypoint (+ `createadmin` CLI subcommand)
  teaclient/          reference CLI, built on pkg/teaclient
  fixtures/            generic fixture-replay tool, built on the existing /admin/v1 API
  bundlecheck/          standalone bundle validity checker (no server/DB needed)
  openteapublisher/       opentea-publisher entrypoint (+ `createstaff` CLI subcommand) -- a
                            separate service, own database, see design/publisher-service.md §17
pkg/                 importable from outside this module (unlike internal/...)
  tea/                 JSON-facing wire types + enum constants matching spec/openapi.yaml
  teaclient/            reference client library for the /tea/v1 read API
  teapublisher/           shared wire types for the publisher-only delta (design/publisher-openapi.yaml)
  teapublisherclient/       reference client library for /publisher/v1
internal/
  model/             admin-domain types only (User, Stats, roles) -- not part of the TEA spec
  db/                 SQLite connection + migrations (generalized via OpenWithMigrations for
                        a second, independently-migrated database -- internal/openteapublisher's)
  repo/                data-access layer (one file per aggregate)
  storage/              content-addressed blob storage interface + filesystem impl
  authn/                 shared session-cookie/bearer-token resolution (used by api, admin, webadmin)
  api/                   spec-conformant read API handlers (/tea/v1)
  admin/                  unofficial ingestion API handlers (/admin/v1), now auth-gated
  webadmin/                browser admin GUI (/admin/ui), server-rendered html/template
  publisher/                opentea's own /publisher/v1 server implementation (see README-admin.md)
  files/                   blob-serving handler (/files/{sha256})
  bundle/                  product import/export bundle format (see docs/bundle-format.md)
  openteapublisher/         opentea-publisher's own handlers + data layer (separate database from
                              opentea's own -- see design/publisher-service.md §17)
  httpx/, pagination/, idgen/, config/   shared helpers
testdata/fixtures/   reference test data for cmd/fixtures
packaging/systemd/   Debian systemd unit + example environment file (see README-deploy.md)
docs/bundle-format.md   product import/export bundle specification + JSON Schema reference
Dockerfile            builds + runs just the server for local testing (see README-docker.md)
Makefile             build/install/test all four cmd/ binaries (`make help` for targets)
```

## Testing

```bash
make test        # go test ./...
make check         # fmt-check + vet + test -- what CI/a pre-commit hook should run
```

Repo-layer unit tests cover pagination boundaries, collection versioning, and per-owner CLE event sequencing. `cmd/opentea/integration_test.go` drives the full admin-ingestion-then-read-API flow end to end (see `SMOKE_TEST.md` for the same flow as `curl` commands).

## License

BSD 2-Clause. See [`LICENSE`](LICENSE).
