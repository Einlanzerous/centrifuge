package worker

import (
	"strings"
	"testing"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/db"
)

func nlWith(body string, rawHTML string) db.Newsletter {
	nl := db.Newsletter{}
	if body != "" {
		nl.BodyText = &body
	}
	if rawHTML != "" {
		nl.RawHTML = &rawHTML
	}
	return nl
}

func hasFinding(findings []string, prefix string) bool {
	for _, f := range findings {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

func TestGateSilentCollapse(t *testing.T) {
	big := strings.Repeat("word ", 2000) // ~10k chars
	one := []ai.ScoredItem{{Title: "Dump", Kind: ai.KindStory, Summary: "s", Snippet: "x"}}

	got := gateFindings(nlWith(big, ""), ai.ScoreResult{Items: one, Shape: ai.ShapeDigest, Chunks: 2}, nil)
	if !hasFinding(got, "silent_collapse") {
		t.Errorf("digest with 1 item from big body: findings = %v, want silent_collapse", got)
	}

	got = gateFindings(nlWith(big, ""), ai.ScoreResult{Items: one, Shape: ai.ShapeEssay, Chunks: 1}, nil)
	if hasFinding(got, "silent_collapse") {
		t.Errorf("a probed essay's single story is correct: findings = %v", got)
	}

	got = gateFindings(nlWith("small body", ""), ai.ScoreResult{Items: one, Chunks: 1}, nil)
	if hasFinding(got, "silent_collapse") {
		t.Errorf("small body cannot collapse: findings = %v", got)
	}
}

func TestGateEmptySummary(t *testing.T) {
	items := []ai.ScoredItem{
		{Title: "A", Kind: ai.KindStory, Summary: ""},
		{Title: "B", Kind: ai.KindBlurb, Summary: ""}, // blurbs need no summary
	}
	got := gateFindings(nlWith("body", ""), ai.ScoreResult{Items: items, Chunks: 1}, nil)
	if !hasFinding(got, "empty_summary") {
		t.Errorf("findings = %v, want empty_summary for the story", got)
	}
	if n := len(got); n != 1 {
		t.Errorf("findings = %v, want exactly one (blurb exempt)", got)
	}
}

func TestGateSnippetChecksNeedSegmentsAndHTML(t *testing.T) {
	rawHTML := "<html><body><p>Intro line.</p><p>The quick brown fox story begins right here in earnest.</p><p>Another item follows later.</p></body></html>"
	anchored := ai.ScoredItem{Title: "A", Kind: ai.KindStory, Summary: "s",
		Snippet: "The quick brown fox story begins right here in earnest."}
	unanchored := ai.ScoredItem{Title: "B", Kind: ai.KindStory, Summary: "s",
		Snippet: "Completely fabricated words appearing nowhere in the email at all."}
	missing := ai.ScoredItem{Title: "C", Kind: ai.KindStory, Summary: "s", Snippet: ""}

	got := gateFindings(nlWith("body", rawHTML),
		ai.ScoreResult{Items: []ai.ScoredItem{anchored, unanchored, missing}, Chunks: 1}, nil)
	if hasFinding(got, "unanchored_snippet: story 0") {
		t.Errorf("findings = %v: the verbatim snippet must anchor", got)
	}
	if !hasFinding(got, "unanchored_snippet: story 1") {
		t.Errorf("findings = %v, want unanchored_snippet for the fabricated one", got)
	}
	if !hasFinding(got, "missing_snippet: story 2") {
		t.Errorf("findings = %v, want missing_snippet", got)
	}

	// A lone item renders inline (no slicing) and a text-only newsletter has no
	// HTML to slice: neither runs snippet checks.
	got = gateFindings(nlWith("body", rawHTML), ai.ScoreResult{Items: []ai.ScoredItem{missing}, Chunks: 1}, nil)
	if hasFinding(got, "missing_snippet") {
		t.Errorf("findings = %v: lone item needs no snippet", got)
	}
	got = gateFindings(nlWith("body", ""), ai.ScoreResult{Items: []ai.ScoredItem{anchored, missing}, Chunks: 1}, nil)
	if hasFinding(got, "missing_snippet") {
		t.Errorf("findings = %v: no raw HTML, nothing to slice", got)
	}
}

func TestGateTruncationRecorded(t *testing.T) {
	items := []ai.ScoredItem{{Title: "A", Kind: ai.KindStory, Summary: "s"}}
	err := &ai.TruncatedError{Recovered: 1, Reason: "1 of 2 chunk(s) truncated"}
	got := gateFindings(nlWith("body", ""), ai.ScoreResult{Items: items, Chunks: 2}, err)
	if !hasFinding(got, "truncated") {
		t.Errorf("findings = %v, want truncated", got)
	}
}

func TestGateUnknownShapeNeverCollapseFlagged(t *testing.T) {
	// Without an affirmative digest probe (chunking disabled, short body, or an
	// unusable probe answer) a lone item carries no collapse evidence — a long
	// single-story essay scored whole must not be flagged on every clean pass.
	big := strings.Repeat("word ", 2000)
	one := []ai.ScoredItem{{Title: "Essay", Kind: ai.KindStory, Summary: "s", Snippet: "x"}}
	got := gateFindings(nlWith(big, ""), ai.ScoreResult{Items: one, Shape: ai.ShapeUnknown, Chunks: 1}, nil)
	if hasFinding(got, "silent_collapse") {
		t.Errorf("findings = %v: unprobed shape must not trip silent_collapse", got)
	}
}

func TestGatePartialEmptyChunksRecorded(t *testing.T) {
	items := []ai.ScoredItem{{Title: "A", Kind: ai.KindStory, Summary: "s"}}
	got := gateFindings(nlWith("body", ""), ai.ScoreResult{Items: items, Shape: ai.ShapeDigest, Chunks: 3, EmptyChunks: 1}, nil)
	if !hasFinding(got, "empty_chunks") {
		t.Errorf("findings = %v, want empty_chunks for a partial [] chunk", got)
	}
}

func TestGateNonStorySnippetsChecked(t *testing.T) {
	// The reader bounds story segments on ALL sibling snippets, so an ad with
	// no snippet bleeds into its neighbor — flag it like any other.
	rawHTML := "<html><body><p>Story one begins here with a full and proper opening sentence.</p><p>Sponsored content follows.</p></body></html>"
	items := []ai.ScoredItem{
		{Title: "A", Kind: ai.KindStory, Summary: "s", Snippet: "Story one begins here with a full and proper opening sentence."},
		{Title: "Sponsor", Kind: ai.KindAd, Snippet: ""},
	}
	got := gateFindings(nlWith("body", rawHTML), ai.ScoreResult{Items: items, Chunks: 1}, nil)
	if !hasFinding(got, "missing_snippet: ad 1") {
		t.Errorf("findings = %v, want missing_snippet for the ad", got)
	}
}
