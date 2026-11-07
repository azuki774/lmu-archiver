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

func apiRace(id string, at time.Time, track, eventType, class string) storage.EventData {
	return storage.EventData{
		Event:         storage.Event{ID: id, At: at, ImportedAt: at, Title: id, Type: eventType, Track: track, RawPath: "/raw/results.json"},
		Participation: storage.Participation{EventID: id, Name: "Driver", CarClass: class},
		Results:       []storage.SessionResult{{EventID: id, SessionType: "race", SessionNo: "1", RaceNo: 1}},
	}
}

func TestRaceFilterAPISelection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "archive.sqlite3")
	writer, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := writer.ImportEvents(ctx, storage.Player{SteamID: "self", Name: "Driver"}, []storage.EventData{
		apiRace("e1", base, "Monza", "daily", "Hypercar"),
		apiRace("e2", base.AddDate(0, 0, 1), "Spa", "specialevent", "Hypercar"),
		apiRace("e3", base.AddDate(0, 0, 2), "Monza", "daily", "LMP2"),
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
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		return response
	}
	ids := func(t *testing.T, target string) []string {
		t.Helper()
		response := request(t, target)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, response.Code, response.Body.String())
		}
		var races []storage.Race
		if err := json.Unmarshal(response.Body.Bytes(), &races); err != nil {
			t.Fatal(err)
		}
		got := make([]string, len(races))
		for i, race := range races {
			got[i] = race.Event.ID
		}
		return got
	}
	equal := func(t *testing.T, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("ids = %v, want %v", got, want)
			}
		}
	}

	for _, tc := range []struct {
		target string
		want   []string
	}{
		{"/api/v1/races", []string{"e3", "e2", "e1"}},
		{"/api/v1/races?from=2026-01-02&to=2026-01-04&track=Monza", []string{"e3"}},
		{"/api/v1/races?track=Monza&class=Hypercar", []string{"e1"}},
		{"/api/v1/races?event-type=specialevent", []string{"e2"}},
		{"/api/v1/races?track=NoSuchTrack", []string{}},
		{"/api/v1/races?event-type=NoSuchType", []string{}},
	} {
		t.Run(tc.target, func(t *testing.T) { equal(t, ids(t, tc.target), tc.want) })
	}

	for _, target := range []string{
		"/api/v1/races?from=not-a-date",
		"/api/v1/races?to=not-a-date",
		"/api/v1/races?from=2026-01-03&to=2026-01-02",
		"/api/v1/races?from=2026-01-02&to=2026-01-02",
	} {
		response := request(t, target)
		var body struct {
			Error string `json:"error"`
		}
		if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Error == "" {
			t.Fatalf("%s: %d %s", target, response.Code, response.Body.String())
		}
	}
}
