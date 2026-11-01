package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/azuki774/lmu-archiver/internal/auth"
	"github.com/azuki774/lmu-archiver/internal/httpclient"
	"github.com/azuki774/lmu-archiver/internal/ingest"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func runArchive(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: lmu-archiver archive <command> [flags]\n\nCommands:\n  migrate         Create or migrate the SQLite archive\n  import-results  Import a raw results JSON file for one Steam ID\n  record-rating   Authenticate and save a current DR/SR observation\n  races           Print latest events, own sessions, and reference ratings as JSON\n  ratings         Print rating observations in a time range as JSON")
		return 0
	}
	command := args[0]
	flags := flag.NewFlagSet("archive "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dbPath := flags.String("db", "data/archive.sqlite3", "SQLite archive path (created/migrated automatically)")
	var steamID, input, lmuURL, raceOSURL, rawDir, fromText, toText string
	var limit int
	now := time.Now().UTC()
	switch command {
	case "migrate":
	case "import-results":
		flags.StringVar(&steamID, "steam-id", "", "Tracked player's Steam ID (required)")
		flags.StringVar(&input, "input", "", "Raw results JSON array or null page (required; file stays separate)")
	case "record-rating":
		flags.StringVar(&lmuURL, "lmu-url", "http://127.0.0.1:6937", "LMU REST API base URL (HTTP)")
		flags.StringVar(&raceOSURL, "raceos-url", "https://raceos.gg", "RaceOS API base URL (use only trusted servers)")
		flags.StringVar(&rawDir, "raw-dir", "data/raw", "Directory for credential-free profile JSON")
	case "races":
		flags.StringVar(&steamID, "steam-id", "", "Tracked player's Steam ID (required)")
		flags.IntVar(&limit, "limit", 10, "Maximum number of events with own race results")
	case "ratings":
		flags.StringVar(&steamID, "steam-id", "", "Tracked player's Steam ID (required)")
		flags.StringVar(&fromText, "from", now.AddDate(0, 0, -30).Format(time.RFC3339Nano), "Inclusive observation timestamp (RFC3339)")
		flags.StringVar(&toText, "to", now.Format(time.RFC3339Nano), "Exclusive observation timestamp (RFC3339)")
	default:
		fmt.Fprintln(stderr, "Error: unknown archive command")
		return 2
	}
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: lmu-archiver archive %s [flags]\n", command)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	usageError := func(message string) int { fmt.Fprintln(stderr, "Error:", message); return 2 }
	if flags.NArg() != 0 {
		return usageError("archive commands do not accept positional arguments")
	}
	if strings.TrimSpace(*dbPath) == "" {
		return usageError("--db is required")
	}
	if (command == "import-results" || command == "races" || command == "ratings") && strings.TrimSpace(steamID) == "" {
		return usageError("--steam-id is required")
	}
	if command == "races" && limit <= 0 {
		return usageError("--limit must be positive")
	}
	if command == "record-rating" && strings.TrimSpace(rawDir) == "" {
		return usageError("--raw-dir is required")
	}
	var from, to time.Time
	if command == "ratings" {
		var err error
		from, err = time.Parse(time.RFC3339Nano, fromText)
		if err != nil {
			return usageError("--from must be RFC3339")
		}
		to, err = time.Parse(time.RFC3339Nano, toText)
		if err != nil {
			return usageError("--to must be RFC3339")
		}
		if !from.Before(to) {
			return usageError("--from must precede --to")
		}
	}
	fail := func(err error) int { fmt.Fprintln(stderr, "Error:", err); return 1 }
	var player storage.Player
	var events []storage.EventData
	var snapshot storage.RatingSnapshot
	if command == "import-results" {
		if input == "" {
			return usageError("--input is required")
		}
		absolute, err := filepath.Abs(input)
		if err != nil {
			return fail(err)
		}
		file, err := os.Open(absolute)
		if err != nil {
			return fail(err)
		}
		player, events, err = ingest.Results(file, steamID, absolute, now)
		closeErr := file.Close()
		if err != nil {
			return fail(err)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
	}
	if command == "record-rating" {
		httpClient := httpclient.New()
		defer httpClient.CloseIdleConnections()
		client, err := auth.NewClient(lmuURL, raceOSURL, httpClient)
		if err != nil {
			return usageError(err.Error())
		}
		session, err := client.Authenticate(ctx)
		if err != nil {
			return fail(err)
		}
		var safe []byte
		player, snapshot, safe, err = ingest.Profile(session.Profile, time.Now().UTC(), "")
		if err != nil {
			return fail(err)
		}
		snapshot.RawPath, err = saveProfile(rawDir, safe, snapshot.ObservedAt)
		if err != nil {
			return fail(err)
		}
	}
	store, err := storage.Open(ctx, *dbPath)
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	switch command {
	case "migrate":
		_, err = fmt.Fprintln(stdout, "Archive schema is current.")
	case "import-results":
		err = store.ImportEvents(ctx, player, events)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "Imported %d events for the tracked player.\n", len(events))
		}
	case "record-rating":
		err = store.RecordRating(ctx, player, snapshot)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "Recorded rating observation at %s. Profile: %s\n", snapshot.ObservedAt.Format(time.RFC3339Nano), snapshot.RawPath)
		}
	case "races":
		var races []storage.Race
		races, err = store.LatestRaces(ctx, steamID, limit)
		if err == nil {
			err = json.NewEncoder(stdout).Encode(races)
		}
	case "ratings":
		var ratings []storage.RatingSnapshot
		ratings, err = store.Ratings(ctx, steamID, from, to)
		if err == nil {
			err = json.NewEncoder(stdout).Encode(ratings)
		}
	}
	if err != nil {
		return fail(err)
	}
	return 0
}

func saveProfile(dir string, safe []byte, at time.Time) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(absolute, "profile-"+at.UTC().Format("20060102T150405.000000Z")+"-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	_, writeErr := file.Write(safe)
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(path)
		return "", writeErr
	}
	if closeErr != nil {
		os.Remove(path)
		return "", closeErr
	}
	return path, nil
}
