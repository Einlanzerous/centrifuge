package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Defaults for the Ollama client. The model is heavy (gemma4:31b is ~20 GB Q4)
// and runs on the LAN host, so a single generate call routinely takes seconds —
// the timeout is generous and retries are few but deliberate.
const (
	DefaultTimeout    = 120 * time.Second
	DefaultMaxRetries = 2
	defaultRetryWait  = 2 * time.Second
)

// TransportError is returned when the request never produced a usable HTTP
// response — a dial/timeout failure or a 5xx from Ollama. These are transient:
// the worker may retry the whole newsletter later.
type TransportError struct {
	// StatusCode is the HTTP status when the failure was a bad response, or 0
	// when the request never completed (dial error, timeout, context cancel).
	StatusCode int
	Err        error
}

func (e *TransportError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("ollama transport: status %d: %v", e.StatusCode, e.Err)
	}
	return fmt.Sprintf("ollama transport: %v", e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// DecodeError is returned when Ollama answered with a 2xx but the envelope
// could not be decoded. This is not transient — retrying the same input yields
// the same garbage — so the worker should skip (mark failed), not requeue.
type DecodeError struct{ Err error }

func (e *DecodeError) Error() string { return fmt.Sprintf("ollama decode: %v", e.Err) }
func (e *DecodeError) Unwrap() error { return e.Err }

// Client is a typed client for a single Ollama server's /api/generate endpoint.
// It is safe for concurrent use.
type Client struct {
	httpClient *http.Client
	baseURL    string
	model      string
	maxRetries int
	retryWait  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithModel sets the model tag sent on every request.
func WithModel(model string) Option {
	return func(c *Client) {
		if model != "" {
			c.model = model
		}
	}
}

// WithTimeout sets the per-request HTTP timeout (the ceiling for one generate
// call, retries excluded).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.httpClient.Timeout = d
		}
	}
}

// WithMaxRetries sets how many times a transient failure is retried (0 = a
// single attempt, no retries).
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// WithRetryWait sets the base backoff between retries (it scales linearly with
// the attempt number).
func WithRetryWait(d time.Duration) Option {
	return func(c *Client) {
		if d >= 0 {
			c.retryWait = d
		}
	}
}

// WithHTTPClient swaps the underlying *http.Client (mainly for tests). A nil
// client is ignored. Any timeout already configured on the supplied client is
// honored unless WithTimeout overrides it.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// NewClient builds a Client for the Ollama server at baseURL. The model
// defaults to gemma4:31b and can be overridden with WithModel.
func NewClient(baseURL, model string, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: DefaultTimeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		maxRetries: DefaultMaxRetries,
		retryWait:  defaultRetryWait,
	}
	if c.model == "" {
		c.model = DefaultModelFallback
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// DefaultModelFallback is used only when NewClient is given an empty model and
// no WithModel option. Real callers pass config.OllamaModel.
const DefaultModelFallback = "gemma4:31b"

// Model returns the model tag this client sends, for provenance stamping.
func (c *Client) Model() string { return c.model }

// generateRequest is the /api/generate payload. format:"json" instructs Ollama
// to constrain output to valid JSON; stream:false collapses the reply into one
// response object instead of a token stream.
type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Format  string         `json:"format"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

// generateResponse is the subset of Ollama's reply we consume. Response holds
// the model's text — JSON, given format:"json".
type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends prompt to the model and returns the raw response text, which
// is expected to be JSON (the endpoint runs with format:"json"). Options are
// optional Ollama runtime options (temperature, num_ctx, ...).
//
// Transient failures (dial errors, timeouts, 5xx) are retried up to maxRetries
// with linear backoff and surface as *TransportError; a 2xx body that won't
// decode surfaces as *DecodeError and is not retried. The caller can branch on
// the error type to decide requeue-vs-skip.
func (c *Client) Generate(ctx context.Context, prompt string, options map[string]any) (string, error) {
	body, err := json.Marshal(generateRequest{
		Model:   c.model,
		Prompt:  prompt,
		Format:  "json",
		Stream:  false,
		Options: options,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, c.retryWait*time.Duration(attempt)); err != nil {
				return "", &TransportError{Err: err}
			}
		}

		resp, err := c.doOnce(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// Retry only transient failures. A decode error is deterministic, and a
		// 4xx means we sent a bad request — neither improves on retry.
		if !retryable(err) {
			return "", err
		}
	}
	return "", lastErr
}

// retryable reports whether err is worth another attempt: network-level
// transport failures (no status) and 5xx responses are transient; decode
// errors and 4xx responses are terminal.
func retryable(err error) bool {
	var te *TransportError
	if !errors.As(err, &te) {
		return false
	}
	return te.StatusCode == 0 || te.StatusCode >= 500
}

// doOnce performs a single generate request. It distinguishes transport
// failures (retryable) from decode failures (terminal) via the error type.
func (c *Client) doOnce(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", &TransportError{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &TransportError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Carry the status so the retry loop can class it: 5xx (server busy /
		// model loading) is transient, 4xx is a bad request and terminal.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", &TransportError{
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("unexpected status: %s", strings.TrimSpace(string(snippet))),
		}
	}

	var gr generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return "", &DecodeError{Err: fmt.Errorf("decode envelope: %w", err)}
	}
	if strings.TrimSpace(gr.Response) == "" {
		return "", &DecodeError{Err: errors.New("empty response field")}
	}
	return gr.Response, nil
}

// sleep waits for d or until ctx is cancelled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
