package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/query"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func TestReadAPIPlayerSelectionAndObservationBoundaries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "archive.sqlite3")
	writer, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		at    time.Time
		value float64
	}{{at.Add(-time.Microsecond), 1}, {at, 2}, {at.AddDate(0, 0, 1), 3}} {
		if err := writer.RecordRating(ctx, storage.Player{SteamID: "self"}, storage.RatingSnapshot{ObservedAt: row.at, Source: "authenticate", DRElo: row.value}); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()
	reader, err := storage.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	handler := NewHandler(query.Service{Store: reader})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ratings?from=2026-01-01&to=2026-01-02", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("query failed: %d %s", response.Code, response.Body.String())
	}
	var ratings []storage.RatingSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &ratings); err != nil {
		t.Fatal(err)
	}
	if len(ratings) != 1 || ratings[0].DRElo != 2 || !ratings[0].ObservedAt.Equal(at) {
		t.Fatalf("date bounds changed: %+v", ratings)
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/v1/races?limit=0", 400},
		{"GET", "/api/v1/races?limit=bad", 400},
		{"GET", "/api/v1/ratings?from=2026-01-02&to=2026-01-01", 400},
		{"GET", "/api/v1/ratings?from=not-a-date", 400},
		{"POST", "/api/v1/races", 405},
		{"GET", "/unknown", 404},
	} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest(tc.method, tc.path, nil))
		var body struct {
			Error string `json:"error"`
		}
		if r.Code != tc.status || json.Unmarshal(r.Body.Bytes(), &body) != nil || body.Error == "" {
			t.Fatalf("invalid error contract %s: %d %s", tc.path, r.Code, r.Body.String())
		}
	}
	// Another tracked account makes implicit player selection ambiguous.
	writer, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.RecordRating(ctx, storage.Player{SteamID: "other"}, storage.RatingSnapshot{ObservedAt: at, Source: "authenticate"}); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	ambiguous := httptest.NewRecorder()
	handler.ServeHTTP(ambiguous, httptest.NewRequest("GET", "/api/v1/ratings", nil))
	if ambiguous.Code != 400 {
		t.Fatalf("ambiguous player silently selected: %d", ambiguous.Code)
	}
	explicit := httptest.NewRecorder()
	handler.ServeHTTP(explicit, httptest.NewRequest("GET", "/api/v1/ratings?steam-id=self&from=2026-01-01&to=2026-01-02", nil))
	if explicit.Code != 200 {
		t.Fatalf("explicit player rejected: %d", explicit.Code)
	}
}
