package views

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptSrcPattern captura o src de cada <script> dos templates base, na ordem
// em que o navegador vai executá-los.
var scriptSrcPattern = regexp.MustCompile(`<script[^>]*\ssrc="([^"]+)"`)

// pageLifecycleScript define window.BridopenPage; todo script que registra
// módulos de página depende dele já ter sido executado.
const pageLifecycleScript = "/static/js/core/page-lifecycle.js"

// expectedBaseScripts é o contrato de quais scripts cada template base
// carrega, sem o cache-buster (?v=N). Mudou a lista no HTML, mude aqui.
var expectedBaseScripts = map[string][]string{
	"templates/base.html": {
		"/static/js/core/theme-init.js",
		"https://cdn.tailwindcss.com",
		"/static/js/core/tailwind-config.js",
		"/static/js/core/page-lifecycle.js",
		"/static/js/core/toast.js",
		"/static/js/notes/note-grid-colors.js",
		"/static/js/notes/note-card-menu.js",
		"/static/js/notes/note-delete-modal.js",
		"/static/js/notes/note-color-picker.js",
		"/static/js/notes/note-title-counter.js",
		"/static/js/notes/note-search-page.js",
		"/static/js/notes/note-reading-pane.js",
		"/static/js/notes/note-tags-page.js",
		"/static/js/account/edit-personal-info.js",
		"/static/js/account/password-change-modal.js",
		"/static/js/account/recovery-email-modal.js",
		"/static/js/account/account-delete-modal.js",
		"/static/js/account/two-factor-settings.js",
		"/static/js/core/action-lock.js",
		"/static/js/core/index.js",
		"/static/js/core/sidebar.js",
		"/static/js/core/partial-nav.js",
	},
	"templates/base-auth.html": {
		"/static/js/core/theme-init.js",
		"https://cdn.tailwindcss.com",
		"/static/js/core/tailwind-config.js",
		"/static/js/core/page-lifecycle.js",
		"/static/js/core/toast.js",
		"/static/js/core/index.js",
	},
}

func baseScripts(t *testing.T, base string) []string {
	t.Helper()
	html, err := fs.ReadFile(Files, base)
	require.NoError(t, err)

	var srcs []string
	for _, m := range scriptSrcPattern.FindAllStringSubmatch(string(html), -1) {
		src := m[1]
		if i := strings.IndexByte(src, '?'); i >= 0 {
			src = src[:i]
		}
		srcs = append(srcs, src)
	}
	return srcs
}

// TestBaseTemplatesLoadExpectedScripts confere, para cada template base, a
// lista e a ordem exatas dos scripts carregados.
func TestBaseTemplatesLoadExpectedScripts(t *testing.T) {
	t.Parallel()

	for base, expected := range expectedBaseScripts {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, expected, baseScripts(t, base))
		})
	}
}

// TestBaseTemplateScriptsExist garante que todo script local referenciado por
// um template base existe no FS embutido.
func TestBaseTemplateScriptsExist(t *testing.T) {
	t.Parallel()

	for base := range expectedBaseScripts {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			for _, src := range baseScripts(t, base) {
				if !strings.HasPrefix(src, "/static/") {
					continue
				}
				info, err := fs.Stat(Files, strings.TrimPrefix(src, "/"))
				require.NoError(t, err, "script %s referenciado em %s não existe", src, base)
				assert.False(t, info.IsDir())
			}
		})
	}
}

// TestBaseTemplatesLoadPageLifecycleBeforeDependents garante que nenhum
// script que use window.BridopenPage é executado antes de page-lifecycle.js.
// Foi exatamente essa ordem faltando em base-auth.html que quebrou o olhinho
// de senha, o captcha e o botão de cadastro nas telas de autenticação.
func TestBaseTemplatesLoadPageLifecycleBeforeDependents(t *testing.T) {
	t.Parallel()

	for base := range expectedBaseScripts {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			lifecycleLoaded := false
			for _, src := range baseScripts(t, base) {
				if src == pageLifecycleScript {
					lifecycleLoaded = true
					continue
				}
				if !strings.HasPrefix(src, "/static/") {
					continue
				}
				js, err := fs.ReadFile(Files, strings.TrimPrefix(src, "/"))
				require.NoError(t, err)
				if !strings.Contains(string(js), "window.BridopenPage") {
					continue
				}
				assert.True(t, lifecycleLoaded,
					"%s usa window.BridopenPage mas %s carrega ele antes de %s", src, base, pageLifecycleScript)
			}
		})
	}
}
