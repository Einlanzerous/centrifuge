package db

import "time"

// Source kinds.
const (
	SourceKindNewsletter = "newsletter"
	SourceKindRSS        = "rss"
)

// Newsletter processing_status values. The scoring worker drives the
// transitions pending_scoring -> scoring -> scored | failed.
const (
	StatusPending = "pending_scoring"
	StatusScoring = "scoring"
	StatusScored  = "scored"
	StatusFailed  = "failed"
)

// Story kinds. Only KindStory is fully scored; the rest are persisted-but-
// unscored so engagement can still learn from them. kind is user-mutable.
const (
	KindStory = "story"
	KindBlurb = "blurb"
	KindAd    = "ad"
	KindPromo = "promo"
)

// Source is a first-class publication or feed (CTFG-12).
type Source struct {
	ID        string
	Name      string
	Kind      string
	Identity  string
	CreatedAt time.Time
}

// Newsletter is one raw delivery — a container with no score of its own. Its
// stories carry the relevance, summary, and engagement (CTFG-12/13). Nullable
// columns are modeled as pointers.
type Newsletter struct {
	ID               string
	SourceID         string
	MessageID        *string
	Subject          *string
	RawHTML          *string
	BodyText         *string
	ReceivedAt       *time.Time
	DedupeHash       *string
	IngestedAt       time.Time
	ProcessingStatus string
}

// DefaultSessionLabel is the single implicit user's session label. Centrifuge
// is single-user today; the read API always operates on this session.
const DefaultSessionLabel = "default"

// Session tracks "Since you last looked" state for the Today view (CTFG-26).
// LastViewedAt is nil until the feed has been marked seen at least once.
type Session struct {
	ID           string
	Label        string
	LastViewedAt *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Story is the unit of relevance, scoring, and engagement (CTFG-13). The
// scoring worker writes the Scoring fields in place after segmentation; they
// are nil until then.
type Story struct {
	ID           string
	NewsletterID string
	SourceID     string
	Position     int
	Kind         string
	Section      *string
	Title        *string
	URL          *string
	Snippet      *string

	// Scoring (nil until the worker scores the story).
	Summary        *string
	RelevanceScore *int
	PrimaryTopic   *string
	Labels         []string
	Model          *string
	PromptVersion  *string
	ScoredAt       *time.Time

	// Engagement.
	Bookmarked bool
	UserRating *int
	OpenedAt   *time.Time
}
