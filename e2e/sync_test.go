//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/archive"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func TestCLIFileBackedSyncAndUpdates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	buildCtx, buildCancel := context.WithTimeout(context.Background(), time.Minute)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", dir+string(os.PathSeparator), "./cmd/lmu-archiver", "./cmd/mock-apis")
	build.Dir = ".."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build binaries: %v\n%s", err, output)
	}
	dataDir := copyMockData(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	mock := exec.CommandContext(ctx, filepath.Join(dir, "mock-apis"), "--data-dir", dataDir, "--lmu-listen", "127.0.0.1:0", "--raceos-listen", "127.0.0.1:0")
	mock.Dir = dir // Neither binary may depend on the repository's working directory.
	mock.Stderr = os.Stderr
	stdout, err := mock.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = mock.Wait()
	})
	addresses := make(chan string, 2)
	go func() {
		defer close(addresses)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			for _, prefix := range []string{"LMU mock: ", "RaceOS mock: "} {
				if strings.HasPrefix(scanner.Text(), prefix) {
					addresses <- strings.TrimPrefix(scanner.Text(), prefix)
				}
			}
		}
	}()
	for _, key := range []string{"LMU_INT_API", "RACEOS_URL"} {
		select {
		case address, ok := <-addresses:
			if !ok {
				t.Fatal("mock exited before publishing both addresses")
			}
			t.Setenv(key, address)
		case <-ctx.Done():
			t.Fatal("mock startup timed out")
		}
	}
	binary := filepath.Join(dir, "lmu-archiver")
	db := filepath.Join(dir, "archive.sqlite3")
	runCLI := func(args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(ctx, binary, args...)
		command.Dir = dir
		var out, stderr bytes.Buffer
		command.Stdout, command.Stderr = &out, &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("CLI %v: %v\n%s", args, err, stderr.String())
		}
		for _, secret := range []string{"mock-steam-ticket", "mock-access-token"} {
			if bytes.Contains(out.Bytes(), []byte(secret)) || bytes.Contains(stderr.Bytes(), []byte(secret)) {
				t.Fatal("CLI exposed credentials")
			}
		}
		return out.Bytes()
	}
	syncArgs := []string{"sync", "--db", db, "--raw-dir", filepath.Join(dir, "raw"), "--json"}
	var initial archive.SyncResult
	if err := json.Unmarshal(runCLI(syncArgs...), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Events != 25 || initial.SteamID != "mock-steam-id" || !initial.RatingRecorded {
		t.Fatalf("initial sync: %+v", initial)
	}
	var races []storage.Race
	queryRaces := func() {
		t.Helper()
		if err := json.Unmarshal(runCLI("races", "--db", db, "--limit", "100", "--json"), &races); err != nil {
			t.Fatal(err)
		}
	}
	queryRaces()
	if len(races) != 25 {
		t.Fatalf("initial races = %d", len(races))
	}
	for i, race := range races {
		if race.Event.ID != fmt.Sprintf("mock-event-%03d", i+1) {
			t.Fatalf("missing or repeated page boundary: index %d = %s", i, race.Event.ID)
		}
	}
	if *races[0].Results[0].Position != 2 || *races[0].Results[0].BestLapUS != 99_835_000 {
		t.Fatal("initial result values changed")
	}
	// Prepare changed response files only after the first complete sync finishes.
	var events []map[string]any
	for page := 1; page <= 2; page++ {
		body, err := os.ReadFile(filepath.Join(dataDir, fmt.Sprintf("raceos/results/daily/page-%d.json", page)))
		if err != nil {
			t.Fatal(err)
		}
		var rows []map[string]any
		if err := json.Unmarshal(body, &rows); err != nil {
			t.Fatal(err)
		}
		events = append(events, rows...)
	}
	// Correct an existing second-page race while retaining its identity.
	events[20]["title"] = "Corrected Mock Race"
	newBody, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	var added map[string]any
	if err := json.Unmarshal(newBody, &added); err != nil {
		t.Fatal(err)
	}
	added["eventId"], added["date"], added["title"] = "mock-event-026", "2026-01-26T12:00:00Z", "New Mock Race"
	added["race"].(map[string]any)["races"].([]any)[0].(map[string]any)["results"].([]any)[0].(map[string]any)["position"] = "1"
	events = append([]map[string]any{added}, events...)
	writeJSON := func(path string, value any) {
		t.Helper()
		body, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dataDir, path), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("raceos/results/daily/page-1.json", events[:20])
	writeJSON("raceos/results/daily/page-2.json", events[20:])
	body, err := os.ReadFile(filepath.Join(dataDir, "raceos/authenticate.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile map[string]any
	if err := json.Unmarshal(body, &profile); err != nil {
		t.Fatal(err)
	}
	player := profile["player"].(map[string]any)
	player["driverRank"].(map[string]any)["elo"] = 1520.5
	player["safetyRank"].(map[string]any)["rating"] = 4.5
	writeJSON("raceos/authenticate.json", profile)
	var updated archive.SyncResult
	if err := json.Unmarshal(runCLI(syncArgs...), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Events != 26 || !updated.RatingRecorded {
		t.Fatalf("updated sync: %+v", updated)
	}
	queryRaces()
	if len(races) != 26 || races[0].Event.ID != "mock-event-026" || *races[0].Results[0].Position != 1 {
		t.Fatal("new race not imported or duplicates created")
	}
	if races[21].Event.ID != "mock-event-021" || races[21].Event.Title != "Corrected Mock Race" {
		t.Fatal("existing second-page event was not updated")
	}
	var replay archive.SyncResult
	if err := json.Unmarshal(runCLI("sync", "--skip-download", "--input", updated.CapturePath, "--db", db, "--json"), &replay); err != nil {
		t.Fatal(err)
	}
	if replay.Events != 26 || replay.RatingRecorded {
		t.Fatalf("replay duplicated data: %+v", replay)
	}
	var ratings []storage.RatingSnapshot
	if err := json.Unmarshal(runCLI("ratings", "--db", db, "--from", "2000-01-01", "--to", "2100-01-01", "--json"), &ratings); err != nil {
		t.Fatal(err)
	}
	if len(ratings) != 2 || ratings[0].DRElo != 1500.25 || ratings[0].SRRating != 4.25 || ratings[1].DRElo != 1520.5 || ratings[1].SRRating != 4.5 {
		t.Fatalf("wrong rating history: %+v", ratings)
	}
	queryRaces()
	if len(races) != 26 {
		t.Fatal("replay duplicated events")
	}
}
