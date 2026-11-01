package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/azuki774/lmu-archiver/internal/mockapi"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("mock-apis", flag.ContinueOnError)
	flags.SetOutput(stderr)
	lmuAddress := flags.String("lmu-listen", "127.0.0.1:6937", "LMU mock listen address")
	raceOSAddress := flags.String("raceos-listen", "127.0.0.1:8081", "RaceOS mock listen address")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "Error: mock-apis does not accept positional arguments")
		return 2
	}
	lmuListener, err := net.Listen("tcp", *lmuAddress)
	if err != nil {
		fmt.Fprintln(stderr, "Error: LMU listen:", err)
		return 1
	}
	defer lmuListener.Close()
	raceOSListener, err := net.Listen("tcp", *raceOSAddress)
	if err != nil {
		fmt.Fprintln(stderr, "Error: RaceOS listen:", err)
		return 1
	}
	defer raceOSListener.Close()

	lmu := &http.Server{Handler: mockapi.NewLMUHandler(), ReadHeaderTimeout: 5 * time.Second}
	raceOS := &http.Server{Handler: mockapi.NewRaceOSHandler(), ReadHeaderTimeout: 5 * time.Second}
	results := make(chan error, 2)
	go func() { results <- lmu.Serve(lmuListener) }()
	go func() { results <- raceOS.Serve(raceOSListener) }()
	fmt.Fprintln(stdout, "LMU mock: http://"+lmuListener.Addr().String())
	fmt.Fprintln(stdout, "RaceOS mock: http://"+raceOSListener.Addr().String())

	exitCode := 0
	remaining := 2
	select {
	case <-ctx.Done():
	case err := <-results:
		remaining--
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "Error: serving mock API:", err)
			exitCode = 1
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, server := range []*http.Server{lmu, raceOS} {
		if err := server.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintln(stderr, "Error: shutting down mock API:", err)
			_ = server.Close()
			exitCode = 1
		}
	}
	for range remaining {
		if err := <-results; !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "Error: serving mock API:", err)
			exitCode = 1
		}
	}
	return exitCode
}
