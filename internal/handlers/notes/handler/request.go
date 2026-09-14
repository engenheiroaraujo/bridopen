package handlers

import (
	"net/http"

	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"strconv"
	"strings"
)

func noteIDFromRequest(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, apperrors.ErrPageNotFound
	}
	return id, nil
}

func noteAttachmentIDFromRequest(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("attachmentId"))
	if err != nil || id <= 0 {
		return 0, apperrors.ErrPageNotFound
	}
	return id, nil
}

func noteTagIDFromRequest(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, apperrors.ErrPageNotFound
	}
	return id, nil
}

func noteActionRedirect(r *http.Request, fallback string) string {
	redirect := strings.TrimSpace(r.PostFormValue("redirect"))
	if redirect == "" ||
		!strings.HasPrefix(redirect, "/") ||
		strings.HasPrefix(redirect, "//") ||
		strings.Contains(redirect, "\\") {
		return fallback
	}
	return redirect
}
