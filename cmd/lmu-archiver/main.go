package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/azuki774/lmu-archiver/internal/api"
	"github.com/azuki774/lmu-archiver/internal/archive"
	"github.com/azuki774/lmu-archiver/internal/auth"
	"github.com/azuki774/lmu-archiver/internal/httpclient"
	"github.com/azuki774/lmu-archiver/internal/query"
	"github.com/azuki774/lmu-archiver/internal/storage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `Usage: lmu-archiver [--config PATH] <command> [flags]

Commands:
  sync        Download results and current DR/SR, save JSON, and update SQLite
  serve       Serve the local archive as a read-only HTTP API
  races       Show latest archived race events
  ratings     Show archived rating observations
  auth check  Check authentication without saving credentials
  db migrate  Create or upgrade the archive schema

Use sync --skip-download --input PATH to replay a capture or results JSON offline.
Run a command with --help for its options.
URL precedence: flags > environment (RACEOS_URL, LMU_INT_API) > JSON config > defaults.
The archive belongs to the first Steam account imported into its SQLite file; use a separate file for another account.`)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fail := func(err error, code int) int { fmt.Fprintln(stderr, "Error:", err); return code }
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		usage(stdout)
		return 0
	}
	cfg, args, err := loadConfig(args)
	if err != nil {
		return fail(err, 2)
	}
	if value, ok := os.LookupEnv("LMU_INT_API"); ok {
		cfg.LMUURL = value
	}
	if value, ok := os.LookupEnv("RACEOS_URL"); ok {
		cfg.RaceOSURL = value
	}
	if len(args) == 0 {
		usage(stdout)
		return 0
	}
	command := args[0]
	tail := args[1:]
	if command == "auth" || command == "db" {
		child := "check"
		if command == "db" {
			child = "migrate"
		}
		if len(tail) == 0 || tail[0] == "--help" || tail[0] == "-h" {
			fmt.Fprintf(stdout, "Usage: lmu-archiver %s %s [flags]\n", command, child)
			return 0
		}
		if tail[0] != child {
			return fail(fmt.Errorf("expected %s %s", command, child), 2)
		}
		command += " " + child
		tail = tail[1:]
	}
	switch command {
	case "sync", "serve", "races", "ratings", "auth check", "db migrate":
	default:
		usage(stderr)
		return fail(errors.New("unknown command"), 2)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	for _, arg := range tail {
		if arg == "--help" || arg == "-h" {
			flags.SetOutput(stdout)
			break
		}
	}
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: lmu-archiver %s [flags]\n", command)
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output(), "  --config PATH\n        Optional JSON config; URL precedence: flags > environment > config > defaults")
	}
	jsonOutput := false
	skipDownload := false
	input, from, to := "", "", ""
	syncSteamID := ""
	track, class, raceEventType := "", "", ""
	limit := "10"
	offset := "0"
	eventTypes := strings.Join(cfg.EventTypes, ",")
	if command != "auth check" {
		flags.StringVar(&cfg.DB, "db", cfg.DB, "SQLite archive path")
	}
	switch command {
	case "sync":
		flags.BoolVar(&skipDownload, "skip-download", false, "Import saved JSON without contacting LMU or RaceOS")
		flags.StringVar(&input, "input", "", "Capture directory or results JSON (required with --skip-download)")
		flags.StringVar(&syncSteamID, "steam-id", "", "Steam ID for results JSON without a manifest, or to assert the authenticated account")
		flags.StringVar(&cfg.RawDir, "raw-dir", cfg.RawDir, "Directory for new private JSON captures")
		flags.StringVar(&eventTypes, "event-types", eventTypes, "Comma-separated event types: daily,specialevent; championship is unverified")
		flags.BoolVar(&jsonOutput, "json", false, "Print a JSON sync summary")
	case "serve":
		flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "HTTP listen address; no authentication or TLS is provided")
	case "races":
		flags.StringVar(&limit, "limit", "10", "Maximum number of race events")
		flags.StringVar(&offset, "offset", "0", "Number of race events to skip before applying --limit")
		flags.StringVar(&from, "from", "", "Inclusive timestamp: YYYY-MM-DD (UTC) or RFC3339; unbounded by default")
		flags.StringVar(&to, "to", "", "Exclusive timestamp: YYYY-MM-DD (UTC) or RFC3339; unbounded by default")
		flags.StringVar(&track, "track", "", "Exact event track name")
		flags.StringVar(&class, "class", "", "Exact tracked player's car class")
		flags.StringVar(&raceEventType, "event-type", "", "Exact event type, for example daily or specialevent")
		flags.BoolVar(&jsonOutput, "json", false, "Print JSON instead of a table")
	case "ratings":
		flags.StringVar(&from, "from", "", "Inclusive timestamp: YYYY-MM-DD (UTC) or RFC3339; default 30 days ago")
		flags.StringVar(&to, "to", "", "Exclusive timestamp: YYYY-MM-DD (UTC) or RFC3339; default now")
		flags.BoolVar(&jsonOutput, "json", false, "Print JSON instead of a table")
	}
	if command == "sync" || command == "auth check" {
		flags.StringVar(&cfg.LMUURL, "lmu-url", cfg.LMUURL, "LMU REST API HTTP origin (environment: LMU_INT_API)")
		flags.StringVar(&cfg.RaceOSURL, "raceos-url", cfg.RaceOSURL, "Trusted RaceOS HTTP(S) origin; receives the Steam ticket (environment: RACEOS_URL)")
	}
	if err := flags.Parse(tail); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return fail(errors.New("unexpected positional arguments"), 2)
	}
	if command != "auth check" && strings.TrimSpace(cfg.DB) == "" {
		return fail(errors.New("--db must not be empty"), 2)
	}
	switch command {
	case "auth check":
		client := httpclient.New()
		defer client.CloseIdleConnections()
		authClient, err := auth.NewClient(cfg.LMUURL, cfg.RaceOSURL, client)
		if err != nil {
			return fail(err, 2)
		}
		if _, err := authClient.Authenticate(ctx); err != nil {
			return fail(err, 1)
		}
		if _, err := fmt.Fprintln(stdout, "Authentication succeeded."); err != nil {
			return fail(err, 1)
		}
		return 0
	case "db migrate":
		store, err := storage.Open(ctx, cfg.DB)
		if err != nil {
			return fail(err, 1)
		}
		defer store.Close()
		if _, err := fmt.Fprintln(stdout, "Archive schema is current."); err != nil {
			return fail(err, 1)
		}
		return 0
	case "sync":
		if skipDownload && input == "" {
			return fail(errors.New("--skip-download requires --input"), 2)
		}
		if !skipDownload && input != "" {
			return fail(errors.New("--input requires --skip-download"), 2)
		}
		fmt.Fprintln(stderr, "Syncing archive...")
		result, err := archive.Sync(ctx, archive.SyncOptions{DBPath: cfg.DB, RawDir: cfg.RawDir, Input: input, SteamID: syncSteamID, LMUURL: cfg.LMUURL, RaceOSURL: cfg.RaceOSURL, SkipDownload: skipDownload, EventTypes: strings.Split(eventTypes, ",")})
		if err != nil {
			return fail(err, 1)
		}
		if jsonOutput {
			if err := json.NewEncoder(stdout).Encode(result); err != nil {
				return fail(err, 1)
			}
		} else {
			if _, err := fmt.Fprintf(stdout, "Synced %d events for %s. New rating observation: %t. Capture: %s\n", result.Events, strconv.Quote(result.SteamID), result.RatingRecorded, strconv.Quote(result.CapturePath)); err != nil {
				return fail(err, 1)
			}
		}
		return 0
	}
	store, err := storage.OpenReadOnly(ctx, cfg.DB)
	if err != nil {
		return fail(err, 1)
	}
	defer store.Close()
	service := query.Service{Store: store}
	if command == "serve" {
		if err := serve(ctx, service, cfg.Listen, stderr); err != nil {
			return fail(err, 1)
		}
		return 0
	}
	if command == "races" {
		values, readErr := service.Races(ctx, query.RacesInput{
			From: from, To: to, Track: track, Class: class, EventType: raceEventType, Limit: limit, Offset: offset,
		})
		if readErr != nil {
			var input *query.InputError
			if errors.As(readErr, &input) {
				return fail(readErr, 2)
			}
			return fail(readErr, 1)
		}
		if jsonOutput {
			err = json.NewEncoder(stdout).Encode(values)
		} else {
			err = printRaces(stdout, values)
		}
	} else {
		values, readErr := service.Ratings(ctx, from, to)
		if readErr != nil {
			var input *query.InputError
			if errors.As(readErr, &input) {
				return fail(readErr, 2)
			}
			return fail(readErr, 1)
		}
		if jsonOutput {
			err = json.NewEncoder(stdout).Encode(values)
		} else {
			err = printRatings(stdout, values)
		}
	}
	if err != nil {
		return fail(err, 1)
	}
	return 0
}

func serve(ctx context.Context, service query.Service, address string, stderr io.Writer) error {
	// Fail before listening when the archive has no sole player.
	if _, err := service.Store.ResolveSteamID(ctx); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: api.NewHandler(service), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Fprintf(stderr, "Serving archive at http://%s (read-only; no authentication or TLS)\n", listener.Addr())
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			server.Close()
			return err
		}
		err := <-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func printRaces(w io.Writer, races []storage.Race) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "DATE (UTC)\tEVENT\tTRACK\tRACE POSITIONS\tREFERENCE DR/SR (OBSERVED AT)")
	for _, race := range races {
		var positions []string
		for _, result := range race.Results {
			if result.SessionType == "race" {
				position := "—"
				if result.Position != nil {
					position = strconv.Itoa(*result.Position)
				}
				positions = append(positions, position)
			}
		}
		reference := "—"
		if r := race.ReferenceRating; r != nil {
			reference = fmt.Sprintf("%g / %g (%s)", r.DRElo, r.SRRating, r.ObservedAt.Format(time.RFC3339Nano))
		}
		fmt.Fprintf(table, "%s\t%q\t%q\t%s\t%s\n", race.Event.At.Format(time.RFC3339), race.Event.Title, race.Event.Track, strings.Join(positions, ","), reference)
	}
	return table.Flush()
}
func printRatings(w io.Writer, ratings []storage.RatingSnapshot) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "OBSERVED AT (UTC)\tDR ELO\tDR RANK\tSR\tSR RANK")
	for _, r := range ratings {
		fmt.Fprintf(table, "%s\t%g\t%q\t%g\t%q\n", r.ObservedAt.Format(time.RFC3339Nano), r.DRElo, r.DRRank, r.SRRating, r.SRRank)
	}
	return table.Flush()
}
