package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/azuki774/lmu-archiver/internal/auth"
	"github.com/azuki774/lmu-archiver/internal/httpclient"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	usage := func(w io.Writer) {
		fmt.Fprintln(w, "Usage: lmu-archiver <auth|archive> [flags]")
		fmt.Fprintln(w, "\nCommands:")
		fmt.Fprintln(w, "  auth     Authenticate; no credentials are printed or saved")
		fmt.Fprintln(w, "  archive  Import race results, record ratings, and query SQLite")
		fmt.Fprintln(w, "\nRun a command with --help for its options.")
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		usage(stdout)
		return 0
	}
	if args[0] == "archive" {
		return runArchive(ctx, args[1:], stdout, stderr)
	}
	if args[0] != "auth" {
		fmt.Fprintln(stderr, "Error: expected auth or archive")
		usage(stderr)
		return 2
	}
	flags := flag.NewFlagSet("auth", flag.ContinueOnError)
	flags.SetOutput(stderr)
	lmuURL := flags.String("lmu-url", "http://127.0.0.1:6937", "LMU REST API base URL (HTTP)")
	raceOSURL := flags.String("raceos-url", "https://raceos.gg", "RaceOS API base URL (HTTP or HTTPS; use only trusted servers)")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: lmu-archiver auth [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "Error: auth does not accept positional arguments")
		return 2
	}
	httpClient := httpclient.New()
	defer httpClient.CloseIdleConnections()
	client, err := auth.NewClient(*lmuURL, *raceOSURL, httpClient)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 2
	}
	if _, err := client.Authenticate(ctx); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Authentication succeeded.")
	return 0
}
