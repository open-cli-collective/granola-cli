package mcpclient

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPacedTransportCapsRequestsPerRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	tr := newPacedTransport(nil, nil, 1000, 1000)
	tr.max = 3
	client := &http.Client{Transport: tr}
	for i := 0; i < 3; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
	}
	if _, err := client.Get(srv.URL); !errors.Is(err, ErrRequestCap) {
		t.Fatalf("fourth request: got %v, want ErrRequestCap", err)
	}
}

func TestPacedTransportPaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	client := &http.Client{Transport: newPacedTransport(nil, nil, 20, 1)}
	start := time.Now()
	for i := 0; i < 3; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	// Burst 1 at 20/s: the second and third requests each wait ~50ms.
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Fatalf("three requests took %s; pacing is not applied", elapsed)
	}
}

func TestVerboseLogRedactsCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	var log bytes.Buffer
	client := &http.Client{Transport: newPacedTransport(nil, &log, 1000, 1000)}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/cb?code=secret-code-123&state=s", nil)
	req.Header.Set("Authorization", "Bearer secret-token-456")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	out := log.String()
	for _, secret := range []string{"secret-code-123", "secret-token-456"} {
		if strings.Contains(out, secret) {
			t.Fatalf("verbose log leaked %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, "Authorization: Bearer <redacted, len=") {
		t.Fatalf("log does not show the redacted header: %s", out)
	}
}

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error { b.closed = true; return nil }

func TestPacedTransportClosesBodyOnEarlyFailure(t *testing.T) {
	tr := newPacedTransport(nil, nil, 1000, 1000)
	tr.max = 0
	body := &trackingBody{Reader: strings.NewReader("{}")}
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/", body)
	if _, err := tr.RoundTrip(req); !errors.Is(err, ErrRequestCap) {
		t.Fatalf("got %v, want ErrRequestCap", err)
	}
	if !body.closed {
		t.Fatal("request body was not closed on the request-cap path")
	}
}
