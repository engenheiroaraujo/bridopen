package handlers

import (
	"errors"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

func (uh *userHandler) Signup(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	firstName := strings.TrimSpace(r.PostFormValue("first_name"))
	lastName := strings.TrimSpace(r.PostFormValue("last_name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	password := strings.TrimSpace(r.PostFormValue("password"))
	passwordConfirm := strings.TrimSpace(r.PostFormValue("password_confirm"))

	data := userdto.NewUserRequest(email, password)

	if firstName == "" || lastName == "" {
		data.AddFieldError("first_name", "Nome e sobrenome é obrigatório")
	}

	if email == "" {
		data.AddFieldError("email", "E-mail é obrigatório")

	} else if !validations.IsEmailValid(email) {
		data.AddFieldError("email", "E-mail inválido")
	}

	if password == "" {
		data.AddFieldError("password", "A senha é obrigatória")
	}

	if msg := validations.PasswordLengthError("A senha", password); msg != "" {
		data.AddFieldError("password", msg)
	}

	if password != passwordConfirm {
		data.AddFieldError("password_confirm", "As senhas não conferem")
	}

	uh.validateCaptcha(r, &data)

	if !data.Valid() {
		data.Password = ""
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorValidation, audit.EntityUser, 0)
		return uh.renderCaptchaAuthForm(w, r, http.StatusUnprocessableEntity, "user-signup.html", data, true)
	}

	userID, token, taken, err := uh.account.RegisterUser(r.Context(), firstName, lastName, data.Email, data.Password)
	if err != nil {
		uh.audit.RecordFailure(r.Context(), userID, audit.EventErrorProcessing, audit.EntityUser, 0)
		return err
	}
	if taken {
		data.AddFieldError("email", "Já existe um cadastro com esse e-mail")
		return uh.renderCaptchaAuthForm(w, r, http.StatusUnprocessableEntity, "user-signup.html", data, true)
	}
	uh.audit.RecordSuccess(r.Context(), 0, audit.EventSecurityTokenGenerated, audit.EntityUser, 0)

	idStr := ""
	if userID > 0 {
		idStr = strconv.FormatInt(userID, 10)
	}
	slog.Info("usuario registrado", slog.String("user_id", idStr))
	if userID > 0 {
		uh.audit.RecordSuccess(r.Context(), userID, audit.EventUserCreated, audit.EntityUser, userID)
	}

	page := userdto.SignupPage{
		Success:         true,
		RegisteredEmail: email,
	}

	body, err := uh.render.RenderMailBody(r, "confirmation.html", map[string]string{"token": token})
	if err != nil {
		return err
	}

	if err = uh.mail.Send(mailer.MailMessage{
		To:      []string{data.Email},
		Subject: "Confirmação de cadastro no Bridopen",
		IsHtml:  true,
		Body:    body,
	}); err != nil {
		slog.Error(err.Error())
		return err
	}
	slog.Info("email de confirmacao de cadastro enviado", slog.String("user_id", idStr))

	return uh.render.RenderAuthPage(w, r, http.StatusOK, "user-signup.html", page)
}

func (uh *userHandler) ResendEmailForm(w http.ResponseWriter, r *http.Request) error {
	return uh.renderResendEmailForm(w, r, http.StatusOK, userdto.UserRequest{})
}

func (uh *userHandler) ResendEmail(w http.ResponseWriter, r *http.Request) error {
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
		data.Email = ""
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorValidation, audit.EntityUser, 0)
		return uh.renderResendEmailForm(w, r, http.StatusUnprocessableEntity, data)
	}

	token, recipients, err := uh.account.RequestEmailConfirmation(r.Context(), email)
	if err != nil {
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorProcessing, audit.EntityUser, 0)
		return err
	}

	if token != "" {
		uh.audit.RecordSuccess(r.Context(), 0, audit.EventSecurityTokenGenerated, audit.EntityUser, 0)
		body, err := uh.render.RenderMailBody(r, "resendemail.html", map[string]string{"token": token})
		if err != nil {
			uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorProcessing, audit.EntityUser, 0)
			return err
		}
		if err := uh.mail.Send(mailer.MailMessage{
			To:      recipients,
			Subject: "Reenvio da confirmação de cadastro no Bridopen",
			IsHtml:  true,
			Body:    body,
		}); err != nil {
			uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorExternal, audit.EntityUser, 0)
			return err
		}
	}

	slog.Info("reenvio do email de confirmacao de cadastro processada", slog.Bool("email_enviado", token != ""))
	return uh.render.RenderAuthPage(w, r, http.StatusOK, "user-resend.html", userdto.SignupPage{Success: true})
}

func (uh *userHandler) Confirm(w http.ResponseWriter, r *http.Request) error {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		form := userdto.NewUserRequest("", "")
		form.AddFieldError("confirmation", "O link de confirmação é inválido ou está incompleto.")
		return uh.render.RenderAuthPage(w, r, http.StatusUnprocessableEntity, "user-signup.html", userdto.SignupPage{Success: false, Form: form})
	}

	email, userID, err := uh.account.ConfirmUser(r.Context(), token)
	if err != nil {
		form := userdto.NewUserRequest("", "")
		form.AddFieldError("confirmation", "Este cadastro já foi confirmado ou o token é inválido.")
		return uh.render.RenderAuthPage(w, r, http.StatusUnprocessableEntity, "user-signup.html", userdto.SignupPage{Success: false, Form: form})
	}

	page := userdto.SignupPage{
		Success:         true,
		Verified:        true,
		RegisteredEmail: email,
	}
	slog.Info("cadastro confirmado", slog.Int64("user_id", userID))
	uh.audit.RecordSuccess(r.Context(), userID, audit.EventUserUpdated, audit.EntityUser, userID)
	return uh.render.RenderAuthPage(w, r, http.StatusOK, "user-signup.html", page)
}

func (uh *userHandler) ConfirmRecoveryEmail(w http.ResponseWriter, r *http.Request) error {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		form := userdto.NewUserRequest("", "")
		form.AddFieldError("confirmation", "O link de confirmação do e-mail de recuperação é inválido ou está incompleto.")
		return uh.render.RenderAuthPage(w, r, http.StatusUnprocessableEntity, "user-recovery-email-confirmation.html", userdto.SignupPage{Success: false, Form: form})
	}

	primaryEmail, recoveryEmail, userID, err := uh.account.ConfirmRecoveryEmail(r.Context(), token)
	if err != nil {
		form := userdto.NewUserRequest("", "")
		form.AddFieldError("confirmation", "Este e-mail de recuperação já foi confirmado ou o token é inválido.")
		uh.audit.RecordFailure(r.Context(), 0, audit.EventSecurityTokenRevoked, audit.EntityUser, 0)
		return uh.render.RenderAuthPage(w, r, http.StatusUnprocessableEntity, "user-recovery-email-confirmation.html", userdto.SignupPage{Success: false, Form: form})
	}

	body, err := uh.render.RenderMailBody(r, "recovery-email-updated.html", map[string]string{"email": recoveryEmail})
	if err == nil {
		if sendErr := uh.mail.Send(mailer.MailMessage{
			To:      []string{primaryEmail},
			Subject: "E-mail de recuperação alterado no Bridopen",
			IsHtml:  true,
			Body:    body,
		}); sendErr != nil {
			slog.Warn("falha ao enviar aviso de e-mail de recuperacao alterado", slog.String("err", sendErr.Error()))
		}
	} else {
		slog.Warn("falha ao renderizar aviso de e-mail de recuperacao alterado", slog.String("err", err.Error()))
	}

	uh.audit.RecordSuccess(r.Context(), userID, audit.EventUserUpdated, audit.EntityUser, userID)
	slog.Info("email de recuperacao confirmado", slog.Int64("user_id", userID))
	if uh.session.GetInt64(r.Context(), "userId") == userID {
		http.Redirect(w, r, "/me", http.StatusSeeOther)
		return nil
	}
	return uh.render.RenderAuthPage(w, r, http.StatusOK, "user-recovery-email-confirmation.html", userdto.SignupPage{
		Success:         true,
		RegisteredEmail: recoveryEmail,
	})
}
