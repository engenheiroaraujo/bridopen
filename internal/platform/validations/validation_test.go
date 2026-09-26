package validations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsEmailValid verifica aceitação e rejeição de formatos de e-mail em IsEmailValid.
func TestIsEmailValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		email string
		ok    bool
	}{
		{"valid", "user@example.com", true},
		{"trimmed", "  user@example.com  ", true},
		{"plus", "user+tag@example.com", true},
		{"empty", "", false},
		{"no_at", "userexample.com", false},
		{"no_domain", "user@", false},
		{"spaces", "user @example.com", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.ok, IsEmailValid(tc.email))
		})
	}
}

// TestFormValidator verifica Valid, AddFieldError e sobrescrita de erro de campo no FormValidator.
func TestFormValidator(t *testing.T) {
	t.Parallel()

	var fv FormValidator
	assert.True(t, fv.Valid())

	fv.AddFieldError("email", "obrigatório")
	require.False(t, fv.Valid())
	assert.Equal(t, "obrigatório", fv.FieldErrors["email"])

	fv.AddFieldError("email", "inválido")
	assert.Equal(t, "inválido", fv.FieldErrors["email"])
}

// TestPasswordLengthError verifica os limites de senha e o prefixo variável da mensagem.
func TestPasswordLengthError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", PasswordLengthError("A senha", strings.Repeat("a", MinPasswordLen)))
	assert.Equal(t, "", PasswordLengthError("A senha", strings.Repeat("a", MaxPasswordLen)))
	assert.Contains(t, PasswordLengthError("A senha", "curta"), "no mínimo 12")
	assert.Contains(t, PasswordLengthError("A nova senha", strings.Repeat("a", MaxPasswordLen+1)), "no máximo 128")
	assert.Contains(t, PasswordLengthError("A nova senha", "curta"), "A nova senha")
}

// TestPersonalNameError verifica obrigatoriedade, tamanho e caracteres aceitos no nome.
func TestPersonalNameError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", PersonalNameError("Nome", "Ana Paula", MaxPersonalFirstNameLen))
	assert.Equal(t, "", PersonalNameError("Nome", "Jean-Luc", MaxPersonalFirstNameLen))
	assert.Equal(t, "", PersonalNameError("Nome", "Conceição", MaxPersonalFirstNameLen))
	assert.Contains(t, PersonalNameError("Nome", "   ", MaxPersonalFirstNameLen), "obrigatório")
	assert.Contains(t, PersonalNameError("Nome", strings.Repeat("á", MaxPersonalFirstNameLen+1), MaxPersonalFirstNameLen), "no máximo 30")
	assert.Contains(t, PersonalNameError("Nome", "Ana2", MaxPersonalFirstNameLen), "apenas letras")
}

// TestPersonalPhoneError verifica que o telefone é opcional e valida formato e tamanho.
func TestPersonalPhoneError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", PersonalPhoneError(""))
	assert.Equal(t, "", PersonalPhoneError("+55 (11) 99999-9999"))
	assert.Contains(t, PersonalPhoneError(strings.Repeat("9", MaxPersonalPhoneLen+1)), "no máximo 20")
	assert.Contains(t, PersonalPhoneError("telefone"), "inválido")
}

// TestIsHexColor verifica aceitação de #rgb e #rrggbb e rejeição dos demais formatos.
func TestIsHexColor(t *testing.T) {
	t.Parallel()

	assert.True(t, IsHexColor("#abc"))
	assert.True(t, IsHexColor("#AABBCC"))
	assert.True(t, IsHexColor("  #aabbcc  "))
	assert.False(t, IsHexColor(""))
	assert.False(t, IsHexColor("aabbcc"))
	assert.False(t, IsHexColor("#abcd"))
	assert.False(t, IsHexColor("#gggggg"))
}

// TestNormalizeHexColor verifica apara de bordas e conversão para minúsculas.
func TestNormalizeHexColor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "#aabbcc", NormalizeHexColor("  #AABBCC "))
	assert.Equal(t, "", NormalizeHexColor("   "))
}

// TestTruncateRunes verifica corte por runas, preservando acentuação.
func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", TruncateRunes("abc", 0))
	assert.Equal(t, "", TruncateRunes("abc", -1))
	assert.Equal(t, "abc", TruncateRunes("abc", 5))
	assert.Equal(t, "áé", TruncateRunes("áéíóu", 2))
}

// TestNormalizeTagName verifica remoção de "#", apara e colapso de espaços.
func TestNormalizeTagName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Hello World", NormalizeTagName("  #Hello   World  "))
	assert.Equal(t, "", NormalizeTagName("   #   "))
	assert.Equal(t, "", NormalizeTagName(""))
}

// TestNormalizeTagIDs verifica descarte de ids não positivos e duplicados, preservando a ordem.
func TestNormalizeTagIDs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []int{1, 3}, NormalizeTagIDs([]int{1, 0, -2, 1, 3}))
	assert.Empty(t, NormalizeTagIDs(nil))
}

// TestNormalizeTagSlug verifica a geração de slug a partir do nome da etiqueta.
func TestNormalizeTagSlug(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "trabalho", NormalizeTagSlug("  Trabalho  "))
	assert.Equal(t, "ana-paula", NormalizeTagSlug("Ana Paula"))
	assert.Equal(t, "reunião-1", NormalizeTagSlug("Reunião #1"))
	assert.Equal(t, "", NormalizeTagSlug("###"))
}
