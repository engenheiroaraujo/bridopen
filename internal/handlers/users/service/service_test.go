package service

import (
	"context"
	"errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	platformpassword "github.com/engenheiroaraujo/bridopen/internal/platform/password"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequestRecoveryEmailChangeTokenAndPending verifica sucesso com token/pending na troca de recuperação.
func TestRequestRecoveryEmailChangeTokenAndPending(t *testing.T) {
	hash, err := platformpassword.HashPassword("current-password")
	require.NoError(t, err)

	users := newFakeUserRepository()
	users.account = &models.User{
		Email:                pgtype.Text{String: "user@example.com", Valid: true},
		PendingRecoveryEmail: pgtype.Text{String: "pending@example.com", Valid: true},
	}
	users.passwordHash = hash
	users.active = true
	users.beginPrimary = "user@example.com"
	users.beginToken = "tok"

	svc := NewAccountService(users, newFakeUserPersonalRepository(), nil, nil)
	_, _, _, token, err := svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.NoError(t, err)
	assert.Equal(t, "tok", token)
}

// TestEncryptDecryptKeyAndOpenErrors verifica erros de chave curta e ciphertext corrompido em encrypt/decrypt.
func TestEncryptDecryptKeyAndOpenErrors(t *testing.T) {
	service := NewTwoFactorService(nil, nil, "secret-with-more-than-32-characters")
	service.encryptionKey = []byte("short")
	_, err := service.encrypt("x")
	require.Error(t, err)
	_, err = service.decrypt([]byte("abcdefghijklmnop"))
	require.Error(t, err)

	service = NewTwoFactorService(nil, nil, "secret-with-more-than-32-characters")
	enc, err := service.encrypt("hello")
	require.NoError(t, err)
	enc[len(enc)-1] ^= 0xff
	_, err = service.decrypt(enc)
	require.Error(t, err)
}

// TestVerifySetupTOTPConsumeError verifica propagação de erro de ConsumeActiveChallenge em VerifySetup TOTP.
func TestVerifySetupTOTPConsumeError(t *testing.T) {
	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "u@e.com", Active: true}
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	secret := "JBSWY3DPEHPK3PXP"
	enc, err := service.encrypt(secret)
	require.NoError(t, err)
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodTOTP, Purpose: models.TwoFactorPurposeEnable,
		TOTPSecretEncrypted: enc, ExpiresAt: time.Now().Add(time.Minute),
	})
	code, err := totpCode(secret, time.Now().UTC())
	require.NoError(t, err)
	repo.consumeErr = errors.New("consume boom")
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodTOTP, code)
	require.ErrorIs(t, err, repo.consumeErr)
}

// TestVerifyDisableChallengeRepoError verifica propagação de erro ao buscar challenge em VerifyDisable.
func TestVerifyDisableChallengeRepoError(t *testing.T) {
	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	repo.getChallengeErr = errors.New("challenge boom")
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), repo.getChallengeErr)
}

// TestNewAccountService verifica que NewAccountService injeta os repositórios informados.
func TestNewAccountService(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	personal := newFakeUserPersonalRepository()
	twoFactor := newFakeTwoFactorRepository()
	sessions := newFakeUserSessionRepository()

	svc := NewAccountService(users, personal, twoFactor, sessions)
	require.NotNil(t, svc)
	assert.Equal(t, users, svc.users)
	assert.Equal(t, personal, svc.personal)
	assert.Equal(t, twoFactor, svc.twoFactor)
	assert.Equal(t, sessions, svc.sessionActive)
}

// TestNormalizeActiveSessionLimit verifica que NormalizeActiveSessionLimit aplica faixa válida e default 10.
func TestNormalizeActiveSessionLimit(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 10, NormalizeActiveSessionLimit(10))
	assert.Equal(t, 100, NormalizeActiveSessionLimit(100))
	assert.Equal(t, 250, NormalizeActiveSessionLimit(250))
	assert.Equal(t, 10, NormalizeActiveSessionLimit(0))
	assert.Equal(t, 10, NormalizeActiveSessionLimit(7))
	assert.Equal(t, 10, NormalizeActiveSessionLimit(-1))
}

// TestValidatePersonalForm verifica validação do formulário de dados pessoais (obrigatórios e telefone).
func TestValidatePersonalForm(t *testing.T) {
	t.Parallel()

	errs := ValidatePersonalForm("Maria", "Silva", "(11) 99999-9999")
	assert.Empty(t, errs)

	errs = ValidatePersonalForm("", "", "abc")
	assert.Equal(t, "Nome é obrigatório", errs["first_name"])
	assert.Equal(t, "Sobrenome é obrigatório", errs["last_name"])
	assert.Contains(t, errs["phone_number"], "Telefone inválido")
}

// TestValidateRecoveryEmailChangeForm verifica validação do formulário de troca de e-mail de recuperação.
func TestValidateRecoveryEmailChangeForm(t *testing.T) {
	t.Parallel()

	errs := ValidateRecoveryEmailChangeForm("user@example.com", "recovery@example.com", "", "password123", "new@example.com")
	assert.Empty(t, errs)

	errs = ValidateRecoveryEmailChangeForm("user@example.com", "", "", "", "")
	assert.Equal(t, "Informe a senha atual.", errs["current_password"])
	assert.Equal(t, "Informe o e-mail de recuperação.", errs["recovery_email"])

	errs = ValidateRecoveryEmailChangeForm("user@example.com", "", "", "password123", "not-an-email")
	assert.Equal(t, "E-mail de recuperação inválido.", errs["recovery_email"])

	errs = ValidateRecoveryEmailChangeForm("user@example.com", "", "", "password123", "user@example.com")
	assert.Equal(t, "Use um e-mail diferente do e-mail principal.", errs["recovery_email"])

	errs = ValidateRecoveryEmailChangeForm("user@example.com", "recovery@example.com", "", "password123", "recovery@example.com")
	assert.Equal(t, "Este e-mail já está configurado como recuperação.", errs["recovery_email"])
}

// TestValidatePasswordChangeForm verifica validação do formulário de troca de senha (tamanho, confirmação e diferença).
func TestValidatePasswordChangeForm(t *testing.T) {
	t.Parallel()

	errs := ValidatePasswordChangeForm("current-password", "new-password-12", "new-password-12")
	assert.Empty(t, errs)

	errs = ValidatePasswordChangeForm("", "", "")
	assert.Equal(t, "Informe a senha atual.", errs["current_password"])
	assert.Equal(t, "A nova senha deve ter no mínimo 12 caracteres.", errs["password"])
	assert.Equal(t, "Confirme a nova senha.", errs["password_confirm"])

	errs = ValidatePasswordChangeForm("current-password", strings.Repeat("a", 129), strings.Repeat("a", 129))
	assert.Equal(t, "A nova senha deve ter no máximo 128 caracteres.", errs["password"])

	errs = ValidatePasswordChangeForm("current-password", "new-password-12", "different-confirm")
	assert.Equal(t, "As senhas não conferem.", errs["password_confirm"])

	errs = ValidatePasswordChangeForm("same-password-1", "same-password-1", "same-password-1")
	assert.Equal(t, "A nova senha deve ser diferente da senha atual.", errs["password"])
}

// TestTwoFactorMessage verifica que apperrors.TwoFactorMessage traduz erros 2FA conhecidos em mensagens amigáveis.
func TestTwoFactorMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		want string
	}{
		{apperrors.ErrRecoveryCodesUnavailable, "Não foi possível gerar códigos de recuperação agora."},
		{apperrors.ErrTwoFactorInvalidCode, "Código de verificação inválido."},
		{apperrors.ErrTwoFactorChallengeExpired, "Código expirado. Faça login novamente para receber um novo código."},
		{apperrors.ErrTwoFactorTooManyAttempts, "Limite de tentativas excedido. Faça login novamente."},
		{apperrors.ErrTwoFactorCooldown, "Aguarde um pouco antes de solicitar outro código."},
		{apperrors.ErrTwoFactorRequiresConfirmed, "A conta precisa estar confirmada para usar verificação em duas etapas."},
		{apperrors.ErrTwoFactorInvalidMethod, "Método de verificação em duas etapas inválido."},
		{apperrors.ErrTwoFactorAlreadyEnabled, "A verificação em duas etapas já está ativa."},
		{apperrors.ErrTwoFactorDisabled, "A verificação em duas etapas já está desativada."},
		{errors.New("other"), "Não foi possível validar o código agora."},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, apperrors.TwoFactorMessage(tc.err))
	}
}

// TestBuildMePage verifica que BuildMePage agrega conta, pessoais e status 2FA na MePage.
func TestBuildMePage(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	personal := newFakeUserPersonalRepository()
	twoFactor := newFakeTwoFactorRepository()
	created := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	users.account = &models.User{
		Email:                pgtype.Text{String: "user@example.com", Valid: true},
		RecoveryEmail:        pgtype.Text{String: "recovery@example.com", Valid: true},
		PendingRecoveryEmail: pgtype.Text{String: "pending@example.com", Valid: true},
		CreatedAt:            pgtype.Date{Time: created, Valid: true},
	}
	personal.info = &models.UserPersonalInformation{
		Id:          pgtype.Numeric{Int: big.NewInt(9), Valid: true},
		FirstName:   pgtype.Text{String: "Ana", Valid: true},
		LastName:    pgtype.Text{String: "Souza", Valid: true},
		PhoneNumber: pgtype.Text{String: "11999990000", Valid: true},
	}
	twoFactor.status = &models.TwoFactorStatus{Active: true, Enabled: true, Method: "email"}

	svc := NewAccountService(users, personal, twoFactor, nil)
	page, err := svc.BuildMePage(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", page.Email)
	assert.Contains(t, page.RecoveryEmail, "recovery@example.com")
	assert.Equal(t, "pending@example.com", page.PendingRecoveryEmail)
	assert.Equal(t, "Ana", page.FirstName)
	assert.Equal(t, "Souza", page.LastName)
	assert.Equal(t, "11999990000", page.Phone)
	assert.Equal(t, 9, page.PersonalInfoID)
	assert.NotEmpty(t, page.MemberSince)
	assert.True(t, page.TwoFactorEnabled)
	_ = created
}

// TestBuildMePageWithoutPersonalAndTwoFactor verifica BuildMePage sem dados pessoais e sem repositório 2FA.
func TestBuildMePageWithoutPersonalAndTwoFactor(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	personal := newFakeUserPersonalRepository()
	users.account = &models.User{
		Email: pgtype.Text{String: "user@example.com", Valid: true},
	}

	svc := NewAccountService(users, personal, nil, nil)
	page, err := svc.BuildMePage(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", page.Email)
	assert.Empty(t, page.FirstName)
	assert.Zero(t, page.PersonalInfoID)
	assert.False(t, page.TwoFactorEnabled)
}

// TestBuildMePageErrors verifica propagação de erros de conta, pessoais e 2FA em BuildMePage.
func TestBuildMePageErrors(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	personal := newFakeUserPersonalRepository()
	twoFactor := newFakeTwoFactorRepository()
	users.findAccountErr = errors.New("account boom")
	svc := NewAccountService(users, personal, twoFactor, nil)
	_, err := svc.BuildMePage(context.Background(), 1)
	require.ErrorIs(t, err, users.findAccountErr)

	users.findAccountErr = nil
	users.account = &models.User{Email: pgtype.Text{String: "a@b.com", Valid: true}}
	personal.findErr = errors.New("personal boom")
	_, err = svc.BuildMePage(context.Background(), 1)
	require.ErrorIs(t, err, personal.findErr)

	personal.findErr = nil
	twoFactor.getStatusErr = errors.New("2fa boom")
	_, err = svc.BuildMePage(context.Background(), 1)
	require.ErrorIs(t, err, twoFactor.getStatusErr)
}

// TestBuildSecurityPage verifica que BuildSecurityPage lista sessões ativas e marca a corrente.
func TestBuildSecurityPage(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	personal := newFakeUserPersonalRepository()
	sessions := newFakeUserSessionRepository()
	users.account = &models.User{Email: pgtype.Text{String: "user@example.com", Valid: true}}
	sessions.sessions = []models.UserSession{
		{SessionID: "current", UserAgent: "ua", IPAddress: "1.1.1.1"},
		{SessionID: "other", UserAgent: "ua2", IPAddress: "2.2.2.2"},
	}

	svc := NewAccountService(users, personal, nil, sessions)
	page, err := svc.BuildSecurityPage(context.Background(), 1, "current", 100)
	require.NoError(t, err)
	assert.Equal(t, "/me/security", page.NavPath)
	assert.Equal(t, 100, page.ActiveSessionLimit)
	require.Len(t, page.ActiveSessions, 2)
	assert.True(t, page.ActiveSessions[0].Current)
	assert.False(t, page.ActiveSessions[1].Current)
}

// TestBuildSecurityPageNilSessionRepo verifica BuildSecurityPage com session repo nulo retorna página sem sessões.
func TestBuildSecurityPageNilSessionRepo(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	personal := newFakeUserPersonalRepository()
	users.account = &models.User{Email: pgtype.Text{String: "user@example.com", Valid: true}}
	svc := NewAccountService(users, personal, nil, nil)

	page, err := svc.BuildSecurityPage(context.Background(), 1, "sid", 10)
	require.NoError(t, err)
	assert.Equal(t, "/me/security", page.NavPath)
	assert.Empty(t, page.ActiveSessions)
}

// TestBuildSecurityPageErrors verifica erros de conta e listagem em BuildSecurityPage.
func TestBuildSecurityPageErrors(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	personal := newFakeUserPersonalRepository()
	sessions := newFakeUserSessionRepository()
	users.findAccountErr = errors.New("boom")
	svc := NewAccountService(users, personal, nil, sessions)
	_, err := svc.BuildSecurityPage(context.Background(), 1, "sid", 10)
	require.Error(t, err)

	users.findAccountErr = nil
	users.account = &models.User{Email: pgtype.Text{String: "user@example.com", Valid: true}}
	sessions.listErr = errors.New("list boom")
	_, err = svc.BuildSecurityPage(context.Background(), 1, "sid", 10)
	require.ErrorIs(t, err, sessions.listErr)
}

// TestRevokeSessions verifica RevokeOtherSessions/RevokeSession com trim, sessão vazia e repo nulo.
func TestRevokeSessions(t *testing.T) {
	t.Parallel()

	sessions := newFakeUserSessionRepository()
	svc := NewAccountService(newFakeUserRepository(), newFakeUserPersonalRepository(), nil, sessions)

	require.NoError(t, svc.RevokeOtherSessions(context.Background(), 1, " current "))
	assert.Equal(t, "current", sessions.revokedOtherFor)
	require.NoError(t, svc.RevokeSession(context.Background(), 1, " sid "))
	assert.Equal(t, "sid", sessions.revoked)

	require.ErrorIs(t, svc.RevokeOtherSessions(context.Background(), 1, " "), apperrors.ErrNotFound)
	require.ErrorIs(t, svc.RevokeSession(context.Background(), 1, ""), apperrors.ErrNotFound)

	svcNil := NewAccountService(newFakeUserRepository(), newFakeUserPersonalRepository(), nil, nil)
	require.ErrorIs(t, svcNil.RevokeOtherSessions(context.Background(), 1, "sid"), apperrors.ErrNotFound)
	require.ErrorIs(t, svcNil.RevokeSession(context.Background(), 1, "sid"), apperrors.ErrNotFound)
}

// TestCancelRecoveryEmailChange verifica que CancelRecoveryEmailChange encaminha userID e pending aparados ao repositório.
func TestCancelRecoveryEmailChange(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepository()
	svc := NewAccountService(users, newFakeUserPersonalRepository(), nil, nil)
	require.NoError(t, svc.CancelRecoveryEmailChange(context.Background(), 7, " pending@example.com "))
	assert.Equal(t, int64(7), users.cancelUserID)
	assert.Equal(t, "pending@example.com", users.cancelPending)
}

// TestRequestRecoveryEmailChange verifica fluxo feliz de solicitação de troca de e-mail de recuperação.
func TestRequestRecoveryEmailChange(t *testing.T) {
	t.Parallel()

	hash, err := platformpassword.HashPassword("current-password")
	require.NoError(t, err)

	users := newFakeUserRepository()
	users.account = &models.User{
		Email:         pgtype.Text{String: "user@example.com", Valid: true},
		RecoveryEmail: pgtype.Text{String: "old@example.com", Valid: true},
	}
	users.passwordHash = hash
	users.active = true
	users.beginPrimary = "user@example.com"
	users.beginToken = "token-abc"

	svc := NewAccountService(users, newFakeUserPersonalRepository(), nil, nil)
	fieldErrors, primary, pending, token, err := svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.NoError(t, err)
	assert.Nil(t, fieldErrors)
	assert.Equal(t, "user@example.com", primary)
	assert.Equal(t, "new@example.com", pending)
	assert.Equal(t, "token-abc", token)
}

// TestRequestRecoveryEmailChangeValidationAndErrors verifica validação e erros (senha, inativo, e-mail em uso, begin) na troca de recuperação.
func TestRequestRecoveryEmailChangeValidationAndErrors(t *testing.T) {
	t.Parallel()

	hash, err := platformpassword.HashPassword("current-password")
	require.NoError(t, err)

	users := newFakeUserRepository()
	users.account = &models.User{Email: pgtype.Text{String: "user@example.com", Valid: true}}
	users.passwordHash = hash
	users.active = true
	svc := NewAccountService(users, newFakeUserPersonalRepository(), nil, nil)

	fieldErrors, _, _, _, err := svc.RequestRecoveryEmailChange(context.Background(), 1, "", "")
	require.NoError(t, err)
	assert.NotEmpty(t, fieldErrors)

	users.findAccountErr = errors.New("find boom")
	_, _, _, _, err = svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.ErrorIs(t, err, users.findAccountErr)
	users.findAccountErr = nil

	users.findPasswordErr = errors.New("pwd boom")
	_, _, _, _, err = svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.ErrorIs(t, err, users.findPasswordErr)
	users.findPasswordErr = nil

	users.active = false
	_, _, _, _, err = svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.ErrorIs(t, err, apperrors.ErrUserInactive)
	users.active = true

	fieldErrors, _, _, _, err = svc.RequestRecoveryEmailChange(context.Background(), 1, "wrong-password", "new@example.com")
	require.NoError(t, err)
	assert.Equal(t, "Senha atual incorreta.", fieldErrors["current_password"])

	users.emailInUse = true
	fieldErrors, _, _, _, err = svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.NoError(t, err)
	assert.Equal(t, "Este e-mail não pode ser usado como recuperação.", fieldErrors["recovery_email"])
	users.emailInUse = false

	users.emailInUseErr = errors.New("inuse boom")
	_, _, _, _, err = svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.ErrorIs(t, err, users.emailInUseErr)
	users.emailInUseErr = nil

	users.beginErr = errors.New("begin boom")
	_, _, _, _, err = svc.RequestRecoveryEmailChange(context.Background(), 1, "current-password", "new@example.com")
	require.ErrorIs(t, err, users.beginErr)
}

// TestSavePersonalInformation verifica create/update de dados pessoais, IDs inválidos e erros de repositório.
func TestSavePersonalInformation(t *testing.T) {
	t.Parallel()

	personal := newFakeUserPersonalRepository()
	svc := NewAccountService(newFakeUserRepository(), personal, nil, nil)

	created, err := svc.SavePersonalInformation(context.Background(), 1, 0, " Ana ", " Silva ", " 1199 ")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "Ana", personal.createdFirst)
	assert.Equal(t, "Silva", personal.createdLast)
	assert.Equal(t, "1199", personal.createdPhone)

	personal.info = &models.UserPersonalInformation{Id: pgtype.Numeric{Int: big.NewInt(5), Valid: true}}
	created, err = svc.SavePersonalInformation(context.Background(), 1, 0, "Ana", "Silva", "1199")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, 5, personal.updatedID)

	created, err = svc.SavePersonalInformation(context.Background(), 1, 5, "Ana", "Silva", "1199")
	require.NoError(t, err)
	assert.False(t, created)

	_, err = svc.SavePersonalInformation(context.Background(), 1, 99, "Ana", "Silva", "1199")
	require.ErrorContains(t, err, "identificador de dados pessoais inválido")

	personal.info = nil
	_, err = svc.SavePersonalInformation(context.Background(), 1, 3, "Ana", "Silva", "1199")
	require.ErrorContains(t, err, "identificador de dados pessoais inválido")

	personal.findErr = errors.New("find boom")
	_, err = svc.SavePersonalInformation(context.Background(), 1, 0, "Ana", "Silva", "1199")
	require.ErrorIs(t, err, personal.findErr)
	personal.findErr = nil

	personal.info = &models.UserPersonalInformation{Id: pgtype.Numeric{Int: big.NewInt(5), Valid: true}}
	personal.updateErr = apperrors.ErrNotFound
	_, err = svc.SavePersonalInformation(context.Background(), 1, 5, "Ana", "Silva", "1199")
	require.ErrorIs(t, err, apperrors.ErrNotFound)

	personal.updateErr = errors.New("update boom")
	_, err = svc.SavePersonalInformation(context.Background(), 1, 5, "Ana", "Silva", "1199")
	require.ErrorIs(t, err, personal.updateErr)

	personal.info = nil
	personal.createErr = errors.New("create boom")
	_, err = svc.SavePersonalInformation(context.Background(), 1, 0, "Ana", "Silva", "1199")
	require.ErrorIs(t, err, personal.createErr)
}

// TestChangePassword verifica troca de senha com validação, senha atual e falhas de repositório.
func TestChangePassword(t *testing.T) {
	t.Parallel()

	hash, err := platformpassword.HashPassword("current-password")
	require.NoError(t, err)

	users := newFakeUserRepository()
	users.passwordHash = hash
	users.active = true
	svc := NewAccountService(users, newFakeUserPersonalRepository(), nil, nil)

	fieldErrors, err := svc.ChangePassword(context.Background(), 1, "current-password", "brand-new-pass", "brand-new-pass")
	require.NoError(t, err)
	assert.Nil(t, fieldErrors)
	assert.NotEmpty(t, users.updatedPassword)
	assert.True(t, platformpassword.ValidatePassword("brand-new-pass", users.updatedPassword))

	fieldErrors, err = svc.ChangePassword(context.Background(), 1, "", "short", "short")
	require.NoError(t, err)
	assert.NotEmpty(t, fieldErrors)

	users.findPasswordErr = errors.New("pwd boom")
	_, err = svc.ChangePassword(context.Background(), 1, "current-password", "brand-new-pass", "brand-new-pass")
	require.ErrorIs(t, err, users.findPasswordErr)
	users.findPasswordErr = nil

	users.active = false
	_, err = svc.ChangePassword(context.Background(), 1, "current-password", "brand-new-pass", "brand-new-pass")
	require.ErrorIs(t, err, apperrors.ErrUserInactive)
	users.active = true

	fieldErrors, err = svc.ChangePassword(context.Background(), 1, "wrong-password", "brand-new-pass", "brand-new-pass")
	require.NoError(t, err)
	assert.Equal(t, "Senha atual incorreta.", fieldErrors["current_password"])

	// Mesma senha do hash atual (o formulário permite strings diferentes que batem no hash — raro; use a atual exata).
	fieldErrors, err = svc.ChangePassword(context.Background(), 1, "current-password", "current-password", "current-password")
	require.NoError(t, err)
	assert.Equal(t, "A nova senha deve ser diferente da senha atual.", fieldErrors["password"])

	users.updatePasswordErr = errors.New("update boom")
	_, err = svc.ChangePassword(context.Background(), 1, "current-password", "another-new-pw", "another-new-pw")
	require.ErrorIs(t, err, users.updatePasswordErr)
	users.updatePasswordErr = nil

	// bcrypt rejeita senhas com mais de 72 bytes; o formulário permite até 128.
	longPassword := strings.Repeat("a", 80)
	_, err = svc.ChangePassword(context.Background(), 1, "current-password", longPassword, longPassword)
	require.Error(t, err)
}

type fakeUserRepository struct {
	account           *models.User
	findAccountErr    error
	passwordHash      string
	active            bool
	findPasswordErr   error
	emailInUse        bool
	emailInUseErr     error
	beginPrimary      string
	beginToken        string
	beginErr          error
	cancelUserID      int64
	cancelPending     string
	cancelErr         error
	updatedPassword   string
	updatePasswordErr error
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{active: true}
}

func (f *fakeUserRepository) FindByEmail(context.Context, string) (bool, error) {
	return false, nil
}
func (f *fakeUserRepository) FindByUser(context.Context, string) (*models.User, *models.UserPersonalInformation, error) {
	return nil, nil, nil
}
func (f *fakeUserRepository) CreateResetPasswordToken(context.Context, string, string) (string, []string, error) {
	return "", nil, nil
}
func (f *fakeUserRepository) CreateEmailConfirmationToken(context.Context, string, string) (string, []string, error) {
	return "", nil, nil
}
func (f *fakeUserRepository) GetUserConfirmationByToken(context.Context, string, string) (*models.User, *models.UserConfirmationToken, string, error) {
	return nil, nil, "", nil
}
func (f *fakeUserRepository) PutPasswordByToken(context.Context, string, string) (string, int64, error) {
	return "", 0, nil
}
func (f *fakeUserRepository) Create(context.Context, string, string, string, string, string) (*models.User, *models.UserPersonalInformation, string, error) {
	return nil, nil, "", nil
}
func (f *fakeUserRepository) ConfirmUserByToken(context.Context, string) (string, int64, error) {
	return "", 0, nil
}
func (f *fakeUserRepository) DeleteExpiredTokens(context.Context) error { return nil }
func (f *fakeUserRepository) ExportUserData(context.Context, int64) (*models.AccountExport, error) {
	return nil, nil
}
func (f *fakeUserRepository) ListAttachmentStorageKeys(context.Context, int64) ([]string, error) {
	return nil, nil
}
func (f *fakeUserRepository) DeleteUserAndData(context.Context, int64) error { return nil }
func (f *fakeUserRepository) ConfirmRecoveryEmailByToken(context.Context, string) (string, string, int64, error) {
	return "", "", 0, nil
}

func (f *fakeUserRepository) FindAccountBasics(context.Context, int64) (*models.User, error) {
	if f.findAccountErr != nil {
		return nil, f.findAccountErr
	}
	if f.account == nil {
		return nil, apperrors.ErrNotFound
	}
	cp := *f.account
	return &cp, nil
}

func (f *fakeUserRepository) EmailAddressInUse(context.Context, int64, string) (bool, error) {
	if f.emailInUseErr != nil {
		return false, f.emailInUseErr
	}
	return f.emailInUse, nil
}

func (f *fakeUserRepository) BeginRecoveryEmailChange(_ context.Context, _ int64, _, _ string) (string, string, error) {
	if f.beginErr != nil {
		return "", "", f.beginErr
	}
	return f.beginPrimary, f.beginToken, nil
}

func (f *fakeUserRepository) CancelRecoveryEmailChange(_ context.Context, userID int64, pendingEmail string) error {
	f.cancelUserID = userID
	f.cancelPending = pendingEmail
	return f.cancelErr
}

func (f *fakeUserRepository) FindPasswordAuthData(context.Context, int64) (string, string, bool, error) {
	if f.findPasswordErr != nil {
		return "", "", false, f.findPasswordErr
	}
	return "user@example.com", f.passwordHash, f.active, nil
}

func (f *fakeUserRepository) UpdatePasswordByID(_ context.Context, _ int64, newPassword string) error {
	if f.updatePasswordErr != nil {
		return f.updatePasswordErr
	}
	f.updatedPassword = newPassword
	return nil
}

type fakeUserPersonalRepository struct {
	info         *models.UserPersonalInformation
	findErr      error
	createErr    error
	updateErr    error
	createdFirst string
	createdLast  string
	createdPhone string
	updatedID    int
}

func newFakeUserPersonalRepository() *fakeUserPersonalRepository {
	return &fakeUserPersonalRepository{}
}

func (f *fakeUserPersonalRepository) FindByUserID(context.Context, int64) (*models.UserPersonalInformation, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.info, nil
}

func (f *fakeUserPersonalRepository) Create(_ context.Context, _ int64, firstName, lastName, phoneNumber string) (*models.UserPersonalInformation, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.createdFirst = firstName
	f.createdLast = lastName
	f.createdPhone = phoneNumber
	return &models.UserPersonalInformation{
		Id:          pgtype.Numeric{Int: big.NewInt(1), Valid: true},
		FirstName:   pgtype.Text{String: firstName, Valid: true},
		LastName:    pgtype.Text{String: lastName, Valid: true},
		PhoneNumber: pgtype.Text{String: phoneNumber, Valid: true},
	}, nil
}

func (f *fakeUserPersonalRepository) Update(_ context.Context, _ int64, id int, firstName, lastName, phoneNumber string) (*models.UserPersonalInformation, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updatedID = id
	return &models.UserPersonalInformation{
		Id:          pgtype.Numeric{Int: big.NewInt(int64(id)), Valid: true},
		FirstName:   pgtype.Text{String: firstName, Valid: true},
		LastName:    pgtype.Text{String: lastName, Valid: true},
		PhoneNumber: pgtype.Text{String: phoneNumber, Valid: true},
	}, nil
}

type fakeUserSessionRepository struct {
	sessions        []models.UserSession
	listErr         error
	revokedOtherFor string
	revoked         string
	revokeOtherErr  error
	revokeErr       error
}

func newFakeUserSessionRepository() *fakeUserSessionRepository {
	return &fakeUserSessionRepository{}
}

func (f *fakeUserSessionRepository) Create(context.Context, int64, string, string, string) error {
	return nil
}
func (f *fakeUserSessionRepository) ListActive(context.Context, int64, int) ([]models.UserSession, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.sessions, nil
}
func (f *fakeUserSessionRepository) IsActive(context.Context, int64, string) (bool, error) {
	return true, nil
}
func (f *fakeUserSessionRepository) Touch(context.Context, string) error { return nil }
func (f *fakeUserSessionRepository) RevokeOther(_ context.Context, _ int64, currentSessionID string) error {
	if f.revokeOtherErr != nil {
		return f.revokeOtherErr
	}
	f.revokedOtherFor = currentSessionID
	return nil
}
func (f *fakeUserSessionRepository) Revoke(_ context.Context, _ int64, sessionID string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revoked = sessionID
	return nil
}

// TestValidatePersonalName verifica regras de validateUserPersonalName (acentos, compostos, tamanho e rejeições).
func TestValidatePersonalName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{"simple", "Maria", true},
		{"accent", "José", true},
		{"compound", "Ana Paula", true},
		{"hyphen", "Almeida-Silva", true},
		{"apostrophe", "D'Ávila", true},
		{"empty", "", false},
		{"digits", "Maria2", false},
		{"symbol", "Jo@ão", false},
		{"too_long", strings.Repeat("a", validations.MaxPersonalFirstNameLen+1), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := validations.PersonalNameError("Nome", tc.value, validations.MaxPersonalFirstNameLen)
			assert.Equal(t, tc.ok, msg == "", "validatePersonalName(%q) msg=%q", tc.value, msg)
			if tc.name == "too_long" {
				assert.Contains(t, msg, "no máximo")
			}
		})
	}
}

// TestValidatePersonalPhone verifica regras de validateUserPersonalPhone (vazio, formatado e inválidos).
func TestValidatePersonalPhone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{"empty", "", true},
		{"formatted", "(11) 98765-4321", true},
		{"international", "+55 11 98765 4321", true},
		{"letters", "abc", false},
		{"dot", "11.98765", false},
		{"too_long", strings.Repeat("1", validations.MaxPersonalPhoneLen+1), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := validations.PersonalPhoneError(tc.value)
			assert.Equal(t, tc.ok, msg == "", "validatePersonalPhone(%q) msg=%q", tc.value, msg)
			if tc.name == "too_long" {
				assert.Contains(t, msg, "no máximo")
			}
		})
	}
}

// TestTwoFactorStatus verifica Status do 2FA no sucesso e propagação de erro do repositório.
func TestTwoFactorStatus(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	view, err := service.Status(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.True(t, view.Enabled)
	assert.Equal(t, models.TwoFactorMethodEmail, view.Method)

	repo.getStatusErr = errors.New("status boom")
	_, err = service.Status(context.Background(), 1)
	require.ErrorIs(t, err, repo.getStatusErr)
}

// TestStartSetupInvalidAndGuards verifica guardiões de StartSetup (status, conta, já ativo e método inválido).
func TestStartSetupInvalidAndGuards(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	repo.getStatusErr = errors.New("boom")
	_, err := service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, repo.getStatusErr)
	repo.getStatusErr = nil

	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "u@e.com", Active: false}
	_, err = service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorRequiresConfirmed)

	repo.status.Active = true
	repo.status.Enabled = true
	_, err = service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorAlreadyEnabled)

	repo.status.Enabled = false
	_, err = service.StartSetup(context.Background(), 1, "sms")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidMethod)
}

// TestStartSetupTOTP verifica StartSetup TOTP gera segredo manual e QR em data URL.
func TestStartSetupTOTP(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	result, err := service.StartSetup(context.Background(), 1, models.TwoFactorMethodTOTP)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, models.TwoFactorMethodTOTP, result.Method)
	assert.NotEmpty(t, result.QRCodeData)
	assert.NotEmpty(t, result.ManualSecret)
	assert.True(t, strings.HasPrefix(result.QRCodeData, "data:image/png;base64,"))
}

// TestStartSetupEmailErrors verifica erros de renderer, e-mail, challenge e create em StartSetup por e-mail.
func TestStartSetupEmailErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}

	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	_, err := service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorMailRenderer)

	// limpa o challenge criado antes de sendEmailCode falhar
	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	renderer := &fakeTwoFactorEmailRenderer{body: []byte("ok"), err: errors.New("render boom")}
	service = NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters", renderer)
	_, err = service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, renderer.err)

	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	createdAt := time.Now().Add(-2 * time.Minute)
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  "x",
		ExpiresAt: time.Now().Add(time.Minute),
		CreatedAt: &createdAt,
	})
	renderer = &fakeTwoFactorEmailRenderer{body: []byte("ok")}
	mail := failingMailService{err: errors.New("mail boom")}
	service = NewTwoFactorService(repo, mail, "secret-with-more-than-32-characters", renderer)
	// cooldown esgotado — createChallenge depois o envio falha
	_, err = service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, mail.err)

	repo.getChallengeErr = errors.New("challenge boom")
	_, err = service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, repo.getChallengeErr)
	repo.getChallengeErr = nil

	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	repo.createErr = errors.New("create boom")
	_, err = service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, repo.createErr)
}

// TestStartSetupCooldownNilCreatedAt verifica cooldown de StartSetup quando CreatedAt do challenge é nil.
func TestStartSetupCooldownNilCreatedAt(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	ch := &models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  "x",
		ExpiresAt: time.Now().Add(time.Minute),
	}
	repo.addChallenge(ch)
	// força CreatedAt nil após o default de addChallenge
	repo.mu.Lock()
	for id := range repo.challenges {
		repo.challenges[id].CreatedAt = nil
	}
	repo.mu.Unlock()

	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters", &fakeTwoFactorEmailRenderer{body: []byte("x")})
	_, err := service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorCooldown)
}

// TestVerifySetupWrapperAndTOTP verifica wrapper VerifySetup e ativação TOTP com códigos de recuperação.
func TestVerifySetupWrapperAndTOTP(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	secret := "JBSWY3DPEHPK3PXP"
	encrypted, err := service.encrypt(secret)
	require.NoError(t, err)
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:              1,
		Method:              models.TwoFactorMethodTOTP,
		Purpose:             models.TwoFactorPurposeEnable,
		TOTPSecretEncrypted: encrypted,
		ExpiresAt:           time.Now().Add(time.Minute),
	})
	code, err := totpCode(secret, time.Now().UTC())
	require.NoError(t, err)

	require.NoError(t, service.VerifySetup(context.Background(), 1, models.TwoFactorMethodTOTP, code))
	assert.True(t, repo.status.Enabled)
	assert.Equal(t, models.TwoFactorMethodTOTP, repo.status.Method)
}

// TestVerifySetupWithRecoveryCodesBranches verifica ramos de VerifySetupWithRecoveryCodes (erros, método, challenge e ativação).
func TestVerifySetupWithRecoveryCodesBranches(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	_, err := service.VerifySetupWithRecoveryCodes(context.Background(), 1, "sms", "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidMethod)

	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "abc")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)

	repo.getStatusErr = errors.New("status boom")
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, repo.getStatusErr)
	repo.getStatusErr = nil

	repo.status.Active = false
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorRequiresConfirmed)
	repo.status.Active = true
	repo.status.Enabled = true
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorAlreadyEnabled)
	repo.status.Enabled = false

	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorChallengeExpired)

	repo.getChallengeErr = errors.New("get boom")
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, repo.getChallengeErr)
	repo.getChallengeErr = nil

	challenge := repo.addChallenge(&models.TwoFactorChallenge{
		UserID:       1,
		Method:       models.TwoFactorMethodEmail,
		Purpose:      models.TwoFactorPurposeEnable,
		CodeHash:     service.hashVerificationCode("123456"),
		ExpiresAt:    time.Now().Add(time.Minute),
		AttemptCount: twoFactorMaxAttempts,
	})
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorTooManyAttempts)
	_ = challenge

	repo.challenges = make(map[int64]*models.TwoFactorChallenge)
	repo.nextID = 1
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "000000")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)

	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	repo.consumeErr = errors.New("consume boom")
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, repo.consumeErr)
	repo.consumeErr = nil

	recovery := newFakeRecoveryCodeRepository()
	recovery.replaceErr = errors.New("replace boom")
	service = NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters").WithRecoveryCodes(recovery)
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, recovery.replaceErr)

	recovery.replaceErr = nil
	service = NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters").WithRecoveryCodes(recovery)
	repo.activateErr = errors.New("activate boom")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.ErrorIs(t, err, repo.activateErr)
	assert.Empty(t, recovery.hashes[1])
}

// TestVerifySetupTOTPInvalidAndDecryptError verifica código TOTP inválido e falha de decrypt em VerifySetup TOTP.
func TestVerifySetupTOTPInvalidAndDecryptError(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:              1,
		Method:              models.TwoFactorMethodTOTP,
		Purpose:             models.TwoFactorPurposeEnable,
		TOTPSecretEncrypted: []byte("short"),
		ExpiresAt:           time.Now().Add(time.Minute),
	})
	_, err := service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodTOTP, "123456")
	require.Error(t, err)

	secret := "JBSWY3DPEHPK3PXP"
	encrypted, err := service.encrypt(secret)
	require.NoError(t, err)
	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:              1,
		Method:              models.TwoFactorMethodTOTP,
		Purpose:             models.TwoFactorPurposeEnable,
		TOTPSecretEncrypted: encrypted,
		ExpiresAt:           time.Now().Add(time.Minute),
	})
	_, err = service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodTOTP, "000000")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)
}

// TestGenerateRecoveryCodes verifica GenerateRecoveryCodes no sucesso e erros de status/repositório.
func TestGenerateRecoveryCodes(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	_, err := service.GenerateRecoveryCodes(context.Background(), 1)
	require.ErrorIs(t, err, apperrors.ErrRecoveryCodesUnavailable)

	recovery := newFakeRecoveryCodeRepository()
	service = service.WithRecoveryCodes(recovery)
	codes, err := service.GenerateRecoveryCodes(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, codes, recoveryCodeCount)

	repo.status.Enabled = false
	_, err = service.GenerateRecoveryCodes(context.Background(), 1)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorDisabled)
	repo.status.Enabled = true

	repo.getStatusErr = errors.New("status boom")
	_, err = service.GenerateRecoveryCodes(context.Background(), 1)
	require.ErrorIs(t, err, repo.getStatusErr)
	repo.getStatusErr = nil

	recovery.replaceErr = errors.New("replace boom")
	_, err = service.GenerateRecoveryCodes(context.Background(), 1)
	require.ErrorIs(t, err, recovery.replaceErr)
}

// TestStartDisable verifica StartDisable por e-mail e TOTP no fluxo feliz.
func TestStartDisable(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	renderer := &fakeTwoFactorEmailRenderer{body: []byte("mail")}
	mail := &capturingMailService{}
	service := NewTwoFactorService(repo, mail, "secret-with-more-than-32-characters", renderer)

	repo.getStatusErr = errors.New("status boom")
	_, err := service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, repo.getStatusErr)
	repo.getStatusErr = nil

	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true, Enabled: false}
	_, err = service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorDisabled)

	repo.status = enabledEmailStatus()
	result, err := service.StartDisable(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, models.TwoFactorMethodEmail, result.Method)
	assert.Equal(t, 1, mail.sent)

	repo.status = &models.TwoFactorStatus{
		UserID:  1,
		Email:   "user@example.com",
		Active:  true,
		Enabled: true,
		Method:  models.TwoFactorMethodTOTP,
	}
	result, err = service.StartDisable(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, models.TwoFactorMethodTOTP, result.Method)

	repo.status.Method = "sms"
	_, err = service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidMethod)
}

// TestStartDisableCooldownAndErrors verifica cooldown e erros de status/método em StartDisable.
func TestStartDisableCooldownAndErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	createdAt := time.Now()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeDisable,
		CodeHash:  "x",
		ExpiresAt: time.Now().Add(time.Minute),
		CreatedAt: &createdAt,
	})
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters", &fakeTwoFactorEmailRenderer{body: []byte("x")})
	_, err := service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorCooldown)

	repo.status = &models.TwoFactorStatus{
		UserID: 1, Email: "u@e.com", Active: true, Enabled: true, Method: models.TwoFactorMethodTOTP,
	}
	createdAt = time.Now()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodTOTP,
		Purpose:   models.TwoFactorPurposeDisable,
		ExpiresAt: time.Now().Add(time.Minute),
		CreatedAt: &createdAt,
	})
	_, err = service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorCooldown)
}

// TestVerifyDisableMoreBranches verifica ramos adicionais de VerifyDisable (status, challenge e TOTP).
func TestVerifyDisableMoreBranches(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	repo.getStatusErr = errors.New("status boom")
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), repo.getStatusErr)
	repo.getStatusErr = nil

	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "u@e.com", Active: true, Enabled: false}
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), apperrors.ErrTwoFactorDisabled)

	repo.status = enabledEmailStatus()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:       1,
		Method:       models.TwoFactorMethodEmail,
		Purpose:      models.TwoFactorPurposeDisable,
		CodeHash:     service.hashVerificationCode("123456"),
		ExpiresAt:    time.Now().Add(time.Minute),
		AttemptCount: twoFactorMaxAttempts,
	})
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), apperrors.ErrTwoFactorTooManyAttempts)

	repo.status.Method = "sms"
	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    "sms",
		Purpose:   models.TwoFactorPurposeDisable,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), apperrors.ErrTwoFactorInvalidMethod)

	repo.status = &models.TwoFactorStatus{
		UserID:              1,
		Email:               "u@e.com",
		Active:              true,
		Enabled:             true,
		Method:              models.TwoFactorMethodTOTP,
		TOTPSecretEncrypted: []byte("bad"),
	}
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodTOTP,
		Purpose:   models.TwoFactorPurposeDisable,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.Error(t, service.VerifyDisable(context.Background(), 1, "123456"))

	secret := "JBSWY3DPEHPK3PXP"
	enc, err := service.encrypt(secret)
	require.NoError(t, err)
	repo.status.TOTPSecretEncrypted = enc
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodTOTP,
		Purpose:   models.TwoFactorPurposeDisable,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "000000"), apperrors.ErrTwoFactorInvalidCode)
}

// TestBeginLogin verifica BeginLogin para e-mail e TOTP no fluxo feliz e conta sem 2FA.
func TestBeginLogin(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	renderer := &fakeTwoFactorEmailRenderer{body: []byte("x")}
	mail := &capturingMailService{}
	service := NewTwoFactorService(repo, mail, "secret-with-more-than-32-characters", renderer)

	repo.getStatusErr = errors.New("status boom")
	_, err := service.BeginLogin(context.Background(), 1)
	require.ErrorIs(t, err, repo.getStatusErr)
	repo.getStatusErr = nil

	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "u@e.com", Active: true, Enabled: false}
	view, err := service.BeginLogin(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, view.Enabled)

	repo.status = enabledEmailStatus()
	view, err = service.BeginLogin(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, view.Enabled)
	assert.Equal(t, 1, mail.sent)

	createdAt := time.Now()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeLogin,
		CodeHash:  "x",
		ExpiresAt: time.Now().Add(time.Minute),
		CreatedAt: &createdAt,
	})
	view, err = service.BeginLogin(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, view.Enabled)

	repo.status = &models.TwoFactorStatus{
		UserID: 1, Email: "u@e.com", Active: true, Enabled: true, Method: models.TwoFactorMethodTOTP,
	}
	view, err = service.BeginLogin(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, models.TwoFactorMethodTOTP, view.Method)

	repo.createErr = errors.New("create boom")
	_, err = service.BeginLogin(context.Background(), 1)
	require.ErrorIs(t, err, repo.createErr)
}

// TestBeginLoginEmailCreateAndSendErrors verifica erros de create challenge e envio de e-mail em BeginLogin.
func TestBeginLoginEmailCreateAndSendErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	repo.getChallengeErr = errors.New("cooldown lookup boom")
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters", &fakeTwoFactorEmailRenderer{body: []byte("x")})
	_, err := service.BeginLogin(context.Background(), 1)
	require.ErrorIs(t, err, repo.getChallengeErr)
	repo.getChallengeErr = nil

	repo.createErr = errors.New("create boom")
	_, err = service.BeginLogin(context.Background(), 1)
	require.ErrorIs(t, err, repo.createErr)
	repo.createErr = nil

	mail := failingMailService{err: errors.New("mail boom")}
	service = NewTwoFactorService(repo, mail, "secret-with-more-than-32-characters", &fakeTwoFactorEmailRenderer{body: []byte("x")})
	_, err = service.BeginLogin(context.Background(), 1)
	require.ErrorIs(t, err, mail.err)
}

// TestVerifyLoginBranches verifica ramos de VerifyLogin (código e-mail, expirado, tentativas e sucesso).
func TestVerifyLoginBranches(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	_, err := service.VerifyLogin(context.Background(), 1, " ")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)

	repo.getStatusErr = errors.New("status boom")
	_, err = service.VerifyLogin(context.Background(), 1, "123456")
	require.ErrorIs(t, err, repo.getStatusErr)
	repo.getStatusErr = nil

	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "u@e.com", Active: true, Enabled: false}
	view, err := service.VerifyLogin(context.Background(), 1, "123456")
	require.NoError(t, err)
	assert.False(t, view.Enabled)

	repo.status = enabledEmailStatus()
	_, err = service.VerifyLogin(context.Background(), 1, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorChallengeExpired)

	repo.status.Method = "sms"
	_, err = service.VerifyLogin(context.Background(), 1, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidMethod)

	repo.status = enabledEmailStatus()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:       1,
		Method:       models.TwoFactorMethodEmail,
		Purpose:      models.TwoFactorPurposeLogin,
		CodeHash:     service.hashVerificationCode("123456"),
		ExpiresAt:    time.Now().Add(time.Minute),
		AttemptCount: twoFactorMaxAttempts,
	})
	_, err = service.VerifyLogin(context.Background(), 1, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorTooManyAttempts)

	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeLogin,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifyLogin(context.Background(), 1, "!!!!!!")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)

	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeLogin,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifyLogin(context.Background(), 1, "000000")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)

	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeLogin,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	view, err = service.VerifyLogin(context.Background(), 1, "123456")
	require.NoError(t, err)
	assert.True(t, view.Enabled)
}

// TestVerifyLoginTOTPAndRecoveryErrors verifica erros de TOTP/recuperação e decrypt em VerifyLogin.
func TestVerifyLoginTOTPAndRecoveryErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	secret := "JBSWY3DPEHPK3PXP"
	enc, err := service.encrypt(secret)
	require.NoError(t, err)
	repo.status = &models.TwoFactorStatus{
		UserID: 1, Email: "u@e.com", Active: true, Enabled: true,
		Method: models.TwoFactorMethodTOTP, TOTPSecretEncrypted: enc,
	}
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodTOTP, Purpose: models.TwoFactorPurposeLogin,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	code, err := totpCode(secret, time.Now().UTC())
	require.NoError(t, err)
	_, err = service.VerifyLogin(context.Background(), 1, code)
	require.NoError(t, err)

	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodTOTP, Purpose: models.TwoFactorPurposeLogin,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifyLogin(context.Background(), 1, "000000")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)

	repo.status.TOTPSecretEncrypted = []byte("x")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodTOTP, Purpose: models.TwoFactorPurposeLogin,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifyLogin(context.Background(), 1, "123456")
	require.Error(t, err)

	repo.status = enabledEmailStatus()
	recovery := newFakeRecoveryCodeRepository()
	service = NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters").WithRecoveryCodes(recovery)
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeLogin,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	recovery.useErr = errors.New("use boom")
	_, err = service.VerifyLogin(context.Background(), 1, "ABCD-EF12-3456-7890")
	require.ErrorIs(t, err, recovery.useErr)

	// recovery repo nil → código inválido para entrada no formato de recovery
	service = NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeLogin,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifyLogin(context.Background(), 1, "ABCD-EF12-3456-7890")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)
}

// TestConsumeChallengeNonNotFoundError verifica que consumeChallenge propaga erro diferente de NotFound.
func TestConsumeChallengeNonNotFoundError(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeDisable,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	repo.consumeErr = errors.New("db boom")
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), repo.consumeErr)
}

// TestDecryptAndHelpers verifica decrypt e helpers auxiliares (normalize, hashMatches, etc.).
func TestDecryptAndHelpers(t *testing.T) {
	t.Parallel()

	service := NewTwoFactorService(nil, nil, "secret-with-more-than-32-characters")
	enc, err := service.encrypt("hello")
	require.NoError(t, err)
	plain, err := service.decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "hello", plain)

	_, err = service.decrypt([]byte("tiny"))
	require.Error(t, err)

	assert.Equal(t, "", normalizeVerificationCode("12ab56"))
	assert.Equal(t, "", normalizeRecoveryCode(""))
	assert.Equal(t, "", normalizeRecoveryCode("ZZZZ-ZZZZ-ZZZZ-ZZZZ"))
	assert.Equal(t, "", normalizeRecoveryCode("ABCD"))
	assert.Equal(t, "", formatRecoveryCode("bad"))
	assert.Equal(t, "", service.hashRecoveryCode("bad"))

	assert.False(t, validateTOTP("!!!", "123456", time.Now().UTC()))

	url := otpauthURL("user@example.com", "SECRET")
	assert.Contains(t, url, "otpauth://totp/")
	assert.Contains(t, url, "secret=SECRET")

	code, err := generateNumericCode()
	require.NoError(t, err)
	assert.Len(t, code, 6)

	rec, err := generateRecoveryCode()
	require.NoError(t, err)
	assert.Len(t, rec, recoveryCodeNormalizedLen)

	secret, err := generateTOTPSecret()
	require.NoError(t, err)
	assert.NotEmpty(t, secret)
}

// TestActivateWithoutRecoveryCodes verifica ativação 2FA sem repositório de códigos de recuperação.
func TestActivateWithoutRecoveryCodes(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "u@e.com", Active: true}
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeEnable,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	codes, err := service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.NoError(t, err)
	assert.Nil(t, codes)
	assert.True(t, repo.status.Enabled)
}

// TestGetActiveLoginChallengeRepoError verifica propagação de erro do repositório em GetActiveLoginChallenge.
func TestGetActiveLoginChallengeRepoError(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	repo.getChallengeErr = errors.New("db boom")
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	_, err := service.VerifyLogin(context.Background(), 1, "123456")
	require.ErrorIs(t, err, repo.getChallengeErr)
}

// TestDisableWithoutVerificationErrors verifica erros de DisableWithoutVerification quando o repositório falha.
func TestDisableWithoutVerificationErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeDisable,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	repo.disableErr = errors.New("disable boom")
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), repo.disableErr)
}

// TestConsumeChallengeMapsNotFound verifica que consumeChallenge mapeia ErrNotFound para challenge expirado.
func TestConsumeChallengeMapsNotFound(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeDisable,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	repo.consumeErr = apperrors.ErrNotFound
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), apperrors.ErrTwoFactorChallengeExpired)
}

// TestDisableWithoutVerificationStatusError verifica DisableWithoutVerification quando GetStatus falha.
func TestDisableWithoutVerificationStatusError(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeDisable,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	// primeiro GetStatus em VerifyDisable sucede; o segundo em disableWithoutVerification falha
	repo.failStatusAfterN = 1
	require.ErrorContains(t, service.VerifyDisable(context.Background(), 1, "123456"), "status boom after n")
}

// TestDisableWithoutVerificationAlreadyDisabled verifica DisableWithoutVerification quando o 2FA já está desativado.
func TestDisableWithoutVerificationAlreadyDisabled(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	repo.disableOnSecondStatus = true
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeDisable,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	require.ErrorIs(t, service.VerifyDisable(context.Background(), 1, "123456"), apperrors.ErrTwoFactorDisabled)
}

// TestStartSetupTOTPErrors verifica erros de create challenge e status em StartSetup TOTP.
func TestStartSetupTOTPErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	createdAt := time.Now()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodTOTP, Purpose: models.TwoFactorPurposeEnable,
		ExpiresAt: time.Now().Add(time.Minute), CreatedAt: &createdAt,
	})
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	_, err := service.StartSetup(context.Background(), 1, models.TwoFactorMethodTOTP)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorCooldown)

	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	repo.createErr = errors.New("create boom")
	_, err = service.StartSetup(context.Background(), 1, models.TwoFactorMethodTOTP)
	require.ErrorIs(t, err, repo.createErr)
}

// TestStartDisableCreateAndSendErrors verifica erros de create challenge e envio em StartDisable por e-mail.
func TestStartDisableCreateAndSendErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	repo.createErr = errors.New("create boom")
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters", &fakeTwoFactorEmailRenderer{body: []byte("x")})
	_, err := service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, repo.createErr)
	repo.createErr = nil

	mail := failingMailService{err: errors.New("mail boom")}
	service = NewTwoFactorService(repo, mail, "secret-with-more-than-32-characters", &fakeTwoFactorEmailRenderer{body: []byte("x")})
	// a criação anterior pode ter deixado challenge se createErr foi definido antes do create — limpar
	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	_, err = service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, mail.err)

	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	repo.status = &models.TwoFactorStatus{UserID: 1, Email: "u@e.com", Active: true, Enabled: true, Method: models.TwoFactorMethodTOTP}
	repo.createErr = errors.New("totp create boom")
	service = NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	_, err = service.StartDisable(context.Background(), 1)
	require.ErrorIs(t, err, repo.createErr)
}

// TestVerifyLoginConsumeErrors verifica erros ao consumir challenge em VerifyLogin.
func TestVerifyLoginConsumeErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeLogin,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	repo.consumeErr = errors.New("consume boom")
	_, err := service.VerifyLogin(context.Background(), 1, "123456")
	require.ErrorIs(t, err, repo.consumeErr)

	secret := "JBSWY3DPEHPK3PXP"
	enc, err := service.encrypt(secret)
	require.NoError(t, err)
	repo.consumeErr = nil
	repo.status = &models.TwoFactorStatus{
		UserID: 1, Email: "u@e.com", Active: true, Enabled: true,
		Method: models.TwoFactorMethodTOTP, TOTPSecretEncrypted: enc,
	}
	repo.challenges = map[int64]*models.TwoFactorChallenge{}
	repo.nextID = 1
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodTOTP, Purpose: models.TwoFactorPurposeLogin,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	code, err := totpCode(secret, time.Now().UTC())
	require.NoError(t, err)
	repo.consumeErr = apperrors.ErrNotFound
	_, err = service.VerifyLogin(context.Background(), 1, code)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorChallengeExpired)
}

// TestVerifyLoginRecoveryConsumeError verifica erro ao consumir código de recuperação em VerifyLogin.
func TestVerifyLoginRecoveryConsumeError(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	recovery := newFakeRecoveryCodeRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters").WithRecoveryCodes(recovery)
	code := "ABCD-EF12-3456-7890"
	recovery.hashes[1] = map[string]bool{service.hashRecoveryCode(code): true}
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID: 1, Method: models.TwoFactorMethodEmail, Purpose: models.TwoFactorPurposeLogin,
		CodeHash: service.hashVerificationCode("123456"), ExpiresAt: time.Now().Add(time.Minute),
	})
	repo.consumeErr = errors.New("consume boom")
	_, err := service.VerifyLogin(context.Background(), 1, code)
	require.ErrorIs(t, err, repo.consumeErr)
}

// TestTOTPValidationAcceptsCurrentCodeAndRejectsWrongCode verifica que validateTOTP aceita o código atual e rejeita código errado.
func TestTOTPValidationAcceptsCurrentCodeAndRejectsWrongCode(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1234567890, 0).UTC()

	code, err := totpCode(secret, now)
	require.NoError(t, err)
	assert.True(t, validateTOTP(secret, code, now), "deve aceitar código TOTP válido")

	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "000001"
	}
	assert.False(t, validateTOTP(secret, wrongCode, now), "deve rejeitar código TOTP inválido")
}

// TestTwoFactorCodeHashUsesMFASecret verifica que o hash do código de verificação depende do segredo MFA.
func TestTwoFactorCodeHashUsesMFASecret(t *testing.T) {
	t.Parallel()

	serviceA := NewTwoFactorService(nil, nil, "secret-a-with-more-than-32-characters")
	serviceB := NewTwoFactorService(nil, nil, "secret-b-with-more-than-32-characters")

	hashA := serviceA.hashVerificationCode("123456")
	require.NotEmpty(t, hashA)
	require.NotEqual(t, "123456", hashA)
	assert.True(t, serviceA.hashMatches(hashA, "123456"))
	assert.False(t, serviceA.hashMatches(hashA, "654321"))
	assert.NotEqual(t, hashA, serviceB.hashVerificationCode("123456"), "hash deve variar com a chave MFA")
}

// TestVerifyDisableRequiresCode verifica que VerifyDisable exige código e não desativa sem ele.
func TestVerifyDisableRequiresCode(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")

	err := service.VerifyDisable(context.Background(), 1, "")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)
	assert.False(t, repo.disabled, "não deve desativar 2FA sem código")
}

// TestStartSetupEmailUsesInjectedRenderer verifica que StartSetup por e-mail usa o renderer injetado e envia a mensagem.
func TestStartSetupEmailUsesInjectedRenderer(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{
		UserID: 1,
		Email:  "user@example.com",
		Active: true,
	}
	renderer := &fakeTwoFactorEmailRenderer{body: []byte("rendered-two-factor-mail")}
	mail := &capturingMailService{}
	service := NewTwoFactorService(repo, mail, "secret-with-more-than-32-characters", renderer)

	result, err := service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, models.TwoFactorMethodEmail, result.Method)
	require.Len(t, renderer.codes, 1)
	assert.Len(t, renderer.codes[0], 6)
	assert.Equal(t, 1, mail.sent)
	assert.Equal(t, "rendered-two-factor-mail", string(mail.body))
}

// TestStartSetupEmailCooldown verifica que StartSetup por e-mail respeita cooldown de challenge ativo.
func TestStartSetupEmailCooldown(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{
		UserID: 1,
		Email:  "user@example.com",
		Active: true,
	}
	createdAt := time.Now()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  "hash",
		ExpiresAt: time.Now().Add(time.Minute),
		CreatedAt: &createdAt,
	})
	renderer := &fakeTwoFactorEmailRenderer{body: []byte("rendered")}
	mail := &capturingMailService{}
	service := NewTwoFactorService(repo, mail, "secret-with-more-than-32-characters", renderer)

	_, err := service.StartSetup(context.Background(), 1, models.TwoFactorMethodEmail)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorCooldown)
	assert.Empty(t, renderer.codes)
	assert.Zero(t, mail.sent)
}

// TestVerifySetupEmailGeneratesRecoveryCodes verifica que VerifySetupWithRecoveryCodes ativa 2FA e gera códigos hasheados.
func TestVerifySetupEmailGeneratesRecoveryCodes(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = &models.TwoFactorStatus{
		UserID: 1,
		Email:  "user@example.com",
		Active: true,
	}
	recoveryCodes := newFakeRecoveryCodeRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters").
		WithRecoveryCodes(recoveryCodes)
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeEnable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})

	codes, err := service.VerifySetupWithRecoveryCodes(context.Background(), 1, models.TwoFactorMethodEmail, "123456")
	require.NoError(t, err)
	require.Len(t, codes, recoveryCodeCount)
	require.Len(t, recoveryCodes.hashes[1], recoveryCodeCount)

	for _, code := range codes {
		assert.NotEmpty(t, normalizeRecoveryCode(code), "código de recuperação mal formatado: %q", code)
		_, storedAsPlainText := recoveryCodes.hashes[1][code]
		assert.False(t, storedAsPlainText, "código não deve ser armazenado em texto puro: %q", code)
	}

	require.NotNil(t, repo.status)
	assert.True(t, repo.status.Enabled)
	assert.Equal(t, models.TwoFactorMethodEmail, repo.status.Method)
}

// TestVerifyLoginAcceptsRecoveryCodeOnce verifica que VerifyLogin aceita código de recuperação uma única vez.
func TestVerifyLoginAcceptsRecoveryCodeOnce(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	recoveryCodes := newFakeRecoveryCodeRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters").
		WithRecoveryCodes(recoveryCodes)

	code := "ABCD-EF12-3456-7890"
	recoveryCodes.hashes[1] = map[string]bool{
		service.hashRecoveryCode(code): true,
	}
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeLogin,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})

	_, err := service.VerifyLogin(context.Background(), 1, strings.ToLower(code))
	require.NoError(t, err)

	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeLogin,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	_, err = service.VerifyLogin(context.Background(), 1, code)
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)
}

// TestVerifyDisableEmailCode verifica que VerifyDisable com código de e-mail válido desativa o 2FA.
func TestVerifyDisableEmailCode(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeDisable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})

	require.NoError(t, service.VerifyDisable(context.Background(), 1, "123456"))
	assert.True(t, repo.disabled)
}

// TestVerifyDisableInvalidEmailCode verifica que código de e-mail inválido incrementa tentativas e não desativa.
func TestVerifyDisableInvalidEmailCode(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	challenge := repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeDisable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})

	err := service.VerifyDisable(context.Background(), 1, "000000")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorInvalidCode)
	assert.False(t, repo.disabled)
	assert.Equal(t, 1, repo.challenges[challenge.ID].AttemptCount)
}

// TestVerifyDisableExpiredChallenge verifica que challenge expirado em VerifyDisable retorna apperrors.ErrTwoFactorChallengeExpired.
func TestVerifyDisableExpiredChallenge(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeDisable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(-time.Minute),
	})

	err := service.VerifyDisable(context.Background(), 1, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorChallengeExpired)
	assert.False(t, repo.disabled)
}

// TestVerifyDisableConsumedChallenge verifica que challenge já consumido em VerifyDisable é tratado como expirado.
func TestVerifyDisableConsumedChallenge(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	consumedAt := time.Now()
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:     1,
		Method:     models.TwoFactorMethodEmail,
		Purpose:    models.TwoFactorPurposeDisable,
		CodeHash:   service.hashVerificationCode("123456"),
		ExpiresAt:  time.Now().Add(time.Minute),
		ConsumedAt: &consumedAt,
	})

	err := service.VerifyDisable(context.Background(), 1, "123456")
	require.ErrorIs(t, err, apperrors.ErrTwoFactorChallengeExpired)
	assert.False(t, repo.disabled)
}

// TestVerifyDisableTOTPCode verifica que VerifyDisable aceita código TOTP válido e desativa o 2FA.
func TestVerifyDisableTOTPCode(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	secret := "JBSWY3DPEHPK3PXP"
	encryptedSecret, err := service.encrypt(secret)
	require.NoError(t, err)

	repo.status = &models.TwoFactorStatus{
		UserID:              1,
		Email:               "user@example.com",
		Active:              true,
		Enabled:             true,
		Method:              models.TwoFactorMethodTOTP,
		TOTPSecretEncrypted: encryptedSecret,
	}
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodTOTP,
		Purpose:   models.TwoFactorPurposeDisable,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	code, err := totpCode(secret, time.Now().UTC())
	require.NoError(t, err)

	require.NoError(t, service.VerifyDisable(context.Background(), 1, code))
	assert.True(t, repo.disabled)
}

// TestVerifyDisableConcurrentReuse verifica que reuso concorrente do challenge de disable só permite uma desativação.
func TestVerifyDisableConcurrentReuse(t *testing.T) {
	t.Parallel()

	repo := newFakeTwoFactorRepository()
	repo.status = enabledEmailStatus()
	service := NewTwoFactorService(repo, fakeMailService{}, "secret-with-more-than-32-characters")
	repo.addChallenge(&models.TwoFactorChallenge{
		UserID:    1,
		Method:    models.TwoFactorMethodEmail,
		Purpose:   models.TwoFactorPurposeDisable,
		CodeHash:  service.hashVerificationCode("123456"),
		ExpiresAt: time.Now().Add(time.Minute),
	})

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- service.VerifyDisable(context.Background(), 1, "123456")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	failures := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, apperrors.ErrTwoFactorChallengeExpired) || errors.Is(err, apperrors.ErrTwoFactorDisabled) {
			failures++
			continue
		}
		require.Failf(t, "erro inesperado em tentativa concorrente", "%v", err)
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)
}

type fakeMailService struct{}

func (fakeMailService) Send(mailer.MailMessage) error {
	return nil
}

type capturingMailService struct {
	sent int
	body []byte
}

func (s *capturingMailService) Send(message mailer.MailMessage) error {
	s.sent++
	s.body = append([]byte(nil), message.Body...)
	return nil
}

type fakeTwoFactorEmailRenderer struct {
	body  []byte
	codes []string
	err   error
}

func (r *fakeTwoFactorEmailRenderer) RenderTwoFactorCode(code string) ([]byte, error) {
	r.codes = append(r.codes, code)
	if r.err != nil {
		return nil, r.err
	}
	return append([]byte(nil), r.body...), nil
}

type failingMailService struct {
	err error
}

func (s failingMailService) Send(mailer.MailMessage) error {
	return s.err
}

type fakeTwoFactorRepository struct {
	mu                    sync.Mutex
	status                *models.TwoFactorStatus
	challenges            map[int64]*models.TwoFactorChallenge
	nextID                int64
	disabled              bool
	getStatusErr          error
	getStatusCalls        int
	failStatusAfterN      int
	disableOnSecondStatus bool
	createErr             error
	getChallengeErr       error
	incrementErr          error
	consumeErr            error
	activateErr           error
	disableErr            error
}

type fakeRecoveryCodeRepository struct {
	mu         sync.Mutex
	hashes     map[int64]map[string]bool
	replaceErr error
	useErr     error
	deleteErr  error
}

func newFakeTwoFactorRepository() *fakeTwoFactorRepository {
	return &fakeTwoFactorRepository{
		challenges: make(map[int64]*models.TwoFactorChallenge),
		nextID:     1,
	}
}

func newFakeRecoveryCodeRepository() *fakeRecoveryCodeRepository {
	return &fakeRecoveryCodeRepository{hashes: make(map[int64]map[string]bool)}
}

func enabledEmailStatus() *models.TwoFactorStatus {
	return &models.TwoFactorStatus{
		UserID:  1,
		Email:   "user@example.com",
		Active:  true,
		Enabled: true,
		Method:  models.TwoFactorMethodEmail,
	}
}

func (r *fakeTwoFactorRepository) addChallenge(challenge *models.TwoFactorChallenge) *models.TwoFactorChallenge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if challenge.CreatedAt == nil {
		now := time.Now()
		challenge.CreatedAt = &now
	}
	challenge.ID = r.nextID
	r.nextID++
	stored := cloneChallenge(challenge)
	r.challenges[stored.ID] = stored
	return cloneChallenge(stored)
}

func (r *fakeTwoFactorRepository) GetStatus(context.Context, int64) (*models.TwoFactorStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getStatusCalls++
	if r.getStatusErr != nil {
		return nil, r.getStatusErr
	}
	if r.failStatusAfterN > 0 && r.getStatusCalls > r.failStatusAfterN {
		return nil, errors.New("status boom after n")
	}
	if r.status == nil {
		return nil, apperrors.ErrNotFound
	}
	status := *r.status
	status.TOTPSecretEncrypted = append([]byte(nil), r.status.TOTPSecretEncrypted...)
	if r.disableOnSecondStatus && r.getStatusCalls > 1 {
		status.Enabled = false
		status.Method = ""
	}
	return &status, nil
}

func (r *fakeTwoFactorRepository) CreateChallenge(_ context.Context, userID int64, method, purpose, codeHash string, encryptedSecret []byte, ttl time.Duration) (*models.TwoFactorChallenge, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	return r.addChallenge(&models.TwoFactorChallenge{
		UserID:              userID,
		Method:              method,
		Purpose:             purpose,
		CodeHash:            codeHash,
		TOTPSecretEncrypted: append([]byte(nil), encryptedSecret...),
		ExpiresAt:           time.Now().Add(ttl),
	}), nil
}

func (r *fakeTwoFactorRepository) GetActiveChallenge(_ context.Context, userID int64, method, purpose string) (*models.TwoFactorChallenge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getChallengeErr != nil {
		return nil, r.getChallengeErr
	}
	for _, challenge := range r.challenges {
		if challenge.UserID == userID &&
			challenge.Method == method &&
			challenge.Purpose == purpose &&
			challenge.ConsumedAt == nil &&
			challenge.ExpiresAt.After(time.Now()) {
			return cloneChallenge(challenge), nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (r *fakeTwoFactorRepository) IncrementActiveChallengeAttempts(_ context.Context, challengeID int64, maxAttempts int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.incrementErr != nil {
		return r.incrementErr
	}
	challenge, ok := r.challenges[challengeID]
	if !ok || challenge.ConsumedAt != nil || !challenge.ExpiresAt.After(time.Now()) || challenge.AttemptCount >= maxAttempts {
		return apperrors.ErrNotFound
	}
	challenge.AttemptCount++
	return nil
}

func (r *fakeTwoFactorRepository) ConsumeActiveChallenge(_ context.Context, challengeID, userID int64, method, purpose string, maxAttempts int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.consumeErr != nil {
		return r.consumeErr
	}
	challenge, ok := r.challenges[challengeID]
	if !ok ||
		challenge.UserID != userID ||
		challenge.Method != method ||
		challenge.Purpose != purpose ||
		challenge.ConsumedAt != nil ||
		!challenge.ExpiresAt.After(time.Now()) ||
		challenge.AttemptCount >= maxAttempts {
		return apperrors.ErrNotFound
	}
	now := time.Now()
	challenge.ConsumedAt = &now
	return nil
}

func (r *fakeTwoFactorRepository) ActivateEmail(context.Context, int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activateErr != nil {
		return r.activateErr
	}
	if r.status != nil {
		r.status.Enabled = true
		r.status.Method = models.TwoFactorMethodEmail
	}
	return nil
}

func (r *fakeTwoFactorRepository) ActivateTOTP(context.Context, int64, []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activateErr != nil {
		return r.activateErr
	}
	if r.status != nil {
		r.status.Enabled = true
		r.status.Method = models.TwoFactorMethodTOTP
	}
	return nil
}

func (r *fakeTwoFactorRepository) Disable(context.Context, int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disableErr != nil {
		return r.disableErr
	}
	r.disabled = true
	if r.status != nil {
		r.status.Enabled = false
		r.status.Method = ""
		r.status.TOTPSecretEncrypted = nil
	}
	return nil
}

func (r *fakeTwoFactorRepository) DeleteExpiredChallenges(context.Context) error {
	return nil
}

func (r *fakeRecoveryCodeRepository) Replace(_ context.Context, userID int64, codeHashes []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replaceErr != nil {
		return r.replaceErr
	}
	r.hashes[userID] = make(map[string]bool, len(codeHashes))
	for _, codeHash := range codeHashes {
		r.hashes[userID][codeHash] = true
	}
	return nil
}

func (r *fakeRecoveryCodeRepository) Use(_ context.Context, userID int64, codeHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.useErr != nil {
		return r.useErr
	}
	hashes := r.hashes[userID]
	if hashes == nil || !hashes[codeHash] {
		return apperrors.ErrNotFound
	}
	delete(hashes, codeHash)
	return nil
}

func (r *fakeRecoveryCodeRepository) Delete(_ context.Context, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.hashes, userID)
	return nil
}

func cloneChallenge(challenge *models.TwoFactorChallenge) *models.TwoFactorChallenge {
	if challenge == nil {
		return nil
	}
	cloned := *challenge
	cloned.TOTPSecretEncrypted = append([]byte(nil), challenge.TOTPSecretEncrypted...)
	return &cloned
}
