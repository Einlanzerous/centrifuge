package httpapi

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/db"
)

// todayLimit caps the Today feed; the design shows a masonry of a handful to a
// couple dozen cards, so a generous ceiling is plenty.
const todayLimit = 200

// briefLimit caps the "spin today's brief" empty-state assembly.
const briefLimit = 15

type todayResponse struct {
	Since      *time.Time `json:"since"`
	SinceHuman string     `json:"since_human"`
	IsEmpty    bool       `json:"is_empty"` // nothing new since the last visit
	Brief      bool       `json:"brief"`    // items are the older "brief" assembly
	Topics     []topicDTO `json:"topics"`
	Items      []itemDTO  `json:"items"`
}

// handleToday serves the "Since You Last Looked" feed: scored stories that
// arrived after the session's last_viewed_at, with a topic-count summary for
// the hero chips. When nothing is new it reports is_empty; a ?brief=1 request
// then assembles older unsurfaced stories ("spin today's brief").
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	ctx := r.Context()

	sess, err := db.NewSessionRepo(s.pool).GetOrCreateDefault(ctx)
	if err != nil {
		s.logger.Error("today: load session", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load session")
		return
	}

	repo := db.NewStoryRepo(s.pool)
	items, err := repo.ListScoredSince(ctx, sess.LastViewedAt, todayLimit)
	if err != nil {
		s.logger.Error("today: list scored", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load feed")
		return
	}

	resp := todayResponse{
		Since:      sess.LastViewedAt,
		SinceHuman: humanSince(sess.LastViewedAt),
	}

	if len(items) == 0 {
		resp.IsEmpty = true
		// Only fill the brief on request, so the UI can show its empty-state
		// prompt first and assemble older stories when the user asks.
		if r.URL.Query().Get("brief") != "" {
			brief, err := repo.ListBrief(ctx, briefLimit)
			if err != nil {
				s.logger.Error("today: list brief", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to assemble brief")
				return
			}
			items = brief
			resp.Brief = true
		}
	}

	resp.Items = toItems(items)
	resp.Topics = topicChips(items)
	writeJSON(w, http.StatusOK, resp)
}

// handleTodaySeen marks the Today feed as seen, advancing last_viewed_at to now
// so the next visit's "since" delta and new-item set are measured from here.
func (s *Server) handleTodaySeen(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	ctx := r.Context()
	repo := db.NewSessionRepo(s.pool)
	if _, err := repo.GetOrCreateDefault(ctx); err != nil {
		s.logger.Error("seen: ensure session", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	sess, err := repo.TouchLastViewed(ctx, db.DefaultSessionLabel, time.Now().UTC())
	if err != nil {
		s.logger.Error("seen: touch", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"last_viewed_at": sess.LastViewedAt})
}

// topicChips counts the topics present in a set of items, most frequent first —
// the hero chips. Colors come from the same stable palette as the registry.
func topicChips(items []db.StoryView) []topicDTO {
	counts := map[string]int{}
	order := []string{}
	for _, v := range items {
		topic := deref(v.PrimaryTopic)
		if topic == "" {
			continue
		}
		if _, seen := counts[topic]; !seen {
			order = append(order, topic)
		}
		counts[topic]++
	}
	chips := make([]topicDTO, 0, len(order))
	for _, topic := range order {
		chips = append(chips, topicDTO{Topic: topic, Color: topicColor(topic), Count: counts[topic]})
	}
	sort.SliceStable(chips, func(i, j int) bool { return chips[i].Count > chips[j].Count })
	return chips
}

// humanSince renders the gap since t as the hero's "X ago" phrase. A nil t (the
// feed has never been marked seen) reads as "the beginning".
func humanSince(t *time.Time) string {
	if t == nil {
		return "the beginning"
	}
	d := time.Since(*t)
	switch {
	case d < time.Minute:
		return "moments ago"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	default:
		return plural(int(d.Hours()/24), "day")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return strconv.Itoa(n) + " " + unit + "s ago"
}
