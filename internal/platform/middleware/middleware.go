package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
)

// userSessionChecker é o contrato mínimo que o middleware precisa da camada de
// sessão. Declarado aqui, no consumidor: o pacote de repositório não o conhece,
// apenas o satisfaz — e o middleware deixa de depender do módulo de users.
type userSessionChecker interface {
	IsActive(ctx context.Context, userID int64, sessionID string) (bool, error)
	Touch(ctx context.Context, sessionID string) error
}

type authMiddleware struct {
	session     *scs.SessionManager
	userSession userSessionChecker
}

type errorHandlerMiddleware struct {
	render *render.RenderTemplate
}

type responseLogWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func NewErrorHandlerMiddleware(render *render.RenderTemplate) *errorHandlerMiddleware {
	return &errorHandlerMiddleware{render: render}
}

func NewAuthMiddleware(session *scs.SessionManager, userSession userSessionChecker) *authMiddleware {
	return &authMiddleware{session: session, userSession: userSession}
}

func AccessLog(session *scs.SessionManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &responseLogWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if rec.status >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if rec.status >= http.StatusBadRequest {
			level = slog.LevelWarn
		}

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("duration", time.Since(start)),
			slog.String("remote_addr", r.RemoteAddr),
		}
		if requestID := r.Header.Get("X-Request-Id"); requestID != "" {
			attrs = append(attrs, slog.String("request_id", requestID))
		}
		if session != nil {
			if userID := session.GetInt64(r.Context(), "userId"); userID != 0 {
				attrs = append(attrs, slog.Int64("user_id", userID))
			}
		}

		slog.LogAttrs(r.Context(), level, "http request", attrs...)
	})
}

func (w *responseLogWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseLogWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func (ah *authMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := ah.session.GetInt64(r.Context(), "userId")
		sessionID := ah.session.GetString(r.Context(), "userSessionId")
		if userID == 0 || sessionID == "" {
			slog.Warn("usuario nao esta logado")
			_ = ah.session.Destroy(r.Context())
			http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
			return
		}

		if ah.userSession != nil {
			active, err := ah.userSession.IsActive(r.Context(), userID, sessionID)
			if err != nil {
				slog.Error("falha ao validar sessao ativa", slog.String("err", err.Error()))
				http.Error(w, apperrors.ErrInternal.Error(), http.StatusInternalServerError)
				return
			}
			if !active {
				slog.Warn("sessao revogada ou expirada", slog.Int64("user_id", userID))
				_ = ah.session.Destroy(r.Context())
				http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
				return
			}
			ah.touchSession(r, sessionID)
		}

		next.ServeHTTP(w, r)
	})
}

func (ah *authMiddleware) touchSession(r *http.Request, sessionID string) {
	const touchEvery = int64(time.Minute / time.Second)

	now := time.Now().Unix()
	lastTouch := ah.session.GetInt64(r.Context(), "userSessionLastTouchUnix")
	if lastTouch > 0 && now-lastTouch < touchEvery {
		return
	}
	if err := ah.userSession.Touch(r.Context(), sessionID); err != nil {
		slog.Warn("falha ao atualizar ultimo acesso da sessao", slog.String("err", err.Error()))
		return
	}
	ah.session.Put(r.Context(), "userSessionLastTouchUnix", now)
}

func (em *errorHandlerMiddleware) HandlerError(next func(w http.ResponseWriter, r *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := next(w, r); err != nil {
			renderErrorPage := func(status int, page string, fallback string) {
				// 404 e generic-error usam o layout auth.
				if renderErr := em.render.RenderAuthPage(w, r, status, page, fallback); renderErr != nil {
					slog.Error("falha ao renderizar pagina de erro", "page", page, "err", renderErr)
					http.Error(w, fallback, status)
				}
			}

			// Erros de contexto normalmente indicam que o cliente fechou a conexao
			// ou que a requisicao expirou. Nesses casos nao vale responder nem logar
			// como falha interna do servidor.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}

			// Erros com status HTTP conhecido permitem que o handler escolha a
			// resposta correta sem duplicar regra de renderizacao em cada rota.
			if statusErr, ok := errors.AsType[apperrors.StatusError](err); ok {
				if statusErr.StatusCode() == http.StatusNotFound {
					renderErrorPage(statusErr.StatusCode(), "404.html", statusErr.Error())
					return
				}
				if statusErr.StatusCode() >= http.StatusInternalServerError {
					slog.Error("handler falhou", "status", statusErr.StatusCode(), "err", err)
					renderErrorPage(statusErr.StatusCode(), "generic-error.html", apperrors.ErrInternal.Error())
					return
				}
				http.Error(w, err.Error(), statusErr.StatusCode())
				return
			}

			// Erros vindos da camada de repositorio podem conter detalhes internos
			// do banco. Logamos o erro real e mostramos uma mensagem generica.
			if repoErr, ok := errors.AsType[*apperrors.RepositoryError](err); ok {
				slog.Error(repoErr.Error())
				renderErrorPage(http.StatusInternalServerError, "generic-error.html", apperrors.ErrInternal.Error())
				return
			}

			// Qualquer erro nao mapeado tambem vira erro interno. O detalhe fica
			// apenas no log para nao expor informacao tecnica ao usuario.
			slog.Error("handler falhou", "err", err)
			renderErrorPage(http.StatusInternalServerError, "generic-error.html", apperrors.ErrInternal.Error())
		}
	})
}
