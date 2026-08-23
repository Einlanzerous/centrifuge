-- CTFG-62: escalation gate. gate_findings records the mechanically-detectable
-- scoring defects (silent collapse, missing/unanchorable snippet, empty story
-- summary, truncation) the worker observed when it persisted this newsletter's
-- stories — NULL means the scoring passed clean. A populated value marks the
-- newsletter as a candidate for escalation to a stronger model.
ALTER TABLE newsletters ADD COLUMN gate_findings text[];
