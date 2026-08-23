package ai

import (
	"fmt"
	"strings"
)

// Shape classifies a newsletter's layout for the chunking decision (CTFG-62).
// Digests are split into chunks before scoring; a single continuous essay must
// never be split — chunking one yields fragment "stories" where exactly one is
// correct.
type Shape string

const (
	// ShapeUnknown means the layout was not probed — the newsletter was short
	// enough to score whole, or the probe's answer was unusable.
	ShapeUnknown Shape = ""
	ShapeEssay   Shape = "essay"
	ShapeDigest  Shape = "digest"
)

// shapeSchema is the structured-output format for the layout probe: exactly
// {"shape": "essay"} or {"shape": "digest"}.
func shapeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"shape": map[string]any{"type": "string", "enum": []string{string(ShapeEssay), string(ShapeDigest)}},
		},
		"required": []string{"shape"},
	}
}

// buildShapePrompt renders the layout-classification probe for one newsletter.
// It is deliberately tiny: a one-word judgment both scoring models make
// reliably even where full segmentation degrades, bounded to a handful of
// output tokens by the caller.
func buildShapePrompt(in PromptInput) string {
	var b strings.Builder
	b.WriteString(`You are a newsletter layout classifier. You are given the text of ONE email
newsletter. Decide its shape:
- "essay": one continuous piece developing a single narrative or argument (a
  letter, column, or article), even when it ends with footnotes or links.
- "digest": a bundle of multiple distinct items — separate stories, briefs,
  sections, sponsor blocks, or link roundups.

Return ONLY a JSON object: {"shape": "essay"} or {"shape": "digest"}.

`)
	fmt.Fprintf(&b, "SOURCE: %s\n", strings.TrimSpace(in.SourceName))
	fmt.Fprintf(&b, "SUBJECT: %s\n\n", strings.TrimSpace(in.Subject))
	b.WriteString("BODY:\n")
	b.WriteString(strings.TrimSpace(in.Body))
	b.WriteString("\n")
	return b.String()
}

// parseShape extracts the probed shape from the model's response. Anything that
// doesn't decode to one of the two known values is ShapeUnknown — the caller
// falls back to scoring whole rather than risking a chunked essay.
func parseShape(raw string) Shape {
	var v struct {
		Shape string `json:"shape"`
	}
	if err := decodeFirst(stripFence(strings.TrimSpace(raw)), &v); err != nil {
		return ShapeUnknown
	}
	switch Shape(strings.ToLower(strings.TrimSpace(v.Shape))) {
	case ShapeEssay:
		return ShapeEssay
	case ShapeDigest:
		return ShapeDigest
	}
	return ShapeUnknown
}
