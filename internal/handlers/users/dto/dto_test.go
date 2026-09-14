package dto

import (
	"testing"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMePageTwoFactorStatusUsesCentralLabels verifica que SetTwoFactorStatus preenche rótulos centrais de status e ação do 2FA na MePage.
func TestMePageTwoFactorStatusUsesCentralLabels(t *testing.T) {
	t.Parallel()

	page := NewMePage("user@example.com", "recovery@example.com", "", "", "", "", nil)
	page.SetTwoFactorStatus(true, true, "totp")

	require.True(t, page.TwoFactorEnabled)
	assert.Equal(t, "Habilitado por aplicativo autenticador", page.TwoFactorStatus)
	assert.Equal(t, "Desabilitar", page.TwoFactorActionLabel)
}

// TestTwoFactorMethodLabelUsesCentralLabels verifica que TwoFactorMethodLabel devolve as descrições centrais por método (e-mail, TOTP e padrão).
func TestTwoFactorMethodLabelUsesCentralLabels(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "código enviado para o e-mail cadastrado", TwoFactorMethodLabel("email"))
	assert.Equal(t, "código do aplicativo autenticador", TwoFactorMethodLabel("totp"))
	assert.Equal(t, "código de verificação", TwoFactorMethodLabel("  other  "))
	assert.Equal(t, "código de verificação", TwoFactorMethodLabel("   "))
}

// TestNewMePage verifica que NewMePage normaliza campos, monta nome completo e usa placeholders quando dados faltam.
func TestNewMePage(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	page := NewMePage(
		"a@b.com",
		"  recovery@x.com  ",
		"  pending@x.com  ",
		"  Ana  ",
		"  Silva  ",
		"  1199999  ",
		&created,
	)

	assert.Equal(t, "a@b.com", page.Email)
	assert.Equal(t, "recovery@x.com", page.RecoveryEmail)
	assert.Equal(t, "pending@x.com", page.PendingRecoveryEmail)
	assert.Equal(t, "Ana", page.FirstName)
	assert.Equal(t, "Silva", page.LastName)
	assert.Equal(t, "1199999", page.Phone)
	assert.Equal(t, "1199999", page.PhoneNumber)
	assert.Equal(t, "Ana Silva", page.FullName)
	assert.True(t, page.HasPersonalInfo)
	assert.Equal(t, "/me", page.NavPath)
	assert.Equal(t, created.In(time.Local).Format("02/01/2006"), page.MemberSince)
	assert.False(t, page.TwoFactorEnabled)
	assert.Equal(t, "Desabilitado", page.TwoFactorStatus)

	empty := NewMePage("e@e.com", "", "", "", "", "", nil)
	assert.Equal(t, "Não informado", empty.RecoveryEmail)
	assert.Equal(t, "Não informado", empty.PhoneNumber)
	assert.Equal(t, "Não informado", empty.FullName)
	assert.False(t, empty.HasPersonalInfo)
	assert.Equal(t, "Não informado", empty.MemberSince)
}

// TestMePageSetTwoFactorStatusViewNil verifica que SetTwoFactorStatusView trata view nula e aplica Active/Enabled/labels quando informada.
func TestMePageSetTwoFactorStatusViewNil(t *testing.T) {
	t.Parallel()

	var page MePage
	page.SetTwoFactorStatusView(nil)
	assert.False(t, page.TwoFactorEnabled)
	assert.Equal(t, "Desabilitado", page.TwoFactorStatus)
	assert.Equal(t, "Habilitar", page.TwoFactorActionLabel)

	page.SetTwoFactorStatusView(&models.TwoFactorStatusView{
		Active:      true,
		Enabled:     true,
		Method:      "  email  ",
		StatusLabel: "custom",
		ActionLabel: "act",
	})
	assert.True(t, page.TwoFactorActive)
	assert.True(t, page.TwoFactorEnabled)
	assert.Equal(t, "email", page.TwoFactorMethod)
	assert.Equal(t, "custom", page.TwoFactorStatus)
	assert.Equal(t, "act", page.TwoFactorActionLabel)
}

// TestMePageSetSelectedTwoFactorMethod verifica que SetSelectedTwoFactorMethod faz trim e grava método, título e descrição selecionados.
func TestMePageSetSelectedTwoFactorMethod(t *testing.T) {
	t.Parallel()

	var page MePage
	page.SetSelectedTwoFactorMethod("  totp  ", "  App  ", "  Desc  ")
	assert.Equal(t, "totp", page.SelectedTwoFactorMethod)
	assert.Equal(t, "App", page.SelectedTwoFactorMethodTitle)
	assert.Equal(t, "Desc", page.SelectedTwoFactorMethodDescription)
}

// TestPersonalFieldsFromModel verifica que PersonalFieldsFromModel monta nome/telefone e detecta presença de dados pessoais.
func TestPersonalFieldsFromModel(t *testing.T) {
	t.Parallel()

	full, phone, has := PersonalFieldsFromModel("Ana", "Silva", "1199")
	assert.Equal(t, "Ana Silva", full)
	assert.Equal(t, "1199", phone)
	assert.True(t, has)

	full, phone, has = PersonalFieldsFromModel("", "", "")
	assert.Equal(t, "Não informado", full)
	assert.Equal(t, "Não informado", phone)
	assert.False(t, has)

	_, _, has = PersonalFieldsFromModel("  ", "  X  ", "")
	assert.True(t, has)
}

// TestFormatFullNameBranches verifica os ramos de formatFullName/FormatFullNameForDisplay com nome, sobrenome ou placeholder.
func TestFormatFullNameBranches(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Ana Silva", formatFullName(" Ana ", " Silva "))
	assert.Equal(t, "Ana", formatFullName("Ana", ""))
	assert.Equal(t, "Silva", formatFullName("", "Silva"))
	assert.Equal(t, "Não informado", formatFullName("  ", "  "))
	assert.Equal(t, "Ana Silva", FormatFullNameForDisplay("Ana", "Silva"))
}

// TestDisplayOrPlaceholder verifica que displayOrPlaceholder devolve o valor aparado ou o placeholder padrão.
func TestDisplayOrPlaceholder(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "valor", displayOrPlaceholder("  valor  "))
	assert.Equal(t, "Não informado", displayOrPlaceholder(""))
	assert.Equal(t, "Não informado", displayOrPlaceholder("   "))
	assert.Equal(t, "x", DisplayOrPlaceholder("x"))
}

// TestFormatMemberSinceAndDateTime verifica a formatação de MemberSince e DateTime, inclusive para valores vazios/nulos.
func TestFormatMemberSinceAndDateTime(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Não informado", formatMemberSince(nil))
	ts := time.Date(2023, 12, 1, 8, 5, 0, 0, time.UTC)
	assert.Equal(t, ts.In(time.Local).Format("02/01/2006"), formatMemberSince(&ts))

	assert.Equal(t, "Não informado", formatDateTime(time.Time{}))
	assert.Equal(t, ts.In(time.Local).Format("02/01/2006 15:04"), formatDateTime(ts))
}

// TestDisplayBrowser verifica a identificação do navegador a partir do User-Agent.
func TestDisplayBrowser(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 Edg/120.0", "Microsoft Edge"},
		{"Mozilla/5.0 OPR/90.0", "Opera"},
		{"Opera/9.80", "Opera"},
		{"Mozilla/5.0 Firefox/115.0", "Firefox"},
		{"Mozilla/5.0 Chrome/120.0 Safari/537", "Chrome"},
		{"Mozilla/5.0 Chromium/120.0", "Chrome"},
		{"Mozilla/5.0 Version/17 Safari/605", "Safari"},
		{"curl/8.0", "Navegador desconhecido"},
		{"", "Navegador desconhecido"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want+"_"+tc.ua, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, displayBrowser(tc.ua))
		})
	}
}

// TestDisplayDevice verifica a identificação do dispositivo/SO a partir do User-Agent.
func TestDisplayDevice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (Windows NT 10.0)", "Windows"},
		{"Mozilla/5.0 (Linux; Android 14)", "Android"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS)", "iOS"},
		{"Mozilla/5.0 (iPad; CPU OS)", "iOS"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X)", "macOS"},
		{"Mozilla/5.0 (Mac OS X 10_15)", "macOS"},
		{"Mozilla/5.0 (X11; Linux x86_64)", "Linux"},
		{"UnknownBot/1.0", "dispositivo desconhecido"},
		{"", "dispositivo desconhecido"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want+"_"+tc.ua, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, displayDevice(tc.ua))
		})
	}
}

// TestNewActiveSessionResponse verifica que NewActiveSessionResponse monta sessão ativa com device/browser e marca a atual.
func TestNewActiveSessionResponse(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 1, 2, 3, 4, 0, 0, time.UTC)
	lastSeen := time.Date(2024, 1, 3, 4, 5, 0, 0, time.UTC)
	session := models.UserSession{
		ID:         9,
		SessionID:  "sess-current",
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0) Chrome/120.0 Safari/537",
		IPAddress:  "  1.2.3.4  ",
		CreatedAt:  created,
		LastSeenAt: lastSeen,
	}

	res := NewActiveSessionResponse(session, "  sess-current  ")
	assert.Equal(t, int64(9), res.ID)
	assert.Equal(t, "sess-current", res.SessionID)
	assert.Equal(t, "Windows", res.Device)
	assert.Equal(t, "Chrome", res.Browser)
	assert.Equal(t, "1.2.3.4", res.IPAddress)
	assert.Equal(t, created.In(time.Local).Format("02/01/2006 15:04"), res.CreatedAt)
	assert.Equal(t, lastSeen.In(time.Local).Format("02/01/2006 15:04"), res.LastSeenAt)
	assert.True(t, res.Current)

	empty := NewActiveSessionResponse(models.UserSession{
		SessionID: "other",
		UserAgent: "",
		IPAddress: "",
	}, "sess-current")
	assert.False(t, empty.Current)
	assert.Equal(t, "dispositivo desconhecido", empty.Device)
	assert.Equal(t, "Navegador desconhecido", empty.Browser)
	assert.Equal(t, "Não informado", empty.IPAddress)
	assert.Equal(t, "Não informado", empty.CreatedAt)
	assert.Equal(t, "Não informado", empty.LastSeenAt)
}

// TestNewActiveSessionResponseList verifica que NewActiveSessionResponseList mapeia a lista e marca a sessão corrente.
func TestNewActiveSessionResponseList(t *testing.T) {
	t.Parallel()

	sessions := []models.UserSession{
		{ID: 1, SessionID: "a", UserAgent: "Firefox/1 Windows"},
		{ID: 2, SessionID: "b", UserAgent: "Safari/1 iPhone"},
	}
	list := NewActiveSessionResponseList(sessions, "b")
	require.Len(t, list, 2)
	assert.False(t, list[0].Current)
	assert.True(t, list[1].Current)
	assert.Equal(t, "Firefox", list[0].Browser)
	assert.Equal(t, "iOS", list[1].Device)

	assert.Empty(t, NewActiveSessionResponseList(nil, ""))
}

// TestPasswordsMatch verifica que PasswordsMatch aceita senhas iguais e rejeita divergentes.
func TestPasswordsMatch(t *testing.T) {
	t.Parallel()

	pw, ok := PasswordsMatch("secret", "secret")
	assert.True(t, ok)
	assert.Equal(t, "secret", pw)

	pw, ok = PasswordsMatch("a", "b")
	assert.False(t, ok)
	assert.Empty(t, pw)
}

// TestNewUserRequest verifica que NewUserRequest faz trim do e-mail e preserva a senha.
func TestNewUserRequest(t *testing.T) {
	t.Parallel()

	req := NewUserRequest("  user@example.com  ", "pass")
	assert.Equal(t, "user@example.com", req.Email)
	assert.Equal(t, "pass", req.Password)
}

// TestNewUserPersonalRequest verifica que NewUserPersonalRequest preenche id, userId e campos pessoais.
func TestNewUserPersonalRequest(t *testing.T) {
	t.Parallel()

	req := NewUserPersonalRequest(3, 99, "Ana", "Silva", "1199")
	assert.Equal(t, 3, req.Id)
	assert.Equal(t, int64(99), req.UserId)
	assert.Equal(t, "Ana", req.FirstName)
	assert.Equal(t, "Silva", req.LastName)
	assert.Equal(t, "1199", req.PhoneNumber)
}
