// Package credentials stores the Granola OAuth session (the dynamically
// registered client and its token) in the OS credential store under the
// credential_ref granola-cli/default.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/open-cli-collective/cli-common/credstore"
)

const (
	ServiceName = "granola-cli"
	Profile     = "default"

	// KeyClient holds the OAuth client registered with Granola's
	// authorization server during login.
	KeyClient = "oauth_client"
	// KeyToken holds the access/refresh token pair for that client.
	KeyToken = "oauth_token"
)

// Keys is the complete declared key set for the bundle.
var Keys = []string{KeyClient, KeyToken}

// Ref is the credential_ref this tool reads and writes.
const Ref = ServiceName + "/" + Profile

// ErrNotLoggedIn means no session is stored.
var ErrNotLoggedIn = errors.New("no Granola session stored; run `granola login`")

// Client is the OAuth client registration obtained through Dynamic Client
// Registration. ClientSecret is empty for a public client.
type Client struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	AuthStyle    int    `json:"auth_style"`
	RedirectURL  string `json:"redirect_url"`
}

// Token is the persisted OAuth token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// Session is everything needed to call the MCP server without a browser.
type Session struct {
	Client Client
	Token  Token
}

// Store is the subset of credstore.Store this package uses.
type Store interface {
	Get(profile, key string) (string, error)
	Set(profile, key, value string, opts ...credstore.SetOpt) error
	SetBundle(profile string, values map[string]string, opts ...credstore.SetOpt) (credstore.Result, error)
	ListBundle(profile string) ([]string, error)
	DeleteBundle(profile string) ([]string, error)
	Backend() (credstore.Backend, credstore.Source)
	Close() error
}

// Opener opens a credential store.
type Opener func() (Store, error)

// Manager reads and writes the session through one store opener.
type Manager struct {
	open Opener
}

// New returns a Manager over the given opener; tests pass a memory store.
func New(open Opener) *Manager {
	return &Manager{open: open}
}

// Default returns a Manager over the OS credential store.
func Default() *Manager {
	return New(func() (Store, error) {
		return credstore.Open(ServiceName, &credstore.Options{AllowedKeys: Keys})
	})
}

// Load returns the stored session, or ErrNotLoggedIn when none exists.
func (m *Manager) Load() (*Session, error) {
	s, err := m.open()
	if err != nil {
		return nil, fmt.Errorf("open credential store: %w", err)
	}
	defer s.Close()

	rawClient, err := s.Get(Profile, KeyClient)
	if errors.Is(err, credstore.ErrNotFound) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, fmt.Errorf("read %s/%s: %w", Ref, KeyClient, err)
	}
	rawToken, err := s.Get(Profile, KeyToken)
	if errors.Is(err, credstore.ErrNotFound) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, fmt.Errorf("read %s/%s: %w", Ref, KeyToken, err)
	}
	var sess Session
	if err := json.Unmarshal([]byte(rawClient), &sess.Client); err != nil {
		return nil, fmt.Errorf("decode %s/%s: stored value is malformed; run `granola login`", Ref, KeyClient)
	}
	if err := json.Unmarshal([]byte(rawToken), &sess.Token); err != nil {
		return nil, fmt.Errorf("decode %s/%s: stored value is malformed; run `granola login`", Ref, KeyToken)
	}
	return &sess, nil
}

// SaveSession writes the client and token together, replacing any prior
// session.
func (m *Manager) SaveSession(sess Session) error {
	client, err := json.Marshal(sess.Client) //nolint:gosec // G117: serialized only into the OS credential store
	if err != nil {
		return err
	}
	token, err := json.Marshal(sess.Token) //nolint:gosec // G117: serialized only into the OS credential store
	if err != nil {
		return err
	}
	s, err := m.open()
	if err != nil {
		return fmt.Errorf("open credential store: %w", err)
	}
	defer s.Close()
	_, err = s.SetBundle(Profile, map[string]string{
		KeyClient: string(client),
		KeyToken:  string(token),
	}, credstore.WithOverwrite())
	if err != nil {
		return fmt.Errorf("write %s: %w", Ref, err)
	}
	return nil
}

// SaveToken persists a token the authorization server issued on refresh.
func (m *Manager) SaveToken(tok Token) error {
	raw, err := json.Marshal(tok) //nolint:gosec // G117: serialized only into the OS credential store
	if err != nil {
		return err
	}
	s, err := m.open()
	if err != nil {
		return fmt.Errorf("open credential store: %w", err)
	}
	defer s.Close()
	if err := s.Set(Profile, KeyToken, string(raw), credstore.WithOverwrite()); err != nil {
		return fmt.Errorf("write %s/%s: %w", Ref, KeyToken, err)
	}
	return nil
}

// Status reports which declared keys are present and the backend in use.
// It never returns a credential value.
type Status struct {
	Ref     string `json:"ref"`
	Backend string `json:"backend"`
	// BackendSource says how the backend was chosen (auto, env, config, explicit).
	BackendSource string          `json:"backend_source"`
	Keys          map[string]bool `json:"keys"`
	// TokenExpiry is the access token's expiry, when a token is stored.
	TokenExpiry *time.Time `json:"token_expiry,omitempty"`
	// HasRefreshToken reports whether silent refresh is possible.
	HasRefreshToken bool `json:"has_refresh_token"`
}

// Inspect returns the presence of every declared key.
func (m *Manager) Inspect() (*Status, error) {
	s, err := m.open()
	if err != nil {
		return nil, fmt.Errorf("open credential store: %w", err)
	}
	defer s.Close()
	backend, source := s.Backend()
	present, err := s.ListBundle(Profile)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", Ref, err)
	}
	st := &Status{Ref: Ref, Backend: string(backend), BackendSource: string(source), Keys: map[string]bool{}}
	for _, k := range Keys {
		st.Keys[k] = false
	}
	for _, k := range present {
		if _, ok := st.Keys[k]; ok {
			st.Keys[k] = true
		}
	}
	if st.Keys[KeyToken] {
		raw, err := s.Get(Profile, KeyToken)
		if err == nil {
			var tok Token
			if json.Unmarshal([]byte(raw), &tok) == nil {
				if !tok.Expiry.IsZero() {
					exp := tok.Expiry
					st.TokenExpiry = &exp
				}
				st.HasRefreshToken = tok.RefreshToken != ""
			}
		}
	}
	return st, nil
}

// Clear removes every key under the active ref and returns what was removed.
func (m *Manager) Clear() ([]string, error) {
	s, err := m.open()
	if err != nil {
		return nil, fmt.Errorf("open credential store: %w", err)
	}
	defer s.Close()
	removed, err := s.DeleteBundle(Profile)
	if err != nil {
		return removed, fmt.Errorf("clear %s: %w", Ref, err)
	}
	return removed, nil
}
