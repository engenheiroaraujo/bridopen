package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	userrepo "github.com/engenheiroaraujo/bridopen/internal/handlers/users/repositories"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/key"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	platformpassword "github.com/engenheiroaraujo/bridopen/internal/platform/password"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
	"github.com/skip2/go-qrcode"
)

// -----------------------------------------------------------------------------
// Contratos de repositório
//
// As interfaces abaixo são declaradas aqui, no consumidor. O pacote de
// repositório não as conhece: apenas satisfaz os métodos. Trocar a fonte de
// dados é trocar o que se injeta, sem tocar neste arquivo.
// -----------------------------------------------------------------------------

// UserRepository cobre conta, autenticação, tokens e e-mail de recuperação.
type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (bool, error)
	FindByUser(ctx context.Context, email string) (*models.User, *models.UserPersonalInformation, error)
	FindPasswordAuthData(ctx context.Context, userID int64) (email, passwordHash string, active bool, err error)

	Create(ctx context.Context, firstName, lastName, email, password, rawToken string) (*models.User, *models.UserPersonalInformation, string, error)

	CreateResetPasswordToken(ctx context.Context, email, rawToken string) (string, []string, error)
	CreateEmailConfirmationToken(ctx context.Context, email, rawToken string) (string, []string, error)
	GetUserConfirmationByToken(ctx context.Context, token, purpose string) (*models.User, *models.UserConfirmationToken, string, error)
	PutPasswordByToken(ctx context.Context, newPassword, token string) (string, int64, error)
	ConfirmUserByToken(ctx context.Context, token string) (email string, userID int64, err error)
	DeleteExpiredTokens(ctx context.Context) error
	UpdatePasswordByID(ctx context.Context, userID int64, newPassword string) error

	ExportUserData(ctx context.Context, userID int64) (*models.AccountExport, error)
	DeleteUserAndData(ctx context.Context, userID int64) error
	ListAttachmentStorageKeys(ctx context.Context, userID int64) ([]string, error)
	FindAccountBasics(ctx context.Context, userID int64) (*models.User, error)

	EmailAddressInUse(ctx context.Context, userID int64, email string) (bool, error)
	BeginRecoveryEmailChange(ctx context.Context, userID int64, pendingEmail, rawToken string) (primaryEmail string, token string, err error)
	CancelRecoveryEmailChange(ctx context.Context, userID int64, pendingEmail string) error
	ConfirmRecoveryEmailByToken(ctx context.Context, token string) (primaryEmail string, recoveryEmail string, userID int64, err error)
}

// UserPersonalRepository cobre os dados pessoais do utilizador.
type UserPersonalRepository interface {
	FindByUserID(ctx context.Context, userID int64) (*models.UserPersonalInformation, error)
	Create(ctx context.Context, userID int64, firstName, lastName, phoneNumber string) (*models.UserPersonalInformation, error)
	Update(ctx context.Context, userID int64, id int, firstName, lastName, phoneNumber string) (*models.UserPersonalInformation, error)
}

// UserSessionRepository cobre as sessões ativas do utilizador.
type UserSessionRepository interface {
	Create(ctx context.Context, userID int64, sessionID, userAgent, ipAddress string) error
	ListActive(ctx context.Context, userID int64, limit int) ([]models.UserSession, error)
	IsActive(ctx context.Context, userID int64, sessionID string) (bool, error)
	Touch(ctx context.Context, sessionID string) error
	RevokeOther(ctx context.Context, userID int64, currentSessionID string) error
	Revoke(ctx context.Context, userID int64, sessionID string) error
}

// TwoFactorRepository cobre o estado e os desafios de verificação em duas etapas.
type TwoFactorRepository interface {
	GetStatus(ctx context.Context, userID int64) (*models.TwoFactorStatus, error)
	CreateChallenge(ctx context.Context, userID int64, method, purpose, codeHash string, encryptedSecret []byte, ttl time.Duration) (*models.TwoFactorChallenge, error)
	GetActiveChallenge(ctx context.Context, userID int64, method, purpose string) (*models.TwoFactorChallenge, error)
	IncrementActiveChallengeAttempts(ctx context.Context, challengeID int64, maxAttempts int) error
	ConsumeActiveChallenge(ctx context.Context, challengeID, userID int64, method, purpose string, maxAttempts int) error
	ActivateEmail(ctx context.Context, userID int64) error
	ActivateTOTP(ctx context.Context, userID int64, encryptedSecret []byte) error
	Disable(ctx context.Context, userID int64) error
	DeleteExpiredChallenges(ctx context.Context) error
}

// RecoveryCodeRepository cobre os códigos de recuperação de 2FA.
type RecoveryCodeRepository interface {
	Replace(ctx context.Context, userID int64, codeHashes []string) error
	Use(ctx context.Context, userID int64, codeHash string) error
	Delete(ctx context.Context, userID int64) error
}

const (
	twoFactorIssuer             = "Bridopen"
	twoFactorCodeTTL            = 10 * time.Minute
	twoFactorTOTPSetupTTL       = 10 * time.Minute
	twoFactorChallengeCooldown  = 1 * time.Minute
	twoFactorMaxAttempts        = 5
	totpDigits                  = 6
	totpPeriodSeconds           = 30
	totpValidationWindowPeriods = 1
	recoveryCodeCount           = 10
	recoveryCodeRawBytes        = 8
	recoveryCodeNormalizedLen   = recoveryCodeRawBytes * 2
	recoveryCodeHashContext     = "two_factor_recovery_code:"
)

type TwoFactorEmailRenderer interface {
	RenderTwoFactorCode(code string) ([]byte, error)
}

type TwoFactorService struct {
	repo          TwoFactorRepository
	mail          mailer.MailService
	emailRenderer TwoFactorEmailRenderer
	recoveryCodes RecoveryCodeRepository
	encryptionKey []byte
}

type TwoFactorSetupResult struct {
	Method       string `json:"method"`
	Message      string `json:"message"`
	QRCodeData   string `json:"qr_code_data,omitempty"`
	ManualSecret string `json:"manual_secret,omitempty"`
}

type TwoFactorStatusView = models.TwoFactorStatusView

func NewTwoFactorService(repo TwoFactorRepository, mail mailer.MailService, encryptionSecret string, renderers ...TwoFactorEmailRenderer) *TwoFactorService {
	key := sha256.Sum256([]byte(encryptionSecret))
	var emailRenderer TwoFactorEmailRenderer
	if len(renderers) > 0 {
		emailRenderer = renderers[0]
	}
	return &TwoFactorService{repo: repo, mail: mail, emailRenderer: emailRenderer, encryptionKey: key[:]}
}

func (s *TwoFactorService) WithRecoveryCodes(repo RecoveryCodeRepository) *TwoFactorService {
	s.recoveryCodes = repo
	return s
}

func (s *TwoFactorService) Status(ctx context.Context, userID int64) (*TwoFactorStatusView, error) {
	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	return NewTwoFactorStatusView(status), nil
}

func NewTwoFactorStatusView(status *models.TwoFactorStatus) *TwoFactorStatusView {
	return models.NewTwoFactorStatusView(status)
}

func (s *TwoFactorService) sendEmailCode(email, code string) error {
	if s.emailRenderer == nil {
		return apperrors.ErrTwoFactorMailRenderer
	}
	body, err := s.emailRenderer.RenderTwoFactorCode(code)
	if err != nil {
		return err
	}
	return s.mail.Send(mailer.MailMessage{
		To:      []string{email},
		Subject: "Código de verificação em duas etapas",
		IsHtml:  true,
		Body:    body,
	})
}

const (
	defaultActiveSessionLimit = 10
)

// AccountService orquestra dados de users e users_personal_information.
// AttachmentRemover é o contrato mínimo que a exclusão de conta precisa do
// armazenamento de anexos. Declarado aqui, no consumidor, para este módulo não
// depender do módulo de notas.
type AttachmentRemover interface {
	Delete(ctx context.Context, storageKey string) error
}

type AccountService struct {
	users         UserRepository
	personal      UserPersonalRepository
	twoFactor     TwoFactorRepository
	sessionActive UserSessionRepository
	attachments   AttachmentRemover
}

func NewAccountService(users UserRepository, personal UserPersonalRepository, twoFactor TwoFactorRepository, sessionActive UserSessionRepository) *AccountService {
	return &AccountService{users: users, personal: personal, twoFactor: twoFactor, sessionActive: sessionActive}
}

// WithAttachmentStorage liga o armazenamento de anexos, usado ao excluir a
// conta para remover os arquivos físicos do utilizador.
func (s *AccountService) WithAttachmentStorage(storage AttachmentRemover) *AccountService {
	s.attachments = storage
	return s
}

// AuthenticatedUser são os dados de login já validados.
type AuthenticatedUser struct {
	UserID           int64
	Email            string
	FirstName        string
	TwoFactorEnabled bool
}

// Authenticate valida as credenciais. Devolve apperrors.ErrInvalidCredentials
// quando o par e-mail/senha não confere e apperrors.ErrUserInactive quando a
// conta existe mas ainda não foi confirmada.
func (s *AccountService) Authenticate(ctx context.Context, email, password string) (*AuthenticatedUser, error) {
	user, personal, err := s.users.FindByUser(ctx, email)
	if err != nil {
		return nil, apperrors.ErrInvalidCredentials
	}
	if !platformpassword.ValidatePassword(password, user.Password.String) {
		return nil, apperrors.ErrInvalidCredentials
	}

	result := &AuthenticatedUser{
		Email:            user.Email.String,
		TwoFactorEnabled: user.TwoFactorEnabled.Valid && user.TwoFactorEnabled.Bool,
	}
	if user.Id.Int != nil {
		result.UserID = user.Id.Int.Int64()
	}
	if personal != nil && personal.FirstName.Valid {
		result.FirstName = personal.FirstName.String
	}

	if !user.Active.Bool {
		return result, apperrors.ErrUserInactive
	}
	return result, nil
}

// CreateUserSession registra a sessão ativa do utilizador.
func (s *AccountService) CreateUserSession(ctx context.Context, userID int64, sessionID, userAgent, ipAddress string) error {
	if s.sessionActive == nil {
		return nil
	}
	return s.sessionActive.Create(ctx, userID, sessionID, userAgent, ipAddress)
}

// RegisterUser cria a conta e o token de confirmação de e-mail. Devolve
// taken=true, sem criar nada, quando o e-mail já está cadastrado.
func (s *AccountService) RegisterUser(ctx context.Context, firstName, lastName, email, password string) (userID int64, token string, taken bool, err error) {
	taken, err = s.users.FindByEmail(ctx, email)
	if err != nil {
		return 0, "", false, err
	}
	if taken {
		return 0, "", true, nil
	}

	hashedPassword, err := platformpassword.HashPassword(password)
	if err != nil {
		return 0, "", false, err
	}
	rawToken, err := key.GenerateTokenKey()
	if err != nil {
		return 0, "", false, err
	}

	user, _, token, err := s.users.Create(ctx, firstName, lastName, email, hashedPassword, rawToken)
	if err != nil {
		if user != nil && user.Id.Int != nil {
			userID = user.Id.Int.Int64()
		}
		return userID, "", false, err
	}
	if user != nil && user.Id.Int != nil {
		userID = user.Id.Int.Int64()
	}
	return userID, token, false, nil
}

// RequestEmailConfirmation gera um novo token de confirmação de cadastro.
// Token vazio significa que não há e-mail a enviar (conta inexistente ou já ativa).
func (s *AccountService) RequestEmailConfirmation(ctx context.Context, email string) (string, []string, error) {
	rawToken, err := key.GenerateTokenKey()
	if err != nil {
		return "", nil, err
	}
	return s.users.CreateEmailConfirmationToken(ctx, email, rawToken)
}

// ConfirmUser ativa a conta a partir do token de confirmação.
func (s *AccountService) ConfirmUser(ctx context.Context, token string) (string, int64, error) {
	return s.users.ConfirmUserByToken(ctx, token)
}

// ConfirmRecoveryEmail promove o e-mail de recuperação pendente a definitivo.
func (s *AccountService) ConfirmRecoveryEmail(ctx context.Context, token string) (string, string, int64, error) {
	return s.users.ConfirmRecoveryEmailByToken(ctx, token)
}

// ExportUserData reúne os dados exportáveis da conta.
func (s *AccountService) ExportUserData(ctx context.Context, userID int64) (*models.AccountExport, error) {
	return s.users.ExportUserData(ctx, userID)
}

// DeleteAccount confere a senha, apaga a conta e remove os arquivos físicos dos
// anexos. Devolve apperrors.ErrInvalidCredentials quando a senha não confere ou
// a conta está inativa. Falha ao remover arquivo é apenas registrada: a conta já
// saiu do banco.
func (s *AccountService) DeleteAccount(ctx context.Context, userID int64, password string) error {
	_, passwordHash, active, err := s.users.FindPasswordAuthData(ctx, userID)
	if err != nil {
		return err
	}
	if !active || !platformpassword.ValidatePassword(password, passwordHash) {
		return apperrors.ErrInvalidCredentials
	}

	attachmentKeys, err := s.users.ListAttachmentStorageKeys(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.users.DeleteUserAndData(ctx, userID); err != nil {
		return err
	}
	if s.attachments != nil {
		for _, storageKey := range attachmentKeys {
			if err := s.attachments.Delete(ctx, storageKey); err != nil {
				slog.Warn("falha ao remover arquivo fisico de anexo ao excluir conta", slog.String("err", err.Error()), slog.String("storage_key", storageKey), slog.Int64("user_id", userID))
			}
		}
	}
	return nil
}

// RequestPasswordReset gera o token de redefinição de senha. Token vazio
// significa que não há e-mail a enviar.
func (s *AccountService) RequestPasswordReset(ctx context.Context, email string) (string, []string, error) {
	rawToken, err := key.GenerateTokenKey()
	if err != nil {
		return "", nil, err
	}
	return s.users.CreateResetPasswordToken(ctx, email, rawToken)
}

// ValidatePasswordResetToken confere se o token de redefinição continua válido.
func (s *AccountService) ValidatePasswordResetToken(ctx context.Context, token string) error {
	_, _, _, err := s.users.GetUserConfirmationByToken(ctx, token, userrepo.TokenPurposePasswordReset)
	return err
}

// ResetPassword aplica a nova senha a partir do token de redefinição.
func (s *AccountService) ResetPassword(ctx context.Context, token, password string) (string, int64, error) {
	hashedPassword, err := platformpassword.HashPassword(password)
	if err != nil {
		return "", 0, err
	}
	return s.users.PutPasswordByToken(ctx, hashedPassword, token)
}

// encrypt/decrypt usam AES-GCM com uma chave de 32 bytes derivada por sha256 em
// NewTwoFactorService, então aes.NewCipher e cipher.NewGCM nunca falham aqui.

func (s *TwoFactorService) encrypt(value string) ([]byte, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(value), nil), nil
}

func (s *TwoFactorService) decrypt(value []byte) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(value) < gcm.NonceSize() {
		return "", errors.New("segredo TOTP criptografado inválido")
	}
	nonce := value[:gcm.NonceSize()]
	cipherText := value[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func generateNumericCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func generateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func otpauthURL(email, secret string) string {
	label := twoFactorIssuer + ":" + email
	values := url.Values{}
	values.Set("secret", secret)
	values.Set("issuer", twoFactorIssuer)
	values.Set("algorithm", "SHA1")
	values.Set("digits", strconv.Itoa(totpDigits))
	values.Set("period", strconv.Itoa(totpPeriodSeconds))
	return "otpauth://totp/" + url.PathEscape(label) + "?" + values.Encode()
}

func normalizeVerificationCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return ""
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return code
}

func (s *TwoFactorService) hashVerificationCode(code string) string {
	mac := hmac.New(sha256.New, s.encryptionKey)
	_, _ = mac.Write([]byte(normalizeVerificationCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *TwoFactorService) hashMatches(hash, code string) bool {
	expected := []byte(strings.TrimSpace(hash))
	actual := []byte(s.hashVerificationCode(code))
	return hmac.Equal(expected, actual)
}

func validateTOTP(secret, code string, now time.Time) bool {
	for offset := -totpValidationWindowPeriods; offset <= totpValidationWindowPeriods; offset++ {
		t := now.Add(time.Duration(offset*totpPeriodSeconds) * time.Second)
		expected, err := totpCode(secret, t)
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

func totpCode(secret string, t time.Time) (string, error) {
	decoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := decoder.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	counter := uint64(math.Floor(float64(t.Unix()) / float64(totpPeriodSeconds)))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)
	otp := binCode % uint32(math.Pow10(totpDigits))
	return fmt.Sprintf("%06d", otp), nil
}

func (s *TwoFactorService) StartSetup(ctx context.Context, userID int64, method string) (*TwoFactorSetupResult, error) {
	method = strings.TrimSpace(method)
	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !status.Active {
		return nil, apperrors.ErrTwoFactorRequiresConfirmed
	}
	if status.Enabled {
		return nil, apperrors.ErrTwoFactorAlreadyEnabled
	}

	switch method {
	case models.TwoFactorMethodEmail:
		if err := s.ensureChallengeCooldown(ctx, userID, method, models.TwoFactorPurposeEnable); err != nil {
			return nil, err
		}
		code, err := generateNumericCode()
		if err != nil {
			return nil, err
		}
		if _, err := s.repo.CreateChallenge(ctx, userID, method, models.TwoFactorPurposeEnable, s.hashVerificationCode(code), nil, twoFactorCodeTTL); err != nil {
			return nil, err
		}
		if err := s.sendEmailCode(status.Email, code); err != nil {
			return nil, err
		}
		return &TwoFactorSetupResult{
			Method:  method,
			Message: "Enviamos um código de verificação para o e-mail cadastrado.",
		}, nil
	case models.TwoFactorMethodTOTP:
		if err := s.ensureChallengeCooldown(ctx, userID, method, models.TwoFactorPurposeEnable); err != nil {
			return nil, err
		}
		secret, err := generateTOTPSecret()
		if err != nil {
			return nil, err
		}
		encrypted, err := s.encrypt(secret)
		if err != nil {
			return nil, err
		}
		if _, err := s.repo.CreateChallenge(ctx, userID, method, models.TwoFactorPurposeEnable, "", encrypted, twoFactorTOTPSetupTTL); err != nil {
			return nil, err
		}
		otpURL := otpauthURL(status.Email, secret)
		png, err := qrcode.Encode(otpURL, qrcode.Medium, 192)
		if err != nil {
			return nil, err
		}
		return &TwoFactorSetupResult{
			Method:       method,
			Message:      "Escaneie o QR Code e informe o código gerado pelo aplicativo.",
			QRCodeData:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
			ManualSecret: secret,
		}, nil
	default:
		return nil, apperrors.ErrTwoFactorInvalidMethod
	}
}

func (s *TwoFactorService) VerifySetup(ctx context.Context, userID int64, method, code string) error {
	_, err := s.VerifySetupWithRecoveryCodes(ctx, userID, method, code)
	return err
}

func (s *TwoFactorService) VerifySetupWithRecoveryCodes(ctx context.Context, userID int64, method, code string) ([]string, error) {
	method = strings.TrimSpace(method)
	if !isTwoFactorMethod(method) {
		return nil, apperrors.ErrTwoFactorInvalidMethod
	}
	code = normalizeVerificationCode(code)
	if code == "" {
		return nil, apperrors.ErrTwoFactorInvalidCode
	}

	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !status.Active {
		return nil, apperrors.ErrTwoFactorRequiresConfirmed
	}
	if status.Enabled {
		return nil, apperrors.ErrTwoFactorAlreadyEnabled
	}

	challenge, err := s.repo.GetActiveChallenge(ctx, userID, method, models.TwoFactorPurposeEnable)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, apperrors.ErrTwoFactorChallengeExpired
		}
		return nil, err
	}
	if challenge.AttemptCount >= twoFactorMaxAttempts {
		return nil, apperrors.ErrTwoFactorTooManyAttempts
	}

	switch method {
	case models.TwoFactorMethodEmail:
		if !s.hashMatches(challenge.CodeHash, code) {
			_ = s.repo.IncrementActiveChallengeAttempts(ctx, challenge.ID, twoFactorMaxAttempts)
			return nil, apperrors.ErrTwoFactorInvalidCode
		}
		if err := s.consumeChallenge(ctx, challenge, models.TwoFactorPurposeEnable); err != nil {
			return nil, err
		}
		return s.activateWithRecoveryCodes(ctx, userID, func() error {
			return s.repo.ActivateEmail(ctx, userID)
		})
	case models.TwoFactorMethodTOTP:
		secret, err := s.decrypt(challenge.TOTPSecretEncrypted)
		if err != nil {
			return nil, err
		}
		if !validateTOTP(secret, code, time.Now().UTC()) {
			_ = s.repo.IncrementActiveChallengeAttempts(ctx, challenge.ID, twoFactorMaxAttempts)
			return nil, apperrors.ErrTwoFactorInvalidCode
		}
		if err := s.consumeChallenge(ctx, challenge, models.TwoFactorPurposeEnable); err != nil {
			return nil, err
		}
		return s.activateWithRecoveryCodes(ctx, userID, func() error {
			return s.repo.ActivateTOTP(ctx, userID, challenge.TOTPSecretEncrypted)
		})
	default:
		return nil, apperrors.ErrTwoFactorInvalidMethod
	}
}

func isTwoFactorMethod(method string) bool {
	return method == models.TwoFactorMethodEmail || method == models.TwoFactorMethodTOTP
}

func (s *TwoFactorService) BeginLogin(ctx context.Context, userID int64) (*TwoFactorStatusView, error) {
	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	view := NewTwoFactorStatusView(status)
	if !view.Enabled {
		return view, nil
	}
	if view.Method == models.TwoFactorMethodEmail {
		if err := s.ensureChallengeCooldown(ctx, userID, models.TwoFactorMethodEmail, models.TwoFactorPurposeLogin); err != nil {
			if errors.Is(err, apperrors.ErrTwoFactorCooldown) {
				return view, nil
			}
			return nil, err
		}
		code, err := generateNumericCode()
		if err != nil {
			return nil, err
		}
		if _, err := s.repo.CreateChallenge(ctx, userID, models.TwoFactorMethodEmail, models.TwoFactorPurposeLogin, s.hashVerificationCode(code), nil, twoFactorCodeTTL); err != nil {
			return nil, err
		}
		if err := s.sendEmailCode(status.Email, code); err != nil {
			return nil, err
		}
	} else if view.Method == models.TwoFactorMethodTOTP {
		if _, err := s.repo.CreateChallenge(ctx, userID, models.TwoFactorMethodTOTP, models.TwoFactorPurposeLogin, "", nil, twoFactorCodeTTL); err != nil {
			return nil, err
		}
	}
	return view, nil
}

func (s *TwoFactorService) VerifyLogin(ctx context.Context, userID int64, code string) (*TwoFactorStatusView, error) {
	rawCode := strings.TrimSpace(code)
	if rawCode == "" {
		return nil, apperrors.ErrTwoFactorInvalidCode
	}

	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	view := NewTwoFactorStatusView(status)
	if !view.Enabled {
		return view, nil
	}

	challenge, err := s.getActiveLoginChallenge(ctx, userID, view.Method)
	if err != nil {
		return nil, err
	}
	if challenge.AttemptCount >= twoFactorMaxAttempts {
		return nil, apperrors.ErrTwoFactorTooManyAttempts
	}

	if recoveryCode := normalizeRecoveryCode(rawCode); recoveryCode != "" {
		if err := s.useRecoveryCode(ctx, userID, recoveryCode); err != nil {
			_ = s.repo.IncrementActiveChallengeAttempts(ctx, challenge.ID, twoFactorMaxAttempts)
			return nil, err
		}
		if err := s.consumeChallenge(ctx, challenge, models.TwoFactorPurposeLogin); err != nil {
			return nil, err
		}
		return view, nil
	}

	code = normalizeVerificationCode(rawCode)
	if code == "" {
		return nil, apperrors.ErrTwoFactorInvalidCode
	}

	switch view.Method {
	case models.TwoFactorMethodEmail:
		if !s.hashMatches(challenge.CodeHash, code) {
			_ = s.repo.IncrementActiveChallengeAttempts(ctx, challenge.ID, twoFactorMaxAttempts)
			return nil, apperrors.ErrTwoFactorInvalidCode
		}
		if err := s.consumeChallenge(ctx, challenge, models.TwoFactorPurposeLogin); err != nil {
			return nil, err
		}
		return view, nil
	case models.TwoFactorMethodTOTP:
		secret, err := s.decrypt(status.TOTPSecretEncrypted)
		if err != nil {
			return nil, err
		}
		if !validateTOTP(secret, code, time.Now().UTC()) {
			_ = s.repo.IncrementActiveChallengeAttempts(ctx, challenge.ID, twoFactorMaxAttempts)
			return nil, apperrors.ErrTwoFactorInvalidCode
		}
		if err := s.consumeChallenge(ctx, challenge, models.TwoFactorPurposeLogin); err != nil {
			return nil, err
		}
		return view, nil
	default:
		return nil, apperrors.ErrTwoFactorInvalidMethod
	}
}

func (s *TwoFactorService) getActiveLoginChallenge(ctx context.Context, userID int64, method string) (*models.TwoFactorChallenge, error) {
	if !isTwoFactorMethod(method) {
		return nil, apperrors.ErrTwoFactorInvalidMethod
	}
	challenge, err := s.repo.GetActiveChallenge(ctx, userID, method, models.TwoFactorPurposeLogin)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, apperrors.ErrTwoFactorChallengeExpired
		}
		return nil, err
	}
	return challenge, nil
}

func (s *TwoFactorService) useRecoveryCode(ctx context.Context, userID int64, code string) error {
	if s.recoveryCodes == nil {
		return apperrors.ErrTwoFactorInvalidCode
	}
	if err := s.recoveryCodes.Use(ctx, userID, s.hashRecoveryCode(code)); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrTwoFactorInvalidCode
		}
		return err
	}
	return nil
}

func (s *TwoFactorService) consumeChallenge(ctx context.Context, challenge *models.TwoFactorChallenge, purpose string) error {
	if err := s.repo.ConsumeActiveChallenge(ctx, challenge.ID, challenge.UserID, challenge.Method, purpose, twoFactorMaxAttempts); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrTwoFactorChallengeExpired
		}
		return err
	}
	return nil
}

func (s *TwoFactorService) ensureChallengeCooldown(ctx context.Context, userID int64, method, purpose string) error {
	challenge, err := s.repo.GetActiveChallenge(ctx, userID, method, purpose)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return err
	}
	if challenge.CreatedAt == nil {
		return apperrors.ErrTwoFactorCooldown
	}
	if time.Since(challenge.CreatedAt.UTC()) < twoFactorChallengeCooldown {
		return apperrors.ErrTwoFactorCooldown
	}
	return nil
}

func (s *TwoFactorService) disableWithoutVerification(ctx context.Context, userID int64) error {
	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return err
	}
	if !status.Enabled {
		return apperrors.ErrTwoFactorDisabled
	}
	return s.repo.Disable(ctx, userID)
}

func (s *TwoFactorService) StartDisable(ctx context.Context, userID int64) (*TwoFactorSetupResult, error) {
	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !status.Enabled {
		return nil, apperrors.ErrTwoFactorDisabled
	}

	switch status.Method {
	case models.TwoFactorMethodEmail:
		if err := s.ensureChallengeCooldown(ctx, userID, models.TwoFactorMethodEmail, models.TwoFactorPurposeDisable); err != nil {
			return nil, err
		}
		code, err := generateNumericCode()
		if err != nil {
			return nil, err
		}
		if _, err := s.repo.CreateChallenge(ctx, userID, models.TwoFactorMethodEmail, models.TwoFactorPurposeDisable, s.hashVerificationCode(code), nil, twoFactorCodeTTL); err != nil {
			return nil, err
		}
		if err := s.sendEmailCode(status.Email, code); err != nil {
			return nil, err
		}
		return &TwoFactorSetupResult{
			Method:  models.TwoFactorMethodEmail,
			Message: "Enviamos um código de verificação para confirmar a desativação.",
		}, nil
	case models.TwoFactorMethodTOTP:
		if err := s.ensureChallengeCooldown(ctx, userID, models.TwoFactorMethodTOTP, models.TwoFactorPurposeDisable); err != nil {
			return nil, err
		}
		if _, err := s.repo.CreateChallenge(ctx, userID, models.TwoFactorMethodTOTP, models.TwoFactorPurposeDisable, "", nil, twoFactorCodeTTL); err != nil {
			return nil, err
		}
		return &TwoFactorSetupResult{
			Method:  models.TwoFactorMethodTOTP,
			Message: "Informe o código atual do aplicativo autenticador para confirmar a desativação.",
		}, nil
	default:
		return nil, apperrors.ErrTwoFactorInvalidMethod
	}
}

func (s *TwoFactorService) VerifyDisable(ctx context.Context, userID int64, code string) error {
	code = normalizeVerificationCode(code)
	if code == "" {
		return apperrors.ErrTwoFactorInvalidCode
	}

	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return err
	}
	if !status.Enabled {
		return apperrors.ErrTwoFactorDisabled
	}

	challenge, err := s.repo.GetActiveChallenge(ctx, userID, status.Method, models.TwoFactorPurposeDisable)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrTwoFactorChallengeExpired
		}
		return err
	}
	if challenge.AttemptCount >= twoFactorMaxAttempts {
		return apperrors.ErrTwoFactorTooManyAttempts
	}

	switch status.Method {
	case models.TwoFactorMethodEmail:
		if !s.hashMatches(challenge.CodeHash, code) {
			_ = s.repo.IncrementActiveChallengeAttempts(ctx, challenge.ID, twoFactorMaxAttempts)
			return apperrors.ErrTwoFactorInvalidCode
		}
	case models.TwoFactorMethodTOTP:
		secret, err := s.decrypt(status.TOTPSecretEncrypted)
		if err != nil {
			return err
		}
		if !validateTOTP(secret, code, time.Now().UTC()) {
			_ = s.repo.IncrementActiveChallengeAttempts(ctx, challenge.ID, twoFactorMaxAttempts)
			return apperrors.ErrTwoFactorInvalidCode
		}
	default:
		return apperrors.ErrTwoFactorInvalidMethod
	}

	if err := s.consumeChallenge(ctx, challenge, models.TwoFactorPurposeDisable); err != nil {
		return err
	}
	return s.disableWithoutVerification(ctx, userID)
}

func (s *TwoFactorService) GenerateRecoveryCodes(ctx context.Context, userID int64) ([]string, error) {
	if s.recoveryCodes == nil {
		return nil, apperrors.ErrRecoveryCodesUnavailable
	}
	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !status.Enabled {
		return nil, apperrors.ErrTwoFactorDisabled
	}
	codes, hashes, err := s.newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := s.recoveryCodes.Replace(ctx, userID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *TwoFactorService) activateWithRecoveryCodes(ctx context.Context, userID int64, activate func() error) ([]string, error) {
	var codes []string
	var hashes []string
	var err error

	if s.recoveryCodes != nil {
		codes, hashes, err = s.newRecoveryCodes()
		if err != nil {
			return nil, err
		}
		if err := s.recoveryCodes.Replace(ctx, userID, hashes); err != nil {
			return nil, err
		}
	}

	if err := activate(); err != nil {
		if s.recoveryCodes != nil {
			_ = s.recoveryCodes.Delete(ctx, userID)
		}
		return nil, err
	}
	return codes, nil
}

func (s *TwoFactorService) newRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	seen := make(map[string]struct{}, recoveryCodeCount)

	for len(codes) < recoveryCodeCount {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, formatRecoveryCode(code))
		hashes = append(hashes, s.hashRecoveryCode(code))
	}
	return codes, hashes, nil
}

func generateRecoveryCode() (string, error) {
	raw := make([]byte, recoveryCodeRawBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(raw)), nil
}

func normalizeRecoveryCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}

	var normalized strings.Builder
	normalized.Grow(recoveryCodeNormalizedLen)
	for _, r := range strings.ToUpper(code) {
		switch {
		case r == '-' || unicode.IsSpace(r):
			continue
		case (r >= '0' && r <= '9') || (r >= 'A' && r <= 'F'):
			normalized.WriteRune(r)
		default:
			return ""
		}
	}
	value := normalized.String()
	if len(value) != recoveryCodeNormalizedLen {
		return ""
	}
	return value
}

func formatRecoveryCode(code string) string {
	code = normalizeRecoveryCode(code)
	if code == "" {
		return ""
	}
	parts := make([]string, 0, recoveryCodeNormalizedLen/4)
	for i := 0; i < len(code); i += 4 {
		parts = append(parts, code[i:i+4])
	}
	return strings.Join(parts, "-")
}

func (s *TwoFactorService) hashRecoveryCode(code string) string {
	code = normalizeRecoveryCode(code)
	if code == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.encryptionKey)
	_, _ = mac.Write([]byte(recoveryCodeHashContext))
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidatePersonalForm valida campos do formulário de dados pessoais.
func ValidatePersonalForm(firstName, lastName, phone string) map[string]string {
	errs := make(map[string]string)

	if msg := validations.PersonalNameError("Nome", firstName, validations.MaxPersonalFirstNameLen); msg != "" {
		errs["first_name"] = msg
	}
	if msg := validations.PersonalNameError("Sobrenome", lastName, validations.MaxPersonalLastNameLen); msg != "" {
		errs["last_name"] = msg
	}
	if msg := validations.PersonalPhoneError(phone); msg != "" {
		errs["phone_number"] = msg
	}
	return errs
}

// SavePersonalInformation cria ou atualiza dados pessoais do utilizador autenticado.
// formID é o id enviado pelo formulário (0 se ainda não existir registo).
func (s *AccountService) SavePersonalInformation(ctx context.Context, userID int64, formID int, firstName, lastName, phone string) (created bool, err error) {
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	phone = strings.TrimSpace(phone)

	existing, err := s.personal.FindByUserID(ctx, userID)
	if err != nil {
		return false, err
	}

	recordID := formID
	if existing != nil {
		existingID := userrepo.PersonalRecordID(existing)
		switch {
		case recordID == 0:
			recordID = existingID
		case recordID != existingID:
			return false, fmt.Errorf("identificador de dados pessoais inválido")
		}
	} else if recordID > 0 {
		return false, fmt.Errorf("identificador de dados pessoais inválido")
	}

	if recordID > 0 {
		_, err = s.personal.Update(ctx, userID, recordID, firstName, lastName, phone)
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, err
		}
		return false, err
	}

	_, err = s.personal.Create(ctx, userID, firstName, lastName, phone)
	return true, err
}

// BuildMePage agrega conta (email, cadastro) e informações pessoais para /me.
func (s *AccountService) BuildMePage(ctx context.Context, userID int64) (userdto.MePage, error) {
	data, err := s.users.FindAccountBasics(ctx, userID)
	if err != nil {
		return userdto.MePage{}, err
	}

	var firstName, lastName, phone string
	personal, err := s.personal.FindByUserID(ctx, userID)
	if err != nil {
		return userdto.MePage{}, err
	}
	if personal != nil {
		if personal.FirstName.Valid {
			firstName = personal.FirstName.String
		}
		if personal.LastName.Valid {
			lastName = personal.LastName.String
		}
		if personal.PhoneNumber.Valid {
			phone = personal.PhoneNumber.String
		}
	}

	var createdAt *time.Time
	if data.CreatedAt.Valid {
		t := data.CreatedAt.Time
		createdAt = &t
	}

	page := userdto.NewMePage(data.Email.String, data.RecoveryEmail.String, data.PendingRecoveryEmail.String, firstName, lastName, phone, createdAt)
	page.PersonalInfoID = userrepo.PersonalRecordID(personal)
	if s.twoFactor != nil {
		status, err := s.twoFactor.GetStatus(ctx, userID)
		if err != nil {
			return userdto.MePage{}, err
		}
		page.SetTwoFactorStatus(status.Active, status.Enabled, status.Method)
	}
	return page, nil
}

func (s *AccountService) BuildSecurityPage(ctx context.Context, userID int64, currentSessionID string, sessionLimit int) (userdto.MePage, error) {
	page, err := s.BuildMePage(ctx, userID)
	if err != nil {
		return userdto.MePage{}, err
	}
	page.NavPath = "/me/security"
	page.ActiveSessionLimit = NormalizeActiveSessionLimit(sessionLimit)

	if s.sessionActive == nil {
		return page, nil
	}

	sessions, err := s.sessionActive.ListActive(ctx, userID, page.ActiveSessionLimit)
	if err != nil {
		return userdto.MePage{}, err
	}
	page.ActiveSessions = userdto.NewActiveSessionResponseList(sessions, currentSessionID)
	return page, nil
}

func NormalizeActiveSessionLimit(limit int) int {
	switch limit {
	case 10, 100, 250:
		return limit
	default:
		return defaultActiveSessionLimit
	}
}

func ValidatePasswordChangeForm(currentPassword, newPassword, passwordConfirm string) map[string]string {
	errs := make(map[string]string)
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)
	passwordConfirm = strings.TrimSpace(passwordConfirm)

	if currentPassword == "" {
		errs["current_password"] = "Informe a senha atual."
	}
	if newPassword == "" {
		errs["password"] = "Informe a nova senha."
	}
	if len(newPassword) < 12 {
		errs["password"] = validations.PasswordLengthError("A nova senha", newPassword)
	} else if len(newPassword) > 128 {
		errs["password"] = validations.PasswordLengthError("A nova senha", newPassword)
	}
	if passwordConfirm == "" {
		errs["password_confirm"] = "Confirme a nova senha."
	} else if newPassword != passwordConfirm {
		errs["password_confirm"] = "As senhas não conferem."
	}
	if currentPassword != "" && newPassword != "" && currentPassword == newPassword {
		errs["password"] = "A nova senha deve ser diferente da senha atual."
	}

	return errs
}

func (s *AccountService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword, passwordConfirm string) (map[string]string, error) {
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)
	passwordConfirm = strings.TrimSpace(passwordConfirm)

	fieldErrors := ValidatePasswordChangeForm(currentPassword, newPassword, passwordConfirm)
	if len(fieldErrors) > 0 {
		return fieldErrors, nil
	}

	_, currentHash, active, err := s.users.FindPasswordAuthData(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, apperrors.ErrUserInactive
	}

	if !platformpassword.ValidatePassword(currentPassword, currentHash) {
		return map[string]string{"current_password": "Senha atual incorreta."}, nil
	}

	hashedPassword, err := platformpassword.HashPassword(newPassword)
	if err != nil {
		return nil, err
	}

	if err := s.users.UpdatePasswordByID(ctx, userID, hashedPassword); err != nil {
		return nil, err
	}
	return nil, nil
}

func ValidateRecoveryEmailChangeForm(primaryEmail, currentRecoveryEmail, pendingRecoveryEmail, currentPassword, recoveryEmail string) map[string]string {
	errs := make(map[string]string)
	primaryEmail = strings.TrimSpace(primaryEmail)
	currentRecoveryEmail = strings.TrimSpace(currentRecoveryEmail)
	pendingRecoveryEmail = strings.TrimSpace(pendingRecoveryEmail)
	currentPassword = strings.TrimSpace(currentPassword)
	recoveryEmail = strings.TrimSpace(recoveryEmail)

	if currentPassword == "" {
		errs["current_password"] = "Informe a senha atual."
	}
	if recoveryEmail == "" {
		errs["recovery_email"] = "Informe o e-mail de recuperação."
	} else if !validations.IsEmailValid(recoveryEmail) {
		errs["recovery_email"] = "E-mail de recuperação inválido."
	} else if strings.EqualFold(recoveryEmail, primaryEmail) {
		errs["recovery_email"] = "Use um e-mail diferente do e-mail principal."
	} else if currentRecoveryEmail != "" && strings.EqualFold(recoveryEmail, currentRecoveryEmail) {
		errs["recovery_email"] = "Este e-mail já está configurado como recuperação."
	}

	return errs
}

func (s *AccountService) CancelRecoveryEmailChange(ctx context.Context, userID int64, pendingEmail string) error {
	return s.users.CancelRecoveryEmailChange(ctx, userID, strings.TrimSpace(pendingEmail))
}

func (s *AccountService) RequestRecoveryEmailChange(ctx context.Context, userID int64, currentPassword, recoveryEmail string) (fieldErrors map[string]string, primaryEmail, pendingEmail, token string, err error) {
	currentPassword = strings.TrimSpace(currentPassword)
	recoveryEmail = strings.TrimSpace(recoveryEmail)

	account, err := s.users.FindAccountBasics(ctx, userID)
	if err != nil {
		return nil, "", "", "", err
	}

	primaryEmail = strings.TrimSpace(account.Email.String)
	currentRecoveryEmail := ""
	if account.RecoveryEmail.Valid {
		currentRecoveryEmail = account.RecoveryEmail.String
	}
	currentPendingRecoveryEmail := ""
	if account.PendingRecoveryEmail.Valid {
		currentPendingRecoveryEmail = account.PendingRecoveryEmail.String
	}

	fieldErrors = ValidateRecoveryEmailChangeForm(primaryEmail, currentRecoveryEmail, currentPendingRecoveryEmail, currentPassword, recoveryEmail)
	if len(fieldErrors) > 0 {
		return fieldErrors, primaryEmail, recoveryEmail, "", nil
	}

	_, currentHash, active, err := s.users.FindPasswordAuthData(ctx, userID)
	if err != nil {
		return nil, "", "", "", err
	}
	if !active {
		return nil, "", "", "", apperrors.ErrUserInactive
	}
	if !platformpassword.ValidatePassword(currentPassword, currentHash) {
		return map[string]string{"current_password": "Senha atual incorreta."}, primaryEmail, recoveryEmail, "", nil
	}

	inUse, err := s.users.EmailAddressInUse(ctx, userID, recoveryEmail)
	if err != nil {
		return nil, "", "", "", err
	}
	if inUse {
		return map[string]string{"recovery_email": "Este e-mail não pode ser usado como recuperação."}, primaryEmail, recoveryEmail, "", nil
	}

	rawToken, err := key.GenerateTokenKey()
	if err != nil {
		return nil, "", "", "", err
	}
	primaryEmail, token, err = s.users.BeginRecoveryEmailChange(ctx, userID, recoveryEmail, rawToken)
	if err != nil {
		return nil, "", "", "", err
	}
	return nil, primaryEmail, recoveryEmail, token, nil
}

func (s *AccountService) RevokeOtherSessions(ctx context.Context, userID int64, currentSessionID string) error {
	currentSessionID = strings.TrimSpace(currentSessionID)
	if currentSessionID == "" || s.sessionActive == nil {
		return apperrors.ErrNotFound
	}
	return s.sessionActive.RevokeOther(ctx, userID, currentSessionID)
}

func (s *AccountService) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || s.sessionActive == nil {
		return apperrors.ErrNotFound
	}
	return s.sessionActive.Revoke(ctx, userID, sessionID)
}
