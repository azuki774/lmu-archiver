// Package mockapi provides minimal local LMU and RaceOS APIs for development.
// It implements only the endpoints currently used by lmu-archiver, not full API emulation.
package mockapi

import "net/http"

const steamTicket = "mock-steam-ticket"

// NewLMUHandler returns a mux with the supported LMU routes registered.
// Additional endpoints can be registered on the returned mux.
func NewLMUHandler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/profile/getAuthSessionTicket", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, `{"authSessionTicket":"`+steamTicket+`"}`)
	})
	return mux
}
