package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "archive.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func eventData(id string, at time.Time) EventData {
	return EventData{
		Event:         Event{ID: id, At: at, ImportedAt: at, Title: id, Type: "daily", RawPath: "/raw/results.json"},
		Participation: Participation{EventID: id, Name: "Driver", DRAdjustment: new(1)},
		Results: []SessionResult{
			{EventID: id, SessionType: "qualifying", SessionNo: "1", RaceNo: 1, Position: new(3)},
			{EventID: id, SessionType: "race", SessionNo: "1", RaceNo: 1, Position: new(2), CompletedLaps: new(10)},
		},
	}
}
func record(t *testing.T, s *Store, p Player, at time.Time, elo float64) {
	t.Helper()
	if err := s.RecordRating(context.Background(), p, RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: elo, SRRating: elo / 100, DRRank: "Silver", SRRank: "Gold"}); err != nil {
		t.Fatal(err)
	}
}

func TestLatestEventsAndAsOfReference(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	self := Player{SteamID: "self", Name: "Driver"}
	events := []EventData{eventData("old", base.Add(-time.Hour)), eventData("a", base), eventData("b", base)}
	qualifyingOnly := eventData("later-qualifying", base.Add(time.Hour))
	qualifyingOnly.Results = qualifyingOnly.Results[:1]
	events = append(events, qualifyingOnly)
	if err := s.ImportEvents(ctx, self, events); err != nil {
		t.Fatal(err)
	}
	record(t, s, self, base.Add(-time.Minute), 100)
	record(t, s, self, base, 110)
	record(t, s, self, base, 120)                  // same timestamp: later observation wins
	record(t, s, self, base.Add(time.Minute), 999) // never use a future observation
	races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 2 || races[0].Event.ID != "b" || races[1].Event.ID != "a" {
		t.Fatalf("wrong latest events: %+v", races)
	}
	for _, r := range races {
		if len(r.Results) != 2 {
			t.Fatalf("event was limited by session rows: %+v", r)
		}
		if r.ReferenceRating == nil || r.ReferenceRating.DRElo != 120 || !r.ReferenceRating.ObservedAt.Equal(base) {
			t.Fatalf("wrong as-of reference: %+v", r.ReferenceRating)
		}
		if r.Results[1].GridPosition != nil || r.Results[1].BestLapUS != nil {
			t.Fatal("missing values became zero")
		}
	}
	all, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[2].ReferenceRating != nil {
		t.Fatalf("expected no observation before oldest event: %+v", all)
	}
	// A late historical import must change the reference without rewriting events.
	record(t, s, self, base.Add(-time.Hour), 70)
	all, err = s.LatestRaces(ctx, "self", RaceOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if all[2].ReferenceRating == nil || all[2].ReferenceRating.DRElo != 70 {
		t.Fatal("reference was cached on the event")
	}
	// Strictly prior observations are used when no equal-time observation exists.
	earlier := eventData("between", base.Add(-30*time.Second))
	if err := s.ImportEvents(ctx, self, []EventData{earlier}); err != nil {
		t.Fatal(err)
	}
	all, err = s.LatestRaces(ctx, "self", RaceOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if all[2].Event.ID != "between" || all[2].ReferenceRating.DRElo != 100 {
		t.Fatal("latest prior observation not selected")
	}
}

func TestAtomicReimportReplacesTrackedSessions(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	self := Player{SteamID: "self", Name: "Old", RaceOSPlayerID: "persistent-id"}
	original := eventData("event", at)
	if err := s.ImportEvents(ctx, self, []EventData{original}); err != nil {
		t.Fatal(err)
	}
	updated := eventData("event", at)
	updated.Results = updated.Results[1:]
	updated.Results[0].Position = new(1)
	for range 2 {
		if err := s.ImportEvents(ctx, Player{SteamID: "self"}, []EventData{updated}); err != nil {
			t.Fatal(err)
		}
	}
	races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 1 || len(races[0].Results) != 1 || *races[0].Results[0].Position != 1 {
		t.Fatal("reimport did not replace the tracked result")
	}
	if rows := countRows(t, s, `SELECT count(*) FROM session_results WHERE event_id='event'`); rows != 1 {
		t.Fatalf("reimport duplicated tracked sessions: %d", rows)
	}
	var name, id string
	if err := s.db.QueryRow(`SELECT display_name,raceos_player_id FROM players WHERE steam_id='self'`).Scan(&name, &id); err != nil {
		t.Fatal(err)
	}
	if name != "Old" || id != "persistent-id" {
		t.Fatal("empty import metadata erased identity")
	}
	// A valid replacement followed by an invalid event must roll back both.
	invalid := eventData("invalid", at)
	invalid.Results[1].Position = new(-1)
	if err := s.ImportEvents(ctx, self, []EventData{original, invalid}); err == nil {
		t.Fatal("invalid position accepted")
	}
	races, err = s.LatestRaces(ctx, "self", RaceOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 1 || len(races[0].Results) != 1 || *races[0].Results[0].Position != 1 {
		t.Fatal("failed import partially mutated archive")
	}
	missing := eventData("missing", at)
	missing.Results = nil
	if err := s.ImportEvents(ctx, self, []EventData{missing}); err == nil {
		t.Fatal("event without own results accepted")
	}
	if _, err := s.db.Exec(`INSERT INTO event_participations(event_id,player_id,display_name,car_class,joker_played) VALUES('absent',1,'','',0)`); err == nil {
		t.Fatal("foreign key violation accepted")
	}
}

func TestObservationRangeAndValidation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	player := Player{SteamID: "self"}
	record(t, s, player, at.Add(-time.Microsecond), 1)
	record(t, s, player, at, 2)
	record(t, s, player, at, 3)
	record(t, s, player, at.Add(time.Microsecond), 4)
	values, err := s.Ratings(ctx, "self", at, at.Add(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].DRElo != 2 || values[1].DRElo != 3 {
		t.Fatalf("incorrect inclusive/exclusive bounds or tie order: %+v", values)
	}
	fractional := at.Add(time.Nanosecond)
	values, err = s.Ratings(ctx, "self", at, fractional)
	if err != nil || len(values) != 2 {
		t.Fatalf("fractional exclusive end excluded earlier observations: %+v %v", values, err)
	}
	values, err = s.Ratings(ctx, "self", fractional, at.Add(time.Microsecond))
	if err != nil || len(values) != 0 {
		t.Fatalf("fractional inclusive start included earlier observations: %+v %v", values, err)
	}
	empty, err := s.Ratings(ctx, "absent", at, at.Add(time.Hour))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("missing account: %+v %v", empty, err)
	}
	if _, err := s.Ratings(ctx, "self", at, at); err == nil {
		t.Fatal("empty time interval accepted")
	}
	if _, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 0}); err == nil {
		t.Fatal("nonpositive limit accepted")
	}
}
