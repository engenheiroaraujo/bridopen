package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/engenheiroaraujo/bridopen/internal/platform/buildinfo"
	appconfig "github.com/engenheiroaraujo/bridopen/internal/platform/config"
	"github.com/engenheiroaraujo/bridopen/internal/platform/log"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	corehandlers "github.com/engenheiroaraujo/bridopen/internal/platform/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config = appconfig.Config

var (
	osExit            = os.Exit
	loadAppConfig     = appconfig.Load
	openDBPool        = pgxpool.New
	setupSessionStore = func(sm *scs.SessionManager, pool *pgxpool.Pool) {
		sm.Store = pgxstore.NewWithCleanupInterval(pool, 30*time.Minute)
	}
	routeLoader    = LoadRoutes
	listenAndServe = defaultListenAndServe
)

func defaultListenAndServe(server *http.Server) error {
	return server.ListenAndServe()
}

func main() {
	config := loadAppConfig()

	slog.SetDefault(log.NewLogger(os.Stderr, config.GetLevelLog()))
	slog.Info("starting application",
		slog.String("version", buildinfo.Version),
		slog.String("commit", buildinfo.Commit),
		slog.String("build_time", buildinfo.BuildTime),
	)

	// conecta no banco de dados postgres
	dbpool, err := openDBPool(context.Background(), config.DBConnURL)
	if err != nil {
		slog.Error(err.Error())
		osExit(1)
		return
	}
	slog.Info("Conexão com o banco de dados bem sucedida")
	defer dbpool.Close()

	fmt.Printf("Servidor rodando na porta %s\n", config.ServerPort)

	//teste o envio de email
	mailPort, _ := strconv.Atoi(config.MailPort)
	mailService := mailer.NewSMTPMailService(mailer.SMTPConfig{
		Host:     config.MailHost,
		Port:     mailPort,
		Username: config.MailUserName,
		Password: config.MailPassword,
		From:     config.MailFrom,
	})

	sessionManager := scs.New()
	sessionManager.Lifetime = time.Hour
	sessionManager.IdleTimeout = 30 * time.Minute
	sessionManager.Cookie.Name = "bridopen_session"
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.Persist = false
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	sessionManager.Cookie.Secure = config.UseSecureCookies()
	if config.CookieDomain != "" {
		sessionManager.Cookie.Domain = config.CookieDomain
	}
	sessionManager.HashTokenInStore = true
	// limpa as sessoes expiradas da tabelade sessions a cada 30 minutos
	setupSessionStore(sessionManager, dbpool)

	csrfMiddleware := csrfMiddleware(sessionManager, config.BaseURL, config.CSRFKey, config.UseSecureCookies(), config.TrustedOriginList())

	mux := routeLoader(sessionManager, mailService, dbpool, config.BaseURL, config.MFAEncryptionSecret(), config.AttachmentStorageDir, config.ClamAVAddr)
	handler := securityHeaders(sessionManager.LoadAndSave(corehandlers.AccessLog(sessionManager, csrfMiddleware(mux))))

	server := &http.Server{
		Addr:              fmt.Sprintf(":%s", config.ServerPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	slog.Info("servidor iniciado", slog.String("port", config.ServerPort), slog.String("env", config.AppEnv))
	if err := listenAndServe(server); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("servidor interrompido com erro", slog.String("err", err.Error()))
		osExit(1)
	}
}
