package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHashAndValidatePassword verifica que HashPassword e ValidatePassword aceitam a senha correta e rejeitam a incorreta.
func TestHashAndValidatePassword(t *testing.T) {
	hashed, err := HashPassword("senha-forte")
	require.NoError(t, err)
	assert.True(t, ValidatePassword("senha-forte", hashed))
	assert.False(t, ValidatePassword("outra", hashed))
}

// TestHashPasswordTooLong verifica que HashPassword retorna erro amigável para senhas acima do limite real do bcrypt (72 bytes).
func TestHashPasswordTooLong(t *testing.T) {
	tooLong := strings.Repeat("a", 73)
	_, err := HashPassword(tooLong)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Falha ao gerar a hash da senha")
}
