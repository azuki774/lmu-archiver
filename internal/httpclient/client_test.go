package httpclient

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestConcurrentRequestsShareCooldownWithoutIdleBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := New()
		transport := client.Transport.(*limitedTransport)
		var starts []time.Time
		transport.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			starts = append(starts, time.Now())
			time.Sleep(250 * time.Millisecond)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
		})
		// Distinct consumers still share one transport and its cooldown.
		other := &http.Client{Transport: client.Transport}
		request := func(c *http.Client) {
			resp, err := c.Get("http://api.example/data")
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
		}
		begin := time.Now()
		var wg sync.WaitGroup
		for i := range 6 {
			wg.Go(func() {
				if i%2 == 0 {
					request(client)
				} else {
					request(other)
				}
			})
		}
		wg.Wait()
		if len(starts) != 6 {
			t.Fatalf("sent %d requests, want 6", len(starts))
		}
		if !starts[0].Equal(begin) {
			t.Fatalf("first request waited %s", starts[0].Sub(begin))
		}
		time.Sleep(10 * time.Second)
		idleEnd := time.Now()
		request(client)
		request(other)
		if len(starts) != 8 {
			t.Fatalf("sent %d requests, want 8", len(starts))
		}
		if !starts[6].Equal(idleEnd) {
			t.Fatal("first request after idle was delayed")
		}
		for i := 1; i < len(starts); i++ {
			if gap := starts[i].Sub(starts[i-1]); gap < 1250*time.Millisecond {
				t.Errorf("request %d started after %s; want headers plus one second", i, gap)
			}
		}
	})
}

func TestFailedAttemptConsumesCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := New()
		transport := client.Transport.(*limitedTransport)
		failure := errors.New("connection failed")
		var starts []time.Time
		transport.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			starts = append(starts, time.Now())
			return nil, failure
		})
		for range 2 {
			_, err := client.Get("http://api.example/data")
			if !errors.Is(err, failure) {
				t.Fatalf("unexpected failure: %v", err)
			}
		}
		if len(starts) != 2 {
			t.Fatalf("sent %d attempts, want 2", len(starts))
		}
		if gap := starts[1].Sub(starts[0]); gap != time.Second {
			t.Fatalf("failed attempts spaced %s apart, want one second", gap)
		}
	})
}

type trackedBody struct {
	*strings.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestCancellationDoesNotSendOrReserveSlot(t *testing.T) {
	for _, stage := range []string{"gate", "cooldown"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := New()
				transport := client.Transport.(*limitedTransport)
				release := make(chan struct{})
				var starts []time.Time
				transport.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
					starts = append(starts, time.Now())
					if len(starts) == 1 {
						<-release
					}
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
				})
				firstDone := make(chan error, 1)
				go func() {
					resp, err := client.Get("http://api.example/first")
					if err == nil {
						resp.Body.Close()
					}
					firstDone <- err
				}()
				synctest.Wait()
				if stage == "cooldown" {
					close(release)
					if err := <-firstDone; err != nil {
						t.Fatal(err)
					}
				}
				completed := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				body := &trackedBody{Reader: strings.NewReader("private-ticket")}
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://api.example/canceled", body)
				if err != nil {
					t.Fatal(err)
				}
				_, err = client.Do(req)
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("cancellation not propagated: %v", err)
				}
				if len(starts) != 1 || !body.closed {
					t.Fatalf("canceled request: sent=%d, body closed=%v", len(starts)-1, body.closed)
				}
				if stage == "gate" {
					close(release)
					if err := <-firstDone; err != nil {
						t.Fatal(err)
					}
					completed = time.Now()
				}
				resp, err := client.Get("http://api.example/next")
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if len(starts) != 2 {
					t.Fatalf("sent %d requests, want 2", len(starts))
				}
				if gap := starts[1].Sub(completed); gap != time.Second {
					t.Fatalf("next request delayed %s after completion; canceled request must not reserve a slot", gap)
				}
			})
		})
	}
}

func TestClientTimeoutIncludesCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := New()
		client.Timeout = 100 * time.Millisecond
		transport := client.Transport.(*limitedTransport)
		calls := 0
		transport.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
		})
		resp, err := client.Get("http://api.example/first")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		_, err = client.Get("http://api.example/timed-out")
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("waiting request escaped timeout: error=%v, sends=%d", err, calls)
		}
	})
}
