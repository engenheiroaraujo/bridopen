package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTwoFactorStatusView verifica que NewTwoFactorStatusView/FromFields montam labels corretos para nil, desabilitado, e-mail e TOTP.
func TestNewTwoFactorStatusView(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		view := NewTwoFactorStatusView(nil)
		require.NotNil(t, view)
		assert.False(t, view.Enabled)
		assert.Equal(t, "Desabilitado", view.StatusLabel)
		assert.Equal(t, "Habilitar", view.ActionLabel)
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		view := NewTwoFactorStatusView(&TwoFactorStatus{Active: true, Enabled: false})
		assert.True(t, view.Active)
		assert.False(t, view.Enabled)
		assert.Equal(t, "Desabilitado", view.StatusLabel)
		assert.Equal(t, "Habilitar", view.ActionLabel)
	})

	t.Run("email", func(t *testing.T) {
		t.Parallel()
		view := NewTwoFactorStatusView(&TwoFactorStatus{Active: true, Enabled: true, Method: "email"})
		assert.True(t, view.Enabled)
		assert.Equal(t, "email", view.Method)
		assert.Equal(t, "Habilitado por e-mail", view.StatusLabel)
		assert.Equal(t, "Desabilitar", view.ActionLabel)
	})

	t.Run("totp", func(t *testing.T) {
		t.Parallel()
		view := NewTwoFactorStatusViewFromFields(true, true, "totp")
		assert.Equal(t, "Habilitado por aplicativo autenticador", view.StatusLabel)
		assert.Equal(t, "Desabilitar", view.ActionLabel)
	})
}

// TestTwoFactorLabels verifica os rótulos de status, ação e método de login do 2FA.
func TestTwoFactorLabels(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Habilitado por e-mail", TwoFactorStatusLabel("email"))
	assert.Equal(t, "Habilitado por aplicativo autenticador", TwoFactorStatusLabel("totp"))
	assert.Equal(t, "Habilitado", TwoFactorStatusLabel("other"))

	assert.Equal(t, "Desabilitar", TwoFactorActionLabel(true))
	assert.Equal(t, "Habilitar", TwoFactorActionLabel(false))

	assert.Equal(t, "código enviado para o e-mail cadastrado", TwoFactorLoginMethodLabel("email"))
	assert.Equal(t, "código do aplicativo autenticador", TwoFactorLoginMethodLabel("totp"))
	assert.Equal(t, "código de verificação", TwoFactorLoginMethodLabel(""))
}
