package legal

import (
	"net/http"

	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
)

type Handler struct {
	render *render.RenderTemplate
}

func NewHandler(render *render.RenderTemplate) *Handler {
	return &Handler{render: render}
}

func (h *Handler) PrivacyPolicy(w http.ResponseWriter, r *http.Request) error {
	return h.render.RenderAuthPage(w, r, http.StatusOK, "privacy.html", nil)
}

func (h *Handler) TermsOfService(w http.ResponseWriter, r *http.Request) error {
	return h.render.RenderAuthPage(w, r, http.StatusOK, "terms.html", nil)
}

func (h *Handler) CookiePolicy(w http.ResponseWriter, r *http.Request) error {
	return h.render.RenderAuthPage(w, r, http.StatusOK, "cookies.html", nil)
}
