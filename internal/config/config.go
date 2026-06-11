// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults applied when the corresponding environment variable is unset.
const (
	DefaultOllamaURL          = "http://ollama:11434"
	DefaultModel              = "gemma4:31b"
	DefaultPort               = 8080
	DefaultLogLevel           = "info"
	DefaultIngestMaxChars     = 24000
	DefaultOllamaTimeout      = 900 * time.Second
	DefaultOllamaMaxRetries   = 2
	DefaultScoringInterval    = 30 * time.Second
	DefaultScoringBatch       = 5
	DefaultScoringMaxAttempts = 3
	// DefaultOllamaNumPredict caps how many tokens the model may generate per
	// scoring call. Bounded output makes a model that fails to close its JSON
	// truncate fast (→ salvage, CTFG-33) instead of running to the context limit
	// or the client timeout and looping (CTFG-42). ~6k covers a large digest
	// (~30 items) while keeping the worst case (~280s at 22 tok/s) well under
	// OLLAMA_TIMEOUT_SECONDS — keep that ordering when tuning either value.
	DefaultOllamaNumPredict = 6144
	// DefaultOllamaTemperature pins sampling to greedy decoding. At the model's
	// own default (~0.8) gemma4:31b occasionally enters a repetition spiral
	// inside a long string value and never closes the JSON element, so the whole
	// digest truncates with nothing salvageable (CTFG-43). 0 also matches the
	// score-fixtures eval gate, so prod output is reproducible there.
	DefaultOllamaTemperature = 0.0
)

// DefaultRelevanceTopics is the fallback topic list used to bias scoring when
// RELEVANCE_TOPICS is not provided.
var DefaultRelevanceTopics = []string{
	"AI engineering",
	"urbanism",
	"transit/trains",
	"nuclear",
	"tech",
	"video games",
}

// Config holds all runtime settings for the centrifuge service. It is populated
// by Load from the process environment.
type Config struct {
	// DatabaseURL is the Postgres connection string (DATABASE_URL). Required.
	DatabaseURL string

	// OllamaURL is the base URL of the Ollama server used by the scoring worker.
	OllamaURL string

	// OllamaModel is the model tag passed to Ollama for relevance scoring.
	OllamaModel string

	// OllamaTimeout is the per-request ceiling for one generate call. The model
	// is heavy, so this is generous.
	OllamaTimeout time.Duration

	// OllamaMaxRetries is how many times a transient Ollama failure is retried.
	OllamaMaxRetries int

	// OllamaNumPredict caps tokens generated per scoring call (Ollama's
	// num_predict). <= 0 leaves it unbounded (Ollama default), but the deployed
	// default is finite to prevent runaway generations (CTFG-42).
	OllamaNumPredict int

	// OllamaTemperature is the sampling temperature passed on every scoring
	// call. Always sent — 0 (the default) means greedy decoding, not "unset"
	// (CTFG-43).
	OllamaTemperature float64

	// IngestToken authenticates inbound ingestion requests.
	IngestToken string

	// IngestMaxChars caps the cleaned body text derived from each newsletter
	// before it reaches the scorer. 0 disables truncation.
	IngestMaxChars int

	// Port is the TCP port the HTTP server listens on.
	Port int

	// LogLevel is the slog level name (debug|info|warn|error).
	LogLevel string

	// RelevanceTopics biases the scoring worker toward topics of interest.
	RelevanceTopics []string

	// ScoringEnabled turns the background scoring worker on or off. Off is
	// useful in local dev with no reachable Ollama.
	ScoringEnabled bool

	// ScoringInterval is how often the worker polls for pending newsletters.
	ScoringInterval time.Duration

	// ScoringBatch is how many newsletters the worker claims per poll.
	ScoringBatch int

	// ScoringMaxAttempts bounds how many times a newsletter whose model output
	// came back truncated is re-scored before the worker keeps what it salvaged
	// or marks it failed (CTFG-33).
	ScoringMaxAttempts int

	// CORSAllowOrigin is the Access-Control-Allow-Origin served by the read API
	// for the browser frontend. Defaults to "*" (the API carries no
	// credentials). Set to a specific origin to lock it down.
	CORSAllowOrigin string

	// PublicBaseURL is the externally reachable base URL of the service, used to
	// build absolute links in the RSS feed. Empty falls back to the request's
	// own scheme+host.
	PublicBaseURL string
}

// Load reads configuration from the environment, applies defaults, and returns
// a populated Config. It returns a descriptive error (never panics) when a
// required variable is missing or a value cannot be parsed.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		OllamaURL:          getEnvDefault("OLLAMA_URL", DefaultOllamaURL),
		OllamaModel:        getEnvDefault("OLLAMA_MODEL", DefaultModel),
		OllamaTimeout:      DefaultOllamaTimeout,
		OllamaMaxRetries:   DefaultOllamaMaxRetries,
		OllamaNumPredict:   DefaultOllamaNumPredict,
		OllamaTemperature:  DefaultOllamaTemperature,
		IngestToken:        os.Getenv("INGEST_TOKEN"),
		IngestMaxChars:     DefaultIngestMaxChars,
		Port:               DefaultPort,
		LogLevel:           getEnvDefault("LOG_LEVEL", DefaultLogLevel),
		RelevanceTopics:    parseTopics(os.Getenv("RELEVANCE_TOPICS")),
		ScoringEnabled:     true,
		ScoringInterval:    DefaultScoringInterval,
		ScoringBatch:       DefaultScoringBatch,
		ScoringMaxAttempts: DefaultScoringMaxAttempts,
		CORSAllowOrigin:    getEnvDefault("CORS_ALLOW_ORIGIN", "*"),
		PublicBaseURL:      strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
	}

	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid PORT %q: %w", v, err)
		}
		if p <= 0 || p > 65535 {
			return nil, fmt.Errorf("config: PORT must be 1-65535, got %d", p)
		}
		cfg.Port = p
	}

	if v := os.Getenv("OLLAMA_TIMEOUT_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid OLLAMA_TIMEOUT_SECONDS %q: %w", v, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("config: OLLAMA_TIMEOUT_SECONDS must be > 0, got %d", n)
		}
		cfg.OllamaTimeout = time.Duration(n) * time.Second
	}

	if v := os.Getenv("OLLAMA_MAX_RETRIES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid OLLAMA_MAX_RETRIES %q: %w", v, err)
		}
		if n < 0 {
			return nil, fmt.Errorf("config: OLLAMA_MAX_RETRIES must be >= 0, got %d", n)
		}
		cfg.OllamaMaxRetries = n
	}

	if v := os.Getenv("OLLAMA_NUM_PREDICT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid OLLAMA_NUM_PREDICT %q: %w", v, err)
		}
		cfg.OllamaNumPredict = n
	}

	if v := os.Getenv("OLLAMA_TEMPERATURE"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("config: invalid OLLAMA_TEMPERATURE %q: %w", v, err)
		}
		if f < 0 {
			return nil, fmt.Errorf("config: OLLAMA_TEMPERATURE must be >= 0, got %v", f)
		}
		cfg.OllamaTemperature = f
	}

	if v := os.Getenv("INGEST_MAX_CHARS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid INGEST_MAX_CHARS %q: %w", v, err)
		}
		if n < 0 {
			return nil, fmt.Errorf("config: INGEST_MAX_CHARS must be >= 0, got %d", n)
		}
		cfg.IngestMaxChars = n
	}

	if v := os.Getenv("SCORING_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid SCORING_ENABLED %q: %w", v, err)
		}
		cfg.ScoringEnabled = b
	}

	if v := os.Getenv("SCORING_INTERVAL_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid SCORING_INTERVAL_SECONDS %q: %w", v, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("config: SCORING_INTERVAL_SECONDS must be > 0, got %d", n)
		}
		cfg.ScoringInterval = time.Duration(n) * time.Second
	}

	if v := os.Getenv("SCORING_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid SCORING_BATCH_SIZE %q: %w", v, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("config: SCORING_BATCH_SIZE must be > 0, got %d", n)
		}
		cfg.ScoringBatch = n
	}

	if v := os.Getenv("SCORING_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid SCORING_MAX_ATTEMPTS %q: %w", v, err)
		}
		if n < 1 {
			return nil, fmt.Errorf("config: SCORING_MAX_ATTEMPTS must be >= 1, got %d", n)
		}
		cfg.ScoringMaxAttempts = n
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("config: DATABASE_URL is required")
	}
	return nil
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseTopics splits a comma-separated topic list, trimming whitespace and
// dropping empty entries. An empty input yields the default topic list.
func parseTopics(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		out := make([]string, len(DefaultRelevanceTopics))
		copy(out, DefaultRelevanceTopics)
		return out
	}

	parts := strings.Split(raw, ",")
	topics := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			topics = append(topics, t)
		}
	}
	if len(topics) == 0 {
		out := make([]string, len(DefaultRelevanceTopics))
		copy(out, DefaultRelevanceTopics)
		return out
	}
	return topics
}
