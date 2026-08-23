package ai

import (
	"strings"
	"testing"
)

func TestSplitChunksShortBodySingle(t *testing.T) {
	got := splitChunks("short body.", 5000)
	if len(got) != 1 || got[0] != "short body." {
		t.Fatalf("chunks = %q, want the body untouched", got)
	}
}

func TestSplitChunksBalancedAtSentences(t *testing.T) {
	// 40 sentences of ~26 runes ≈ 1040 runes; target 300 → 4 chunks of ~260.
	sentence := "The quick brown fox jumps. "
	body := strings.TrimSpace(strings.Repeat(sentence, 40))
	chunks := splitChunks(body, 300)

	if len(chunks) != 4 {
		t.Fatalf("chunks = %d, want 4 (ceil(len/target))", len(chunks))
	}
	var rejoined []string
	for i, c := range chunks {
		if len([]rune(c)) > 300+60 {
			t.Errorf("chunk %d is %d runes, far over target", i, len([]rune(c)))
		}
		if !strings.HasSuffix(c, ".") {
			t.Errorf("chunk %d does not end at a sentence: %q", i, c[max(0, len(c)-20):])
		}
		if !strings.HasPrefix(c, "The") {
			t.Errorf("chunk %d does not start at a sentence: %q", i, c[:min(20, len(c))])
		}
		rejoined = append(rejoined, c)
	}
	if strings.Join(rejoined, " ") != body {
		t.Error("chunks do not reassemble into the original body")
	}
}

func TestSplitChunksNoSeamsHardCuts(t *testing.T) {
	body := strings.Repeat("x", 1000) // degenerate: no spaces at all
	chunks := splitChunks(body, 300)
	if len(chunks) != 4 {
		t.Fatalf("chunks = %d, want 4", len(chunks))
	}
	if strings.Join(chunks, "") != body {
		t.Error("hard-cut chunks lost content")
	}
}

func TestMergeChunkItemsBoundaryDuplicate(t *testing.T) {
	perChunk := [][]ScoredItem{
		{
			{Title: "Alpha", Kind: KindStory, Summary: "a", Snippet: "alpha opens"},
			{Title: "Split Story", Kind: KindBlurb, Summary: "", Snippet: "split opens", RelevanceScore: 10},
		},
		{
			{Title: "split story", Kind: KindStory, Summary: "the full summary seen by chunk two", RelevanceScore: 40, Labels: []string{"x"}},
			{Title: "Beta", Kind: KindStory, Summary: "b"},
		},
	}
	got := mergeChunkItems(perChunk)
	if len(got) != 3 {
		t.Fatalf("items = %d, want 3 (boundary halves merged)", len(got))
	}
	m := got[1]
	if m.Title != "Split Story" || m.Snippet != "split opens" {
		t.Errorf("merged item lost the opening half's identity: %+v", m)
	}
	if m.Summary != "the full summary seen by chunk two" || m.RelevanceScore != 40 || m.Kind != KindStory {
		t.Errorf("merged item did not prefer the richer half: %+v", m)
	}
}

func TestMergeChunkItemsKeepsLegitimateRepeats(t *testing.T) {
	// A sponsor block repeated mid-digest is NOT a boundary split; only the
	// adjacent last/first pair merges.
	perChunk := [][]ScoredItem{
		{{Title: "Sponsor", Kind: KindAd}, {Title: "Alpha", Kind: KindStory, Summary: "a"}},
		{{Title: "Beta", Kind: KindStory, Summary: "b"}, {Title: "Sponsor", Kind: KindAd}},
	}
	got := mergeChunkItems(perChunk)
	if len(got) != 4 {
		t.Fatalf("items = %d, want 4 (no false merge)", len(got))
	}
}

func TestMergeChunkItemsSkipsEmptyChunks(t *testing.T) {
	perChunk := [][]ScoredItem{
		{{Title: "Alpha", Kind: KindStory, Summary: "a"}},
		nil,
		{{Title: "Beta", Kind: KindStory, Summary: "b"}},
	}
	got := mergeChunkItems(perChunk)
	if len(got) != 2 || got[0].Title != "Alpha" || got[1].Title != "Beta" {
		t.Fatalf("items = %+v, want Alpha then Beta", got)
	}
}

func TestParseShape(t *testing.T) {
	cases := map[string]Shape{
		`{"shape":"essay"}`:      ShapeEssay,
		`{"shape":"digest"}`:     ShapeDigest,
		`{"shape":"DIGEST"}`:     ShapeDigest,
		`{"shape":"newsletter"}`: ShapeUnknown,
		`garbage`:                ShapeUnknown,
		``:                       ShapeUnknown,
	}
	for raw, want := range cases {
		if got := parseShape(raw); got != want {
			t.Errorf("parseShape(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestMergeChunkItemsGapBreaksAdjacency(t *testing.T) {
	// A whole chunk of text sits between the two same-titled items (the middle
	// chunk contributed nothing), so they cannot be halves of one story.
	perChunk := [][]ScoredItem{
		{{Title: "Presented by X", Kind: KindAd}},
		nil,
		{{Title: "Presented by X", Kind: KindAd}},
	}
	got := mergeChunkItems(perChunk)
	if len(got) != 2 {
		t.Fatalf("items = %d, want 2 (gap chunk breaks boundary adjacency)", len(got))
	}
}

func TestMergeSplitStoryBackfillsSnippet(t *testing.T) {
	a := ScoredItem{Title: "Split", Kind: KindStory, Snippet: ""}
	b := ScoredItem{Title: "split", Kind: KindStory, Snippet: "the real anchor", Summary: "s"}
	m := mergeSplitStory(a, b)
	if m.Snippet != "the real anchor" {
		t.Errorf("snippet = %q, want backfilled from the continuation half", m.Snippet)
	}
}
