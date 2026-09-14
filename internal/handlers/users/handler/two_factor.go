package handlers

import (
	"encoding/json"
	"errors"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
	userservice "github.com/engenheiroaraujo/bridopen/internal/handlers/users/service"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
)

type twoFactorHandler struct {
	render  *render.RenderTemplate
	session *scs.SessionManager
	service *userservice.TwoFactorService
	audit   audit.Recorder
}

type twoFactorResponse struct {
	OK            bool                              `json:"ok"`
	Message       string                            `json:"message"`
	Status        *userservice.TwoFactorStatusView  `json:"status,omitempty"`
	Setup         *userservice.TwoFactorSetupResult `json:"setup,omitempty"`
	RecoveryCodes []string                          `json:"recovery_codes,omitempty"`
}

func NewTwoFactorHandler(render *render.RenderTemplate, session *scs.SessionManager, service *userservice.TwoFactorService, auditRecorder audit.Recorder) *twoFactorHandler {
	return &twoFactorHandler{
		render:  render,
		session: session,
		service: service,
		audit:   auditRecorder,
	}
}

func (h *twoFactorHandler) Start(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID := h.session.GetInt64(r.Context(), "userId")
	method := strings.TrimSpace(r.PostFormValue("method"))
	setup, err := h.service.StartSetup(r.Context(), userID, method)
	if err != nil {
		h.audit.RecordFailure(r.Context(), userID, audit.EventMFAEnableFailed, audit.EntityUser, userID)
		return h.writeError(w, r, err)
	}

	h.audit.RecordSuccess(r.Context(), userID, audit.EventMFASetupStarted, audit.EntityUser, userID)
	slog.Info("configuracao de 2fa iniciada", slog.Int64("user_id", userID), slog.String("method", method))
	status, err := h.service.Status(r.Context(), userID)
	if err != nil {
		return err
	}
	return h.writeJSON(w, http.StatusOK, twoFactorResponse{
		OK:      true,
		Message: setup.Message,
		Status:  status,
		Setup:   setup,
	})
}

func (h *twoFactorHandler) Verify(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID := h.session.GetInt64(r.Context(), "userId")
	method := strings.TrimSpace(r.PostFormValue("method"))
	code := strings.TrimSpace(r.PostFormValue("code"))
	recoveryCodes, err := h.service.VerifySetupWithRecoveryCodes(r.Context(), userID, method, code)
	if err != nil {
		h.audit.RecordFailure(r.Context(), userID, audit.EventMFAEnableFailed, audit.EntityUser, userID)
		return h.writeError(w, r, err)
	}

	h.audit.RecordSuccess(r.Context(), userID, audit.EventMFAEnabled, audit.EntityUser, userID)
	slog.Info("2fa habilitado", slog.Int64("user_id", userID), slog.String("method", method), slog.Int("recovery_codes", len(recoveryCodes)))
	if err := sessionRenewToken(h.session, r.Context()); err != nil {
		return err
	}
	status, err := h.service.Status(r.Context(), userID)
	if err != nil {
		return err
	}
	return h.writeJSON(w, http.StatusOK, twoFactorResponse{
		OK:            true,
		Message:       "Verificação em duas etapas ativada com sucesso.",
		Status:        status,
		RecoveryCodes: recoveryCodes,
	})
}

func (h *twoFactorHandler) RegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}

	userID := h.session.GetInt64(r.Context(), "userId")
	recoveryCodes, err := h.service.GenerateRecoveryCodes(r.Context(), userID)
	if err != nil {
		h.audit.RecordFailure(r.Context(), userID, audit.EventSecurityTokenGenerated, audit.EntityUser, userID)
		return h.writeError(w, r, err)
	}

	h.audit.RecordSuccess(r.Context(), userID, audit.EventSecurityTokenGenerated, audit.EntityUser, userID)
	slog.Info("codigos de recuperacao 2fa regenerados", slog.Int64("user_id", userID), slog.Int("count", len(recoveryCodes)))
	status, err := h.service.Status(r.Context(), userID)
	if err != nil {
		return err
	}
	return h.writeJSON(w, http.StatusOK, twoFactorResponse{
		OK:            true,
		Message:       "Novos códigos de recuperação gerados.",
		Status:        status,
		RecoveryCodes: recoveryCodes,
	})
}

func (h *twoFactorHandler) DisableStart(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}

	userID := h.session.GetInt64(r.Context(), "userId")
	setup, err := h.service.StartDisable(r.Context(), userID)
	if err != nil {
		h.audit.RecordFailure(r.Context(), userID, audit.EventMFADisableFailed, audit.EntityUser, userID)
		return h.writeError(w, r, err)
	}

	h.audit.RecordSuccess(r.Context(), userID, audit.EventMFADisableStarted, audit.EntityUser, userID)
	slog.Info("desabilitacao de 2fa iniciada", slog.Int64("user_id", userID), slog.String("method", setup.Method))
	status, err := h.service.Status(r.Context(), userID)
	if err != nil {
		return err
	}
	return h.writeJSON(w, http.StatusOK, twoFactorResponse{
		OK:      true,
		Message: setup.Message,
		Status:  status,
		Setup:   setup,
	})
}

func (h *twoFactorHandler) DisableVerify(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID := h.session.GetInt64(r.Context(), "userId")
	code := strings.TrimSpace(r.PostFormValue("code"))
	if err := h.service.VerifyDisable(r.Context(), userID, code); err != nil {
		h.audit.RecordFailure(r.Context(), userID, audit.EventMFADisableFailed, audit.EntityUser, userID)
		return h.writeError(w, r, err)
	}

	h.audit.RecordSuccess(r.Context(), userID, audit.EventMFADisabled, audit.EntityUser, userID)
	slog.Info("2fa desabilitado", slog.Int64("user_id", userID))
	if err := sessionRenewToken(h.session, r.Context()); err != nil {
		return err
	}
	status, err := h.service.Status(r.Context(), userID)
	if err != nil {
		return err
	}
	return h.writeJSON(w, http.StatusOK, twoFactorResponse{
		OK:      true,
		Message: "Verificação em duas etapas desativada.",
		Status:  status,
	})
}

func (h *twoFactorHandler) writeError(w http.ResponseWriter, r *http.Request, err error) error {
	userID := h.session.GetInt64(r.Context(), "userId")
	statusView, statusErr := h.service.Status(r.Context(), userID)
	if statusErr != nil {
		statusView = nil
	}

	httpStatus := http.StatusUnprocessableEntity
	switch {
	case errors.Is(err, apperrors.ErrTwoFactorRequiresConfirmed):
		httpStatus = http.StatusForbidden
	case errors.Is(err, apperrors.ErrTwoFactorAlreadyEnabled):
		httpStatus = http.StatusConflict
	case errors.Is(err, apperrors.ErrTwoFactorTooManyAttempts), errors.Is(err, apperrors.ErrTwoFactorCooldown):
		httpStatus = http.StatusTooManyRequests
	}

	return h.writeJSON(w, httpStatus, twoFactorResponse{
		OK:      false,
		Message: apperrors.TwoFactorMessage(err),
		Status:  statusView,
	})
}

func (h *twoFactorHandler) writeJSON(w http.ResponseWriter, status int, data twoFactorResponse) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}
