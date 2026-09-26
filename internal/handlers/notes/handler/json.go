package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	notedto "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/dto"
)

func noteWantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest")
}

func noteTagWantsJSON(r *http.Request) bool {
	return noteWantsJSON(r)
}

type noteTagJSONResponse struct {
	OK          bool                    `json:"ok"`
	Tag         notedto.NoteTagResponse `json:"tag,omitempty"`
	FieldErrors map[string]string       `json:"field_errors,omitempty"`
}

type noteActionJSONResponse struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
	Redirect string `json:"redirect,omitempty"`
	Reload   bool   `json:"reload,omitempty"`
}

func writeNoteTagJSON(w http.ResponseWriter, status int, payload noteTagJSONResponse) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

func writeNoteActionJSON(w http.ResponseWriter, status int, payload noteActionJSONResponse) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}
