package mockapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// NewRaceOSHandler returns a mux with the supported RaceOS routes registered.
// Additional endpoints can be registered on the returned mux.
func NewRaceOSHandler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /authenticate", authenticate)
	mux.HandleFunc("GET /api/v1/results", results)
	return mux
}

func authenticate(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, `{"accessToken":"mock-access-token","player":{"id":"mock-player","name":"Mock Driver","platforms":[{"id":"mock-steam-id","platform":"steam"}],"driverRank":{"elo":1500.25,"rank":"Silver","tier":1,"progress":25.5},"safetyRank":{"rating":4.25,"rank":"Gold","tier":2,"progress":80.125}}}`)
}

func results(w http.ResponseWriter, r *http.Request) {
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
	case "daily":
		if page == 1 {
			writeJSON(w, http.StatusOK, `[{"eventId":"mock-event","date":"2026-01-01T12:00:00Z","title":"Mock Race","track":"Mock Circuit","eventType":"daily","drivers":[{"steamId":"mock-steam-id","name":"Mock Driver","class":"GT3"}],"race":{"sessionNo":"1","races":[{"results":[{"steamId":"mock-steam-id","position":"2","classPosition":"1","Laps":18,"bestLapTime":"99.835","finishStatus":"Finished Normally"}]}]}}]`)
			return
		}
	case "specialevent", "championship":
	default:
		writeJSON(w, http.StatusBadRequest, `{"error":"unsupported event type"}`)
		return
	}
	writeJSON(w, http.StatusOK, `null`)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
