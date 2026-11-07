package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type archiveCounts struct {
	players, events, participations, results, ratings, captures int
}

func countRows(t *testing.T, s *Store, query string) int {
	t.Helper()
	var count int
	if err := s.db.QueryRow(query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func countArchive(t *testing.T, s *Store) archiveCounts {
	t.Helper()
	return archiveCounts{
		players:        countRows(t, s, `SELECT count(*) FROM players`),
		events:         countRows(t, s, `SELECT count(*) FROM events`),
		participations: countRows(t, s, `SELECT count(*) FROM event_participations`),
		results:        countRows(t, s, `SELECT count(*) FROM session_results`),
		ratings:        countRows(t, s, `SELECT count(*) FROM rating_snapshots`),
		captures:       countRows(t, s, `SELECT count(*) FROM capture_observations`),
	}
}

func TestFirstImportClaimsArchiveForOneAccount(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if _, err := s.ResolveSteamID(ctx); !errors.Is(err, ErrPlayerRequired) {
		t.Fatalf("empty archive resolved: %v", err)
	}
	// An empty payload still claims the archive for its first account.
	first := Player{SteamID: "self", Name: "First", RaceOSPlayerID: "raceos-1"}
	if err := s.ImportEvents(ctx, first, nil); err != nil {
		t.Fatal(err)
	}
	id, err := s.ResolveSteamID(ctx)
	if err != nil || id != "self" {
		t.Fatalf("resolve after claim: %q %v", id, err)
	}
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := s.ImportEvents(ctx, first, []EventData{eventData("event", at)}); err != nil {
		t.Fatal(err)
	}
	// The same account may update metadata and replace its sessions.
	updated := eventData("event", at)
	updated.Event.Title = "updated"
	updated.Results = updated.Results[1:]
	updated.Results[0].Position = new(1)
	reimport := Player{SteamID: "self", Name: "Renamed", RaceOSPlayerID: "raceos-2"}
	for range 2 {
		if err := s.ImportEvents(ctx, reimport, []EventData{updated}); err != nil {
			t.Fatal(err)
		}
	}
	var name, raceos string
	if err := s.db.QueryRow(`SELECT display_name,raceos_player_id FROM players`).Scan(&name, &raceos); err != nil {
		t.Fatal(err)
	}
	if name != "Renamed" || raceos != "raceos-2" {
		t.Fatalf("same-account metadata not updated: name=%q raceos=%q", name, raceos)
	}
	races, err := s.LatestRaces(ctx, "self", RaceOptions{Limit: 10})
	if err != nil || len(races) != 1 || races[0].Event.Title != "updated" || len(races[0].Results) != 1 || *races[0].Results[0].Position != 1 {
		t.Fatalf("same-account reimport: %+v %v", races, err)
	}
	if rows := countRows(t, s, `SELECT count(*) FROM players`); rows != 1 {
		t.Fatalf("claim created %d players", rows)
	}
	if rows := countRows(t, s, `SELECT count(*) FROM session_results WHERE event_id='event'`); rows != 1 {
		t.Fatalf("reimport duplicated tracked sessions: %d", rows)
	}
}

func TestForeignAccountWritesLeaveArchiveUntouched(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	owner := Player{SteamID: "owner", Name: "Owner", RaceOSPlayerID: "raceos-owner"}
	if err := s.ImportEvents(ctx, owner, []EventData{eventData("owned", at)}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRating(ctx, owner, RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 1500}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportCapture(ctx, owner, nil, "owner-capture", &RatingSnapshot{ObservedAt: at.Add(time.Hour), Source: "authenticate", DRElo: 1550}); err != nil {
		t.Fatal(err)
	}
	before := countArchive(t, s)
	if before != (archiveCounts{players: 1, events: 1, participations: 1, results: 2, ratings: 2, captures: 1}) {
		t.Fatalf("unexpected starting archive: %+v", before)
	}
	foreign := Player{SteamID: "intruder", Name: "Intruder", RaceOSPlayerID: "raceos-intruder"}
	foreignRating := RatingSnapshot{ObservedAt: at.Add(2 * time.Hour), Source: "authenticate", DRElo: 9999}
	overwrite := eventData("owned", at)
	overwrite.Event.Title = "hijacked"
	attempts := []struct {
		name  string
		write func() error
	}{
		{"events without payload", func() error { return s.ImportEvents(ctx, foreign, nil) }},
		{"events with a new event", func() error { return s.ImportEvents(ctx, foreign, []EventData{eventData("foreign", at)}) }},
		{"events overwriting the owner's event", func() error { return s.ImportEvents(ctx, foreign, []EventData{overwrite}) }},
		{"rating", func() error { return s.RecordRating(ctx, foreign, foreignRating) }},
		{"capture with rating only", func() error {
			_, err := s.ImportCapture(ctx, foreign, nil, "foreign-capture", &foreignRating)
			return err
		}},
		{"capture without rating", func() error {
			_, err := s.ImportCapture(ctx, foreign, nil, "foreign-null-capture", nil)
			return err
		}},
		{"capture reusing the owner's marker", func() error {
			_, err := s.ImportCapture(ctx, foreign, []EventData{eventData("foreign", at)}, "owner-capture", &foreignRating)
			return err
		}},
	}
	for _, attempt := range attempts {
		err := attempt.write()
		if !errors.Is(err, ErrPlayerMismatch) {
			t.Errorf("%s: want ErrPlayerMismatch, got %v", attempt.name, err)
		}
		if after := countArchive(t, s); after != before {
			t.Errorf("%s: archive changed: before=%+v after=%+v", attempt.name, before, after)
		}
	}
	// Login inputs without an account are rejected before the owner check and
	// must also leave every table alone.
	if err := s.ImportEvents(ctx, Player{}, nil); err == nil || errors.Is(err, ErrPlayerMismatch) {
		t.Fatalf("blank account accepted as an owner mismatch: %v", err)
	}
	if after := countArchive(t, s); after != before {
		t.Fatalf("blank account changed archive: %+v", after)
	}
	var steamID, name, raceos string
	if err := s.db.QueryRow(`SELECT steam_id,display_name,raceos_player_id FROM players`).Scan(&steamID, &name, &raceos); err != nil {
		t.Fatal(err)
	}
	if steamID != "owner" || name != "Owner" || raceos != "raceos-owner" {
		t.Fatalf("player metadata changed: %q %q %q", steamID, name, raceos)
	}
	var title string
	if err := s.db.QueryRow(`SELECT title FROM events WHERE event_id='owned'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "owned" {
		t.Fatalf("owner event overwritten by a rejected import: %q", title)
	}
	if rows := countRows(t, s, `SELECT count(*) FROM capture_observations WHERE capture_id='owner-capture' AND player_id=(SELECT player_id FROM players)`); rows != 1 {
		t.Fatalf("owner capture marker changed: %d", rows)
	}
	if rows := countRows(t, s, `SELECT count(*) FROM session_results WHERE player_id=(SELECT player_id FROM players)`); rows != 2 {
		t.Fatalf("owner results changed: %d", rows)
	}
	// A rejected foreign capture consumes neither its marker nor its observation,
	// so the owner can still use that capture ID.
	saved, err := s.ImportCapture(ctx, owner, nil, "foreign-capture", &foreignRating)
	if err != nil || !saved {
		t.Fatalf("owner could not reuse the foreign capture ID: %t %v", saved, err)
	}
	if after := countArchive(t, s); after != (archiveCounts{players: 1, events: 1, participations: 1, results: 2, ratings: 3, captures: 2}) {
		t.Fatalf("owner capture after foreign attempts: %+v", after)
	}
}

func TestConcurrentFirstImportsCreateSingleOwner(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	ids := []string{"alpha", "beta"}
	errs := make([]error, len(ids))
	var wait sync.WaitGroup
	for i, id := range ids {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs[i] = s.ImportEvents(ctx, Player{SteamID: id}, nil)
		}()
	}
	wait.Wait()
	claimed := 0
	for _, err := range errs {
		if err == nil {
			claimed++
			continue
		}
		if !errors.Is(err, ErrPlayerMismatch) {
			t.Fatalf("losing concurrent import: %v", err)
		}
	}
	owner, err := s.ResolveSteamID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed != 1 || owner == "" {
		t.Fatalf("claimed=%d owner=%q", claimed, owner)
	}
	if rows := countRows(t, s, `SELECT count(*) FROM players`); rows != 1 {
		t.Fatalf("concurrent imports created %d owners", rows)
	}
}

func TestSeparateArchivesOwnDifferentAccounts(t *testing.T) {
	ctx := context.Background()
	first, second := testStore(t), testStore(t)
	if err := first.ImportEvents(ctx, Player{SteamID: "alpha", Name: "Alpha"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := second.ImportEvents(ctx, Player{SteamID: "beta", Name: "Beta"}, nil); err != nil {
		t.Fatal(err)
	}
	if id, err := first.ResolveSteamID(ctx); err != nil || id != "alpha" {
		t.Fatalf("first archive owner: %q %v", id, err)
	}
	if id, err := second.ResolveSteamID(ctx); err != nil || id != "beta" {
		t.Fatalf("second archive owner: %q %v", id, err)
	}
	if err := first.ImportEvents(ctx, Player{SteamID: "beta"}, nil); !errors.Is(err, ErrPlayerMismatch) {
		t.Fatalf("cross-archive import: %v", err)
	}
	if id, err := first.ResolveSteamID(ctx); err != nil || id != "alpha" {
		t.Fatalf("rejected import changed first archive owner: %q %v", id, err)
	}
	if rows := countRows(t, second, `SELECT count(*) FROM players`); rows != 1 {
		t.Fatalf("rejected import changed the second archive: %d players", rows)
	}
}

func TestLegacyMultiPlayerArchiveRejectsReadsAndWrites(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	owner := Player{SteamID: "owner", Name: "Owner"}
	if err := s.ImportEvents(ctx, owner, []EventData{eventData("owned", at)}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRating(ctx, owner, RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 1500}); err != nil {
		t.Fatal(err)
	}
	// Simulate an archive written by an older multi-account version.
	if _, err := s.db.Exec(`INSERT INTO players(steam_id,raceos_player_id,display_name) VALUES('legacy','','Legacy')`); err != nil {
		t.Fatal(err)
	}
	before := countArchive(t, s)
	if before.players != 2 {
		t.Fatalf("legacy archive setup: %+v", before)
	}
	if _, err := s.ResolveSteamID(ctx); !errors.Is(err, ErrMultiplePlayers) {
		t.Fatalf("legacy archive resolved: %v", err)
	}
	rating := RatingSnapshot{ObservedAt: at.Add(time.Hour), Source: "authenticate", DRElo: 1600}
	attempts := []struct {
		name  string
		write func() error
	}{
		{"owner events", func() error { return s.ImportEvents(ctx, owner, nil) }},
		{"legacy events", func() error { return s.ImportEvents(ctx, Player{SteamID: "legacy"}, nil) }},
		{"owner rating", func() error { return s.RecordRating(ctx, owner, rating) }},
		{"owner capture", func() error {
			_, err := s.ImportCapture(ctx, owner, nil, "legacy-capture", &rating)
			return err
		}},
	}
	for _, attempt := range attempts {
		err := attempt.write()
		if !errors.Is(err, ErrMultiplePlayers) {
			t.Errorf("%s: want ErrMultiplePlayers, got %v", attempt.name, err)
		}
		if after := countArchive(t, s); after != before {
			t.Errorf("%s: legacy archive changed: before=%+v after=%+v", attempt.name, before, after)
		}
	}
	if rows := countRows(t, s, `SELECT count(*) FROM players`); rows != 2 {
		t.Fatalf("legacy players changed: %d", rows)
	}
	if rows := countRows(t, s, `SELECT count(*) FROM rating_snapshots WHERE dr_elo=1500`); rows != 1 {
		t.Fatalf("legacy observation changed: %d", rows)
	}
}
