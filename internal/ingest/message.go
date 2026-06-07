package ingest

import "time"

// Attachment is metadata about a MIME attachment — ingestion records what was
// attached (filename, type, size), not necessarily the bytes.
type Attachment struct {
	Filename    string
	ContentType string
	Size        int
}

// InboundMessage is the normalized, source-agnostic representation every
// ingestion entrypoint produces. The RFC822 webhook, the JSON drop, and any
// future live feed all converge on this shape before reaching the ingestion
// core, which is what makes the email source irrelevant.
type InboundMessage struct {
	// FromAddr is the sender's email address; it identifies the source.
	FromAddr string
	// FromName is the sender's display name; it names the source on first sight.
	FromName string
	Subject  string
	// MessageID is the RFC822 Message-ID (angle brackets stripped), or "" when
	// the sender omitted it — common for bulk mail, hence the hash fallback.
	MessageID string
	// RawHTML is the verbatim HTML body, persisted as-is.
	RawHTML string
	// BodyText is the cleaned plaintext rendering used for dedupe and (later)
	// scoring. The sanitizer (CTFG-19) derives it from RawHTML when empty.
	BodyText string
	// ReceivedAt is when the message was received; defaults to ingest time.
	ReceivedAt  time.Time
	Attachments []Attachment
	// Links are outbound URLs pulled from a sparse body so the scorer has
	// something to work with even when the email is mostly a single link.
	Links []string
}
