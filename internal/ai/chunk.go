package ai

import (
	"strings"
	"unicode"
)

// DefaultChunkChars is the target chunk size (in runes of prepped body text)
// for digest scoring. ~5k is where segmentation stays healthy for both local
// models measured; morning_brew collapsed to a single item at 6,010 chars but
// segmented at 5,085, so the threshold errs low (CTFG-62,
// design/2026-08-13-muse-glimmer-scoring-eval.md).
const DefaultChunkChars = 5000

// splitChunks splits flat prepped body text into pieces of roughly target runes
// each, cutting at sentence boundaries. The prepped text has no line structure
// (ingest.CleanText collapses all whitespace), so sentence ends are the only
// reliable seams. Sizes are balanced across ceil(len/target) chunks rather than
// greedily packed, so no run ends in a sliver chunk and no chunk drifts far
// above target. target <= 0 or a body within target returns the body as one
// chunk.
func splitChunks(body string, target int) []string {
	runes := []rune(body)
	if target <= 0 || len(runes) <= target {
		return []string{body}
	}

	n := (len(runes) + target - 1) / target // chunk count, ceil
	per := len(runes) / n                   // balanced size
	window := per / 5                       // boundary search span around each ideal cut

	var out []string
	start := 0
	for c := 1; c < n; c++ {
		ideal := start + per
		if ideal >= len(runes) {
			break
		}
		cut := findSentenceCut(runes, ideal, window)
		if cut <= start {
			cut = ideal // degenerate text with no seam: hard cut
		}
		out = append(out, strings.TrimSpace(string(runes[start:cut])))
		start = cut
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

// findSentenceCut returns the index just past the sentence end nearest ideal,
// searching within ±window runes. A sentence end is a terminator rune followed
// by a space. Falls back to the nearest space, then to ideal itself.
func findSentenceCut(runes []rune, ideal, window int) int {
	lo, hi := ideal-window, ideal+window
	if lo < 1 {
		lo = 1
	}
	if hi > len(runes)-1 {
		hi = len(runes) - 1
	}

	bestSentence, bestSpace := -1, -1
	for d := 0; d <= hi-lo; d++ {
		// Alternate outward from ideal so the nearest seam wins.
		for _, i := range [2]int{ideal - d, ideal + d} {
			if i < lo || i > hi {
				continue
			}
			if runes[i] == ' ' {
				if bestSpace < 0 {
					bestSpace = i + 1
				}
				if isSentenceEnd(runes[i-1]) {
					bestSentence = i + 1
				}
			}
			if bestSentence >= 0 {
				return bestSentence
			}
		}
	}
	if bestSpace >= 0 {
		return bestSpace
	}
	return ideal
}

// isSentenceEnd reports whether r terminates a sentence. Closing quotes and
// brackets directly after a terminator are rare in prepped newsletter text and
// not worth chasing.
func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '…':
		return true
	}
	return unicode.Is(unicode.STerm, r)
}

// mergeChunkItems concatenates per-chunk items in reading order and repairs the
// one artifact chunking introduces: a story cut by a chunk boundary appears
// twice — its opening ends one chunk, its continuation starts the next. When
// the last item of a chunk and the first item of the *immediately following*
// chunk carry the same normalized title they are one story, merged preferring
// the first occurrence's opening (its snippet anchors the story's true start)
// and the richer of the two summaries. Titles elsewhere are left alone —
// digests repeat sponsor blocks legitimately — and a gap chunk between two
// same-titled items (empty or failed, contributing nothing) breaks adjacency:
// items separated by a whole chunk of text cannot be halves of one story.
func mergeChunkItems(perChunk [][]ScoredItem) []ScoredItem {
	var out []ScoredItem
	lastChunk := -2 // index of the chunk whose items currently end out
	for ci, items := range perChunk {
		if len(items) == 0 {
			continue
		}
		rest := items
		if len(out) > 0 && ci == lastChunk+1 && sameStory(out[len(out)-1], items[0]) {
			out[len(out)-1] = mergeSplitStory(out[len(out)-1], items[0])
			rest = items[1:]
		}
		out = append(out, rest...)
		lastChunk = ci
	}
	return out
}

// sameStory reports whether two boundary-adjacent items are halves of one story
// split by a chunk cut, matched on normalized title.
func sameStory(a, b ScoredItem) bool {
	ta, tb := normalizeTitle(a.Title), normalizeTitle(b.Title)
	return ta != "" && ta == tb
}

// normalizeTitle lowercases and collapses whitespace for boundary matching.
func normalizeTitle(t string) string {
	return strings.ToLower(strings.Join(strings.Fields(t), " "))
}

// mergeSplitStory folds the continuation half b into the opening half a. The
// opening keeps title, snippet, and section (they locate the story in the
// newsletter); the summary and topic prefer whichever half has substance; the
// score takes the max (the half that saw more of the body judged better);
// labels union up to the cap.
func mergeSplitStory(a, b ScoredItem) ScoredItem {
	// The opening half's snippet is preferred (it anchors the story's true
	// start) but an empty one is useless to the reader — backfill from the
	// continuation so the segment still anchors somewhere.
	if a.Snippet == "" {
		a.Snippet = b.Snippet
	}
	if a.Section == "" {
		a.Section = b.Section
	}
	if len(b.Summary) > len(a.Summary) {
		a.Summary = b.Summary
	}
	if a.PrimaryTopic == "" {
		a.PrimaryTopic = b.PrimaryTopic
	}
	if b.RelevanceScore > a.RelevanceScore {
		a.RelevanceScore = b.RelevanceScore
	}
	if a.Kind != KindStory && b.Kind == KindStory {
		a.Kind = KindStory
	}
	a.Labels = normalizeLabels(append(a.Labels, b.Labels...))
	return a
}
