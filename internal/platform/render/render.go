package render

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"html/template"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/engenheiroaraujo/bridopen/views"
	"github.com/gorilla/csrf"
)

const (
	pagesDir = "templates/pages"
	mailsDir = "templates/mails"
)

// pageBaseFiles e authPageBaseFiles compõem, junto de cada arquivo em
// pagesDir, o conjunto parseado no startup (e, em modo live-reload, também a
// cada request).
var pageBaseFiles = []string{
	"templates/base.html",
	"templates/partials/site-footer.html",
	"templates/partials/app-sidebar.html",
	"templates/partials/note-field-errors.html",
	"templates/partials/note-title-field.html",
	"templates/partials/note-tag-picker.html",
	"templates/partials/note-attachments.html",
	"templates/partials/two-factor-method-settings.html",
	"templates/partials/me-overview-sections.html",
	"templates/partials/me-toast-events.html",
	"templates/partials/me-edit-personal-modal.html",
	"templates/partials/me-password-modal.html",
	"templates/partials/me-recovery-email-modal.html",
	"templates/partials/me-account-delete-modal.html",
}

var authPageBaseFiles = []string{
	"templates/base-auth.html",
	"templates/partials/auth-layout.html",
	"templates/partials/site-footer.html",
}

// pagePlaceholderFuncs e authPlaceholderFuncs só precisam existir com o nome e
// a assinatura corretos para o parse no startup; os valores reais são
// religados por request (via Funcs() sobre um clone do template cacheado, ou
// direto no parse quando em modo live-reload).
var pagePlaceholderFuncs = template.FuncMap{
	"csrfField":       func() template.HTML { return "" },
	"csrfToken":       func() string { return "" },
	"isAuthenticated": func() bool { return false },
	"firstName":       func() string { return "" },
	"navPath":         func() string { return "" },
}

var authPlaceholderFuncs = template.FuncMap{
	"csrfField":       func() template.HTML { return "" },
	"csrfToken":       func() string { return "" },
	"isAuthenticated": func() bool { return false },
}

type RenderTemplate struct {
	session     *scs.SessionManager
	baseURL     string
	templatesFS fs.FS
	// liveReload é true em dev local (baseURL com "localhost"): os templates
	// continuam sendo parseados uma vez no startup (para pegar erro de
	// sintaxe cedo), mas cada request reparseia do disco para refletir edições
	// sem precisar reiniciar o processo. Em produção/homologação fica sempre
	// false: nenhum parsing acontece fora do startup.
	liveReload    bool
	pages         map[string]*template.Template
	authPages     map[string]*template.Template
	mailTemplates map[string]*template.Template
}

func NewRender(session *scs.SessionManager, baseURL string) *RenderTemplate {
	baseURL = strings.TrimRight(baseURL, "/")
	liveReload := strings.Contains(baseURL, "localhost")
	templatesFS := templateFSFor(liveReload)

	pages, err := loadPageSet(templatesFS, pagesDir, pageBaseFiles, pagePlaceholderFuncs)
	if err != nil {
		panic(fmt.Sprintf("render: falha ao carregar templates de página: %v", err))
	}

	authPages, err := loadPageSet(templatesFS, pagesDir, authPageBaseFiles, authPlaceholderFuncs)
	if err != nil {
		panic(fmt.Sprintf("render: falha ao carregar templates de autenticação: %v", err))
	}

	mailTemplates, err := loadMailTemplates(templatesFS)
	if err != nil {
		panic(fmt.Sprintf("render: falha ao carregar templates de e-mail: %v", err))
	}

	return &RenderTemplate{
		session:       session,
		baseURL:       baseURL,
		templatesFS:   templatesFS,
		liveReload:    liveReload,
		pages:         pages,
		authPages:     authPages,
		mailTemplates: mailTemplates,
	}
}

// templateFSFor decide, uma única vez na inicialização, se os templates vêm
// do binário (produção/homologação) ou do disco (dev local). Antes essa
// escolha era refeita a cada request lendo r.Host.
func templateFSFor(liveReload bool) fs.FS {
	if liveReload {
		return os.DirFS("views")
	}
	return views.Files
}

// parsePageTemplate parseia baseFiles + dir/page em um único *template.Template.
func parsePageTemplate(fsys fs.FS, dir string, baseFiles []string, funcs template.FuncMap, page string) (*template.Template, error) {
	files := append(append([]string{}, baseFiles...), dir+"/"+page)
	return template.New("").Funcs(funcs).ParseFS(fsys, files...)
}

// loadPageSet parseia, para cada arquivo .html em dir, baseFiles+esse arquivo,
// retornando um *template.Template pronto por nome de página. É usada tanto
// para o conjunto de páginas comuns quanto para o de páginas de autenticação.
func loadPageSet(fsys fs.FS, dir string, baseFiles []string, funcs template.FuncMap) (map[string]*template.Template, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}

	set := make(map[string]*template.Template, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		name := entry.Name()
		t, err := parsePageTemplate(fsys, dir, baseFiles, funcs, name)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", name, err)
		}
		set[name] = t
	}
	return set, nil
}

// parseMailTemplate parseia um único arquivo de templates/mails, sem
// base/partials nem funções customizadas.
func parseMailTemplate(fsys fs.FS, mailTempl string) (*template.Template, error) {
	return template.ParseFS(fsys, mailsDir+"/"+mailTempl)
}

// loadMailTemplates parseia cada arquivo .html em templates/mails isoladamente.
func loadMailTemplates(fsys fs.FS) (map[string]*template.Template, error) {
	entries, err := fs.ReadDir(fsys, mailsDir)
	if err != nil {
		return nil, err
	}

	set := make(map[string]*template.Template, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		name := entry.Name()
		t, err := parseMailTemplate(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("template de e-mail %q: %w", name, err)
		}
		set[name] = t
	}
	return set, nil
}

func (rt *RenderTemplate) sessionExists(ctx context.Context, key string) (exists bool) {
	if rt == nil || rt.session == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			exists = false
		}
	}()
	return rt.session.Exists(ctx, key)
}

func (rt *RenderTemplate) sessionString(ctx context.Context, key string) (value string) {
	if rt == nil || rt.session == nil {
		return ""
	}
	defer func() {
		if recover() != nil {
			value = ""
		}
	}()
	return rt.session.GetString(ctx, key)
}

// PartialNavHeader é o sinal que o cliente (partial-nav.js) envia para pedir só
// o fragmento da rota — título, conteúdo do main e modais da página — em vez da
// página inteira. O shell (sidebar, rodapé, scripts) permanece no DOM.
const (
	PartialNavHeader = "X-Nav-Mode"
	PartialNavValue  = "partial"
)

// pageFragment é o corpo JSON devolvido a uma requisição de navegação parcial.
type pageFragment struct {
	Title  string `json:"title"`
	HTML   string `json:"html"`
	Modals string `json:"modals"`
}

// isPartialNavRequest indica se a requisição pede fragmento. Restrito a GET: o
// header nunca acompanha envio de formulário, e re-render de formulário
// inválido (422 em POST) precisa devolver a página inteira.
func isPartialNavRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.Header.Get(PartialNavHeader) == PartialNavValue
}

func (rt *RenderTemplate) RenderPage(w http.ResponseWriter, r *http.Request, status int, page string, data any) error {
	funcs := template.FuncMap{
		"csrfField": func() template.HTML {
			return csrf.TemplateField(r)
		},
		"csrfToken": func() string {
			return csrf.Token(r)
		},
		"isAuthenticated": func() bool {
			return rt.sessionExists(r.Context(), "userId")
		},
		"firstName": func() string {
			return rt.sessionString(r.Context(), "firstName")
		},
		// navPath permite ao sidebar global marcar o item ativo já no SSR, sem
		// depender de cada página passar NavPath no seu DTO.
		"navPath": func() string {
			return r.URL.Path
		},
	}

	t, err := rt.resolvePageTemplate(rt.pages, pageBaseFiles, funcs, page)
	if err != nil {
		slog.Error("render: falha ao carregar template", "page", page, "err", err)
		return err
	}

	if isPartialNavRequest(r) {
		return rt.writeFragment(w, t, status, page, data)
	}

	buff := &bytes.Buffer{}
	if err := t.ExecuteTemplate(buff, "base", data); err != nil {
		slog.Error("render: falha ao executar template", "page", page, "err", err)
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(status)
	if _, err = buff.WriteTo(w); err != nil {
		slog.Error("render: falha ao escrever resposta", "page", page, "err", err)
		return err
	}
	return nil
}

// writeFragment executa apenas os blocos que compõem o conteúdo trocável e
// devolve JSON. O shell não é renderizado.
func (rt *RenderTemplate) writeFragment(w http.ResponseWriter, t *template.Template, status int, page string, data any) error {
	render := func(name string) (string, error) {
		buff := &bytes.Buffer{}
		if err := t.ExecuteTemplate(buff, name, data); err != nil {
			return "", err
		}
		return buff.String(), nil
	}

	title, err := render("title")
	if err != nil {
		slog.Error("render: falha ao executar title do fragmento", "page", page, "err", err)
		return err
	}
	main, err := render("main")
	if err != nil {
		slog.Error("render: falha ao executar main do fragmento", "page", page, "err", err)
		return err
	}
	// pageModals é um block com padrão vazio: toda página o resolve.
	modals, err := render("pageModals")
	if err != nil {
		slog.Error("render: falha ao executar modais do fragmento", "page", page, "err", err)
		return err
	}

	body, err := json.Marshal(pageFragment{
		Title:  strings.TrimSpace(title) + " — Bridopen",
		HTML:   main,
		Modals: modals,
	})
	if err != nil {
		slog.Error("render: falha ao serializar fragmento", "page", page, "err", err)
		return err
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	// Evita que um proxy/cache sirva o JSON para uma navegação normal.
	w.Header().Set("Vary", PartialNavHeader)
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		slog.Error("render: falha ao escrever fragmento", "page", page, "err", err)
		return err
	}
	return nil
}

func (rt *RenderTemplate) RenderAuthPage(w http.ResponseWriter, r *http.Request, status int, page string, data any) error {
	funcs := template.FuncMap{
		"csrfField": func() template.HTML {
			return csrf.TemplateField(r)
		},
		"csrfToken": func() string {
			return csrf.Token(r)
		},
		"isAuthenticated": func() bool {
			return rt.sessionExists(r.Context(), "userId")
		},
	}

	t, err := rt.resolvePageTemplate(rt.authPages, authPageBaseFiles, funcs, page)
	if err != nil {
		slog.Error("render: falha ao carregar template auth", "page", page, "err", err)
		return err
	}

	buff := &bytes.Buffer{}
	if err := t.ExecuteTemplate(buff, "base-auth", data); err != nil {
		slog.Error("render: falha ao executar template auth", "page", page, "err", err)
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(status)
	if _, err = buff.WriteTo(w); err != nil {
		slog.Error("render: falha ao escrever resposta auth", "page", page, "err", err)
		return err
	}
	return nil
}

// resolvePageTemplate devolve o template pronto para executar nesta request:
// em modo live-reload, reparseia do disco agora (com as funções reais já
// religadas); caso contrário, clona o template cacheado e só religa as
// funções no clone — o cacheado nunca é mutado, então é seguro compartilhá-lo
// entre requests concorrentes.
func (rt *RenderTemplate) resolvePageTemplate(cache map[string]*template.Template, baseFiles []string, funcs template.FuncMap, page string) (*template.Template, error) {
	if rt.liveReload {
		return parsePageTemplate(rt.templatesFS, pagesDir, baseFiles, funcs, page)
	}

	master, ok := cache[page]
	if !ok {
		return nil, fmt.Errorf("render: template não encontrado: %s", page)
	}

	t, err := master.Clone()
	if err != nil {
		return nil, err
	}
	t.Funcs(funcs)
	return t, nil
}

func (rt *RenderTemplate) RenderMailBody(r *http.Request, mailTempl string, data map[string]string) ([]byte, error) {
	data["hostAddr"] = rt.baseURL

	var t *template.Template
	if rt.liveReload {
		parsed, err := parseMailTemplate(rt.templatesFS, mailTempl)
		if err != nil {
			slog.Error(err.Error())
			return nil, err
		}
		t = parsed
	} else {
		cached, ok := rt.mailTemplates[mailTempl]
		if !ok {
			err := fmt.Errorf("render: template de e-mail não encontrado: %s", mailTempl)
			slog.Error(err.Error())
			return nil, err
		}
		t = cached
	}

	w := &bytes.Buffer{}
	if err := t.Execute(w, data); err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	return w.Bytes(), nil
}
