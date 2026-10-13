package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/open-cli-collective/cli-common/credstore"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/open-cli-collective/granola-cli/internal/clierr"
	"github.com/open-cli-collective/granola-cli/internal/credentials"
	"github.com/open-cli-collective/granola-cli/internal/mcpclient"
)

// Distinctive values the no-leak test greps for.
const (
	testAccessToken  = "grn-test-access-ZX81QK7P"
	testRefreshToken = "grn-test-refresh-QW55VR2M"
	testClientID     = "client-test-7731"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func fixtureResult(t *testing.T, name string) *mcp.CallToolResult {
	t.Helper()
	var res mcp.CallToolResult
	if err := json.Unmarshal(readFixture(t, name), &res); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return &res
}

// fakeGranola is an in-process stand-in for Granola's MCP server and its
// OAuth authorization server, serving the synthetic fixtures.
type fakeGranola struct {
	t   *testing.T
	srv *httptest.Server

	server *mcp.Server

	mu          sync.Mutex
	validToken  string
	calls       []toolCall
	results     map[string]string // tool name -> fixture file
	mcpRequests int

	// OAuth state.
	registered   map[string]any
	codeVerifier string
	challenge    string
	refreshCount int
	issuedToken  string
	// refreshStatus, when set, makes the token endpoint answer refresh
	// requests with this HTTP status.
	refreshStatus int
}

type toolCall struct {
	Name string
	Args map[string]any
}

func newFakeGranola(t *testing.T) *fakeGranola {
	t.Helper()
	f := &fakeGranola{
		t:          t,
		validToken: testAccessToken,
		results: map[string]string{
			"list_meetings":          "call_list_meetings.json",
			"get_meetings":           "call_get_meetings.json",
			"query_granola_meetings": "call_query_granola_meetings.json",
			"get_account_info":       "call_get_account_info.json",
			"list_meeting_folders":   "call_list_meeting_folders_paid_only.json",
			"get_meeting_transcript": "call_get_meeting_transcript_paid_only.json",
		},
		issuedToken: "grn-test-issued-KP90",
	}

	var list struct {
		Tools []*mcp.Tool `json:"tools"`
	}
	if err := json.Unmarshal(readFixture(t, "tools_list.json"), &list); err != nil {
		t.Fatalf("decode tools_list.json: %v", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "fake-granola", Version: "0"}, nil)
	f.server = server
	for _, tool := range list.Tools {
		name := tool.Name
		server.AddTool(tool, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{}
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, err
				}
			}
			f.mu.Lock()
			f.calls = append(f.calls, toolCall{Name: name, Args: args})
			fixture := f.results[name]
			f.mu.Unlock()
			return fixtureResult(t, fixture), nil
		})
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.mcpRequests++
		want := "Bearer " + f.validToken
		f.mu.Unlock()
		if r.Header.Get("Authorization") != want {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_token", error_description="Authorization needed", resource_metadata="%s/.well-known/oauth-protected-resource"`, f.srv.URL))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{
			"resource":                 f.srv.URL + "/mcp",
			"authorization_servers":    []string{f.srv.URL},
			"bearer_methods_supported": []string{"header"},
			"scopes_supported":         []string{"mcp"},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{
			"issuer":                                f.srv.URL,
			"authorization_endpoint":                f.srv.URL + "/oauth2/authorize",
			"token_endpoint":                        f.srv.URL + "/oauth2/token",
			"registration_endpoint":                 f.srv.URL + "/oauth2/register",
			"code_challenge_methods_supported":      []string{"S256"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"response_types_supported":              []string{"code"},
			"scopes_supported":                      []string{"email", "offline_access", "openid", "profile"},
			"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		})
	})
	mux.HandleFunc("/oauth2/register", func(w http.ResponseWriter, r *http.Request) {
		var meta map[string]any
		_ = json.NewDecoder(r.Body).Decode(&meta)
		f.mu.Lock()
		f.registered = meta
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeTestJSON(w, map[string]any{
			"client_id":                  testClientID,
			"redirect_uris":              meta["redirect_uris"],
			"token_endpoint_auth_method": "none",
			"grant_types":                []string{"authorization_code", "refresh_token"},
		})
	})
	mux.HandleFunc("/oauth2/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.mu.Lock()
		f.challenge = q.Get("code_challenge")
		f.mu.Unlock()
		target, _ := url.Parse(q.Get("redirect_uri"))
		tq := target.Query()
		tq.Set("code", "test-auth-code")
		tq.Set("state", q.Get("state"))
		target.RawQuery = tq.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			f.codeVerifier = r.Form.Get("code_verifier")
			f.validToken = f.issuedToken
			writeTestJSON(w, map[string]any{
				"access_token": f.issuedToken, "refresh_token": "grn-test-issued-refresh",
				"token_type": "Bearer", "expires_in": 3600,
			})
		case "refresh_token":
			if f.refreshStatus != 0 {
				w.WriteHeader(f.refreshStatus)
				return
			}
			if r.Form.Get("refresh_token") != testRefreshToken {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				writeTestJSON(w, map[string]any{"error": "invalid_grant"})
				return
			}
			f.refreshCount++
			f.validToken = "grn-test-refreshed-access"
			writeTestJSON(w, map[string]any{
				"access_token": f.validToken, "refresh_token": "grn-test-rotated-refresh",
				"token_type": "Bearer", "expires_in": 3600,
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeGranola) lastCall() toolCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		f.t.Fatal("the fake server received no tool call")
	}
	return f.calls[len(f.calls)-1]
}

// markWritable re-registers a tool without the readOnlyHint annotation.
func (f *fakeGranola) markWritable(name string) {
	var list struct {
		Tools []*mcp.Tool `json:"tools"`
	}
	_ = json.Unmarshal(readFixture(f.t, "tools_list.json"), &list)
	for _, tool := range list.Tools {
		if tool.Name == name {
			tool.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: false}
			f.server.AddTool(tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				f.mu.Lock()
				f.calls = append(f.calls, toolCall{Name: name})
				f.mu.Unlock()
				return &mcp.CallToolResult{}, nil
			})
		}
	}
}

func (f *fakeGranola) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeGranola) pkceMatched() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	sum := sha256.Sum256([]byte(f.codeVerifier))
	return f.codeVerifier != "" && base64.RawURLEncoding.EncodeToString(sum[:]) == f.challenge
}

// memStore is a memory credential store shared across Opener calls.
type memStore struct{ *credstore.Store }

func (memStore) Close() error { return nil }

// setupTest points the CLI at the fake server and a memory credential store.
func setupTest(t *testing.T, f *fakeGranola) *credstore.Store {
	t.Helper()
	store, err := credstore.Open(credentials.ServiceName, &credstore.Options{
		Backend:     credstore.BackendMemory,
		AllowedKeys: credentials.Keys,
	})
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	prevCreds := creds
	creds = credentials.New(func() (credentials.Store, error) { return memStore{store}, nil })
	prevEndpoint := endpointOverride
	prevFactory := httpClientFactory
	httpClientFactory = func(log io.Writer) *http.Client { return mcpclient.NewHTTPClientWithRate(log, 1000, 1000) }
	if f != nil {
		endpointOverride = f.srv.URL + "/mcp"
	}
	t.Cleanup(func() {
		creds = prevCreds
		endpointOverride = prevEndpoint
		httpClientFactory = prevFactory
	})
	return store
}

// storeSession writes a session whose token URL points at the fake server.
func storeSession(t *testing.T, f *fakeGranola, expiry time.Time) {
	t.Helper()
	err := creds.SaveSession(credentials.Session{
		Client: credentials.Client{
			ClientID:    testClientID,
			AuthURL:     f.srv.URL + "/oauth2/authorize",
			TokenURL:    f.srv.URL + "/oauth2/token",
			AuthStyle:   1,
			RedirectURL: "http://127.0.0.1:1/callback",
		},
		Token: credentials.Token{
			AccessToken:  testAccessToken,
			RefreshToken: testRefreshToken,
			TokenType:    "Bearer",
			Expiry:       expiry,
		},
	})
	if err != nil {
		t.Fatalf("store session: %v", err)
	}
}

// runCLI executes the command tree and returns stdout, stderr, and the exit
// code Execute would use.
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	resetFlags(rootCmd)
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)
	err := rootCmd.ExecuteContext(context.Background())
	code := 0
	if err != nil {
		code = clierr.CodeOf(classifyError(err))
	}
	return stdout.String(), stderr.String(), code
}

// resetFlags restores every flag to its default so package-level flag
// variables do not carry over between runCLI calls.
func resetFlags(c *cobra.Command) {
	reset := func(fl *pflag.Flag) {
		if sv, ok := fl.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = fl.Value.Set(fl.DefValue)
		}
		fl.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}
