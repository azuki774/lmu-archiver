package mockapi

import (
	"encoding/json"
	"io"
	"net/http"
)

// NewRaceOSHandler returns a mux with the supported RaceOS routes registered.
// Additional endpoints can be registered on the returned mux.
func NewRaceOSHandler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /authenticate", authenticate)
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
	writeJSON(w, http.StatusOK, `{"accessToken":"mock-access-token"}`)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
