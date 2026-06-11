-- CTFG-33: scoring robustness. scoring_attempts bounds how many times the worker
-- re-scores a newsletter whose model output came back truncated/unparseable (a
-- transient failure that often clears on a re-run) before it gives up; the
-- claim increments it. scoring_error records why a newsletter was ultimately
-- marked failed, so failures are diagnosable without digging through worker logs.
ALTER TABLE newsletters ADD COLUMN scoring_attempts integer NOT NULL DEFAULT 0;
ALTER TABLE newsletters ADD COLUMN scoring_error text;
