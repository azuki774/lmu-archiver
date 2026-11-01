package mockapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// Routes supply fixed path components; request paths are never filesystem paths.
func writeJSONFile(w http.ResponseWriter, dataDir string, parts ...string) {
	body, err := os.ReadFile(filepath.Join(append([]string{dataDir}, parts...)...))
	if err != nil || !json.Valid(body) {
		writeJSON(w, http.StatusInternalServerError, `{"error":"mock response file is missing, unreadable, or invalid JSON"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
