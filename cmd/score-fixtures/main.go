// Command score-fixtures runs the full prep -> model -> validate scoring path
// over a directory of real-newsletter fixtures and prints the segmented,
// scored items per fixture. It is the eval gate for scoring quality (CTFG-23):
// tweak the prompt or RELEVANCE_TOPICS, re-run, and eyeball the deltas.
//
// Unlike the test suite it talks to a live Ollama, so it is a manual tool
// (`make score-fixtures`), never part of `go test`. It needs no database.
//
// Fixtures are sourced from files:
//   - *.html / *.htm  -> run through ingest.CleanText (the real email prep path)
//   - *.txt / *.md    -> treated as already-clean body text
//
// Usage:
//
//	go run ./cmd/score-fixtures [-dir DIR] [-url URL] [-model M] [-topics "a,b"]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/config"
	"github.com/Einlanzerous/centrifuge/internal/ingest"
)

func main() {
	dir := flag.String("dir", "internal/ai/testdata/fixtures", "directory of fixture files")
	url := flag.String("url", getenv("OLLAMA_URL", config.DefaultOllamaURL), "Ollama base URL")
	model := flag.String("model", getenv("OLLAMA_MODEL", config.DefaultModel), "Ollama model tag")
	topicsFlag := flag.String("topics", getenv("RELEVANCE_TOPICS", ""), "comma-separated focus topics (default: built-in list)")
	maxChars := flag.Int("max-chars", config.DefaultIngestMaxChars, "cap on prepped body chars fed to the model")
	timeout := flag.Duration("timeout", config.DefaultOllamaTimeout, "per-request Ollama timeout")
	raw := flag.Bool("raw", false, "print the model's unparsed JSON response per fixture (debug)")
	flag.Parse()

	topics := config.DefaultRelevanceTopics
	if t := parseCSV(*topicsFlag); len(t) > 0 {
		topics = t
	}

	files, err := fixtureFiles(*dir)
	if err != nil {
		fail(err.Error())
	}
	if len(files) == 0 {
		fail(fmt.Sprintf("no .html/.txt/.md fixtures found in %s", *dir))
	}

	scorer := ai.NewScorer(
		ai.NewClient(*url, *model, ai.WithTimeout(*timeout)),
		topics,
		// temperature 0 keeps re-runs comparable for eyeballing deltas.
		ai.WithGenerateOptions(map[string]any{"temperature": 0}),
	)

	fmt.Printf("model=%s  url=%s  prompt=%s\n", *model, *url, ai.PromptVersion)
	fmt.Printf("topics=[%s]\n\n", strings.Join(topics, ", "))

	ctx := context.Background()
	var failures int
	for _, f := range files {
		if err := scoreFixture(ctx, scorer, f, *maxChars, *raw); err != nil {
			failures++
			fmt.Printf("fixture: %s\n  ERROR: %v\n\n", filepath.Base(f), err)
		}
	}

	fmt.Printf("scored %d fixture(s), %d error(s)\n", len(files), failures)
	if failures > 0 {
		os.Exit(1)
	}
}

// scoreFixture preps one fixture file, scores it, and prints the result.
func scoreFixture(ctx context.Context, scorer *ai.Scorer, path string, maxChars int, rawMode bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	name := filepath.Base(path)
	body, kind := prep(path, string(content), maxChars)
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("prepped body is empty")
	}

	in := ai.ScoreInput{SourceName: sourceName(name), Body: body}

	if rawMode {
		resp, err := scorer.Raw(ctx, in)
		if err != nil {
			return err
		}
		fmt.Printf("fixture: %s  (%s, %d prepped chars)\n--- raw model response ---\n%s\n---\n\n", name, kind, len([]rune(body)), resp)
		return nil
	}

	start := time.Now()
	items, err := scorer.Score(ctx, in)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		return err
	}

	fmt.Printf("fixture: %s  (%s, %d prepped chars, %s)\n", name, kind, len([]rune(body)), elapsed)
	fmt.Printf("  items: %d  (%s)\n", len(items), kindBreakdown(items))
	for i, it := range items {
		fmt.Printf("  [%d] %-6s score=%-3d topic=%q labels=%v\n", i, it.Kind, it.RelevanceScore, it.PrimaryTopic, it.Labels)
		if it.Title != "" {
			fmt.Printf("      title:   %s\n", oneline(it.Title))
		}
		if it.URL != "" {
			fmt.Printf("      url:     %s\n", it.URL)
		}
		if it.Summary != "" {
			fmt.Printf("      summary: %s\n", oneline(it.Summary))
		}
	}
	fmt.Println()
	return nil
}

// prep turns raw fixture bytes into model-ready body text, mirroring the
// production path: HTML through the sanitizer, plain text collapsed/truncated.
func prep(path, raw string, maxChars int) (body, kind string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return ingest.CleanText(raw, maxChars), "html"
	default:
		return truncateRunes(strings.Join(strings.Fields(raw), " "), maxChars), "text"
	}
}

func fixtureFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".html", ".htm", ".txt", ".md", ".markdown", ".text":
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func kindBreakdown(items []ai.ScoredItem) string {
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Kind]++
	}
	var parts []string
	for _, k := range []string{ai.KindStory, ai.KindBlurb, ai.KindAd, ai.KindPromo} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
		}
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, ", ")
}

// sourceName derives a readable publication name from a fixture filename, used
// only as prompt context.
func sourceName(file string) string {
	stem := strings.TrimSuffix(file, filepath.Ext(file))
	stem = strings.NewReplacer("_", " ", "-", " ").Replace(stem)
	return strings.TrimSpace(stem)
}

func oneline(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func parseCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "score-fixtures: "+msg)
	os.Exit(1)
}
