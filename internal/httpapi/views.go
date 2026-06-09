package httpapi

import (
	"time"

	"github.com/Einlanzerous/centrifuge/internal/db"
)

// itemDTO is the JSON shape of a story as the UI consumes it (see
// design/DESIGN.md "Data model the UI assumes"). It flattens the enriched
// StoryView: source name inline, rating as a string token, opened as read, and
// a palette color for the topic. Body is populated only by the detail endpoint.
type itemDTO struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	Snippet        string    `json:"snippet,omitempty"`
	URL            string    `json:"url,omitempty"`
	PrimaryTopic   string    `json:"primary_topic"`
	TopicColor     string    `json:"topic_color"`
	ImageURL       string    `json:"image_url,omitempty"`
	Labels         []string  `json:"labels"`
	Section        string    `json:"section,omitempty"`
	Kind           string    `json:"kind"`
	RelevanceScore *int      `json:"relevance_score,omitempty"`
	SourceID       string    `json:"source_id"`
	SourceName     string    `json:"source_name"`
	Received       time.Time `json:"received"`
	Bookmarked     bool      `json:"bookmarked"`
	Rating         string    `json:"rating"` // "up" | "down" | "none"
	Read           bool      `json:"read"`
	Body           string    `json:"body,omitempty"`
	Segmented      bool      `json:"segmented"`         // body is a multi-story digest, not this story
	Content        string    `json:"content,omitempty"` // this story's verbatim text (digest items)
}

// toItem maps an enriched StoryView to its JSON DTO. withBody includes the raw
// HTML body (the Reader modal); list views pass false.
func toItem(v db.StoryView, withBody bool) itemDTO {
	it := itemDTO{
		ID:             v.ID,
		Title:          deref(v.Title),
		Summary:        deref(v.Summary),
		Snippet:        deref(v.Snippet),
		URL:            deref(v.URL),
		PrimaryTopic:   deref(v.PrimaryTopic),
		TopicColor:     topicColor(deref(v.PrimaryTopic)),
		ImageURL:       deref(v.ImageURL),
		Labels:         v.Labels,
		Section:        deref(v.Section),
		Kind:           v.Kind,
		RelevanceScore: v.RelevanceScore,
		SourceID:       v.SourceID,
		SourceName:     v.SourceName,
		Received:       v.ReceivedAt,
		Bookmarked:     v.Bookmarked,
		Rating:         ratingToken(v.UserRating),
		Read:           v.OpenedAt != nil,
		Segmented:      v.Segmented,
	}
	if it.Labels == nil {
		it.Labels = []string{}
	}
	if withBody {
		it.Body = deref(v.Body)
		it.Content = deref(v.SegmentText)
	}
	return it
}

func toItems(vs []db.StoryView) []itemDTO {
	out := make([]itemDTO, len(vs))
	for i := range vs {
		out[i] = toItem(vs[i], false)
	}
	return out
}

// topicDTO is one entry of the topic registry the sidebar/chips render.
type topicDTO struct {
	Topic string `json:"topic"`
	Color string `json:"color"`
	Count int    `json:"count"`
}

// sourceDTO is one entry of the source registry ("best sources" + sidebar).
type sourceDTO struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	StoryCount      int      `json:"story_count"`
	ScoredCount     int      `json:"scored_count"`
	AvgRelevance    *float64 `json:"avg_relevance"`
	BookmarkCount   int      `json:"bookmark_count"`
	PositiveRatings int      `json:"positive_ratings"`
	NegativeRatings int      `json:"negative_ratings"`
}

func toTopics(tcs []db.TopicCount) []topicDTO {
	out := make([]topicDTO, len(tcs))
	for i, tc := range tcs {
		out[i] = topicDTO{Topic: tc.Topic, Color: topicColor(tc.Topic), Count: tc.Count}
	}
	return out
}

func toSources(stats []db.SourceStats) []sourceDTO {
	out := make([]sourceDTO, len(stats))
	for i, s := range stats {
		out[i] = sourceDTO{
			ID: s.SourceID, Name: s.SourceName, StoryCount: s.StoryCount,
			ScoredCount: s.ScoredCount, AvgRelevance: s.AvgRelevance,
			BookmarkCount: s.BookmarkCount, PositiveRatings: s.PositiveRatings,
			NegativeRatings: s.NegativeRatings,
		}
	}
	return out
}

// ratingToken renders a stored thumbs value as the UI's token.
func ratingToken(r *int) string {
	switch {
	case r == nil:
		return "none"
	case *r > 0:
		return "up"
	default:
		return "down"
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
