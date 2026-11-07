package gre

import (
	"context"
	"errors"
	"sync"
	"time"
)

// refreshMargin is how long before expiry a cached token stops being reused.
// SUNAT tokens last one hour (expires_in), so renewing ten minutes early
// avoids sending a token that expires in flight.
const refreshMargin = 10 * time.Minute

// tokenSource caches one bearer token and renews it before it expires.
// It is safe for concurrent use: concurrent callers that find the cache
// empty share a single token request.
type tokenSource struct {
	fetch func(ctx context.Context) (token string, ttl time.Duration, err error)
	now   func() time.Time

	mu        sync.Mutex // guards token and refreshAt, never held during I/O
	token     string
	refreshAt time.Time

	sem chan struct{} // capacity 1: held by the goroutine fetching a token
}

func newTokenSource(fetch func(context.Context) (string, time.Duration, error), now func() time.Time) *tokenSource {
	return &tokenSource{fetch: fetch, now: now, sem: make(chan struct{}, 1)}
}

// get returns a valid token. rejected is a token SUNAT answered 401 to: it is
// treated as invalid even if it has not expired yet. Pass "" otherwise.
func (s *tokenSource) get(ctx context.Context, rejected string) (string, error) {
	if tok := s.cached(rejected); tok != "" {
		return tok, nil
	}
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-s.sem }()
	// Another goroutine may have renewed the token while this one waited.
	if tok := s.cached(rejected); tok != "" {
		return tok, nil
	}
	tok, ttl, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}
	if tok == "" {
		return "", errors.New("gre: empty access token")
	}
	margin := refreshMargin
	if ttl < 2*margin {
		margin = ttl / 5
	}
	s.mu.Lock()
	s.token, s.refreshAt = tok, s.now().Add(ttl-margin)
	s.mu.Unlock()
	return tok, nil
}

func (s *tokenSource) cached(rejected string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == "" || s.token == rejected || !s.now().Before(s.refreshAt) {
		return ""
	}
	return s.token
}
