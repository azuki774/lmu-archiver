package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/mockapi"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func TestEndpointEnvironmentPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		config, environment, flags, empty bool
		wantCode                          int
	}{
		{name: "config without environment", config: true},
		{name: "environment without config", environment: true},
		{name: "environment overrides config", config: true, environment: true},
		{name: "flags override environment and config", config: true, environment: true, flags: true},
		{name: "empty environment does not fall back", config: true, environment: true, empty: true, wantCode: 2},
		{name: "flags override empty environment", config: true, environment: true, flags: true, empty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			for _, key := range []string{"LMU_INT_API", "RACEOS_URL"} {
				old, present := os.LookupEnv(key)
				t.Cleanup(func() {
					if present {
						_ = os.Setenv(key, old)
					} else {
						_ = os.Unsetenv(key)
					}
				})
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			dataDir := filepath.Join(dir, "mock")
			if err := os.CopyFS(dataDir, os.DirFS("../../testdata/mock-api")); err != nil {
				t.Fatal(err)
			}
			var ticketCalls, authCalls, wrongCalls atomic.Int32
			lmuHandler := mockapi.NewLMUHandler(dataDir)
			lmu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ticketCalls.Add(1)
				lmuHandler.ServeHTTP(w, r)
			}))
			defer lmu.Close()
			raceHandler := mockapi.NewRaceOSHandler(dataDir)
			raceOS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authCalls.Add(1)
				raceHandler.ServeHTTP(w, r)
			}))
			defer raceOS.Close()
			wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wrongCalls.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer wrong.Close()
			args := []string{"auth", "check"}
			if tc.config {
				cfg := config{LMUURL: lmu.URL, RaceOSURL: raceOS.URL}
				if tc.environment || tc.flags {
					cfg.LMUURL, cfg.RaceOSURL = wrong.URL, wrong.URL
				}
				body, err := json.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "config.json")
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--config", path)
			}
			if tc.environment {
				lmuURL, raceURL := lmu.URL, raceOS.URL
				if tc.flags {
					lmuURL, raceURL = wrong.URL, wrong.URL
				}
				if tc.empty {
					raceURL = ""
				}
				t.Setenv("LMU_INT_API", lmuURL)
				t.Setenv("RACEOS_URL", raceURL)
			}
			if tc.flags {
				args = append(args, "--lmu-url", lmu.URL, "--raceos-url", raceOS.URL)
			}
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), args, &stdout, &stderr); code != tc.wantCode {
				t.Fatalf("exit = %d, want %d: %s", code, tc.wantCode, stderr.String())
			}
			if wrongCalls.Load() != 0 {
				t.Fatal("contacted a lower-precedence endpoint")
			}
			wantCalls := int32(1)
			if tc.wantCode != 0 {
				wantCalls = 0
			}
			if ticketCalls.Load() != wantCalls || authCalls.Load() != wantCalls {
				t.Fatalf("ticket/auth calls = %d/%d, want %d each", ticketCalls.Load(), authCalls.Load(), wantCalls)
			}
		})
	}
}

func TestEnvironmentCannotSelectForeignUser(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("STEAM_ID", "foreign")
	ctx := context.Background()
	db := filepath.Join(dir, "archive.sqlite3")
	store, err := storage.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.RecordRating(ctx, storage.Player{SteamID: "self"}, storage.RatingSnapshot{ObservedAt: at, Source: "authenticate", DRElo: 1500}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"ratings", "--db", db, "--from", "2026-01-01", "--to", "2026-01-02", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr.String())
	}
	var ratings []storage.RatingSnapshot
	if err := json.Unmarshal(stdout.Bytes(), &ratings); err != nil {
		t.Fatal(err)
	}
	if len(ratings) != 1 || ratings[0].DRElo != 1500 {
		t.Fatalf("environment selected a foreign account: %+v", ratings)
	}
}

func TestConfigSteamIDIsRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"db":"archive.sqlite3","steamId":"foreign"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--config", configPath, "races"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want config rejection: %s", code, stderr.String())
	}
}
