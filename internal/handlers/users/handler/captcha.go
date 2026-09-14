package handlers

import (
	"html/template"
	"net/http"
	"strings"

	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	platformcaptcha "github.com/engenheiroaraujo/bridopen/internal/platform/captcha"
)

const (
	signinCaptchaFailuresKey    = "signinCaptchaFailures"
	twoFactorCaptchaFailuresKey = "twoFactorCaptchaFailures"
	adaptiveCaptchaThreshold    = int64(3)
)

// Ganchos de teste para geração/validação de captcha.
var (
	generateCaptchaFn = platformcaptcha.GenerateCaptcha
	validateCaptchaFn = platformcaptcha.ValidateCaptcha
)

type captchaFieldErrorer interface {
	AddFieldError(field, message string)
}

func (uh *userHandler) renderTwoFactorLoginPage(w http.ResponseWriter, r *http.Request, status int, method string, form userdto.TwoFactorCodeRequest) error {
	page := userdto.TwoFactorLoginPage{
		Method:      method,
		MethodLabel: userdto.TwoFactorMethodLabel(method),
		Form:        form,
	}
	if uh.twoFactorCaptchaRequired(r) || hasCaptchaError(form.FieldErrors) {
		captcha, err := uh.newAuthCaptcha(hasCaptchaError(form.FieldErrors))
		if err != nil {
			return err
		}
		page.Captcha = captcha
	}
	return uh.render.RenderAuthPage(w, r, status, "user-two-factor.html", page)
}

func (uh *userHandler) SignupForm(w http.ResponseWriter, r *http.Request) error {
	return uh.renderCaptchaAuthForm(w, r, http.StatusOK, "user-signup.html", userdto.UserRequest{}, true)
}

func (uh *userHandler) renderForgetPasswordForm(w http.ResponseWriter, r *http.Request, status int, form userdto.UserRequest) error {
	return uh.renderCaptchaAuthForm(w, r, status, "user-forget-password.html", form, true)
}

func (uh *userHandler) renderResendEmailForm(w http.ResponseWriter, r *http.Request, status int, form userdto.UserRequest) error {
	return uh.renderCaptchaAuthForm(w, r, status, "user-resend.html", form, true)
}

func (uh *userHandler) renderSigninForm(w http.ResponseWriter, r *http.Request, status int, form userdto.UserRequest) error {
	return uh.renderCaptchaAuthForm(w, r, status, "user-signin.html", form, uh.signinCaptchaRequired(r) || hasCaptchaError(form.FieldErrors))
}

func (uh *userHandler) renderCaptchaAuthForm(w http.ResponseWriter, r *http.Request, status int, page string, form userdto.UserRequest, includeCaptcha bool) error {
	data := userdto.SignupPage{Form: form}
	if includeCaptcha {
		captcha, err := uh.newAuthCaptcha(hasCaptchaError(form.FieldErrors))
		if err != nil {
			return err
		}
		data.Captcha = captcha
	}
	return uh.render.RenderAuthPage(w, r, status, page, data)
}

func (uh *userHandler) newAuthCaptcha(open bool) (userdto.AuthCaptcha, error) {
	id, content, err := generateCaptchaFn()
	if err != nil {
		return userdto.AuthCaptcha{}, err
	}
	return userdto.AuthCaptcha{
		Enabled: true,
		ID:      id,
		Content: template.URL(content),
		Open:    open,
	}, nil
}

func (uh *userHandler) validateCaptcha(r *http.Request, form captchaFieldErrorer) bool {
	checked := strings.TrimSpace(r.PostFormValue("captcha_checked"))
	id := strings.TrimSpace(r.PostFormValue("captcha_id"))
	answer := strings.TrimSpace(r.PostFormValue("captcha_answer"))

	if checked == "" {
		form.AddFieldError("captcha", "Confirme que você é uma pessoa para continuar")
		return false
	}
	if id == "" || answer == "" {
		form.AddFieldError("captcha", "Digite o código de segurança")
		return false
	}
	if !validateCaptchaFn(id, answer) {
		form.AddFieldError("captcha", "Captcha inválido. Tente novamente")
		return false
	}
	return true
}

func hasCaptchaError(fieldErrors map[string]string) bool {
	if fieldErrors == nil {
		return false
	}
	_, ok := fieldErrors["captcha"]
	return ok
}

func (uh *userHandler) signinCaptchaRequired(r *http.Request) bool {
	return uh.adaptiveCaptchaRequired(r, signinCaptchaFailuresKey)
}

func (uh *userHandler) twoFactorCaptchaRequired(r *http.Request) bool {
	return uh.adaptiveCaptchaRequired(r, twoFactorCaptchaFailuresKey)
}

func (uh *userHandler) adaptiveCaptchaRequired(r *http.Request, key string) bool {
	return uh.session.GetInt64(r.Context(), key) >= adaptiveCaptchaThreshold
}

func (uh *userHandler) incrementAdaptiveCaptcha(r *http.Request, key string) {
	attempts := uh.session.GetInt64(r.Context(), key)
	uh.session.Put(r.Context(), key, attempts+1)
}

func (uh *userHandler) resetAdaptiveCaptcha(r *http.Request, key string) {
	uh.session.Remove(r.Context(), key)
}
