package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/query"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func TestReadAPISingleAccountAndObservationBoundaries(t *testing.T) {
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
	// Importing a foreign account must fail without touching the archive.
	writer, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.RecordRating(ctx, storage.Player{SteamID: "other"}, storage.RatingSnapshot{ObservedAt: at, Source: "authenticate"}); !errors.Is(err, storage.ErrPlayerMismatch) {
		writer.Close()
		t.Fatalf("foreign import = %v, want ErrPlayerMismatch", err)
	}
	writer.Close()
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/ratings?from=2026-01-01&to=2026-01-02", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("single account query failed: %d %s", response.Code, response.Body.String())
	}
	ratings = nil
	if err := json.Unmarshal(response.Body.Bytes(), &ratings); err != nil {
		t.Fatal(err)
	}
	if len(ratings) != 1 || ratings[0].DRElo != 2 || !ratings[0].ObservedAt.Equal(at) {
		t.Fatalf("archive changed after a rejected foreign import: %+v", ratings)
	}
}

func TestRatingsLimitSelectsNewestChangesInChartOrder(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "archive.sqlite3")
	writer, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	type point struct {
		at     time.Time
		dr, sr float64
		drTier int
	}
	save := func(id string, p point) {
		t.Helper()
		if err := writer.RecordRating(ctx, storage.Player{SteamID: id}, storage.RatingSnapshot{
			ObservedAt: p.at, Source: "authenticate", DRElo: p.dr, SRRating: p.sr, DRTier: p.drTier,
		}); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var points []point
	for i := range 12 {
		p := point{at: base.Add(time.Duration(i) * time.Hour), dr: 1000 + float64(i), sr: 4, drTier: 1}
		save("self", p)
		points = append(points, p)
	}
	save("self", point{at: base.Add(12 * time.Hour), dr: 1011, sr: 4, drTier: 1}) // unchanged: not a point
	srChange := point{at: base.Add(13 * time.Hour), dr: 1011, sr: 4.25, drTier: 1}
	tierChange := point{at: srChange.at, dr: 1011, sr: 4.25, drTier: 2}
	save("self", srChange)
	save("self", tierChange)
	points = append(points, srChange, tierChange)
	late := point{at: base.Add(-time.Hour), dr: 500, sr: 3, drTier: 1}
	save("self", late) // imported last, but not the latest by observation time

	reader, err := storage.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	handler := NewHandler(query.Service{Store: reader})
	for _, tc := range []struct {
		path string
		want []point
	}{
		{"/api/v1/ratings?limit=1", []point{tierChange}},
		{"/api/v1/ratings?limit=3", points[len(points)-3:]},
		{"/api/v1/ratings?limit=20", append([]point{late}, points...)},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", tc.path, response.Code, response.Body.String())
		}
		var got []storage.RatingSnapshot
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %d observations, want %d", tc.path, len(got), len(tc.want))
		}
		for i, want := range tc.want {
			if !got[i].ObservedAt.Equal(want.at) || got[i].DRElo != want.dr || got[i].SRRating != want.sr || got[i].DRTier != want.drTier {
				t.Fatalf("%s: observation %d = %+v, want %+v", tc.path, i, got[i], want)
			}
		}
	}
	// A registered account with no observations has an empty, non-nil history.
	emptyPath := filepath.Join(t.TempDir(), "empty.sqlite3")
	emptyWriter, err := storage.Open(ctx, emptyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := emptyWriter.ImportEvents(ctx, storage.Player{SteamID: "empty"}, nil); err != nil {
		emptyWriter.Close()
		t.Fatal(err)
	}
	emptyWriter.Close()
	emptyReader, err := storage.OpenReadOnly(ctx, emptyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer emptyReader.Close()
	empty := httptest.NewRecorder()
	NewHandler(query.Service{Store: emptyReader}).ServeHTTP(
		empty, httptest.NewRequest(http.MethodGet, "/api/v1/ratings?limit=3", nil))
	var noRatings []storage.RatingSnapshot
	if empty.Code != http.StatusOK || json.Unmarshal(empty.Body.Bytes(), &noRatings) != nil || noRatings == nil || len(noRatings) != 0 {
		t.Fatalf("empty history: %d %s", empty.Code, empty.Body.String())
	}
	for _, parameters := range []string{
		"limit=0", "limit=-1", "limit=invalid", "limit=999999999999999999999",
		"limit=3&from=2026-01-01", "limit=3&to=2026-01-02",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ratings?"+parameters, nil))
		var body struct {
			Error string `json:"error"`
		}
		if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Error == "" {
			t.Fatalf("invalid parameters %q: %d %s", parameters, response.Code, response.Body.String())
		}
	}
}
