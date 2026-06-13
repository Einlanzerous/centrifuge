package db

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"
)

// StoryView is a story enriched with the read-side fields the API serves: its
// source's display name and the timeline timestamp (the parent newsletter's
// received_at, falling back to ingested_at). Body holds the raw HTML and is
// populated only by GetEnriched (the Reader modal); list queries leave it nil.
type StoryView struct {
	Story
	SourceName string
	ReceivedAt time.Time
	// Body is the parent newsletter's raw HTML (detail endpoint only).
	Body *string
	// Segmented is true when the parent newsletter produced more than one story
	// (a digest). The UI renders an essay's body inline but offers a digest's
	// full newsletter behind a "view full" toggle.
	Segmented bool
	// SegmentText is this story's verbatim article text, sliced out of the parent
	// newsletter's cleaned text (digest items only; nil when extraction misses).
	SegmentText *string
}

// storyViewCols selects every story column (aliased st) plus the joined source
// name and coalesced received timestamp. The trailing comma-separated extras
// are scanned by scanStoryView in order.
const storyViewCols = `st.id, st.newsletter_id, st.source_id, st.position, st.kind, st.section, ` +
	`st.title, st.url, st.snippet, st.summary, st.relevance_score, st.primary_topic, st.labels, ` +
	`st.model, st.prompt_version, st.scored_at, st.bookmarked, st.user_rating, st.opened_at, ` +
	`st.image_url, s.name, COALESCE(n.received_at, n.ingested_at)`

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
		&v.ImageURL, &v.SourceName, &v.ReceivedAt)
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
	// Return the parent newsletter's raw HTML plus a "segmented" flag (more than
	// one story => a digest). The UI renders an essay's body inline, but for a
	// digest the same raw_html is the whole email, so it is shown only behind a
	// "view full newsletter" toggle. Proper per-segment HTML is a follow-up.
	const q = `SELECT ` + storyViewCols + `, n.raw_html,
  (SELECT count(*) FROM stories sc WHERE sc.newsletter_id = n.id AND sc.kind = 'story') > 1
` + storyViewFrom + `
WHERE st.id = $1`
	var v StoryView
	var labels []byte
	err := r.db.QueryRow(ctx, q, storyID).Scan(&v.ID, &v.NewsletterID, &v.SourceID, &v.Position, &v.Kind,
		&v.Section, &v.Title, &v.URL, &v.Snippet,
		&v.Summary, &v.RelevanceScore, &v.PrimaryTopic, &labels, &v.Model, &v.PromptVersion, &v.ScoredAt,
		&v.Bookmarked, &v.UserRating, &v.OpenedAt,
		&v.ImageURL, &v.SourceName, &v.ReceivedAt, &v.Body, &v.Segmented)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		if err := json.Unmarshal(labels, &v.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
	}

	// For a digest item, slice this story's verbatim text (with paragraph breaks)
	// out of the parent newsletter's HTML, bounded by the surrounding stories.
	// Best-effort: on a miss the UI falls back to the summary.
	if v.Segmented && v.Body != nil {
		sibs, err := r.siblingSnippets(ctx, v.NewsletterID)
		if err != nil {
			return nil, fmt.Errorf("sibling snippets: %w", err)
		}
		if seg := extractSegmentText(htmlToText(*v.Body), sibs, v.Position); seg != "" {
			v.SegmentText = &seg
		}
	}
	return &v, nil
}

// siblingSnippet pairs a story's position with its verbatim opening snippet and
// title, used as boundary anchors when slicing one segment out of the
// newsletter. The snippet marks where an item's body begins; the title marks
// where the item visually begins (it physically precedes the snippet), so it
// bounds the *previous* item's text more tightly.
type siblingSnippet struct {
	position int
	snippet  string
	title    string
}

// siblingSnippets returns every story in a newsletter (any kind) ordered by
// position, so adjacent stories bound each other's text span.
func (r *StoryRepo) siblingSnippets(ctx context.Context, newsletterID string) ([]siblingSnippet, error) {
	rows, err := r.db.Query(ctx,
		`SELECT position, COALESCE(snippet, ''), COALESCE(title, '') FROM stories WHERE newsletter_id = $1 ORDER BY position`,
		newsletterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []siblingSnippet
	for rows.Next() {
		var s siblingSnippet
		if err := rows.Scan(&s.position, &s.snippet, &s.title); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// readerSkippableTags are subtrees whose text is noise in the Reader.
var readerSkippableTags = map[string]bool{
	"script": true, "style": true, "head": true, "title": true, "noscript": true,
}

// blockTags introduce a line break in extracted text so paragraph and list
// structure survives. Everything else is inline and its text is concatenated
// verbatim — so a styled first letter ("T" + "reated") or a parenthesized link
// ("(" + link + ")") does not gain stray spaces.
var blockTags = map[string]bool{
	"p": true, "br": true, "div": true, "li": true, "tr": true, "table": true,
	"blockquote": true, "ul": true, "ol": true, "hr": true, "section": true,
	"article": true, "header": true, "footer": true, "figure": true,
	"figcaption": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true,
}

// htmlToText renders newsletter HTML to plain text that keeps paragraph breaks
// (block elements become newlines) while concatenating inline text verbatim.
// ingest.CleanText fully flattens whitespace for the scorer; the Reader instead
// needs the structure preserved, so this is a separate, structure-aware pass.
func htmlToText(rawHTML string) string {
	z := html.NewTokenizer(strings.NewReader(rawHTML))
	var b strings.Builder
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return normalizeBlockText(b.String())
		case html.StartTagToken:
			name, _ := z.TagName()
			n := string(name)
			if readerSkippableTags[n] {
				skip++
				continue
			}
			if skip == 0 && blockTags[n] {
				b.WriteByte('\n')
			}
		case html.SelfClosingTagToken:
			if name, _ := z.TagName(); skip == 0 && blockTags[string(name)] {
				b.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			n := string(name)
			if readerSkippableTags[n] {
				if skip > 0 {
					skip--
				}
				continue
			}
			if skip == 0 && blockTags[n] {
				b.WriteByte('\n')
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		}
	}
}

// normalizeBlockText collapses intra-line whitespace and limits blank-line runs
// so the kept paragraph structure reads cleanly.
func normalizeBlockText(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.Join(strings.Fields(ln), " ")
	}
	out := strings.Join(lines, "\n")
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}

// anchorRegexp builds a whitespace-tolerant matcher for a snippet's opening
// words, used to locate that story's start inside the structured newsletter
// text. Returns nil for snippets too short to anchor reliably.
func anchorRegexp(snippet string) *regexp.Regexp {
	words := strings.Fields(strings.TrimRight(snippet, ".… "))
	if len(words) < 3 {
		return nil
	}
	if len(words) > 10 {
		words = words[:10]
	}
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	re, err := regexp.Compile(strings.Join(words, `\s+`))
	if err != nil {
		return nil
	}
	return re
}

// extractSegmentText slices the verbatim text (paragraph breaks intact) of the
// story at position out of the structured newsletter text. Each story's snippet
// is the literal start of its section, so the span runs from this story's anchor
// to whichever other story's anchor comes next *physically* — sponsors/promos
// are interleaved between stories and numbered out of physical order, so the
// boundary can't rely on scorer position. Returns "" if the start can't be found.
func extractSegmentText(text string, sibs []siblingSnippet, position int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	var cur *regexp.Regexp
	for _, s := range sibs {
		if s.position == position {
			cur = anchorRegexp(s.snippet)
			break
		}
	}
	if cur == nil {
		return ""
	}
	loc := cur.FindStringIndex(text)
	if loc == nil {
		return ""
	}
	start := loc[0]

	end := len(text)
	var nextTitle string
	rest := text[start+1:]
	for _, s := range sibs {
		if s.position == position {
			continue
		}
		re := anchorRegexp(s.snippet)
		if re == nil {
			continue
		}
		if l := re.FindStringIndex(rest); l != nil {
			if e := start + 1 + l[0]; e < end {
				end = e
				nextTitle = s.title
			}
		}
	}

	// The boundary above is the *next* item's body (its snippet), but a digest
	// renders each item as [section label][title][byline][snippet], so that
	// item's label/title/byline physically precede its snippet and would bleed
	// into this story. Pull the end back to the next item's title and drop any
	// preceding section label (CTFG-44).
	end = trimNextLeadIn(text, start, end, nextTitle)

	return strings.TrimSpace(text[start:end])
}

// trimNextLeadIn pulls end back so the next item's lead-in does not bleed into
// this story. It looks for nextTitle within a bounded window just before end
// (so a title-like phrase deep in this body cannot truncate it), cuts there,
// then also drops an immediately-preceding ALL-CAPS section label. Returns end
// unchanged when the title is too short to match safely or is not found.
func trimNextLeadIn(text string, start, end int, nextTitle string) int {
	title := strings.Join(strings.Fields(nextTitle), " ")
	if len([]rune(title)) < 4 {
		return end
	}

	const window = 400
	from := end - window
	if from < start {
		from = start
	}
	region := text[from:end]
	idx := strings.LastIndex(strings.ToLower(region), strings.ToLower(title))
	if idx < 0 {
		return end
	}
	cut := from + idx

	return dropPrecedingLabel(text, start, cut)
}

// dropPrecedingLabel moves cut back to swallow a lone ALL-CAPS section label
// (e.g. "CYBERSECURITY") sitting on the line immediately before cut. It leaves
// the current story's own em-dash byline ("—BH") alone.
func dropPrecedingLabel(text string, start, cut int) int {
	seg := strings.TrimRight(text[start:cut], " \t\n")
	nl := strings.LastIndex(seg, "\n")
	lineStart := start
	if nl >= 0 {
		lineStart = start + nl + 1
	}
	line := strings.TrimSpace(text[lineStart : start+len(seg)])
	if isSectionLabel(line) {
		return lineStart
	}
	return cut
}

// isSectionLabel reports whether s is a short, all-uppercase category header
// (digits, spaces, and punctuation allowed). Lines beginning with a dash are
// bylines ("—BH"), not labels, and are excluded.
func isSectionLabel(s string) bool {
	if s == "" || len([]rune(s)) > 32 {
		return false
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "—") {
		return false
	}
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
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
