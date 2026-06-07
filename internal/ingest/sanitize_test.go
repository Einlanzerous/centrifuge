package ingest

import (
	"strings"
	"testing"
)

func TestCleanTextStripsNoiseAndExtracts(t *testing.T) {
	raw := `<html><head><title>Ignore me</title>` +
		`<style>.x{color:red}</style></head>` +
		`<body><script>alert(1)</script>` +
		`<h1>Top Story</h1><p>Markets &amp; trains rose today.</p>` +
		`<noscript>enable js</noscript></body></html>`

	got := CleanText(raw, 0)
	want := "Top Story Markets & trains rose today."
	if got != want {
		t.Errorf("CleanText = %q, want %q", got, want)
	}
	for _, noise := range []string{"Ignore me", "color:red", "alert(1)", "enable js"} {
		if strings.Contains(got, noise) {
			t.Errorf("CleanText leaked %q: %q", noise, got)
		}
	}
}

func TestCleanTextTruncates(t *testing.T) {
	raw := "<p>" + strings.Repeat("a", 100) + "</p>"
	got := CleanText(raw, 10)
	if len([]rune(got)) != 10 {
		t.Errorf("len = %d, want 10", len([]rune(got)))
	}
}

func TestCleanTextMalformedHTMLNoPanic(t *testing.T) {
	// Unclosed tags / stray brackets must not panic and should still yield text.
	got := CleanText("<p>hello <b>world", 0)
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("CleanText = %q, want hello+world", got)
	}
}

func TestTruncateCharsRuneSafe(t *testing.T) {
	// Truncation counts runes, not bytes — multibyte chars must not be split.
	got := truncateChars("héllo wörld", 5)
	if got != "héllo" {
		t.Errorf("truncateChars = %q, want %q", got, "héllo")
	}
}
