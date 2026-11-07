package storage

import (
	"context"
	"testing"
	"time"
)

func filterEvent(id string, at time.Time, track, eventType, class, sessionClass string) EventData {
	return EventData{
		Event:         Event{ID: id, At: at, ImportedAt: at, Title: id, Type: eventType, Track: track, RawPath: "/raw/results.json"},
		Participation: Participation{EventID: id, Name: "Driver", CarClass: class, DRAdjustment: new(1)},
		Results: []SessionResult{
			{EventID: id, SessionType: "qualifying", SessionNo: "1", RaceNo: 1, Position: new(3), CarClass: sessionClass},
			{EventID: id, SessionType: "race", SessionNo: "1", RaceNo: 1, Position: new(2), CompletedLaps: new(10), CarClass: sessionClass},
		},
	}
}

func TestRaceFilters(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	self := Player{SteamID: "self", Name: "Driver"}
	events := []EventData{
		filterEvent("old", base, "Monza", "daily", "Hypercar", "LMP2"),
		filterEvent("spa", base.AddDate(0, 0, 1), "Spa", "daily", "LMP2", "LMP2"),
		filterEvent("special", base.AddDate(0, 0, 2), "Monza", "specialevent", "Hypercar", "Hypercar"),
		filterEvent("case", base.AddDate(0, 0, 3), "Monza", "Daily", "Hypercar", "Hypercar"),
		filterEvent("shared", base.AddDate(0, 0, 4), "Monza", "daily", "Hypercar", "Hypercar"),
		filterEvent("pct", base.AddDate(0, 0, 5), "Monza%", "daily", "Hypercar", "Hypercar"),
	}
	qualifyingOnly := filterEvent("qual", base.AddDate(0, 0, 6), "Monza", "daily", "Hypercar", "Hypercar")
	qualifyingOnly.Results = qualifyingOnly.Results[:1]
	events = append(events, qualifyingOnly)
	if err := s.ImportEvents(ctx, self, events); err != nil {
		t.Fatal(err)
	}
	// An observation before the queried period still supplies the reference rating.
	record(t, s, self, base.AddDate(0, 0, -1), 50)

	day := func(d int) *time.Time {
		value := time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC)
		return &value
	}
	stamp := func(text string) *time.Time {
		value, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			t.Fatal(err)
		}
		return &value
	}
	ids := func(t *testing.T, options RaceOptions) []string {
		t.Helper()
		if options.Limit == 0 {
			options.Limit = 10
		}
		races, err := s.LatestRaces(ctx, "self", options)
		if err != nil {
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
		name    string
		options RaceOptions
		want    []string
	}{
		{"unfiltered", RaceOptions{}, []string{"pct", "shared", "case", "special", "spa", "old"}},
		{"from-inclusive", RaceOptions{From: day(2)}, []string{"pct", "shared", "case", "special", "spa"}},
		{"to-exclusive", RaceOptions{To: day(4)}, []string{"special", "spa", "old"}},
		{"both-bounds", RaceOptions{From: day(2), To: day(3)}, []string{"spa"}},
		{"exact-from", RaceOptions{From: stamp("2026-01-02T12:00:00Z"), To: day(3)}, []string{"spa"}},
		{"exact-to", RaceOptions{From: day(2), To: stamp("2026-01-02T12:00:00Z")}, []string{}},
		{"to-only", RaceOptions{To: day(3)}, []string{"spa", "old"}},
		{"from-only", RaceOptions{From: day(5)}, []string{"pct", "shared"}},
		{"rfc3339-offset-from", RaceOptions{From: stamp("2026-01-02T14:00:00+02:00"), To: day(3)}, []string{"spa"}},
		{"rfc3339-offset-to", RaceOptions{From: day(2), To: stamp("2026-01-02T14:00:00+02:00")}, []string{}},
		{"submicro-from", RaceOptions{From: stamp("2026-01-02T12:00:00.000000001Z"), To: day(3)}, []string{}},
		{"submicro-to", RaceOptions{From: day(2), To: stamp("2026-01-02T12:00:00.000000001Z")}, []string{"spa"}},
		{"track", RaceOptions{Track: "Monza"}, []string{"shared", "case", "special", "old"}},
		{"track-case-sensitive", RaceOptions{Track: "monza"}, []string{}},
		{"track-wildcard-literal", RaceOptions{Track: "%"}, []string{}},
		{"track-percent-literal", RaceOptions{Track: "Monza%"}, []string{"pct"}},
		{"class-participation", RaceOptions{Class: "Hypercar"}, []string{"pct", "shared", "case", "special", "old"}},
		{"class-not-session", RaceOptions{Class: "LMP2"}, []string{"spa"}},
		{"class-case-sensitive", RaceOptions{Class: "hypercar"}, []string{}},
		{"event-type", RaceOptions{EventType: "daily"}, []string{"pct", "shared", "spa", "old"}},
		{"event-type-case-sensitive", RaceOptions{EventType: "Daily"}, []string{"case"}},
		{"event-type-other", RaceOptions{EventType: "specialevent"}, []string{"special"}},
		{"event-type-unknown", RaceOptions{EventType: "unknown"}, []string{}},
		{"combined", RaceOptions{From: day(1), To: day(6), Track: "Monza", Class: "Hypercar", EventType: "daily"}, []string{"shared", "old"}},
		{"combined-narrow", RaceOptions{From: day(3), To: day(4), Track: "Monza", Class: "Hypercar", EventType: "specialevent"}, []string{"special"}},
	} {
		t.Run(tc.name, func(t *testing.T) { equal(t, ids(t, tc.options), tc.want) })
	}

	// Qualifying-only events never participate in the race history.
	equal(t, ids(t, RaceOptions{Track: "Monza", Class: "Hypercar", EventType: "daily"}), []string{"shared", "old"})

	// The reference observation predates the filter period; filtering only
	// bounds selected events.
	races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10, From: day(5), To: day(6)})
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 1 || races[0].Event.ID != "shared" || races[0].ReferenceRating == nil || races[0].ReferenceRating.DRElo != 50 {
		t.Fatalf("reference rating was bounded by the event filter: %+v", races)
	}
	if !races[0].ReferenceRating.ObservedAt.Equal(base.AddDate(0, 0, -1)) {
		t.Fatalf("wrong observation: %+v", races[0].ReferenceRating)
	}

	// Direct callers cannot supply an empty or reversed period.
	empty := day(3)
	if _, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10, From: empty, To: empty}); err == nil {
		t.Fatal("empty period accepted")
	}
	if _, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10, From: day(4), To: day(3)}); err == nil {
		t.Fatal("reversed period accepted")
	}
}
