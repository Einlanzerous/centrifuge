package ai

// Prompt eval harness (CTFG-36 / CTFG-23). This is NOT a unit test: it runs the
// real prompt through a live Ollama model against real newsletters pulled from a
// live database, and reports the per-item kind classification so prompt changes
// can be judged against ground truth. It self-skips unless EVAL=1 so it never
// runs in CI.
//
// Usage:
//
//	EVAL=1 \
//	OLLAMA_URL=http://localhost:11434 OLLAMA_MODEL=gemma4:31b \
//	DATABASE_URL=postgres://.../centrifuge \
//	go test ./internal/ai/ -run TestPromptEval -v -timeout 20m
//
// It only issues read-only SELECTs against the database.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// evalCase is one newsletter to score plus the ground-truth expectation. wantMin
// /wantMax bound how many kind=="story" items a correct segmentation yields;
// note is the human rationale.
type evalCase struct {
	id      string
	wantMin int
	wantMax int
	note    string
}

// evalCases mixes the under-segmented digests (defect A — too few stories) with
// regression guards (a genuine single essay, a well-segmented digest) so a
// prompt change that recovers demoted stories without ballooning blurb→story is
// visible in one run.
var evalCases = []evalCase{
	// Under-segmented: 1440 Fish Espionage. Lead sea-creatures story + a collagen
	// ad + three genuine multi-sentence news briefs (World Cup, Men in Blazers
	// partnership=promo, Park Police 86 47). Expect the two real briefs recovered.
	{"a8de9e67-8512-4e23-b001-de1fc4640e9a", 3, 4, "1440 Fish Espionage: World Cup + Park Police briefs are real stories"},
	// Murky guard: High Speed Rail Alliance (transit is a focus topic). Mostly
	// events/sponsors with 1 clear story + a borderline interview writeup; the
	// point is that it must not balloon, not that stories must be recovered.
	{"74b9291e-dd27-4849-b8ed-884633a91fa5", 1, 3, "HSR Alliance: 1 clear story + events/sponsors, keep stable"},
	// Guard — same publication, already good: 1440 Sagrada. Must not balloon.
	{"df717960-0d7d-4c52-84b9-4047a7a63d92", 3, 5, "1440 Sagrada: already ~4 stories, keep stable"},
	// Guard — genuine single essay. Must stay exactly one story, never over-split.
	{"523b043b-fb10-440f-8f00-26a70bb66901", 1, 1, "bigtechnology SpaceX: single essay"},
	// Guard — single essay + subscribe promo (Heather Cox Richardson).
	{"cb2efe0e-6063-48e9-b5e2-9e0b8faaaaa3", 1, 1, "HCR: one long essay, promo aside"},
	// Guard — well-segmented digest with many ads/blurbs (Morning Brew). Watch
	// for blurb→story over-promotion inflating the story count.
	{"ad42e80d-5288-451e-b741-f4bee6f02d8a", 3, 6, "Morning Brew: keep blurbs as blurbs"},
}

func TestPromptEval(t *testing.T) {
	if os.Getenv("EVAL") == "" {
		t.Skip("set EVAL=1 to run the prompt eval harness")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL required for the eval harness")
	}
	ollamaURL := envOr("OLLAMA_URL", "http://localhost:11434")
	model := envOr("OLLAMA_MODEL", "gemma4:31b")
	topics := splitTopics(envOr("RELEVANCE_TOPICS", "AI engineering,urbanism,transit/trains,nuclear,tech,video games"))

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	// Match production's generate options exactly so the eval predicts what a
	// re-score would actually produce: greedy decoding (CTFG-43) and the same
	// num_predict token cap (CTFG-42, default 6144). EVAL_NUM_PREDICT overrides
	// the cap (0 = uncapped) to measure how much the cap alone truncates a digest.
	numPredict := 6144
	if v := os.Getenv("EVAL_NUM_PREDICT"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil {
			numPredict = n
		}
	}
	genOpts := map[string]any{"temperature": 0}
	if numPredict > 0 {
		genOpts["num_predict"] = numPredict
	}
	scorer := NewScorer(
		NewClient(ollamaURL, model, WithTimeout(8*time.Minute), WithMaxRetries(0)),
		topics,
		WithGenerateOptions(genOpts),
	)
	fmt.Printf("# eval: model=%s num_predict=%d source=%v\n", model, numPredict, os.Getenv("EVAL_NO_SOURCE") == "")

	// Optional subset: EVAL_CASES=<comma-separated 8-char id prefixes> runs only
	// those, so a slow model can be exercised a couple of cases at a time.
	only := map[string]bool{}
	for _, p := range splitTopics(os.Getenv("EVAL_CASES")) {
		only[p] = true
	}

	// Stream to stdout (go test -v) so partial results survive a timeout kill —
	// t.Logf buffers until the test returns, which a 25m timeout never reaches.
	var fails int
	for _, c := range evalCases {
		if len(only) > 0 && !only[short(c.id)] {
			continue
		}
		var source, subject, body string
		err := pool.QueryRow(context.Background(),
			`SELECT s.name, n.subject, COALESCE(n.body_text,'')
			   FROM newsletters n JOIN sources s ON s.id=n.source_id WHERE n.id=$1`,
			c.id).Scan(&source, &subject, &body)
		if err != nil {
			fmt.Printf("[ERR ] %s load: %v\n", short(c.id), err)
			fails++
			continue
		}

		// Production's worker does NOT pass SourceName to the scorer (it scores
		// from subject+body only). EVAL_NO_SOURCE=1 reproduces that exactly, so the
		// eval can measure whether wiring the publication name in changes
		// segmentation. Default (unset) passes it, matching BuildPrompt's intent.
		in := ScoreInput{SourceName: source, Subject: subject, Body: body}
		if os.Getenv("EVAL_NO_SOURCE") != "" {
			in.SourceName = ""
		}

		// Per-case deadline so one stuck generation cannot consume the whole run.
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		start := time.Now()
		items, err := scorer.Score(ctx, in)
		cancel()
		dur := time.Since(start).Round(time.Second)
		// A TruncatedError still returns the salvaged leading items — and at
		// temperature 0 production persists exactly those (CTFG-45), so they ARE
		// the re-score outcome. Report them with a TRUNC marker rather than
		// discarding. An EmptyError ("[]", CTFG-59) is the model segmenting nothing
		// — report it with an EMPTY marker (it falls through to 0 items/0 stories,
		// which fails any non-zero want range). Any other error is a real failure.
		truncated, empty := false, false
		if err != nil {
			var tr *TruncatedError
			var ee *EmptyError
			switch {
			case errors.As(err, &tr):
				truncated = true
			case errors.As(err, &ee):
				empty = true
			default:
				fmt.Printf("[ERR ] %-22s %s score (%s): %v\n", source, short(c.id), dur, err)
				fails++
				continue
			}
		}

		stories := 0
		var b strings.Builder
		for i, it := range items {
			if it.Kind == KindStory {
				stories++
			}
			fmt.Fprintf(&b, "\n      %s%s", padKind(it.Kind), title(it, i))
		}
		ok := stories >= c.wantMin && stories <= c.wantMax
		status := "PASS"
		if !ok {
			status = "FAIL"
			fails++
		}
		marker := ""
		if truncated {
			marker = " [TRUNC]"
		}
		if empty {
			marker = " [EMPTY]"
		}
		fmt.Printf("[%s] %-22s %s  %d items, %d stories (want %d-%d) in %s%s — %s%s\n",
			status, source, short(c.id), len(items), stories, c.wantMin, c.wantMax, dur, marker, c.note, b.String())
	}
	if fails > 0 {
		t.Errorf("%d eval case(s) failed or outside expected story range", fails)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitTopics(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func short(id string) string { return id[:8] }

func padKind(k string) string {
	for len(k) < 6 {
		k += " "
	}
	return k + "  "
}

func title(it ScoredItem, i int) string {
	t := it.Title
	if t == "" {
		t = it.Snippet
	}
	if len(t) > 48 {
		t = t[:48]
	}
	return t
}
