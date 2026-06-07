package ai

import (
	"fmt"
	"strings"
)

// PromptVersion is stamped onto every story the worker scores (stories.
// prompt_version) so results are attributable to the exact instructions that
// produced them. Bump it whenever the prompt text or the expected output
// contract below changes — the eval harness (CTFG-23) diffs across versions.
const PromptVersion = "2026-06-07.1"

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
- "story": substantive editorial content worth reading (an article, essay, analysis, news item).
- "blurb": a one-line mention, link roundup entry, or housekeeping note.
- "ad": paid placement, sponsorship, or "this issue is brought to you by".
- "promo": the publication promoting itself (merch, referrals, subscribe nags, event plugs).

Score relevance from 0-100 for how well the item matches the reader's focus topics:
`)
	fmt.Fprintf(&b, "  %s\n", topics)
	b.WriteString(`
Higher means more aligned. Off-topic-but-well-written is NOT highly relevant.
Score every item, but only "story" items need a real summary.

Reader focus topics seed primary_topic, but you MAY mint a new short label when
none fits well. primary_topic is exactly ONE label; labels is 0-5 secondary tags.

Return ONLY a JSON array (no prose, no markdown fences). Each element:
{
  "title": "short item title",
  "snippet": "a short verbatim-ish excerpt or the lead sentence",
  "url": "the item's primary link if present, else omit or empty",
  "kind": "story|blurb|ad|promo",
  "section": "the publication's section heading for this item if any, else omit",
  "summary": "2-3 sentence neutral summary (stories only; else empty)",
  "relevance_score": 0,
  "primary_topic": "one label",
  "labels": ["secondary", "tags"]
}

If the newsletter is empty or unintelligible, return [].

`)
	fmt.Fprintf(&b, "SOURCE: %s\n", strings.TrimSpace(in.SourceName))
	fmt.Fprintf(&b, "SUBJECT: %s\n\n", strings.TrimSpace(in.Subject))
	b.WriteString("BODY:\n")
	b.WriteString(strings.TrimSpace(in.Body))
	b.WriteString("\n")
	return b.String()
}
