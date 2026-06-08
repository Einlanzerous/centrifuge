package httpapi

import (
	"encoding/xml"
	"net/http"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/db"
)

// feedLimit caps how many recent curated stories the RSS feed carries.
const feedLimit = 50

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link,omitempty"`
	Description string  `xml:"description"`
	Category    string  `xml:"category,omitempty"`
	PubDate     string  `xml:"pubDate"`
	GUID        rssGUID `xml:"guid"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

// handleFeed serves an RSS 2.0 reflection of the most recent curated stories —
// a feed reader's view of the same items the dashboard surfaces.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	items, err := db.NewStoryRepo(s.pool).Archive(r.Context(), db.ArchiveFilter{Limit: feedLimit})
	if err != nil {
		s.logger.Error("feed: query", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to build feed")
		return
	}

	base := s.baseURL(r)
	channel := rssChannel{
		Title:       "Centrifuge",
		Link:        base,
		Description: "Curated newsletter stories, ranked by relevance.",
	}
	if len(items) > 0 {
		channel.LastBuildDate = items[0].ReceivedAt.UTC().Format(time.RFC1123Z)
	}
	for _, v := range items {
		link := deref(v.URL)
		if link == "" {
			link = base + "/#/items/" + v.ID
		}
		desc := deref(v.Summary)
		if desc == "" {
			desc = deref(v.Snippet)
		}
		channel.Items = append(channel.Items, rssItem{
			Title:       deref(v.Title),
			Link:        link,
			Description: desc,
			Category:    deref(v.PrimaryTopic),
			PubDate:     v.ReceivedAt.UTC().Format(time.RFC1123Z),
			GUID:        rssGUID{IsPermaLink: false, Value: v.ID},
		})
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(rss{Version: "2.0", Channel: channel}); err != nil {
		s.logger.Error("feed: encode", "error", err)
	}
}

// baseURL is the externally reachable origin used for absolute feed links. It
// prefers the configured PUBLIC_BASE_URL and otherwise derives scheme+host from
// the request (honoring an X-Forwarded-Proto from a TLS-terminating proxy).
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.PublicBaseURL != "" {
		return s.cfg.PublicBaseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
