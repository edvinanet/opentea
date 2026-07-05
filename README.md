# opentea

A reference implementation set for the [CycloneDX Transparency Exchange API (TEA)](https://github.com/cyclonedx/transparency-exchange-api) (spec v0.4.0, Beta 2): a server, a client, and shared test data, built for interoperability testing — the server should work with any conformant client, and the client should work against any conformant server, not just each other.

## Scope (Phase 1)

- The spec-conformant, read-only consumer API at `/tea/v1/...` — Products, Product Releases, Components, Component Releases, Collections, Artifacts, CLE (lifecycle) data, and TEI discovery.
- An unofficial internal ingestion API at `/admin/v1/...` for loading data (there is no publisher/write API in the official TEA spec yet), plus a browser admin GUI at `/admin/ui/...` — user accounts, roles, a stats dashboard, and API tokens. Both are authenticated; see [README-admin.md](README-admin.md).
- A blob server at `/files/{sha256}` serving uploaded artifact/distribution files.

Metadata is stored in SQLite; uploaded files are stored content-addressed on the local filesystem, behind a small `Storage` interface (`internal/storage`) so a different backend (e.g. S3) could be swapped in later.

The trust/evidence-bundle overlay and a real publisher API are out of scope for this phase.

Alongside the server: a reference client library + CLI (`pkg/teaclient`, `cmd/teaclient`) that
works against any conformant TEA server, and a fixtures replay tool (`cmd/fixtures`) with
reference test data (`testdata/fixtures/`) for interoperability testing — see
[README-client.md](README-client.md). Also a product import/export bundle format for backup,
ownership transfer, and provider migration (`internal/bundle`, admin-only `/admin/v1`
endpoints), with a standalone validator CLI (`cmd/bundlecheck`) — see
[docs/bundle-format.md](docs/bundle-format.md).

## Building

```bash
make all           # or just `make build` -- builds everything: server + consumer + tools
make build-server   # just bin/opentea
make build-consumer # just bin/teaclient
make build-tools    # just bin/fixtures, bin/bundlecheck
sudo make install   # installs everything to /usr/local/bin (override with PREFIX=... or BINDIR=...)
make help           # list all Makefile targets
```

## Running

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
| `TEA_LISTEN_ADDR` | `:8080` | HTTP(S) listen address |
| `TEA_DB_PATH` | `data/opentea.db` | SQLite database file |
| `TEA_BLOB_DIR` | `data/blobs` | Uploaded file storage directory |
| `TEA_ROOT_URL` | `http://localhost:8080` | This server's own root URL, used in discovery responses and generated file URLs |
| `TEA_VERSIONS` | `0.4.0` | Comma-separated list of supported TEA API versions, reported by discovery |
| `TEA_ORG_NAME` | *(none)* | Organisation name; shown in the admin GUI and `GET /admin/v1/stats` when set |
| `TEA_TLS_CERT_FILE` / `TEA_TLS_KEY_FILE` | *(none)* | If both are set, the server listens with HTTPS instead of plain HTTP. Setting only one is a startup error |
| `TEA_CONFIG_FILE` | `/etc/opentea/opentea.conf` | Path to the config file itself. A missing *default* path is fine (skipped); an explicitly-set path that's missing/malformed is a startup error |

See [SMOKE_TEST.md](SMOKE_TEST.md) for a full walkthrough of creating and reading back data.
See [README-deploy.md](README-deploy.md) for the config file format and systemd deployment.

See [README-deploy.md](README-deploy.md) for deploying on Debian with systemd (unit files under
`packaging/systemd/`).

## Project layout

```
cmd/
  opentea/           server entrypoint (+ `createadmin` CLI subcommand)
  teaclient/          reference CLI, built on pkg/teaclient
  fixtures/            generic fixture-replay tool, built on the existing /admin/v1 API
  bundlecheck/          standalone bundle validity checker (no server/DB needed)
pkg/                 importable from outside this module (unlike internal/...)
  tea/                 JSON-facing wire types + enum constants matching spec/openapi.yaml
  teaclient/            reference client library for the /tea/v1 read API
internal/
  model/             admin-domain types only (User, Stats, roles) -- not part of the TEA spec
  db/                 SQLite connection + migrations
  repo/                data-access layer (one file per aggregate)
  storage/              content-addressed blob storage interface + filesystem impl
  authn/                 shared session-cookie/bearer-token resolution (used by api, admin, webadmin)
  api/                   spec-conformant read API handlers (/tea/v1)
  admin/                  unofficial ingestion API handlers (/admin/v1), now auth-gated
  webadmin/                browser admin GUI (/admin/ui), server-rendered html/template
  files/                   blob-serving handler (/files/{sha256})
  bundle/                  product import/export bundle format (see docs/bundle-format.md)
  httpx/, pagination/, idgen/, config/   shared helpers
testdata/fixtures/   reference test data for cmd/fixtures
packaging/systemd/   Debian systemd unit + example environment file (see README-deploy.md)
docs/bundle-format.md   product import/export bundle specification + JSON Schema reference
Makefile             build/install/test all four cmd/ binaries (`make help` for targets)
```

## Testing

```bash
make test        # go test ./...
make check         # fmt-check + vet + test -- what CI/a pre-commit hook should run
```

Repo-layer unit tests cover pagination boundaries, collection versioning, and per-owner CLE event sequencing. `cmd/opentea/integration_test.go` drives the full admin-ingestion-then-read-API flow end to end (see `SMOKE_TEST.md` for the same flow as `curl` commands).
