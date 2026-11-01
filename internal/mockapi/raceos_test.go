package mockapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyMockData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../testdata/mock-api")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAuthenticateRejectsInvalidRequests(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		status int
	}{
		{"malformed JSON", `{"token":`, http.StatusBadRequest},
		{"trailing JSON", `{"token":"mock-steam-ticket","game":"lmu","platform":"steam"} {}`, http.StatusBadRequest},
		{"trailing garbage", `{"token":"mock-steam-ticket","game":"lmu","platform":"steam"} garbage`, http.StatusBadRequest},
		{"wrong field type", `{"token":42,"game":"lmu","platform":"steam"}`, http.StatusBadRequest},
		{"missing credentials", `{}`, http.StatusUnauthorized},
		{"wrong ticket", `{"token":"real-ticket","game":"lmu","platform":"steam"}`, http.StatusUnauthorized},
		{"wrong game", `{"token":"mock-steam-ticket","game":"other","platform":"steam"}`, http.StatusUnauthorized},
		{"wrong platform", `{"token":"mock-steam-ticket","game":"lmu","platform":"other"}`, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/authenticate", strings.NewReader(tc.body))
			NewRaceOSHandler(copyMockData(t)).ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if strings.Contains(response.Body.String(), "accessToken") {
				t.Fatal("invalid request received an access token")
			}
		})
	}
}

func TestResultsPagesAndLiveFileChanges(t *testing.T) {
	dir := copyMockData(t)
	handler := NewRaceOSHandler(dir)
	fetch := func(page string) []map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/results?eventType=daily&take=20&page="+page, nil)
		request.Header.Set("Game-Authorization", "Bearer mock-access-token")
		handler.ServeHTTP(response, request)
		var events []map[string]any
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &events) != nil {
			t.Fatalf("page %s: %d %s", page, response.Code, response.Body.String())
		}
		return events
	}
	first, second := fetch("1"), fetch("2")
	if len(first) != 20 || first[0]["eventId"] != "mock-event-001" || len(second) != 5 || second[0]["eventId"] != "mock-event-021" || second[4]["eventId"] != "mock-event-025" {
		t.Fatal("page boundaries lost or repeated events")
	}
	if fetch("3") != nil {
		t.Fatal("explicit null terminator was lost")
	}
	// Changing the on-disk data must affect the existing handler, without restart.
	second[0]["title"] = "Corrected Mock Race"
	body, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "raceos/results/daily/page-2.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	if fetch("2")[0]["title"] != "Corrected Mock Race" {
		t.Fatal("handler served cached data")
	}
}

func TestMissingAndInvalidResponseFilesFailSafely(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "invalid"}[invalid], func(t *testing.T) {
			dir := t.TempDir()
			if invalid {
				if err := os.Mkdir(filepath.Join(dir, "lmu"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "lmu/auth-session-ticket.json"), []byte(`{"authSessionTicket":"private-secret"} {}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			response := httptest.NewRecorder()
			NewLMUHandler(dir).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rest/profile/getAuthSessionTicket", nil))
			if response.Code != 500 || !json.Valid(response.Body.Bytes()) || strings.Contains(response.Body.String(), dir) || strings.Contains(response.Body.String(), "private-secret") {
				t.Fatalf("unsafe fixture failure: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
