package buildinfo

import (
	"encoding/json"
	"net/http"
)

// Handler writes the running build metadata as JSON.
func Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(Current())
}
