package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// StoryView is a story enriched with the read-side fields the API serves: its
// source's display name and the timeline timestamp (the parent newsletter's
// received_at, falling back to ingested_at). Body holds the raw HTML and is
// populated only by GetEnriched (the Reader modal); list queries leave it nil.
type StoryView struct {
	Story
	SourceName string
	ReceivedAt time.Time
	Body       *string
}

// storyViewCols selects every story column (aliased st) plus the joined source
// name and coalesced received timestamp. The trailing comma-separated extras
// are scanned by scanStoryView in order.
const storyViewCols = `st.id, st.newsletter_id, st.source_id, st.position, st.kind, st.section, ` +
	`st.title, st.url, st.snippet, st.summary, st.relevance_score, st.primary_topic, st.labels, ` +
	`st.model, st.prompt_version, st.scored_at, st.bookmarked, st.user_rating, st.opened_at, ` +
	`s.name, COALESCE(n.received_at, n.ingested_at)`

const storyViewFrom = `FROM stories st
JOIN sources s ON s.id = st.source_id
JOIN newsletters n ON n.id = st.newsletter_id`

func scanStoryView(row pgx.Row) (*StoryView, error) {
	var v StoryView
	var labels []byte
	err := row.Scan(&v.ID, &v.NewsletterID, &v.SourceID, &v.Position, &v.Kind,
		&v.Section, &v.Title, &v.URL, &v.Snippet,
		&v.Summary, &v.RelevanceScore, &v.PrimaryTopic, &labels, &v.Model, &v.PromptVersion, &v.ScoredAt,
		&v.Bookmarked, &v.UserRating, &v.OpenedAt,
		&v.SourceName, &v.ReceivedAt)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		if err := json.Unmarshal(labels, &v.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
	}
	return &v, nil
}

func collectStoryViews(rows pgx.Rows) ([]StoryView, error) {
	defer rows.Close()
	var out []StoryView
	for rows.Next() {
		v, err := scanStoryView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// ListScoredSince returns scored stories that became available after since,
// best first — the Today feed. A nil since (the session has never been marked
// seen) returns the most recent scored stories. Only kind='story' items with a
// relevance score are surfaced; ads/blurbs/promos never reach the feed.
func (r *StoryRepo) ListScoredSince(ctx context.Context, since *time.Time, limit int) ([]StoryView, error) {
	const q = `SELECT ` + storyViewCols + `
` + storyViewFrom + `
WHERE st.kind = 'story' AND st.relevance_score IS NOT NULL
  AND ($1::timestamptz IS NULL OR st.scored_at > $1)
ORDER BY st.relevance_score DESC, st.scored_at DESC
LIMIT $2`
	rows, err := r.db.Query(ctx, q, since, limit)
	if err != nil {
		return nil, err
	}
	return collectStoryViews(rows)
}

// ListBrief returns the top unopened scored stories regardless of age — the
// "spin today's brief" empty-state path, which assembles older unsurfaced
// stories when nothing is new since the last visit.
func (r *StoryRepo) ListBrief(ctx context.Context, limit int) ([]StoryView, error) {
	const q = `SELECT ` + storyViewCols + `
` + storyViewFrom + `
WHERE st.kind = 'story' AND st.relevance_score IS NOT NULL AND st.opened_at IS NULL
ORDER BY st.relevance_score DESC, st.scored_at DESC
LIMIT $1`
	rows, err := r.db.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	return collectStoryViews(rows)
}

// ArchiveFilter narrows an archive query. Zero-valued fields are ignored, so an
// empty filter returns all curated stories. Limit defaults are applied by the
// caller.
type ArchiveFilter struct {
	Topic    string     // primary_topic exact match
	SourceID string     // source_id exact match
	From     *time.Time // received_at >= From
	To       *time.Time // received_at < To
	Query    string     // case-insensitive substring over title/summary/snippet
	Limit    int
	Offset   int
}

// Archive returns curated stories matching filter, newest first. Day-grouping
// for the UI is done by the caller from ReceivedAt. Only kind='story' items are
// returned — a "mark as ad" correction removes a story from this feed.
func (r *StoryRepo) Archive(ctx context.Context, f ArchiveFilter) ([]StoryView, error) {
	var where strings.Builder
	where.WriteString(`WHERE st.kind = 'story'`)
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		fmt.Fprintf(&where, " AND %s$%d", cond, len(args))
	}
	if f.Topic != "" {
		add("st.primary_topic = ", f.Topic)
	}
	if f.SourceID != "" {
		add("st.source_id = ", f.SourceID)
	}
	if f.From != nil {
		add("COALESCE(n.received_at, n.ingested_at) >= ", *f.From)
	}
	if f.To != nil {
		add("COALESCE(n.received_at, n.ingested_at) < ", *f.To)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+q+"%")
		fmt.Fprintf(&where, " AND (st.title ILIKE $%d OR st.summary ILIKE $%d OR st.snippet ILIKE $%d)",
			len(args), len(args), len(args))
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	args = append(args, limit)
	limitIdx := len(args)
	args = append(args, f.Offset)
	offsetIdx := len(args)

	q := fmt.Sprintf(`SELECT %s
%s
%s
ORDER BY COALESCE(n.received_at, n.ingested_at) DESC, st.position
LIMIT $%d OFFSET $%d`, storyViewCols, storyViewFrom, where.String(), limitIdx, offsetIdx)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return collectStoryViews(rows)
}

// GetEnriched returns one story with its source name, received timestamp, and
// the raw HTML body for the Reader modal. Returns pgx.ErrNoRows if unknown.
func (r *StoryRepo) GetEnriched(ctx context.Context, storyID string) (*StoryView, error) {
	const q = `SELECT ` + storyViewCols + `, n.raw_html
` + storyViewFrom + `
WHERE st.id = $1`
	var v StoryView
	var labels []byte
	err := r.db.QueryRow(ctx, q, storyID).Scan(&v.ID, &v.NewsletterID, &v.SourceID, &v.Position, &v.Kind,
		&v.Section, &v.Title, &v.URL, &v.Snippet,
		&v.Summary, &v.RelevanceScore, &v.PrimaryTopic, &labels, &v.Model, &v.PromptVersion, &v.ScoredAt,
		&v.Bookmarked, &v.UserRating, &v.OpenedAt,
		&v.SourceName, &v.ReceivedAt, &v.Body)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		if err := json.Unmarshal(labels, &v.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
	}
	return &v, nil
}

// TopicCount is one row of the topic registry: a dynamic primary_topic and how
// many curated stories carry it. The API assigns a stable palette color per
// topic name on top of this.
type TopicCount struct {
	Topic string
	Count int
}

// TopicRegistry returns every primary_topic present on curated stories with its
// story count, most common first. The taxonomy is dynamic (CTFG-28), so this is
// derived from the data rather than a fixed list.
func (r *StoryRepo) TopicRegistry(ctx context.Context) ([]TopicCount, error) {
	const q = `
SELECT primary_topic, COUNT(*)
FROM stories
WHERE kind = 'story' AND primary_topic IS NOT NULL AND primary_topic <> ''
GROUP BY primary_topic
ORDER BY COUNT(*) DESC, primary_topic`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopicCount
	for rows.Next() {
		var tc TopicCount
		if err := rows.Scan(&tc.Topic, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}
