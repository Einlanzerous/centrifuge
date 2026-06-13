package ai

import (
	"errors"
	"strings"
	"testing"
)

func TestParseItemsHappyPath(t *testing.T) {
	raw := `[
	  {"title":"Big AI news","snippet":"A model shipped","url":"https://ex.com/a",
	   "kind":"story","section":"Top","summary":"It happened.","relevance_score":92,
	   "primary_topic":"AI engineering","labels":["llm","release"]},
	  {"title":"Sponsored","kind":"ad","relevance_score":0,"snippet":"buy this"}
	]`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	a := items[0]
	if a.Title != "Big AI news" || a.URL != "https://ex.com/a" || a.Kind != "story" {
		t.Errorf("item0 = %+v", a)
	}
	if a.RelevanceScore != 92 || a.PrimaryTopic != "AI engineering" {
		t.Errorf("item0 scoring = %d / %q", a.RelevanceScore, a.PrimaryTopic)
	}
	if len(a.Labels) != 2 {
		t.Errorf("item0 labels = %v", a.Labels)
	}
	if items[1].Kind != "ad" {
		t.Errorf("item1 kind = %q", items[1].Kind)
	}
}

func TestParseItemsWrappedObject(t *testing.T) {
	for _, raw := range []string{
		`{"items":[{"title":"x","kind":"story"}]}`,
		`{"stories":[{"title":"x","kind":"story"}]}`,
		`{"results":[{"title":"x"}]}`,
	} {
		items, err := ParseItems(raw)
		if err != nil {
			t.Fatalf("ParseItems(%q): %v", raw, err)
		}
		if len(items) != 1 || items[0].Title != "x" {
			t.Errorf("ParseItems(%q) = %+v", raw, items)
		}
	}
}

func TestParseItemsSingleBareObject(t *testing.T) {
	raw := `{"title":"Lone essay","kind":"story","relevance_score":70,"summary":"One thing."}`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Lone essay" || items[0].RelevanceScore != 70 {
		t.Errorf("items = %+v", items)
	}
}

func TestParseItemsLenientTypes(t *testing.T) {
	// score as a string, labels as a single string, float score rounds.
	raw := `[{"title":"a","relevance_score":"85","labels":"solo"},
	         {"title":"b","relevance_score":12.6,"labels":["x","x","  ","y"]}]`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if items[0].RelevanceScore != 85 {
		t.Errorf("string score = %d, want 85", items[0].RelevanceScore)
	}
	if len(items[0].Labels) != 1 || items[0].Labels[0] != "solo" {
		t.Errorf("single-string labels = %v", items[0].Labels)
	}
	if items[1].RelevanceScore != 13 {
		t.Errorf("float score = %d, want 13 (rounded)", items[1].RelevanceScore)
	}
	if len(items[1].Labels) != 2 { // x deduped, blank dropped
		t.Errorf("labels = %v, want [x y]", items[1].Labels)
	}
}

func TestParseItemsClampAndCoerce(t *testing.T) {
	raw := `[{"title":"hi","relevance_score":250,"kind":"editorial"},
	         {"title":"lo","relevance_score":-5,"kind":""}]`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if items[0].RelevanceScore != 100 {
		t.Errorf("over-max score = %d, want clamped 100", items[0].RelevanceScore)
	}
	if items[0].Kind != "story" {
		t.Errorf("unknown kind = %q, want coerced story", items[0].Kind)
	}
	if items[1].RelevanceScore != 0 {
		t.Errorf("negative score = %d, want clamped 0", items[1].RelevanceScore)
	}
	if items[1].Kind != "story" {
		t.Errorf("empty kind = %q, want story", items[1].Kind)
	}
}

func TestParseItemsDropsEmptyAndJunkURL(t *testing.T) {
	raw := `[{"title":"keep","url":"mailto:x@y.com"},
	         {"kind":"blurb"},
	         {"snippet":"only snippet"}]`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (empty one dropped)", len(items))
	}
	if items[0].URL != "" {
		t.Errorf("non-http URL = %q, want dropped", items[0].URL)
	}
}

func TestParseItemsToleratesTrailingJunk(t *testing.T) {
	// Models occasionally append prose after the JSON, even under structured
	// output (this is the '<' after top-level value failure seen in eval).
	raw := `[{"title":"a","kind":"story","relevance_score":50}]

<end of response>`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len(items) != 1 || items[0].Title != "a" {
		t.Errorf("items = %+v", items)
	}
}

func TestParseItemsStripsCodeFence(t *testing.T) {
	raw := "```json\n[{\"title\":\"fenced\",\"kind\":\"story\"}]\n```"
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len(items) != 1 || items[0].Title != "fenced" {
		t.Errorf("items = %+v", items)
	}
}

func TestParseItemsSingleObjectWithTrailingJunk(t *testing.T) {
	raw := `{"title":"solo","kind":"story","relevance_score":40} trailing words`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len(items) != 1 || items[0].Title != "solo" || items[0].RelevanceScore != 40 {
		t.Errorf("items = %+v", items)
	}
}

func TestParseItemsEmptyArray(t *testing.T) {
	items, err := ParseItems(`[]`)
	if err != nil {
		t.Fatalf("ParseItems([]) error: %v", err)
	}
	if items != nil {
		t.Errorf("items = %v, want nil for empty array", items)
	}
}

func TestParseItemsSalvagesTruncatedArray(t *testing.T) {
	// A digest cut off mid-third-object (CTFG-33): the two complete items are
	// salvaged and a *TruncatedError is returned so the worker can retry and
	// fall back to the partial result instead of losing the whole newsletter.
	raw := `[{"title":"first","kind":"story","relevance_score":90},` +
		`{"title":"second","kind":"blurb","relevance_score":40},` +
		`{"title":"thir`
	items, err := ParseItems(raw)
	var te *TruncatedError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TruncatedError", err)
	}
	if len(items) != 2 || items[0].Title != "first" || items[1].Title != "second" {
		t.Fatalf("salvaged items = %+v, want the two complete ones", items)
	}
	if te.Recovered != 2 {
		t.Errorf("Recovered = %d, want 2", te.Recovered)
	}
}

func TestParseItemsTruncatedBeforeFirstItem(t *testing.T) {
	// Cut off before any object closes: truncation is still signaled (the worker
	// retries) but nothing is salvaged.
	items, err := ParseItems(`[{"title":"only par`)
	var te *TruncatedError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TruncatedError", err)
	}
	if len(items) != 0 {
		t.Errorf("items = %+v, want none salvaged", items)
	}
}

func TestParseItemsSalvagesPartialFirstObject(t *testing.T) {
	// The recurring gemma temp-0 failure (CTFG-46): a runaway "/" run inside the
	// trailing "url" string of the FIRST object never closes it, so the digest
	// truncates with zero *complete* objects. The fields that finished before the
	// doomed url — title, snippet, summary, score — are still good and must be
	// recovered; otherwise recovered=0 loses the entire newsletter (and the worker
	// marks it failed).
	raw := `[{"title":"Ride with Pride","kind":"story","relevance_score":95,` +
		`"primary_topic":"transit","summary":"A pride train.","snippet":"We unveiled it.",` +
		`"url":"//////////////////////////////////////////`
	items, err := ParseItems(raw)
	var te *TruncatedError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TruncatedError", err)
	}
	if len(items) != 1 {
		t.Fatalf("salvaged items = %+v, want 1 partial item", items)
	}
	if items[0].Title != "Ride with Pride" || items[0].Summary != "A pride train." || items[0].RelevanceScore != 95 {
		t.Errorf("recovered item lost completed fields: %+v", items[0])
	}
	if items[0].URL != "" {
		t.Errorf("URL = %q, want empty (incomplete trailing field dropped)", items[0].URL)
	}
	if te.Recovered != 1 {
		t.Errorf("Recovered = %d, want 1", te.Recovered)
	}
}

func TestScanArrayElements(t *testing.T) {
	// Braces inside a string value and a nested object must not confuse the depth
	// count; a properly closed array reports closed=true.
	closed := `[{"title":"a } brace in string","labels":["x"]},{"title":"b","meta":{"k":"v"}}]`
	elems, ok := scanArrayElements(closed)
	if !ok {
		t.Error("closed array reported truncated")
	}
	if len(elems) != 2 {
		t.Fatalf("got %d elems, want 2", len(elems))
	}

	// Truncated mid-second-object after a complete pair: the finished first
	// element plus a partial salvage of the second (its completed "title":"b"
	// pair), closed=false. The dangling "x": is dropped.
	elems, ok = scanArrayElements(`[{"title":"a"},{"title":"b","x":`)
	if ok {
		t.Error("truncated array reported closed")
	}
	if len(elems) != 2 {
		t.Fatalf("got %d elems, want complete first + partial second", len(elems))
	}
	if string(elems[1]) != `{"title":"b"}` {
		t.Errorf("partial salvage = %s, want {\"title\":\"b\"}", elems[1])
	}

	// Truncated inside the very first value, before any pair completes: nothing
	// to salvage from that object.
	elems, ok = scanArrayElements(`[{"title":"a } brace`)
	if ok {
		t.Error("truncated array reported closed")
	}
	if len(elems) != 0 {
		t.Errorf("got %d elems, want 0 (no complete pair)", len(elems))
	}
}

func TestParseItemsErrors(t *testing.T) {
	cases := map[string]string{
		"garbage":         "not json at all",
		"empty":           "   ",
		"object no array": `{"foo":"bar","n":3}`,
		"all dropped":     `[{"kind":"story"},{"relevance_score":5}]`,
	}
	for name, raw := range cases {
		if _, err := ParseItems(raw); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseItemsTruncates(t *testing.T) {
	long := strings.Repeat("x", MaxSummaryChars+500)
	longSnip := strings.Repeat("y", MaxSnippetChars+500)
	raw := `[{"title":"t","summary":"` + long + `","snippet":"` + longSnip + `"}]`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len([]rune(items[0].Summary)) != MaxSummaryChars {
		t.Errorf("summary len = %d, want %d", len([]rune(items[0].Summary)), MaxSummaryChars)
	}
	if len([]rune(items[0].Snippet)) != MaxSnippetChars {
		t.Errorf("snippet len = %d, want %d", len([]rune(items[0].Snippet)), MaxSnippetChars)
	}
}

func TestParseItemsLabelCap(t *testing.T) {
	raw := `[{"title":"t","labels":["a","b","c","d","e","f","g"]}]`
	items, err := ParseItems(raw)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	if len(items[0].Labels) != MaxLabels {
		t.Errorf("labels = %d, want capped at %d", len(items[0].Labels), MaxLabels)
	}
}

func TestBuildPromptIncludesTopicsAndBody(t *testing.T) {
	p := BuildPrompt(PromptInput{
		SourceName: "Morning Brew",
		Subject:    "Markets today",
		Body:       "Stocks went up.",
		Topics:     []string{"AI engineering", "nuclear"},
	})
	for _, want := range []string{"AI engineering", "nuclear", "Morning Brew", "Markets today", "Stocks went up.", "JSON array"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildPromptEmptyTopics(t *testing.T) {
	p := BuildPrompt(PromptInput{Body: "x"})
	if !strings.Contains(p, "none specified") {
		t.Errorf("empty topics not handled: %s", p)
	}
}
