package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	legalhandler "github.com/engenheiroaraujo/bridopen/internal/handlers/legal"
	notehandler "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/handler"
	noterepo "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/repositories"
	noteservice "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/service"
	userhandler "github.com/engenheiroaraujo/bridopen/internal/handlers/users/handler"
	userrepo "github.com/engenheiroaraujo/bridopen/internal/handlers/users/repositories"
	userservice "github.com/engenheiroaraujo/bridopen/internal/handlers/users/service"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	"github.com/engenheiroaraujo/bridopen/internal/platform/buildinfo"
	"github.com/engenheiroaraujo/bridopen/internal/platform/database"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	corehandlers "github.com/engenheiroaraujo/bridopen/internal/platform/middleware"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/engenheiroaraujo/bridopen/views"
)

var viewsFS fs.FS = views.Files

func LoadRoutes(sessionManager *scs.SessionManager, mail mailer.MailService, dbpool database.Pool, baseURL, mfaSecret, attachmentStorageDir, clamAVAddr string) http.Handler {
	mux := http.NewServeMux()

	static, err := fs.Sub(viewsFS, "static")
	if err != nil {
		slog.Error(err.Error())
		panic(err)
	}

	staticHandler := http.FileServerFS(static)

	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler))
	mux.HandleFunc("GET /version", buildinfo.Handler)

	noteRepo := noterepo.NewNoteRepository(dbpool)
	var attachmentScanner noteservice.AttachmentScanner = noteservice.NoopAttachmentScanner{}
	if strings.TrimSpace(clamAVAddr) != "" {
		attachmentScanner = noteservice.NewClamAVScanner(clamAVAddr)
	}
	attachmentStorage := noteservice.NewLocalAttachmentStorage(attachmentStorageDir, attachmentScanner)
	userRepo := userrepo.NewUserRepository(dbpool)
	userPersonalRepo := userrepo.NewUserPersonalRepository(dbpool)
	userSessionRepo := userrepo.NewUserSessionRepository(dbpool)
	twoFactorRepo := userrepo.NewTwoFactorRepository(dbpool)
	recoveryCodeRepo := userrepo.NewRecoveryCodeRepository(dbpool)
	auditRecorder := audit.NewRecorder(audit.NewPostgresRepository(dbpool))
	if err := userRepo.DeleteExpiredTokens(context.Background()); err != nil {
		slog.Warn("falha ao limpar tokens expirados", slog.String("err", err.Error()))
	}
	if err := twoFactorRepo.DeleteExpiredChallenges(context.Background()); err != nil {
		slog.Warn("falha ao limpar desafios 2FA expirados", slog.String("err", err.Error()))
	}

	twoFactorEmailRenderer := render.NewTwoFactorEmailRenderer()
	render := render.NewRender(sessionManager, baseURL)

	noteService := noteservice.NewNoteService(noteRepo, attachmentStorage, auditRecorder)
	noteHandler := notehandler.NewNoteHandler(render, sessionManager, noteService)
	legalHandler := legalhandler.NewHandler(render)
	twoFactorService := userservice.NewTwoFactorService(twoFactorRepo, mail, mfaSecret, twoFactorEmailRenderer).WithRecoveryCodes(recoveryCodeRepo)
	accountService := userservice.NewAccountService(userRepo, userPersonalRepo, twoFactorRepo, userSessionRepo).
		WithAttachmentStorage(attachmentStorage)
	userHandler := userhandler.NewUserHandler(render, sessionManager, mail, twoFactorService, accountService, auditRecorder)
	userPersonalHandler := userhandler.NewUserPersonalHandler(render, sessionManager, accountService, mail, auditRecorder)
	twoFactorHandler := userhandler.NewTwoFactorHandler(render, sessionManager, twoFactorService, auditRecorder)

	authMidd := corehandlers.NewAuthMiddleware(sessionManager, userSessionRepo)
	errorMidd := corehandlers.NewErrorHandlerMiddleware(render)
	authRateLimit := NewIPRateLimiter(8, time.Minute)

	mux.Handle("GET /{$}", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteList)))
	mux.Handle("GET /note/{id}", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteView)))
	mux.Handle("GET /note/new", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteNew)))
	mux.Handle("POST /note", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteSave)))
	mux.Handle("DELETE /note/{id}", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteDelete)))
	mux.Handle("GET /note/{id}/edit", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteEdit)))
	mux.Handle("POST /note/{id}/attachments", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteAttachmentUpload)))
	mux.Handle("GET /note/{id}/attachments/{attachmentId}", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteAttachmentDownload)))
	mux.Handle("DELETE /note/{id}/attachments/{attachmentId}", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteAttachmentDelete)))
	mux.Handle("GET /notes/archive", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteArchiveList)))
	mux.Handle("GET /notes/trash", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteTrashList)))
	mux.Handle("GET /tags", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteTagList)))
	mux.Handle("POST /tags", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteTagCreate)))
	mux.Handle("POST /tags/{id}", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteTagUpdate)))
	mux.Handle("POST /tags/{id}/delete", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteTagDelete)))
	mux.Handle("POST /note/{id}/pin", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NotePin)))
	mux.Handle("POST /note/{id}/unpin", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteUnpin)))
	mux.Handle("POST /note/{id}/archive", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteArchive)))
	mux.Handle("POST /note/{id}/unarchive", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteUnarchive)))
	mux.Handle("POST /note/{id}/restore", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteRestore)))
	mux.Handle("DELETE /note/{id}/destroy", authMidd.RequireAuth(errorMidd.HandlerError(noteHandler.NoteDeletePermanently)))

	mux.Handle("GET /privacy", errorMidd.HandlerError(legalHandler.PrivacyPolicy))
	mux.Handle("GET /terms", errorMidd.HandlerError(legalHandler.TermsOfService))
	mux.Handle("GET /cookies", errorMidd.HandlerError(legalHandler.CookiePolicy))

	mux.Handle("GET /user/signup", errorMidd.HandlerError(userHandler.SignupForm))
	mux.Handle("POST /user/signup", authRateLimit.Limit(errorMidd.HandlerError(userHandler.Signup)))

	mux.Handle("GET /user/signin", errorMidd.HandlerError(userHandler.SigninForm))
	mux.Handle("POST /user/signin", authRateLimit.Limit(errorMidd.HandlerError(userHandler.Signin)))
	mux.Handle("GET /user/twofactor", errorMidd.HandlerError(userHandler.TwoFactorLoginForm))
	mux.Handle("POST /user/twofactor", authRateLimit.Limit(errorMidd.HandlerError(userHandler.TwoFactorLoginVerify)))

	mux.Handle("POST /user/signout", authMidd.RequireAuth(errorMidd.HandlerError(userHandler.Signout)))

	mux.Handle("GET /user/forgetpassword", errorMidd.HandlerError(userHandler.ForgetPasswordForm))
	mux.Handle("POST /user/forgetpassword", authRateLimit.Limit(errorMidd.HandlerError(userHandler.ForgetPassword)))

	mux.Handle("GET /user/password/{token}", errorMidd.HandlerError(userHandler.ResetPasswordForm))
	mux.Handle("POST /user/password/{token}", authRateLimit.Limit(errorMidd.HandlerError(userHandler.ResetPassword)))

	mux.Handle("GET /user/resend", errorMidd.HandlerError(userHandler.ResendEmailForm))
	mux.Handle("POST /user/resend", authRateLimit.Limit(errorMidd.HandlerError(userHandler.ResendEmail)))

	mux.Handle("GET /me", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.Me)))
	mux.Handle("GET /me/overview/personal-information", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.MePersonalInformation)))
	mux.Handle("GET /me/overview/account-settings", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.MeAccountSettings)))
	mux.Handle("GET /me/overview/privacy-data", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.MePrivacyData)))
	mux.Handle("GET /me/security", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.Security)))
	mux.Handle("GET /me/security/two-step-verification", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.SecurityTwoStepVerification)))
	mux.Handle("GET /me/security/active-sessions", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.ActiveSessions)))
	mux.Handle("GET /me/security/two-factor/{method}", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.SecurityTwoFactorMethod)))
	mux.Handle("POST /me/security/sessions/revoke-other", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.RevokeOtherSessions)))
	mux.Handle("POST /me/security/sessions/{sessionID}/logout", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.RevokeSession)))
	mux.Handle("POST /me/personal", authMidd.RequireAuth(errorMidd.HandlerError(userPersonalHandler.UserPersonalSave)))
	mux.Handle("POST /me/password", authMidd.RequireAuth(authRateLimit.Limit(errorMidd.HandlerError(userPersonalHandler.PasswordSave))))
	mux.Handle("POST /me/recovery-email", authMidd.RequireAuth(authRateLimit.Limit(errorMidd.HandlerError(userPersonalHandler.RecoveryEmailSave))))
	mux.Handle("POST /me/twofactor/start", authMidd.RequireAuth(authRateLimit.Limit(errorMidd.HandlerError(twoFactorHandler.Start))))
	mux.Handle("POST /me/twofactor/verify", authMidd.RequireAuth(authRateLimit.Limit(errorMidd.HandlerError(twoFactorHandler.Verify))))
	mux.Handle("POST /me/twofactor/recovery-codes/regenerate", authMidd.RequireAuth(authRateLimit.Limit(errorMidd.HandlerError(twoFactorHandler.RegenerateRecoveryCodes))))
	mux.Handle("POST /me/twofactor/disable/start", authMidd.RequireAuth(authRateLimit.Limit(errorMidd.HandlerError(twoFactorHandler.DisableStart))))
	mux.Handle("POST /me/twofactor/disable/verify", authMidd.RequireAuth(authRateLimit.Limit(errorMidd.HandlerError(twoFactorHandler.DisableVerify))))
	mux.Handle("GET /account/export", authMidd.RequireAuth(errorMidd.HandlerError(userHandler.ExportAccountData)))
	mux.Handle("POST /account/delete", authMidd.RequireAuth(errorMidd.HandlerError(userHandler.DeleteAccount)))

	mux.Handle("GET /confirmation/{token}", errorMidd.HandlerError(userHandler.Confirm))
	mux.Handle("GET /recovery-email/confirmation/{token}", errorMidd.HandlerError(userHandler.ConfirmRecoveryEmail))

	mux.Handle("/", errorMidd.HandlerError(func(w http.ResponseWriter, r *http.Request) error {
		return apperrors.ErrPageNotFound
	}))

	return mux
}
