# syntax=docker/dockerfile:1
# SPDX-License-Identifier: BSD-2-Clause
# SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden
#
# Builds and runs opentea-publisher (cmd/openteapublisher) -- a separate
# service from opentea itself (design/publisher-service.md §17: own binary,
# own database, own deployment, sharing this repo only for code reuse and
# integrated testing). Mirrors ../Dockerfile's own structure and reasoning
# throughout; see that file's comments for anything not re-explained here.
# Build from the repo root:
#   docker build -f docker/openteapublisher.Dockerfile -t openteapublisher .

# ---- build stage ----
FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/

# See ../Dockerfile for why CGO_ENABLED=0 (modernc.org/sqlite is pure Go)
# and -buildvcs=false (.git/ is excluded from the build context).
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/openteapublisher ./cmd/openteapublisher

# ---- runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S openteapublisher && adduser -S -G openteapublisher openteapublisher-server && \
    mkdir -p /var/lib/openteapublisher && chown openteapublisher-server:openteapublisher /var/lib/openteapublisher

COPY --from=builder /out/openteapublisher /usr/local/bin/openteapublisher

USER openteapublisher-server
WORKDIR /var/lib/openteapublisher
VOLUME ["/var/lib/openteapublisher"]

# Same OPENTEAPUBLISHER_* config surface as running the binary directly
# (cmd/openteapublisher/config.go) -- override any of these, see
# ../README-docker.md's opentea-publisher section.
ENV OPENTEAPUBLISHER_LISTEN_ADDR=:8090 \
    OPENTEAPUBLISHER_DB_PATH=/var/lib/openteapublisher/openteapublisher.db \
    OPENTEAPUBLISHER_ROOT_URL=http://localhost:8090

EXPOSE 8090

# GET /login is the only unauthenticated route this app has (no public,
# DB-touching endpoint like opentea's own /tea/v1/products) -- confirms the
# process is up and serving HTTP, not that the database is reachable.
# Assumes plain HTTP -- disable or override if TLS is enabled
# (OPENTEAPUBLISHER_TLS_CERT_FILE/KEY_FILE).
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:${OPENTEAPUBLISHER_LISTEN_ADDR#*:}/login || exit 1

ENTRYPOINT ["/usr/local/bin/openteapublisher"]
