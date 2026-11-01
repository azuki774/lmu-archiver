package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/mockapi"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func TestOnlineCaptureAndOfflineReplay(t *testing.T) {
	ctx := context.Background()
	lmu := httptest.NewServer(mockapi.NewLMUHandler())
	defer lmu.Close()
	mock := mockapi.NewRaceOSHandler()
	raceOS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/authenticate" {
			recorder := httptest.NewRecorder()
			mock.ServeHTTP(recorder, r)
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Error(err)
				return
			}
			response["player"].(map[string]any)["token"] = "private-nested-secret"
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		mock.ServeHTTP(w, r)
	}))
	defer raceOS.Close()
	dir := t.TempDir()
	db := filepath.Join(dir, "archive.sqlite3")
	result, err := Sync(ctx, SyncOptions{DBPath: db, RawDir: filepath.Join(dir, "raw"), LMUURL: lmu.URL, RaceOSURL: raceOS.URL})
	if err != nil {
		t.Fatal(err)
	}
	if result.Events != 1 || !result.RatingRecorded || result.SteamID != "mock-steam-id" {
		t.Fatalf("unexpected sync: %+v", result)
	}
	raw, err := os.ReadFile(filepath.Join(result.CapturePath, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "accessToken") {
		t.Fatal("saved profile exposed credentials")
	}
	info, err := os.Stat(filepath.Join(result.CapturePath, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("saved profile is not private")
	}
	original := readRatings(t, db)
	if len(original) != 1 || original[0].DRElo != 1500.25 || original[0].SRRating != 4.25 {
		t.Fatalf("wrong exact ratings: %+v", original)
	}
	// Invalid origins prove offline replay does not even validate an API client.
	replay := SyncOptions{DBPath: db, Input: result.CapturePath, SkipDownload: true, LMUURL: "invalid", RaceOSURL: "invalid"}
	for range 2 {
		repeated, err := Sync(ctx, replay)
		if err != nil {
			t.Fatal(err)
		}
		if repeated.RatingRecorded {
			t.Fatal("replay fabricated a new rating observation")
		}
	}
	ratings := readRatings(t, db)
	if len(ratings) != 1 || !ratings[0].ObservedAt.Equal(original[0].ObservedAt) {
		t.Fatalf("replay changed observation: %+v", ratings)
	}
	store, err := storage.OpenReadOnly(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	races, err := store.LatestRaces(ctx, result.SteamID, 10)
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 1 || *races[0].Results[0].Position != 2 || *races[0].Results[0].BestLapUS != 99_835_000 {
		t.Fatalf("wrong archived own results: %+v", races)
	}
	// A portable capture can rebuild another database without backdating to replay.
	moved := filepath.Join(dir, "moved")
	if err := os.CopyFS(moved, os.DirFS(result.CapturePath)); err != nil {
		t.Fatal(err)
	}
	replay.Input = moved
	replay.DBPath = filepath.Join(dir, "rebuilt.sqlite3")
	if _, err := Sync(ctx, replay); err != nil {
		t.Fatal(err)
	}
	rebuilt := readRatings(t, replay.DBPath)
	if len(rebuilt) != 1 || !rebuilt[0].ObservedAt.Equal(original[0].ObservedAt) || rebuilt[0].RawPath != filepath.Join(moved, "profile.json") {
		t.Fatalf("capture is not portable: %+v", rebuilt)
	}
}

func readRatings(t *testing.T, path string) []storage.RatingSnapshot {
	t.Helper()
	store, err := storage.OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ratings, err := store.Ratings(context.Background(), "mock-steam-id", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return ratings
}

func TestFailedDownloadOrInvalidLaterEventDoesNotImport(t *testing.T) {
	for _, mode := range []string{"invalid later event", "repeated page", "rate limited"} {
		t.Run(mode, func(t *testing.T) {
			lmu := httptest.NewServer(mockapi.NewLMUHandler())
			defer lmu.Close()
			mock := mockapi.NewRaceOSHandler()
			raceOS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/authenticate" {
					mock.ServeHTTP(w, r)
					return
				}
				page := r.URL.Query().Get("page")
				if page == "1" {
					mock.ServeHTTP(w, r)
					return
				}
				switch mode {
				case "invalid later event":
					if page == "2" {
						fmt.Fprint(w, `[{"eventId":"bad","date":"2026-01-01T00:00:00Z","drivers":[]}]`)
					} else {
						fmt.Fprint(w, `null`)
					}
				case "repeated page":
					r.URL.RawQuery = "eventType=daily&page=1&take=20"
					mock.ServeHTTP(w, r)
				case "rate limited":
					w.WriteHeader(429)
					fmt.Fprint(w, `private-error-body`)
				}
			}))
			defer raceOS.Close()
			dir := t.TempDir()
			db := filepath.Join(dir, "archive.sqlite3")
			_, err := Sync(context.Background(), SyncOptions{DBPath: db, RawDir: filepath.Join(dir, "raw"), LMUURL: lmu.URL, RaceOSURL: raceOS.URL, EventTypes: []string{"daily"}})
			if err == nil {
				t.Fatal("incomplete/invalid acquisition was accepted")
			}
			if strings.Contains(err.Error(), "private-error-body") {
				t.Fatal("upstream response leaked")
			}
			if _, err := os.Stat(db); !os.IsNotExist(err) {
				t.Fatal("failed acquisition created or changed the archive")
			}
		})
	}
}
