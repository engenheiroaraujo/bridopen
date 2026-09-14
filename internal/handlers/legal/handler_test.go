package legal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/gorilla/csrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHandler verifica que NewHandler cria o handler com o render injetado.
func TestNewHandler(t *testing.T) {
	t.Parallel()

	session := scs.New()
	h := NewHandler(render.NewRender(session, "http://example.com"))
	require.NotNil(t, h)
	assert.NotNil(t, h.render)
}

// TestLegalPagesRenderOK verifica que as páginas legais (privacidade, termos e cookies) renderizam 200.
func TestLegalPagesRenderOK(t *testing.T) {
	t.Parallel()

	session := scs.New()
	h := NewHandler(render.NewRender(session, "http://example.com"))
	csrfKey := []byte("01234567890123456789012345678901")

	cases := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request) error
	}{
		{"privacy", h.PrivacyPolicy},
		{"terms", h.TermsOfService},
		{"cookies", h.CookiePolicy},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, tc.fn(w, r))
			})
			handler := csrf.Protect(csrfKey)(session.LoadAndSave(inner))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/"+tc.name, nil)
			req.Host = "example.com"
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			assert.NotEmpty(t, body)
			assert.Contains(t, body, "site-footer")
			assert.NotContains(t, body, "app-navbar")
			assert.Contains(t, body, "/terms")
			assert.Contains(t, body, "/privacy")
			assert.Contains(t, body, "/cookies")
		})
	}
}
