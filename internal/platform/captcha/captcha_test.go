package captcha

import (
	"testing"

	"github.com/mojocn/base64Captcha"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateCaptcha verifica que GenerateCaptcha retorna id e imagem não vazios sem erro.
func TestGenerateCaptcha(t *testing.T) {
	id, image, err := GenerateCaptcha()
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.NotEmpty(t, image)
}

// TestValidateCaptcha verifica que ValidateCaptcha aceita a resposta correta e rejeita a incorreta.
func TestValidateCaptcha(t *testing.T) {
	const id = "unit-test-captcha-id-platform"
	require.NoError(t, base64Captcha.DefaultMemStore.Set(id, "314159"))
	assert.True(t, ValidateCaptcha(id, "314159"))
	assert.False(t, ValidateCaptcha(id, "000000"))
}
