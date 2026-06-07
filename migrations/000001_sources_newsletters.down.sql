-- CTFG-12 rollback. Drop in dependency order; newsletters references sources.
DROP TABLE IF EXISTS newsletters;
DROP TABLE IF EXISTS sources;
-- pgcrypto is left in place: other schema may rely on it and dropping a shared
-- extension on rollback is surprising.
