.PHONY: dev build test migrate docker tidy score-fixtures

# ── Development ───────────────────────────────────────────────────────────────

dev:
	go run ./cmd/centrifuge

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/centrifuge ./cmd/centrifuge

# ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test ./...

# ── Scoring eval ──────────────────────────────────────────────────────────────
# Run the scoring pipeline over the fixture newsletters and print the results.
# Needs a reachable Ollama (OLLAMA_URL / OLLAMA_MODEL); no database required.
# Redirect to a file and diff across prompt/model tweaks to compare quality.

score-fixtures:
	go run ./cmd/score-fixtures

# ── Database ──────────────────────────────────────────────────────────────────

migrate:
	go run ./cmd/centrifuge migrate

# ── Docker ────────────────────────────────────────────────────────────────────

docker:
	docker compose up --build -d

# ── Maintenance ───────────────────────────────────────────────────────────────

tidy:
	go mod tidy
