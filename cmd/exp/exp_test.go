package main

import (
	"bytes"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/engenheiroaraujo/bridopen/views"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunSuccess verifica que run lista arquivos do FS embutido com sucesso.
func TestRunSuccess(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := run(&buf, views.Files)
	require.NoError(t, err)
	assert.NotEmpty(t, buf.String())
}

// TestRunMissingFile verifica que run retorna ErrNotExist quando o arquivo esperado não existe.
func TestRunMissingFile(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := run(&buf, fstest.MapFS{})
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.NotEmpty(t, buf.String())
}

// TestMainSuccess verifica que main conclui sem exit quando o FS embutido está disponível.
func TestMainSuccess(t *testing.T) {
	originalExit := osExit
	originalOut := stdout
	originalErr := stderr
	t.Cleanup(func() {
		osExit = originalExit
		stdout = originalOut
		stderr = originalErr
		embedFS = views.Files
	})

	var out bytes.Buffer
	stdout = &out
	stderr = &bytes.Buffer{}
	osExit = func(code int) { t.Fatalf("unexpected exit %d", code) }
	embedFS = views.Files

	main()
	assert.NotEmpty(t, out.String())
}

// TestMainErrorExits verifica que main encerra com código 1 quando a listagem falha.
func TestMainErrorExits(t *testing.T) {
	originalExit := osExit
	originalOut := stdout
	originalErr := stderr
	t.Cleanup(func() {
		osExit = originalExit
		stdout = originalOut
		stderr = originalErr
		embedFS = views.Files
	})

	var errBuf bytes.Buffer
	stdout = &bytes.Buffer{}
	stderr = &errBuf
	exited := 0
	osExit = func(code int) {
		exited = code
		panic("exit")
	}
	embedFS = fstest.MapFS{}

	assert.Panics(t, func() { main() })
	assert.Equal(t, 1, exited)
	assert.NotEmpty(t, errBuf.String())
}
