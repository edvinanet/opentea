# syntax=docker/dockerfile:1
#
# Testing-only variant of the main ../Dockerfile: adds the fixtures loader
# (cmd/fixtures) and the reference test-data scenarios (testdata/fixtures/),
# plus a one-shot `opentea-seed` command to populate an already-running
# container via `docker exec`. NOT for production use -- see
# ../README-docker.md's "Pre-loadable test data" section. Build from the
# repo root: `docker build -f docker/testdata.Dockerfile -t opentea-testdata .`

# ---- build stage ----
FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/

# See ../Dockerfile for why CGO_ENABLED=0 and -buildvcs=false.
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/opentea ./cmd/opentea
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/fixtures ./cmd/fixtures

# ---- runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S opentea && adduser -S -G opentea opentea-server && \
    mkdir -p /var/lib/opentea && chown opentea-server:opentea /var/lib/opentea

COPY --from=builder /out/opentea /usr/local/bin/opentea
COPY --from=builder /out/fixtures /usr/local/bin/fixtures
COPY docker/opentea-seed /usr/local/bin/opentea-seed
COPY testdata/fixtures/ /usr/local/share/opentea/testdata/
RUN chmod +x /usr/local/bin/opentea-seed

USER opentea-server
WORKDIR /var/lib/opentea
VOLUME ["/var/lib/opentea"]

ENV TEA_LISTEN_ADDR=:8080 \
    TEA_DB_PATH=/var/lib/opentea/opentea.db \
    TEA_BLOB_DIR=/var/lib/opentea/blobs \
    TEA_ROOT_URL=http://localhost:8080

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:${TEA_LISTEN_ADDR#*:}/tea/v1/products || exit 1

ENTRYPOINT ["/usr/local/bin/opentea"]
