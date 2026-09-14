package validations

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

/*
Toda regra de validação compartilhada do projeto vive aqui. Antes de escrever um
regexp, um limite de tamanho ou uma checagem de formato dentro de um handler,
service ou dto, verifique se já existe o equivalente neste arquivo — e, se não
existir, declare aqui e importe de lá.
*/

// Limites de senha, aplicados no cadastro, na redefinição e na troca de senha.
const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
)

// Limites de dados pessoais.
const (
	MaxPersonalFirstNameLen = 30
	MaxPersonalLastNameLen  = 50
	MaxPersonalPhoneLen     = 20
)

// Limites de nota, etiqueta e anexo.
const (
	MaxNoteTitleLen           = 50
	MaxNoteTags               = 8
	MaxNoteTagNameLen         = 32
	MaxNoteAttachmentsPerNote = 10
)

var (
	emailRegexp = regexp.MustCompile(`(?i)^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)

	// Letras latinas (inclui acentuação portuguesa); permite espaço, hífen e apóstrofo entre partes.
	personalNameRegexp = regexp.MustCompile(`^[\p{Latin}]+(?:[ '\-][\p{Latin}]+)*$`)

	// Telefone opcional: apenas dígitos, espaço, +, (, ), -.
	personalPhoneRegexp = regexp.MustCompile(`^[\d\s()+\-]*$`)
)

// FormValidator acumula erros por campo e decide se o formulário é válido.
type FormValidator struct {
	FieldErrors map[string]string
}

// Valid indica se não há erros de campo registrados.
func (fv *FormValidator) Valid() bool {
	return len(fv.FieldErrors) == 0
}

// AddFieldError registra a mensagem do campo (a última mensagem prevalece).
func (fv *FormValidator) AddFieldError(field, message string) {
	if fv.FieldErrors == nil {
		fv.FieldErrors = make(map[string]string)
	}
	fv.FieldErrors[field] = message
}

// IsEmailValid confere o formato do endereço de e-mail.
func IsEmailValid(email string) bool {
	return emailRegexp.MatchString(strings.TrimSpace(email))
}

// PasswordLengthError devolve a mensagem de tamanho inválido, ou "" quando a
// senha está dentro do aceito. O prefixo permite variar o rótulo ("A senha",
// "A nova senha") mantendo a regra num lugar só.
func PasswordLengthError(prefix, password string) string {
	switch {
	case len(password) < MinPasswordLen:
		return fmt.Sprintf("%s deve ter no mínimo %d caracteres.", prefix, MinPasswordLen)
	case len(password) > MaxPasswordLen:
		return fmt.Sprintf("%s deve ter no máximo %d caracteres.", prefix, MaxPasswordLen)
	}
	return ""
}

// PersonalNameError valida nome/sobrenome, devolvendo a mensagem de erro ou "".
func PersonalNameError(fieldLabel, value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fieldLabel + " é obrigatório"
	}
	if utf8.RuneCountInString(value) > maxLen {
		return fmt.Sprintf("%s deve ter no máximo %d caracteres", fieldLabel, maxLen)
	}
	if !personalNameRegexp.MatchString(value) {
		return fieldLabel + " deve conter apenas letras (com ou sem acento)"
	}
	return ""
}

// PersonalPhoneError valida o telefone (opcional), devolvendo a mensagem ou "".
func PersonalPhoneError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if utf8.RuneCountInString(value) > MaxPersonalPhoneLen {
		return fmt.Sprintf("Telefone deve ter no máximo %d caracteres", MaxPersonalPhoneLen)
	}
	if !personalPhoneRegexp.MatchString(value) {
		return "Telefone inválido: use apenas números, espaço, +, ( ), -"
	}
	return ""
}

// IsHexColor confere se o valor é uma cor hexadecimal no formato #rgb ou
// #rrggbb. É o núcleo compartilhado pelas variantes de sanitização de cor.
func IsHexColor(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "#") {
		return false
	}
	body := value[1:]
	if len(body) != 3 && len(body) != 6 {
		return false
	}
	for _, r := range body {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// TruncateRunes corta o valor em max runas (não bytes), preservando caracteres
// acentuados. max <= 0 devolve string vazia.
func TruncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

// NormalizeTagName limpa o nome da etiqueta: remove o "#" inicial, apara as
// bordas e colapsa espaços internos em um só.
func NormalizeTagName(name string) string {
	name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "#"))
	if name == "" {
		return ""
	}
	return strings.Join(strings.Fields(name), " ")
}

// NormalizeTagIDs descarta ids não positivos e duplicados, preservando a ordem
// da primeira ocorrência.
func NormalizeTagIDs(ids []int) []int {
	seen := make(map[int]bool, len(ids))
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

// NormalizeHexColor apara as bordas e uniformiza para minúsculas. Não valida o
// formato — use IsHexColor para isso.
func NormalizeHexColor(color string) string {
	return strings.ToLower(strings.TrimSpace(color))
}

// NormalizeTagSlug converte um nome de etiqueta em slug: minúsculas, letras e
// dígitos preservados (inclusive acentuados), qualquer outro caractere vira um
// único hífen, sem hífen nas bordas.
func NormalizeTagSlug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
