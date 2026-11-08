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

func TestTrackStatsAndFiltersAPI(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "archive.sqlite3")
	writer, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	lap := func(seconds int64) *int64 { return new(seconds * 1_000_000) }
	if err := writer.ImportEvents(ctx, storage.Player{SteamID: "self", Name: "Driver"}, []storage.EventData{
		{
			Event:         storage.Event{ID: "a", At: base, ImportedAt: base, Title: "a", Type: "daily", Track: "Monza", RawPath: "/raw/results.json"},
			Participation: storage.Participation{EventID: "a", Name: "Driver", CarClass: "GT3"},
			Results: []storage.SessionResult{
				{EventID: "a", SessionType: "race", SessionNo: "1", RaceNo: 1, Position: new(1), ClassPosition: new(1), BestLapUS: lap(90), CompletedLaps: new(10)},
				{EventID: "a", SessionType: "race", SessionNo: "1", RaceNo: 2, Position: new(3), ClassPosition: new(2), BestLapUS: lap(89), CompletedLaps: new(10)},
			},
		},
		{
			Event:         storage.Event{ID: "b", At: base.AddDate(0, 0, 1), ImportedAt: base, Title: "b", Type: "specialevent", Track: "Spa", RawPath: "/raw/results.json"},
			Participation: storage.Participation{EventID: "b", Name: "Driver", CarClass: "LMP2"},
			Results: []storage.SessionResult{
				{EventID: "b", SessionType: "race", SessionNo: "1", RaceNo: 1, Position: new(2), BestLapUS: lap(120), CompletedLaps: new(10)},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	reader, err := storage.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	handler := NewHandler(query.Service{Store: reader})
	request := func(t *testing.T, target string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		return recorder
	}

	stats := func(t *testing.T, target string) []storage.TrackStats {
		t.Helper()
		recorder := request(t, target)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, recorder.Code, recorder.Body.String())
		}
		var values []storage.TrackStats
		if err := json.Unmarshal(recorder.Body.Bytes(), &values); err != nil {
			t.Fatal(err)
		}
		return values
	}

	rows := stats(t, "/api/v1/stats/tracks")
	if len(rows) != 2 || rows[0].Track != "Monza" || rows[1].Track != "Spa" {
		t.Fatalf("track rows changed: %+v", rows)
	}
	monza := rows[0]
	if monza.Races != 1 || monza.Starts != 2 || monza.Overall.Wins != 1 || monza.Overall.Podiums != 2 ||
		monza.Overall.AvgPosition == nil || *monza.Overall.AvgPosition != 2 || monza.BestLapUs == nil || *monza.BestLapUs != 89_000_000 {
		t.Fatalf("Monza aggregation changed: %+v", monza)
	}
	if monza.Class.Wins != 1 || monza.Class.Podiums != 2 {
		t.Fatalf("Monza class aggregation changed: %+v", monza.Class)
	}
	// Filters match the race selection exactly.
	rows = stats(t, "/api/v1/stats/tracks?track=Spa&class=LMP2&event-type=specialevent&from=2026-01-01&to=2026-01-03")
	if len(rows) != 1 || rows[0].Track != "Spa" || rows[0].Starts != 1 {
		t.Fatalf("filtered aggregation changed: %+v", rows)
	}
	if got := stats(t, "/api/v1/stats/tracks?track=Unknown"); len(got) != 0 {
		t.Fatalf("unknown track returned rows: %+v", got)
	}

	filters := request(t, "/api/v1/filters")
	if filters.Code != http.StatusOK {
		t.Fatalf("filters failed: %d %s", filters.Code, filters.Body.String())
	}
	var values storage.RaceFilterValues
	if err := json.Unmarshal(filters.Body.Bytes(), &values); err != nil {
		t.Fatal(err)
	}
	if len(values.Tracks) != 2 || values.Tracks[0] != "Monza" || values.Tracks[1] != "Spa" ||
		len(values.Classes) != 2 || values.Classes[0] != "GT3" || values.Classes[1] != "LMP2" ||
		len(values.EventTypes) != 2 || values.EventTypes[0] != "daily" || values.EventTypes[1] != "specialevent" {
		t.Fatalf("filter values changed: %+v", values)
	}

	// Invalid input and unsupported methods keep the documented error contract.
	for _, tc := range []struct {
		method, target string
		status         int
	}{
		{http.MethodGet, "/api/v1/stats/tracks?from=not-a-date", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/stats/tracks?from=2026-01-02&to=2026-01-01", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/stats/tracks", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/filters", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/stats", http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.target, nil))
		var body struct {
			Error string `json:"error"`
		}
		if recorder.Code != tc.status || json.Unmarshal(recorder.Body.Bytes(), &body) != nil || body.Error == "" {
			t.Fatalf("invalid error contract %s: %d %s", tc.target, recorder.Code, recorder.Body.String())
		}
	}
}
