# Running opentea in Docker

A `Dockerfile` builds and runs just the server (`cmd/opentea`) — meant for quick local testing,
not as a replacement for the systemd/FHS deployment path in
[README-deploy.md](README-deploy.md) (still the recommended way to run this in production). The
reference client, fixtures, and bundlecheck tools are dev-time CLIs, not part of a running
deployment, so they aren't included in the image.

Multi-stage build: a `golang:1.26-alpine` build stage compiles a fully static binary
(`CGO_ENABLED=0` — safe here since the SQLite driver, `modernc.org/sqlite`, is pure Go with no
cgo dependency), copied into a minimal `alpine:3.20` runtime stage that runs as a dedicated
non-root user (`opentea-server`), matching the same security posture as the systemd unit.

## Quick start

```bash
docker build -t opentea .

# Bootstrap the first admin user (once) -- a named volume persists the DB/blobs
# across container runs.
docker run --rm -v opentea-data:/var/lib/opentea opentea \
    createadmin -username=admin -password=<a-real-password>

# Run the server
docker run -d --name opentea -p 8080:8080 -v opentea-data:/var/lib/opentea opentea
```

```bash
curl http://localhost:8080/tea/v1/products
# log into http://localhost:8080/admin/ui/login with the admin user from step 1
```

`docker run opentea createadmin ...` works because `ENTRYPOINT` is the `opentea` binary itself
— any arguments after the image name are passed straight through, exactly like running the
binary directly (see `README-admin.md`).

## Persisting data

Use a **named volume** (`-v opentea-data:/var/lib/opentea`, as above) rather than a host bind
mount unless you specifically need to access the SQLite file/blobs from the host — Docker
manages a named volume's ownership for you, matching the image's `opentea-server` user. A bind
mount (`-v $(pwd)/data:/var/lib/opentea`) requires the host directory to already be writable by
that user's UID/GID inside the container, or the server will fail to open its database.

## Configuration

Same `TEA_*` environment variables as the non-container deployment (see `README.md`'s
Configuration section and `internal/config`) — pass them with `-e`:

```bash
docker run -d --name opentea -p 8080:8080 -v opentea-data:/var/lib/opentea \
    -e TEA_ROOT_URL=https://tea.example.com \
    -e TEA_ORG_NAME="Acme Corp" \
    opentea
```

A `KEY=VALUE` config file (`packaging/opentea.conf.example`) works too — mount it and point
`TEA_CONFIG_FILE` at the mounted path:

```bash
docker run -d --name opentea -p 8080:8080 -v opentea-data:/var/lib/opentea \
    -v $(pwd)/opentea.conf:/etc/opentea/opentea.conf:ro \
    opentea
```

### TLS

Mount the cert/key and set `TEA_TLS_CERT_FILE`/`TEA_TLS_KEY_FILE` to their in-container paths:

```bash
docker run -d --name opentea -p 8443:8443 -v opentea-data:/var/lib/opentea \
    -v $(pwd)/tls:/etc/opentea/tls:ro \
    -e TEA_LISTEN_ADDR=:8443 \
    -e TEA_TLS_CERT_FILE=/etc/opentea/tls/cert.pem \
    -e TEA_TLS_KEY_FILE=/etc/opentea/tls/key.pem \
    opentea
```

The image's built-in `HEALTHCHECK` assumes plain HTTP and will report unhealthy against a
TLS-only container — disable it (`docker run --no-healthcheck ...`) or override it if you enable
TLS.

## Pre-loadable test data

`docker/testdata.Dockerfile` is a separate, testing-only image variant that adds the fixtures
loader (`cmd/fixtures`) and the reference scenarios (`testdata/fixtures/`) on top of the main
image, plus a one-shot `opentea-seed` command you run against an *already-running* container:

```bash
docker build -f docker/testdata.Dockerfile -t opentea-testdata .

docker run -d --name opentea-testdata -p 8080:8080 -v opentea-testdata-data:/var/lib/opentea opentea-testdata
docker exec opentea-testdata opentea-seed
```

`opentea-seed` bootstraps a fixed test-only admin login (`admin` / `admin-testdata-only` —
override with `docker exec -e OPENTEA_SEED_USERNAME=... -e OPENTEA_SEED_PASSWORD=... opentea-testdata opentea-seed`)
if one doesn't already exist, then loads both reference scenarios
(`testdata/fixtures/log4j-tomcat.json`, `edge-cases.json`) via the same `/admin/v1` API a real
publisher would use. It talks to the server over `localhost` inside the container, so there's
no need to wait for startup or expose extra ports.

```bash
curl http://localhost:8080/tea/v1/products
# log into http://localhost:8080/admin/ui/login as admin / admin-testdata-only
```

Re-running `opentea-seed` against a server that already has the data loaded creates a *second*
copy of it rather than failing — products aren't deduped the way the bundle import/export format
is (see `docs/bundle-format.md`). Start from a fresh volume (`docker volume rm
opentea-testdata-data`) if you want a single clean copy again.

This variant is never meant for production — the admin password is fixed and public in this
repo. Use the plain `Dockerfile`/`docker-compose` setup above for anything real.

## docker-compose

```yaml
services:
  opentea:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - opentea-data:/var/lib/opentea
    environment:
      TEA_ROOT_URL: http://localhost:8080
volumes:
  opentea-data:
```

Bootstrap the admin user once with `docker compose run --rm opentea createadmin -username=admin
-password=<a-real-password>` before `docker compose up -d`.

## Caveats

- `docker build` may print `WARNING: current commit information was not captured by the build:
  failed to read current commit information with git rev-parse --is-inside-work-tree` as its
  last line. This is harmless — it's BuildKit trying (and, in some environments, failing) to
  attach git commit provenance metadata to the image, unrelated to the actual build steps or the
  binary itself. If the build log shows all steps completing and `docker images` lists the
  image, the build succeeded; the warning can be ignored (or suppressed with `docker build
  --provenance=false`, though that alone doesn't always silence it).
- No image publishing/registry/CI pipeline exists yet — this is a local `docker build` only.
