package dto

import (
	"html/template"

	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

type UserRequest struct {
	Email    string
	Password string
	validations.FormValidator
}

type AccountPage struct {
	Email string
}

// MePage dados exibidos em /me (conta do utilizador autenticado).
type MePage struct {
	Email                              string
	RecoveryEmail                      string
	PendingRecoveryEmail               string
	FullName                           string
	FirstName                          string
	LastName                           string
	PhoneNumber                        string
	Phone                              string
	MemberSince                        string
	NavPath                            string
	FocusSection                       string
	HasPersonalInfo                    bool
	PersonalInfoID                     int
	OpenEditModal                      bool
	OpenPasswordModal                  bool
	OpenRecoveryEmailModal             bool
	OpenDeleteModal                    bool
	PasswordChangeSuccess              bool
	RecoveryEmailChangeRequested       bool
	TwoFactorActive                    bool
	TwoFactorEnabled                   bool
	TwoFactorMethod                    string
	TwoFactorStatus                    string
	TwoFactorActionLabel               string
	SelectedTwoFactorMethod            string
	SelectedTwoFactorMethodTitle       string
	SelectedTwoFactorMethodDescription string
	ActiveSessions                     []ActiveSessionResponse
	ActiveSessionLimit                 int
	validations.FormValidator
}

type ActiveSessionResponse struct {
	ID         int64
	SessionID  string
	Device     string
	Browser    string
	IPAddress  string
	CreatedAt  string
	LastSeenAt string
	Current    bool
}

// SignupPage junta formulário de registro e estado de sucesso no mesmo template.
type AuthCaptcha struct {
	Enabled bool
	ID      string
	Content template.URL
	Open    bool
}

type SignupPage struct {
	Form            UserRequest
	Success         bool
	Verified        bool
	RegisteredEmail string
	Token           string
	Captcha         AuthCaptcha
}

type TwoFactorCodeRequest struct {
	Code string
	validations.FormValidator
}

type TwoFactorLoginPage struct {
	Method      string
	MethodLabel string
	Form        TwoFactorCodeRequest
	Captcha     AuthCaptcha
}
