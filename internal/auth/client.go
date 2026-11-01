// Package auth exchanges an LMU Steam session ticket for a RaceOS session.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const raceOSURL = "https://raceos.gg"

// Session contains credentials for subsequent requests in the same process.
// AccessToken is a secret: never log, print, or persist it.
type Session struct {
	AccessToken string `json:"-"`
}

// Client authenticates against RaceOS using a running LMU instance.
type Client struct {
	http      *http.Client
	lmuURL    string
	raceOSURL string
}

// NewClient configures a direct connection to the local LMU REST API.
// The base URL must be an HTTP origin, without credentials, path, query, or fragment.
func NewClient(lmuURL string) (*Client, error) {
	u, err := url.Parse(lmuURL)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("LMU URL must be an HTTP origin such as http://127.0.0.1:6937")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		http: &http.Client{
			Transport: transport,
			Timeout:   20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		lmuURL:    strings.TrimRight(lmuURL, "/"),
		raceOSURL: raceOSURL,
	}, nil
}

// Authenticate obtains a fresh ticket and exchanges it for an in-memory session.
// It performs no data retrieval, retries, credential caching, or token refresh.
func (c *Client) Authenticate(ctx context.Context) (Session, error) {
	var ticket struct {
		Value string `json:"authSessionTicket"`
	}
	if err := c.request(ctx, "LMU ticket", http.MethodGet,
		c.lmuURL+"/rest/profile/getAuthSessionTicket", nil, &ticket); err != nil {
		return Session{}, err
	}
	if strings.TrimSpace(ticket.Value) == "" {
		return Session{}, errors.New("LMU ticket: missing authSessionTicket")
	}

	// Keep the conservative spacing used by the existing PoC.
	timer := time.NewTimer(1200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Session{}, ctx.Err()
	case <-timer.C:
	}

	body, err := json.Marshal(struct {
		Token    string `json:"token"`
		Game     string `json:"game"`
		Platform string `json:"platform"`
	}{ticket.Value, "lmu", "steam"})
	if err != nil {
		return Session{}, errors.New("RaceOS authentication: cannot encode request")
	}
	var response struct {
		AccessToken string `json:"accessToken"`
	}
	if err := c.request(ctx, "RaceOS authentication", http.MethodPost,
		c.raceOSURL+"/authenticate", body, &response); err != nil {
		return Session{}, err
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return Session{}, errors.New("RaceOS authentication: missing accessToken")
	}
	return Session{AccessToken: response.AccessToken}, nil
}

func (c *Client) request(ctx context.Context, operation, method, endpoint string, body []byte, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: cannot create request", operation)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", operation, ctx.Err())
		}
		return fmt.Errorf("%s: request failed (connection, TLS, or timeout)", operation)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: HTTP %d", operation, resp.StatusCode)
	}
	// Neither decoder errors nor response bodies are safe to include in errors.
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(result); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", operation, ctx.Err())
		}
		return fmt.Errorf("%s: invalid JSON response", operation)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", operation, ctx.Err())
		}
		return fmt.Errorf("%s: invalid JSON response", operation)
	}
	return nil
}
