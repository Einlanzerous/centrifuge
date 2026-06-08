package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/db"
)

// archiveMaxLimit bounds a single archive page.
const archiveMaxLimit = 500

type dayGroup struct {
	Date  string    `json:"date"`  // YYYY-MM-DD (UTC)
	Label string    `json:"label"` // "Today" / "Yesterday" / weekday
	Items []itemDTO `json:"items"`
}

type archiveResponse struct {
	Days    []dayGroup  `json:"days"`
	Total   int         `json:"total"`
	Sources []sourceDTO `json:"sources"`
	Topics  []topicDTO  `json:"topics"`
}

// handleArchive serves the chronological, day-grouped feed of all curated
// stories, filterable by topic, source, date-range, and search. It also returns
// the source and topic registries the filter sidebar renders.
func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()

	filter := db.ArchiveFilter{
		Topic:    q.Get("topic"),
		SourceID: q.Get("source"),
		Query:    q.Get("q"),
	}

	from, err := parseTimeParam(q.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from: want RFC3339 or YYYY-MM-DD")
		return
	}
	to, err := parseTimeParam(q.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to: want RFC3339 or YYYY-MM-DD")
		return
	}
	filter.From, filter.To = from, to

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if n > archiveMaxLimit {
			n = archiveMaxLimit
		}
		filter.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid offset")
			return
		}
		filter.Offset = n
	}

	repo := db.NewStoryRepo(s.pool)
	items, err := repo.Archive(ctx, filter)
	if err != nil {
		s.logger.Error("archive: query", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load archive")
		return
	}

	sources, err := repo.SourceAggregate(ctx)
	if err != nil {
		s.logger.Error("archive: sources", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load sources")
		return
	}
	topics, err := repo.TopicRegistry(ctx)
	if err != nil {
		s.logger.Error("archive: topics", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load topics")
		return
	}

	writeJSON(w, http.StatusOK, archiveResponse{
		Days:    groupByDay(items),
		Total:   len(items),
		Sources: toSources(sources),
		Topics:  toTopics(topics),
	})
}

// groupByDay buckets newest-first items into per-day groups, preserving order.
func groupByDay(items []db.StoryView) []dayGroup {
	var days []dayGroup
	idx := map[string]int{}
	for _, v := range items {
		date := v.ReceivedAt.UTC().Format("2006-01-02")
		i, ok := idx[date]
		if !ok {
			i = len(days)
			idx[date] = i
			days = append(days, dayGroup{Date: date, Label: dayLabel(v.ReceivedAt)})
		}
		days[i].Items = append(days[i].Items, toItem(v, false))
	}
	return days
}

// dayLabel renders a received timestamp's relative day label (UTC).
func dayLabel(t time.Time) string {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	day := t.UTC().Truncate(24 * time.Hour)
	switch {
	case day.Equal(today):
		return "Today"
	case day.Equal(today.Add(-24 * time.Hour)):
		return "Yesterday"
	case day.After(today.Add(-7 * 24 * time.Hour)):
		return t.UTC().Weekday().String()
	default:
		return t.UTC().Format("Jan 2, 2006")
	}
}

// parseTimeParam accepts an RFC3339 timestamp or a bare YYYY-MM-DD date (UTC
// midnight). An empty string yields nil (no bound).
func parseTimeParam(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
