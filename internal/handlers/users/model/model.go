package model

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type User struct {
	Id                           pgtype.Numeric
	Email                        pgtype.Text
	RecoveryEmail                pgtype.Text
	PendingRecoveryEmail         pgtype.Text
	Password                     pgtype.Text
	Active                       pgtype.Bool
	TwoFactorEnabled             pgtype.Bool
	TwoFactorMethod              pgtype.Text
	TwoFactorTOTPSecretEncrypted []byte
	TwoFactorEnabledAt           pgtype.Timestamp
	TwoFactorDisabledAt          pgtype.Timestamp
	CreatedAt                    pgtype.Date
	UpdatedAt                    pgtype.Date
}

type UserConfirmationToken struct {
	Id        pgtype.Numeric
	UserId    pgtype.Numeric
	Token     pgtype.Text
	TokenHash pgtype.Text
	Purpose   pgtype.Text
	ExpiresAt pgtype.Timestamp
	Confirmed pgtype.Bool
	CreatedAt pgtype.Date
	UpdatedAt pgtype.Date
}

type UserSession struct {
	ID         int64
	UserID     int64
	SessionID  string
	UserAgent  string
	IPAddress  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	RevokedAt  *time.Time
}

type AccountExport struct {
	User  AccountExportUser   `json:"user"`
	Notes []AccountExportNote `json:"notes"`
}

type AccountExportUser struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Email            string     `json:"email"`
	PhoneNumber      string     `json:"phone_number"`
	Active           bool       `json:"active"`
	TwoFactorEnabled bool       `json:"two_factor_enabled"`
	TwoFactorMethod  string     `json:"two_factor_method,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
}

type AccountExportNote struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Color     string     `json:"color"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type TwoFactorStatus struct {
	UserID              int64
	Email               string
	Active              bool
	Enabled             bool
	Method              string
	TOTPSecretEncrypted []byte
	EnabledAt           *time.Time
	DisabledAt          *time.Time
}

type TwoFactorChallenge struct {
	ID                  int64
	UserID              int64
	Method              string
	Purpose             string
	CodeHash            string
	TOTPSecretEncrypted []byte
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	AttemptCount        int
	CreatedAt           *time.Time
	UpdatedAt           *time.Time
}

type TwoFactorStatusView struct {
	Active      bool   `json:"active"`
	Enabled     bool   `json:"enabled"`
	Method      string `json:"method"`
	StatusLabel string `json:"status_label"`
	ActionLabel string `json:"action_label"`
}

func NewTwoFactorStatusView(status *TwoFactorStatus) *TwoFactorStatusView {
	view := &TwoFactorStatusView{
		ActionLabel: TwoFactorActionLabel(false),
		StatusLabel: "Desabilitado",
	}
	if status == nil {
		return view
	}
	view.Active = status.Active
	view.Enabled = status.Enabled
	view.Method = status.Method
	if !view.Enabled {
		return view
	}
	view.ActionLabel = TwoFactorActionLabel(true)
	view.StatusLabel = TwoFactorStatusLabel(view.Method)
	return view
}

func NewTwoFactorStatusViewFromFields(active, enabled bool, method string) *TwoFactorStatusView {
	return NewTwoFactorStatusView(&TwoFactorStatus{
		Active:  active,
		Enabled: enabled,
		Method:  method,
	})
}

func TwoFactorStatusLabel(method string) string {
	switch method {
	case "email":
		return "Habilitado por e-mail"
	case "totp":
		return "Habilitado por aplicativo autenticador"
	default:
		return "Habilitado"
	}
}

func TwoFactorActionLabel(enabled bool) string {
	if enabled {
		return "Desabilitar"
	}
	return "Habilitar"
}

func TwoFactorLoginMethodLabel(method string) string {
	switch method {
	case "email":
		return "código enviado para o e-mail cadastrado"
	case "totp":
		return "código do aplicativo autenticador"
	default:
		return "código de verificação"
	}
}

// Métodos e propósitos de verificação em duas etapas. Vivem no model para
// handler, service e repositório compartilharem os mesmos valores sem
// dependência entre camadas.
const (
	TwoFactorMethodEmail = "email"
	TwoFactorMethodTOTP  = "totp"

	TwoFactorPurposeEnable  = "enable"
	TwoFactorPurposeLogin   = "login"
	TwoFactorPurposeDisable = "disable"
)
