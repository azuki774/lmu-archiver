package main

import (
	"bytes"
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

func TestRecordRatingPersistsOnlyProfileAndExactValues(t *testing.T) {
	lmu := httptest.NewServer(mockapi.NewLMUHandler())
	defer lmu.Close()
	raceOS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/authenticate" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, `{"accessToken":"private-access-token","player":{"id":"player-id","name":"Driver","token":"private-nested-token","platforms":[{"id":"self","platform":"steam"}],"driverRank":{"elo":1500.25,"rank":"Silver","tier":1,"progress":25.5},"safetyRank":{"rating":4.25,"rank":"Gold","tier":2,"progress":80.125}}}`)
	}))
	defer raceOS.Close()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "archive.sqlite3")
	rawDir := filepath.Join(dir, "raw")
	var stdout, stderr bytes.Buffer
	before := time.Now().UTC()
	code := run(context.Background(), []string{"archive", "record-rating", "--db", dbPath, "--raw-dir", rawDir, "--lmu-url", lmu.URL, "--raceos-url", raceOS.URL}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("record failed: %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "private-") {
		t.Fatal("credential leaked to output")
	}
	files, err := os.ReadDir(rawDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one sanitized source file: %d", len(files))
	}
	raw, err := os.ReadFile(filepath.Join(rawDir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) || strings.Contains(string(raw), "private-") || strings.Contains(string(raw), "accessToken") {
		t.Fatal("raw profile is not credential-free JSON")
	}
	info, err := files[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("profile file is not private")
	}
	store, err := storage.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	observations, err := store.Ratings(context.Background(), "self", before, time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].DRElo != 1500.25 || observations[0].SRRating != 4.25 || observations[0].RawPath != filepath.Join(rawDir, files[0].Name()) {
		t.Fatalf("wrong stored observation: %+v", observations)
	}
}

func TestIncompleteProfileIsNotArchived(t *testing.T) {
	lmu := httptest.NewServer(mockapi.NewLMUHandler())
	defer lmu.Close()
	raceOS := httptest.NewServer(mockapi.NewRaceOSHandler())
	defer raceOS.Close()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "archive.sqlite3")
	rawDir := filepath.Join(dir, "raw")
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"archive", "record-rating", "--db", dbPath, "--raw-dir", rawDir, "--lmu-url", lmu.URL, "--raceos-url", raceOS.URL}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "profile") {
		t.Fatalf("expected missing-profile error: %d %s", code, errOut.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("missing profile created an archive")
	}
	if _, err := os.Stat(rawDir); !os.IsNotExist(err) {
		t.Fatal("missing profile created a source file")
	}
	// Authentication itself still accepts a valid session without a profile.
	out.Reset()
	errOut.Reset()
	code = run(context.Background(), []string{"auth", "--lmu-url", lmu.URL, "--raceos-url", raceOS.URL}, &out, &errOut)
	if code != 0 {
		t.Fatalf("profile-less authentication regressed: %d %s", code, errOut.String())
	}
}
