package mcpclient

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Client-side request pacing. Granola documents an average MCP rate limit of
// about 100 requests per minute across all tools; one CLI invocation makes a
// handful of HTTP requests (initialize, the initialized notification, the
// tool call, session close), so pacing at one request per second with a small
// burst keeps a single run far below that and a tight loop of runs near it.
const (
	requestsPerSecond = 1
	requestBurst      = 4
	// maxRequestsPerRun bounds one invocation. No command needs more than a
	// few requests; hitting the cap means something is looping.
	maxRequestsPerRun = 30
)

// ErrRequestCap is returned once an invocation has made maxRequestsPerRun
// HTTP requests.
var ErrRequestCap = errors.New("request cap reached for this invocation; granola makes at most a few requests per command")

// pacedTransport rate-limits and counts outbound requests, and optionally
// logs each request line to Log with credentials redacted. It never retries.
type pacedTransport struct {
	base    http.RoundTripper
	limiter *rate.Limiter
	max     int
	Log     io.Writer

	mu    sync.Mutex
	count int
}

func newPacedTransport(base http.RoundTripper, log io.Writer, perSecond float64, burst int) *pacedTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &pacedTransport{
		base:    base,
		limiter: rate.NewLimiter(rate.Limit(perSecond), burst),
		max:     maxRequestsPerRun,
		Log:     log,
	}
}

func (t *pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if t.count >= t.max {
		t.mu.Unlock()
		return closeAndFail(req, ErrRequestCap)
	}
	t.count++
	t.mu.Unlock()

	if err := t.limiter.Wait(req.Context()); err != nil {
		return closeAndFail(req, err)
	}
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	if t.Log != nil {
		status := "error"
		if resp != nil {
			status = resp.Status
		}
		fmt.Fprintf(t.Log, "http: %s %s -> %s (%s) %s\n", req.Method, redactURL(req), status,
			time.Since(start).Round(time.Millisecond), redactAuth(req.Header.Get("Authorization")))
	}
	return resp, err
}

// closeAndFail honors the RoundTripper contract of closing the request body
// on every path, including errors returned before the base transport runs.
func closeAndFail(req *http.Request, err error) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, err
}

// redactURL drops the query string, which carries OAuth codes and state on
// some requests.
func redactURL(req *http.Request) string {
	u := *req.URL
	if u.RawQuery != "" {
		u.RawQuery = "<redacted>"
	}
	return u.String()
}

// redactAuth reduces an Authorization header to its scheme and length.
func redactAuth(v string) string {
	if v == "" {
		return ""
	}
	scheme, _, _ := strings.Cut(v, " ")
	return fmt.Sprintf("Authorization: %s <redacted, len=%d>", scheme, len(v))
}

// NewHTTPClient returns the paced HTTP client used for every request this
// tool makes, including OAuth discovery and token calls. It sets no client
// timeout because MCP responses can stream; callers bound each command with a
// context deadline instead.
func NewHTTPClient(log io.Writer) *http.Client {
	return NewHTTPClientWithRate(log, requestsPerSecond, requestBurst)
}

// NewHTTPClientWithRate is NewHTTPClient with explicit pacing; tests use it to
// run against a local server without waiting. The per-run request cap still
// applies.
func NewHTTPClientWithRate(log io.Writer, perSecond float64, burst int) *http.Client {
	return &http.Client{Transport: newPacedTransport(nil, log, perSecond, burst)}
}
