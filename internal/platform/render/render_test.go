package render

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexedwards/scs/v2"
	notedto "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/dto"
	notemodel "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	"github.com/engenheiroaraujo/bridopen/views"
	"github.com/gorilla/csrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chdirModuleRoot(t *testing.T) {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			t.Chdir(dir)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func withCSRFSession(session *scs.SessionManager, next http.HandlerFunc) http.Handler {
	return csrf.Protect([]byte("01234567890123456789012345678901"))(session.LoadAndSave(next))
}

// TestNewRenderTrimsBaseURL verifica que NewRender remove a barra final do baseURL.
func TestNewRenderTrimsBaseURL(t *testing.T) {
	t.Parallel()

	rt := NewRender(scs.New(), "http://example.com/")
	assert.Equal(t, "http://example.com", rt.baseURL)
}

// TestSessionHelpersFallbackWithoutSessionContext verifica que helpers de sessao retornam valores neutros sem contexto SCS.
func TestSessionHelpersFallbackWithoutSessionContext(t *testing.T) {
	t.Parallel()

	rt := NewRender(scs.New(), "http://example.com")
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	assert.False(t, rt.sessionExists(req.Context(), "userId"))
	assert.Empty(t, rt.sessionString(req.Context(), "firstName"))
}

// TestSessionHelpersReadSessionContext verifica que helpers de sessao leem valores quando o contexto SCS existe.
func TestSessionHelpersReadSessionContext(t *testing.T) {
	t.Parallel()

	session := scs.New()
	rt := NewRender(session, "http://example.com")
	handler := session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(1))
		session.Put(r.Context(), "firstName", "Ada")

		assert.True(t, rt.sessionExists(r.Context(), "userId"))
		assert.Equal(t, "Ada", rt.sessionString(r.Context(), "firstName"))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
}

// TestRenderPageEmbedFS verifica RenderPage via FS embutido, headers de cache e conteudo da home.
func TestRenderPageEmbedFS(t *testing.T) {
	t.Parallel()

	session := scs.New()
	rt := NewRender(session, "http://example.com")
	page := notedto.NewNoteListPage(nil, nil, nil, "", "", "", "pinned", false)
	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(1))
		session.Put(r.Context(), "userEmail", "u@example.com")
		session.Put(r.Context(), "firstName", "Ada")
		require.NoError(t, rt.RenderPage(w, r, http.StatusOK, "home.html", page))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	assert.Contains(t, rec.Body.String(), "Últimas anotações")
}

// TestRenderPageLocalhostDisk verifica RenderPage lendo templates do disco em host localhost.
func TestRenderPageLocalhostDisk(t *testing.T) {
	chdirModuleRoot(t)

	session := scs.New()
	rt := NewRender(session, "http://localhost:8080")
	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, rt.RenderPage(w, r, http.StatusOK, "terms.html", nil))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/terms", nil)
	req.Host = "localhost:8080"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Termos de Serviço")
}

// TestRenderPageMissingTemplate verifica que RenderPage retorna erro quando o template não existe.
func TestRenderPageMissingTemplate(t *testing.T) {
	t.Parallel()

	session := scs.New()
	rt := NewRender(session, "http://example.com")
	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.Error(t, rt.RenderPage(w, r, http.StatusOK, "does-not-exist.html", nil))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)
}

// TestRenderPageWriteError verifica propagação de erro de escrita em RenderPage (ex.: cliente desconecta a meio da resposta).
func TestRenderPageWriteError(t *testing.T) {
	session := scs.New()
	rt := NewRender(session, "http://example.com")
	page := notedto.NewNoteListPage([]notemodel.Note{}, nil, nil, "", "", "", "pinned", false)

	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.ErrorContains(t, rt.RenderPage(&failingWriter{ResponseWriter: w}, r, http.StatusOK, "home.html", page), "write failed")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)
}

// TestRenderAuthPageEmbedFS verifica RenderAuthPage via FS embutido com status e corpo não vazios.
func TestRenderAuthPageEmbedFS(t *testing.T) {
	t.Parallel()

	session := scs.New()
	rt := NewRender(session, "http://example.com")
	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, rt.RenderAuthPage(w, r, http.StatusNotFound, "404.html", "missing"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotEmpty(t, rec.Body.String())
}

// TestRenderAuthPageUsesCSRFField verifica que a página de autenticação inclui o campo CSRF no HTML.
func TestRenderAuthPageUsesCSRFField(t *testing.T) {
	t.Parallel()

	session := scs.New()
	rt := NewRender(session, "http://example.com")
	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, rt.RenderAuthPage(w, r, http.StatusOK, "user-signin.html", userdto.SignupPage{
			Form: userdto.UserRequest{},
		}))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/user/signin", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "csrf")
}

// TestRenderAuthPageLocalhostDisk verifica RenderAuthPage lendo templates do disco em host localhost.
func TestRenderAuthPageLocalhostDisk(t *testing.T) {
	chdirModuleRoot(t)

	session := scs.New()
	rt := NewRender(session, "http://localhost")
	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, rt.RenderAuthPage(w, r, http.StatusOK, "generic-error.html", "erro"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	req.Host = "localhost"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Body.String())
}

// TestRenderAuthPageErrors verifica erros de template ausente e de escrita em RenderAuthPage.
func TestRenderAuthPageErrors(t *testing.T) {
	session := scs.New()
	rt := NewRender(session, "http://example.com")

	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.Error(t, rt.RenderAuthPage(w, r, http.StatusOK, "missing-auth.html", nil))
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	handler = withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		require.ErrorContains(t, rt.RenderAuthPage(&failingWriter{ResponseWriter: w}, r, http.StatusOK, "404.html", "x"), "write failed")
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)
}

// TestRenderMailBodyEmbedFS verifica RenderMailBody usando os templates de e-mail cacheados via FS embutido.
func TestRenderMailBodyEmbedFS(t *testing.T) {
	t.Parallel()

	session := scs.New()
	rt := NewRender(session, "http://example.com/")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	body, err := rt.RenderMailBody(req, "confirmation.html", map[string]string{"token": "abc"})
	require.NoError(t, err)
	assert.Contains(t, string(body), "http://example.com/confirmation/abc")

	_, err = rt.RenderMailBody(req, "no-mail.html", map[string]string{})
	require.Error(t, err)
}

// TestRenderMailBodyLocalhostDisk verifica RenderMailBody usando os templates de e-mail cacheados a partir do disco.
func TestRenderMailBodyLocalhostDisk(t *testing.T) {
	chdirModuleRoot(t)

	session := scs.New()
	rt := NewRender(session, "http://localhost:3000")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:3000"
	body, err := rt.RenderMailBody(req, "confirmation.html", map[string]string{"token": "xyz"})
	require.NoError(t, err)
	assert.Contains(t, string(body), "xyz")
}

// TestLoadPageSetAndMailTemplates verifica os carregadores usados em NewRender, tanto via FS embutido quanto via disco, e o caso de diretório inexistente.
func TestLoadPageSetAndMailTemplates(t *testing.T) {
	chdirModuleRoot(t)

	pages, err := loadPageSet(views.Files, "templates/pages", pageBaseFiles, pagePlaceholderFuncs)
	require.NoError(t, err)
	assert.Contains(t, pages, "home.html")

	authPages, err := loadPageSet(os.DirFS("views"), "templates/pages", authPageBaseFiles, authPlaceholderFuncs)
	require.NoError(t, err)
	assert.Contains(t, authPages, "404.html")

	mails, err := loadMailTemplates(views.Files)
	require.NoError(t, err)
	assert.Contains(t, mails, "confirmation.html")

	_, err = loadPageSet(views.Files, "templates/does-not-exist", pageBaseFiles, pagePlaceholderFuncs)
	require.Error(t, err)
}

type failingWriter struct {
	http.ResponseWriter
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// TestTwoFactorEmailRendererRendersCode verifica que o e-mail 2FA contém o título e o código informado.
func TestTwoFactorEmailRendererRendersCode(t *testing.T) {
	t.Parallel()

	body, err := NewTwoFactorEmailRenderer().RenderTwoFactorCode("123456")
	require.NoError(t, err)
	assert.Contains(t, string(body), "Código de verificação")
	assert.Contains(t, string(body), "123456")
}

// renderAuthenticated executa RenderPage para a home com sessão autenticada,
// aplicando os headers informados na requisição.
func renderAuthenticated(t *testing.T, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	session := scs.New()
	rt := NewRender(session, "http://example.com")
	page := notedto.NewNoteListPage(nil, nil, nil, "", "", "", "pinned", false)
	handler := withCSRFSession(session, func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(1))
		session.Put(r.Context(), "firstName", "Ada")
		require.NoError(t, rt.RenderPage(w, r, http.StatusOK, "home.html", page))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	handler.ServeHTTP(rec, req)
	return rec
}

// TestRenderPageShellHasSidebarAndNoNavbar verifica que o shell autenticado
// serve o sidebar global e que o navbar foi de fato removido.
func TestRenderPageShellHasSidebarAndNoNavbar(t *testing.T) {
	t.Parallel()

	body := renderAuthenticated(t, nil).Body.String()

	assert.Contains(t, body, `id="app-sidebar"`)
	assert.Contains(t, body, `id="main-content"`)
	assert.Contains(t, body, `id="page-modals"`)
	// Controles que vieram do navbar precisam continuar existindo.
	assert.Contains(t, body, `/user/signout`)
	assert.Contains(t, body, `data-theme-toggle`)
	assert.Contains(t, body, `/notes/archive`)
	assert.Contains(t, body, `/me/overview/personal-information`)

	// Restos do navbar antigo não podem sobrar.
	assert.NotContains(t, body, `id="navMenu"`)
	assert.NotContains(t, body, `id="hamburger"`)
	assert.NotContains(t, body, `navbarMobileBackdrop`)
	assert.NotContains(t, body, `data-nav-dropdown`)
	assert.NotContains(t, body, `navbar-enter-init.js`)
}

// TestRenderPagePartialFragment verifica que um GET com o header de navegação
// parcial devolve só o fragmento em JSON, sem o shell.
func TestRenderPagePartialFragment(t *testing.T) {
	t.Parallel()

	rec := renderAuthenticated(t, map[string]string{PartialNavHeader: PartialNavValue})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.Equal(t, PartialNavHeader, rec.Header().Get("Vary"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var fragment struct {
		Title  string `json:"title"`
		HTML   string `json:"html"`
		Modals string `json:"modals"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fragment))

	assert.Equal(t, "Início — Bridopen", fragment.Title)
	assert.Contains(t, fragment.HTML, "Últimas anotações")
	// O fragmento carrega só o conteúdo: nada de shell dentro dele.
	assert.NotContains(t, fragment.HTML, "<!DOCTYPE html>")
	assert.NotContains(t, fragment.HTML, `id="app-sidebar"`)
	assert.NotContains(t, fragment.HTML, "<script")
}

// TestIsPartialNavRequest verifica o gate do fragmento: só GET com o header
// exato. Um POST — re-render de formulário inválido — precisa devolver a página
// inteira mesmo que o header venha junto, para não entregar JSON ao navegador.
func TestIsPartialNavRequest(t *testing.T) {
	t.Parallel()

	comHeader := func(method, value string) *http.Request {
		r := httptest.NewRequest(method, "/", nil)
		if value != "" {
			r.Header.Set(PartialNavHeader, value)
		}
		return r
	}

	assert.True(t, isPartialNavRequest(comHeader(http.MethodGet, PartialNavValue)))
	assert.False(t, isPartialNavRequest(comHeader(http.MethodGet, "")))
	assert.False(t, isPartialNavRequest(comHeader(http.MethodGet, "outro")))
	assert.False(t, isPartialNavRequest(comHeader(http.MethodPost, PartialNavValue)))
	assert.False(t, isPartialNavRequest(comHeader(http.MethodDelete, PartialNavValue)))
}
