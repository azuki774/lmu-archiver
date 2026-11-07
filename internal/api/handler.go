// Package api serves the local archive without contacting upstream services.
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/azuki774/lmu-archiver/internal/query"
)

// NewHandler serves the single account stored in the archive. Requests cannot
// select an account; the DB owner is resolved internally.
func NewHandler(service query.Service) http.Handler {
	mux := http.NewServeMux()
	register := func(path string, read func(*http.Request) (any, error)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			value, err := read(r)
			if err != nil {
				var input *query.InputError
				if errors.As(err, &input) {
					writeError(w, http.StatusBadRequest, input.Message)
				} else {
					writeError(w, http.StatusInternalServerError, "archive query failed")
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			// Encode before sending headers so failures cannot yield a partial success.
			data, err := json.Marshal(value)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "archive encoding failed")
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(append(data, '\n'))
		})
	}
	register("/api/v1/races", func(r *http.Request) (any, error) {
		q := r.URL.Query()
		return service.Races(r.Context(), q.Get("limit"))
	})
	register("/api/v1/ratings", func(r *http.Request) (any, error) {
		q := r.URL.Query()
		if q.Has("limit") {
			if q.Has("from") || q.Has("to") {
				return nil, &query.InputError{Message: "limit cannot be combined with from or to"}
			}
			return service.LatestRatings(r.Context(), q.Get("limit"))
		}
		return service.Ratings(r.Context(), q.Get("from"), q.Get("to"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, http.StatusNotFound, "not found") })
	return mux
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{message})
}
