# syntax=docker/dockerfile:1
#
# Builds and runs the opentea server only (cmd/opentea) -- the reference
# client/fixtures/bundlecheck tools are dev-time CLIs, not part of a running
# deployment, so they're intentionally not included here. See
# README-docker.md for usage and README-deploy.md for the non-container
# (systemd/FHS) deployment path.

# ---- build stage ----
FROM golang:1.26-alpine AS builder
WORKDIR /src

# Cache module downloads in their own layer, separate from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/

# CGO_ENABLED=0 produces a fully static binary: the only dependency that
# would otherwise need cgo is the SQLite driver, and this project uses
# modernc.org/sqlite (pure Go) specifically to avoid that.
#
# -buildvcs=false: .git/ is deliberately excluded from the build context
# (see .dockerignore) to keep it out of image layers, so Go's automatic VCS
# stamping has nothing to find here -- this silences the resulting warning
# rather than leaving it looking like something's wrong.
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/opentea ./cmd/opentea

# ---- runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S opentea && adduser -S -G opentea opentea-server && \
    mkdir -p /var/lib/opentea && chown opentea-server:opentea /var/lib/opentea

COPY --from=builder /out/opentea /usr/local/bin/opentea

USER opentea-server
WORKDIR /var/lib/opentea
VOLUME ["/var/lib/opentea"]

# Same TEA_* config surface as the systemd deployment (internal/config,
# packaging/systemd/opentea.env.example) -- override any of these, or mount
# a KEY=VALUE config file and set TEA_CONFIG_FILE, per docs/README-deploy.md.
ENV TEA_LISTEN_ADDR=:8080 \
    TEA_DB_PATH=/var/lib/opentea/opentea.db \
    TEA_BLOB_DIR=/var/lib/opentea/blobs \
    TEA_ROOT_URL=http://localhost:8080

EXPOSE 8080

# Exercises both the HTTP server and DB access (GET /tea/v1/products needs
# no auth). Assumes plain HTTP on TEA_LISTEN_ADDR's port -- if you enable TLS
# (TEA_TLS_CERT_FILE/TEA_TLS_KEY_FILE), disable or override this healthcheck,
# since it doesn't attempt HTTPS.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:${TEA_LISTEN_ADDR#*:}/tea/v1/products || exit 1

ENTRYPOINT ["/usr/local/bin/opentea"]
