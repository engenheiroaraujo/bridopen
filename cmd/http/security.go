package main

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/engenheiroaraujo/bridopen/internal/platform/key"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/gorilla/csrf"
)

type csrfErrorPage struct {
	ActionHref  string
	ActionLabel string
}

func csrfMiddleware(sessionManager *scs.SessionManager, baseURL, csrfKey string, secure bool, trustedOrigins []string) func(http.Handler) http.Handler {
	csrfErrorRender := render.NewRender(sessionManager, baseURL)
	csrfMiddleware := csrf.Protect(
		[]byte(csrfKey),
		csrf.CookieName("bridopen_csrf"),
		csrf.Path("/"),
		csrf.Secure(secure),
		csrf.HttpOnly(true),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.MaxAge(3600),
		csrf.TrustedOrigins(trustedOrigins),
		csrf.ErrorHandler(handleCSRFError(csrfErrorRender)),
	)
	return csrfMiddleware
}

func handleCSRFError(csrfErrorRender *render.RenderTemplate) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reason := csrf.FailureReason(r)
		reasonText := "unknown"
		if reason != nil {
			reasonText = reason.Error()
		}
		slog.Warn("csrf bloqueou a requisicao",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("origin", r.Header.Get("Origin")),
			slog.String("reason", reasonText),
		)
		page := csrfErrorPageForPath(r.URL.Path)
		if err := csrfErrorRender.RenderAuthPage(w, r, http.StatusForbidden, "csrf-error.html", page); err != nil {
			slog.Error("falha ao renderizar pagina de csrf", slog.String("err", err.Error()))
			http.Error(w, "Sessão expirada. Recarregue a página e tente novamente.", http.StatusForbidden)
		}
	})
}

func csrfErrorPageForPath(path string) csrfErrorPage {
	switch {
	case strings.HasPrefix(path, "/user/signup"):
		return csrfErrorPage{ActionHref: "/user/signup", ActionLabel: "Voltar para o cadastro"}
	case strings.HasPrefix(path, "/user/forgetpassword"):
		return csrfErrorPage{ActionHref: "/user/forgetpassword", ActionLabel: "Redefinir senha novamente"}
	case strings.HasPrefix(path, "/user/signin"):
		return csrfErrorPage{ActionHref: "/user/signin", ActionLabel: "Voltar para o login"}
	default:
		return csrfErrorPage{ActionHref: "/", ActionLabel: "Recarregar o app"}
	}
}

func SecurityHeaders(next http.Handler) http.Handler {
	return securityHeaders(next)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self' https://cdn.tailwindcss.com 'unsafe-inline'",
			"style-src 'self' 'unsafe-inline'",
			"img-src 'self' data:",
			"font-src 'self' data:",
			"connect-src 'self'",
			"base-uri 'self'",
			"form-action 'self'",
			"frame-ancestors 'none'",
		}, "; "))
		next.ServeHTTP(w, r)
	})
}

type ipRateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func NewIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (l *ipRateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(key.ClientIP(r), time.Now()) {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *ipRateLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	entries := l.requests[ip]
	kept := entries[:0]
	for _, ts := range entries {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}

	if len(kept) >= l.limit {
		l.requests[ip] = kept
		return false
	}

	l.requests[ip] = append(kept, now)
	return true
}
