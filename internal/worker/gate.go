package worker

import (
	"errors"
	"fmt"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/db"
)

// collapseGateChars is the body size above which a single-item segmentation of
// a non-essay newsletter is treated as a silent collapse — a whole-newsletter
// dump — rather than a plausible result (CTFG-62 M3). Live data 2026-08-13: 7
// of 17 real newsletters yielded exactly one story this way.
const collapseGateChars = 8000

// gateFindings evaluates the escalation gate (CTFG-62 M3) over one scored
// newsletter: every check is a cheap mechanical assertion on the persisted
// outcome, no confidence model involved. A non-empty result marks the
// newsletter for escalation to a stronger model; the items are still persisted
// (flagged best-effort beats an empty reader).
func gateFindings(nl db.Newsletter, res ai.ScoreResult, scoreErr error) []string {
	var out []string

	body := deref(nl.BodyText)
	if len(res.Items) == 1 && res.Shape != ai.ShapeEssay && len([]rune(body)) > collapseGateChars {
		out = append(out, fmt.Sprintf("silent_collapse: 1 item from %d-char body", len([]rune(body))))
	}

	// Snippet checks matter only when the reader will slice segments: that takes
	// more than one item (a lone item renders the newsletter body inline) and
	// the raw HTML the reader slices from.
	segments := len(res.Items) > 1 && nl.RawHTML != nil
	var segText string
	if segments {
		segText = db.SegmentSourceText(*nl.RawHTML)
	}
	for i, it := range res.Items {
		if it.Kind != ai.KindStory {
			continue
		}
		if it.Summary == "" {
			out = append(out, fmt.Sprintf("empty_summary: story %d %q", i, it.Title))
		}
		if !segments {
			continue
		}
		if it.Snippet == "" {
			out = append(out, fmt.Sprintf("missing_snippet: story %d %q", i, it.Title))
			continue
		}
		if !db.SnippetAnchors(segText, it.Snippet) {
			out = append(out, fmt.Sprintf("unanchored_snippet: story %d %q", i, it.Title))
		}
	}

	var tr *ai.TruncatedError
	if errors.As(scoreErr, &tr) {
		out = append(out, "truncated: "+tr.Error())
	}
	return out
}
