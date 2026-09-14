package handlers

import (
	"encoding/json"
	"errors"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"log/slog"
	"net/http"
	"strings"

	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
)

func (uh *userHandler) ExportAccountData(w http.ResponseWriter, r *http.Request) error {
	userID := uh.session.GetInt64(r.Context(), "userId")
	data, err := uh.account.ExportUserData(r.Context(), userID)
	if err != nil {
		return err
	}
	uh.audit.RecordSuccess(r.Context(), userID, audit.EventDataExportCompleted, audit.EntityUser, userID)
	slog.Info("dados da conta exportados", slog.Int64("user_id", userID))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="bridopen-dados.json"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(data)
}

func (uh *userHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return apperrors.WithStatus(errors.New("método não permitido"), http.StatusMethodNotAllowed)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	userID := uh.session.GetInt64(r.Context(), "userId")
	confirmation := strings.TrimSpace(r.PostFormValue("account_delete_confirmation"))
	password := strings.TrimSpace(r.PostFormValue("password"))
	data := userdto.NewUserRequest("", password)

	if confirmation != "excluir conta" {
		data.AddFieldError("confirmation", `Digite "excluir conta" para confirmar.`)
	}

	if password == "" {
		data.AddFieldError("password", "Senha é obrigatória")
	}

	if !data.Valid() {
		data.Password = ""
		uh.audit.RecordFailure(r.Context(), userID, audit.EventErrorValidation, audit.EntityUser, userID)
		return uh.renderDeleteAccountForm(w, r, http.StatusUnprocessableEntity, userID, data.FieldErrors)
	}

	if err := uh.account.DeleteAccount(r.Context(), userID, data.Password); err != nil {
		if errors.Is(err, apperrors.ErrInvalidCredentials) {
			uh.audit.RecordFailure(r.Context(), userID, audit.EventAuthLoginFailure, audit.EntityUser, userID)
			data.AddFieldError("password", "Senha inválida")
			return uh.renderDeleteAccountForm(w, r, http.StatusUnprocessableEntity, userID, data.FieldErrors)
		}
		return err
	}
	uh.audit.RecordSuccess(r.Context(), userID, audit.EventUserDeleted, audit.EntityUser, userID)
	slog.Info("conta excluida pelo titular", slog.Int64("user_id", userID))
	if err := sessionDestroy(uh.session, r.Context()); err != nil {
		return err
	}
	http.Redirect(w, r, "/user/signin", http.StatusSeeOther)
	return nil
}

func (uh *userHandler) renderDeleteAccountForm(w http.ResponseWriter, r *http.Request, status int, userID int64, fieldErrors map[string]string) error {
	page, err := uh.account.BuildMePage(r.Context(), userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			uh.audit.RecordFailure(r.Context(), userID, audit.EventErrorResourceNotFound, audit.EntityUser, userID)
			return apperrors.WithStatus(errors.New("conta não encontrada"), http.StatusNotFound)
		}
		return err
	}

	page.NavPath = meOverviewPrivacyDataPath
	page.OpenDeleteModal = true
	for field, message := range fieldErrors {
		page.AddFieldError(field, message)
	}

	return uh.render.RenderPage(w, r, status, "me.html", page)
}
