package db

import "testing"

func TestHTMLToText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "inline concat keeps styled first letter and parenthesized link intact",
			in:   `<p><span>T</span>reated mice (<a href="x">scroll</a>) ran.</p><p>Second paragraph.</p>`,
			want: "Treated mice (scroll) ran.\n\nSecond paragraph.",
		},
		{
			name: "script and style subtrees are dropped",
			in:   `<div>Keep this.<script>junk()</script><style>.a{}</style></div>`,
			want: "Keep this.",
		},
		{
			name: "br breaks within a paragraph; list items become separate blocks",
			in:   `<p>One<br>Two</p><ul><li>a</li><li>b</li></ul>`,
			want: "One\nTwo\n\na\n\nb",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := htmlToText(c.in); got != c.want {
				t.Fatalf("htmlToText() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestExtractSegmentText_PhysicalBoundary(t *testing.T) {
	// Story B is numbered (position 2) before the sponsor (position 3), but the
	// sponsor physically sits between story A and story B. The slice for A must
	// stop at the sponsor, not run to story B.
	text := "Story A opening words here about alpha and more.\n\n" +
		"In partnership with a sponsor block about beta promo here.\n\n" +
		"Story B opening words about gamma and delta follow."
	sibs := []siblingSnippet{
		{position: 1, snippet: "Story A opening words here about alpha and more"},
		{position: 3, snippet: "In partnership with a sponsor block about beta promo"},
		{position: 2, snippet: "Story B opening words about gamma and delta follow"},
	}
	got := extractSegmentText(text, sibs, 1)
	want := "Story A opening words here about alpha and more."
	if got != want {
		t.Fatalf("extractSegmentText() = %q, want %q", got, want)
	}
}

func TestExtractSegmentText_LastSegmentRunsToEnd(t *testing.T) {
	text := "First story alpha beta gamma here.\n\nSecond story delta epsilon zeta here."
	sibs := []siblingSnippet{
		{position: 1, snippet: "First story alpha beta gamma here"},
		{position: 2, snippet: "Second story delta epsilon zeta here"},
	}
	got := extractSegmentText(text, sibs, 2)
	want := "Second story delta epsilon zeta here."
	if got != want {
		t.Fatalf("extractSegmentText() = %q, want %q", got, want)
	}
}

func TestExtractSegmentText_MissReturnsEmpty(t *testing.T) {
	text := "Nothing here matches the anchor at all."
	sibs := []siblingSnippet{{position: 1, snippet: "totally different opening words entirely"}}
	if got := extractSegmentText(text, sibs, 1); got != "" {
		t.Fatalf("extractSegmentText() = %q, want empty", got)
	}
}

func TestExtractSegmentText_TrimsNextLeadIn(t *testing.T) {
	// Reproduces the IT Brew bleed (CTFG-44): the next item's section label,
	// title, and byline physically precede its snippet, so without trimming they
	// bleed into this story. This story keeps its own em-dash byline.
	text := "Microsoft Research found that LLMs corrupt a doc during long workflows.\n\n" +
		"How to keep document integrity intact during agentic workflows.—BH\n\n" +
		"CYBERSECURITY\n\n" +
		"There will be bugs\n\n" +
		"Rich Mogull\n\n" +
		"Sorry, tired security pro: you will need to patch faster than ever."
	sibs := []siblingSnippet{
		{position: 2, snippet: "Microsoft Research found that LLMs corrupt a doc during long workflows", title: "Making degrade"},
		{position: 4, snippet: "Sorry, tired security pro: you will need to patch faster than ever", title: "There will be bugs"},
	}
	got := extractSegmentText(text, sibs, 2)
	want := "Microsoft Research found that LLMs corrupt a doc during long workflows.\n\n" +
		"How to keep document integrity intact during agentic workflows.—BH"
	if got != want {
		t.Fatalf("extractSegmentText() = %q, want %q", got, want)
	}
}

func TestExtractSegmentText_KeepsOwnBylineWhenNextTitleAbsent(t *testing.T) {
	// The next title is garbled (temperature artifact) and not present verbatim,
	// so the lead-in trim must not fire; fall back to the snippet boundary and
	// keep this story's own byline.
	text := "First story alpha beta gamma here, a real sentence.—SK\n\n" +
		"Second story delta epsilon zeta here, another sentence."
	sibs := []siblingSnippet{
		{position: 1, snippet: "First story alpha beta gamma here, a real sentence", title: "Alpha Roundup"},
		{position: 2, snippet: "Second story delta epsilon zeta here, another sentence", title: "qwxv mangled nonsense"},
	}
	got := extractSegmentText(text, sibs, 1)
	want := "First story alpha beta gamma here, a real sentence.—SK"
	if got != want {
		t.Fatalf("extractSegmentText() = %q, want %q", got, want)
	}
}

func TestExtractSegmentText_ShortNextTitleFallsBackToSnippet(t *testing.T) {
	// A 1-3 char next title is too short to anchor safely, so trimming is skipped
	// and the snippet boundary stands (no panic, no over-trim).
	text := "Lead story words about alpha and beta and gamma here.\n\n" +
		"AI\n\nNext story words about delta and epsilon here."
	sibs := []siblingSnippet{
		{position: 1, snippet: "Lead story words about alpha and beta and gamma here", title: "Lead Story"},
		{position: 2, snippet: "Next story words about delta and epsilon here", title: "AI"},
	}
	got := extractSegmentText(text, sibs, 1)
	want := "Lead story words about alpha and beta and gamma here.\n\nAI"
	if got != want {
		t.Fatalf("extractSegmentText() = %q, want %q", got, want)
	}
}

func TestIsSectionLabel(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"CYBERSECURITY", true},
		{"TOP NEWS", true},
		{"AI & DATA", true},
		{"Making degrade", false}, // mixed case
		{"—BH", false},            // byline
		{"-SK", false},            // byline
		{"", false},
		{"A VERY LONG ALL CAPS LINE THAT IS REALLY A SENTENCE NOT A LABEL", false}, // too long
	}
	for _, c := range cases {
		if got := isSectionLabel(c.in); got != c.want {
			t.Errorf("isSectionLabel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
