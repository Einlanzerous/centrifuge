# Centrifuge — UI design spec

Distilled from a Claude Design handoff bundle (see `HANDOFF.md`, `chat-transcript.md`, and the
runnable prototype in `prototype/`). The prototype is HTML/CSS/JS using Vue 3 + Tailwind via CDN;
**production target is Vue 3 (`<script setup>`) + Tailwind**, recreated pixel-perfect. See ticket
**CTFG-27** (frontend) and **CTFG-26** (the API that backs it).

> **⚠️ Scope guardrail — the design is a UX reference, not a source of truth for topics.**
> The prototype hardcodes five example topics (AI / Transit / Local-Gov / Nuclear / Strategy) with
> fixed colors. **These are illustrative mock data only.** Do **not** treat them as the canonical
> taxonomy or as a statement of the user's interests. Canonical behavior:
> - The topic/label set is **dynamic and grows over time**, seeded by the user's stated interests
>   (AI engineering, urbanism, transit/trains, nuclear, tech, video games) and then **adapted from
>   what the user actually engages with** (opens, bookmarks, thumbs) — and disengages from (ignored
>   sources/labels get demoted or dropped). This adaptive curation is the core product mechanic
>   (ticket **CTFG-28**), not a cosmetic afterthought.
> - Therefore topic **colors must be assigned from an extensible palette** as topics emerge, not
>   pinned to a fixed list of five.

## Aesthetic / design system

- **Type:** Geist (sans, UI), Instrument Serif (display — hero phrase, reader `<h1>`, blockquotes),
  Geist Mono (tags, labels, meta). Google Fonts.
- **Neutrals:** warm off-white canvas via an `ink` scale (`#f7f5f1` → `#0f0d0a`). Subtly-toned, low
  saturation. **Dark mode** is class-based (`html.dark`), with `--bg/--paper/--border` CSS vars.
- **Topic accents:** each topic gets a hue with shared chroma/lightness (oklch-derived). The
  prototype defines 5 (amber/teal/indigo/lime/magenta) as `{50,200,500,700,ink}` ramps — treat these
  as the *start* of an extensible palette.
- **Topic "art":** per-topic background headers are generated CSS gradient/conic compositions with a
  subtle film-grain overlay — **placeholders for real newsletter hero images** (swap in at the `.art`
  div). Never hand-draw figurative SVG.
- **Layout:** CSS-columns masonry (2/3/4 cols at 640/1024/1440px). Hairline `--border` rules. Smooth
  `fade`/`pop` view + element transitions.

## Views

1. **Today — "Since You Last Looked"**
   - Hero line: *"Since you last looked **X hours/days ago**, here are the key topics of interest."*
     (the phrase is rendered with a gradient text fill). Requires `last_viewed_at` per user session.
   - Topic-count chips: top topics with item counts (top 3 emphasized).
   - Masonry of 5–15 cards. **Card** = topic-art header, colored topic tag (`dot` + short label,
     Geist Mono), source name, compelling summary, **bookmark** toggle, **thumbs up / down**.
   - **Empty state** (no new items): subtle alert offering *"Spin today's brief"* — assemble older,
     unsurfaced stories.

2. **Archive / Inbox**
   - Chronological feed of **all** newsletters, **grouped by day** (Today / Yesterday / weekday…).
   - **Filter sidebar:** topic checkboxes, source list **with per-source counts**, date-range presets,
     and a search field. (Maps directly to API query params.)

3. **Reader modal**
   - Opens any item; topic-art header carries bookmark/thumbs; body renders the **stored raw HTML**
     newsletter, prose-styled (`Instrument Serif` headings, blockquotes). Custom scrollbar.

4. **Tweaks panel** (dev/UX affordance) — density, card-art style (vibrant/calm/none), light/dark,
   empty-state toggle, and an "hours since last visit" slider that filters the Today feed. Largely a
   prototype convenience; productionize selectively.

## Interactions / state (all reactive in the prototype)

Bookmark toggle, thumbs up/down (single-choice, toggleable), view switch (Today ⇄ Archive), live
filtering (topic/source/date/search), the hours-since-visit slider, toast feedback, and the
enter/leave transitions above. These are the behaviors CTFG-27 must reproduce against the real API.

## Data model the UI assumes → maps to API (CTFG-26) + schema (CTFG-12/13)

- **item**: `id`, `primary_topic` (dynamic label, drives color/art — *not* a fixed enum), `labels[]`
  (secondary, dynamic), `source` (id ref), `title`, `summary`, `received` (ts), `read` (bool),
  `bookmarked` (the prototype's `important`), `rating` (thumbs: up / down / none), `body` (raw HTML).
- **source**: `id`, `name`, `author`.
- **topic**: `key`, `label`, `color` — served from a **registry** so a growing set keeps stable
  colors (palette-assigned). Engagement/focus weights on topics+sources drive CTFG-28.

The Today view needs "items since `last_viewed_at`" + a topic summary; Archive needs filtered/grouped
queries + source counts; the Reader needs raw HTML; bookmark/thumbs are mutations that also feed the
engagement signal.
