// Package archive orchestrates downloads and replayable local captures.
package archive

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/azuki774/lmu-archiver/internal/auth"
	"github.com/azuki774/lmu-archiver/internal/httpclient"
	"github.com/azuki774/lmu-archiver/internal/ingest"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

type SyncOptions struct {
	DBPath, RawDir, Input, SteamID, LMUURL, RaceOSURL string
	SkipDownload                                      bool
	EventTypes                                        []string
}

type SyncResult struct {
	SteamID        string `json:"steamId"`
	CapturePath    string `json:"capturePath"`
	Events         int    `json:"events"`
	RatingRecorded bool   `json:"ratingRecorded"`
}

type manifest struct {
	Version    int       `json:"version"`
	CaptureID  string    `json:"captureId"`
	SteamID    string    `json:"steamId"`
	ObservedAt time.Time `json:"observedAt"`
	Results    string    `json:"results"`
	Profile    string    `json:"profile"`
	EventTypes []string  `json:"eventTypes"`
}

// Sync separates acquisition from ingestion. Offline replay never constructs an
// HTTP client, and all inputs are decoded before opening the writable database.
func Sync(ctx context.Context, options SyncOptions) (SyncResult, error) {
	var result SyncResult
	if strings.TrimSpace(options.DBPath) == "" {
		return result, errors.New("database path is required")
	}
	input := options.Input
	if options.SkipDownload {
		if input == "" {
			return result, errors.New("--skip-download requires --input (capture directory or results JSON)")
		}
	} else {
		if input != "" {
			return result, errors.New("--input requires --skip-download")
		}
		var err error
		input, err = download(ctx, options)
		result.CapturePath = input
		if err != nil {
			return result, err
		}
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return result, err
	}
	result.CapturePath = absolute
	info, err := os.Stat(absolute)
	if err != nil {
		return result, fmt.Errorf("read capture: %w", err)
	}
	player := storage.Player{SteamID: options.SteamID}
	var rating *storage.RatingSnapshot
	var captureID string
	resultsPath := absolute
	if info.IsDir() {
		var m manifest
		if err := decodeFile(filepath.Join(absolute, "manifest.json"), &m); err != nil {
			return result, fmt.Errorf("read capture manifest: %w", err)
		}
		if m.Version != 1 || m.CaptureID == "" || m.SteamID == "" || m.ObservedAt.IsZero() || m.Results == "" || m.Profile == "" {
			return result, errors.New("capture manifest is incomplete or unsupported")
		}
		if options.SteamID != "" && options.SteamID != m.SteamID {
			return result, errors.New("configured Steam ID does not match capture")
		}
		resultsPath, err = captureFile(absolute, m.Results)
		if err != nil {
			return result, err
		}
		profilePath, err := captureFile(absolute, m.Profile)
		if err != nil {
			return result, err
		}
		profile, err := os.ReadFile(profilePath)
		if err != nil {
			return result, fmt.Errorf("read saved profile: %w", err)
		}
		var observation storage.RatingSnapshot
		player, observation, _, err = ingest.Profile(profile, m.ObservedAt, profilePath)
		if err != nil {
			return result, err
		}
		if player.SteamID != m.SteamID {
			return result, errors.New("saved profile does not match capture Steam ID")
		}
		rating = &observation
		captureID = m.CaptureID
	} else if strings.TrimSpace(player.SteamID) == "" {
		return result, errors.New("results JSON without a manifest requires --steam-id")
	}
	file, err := os.Open(resultsPath)
	if err != nil {
		return result, fmt.Errorf("read results: %w", err)
	}
	parsedPlayer, events, err := ingest.Results(file, player.SteamID, resultsPath, time.Now().UTC())
	closeErr := file.Close()
	if err != nil {
		return result, fmt.Errorf("ingest results (capture %s): %w", absolute, err)
	}
	if closeErr != nil {
		return result, closeErr
	}
	if player.Name == "" {
		player.Name = parsedPlayer.Name
	}
	result.SteamID = player.SteamID
	result.Events = len(events)
	store, err := storage.Open(ctx, options.DBPath)
	if err != nil {
		return result, fmt.Errorf("open database (capture retained at %s): %w", absolute, err)
	}
	defer store.Close()
	result.RatingRecorded, err = store.ImportCapture(ctx, player, events, captureID, rating)
	if err != nil {
		return result, fmt.Errorf("save database (capture retained at %s): %w", absolute, err)
	}
	return result, nil
}

func captureFile(dir, name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", errors.New("capture file paths must be relative to the capture directory")
	}
	return filepath.Join(dir, name), nil
}

func download(ctx context.Context, options SyncOptions) (string, error) {
	if strings.TrimSpace(options.RawDir) == "" {
		return "", errors.New("raw directory is required")
	}
	types := options.EventTypes
	if len(types) == 0 {
		types = []string{"daily", "specialevent"}
	}
	seenTypes := map[string]bool{}
	for _, kind := range types {
		if kind != "daily" && kind != "specialevent" && kind != "championship" {
			return "", errors.New("event types must be daily, specialevent, or championship (unverified)")
		}
		if seenTypes[kind] {
			return "", errors.New("event types must not repeat")
		}
		seenTypes[kind] = true
	}
	httpClient := httpclient.New()
	defer httpClient.CloseIdleConnections()
	client, err := auth.NewClient(options.LMUURL, options.RaceOSURL, httpClient)
	if err != nil {
		return "", err
	}
	session, err := client.Authenticate(ctx)
	if err != nil {
		return "", err
	}
	observedAt := time.Now().UTC()
	player, _, profile, err := ingest.Profile(session.Profile, observedAt, "")
	if err != nil {
		return "", err
	}
	if options.SteamID != "" && options.SteamID != player.SteamID {
		return "", errors.New("configured Steam ID does not match authenticated player")
	}
	root, err := filepath.Abs(options.RawDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, observedAt.Format("20060102T150405.000000Z")+"-")
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		return dir, fmt.Errorf("download failed; partial capture at %s (not imported): %w", dir, err)
	}
	if err := writeJSONFile(filepath.Join(dir, "profile.json"), json.RawMessage(profile)); err != nil {
		return fail(err)
	}
	var results []json.RawMessage
	seenEvents := map[string]bool{}
	for _, kind := range types {
		seenPages := map[string]bool{}
		for page := 1; ; page++ {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			query := url.Values{"eventType": {kind}, "page": {strconv.Itoa(page)}, "take": {"20"}}
			endpoint := strings.TrimRight(options.RaceOSURL, "/") + "/api/v1/results?" + query.Encode()
			rows, err := fetchPage(ctx, httpClient, endpoint, session.AccessToken)
			if err != nil {
				return fail(fmt.Errorf("%s page %d: %w", kind, page, err))
			}
			if err := writeJSONFile(filepath.Join(dir, fmt.Sprintf("%s-page-%04d.json", kind, page)), rows); err != nil {
				return fail(err)
			}
			if len(rows) == 0 {
				break
			}
			// ID sequences detect APIs that ignore page or cycle already-seen pages.
			var ids strings.Builder
			for _, row := range rows {
				var identity struct {
					ID string `json:"eventId"`
				}
				if err := json.Unmarshal(row, &identity); err != nil || identity.ID == "" {
					return fail(errors.New("results page contains an event without an ID"))
				}
				ids.WriteString(strconv.Quote(identity.ID))
				ids.WriteByte('\n')
				if !seenEvents[identity.ID] {
					results = append(results, row)
					seenEvents[identity.ID] = true
				}
			}
			signature := ids.String()
			if seenPages[signature] {
				return fail(errors.New("results API repeated a page; pagination did not advance"))
			}
			seenPages[signature] = true
		}
	}
	if results == nil {
		results = []json.RawMessage{}
	}
	if err := writeJSONFile(filepath.Join(dir, "results.json"), results); err != nil {
		return fail(err)
	}
	m := manifest{Version: 1, CaptureID: rand.Text(), SteamID: player.SteamID, ObservedAt: observedAt, Results: "results.json", Profile: "profile.json", EventTypes: types}
	if err := writeJSONFile(filepath.Join(dir, "manifest.json"), m); err != nil {
		return fail(err)
	}
	return dir, nil
}

func fetchPage(ctx context.Context, client *http.Client, endpoint, token string) ([]json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("cannot create results request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Game-Authorization", "Bearer "+token)
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("results request failed (connection, TLS, or timeout)")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("results API: HTTP %d", response.StatusCode)
	}
	var rows []json.RawMessage
	if err := decodeJSON(response.Body, &rows); err != nil {
		return nil, errors.New("results response must be a JSON array or null")
	}
	return rows, nil
}

func writeJSONFile(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	writeErr := json.NewEncoder(file).Encode(value)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
func decodeFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeJSON(bytes.NewReader(data), value)
}
func decodeJSON(input io.Reader, value any) error {
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(value); err != nil {
		return errors.New("invalid JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON or invalid data")
	}
	return nil
}
