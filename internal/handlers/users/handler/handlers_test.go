package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	noteservice "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/service"
	usermodel "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	userservice "github.com/engenheiroaraujo/bridopen/internal/handlers/users/service"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/mailer"
	platformpassword "github.com/engenheiroaraujo/bridopen/internal/platform/password"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/gorilla/csrf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCSRFKey = []byte("01234567890123456789012345678901")

func installPassingCaptcha(t *testing.T) {
	t.Helper()
	origGen, origVal := generateCaptchaFn, validateCaptchaFn
	t.Cleanup(func() {
		generateCaptchaFn = origGen
		validateCaptchaFn = origVal
	})
	generateCaptchaFn = func() (string, string, error) { return "cid", "img", nil }
	validateCaptchaFn = func(string, string) bool { return true }
}

func csrfProtectUser(next http.Handler) http.Handler {
	return csrf.Protect(testCSRFKey, csrf.Secure(false), csrf.Path("/"), csrf.TrustedOrigins([]string{"example.com"}))(next)
}

func serveUser(t *testing.T, session *scs.SessionManager, userID int64, setup func(context.Context), fn func(http.ResponseWriter, *http.Request) error, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	req.Host = "example.com"
	req.URL.Scheme = "http"
	req.URL.Host = "example.com"
	req.Header.Set("Referer", "http://example.com/")
	req.Header.Del("Origin")

	var token string
	issue := csrfProtectUser(session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = csrf.Token(r)
		w.WriteHeader(http.StatusNoContent)
	})))
	issueRec := httptest.NewRecorder()
	issue.ServeHTTP(issueRec, csrf.PlaintextHTTPRequest(httptest.NewRequest(http.MethodGet, "http://example.com/", nil)))
	for _, c := range issueRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set("X-CSRF-Token", token)

	rec := httptest.NewRecorder()
	handler := csrfProtectUser(session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID > 0 {
			session.Put(r.Context(), "userId", userID)
			session.Put(r.Context(), "userSessionId", "sess-1")
			session.Put(r.Context(), "userEmail", "user@example.com")
			session.Put(r.Context(), "firstName", "Ada")
		}
		if setup != nil {
			setup(r.Context())
		}
		if err := fn(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})))
	handler.ServeHTTP(rec, csrf.PlaintextHTTPRequest(req))
	return rec
}

func formReq(method, target string, values url.Values) *http.Request {
	body := ""
	if values != nil {
		body = values.Encode()
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req
}

func captchaValues(extra url.Values) url.Values {
	if extra == nil {
		extra = url.Values{}
	}
	extra.Set("captcha_checked", "1")
	extra.Set("captcha_id", "cid")
	extra.Set("captcha_answer", "123456")
	return extra
}

func num(id int64) pgtype.Numeric { return pgtype.Numeric{Int: big.NewInt(id), Valid: true} }
func txt(s string) pgtype.Text    { return pgtype.Text{String: s, Valid: true} }

type handlerMail struct{ err error }

func (m handlerMail) Send(mailer.MailMessage) error { return m.err }

type attachStore struct {
	deleteErr error
	deleted   []string
}

func (a *attachStore) Save(context.Context, int, int, multipart.File, *multipart.FileHeader) (noteservice.StoredAttachment, error) {
	return noteservice.StoredAttachment{}, errors.New("unused")
}
func (a *attachStore) Path(string) (string, error) { return "", errors.New("unused") }
func (a *attachStore) Delete(_ context.Context, key string) error {
	a.deleted = append(a.deleted, key)
	return a.deleteErr
}

type fakeHandlerUserRepo struct {
	findByEmail       bool
	findByEmailErr    error
	user              *usermodel.User
	personal          *usermodel.UserPersonalInformation
	findByUserErr     error
	createUser        *usermodel.User
	createToken       string
	createErr         error
	resetToken        string
	resetRecipients   []string
	resetErr          error
	resendToken       string
	resendRecipients  []string
	resendErr         error
	confirmEmail      string
	confirmUserID     int64
	confirmErr        error
	recoveryPrimary   string
	recoveryEmail     string
	recoveryUserID    int64
	recoveryErr       error
	getTokenErr       error
	putPasswordEmail  string
	putPasswordUserID int64
	putPasswordErr    error
	exportData        *usermodel.AccountExport
	exportErr         error
	attachKeys        []string
	attachKeysErr     error
	deleteErr         error
	account           *usermodel.User
	accountErr        error
	accountErrAfter   int
	accountCalls      int
	passwordHash      string
	passwordActive    bool
	passwordErr       error
	emailInUse        bool
	beginPrimary      string
	beginToken        string
	beginErr          error
	cancelErr         error
	updatePasswordErr error
}

func (f *fakeHandlerUserRepo) FindByEmail(context.Context, string) (bool, error) {
	return f.findByEmail, f.findByEmailErr
}
func (f *fakeHandlerUserRepo) FindByUser(context.Context, string) (*usermodel.User, *usermodel.UserPersonalInformation, error) {
	if f.findByUserErr != nil {
		return nil, nil, f.findByUserErr
	}
	return f.user, f.personal, nil
}
func (f *fakeHandlerUserRepo) CreateResetPasswordToken(context.Context, string, string) (string, []string, error) {
	return f.resetToken, f.resetRecipients, f.resetErr
}
func (f *fakeHandlerUserRepo) CreateEmailConfirmationToken(context.Context, string, string) (string, []string, error) {
	return f.resendToken, f.resendRecipients, f.resendErr
}
func (f *fakeHandlerUserRepo) GetUserConfirmationByToken(context.Context, string, string) (*usermodel.User, *usermodel.UserConfirmationToken, string, error) {
	if f.getTokenErr != nil {
		return nil, nil, "", f.getTokenErr
	}
	return &usermodel.User{}, &usermodel.UserConfirmationToken{}, "purpose", nil
}
func (f *fakeHandlerUserRepo) PutPasswordByToken(context.Context, string, string) (string, int64, error) {
	return f.putPasswordEmail, f.putPasswordUserID, f.putPasswordErr
}
func (f *fakeHandlerUserRepo) Create(context.Context, string, string, string, string, string) (*usermodel.User, *usermodel.UserPersonalInformation, string, error) {
	if f.createErr != nil {
		u := &usermodel.User{Id: num(1)}
		if f.createUser != nil {
			u = f.createUser
		}
		return u, nil, "", f.createErr
	}
	u := f.createUser
	if u == nil {
		u = &usermodel.User{Id: num(9), Email: txt("new@example.com")}
	}
	tok := f.createToken
	if tok == "" {
		tok = "tok"
	}
	return u, &usermodel.UserPersonalInformation{}, tok, nil
}
func (f *fakeHandlerUserRepo) ConfirmUserByToken(context.Context, string) (string, int64, error) {
	return f.confirmEmail, f.confirmUserID, f.confirmErr
}
func (f *fakeHandlerUserRepo) DeleteExpiredTokens(context.Context) error { return nil }
func (f *fakeHandlerUserRepo) ExportUserData(context.Context, int64) (*usermodel.AccountExport, error) {
	return f.exportData, f.exportErr
}
func (f *fakeHandlerUserRepo) ListAttachmentStorageKeys(context.Context, int64) ([]string, error) {
	return f.attachKeys, f.attachKeysErr
}
func (f *fakeHandlerUserRepo) DeleteUserAndData(context.Context, int64) error { return f.deleteErr }
func (f *fakeHandlerUserRepo) FindAccountBasics(context.Context, int64) (*usermodel.User, error) {
	f.accountCalls++
	if f.accountErrAfter > 0 {
		if f.accountCalls >= f.accountErrAfter {
			if f.accountErr != nil {
				return nil, f.accountErr
			}
			return nil, errors.New("account after n")
		}
	} else if f.accountErr != nil {
		return nil, f.accountErr
	}
	if f.account == nil {
		return nil, apperrors.ErrNotFound
	}
	cp := *f.account
	return &cp, nil
}
func (f *fakeHandlerUserRepo) EmailAddressInUse(context.Context, int64, string) (bool, error) {
	return f.emailInUse, nil
}
func (f *fakeHandlerUserRepo) BeginRecoveryEmailChange(context.Context, int64, string, string) (string, string, error) {
	return f.beginPrimary, f.beginToken, f.beginErr
}
func (f *fakeHandlerUserRepo) CancelRecoveryEmailChange(context.Context, int64, string) error {
	return f.cancelErr
}
func (f *fakeHandlerUserRepo) ConfirmRecoveryEmailByToken(context.Context, string) (string, string, int64, error) {
	return f.recoveryPrimary, f.recoveryEmail, f.recoveryUserID, f.recoveryErr
}
func (f *fakeHandlerUserRepo) FindPasswordAuthData(context.Context, int64) (string, string, bool, error) {
	if f.passwordErr != nil {
		return "", "", false, f.passwordErr
	}
	return "user@example.com", f.passwordHash, f.passwordActive, nil
}
func (f *fakeHandlerUserRepo) UpdatePasswordByID(context.Context, int64, string) error {
	return f.updatePasswordErr
}

type fakeHandlerPersonalRepo struct {
	info      *usermodel.UserPersonalInformation
	findErr   error
	createErr error
	updateErr error
}

func (f *fakeHandlerPersonalRepo) FindByUserID(context.Context, int64) (*usermodel.UserPersonalInformation, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.info, nil
}
func (f *fakeHandlerPersonalRepo) Create(_ context.Context, _ int64, first, last, phone string) (*usermodel.UserPersonalInformation, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &usermodel.UserPersonalInformation{Id: num(1), FirstName: txt(first), LastName: txt(last), PhoneNumber: txt(phone)}, nil
}
func (f *fakeHandlerPersonalRepo) Update(_ context.Context, _ int64, id int, first, last, phone string) (*usermodel.UserPersonalInformation, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &usermodel.UserPersonalInformation{Id: num(int64(id)), FirstName: txt(first), LastName: txt(last), PhoneNumber: txt(phone)}, nil
}

type fakeHandlerSessionRepo struct {
	createErr error
	revokeErr error
	sessions  []usermodel.UserSession
	listErr   error
}

func (f *fakeHandlerSessionRepo) Create(context.Context, int64, string, string, string) error {
	return f.createErr
}
func (f *fakeHandlerSessionRepo) ListActive(context.Context, int64, int) ([]usermodel.UserSession, error) {
	return f.sessions, f.listErr
}
func (f *fakeHandlerSessionRepo) IsActive(context.Context, int64, string) (bool, error) {
	return true, nil
}
func (f *fakeHandlerSessionRepo) Touch(context.Context, string) error { return nil }
func (f *fakeHandlerSessionRepo) RevokeOther(context.Context, int64, string) error {
	return f.revokeErr
}
func (f *fakeHandlerSessionRepo) Revoke(context.Context, int64, string) error { return f.revokeErr }

type fakeHandlerTwoFactorRepo struct {
	status       *usermodel.TwoFactorStatus
	err          error
	errAfter     int
	getCalls     int
	challenge    *usermodel.TwoFactorChallenge
	challengeErr error
	consumeErr   error
	activateErr  error
	disableErr   error
	createErr    error
}

func (f *fakeHandlerTwoFactorRepo) GetStatus(context.Context, int64) (*usermodel.TwoFactorStatus, error) {
	f.getCalls++
	if f.err != nil {
		return nil, f.err
	}
	if f.errAfter > 0 && f.getCalls >= f.errAfter {
		return nil, errors.New("status after n")
	}
	if f.status == nil {
		return &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}, nil
	}
	cp := *f.status
	return &cp, nil
}
func (f *fakeHandlerTwoFactorRepo) CreateChallenge(_ context.Context, userID int64, method, purpose, codeHash string, _ []byte, _ time.Duration) (*usermodel.TwoFactorChallenge, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.challenge = &usermodel.TwoFactorChallenge{
		ID: 1, UserID: userID, Method: method, Purpose: purpose, CodeHash: codeHash,
		ExpiresAt: time.Now().Add(time.Minute),
	}
	cp := *f.challenge
	return &cp, nil
}
func (f *fakeHandlerTwoFactorRepo) GetActiveChallenge(_ context.Context, _ int64, method, purpose string) (*usermodel.TwoFactorChallenge, error) {
	if f.challengeErr != nil {
		return nil, f.challengeErr
	}
	if f.challenge == nil || f.challenge.Method != method || f.challenge.Purpose != purpose {
		return nil, apperrors.ErrNotFound
	}
	cp := *f.challenge
	return &cp, nil
}
func (f *fakeHandlerTwoFactorRepo) IncrementActiveChallengeAttempts(context.Context, int64, int) error {
	return nil
}
func (f *fakeHandlerTwoFactorRepo) ConsumeActiveChallenge(context.Context, int64, int64, string, string, int) error {
	return f.consumeErr
}
func (f *fakeHandlerTwoFactorRepo) ActivateEmail(context.Context, int64) error { return f.activateErr }
func (f *fakeHandlerTwoFactorRepo) ActivateTOTP(context.Context, int64, []byte) error {
	return f.activateErr
}
func (f *fakeHandlerTwoFactorRepo) Disable(context.Context, int64) error          { return f.disableErr }
func (f *fakeHandlerTwoFactorRepo) DeleteExpiredChallenges(context.Context) error { return nil }

type fakeRecoveryRepo struct{}

func (fakeRecoveryRepo) Replace(context.Context, int64, []string) error { return nil }
func (fakeRecoveryRepo) Use(context.Context, int64, string) error       { return nil }
func (fakeRecoveryRepo) Delete(context.Context, int64) error            { return nil }

type twoFactorMailRenderer struct {
	lastCode string
}

func (r *twoFactorMailRenderer) RenderTwoFactorCode(code string) ([]byte, error) {
	r.lastCode = code
	return []byte("mail"), nil
}

func newTestUserHandler(t *testing.T, users *fakeHandlerUserRepo, sessions *fakeHandlerSessionRepo, personal *fakeHandlerPersonalRepo, tf *fakeHandlerTwoFactorRepo, mailErr error) (*userHandler, *scs.SessionManager, *userservice.AccountService, *userservice.TwoFactorService) {
	t.Helper()
	installPassingCaptcha(t)
	if users == nil {
		users = &fakeHandlerUserRepo{}
	}
	if sessions == nil {
		sessions = &fakeHandlerSessionRepo{}
	}
	if personal == nil {
		personal = &fakeHandlerPersonalRepo{info: &usermodel.UserPersonalInformation{Id: num(1), FirstName: txt("Ada"), LastName: txt("Lovelace")}}
	}
	if tf == nil {
		tf = &fakeHandlerTwoFactorRepo{status: &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}}
	}
	if users.account == nil {
		users.account = &usermodel.User{Id: num(1), Email: txt("user@example.com"), Active: pgtype.Bool{Bool: true, Valid: true}}
	}
	session := scs.New()
	account := userservice.NewAccountService(users, personal, tf, sessions).WithAttachmentStorage(&attachStore{})
	renderer := &twoFactorMailRenderer{}
	twoFactor := userservice.NewTwoFactorService(tf, handlerMail{err: mailErr}, "secret-with-more-than-32-characters", renderer).WithRecoveryCodes(fakeRecoveryRepo{})
	h := NewUserHandler(render.NewRender(session, "http://example.com"), session, handlerMail{err: mailErr}, twoFactor, account, audit.NewRecorder(nil))
	h.audit = audit.NewRecorder(nil)
	// guarda o renderer no jar de cookies da sessão via campo auxiliar de teste em tf, se necessário
	_ = renderer
	return h, session, account, twoFactor
}

func newTestUserHandlerWithRenderer(t *testing.T, users *fakeHandlerUserRepo, sessions *fakeHandlerSessionRepo, personal *fakeHandlerPersonalRepo, tf *fakeHandlerTwoFactorRepo, mailErr error, renderer *twoFactorMailRenderer) (*userHandler, *scs.SessionManager, *userservice.TwoFactorService) {
	t.Helper()
	installPassingCaptcha(t)
	if users == nil {
		users = &fakeHandlerUserRepo{}
	}
	if sessions == nil {
		sessions = &fakeHandlerSessionRepo{}
	}
	if personal == nil {
		personal = &fakeHandlerPersonalRepo{info: &usermodel.UserPersonalInformation{Id: num(1), FirstName: txt("Ada"), LastName: txt("Lovelace")}}
	}
	if tf == nil {
		tf = &fakeHandlerTwoFactorRepo{status: &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}}
	}
	if users.account == nil {
		users.account = &usermodel.User{Id: num(1), Email: txt("user@example.com"), Active: pgtype.Bool{Bool: true, Valid: true}}
	}
	if renderer == nil {
		renderer = &twoFactorMailRenderer{}
	}
	session := scs.New()
	account := userservice.NewAccountService(users, personal, tf, sessions).WithAttachmentStorage(&attachStore{})
	twoFactor := userservice.NewTwoFactorService(tf, handlerMail{err: mailErr}, "secret-with-more-than-32-characters", renderer).WithRecoveryCodes(fakeRecoveryRepo{})
	h := NewUserHandler(render.NewRender(session, "http://example.com"), session, handlerMail{err: mailErr}, twoFactor, account, audit.NewRecorder(nil))
	return h, session, twoFactor
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	h, err := platformpassword.HashPassword(password)
	require.NoError(t, err)
	return h
}

func activeUser(passwordHash string) *usermodel.User {
	return &usermodel.User{
		Id:       num(1),
		Email:    txt("user@example.com"),
		Password: txt(passwordHash),
		Active:   pgtype.Bool{Bool: true, Valid: true},
	}
}

// TestHasCaptchaError verifica que hasCaptchaError detecta erro de captcha no mapa de erros do formulário.
func TestHasCaptchaError(t *testing.T) {
	t.Parallel()
	assert.True(t, hasCaptchaError(map[string]string{"captcha": "obrigatório"}))
	assert.False(t, hasCaptchaError(map[string]string{"email": "erro"}))
	assert.False(t, hasCaptchaError(nil))
}

// TestFirstFormValue verifica que firstFormValue retorna o primeiro valor presente entre as chaves informadas.
func TestFirstFormValue(t *testing.T) {
	t.Parallel()

	form := url.Values{}
	form.Set("code", "123456")
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, req.ParseForm())

	assert.Equal(t, "123456", firstFormValue(req, "token", "code"))
	assert.Equal(t, "", firstFormValue(req, "missing"))
}

// TestTwoFactorMethodPageContent verifica título/descrição das páginas de método 2FA e rejeição de método inválido.
func TestTwoFactorMethodPageContent(t *testing.T) {
	t.Parallel()

	title, desc, ok := twoFactorMethodPageContent(usermodel.TwoFactorMethodEmail)
	assert.True(t, ok)
	assert.Equal(t, "Código por e-mail", title)
	assert.NotEmpty(t, desc)

	title, desc, ok = twoFactorMethodPageContent(usermodel.TwoFactorMethodTOTP)
	assert.True(t, ok)
	assert.Equal(t, "Aplicativo autenticador", title)
	assert.NotEmpty(t, desc)

	_, _, ok = twoFactorMethodPageContent("invalid")
	assert.False(t, ok)
}

// TestActiveSessionLimitFromRequest verifica que activeSessionLimitFromRequest lê o limite da query e aplica mínimos/padrões.
func TestActiveSessionLimitFromRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/?sessions=100", nil)
	assert.Equal(t, 100, activeSessionLimitFromRequest(req))

	req = httptest.NewRequest(http.MethodGet, "/?sessions=abc", nil)
	assert.Equal(t, 0, activeSessionLimitFromRequest(req))

	req = httptest.NewRequest(http.MethodGet, "/?sessions=5", nil)
	assert.Equal(t, 10, activeSessionLimitFromRequest(req))
}

// TestTwoFactorWriteJSON verifica que writeJSON serializa a resposta 2FA com Content-Type e status corretos.
func TestTwoFactorWriteJSON(t *testing.T) {
	t.Parallel()

	h := &twoFactorHandler{}
	rec := httptest.NewRecorder()
	err := h.writeJSON(rec, http.StatusOK, twoFactorResponse{OK: true, Message: "ok"})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var body twoFactorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.OK)
	assert.Equal(t, "ok", body.Message)
}

// TestTwoFactorWriteErrorStatusMapping verifica o mapeamento de erros de serviço 2FA para status HTTP em writeError.
func TestTwoFactorWriteErrorStatusMapping(t *testing.T) {
	t.Parallel()

	session := scs.New()
	repo := &fakeTwoFactorStatusRepo{status: &usermodel.TwoFactorStatus{
		UserID: 1, Email: "user@example.com", Active: true,
	}}
	service := userservice.NewTwoFactorService(repo, nil, "secret-with-more-than-32-characters")
	h := &twoFactorHandler{session: session, service: service}

	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"forbidden", apperrors.ErrTwoFactorRequiresConfirmed, http.StatusForbidden},
		{"conflict", apperrors.ErrTwoFactorAlreadyEnabled, http.StatusConflict},
		{"too_many", apperrors.ErrTwoFactorTooManyAttempts, http.StatusTooManyRequests},
		{"cooldown", apperrors.ErrTwoFactorCooldown, http.StatusTooManyRequests},
		{"invalid", apperrors.ErrTwoFactorInvalidCode, http.StatusUnprocessableEntity},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/2fa", nil)
			ctx, err := session.Load(req.Context(), "test-session")
			require.NoError(t, err)
			req = req.WithContext(ctx)

			require.NoError(t, h.writeError(rec, req, tc.err))
			assert.Equal(t, tc.status, rec.Code)

			var body twoFactorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.False(t, body.OK)
			assert.NotEmpty(t, body.Message)
		})
	}
}

type fakeTwoFactorStatusRepo struct {
	status *usermodel.TwoFactorStatus
}

func (f *fakeTwoFactorStatusRepo) GetStatus(context.Context, int64) (*usermodel.TwoFactorStatus, error) {
	if f.status == nil {
		return nil, apperrors.ErrNotFound
	}
	status := *f.status
	return &status, nil
}

func (f *fakeTwoFactorStatusRepo) CreateChallenge(context.Context, int64, string, string, string, []byte, time.Duration) (*usermodel.TwoFactorChallenge, error) {
	return nil, apperrors.ErrNotFound
}
func (f *fakeTwoFactorStatusRepo) GetActiveChallenge(context.Context, int64, string, string) (*usermodel.TwoFactorChallenge, error) {
	return nil, apperrors.ErrNotFound
}
func (f *fakeTwoFactorStatusRepo) IncrementActiveChallengeAttempts(context.Context, int64, int) error {
	return apperrors.ErrNotFound
}
func (f *fakeTwoFactorStatusRepo) ConsumeActiveChallenge(context.Context, int64, int64, string, string, int) error {
	return apperrors.ErrNotFound
}
func (f *fakeTwoFactorStatusRepo) ActivateEmail(context.Context, int64) error { return nil }
func (f *fakeTwoFactorStatusRepo) ActivateTOTP(context.Context, int64, []byte) error {
	return nil
}
func (f *fakeTwoFactorStatusRepo) Disable(context.Context, int64) error          { return nil }
func (f *fakeTwoFactorStatusRepo) DeleteExpiredChallenges(context.Context) error { return nil }

// TestSessionAndMailHardPaths verifica falhas de renew/destroy de sessão e envio de e-mail nos fluxos críticos.
func TestSessionAndMailHardPaths(t *testing.T) {
	hash := mustHash(t, "password-12345")
	users := &fakeHandlerUserRepo{
		user:              activeUser(hash),
		personal:          &usermodel.UserPersonalInformation{FirstName: txt("Ada")},
		account:           &usermodel.User{Id: num(1), Email: txt("user@example.com"), Active: pgtype.Bool{Bool: true, Valid: true}},
		passwordHash:      hash,
		passwordActive:    true,
		resetToken:        "rt",
		resetRecipients:   []string{"user@example.com"},
		resendToken:       "rt",
		resendRecipients:  []string{"user@example.com"},
		createToken:       "tok",
		recoveryPrimary:   "user@example.com",
		recoveryEmail:     "rec@example.com",
		recoveryUserID:    1,
		beginPrimary:      "user@example.com",
		beginToken:        "btok",
		putPasswordEmail:  "user@example.com",
		putPasswordUserID: 1,
	}
	renderer := &twoFactorMailRenderer{}
	tfRepo := &fakeHandlerTwoFactorRepo{status: &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}}
	h, session, twoFactor := newTestUserHandlerWithRenderer(t, users, nil, nil, tfRepo, nil, renderer)
	uph := NewUserPersonalHandler(h.render, session, h.account, h.mail, audit.NewRecorder(nil))
	tfh := NewTwoFactorHandler(h.render, session, twoFactor, audit.NewRecorder(nil))

	origRenew, origDestroy := sessionRenewToken, sessionDestroy
	t.Cleanup(func() {
		sessionRenewToken = origRenew
		sessionDestroy = origDestroy
	})

	sessionRenewToken = func(_ *scs.SessionManager, _ context.Context) error { return errors.New("renew boom") }
	rec := serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	users.user.TwoFactorEnabled = pgtype.Bool{Bool: true, Valid: true}
	tfRepo.status.Enabled = true
	tfRepo.status.Method = usermodel.TwoFactorMethodEmail
	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessionRenewToken = origRenew
	users.user.TwoFactorEnabled = pgtype.Bool{Bool: false, Valid: true}
	tfRepo.status.Enabled = false

	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	tfRepo.challenge = nil
	rec = serveUser(t, session, 1, nil, tfh.Start, formReq(http.MethodPost, "/api/2fa/start", url.Values{"method": {usermodel.TwoFactorMethodEmail}}))
	require.Equal(t, http.StatusOK, rec.Code)
	sessionRenewToken = func(_ *scs.SessionManager, _ context.Context) error { return errors.New("renew boom") }
	rec = serveUser(t, session, 1, nil, tfh.Verify, formReq(http.MethodPost, "/api/2fa/verify", url.Values{"method": {usermodel.TwoFactorMethodEmail}, "code": {renderer.lastCode}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessionRenewToken = origRenew

	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true, Enabled: true, Method: usermodel.TwoFactorMethodEmail}
	tfRepo.challenge = nil
	rec = serveUser(t, session, 1, nil, tfh.DisableStart, formReq(http.MethodPost, "/api/2fa/disable/start", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	sessionRenewToken = func(_ *scs.SessionManager, _ context.Context) error { return errors.New("renew boom") }
	rec = serveUser(t, session, 1, nil, tfh.DisableVerify, formReq(http.MethodPost, "/api/2fa/disable/verify", url.Values{"code": {renderer.lastCode}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessionRenewToken = origRenew

	sessionDestroy = func(_ *scs.SessionManager, _ context.Context) error { return errors.New("destroy boom") }
	rec = serveUser(t, session, 1, nil, h.Signout, formReq(http.MethodPost, "/user/signout", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{
		"account_delete_confirmation": {"excluir conta"}, "password": {"password-12345"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 1, func(ctx context.Context) { session.Put(ctx, "userSessionId", "") }, uph.RevokeOtherSessions, formReq(http.MethodPost, "/revoke", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	req := formReq(http.MethodPost, "/me/security/sessions/sess-1", nil)
	req.SetPathValue("sessionID", "sess-1")
	rec = serveUser(t, session, 1, nil, uph.RevokeSession, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessionDestroy = origDestroy

	sessionRenewToken = func(_ *scs.SessionManager, _ context.Context) error { return errors.New("renew boom") }
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"password-12345"}, "password": {"password-abcdef"}, "password_confirm": {"password-abcdef"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessionRenewToken = origRenew

	users.user.TwoFactorEnabled = pgtype.Bool{Bool: true, Valid: true}
	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true, Enabled: true, Method: usermodel.TwoFactorMethodEmail}
	tfRepo.challenge = &usermodel.TwoFactorChallenge{
		ID: 1, UserID: 1, Method: usermodel.TwoFactorMethodEmail, Purpose: usermodel.TwoFactorPurposeLogin,
		CodeHash: "dead", ExpiresAt: time.Now().Add(time.Minute), AttemptCount: 5,
	}
	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, pendingTwoFactorUserIDKey, int64(1))
		session.Put(ctx, pendingTwoFactorMethodKey, usermodel.TwoFactorMethodEmail)
	}, h.TwoFactorLoginVerify, formReq(http.MethodPost, "/user/twofactor", captchaValues(url.Values{"code": {"000000"}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.user.TwoFactorEnabled = pgtype.Bool{Bool: false, Valid: true}

	// caminho de aviso no envio de e-mail em ConfirmRecoveryEmail
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, errors.New("mail boom"))
	req = httptest.NewRequest(http.MethodGet, "/recovery/tok", nil)
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ConfirmRecoveryEmail, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// erros de BuildMePage em renderMePasswordForm / renderMeRecoveryEmailForm
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, nil)
	uph = NewUserPersonalHandler(h.render, session, h.account, h.mail, audit.NewRecorder(nil))
	users.accountCalls = 0
	users.accountErrAfter = 1
	users.accountErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"bad"}, "password": {"password-abcdef"}, "password_confirm": {"password-abcdef"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountCalls = 0
	users.accountErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"bad"}, "password": {"password-abcdef"}, "password_confirm": {"password-abcdef"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	users.accountCalls = 0
	users.accountErrAfter = 2
	users.accountErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"bad"}, "recovery_email": {"x@example.com"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountCalls = 0
	users.accountErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"bad"}, "recovery_email": {"x@example.com"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErrAfter = 0
	users.accountErr = nil

	// bcrypt rejeita senhas com mais de 72 bytes; o formulário permite até 128, então esse
	// intervalo é um erro real de HashPassword alcançável com uma senha genuína, sem mock.
	tooLongForBcrypt := strings.Repeat("a", 100)
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"hashfail@example.com"},
		"password": {tooLongForBcrypt}, "password_confirm": {tooLongForBcrypt},
	})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"ok@example.com"},
		"password": {""}, "password_confirm": {""},
	})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {""},
		"password": {"password-12345"}, "password_confirm": {"password-12345"},
	})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = serveUser(t, session, 0, nil, h.ResendEmail, formReq(http.MethodPost, "/user/resend", captchaValues(url.Values{"email": {"bad"}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	bad := httptest.NewRequest(http.MethodPost, "/user/resend", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 0, nil, h.ResendEmail, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	users.accountErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"1"}, "first_name": {""}, "last_name": {""}, "phone_number": {"x"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestNewHandlersAndCaptchaHelpers verifica a criação dos handlers e os caminhos de validateCaptcha/newAuthCaptcha.
func TestNewHandlersAndCaptchaHelpers(t *testing.T) {
	installPassingCaptcha(t)
	h, session, _, tf := newTestUserHandler(t, nil, nil, nil, nil, nil)
	assert.NotNil(t, NewUserHandler(h.render, session, h.mail, tf, h.account, h.audit))
	assert.NotNil(t, NewUserPersonalHandler(h.render, session, h.account, h.mail, h.audit))
	assert.NotNil(t, NewTwoFactorHandler(h.render, session, tf, h.audit))

	form := &userdtoStub{}
	req := formReq(http.MethodPost, "/", url.Values{})
	require.NoError(t, req.ParseForm())
	assert.False(t, h.validateCaptcha(req, form))
	req = formReq(http.MethodPost, "/", url.Values{"captcha_checked": {"1"}})
	require.NoError(t, req.ParseForm())
	assert.False(t, h.validateCaptcha(req, form))
	req = formReq(http.MethodPost, "/", captchaValues(nil))
	require.NoError(t, req.ParseForm())
	assert.True(t, h.validateCaptcha(req, form))

	orig := validateCaptchaFn
	validateCaptchaFn = func(string, string) bool { return false }
	assert.False(t, h.validateCaptcha(req, form))
	validateCaptchaFn = orig
}

type userdtoStub struct{ n int }

func (u *userdtoStub) AddFieldError(string, string) { u.n++ }

// TestAuthSigninSignoutTwoFactor verifica fluxos de signin/signout e redirecionamento para login 2FA.
func TestAuthSigninSignoutTwoFactor(t *testing.T) {
	hash := mustHash(t, "password-12345")
	users := &fakeHandlerUserRepo{
		user:     activeUser(hash),
		personal: &usermodel.UserPersonalInformation{FirstName: txt("Ada")},
		account:  &usermodel.User{Id: num(1), Email: txt("user@example.com"), Active: pgtype.Bool{Bool: true, Valid: true}},
	}
	h, session, _, _ := newTestUserHandler(t, users, nil, nil, nil, nil)

	rec := serveUser(t, session, 0, nil, h.SigninForm, httptest.NewRequest(http.MethodGet, "/user/signin", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodGet, "/user/signin", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	badBody := httptest.NewRequest(http.MethodPost, "/user/signin", &errReader{})
	badBody.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 0, nil, h.Signin, badBody)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {""}, "password": {""}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"bad"}, "password": {"x"}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// força o caminho em que o captcha é obrigatório
	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, signinCaptchaFailuresKey, int64(3))
	}, h.Signin, formReq(http.MethodPost, "/user/signin", url.Values{"email": {"user@example.com"}, "password": {"password-12345"}}))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	users.findByUserErr = errors.New("missing")
	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.findByUserErr = nil

	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"wrong-password-1"}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	users.user.Active = pgtype.Bool{Bool: false, Valid: true}
	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	users.user.Active = pgtype.Bool{Bool: true, Valid: true}

	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	// caminho de login com 2FA
	users.user.TwoFactorEnabled = pgtype.Bool{Bool: true, Valid: true}
	tf := &fakeHandlerTwoFactorRepo{status: &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true, Enabled: true, Method: usermodel.TwoFactorMethodEmail}}
	h, session, _, twoFactor := newTestUserHandler(t, users, nil, nil, tf, nil)
	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/user/twofactor", rec.Header().Get("Location"))

	rec = serveUser(t, session, 0, nil, h.TwoFactorLoginForm, httptest.NewRequest(http.MethodGet, "/user/twofactor", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, pendingTwoFactorUserIDKey, int64(1))
		session.Put(ctx, pendingTwoFactorMethodKey, usermodel.TwoFactorMethodEmail)
		session.Put(ctx, pendingTwoFactorEmailKey, "user@example.com")
		session.Put(ctx, pendingTwoFactorFirstNameKey, "Ada")
	}, h.TwoFactorLoginForm, httptest.NewRequest(http.MethodGet, "/user/twofactor", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = serveUser(t, session, 0, nil, h.TwoFactorLoginVerify, formReq(http.MethodGet, "/user/twofactor", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	h.twoFactor = nil
	rec = serveUser(t, session, 0, nil, h.TwoFactorLoginVerify, formReq(http.MethodPost, "/user/twofactor", captchaValues(nil)))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	h.twoFactor = twoFactor

	rec = serveUser(t, session, 0, nil, h.TwoFactorLoginVerify, formReq(http.MethodPost, "/user/twofactor", captchaValues(url.Values{"code": {"123456"}})))
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, pendingTwoFactorUserIDKey, int64(1))
		session.Put(ctx, pendingTwoFactorMethodKey, usermodel.TwoFactorMethodEmail)
		session.Put(ctx, twoFactorCaptchaFailuresKey, int64(3))
	}, h.TwoFactorLoginVerify, formReq(http.MethodPost, "/user/twofactor", url.Values{"code": {"123456"}}))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, pendingTwoFactorUserIDKey, int64(1))
		session.Put(ctx, pendingTwoFactorMethodKey, usermodel.TwoFactorMethodEmail)
	}, h.TwoFactorLoginVerify, formReq(http.MethodPost, "/user/twofactor", captchaValues(url.Values{"code": {""}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// falha na verificação de login / limpeza de expirado
	tf.challengeErr = apperrors.ErrTwoFactorChallengeExpired
	// VerifyLogin falha sem challenge — usa o caminho de código inválido
	tf.challengeErr = nil
	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, pendingTwoFactorUserIDKey, int64(1))
		session.Put(ctx, pendingTwoFactorMethodKey, usermodel.TwoFactorMethodEmail)
		session.Put(ctx, pendingTwoFactorEmailKey, "user@example.com")
	}, h.TwoFactorLoginVerify, formReq(http.MethodPost, "/user/twofactor", captchaValues(url.Values{"code": {"000000"}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// logout
	sessions := &fakeHandlerSessionRepo{revokeErr: errors.New("revoke")}
	h, session, _, _ = newTestUserHandler(t, users, sessions, nil, nil, nil)
	rec = serveUser(t, session, 1, nil, h.Signout, formReq(http.MethodGet, "/user/signout", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 1, nil, h.Signout, formReq(http.MethodPost, "/user/signout", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	// erro ao criar sessão em completeSignin
	sessions.createErr = errors.New("create sess")
	h, session, _, _ = newTestUserHandler(t, users, sessions, nil, nil, nil)
	users.user.TwoFactorEnabled = pgtype.Bool{Bool: false, Valid: true}
	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read fail") }

// TestSignupResendConfirm verifica cadastro, reenvio de confirmação e confirmação de e-mail/recuperação.
func TestSignupResendConfirm(t *testing.T) {
	users := &fakeHandlerUserRepo{account: &usermodel.User{Id: num(1), Email: txt("user@example.com")}}
	h, session, _, _ := newTestUserHandler(t, users, nil, nil, nil, nil)

	rec := serveUser(t, session, 0, nil, h.SignupForm, httptest.NewRequest(http.MethodGet, "/user/signup", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodGet, "/user/signup", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	badBody := httptest.NewRequest(http.MethodPost, "/user/signup", &errReader{})
	badBody.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 0, nil, h.Signup, badBody)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {""}, "last_name": {""}, "email": {"bad"}, "password": {"short"}, "password_confirm": {"other"},
	})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	users.findByEmail = true
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"new@example.com"},
		"password": {"password-12345"}, "password_confirm": {"password-12345"},
	})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.findByEmail = false
	users.findByEmailErr = errors.New("db")
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"new@example.com"},
		"password": {"password-12345"}, "password_confirm": {"password-12345"},
	})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.findByEmailErr = nil

	longPass := strings.Repeat("a", 129)
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"new@example.com"},
		"password": {longPass}, "password_confirm": {longPass},
	})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// cadastro com sucesso
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"new@example.com"},
		"password": {"password-12345"}, "password_confirm": {"password-12345"},
	})))
	assert.Equal(t, http.StatusOK, rec.Code)

	users.createErr = errors.New("create")
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"new2@example.com"},
		"password": {"password-12345"}, "password_confirm": {"password-12345"},
	})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.createErr = nil

	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, errors.New("mail"))
	rec = serveUser(t, session, 0, nil, h.Signup, formReq(http.MethodPost, "/user/signup", captchaValues(url.Values{
		"first_name": {"Ada"}, "last_name": {"Lovelace"}, "email": {"new3@example.com"},
		"password": {"password-12345"}, "password_confirm": {"password-12345"},
	})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// reenvio
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, nil)
	rec = serveUser(t, session, 0, nil, h.ResendEmailForm, httptest.NewRequest(http.MethodGet, "/user/resend", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 0, nil, h.ResendEmail, formReq(http.MethodGet, "/user/resend", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 0, nil, h.ResendEmail, formReq(http.MethodPost, "/user/resend", captchaValues(url.Values{"email": {""}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.resendToken = "rtok"
	users.resendRecipients = []string{"user@example.com"}
	rec = serveUser(t, session, 0, nil, h.ResendEmail, formReq(http.MethodPost, "/user/resend", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusOK, rec.Code)
	users.resendErr = errors.New("resend")
	rec = serveUser(t, session, 0, nil, h.ResendEmail, formReq(http.MethodPost, "/user/resend", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.resendErr = nil
	users.resendToken = ""
	rec = serveUser(t, session, 0, nil, h.ResendEmail, formReq(http.MethodPost, "/user/resend", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusOK, rec.Code)

	// confirmação
	req := httptest.NewRequest(http.MethodGet, "/confirmation/", nil)
	req.SetPathValue("token", "")
	rec = serveUser(t, session, 0, nil, h.Confirm, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.confirmErr = errors.New("bad")
	req = httptest.NewRequest(http.MethodGet, "/confirmation/t", nil)
	req.SetPathValue("token", "t")
	rec = serveUser(t, session, 0, nil, h.Confirm, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.confirmErr = nil
	users.confirmEmail = "user@example.com"
	users.confirmUserID = 1
	rec = serveUser(t, session, 0, nil, h.Confirm, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// confirmação de recuperação
	req = httptest.NewRequest(http.MethodGet, "/recovery/", nil)
	req.SetPathValue("token", "")
	rec = serveUser(t, session, 0, nil, h.ConfirmRecoveryEmail, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.recoveryErr = errors.New("bad")
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ConfirmRecoveryEmail, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.recoveryErr = nil
	users.recoveryPrimary = "user@example.com"
	users.recoveryEmail = "rec@example.com"
	users.recoveryUserID = 1
	rec = serveUser(t, session, 0, nil, h.ConfirmRecoveryEmail, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 1, nil, h.ConfirmRecoveryEmail, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

// TestPasswordAndAccountLifecycle verifica esqueci senha, reset, exportação e exclusão de conta.
func TestPasswordAndAccountLifecycle(t *testing.T) {
	users := &fakeHandlerUserRepo{
		account:           &usermodel.User{Id: num(1), Email: txt("user@example.com")},
		resetToken:        "rt",
		resetRecipients:   []string{"user@example.com"},
		putPasswordEmail:  "user@example.com",
		putPasswordUserID: 1,
		passwordHash:      mustHash(t, "password-12345"),
		passwordActive:    true,
		exportData:        &usermodel.AccountExport{},
		attachKeys:        []string{"k1"},
	}
	h, session, accountSvc, _ := newTestUserHandler(t, users, nil, nil, nil, nil)

	rec := serveUser(t, session, 0, nil, h.ForgetPasswordForm, httptest.NewRequest(http.MethodGet, "/user/forgetpassword", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 0, nil, h.ForgetPassword, formReq(http.MethodGet, "/user/forgetpassword", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 0, nil, h.ForgetPassword, formReq(http.MethodPost, "/user/forgetpassword", captchaValues(url.Values{"email": {""}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = serveUser(t, session, 0, nil, h.ForgetPassword, formReq(http.MethodPost, "/user/forgetpassword", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusOK, rec.Code)
	users.resetErr = apperrors.ErrUserInactive
	users.resetToken = ""
	rec = serveUser(t, session, 0, nil, h.ForgetPassword, formReq(http.MethodPost, "/user/forgetpassword", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusOK, rec.Code)
	users.resetErr = errors.New("db")
	rec = serveUser(t, session, 0, nil, h.ForgetPassword, formReq(http.MethodPost, "/user/forgetpassword", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.resetErr = nil
	users.resetToken = "rt"

	req := httptest.NewRequest(http.MethodGet, "/user/password/", nil)
	req.SetPathValue("token", "")
	rec = serveUser(t, session, 0, nil, h.ResetPasswordForm, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPasswordForm, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	users.getTokenErr = pgx.ErrNoRows
	rec = serveUser(t, session, 0, nil, h.ResetPasswordForm, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.getTokenErr = errors.New("db")
	rec = serveUser(t, session, 0, nil, h.ResetPasswordForm, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.getTokenErr = nil

	req = formReq(http.MethodPost, "/user/password/tok", url.Values{"password": {"short"}, "password_confirm": {"short"}})
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	req = formReq(http.MethodPost, "/user/password/", url.Values{"password": {"password-12345"}, "password_confirm": {"password-12345"}})
	req.SetPathValue("token", "")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	req = formReq(http.MethodPost, "/user/password/tok", url.Values{"password": {"password-12345"}, "password_confirm": {"password-12345"}})
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	users.putPasswordErr = errors.New("put")
	req = formReq(http.MethodPost, "/user/password/tok", url.Values{"password": {"password-12345"}, "password_confirm": {"password-12345"}})
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	users.putPasswordErr = nil

	rec = serveUser(t, session, 1, nil, h.ExportAccountData, httptest.NewRequest(http.MethodGet, "/me/export", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	users.exportErr = errors.New("export")
	rec = serveUser(t, session, 1, nil, h.ExportAccountData, httptest.NewRequest(http.MethodGet, "/me/export", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.exportErr = nil

	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodGet, "/me/delete", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{"account_delete_confirmation": {"no"}, "password": {""}}))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{"account_delete_confirmation": {"excluir conta"}, "password": {"wrong-password-xx"}}))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	store := &attachStore{deleteErr: errors.New("disk")}
	accountSvc.WithAttachmentStorage(store)
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{"account_delete_confirmation": {"excluir conta"}, "password": {"password-12345"}}))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

// TestPersonalHandlerCoverage verifica páginas Me/Security e gravação de dados pessoais no handler.
func TestPersonalHandlerCoverage(t *testing.T) {
	users := &fakeHandlerUserRepo{
		account:        &usermodel.User{Id: num(1), Email: txt("user@example.com"), Active: pgtype.Bool{Bool: true, Valid: true}},
		passwordHash:   mustHash(t, "password-12345"),
		passwordActive: true,
		beginPrimary:   "user@example.com",
		beginToken:     "rtok",
	}
	personal := &fakeHandlerPersonalRepo{info: &usermodel.UserPersonalInformation{Id: num(1), FirstName: txt("Ada"), LastName: txt("Lovelace")}}
	sessions := &fakeHandlerSessionRepo{sessions: []usermodel.UserSession{{SessionID: "sess-1"}}}
	h, session, account, _ := newTestUserHandler(t, users, sessions, personal, nil, nil)
	uph := NewUserPersonalHandler(h.render, session, account, h.mail, h.audit)

	rec := serveUser(t, session, 1, nil, uph.Me, httptest.NewRequest(http.MethodGet, "/me", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.MePersonalInformation, httptest.NewRequest(http.MethodGet, "/me/overview/personal-information", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.MeAccountSettings, httptest.NewRequest(http.MethodGet, "/me/overview/account-settings", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.MePrivacyData, httptest.NewRequest(http.MethodGet, "/me/overview/privacy-data", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	users.accountErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.MePersonalInformation, httptest.NewRequest(http.MethodGet, "/me/overview/personal-information", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.MePersonalInformation, httptest.NewRequest(http.MethodGet, "/me/overview/personal-information", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = nil

	rec = serveUser(t, session, 1, nil, uph.Security, httptest.NewRequest(http.MethodGet, "/me/security", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.SecurityTwoStepVerification, httptest.NewRequest(http.MethodGet, "/me/security/two-step-verification", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.ActiveSessions, httptest.NewRequest(http.MethodGet, "/me/security/active-sessions?sessions=20", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	req := httptest.NewRequest(http.MethodGet, "/me/security/two-factor/email", nil)
	req.SetPathValue("method", usermodel.TwoFactorMethodEmail)
	rec = serveUser(t, session, 1, nil, uph.SecurityTwoFactorMethod, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	req.SetPathValue("method", "bad")
	rec = serveUser(t, session, 1, nil, uph.SecurityTwoFactorMethod, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = serveUser(t, session, 1, nil, uph.RevokeOtherSessions, formReq(http.MethodGet, "/me/security/sessions/revoke-others", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 1, func(ctx context.Context) {
		session.Put(ctx, "userSessionId", "")
	}, uph.RevokeOtherSessions, formReq(http.MethodPost, "/me/security/sessions/revoke-others", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.RevokeOtherSessions, formReq(http.MethodPost, "/me/security/sessions/revoke-others", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	sessions.revokeErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.RevokeOtherSessions, formReq(http.MethodPost, "/me/security/sessions/revoke-others", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessions.revokeErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.RevokeOtherSessions, formReq(http.MethodPost, "/me/security/sessions/revoke-others", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessions.revokeErr = nil

	req = formReq(http.MethodPost, "/me/security/sessions/", nil)
	req.SetPathValue("sessionID", "")
	rec = serveUser(t, session, 1, nil, uph.RevokeSession, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	req = formReq(http.MethodPost, "/me/security/sessions/other", nil)
	req.SetPathValue("sessionID", "other")
	rec = serveUser(t, session, 1, nil, uph.RevokeSession, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	req = formReq(http.MethodPost, "/me/security/sessions/sess-1", nil)
	req.SetPathValue("sessionID", "sess-1")
	rec = serveUser(t, session, 1, nil, uph.RevokeSession, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"password-12345"}, "password": {"password-67890"}, "password_confirm": {"password-67890"},
	}))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"bad"}, "password": {"password-67890"}, "password_confirm": {"password-67890"},
	}))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"password-12345"}, "recovery_email": {"rec@example.com"},
	}))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"bad"}, "recovery_email": {"rec@example.com"},
	}))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"1"}, "first_name": {"Ada"}, "last_name": {"Lovelace"}, "phone_number": {"11999999999"},
	}))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"1"}, "first_name": {""}, "last_name": {""}, "phone_number": {"xxx"},
	}))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	// criação de dados pessoais (sem id / info vazia)
	personal.info = nil
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"0"}, "first_name": {"Ada"}, "last_name": {"Lovelace"}, "phone_number": {"11999999999"},
	}))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

// TestTwoFactorHandlerMethods verifica métodos HTTP do TwoFactorHandler (start/verify/disable/recovery) e erros.
func TestTwoFactorHandlerMethods(t *testing.T) {
	tfRepo := &fakeHandlerTwoFactorRepo{status: &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}}
	h, session, _, twoFactor := newTestUserHandler(t, nil, nil, nil, tfRepo, nil)
	tfh := NewTwoFactorHandler(h.render, session, twoFactor, audit.NewRecorder(nil))

	for _, fn := range []func(http.ResponseWriter, *http.Request) error{tfh.Start, tfh.Verify, tfh.RegenerateRecoveryCodes, tfh.DisableStart, tfh.DisableVerify} {
		rec := serveUser(t, session, 1, nil, fn, formReq(http.MethodGet, "/api/2fa", nil))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	}

	rec := serveUser(t, session, 1, nil, tfh.Start, formReq(http.MethodPost, "/api/2fa/start", url.Values{"method": {usermodel.TwoFactorMethodEmail}}))
	assert.Equal(t, http.StatusOK, rec.Code)

	tfRepo.status.Enabled = true
	tfRepo.status.Method = usermodel.TwoFactorMethodEmail
	rec = serveUser(t, session, 1, nil, tfh.Start, formReq(http.MethodPost, "/api/2fa/start", url.Values{"method": {usermodel.TwoFactorMethodEmail}}))
	assert.Equal(t, http.StatusConflict, rec.Code)

	rec = serveUser(t, session, 1, nil, tfh.DisableStart, formReq(http.MethodPost, "/api/2fa/disable/start", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = serveUser(t, session, 1, nil, tfh.RegenerateRecoveryCodes, formReq(http.MethodPost, "/api/2fa/recovery", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	badBody := httptest.NewRequest(http.MethodPost, "/api/2fa/verify", &errReader{})
	badBody.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 1, nil, tfh.Verify, badBody)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveUser(t, session, 1, nil, tfh.Verify, formReq(http.MethodPost, "/api/2fa/verify", url.Values{"method": {usermodel.TwoFactorMethodEmail}, "code": {"000000"}}))
	assert.Contains(t, []int{http.StatusUnprocessableEntity, http.StatusInternalServerError, http.StatusConflict, http.StatusForbidden}, rec.Code)

	rec = serveUser(t, session, 1, nil, tfh.DisableVerify, formReq(http.MethodPost, "/api/2fa/disable/verify", url.Values{"code": {"000000"}}))
	assert.Contains(t, []int{http.StatusUnprocessableEntity, http.StatusInternalServerError, http.StatusConflict, http.StatusForbidden}, rec.Code)

	// falha na busca de status em writeError
	tfRepo.err = errors.New("status boom")
	rec = serveUser(t, session, 1, nil, tfh.Start, formReq(http.MethodPost, "/api/2fa/start", url.Values{"method": {usermodel.TwoFactorMethodEmail}}))
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

// silencia import não usado se render for usado só via helpers
var _ = render.NewRender
var _ = scs.New
var _ = time.Minute

// TestTwoFactorVerifyDisableSuccessPaths verifica caminhos de sucesso de ativação/verificação e desativação do 2FA via handler.
func TestTwoFactorVerifyDisableSuccessPaths(t *testing.T) {
	tfRepo := &fakeHandlerTwoFactorRepo{status: &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}}
	renderer := &twoFactorMailRenderer{}
	h, session, twoFactor := newTestUserHandlerWithRenderer(t, nil, nil, nil, tfRepo, nil, renderer)
	tfh := NewTwoFactorHandler(h.render, session, twoFactor, audit.NewRecorder(nil))

	rec := serveUser(t, session, 1, nil, tfh.Start, formReq(http.MethodPost, "/api/2fa/start", url.Values{"method": {usermodel.TwoFactorMethodEmail}}))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, renderer.lastCode)

	rec = serveUser(t, session, 1, nil, tfh.Verify, formReq(http.MethodPost, "/api/2fa/verify", url.Values{
		"method": {usermodel.TwoFactorMethodEmail}, "code": {renderer.lastCode},
	}))
	assert.Equal(t, http.StatusOK, rec.Code)

	// fluxo de desativação
	tfRepo.status.Enabled = true
	tfRepo.status.Method = usermodel.TwoFactorMethodEmail
	tfRepo.challenge = nil
	rec = serveUser(t, session, 1, nil, tfh.DisableStart, formReq(http.MethodPost, "/api/2fa/disable/start", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, renderer.lastCode)
	rec = serveUser(t, session, 1, nil, tfh.DisableVerify, formReq(http.MethodPost, "/api/2fa/disable/verify", url.Values{"code": {renderer.lastCode}}))
	assert.Equal(t, http.StatusOK, rec.Code)

	// erro de parse em Start
	bad := httptest.NewRequest(http.MethodPost, "/api/2fa/start", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 1, nil, tfh.Start, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	bad = httptest.NewRequest(http.MethodPost, "/api/2fa/disable/verify", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 1, nil, tfh.DisableVerify, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// falha de GenerateRecoveryCodes com 2FA desativado
	tfRepo.status.Enabled = false
	rec = serveUser(t, session, 1, nil, tfh.RegenerateRecoveryCodes, formReq(http.MethodPost, "/api/2fa/recovery", nil))
	assert.NotEqual(t, http.StatusOK, rec.Code)

	rec = serveUser(t, session, 1, nil, tfh.DisableStart, formReq(http.MethodPost, "/api/2fa/disable/start", nil))
	assert.NotEqual(t, http.StatusOK, rec.Code)

	// falha de Status após Start bem-sucedido (segunda chamada a GetStatus)
	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	tfRepo.challenge = nil
	tfRepo.errAfter = 2
	tfRepo.getCalls = 0
	rec = serveUser(t, session, 1, nil, tfh.Start, formReq(http.MethodPost, "/api/2fa/start", url.Values{"method": {usermodel.TwoFactorMethodEmail}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	tfRepo.errAfter = 0

	// falha de Status após Verify / DisableVerify / Regenerate
	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true}
	tfRepo.challenge = nil
	rec = serveUser(t, session, 1, nil, tfh.Start, formReq(http.MethodPost, "/api/2fa/start", url.Values{"method": {usermodel.TwoFactorMethodEmail}}))
	require.Equal(t, http.StatusOK, rec.Code)
	tfRepo.getCalls = 0
	tfRepo.errAfter = 2
	rec = serveUser(t, session, 1, nil, tfh.Verify, formReq(http.MethodPost, "/api/2fa/verify", url.Values{
		"method": {usermodel.TwoFactorMethodEmail}, "code": {renderer.lastCode},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	tfRepo.errAfter = 0

	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true, Enabled: true, Method: usermodel.TwoFactorMethodEmail}
	tfRepo.challenge = nil
	tfRepo.getCalls = 0
	tfRepo.errAfter = 2
	rec = serveUser(t, session, 1, nil, tfh.DisableStart, formReq(http.MethodPost, "/api/2fa/disable/start", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	tfRepo.errAfter = 0

	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true, Enabled: true, Method: usermodel.TwoFactorMethodEmail}
	tfRepo.challenge = nil
	rec = serveUser(t, session, 1, nil, tfh.DisableStart, formReq(http.MethodPost, "/api/2fa/disable/start", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	tfRepo.getCalls = 0
	// VerifyDisable: GetStatus + GetStatus em disableWithoutVerification + Status do handler
	tfRepo.errAfter = 3
	rec = serveUser(t, session, 1, nil, tfh.DisableVerify, formReq(http.MethodPost, "/api/2fa/disable/verify", url.Values{"code": {renderer.lastCode}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	tfRepo.errAfter = 0

	tfRepo.status = &usermodel.TwoFactorStatus{UserID: 1, Email: "user@example.com", Active: true, Enabled: true, Method: usermodel.TwoFactorMethodEmail}
	tfRepo.getCalls = 0
	tfRepo.errAfter = 2
	rec = serveUser(t, session, 1, nil, tfh.RegenerateRecoveryCodes, formReq(http.MethodPost, "/api/2fa/recovery", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	tfRepo.errAfter = 0
}

// TestAuthTwoFactorLoginSuccessAndErrors verifica login 2FA bem-sucedido e ramos de erro no fluxo autenticado.
func TestAuthTwoFactorLoginSuccessAndErrors(t *testing.T) {
	hash := mustHash(t, "password-12345")
	users := &fakeHandlerUserRepo{
		user:     activeUser(hash),
		personal: &usermodel.UserPersonalInformation{FirstName: txt("Ada")},
		account:  &usermodel.User{Id: num(1), Email: txt("user@example.com"), Active: pgtype.Bool{Bool: true, Valid: true}},
	}
	users.user.TwoFactorEnabled = pgtype.Bool{Bool: true, Valid: true}
	tfRepo := &fakeHandlerTwoFactorRepo{status: &usermodel.TwoFactorStatus{
		UserID: 1, Email: "user@example.com", Active: true, Enabled: true, Method: usermodel.TwoFactorMethodEmail,
	}}
	renderer := &twoFactorMailRenderer{}
	h, session, twoFactor := newTestUserHandlerWithRenderer(t, users, nil, nil, tfRepo, nil, renderer)

	// BeginLogin falha
	tfRepo.err = errors.New("begin fail")
	rec := serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	tfRepo.err = nil

	rec = serveUser(t, session, 0, nil, h.Signin, formReq(http.MethodPost, "/user/signin", captchaValues(url.Values{"email": {"user@example.com"}, "password": {"password-12345"}})))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.NotEmpty(t, renderer.lastCode)

	// login 2FA com verificação bem-sucedida
	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, pendingTwoFactorUserIDKey, int64(1))
		session.Put(ctx, pendingTwoFactorMethodKey, usermodel.TwoFactorMethodEmail)
		session.Put(ctx, pendingTwoFactorEmailKey, "user@example.com")
		session.Put(ctx, pendingTwoFactorFirstNameKey, "Ada")
	}, h.TwoFactorLoginVerify, formReq(http.MethodPost, "/user/twofactor", captchaValues(url.Values{"code": {renderer.lastCode}})))
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	// erro de ParseForm na verificação
	bad := httptest.NewRequest(http.MethodPost, "/user/twofactor", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 0, func(ctx context.Context) {
		session.Put(ctx, pendingTwoFactorUserIDKey, int64(1))
		session.Put(ctx, pendingTwoFactorMethodKey, usermodel.TwoFactorMethodEmail)
	}, h.TwoFactorLoginVerify, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	_ = twoFactor
}

// TestPasswordSignupTokenAndMailGaps verifica lacunas de token/e-mail em signup, reset e confirmação.
func TestPasswordSignupTokenAndMailGaps(t *testing.T) {
	users := &fakeHandlerUserRepo{
		account:           &usermodel.User{Id: num(1), Email: txt("user@example.com")},
		resetToken:        "rt",
		resetRecipients:   []string{"user@example.com"},
		putPasswordEmail:  "user@example.com",
		putPasswordUserID: 1,
		passwordHash:      mustHash(t, "password-12345"),
		passwordActive:    true,
		resendToken:       "rt",
		resendRecipients:  []string{"user@example.com"},
	}
	h, session, _, _ := newTestUserHandler(t, users, nil, nil, nil, nil)

	// e-mail inválido em esqueci a senha
	rec := serveUser(t, session, 0, nil, h.ForgetPassword, formReq(http.MethodPost, "/user/forgetpassword", captchaValues(url.Values{"email": {"bad"}})))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// falhas de render/envio de e-mail em esqueci a senha
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, errors.New("mail boom"))
	rec = serveUser(t, session, 0, nil, h.ForgetPassword, formReq(http.MethodPost, "/user/forgetpassword", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// ResetPassword: senhas divergentes / longa demais / falha no hash / aviso de e-mail
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, nil)
	req := formReq(http.MethodPost, "/user/password/tok", url.Values{"password": {"password-12345"}, "password_confirm": {"password-99999"}})
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	long := strings.Repeat("a", 129)
	req = formReq(http.MethodPost, "/user/password/tok", url.Values{"password": {long}, "password_confirm": {long}})
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// bcrypt rejeita senhas com mais de 72 bytes; o formulário permite até 128, então esse
	// intervalo é um erro real de HashPassword alcançável com uma senha genuína, sem mock.
	tooLongForBcrypt := strings.Repeat("a", 100)
	req = formReq(http.MethodPost, "/user/password/tok", url.Values{"password": {tooLongForBcrypt}, "password_confirm": {tooLongForBcrypt}})
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// falha de e-mail no reenvio
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, errors.New("mail"))
	rec = serveUser(t, session, 0, nil, h.ResendEmail, formReq(http.MethodPost, "/user/resend", captchaValues(url.Values{"email": {"user@example.com"}})))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// erro de parse em esqueci a senha
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, nil)
	bad := httptest.NewRequest(http.MethodPost, "/user/forgetpassword", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 0, nil, h.ForgetPassword, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// redefinição de senha com falha de e-mail ainda sucede (apenas aviso)
	h, session, _, _ = newTestUserHandler(t, users, nil, nil, nil, errors.New("mail"))
	req = formReq(http.MethodPost, "/user/password/tok", url.Values{"password": {"password-12345"}, "password_confirm": {"password-12345"}})
	req.SetPathValue("token", "tok")
	rec = serveUser(t, session, 0, nil, h.ResetPassword, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAccountLifecycleAndPersonalGaps verifica lacunas de ciclo de vida da conta e dados pessoais no handler.
func TestAccountLifecycleAndPersonalGaps(t *testing.T) {
	users := &fakeHandlerUserRepo{
		account:        &usermodel.User{Id: num(1), Email: txt("user@example.com"), Active: pgtype.Bool{Bool: true, Valid: true}},
		passwordHash:   mustHash(t, "password-12345"),
		passwordActive: true,
		attachKeys:     []string{"k1"},
		beginPrimary:   "user@example.com",
		beginToken:     "tok",
	}
	personal := &fakeHandlerPersonalRepo{info: &usermodel.UserPersonalInformation{Id: num(1), FirstName: txt("Ada"), LastName: txt("Lovelace")}}
	sessions := &fakeHandlerSessionRepo{}
	h, session, account, _ := newTestUserHandler(t, users, sessions, personal, nil, nil)
	uph := NewUserPersonalHandler(h.render, session, account, h.mail, audit.NewRecorder(nil))

	bad := httptest.NewRequest(http.MethodPost, "/me/delete", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := serveUser(t, session, 1, nil, h.DeleteAccount, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	users.passwordErr = errors.New("pwd")
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{
		"account_delete_confirmation": {"excluir conta"}, "password": {"password-12345"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.passwordErr = nil

	users.attachKeysErr = errors.New("keys")
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{
		"account_delete_confirmation": {"excluir conta"}, "password": {"password-12345"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.attachKeysErr = nil

	users.deleteErr = errors.New("del")
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{
		"account_delete_confirmation": {"excluir conta"}, "password": {"password-12345"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.deleteErr = nil

	// formulário de exclusão não encontrado
	users.accountErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{
		"account_delete_confirmation": {"no"}, "password": {""},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, h.DeleteAccount, formReq(http.MethodPost, "/me/delete", url.Values{
		"account_delete_confirmation": {"no"}, "password": {""},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = nil

	// lacunas de dados pessoais
	users.accountErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.SecurityTwoStepVerification, httptest.NewRequest(http.MethodGet, "/me/security/two-step-verification", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.ActiveSessions, httptest.NewRequest(http.MethodGet, "/me/security/active-sessions", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = nil

	req := httptest.NewRequest(http.MethodGet, "/me/security/two-factor/email", nil)
	req.SetPathValue("method", usermodel.TwoFactorMethodEmail)
	users.accountErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.SecurityTwoFactorMethod, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.SecurityTwoFactorMethod, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = nil

	rec = serveUser(t, session, 1, nil, uph.RevokeSession, formReq(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessions.revokeErr = apperrors.ErrNotFound
	req = formReq(http.MethodPost, "/me/security/sessions/x", nil)
	req.SetPathValue("sessionID", "x")
	rec = serveUser(t, session, 1, nil, uph.RevokeSession, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessions.revokeErr = errors.New("db")
	req = formReq(http.MethodPost, "/me/security/sessions/x", nil)
	req.SetPathValue("sessionID", "x")
	rec = serveUser(t, session, 1, nil, uph.RevokeSession, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	sessions.revokeErr = nil

	bad = httptest.NewRequest(http.MethodPost, "/me/password", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	users.passwordErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"password-12345"}, "password": {"password-67890"}, "password_confirm": {"password-67890"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.passwordErr = apperrors.ErrUserInactive
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"password-12345"}, "password": {"password-67890"}, "password_confirm": {"password-67890"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.passwordErr = errors.New("db")
	rec = serveUser(t, session, 1, nil, uph.PasswordSave, formReq(http.MethodPost, "/me/password", url.Values{
		"current_password": {"password-12345"}, "password": {"password-67890"}, "password_confirm": {"password-67890"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.passwordErr = nil

	bad = httptest.NewRequest(http.MethodPost, "/me/recovery", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	users.passwordErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"password-12345"}, "recovery_email": {"rec@example.com"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.passwordErr = apperrors.ErrUserInactive
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"password-12345"}, "recovery_email": {"rec@example.com"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.passwordErr = nil
	users.beginErr = errors.New("begin")
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"password-12345"}, "recovery_email": {"rec@example.com"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.beginErr = nil

	h, session, account, _ = newTestUserHandler(t, users, sessions, personal, nil, errors.New("mail"))
	uph = NewUserPersonalHandler(h.render, session, account, h.mail, audit.NewRecorder(nil))
	rec = serveUser(t, session, 1, nil, uph.RecoveryEmailSave, formReq(http.MethodPost, "/me/recovery", url.Values{
		"current_password": {"password-12345"}, "recovery_email": {"rec2@example.com"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	bad = httptest.NewRequest(http.MethodPost, "/me/personal", &errReader{})
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, bad)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	personal.updateErr = apperrors.ErrNotFound
	personal.info = &usermodel.UserPersonalInformation{Id: num(1), FirstName: txt("Ada"), LastName: txt("Lovelace")}
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"1"}, "first_name": {"Ada"}, "last_name": {"Lovelace"}, "phone_number": {"11999999999"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	personal.updateErr = errors.New("identificador de dados pessoais inválido")
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"1"}, "first_name": {"Ada"}, "last_name": {"Lovelace"}, "phone_number": {"11999999999"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	personal.updateErr = errors.New("other")
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"1"}, "first_name": {"Ada"}, "last_name": {"Lovelace"}, "phone_number": {"11999999999"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	personal.updateErr = nil

	// render do formulário não encontrado
	users.accountErr = apperrors.ErrNotFound
	rec = serveUser(t, session, 1, nil, uph.UserPersonalSave, formReq(http.MethodPost, "/me/personal", url.Values{
		"id": {"1"}, "first_name": {""}, "last_name": {""}, "phone_number": {"x"},
	}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	users.accountErr = nil
}
