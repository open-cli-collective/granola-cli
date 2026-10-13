// Package oauthlogin runs the one-time browser sign-in for Granola's MCP
// server: MCP authorization discovery, Dynamic Client Registration, and the
// authorization code flow with PKCE against a loopback redirect on 127.0.0.1.
// Every endpoint is discovered at runtime from the server's metadata; the SDK's
// AuthorizationCodeHandler performs discovery, registration, and the exchange.
package oauthlogin

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/open-cli-collective/granola-cli/internal/credentials"
	"github.com/open-cli-collective/granola-cli/internal/mcpclient"
)

const (
	callbackPath = "/callback"
	clientName   = "granola-cli"
)

// Options configures Login.
type Options struct {
	// Endpoint overrides the MCP server URL (tests only).
	Endpoint string
	// HTTPClient is used for discovery, registration, token exchange, and the
	// MCP connection that triggers the flow.
	HTTPClient *http.Client
	// OpenBrowser opens the authorization URL. Errors are reported but not
	// fatal: the URL is always printed so the user can open it by hand.
	OpenBrowser func(url string) error
	// Out receives progress lines (stderr in the CLI).
	Out io.Writer
	// Wait bounds how long Login waits for the browser redirect.
	Wait time.Duration
}

// Login runs the browser flow and returns the resulting session. It does not
// persist the session; the caller decides when to save it.
func Login(ctx context.Context, opts Options) (*credentials.Session, error) {
	if opts.Wait <= 0 {
		opts.Wait = 5 * time.Minute
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback for the OAuth redirect: %w", err)
	}
	redirectURL := fmt.Sprintf("http://%s%s", ln.Addr().String(), callbackPath)

	results := make(chan callbackResult, 1)
	srv := &http.Server{Handler: callbackHandler(results), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	var (
		mu       sync.Mutex
		captured *credentials.Session
	)
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:              clientName,
				RedirectURIs:            []string{redirectURL},
				GrantTypes:              []string{"authorization_code", "refresh_token"},
				ResponseTypes:           []string{"code"},
				TokenEndpointAuthMethod: "none",
			},
		},
		RedirectURL:           redirectURL,
		RequestRefreshToken:   true,
		AcceptUnadvertisedIss: true,
		Client:                opts.HTTPClient,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			fmt.Fprintf(opts.Out, "Opening your browser to sign in to Granola. If it does not open, visit:\n\n  %s\n\n", args.URL)
			if opts.OpenBrowser != nil {
				if err := opts.OpenBrowser(args.URL); err != nil {
					fmt.Fprintf(opts.Out, "could not open a browser (%v); open the URL above manually\n", err)
				}
			}
			fmt.Fprintf(opts.Out, "Waiting up to %s for the sign-in to complete...\n", opts.Wait)
			return waitForCallback(ctx, results, opts.Wait)
		},
		NewTokenSource: func(ctx context.Context, cfg *oauth2.Config, tok *oauth2.Token) (oauth2.TokenSource, error) {
			mu.Lock()
			captured = &credentials.Session{
				Client: credentials.Client{
					ClientID:     cfg.ClientID,
					ClientSecret: cfg.ClientSecret,
					AuthURL:      cfg.Endpoint.AuthURL,
					TokenURL:     cfg.Endpoint.TokenURL,
					AuthStyle:    int(cfg.Endpoint.AuthStyle),
					RedirectURL:  cfg.RedirectURL,
				},
				Token: mcpclient.FromOAuth2(tok),
			}
			mu.Unlock()
			return cfg.TokenSource(ctx, tok), nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("configure OAuth: %w", err)
	}

	// Connecting without a token draws a 401 from the server, which makes the
	// transport run the handler's Authorize flow and then retry initialize
	// with the new token. A successful connect proves the token works.
	cs, err := mcpclient.Connect(ctx, mcpclient.Options{
		Endpoint:     opts.Endpoint,
		HTTPClient:   opts.HTTPClient,
		OAuthHandler: handler,
	})
	if err != nil {
		return nil, fmt.Errorf("sign in to Granola: %w", err)
	}
	_ = cs.Close()

	mu.Lock()
	defer mu.Unlock()
	if captured == nil {
		return nil, errors.New("sign-in completed without issuing a token")
	}
	return captured, nil
}

type callbackResult struct {
	code, state, iss, err, desc string
}

func waitForCallback(ctx context.Context, results <-chan callbackResult, wait time.Duration) (*auth.AuthorizationResult, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case r := <-results:
		if r.err != "" {
			return nil, fmt.Errorf("authorization denied: %s %s", r.err, r.desc)
		}
		return &auth.AuthorizationResult{Code: r.code, State: r.state, Iss: r.iss}, nil
	case <-timer.C:
		return nil, fmt.Errorf("timed out after %s waiting for the Granola sign-in in the browser", wait)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func callbackHandler(results chan<- callbackResult) http.Handler {
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		res := callbackResult{
			code:  q.Get("code"),
			state: q.Get("state"),
			iss:   q.Get("iss"),
			err:   q.Get("error"),
			desc:  q.Get("error_description"),
		}
		if res.code == "" && res.err == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if res.err != "" {
			fmt.Fprintf(w, "<!DOCTYPE html><title>Granola sign-in failed</title><p>Sign-in failed: %s</p><p>You can close this window.</p>",
				html.EscapeString(res.err))
		} else {
			fmt.Fprint(w, "<!DOCTYPE html><title>Granola sign-in complete</title><p>Signed in. You can close this window and return to the terminal.</p>")
		}
		once.Do(func() { results <- res })
	})
	return mux
}
