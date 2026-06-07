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
- **Decoupled scoring worker** (`internal/worker` + `internal/ai`): polls
  `pending_scoring` newsletters, asks Ollama (`OLLAMA_MODEL` on `OLLAMA_URL`) to
  segment each into stories and score them against `RELEVANCE_TOPICS`, persists
  the stories, and advances the newsletter to `scored`. Running out-of-band
  keeps ingestion latency low and lets scoring back-pressure independently. See
  [Scoring](#scoring).

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

## Scoring

The scoring worker (`internal/worker`) is the decoupled heart of the pipeline.
On an interval it **claims** a batch of `pending_scoring` newsletters
atomically (`UPDATE … FOR UPDATE SKIP LOCKED`, flipping them to `scoring`), so
concurrent workers never double-process. For each one it runs a single
**segment + score** pass through Ollama and persists the result in one
transaction:

- The model is asked to split the newsletter into the 1..N items it contains and
  classify each as `story` / `blurb` / `ad` / `promo`, returning a JSON array.
  Structured output (a JSON-Schema `format`) forces the array shape so digests
  segment instead of collapsing into one object.
- Each item is scored 0–100 for relevance against the current focus topics
  (seeded by `RELEVANCE_TOPICS`, engagement-weighted over time), with one
  `primary_topic`, secondary `labels`, and — for stories — a short `summary`.
- Items become `stories` rows. **Only `story`-kind items are fully scored**;
  ads/blurbs/promos are persisted unscored so engagement can still learn from
  them. Each scored story records the `model` and `prompt_version` that produced
  it.
- The newsletter is then flipped to `scored`. The whole persist step is one
  transaction, so a crash never leaves partial stories behind a still-`scoring`
  newsletter.

**Resilience — a newsletter is never lost.** A transient model failure
(network / 5xx) requeues the newsletter to `pending_scoring` for a later poll; a
terminal failure (undecodable or unparseable output) marks it `failed` rather
than persisting garbage. The Ollama client itself retries transient calls with
backoff. On startup the worker requeues any newsletter left in `scoring` by an
interrupted run. Set `SCORING_ENABLED=false` to run intake-only (e.g. local dev
with no reachable Ollama).

### Eval harness

Scoring quality is the main product risk, so `make score-fixtures` runs the full
prep → model → validate path over a directory of real-newsletter fixtures and
prints the segmented, scored items per fixture. It needs a reachable Ollama (no
database). Fixtures live in `internal/ai/testdata/fixtures/`: `*.html` runs
through the real sanitizer, `*.txt`/`*.md` is treated as already-clean body
text. Tweak the prompt or `RELEVANCE_TOPICS`, re-run, and eyeball the deltas
(redirect to a file to diff). `-raw` prints the model's unparsed response for
debugging.

## Configuration

All configuration is read from the environment.

| Variable           | Required | Default                                                          | Description                                           |
| ------------------ | -------- | ---------------------------------------------------------------- | ----------------------------------------------------- |
| `DATABASE_URL`     | yes      | —                                                                | Postgres connection string.                           |
| `OLLAMA_URL`       | no       | `http://ollama:11434`                                            | Base URL of the Ollama server.                        |
| `OLLAMA_MODEL`     | no       | `gemma4:31b`                                                     | Model tag used for relevance scoring.                 |
| `OLLAMA_TIMEOUT_SECONDS` | no | `120`                                                          | Per-request timeout for one generate call.            |
| `OLLAMA_MAX_RETRIES` | no     | `2`                                                              | Retries for a transient (network / 5xx) Ollama failure. |
| `INGEST_TOKEN`     | no       | —                                                                | Token authenticating inbound ingestion requests (`X-Ingest-Token` header or `?token=`). Unset disables the check (local dev). |
| `INGEST_MAX_CHARS` | no       | `24000`                                                          | Cap on the cleaned body text fed to the scorer; `0` disables truncation. |
| `PORT`             | no       | `8080`                                                           | TCP port the HTTP server listens on.                  |
| `LOG_LEVEL`        | no       | `info`                                                           | Log level: `debug`, `info`, `warn`, or `error`.       |
| `RELEVANCE_TOPICS` | no       | `AI engineering,urbanism,transit/trains,nuclear,tech,video games` | Comma-separated topics that bias scoring.             |
| `SCORING_ENABLED`  | no       | `true`                                                           | Run the background scoring worker. `false` = intake-only. |
| `SCORING_INTERVAL_SECONDS` | no | `30`                                                          | How often the worker polls for pending newsletters.   |
| `SCORING_BATCH_SIZE` | no     | `5`                                                              | Newsletters claimed per poll (processed sequentially). |

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

# Eval scoring quality against the fixtures (needs a reachable Ollama, no DB).
OLLAMA_URL=http://localhost:11434 make score-fixtures
```
