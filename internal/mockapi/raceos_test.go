package mockapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
			NewRaceOSHandler().ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if strings.Contains(response.Body.String(), "accessToken") {
				t.Fatal("invalid request received an access token")
			}
		})
	}
}
