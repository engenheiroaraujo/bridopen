package handlers

import (
	"errors"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

func (uh *userHandler) ForgetPasswordForm(w http.ResponseWriter, r *http.Request) error {
	return uh.renderForgetPasswordForm(w, r, http.StatusOK, userdto.UserRequest{})
}

func (uh *userHandler) ForgetPassword(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	email := strings.TrimSpace(r.PostFormValue("email"))
	data := userdto.NewUserRequest(email, "")

	if email == "" {
		data.AddFieldError("email", "E-mail é obrigatório")
	} else if !validations.IsEmailValid(email) {
		data.AddFieldError("email", "E-mail inválido")
	}
	uh.validateCaptcha(r, &data)

	if !data.Valid() {
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorValidation, audit.EntityUser, 0)
		return uh.renderForgetPasswordForm(w, r, http.StatusUnprocessableEntity, data)
	}

	uh.audit.RecordSuccess(r.Context(), 0, audit.EventSecurityTokenGenerated, audit.EntityUser, 0)

	token, recipients, err := uh.account.RequestPasswordReset(r.Context(), email)
	if err != nil && !errors.Is(err, apperrors.ErrUserInactive) {
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorProcessing, audit.EntityUser, 0)
		return err
	}

	if token != "" {
		body, err := uh.render.RenderMailBody(r, "forgetpassword.html", map[string]string{"token": token})
		if err != nil {
			return err
		}
		if err := uh.mail.Send(mailer.MailMessage{
			To:      recipients,
			Subject: "Redefinir senha no Bridopen",
			IsHtml:  true,
			Body:    body,
		}); err != nil {
			return err
		}
	}

	slog.Info("solicitacao de redefinicao de senha processada", slog.Bool("email_enviado", token != ""))
	uh.audit.RecordSuccess(r.Context(), 0, audit.EventUserPasswordResetRequested, audit.EntityUser, 0)
	return uh.render.RenderAuthPage(w, r, http.StatusOK, "user-forget-password.html", userdto.SignupPage{Success: true})
}

func (uh *userHandler) ResetPasswordForm(w http.ResponseWriter, r *http.Request) error {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		return uh.renderInvalidPasswordToken(w, r, token)
	}

	err := uh.account.ValidatePasswordResetToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uh.renderInvalidPasswordToken(w, r, token)
		}
		slog.Error("erro ao obter token de redefinicao", "err", err)
		uh.audit.RecordFailure(r.Context(), 0, audit.EventSecurityTokenRevoked, audit.EntityUser, 0)
		return uh.render.RenderAuthPage(w, r, http.StatusInternalServerError, "generic-error.html", nil)
	}
	uh.audit.RecordSuccess(r.Context(), 0, audit.EventSecurityTokenGenerated, audit.EntityUser, 0)
	return uh.render.RenderAuthPage(w, r, http.StatusOK, "user-reset-password.html", userdto.SignupPage{Token: token, Form: userdto.UserRequest{}})
}

func (uh *userHandler) renderInvalidPasswordToken(w http.ResponseWriter, r *http.Request, token string) error {
	form := userdto.NewUserRequest("", "")
	form.AddFieldError("link", "O token de redefinição de senha é inválido ou está expirado.")
	uh.audit.RecordFailure(r.Context(), 0, audit.EventSecurityTokenRevoked, audit.EntityUser, 0)
	return uh.render.RenderAuthPage(w, r, http.StatusUnprocessableEntity, "user-reset-password.html", userdto.SignupPage{Token: token, Form: form})
}

func (uh *userHandler) ResetPassword(w http.ResponseWriter, r *http.Request) error {
	data := userdto.NewUserRequest("", "")
	firstPassword := strings.TrimSpace(r.PostFormValue("password"))
	secondPassword := strings.TrimSpace(r.PostFormValue("password_confirm"))
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		return uh.renderInvalidPasswordToken(w, r, token)
	}

	password, valid := userdto.PasswordsMatch(firstPassword, secondPassword)
	if !valid {
		data.AddFieldError("password", "as senhas não conferem")
	}
	if msg := validations.PasswordLengthError("A senha", password); msg != "" {
		data.AddFieldError("password", msg)
	}

	if !data.Valid() {
		firstPassword = ""
		secondPassword = ""
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorValidation, audit.EntityUser, 0)
		return uh.render.RenderAuthPage(w, r, http.StatusUnprocessableEntity, "user-reset-password.html", userdto.SignupPage{Form: data})
	}

	email, userID, err := uh.account.ResetPassword(r.Context(), token, password)
	if err != nil {
		slog.Error("erro ao atualizar senha do usuario", "err", err)
		data.AddFieldError("validation", "não foi possível alterar a senha. Solicite uma nova alteração.")
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorProcessing, audit.EntityUser, 0)
		return uh.render.RenderAuthPage(w, r, http.StatusUnprocessableEntity, "user-reset-password.html", userdto.SignupPage{Token: token, Form: data})
	}

	body, err := uh.render.RenderMailBody(r, "rememberpassword.html", map[string]string{})
	if err != nil {
		return err
	}

	if err := uh.mail.Send(mailer.MailMessage{
		To:      []string{email},
		Subject: "Redefinição de senha com sucesso",
		IsHtml:  true,
		Body:    body,
	}); err != nil {
		slog.Warn("falha ao enviar email de senha redefinida", slog.String("err", err.Error()))
	}

	slog.Info("senha redefinida com sucesso")
	uh.audit.RecordSuccess(r.Context(), userID, audit.EventUserPasswordChanged, audit.EntityUser, userID)
	return uh.render.RenderAuthPage(w, r, http.StatusOK, "user-reset-password.html", userdto.SignupPage{Success: true, Form: userdto.UserRequest{}})
}

func (uph *userPersonalHandler) PasswordSave(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID := uph.getUserIDFromSession(r)
	currentPassword := firstFormValue(r, "account_current_secret", "current_password")
	newPassword := firstFormValue(r, "account_new_secret", "password")
	passwordConfirm := firstFormValue(r, "account_new_secret_confirm", "password_confirm")

	fieldErrors, err := uph.accountSvc.ChangePassword(r.Context(), userID, currentPassword, newPassword, passwordConfirm)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		if errors.Is(err, apperrors.ErrUserInactive) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventUserPasswordChanged, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta inativa"), http.StatusForbidden)
		}
		uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorProcessing, audit.EntityUser, userID)
		return err
	}

	if len(fieldErrors) > 0 {
		uph.audit.RecordFailure(r.Context(), userID, audit.EventUserPasswordChanged, audit.EntityUser, userID)
		return uph.renderMePasswordForm(w, r, http.StatusUnprocessableEntity, userID, fieldErrors, false)
	}

	if err := sessionRenewToken(uph.session, r.Context()); err != nil {
		return err
	}
	uph.audit.RecordSuccess(r.Context(), userID, audit.EventUserPasswordChanged, audit.EntityUser, userID)
	return uph.renderMePasswordForm(w, r, http.StatusOK, userID, nil, true)
}

func (uph *userPersonalHandler) renderMePasswordForm(w http.ResponseWriter, r *http.Request, status int, userID int64, fieldErrors map[string]string, success bool) error {
	page, err := uph.accountSvc.BuildMePage(r.Context(), userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		return err
	}
	page.NavPath = meOverviewAccountSettingsPath
	page.OpenPasswordModal = !success
	page.PasswordChangeSuccess = success

	for field, message := range fieldErrors {
		page.AddFieldError(field, message)
	}

	return uph.render.RenderPage(w, r, status, "me.html", page)
}
