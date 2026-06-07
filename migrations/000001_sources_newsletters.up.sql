-- CTFG-12: publication registry (sources) + raw-delivery container (newsletters).
-- Story-grain pivot: a newsletter is a container, not the scored unit. Relevance,
-- summary, and engagement live on stories (000002). processing_status is the
-- decoupling point between ingestion (Phase 2) and the scoring worker (Phase 3).

-- gen_random_uuid() is in core since PG13, but pgcrypto is kept explicit for
-- portability and to document intent. It is a trusted extension, so the database
-- owner can create it.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- sources: a first-class publication or feed. One row per newsletter sender or
-- RSS feed. Enables per-source rollups ("I like 2/8 from this pub").
CREATE TABLE sources (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    -- newsletter = inbound email publication; rss = polled feed (CTFG-30).
    kind       text NOT NULL CHECK (kind IN ('newsletter', 'rss')),
    -- from-address for newsletters, feed URL for rss. Stable per-source identity.
    identity   text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, identity)
);

-- newsletters: one raw delivery (one email; for RSS, one fetch). Verbatim
-- container with no score of its own.
CREATE TABLE newsletters (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id         uuid NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    -- RFC822 Message-ID (email) or feed item guid. Unique when present; many
    -- backfill/HTML drops won't have one, hence nullable + partial unique index.
    message_id        text,
    subject           text,
    raw_html          text,
    body_text         text,
    received_at       timestamptz,
    -- content hash for dedup when message_id is absent.
    dedupe_hash       text,
    ingested_at       timestamptz NOT NULL DEFAULT now(),
    -- worker polls pending_scoring; transitions to scoring -> scored | failed.
    processing_status text NOT NULL DEFAULT 'pending_scoring'
        CHECK (processing_status IN ('pending_scoring', 'scoring', 'scored', 'failed'))
);

CREATE UNIQUE INDEX newsletters_message_id_key
    ON newsletters (message_id) WHERE message_id IS NOT NULL;
CREATE INDEX newsletters_dedupe_hash_idx ON newsletters (dedupe_hash);
CREATE INDEX newsletters_processing_status_idx ON newsletters (processing_status);
CREATE INDEX newsletters_source_id_idx ON newsletters (source_id);
