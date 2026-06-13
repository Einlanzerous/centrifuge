package mailfeed

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/Einlanzerous/centrifuge/internal/ingest"
)

// rawMessage is a minimal but well-formed RFC822 message ParseRFC822 accepts.
func rawMessage(subject string) []byte {
	return []byte("From: News <news@example.com>\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/html\r\n" +
		"\r\n" +
		"<p>hello world</p>\r\n")
}

// stubMailClient is an in-memory MailClient: no network.
type stubMailClient struct {
	mu sync.Mutex

	ids      []string          // returned by ListUnprocessed
	raws     map[string][]byte // id -> raw bytes (missing => GetRaw error)
	lastMax  int               // max arg seen by ListUnprocessed
	labeled  []string          // ids passed to MarkProcessed
	listErr  error
	labelErr error
}

func (s *stubMailClient) ListUnprocessed(_ context.Context, max int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastMax = max
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]string(nil), s.ids...), nil
}

func (s *stubMailClient) GetRaw(_ context.Context, id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.raws[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return raw, nil
}

func (s *stubMailClient) MarkProcessed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.labelErr != nil {
		return s.labelErr
	}
	s.labeled = append(s.labeled, id)
	return nil
}

func (s *stubMailClient) labeledIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.labeled...)
}

// stubIngester records ingested subjects and returns a configurable outcome.
type stubIngester struct {
	created   bool  // Result.Created returned
	err       error // returned on every call
	ingested  []string
	callCount int
}

func (s *stubIngester) Ingest(_ context.Context, msg ingest.InboundMessage) (*ingest.Result, error) {
	s.callCount++
	if s.err != nil {
		return nil, s.err
	}
	s.ingested = append(s.ingested, msg.Subject)
	return &ingest.Result{Newsletter: &db.Newsletter{}, Created: s.created}, nil
}

func quietPoller(ing Ingester, client MailClient, opts ...Option) *Poller {
	opts = append([]Option{WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))}, opts...)
	return New(ing, client, opts...)
}

func TestRunOnceIngestsAndLabels(t *testing.T) {
	client := &stubMailClient{
		ids:  []string{"a", "b"},
		raws: map[string][]byte{"a": rawMessage("A"), "b": rawMessage("B")},
	}
	ing := &stubIngester{created: true}
	p := quietPoller(ing, client)

	p.runOnce(context.Background())

	if ing.callCount != 2 {
		t.Fatalf("expected 2 ingests, got %d", ing.callCount)
	}
	if got := client.labeledIDs(); len(got) != 2 {
		t.Fatalf("expected both messages labeled, got %v", got)
	}
}

func TestRunOnceParseErrorDoesNotBlockRest(t *testing.T) {
	client := &stubMailClient{
		ids: []string{"bad", "good"},
		raws: map[string][]byte{
			"bad":  []byte("this is not a valid email header line\r\n"),
			"good": rawMessage("Good"),
		},
	}
	ing := &stubIngester{created: true}
	p := quietPoller(ing, client)

	p.runOnce(context.Background())

	// Only the good message reaches ingestion...
	if ing.callCount != 1 || (len(ing.ingested) == 1 && ing.ingested[0] != "Good") {
		t.Fatalf("expected only the good message ingested, got %v", ing.ingested)
	}
	// ...but both are labeled: the good one normally, the poison one to retire it.
	labeled := client.labeledIDs()
	if len(labeled) != 2 {
		t.Fatalf("expected both messages labeled (incl. poison), got %v", labeled)
	}
}

func TestRunOnceIngestErrorLeavesUnlabeled(t *testing.T) {
	client := &stubMailClient{
		ids:  []string{"a"},
		raws: map[string][]byte{"a": rawMessage("A")},
	}
	ing := &stubIngester{err: errors.New("db down")}
	p := quietPoller(ing, client)

	p.runOnce(context.Background())

	if got := client.labeledIDs(); len(got) != 0 {
		t.Fatalf("expected no labels on ingest failure (so it retries), got %v", got)
	}
}

func TestRunOnceDuplicateStillLabeled(t *testing.T) {
	client := &stubMailClient{
		ids:  []string{"a"},
		raws: map[string][]byte{"a": rawMessage("A")},
	}
	ing := &stubIngester{created: false} // deduped against an existing row
	p := quietPoller(ing, client)

	p.runOnce(context.Background())

	if got := client.labeledIDs(); len(got) != 1 {
		t.Fatalf("expected a deduped message to still be labeled, got %v", got)
	}
}

func TestRunOnceRequestsBatchSize(t *testing.T) {
	client := &stubMailClient{}
	p := quietPoller(&stubIngester{}, client, WithBatch(7))

	p.runOnce(context.Background())

	if client.lastMax != 7 {
		t.Fatalf("expected ListUnprocessed called with batch 7, got %d", client.lastMax)
	}
}

func TestRunOnceListErrorIsSwallowed(t *testing.T) {
	client := &stubMailClient{listErr: errors.New("api down")}
	ing := &stubIngester{}
	p := quietPoller(ing, client)

	// Must not panic or call downstream seams when listing fails.
	p.runOnce(context.Background())

	if ing.callCount != 0 {
		t.Fatalf("expected no ingests when list fails, got %d", ing.callCount)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	client := &stubMailClient{}
	p := quietPoller(&stubIngester{}, client, WithInterval(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
