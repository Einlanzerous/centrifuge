# Scaffolded scoring eval — gemma4 vs muse-glimmer under CTFG-62/63 — 2026-08-23

Context: overnight autonomous session implementing CTFG-63 (done:false guard +
prose prompt) and CTFG-62 (shape-probed chunking, escalation gate, num_ctx
pinning) and re-running the CTFG-61 bakeoff question under the new scaffolding,
per that eval's recommendation #5. Ollama **0.32.14** (moved from 0.32.9 since
the Aug 13 eval), 5 fixtures via `make score-fixtures`, temp 0,
num_predict 6144, num_ctx 16384, chunking at 5k, GPU exclusive.

**Verdict: gemma4:31b keeps the scoring role, now behind the full scaffolding.
muse-glimmer:30b is re-rejected — its digest collapse survives every prompt
variant we have, including one written for it.** And separately: half of the
"model problem" measured in August turned out to be an operational problem
(mmap thrash + oversized context) that is now fixed in config.

## Operational findings first — they invalidated the planned baseline

1. **mmap page-thrash made model loads look like hangs.** The box has 31 GB RAM
   (swap 8/8 GB full); the gemma file is ~19 GB. An mmap'd load finished
   `load_tensors` then paged-in at 125–870 MB/s *for 15+ minutes* — ~90 GB read
   for a 19 GB model — with `/api/ps` empty throughout. Two baseline attempts
   died this way (one needed `docker restart ollama`).
   **Fix: `options.use_mmap=false`** (streams tensors once through pinned
   buffers): cold load measured at **57 s**. Now prod default
   (`OLLAMA_USE_MMAP`, CTFG-62 branch).
2. **Server-level 64K context made every load heavier and hurt quality.**
   The July eval already measured 32K beating 64K on morning_brew; scoring now
   pins `options.num_ctx=16384` per request (`OLLAMA_NUM_CTX`), ~4× smaller KV
   cache. The old-code baseline on 0.32.14 was skipped after the wedges ate its
   time window; old-code behavior is documented in the Aug 13 report instead.
3. **Ollama 0.32.14 cuts generations with `done:false` fairly often.** 3 of 7
   gemma generations in one pass ended as partial envelopes (example_digest,
   letters, plus a chunk). The CTFG-63 guard turned all of them into attributed
   truncation-salvage instead of silent "complete" answers. The guard is not
   optional on this runtime.

## gemma4:31b + scaffolding (prompt 2026-08-23.1, chunked)

| fixture | shape probe | chunks | items (old stack, Aug 13) | time | notes |
|---|---|---|---|---|---|
| economics_explained 5.9k | digest | 2 | **8** (was: 1 truncated ad, 4m25s) | 64s | 7/8 anchored; 1 prompt-leak item (unanchored → gate) |
| example_digest 1.5k | — | 1 | **7** (was: truncated) | 39s | done:false cut, salvaged cleanly |
| example_essay 1.4k | — | 1 | 1 | 11s | correct |
| letters 10.7k | **essay** ✓ | 1 (unsplit) | 1 story (correct count) | 25s | summary hit a phrase-loop spiral + done:false cut |
| morning_brew 13.6k | **digest** ✓ | 3 | **8**, all anchored (was: 4, truncated) | 5m26s | 1 of 3 chunks rambled to cap; 2 stories no summary |

Aggregate: **no whole-newsletter dumps, ~96% snippet anchoring, correct
essay/digest discrimination, every failure mechanically flagged.** Remaining
gemma defects are the weights-level class C (phrase loops — now partially
repaired by the CTFG-56 n-gram collapse; one prompt-instruction leak; empty
summaries on 2 stories) and ~12 min/pass wall clock. All remaining defect
instances are caught by the escalation gate (unanchored/missing snippet, empty
summary, truncation).

## muse-glimmer:30b — re-rejected, with new evidence

Under the **standard** (prose) prompt: example_digest (1.5k chars, below any
chunk threshold) → **1 item**. morning_brew chunked to 3×4.5k → **1 item per
chunk**. letters/economics → 1 item each (defensible as essays, but stories
carry no summary and relevance_score 0 with topic "tech"). Byte-identical
across 2 passes, ~40–80 s per full pass, zero artifacts, 100% anchoring of
what it does emit — pristine and wrong.

Under a purpose-built **compact** prompt (minimum instruction bulk, same
contract — `BuildPromptCompact`, stamped 2026-08-23.1c): example_digest still
→ 1 item, now misclassified promo. The CTFG-61 "short prompt → 11 items"
diagnostic does not transfer to any full-contract prompt we can actually ship.
Meanwhile the Aug 13 addendum's good glimmer numbers came from the **old
JSON-template prompt** — the exact block that fixes gemma when removed. The
two models want opposite prompts, and glimmer's wants are not satisfiable by
anything measured tonight. The compact variant stays in the code as an
explicit `SCORING_PROMPT_STYLE=compact` experiment knob, never a default.

Conclusion unchanged from CTFG-61 but stronger: glimmer's segmentation
collapse is prompt-shape-driven, deterministic, and silent; scoring stays on
gemma. Speed is not worth a reader that misses 6 of 7 items.

## What shipped tonight (see PRs #45, #47)

- CTFG-63 both halves: PartialError plumbing + prose prompt (PromptVersion
  2026-08-23.1).
- CTFG-62: shape probe (essay/digest, ~5 s, schema-constrained), sentence-
  aligned ~5k chunking with boundary-split merge, `snippet` required in the
  schema, escalation gate persisted to `newsletters.gate_findings`
  (migration 000006), `num_ctx` pinning, `use_mmap=false` default.
- CTFG-56 extension: n-gram phrase-loop collapse in the sanitizer.
- Per-model prompt-style seam (auto/standard/compact).

## E2E: 61 real newsletters, gemma + full scaffolding (appended same night)

61 fresh newsletters pulled from the live Gmail inbox (Aug 17-22: TLDR ×19,
HCR ×11, Morning Brew ×5, 1440 ×5, ByteByteGo, IT Brew, and 7 more sources)
were ingested into a clean local instance and scored by the worker
end-to-end (v1.6.0 code, gemma4:31b, ~2 min/newsletter):

- **61/61 scored, 0 failed.**
- **692 items, 231 stories** — 11.3 items/newsletter (old live baseline:
  41 stories from 17 newsletters, 7 of them whole-newsletter dumps).
- **Zero silent collapses.** Every 1-item newsletter (6) was a genuine
  essay, probe-confirmed. The class-A dump did not occur once.
- Per-source segmentation is shape-correct: Morning Brew 18-43 items,
  1440 9-41, TLDR 4-28, HCR essays 1-6.
- Residual quality tail, all mechanically flagged: 22 stories (9.5%)
  without summaries, 8 items without snippets, 22 unanchored snippets,
  23 truncation events (mostly Ollama 0.32.14 done:false cuts, all
  salvaged with attribution). **32 of 61 newsletters carry ≥1 gate
  finding** — that is the escalation executor's queue (CTFG-65), and the
  measured case for building it.

Verdict: the scaffolding did what CTFG-61 predicted. The reader is usable
on local-only inference today; the flagged tail is the remaining gap
between usable and good, and it is now enumerable instead of invisible.

## Next

1. ~~Full e2e over 61 real newsletters~~ — done, above.
2. Escalation executor (CTFG-65) — the gate records candidates; nothing
   re-scores them yet. 32/61 flagged says this is the next lever.
3. If glimmer is ever revisited: the experiment is prompt-shape search under
   `SCORING_PROMPT_STYLE=compact`-style variants, gated on example_digest ≥5
   items. Do not retry config-level levers; they are exhausted.
