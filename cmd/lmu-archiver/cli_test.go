package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/storage"
	_ "modernc.org/sqlite"
)

func TestOfflineSyncConfigPrecedenceAndReadOnlyQuery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("LMU_INT_API", "invalid-offline-origin")
	t.Setenv("RACEOS_URL", "")
	input := filepath.Join(dir, "results.json")
	if err := os.WriteFile(input, []byte(`[{"eventId":"event","date":"2026-01-01T12:00:00Z","title":"Offline Race","drivers":[{"steamId":"self","name":"Driver"}],"race":{"sessionNo":"1","races":[{"results":[{"steamId":"self","position":"2"}]}]}}]`), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	configData, _ := json.Marshal(config{DB: filepath.Join(dir, "wrong.sqlite3"), LMUURL: "invalid", RaceOSURL: "invalid"})
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "archive.sqlite3")
	var stdout, stderr bytes.Buffer
	args := []string{"--config", configPath, "sync", "--skip-download", "--input", input, "--db", db, "--steam-id", "self", "--json"}
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("offline sync: %d %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "wrong.sqlite3")); !os.IsNotExist(err) {
		t.Fatal("config overrode an explicit DB flag")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"races", "--config", configPath, "--db", db, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("query: %d %s", code, stderr.String())
	}
	var races []storage.Race
	if err := json.Unmarshal(stdout.Bytes(), &races); err != nil {
		t.Fatal(err)
	}
	if len(races) != 1 || races[0].Event.ID != "event" || *races[0].Results[0].Position != 2 {
		t.Fatalf("wrong query result: %+v", races)
	}
	stdout.Reset()
	stderr.Reset()
	absent := filepath.Join(dir, "absent", "archive.sqlite3")
	if code := run(context.Background(), []string{"races", "--db", absent}, &stdout, &stderr); code == 0 {
		t.Fatal("missing archive was accepted")
	}
	if _, err := os.Stat(filepath.Dir(absent)); !os.IsNotExist(err) {
		t.Fatal("query created database directories")
	}
}

func TestSyncRejectsAmbiguousInputBeforeCreatingFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	for _, args := range [][]string{{"sync", "--skip-download"}, {"sync", "--input", "results.json"}} {
		db := filepath.Join(dir, "archive.sqlite3")
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), append(args, "--db", db), &stdout, &stderr); code != 2 {
			t.Fatalf("expected usage error, got %d", code)
		}
		if _, err := os.Stat(db); !os.IsNotExist(err) {
			t.Fatal("invalid flags created archive")
		}
	}
}

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRacesFailsWhenOutputCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	db := filepath.Join(dir, "archive.sqlite3")
	store, err := storage.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRating(context.Background(), storage.Player{SteamID: "self"}, storage.RatingSnapshot{ObservedAt: time.Now().UTC(), Source: "authenticate"}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			args := []string{"races", "--db", db}
			if format == "json" {
				args = append(args, "--json")
			}
			var stderr bytes.Buffer
			if code := run(context.Background(), args, failingOutput{}, &stderr); code != 1 {
				t.Fatalf("output loss was reported as exit %d", code)
			}
		})
	}
}

func TestReadCommandsRejectSteamIDFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	db := filepath.Join(dir, "archive.sqlite3")
	store, err := storage.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRating(context.Background(), storage.Player{SteamID: "self"}, storage.RatingSnapshot{ObservedAt: time.Now().UTC(), Source: "authenticate"}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	for _, args := range [][]string{
		{"races", "--db", db, "--steam-id", "self"},
		{"ratings", "--db", db, "--steam-id", "self"},
		{"serve", "--db", db, "--listen", "127.0.0.1:0", "--steam-id", "self"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Fatalf("%v: exit = %d, want usage error: %s", args, code, stderr.String())
		}
	}
}

func TestLegacyMultiUserArchiveFailsStartupAndReads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	db := filepath.Join(dir, "archive.sqlite3")
	store, err := storage.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	raw, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"self", "other"} {
		if _, err := raw.Exec(`INSERT INTO players(steam_id,display_name) VALUES(?,?)`, id, id); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	raw.Close()
	for _, args := range [][]string{
		{"races", "--db", db},
		{"ratings", "--db", db},
		{"serve", "--db", db, "--listen", "127.0.0.1:0"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 1 {
			t.Fatalf("%v: exit = %d, want failure: %s", args, code, stderr.String())
		}
	}
}

func TestEmptyArchiveFailsStartupAndReads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	db := filepath.Join(dir, "archive.sqlite3")
	store, err := storage.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	for _, args := range [][]string{
		{"races", "--db", db},
		{"ratings", "--db", db},
		{"serve", "--db", db, "--listen", "127.0.0.1:0"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 1 {
			t.Fatalf("%v: exit = %d, want failure: %s", args, code, stderr.String())
		}
	}
}
