.PHONY: dev build test migrate docker tidy

# ── Development ───────────────────────────────────────────────────────────────

dev:
	go run ./cmd/centrifuge

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/centrifuge ./cmd/centrifuge

# ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test ./...

# ── Database ──────────────────────────────────────────────────────────────────

migrate:
	go run ./cmd/centrifuge migrate

# ── Docker ────────────────────────────────────────────────────────────────────

docker:
	docker compose up --build -d

# ── Maintenance ───────────────────────────────────────────────────────────────

tidy:
	go mod tidy
