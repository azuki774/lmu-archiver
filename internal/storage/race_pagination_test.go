package storage

import (
	"context"
	"testing"
	"time"
)

// TestLatestRacesOffsetPagination uses a real archive: offsets must page the
// event selection only, never the joined session rows.
func TestLatestRacesOffsetPagination(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	self := Player{SteamID: "self", Name: "Driver"}
	// Same timestamp: ordering falls back to event_id DESC. Ascending insertion
	// order proves the page order cannot come from row insertion.
	if err := s.ImportEvents(ctx, self, []EventData{
		eventData("a", base), eventData("b", base), eventData("c", base),
		eventData("old", base.Add(-time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	// An event without the owner's race session must not consume a page slot.
	qualifyingOnly := eventData("qualifying-only", base.Add(2*time.Hour))
	qualifyingOnly.Results = qualifyingOnly.Results[:1]
	if err := s.ImportEvents(ctx, self, []EventData{qualifyingOnly}); err != nil {
		t.Fatal(err)
	}

	ordered := []string{"c", "b", "a", "old"}
	for offset := range ordered {
		races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 1, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		if len(races) != 1 || races[0].Event.ID != ordered[offset] {
			t.Fatalf("offset %d returned %+v", offset, races)
		}
		// A page limits events, not the sessions within an event.
		if len(races[0].Results) != 2 {
			t.Fatalf("offset %d dropped sessions: %+v", offset, races[0].Results)
		}
	}

	// Exact pages concatenate to the full ordered list.
	var concatenated []string
	for offset := 0; offset < len(ordered); offset += 2 {
		races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 2, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		for _, race := range races {
			concatenated = append(concatenated, race.Event.ID)
		}
	}
	if len(concatenated) != len(ordered) {
		t.Fatalf("pages did not concatenate: %v", concatenated)
	}
	for i := range ordered {
		if concatenated[i] != ordered[i] {
			t.Fatalf("pages out of order: %v", concatenated)
		}
	}

	// The last page is partial rather than padded.
	last, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 1 || last[0].Event.ID != "old" {
		t.Fatalf("wrong final partial page: %+v", last)
	}
	// Offsets at or beyond the event count are empty, not nil and not an error.
	for _, offset := range []int{4, 1000} {
		races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 2, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		if races == nil || len(races) != 0 {
			t.Fatalf("offset %d was not an empty page: %+v", offset, races)
		}
	}

	// Direct callers keep the existing limit validation and reject a negative offset.
	if _, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10, Offset: -1}); err == nil {
		t.Fatal("negative offset accepted")
	}
	if _, err := s.LatestRaces(ctx, "self", RaceOptions{Offset: 1}); err == nil {
		t.Fatal("nonpositive limit accepted")
	}
	if _, err := s.LatestRaces(ctx, "", RaceOptions{Limit: 10}); err == nil {
		t.Fatal("missing Steam ID accepted")
	}
}

func TestRaceOffsetAfterFilters(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	events := []EventData{
		filterEvent("old", from, "Track", "daily", "GT3", "GT3"),
		filterEvent("a", from.Add(time.Hour), "Track", "daily", "GT3", "GT3"),
		filterEvent("b", from.Add(time.Hour), "Track", "daily", "GT3", "GT3"),
		filterEvent("wrong-track", from.Add(2*time.Hour), "Other", "daily", "GT3", "GT3"),
		filterEvent("wrong-class", from.Add(3*time.Hour), "Track", "daily", "Hypercar", "Hypercar"),
		filterEvent("wrong-type", from.Add(4*time.Hour), "Track", "specialevent", "GT3", "GT3"),
		filterEvent("before-from", from.Add(-time.Hour), "Track", "daily", "GT3", "GT3"),
		filterEvent("at-to", to, "Track", "daily", "GT3", "GT3"),
	}
	if err := s.ImportEvents(ctx, Player{SteamID: "self"}, events); err != nil {
		t.Fatal(err)
	}
	for offset, id := range []string{"b", "a", "old"} {
		races, err := s.LatestRaces(ctx, "self", RaceOptions{
			From: &from, To: &to, Track: "Track", Class: "GT3", EventType: "daily",
			Limit: 1, Offset: offset,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(races) != 1 || races[0].Event.ID != id {
			t.Fatalf("filtered offset %d: %+v, want %s", offset, races, id)
		}
	}
	races, err := s.LatestRaces(ctx, "self", RaceOptions{
		From: &from, To: &to, Track: "Track", Class: "GT3", EventType: "daily",
		Limit: 1, Offset: 3,
	})
	if err != nil || len(races) != 0 {
		t.Fatalf("filtered end offset: %+v %v", races, err)
	}
}
