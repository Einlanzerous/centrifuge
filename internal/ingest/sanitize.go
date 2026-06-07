package ingest

import (
	"strings"

	"golang.org/x/net/html"
)

// DefaultMaxBodyChars bounds the cleaned body text persisted with each
// newsletter and later fed to the scorer, keeping a large digest from blowing
// the model's context window. It is overridable per-Ingestor (INGEST_MAX_CHARS).
const DefaultMaxBodyChars = 24000

// skippableTags are elements whose text content is noise to a relevance scorer
// (scripts, styles, document head). Their subtrees are dropped entirely.
var skippableTags = map[string]bool{
	"script":   true,
	"style":    true,
	"head":     true,
	"title":    true,
	"noscript": true,
}

// CleanText turns raw HTML into a whitespace-normalized plaintext stream for the
// scorer. The sane default (per CTFG-1's open question on sanitization depth):
// drop script/style/head/title/noscript subtrees and HTML comments, decode
// entities, flatten remaining text to a single whitespace-collapsed stream, and
// truncate to maxChars (preferring the lead content). maxChars <= 0 disables
// truncation. raw_html is kept verbatim in the DB; only this derived text is
// what the model sees.
func CleanText(rawHTML string, maxChars int) string {
	return truncateChars(collapseSpaces(extractText(rawHTML)), maxChars)
}

// extractText walks the HTML token stream, accumulating text outside skippable
// subtrees. It uses a tokenizer rather than a full parse so it stays cheap and
// never panics on malformed markup.
func extractText(rawHTML string) string {
	z := html.NewTokenizer(strings.NewReader(rawHTML))
	var b strings.Builder
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String() // io.EOF or a parse error — return what we have
		case html.StartTagToken:
			if name, _ := z.TagName(); skippableTags[string(name)] {
				skip++
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); skippableTags[string(name)] && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
				b.WriteByte(' ')
			}
		}
	}
}

// collapseSpaces normalizes every run of whitespace to a single space and trims
// the ends. Document structure is intentionally flattened.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncateChars caps s to at most maxChars runes. maxChars <= 0 means no limit.
func truncateChars(s string, maxChars int) string {
	if maxChars <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
