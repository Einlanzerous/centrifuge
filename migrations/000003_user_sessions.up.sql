-- CTFG-26: user_sessions — tracks "Since you last looked" state for the Today
-- view. Pulled out of Phase 1 to land alongside the read API that needs it.
--
-- Centrifuge is presently single-user (a personal homelab tool), so the API
-- operates on one well-known 'default' session. The table is keyed by a unique
-- label so a future multi-user / authenticated build can add rows without a
-- schema change — last_viewed_at simply becomes per-identity.

CREATE TABLE user_sessions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- stable handle for a session; 'default' is the single implicit user today.
    label          text NOT NULL UNIQUE,
    -- when the user last marked the Today feed as seen. NULL = never looked, in
    -- which case the Today view treats every scored story as new.
    last_viewed_at timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
