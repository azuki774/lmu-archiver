package mockapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// NewRaceOSHandler returns a mux with the supported RaceOS routes registered.
// Additional endpoints can be registered on the returned mux.
func NewRaceOSHandler(dataDir string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /authenticate", func(w http.ResponseWriter, r *http.Request) {
		authenticate(w, r, dataDir)
	})
	mux.HandleFunc("GET /api/v1/results", func(w http.ResponseWriter, r *http.Request) {
		results(w, r, dataDir)
	})
	return mux
}

func authenticate(w http.ResponseWriter, r *http.Request, dataDir string) {
	var request struct {
		Token    string `json:"token"`
		Game     string `json:"game"`
		Platform string `json:"platform"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":"invalid JSON request"}`)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, `{"error":"invalid JSON request"}`)
		return
	}
	if request.Token != steamTicket || request.Game != "lmu" || request.Platform != "steam" {
		writeJSON(w, http.StatusUnauthorized, `{"error":"invalid mock credentials"}`)
		return
	}
	writeJSONFile(w, dataDir, "raceos", "authenticate.json")
}

func results(w http.ResponseWriter, r *http.Request, dataDir string) {
	if r.Header.Get("Game-Authorization") != "Bearer mock-access-token" {
		writeJSON(w, http.StatusUnauthorized, `{"error":"invalid mock credentials"}`)
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 || r.URL.Query().Get("take") != "20" {
		writeJSON(w, http.StatusBadRequest, `{"error":"invalid pagination"}`)
		return
	}
	switch r.URL.Query().Get("eventType") {
	case "daily", "specialevent", "championship":
	default:
		writeJSON(w, http.StatusBadRequest, `{"error":"unsupported event type"}`)
		return
	}
	writeJSONFile(w, dataDir, "raceos", "results", r.URL.Query().Get("eventType"), fmt.Sprintf("page-%d.json", page))
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
