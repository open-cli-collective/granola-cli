package mcpclient

import (
	"fmt"
	"sync"
	"testing"

	"golang.org/x/oauth2"

	"github.com/open-cli-collective/granola-cli/internal/credentials"
)

type countingSource struct {
	mu sync.Mutex
	n  int
}

func (c *countingSource) Token() (*oauth2.Token, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return &oauth2.Token{AccessToken: fmt.Sprintf("tok-%d", c.n)}, nil
}

func TestPersistingTokenSourceConcurrentUse(t *testing.T) {
	var saved []string
	p := &persistingTokenSource{
		base: &countingSource{},
		last: "tok-0",
		save: func(tok credentials.Token) error {
			saved = append(saved, tok.AccessToken)
			return nil
		},
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Token(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(saved) != 16 {
		t.Fatalf("saved %d tokens, want 16 (one per new access token)", len(saved))
	}
	if p.last != saved[len(saved)-1] {
		t.Fatalf("last = %q, want the most recently saved token %q", p.last, saved[len(saved)-1])
	}
}
