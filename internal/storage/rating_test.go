package storage

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestCaptureRecordsRatingTransitionsAndKeepsSkippedCaptureIdentity(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	player := Player{SteamID: "self"}
	rating := RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 1500, SRRating: 4.25}
	save := func(id string, want bool) {
		t.Helper()
		saved, err := s.ImportCapture(ctx, player, nil, id, &rating)
		if err != nil || saved != want {
			t.Fatalf("%s: recorded=%t want=%t err=%v", id, saved, want, err)
		}
	}
	save("initial", true)
	rating.ObservedAt = at.Add(time.Hour)
	rating.Source = "replay"
	rating.RawPath = "/different/profile.json"
	save("unchanged", false)
	// A later correction at the skipped capture's timestamp would make the
	// capture look changed on replay without its processed-identity marker.
	correction := rating
	correction.SRRating = 4.5
	if err := s.RecordRating(ctx, player, correction); err != nil {
		t.Fatal(err)
	}
	save("unchanged", false)
	rating.ObservedAt = at.Add(2 * time.Hour)
	save("returned-to-original", true)
	values, err := s.Ratings(ctx, "self", at, at.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 || values[0].SRRating != 4.25 || values[1].SRRating != 4.5 || values[2].SRRating != 4.25 || !values[2].ObservedAt.Equal(at.Add(2*time.Hour)) {
		t.Fatalf("lost A -> B -> A transition or retained unchanged observation: %+v", values)
	}
}

func TestEveryRatingValueParticipatesInChangeDetection(t *testing.T) {
	changes := []struct {
		name   string
		mutate func(*RatingSnapshot)
	}{
		{"DR ELO", func(r *RatingSnapshot) { r.DRElo = math.Nextafter(r.DRElo, math.Inf(1)) }},
		{"DR rank", func(r *RatingSnapshot) { r.DRRank = "Gold" }},
		{"DR tier", func(r *RatingSnapshot) { r.DRTier++ }},
		{"DR progress", func(r *RatingSnapshot) { r.DRProgress++ }},
		{"SR rating", func(r *RatingSnapshot) { r.SRRating++ }},
		{"SR rank", func(r *RatingSnapshot) { r.SRRank = "Platinum" }},
		{"SR tier", func(r *RatingSnapshot) { r.SRTier++ }},
		{"SR progress", func(r *RatingSnapshot) { r.SRProgress++ }},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			player := Player{SteamID: "self"}
			rating := RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 1500, DRRank: "Silver", DRTier: 1, DRProgress: 25, SRRating: 4.25, SRRank: "Gold", SRTier: 2, SRProgress: 80}
			if err := s.RecordRating(ctx, player, rating); err != nil {
				t.Fatal(err)
			}
			rating.ObservedAt = at.Add(time.Hour)
			change.mutate(&rating)
			saved, err := s.ImportCapture(ctx, player, nil, "changed", &rating)
			if err != nil || !saved {
				t.Fatalf("change to %s discarded: %t %v", change.name, saved, err)
			}
			values, err := s.Ratings(ctx, "self", at, at.Add(2*time.Hour))
			if err != nil || len(values) != 2 || !values[1].ObservedAt.Equal(rating.ObservedAt) {
				t.Fatalf("change not persisted: %+v %v", values, err)
			}
		})
	}
}

func TestHistoricalRatingComparesAgainstPrecedingObservation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	player := Player{SteamID: "self"}
	a := RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 100}
	if err := s.RecordRating(ctx, player, a); err != nil {
		t.Fatal(err)
	}
	b := a
	b.ObservedAt = at.Add(3 * time.Hour)
	b.DRElo = 200
	if err := s.RecordRating(ctx, player, b); err != nil {
		t.Fatal(err)
	}
	a.ObservedAt = at.Add(time.Hour)
	if saved, err := s.ImportCapture(ctx, player, nil, "historical-unchanged", &a); err != nil || saved {
		t.Fatalf("compared against future value: %t %v", saved, err)
	}
	b.ObservedAt = at.Add(2 * time.Hour)
	if saved, err := s.ImportCapture(ctx, player, nil, "historical-change", &b); err != nil || !saved {
		t.Fatalf("discarded change equal to future value: %t %v", saved, err)
	}
	values, err := s.Ratings(ctx, "self", at.Add(time.Hour), at.Add(3*time.Hour))
	if err != nil || len(values) != 1 || values[0].DRElo != 200 || !values[0].ObservedAt.Equal(b.ObservedAt) {
		t.Fatalf("wrong historical rating: %+v %v", values, err)
	}
}

func TestInvalidRatingRollsBackEventsAndCaptureIdentity(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rating := RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: math.NaN()}
	player := Player{SteamID: "self"}
	if _, err := s.ImportCapture(ctx, player, []EventData{eventData("event", at)}, "capture", &rating); err == nil {
		t.Fatal("nonfinite rating accepted")
	}
	races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10})
	if err != nil || len(races) != 0 {
		t.Fatalf("rating failure retained event: %+v %v", races, err)
	}
	rating.DRElo = 1500
	if saved, err := s.ImportCapture(ctx, player, []EventData{eventData("event", at)}, "capture", &rating); err != nil || !saved {
		t.Fatalf("rating failure consumed capture identity: %t %v", saved, err)
	}
}
