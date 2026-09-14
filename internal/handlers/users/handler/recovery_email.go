package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
)

func (uph *userPersonalHandler) RecoveryEmailSave(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID := uph.getUserIDFromSession(r)
	currentPassword := firstFormValue(r, "recovery_current_secret", "current_password")
	recoveryEmail := strings.TrimSpace(r.PostFormValue("recovery_email"))

	fieldErrors, primaryEmail, pendingEmail, token, err := uph.accountSvc.RequestRecoveryEmailChange(r.Context(), userID, currentPassword, recoveryEmail)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		if errors.Is(err, apperrors.ErrUserInactive) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventUserUpdated, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta inativa"), http.StatusForbidden)
		}
		uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorProcessing, audit.EntityUser, userID)
		return err
	}

	if len(fieldErrors) > 0 {
		uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorValidation, audit.EntityUser, userID)
		return uph.renderMeRecoveryEmailForm(w, r, http.StatusUnprocessableEntity, userID, recoveryEmail, fieldErrors, false)
	}

	body, err := uph.render.RenderMailBody(r, "recovery-email-confirmation.html", map[string]string{
		"token": token,
		"email": pendingEmail,
	})
	if err != nil {
		_ = uph.accountSvc.CancelRecoveryEmailChange(r.Context(), userID, pendingEmail)
		return err
	}
	if err := uph.mail.Send(mailer.MailMessage{
		To:      []string{pendingEmail},
		Subject: "Confirme seu e-mail de recuperação no Bridopen",
		IsHtml:  true,
		Body:    body,
	}); err != nil {
		_ = uph.accountSvc.CancelRecoveryEmailChange(r.Context(), userID, pendingEmail)
		return err
	}

	slog.Info("confirmacao de e-mail de recuperacao enviada", slog.Int64("user_id", userID), slog.String("primary_email", primaryEmail))
	uph.audit.RecordSuccess(r.Context(), userID, audit.EventSecurityTokenGenerated, audit.EntityUser, userID)
	return uph.renderMeRecoveryEmailForm(w, r, http.StatusOK, userID, pendingEmail, nil, true)
}

func (uph *userPersonalHandler) renderMeRecoveryEmailForm(w http.ResponseWriter, r *http.Request, status int, userID int64, recoveryEmail string, fieldErrors map[string]string, success bool) error {
	page, err := uph.accountSvc.BuildMePage(r.Context(), userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		return err
	}
	page.NavPath = meOverviewAccountSettingsPath
	page.OpenRecoveryEmailModal = !success
	page.RecoveryEmailChangeRequested = success
	if recoveryEmail != "" {
		page.PendingRecoveryEmail = strings.TrimSpace(recoveryEmail)
	}

	for field, message := range fieldErrors {
		page.AddFieldError(field, message)
	}

	return uph.render.RenderPage(w, r, status, "me.html", page)
}
