package ingest

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseMultipartAlternative(t *testing.T) {
	raw := "From: Example News <news@example.com>\r\n" +
		"Subject: =?utf-8?q?Hello_=E2=9C=93?=\r\n" +
		"Message-ID: <abc123@example.com>\r\n" +
		"Date: Mon, 02 Jun 2026 10:00:00 +0000\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=\"BOUND\"\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Plain body here\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"<p>Hello =E2=9C=93 https://example.com/story</p>\r\n" +
		"--BOUND--\r\n"

	msg, err := ParseRFC822([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.FromAddr != "news@example.com" {
		t.Errorf("FromAddr = %q", msg.FromAddr)
	}
	if msg.FromName != "Example News" {
		t.Errorf("FromName = %q", msg.FromName)
	}
	if msg.Subject != "Hello ✓" {
		t.Errorf("Subject = %q, want decoded encoded-word", msg.Subject)
	}
	if msg.MessageID != "abc123@example.com" {
		t.Errorf("MessageID = %q, want angle brackets stripped", msg.MessageID)
	}
	if !strings.Contains(msg.RawHTML, "<p>Hello ✓") {
		t.Errorf("RawHTML = %q, want quoted-printable decoded", msg.RawHTML)
	}
	if !strings.Contains(msg.BodyText, "Plain body here") {
		t.Errorf("BodyText = %q", msg.BodyText)
	}
	if got, want := msg.ReceivedAt.Year(), 2026; got != want {
		t.Errorf("ReceivedAt year = %d, want %d", got, want)
	}
	if len(msg.Links) != 1 || msg.Links[0] != "https://example.com/story" {
		t.Errorf("Links = %v, want one story link", msg.Links)
	}
}

func TestParseSinglepartHTML(t *testing.T) {
	raw := "From: a@b.com\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<h1>Hi</h1>\r\n"

	msg, err := ParseRFC822([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(msg.RawHTML, "<h1>Hi</h1>") {
		t.Errorf("RawHTML = %q", msg.RawHTML)
	}
	if msg.FromAddr != "a@b.com" {
		t.Errorf("FromAddr = %q", msg.FromAddr)
	}
}

func TestParseMixedWithAttachment(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("PDFDATA"))
	raw := "From: a@b.com\r\n" +
		"Content-Type: multipart/mixed; boundary=MIX\r\n" +
		"\r\n" +
		"--MIX\r\n" +
		"Content-Type: text/html\r\n" +
		"\r\n" +
		"<p>see attached</p>\r\n" +
		"--MIX\r\n" +
		"Content-Type: application/pdf; name=\"report.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"report.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		payload + "\r\n" +
		"--MIX--\r\n"

	msg, err := ParseRFC822([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(msg.RawHTML, "see attached") {
		t.Errorf("RawHTML = %q", msg.RawHTML)
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(msg.Attachments))
	}
	att := msg.Attachments[0]
	if att.Filename != "report.pdf" {
		t.Errorf("filename = %q", att.Filename)
	}
	if att.ContentType != "application/pdf" {
		t.Errorf("content-type = %q", att.ContentType)
	}
	if att.Size != len("PDFDATA") {
		t.Errorf("size = %d, want %d (decoded length)", att.Size, len("PDFDATA"))
	}
}

func TestParsePlaintextFallbackAndLinks(t *testing.T) {
	// Sparse body that is mostly a link — the scorer needs the URL recorded.
	raw := "From: digest@news.example\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Read more: https://news.example/a, and https://news.example/b.\r\n"

	msg, err := ParseRFC822([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.RawHTML != "" {
		t.Errorf("RawHTML = %q, want empty (plaintext only)", msg.RawHTML)
	}
	if !strings.Contains(msg.BodyText, "Read more") {
		t.Errorf("BodyText = %q", msg.BodyText)
	}
	want := []string{"https://news.example/a", "https://news.example/b"}
	if len(msg.Links) != 2 || msg.Links[0] != want[0] || msg.Links[1] != want[1] {
		t.Errorf("Links = %v, want %v (trailing punctuation trimmed)", msg.Links, want)
	}
}

func TestParseMalformedErrors(t *testing.T) {
	if _, err := ParseRFC822([]byte("this is not a valid email")); err == nil {
		t.Fatal("expected error for unparseable message")
	}
}
