package credentials

import (
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/open-cli-collective/cli-common/credstore"
)

type sharedStore struct{ *credstore.Store }

func (sharedStore) Close() error { return nil }

func useMemoryStore(t *testing.T) (*Manager, *credstore.Store) {
	t.Helper()
	s, err := credstore.Open(ServiceName, &credstore.Options{Backend: credstore.BackendMemory, AllowedKeys: Keys})
	if err != nil {
		t.Fatal(err)
	}
	return New(func() (Store, error) { return sharedStore{s}, nil }), s
}

func sampleSession() Session {
	return Session{
		Client: Client{ClientID: "client-1", AuthURL: "https://auth.example/authorize", TokenURL: "https://auth.example/token", AuthStyle: 1, RedirectURL: "http://127.0.0.1:5000/callback"},
		Token:  Token{AccessToken: "access-1", RefreshToken: "refresh-1", TokenType: "Bearer", Expiry: time.Date(2026, 1, 27, 15, 30, 0, 0, time.UTC)},
	}
}

func TestSaveLoadRoundTripAndExactKeySet(t *testing.T) {
	m, s := useMemoryStore(t)
	if _, err := m.Load(); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("empty store: got %v, want ErrNotLoggedIn", err)
	}
	if err := m.SaveSession(sampleSession()); err != nil {
		t.Fatal(err)
	}
	got, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, sampleSession()) {
		t.Fatalf("round trip = %+v", *got)
	}
	keys, err := s.ListBundle(Profile)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string(nil), Keys...)
	sort.Strings(want)
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("bundle keys = %v, want exactly %v", keys, want)
	}
}

func TestSaveTokenReplacesOnlyTheToken(t *testing.T) {
	m, _ := useMemoryStore(t)
	if err := m.SaveSession(sampleSession()); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveToken(Token{AccessToken: "access-2", RefreshToken: "refresh-2"}); err != nil {
		t.Fatal(err)
	}
	got, err := m.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token.AccessToken != "access-2" || got.Client != sampleSession().Client {
		t.Fatalf("got %+v", *got)
	}
}

func TestInspectReportsPresenceNotValues(t *testing.T) {
	m, _ := useMemoryStore(t)
	if err := m.SaveSession(sampleSession()); err != nil {
		t.Fatal(err)
	}
	st, err := m.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if st.Ref != "granola-cli/default" || !st.Keys[KeyClient] || !st.Keys[KeyToken] || !st.HasRefreshToken || st.TokenExpiry == nil {
		t.Fatalf("status = %+v", st)
	}
}

func TestClearRemovesEveryKey(t *testing.T) {
	m, s := useMemoryStore(t)
	if err := m.SaveSession(sampleSession()); err != nil {
		t.Fatal(err)
	}
	removed, err := m.Clear()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	if !reflect.DeepEqual(removed, []string{KeyClient, KeyToken}) {
		t.Fatalf("removed = %v", removed)
	}
	if keys, _ := s.ListBundle(Profile); len(keys) != 0 {
		t.Fatalf("keys left after clear: %v", keys)
	}
}

func TestMalformedValueFailsClosed(t *testing.T) {
	m, s := useMemoryStore(t)
	if _, err := s.SetBundle(Profile, map[string]string{KeyClient: "{not json", KeyToken: "{}"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Load(); err == nil || errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("malformed client should fail with a decode error, got %v", err)
	}
}
