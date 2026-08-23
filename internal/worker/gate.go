package worker

import (
	"errors"
	"fmt"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/db"
)

// gateFindings evaluates the escalation gate (CTFG-62 M3) over one scored
// newsletter: every check is a cheap mechanical assertion on the persisted
// outcome, no confidence model involved. A non-empty result marks the
// newsletter for escalation to a stronger model; the items are still persisted
// (flagged best-effort beats an empty reader).
func gateFindings(nl db.Newsletter, res ai.ScoreResult, scoreErr error) []string {
	var out []string

	// Silent collapse: the probe affirmed multiple items exist (digest) and the
	// scorer still merged down to one — the class-A whole-newsletter dump. The
	// probe is the evidence, not body size: an unprobed body (chunking
	// disabled, or under the threshold) carries no shape signal, and flagging
	// every long single-story essay on ShapeUnknown would swamp the gate with
	// false trips (live data 2026-08-13: 7 of 17 newsletters were real dumps).
	if len(res.Items) == 1 && res.Shape == ai.ShapeDigest {
		out = append(out, fmt.Sprintf("silent_collapse: 1 item from digest-probed %d-char body", len([]rune(deref(nl.BodyText)))))
	}

	// A chunk that came back "[]" is the per-chunk form of the CTFG-59 silent
	// empty: possibly legitimate boilerplate, but a third of a newsletter can
	// vanish this way, so it is always recorded.
	if res.EmptyChunks > 0 && res.EmptyChunks < res.Chunks {
		out = append(out, fmt.Sprintf("empty_chunks: %d of %d chunk(s) returned no items", res.EmptyChunks, res.Chunks))
	}

	// Snippet checks matter only when the reader will slice segments: that takes
	// more than one item (a lone item renders the newsletter body inline) and
	// the raw HTML the reader slices from. Every kind is checked, not just
	// stories — the reader bounds each story's segment on ALL sibling snippets,
	// so an ad with an empty or fabricated snippet bleeds its text into the
	// neighboring story's segment.
	segments := len(res.Items) > 1 && nl.RawHTML != nil
	var segText string
	if segments {
		segText = db.SegmentSourceText(*nl.RawHTML)
	}
	for i, it := range res.Items {
		if it.Kind == ai.KindStory && it.Summary == "" {
			out = append(out, fmt.Sprintf("empty_summary: story %d %q", i, it.Title))
		}
		if !segments {
			continue
		}
		if it.Snippet == "" {
			out = append(out, fmt.Sprintf("missing_snippet: %s %d %q", it.Kind, i, it.Title))
			continue
		}
		if !db.SnippetAnchors(segText, it.Snippet) {
			out = append(out, fmt.Sprintf("unanchored_snippet: %s %d %q", it.Kind, i, it.Title))
		}
	}

	var tr *ai.TruncatedError
	if errors.As(scoreErr, &tr) {
		out = append(out, "truncated: "+tr.Error())
	}
	return out
}
