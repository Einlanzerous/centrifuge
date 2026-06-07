-- CTFG-13: the story — the real unit of relevance, scoring, and engagement.
-- A newsletter segments into 1..N stories (single-essay -> 1; digests -> 20+).
-- Stories are written by the Phase-3 worker after LLM segmentation, NOT at
-- ingest, so this table stays empty until scoring runs.

CREATE TABLE stories (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    newsletter_id uuid NOT NULL REFERENCES newsletters (id) ON DELETE CASCADE,
    -- denormalized from the parent newsletter for cheap per-source rollups.
    source_id     uuid NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    -- order within the newsletter (segmentation output order).
    position      int NOT NULL,

    -- kind: only 'story' gets fully scored; ads/blurbs/promos are persisted but
    -- unscored so we can measure and learn. User-mutable ("mark as ad" override,
    -- CTFG-26/27) — flipping to 'ad' is a strong negative signal (CTFG-29).
    kind          text NOT NULL DEFAULT 'story'
        CHECK (kind IN ('story', 'blurb', 'ad', 'promo')),

    -- publication layout section ("Quick Hits", "Tour de headlines"). Provenance
    -- only, NOT a topic.
    section       text,
    title         text,
    -- canonical/outbound URL when resolvable. Soft dedup signal, NOT unique —
    -- digest trackers and "(More)" links make URL identity unreliable.
    url           text,
    snippet       text,

    -- Scoring (worker fills; null until scored).
    summary         text,
    relevance_score int CHECK (relevance_score BETWEEN 0 AND 100),
    -- dominant dynamic label, NOT an enum; taxonomy grows over time (CTFG-28).
    primary_topic   text,
    labels          jsonb,
    model           text,
    prompt_version  text,
    scored_at       timestamptz,

    -- Engagement (columns now; capture in CTFG-29).
    bookmarked  boolean NOT NULL DEFAULT false,
    -- thumbs: -1 / +1.
    user_rating smallint CHECK (user_rating IN (-1, 1)),
    opened_at   timestamptz
);

CREATE INDEX stories_newsletter_id_idx ON stories (newsletter_id);
CREATE INDEX stories_source_id_idx ON stories (source_id);
CREATE INDEX stories_url_idx ON stories (url) WHERE url IS NOT NULL;
