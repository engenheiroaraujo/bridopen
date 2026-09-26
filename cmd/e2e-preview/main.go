// e2e-preview serve as telas do Bridopen renderizadas com os templates e
// assets reais, mas com dados fixos e sem banco de dados. É o servidor que os
// testes em navegador (Playwright, em tests/e2e) acessam.
//
// Não faz parte da aplicação: a sessão é sempre a do usuário fixo "Ada", as
// mutações só redirecionam ou respondem JSON de sucesso, e nada é persistido.
//
//	go run ./cmd/e2e-preview -addr 127.0.0.1:5511
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
	notedto "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/dto"
	notemodel "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/engenheiroaraujo/bridopen/views"
	"github.com/gorilla/csrf"
)

// captchaImage é um SVG inline no lugar da imagem gerada pelo backend.
const captchaImage template.URL = "data:image/svg+xml;utf8," +
	"<svg xmlns='http://www.w3.org/2000/svg' width='100' height='50'>" +
	"<rect width='100' height='50' fill='%23e2e8f0'/>" +
	"<text x='50' y='32' font-size='22' text-anchor='middle' fill='%231e293b'>1234</text></svg>"

// csrfErrorPage espelha o DTO privado de cmd/http para a tela de sessão expirada.
type csrfErrorPage struct {
	ActionHref  string
	ActionLabel string
}

const (
	fixtureEmail     = "ada@exemplo.com"
	fixtureFirstName = "Ada"
	fixtureLastName  = "Lovelace"
	fixturePhone     = "(11) 98765-4321"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:5511", "endereço de escuta")
	flag.Parse()

	// baseURL sem "localhost" para o render usar o FS embutido, igual à produção.
	session := scs.New()
	rt := render.NewRender(session, "http://"+*addr)

	static, err := fs.Sub(views.Files, "static")
	if err != nil {
		log.Fatalf("e2e-preview: static: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	registerAuthPages(mux, rt)
	registerPublicPages(mux, rt)
	registerAppPages(mux, rt)
	registerMutations(mux)

	// Mesmas opções de cookie de cmd/http/security.go: sem Path("/") o navegador restringe o cookie ao diretório da página e
	// os POST/DELETE para outros caminhos falham com "CSRF token invalid".
	protect := csrf.Protect(
		[]byte("e2e-preview-csrf-key-0123456789abcdef"),
		csrf.CookieName("bridopen_csrf"),
		csrf.Path("/"),
		csrf.Secure(false),
		csrf.HttpOnly(true),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.MaxAge(3600),
		csrf.TrustedOrigins([]string{*addr}),
	)
	handler := protect(session.LoadAndSave(withFixtureUser(session, mux)))

	fmt.Fprintf(os.Stderr, "e2e-preview ouvindo em http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("e2e-preview: %v", err)
	}
}

/*
withFixtureUser grava o usuário fixo na sessão de toda request. As telas de autenticação ignoram a sessão, então não há problema em
ela existir também lá; as telas do app dependem dela para isAuthenticated e firstName no shell.
*/
func withFixtureUser(session *scs.SessionManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "userId", int64(1))
		session.Put(r.Context(), "firstName", fixtureFirstName)
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Telas de autenticação (base-auth)
// ---------------------------------------------------------------------------

func registerAuthPages(mux *http.ServeMux, rt *render.RenderTemplate) {
	// Telas cujo captcha é adaptativo em produção começam sem captcha; as demais
	// sempre o exibem. ?captcha=1 ou ?captcha=0 força qualquer um dos estados.
	authForm := func(page string, captchaByDefault bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			data := userdto.SignupPage{Form: userdto.UserRequest{Email: r.URL.Query().Get("email")}}
			data.Form.FieldErrors = fieldErrors(r)
			data.Captcha = captcha(r, captchaByDefault)
			data.Token = r.PathValue("token")
			renderAuth(w, r, rt, http.StatusOK, page, data)
		}
	}
	mux.HandleFunc("GET /user/signin", authForm("user-signin.html", false))
	mux.HandleFunc("GET /user/signup", authForm("user-signup.html", true))
	mux.HandleFunc("GET /user/forgetpassword", authForm("user-forget-password.html", true))
	mux.HandleFunc("GET /user/resend", authForm("user-resend.html", true))
	mux.HandleFunc("GET /user/password/{token}", authForm("user-reset-password.html", false))
	mux.HandleFunc("GET /user/twofactor", func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Query().Get("method")
		if method == "" {
			method = "totp"
		}
		data := userdto.TwoFactorLoginPage{
			Method:      method,
			MethodLabel: userdto.TwoFactorMethodLabel(method),
			Captcha:     captcha(r, false),
		}
		data.Form.FieldErrors = fieldErrors(r)
		renderAuth(w, r, rt, http.StatusOK, "user-two-factor.html", data)
	})
}

// ---------------------------------------------------------------------------
// Páginas públicas e de erro (base-auth, sem formulário)
// ---------------------------------------------------------------------------

func registerPublicPages(mux *http.ServeMux, rt *render.RenderTemplate) {
	for _, page := range []struct{ path, tpl string }{
		{"/privacy", "privacy.html"}, {"/terms", "terms.html"}, {"/cookies", "cookies.html"},
	} {
		tpl := page.tpl
		mux.HandleFunc("GET "+page.path, func(w http.ResponseWriter, r *http.Request) {
			renderAuth(w, r, rt, http.StatusOK, tpl, nil)
		})
	}
	mux.HandleFunc("GET /erro/404", func(w http.ResponseWriter, r *http.Request) {
		renderAuth(w, r, rt, http.StatusNotFound, "404.html", "página não encontrada")
	})
	mux.HandleFunc("GET /erro/generico", func(w http.ResponseWriter, r *http.Request) {
		renderAuth(w, r, rt, http.StatusInternalServerError, "generic-error.html", nil)
	})
	// ?origem=signup|forgetpassword reproduz os dois destinos do botão em cmd/http/security.go.
	mux.HandleFunc("GET /erro/csrf", func(w http.ResponseWriter, r *http.Request) {
		page := csrfErrorPage{ActionHref: "/user/signin", ActionLabel: "Voltar para o login"}
		switch r.URL.Query().Get("origem") {
		case "signup":
			page = csrfErrorPage{ActionHref: "/user/signup", ActionLabel: "Voltar para o cadastro"}
		case "forgetpassword":
			page = csrfErrorPage{ActionHref: "/user/forgetpassword", ActionLabel: "Redefinir senha novamente"}
		}
		renderAuth(w, r, rt, http.StatusForbidden, "csrf-error.html", page)
	})
	mux.HandleFunc("GET /recovery-email/confirmation/{token}", func(w http.ResponseWriter, r *http.Request) {
		data := userdto.SignupPage{Success: r.PathValue("token") == "valido", RegisteredEmail: "recuperacao@exemplo.com"}
		if !data.Success {
			data.Form.FieldErrors = map[string]string{"link": "Este link de confirmação é inválido ou já expirou."}
		}
		renderAuth(w, r, rt, http.StatusOK, "user-recovery-email-confirmation.html", data)
	})
}

// ---------------------------------------------------------------------------
// Telas do app (base, usuário autenticado)
// ---------------------------------------------------------------------------

func registerAppPages(mux *http.ServeMux, rt *render.RenderTemplate) {
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := notemodel.NoteFilter{
			Search: strings.TrimSpace(q.Get("q")),
			Color:  strings.TrimSpace(q.Get("color")),
			Tag:    strings.TrimSpace(q.Get("tag")),
			Sort:   strings.TrimSpace(q.Get("sort")),
			Pinned: q.Get("pinned") == "true",
		}
		// Mesmo padrão de cmd/http: relevância quando há busca, fixadas primeiro quando não há.
		if filter.Sort == "" || (filter.Sort == notemodel.NoteSortRelevance && filter.Search == "") {
			filter.Sort = notemodel.NoteSortPinned
			if filter.Search != "" {
				filter.Sort = notemodel.NoteSortRelevance
			}
		}
		notes := fixtureActiveNotes(filter)
		if q.Get("vazio") == "1" {
			notes = nil
		}
		page := notedto.NewNoteListPage(notes, fixtureColors, fixtureTags, filter.Search, filter.Color, filter.Tag, filter.Sort, filter.Pinned)
		page.ActionRedirect = r.URL.RequestURI()
		renderPage(w, r, rt, http.StatusOK, "home.html", page)
	})

	mux.HandleFunc("GET /note/new", func(w http.ResponseWriter, r *http.Request) {
		data := notedto.NewNoteRequest(0, r.URL.Query().Get("title"), r.URL.Query().Get("content"), r.URL.Query().Get("color"), nil, fixtureTags)
		data.CSRFField = csrf.TemplateField(r)
		status := http.StatusOK
		for field, msg := range fieldErrors(r) {
			data.AddFieldError(field, msg)
			status = http.StatusUnprocessableEntity
		}
		renderPage(w, r, rt, status, "note-new.html", data)
	})

	mux.HandleFunc("GET /note/{id}", func(w http.ResponseWriter, r *http.Request) {
		note := noteFromPath(r)
		if note == nil {
			renderAuth(w, r, rt, http.StatusNotFound, "404.html", "nota não encontrada")
			return
		}
		page := notedto.NewNoteResponseFromNote(note)
		page.AttachmentRedirect = fmt.Sprintf("/note/%d", page.Id)
		renderPage(w, r, rt, http.StatusOK, "note-view.html", page)
	})

	mux.HandleFunc("GET /note/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		note := noteFromPath(r)
		if note == nil {
			renderAuth(w, r, rt, http.StatusNotFound, "404.html", "nota não encontrada")
			return
		}
		data := notedto.NewNoteRequestFromNote(note, fixtureTags)
		data.CSRFField = csrf.TemplateField(r)
		data.AttachmentRedirect = fmt.Sprintf("/note/%d/edit", data.Id)
		status := http.StatusOK
		for field, msg := range fieldErrors(r) {
			data.AddFieldError(field, msg)
			status = http.StatusUnprocessableEntity
		}
		renderPage(w, r, rt, status, "note-edit.html", data)
	})

	mux.HandleFunc("GET /notes/archive", func(w http.ResponseWriter, r *http.Request) {
		notes := fixtureNotesWhere(true, false)
		if r.URL.Query().Get("vazio") == "1" {
			notes = nil
		}
		renderPage(w, r, rt, http.StatusOK, "notes-archive.html", notedto.NewNoteResponseFromNoteList(notes))
	})

	mux.HandleFunc("GET /notes/trash", func(w http.ResponseWriter, r *http.Request) {
		notes := fixtureNotesWhere(false, true)
		if r.URL.Query().Get("vazio") == "1" {
			notes = nil
		}
		renderPage(w, r, rt, http.StatusOK, "notes-trash.html", notedto.NewNoteResponseFromNoteList(notes))
	})

	// ?form=create|edit reabre o modal correspondente com os erros de ?erro=campo:msg, como o backend faz após um POST inválido.
	mux.HandleFunc("GET /tags", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		form := notedto.NewNoteTagForm(0, "", "", "/tags")
		status := http.StatusOK
		if errs := fieldErrors(r); len(errs) > 0 {
			id := 0
			if q.Get("form") == "edit" {
				id = 1
			}
			form = notedto.NewNoteTagForm(id, q.Get("name"), q.Get("color"), "/tags")
			for field, msg := range errs {
				form.AddFieldError(field, msg)
			}
			status = http.StatusUnprocessableEntity
		}
		tags := fixtureTags
		if q.Get("vazio") == "1" {
			tags = nil
		}
		renderPage(w, r, rt, status, "note-tags.html", notedto.NewNoteTagsPage(tags, form))
	})

	mux.HandleFunc("GET /me", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/me/overview/personal-information", http.StatusSeeOther)
	})
	overview := func(section string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			page := mePage(r)
			page.NavPath = r.URL.Path
			page.FocusSection = section
			status := http.StatusOK
			switch r.URL.Query().Get("modal") {
			case "edit":
				page.OpenEditModal = true
			case "password":
				page.OpenPasswordModal = true
			case "recovery":
				page.OpenRecoveryEmailModal = true
			case "delete":
				page.OpenDeleteModal = true
			}
			switch r.URL.Query().Get("toast") {
			case "senha":
				page.PasswordChangeSuccess = true
			case "recovery":
				page.RecoveryEmailChangeRequested = true
				page.PendingRecoveryEmail = "novo@exemplo.com"
			}
			for field, msg := range fieldErrors(r) {
				page.AddFieldError(field, msg)
				status = http.StatusUnprocessableEntity
			}
			renderPage(w, r, rt, status, "me.html", page)
		}
	}
	mux.HandleFunc("GET /me/overview/personal-information", overview("personal-information"))
	mux.HandleFunc("GET /me/overview/account-settings", overview("account-settings"))
	mux.HandleFunc("GET /me/overview/privacy-data", overview("privacy-data"))

	security := func(section string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			page := mePage(r)
			page.NavPath = r.URL.Path
			page.FocusSection = section
			page.ActiveSessionLimit = queryInt(r, "sessions", 10)
			page.ActiveSessions = fixtureSessions(queryInt(r, "sessoes", 3))
			renderPage(w, r, rt, http.StatusOK, "me-security.html", page)
		}
	}
	mux.HandleFunc("GET /me/security", security(""))
	mux.HandleFunc("GET /me/security/two-step-verification", security("two-step-verification"))
	mux.HandleFunc("GET /me/security/active-sessions", security("active-sessions"))

	mux.HandleFunc("GET /me/security/two-factor/{method}", func(w http.ResponseWriter, r *http.Request) {
		method := r.PathValue("method")
		var title, description string
		switch method {
		case "email":
			title, description = "Código por e-mail", "Receba um código de verificação no seu e-mail cadastrado."
		case "totp":
			title, description = "Aplicativo autenticador", "Use Google Authenticator, Microsoft Authenticator, Authy, 1Password, Bitwarden, FreeOTP ou outro app compatível com TOTP."
		default:
			renderAuth(w, r, rt, http.StatusNotFound, "404.html", "método inválido")
			return
		}
		page := mePage(r)
		page.NavPath = "/me/security/two-step-verification"
		page.SetSelectedTwoFactorMethod(method, title, description)
		renderPage(w, r, rt, http.StatusOK, "me-security-two-factor-method.html", page)
	})

	// Anexos: um PNG mínimo para a miniatura carregar sem 404 no console; qualquer outro id volta como texto.
	mux.HandleFunc("GET /note/{id}/attachments/{attachmentId}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("attachmentId") == "10" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(tinyPNG)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("anexo de teste"))
	})

	mux.HandleFunc("GET /account/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="bridopen-export.json"`)
		_ = json.NewEncoder(w).Encode(map[string]string{"email": fixtureEmail})
	})
}

/*
mePage monta a conta fixa. ?sem-dados=1 zera os dados pessoais, ?2fa=enabled&method=totp|email liga a verificação em duas etapas e
?ativa=0 simula conta ainda não confirmada, que é o caso em que o JS recusa iniciar a configuração.
*/
func mePage(r *http.Request) userdto.MePage {
	q := r.URL.Query()
	firstName, lastName, phone := fixtureFirstName, fixtureLastName, fixturePhone
	if q.Get("sem-dados") == "1" {
		firstName, lastName, phone = "", "", ""
	}
	created := fixtureTime
	page := userdto.NewMePage(fixtureEmail, q.Get("recovery"), q.Get("pending"), firstName, lastName, phone, &created)
	page.PersonalInfoID = 7
	enabled := q.Get("2fa") == "enabled"
	method := q.Get("method")
	if enabled && method == "" {
		method = "totp"
	}
	page.SetTwoFactorStatus(q.Get("ativa") != "0", enabled, method)
	return page
}

// ---------------------------------------------------------------------------
// Mutações: só o suficiente para os fluxos do JS terminarem como em produção
// ---------------------------------------------------------------------------

func registerMutations(mux *http.ServeMux) {
	redirectTo := func(fallback string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			target := strings.TrimSpace(r.FormValue("redirect"))
			if target == "" || !strings.HasPrefix(target, "/") {
				target = fallback
			}
			http.Redirect(w, r, target, http.StatusSeeOther)
		}
	}
	jsonOK := func(message string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": message, "redirect": "/"})
		}
	}

	mux.HandleFunc("POST /note", redirectTo("/note/1"))
	mux.HandleFunc("POST /note/{id}/{action}", redirectTo("/"))
	mux.HandleFunc("DELETE /note/{id}", jsonOK("Nota movida para a lixeira."))
	mux.HandleFunc("DELETE /note/{id}/destroy", jsonOK("Nota excluída definitivamente."))
	mux.HandleFunc("DELETE /note/{id}/attachments/{attachmentId}", jsonOK("Anexo removido."))

	mux.HandleFunc("POST /tags", redirectTo("/tags"))
	mux.HandleFunc("POST /tags/{id}", redirectTo("/tags"))
	mux.HandleFunc("POST /tags/{id}/delete", redirectTo("/tags"))

	mux.HandleFunc("POST /me/personal", redirectTo("/me/overview/personal-information"))
	mux.HandleFunc("POST /me/password", redirectTo("/me/overview/account-settings?toast=senha"))
	mux.HandleFunc("POST /me/recovery-email", redirectTo("/me/overview/account-settings?toast=recovery"))
	mux.HandleFunc("POST /me/security/sessions/revoke-other", redirectTo("/me/security/active-sessions?sessoes=1"))
	mux.HandleFunc("POST /me/security/sessions/{sessionID}/logout", redirectTo("/me/security/active-sessions?sessoes=2"))
	mux.HandleFunc("POST /account/delete", redirectTo("/user/signin"))
	mux.HandleFunc("POST /user/signout", redirectTo("/user/signin"))
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

// fieldErrors monta erros de campo a partir de ?erro=campo:mensagem (repetível).
// Sem o "campo:", a mensagem vai para "email".
func fieldErrors(r *http.Request) map[string]string {
	raw := r.URL.Query()["erro"]
	if len(raw) == 0 {
		return nil
	}
	errs := make(map[string]string, len(raw))
	for _, item := range raw {
		field, msg, ok := strings.Cut(item, ":")
		if !ok {
			field, msg = "email", item
		}
		errs[field] = msg
	}
	return errs
}

func captcha(r *http.Request, byDefault bool) userdto.AuthCaptcha {
	enabled := byDefault
	switch r.URL.Query().Get("captcha") {
	case "1":
		enabled = true
	case "0":
		enabled = false
	}
	if !enabled {
		return userdto.AuthCaptcha{}
	}
	return userdto.AuthCaptcha{
		Enabled: true,
		ID:      "e2e-captcha",
		Content: captchaImage,
		Open:    r.URL.Query().Get("captcha_aberto") == "1",
	}
}

func noteFromPath(r *http.Request) *notemodel.Note {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil
	}
	return fixtureNoteByID(id)
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func renderAuth(w http.ResponseWriter, r *http.Request, rt *render.RenderTemplate, status int, page string, data any) {
	if err := rt.RenderAuthPage(w, r, status, page, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderPage(w http.ResponseWriter, r *http.Request, rt *render.RenderTemplate, status int, page string, data any) {
	if err := rt.RenderPage(w, r, status, page, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
