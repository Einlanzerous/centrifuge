# muse-glimmer:30b vs gemma4:31b for newsletter scoring — 2026-08-13

Context: SERV-95 found `muse-glimmer:30b` displaces `gemma4:31b` as the local **coding**
generator. This asks the separate question for centrifuge's scorer, which is a different
job: long-context segmentation + extraction into schema-constrained JSON, single-turn
`/api/generate`, temp 0.

**Verdict: do NOT swap. `gemma4:31b` keeps the scoring role.** Glimmer is 12× faster and
its prose is dramatically cleaner, but it fails the core job — segmentation — on exactly
the large digests centrifuge exists to process, and it fails *silently*.

## Conditions (clean, unlike the July run)

- GPU fully free at start (59 MB / 34 GB), `eido-llama.service` inactive.
- Both models 100% GPU-offloaded, verified `size_vram == size` (gemma 20.3 GB,
  glimmer 16.9 GB). No repeat of the 06-30→07-16 starvation window.
- Ollama **0.32.9** (July eval was 0.32.1), `OLLAMA_CONTEXT_LENGTH=65536`.
- `make score-fixtures`, 5 fixtures, num_predict=6144, temp 0. 2 passes per model,
  grouped by model so there was exactly one VRAM swap.

## Head-to-head

| | muse-glimmer:30b | gemma4:31b |
|---|---|---|
| Wall clock / pass | **52s, 47s** | 627s, 599s |
| Fixtures truncated | **0 of 5** | 3 of 5 |
| Class-C artifacts / pass | **0** | 28 |
| `morning_brew` (13.6k, ~8 sections) | 1 item | 4 salvaged (truncated) |
| `letters` (10.7k, single essay) | 1 empty item | 3 salvaged (truncated) |
| `example_digest` (1.5k) | **7 items, correct kinds** | 1 ad (truncated, 4m25s) |
| `example_essay` (1.4k) | 1 story, clean | 1 story |

Class-C artifact counts are from a signature scan (CJK injection, meta-leak, phrase loops,
known glued tokens) over both passes; both passes scored identically for each model.

### gemma regressed against its own July baseline

Same harness, same fixtures, same weights — only the runtime moved (0.32.1 → 0.32.9):
truncation went **2 of 5 → 3 of 5**, and wall clock **~6.5 min → ~10.4 min** per pass. It
now rambles 4m25s on the *1,547-char* `example_digest` and salvages a single ad. Class C
is worse than described in the July doc — one summary degenerated into
`"the same as the same as ..."` ~30 times, another into
`"// This is a bit repetitive. Let me rewrite it. 10/10. 11/10. 12/10."`, and one leaked a
CJK character mid-sentence (`"the umappedy of the用 a budget reconciliation process"`).

### glimmer's failure mode is worse than truncation

On both large fixtures glimmer returns **one** item:

- `letters` → title is just the source name, **no summary**, `relevance_score: 0`. 4.3s for
  10,660 chars.
- `morning_brew` → correctly summarizes the *first* section, emits `]`, discards the other 7.

Two reasons this is worse than gemma's truncation despite looking cleaner:

1. **It is silent.** Gemma truncates → `TruncatedError` → `⚠ TRUNCATED` → the worker can
   act. Glimmer returns well-formed JSON with one garbage item; no error fires and the
   worker persists it. Same shape as the QAT silent-`[]` regression that was ruled
   prod-unsafe.
2. **It is deterministic.** Reproduced across 2 full passes and 3 direct API repeats
   (out_tok 31, identical). Retry is not a mitigation.

Note `relevance_score: 0` is **not** itself a defect — the focus topics are AI/urbanism/
transit/nuclear/tech/games, so soccer and immigration politics genuinely rate low, and
gemma scored the same content 10. Do not read the zeros as scoring collapse.

## The collapse is prompt-driven, not a capability limit

Diagnostics that narrow it down:

- **Not input truncation.** With a loose prompt, glimmer ingests the whole essay
  (`prompt_eval_count: 2395` ≈ the full 10.7k chars) and extracts 12+ accurate facts. It
  reads and understands the input.
- **Not the schema.** 2×2 over {prod, short} prompt × {prod, lean} schema: prod prompt → 1
  item under *both* schemas; short prompt → 11 items under *both*. Schema shape is
  irrelevant.
- **Prompt bulk is the driver.** Adding single instruction blocks to a minimal prompt keeps
  segmentation healthy (8–12 items); the inline JSON-element template alone collapses it to
  1; and the full production prompt collapses it to 1 regardless of which closing block is
  used. Glimmer short-circuits as instruction bulk grows.

So the capability exists — 11 items on `morning_brew` matches ground truth — but reaching it
needs a prompt written *for* glimmer. That is its own project, not a config flip.

**Chunking partly rescues it:** splitting into ~5k pieces took `morning_brew` from 1 item to
**9** (3+5+1) with clean summaries, in 78s total. On a single long essay chunking instead
produces fragmented title-only items (`letters` → 3 fragments, 1 real summary), which is the
wrong shape for an essay.

## Byproduct worth having: a prompt fix for the incumbent

Replacing the inline JSON-element template in `internal/ai/prompt.go` with a one-paragraph
prose description of the same fields **fixes gemma's ramble-to-cap on `morning_brew`**:

| gemma4:31b, morning_brew | items | out_tok | done |
|---|---|---|---|
| current (JSON template) | truncated | 6144 | `length` |
| prose description ×3 | **10, 10, 10** | 1210 / 1085 / 1085 | **`stop`** |

Reproducible ×3. The template block is redundant — `ItemsSchema()` already enforces shape
via grammar — so this costs nothing. `letters` was inconclusive: 1 of 3 runs returned 3
clean items, the other 2 got a partial envelope from Ollama (see below). Needs its own
eval pass before landing.

## Two incidental findings

1. **`<|eot|>` leaks as literal text.** Every glimmer response ends `...}]<|eot|>` — its
   end-of-turn token is emitted as text, so the Ollama Modelfile is not registering it as a
   stop token. Centrifuge tolerates it (the parser takes the first JSON doc), but it is a
   packaging bug on the `muse-glimmer:30b` tag and would bite any consumer doing a strict
   parse. Worth fixing on the SERV side.
2. **`done:false` is decoded but never checked.** Two gemma `letters` runs returned an
   envelope with `done:false`, no `done_reason`, no `eval_count` — Ollama cutting the
   generation server-side and returning a partial. `generateResponse` (`internal/ai/ollama.go:157`)
   decodes `Done`, but `doOnce` only rejects an empty `Response`. A partial is currently
   treated as complete; it lands in truncation-salvage so it degrades rather than breaks,
   but the signal is available and unused.

## Addendum — with scaffolding built for it, glimmer wins

The comparison above is **glimmer dropped into a harness tuned for gemma since June**
(`PromptVersion 2026-06-13.2`, whole-newsletter, `/api/generate`). Re-run with the one
structural change already on the roadmap — chunk to ≤6k prepped chars, using the real
`ingest.CleanText` prep path — the ranking inverts:

| | glimmer + ≤6k chunking | gemma, current harness |
|---|---|---|
| Items extracted (5 fixtures) | **23** | 13 |
| Fixtures truncated | **0** | 3 |
| Class-C artifacts | **0** | 28 |
| Wall clock, full pass | **96s** | ~620s |

77% more items, zero artifacts, 6.5× faster. `morning_brew` goes 1 → 8 items, `letters`
1 → 3. So "gemma keeps the role" is a statement about a **drop-in swap today**, not about
which model is better. Both of these are true at once: gemma is badly degraded, *and*
glimmer fails our gemma-shaped rig.

Two things ruled out as the lever:

- **Ollama is already current.** 0.32.9 shipped 2026-08-11 and its notes include
  *"Handle boundary condition in Muse Glimmer function calling parser."* There is no
  update to take.
- **`/api/chat` is not the fix.** Both models ship `template: '{{ .Prompt }}'`, so
  `/api/generate` sends the prompt raw with no turn markers — which is why glimmer emits a
  stray `<|eot|>`. Routing through `/api/chat` applies the built-in template and **does**
  eliminate the `<|eot|>` leak, but segmentation does not improve (`morning_brew` went
  7 → 1). Worth adopting for cleanliness, not for quality.

### Correction — under *equal* scaffolding the models are close, and the scaffolding is the win

The 23-vs-13 table above compares glimmer-with-chunking against gemma-**without**. Running
both under the same improved rig (5k chunks + `snippet` added to `ItemsSchema()`'s
`required`) and measuring what actually drives the reader — does the snippet anchor back
into the body per `extractSegmentText` (first 14 tokens, first word verbatim, ≥60% in order):

| | glimmer | gemma |
|---|---|---|
| Items | 27 | 28 |
| Snippet present | 74% (20/27) | **100% (28/28)** |
| **Anchored, of all items** | 74% | **96% (27/28)** |
| Anchored, of those emitted | 100% | 96% |
| Chunk failures | **0** | 2 of 10 (truncation) |
| Stories w/ summary | 10/10 | 12/12 |
| Score spread | 0–85, 8 distinct | 0–100, 8 distinct |
| Class-C artifacts | **0** | present |

**The scaffolding — not the model swap — is the unlock.** Chunking to 5k plus requiring
`snippet` takes gemma from 3-of-5 fixtures truncated and 41% of live newsletters yielding a
single story, to 96% anchored and 12/12 stories summarized. That work is model-independent
and pays off immediately on the incumbent.

Model choice is genuinely close afterwards. glimmer: no class-C artifacts, ~10× faster, zero
chunk failures — but omits `snippet` entirely on some chunks even when the grammar requires
it, which is fatal per-item (no snippet → no segment text → empty story). gemma: complete
snippets, wider score range, more items — but keeps truncating (2 of 10 chunks) and keeps
its class-C garbling.

Live baseline for context (the reason centrifuge is on ice): of 17 real newsletters,
**7 yield exactly one story** (class A dump), and 4 of 41 stories have no summary.

Real-world size distribution — 82% of newsletters are ≥5k chars, and the largest band (41%)
is 5–8k, exactly where glimmer's segmentation proved inconsistent (3 items at 5,085 chars,
1 item at 6,010):

| body_text size | newsletters |
|---|---|
| <5k | 3 |
| 5–8k | 7 |
| 8–15k | 6 |
| >15k | 1 |

What scaffolding actually has to do:

1. **Chunk to ~5k, not 6k.** The threshold is fuzzy and content-dependent, not a clean
   cutoff: `morning_brew` chunk 1 still collapsed to 1 item at 6,010 chars, but produced 3
   items at 5,085. Size correlates; it does not determine.
2. **Chunk by newsletter shape.** Splitting a single long essay is semantically wrong —
   `letters` yields 3 fragments where exactly 1 story is correct. Digests chunk; essays
   must not.
3. **Merge across chunks** — dedupe items split across a boundary, and restore reading
   order.
4. **Guard the silent collapse regardless of model.** A single returned item on a
   >8k-char input should raise, the way truncation does. Without it, this failure is
   invisible in prod.

## Recommendation

1. **Stay on `gemma4:31b` for scoring *today*** — glimmer is not a drop-in, and its
   failure is silent where gemma's is loud and salvageable. But this is a
   "not without scaffolding" verdict, **not** "gemma is the better model." On the
   evidence above, glimmer plus chunking is the strongest configuration measured, and
   gemma on the current stack is the weakest it has ever been.
2. **Do not read SERV-95 as transferable.** Coding-generation skill did not predict
   long-context segmentation behavior; the two roles need separate evals.
3. Land the **prose-prompt rewrite** for gemma on its own ticket — it is a real truncation
   fix for the worst fixture and is model-independent.
4. Chunking (>~8k prepped chars) remains the top structural lever and now has a second
   datapoint: it helps *both* models on digests, and is wrong for single essays. Chunk by
   newsletter shape, not by size alone.
5. **Build the chunker, then re-run this eval as the gate.** Chunking was already
   next-step #1 from the July doc; glimmer changes it from an incremental fix into the
   thing that unlocks a 6.5×-faster, artifact-free scorer. The measured 23-vs-13 result is
   the case for doing it.
6. Route through `/api/chat` when/if glimmer is adopted — it removes the `<|eot|>` leak at
   no cost. Not a quality lever on its own.
