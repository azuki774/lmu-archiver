// Package httpclient configures the shared, rate-limited API connection.
package httpclient

import (
	"net/http"
	"time"
)

// New creates a direct HTTP client with a one-second cooldown after each
// transport attempt. Share this client across all API clients in the process.
// The 20-second timeout includes waiting for permission to send.
func New() *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	return &http.Client{
		Transport: &limitedTransport{base: base, gate: make(chan struct{}, 1)},
		Timeout:   20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type limitedTransport struct {
	base http.RoundTripper
	gate chan struct{}
	// Protected by gate; measured with the monotonic clock.
	next time.Time
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// RoundTripper must close the body even when cancellation prevents dispatch.
	forwarded := false
	defer func() {
		if !forwarded && req.Body != nil {
			req.Body.Close()
		}
	}()
	ctx := req.Context()
	select {
	case t.gate <- struct{}{}:
		defer func() { <-t.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if delay := time.Until(t.next); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	forwarded = true
	resp, err := t.base.RoundTrip(req)
	// Keep the gate through response headers (or failure), not body consumption.
	t.next = time.Now().Add(time.Second)
	return resp, err
}

func (t *limitedTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
