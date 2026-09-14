package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	usermodel "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	userservice "github.com/engenheiroaraujo/bridopen/internal/handlers/users/service"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
)

func (uph *userPersonalHandler) Security(w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, meSecurityTwoStepVerificationPath, http.StatusSeeOther)
	return nil
}

func (uph *userPersonalHandler) SecurityTwoStepVerification(w http.ResponseWriter, r *http.Request) error {
	return uph.renderSecurityPage(w, r, http.StatusOK, meSecurityTwoStepVerificationPath, meTwoStepVerificationSection)
}

func (uph *userPersonalHandler) ActiveSessions(w http.ResponseWriter, r *http.Request) error {
	return uph.renderSecurityPage(w, r, http.StatusOK, meSecurityActiveSessionsPath, meActiveSessionsSection)
}

func (uph *userPersonalHandler) renderSecurityPage(w http.ResponseWriter, r *http.Request, status int, navPath, focusSection string) error {
	userID := uph.getUserIDFromSession(r)
	currentSessionID := uph.session.GetString(r.Context(), "userSessionId")
	sessionLimit := activeSessionLimitFromRequest(r)

	page, err := uph.accountSvc.BuildSecurityPage(r.Context(), userID, currentSessionID, sessionLimit)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		return err
	}
	page.NavPath = navPath
	page.FocusSection = focusSection

	return uph.render.RenderPage(w, r, status, "me-security.html", page)
}

func activeSessionLimitFromRequest(r *http.Request) int {
	limit, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("sessions")))
	if err != nil {
		return 0
	}
	return userservice.NormalizeActiveSessionLimit(limit)
}

func (uph *userPersonalHandler) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}

	userID := uph.getUserIDFromSession(r)
	currentSessionID := uph.session.GetString(r.Context(), "userSessionId")
	if currentSessionID == "" {
		if err := sessionDestroy(uph.session, r.Context()); err != nil {
			return err
		}
		http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
		return nil
	}

	if err := uph.accountSvc.RevokeOtherSessions(r.Context(), userID, currentSessionID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.WithStatus(errors.New("sessão atual não encontrada"), http.StatusNotFound)
		}
		return err
	}

	uph.audit.RecordSuccess(r.Context(), userID, audit.EventSecurityTokenRevoked, audit.EntityUser, userID)
	slog.Info("outras sessoes encerradas", slog.Int64("user_id", userID))
	http.Redirect(w, r, meSecurityActiveSessionsPath, http.StatusSeeOther)
	return nil
}

func (uph *userPersonalHandler) RevokeSession(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}

	userID := uph.getUserIDFromSession(r)
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	currentSessionID := strings.TrimSpace(uph.session.GetString(r.Context(), "userSessionId"))
	if sessionID == "" {
		return apperrors.WithStatus(errors.New("sessão não encontrada"), http.StatusNotFound)
	}

	if err := uph.accountSvc.RevokeSession(r.Context(), userID, sessionID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.WithStatus(errors.New("sessão não encontrada"), http.StatusNotFound)
		}
		return err
	}

	uph.audit.RecordSuccess(r.Context(), userID, audit.EventSecurityTokenRevoked, audit.EntityUser, userID)
	slog.Info("sessao encerrada pelo usuario", slog.Int64("user_id", userID), slog.Bool("current_session", sessionID == currentSessionID))
	if sessionID == currentSessionID {
		if err := sessionDestroy(uph.session, r.Context()); err != nil {
			return err
		}
		http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
		return nil
	}

	http.Redirect(w, r, meSecurityActiveSessionsPath, http.StatusSeeOther)
	return nil
}

func (uph *userPersonalHandler) SecurityTwoFactorMethod(w http.ResponseWriter, r *http.Request) error {
	userID := uph.getUserIDFromSession(r)
	method := strings.TrimSpace(r.PathValue("method"))

	title, description, ok := twoFactorMethodPageContent(method)
	if !ok {
		return apperrors.WithStatus(errors.New("método de verificação em duas etapas inválido"), http.StatusNotFound)
	}

	page, err := uph.accountSvc.BuildMePage(r.Context(), userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		return err
	}
	page.NavPath = meSecurityTwoStepVerificationPath
	page.SetSelectedTwoFactorMethod(method, title, description)

	return uph.render.RenderPage(w, r, http.StatusOK, "me-security-two-factor-method.html", page)
}

func twoFactorMethodPageContent(method string) (string, string, bool) {
	switch method {
	case usermodel.TwoFactorMethodEmail:
		return "Código por e-mail", "Receba um código de verificação no seu e-mail cadastrado.", true
	case usermodel.TwoFactorMethodTOTP:
		return "Aplicativo autenticador", "Use Google Authenticator, Microsoft Authenticator, Authy, 1Password, Bitwarden, FreeOTP ou outro app compatível com TOTP.", true
	default:
		return "", "", false
	}
}
