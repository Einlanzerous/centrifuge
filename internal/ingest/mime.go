package ingest

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
)

// ParseRFC822 turns a raw RFC822 message into a normalized InboundMessage. It
// prefers the text/html body, falls back to text/plain, records attachment
// metadata (not blobs), and extracts outbound links so a sparse body still
// gives the scorer something to work with.
//
// It is deliberately forgiving: a single malformed part is skipped rather than
// failing the whole message, and an unparseable top-level Content-Type degrades
// to text/plain. Only a fundamentally unreadable message (bad headers) errors,
// so the caller can flag-and-skip it.
//
// Charset conversion is not performed — newsletter bodies are overwhelmingly
// UTF-8, and the sanitizer (CTFG-19) operates on the bytes as received.
func ParseRFC822(raw []byte) (*InboundMessage, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse rfc822: %w", err)
	}
	hdr := msg.Header

	out := &InboundMessage{
		Subject:   decodeHeader(hdr.Get("Subject")),
		MessageID: stripAngles(hdr.Get("Message-ID")),
	}
	if addr, err := mail.ParseAddress(hdr.Get("From")); err == nil {
		out.FromAddr = addr.Address
		out.FromName = addr.Name
	} else {
		out.FromAddr = strings.TrimSpace(hdr.Get("From"))
	}
	if t, err := hdr.Date(); err == nil {
		out.ReceivedAt = t.UTC()
	}

	ct := hdr.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		// Unparseable top-level Content-Type: treat the body as plain text
		// rather than discarding the whole message.
		mediaType, params = "text/plain", nil
	}

	var body collectedBody
	if strings.HasPrefix(mediaType, "multipart/") {
		walkMultipart(msg.Body, params["boundary"], &body)
	} else {
		if data, derr := decodeBody(msg.Body, hdr.Get("Content-Transfer-Encoding")); derr == nil {
			body.add(mediaType, string(data))
		}
	}

	out.RawHTML = body.html
	out.BodyText = body.text
	out.Attachments = body.attachments
	out.Links = extractLinks(body.preferred())
	return out, nil
}

// collectedBody accumulates the first html and plaintext bodies plus attachment
// metadata while walking (possibly nested) MIME parts.
type collectedBody struct {
	html        string
	text        string
	attachments []Attachment
}

func (c *collectedBody) add(mediaType, body string) {
	switch mediaType {
	case "text/html":
		if c.html == "" {
			c.html = body
		}
	case "text/plain":
		if c.text == "" {
			c.text = body
		}
	}
}

// preferred returns the body the scorer should see: HTML when present, else the
// plaintext part.
func (c *collectedBody) preferred() string {
	if c.html != "" {
		return c.html
	}
	return c.text
}

// walkMultipart recursively descends a multipart tree, collecting body parts and
// attachment metadata. A malformed part ends the walk for that level but keeps
// whatever was gathered — robustness over completeness.
func walkMultipart(r io.Reader, boundary string, out *collectedBody) {
	if boundary == "" {
		return
	}
	mr := multipart.NewReader(r, boundary)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) || err != nil {
			return
		}

		partType, partParams, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		disposition, _, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		filename := part.FileName()

		switch {
		case disposition == "attachment" || filename != "":
			data, _ := decodeBody(part, part.Header.Get("Content-Transfer-Encoding"))
			out.attachments = append(out.attachments, Attachment{
				Filename:    filename,
				ContentType: partType,
				Size:        len(data),
			})
		case strings.HasPrefix(partType, "multipart/"):
			walkMultipart(part, partParams["boundary"], out)
		default:
			if data, derr := decodeBody(part, part.Header.Get("Content-Transfer-Encoding")); derr == nil {
				out.add(partType, string(data))
			}
		}
	}
}

// decodeBody reads a part body, applying the Content-Transfer-Encoding. Unknown
// or absent encodings are read verbatim (7bit/8bit/binary).
func decodeBody(r io.Reader, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(r))
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, r))
	default:
		return io.ReadAll(r)
	}
}

var headerDecoder = new(mime.WordDecoder)

// decodeHeader expands RFC2047 encoded-words (e.g. a UTF-8 subject), falling
// back to the raw value when it isn't encoded.
func decodeHeader(s string) string {
	dec, err := headerDecoder.DecodeHeader(s)
	if err != nil {
		return s
	}
	return dec
}

// stripAngles removes the surrounding <...> from a Message-ID.
func stripAngles(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}

var urlRe = regexp.MustCompile(`https?://[^\s"'<>)]+`)

// maxLinks caps how many outbound links we record per message.
const maxLinks = 50

// extractLinks pulls deduplicated http(s) URLs out of a body. It is intentionally
// simple — it scans both HTML href values and bare URLs in plaintext — and is
// most useful when the body is mostly a single link.
func extractLinks(body string) []string {
	if body == "" {
		return nil
	}
	matches := urlRe.FindAllString(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimRight(m, ".,);")
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
		if len(out) >= maxLinks {
			break
		}
	}
	return out
}
