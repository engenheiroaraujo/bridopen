package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	usermodel "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/gorilla/csrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponseLogWriterCapturesStatusAndBytes verifica captura de bytes escritos e que o segundo WriteHeader é ignorado.
func TestResponseLogWriterCapturesStatusAndBytes(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := &responseLogWriter{ResponseWriter: rec, status: http.StatusOK}
	n, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, w.bytes)
	assert.Equal(t, http.StatusOK, w.status)

	w.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusOK, w.status, "segunda chamada WriteHeader deve ser ignorada")
}

// TestAccessLogSkipsStatic verifica que AccessLog deixa passar rotas /static sem alterar a resposta.
func TestAccessLogSkipsStatic(t *testing.T) {
	t.Parallel()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := AccessLog(nil, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	handler.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// TestHandlerErrorContextCanceledNoWrite verifica que context.Canceled não gera escrita de resposta de erro.
func TestHandlerErrorContextCanceledNoWrite(t *testing.T) {
	t.Parallel()

	em := NewErrorHandlerMiddleware(nil)
	handler := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return context.Canceled
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

// TestHandlerErrorClientStatusUsesHTTPError verifica que erros 4xx usam http.Error com a mensagem do StatusError.
func TestHandlerErrorClientStatusUsesHTTPError(t *testing.T) {
	t.Parallel()

	em := NewErrorHandlerMiddleware(nil)
	handler := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return apperrors.WithStatus(errors.New("bad request"), http.StatusBadRequest)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "bad request")
}

// TestHandlerErrorStatusNotFoundRenders404 verifica que ErrNotFound renderiza a página 404 com status correto.
func TestHandlerErrorStatusNotFoundRenders404(t *testing.T) {
	t.Parallel()

	session := scs.New()
	em := NewErrorHandlerMiddleware(render.NewRender(session, "http://example.com"))
	inner := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return apperrors.ErrPageNotFound
	})
	handler := csrf.Protect([]byte("01234567890123456789012345678901"))(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotEmpty(t, rec.Body.String())
}

// TestHandlerErrorRepositoryErrorIs500 verifica que erro de repositório resulta em resposta 500.
func TestHandlerErrorRepositoryErrorIs500(t *testing.T) {
	t.Parallel()

	session := scs.New()
	em := NewErrorHandlerMiddleware(render.NewRender(session, "http://example.com"))
	inner := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return apperrors.NewRepositoryError(errors.New("db"))
	})
	handler := csrf.Protect([]byte("01234567890123456789012345678901"))(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestRequireAuthRedirectsWhenNoSession verifica redirecionamento para login quando não há sessão autenticada.
func TestRequireAuthRedirectsWhenNoSession(t *testing.T) {
	t.Parallel()

	session := scs.New()
	auth := NewAuthMiddleware(session, nil)
	called := false
	inner := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	handler := session.LoadAndSave(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	handler.ServeHTTP(rec, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/user/signin", rec.Header().Get("Location"))
}

// TestRequireAuthAllowsActiveSession verifica que sessão ativa autoriza o handler e faz Touch da sessão.
func TestRequireAuthAllowsActiveSession(t *testing.T) {
	t.Parallel()

	session := scs.New()
	repo := &fakeUserSessionRepo{active: true}
	auth := NewAuthMiddleware(session, repo)

	var reached bool
	inner := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	handler := session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(10))
		session.Put(r.Context(), "userSessionId", "sess-1")
		inner.ServeHTTP(w, r)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	handler.ServeHTTP(rec, req)

	assert.True(t, reached)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, repo.touched)
}

// TestRequireAuthRevokedSessionRedirects verifica redirecionamento para login quando a sessão foi revogada.
func TestRequireAuthRevokedSessionRedirects(t *testing.T) {
	t.Parallel()

	session := scs.New()
	repo := &fakeUserSessionRepo{active: false}
	auth := NewAuthMiddleware(session, repo)

	var reached bool
	inner := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	handler := session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(10))
		session.Put(r.Context(), "userSessionId", "sess-1")
		inner.ServeHTTP(w, r)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	handler.ServeHTTP(rec, req)

	assert.False(t, reached)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/user/signin", rec.Header().Get("Location"))
}

// TestAccessLogNonStaticLevelsAndAttrs verifica que AccessLog registra respostas não estáticas em vários status.
func TestAccessLogNonStaticLevelsAndAttrs(t *testing.T) {
	t.Parallel()

	session := scs.New()
	cases := []struct {
		name   string
		status int
		withID bool
	}{
		{"ok_with_user", http.StatusOK, true},
		{"client_error", http.StatusBadRequest, false},
		{"server_error", http.StatusInternalServerError, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.withID {
					session.Put(r.Context(), "userId", int64(42))
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("body"))
			})
			handler := session.LoadAndSave(AccessLog(session, next))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
			req.Header.Set("X-Request-Id", "req-123")
			req.RemoteAddr = "127.0.0.1:1234"
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tc.status, rec.Code)
			assert.Equal(t, "body", rec.Body.String())
		})
	}
}

// TestRequireAuthIsActiveError verifica resposta 500 quando IsActive falha ao consultar a sessão.
func TestRequireAuthIsActiveError(t *testing.T) {
	t.Parallel()

	session := scs.New()
	repo := &fakeUserSessionRepo{activeErr: errors.New("db down")}
	auth := NewAuthMiddleware(session, repo)

	var reached bool
	inner := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	handler := session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(10))
		session.Put(r.Context(), "userSessionId", "sess-1")
		inner.ServeHTTP(w, r)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	handler.ServeHTTP(rec, req)

	assert.False(t, reached)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), apperrors.ErrInternal.Error())
}

// TestTouchSessionCooldownAndTouchError verifica tolerância a falha de Touch e cooldown que evita toques repetidos.
func TestTouchSessionCooldownAndTouchError(t *testing.T) {
	t.Parallel()

	session := scs.New()
	repo := &fakeUserSessionRepo{active: true, touchErr: errors.New("touch failed")}
	auth := NewAuthMiddleware(session, repo)

	inner := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// Primeiro request: Touch falha, lastTouch não é atualizado.
	handler := session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(10))
		session.Put(r.Context(), "userSessionId", "sess-1")
		inner.ServeHTTP(w, r)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 1, repo.touchCalls)

	// Touch bem-sucedido; em seguida o cooldown ignora um segundo Touch dentro de um minuto.
	repo.touchErr = nil
	cookie := rec.Result().Cookies()
	require.NotEmpty(t, cookie)

	handler2 := session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(10))
		session.Put(r.Context(), "userSessionId", "sess-1")
		inner.ServeHTTP(w, r)
	}))
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, c := range cookie {
		req2.AddCookie(c)
	}
	handler2.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusNoContent, rec2.Code)
	assert.Equal(t, 2, repo.touchCalls)

	cookie2 := rec2.Result().Cookies()
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, c := range cookie2 {
		req3.AddCookie(c)
	}
	if len(cookie2) == 0 {
		for _, c := range cookie {
			req3.AddCookie(c)
		}
	}
	handler2.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusNoContent, rec3.Code)
	assert.Equal(t, 2, repo.touchCalls, "cooldown should skip Touch")
}

// TestHandlerErrorDeadlineExceededNoWrite verifica que DeadlineExceeded não gera escrita de resposta de erro.
func TestHandlerErrorDeadlineExceededNoWrite(t *testing.T) {
	t.Parallel()

	em := NewErrorHandlerMiddleware(nil)
	handler := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return context.DeadlineExceeded
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

// TestHandlerErrorStatus5xxRendersGeneric verifica que StatusError 5xx renderiza página genérica com o status original.
func TestHandlerErrorStatus5xxRendersGeneric(t *testing.T) {
	t.Parallel()

	session := scs.New()
	em := NewErrorHandlerMiddleware(render.NewRender(session, "http://example.com"))
	inner := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return apperrors.WithStatus(errors.New("boom"), http.StatusBadGateway)
	})
	handler := csrf.Protect([]byte("01234567890123456789012345678901"))(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.NotEmpty(t, rec.Body.String())
}

// TestHandlerErrorGenericErrorRenders500 verifica que erro genérico sem status resulta em 500.
func TestHandlerErrorGenericErrorRenders500(t *testing.T) {
	t.Parallel()

	session := scs.New()
	em := NewErrorHandlerMiddleware(render.NewRender(session, "http://example.com"))
	inner := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return errors.New("unexpected")
	})
	handler := csrf.Protect([]byte("01234567890123456789012345678901"))(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	req.Host = "example.com"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestHandlerErrorRenderFailureFallback verifica fallback textual quando a renderização da página de erro falha.
func TestHandlerErrorRenderFailureFallback(t *testing.T) {
	t.Parallel()

	em := NewErrorHandlerMiddleware(render.NewRender(scs.New(), "http://example.com"))
	inner := em.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return apperrors.ErrPageNotFound
	})
	handler := csrf.Protect([]byte("01234567890123456789012345678901"))(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Host = "example.com"
	handler.ServeHTTP(&failingResponseWriter{ResponseWriter: rec}, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), apperrors.ErrPageNotFound.Error())
}

type failingResponseWriter struct {
	http.ResponseWriter
	failed bool
}

func (w *failingResponseWriter) Write(p []byte) (int, error) {
	if !w.failed {
		w.failed = true
		return 0, errors.New("write failed")
	}
	return w.ResponseWriter.Write(p)
}

type fakeUserSessionRepo struct {
	active     bool
	activeErr  error
	touched    bool
	touchErr   error
	touchCalls int
}

func (f *fakeUserSessionRepo) Create(context.Context, int64, string, string, string) error {
	return nil
}
func (f *fakeUserSessionRepo) ListActive(context.Context, int64, int) ([]usermodel.UserSession, error) {
	return nil, nil
}
func (f *fakeUserSessionRepo) IsActive(context.Context, int64, string) (bool, error) {
	if f.activeErr != nil {
		return false, f.activeErr
	}
	return f.active, nil
}
func (f *fakeUserSessionRepo) Touch(context.Context, string) error {
	f.touchCalls++
	f.touched = true
	return f.touchErr
}
func (f *fakeUserSessionRepo) RevokeOther(context.Context, int64, string) error { return nil }
func (f *fakeUserSessionRepo) Revoke(context.Context, int64, string) error      { return nil }
