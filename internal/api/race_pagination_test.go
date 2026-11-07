package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/query"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func TestRacesOffsetPaginationAPI(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "archive.sqlite3")
	writer, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var events []storage.EventData
	for _, id := range []string{"a", "b", "c"} {
		events = append(events, storage.EventData{
			Event:         storage.Event{ID: id, At: at, ImportedAt: at, Title: id, Type: "daily", RawPath: "/raw/results.json"},
			Participation: storage.Participation{EventID: id, Name: "Driver"},
			Results: []storage.SessionResult{
				{EventID: id, SessionType: "qualifying", SessionNo: "1", RaceNo: 1, Position: new(3)},
				{EventID: id, SessionType: "race", SessionNo: "1", RaceNo: 1, Position: new(1), CompletedLaps: new(10)},
			},
		})
	}
	if err := writer.ImportEvents(ctx, storage.Player{SteamID: "self", Name: "Driver"}, events); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	reader, err := storage.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	handler := NewHandler(query.Service{Store: reader})

	for _, tc := range []struct {
		target string
		want   []string
	}{
		{"/api/v1/races?limit=1&offset=1", []string{"b"}}, // same timestamp: event_id DESC
		{"/api/v1/races?limit=1&offset=2", []string{"a"}},
		{"/api/v1/races?offset=1", []string{"b", "a"}}, // missing limit keeps the 10 event default
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.target, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", tc.target, response.Code, response.Body.String())
		}
		var races []storage.Race
		if err := json.Unmarshal(response.Body.Bytes(), &races); err != nil {
			t.Fatalf("%s: not a JSON array: %v", tc.target, err)
		}
		if len(races) != len(tc.want) {
			t.Fatalf("%s: %+v", tc.target, races)
		}
		for i, id := range tc.want {
			if races[i].Event.ID != id {
				t.Fatalf("%s: %+v", tc.target, races)
			}
			if len(races[i].Results) != 2 {
				t.Fatalf("%s: session rows were limited by offset: %+v", tc.target, races[i].Results)
			}
		}
	}

	// Missing offset is the default newest page.
	defaultPage := httptest.NewRecorder()
	handler.ServeHTTP(defaultPage, httptest.NewRequest(http.MethodGet, "/api/v1/races", nil))
	if defaultPage.Code != http.StatusOK {
		t.Fatalf("default query: %d %s", defaultPage.Code, defaultPage.Body.String())
	}
	var all []storage.Race
	if err := json.Unmarshal(defaultPage.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Event.ID != "c" || all[2].Event.ID != "a" {
		t.Fatalf("default offset changed the page: %+v", all)
	}

	// An offset beyond the archive is an empty JSON array with HTTP 200.
	beyond := httptest.NewRecorder()
	handler.ServeHTTP(beyond, httptest.NewRequest(http.MethodGet, "/api/v1/races?offset=99", nil))
	if beyond.Code != http.StatusOK || strings.TrimSpace(beyond.Body.String()) != "[]" {
		t.Fatalf("beyond-end offset: %d %s", beyond.Code, beyond.Body.String())
	}

	// Negative, fractional, textual, and overflowing offsets are input errors.
	for _, offset := range []string{"-1", "1.5", "abc", "999999999999999999999"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/races?offset="+offset, nil))
		var body struct {
			Error string `json:"error"`
		}
		if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Error == "" {
			t.Fatalf("offset %q: %d %s", offset, response.Code, response.Body.String())
		}
	}
}
