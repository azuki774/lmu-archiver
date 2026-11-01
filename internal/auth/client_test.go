package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/azuki774/lmu-archiver/internal/httpclient"
)

func TestInvalidTicketStopsBeforeBackend(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"authSessionTicket":""}`, `{"authSessionTicket":"  "}`, `{"authSessionTicket":42}`, `{"authSessionTicket":"secret"} {}`} {
		t.Run(body, func(t *testing.T) {
			lmu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, body)
			}))
			defer lmu.Close()
			var calls atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
			}))
			defer backend.Close()
			client, err := NewClient(lmu.URL, backend.URL, httpclient.New())
			if err != nil {
				t.Fatal(err)
			}
			session, err := client.Authenticate(context.Background())
			if err == nil || session.AccessToken != "" {
				t.Fatalf("invalid ticket produced a session: error=%v", err)
			}
			if calls.Load() != 0 {
				t.Fatal("backend contacted after an invalid ticket")
			}
		})
	}
}

func TestAuthenticationFailureDoesNotExposeCredentials(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"rejected", http.StatusUnauthorized, `{"error":"private-ticket private-token"}`},
		{"malformed", http.StatusOK, `{"accessToken": "private-token"`},
		{"missing", http.StatusOK, `{"player":{}}`},
		{"blank", http.StatusOK, `{"accessToken":"  "}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lmu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"authSessionTicket":"private-ticket"}`)
			}))
			defer lmu.Close()
			var calls atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer backend.Close()
			client, err := NewClient(lmu.URL, backend.URL, httpclient.New())
			if err != nil {
				t.Fatal(err)
			}
			session, err := client.Authenticate(context.Background())
			if err == nil || session.AccessToken != "" {
				t.Fatalf("failed authentication produced a session: error=%v", err)
			}
			if strings.Contains(err.Error(), "private-") {
				t.Fatal("credentials leaked in error")
			}
			if calls.Load() != 1 {
				t.Fatalf("authentication was retried: %d requests", calls.Load())
			}
		})
	}
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	for _, stage := range []string{"ticket", "authentication"} {
		t.Run(stage, func(t *testing.T) {
			var leaks atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				leaks.Add(1)
			}))
			defer destination.Close()
			redirect := func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
			}
			lmu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stage == "ticket" {
					redirect(w, r)
					return
				}
				fmt.Fprint(w, `{"authSessionTicket":"private-ticket"}`)
			}))
			defer lmu.Close()
			backend := httptest.NewServer(http.HandlerFunc(redirect))
			defer backend.Close()
			client, err := NewClient(lmu.URL, backend.URL, httpclient.New())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Authenticate(context.Background())
			if err == nil || !strings.Contains(err.Error(), "HTTP 307") {
				t.Fatalf("redirect not rejected: %v", err)
			}
			if leaks.Load() != 0 {
				t.Fatal("redirect destination received a request")
			}
		})
	}
}

func TestCancelInFlightTicketRequest(t *testing.T) {
	started := make(chan struct{})
	lmu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer lmu.Close()
	client, err := NewClient(lmu.URL, "https://raceos.gg", httpclient.New())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Authenticate(ctx)
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not propagated: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type cancelAfterReadBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelAfterReadBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.cancel()
	}
	return n, err
}

func TestCancelWhileDecodingTicketResponse(t *testing.T) {
	for _, body := range []string{`{"authSessionTicket":"`, `{"authSessionTicket":"private-ticket"}`} {
		t.Run(body, func(t *testing.T) {
			lmu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, body)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer lmu.Close()
			client, err := NewClient(lmu.URL, "https://raceos.gg", httpclient.New())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := client.http.Transport
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				resp, err := transport.RoundTrip(r)
				if err == nil {
					// Cancel only after Do returned and decoding read body bytes.
					resp.Body = &cancelAfterReadBody{resp.Body, cancel}
				}
				return resp, err
			})
			_, err = client.Authenticate(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("body-read cancellation was not propagated: %v", err)
			}
		})
	}
}
