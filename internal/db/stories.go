package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// StoryRepo persists stories and the scoring/engagement signals on them.
type StoryRepo struct{ db DBTX }

// NewStoryRepo returns a StoryRepo over db (a pool or a transaction).
func NewStoryRepo(db DBTX) *StoryRepo { return &StoryRepo{db: db} }

const storyCols = `id, newsletter_id, source_id, position, kind, section, title, url, snippet, ` +
	`summary, relevance_score, primary_topic, labels, model, prompt_version, scored_at, ` +
	`bookmarked, user_rating, opened_at`

func scanStory(row pgx.Row) (*Story, error) {
	var s Story
	var labels []byte
	err := row.Scan(&s.ID, &s.NewsletterID, &s.SourceID, &s.Position, &s.Kind,
		&s.Section, &s.Title, &s.URL, &s.Snippet,
		&s.Summary, &s.RelevanceScore, &s.PrimaryTopic, &labels, &s.Model, &s.PromptVersion, &s.ScoredAt,
		&s.Bookmarked, &s.UserRating, &s.OpenedAt)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		if err := json.Unmarshal(labels, &s.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
	}
	return &s, nil
}

// InsertMany writes all of a newsletter's segmented stories in one statement —
// the worker emits 1..N items per newsletter in a single pass. Only the
// segmentation fields are written; scoring fields stay null until ScoreUpdate.
// sourceID is denormalized from the parent newsletter. The returned stories
// carry their generated ids, in input order. An empty input is a no-op.
func (r *StoryRepo) InsertMany(ctx context.Context, newsletterID, sourceID string, stories []Story) ([]Story, error) {
	if len(stories) == 0 {
		return nil, nil
	}

	var b strings.Builder
	b.WriteString(`INSERT INTO stories (newsletter_id, source_id, position, kind, section, title, url, snippet) VALUES `)
	args := make([]any, 0, len(stories)*8)
	for i, s := range stories {
		if i > 0 {
			b.WriteString(", ")
		}
		n := i * 8
		fmt.Fprintf(&b, "($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8)
		kind := s.Kind
		if kind == "" {
			kind = KindStory
		}
		args = append(args, newsletterID, sourceID, s.Position, kind, s.Section, s.Title, s.URL, s.Snippet)
	}
	b.WriteString(` RETURNING ` + storyCols)

	rows, err := r.db.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Story, 0, len(stories))
	for rows.Next() {
		s, err := scanStory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Score holds the worker's scoring output for a single story.
type Score struct {
	Summary        string
	RelevanceScore int
	PrimaryTopic   string
	Labels         []string
	Model          string
	PromptVersion  string
}

// ScoreUpdate writes a story's scoring fields in place and stamps scored_at.
// Re-scoring overwrites; model+prompt_version record provenance (scoring
// history is deferred). Returns pgx.ErrNoRows if the story id is unknown.
func (r *StoryRepo) ScoreUpdate(ctx context.Context, storyID string, sc Score) error {
	labels, err := json.Marshal(sc.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	const q = `
UPDATE stories
SET summary = $2, relevance_score = $3, primary_topic = $4, labels = $5,
    model = $6, prompt_version = $7, scored_at = now()
WHERE id = $1`
	return r.execOne(ctx, q, storyID, sc.Summary, sc.RelevanceScore, sc.PrimaryTopic, labels, sc.Model, sc.PromptVersion)
}

// SetBookmark toggles a story's bookmark flag.
func (r *StoryRepo) SetBookmark(ctx context.Context, storyID string, bookmarked bool) error {
	return r.execOne(ctx, `UPDATE stories SET bookmarked = $2 WHERE id = $1`, storyID, bookmarked)
}

// ToggleBookmark flips a story's bookmark flag atomically and returns the new
// value. Returns pgx.ErrNoRows if the story id is unknown.
func (r *StoryRepo) ToggleBookmark(ctx context.Context, storyID string) (bool, error) {
	var bookmarked bool
	err := r.db.QueryRow(ctx,
		`UPDATE stories SET bookmarked = NOT bookmarked WHERE id = $1 RETURNING bookmarked`,
		storyID).Scan(&bookmarked)
	return bookmarked, err
}

// SetRating records a thumbs rating (-1 or +1) on a story.
func (r *StoryRepo) SetRating(ctx context.Context, storyID string, rating int) error {
	return r.execOne(ctx, `UPDATE stories SET user_rating = $2 WHERE id = $1`, storyID, rating)
}

// ClearRating removes a story's thumbs rating (the "none" choice).
func (r *StoryRepo) ClearRating(ctx context.Context, storyID string) error {
	return r.execOne(ctx, `UPDATE stories SET user_rating = NULL WHERE id = $1`, storyID)
}

// MarkOpened stamps when a story was opened.
func (r *StoryRepo) MarkOpened(ctx context.Context, storyID string, at time.Time) error {
	return r.execOne(ctx, `UPDATE stories SET opened_at = $2 WHERE id = $1`, storyID, at)
}

// SetKind applies a reader's kind override (e.g. "mark as ad"). Flipping to
// KindAd is also a strong negative engagement signal for CTFG-29.
func (r *StoryRepo) SetKind(ctx context.Context, storyID, kind string) error {
	return r.execOne(ctx, `UPDATE stories SET kind = $2 WHERE id = $1`, storyID, kind)
}

// ListByNewsletter returns a newsletter's stories in position order.
func (r *StoryRepo) ListByNewsletter(ctx context.Context, newsletterID string) ([]Story, error) {
	const q = `SELECT ` + storyCols + ` FROM stories WHERE newsletter_id = $1 ORDER BY position`
	rows, err := r.db.Query(ctx, q, newsletterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Story
	for rows.Next() {
		s, err := scanStory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// SourceStats is the per-source rollup behind "which sources have the best
// stories" (CTFG-5/CTFG-29). AvgRelevance is nil when a source has no scored
// stories.
type SourceStats struct {
	SourceID        string
	SourceName      string
	StoryCount      int
	ScoredCount     int
	AvgRelevance    *float64
	BookmarkCount   int
	PositiveRatings int
	NegativeRatings int
}

// SourceAggregate rolls up story relevance and engagement by source, best
// average first. It comes straight off the denormalized source_id FK — no
// extra schema. Sources with no stories are included (zeroed).
func (r *StoryRepo) SourceAggregate(ctx context.Context) ([]SourceStats, error) {
	const q = `
SELECT s.id, s.name,
    COUNT(st.id) FILTER (WHERE st.kind = 'story')              AS story_count,
    COUNT(st.id) FILTER (WHERE st.relevance_score IS NOT NULL) AS scored_count,
    AVG(st.relevance_score)                                    AS avg_relevance,
    COUNT(st.id) FILTER (WHERE st.bookmarked)                  AS bookmark_count,
    COUNT(st.id) FILTER (WHERE st.user_rating = 1)             AS positive_ratings,
    COUNT(st.id) FILTER (WHERE st.user_rating = -1)            AS negative_ratings
FROM sources s
LEFT JOIN stories st ON st.source_id = s.id
GROUP BY s.id, s.name
ORDER BY avg_relevance DESC NULLS LAST, s.name`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SourceStats
	for rows.Next() {
		var ss SourceStats
		if err := rows.Scan(&ss.SourceID, &ss.SourceName, &ss.StoryCount, &ss.ScoredCount,
			&ss.AvgRelevance, &ss.BookmarkCount, &ss.PositiveRatings, &ss.NegativeRatings); err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// execOne runs a single-row-affecting statement, returning pgx.ErrNoRows when
// nothing matched.
func (r *StoryRepo) execOne(ctx context.Context, sql string, args ...any) error {
	ct, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
