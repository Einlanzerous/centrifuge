package ai

import (
	"context"
	"errors"
	"fmt"
)

// ScoreInput is the per-newsletter material the scorer turns into a prompt. The
// worker derives Body from the cleaned, truncated newsletter text (Phase 2).
type ScoreInput struct {
	SourceName string
	Subject    string
	Body       string
}

// ScoreResult carries the outcome of scoring one newsletter (CTFG-62). Shape
// and Chunks exist so the worker's escalation gate can tell a legitimate
// essay's single story from a digest silently collapsing to one item.
type ScoreResult struct {
	Items []ScoredItem
	// Shape is the probed layout when the newsletter was large enough for the
	// chunking decision; ShapeUnknown when it was scored whole without probing.
	Shape Shape
	// Chunks is how many scoring calls the body was split across (1 = whole).
	Chunks int
}

// Scorer segments and scores one newsletter end to end: build the prompt, call
// the model, validate the response. Bodies larger than chunkChars are probed
// for shape and, when digest-shaped, split into ~chunkChars pieces scored
// separately and merged (CTFG-62) — both local models segment reliably at ~5k
// chars and degrade beyond it. It is the seam the worker depends on, so the
// worker can be tested with a stub instead of a live model.
type Scorer struct {
	client     *Client
	topics     []string
	options    map[string]any
	chunkChars int
}

// ScorerOption configures a Scorer.
type ScorerOption func(*Scorer)

// WithGenerateOptions sets Ollama runtime options (e.g. temperature) sent on
// every generate call.
func WithGenerateOptions(opts map[string]any) ScorerOption {
	return func(s *Scorer) { s.options = opts }
}

// WithChunkChars sets the digest chunking target in prepped-body runes; bodies
// at or under it are always scored whole. n <= 0 disables chunking entirely.
func WithChunkChars(n int) ScorerOption {
	return func(s *Scorer) { s.chunkChars = n }
}

// NewScorer builds a Scorer over client, biased toward the given focus topics
// (the current, engagement-weighted set — see CTFG-28).
func NewScorer(client *Client, topics []string, opts ...ScorerOption) *Scorer {
	s := &Scorer{client: client, topics: topics, chunkChars: DefaultChunkChars}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Score segments and scores in, chunking digest-shaped bodies that exceed the
// chunk target. Errors propagate the client's typed transport/decode errors
// (so the worker can branch requeue-vs-skip) or a validation error from
// ParseItems; a truncation anywhere (cut-off array, done:false envelope, or a
// truncated chunk) surfaces as *TruncatedError alongside everything salvaged.
func (s *Scorer) Score(ctx context.Context, in ScoreInput) (ScoreResult, error) {
	if s.chunkChars <= 0 || len([]rune(in.Body)) <= s.chunkChars {
		items, err := s.scoreOnce(ctx, in)
		return ScoreResult{Items: items, Chunks: 1}, err
	}

	shape, err := s.classifyShape(ctx, in)
	if err != nil {
		return ScoreResult{}, err
	}
	if shape != ShapeDigest {
		// An essay (or an unreadable probe, conservatively treated like one) is
		// never split: chunking a continuous piece yields fragment stories where
		// exactly one is correct.
		items, err := s.scoreOnce(ctx, in)
		return ScoreResult{Items: items, Shape: shape, Chunks: 1}, err
	}

	chunks := splitChunks(in.Body, s.chunkChars)
	perChunk := make([][]ScoredItem, 0, len(chunks))
	var truncatedChunks, emptyChunks int
	var firstReason string
	for _, c := range chunks {
		items, err := s.scoreOnce(ctx, ScoreInput{SourceName: in.SourceName, Subject: in.Subject, Body: c})
		if err != nil {
			var tr *TruncatedError
			var ee *EmptyError
			switch {
			case errors.As(err, &tr):
				// Keep what the chunk salvaged and keep going — the other chunks
				// are independent generations.
				truncatedChunks++
				if firstReason == "" {
					firstReason = tr.Error()
				}
				perChunk = append(perChunk, items)
			case errors.As(err, &ee):
				// A chunk of pure boilerplate (footer, referral block) can be
				// legitimately empty; only all-empty is a newsletter-level empty.
				emptyChunks++
			default:
				return ScoreResult{Shape: shape, Chunks: len(chunks)}, err
			}
			continue
		}
		perChunk = append(perChunk, items)
	}

	res := ScoreResult{Items: mergeChunkItems(perChunk), Shape: shape, Chunks: len(chunks)}
	if truncatedChunks > 0 {
		return res, &TruncatedError{
			Recovered: len(res.Items),
			Reason:    fmt.Sprintf("%d of %d chunk(s) truncated; first: %s", truncatedChunks, len(chunks), firstReason),
		}
	}
	if emptyChunks == len(chunks) {
		return res, &EmptyError{}
	}
	return res, nil
}

// scoreOnce runs one prompt-build → generate → validate pass over a single
// body (a whole newsletter or one chunk).
func (s *Scorer) scoreOnce(ctx context.Context, in ScoreInput) ([]ScoredItem, error) {
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

// classifyShape probes the newsletter's layout (essay vs digest) with a
// schema-constrained one-field generate call, bounded to a few output tokens.
// Transport failures propagate (the worker requeues); any unusable answer is
// ShapeUnknown, which the caller treats conservatively (score whole).
func (s *Scorer) classifyShape(ctx context.Context, in ScoreInput) (Shape, error) {
	prompt := buildShapePrompt(PromptInput{
		SourceName: in.SourceName,
		Subject:    in.Subject,
		Body:       in.Body,
		Topics:     s.topics,
	})
	opts := make(map[string]any, len(s.options)+1)
	for k, v := range s.options {
		opts[k] = v
	}
	opts["num_predict"] = 32 // the probe's whole answer is a handful of tokens

	raw, err := s.client.GenerateFormat(ctx, prompt, shapeSchema(), opts)
	if err != nil {
		var pe *PartialError
		if errors.As(err, &pe) {
			return parseShape(pe.Response), nil
		}
		var de *DecodeError
		if errors.As(err, &de) {
			return ShapeUnknown, nil
		}
		return ShapeUnknown, err
	}
	return parseShape(raw), nil
}

// Model returns the model tag the scorer's client uses, for provenance.
func (s *Scorer) Model() string { return s.client.Model() }

// Raw returns the model's unparsed response for in — the prompt is built the
// same way as Score's whole-body pass (no chunking), but no validation is
// applied. It exists for the eval harness to inspect what the model actually
// emits. A done:false partial envelope (CTFG-63) returns the partial text
// alongside the *PartialError so the harness can still show what arrived.
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
