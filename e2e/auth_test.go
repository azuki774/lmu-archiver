//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azuki774/lmu-archiver/internal/mockapi"
)

func TestCLIAuthentication(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	binary := filepath.Join(t.TempDir(), "lmu-archiver")
	buildCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, "./cmd/lmu-archiver")
	build.Dir = ".."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	for _, tc := range []struct {
		name          string
		lmuFailure    bool
		raceOSStatus  int
		raceOSBody    string
		wantExit      int
		wantError     string
		wantRaceCalls int32
	}{
		{name: "success", wantRaceCalls: 1},
		{
			name: "LMU unavailable", lmuFailure: true, wantExit: 1,
			wantError: "LMU ticket: HTTP 503",
		},
		{
			name: "RaceOS rejects ticket", raceOSStatus: http.StatusUnauthorized,
			raceOSBody: `{"error":"mock-steam-ticket mock-access-token private-response"}`,
			wantExit:   1, wantError: "RaceOS authentication: HTTP 401", wantRaceCalls: 1,
		},
		{
			name: "RaceOS malformed response", raceOSStatus: http.StatusOK,
			raceOSBody: `{"accessToken":"mock-access-token",private-response`,
			wantExit:   1, wantError: "RaceOS authentication: invalid JSON response", wantRaceCalls: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lmuHandler := mockapi.NewLMUHandler()
			lmu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.lmuFailure {
					http.Error(w, "mock-steam-ticket private-response", http.StatusServiceUnavailable)
					return
				}
				lmuHandler.ServeHTTP(w, r)
			}))
			defer lmu.Close()
			var raceCalls atomic.Int32
			raceOSHandler := mockapi.NewRaceOSHandler()
			raceOS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raceCalls.Add(1)
				if tc.raceOSStatus != 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.raceOSStatus)
					_, _ = w.Write([]byte(tc.raceOSBody))
					return
				}
				raceOSHandler.ServeHTTP(w, r)
			}))
			defer raceOS.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "auth", "check", "--lmu-url", lmu.URL, "--raceos-url", raceOS.URL)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if ctx.Err() != nil {
				t.Fatalf("CLI timed out: %v", ctx.Err())
			}
			exitCode := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("execute CLI: %v", err)
				}
				exitCode = exitErr.ExitCode()
			}
			if exitCode != tc.wantExit {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", exitCode, tc.wantExit, stdout.String(), stderr.String())
			}
			if tc.wantExit != 0 && (stdout.Len() != 0 || !strings.Contains(stderr.String(), tc.wantError)) {
				t.Fatalf("unexpected failure output: stdout=%q stderr=%q; want error %q", stdout.String(), stderr.String(), tc.wantError)
			}
			for _, secret := range []string{"mock-steam-ticket", "mock-access-token", "private-response"} {
				if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
					t.Errorf("CLI exposed %q", secret)
				}
			}
			if got := raceCalls.Load(); got != tc.wantRaceCalls {
				t.Errorf("RaceOS requests = %d, want %d", got, tc.wantRaceCalls)
			}
		})
	}
}
