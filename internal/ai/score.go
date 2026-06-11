package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Validation bounds applied to model output. They guard the DB (the stories
// CHECK constraints) and keep a runaway model from persisting garbage.
const (
	// MaxSummaryChars caps a story summary; the model is asked for 2-3
	// sentences, this bounds a model that ignores that.
	MaxSummaryChars = 800
	// MaxSnippetChars caps the excerpt.
	MaxSnippetChars = 1000
	// MaxLabels caps secondary tags per item.
	MaxLabels = 5
)

// validKinds is the closed set the stories.kind CHECK enforces. An unknown or
// empty kind from the model is coerced to KindStory ("story") rather than
// dropped — a mis-labeled content item is better kept and scored than lost.
var validKinds = map[string]bool{
	KindStory: true,
	KindBlurb: true,
	KindAd:    true,
	KindPromo: true,
}

// Story kinds, duplicated from internal/db so this package has no DB import.
// Kept in sync with db.Kind* and the stories.kind CHECK.
const (
	KindStory = "story"
	KindBlurb = "blurb"
	KindAd    = "ad"
	KindPromo = "promo"
)

// ItemsSchema returns the JSON Schema passed to Ollama as the structured-output
// format. Requiring a top-level array is what makes the model segment a digest
// into many items instead of collapsing it into a single object; the per-item
// required fields and the kind enum keep the shape predictable. Validation
// still runs in ParseItems — the schema is a strong nudge, not the guarantee.
func ItemsSchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":           map[string]any{"type": "string"},
				"snippet":         map[string]any{"type": "string"},
				"url":             map[string]any{"type": "string"},
				"kind":            map[string]any{"type": "string", "enum": []string{KindStory, KindBlurb, KindAd, KindPromo}},
				"section":         map[string]any{"type": "string"},
				"summary":         map[string]any{"type": "string"},
				"relevance_score": map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
				"primary_topic":   map[string]any{"type": "string"},
				"labels":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"title", "kind", "relevance_score", "primary_topic", "summary"},
		},
	}
}

// ScoredItem is one validated, normalized item the model segmented out of a
// newsletter. The worker maps these onto db.Story rows (segmentation fields)
// and db.Score (scoring fields, story-kind only).
type ScoredItem struct {
	Title          string
	Snippet        string
	URL            string
	Kind           string
	Section        string
	Summary        string
	RelevanceScore int
	PrimaryTopic   string
	Labels         []string
}

// rawItem mirrors the model's per-item JSON with lenient typing. flexInt
// tolerates a relevance_score that arrives as a JSON number or a numeric
// string; flexStrings tolerates labels as an array or a single string.
type rawItem struct {
	Title          string      `json:"title"`
	Snippet        string      `json:"snippet"`
	URL            string      `json:"url"`
	Kind           string      `json:"kind"`
	Section        string      `json:"section"`
	Summary        string      `json:"summary"`
	RelevanceScore flexInt     `json:"relevance_score"`
	PrimaryTopic   string      `json:"primary_topic"`
	Labels         flexStrings `json:"labels"`
}

// TruncatedError signals the model's JSON array was cut off mid-output — a clean
// 2xx whose content is an unterminated array (observed as "unexpected EOF" while
// parsing). Unlike a structural parse error this is *transient*: a re-run often
// completes, so the worker retries within a budget rather than failing outright.
// Recovered is how many complete leading items were salvaged from the partial
// output; ParseItems returns those items alongside this error so the worker can
// keep them once retries are exhausted instead of losing the whole digest.
type TruncatedError struct{ Recovered int }

func (e *TruncatedError) Error() string {
	return fmt.Sprintf("ai: model output truncated (recovered %d complete item(s))", e.Recovered)
}

// ParseItems strictly validates and normalizes a model response into scored
// items. It tolerates the array being wrapped in an object (a common model
// quirk under format:"json") but treats fundamentally unparseable output as an
// error so the worker marks the newsletter failed instead of persisting junk.
//
// An empty array is a valid "nothing here" answer and returns (nil, nil).
// Individual useless items (no title and no snippet) are dropped; if every item
// is dropped from a non-empty response, that's an error (the model returned
// shapes but no content).
//
// When the model's array is truncated (CTFG-33), ParseItems salvages the
// complete leading items and returns them together with a *TruncatedError, so
// the worker can retry for a complete response and fall back to the salvaged
// items rather than discarding everything.
func ParseItems(raw string) ([]ScoredItem, error) {
	arr, truncated, err := extractArray(raw)
	if err != nil {
		return nil, err
	}
	out := normalizeItems(arr)

	if truncated {
		return out, &TruncatedError{Recovered: len(out)}
	}
	if len(arr) == 0 {
		return nil, nil // a valid "nothing here" answer.
	}
	if len(out) == 0 {
		return nil, errors.New("ai: response had items but none were usable")
	}
	return out, nil
}

// normalizeItems validates and normalizes each raw element, dropping the
// unusable ones (one malformed or contentless element never sinks the batch).
func normalizeItems(arr []json.RawMessage) []ScoredItem {
	out := make([]ScoredItem, 0, len(arr))
	for _, rm := range arr {
		var ri rawItem
		if err := json.Unmarshal(rm, &ri); err != nil {
			continue
		}
		if item, ok := normalizeItem(ri); ok {
			out = append(out, item)
		}
	}
	return out
}

// extractArray pulls the JSON array out of a model response. It accepts a bare
// array, an object wrapping the array under a common key (or any array-valued
// field), or a single bare object (treated as a one-element array). It is
// robust to the two things models do even under structured output: wrapping the
// JSON in a ```json fence, and appending trailing prose after the value (so it
// decodes the FIRST value and ignores the rest).
//
// The bool return reports truncation: when the response is an array that was cut
// off mid-output, extractArray returns the complete leading elements with
// truncated=true (and a nil error) so ParseItems can salvage them.
func extractArray(raw string) (items []json.RawMessage, truncated bool, err error) {
	trimmed := stripFence(strings.TrimSpace(raw))
	if trimmed == "" {
		return nil, false, errors.New("ai: empty model response")
	}

	// Bare array — the expected shape. decodeFirst tolerates trailing bytes.
	var arr []json.RawMessage
	if err := decodeFirst(trimmed, &arr); err == nil {
		return arr, false, nil
	}

	// Object: look for an array-valued field (prefer common wrapper keys).
	var obj map[string]json.RawMessage
	if err := decodeFirst(trimmed, &obj); err != nil {
		// Neither a bare array nor an object decoded. If this is an array that
		// was cut off mid-output, salvage the complete leading elements rather
		// than losing the whole digest (CTFG-33). An unclosed array (no matching
		// ']') is the truncation signal; even zero salvaged elements flags
		// truncation so the worker retries.
		if strings.HasPrefix(trimmed, "[") {
			if elems, closed := scanArrayElements(trimmed); !closed {
				return elems, true, nil
			}
		}
		return nil, false, fmt.Errorf("ai: response is neither array nor object: %w", err)
	}
	for _, key := range []string{"items", "stories", "results", "segments", "data"} {
		if v, ok := obj[key]; ok {
			if a, err := asArray(v); err == nil {
				return a, false, nil
			}
		}
	}
	for _, v := range obj {
		if a, err := asArray(v); err == nil {
			return a, false, nil
		}
	}

	// A single bare item object: wrap it. Re-marshal the decoded object so any
	// trailing bytes from the raw response are excluded.
	_, hasTitle := obj["title"]
	_, hasScore := obj["relevance_score"]
	if hasTitle || hasScore {
		one, err := json.Marshal(obj)
		if err != nil {
			return nil, false, fmt.Errorf("ai: re-marshal single item: %w", err)
		}
		return []json.RawMessage{one}, false, nil
	}
	return nil, false, errors.New("ai: response object had no array of items")
}

// scanArrayElements salvages the complete top-level JSON objects from a possibly
// truncated array string. It returns each finished `{...}` element inside the
// outer `[...]` and whether the array was properly closed with `]`. A response
// cut off mid-array yields closed=false plus the elements that completed before
// the truncation, letting the caller keep them. It is string- and escape-aware
// so braces inside string values never confuse the depth count.
func scanArrayElements(s string) (elems []json.RawMessage, closed bool) {
	open := strings.IndexByte(s, '[')
	if open < 0 {
		return nil, false
	}
	for i := open + 1; i < len(s); {
		switch s[i] {
		case ']':
			return elems, true
		case '{':
			obj, end, ok := scanObject(s, i)
			if !ok {
				return elems, false // object cut off mid-output
			}
			elems = append(elems, json.RawMessage(obj))
			i = end
		default:
			i++ // whitespace, commas, stray tokens between elements
		}
	}
	return elems, false
}

// scanObject returns the complete JSON object beginning at s[start]=='{', the
// index just past its closing '}', and ok=false when the object is truncated.
func scanObject(s string, start int) (obj string, end int, ok bool) {
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], i + 1, true
			}
		}
	}
	return "", len(s), false
}

// decodeFirst decodes the first JSON value in s into v, ignoring any trailing
// bytes (json.Unmarshal rejects them; json.Decoder does not). This tolerates a
// model that appends prose after the JSON.
func decodeFirst(s string, v any) error {
	return json.NewDecoder(strings.NewReader(s)).Decode(v)
}

// stripFence removes a single surrounding Markdown code fence (```json ... ```)
// if present, so fenced output still parses.
func stripFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line (```), including an optional language tag.
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	} else {
		return s
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func asArray(v json.RawMessage) ([]json.RawMessage, error) {
	var a []json.RawMessage
	if err := json.Unmarshal(v, &a); err != nil {
		return nil, err
	}
	return a, nil
}

// normalizeItem clamps, trims, and defaults one raw item into a ScoredItem. The
// bool is false when the item carries no usable content (no title and no
// snippet) and should be dropped.
func normalizeItem(ri rawItem) (ScoredItem, bool) {
	title := strings.TrimSpace(ri.Title)
	snippet := truncate(strings.TrimSpace(ri.Snippet), MaxSnippetChars)
	if title == "" && snippet == "" {
		return ScoredItem{}, false
	}

	kind := strings.ToLower(strings.TrimSpace(ri.Kind))
	if !validKinds[kind] {
		kind = KindStory
	}

	score := int(ri.RelevanceScore)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	url := strings.TrimSpace(ri.URL)
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "" // not a resolvable link; drop rather than store junk.
	}

	return ScoredItem{
		Title:          title,
		Snippet:        snippet,
		URL:            url,
		Kind:           kind,
		Section:        strings.TrimSpace(ri.Section),
		Summary:        truncate(strings.TrimSpace(ri.Summary), MaxSummaryChars),
		RelevanceScore: score,
		PrimaryTopic:   strings.TrimSpace(ri.PrimaryTopic),
		Labels:         normalizeLabels(ri.Labels),
	}, true
}

// normalizeLabels trims, de-dupes (case-insensitively), drops empties, and caps
// the count.
func normalizeLabels(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, l := range in {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		key := strings.ToLower(l)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, l)
		if len(out) == MaxLabels {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// truncate is a rune-safe length cap.
func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// flexInt unmarshals a JSON number or numeric string into an int, rounding
// floats. Unparseable values become 0 rather than failing the whole item.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(strings.Trim(string(b), `"`))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		*f = flexInt(int(v + 0.5))
		return nil
	}
	*f = 0
	return nil
}

// flexStrings unmarshals labels whether the model returns an array of strings
// or a single string.
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" || trimmed == "null" {
		*f = nil
		return nil
	}
	if trimmed[0] == '[' {
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			// Tolerate non-string array elements by parsing loosely.
			var loose []any
			if err2 := json.Unmarshal(b, &loose); err2 != nil {
				return nil
			}
			for _, v := range loose {
				if s, ok := v.(string); ok {
					arr = append(arr, s)
				}
			}
		}
		*f = arr
		return nil
	}
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*f = []string{single}
		return nil
	}
	*f = nil
	return nil
}
