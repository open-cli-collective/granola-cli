// Package mcpclient talks to Granola's hosted MCP server over Streamable HTTP
// using the official MCP Go SDK.
package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/open-cli-collective/granola-cli/internal/credentials"
	"github.com/open-cli-collective/granola-cli/internal/version"
)

// Endpoint is Granola's documented MCP server URL.
const Endpoint = "https://mcp.granola.ai/mcp"

// protocolVersion is requested in initialize. Left unset, the SDK first probes
// with a newer server/discover RPC that Granola's server answers with HTTP 400
// before falling back to initialize; asking for the initialize-era version
// directly saves that request on every command.
const protocolVersion = "2025-11-25"

// ErrSessionRejected means the server refused the stored session.
var ErrSessionRejected = errors.New("the Granola session was rejected")

// Options configures a connection.
type Options struct {
	// Endpoint overrides the server URL (tests only).
	Endpoint string
	// HTTPClient carries pacing and optional request logging.
	HTTPClient *http.Client
	// OAuthHandler authorizes requests.
	OAuthHandler auth.OAuthHandler
}

// Connect opens an MCP client session. The caller must Close it.
func Connect(ctx context.Context, opts Options) (*mcp.ClientSession, error) {
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = Endpoint
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           opts.HTTPClient,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
		OAuthHandler:         opts.OAuthHandler,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "granola-cli", Version: version.Version}, nil)
	return client.Connect(ctx, transport, &mcp.ClientSessionOptions{ProtocolVersion: protocolVersion})
}

// StoredHandler authorizes requests with a persisted session. It refreshes
// the access token silently and writes refreshed tokens back to the
// credential store. It never starts a browser flow: a rejected session is an
// error that tells the user to run `granola login`.
type StoredHandler struct {
	ts oauth2.TokenSource
}

// NewStoredHandler builds a handler from a stored session. httpClient is used
// for token refresh, save persists refreshed tokens, and ctx bounds every
// refresh; the handler lives for one command, so the command's context is the
// right lifetime.
func NewStoredHandler(ctx context.Context, sess *credentials.Session, httpClient *http.Client, save func(credentials.Token) error) *StoredHandler {
	cfg := &oauth2.Config{
		ClientID:     sess.Client.ClientID,
		ClientSecret: sess.Client.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   sess.Client.AuthURL,
			TokenURL:  sess.Client.TokenURL,
			AuthStyle: oauth2.AuthStyle(sess.Client.AuthStyle),
		},
		RedirectURL: sess.Client.RedirectURL,
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	initial := ToOAuth2(sess.Token)
	return &StoredHandler{ts: &persistingTokenSource{
		base: oauth2.ReuseTokenSource(initial, cfg.TokenSource(ctx, initial)),
		last: initial.AccessToken,
		save: save,
	}}
}

// TokenSource implements auth.OAuthHandler.
func (h *StoredHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return h.ts, nil
}

// Authorize implements auth.OAuthHandler. It surfaces the server's response
// instead of re-authorizing.
func (h *StoredHandler) Authorize(_ context.Context, _ *http.Request, resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := strings.TrimSpace(string(body))
	if challenge := resp.Header.Get("WWW-Authenticate"); challenge != "" {
		detail = strings.TrimSpace(detail + " " + challenge)
	}
	return fmt.Errorf("%w (HTTP %d: %s); run `granola login`", ErrSessionRejected, resp.StatusCode, detail)
}

// persistingTokenSource writes a token back to the credential store whenever
// the underlying source returns a new access token. mu makes Token safe for
// concurrent use, as oauth2.TokenSource requires, and keeps one refresh from
// being persisted twice.
type persistingTokenSource struct {
	mu   sync.Mutex
	base oauth2.TokenSource
	last string
	save func(credentials.Token) error
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tok, err := p.base.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: refreshing the access token failed: %w; run `granola login`", ErrSessionRejected, err)
	}
	if tok.AccessToken != p.last {
		if err := p.save(FromOAuth2(tok)); err != nil {
			return nil, fmt.Errorf("persist refreshed token: %w", err)
		}
		p.last = tok.AccessToken
	}
	return tok, nil
}

// ToOAuth2 converts a stored token.
func ToOAuth2(t credentials.Token) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

// FromOAuth2 converts a token for storage.
func FromOAuth2(t *oauth2.Token) credentials.Token {
	return credentials.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}
