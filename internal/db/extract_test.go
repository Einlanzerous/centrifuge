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

// The model often renders the snippet with an extra, dropped, or mangled word in
// the opening (it paraphrases instead of copying verbatim). The fuzzy anchor must
// still locate the story; an exact match would return "" and the Reader would
// show a bare summary. Each case is drawn from a real production miss.
func TestExtractSegmentText_FuzzyOpening(t *testing.T) {
	cases := []struct {
		name        string
		body        string // story 1's true text in the newsletter
		snippet     string // the model's (perturbed) rendering of its opening
		nextSnippet string
		nextBody    string
	}{
		{
			name:        "inserted word (is THE quiet)",
			body:        "It’s 8:42 a.m. The security operations center is quiet, save for the hum of server fans.",
			snippet:     "It’s 8:42 a.m. The security operations center is the quiet, save for the hum of server fans",
			nextSnippet: "Second story opens with delta epsilon zeta and more words",
			nextBody:    "Second story opens with delta epsilon zeta and more words here.",
		},
		{
			name:        "doubled word (with with)",
			body:        "We’re partnering with Elevated Chicago and the John D foundation to expand culture near transit.",
			snippet:     "We’re partnering with with Elevated Chicago and the John D foundation to expand",
			nextSnippet: "Next item alpha bravo charlie delta echo here",
			nextBody:    "Next item alpha bravo charlie delta echo here.",
		},
		{
			name:        "garbled tail (expected ARE SAME AS ABOVE)",
			body:        "OpenAI confidentially filed for an IPO that’s expected to launch later this year, joining peers.",
			snippet:     "OpenAI confidentially filed for an IPO that’s expected are same as above",
			nextSnippet: "Jeep recalled thousands of vehicles over a software glitch",
			nextBody:    "Jeep recalled thousands of vehicles over a software glitch this week.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text := c.body + "\n\n" + c.nextBody
			sibs := []siblingSnippet{
				{position: 1, snippet: c.snippet},
				{position: 2, snippet: c.nextSnippet},
			}
			if got := extractSegmentText(text, sibs, 1); got != c.body {
				t.Fatalf("extractSegmentText() = %q, want %q", got, c.body)
			}
		})
	}
}

// Newsletters repeat each story's opening line in a contents block near the top,
// so a snippet aligns both there (a sliver, bounded by the next contents line)
// and at the real body far below. The slice must be the body, not the sliver.
func TestExtractSegmentText_PrefersBodyOverContentsBlock(t *testing.T) {
	toc := "Alpha story opens here about apples and oranges.\n\n" +
		"Bravo story opens here about bananas and cherries.\n\n"
	bodyA := "Alpha story opens here about apples and oranges, with a long detailed paragraph that runs on."
	bodyB := "Bravo story opens here about bananas and cherries, also with its own detailed body paragraph."
	text := toc + bodyA + "\n\n" + bodyB
	sibs := []siblingSnippet{
		{position: 1, snippet: "Alpha story opens here about apples and oranges"},
		{position: 2, snippet: "Bravo story opens here about bananas and cherries"},
	}
	if got := extractSegmentText(text, sibs, 1); got != bodyA {
		t.Fatalf("extractSegmentText() = %q, want %q", got, bodyA)
	}
}

// Two stories can share a first word (or a near-identical opening, a model
// duplication artifact). The next sibling must bound this story at its own real
// location, not a few words in just because the words rhyme — otherwise the slice
// collapses to a sliver (the production "OpenAI" → 6-char bug).
func TestExtractSegmentText_SharedOpeningDoesNotTruncate(t *testing.T) {
	body1 := "OpenAI filed for an IPO expected to launch later this year joining peers in the market."
	body2 := "OpenAI also released a new model family for developers earlier in the same week."
	text := body1 + "\n\n" + body2
	sibs := []siblingSnippet{
		{position: 1, snippet: "OpenAI filed for an IPO expected to launch later this year joining peers"},
		{position: 2, snippet: "OpenAI also released a new model family for developers earlier"},
	}
	if got := extractSegmentText(text, sibs, 1); got != body1 {
		t.Fatalf("extractSegmentText() = %q, want %q", got, body1)
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

func TestHTMLToTextStripsInvisible(t *testing.T) {
	// Newsletters pad text with zero-width spacer runes (often in long runs) and
	// NBSP. The Reader's extracted text must drop the zero-width ones and fold
	// NBSP into a single space (CTFG-58).
	in := "<p>OpenAI announced it\u200c\u200c\u200c\u00a0\u00a0would acquire\ufeff Ona\u00ad.</p>"
	want := "OpenAI announced it would acquire Ona."
	if got := htmlToText(in); got != want {
		t.Fatalf("htmlToText() = %q, want %q", got, want)
	}
}
