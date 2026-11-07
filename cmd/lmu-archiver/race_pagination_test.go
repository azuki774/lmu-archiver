package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/storage"
)

// seedRacesDB archives three same-timestamp events; event_id DESC must order
// them c, b, a across offsets.
func seedRacesDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	db := filepath.Join(dir, "archive.sqlite3")
	store, err := storage.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
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
	if err := store.ImportEvents(context.Background(), storage.Player{SteamID: "self", Name: "Driver"}, events); err != nil {
		t.Fatal(err)
	}
	return db
}

func runRacesJSON(t *testing.T, args ...string) ([]storage.Race, int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), append([]string{"races"}, args...), &stdout, &stderr)
	var races []storage.Race
	if code == 0 {
		if err := json.Unmarshal(stdout.Bytes(), &races); err != nil {
			t.Fatalf("not a JSON array: %v: %s", err, stdout.String())
		}
	}
	return races, code, stderr.String()
}

func TestRacesOffsetCLI(t *testing.T) {
	db := seedRacesDB(t)
	races, code, stderr := runRacesJSON(t, "--db", db, "--json", "--limit", "1", "--offset", "1")
	if code != 0 {
		t.Fatalf("query: %d %s", code, stderr)
	}
	if len(races) != 1 || races[0].Event.ID != "b" {
		t.Fatalf("wrong offset page: %+v", races)
	}
	// Offset limits events, not the sessions within an event.
	if len(races[0].Results) != 2 {
		t.Fatalf("offset dropped sessions: %+v", races[0].Results)
	}
	// An offset beyond the archive is an empty array, not an error.
	races, code, stderr = runRacesJSON(t, "--db", db, "--json", "--offset", "100")
	if code != 0 || len(races) != 0 {
		t.Fatalf("beyond-end offset: %d %v %s", code, races, stderr)
	}
	// An empty offset keeps the existing newest-page default.
	races, code, stderr = runRacesJSON(t, "--db", db, "--json", "--offset=")
	if code != 0 || len(races) != 3 || races[0].Event.ID != "c" {
		t.Fatalf("empty offset changed the default: %d %+v %s", code, races, stderr)
	}
}

func TestRacesOffsetCLIRejectsInvalid(t *testing.T) {
	db := seedRacesDB(t)
	for _, offset := range []string{"-1", "1.5", "abc", "99999999999999999999"} {
		_, code, stderr := runRacesJSON(t, "--db", db, "--json", "--offset="+offset)
		if code != 2 || !strings.Contains(stderr, "offset") {
			t.Fatalf("offset %q: code %d stderr %q", offset, code, stderr)
		}
	}
	// Existing limit validation is unchanged.
	for _, limit := range []string{"0", "-1", "bad"} {
		_, code, stderr := runRacesJSON(t, "--db", db, "--json", "--limit="+limit)
		if code != 2 || !strings.Contains(stderr, "limit") {
			t.Fatalf("limit %q: code %d stderr %q", limit, code, stderr)
		}
	}
}
