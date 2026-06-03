# centrifuge

Self-hosted newsletter-curation backend — source-agnostic ingestion, decoupled
Ollama-powered relevance scoring, and RSS reflection.

## Why Go

centrifuge is written in **Go** (module `github.com/Einlanzerous/centrifuge`,
`go 1.24`). Go gives us a single statically-linked binary with no runtime to
provision, a strong standard library (the HTTP server, JSON, and structured
logging used here are all stdlib), and first-class concurrency for the decoupled
scoring worker. It matches the house style of sibling services.

The entrypoint lives at `./cmd/centrifuge`.

## Architecture

centrifuge separates **ingestion** from **scoring** so that a slow or
unavailable model never blocks intake:

```
  sources ──▶ ingestion core ──▶ Postgres ──▶ scoring worker ──▶ scored items ──▶ RSS reflection
 (any kind)  (source-agnostic,   (durable     (decoupled,
              normalizes items)   queue)        Ollama-backed)
```

- **Source-agnostic ingestion core** (`internal/ingest`): normalizes inbound
  items from any source and persists them to Postgres. It **never scores
  inline** — it only writes durable rows.
- **Postgres** is the buffer/queue between ingestion and scoring.
- **Decoupled scoring worker** (`internal/ai`): pulls unscored items and asks
  Ollama (`OLLAMA_MODEL` on `OLLAMA_URL`) to rate relevance against
  `RELEVANCE_TOPICS`. Running out-of-band keeps ingestion latency low and lets
  scoring back-pressure independently.

Supporting packages: `internal/config` (env loading), `internal/log`
(structured JSON logging), `internal/httpapi` (HTTP surface), `internal/db`
(connectivity + migrations).

## CLI

| Command              | Behavior                                              |
| -------------------- | ----------------------------------------------------- |
| `centrifuge`         | Starts the HTTP server (graceful shutdown on SIGINT/SIGTERM). |
| `centrifuge migrate` | Applies database migrations, then exits.              |

### HTTP endpoints

- `GET /healthz` → `200 {"status":"ok"}`

## Configuration

All configuration is read from the environment.

| Variable           | Required | Default                                                          | Description                                           |
| ------------------ | -------- | ---------------------------------------------------------------- | ----------------------------------------------------- |
| `DATABASE_URL`     | yes      | —                                                                | Postgres connection string.                           |
| `OLLAMA_URL`       | no       | `http://ollama:11434`                                            | Base URL of the Ollama server.                        |
| `OLLAMA_MODEL`     | no       | `gemma4:31b`                                                     | Model tag used for relevance scoring.                 |
| `INGEST_TOKEN`     | no       | —                                                                | Token authenticating inbound ingestion requests.      |
| `PORT`             | no       | `8080`                                                           | TCP port the HTTP server listens on.                  |
| `LOG_LEVEL`        | no       | `info`                                                           | Log level: `debug`, `info`, `warn`, or `error`.       |
| `RELEVANCE_TOPICS` | no       | `AI engineering,urbanism,transit/trains,nuclear,tech,video games` | Comma-separated topics that bias scoring.             |

A missing `DATABASE_URL` is reported as a clear startup error (the process exits
non-zero rather than panicking).

## Local development

Requires Go 1.24+.

```sh
# Build everything.
go build ./...

# Vet and format.
go vet ./...
gofmt -l .

# Run the server (set at least DATABASE_URL).
DATABASE_URL=postgres://centrifuge:centrifuge@localhost:5432/centrifuge?sslmode=disable \
  go run ./cmd/centrifuge

# Apply migrations and exit.
DATABASE_URL=postgres://centrifuge:centrifuge@localhost:5432/centrifuge?sslmode=disable \
  go run ./cmd/centrifuge migrate

# Health check.
curl -s localhost:8080/healthz
```
