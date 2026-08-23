package ai

import (
	"context"
	"errors"
)

// ScoreInput is the per-newsletter material the scorer turns into a prompt. The
// worker derives Body from the cleaned, truncated newsletter text (Phase 2).
type ScoreInput struct {
	SourceName string
	Subject    string
	Body       string
}

// Scorer segments and scores one newsletter end to end: build the prompt, call
// the model, validate the response. It is the seam the worker depends on, so
// the worker can be tested with a stub instead of a live model.
type Scorer struct {
	client  *Client
	topics  []string
	options map[string]any
}

// ScorerOption configures a Scorer.
type ScorerOption func(*Scorer)

// WithGenerateOptions sets Ollama runtime options (e.g. temperature) sent on
// every generate call.
func WithGenerateOptions(opts map[string]any) ScorerOption {
	return func(s *Scorer) { s.options = opts }
}

// NewScorer builds a Scorer over client, biased toward the given focus topics
// (the current, engagement-weighted set — see CTFG-28).
func NewScorer(client *Client, topics []string, opts ...ScorerOption) *Scorer {
	s := &Scorer{client: client, topics: topics}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Score builds the prompt for in, runs it through the model, and returns the
// validated items. Errors propagate the client's typed transport/decode errors
// (so the worker can branch requeue-vs-skip) or a validation error from
// ParseItems.
func (s *Scorer) Score(ctx context.Context, in ScoreInput) ([]ScoredItem, error) {
	prompt := BuildPrompt(PromptInput{
		SourceName: in.SourceName,
		Subject:    in.Subject,
		Body:       in.Body,
		Topics:     s.topics,
	})
	raw, err := s.client.GenerateFormat(ctx, prompt, ItemsSchema(), s.options)
	if err != nil {
		var pe *PartialError
		if !errors.As(err, &pe) {
			return nil, err
		}
		// Ollama cut the generation server-side (done:false; CTFG-63). Whatever
		// text it did produce is a truncated response in all but shape, so salvage
		// the complete leading items and surface the cut through the truncation
		// path the worker already handles — attributed via Reason.
		items, perr := ParseItems(pe.Response)
		var tr *TruncatedError
		if errors.As(perr, &tr) {
			tr.Reason = pe.Error()
			return items, tr
		}
		return items, &TruncatedError{Recovered: len(items), Reason: pe.Error()}
	}
	return ParseItems(raw)
}

// Raw returns the model's unparsed response for in — the prompt is built the
// same way as Score, but no validation is applied. It exists for the eval
// harness to inspect what the model actually emits. A done:false partial
// envelope (CTFG-63) returns the partial text alongside the *PartialError so
// the harness can still show what arrived.
func (s *Scorer) Raw(ctx context.Context, in ScoreInput) (string, error) {
	prompt := BuildPrompt(PromptInput{
		SourceName: in.SourceName,
		Subject:    in.Subject,
		Body:       in.Body,
		Topics:     s.topics,
	})
	raw, err := s.client.GenerateFormat(ctx, prompt, ItemsSchema(), s.options)
	if err != nil {
		var pe *PartialError
		if errors.As(err, &pe) {
			return pe.Response, err
		}
	}
	return raw, err
}

// Model returns the model tag the scorer's client uses, for provenance.
func (s *Scorer) Model() string { return s.client.Model() }

// Deterministic reports whether the scorer samples greedily (temperature 0), so
// a re-run of the same prompt reproduces the same output byte-for-byte. The
// worker uses this to skip pointless retries of a truncated response: at temp 0
// the retry yields the identical truncation, so it should salvage immediately
// (CTFG-45). An unset temperature means the Ollama default (stochastic), so this
// returns false and the normal retry budget applies.
func (s *Scorer) Deterministic() bool {
	t, ok := s.options["temperature"]
	if !ok {
		return false
	}
	switch v := t.(type) {
	case float64:
		return v == 0
	case float32:
		return v == 0
	case int:
		return v == 0
	default:
		return false
	}
}
