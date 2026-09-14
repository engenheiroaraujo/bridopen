package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	userservice "github.com/engenheiroaraujo/bridopen/internal/handlers/users/service"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
)

const (
	pendingTwoFactorUserIDKey    = "pendingTwoFactorUserID"
	pendingTwoFactorEmailKey     = "pendingTwoFactorEmail"
	pendingTwoFactorFirstNameKey = "pendingTwoFactorFirstName"
	pendingTwoFactorMethodKey    = "pendingTwoFactorMethod"
)

// Ganchos de teste para falhas do gerenciador de sessão difíceis de atingir.
var (
	sessionRenewToken = func(s *scs.SessionManager, ctx context.Context) error { return s.RenewToken(ctx) }
	sessionDestroy    = func(s *scs.SessionManager, ctx context.Context) error { return s.Destroy(ctx) }
)

type userHandler struct {
	render    *render.RenderTemplate
	session   *scs.SessionManager
	mail      mailer.MailService
	twoFactor *userservice.TwoFactorService
	account   *userservice.AccountService
	audit     audit.Recorder
}

func NewUserHandler(
	render *render.RenderTemplate,
	session *scs.SessionManager,
	mail mailer.MailService,
	twoFactor *userservice.TwoFactorService,
	account *userservice.AccountService,
	auditRecorder audit.Recorder,
) *userHandler {
	return &userHandler{
		render:    render,
		session:   session,
		mail:      mail,
		twoFactor: twoFactor,
		account:   account,
		audit:     auditRecorder,
	}
}

func (uph *userPersonalHandler) Me(w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, meOverviewPersonalInformationPath, http.StatusSeeOther)
	return nil
}

func (uph *userPersonalHandler) MePersonalInformation(w http.ResponseWriter, r *http.Request) error {
	return uph.renderMePage(w, r, http.StatusOK, meOverviewPersonalInformationPath, mePersonalInformationSection)
}

func (uph *userPersonalHandler) MeAccountSettings(w http.ResponseWriter, r *http.Request) error {
	return uph.renderMePage(w, r, http.StatusOK, meOverviewAccountSettingsPath, meAccountSettingsSection)
}

func (uph *userPersonalHandler) MePrivacyData(w http.ResponseWriter, r *http.Request) error {
	return uph.renderMePage(w, r, http.StatusOK, meOverviewPrivacyDataPath, mePrivacyDataSection)
}

func (uph *userPersonalHandler) renderMePage(w http.ResponseWriter, r *http.Request, status int, navPath, focusSection string) error {
	userID := uph.getUserIDFromSession(r)

	page, err := uph.accountSvc.BuildMePage(r.Context(), userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uph.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		return err
	}
	page.NavPath = navPath
	page.FocusSection = focusSection

	return uph.render.RenderPage(w, r, status, "me.html", page)
}

func (uph *userPersonalHandler) UserPersonalSave(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID := uph.getUserIDFromSession(r)
	formID, _ := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("id")))
	firstName := r.PostForm.Get("first_name")
	lastName := r.PostForm.Get("last_name")
	phone := r.PostForm.Get("phone_number")

	if fieldErrors := userservice.ValidatePersonalForm(firstName, lastName, phone); len(fieldErrors) > 0 {
		return uph.renderMeForm(w, r, http.StatusUnprocessableEntity, userID, firstName, lastName, phone, fieldErrors)
	}

	created, err := uph.accountSvc.SavePersonalInformation(r.Context(), userID, formID, firstName, lastName, phone)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.WithStatus(errors.New("dados pessoais não encontrados"), http.StatusNotFound)
		}
		if strings.Contains(err.Error(), "identificador de dados pessoais inválido") {
			return apperrors.WithStatus(errors.New("requisição inválida"), http.StatusBadRequest)
		}
		return err
	}

	if created {
		slog.Info("dados pessoais criados", slog.Int64("user_id", userID))
		uph.audit.RecordSuccess(r.Context(), userID, audit.EventDataCreated, audit.EntityPersonalData, userID)
	} else {
		slog.Info("dados pessoais atualizados", slog.Int64("user_id", userID))
		uph.audit.RecordSuccess(r.Context(), userID, audit.EventDataUpdated, audit.EntityPersonalData, userID)
	}

	http.Redirect(w, r, meOverviewPersonalInformationPath, http.StatusSeeOther)
	return nil
}

func (uph *userPersonalHandler) renderMeForm(w http.ResponseWriter, r *http.Request, status int, userID int64, firstName, lastName, phone string, fieldErrors map[string]string) error {
	page, err := uph.accountSvc.BuildMePage(r.Context(), userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		return err
	}

	page.FirstName = strings.TrimSpace(firstName)
	page.LastName = strings.TrimSpace(lastName)
	page.Phone = strings.TrimSpace(phone)
	page.PhoneNumber = userdto.DisplayOrPlaceholder(phone)
	page.FullName = userdto.FormatFullNameForDisplay(firstName, lastName)
	page.HasPersonalInfo = page.FirstName != "" || page.LastName != "" || page.Phone != ""
	page.NavPath = meOverviewPersonalInformationPath
	page.OpenEditModal = true

	for field, message := range fieldErrors {
		page.AddFieldError(field, message)
	}

	return uph.render.RenderPage(w, r, status, "me.html", page)
}
