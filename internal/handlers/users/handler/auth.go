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
	"github.com/engenheiroaraujo/bridopen/internal/platform/key"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

func (uh *userHandler) SigninForm(w http.ResponseWriter, r *http.Request) error {
	return uh.renderSigninForm(w, r, http.StatusOK, userdto.UserRequest{})
}

func (uh *userHandler) Signin(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	email := strings.TrimSpace(r.PostFormValue("email"))
	password := strings.TrimSpace(r.PostFormValue("password"))

	data := userdto.NewUserRequest(email, password)

	if email == "" {
		data.AddFieldError("email", "E-mail é obrigatório")
	} else if !validations.IsEmailValid(email) {
		data.AddFieldError("email", "E-mail inválido")
	}

	if password == "" {
		data.AddFieldError("password", "Senha é obrigatória")
	}
	if uh.signinCaptchaRequired(r) {
		uh.validateCaptcha(r, &data)
	}

	if !data.Valid() {
		data.Password = ""
		if !hasCaptchaError(data.FieldErrors) {
			uh.incrementAdaptiveCaptcha(r, signinCaptchaFailuresKey)
		}
		uh.audit.RecordFailure(r.Context(), 0, audit.EventErrorValidation, audit.EntityUser, 0)
		return uh.renderSigninForm(w, r, http.StatusUnprocessableEntity, data)
	}

	authenticated, err := uh.account.Authenticate(r.Context(), data.Email, data.Password)
	if err != nil && !errors.Is(err, apperrors.ErrUserInactive) {
		uh.audit.RecordFailure(r.Context(), 0, audit.EventAuthLoginFailure, audit.EntityUser, 0)
		uh.incrementAdaptiveCaptcha(r, signinCaptchaFailuresKey)
		data.AddFieldError("validation", "Usuário ou senha inválidos")
		return uh.renderSigninForm(w, r, http.StatusUnprocessableEntity, data)
	}
	uh.resetAdaptiveCaptcha(r, signinCaptchaFailuresKey)

	if errors.Is(err, apperrors.ErrUserInactive) {
		uh.audit.RecordFailure(r.Context(), 0, audit.EventSecurityAccessBlocked, audit.EntityUser, 0)
		http.Redirect(w, r, "/user/resend", http.StatusSeeOther)
		return nil
	}

	if uh.twoFactor != nil && authenticated.TwoFactorEnabled {
		status, err := uh.twoFactor.BeginLogin(r.Context(), authenticated.UserID)
		if err != nil {
			return err
		}
		if err = sessionRenewToken(uh.session, r.Context()); err != nil {
			slog.Error(err.Error())
			return err
		}
		uh.session.Put(r.Context(), pendingTwoFactorUserIDKey, authenticated.UserID)
		uh.session.Put(r.Context(), pendingTwoFactorEmailKey, authenticated.Email)
		uh.session.Put(r.Context(), pendingTwoFactorFirstNameKey, authenticated.FirstName)
		uh.session.Put(r.Context(), pendingTwoFactorMethodKey, status.Method)
		http.Redirect(w, r, "/user/twofactor", http.StatusSeeOther)
		return nil
	}

	return uh.completeSignin(w, r, authenticated.UserID, authenticated.Email, authenticated.FirstName)
}

func (uh *userHandler) completeSignin(w http.ResponseWriter, r *http.Request, userID int64, email, firstName string) error {
	if err := uh.startAuthenticatedSession(r, userID, email, firstName); err != nil {
		slog.Error("falha ao iniciar sessao autenticada", slog.String("err", err.Error()))
		return err
	}

	idStr := ""
	if userID > 0 {
		idStr = strconv.FormatInt(userID, 10)
	}
	slog.Info("usuario efetuou login", slog.String("user_id", idStr))
	uh.audit.RecordSuccess(r.Context(), userID, audit.EventAuthLoginSuccess, audit.EntityUser, userID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func (uh *userHandler) clearPendingTwoFactor(r *http.Request) {
	uh.session.Remove(r.Context(), pendingTwoFactorUserIDKey)
	uh.session.Remove(r.Context(), pendingTwoFactorEmailKey)
	uh.session.Remove(r.Context(), pendingTwoFactorFirstNameKey)
	uh.session.Remove(r.Context(), pendingTwoFactorMethodKey)
	uh.resetAdaptiveCaptcha(r, twoFactorCaptchaFailuresKey)
}

func (uh *userHandler) TwoFactorLoginForm(w http.ResponseWriter, r *http.Request) error {
	userID, method := uh.pendingTwoFactorData(r)
	if userID == 0 || method == "" {
		http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
		return nil
	}
	return uh.renderTwoFactorLoginPage(w, r, http.StatusOK, method, userdto.TwoFactorCodeRequest{})
}

func (uh *userHandler) TwoFactorLoginVerify(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if uh.twoFactor == nil {
		http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
		return nil
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID, method := uh.pendingTwoFactorData(r)
	if userID == 0 || method == "" {
		http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
		return nil
	}

	form := userdto.TwoFactorCodeRequest{Code: strings.TrimSpace(r.PostFormValue("code"))}
	if uh.twoFactorCaptchaRequired(r) {
		uh.validateCaptcha(r, &form)
	}
	if form.Code == "" {
		form.AddFieldError("code", "Informe o código de verificação.")
	}
	if !form.Valid() {
		if !hasCaptchaError(form.FieldErrors) {
			uh.incrementAdaptiveCaptcha(r, twoFactorCaptchaFailuresKey)
		}
		return uh.renderTwoFactorLoginPage(w, r, http.StatusUnprocessableEntity, method, form)
	}

	if _, err := uh.twoFactor.VerifyLogin(r.Context(), userID, form.Code); err != nil {
		uh.audit.RecordFailure(r.Context(), userID, audit.EventMFALoginFailed, audit.EntityUser, userID)
		uh.incrementAdaptiveCaptcha(r, twoFactorCaptchaFailuresKey)
		form.Code = ""
		form.AddFieldError("code", apperrors.TwoFactorMessage(err))
		if errors.Is(err, apperrors.ErrTwoFactorChallengeExpired) || errors.Is(err, apperrors.ErrTwoFactorTooManyAttempts) {
			uh.clearPendingTwoFactor(r)
		}
		return uh.renderTwoFactorLoginPage(w, r, http.StatusUnprocessableEntity, method, form)
	}

	email := uh.session.GetString(r.Context(), pendingTwoFactorEmailKey)
	firstName := uh.session.GetString(r.Context(), pendingTwoFactorFirstNameKey)
	uh.audit.RecordSuccess(r.Context(), userID, audit.EventMFALoginValidated, audit.EntityUser, userID)
	return uh.completeSignin(w, r, userID, email, firstName)
}

func (uh *userHandler) pendingTwoFactorData(r *http.Request) (int64, string) {
	return uh.session.GetInt64(r.Context(), pendingTwoFactorUserIDKey), uh.session.GetString(r.Context(), pendingTwoFactorMethodKey)
}

func (uh *userHandler) Signout(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}

	userID := uh.session.GetInt64(r.Context(), "userId")
	sessionID := uh.session.GetString(r.Context(), "userSessionId")
	if userID != 0 && sessionID != "" {
		if err := uh.account.RevokeSession(r.Context(), userID, sessionID); err != nil {
			slog.Warn("falha ao revogar sessao no logout", slog.String("err", err.Error()))
		}
	}
	if err := sessionDestroy(uh.session, r.Context()); err != nil {
		slog.Error(err.Error())
		return err
	}
	uh.audit.RecordSuccess(r.Context(), userID, audit.EventAuthLogout, audit.EntityUser, userID)
	slog.Info("usuario efetuou logout", slog.Int64("user_id", userID))

	http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
	return nil
}

func (uh *userHandler) startAuthenticatedSession(r *http.Request, userID int64, email, firstName string) error {
	if err := sessionRenewToken(uh.session, r.Context()); err != nil {
		return err
	}

	sessionID, err := key.GenerateSessionID()
	if err != nil {
		return err
	}

	if err := uh.account.CreateUserSession(r.Context(), userID, sessionID, r.UserAgent(), key.ClientIP(r)); err != nil {
		return err
	}

	uh.clearPendingTwoFactor(r)
	uh.session.Put(r.Context(), "userId", userID)
	uh.session.Put(r.Context(), "userEmail", email)
	uh.session.Put(r.Context(), "firstName", firstName)
	uh.session.Put(r.Context(), "userSessionId", sessionID)
	return nil
}
