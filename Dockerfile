# syntax=docker/dockerfile:1.7

# ─── builder ───────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /centrifuge ./cmd/centrifuge

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
