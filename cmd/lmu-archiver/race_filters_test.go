package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/storage"
)

func cliRace(id string, at time.Time, track, eventType, class string) storage.EventData {
	return storage.EventData{
		Event:         storage.Event{ID: id, At: at, ImportedAt: at, Title: id, Type: eventType, Track: track, RawPath: "/raw/results.json"},
		Participation: storage.Participation{EventID: id, Name: "Driver", CarClass: class},
		Results:       []storage.SessionResult{{EventID: id, SessionType: "race", SessionNo: "1", RaceNo: 1}},
	}
}

func TestRacesCommandFilters(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	db := filepath.Join(dir, "archive.sqlite3")
	writer, err := storage.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := writer.ImportEvents(ctx, storage.Player{SteamID: "self", Name: "Driver"}, []storage.EventData{
		cliRace("e1", base, "Monza", "daily", "Hypercar"),
		cliRace("e2", base.AddDate(0, 0, 1), "Spa", "specialevent", "Hypercar"),
		cliRace("e3", base.AddDate(0, 0, 2), "Monza", "daily", "LMP2"),
	}); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	runRaces := func(t *testing.T, flags ...string) (int, []string, string) {
		t.Helper()
		args := append([]string{"races", "--db", db, "--json"}, flags...)
		var stdout, stderr bytes.Buffer
		code := run(ctx, args, &stdout, &stderr)
		if code != 0 {
			return code, nil, stderr.String()
		}
		var races []storage.Race
		if err := json.Unmarshal(stdout.Bytes(), &races); err != nil {
			t.Fatalf("decode %q: %v", stdout.String(), err)
		}
		ids := make([]string, len(races))
		for i, race := range races {
			ids[i] = race.Event.ID
		}
		return code, ids, stderr.String()
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
		name  string
		flags []string
		want  []string
	}{
		{"no-filters", nil, []string{"e3", "e2", "e1"}},
		{"dates-and-track", []string{"--from", "2026-01-02", "--to", "2026-01-04", "--track", "Monza"}, []string{"e3"}},
		{"combined-filters", []string{"--track", "Monza", "--class", "Hypercar", "--event-type", "daily"}, []string{"e1"}},
		{"unknown-event-type", []string{"--event-type", "NoSuchType"}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, ids, stderr := runRaces(t, tc.flags...)
			if code != 0 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			equal(t, ids, tc.want)
		})
	}

	for _, flags := range [][]string{
		{"--from", "not-a-date"},
		{"--to", "not-a-date"},
		{"--from", "2026-01-03", "--to", "2026-01-02"},
		{"--from", "2026-01-02", "--to", "2026-01-02"},
	} {
		if code, _, stderr := runRaces(t, flags...); code != 2 || stderr == "" {
			t.Fatalf("%v: exit %d stderr %q", flags, code, stderr)
		}
	}
}
