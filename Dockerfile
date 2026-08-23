# syntax=docker/dockerfile:1.7

# ─── builder ───────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download && go mod verify

COPY . .

# Build identity (CTFG-64). Stamped into the binary and reported by /healthz,
# which Switchyard's delivery reconciler polls to record what is actually
# running (the SWY-192 contract, rolled out by SERV-128).
#
# Both default to EMPTY rather than to a placeholder. An ARG that is declared
# but never passed expands to an empty string anyway, so the only question is
# what the binary does with it — and `internal/version` maps empty back to
# "dev"/null. A placeholder default would instead put a literal like "docker"
# in the delivery ledger where a version belongs.
#
# There is deliberately no default of the release version: an image built
# outside the release workflow must not be able to claim it is a release.
#
# VERSION is bare semver, no "v" prefix — it is compared with strict equality
# against org.opencontainers.image.version, which metadata-action stamps bare.
# GIT_SHA is the full 40-char commit; /healthz reports it verbatim rather than
# abbreviating.
#
# Declared here, immediately before the build, rather than at the top of the
# stage: an ARG invalidates every layer below it, and GIT_SHA changes on every
# release build. Higher up it would re-run `go mod download` each time instead
# of restoring it from cache.
ARG VERSION=
ARG GIT_SHA=
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w \
      -X github.com/Einlanzerous/centrifuge/internal/version.Version=${VERSION} \
      -X github.com/Einlanzerous/centrifuge/internal/version.Commit=${GIT_SHA}" \
    -o /centrifuge ./cmd/centrifuge

# ─── runtime ───────────────────────────────────────────────────────────────
FROM alpine:3
WORKDIR /app

RUN apk add --no-cache wget tini && \
    addgroup -S centrifuge && \
    adduser -S -G centrifuge centrifuge

COPY --from=builder /centrifuge /centrifuge

USER centrifuge
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q -O /dev/null "http://localhost:8080/healthz" || exit 1

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["sh", "-c", "/centrifuge migrate && exec /centrifuge"]
