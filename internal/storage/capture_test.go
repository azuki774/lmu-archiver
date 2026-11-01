package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportCaptureRollsBackEventsAndObservationIdentity(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	player := Player{SteamID: "self"}
	rating := RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 1234}
	invalid := eventData("invalid", at)
	invalid.Results[0].Position = new(-1)
	if _, err := s.ImportCapture(ctx, player, []EventData{eventData("valid", at), invalid}, "capture", &rating); err == nil {
		t.Fatal("invalid capture accepted")
	}
	races, err := s.LatestRaces(ctx, "self", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 0 {
		t.Fatal("failed transaction retained an event")
	}
	saved, err := s.ImportCapture(ctx, player, []EventData{eventData("valid", at)}, "capture", &rating)
	if err != nil || !saved {
		t.Fatalf("failed transaction consumed observation identity: %t %v", saved, err)
	}
	saved, err = s.ImportCapture(ctx, player, []EventData{eventData("valid", at)}, "capture", &rating)
	if err != nil || saved {
		t.Fatalf("replay duplicated observation: %t %v", saved, err)
	}
	ratings, err := s.Ratings(ctx, "self", at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(ratings) != 1 || ratings[0].DRElo != 1234 {
		t.Fatalf("wrong observations: %+v", ratings)
	}
}

func TestReadOnlyDoesNotCreateMigrateOrWrite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	absent := filepath.Join(dir, "absent", "archive.sqlite3")
	if s, err := OpenReadOnly(ctx, absent); err == nil {
		s.Close()
		t.Fatal("missing database accepted")
	}
	if _, err := os.Stat(filepath.Dir(absent)); !os.IsNotExist(err) {
		t.Fatal("read-only open created parent directories")
	}
	path := filepath.Join(dir, "archive.sqlite3")
	writer, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()
	reader, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.RecordRating(ctx, Player{SteamID: "self"}, RatingSnapshot{ObservedAt: time.Now(), Source: "authenticate"}); err == nil {
		t.Fatal("read-only connection allowed writing")
	}
	reader.Close()
	// A v1 archive must be explicitly migrated, not modified by a reader.
	writer, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := writer.RecordRating(ctx, Player{SteamID: "self"}, RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 1456.25}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.db.Exec(`DROP TABLE capture_observations; DELETE FROM goose_db_version WHERE version_id=2`); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenReadOnly(ctx, path); err == nil {
		s.Close()
		t.Fatal("outdated schema silently migrated or accepted")
	}
	var version int
	if err := writer.db.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil || version != 1 {
		t.Fatalf("read-only open changed old schema: version=%d, err=%v", version, err)
	}
	writer.Close()
	writer, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ratings, err := writer.Ratings(ctx, "self", at, at.Add(time.Hour))
	if err != nil || len(ratings) != 1 || ratings[0].DRElo != 1456.25 {
		t.Fatalf("migration changed existing observations: %+v %v", ratings, err)
	}
}
