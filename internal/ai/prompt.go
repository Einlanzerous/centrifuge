package ai

import (
	"fmt"
	"strings"
)

// PromptVersion is stamped onto every story the worker scores (stories.
// prompt_version) so results are attributable to the exact instructions that
// produced them. Bump it whenever the prompt text or the expected output
// contract below changes — the eval harness (CTFG-23) diffs across versions.
const PromptVersion = "2026-08-23.1"

// PromptVersionCompact stamps stories scored with the compact prompt variant
// (see BuildPromptCompact), so the two styles stay distinguishable in evals.
const PromptVersionCompact = "2026-08-23.1c"

// PromptInput is everything the prompt builder needs about one newsletter. The
// caller derives Body from the cleaned, truncated text (Phase 2) so the model's
// context window is never blown.
type PromptInput struct {
	// SourceName is the publication's display name (sender), used only as
	// context for segmentation — not a topic.
	SourceName string
	// Subject is the email subject line.
	Subject string
	// Body is the cleaned plaintext rendering of the newsletter.
	Body string
	// Topics is the *current* focus set: seeded by RELEVANCE_TOPICS but
	// engagement-weighted over time (CTFG-28). It biases relevance_score and
	// suggests primary_topic, but the model may mint a new label.
	Topics []string
}

// BuildPrompt renders the segmentation + scoring instruction for one
// newsletter. The model is asked to split the email into 1..N items and score
// each, returning a JSON array (see ParseItems for the consumed shape).
func BuildPrompt(in PromptInput) string {
	topics := strings.Join(in.Topics, ", ")
	if topics == "" {
		topics = "(none specified)"
	}

	var b strings.Builder
	b.WriteString(`You are a newsletter curation engine. You are given the text of ONE email
newsletter. Split it into the distinct items it contains and score each one.

A newsletter may be a single essay (then there is exactly ONE item) or a digest
of many items (then there are many). Preserve reading order.

For EACH item, classify its kind:
- "story": substantive editorial content that develops across multiple sentences or
  paragraphs (an article, essay, analysis, news item).
- "blurb": a one-line mention, link roundup entry, headline-only teaser, or housekeeping note.
- "ad": paid placement, sponsorship, or "this issue is brought to you by".
- "promo": the publication promoting itself (merch, referrals, subscribe nags, event plugs).

A single sentence, a headline, or a headline-with-teaser is NOT a "story" — it is a
"blurb" (or "ad"/"promo" if it is selling something). Only classify an item as "story"
when its body actually develops the topic in more than one or two sentences. If the only
text you can find for an item is one short sentence, it is not a story.

Newsletters embed a hidden preheader/preview line (the teaser shown in the inbox) at the
very top, often duplicated and padded with invisible spacer characters. NEVER treat that
preheader/teaser as an item's content, and never use it as the snippet.

Score relevance from 0-100 for how well the item matches the reader's focus topics:
`)
	fmt.Fprintf(&b, "  %s\n", topics)
	b.WriteString(`
Higher means more aligned. Off-topic-but-well-written is NOT highly relevant.
Score every item, but only "story" items need a real summary.

Reader focus topics seed primary_topic, but you MAY mint a new short label when
none fits well. primary_topic is exactly ONE label; labels is 0-5 secondary tags.

Return ONLY a JSON array with one element per item (no prose, no markdown
fences). Describe each item with: a short title; a snippet quoting the lead
sentence of the item's actual body (never the email's preview/teaser line); its
kind; the publication's section heading for the item, omitted when there is
none; a 2-3 sentence neutral summary for stories (empty for everything else);
an integer relevance_score from 0 to 100; exactly one primary_topic label; and
0-5 secondary labels.

If the newsletter is empty or unintelligible, return an empty array.

`)
	fmt.Fprintf(&b, "SOURCE: %s\n", strings.TrimSpace(in.SourceName))
	fmt.Fprintf(&b, "SUBJECT: %s\n\n", strings.TrimSpace(in.Subject))
	b.WriteString("BODY:\n")
	b.WriteString(strings.TrimSpace(in.Body))
	b.WriteString("\n")
	return b.String()
}

// BuildPromptCompact is the low-bulk prompt variant for models whose
// segmentation collapses as instruction bulk grows — muse-glimmer returns a
// single item under the full prompt regardless of schema, but segments a digest
// into 8-12 items under a minimal one (CTFG-61 diagnostics). It carries the
// same output contract as BuildPrompt in as few instruction blocks as possible;
// ItemsSchema() still enforces the shape by grammar. Stories scored with it are
// stamped PromptVersionCompact.
func BuildPromptCompact(in PromptInput) string {
	topics := strings.Join(in.Topics, ", ")
	if topics == "" {
		topics = "(none specified)"
	}

	var b strings.Builder
	b.WriteString(`Split this email newsletter into ALL of its distinct items, in reading order.
A digest yields MANY items — one per story, brief, ad, or sponsor/self-promo
block. A single continuous essay yields exactly ONE item. Never treat the
hidden inbox-preview/teaser line at the top as an item or a snippet.

For EVERY item give: a short title; a snippet — the first sentence of the
item's own body, copied verbatim; its kind (story = developed editorial
content, blurb = one-liner or headline, ad = paid placement, promo =
self-promotion); the section heading when the publication shows one; a 2-3
sentence neutral summary (stories MUST have one; other kinds leave it empty);
an integer relevance_score 0-100 for how well the item matches the reader's
focus topics; one primary_topic label; and 0-5 secondary labels.

`)
	fmt.Fprintf(&b, "Reader focus topics: %s\n\n", topics)
	b.WriteString("Return ONLY a JSON array with one element per item.\n\n")
	fmt.Fprintf(&b, "SOURCE: %s\n", strings.TrimSpace(in.SourceName))
	fmt.Fprintf(&b, "SUBJECT: %s\n\n", strings.TrimSpace(in.Subject))
	b.WriteString("BODY:\n")
	b.WriteString(strings.TrimSpace(in.Body))
	b.WriteString("\n")
	return b.String()
}
