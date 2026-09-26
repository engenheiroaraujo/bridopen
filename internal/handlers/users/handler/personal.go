package handlers

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	userservice "github.com/engenheiroaraujo/bridopen/internal/handlers/users/service"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
)

type userPersonalHandler struct {
	render     *render.RenderTemplate
	session    *scs.SessionManager
	accountSvc *userservice.AccountService
	mail       mailer.MailService
	audit      audit.Recorder
}

const (
	meOverviewPersonalInformationPath = "/me/overview/personal-information"
	meOverviewAccountSettingsPath     = "/me/overview/account-settings"
	meOverviewPrivacyDataPath         = "/me/overview/privacy-data"
	meSecurityTwoStepVerificationPath = "/me/security/two-step-verification"
	meSecurityActiveSessionsPath      = "/me/security/active-sessions"

	mePersonalInformationSection = "personal-information"
	meAccountSettingsSection     = "account-settings"
	mePrivacyDataSection         = "privacy-data"
	meTwoStepVerificationSection = "two-step-verification"
	meActiveSessionsSection      = "active-sessions"
)

func NewUserPersonalHandler(render *render.RenderTemplate, session *scs.SessionManager, accountSvc *userservice.AccountService, mail mailer.MailService, auditRecorder audit.Recorder) *userPersonalHandler {
	return &userPersonalHandler{render: render, session: session, accountSvc: accountSvc, mail: mail, audit: auditRecorder}
}

func (uph *userPersonalHandler) getUserIDFromSession(r *http.Request) int64 {
	return uph.session.GetInt64(r.Context(), "userId")
}

func firstFormValue(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := r.PostFormValue(name); value != "" {
			return value
		}
	}
	return ""
}
