# centrifuge

Self-hosted newsletter-curation backend — source-agnostic ingestion, decoupled
Ollama-powered relevance scoring, and RSS reflection.

## Why Go

centrifuge is written in **Go** (module `github.com/Einlanzerous/centrifuge`,
`go 1.26`). Go gives us a single statically-linked binary with no runtime to
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
  items from any source into an `InboundMessage`, deduplicates them (Message-ID
  first, content-hash fallback), and persists them to Postgres in
  `processing_status=pending_scoring`. It **never scores inline** — it only
  writes durable rows, so a slow or failed model call can never drop or block an
  email. Two entrypoints feed it (see [Ingestion](#ingestion) below).
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
- `POST /ingest` — raw RFC822 email (the production webhook). See below.
- `POST /ingest/html` — JSON `{html, subject?, from?, from_name?, message_id?, received_at?}` drop, for backfill / test-fire.

## Ingestion

Both ingestion endpoints normalize their input to a single `InboundMessage` and
hand it to the source-agnostic core, which makes the email source irrelevant —
webhook, hand-drop, or a future live feed all share one dedupe + persistence
path.

- **`POST /ingest`** accepts a raw RFC822 / multipart message. The MIME parser
  (`net/mail` + `mime/multipart`) prefers the `text/html` body, falls back to
  `text/plain`, decodes quoted-printable/base64, expands RFC2047 encoded-word
  subjects, and records attachment metadata (not blobs). This is the contract
  the future live auto-feed will POST to.
- **`POST /ingest/html`** wraps a JSON HTML drop as an `InboundMessage` — the
  fast path for firing real newsletters at the pipeline before any live feed
  exists. Only `html` is required.

Both require the shared **`INGEST_TOKEN`**, supplied in the `X-Ingest-Token`
header or a `?token=` query param and compared in constant time. When
`INGEST_TOKEN` is unset the check is disabled (local-dev convenience only) — in
production it must be set, since the endpoints accept arbitrary content and must
not be an open relay. Responses are `200 {id, status, source_id, duplicate}`
for both created and deduped deliveries, `400` for malformed input, `401` for a
bad/missing token.

### Sanitization & model-input prep

`raw_html` is stored **verbatim**, but the scorer never sees it directly.
Ingestion derives a cleaned `body_text` from the HTML with a deliberately simple
**sane default**: drop `script`/`style`/`head`/`title`/`noscript` subtrees and
HTML comments, decode entities, and flatten the rest to a single
whitespace-collapsed text stream (structure is not preserved). Extraction uses
`golang.org/x/net/html`'s tokenizer, so malformed markup never panics. The
result is truncated to a configurable budget (**`INGEST_MAX_CHARS`**, default
24000, `0` = unlimited) so a large digest can't blow the model's context window;
truncation keeps the lead content. The cleaned text is also the dedupe
fingerprint (lowercased before hashing).

## Configuration

All configuration is read from the environment.

| Variable           | Required | Default                                                          | Description                                           |
| ------------------ | -------- | ---------------------------------------------------------------- | ----------------------------------------------------- |
| `DATABASE_URL`     | yes      | —                                                                | Postgres connection string.                           |
| `OLLAMA_URL`       | no       | `http://ollama:11434`                                            | Base URL of the Ollama server.                        |
| `OLLAMA_MODEL`     | no       | `gemma4:31b`                                                     | Model tag used for relevance scoring.                 |
| `INGEST_TOKEN`     | no       | —                                                                | Token authenticating inbound ingestion requests (`X-Ingest-Token` header or `?token=`). Unset disables the check (local dev). |
| `INGEST_MAX_CHARS` | no       | `24000`                                                          | Cap on the cleaned body text fed to the scorer; `0` disables truncation. |
| `PORT`             | no       | `8080`                                                           | TCP port the HTTP server listens on.                  |
| `LOG_LEVEL`        | no       | `info`                                                           | Log level: `debug`, `info`, `warn`, or `error`.       |
| `RELEVANCE_TOPICS` | no       | `AI engineering,urbanism,transit/trains,nuclear,tech,video games` | Comma-separated topics that bias scoring.             |

A missing `DATABASE_URL` is reported as a clear startup error (the process exits
non-zero rather than panicking).

## Local development

Requires Go 1.26+.

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
