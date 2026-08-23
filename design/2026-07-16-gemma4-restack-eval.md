# Gemma 4 re-stack eval — 2026-07-16/17

Context: revisiting gemma4 scoring quality ~1 month after the spiral fixes (CTFG-46),
prompted by "grab latest for all gemma-associated items and re-test." Full detail in
the session logs; scratchpad eval logs did not persist, but the findings and exact
repro commands are below.

## What "latest" turned out to mean

- **gemma4 weights: unchanged upstream since May 2026.** All three local tags
  (31b, 12b, e4b) byte-match the registry manifests (verified by sha256 of
  `registry.ollama.ai/v2/library/gemma4/manifests/<tag>`). Google never re-shipped.
- **Ollama runtime: 0.32.0 → 0.32.1** (released 2026-07-16; "Improved Gemma 4 tool
  calling and multi-turn reasoning"). The gemma chat template is built into the
  Ollama **binary** (the model ships a bare `{{ .Prompt }}` template), so runtime
  updates ARE template updates. Note: those fixes target multi-turn tool loops;
  centrifuge's scorer is single-turn `/api/generate` + `format`, so largely orthogonal.

## Operational discoveries (fixed / actioned)

1. **GPU contention:** `eido-llama.service` (Qwen3-30B, `-ngl 99`, ~19 GB) had held
   the R9700 since 2026-06-30 → Ollama offloaded only 27/61 layers of gemma4:31b →
   mostly-CPU inference (12min+/newsletter, 15-min timeouts). **All prod scoring
   between 06-30 and 07-16 ran degraded.** gemma4:31b (~21 GB) + Qwen3-30B (~19 GB)
   cannot share the 32 GB card; sharing policy still undecided. Diagnose:
   `curl localhost:11434/api/ps` → compare `size_vram` vs `size`.
2. **Dead env var:** Ollama ≥0.32 reads `OLLAMA_CONTEXT_LENGTH`, not `OLLAMA_NUM_CTX`;
   the intended 64K default had silently fallen back to 32768.
   Fix: construct-server PR #65 (https://github.com/Einlanzerous/construct-server/pull/65).
   Live container already recreated with the corrected var (ctx 65536 verified).
3. Centrifuge unit tests: all green on this stack.

## 3×-per-model trial (Ollama 0.32.1, 64K ctx, full GPU, num_predict=6144)

`OLLAMA_URL=http://localhost:11434 [OLLAMA_MODEL=gemma4:12b] make score-fixtures`
over `internal/ai/testdata/fixtures` (5 fixtures), 3 runs per model.

| | gemma4:31b ×3 | gemma4:12b ×3 |
|---|---|---|
| Truncated fixtures/run | 2 of 5 — same two every run | 1–2 of 5 — same ones |
| Wall clock per pass | ~6.5 min | ~18 min |
| Errors / timeouts | 0 | 0 |
| Runs 2 vs 3 | byte-identical | byte-identical |

### Findings

1. **Temp-0 output is deterministic once the model is warm.** Runs 2/3 byte-identical
   for both models; only the first post-load run differs slightly. Same newsletters
   fail the same way every time → **retry-on-truncation is a dead end**.
2. **num_ctx changes model behavior.** 31b at 32K ctx segments morning_brew into 11
   clean items; at 64K it deterministically rambles into the 6144-token cap (4 items,
   4m32s). → centrifuge should pin `options.num_ctx` per request so scoring is
   decoupled from server config.
3. **Two failure modes, both deterministic, both on the 2 largest fixtures**
   (letters 10.7k chars, morning_brew 13.6k):
   - **F1 ramble-to-cap:** generates all 6144 tokens, gets cut (morning_brew, both
     models; digest on 12b 2/3 runs).
   - **F2 early-quit:** 31b stops on its own at ~450 tokens (~19s) leaving broken
     JSON, salvages 1 item (letters). Same family as the QAT silent-`[]` regression.
4. **gemma4:12b verdict: not a win.** Its one-off 6-story recall on letters did NOT
   reproduce (1 story in all 3 trial runs); truncates as much or more; ~3× slower
   (dense vs 31b MoE). Same class-C artifacts (glued tokens "the191"/"was's", topic
   typo "sccer", repeated sentences). **Stay on 31b.**
5. **Class C persists on the fully-updated stack** (weights + runtime + full GPU):
   glued words, repetition ("ofunfunded mandates and unfunded mandates", recurring
   broken "the same" referent), and one verbatim prompt-instruction leak into a
   summary (`{// a single sentence is not a story ...}` — from
   `internal/ai/prompt.go` ~line 57). It's in the weights; runtime updates won't fix it.

Other model notes: Qwen3-30B (via EIDO) was tested separately and rejected on
quality. LiteRT-LM (google-ai-edge) evaluated and dismissed: edge/on-device runtime,
tops out ~12B int4, solves constraints we don't have.

## Next steps when resuming (evidence-backed order)

1. **Chunk large newsletters** before scoring (split >~8k prepped chars, score per
   chunk, merge) — failures are deterministic and size-correlated; changing the
   input is the only lever that changed outcomes.
2. **Pin `options.num_ctx` per request** in `internal/ai` (cheap; decouples from
   server default; 32K empirically beat 64K on morning_brew).
3. Merge construct-server PR #65 if not yet merged.
4. Decide GPU sharing policy (eido-llama vs ollama) before running both again.
   Restart EIDO with `sudo systemctl start eido-llama.service` (stopped 2026-07-17
   for this trial).
