package views

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedTemplatesExist verifica que os templates HTML essenciais existem no filesystem embutido.
func TestEmbeddedTemplatesExist(t *testing.T) {
	t.Parallel()

	paths := []string{
		"templates/base.html",
		"templates/base-auth.html",
		"templates/pages/privacy.html",
		"templates/pages/terms.html",
		"templates/pages/cookies.html",
		"templates/pages/user-signin.html",
		"templates/pages/csrf-error.html",
		"templates/pages/404.html",
		"templates/mails/twofactor.html",
		"templates/mails/confirmation.html",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			info, err := fs.Stat(Files, path)
			require.NoError(t, err)
			assert.False(t, info.IsDir())
		})
	}
}

// TestEmbeddedStaticAssetsExist verifica que o diretório static embutido existe e contém arquivos.
func TestEmbeddedStaticAssetsExist(t *testing.T) {
	t.Parallel()

	entries, err := fs.ReadDir(Files, "static")
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
}
