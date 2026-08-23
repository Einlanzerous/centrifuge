# Live email auto-feed (Gmail polling) — CTFG-24

Centrifuge's production feed is a background **poller** that reads
`centrifuge.newsletters@gmail.com`, pulls each new message as raw RFC822, and
runs it through the same ingestion core as the `POST /ingest` webhook.

**Why poll instead of a push webhook?** Centrifuge runs on the homelab behind
NAT with no public ingress. A poll is outbound-only, so nothing has to be
exposed to the internet (a Pub/Sub or Apps Script push would require a public,
authenticated `/ingest` endpoint). The poller mirrors the scoring worker: an
in-process goroutine on an interval, started in `cmd/centrifuge/main.go`.

**Seen-state is a Gmail label, not a database table.** The poller lists mail
*not* carrying the `centrifuge/ingested` label, ingests it, then applies the
label so the next poll skips it. Labeling is best-effort: ingestion already
dedupes by `Message-ID`, so a message re-fetched after a failed label is a
harmless no-op. State is human-visible in the inbox and needs no migration.

## One-time setup

### 1. GCP project + OAuth client

1. In the [Google Cloud Console](https://console.cloud.google.com/), create (or
   pick) a project.
2. **APIs & Services → Library →** enable the **Gmail API**.
3. **APIs & Services → OAuth consent screen:**
   - User type **External**, fill the minimal app info.
   - Leave it in **Testing** mode and add `centrifuge.newsletters@gmail.com` as a
     **test user** (testing-mode refresh tokens for an added test user do not
     expire on the 7-day unverified-app clock).
   - The scope used at runtime is `https://www.googleapis.com/auth/gmail.modify`
     (read messages + apply the label); you don't need to pre-add it here.
4. **APIs & Services → Credentials → Create credentials → OAuth client ID:**
   - Application type **Desktop app**. (Desktop clients permit loopback
     `http://127.0.0.1:<port>` redirects, which the `authorize-gmail` flow uses —
     no redirect URI registration needed.)
   - Copy the **Client ID** and **Client secret**.

### 2. Mint the refresh token

With the client credentials in the environment, run the helper subcommand. It
opens a loopback server, prints a consent URL, and exchanges the returned code:

```sh
GMAIL_CLIENT_ID=<id> GMAIL_CLIENT_SECRET=<secret> \
  go run ./cmd/centrifuge authorize-gmail
# (or, in the container:  centrifuge authorize-gmail)
```

Open the printed URL in a browser **signed in as the newsletters account**,
grant access, and the command prints:

```
GMAIL_REFRESH_TOKEN=1//0g...
```

Store that value. If it prints "no refresh token returned", the account already
granted access — revoke it at <https://myaccount.google.com/permissions> and
retry (the flow forces a consent prompt, which is what returns a refresh token).

### 3. Configure the service

Set these (see `.env.example` for the full list and defaults):

```sh
MAILFEED_ENABLED=true
GMAIL_CLIENT_ID=<id>
GMAIL_CLIENT_SECRET=<secret>
GMAIL_REFRESH_TOKEN=<token from step 2>
# Optional overrides:
# GMAIL_USER=me
# MAILFEED_INTERVAL_SECONDS=120
# MAILFEED_BATCH=25
# MAILFEED_QUERY=-label:centrifuge/ingested
# MAILFEED_LABEL=centrifuge/ingested
```

When `MAILFEED_ENABLED=true`, the three `GMAIL_*` credentials are required or the
service fails fast at startup.

## Production deploy (construct-server)

Two places must carry the new vars — config reads them, but they also have to be
*passed into the container* (the same gotcha that left `SCORING_ENABLED` inert):

1. **The deployed `.env`** is regenerated on every deploy from the env-scoped
   `PROD_ENV_FILE` secret. Add the `MAILFEED_*` / `GMAIL_*` keys to the canonical
   `~/construct-server/.env` and re-push it:
   ```sh
   gh secret set PROD_ENV_FILE -R Einlanzerous/construct-server -e home-server \
     < ~/construct-server/.env
   ```
2. **The compose service** must map the keys into `centrifuge-backend`'s
   `environment:` block in `~/construct-server/docker-compose.yml` — otherwise
   they never reach the container regardless of `.env`.

## How it runs

- `internal/mailfeed/poller.go` — the `Poller` (interval loop, batch handling,
  per-message fetch → `ingest.ParseRFC822` → `Ingester.Ingest` → label).
- `internal/mailfeed/gmail.go` — the `MailClient` backed by the Gmail REST API,
  plus `Authorize` (the consent flow behind `authorize-gmail`).
- A transient Gmail/API error on one tick is logged and retried next tick; a
  per-message error leaves that message unlabeled so it's retried; an unparseable
  message is labeled to retire it. The loop never dies on a single failure.

## Verifying

```sh
go test ./internal/mailfeed/...        # unit tests (stubbed, no network)
```

End-to-end (local, against the real inbox), with the env above and a local
`DATABASE_URL`, scoring optional:

```sh
go run ./cmd/centrifuge
```

Confirm: the poller logs picked-up message counts, new rows appear in
`newsletters` at `pending_scoring`, and the `centrifuge/ingested` label shows up
on those threads in Gmail. A second tick should skip already-labeled mail.
