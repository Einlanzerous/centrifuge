package db

// Exported wrappers over the reader's segment-anchoring internals, so the
// scoring worker's escalation gate (CTFG-62) can verify — at scoring time, with
// the exact matcher the reader will use — that a story's snippet will locate
// its segment. A snippet that fails here renders as an empty story body.

// SegmentSourceText renders the text the reader slices story segments from: the
// newsletter's raw HTML flattened to text with paragraph breaks intact. Gate
// checks must anchor against this, not body_text — the two renderings differ.
func SegmentSourceText(rawHTML string) string { return htmlToText(rawHTML) }

// SnippetAnchors reports whether snippet confidently aligns somewhere in text
// (produced by SegmentSourceText), using the same fuzzy matcher segment
// extraction uses: verbatim first word, ≥60% of the opening tokens in order.
func SnippetAnchors(text, snippet string) bool {
	anchor := anchorTokens(snippet)
	if len(anchor) == 0 {
		return false
	}
	return len(anchorMatches(tokenize(text), anchor, 0)) > 0
}
