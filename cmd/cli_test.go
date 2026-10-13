package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/open-cli-collective/granola-cli/internal/credentials"
)

func loggedIn(t *testing.T) *fakeGranola {
	t.Helper()
	f := newFakeGranola(t)
	setupTest(t, f)
	storeSession(t, f, time.Now().Add(time.Hour))
	return f
}

func fixtureText(t *testing.T, name string) string {
	t.Helper()
	res := fixtureResult(t, name)
	return res.Content[0].(*mcp.TextContent).Text
}

func TestMeetingsListPrintsToolTextVerbatim(t *testing.T) {
	f := loggedIn(t)
	stdout, stderr, code := runCLI(t, "meetings", "list")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if want := fixtureText(t, "call_list_meetings.json") + "\n"; stdout != want {
		t.Fatalf("stdout differs from the tool text.\n got: %q\nwant: %q", stdout, want)
	}
	call := f.lastCall()
	if call.Name != "list_meetings" {
		t.Fatalf("called %s, want list_meetings", call.Name)
	}
	if !reflect.DeepEqual(call.Args, map[string]any{"time_range": "last_30_days"}) {
		t.Fatalf("args = %v", call.Args)
	}
}

func TestMeetingsListFilters(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "mine sets both involvement conditions",
			args: []string{"--time-range", "this_week", "--mine"},
			want: map[string]any{"time_range": "this_week", "involvement": map[string]any{"captured_by_me": true, "listed_as_participant": true}},
		},
		{
			name: "false condition is an exclusion",
			args: []string{"--captured-by-me=false"},
			want: map[string]any{"time_range": "last_30_days", "involvement": map[string]any{"captured_by_me": false}},
		},
		{
			name: "participant and workspace only",
			args: []string{"--participant", "--workspace-only", "--time-range", "last_week"},
			want: map[string]any{"time_range": "last_week", "workspace_only": true, "involvement": map[string]any{"listed_as_participant": true}},
		},
		{
			name: "folder passes folder_id",
			args: []string{"--folder", "fol_4y6LduVdwSKC27"},
			want: map[string]any{"time_range": "last_30_days", "folder_id": "fol_4y6LduVdwSKC27"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := loggedIn(t)
			_, stderr, code := runCLI(t, append([]string{"meetings", "list"}, tc.args...)...)
			if code != 0 {
				t.Fatalf("exit %d, stderr: %s", code, stderr)
			}
			if got := f.lastCall().Args; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUsageErrorsMakeNoRequest(t *testing.T) {
	cases := [][]string{
		{"meetings", "list", "--time-range", "last_year"},
		{"meetings", "list", "--mine", "--captured-by-me=false"},
		{"meetings", "get"},
		{"meetings", "get", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
		{"meetings", "query", "  "},
		{"meetings", "attachments", "a"},
		{"meetings", "attachments", "a", "b", "c", "d", "e", "f", "g"},
		{"tools", "call", "list_meetings", "--args", "not json"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := loggedIn(t)
			_, _, code := runCLI(t, args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			if f.callCount() != 0 {
				t.Fatalf("a usage error still called the server")
			}
		})
	}
}

func TestMeetingsGetQueryAndAccount(t *testing.T) {
	cases := []struct {
		args    []string
		tool    string
		want    map[string]any
		fixture string
	}{
		{
			args:    []string{"meetings", "get", "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d", "1c2a7d3f-5e4b-4a6c-8d9e-8f7a6b5c4d3e"},
			tool:    "get_meetings",
			want:    map[string]any{"meeting_ids": []any{"0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d", "1c2a7d3f-5e4b-4a6c-8d9e-8f7a6b5c4d3e"}},
			fixture: "call_get_meetings.json",
		},
		{
			args:    []string{"meetings", "query", "what did we decide?", "--meeting", "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d"},
			tool:    "query_granola_meetings",
			want:    map[string]any{"query": "what did we decide?", "document_ids": []any{"0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d"}},
			fixture: "call_query_granola_meetings.json",
		},
		{
			args:    []string{"account", "get"},
			tool:    "get_account_info",
			want:    map[string]any{},
			fixture: "call_get_account_info.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			f := loggedIn(t)
			stdout, stderr, code := runCLI(t, tc.args...)
			if code != 0 {
				t.Fatalf("exit %d, stderr: %s", code, stderr)
			}
			call := f.lastCall()
			if call.Name != tc.tool || !reflect.DeepEqual(call.Args, tc.want) {
				t.Fatalf("call = %s %v, want %s %v", call.Name, call.Args, tc.tool, tc.want)
			}
			if want := fixtureText(t, tc.fixture) + "\n"; stdout != want {
				t.Fatalf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestToolErrorTextIsSurfacedAsIs(t *testing.T) {
	cases := map[string][]string{
		"Meeting folders are only available to paid Granola tiers": {"folders", "list"},
		"Transcripts are only available to paid Granola tiers":     {"meetings", "transcript", "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d"},
	}
	for serverText, args := range cases {
		t.Run(args[0], func(t *testing.T) {
			loggedIn(t)
			stdout, stderr, code := runCLI(t, args...)
			if code != 5 {
				t.Fatalf("exit %d, want 5 (upstream)", code)
			}
			if stdout != "" {
				t.Fatalf("stdout should be empty on a tool error, got %q", stdout)
			}
			mustContain(t, stderr, serverText)
		})
	}
}

func TestJSONPrintsRawToolResult(t *testing.T) {
	loggedIn(t)
	stdout, stderr, code := runCLI(t, "meetings", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	var got, want map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	_ = json.Unmarshal(readFixture(t, "call_list_meetings.json"), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON result differs from the wire payload.\n got: %v\nwant: %v", got, want)
	}

	// An isError result is still printed in full, and the exit code says so.
	stdout, _, code = runCLI(t, "folders", "list", "--json")
	if code != 5 {
		t.Fatalf("exit %d, want 5", code)
	}
	mustContain(t, stdout, `"isError": true`)
}

func TestToolsListGetAndCall(t *testing.T) {
	f := loggedIn(t)
	stdout, _, code := runCLI(t, "tools", "list")
	if code != 0 {
		t.Fatalf("tools list exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if lines[0] != "NAME | DESCRIPTION" || len(lines) != 8 {
		t.Fatalf("unexpected table:\n%s", stdout)
	}
	mustContain(t, stdout, "get_meetings | Get detailed meeting information for one or more Granola meetings by ID.\n")

	stdout, _, code = runCLI(t, "tools", "get", "get_meeting_transcript")
	if code != 0 {
		t.Fatalf("tools get exit %d", code)
	}
	mustContain(t, stdout, `"meeting_id"`)

	_, _, code = runCLI(t, "tools", "get", "no_such_tool")
	if code != 2 {
		t.Fatalf("tools get unknown exit %d, want 2", code)
	}

	_, stderr, code := runCLI(t, "tools", "call", "list_meetings", "--args", `{"time_range":"last_week"}`)
	if code != 0 {
		t.Fatalf("tools call exit %d: %s", code, stderr)
	}
	if got := f.lastCall().Args; !reflect.DeepEqual(got, map[string]any{"time_range": "last_week"}) {
		t.Fatalf("args = %v", got)
	}
}

func TestToolsCallRefusesToolsNotMarkedReadOnly(t *testing.T) {
	f := loggedIn(t)
	f.markWritable("get_account_info")
	_, stderr, code := runCLI(t, "tools", "call", "get_account_info")
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	mustContain(t, stderr, "does not mark it read-only")
	if f.callCount() != 0 {
		t.Fatal("a tool not marked read-only was called")
	}
	_, _, code = runCLI(t, "tools", "call", "no_such_tool")
	if code != 2 || f.callCount() != 0 {
		t.Fatalf("unknown tool: exit %d, calls %d", code, f.callCount())
	}
}

func TestNotLoggedInIsAuthError(t *testing.T) {
	f := newFakeGranola(t)
	setupTest(t, f)
	_, stderr, code := runCLI(t, "meetings", "list")
	if code != 3 {
		t.Fatalf("exit %d, want 3", code)
	}
	mustContain(t, stderr, "granola login")
	if f.mcpRequests != 0 {
		t.Fatalf("made %d requests without a session", f.mcpRequests)
	}
}

func TestRejectedSessionIsAuthErrorWithoutBrowser(t *testing.T) {
	f := loggedIn(t)
	f.mu.Lock()
	f.validToken = "some-other-token"
	f.mu.Unlock()
	opened := false
	prev := openBrowser
	openBrowser = func(string) error { opened = true; return nil }
	t.Cleanup(func() { openBrowser = prev })

	_, stderr, code := runCLI(t, "account", "get")
	if code != 3 {
		t.Fatalf("exit %d, want 3; stderr: %s", code, stderr)
	}
	mustContain(t, stderr, "granola login")
	mustContain(t, stderr, "HTTP 401")
	if opened {
		t.Fatal("a normal command must never start the browser flow")
	}
}

func TestExpiredTokenRefreshesAndPersists(t *testing.T) {
	f := newFakeGranola(t)
	setupTest(t, f)
	storeSession(t, f, time.Now().Add(-time.Minute))
	f.mu.Lock()
	f.validToken = "grn-test-refreshed-access"
	f.mu.Unlock()

	_, stderr, code := runCLI(t, "account", "get")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if f.refreshCount != 1 {
		t.Fatalf("refresh count = %d, want 1", f.refreshCount)
	}
	sess, err := creds.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Token.AccessToken != "grn-test-refreshed-access" || sess.Token.RefreshToken != "grn-test-rotated-refresh" {
		t.Fatalf("refreshed token was not persisted: %+v", sess.Token)
	}
	if sess.Client.ClientID != testClientID {
		t.Fatalf("refresh changed the stored client")
	}
}

func TestFailedRefreshIsAuthError(t *testing.T) {
	f := newFakeGranola(t)
	setupTest(t, f)
	storeSession(t, f, time.Now().Add(-time.Minute))
	// Replace the stored refresh token with one the fake server rejects.
	sess, err := creds.Load()
	if err != nil {
		t.Fatal(err)
	}
	sess.Token.RefreshToken = "revoked"
	if err := creds.SaveToken(sess.Token); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, "account", "get")
	if code != 3 {
		t.Fatalf("exit %d, want 3; stderr: %s", code, stderr)
	}
	mustContain(t, stderr, "granola login")
	if f.callCount() != 0 {
		t.Fatal("a tool was called without a valid token")
	}
}

func TestLoginRunsDiscoveryRegistrationAndPKCE(t *testing.T) {
	f := newFakeGranola(t)
	setupTest(t, f)
	f.mu.Lock()
	f.validToken = f.issuedToken
	f.mu.Unlock()
	prev := openBrowser
	// Stand in for the user's browser: follow the authorize redirect back to
	// the loopback listener.
	openBrowser = func(u string) error {
		go func() {
			resp, err := http.Get(u) //nolint:gosec // test-only request to the local fake server
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	t.Cleanup(func() { openBrowser = prev })

	_, stderr, code := runCLI(t, "login", "--wait", "20s")
	if code != 0 {
		t.Fatalf("login exit %d: %s", code, stderr)
	}
	mustContain(t, stderr, "Signed in. Session stored at granola-cli/default.")

	f.mu.Lock()
	reg := f.registered
	f.mu.Unlock()
	uris, _ := reg["redirect_uris"].([]any)
	if len(uris) != 1 || !strings.HasPrefix(uris[0].(string), "http://127.0.0.1:") || !strings.HasSuffix(uris[0].(string), "/callback") {
		t.Fatalf("registered redirect_uris = %v, want one 127.0.0.1 loopback callback", reg["redirect_uris"])
	}
	if reg["token_endpoint_auth_method"] != "none" {
		t.Fatalf("registered as %v, want a public client", reg["token_endpoint_auth_method"])
	}
	if !f.pkceMatched() {
		t.Fatal("token request code_verifier does not match the S256 code_challenge")
	}

	sess, err := creds.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Client.ClientID != testClientID || sess.Client.TokenURL != f.srv.URL+"/oauth2/token" {
		t.Fatalf("stored client = %+v", sess.Client)
	}
	if sess.Token.AccessToken != f.issuedToken || sess.Token.RefreshToken != "grn-test-issued-refresh" {
		t.Fatalf("stored token = %+v", sess.Token)
	}

	// The stored session works for the next command without a browser.
	_, stderr, code = runCLI(t, "account", "get")
	if code != 0 {
		t.Fatalf("post-login command exit %d: %s", code, stderr)
	}
}

func TestLoginTimeoutReportsWithoutStoring(t *testing.T) {
	f := newFakeGranola(t)
	setupTest(t, f)
	_, stderr, code := runCLI(t, "login", "--no-browser", "--wait", "300ms")
	if code == 0 {
		t.Fatal("login without a browser redirect should fail")
	}
	mustContain(t, stderr, "timed out")
	if _, err := creds.Load(); err != credentials.ErrNotLoggedIn {
		t.Fatalf("a failed login stored a session: %v", err)
	}
}

func TestConfigShowAndClear(t *testing.T) {
	loggedIn(t)
	stdout, _, code := runCLI(t, "config", "show", "--json")
	if code != 0 {
		t.Fatalf("config show exit %d", code)
	}
	var st credentials.Status
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatal(err)
	}
	if st.Ref != "granola-cli/default" || !st.Keys["oauth_client"] || !st.Keys["oauth_token"] || !st.HasRefreshToken {
		t.Fatalf("status = %+v", st)
	}
	if st.Backend != "memory" || st.BackendSource != "explicit" {
		t.Fatalf("backend = %q (source %q), want memory (explicit)", st.Backend, st.BackendSource)
	}

	_, stderr, code := runCLI(t, "config", "clear")
	if code != 0 {
		t.Fatalf("config clear exit %d", code)
	}
	mustContain(t, stderr, "removed granola-cli/default/oauth_client")
	mustContain(t, stderr, "removed granola-cli/default/oauth_token")
	if _, err := creds.Load(); err != credentials.ErrNotLoggedIn {
		t.Fatalf("session survived config clear: %v", err)
	}
	stdout, _, _ = runCLI(t, "config", "show")
	mustContain(t, stdout, "oauth_client: missing")
}

// TestNoCredentialLeaks runs every command class with distinctive tokens in
// the store, verbose logging on, and asserts neither token appears in any
// output.
func TestNoCredentialLeaks(t *testing.T) {
	commands := [][]string{
		{"-v", "meetings", "list"},
		{"-v", "meetings", "list", "--json"},
		{"-v", "meetings", "get", "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d"},
		{"-v", "meetings", "query", "q"},
		{"-v", "meetings", "transcript", "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d"},
		{"-v", "folders", "list"},
		{"-v", "account", "get"},
		{"-v", "tools", "list", "--json"},
		{"-v", "tools", "call", "get_account_info", "--json"},
		{"config", "show"},
		{"config", "show", "--json"},
	}
	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			loggedIn(t)
			stdout, stderr, _ := runCLI(t, args...)
			for _, secret := range []string{testAccessToken, testRefreshToken} {
				if strings.Contains(stdout+stderr, secret) {
					t.Fatalf("output leaked a stored token")
				}
			}
		})
	}

	t.Run("rejected session", func(t *testing.T) {
		f := loggedIn(t)
		f.mu.Lock()
		f.validToken = "other"
		f.mu.Unlock()
		stdout, stderr, _ := runCLI(t, "-v", "account", "get")
		if strings.Contains(stdout+stderr, testAccessToken) || strings.Contains(stdout+stderr, testRefreshToken) {
			t.Fatal("auth failure output leaked a stored token")
		}
	})
}

func TestVersionAndHelp(t *testing.T) {
	setupTest(t, nil)
	stdout, _, code := runCLI(t, "version")
	if code != 0 || !strings.HasPrefix(stdout, "granola-cli ") {
		t.Fatalf("version: exit %d, %q", code, stdout)
	}
	for _, group := range []string{"meetings", "folders", "account", "tools", "config", "login"} {
		if _, _, code := runCLI(t, group, "--help"); code != 0 {
			t.Fatalf("%s --help exit %d", group, code)
		}
	}
	_, _, code = runCLI(t, "nope")
	if code != 2 {
		t.Fatalf("unknown command exit %d, want 2", code)
	}
}

func TestMeetingsAttachmentsSavesImagesInOrder(t *testing.T) {
	f := loggedIn(t)
	f.results["get_meeting_attachment"] = "call_get_meeting_attachment.json"
	dir := filepath.Join(t.TempDir(), "shots")
	meeting := "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d"
	ids := []string{"5d2e8f1a-3b4c-4d5e-8f6a-7b8c9d0e1f2a", "6e3f9a2b-4c5d-4e6f-9a7b-8c9d0e1f2a3b"}
	stdout, stderr, code := runCLI(t, append([]string{"meetings", "attachments", meeting}, append(ids, "--out-dir", dir)...)...)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	call := f.lastCall()
	want := map[string]any{"meeting_id": meeting, "attachment_ids": []any{ids[0], ids[1]}}
	if call.Name != "get_meeting_attachment" || !reflect.DeepEqual(call.Args, want) {
		t.Fatalf("call = %s %v, want get_meeting_attachment %v", call.Name, call.Args, want)
	}

	res := fixtureResult(t, "call_get_meeting_attachment.json")
	png := filepath.Join(dir, meeting+"-1.png")
	jpg := filepath.Join(dir, meeting+"-2.jpg")
	wantOut := fmt.Sprintf("Saved %s (image/png, 21 bytes)\n%s\nSaved %s (image/jpeg, 22 bytes)\n%s\n",
		png, res.Content[1].(*mcp.TextContent).Text, jpg, res.Content[3].(*mcp.TextContent).Text)
	if stdout != wantOut {
		t.Fatalf("stdout = %q, want %q", stdout, wantOut)
	}
	for i, path := range []string{png, jpg} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, res.Content[i*2].(*mcp.ImageContent).Data) {
			t.Fatalf("%s does not hold the image bytes", path)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %v, want 0600", path, info.Mode().Perm())
		}
	}
}

func TestMeetingsAttachmentsPartialFailureKeepsImages(t *testing.T) {
	f := loggedIn(t)
	f.results["get_meeting_attachment"] = "call_get_meeting_attachment_partial.json"
	dir := t.TempDir()
	meeting := "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d"
	stdout, stderr, code := runCLI(t, "meetings", "attachments", meeting,
		"5d2e8f1a-3b4c-4d5e-8f6a-7b8c9d0e1f2a", "00000000-0000-0000-0000-000000000000", "--out-dir", dir)
	if code != 5 {
		t.Fatalf("exit %d, want 5 (upstream)", code)
	}
	if _, err := os.Stat(filepath.Join(dir, meeting+"-1.png")); err != nil {
		t.Fatalf("the image that succeeded was not saved: %v", err)
	}
	mustContain(t, stdout, "No attachment with this attachment_id in this meeting")
	mustContain(t, stderr, "one or more attachments failed")
}

func TestMeetingsAttachmentsErrorWithoutImages(t *testing.T) {
	f := loggedIn(t)
	f.results["get_meeting_attachment"] = "call_get_meeting_attachment_not_found.json"
	dir := filepath.Join(t.TempDir(), "unused")
	stdout, stderr, code := runCLI(t, "meetings", "attachments", "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d",
		"00000000-0000-0000-0000-000000000000", "--out-dir", dir)
	if code != 5 {
		t.Fatalf("exit %d, want 5 (upstream)", code)
	}
	if stdout != "" {
		t.Fatalf("stdout should be empty on a tool error, got %q", stdout)
	}
	mustContain(t, stderr, "No attachment with this attachment_id in this meeting")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("--out-dir was created with nothing to save")
	}
}

func TestMeetingsAttachmentsJSONSavesNothing(t *testing.T) {
	f := loggedIn(t)
	f.results["get_meeting_attachment"] = "call_get_meeting_attachment.json"
	dir := filepath.Join(t.TempDir(), "unused")
	stdout, stderr, code := runCLI(t, "meetings", "attachments", "0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d",
		"5d2e8f1a-3b4c-4d5e-8f6a-7b8c9d0e1f2a", "--out-dir", dir, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	mustContain(t, stdout, `"type": "image"`)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("--json wrote files")
	}
}
