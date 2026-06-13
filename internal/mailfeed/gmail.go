package mailfeed

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailScope is the OAuth scope the feed needs: read messages and modify labels
// (the latter to apply the processed label). It is the narrowest scope that
// covers both.
const GmailScope = gmail.GmailModifyScope

// OAuthConfig carries everything needed to build a Gmail-backed MailClient.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	// User is the mailbox to read; "me" is the authorized account.
	User string
	// Query is the Gmail search filter selecting unprocessed mail.
	Query string
	// Label is the Gmail label applied to a message after it is ingested.
	Label string
}

// gmailClient is the production MailClient backed by the Gmail REST API.
type gmailClient struct {
	svc     *gmail.Service
	user    string
	query   string
	labelID string
}

// oauthConfig builds the OAuth2 config for Google's installed-app flow. The same
// config serves both token minting (Authorize) and refresh-token use; only the
// redirect URL differs, and it is irrelevant once a refresh token exists.
func oauthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{GmailScope},
	}
}

// NewGmailClient builds a MailClient from OAuth2 credentials. It mints access
// tokens from the stored refresh token (auto-refreshing as they expire) and
// resolves the processed label once, creating it if absent.
func NewGmailClient(ctx context.Context, oc OAuthConfig) (MailClient, error) {
	cfg := oauthConfig(oc.ClientID, oc.ClientSecret)
	ts := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: oc.RefreshToken})

	svc, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("gmail service: %w", err)
	}

	user := oc.User
	if user == "" {
		user = "me"
	}

	labelID, err := resolveLabel(ctx, svc, user, oc.Label)
	if err != nil {
		return nil, fmt.Errorf("resolve label %q: %w", oc.Label, err)
	}

	return &gmailClient{svc: svc, user: user, query: oc.Query, labelID: labelID}, nil
}

// resolveLabel returns the ID of the named label, creating it if it does not
// exist. The label is the poller's seen-state cursor, so it must always resolve.
func resolveLabel(ctx context.Context, svc *gmail.Service, user, name string) (string, error) {
	resp, err := svc.Users.Labels.List(user).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	for _, l := range resp.Labels {
		if l.Name == name {
			return l.Id, nil
		}
	}
	created, err := svc.Users.Labels.Create(user, &gmail.Label{
		Name:                  name,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return created.Id, nil
}

// ListUnprocessed returns up to max message IDs matching the unprocessed query.
func (c *gmailClient) ListUnprocessed(ctx context.Context, max int) ([]string, error) {
	resp, err := c.svc.Users.Messages.List(c.user).Q(c.query).MaxResults(int64(max)).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		ids = append(ids, m.Id)
	}
	return ids, nil
}

// GetRaw fetches a message in raw format and returns the decoded RFC822 bytes.
func (c *gmailClient) GetRaw(ctx context.Context, id string) ([]byte, error) {
	msg, err := c.svc.Users.Messages.Get(c.user, id).Format("raw").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	// Gmail returns the raw message as web-safe (URL-encoded) base64.
	return base64.URLEncoding.DecodeString(msg.Raw)
}

// MarkProcessed adds the processed label to a message.
func (c *gmailClient) MarkProcessed(ctx context.Context, id string) error {
	_, err := c.svc.Users.Messages.Modify(c.user, id, &gmail.ModifyMessageRequest{
		AddLabelIds: []string{c.labelID},
	}).Context(ctx).Do()
	return err
}

// Authorize runs the one-time OAuth2 installed-app flow and returns a refresh
// token to store in GMAIL_REFRESH_TOKEN. It listens on a loopback port (which
// Google permits for "Desktop app" OAuth clients without registration), prints
// the consent URL for the user to open, captures the redirected auth code, and
// exchanges it. Backs the `centrifuge authorize-gmail` subcommand.
func Authorize(ctx context.Context, clientID, clientSecret string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	cfg := oauthConfig(clientID, clientSecret)
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", port)

	state, err := randomState()
	if err != nil {
		return "", err
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch on redirect")
			return
		}
		if e := q.Get("error"); e != "" {
			http.Error(w, "authorization denied: "+e, http.StatusBadRequest)
			errCh <- fmt.Errorf("authorization denied: %s", e)
			return
		}
		fmt.Fprintln(w, "Authorization complete. You can close this tab and return to the terminal.")
		codeCh <- q.Get("code")
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("Open this URL in a browser signed in as the newsletters account, then grant access:")
	fmt.Println()
	fmt.Println("  " + authURL)
	fmt.Println()
	fmt.Println("Waiting for authorization...")

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}

	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("no refresh token returned (the account may have already granted access); revoke it at https://myaccount.google.com/permissions and retry")
	}
	return tok.RefreshToken, nil
}

// randomState returns an unguessable state value guarding the OAuth redirect.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
