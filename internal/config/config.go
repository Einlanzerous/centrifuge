// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Defaults applied when the corresponding environment variable is unset.
const (
	DefaultOllamaURL = "http://ollama:11434"
	DefaultModel     = "gemma4:31b"
	DefaultPort      = 8080
	DefaultLogLevel  = "info"
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

	// IngestToken authenticates inbound ingestion requests.
	IngestToken string

	// Port is the TCP port the HTTP server listens on.
	Port int

	// LogLevel is the slog level name (debug|info|warn|error).
	LogLevel string

	// RelevanceTopics biases the scoring worker toward topics of interest.
	RelevanceTopics []string
}

// Load reads configuration from the environment, applies defaults, and returns
// a populated Config. It returns a descriptive error (never panics) when a
// required variable is missing or a value cannot be parsed.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		OllamaURL:       getEnvDefault("OLLAMA_URL", DefaultOllamaURL),
		OllamaModel:     getEnvDefault("OLLAMA_MODEL", DefaultModel),
		IngestToken:     os.Getenv("INGEST_TOKEN"),
		Port:            DefaultPort,
		LogLevel:        getEnvDefault("LOG_LEVEL", DefaultLogLevel),
		RelevanceTopics: parseTopics(os.Getenv("RELEVANCE_TOPICS")),
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
