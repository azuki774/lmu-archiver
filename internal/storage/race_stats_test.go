package storage

import (
	"context"
	"testing"
	"time"
)

// statsEvent builds one event with explicit race session rows. Each race
// block supplies position, class position, and best lap in microseconds;
// nil values model DNF or missing data.
func statsEvent(id string, at time.Time, track, eventType, class string, races ...SessionResult) EventData {
	data := EventData{
		Event:         Event{ID: id, At: at, ImportedAt: at, Title: id, Type: eventType, Track: track, RawPath: "/raw/results.json"},
		Participation: Participation{EventID: id, Name: "Driver", CarClass: class, DRAdjustment: new(1)},
		Results: []SessionResult{
			{EventID: id, SessionType: "qualifying", SessionNo: "1", RaceNo: 1, Position: new(3), CarClass: class},
		},
	}
	for i := range races {
		race := races[i]
		race.EventID = id
		race.SessionType = "race"
		race.SessionNo = "1"
		race.RaceNo = i + 1
		data.Results = append(data.Results, race)
	}
	return data
}

func raceRow(position, classPosition *int, bestLapUs *int64) SessionResult {
	return SessionResult{Position: position, ClassPosition: classPosition, BestLapUS: bestLapUs, CompletedLaps: new(10)}
}

func TestTrackStatsAggregatesAllMatchingRaces(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	self := Player{SteamID: "self", Name: "Driver"}
	lap := func(seconds int64) *int64 { return new(seconds * 1_000_000) }
	events := []EventData{
		// Two race blocks in one event: one race, two starts.
		statsEvent("a", base, "Monza", "daily", "GT3",
			raceRow(new(1), new(1), lap(90)), raceRow(new(3), new(2), lap(89))),
		statsEvent("b", base.AddDate(0, 0, 1), "Monza", "daily", "GT3",
			raceRow(new(2), new(1), lap(91))),
		// A DNF keeps starts counted but never joins averages or best positions.
		statsEvent("c", base.AddDate(0, 0, 2), "Monza", "daily", "GT3",
			raceRow(nil, nil, nil)),
		statsEvent("d", base.AddDate(0, 0, 3), "Spa", "specialevent", "LMP2",
			raceRow(new(1), nil, lap(120))),
		// Qualifying-only events never join the aggregation.
		func() EventData {
			data := statsEvent("qual", base.AddDate(0, 0, 4), "Qual Track", "daily", "GT3")
			data.Results = data.Results[:1]
			return data
		}(),
	}
	if err := s.ImportEvents(ctx, self, events); err != nil {
		t.Fatal(err)
	}

	stats, err := s.TrackStats(ctx, "self", RaceOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("unexpected track rows: %+v", stats)
	}
	monza, spa := stats[0], stats[1]
	if monza.Track != "Monza" || spa.Track != "Spa" {
		t.Fatalf("rows are not ordered by race count then track: %+v", stats)
	}
	if monza.Races != 3 || monza.Starts != 4 {
		t.Fatalf("Monza races/starts changed: %+v", monza)
	}
	if monza.Overall.Wins != 1 || monza.Overall.Podiums != 3 || monza.Overall.AvgPosition == nil || *monza.Overall.AvgPosition != 2 || *monza.Overall.BestPosition != 1 {
		t.Fatalf("Monza overall stats changed: %+v", monza.Overall)
	}
	if monza.BestLapUs == nil || *monza.BestLapUs != 89_000_000 {
		t.Fatalf("Monza best lap is not the fastest race lap: %+v", monza.BestLapUs)
	}
	if monza.Class.Wins != 2 || monza.Class.Podiums != 3 || monza.Class.AvgPosition == nil || *monza.Class.AvgPosition != 4.0/3.0 || *monza.Class.BestPosition != 1 {
		t.Fatalf("Monza class stats changed: %+v", monza.Class)
	}
	if spa.Races != 1 || spa.Starts != 1 || spa.Overall.Wins != 1 || spa.Overall.Podiums != 1 {
		t.Fatalf("Spa overall stats changed: %+v", spa)
	}
	// Spa's single race has no class position: zero stays zero and averages stay null.
	if spa.Class.Wins != 0 || spa.Class.Podiums != 0 || spa.Class.AvgPosition != nil || spa.Class.BestPosition != nil {
		t.Fatalf("Spa class stats changed: %+v", spa.Class)
	}
}

func TestTrackStatsFiltersAndBoundaries(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	self := Player{SteamID: "self", Name: "Driver"}
	lap := func(seconds int64) *int64 { return new(seconds * 1_000_000) }
	events := []EventData{
		statsEvent("a", base, "Monza", "daily", "GT3", raceRow(new(1), new(1), lap(90))),
		statsEvent("b", base.AddDate(0, 0, 1), "Monza", "daily", "GT3", raceRow(new(2), new(1), lap(91))),
		statsEvent("c", base.AddDate(0, 0, 2), "Monza", "daily", "GT3", raceRow(nil, nil, nil)),
		statsEvent("d", base.AddDate(0, 0, 3), "Spa", "specialevent", "LMP2", raceRow(new(1), nil, lap(120))),
	}
	if err := s.ImportEvents(ctx, self, events); err != nil {
		t.Fatal(err)
	}
	day := func(d int) *time.Time {
		value := time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC)
		return &value
	}
	tracks := func(t *testing.T, options RaceOptions) []TrackStats {
		t.Helper()
		stats, err := s.TrackStats(ctx, "self", options)
		if err != nil {
			t.Fatal(err)
		}
		return stats
	}
	for _, tc := range []struct {
		name    string
		options RaceOptions
		want    string
		races   int
		starts  int
	}{
		{"track", RaceOptions{Track: "Monza"}, "Monza", 3, 3},
		{"class", RaceOptions{Class: "LMP2"}, "Spa", 1, 1},
		{"event-type", RaceOptions{EventType: "specialevent"}, "Spa", 1, 1},
		{"from-inclusive", RaceOptions{Track: "Monza", From: day(2)}, "Monza", 2, 2},
		{"to-exclusive", RaceOptions{Track: "Monza", To: day(3)}, "Monza", 2, 2},
		{"period", RaceOptions{From: day(2), To: day(3)}, "Monza", 1, 1},
		{"combined", RaceOptions{From: day(2), To: day(4), Track: "Monza", Class: "GT3", EventType: "daily"}, "Monza", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := tracks(t, tc.options)
			if len(stats) != 1 || stats[0].Track != tc.want || stats[0].Races != tc.races || stats[0].Starts != tc.starts {
				t.Fatalf("filter result changed: %+v", stats)
			}
		})
	}
	// The period selects event b only: one podium, no wins, and its own best lap.
	period := tracks(t, RaceOptions{From: day(2), To: day(3)})[0]
	if period.Overall.Wins != 0 || period.Overall.Podiums != 1 || period.Overall.AvgPosition == nil || *period.Overall.AvgPosition != 2 || period.BestLapUs == nil || *period.BestLapUs != 91_000_000 {
		t.Fatalf("period aggregation changed: %+v", period)
	}
	if got := tracks(t, RaceOptions{Track: "Unknown"}); len(got) != 0 {
		t.Fatalf("unknown track returned rows: %+v", got)
	}
	empty := day(3)
	if _, err := s.TrackStats(ctx, "self", RaceOptions{From: empty, To: empty}); err == nil {
		t.Fatal("empty period accepted")
	}
	if _, err := s.TrackStats(ctx, "self", RaceOptions{From: day(4), To: day(3)}); err == nil {
		t.Fatal("reversed period accepted")
	}
	if _, err := s.TrackStats(ctx, "", RaceOptions{}); err == nil {
		t.Fatal("missing Steam ID accepted")
	}
}
