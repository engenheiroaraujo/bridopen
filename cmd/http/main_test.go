package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/engenheiroaraujo/bridopen/internal/platform/database"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/engenheiroaraujo/bridopen/views"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain garante que os testes rodem a partir da raiz do módulo: com o
// cache de templates carregado no startup, o baseURL "localhost" usado pelos
// testes deste pacote passa a ler os templates do disco (views/templates/...)
// já em render.NewRender, e não apenas no primeiro request como antes.
func TestMain(m *testing.M) {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func testMainConfig() Config {
	return Config{
		AppEnv:               "development",
		ServerPort:           "0",
		BaseURL:              "http://localhost:5000",
		DBConnURL:            "postgres://unused",
		LevelLog:             "info",
		MailHost:             "localhost",
		MailPort:             "25",
		MailUserName:         "u",
		MailPassword:         "p",
		MailFrom:             "from@bridopen.com",
		CSRFKey:              "01234567890123456789012345678901",
		CookieSecure:         "true",
		CookieDomain:         "localhost",
		AttachmentStorageDir: "storage/attachments",
		ClamAVAddr:           "",
	}
}

func restoreMainHooks(t *testing.T) {
	t.Helper()
	origExit := osExit
	origLoad := loadAppConfig
	origPool := openDBPool
	origStore := setupSessionStore
	origRoutes := routeLoader
	origListen := listenAndServe
	t.Cleanup(func() {
		osExit = origExit
		loadAppConfig = origLoad
		openDBPool = origPool
		setupSessionStore = origStore
		routeLoader = origRoutes
		listenAndServe = origListen
	})
}

// TestMainDBConnectErrorExits verifica que main encerra com código 1 quando a conexão com o banco falha.
func TestMainDBConnectErrorExits(t *testing.T) {
	restoreMainHooks(t)

	loadAppConfig = testMainConfig
	openDBPool = func(context.Context, string) (*pgxpool.Pool, error) {
		return nil, errors.New("db down")
	}
	exited := -1
	osExit = func(code int) {
		exited = code
		panic("exit")
	}

	require.Panics(t, func() { main() })
	assert.Equal(t, 1, exited)
}

// TestMainListenErrorExits verifica que main encerra com código 1 quando ListenAndServe falha.
func TestMainListenErrorExits(t *testing.T) {
	restoreMainHooks(t)

	loadAppConfig = testMainConfig
	openDBPool = func(context.Context, string) (*pgxpool.Pool, error) {
		pool, err := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/bridopen?connect_timeout=1")
		require.NoError(t, err)
		return pool, nil
	}
	setupSessionStore = func(*scs.SessionManager, *pgxpool.Pool) {}
	routeLoader = func(*scs.SessionManager, mailer.MailService, database.Pool, string, string, string, string) http.Handler {
		return http.NewServeMux()
	}
	listenAndServe = func(*http.Server) error {
		return errors.New("bind failed")
	}
	exited := -1
	osExit = func(code int) {
		exited = code
		panic("exit")
	}

	require.Panics(t, func() { main() })
	assert.Equal(t, 1, exited)
}

// TestMainSuccessAndServerClosed verifica que main conclui sem exit quando o servidor retorna ErrServerClosed.
func TestMainSuccessAndServerClosed(t *testing.T) {
	restoreMainHooks(t)

	cfg := testMainConfig()
	cfg.CookieDomain = ""
	cfg.CookieSecure = "false"
	loadAppConfig = func() Config { return cfg }

	openDBPool = func(context.Context, string) (*pgxpool.Pool, error) {
		pool, err := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/bridopen?connect_timeout=1")
		require.NoError(t, err)
		return pool, nil
	}
	setupSessionStore = func(*scs.SessionManager, *pgxpool.Pool) {}
	routeLoader = func(*scs.SessionManager, mailer.MailService, database.Pool, string, string, string, string) http.Handler {
		return http.NewServeMux()
	}
	listenAndServe = func(*http.Server) error {
		return http.ErrServerClosed
	}
	osExit = func(code int) {
		t.Fatalf("unexpected exit %d", code)
	}

	assert.NotPanics(t, func() { main() })
}

// TestMainListenNilError verifica que main conclui sem exit quando ListenAndServe retorna nil.
func TestMainListenNilError(t *testing.T) {
	restoreMainHooks(t)

	loadAppConfig = testMainConfig
	openDBPool = func(context.Context, string) (*pgxpool.Pool, error) {
		pool, err := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/bridopen?connect_timeout=1")
		require.NoError(t, err)
		return pool, nil
	}
	setupSessionStore = func(sm *scs.SessionManager, pool *pgxpool.Pool) {
		// Exercita o caminho de wiring do store padrão já substituído; mantém o store em memória.
	}
	routeLoader = func(*scs.SessionManager, mailer.MailService, database.Pool, string, string, string, string) http.Handler {
		return http.NewServeMux()
	}
	listenAndServe = func(*http.Server) error { return nil }
	osExit = func(code int) { t.Fatalf("unexpected exit %d", code) }

	assert.NotPanics(t, func() { main() })
}

// TestDefaultSetupSessionStore verifica que o setup padrão de sessão configura um store não nulo.
func TestDefaultSetupSessionStore(t *testing.T) {
	restoreMainHooks(t)

	pool, err := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/bridopen?connect_timeout=1")
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	sm := scs.New()
	// Chama o hook de produção (intervalo de limpeza do pgxstore). Interrompe a limpeza
	// imediatamente substituindo o store após o wiring para que ticks em background não disputem com o teste.
	setupSessionStore(sm, pool)
	require.NotNil(t, sm.Store)
	sm.Store = nil
}

// TestDefaultListenAndServe verifica que defaultListenAndServe propaga ErrServerClosed ao fechar o servidor.
func TestDefaultListenAndServe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	server := &http.Server{
		Addr:    addr,
		Handler: http.NotFoundHandler(),
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = server.Close()
	}()
	err = defaultListenAndServe(server)
	assert.ErrorIs(t, err, http.ErrServerClosed)
}

func newRouteMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return mock
}

func expectCleanupOK(mock pgxmock.PgxPoolIface) {
	mock.ExpectExec(`delete from public.users_confirmation_tokens`).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec(`delete from public.users_two_factor_challenges`).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
}

// TestLoadRoutesRegistersStaticAndPublicRoutes verifica registro de estáticos, rota pública e 404 para caminho desconhecido.
func TestLoadRoutesRegistersStaticAndPublicRoutes(t *testing.T) {
	mock := newRouteMockPool(t)
	expectCleanupOK(mock)

	session := scs.New()
	mail := mailer.NewConsoleMailService("from@bridopen.com")
	handler := LoadRoutes(session, mail, mock, "http://localhost:5000", "mfa-secret", t.TempDir(), "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/js/core/index.js", nil)
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Body.Bytes())

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/privacy", nil)
	req.Host = "example.com"
	session.LoadAndSave(handler).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Handler curinga "/" retorna ErrNotFound para caminhos desconhecidos.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/no-such-bridopen-route", nil)
	req.Host = "example.com"
	session.LoadAndSave(handler).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLoadRoutesWithClamAVAndCleanupErrors verifica que LoadRoutes segue funcional mesmo com erros de cleanup e ClamAV configurado.
func TestLoadRoutesWithClamAVAndCleanupErrors(t *testing.T) {
	mock := newRouteMockPool(t)
	mock.ExpectExec(`delete from public.users_confirmation_tokens`).
		WillReturnError(errors.New("token cleanup failed"))
	mock.ExpectExec(`delete from public.users_two_factor_challenges`).
		WillReturnError(errors.New("challenge cleanup failed"))

	session := scs.New()
	mail := mailer.NewConsoleMailService("from@bridopen.com")
	handler := LoadRoutes(session, mail, mock, "http://localhost:5000", "mfa-secret", t.TempDir(), "127.0.0.1:3310")
	require.NotNil(t, handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/terms", nil)
	wrapped := session.LoadAndSave(handler)
	wrapped.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

type subFSError struct{}

func (subFSError) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func (subFSError) Sub(dir string) (fs.FS, error) {
	return nil, errors.New("static subtree missing")
}

// TestLoadRoutesStaticFSPanic verifica que LoadRoutes entra em pânico se a subárvore static do FS falhar.
func TestLoadRoutesStaticFSPanic(t *testing.T) {
	original := viewsFS
	t.Cleanup(func() { viewsFS = original })
	viewsFS = subFSError{}

	mock := newRouteMockPool(t)
	session := scs.New()
	mail := mailer.NewConsoleMailService("from@bridopen.com")

	require.Panics(t, func() {
		LoadRoutes(session, mail, mock, "http://localhost:5000", "mfa", t.TempDir(), "")
	})
}

// TestLoadRoutesUsesEmbeddedViewsFS verifica que LoadRoutes aceita o filesystem de views embutido.
func TestLoadRoutesUsesEmbeddedViewsFS(t *testing.T) {
	original := viewsFS
	t.Cleanup(func() { viewsFS = original })
	viewsFS = views.Files

	mock := newRouteMockPool(t)
	expectCleanupOK(mock)

	session := scs.New()
	handler := LoadRoutes(session, mailer.NewConsoleMailService("from@bridopen.com"), mock, "http://localhost:5000", "mfa", t.TempDir(), " ")
	require.NotNil(t, handler)

	_, err := fs.Sub(viewsFS, "static")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestIPRateLimiterAllowWithinAndOverLimit verifica limite por IP dentro/acima da cota e limpeza de entradas expiradas.
func TestIPRateLimiterAllowWithinAndOverLimit(t *testing.T) {
	t.Parallel()

	limiter := NewIPRateLimiter(2, time.Minute)
	now := time.Now()

	assert.True(t, limiter.allow("1.2.3.4", now))
	assert.True(t, limiter.allow("1.2.3.4", now.Add(time.Second)))
	assert.False(t, limiter.allow("1.2.3.4", now.Add(2*time.Second)))
	assert.True(t, limiter.allow("9.9.9.9", now))

	// Entradas expiradas fora da janela são descartadas.
	limiter = NewIPRateLimiter(1, time.Second)
	assert.True(t, limiter.allow("exp", now))
	assert.True(t, limiter.allow("exp", now.Add(2*time.Second)))
}

// TestIPRateLimiterHTTPLimit verifica que o middleware HTTP responde 429 após exceder o limite por IP.
func TestIPRateLimiterHTTPLimit(t *testing.T) {
	t.Parallel()

	limiter := NewIPRateLimiter(1, time.Minute)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := limiter.Limit(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:1234"

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	assert.Equal(t, http.StatusOK, rec1.Code)

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

// TestSecurityHeaders verifica que securityHeaders define os cabeçalhos de segurança esperados.
func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := securityHeaders(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	assert.Contains(t, rec.Header().Get("Permissions-Policy"), "camera=()")
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "default-src 'self'")
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// TestCsrfErrorPageForPath verifica o ActionHref da página de erro CSRF conforme o caminho da requisição.
func TestCsrfErrorPageForPath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/user/signup", csrfErrorPageForPath("/user/signup").ActionHref)
	assert.Equal(t, "/user/forgetpassword", csrfErrorPageForPath("/user/forgetpassword").ActionHref)
	assert.Equal(t, "/user/signin", csrfErrorPageForPath("/user/signin").ActionHref)
	assert.Equal(t, "/", csrfErrorPageForPath("/notes").ActionHref)
}

type failingWriter struct {
	http.ResponseWriter
}

func (f *failingWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

// TestHandleCSRFErrorReasonNilAndRenderOK verifica renderização da página CSRF com status 403 quando não há reason.
func TestHandleCSRFErrorReasonNilAndRenderOK(t *testing.T) {
	session := scs.New()
	rt := render.NewRender(session, "http://example.com")
	handler := session.LoadAndSave(handleCSRFError(rt))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/user/signin", nil)
	// Host diferente de localhost força templates do FS embutido.
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "Voltar para o login")
}

// TestHandleCSRFErrorRenderFailure verifica que falha ao escrever a página CSRF ainda responde 403.
func TestHandleCSRFErrorRenderFailure(t *testing.T) {
	session := scs.New()
	rt := render.NewRender(session, "http://example.com")
	inner := handleCSRFError(rt)
	handler := session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(&failingWriter{ResponseWriter: w}, r)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestCsrfMiddlewareBlocksUnsafeRequest verifica que o middleware CSRF bloqueia POST sem token com página de erro.
func TestCsrfMiddlewareBlocksUnsafeRequest(t *testing.T) {
	session := scs.New()
	csrfKey := "01234567890123456789012345678901"
	mw := csrfMiddleware(session, "http://example.com", csrfKey, false, []string{"example.com"})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := session.LoadAndSave(mw(next))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/user/signup", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "Voltar para o cadastro")
}

// TestCsrfMiddlewareAllowsSafeGet verifica que o middleware CSRF permite requisições GET seguras.
func TestCsrfMiddlewareAllowsSafeGet(t *testing.T) {
	session := scs.New()
	csrfKey := "01234567890123456789012345678901"
	mw := csrfMiddleware(session, "http://example.com", csrfKey, false, []string{"example.com"})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := session.LoadAndSave(mw(next))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
