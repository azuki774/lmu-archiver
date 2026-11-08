package storage

import (
	"context"
	"testing"
	"time"
)

func TestRaceFilterValues(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	self := Player{SteamID: "self", Name: "Driver"}
	events := []EventData{
		statsEvent("a", base, "Monza", "daily", "GT3", raceRow(new(1), new(1), nil)),
		statsEvent("b", base.AddDate(0, 0, 1), "Spa", "specialevent", "LMP2", raceRow(new(2), nil, nil)),
		statsEvent("c", base.AddDate(0, 0, 2), "Monza%", "Daily", "GT3", raceRow(new(3), nil, nil)),
		// Qualifying-only events offer no filter values.
		func() EventData {
			data := statsEvent("qual", base.AddDate(0, 0, 3), "Qual Track", "weekly", "GTE", raceRow(new(1), nil, nil))
			data.Results = data.Results[:1]
			return data
		}(),
	}
	if err := s.ImportEvents(ctx, self, events); err != nil {
		t.Fatal(err)
	}
	values, err := s.RaceFilterValues(ctx, "self")
	if err != nil {
		t.Fatal(err)
	}
	want := RaceFilterValues{
		Tracks:     []string{"Monza", "Monza%", "Spa"},
		Classes:    []string{"GT3", "LMP2"},
		EventTypes: []string{"Daily", "daily", "specialevent"},
	}
	if len(values.Tracks) != len(want.Tracks) || len(values.Classes) != len(want.Classes) || len(values.EventTypes) != len(want.EventTypes) {
		t.Fatalf("filter values changed: %+v", values)
	}
	for i, track := range want.Tracks {
		if values.Tracks[i] != track {
			t.Fatalf("tracks changed: %+v", values.Tracks)
		}
	}
	for i, class := range want.Classes {
		if values.Classes[i] != class {
			t.Fatalf("classes changed: %+v", values.Classes)
		}
	}
	for i, eventType := range want.EventTypes {
		if values.EventTypes[i] != eventType {
			t.Fatalf("event types changed: %+v", values.EventTypes)
		}
	}
	if _, err := s.RaceFilterValues(ctx, ""); err == nil {
		t.Fatal("missing Steam ID accepted")
	}
}

func TestRaceFilterValuesEmptyArchive(t *testing.T) {
	s := testStore(t)
	values, err := s.RaceFilterValues(context.Background(), "self")
	if err != nil {
		t.Fatal(err)
	}
	// Empty slices, never null, so filter controls can render options directly.
	if values.Tracks == nil || values.Classes == nil || values.EventTypes == nil ||
		len(values.Tracks) != 0 || len(values.Classes) != 0 || len(values.EventTypes) != 0 {
		t.Fatalf("empty archive returned values: %+v", values)
	}
}
