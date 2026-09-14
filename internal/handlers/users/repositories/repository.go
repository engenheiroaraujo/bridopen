package repositories

import (
	"context"
	"errors"
	"strings"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	"github.com/engenheiroaraujo/bridopen/internal/platform/database"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/key"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	TokenPurposeEmailConfirmation         = "email_confirmation"
	TokenPurposePasswordReset             = "password_reset"
	TokenPurposeRecoveryEmailConfirmation = "recovery_email_confirmation"

	emailConfirmationTokenTTL         = 24 * time.Hour
	recoveryEmailConfirmationTokenTTL = 24 * time.Hour
	passwordResetTokenTTL             = 15 * time.Minute
)

// UserRepository concentra o acesso a dados de conta, autenticação, tokens e
// e-mail de recuperação. Os contratos consumidos pela camada de serviço são
// declarados lá, no pacote service: este pacote apenas os satisfaz.
type UserRepository struct {
	db database.Pool
}

func NewUserRepository(db database.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (ur *UserRepository) FindByEmail(ctx context.Context, email string) (bool, error) {
	var n int64

	query := `select count(*) from public.users where email = $1`

	row := ur.db.QueryRow(ctx, query, email)
	if err := row.Scan(&n); err != nil {
		return false, apperrors.NewRepositoryError(err)
	}
	return n > 0, nil
}

func (ur *UserRepository) FindByUser(ctx context.Context, email string) (*models.User, *models.UserPersonalInformation, error) {
	var user models.User
	var userpersonal models.UserPersonalInformation

	query := `
		select
			users.id
		,	users.email
		,	users.password
		,	users.active
		,	users.recovery_email
		,	users.two_factor_enabled
		,	users.two_factor_method
		,	users.two_factor_totp_secret_encrypted
		,	users_personal_information.first_name
		,	users_personal_information.last_name
		from
			public.users
			left join public.users_personal_information
				on users.id = users_personal_information.user_id
		where 1=1
			and users.email = $1;
	`

	row := ur.db.QueryRow(ctx, query, email)
	if err := row.Scan(
		&user.Id,
		&user.Email,
		&user.Password,
		&user.Active,
		&user.RecoveryEmail,
		&user.TwoFactorEnabled,
		&user.TwoFactorMethod,
		&user.TwoFactorTOTPSecretEncrypted,
		&userpersonal.FirstName,
		&userpersonal.LastName,
	); err != nil {
		return nil, nil, err
	}

	return &user, &userpersonal, nil
}

func (ur *UserRepository) FindPasswordAuthData(ctx context.Context, userID int64) (string, string, bool, error) {
	var email, passwordHash string
	var active bool

	query := `
		select
			users.email
		,	users.password
		,	users.active
		from
			public.users
		where 1=1
			and users.id = $1;
	`
	row := ur.db.QueryRow(ctx, query, userID)
	if err := row.Scan(&email, &passwordHash, &active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, apperrors.ErrNotFound
		}
		return "", "", false, apperrors.NewRepositoryError(err)
	}
	return email, passwordHash, active, nil
}

func (ur *UserRepository) Create(ctx context.Context, firstName, lastName, email, password, rawToken string) (*models.User, *models.UserPersonalInformation, string, error) {
	var user models.User
	var personal models.UserPersonalInformation

	personal.FirstName = pgtype.Text{String: firstName, Valid: true}
	personal.LastName = pgtype.Text{String: lastName, Valid: true}
	user.Email = pgtype.Text{String: email, Valid: true}
	user.Password = pgtype.Text{String: password, Valid: true}

	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return &user, &personal, "", apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	queryUser := "insert into public.users (email, password) values($1, $2) returning id, created_at;"

	row := tx.QueryRow(ctx, queryUser, user.Email, user.Password)
	if err := row.Scan(&user.Id, &user.CreatedAt); err != nil {
		return &user, &personal, "", apperrors.NewRepositoryError(err)
	}

	queryPersonal := `
		insert into public.users_personal_information (user_id, first_name, last_name) values ($1, $2, $3)
		returning user_id, first_name, last_name, created_at;
	`
	row = tx.QueryRow(ctx, queryPersonal, user.Id, personal.FirstName, personal.LastName)
	if err := row.Scan(&personal.UserId, &personal.FirstName, &personal.LastName, &personal.CreatedAt); err != nil {
		return &user, &personal, "", apperrors.NewRepositoryError(err)
	}

	if _, err := ur.createAuthToken(tx, ctx, &user, rawToken, TokenPurposeEmailConfirmation, emailConfirmationTokenTTL); err != nil {
		return &user, &personal, "", apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return &user, &personal, "", apperrors.NewRepositoryError(err)
	}

	return &user, &personal, rawToken, nil
}

func (ur *UserRepository) createAuthToken(tx pgx.Tx, ctx context.Context, user *models.User, rawToken, purpose string, ttl time.Duration) (*models.UserConfirmationToken, error) {
	tokenHash := key.HashToken(rawToken)
	expiresAt := time.Now().UTC().Add(ttl)

	if _, err := tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where user_id = $1
			and purpose = $2
			and confirmed = false;
	`, user.Id, purpose); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}

	var userToken models.UserConfirmationToken
	userToken.UserId = user.Id
	userToken.Token = pgtype.Text{String: tokenHash, Valid: true}
	userToken.TokenHash = pgtype.Text{String: tokenHash, Valid: true}
	userToken.Purpose = pgtype.Text{String: purpose, Valid: true}

	query := `
		insert into public.users_confirmation_tokens (user_id, token, token_hash, purpose, expires_at)
		values($1, $2, $2, $3, $4)
		returning id, created_at, expires_at;
	`

	row := tx.QueryRow(ctx, query, userToken.UserId, userToken.TokenHash, userToken.Purpose, expiresAt)
	if err := row.Scan(&userToken.Id, &userToken.CreatedAt, &userToken.ExpiresAt); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	return &userToken, nil
}

func (ur *UserRepository) CreateResetPasswordToken(ctx context.Context, email, rawToken string) (string, []string, error) {
	user, _, err := ur.FindByUser(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}

	if !user.Active.Valid || !user.Active.Bool {
		return "", nil, apperrors.ErrUserInactive
	}

	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", nil, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	if _, err := ur.createAuthToken(tx, ctx, user, rawToken, TokenPurposePasswordReset, passwordResetTokenTTL); err != nil {
		return "", nil, apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return "", nil, apperrors.NewRepositoryError(err)
	}

	recipients := []string{user.Email.String}
	if user.RecoveryEmail.Valid && strings.TrimSpace(user.RecoveryEmail.String) != "" && !strings.EqualFold(user.RecoveryEmail.String, user.Email.String) {
		recipients = append(recipients, user.RecoveryEmail.String)
	}
	return rawToken, recipients, nil
}

func (ur *UserRepository) CreateEmailConfirmationToken(ctx context.Context, email, rawToken string) (string, []string, error) {
	user, _, err := ur.FindByUser(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}

	if user.Active.Valid && user.Active.Bool {
		return "", nil, nil
	}

	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", nil, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	if _, err := ur.createAuthToken(tx, ctx, user, rawToken, TokenPurposeEmailConfirmation, emailConfirmationTokenTTL); err != nil {
		return "", nil, apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return "", nil, apperrors.NewRepositoryError(err)
	}

	return rawToken, []string{user.Email.String}, nil
}

func (ur *UserRepository) GetUserConfirmationByToken(ctx context.Context, token, purpose string) (*models.User, *models.UserConfirmationToken, string, error) {
	var user models.User
	var userToken models.UserConfirmationToken
	tokenHash := key.HashToken(token)

	query := `
		select
			users.id
		,	users.email
		,	users.password
		,	users.active
		,	users_confirmation_tokens.id
		,	users_confirmation_tokens.user_id
		,	users_confirmation_tokens.token_hash
		,	users_confirmation_tokens.purpose
		,	users_confirmation_tokens.confirmed
		,	users_confirmation_tokens.created_at
		,	users_confirmation_tokens.updated_at
		,	users_confirmation_tokens.expires_at
		from
			public.users
			inner join public.users_confirmation_tokens
				on users.id = users_confirmation_tokens.user_id
		where 1=1
			and users_confirmation_tokens.token_hash = $1
			and users_confirmation_tokens.purpose = $2
			and users_confirmation_tokens.confirmed = false
			and users_confirmation_tokens.expires_at > current_timestamp
	`
	row := ur.db.QueryRow(ctx, query, tokenHash, purpose)
	if err := row.Scan(
		&user.Id,
		&user.Email,
		&user.Password,
		&user.Active,
		&userToken.Id,
		&userToken.UserId,
		&userToken.TokenHash,
		&userToken.Purpose,
		&userToken.Confirmed,
		&userToken.CreatedAt,
		&userToken.UpdatedAt,
		&userToken.ExpiresAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, "", err
		}
		return nil, nil, "", apperrors.NewRepositoryError(err)
	}
	email := ""
	if user.Email.Valid {
		email = user.Email.String
	}
	return &user, &userToken, email, nil
}

func (ur *UserRepository) PutPasswordByToken(ctx context.Context, newPassword, token string) (string, int64, error) {
	tokenHash := key.HashToken(token)

	query := `
		select
			users.id
		,	users.email
		,	users_confirmation_tokens.id
		from
			public.users
			inner join public.users_confirmation_tokens
				on users.id = users_confirmation_tokens.user_id
		where 1=1
			and users_confirmation_tokens.confirmed = false
			and users_confirmation_tokens.token_hash = $1
			and users_confirmation_tokens.purpose = $2
			and users_confirmation_tokens.expires_at > current_timestamp;
	`
	row := ur.db.QueryRow(ctx, query, tokenHash, TokenPurposePasswordReset)
	var userID, tokenID int64
	var email string
	err := row.Scan(&userID, &email, &tokenID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, apperrors.ErrInvalidTokenOrUserAlreadyConfirmed
		}
		return "", 0, apperrors.NewRepositoryError(err)
	}

	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where id = $1;
	`, tokenID)
	if err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}

	_, err = tx.Exec(ctx, `
		update public.users
		set
			password = $1
		,	updated_at = now()
		where id = $2
	`, newPassword, userID)
	if err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}

	_, err = tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where user_id = $1
			and purpose = $2
			and confirmed = false;
	`, userID, TokenPurposePasswordReset)
	if err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}
	return email, userID, nil
}

func (ur *UserRepository) ConfirmUserByToken(ctx context.Context, token string) (email string, userID int64, err error) {
	tokenHash := key.HashToken(token)

	query := `
		select
			users.id
		,	users.email
		,	users_confirmation_tokens.id
		from
			public.users
			inner join public.users_confirmation_tokens
				on users.id = users_confirmation_tokens.user_id
		where 1=1
			and users.active = false
			and users_confirmation_tokens.confirmed = false
			and users_confirmation_tokens.token_hash = $1
			and users_confirmation_tokens.purpose = $2
			and users_confirmation_tokens.expires_at > current_timestamp;
	`
	var tokenID int64
	row := ur.db.QueryRow(ctx, query, tokenHash, TokenPurposeEmailConfirmation)
	if err = row.Scan(&userID, &email, &tokenID); err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}

	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		update public.users
		set
			active = true
		,	updated_at = now()
		where id = $1;
	`, userID)
	if err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}

	_, err = tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where id = $1;
	`, tokenID)
	if err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return "", 0, apperrors.NewRepositoryError(err)
	}
	return email, userID, nil
}

func (ur *UserRepository) DeleteExpiredTokens(ctx context.Context) error {
	_, err := ur.db.Exec(ctx, `
		delete from public.users_confirmation_tokens
		where expires_at < current_timestamp
			or (
				confirmed = true
				and coalesce(updated_at, created_at) < current_timestamp - interval '1 day'
			);
	`)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (ur *UserRepository) UpdatePasswordByID(ctx context.Context, userID int64, newPassword string) error {
	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		update public.users
		set
			password = $1
		,	updated_at = now()
		where id = $2;
	`, newPassword, userID)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	if _, err = tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where user_id = $1
			and purpose = $2
			and confirmed = false;
	`, userID, TokenPurposePasswordReset); err != nil {
		return apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (ur *UserRepository) ExportUserData(ctx context.Context, userID int64) (*models.AccountExport, error) {
	data := &models.AccountExport{}

	var userCreatedAt, userUpdatedAt pgtype.Timestamp

	query := `
		select
			users.id
		,	coalesce(
				nullif(
					trim(
						concat_ws(
							' '
						,	users_personal_information.first_name
						,	users_personal_information.last_name
						)
					), ''
				),''
			) as name
		,	users.email
		,	coalesce(nullif(users_personal_information.phone_number, ''), '') as phone_number
		,	users.active
		,	users.two_factor_enabled
		,	coalesce(two_factor_method, '')
		,	users.created_at
		,	users.updated_at
		from
			public.users
			left join public.users_personal_information
				on users.id = users_personal_information.user_id
		where 1=1
			and users.id = $1
	`

	row := ur.db.QueryRow(ctx, query, userID)
	if err := row.Scan(
		&data.User.ID,
		&data.User.Name,
		&data.User.Email,
		&data.User.PhoneNumber,
		&data.User.Active,
		&data.User.TwoFactorEnabled,
		&data.User.TwoFactorMethod,
		&userCreatedAt,
		&userUpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	data.User.CreatedAt = timePtr(userCreatedAt)
	data.User.UpdatedAt = timePtr(userUpdatedAt)

	rows, err := ur.db.Query(ctx, `
		select id, title, content, color, created_at, updated_at
		from public.notes
		where user_id = $1
		order by id;
	`, userID)
	if err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var note models.AccountExportNote
		var content pgtype.Text
		var noteCreatedAt, noteUpdatedAt pgtype.Timestamp
		if err := rows.Scan(&note.ID, &note.Title, &content, &note.Color, &noteCreatedAt, &noteUpdatedAt); err != nil {
			return nil, apperrors.NewRepositoryError(err)
		}
		if content.Valid {
			note.Content = content.String
		}
		note.CreatedAt = timePtr(noteCreatedAt)
		note.UpdatedAt = timePtr(noteUpdatedAt)
		data.Notes = append(data.Notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}

	return data, nil
}

func (ur *UserRepository) DeleteUserAndData(ctx context.Context, userID int64) error {
	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `delete from public.users where id = $1;`, userID)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	if err = tx.Commit(ctx); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (ur *UserRepository) ListAttachmentStorageKeys(ctx context.Context, userID int64) ([]string, error) {
	rows, err := ur.db.Query(ctx, `
		select
			note_attachments.storage_key
		from
			public.note_attachments
		where 1=1
			and note_attachments.user_id = $1
		order by
			note_attachments.id;
	`, userID)
	if err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, apperrors.NewRepositoryError(err)
		}
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	return keys, nil
}

func (ur *UserRepository) FindAccountBasics(ctx context.Context, userID int64) (*models.User, error) {
	var user models.User

	query := `
		select
			users.email
		,	users.recovery_email
		,	users.pending_recovery_email
		,	users.created_at
		from
			public.users
		where 1=1
			and users.id = $1;
	`
	row := ur.db.QueryRow(ctx, query, userID)
	if err := row.Scan(&user.Email, &user.RecoveryEmail, &user.PendingRecoveryEmail, &user.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &user, apperrors.ErrNotFound
		}
		return &user, apperrors.NewRepositoryError(err)
	}
	return &user, nil
}

func timePtr(ts pgtype.Timestamp) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time.UTC()
	return &t
}

func (ur *UserRepository) EmailAddressInUse(ctx context.Context, userID int64, email string) (bool, error) {
	var inUse bool

	row := ur.db.QueryRow(ctx, `
		select exists (
			select 1
			from public.users
			where id <> $2
				and (
					lower(email) = lower($1)
					or lower(coalesce(recovery_email, '')) = lower($1)
					or lower(coalesce(pending_recovery_email, '')) = lower($1)
				)
		);
	`, email, userID)
	if err := row.Scan(&inUse); err != nil {
		return false, apperrors.NewRepositoryError(err)
	}
	return inUse, nil
}

func (ur *UserRepository) BeginRecoveryEmailChange(ctx context.Context, userID int64, pendingEmail, rawToken string) (string, string, error) {
	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	var user models.User
	row := tx.QueryRow(ctx, `
		update public.users
		set
			pending_recovery_email = $1
		,	updated_at = now()
		where id = $2
		returning id, email;
	`, pendingEmail, userID)
	if err := row.Scan(&user.Id, &user.Email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", apperrors.ErrNotFound
		}
		return "", "", apperrors.NewRepositoryError(err)
	}

	if _, err := ur.createAuthToken(tx, ctx, &user, rawToken, TokenPurposeRecoveryEmailConfirmation, recoveryEmailConfirmationTokenTTL); err != nil {
		return "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", apperrors.NewRepositoryError(err)
	}
	return user.Email.String, rawToken, nil
}

func (ur *UserRepository) CancelRecoveryEmailChange(ctx context.Context, userID int64, pendingEmail string) error {
	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		update public.users
		set
			pending_recovery_email = null
		,	updated_at = now()
		where id = $1
			and pending_recovery_email = $2;
	`, userID, pendingEmail); err != nil {
		return apperrors.NewRepositoryError(err)
	}

	if _, err = tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where user_id = $1
			and purpose = $2
			and confirmed = false;
	`, userID, TokenPurposeRecoveryEmailConfirmation); err != nil {
		return apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (ur *UserRepository) ConfirmRecoveryEmailByToken(ctx context.Context, token string) (string, string, int64, error) {
	tokenHash := key.HashToken(token)

	var userID, tokenID int64
	var primaryEmail, recoveryEmail string
	row := ur.db.QueryRow(ctx, `
		select
			users.id
		,	users.email
		,	users.pending_recovery_email
		,	users_confirmation_tokens.id
		from public.users
			inner join public.users_confirmation_tokens
				on users.id = users_confirmation_tokens.user_id
		where users_confirmation_tokens.confirmed = false
			and users_confirmation_tokens.token_hash = $1
			and users_confirmation_tokens.purpose = $2
			and users_confirmation_tokens.expires_at > current_timestamp
			and users.pending_recovery_email is not null;
	`, tokenHash, TokenPurposeRecoveryEmailConfirmation)
	if err := row.Scan(&userID, &primaryEmail, &recoveryEmail, &tokenID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", 0, apperrors.ErrInvalidTokenOrUserAlreadyConfirmed
		}
		return "", "", 0, apperrors.NewRepositoryError(err)
	}

	tx, err := ur.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", 0, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		update public.users
		set
			recovery_email = pending_recovery_email
		,	pending_recovery_email = null
		,	updated_at = now()
		where id = $1;
	`, userID); err != nil {
		return "", "", 0, apperrors.NewRepositoryError(err)
	}

	if _, err = tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where id = $1;
	`, tokenID); err != nil {
		return "", "", 0, apperrors.NewRepositoryError(err)
	}

	if _, err = tx.Exec(ctx, `
		update public.users_confirmation_tokens
		set
			confirmed = true
		,	updated_at = now()
		where user_id = $1
			and purpose = $2
			and confirmed = false;
	`, userID, TokenPurposeRecoveryEmailConfirmation); err != nil {
		return "", "", 0, apperrors.NewRepositoryError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", 0, apperrors.NewRepositoryError(err)
	}
	return primaryEmail, recoveryEmail, userID, nil
}

func NewUserPersonalRepository(db database.Pool) *UserPersonalRepository {
	return &UserPersonalRepository{db: db}
}

type UserPersonalRepository struct {
	db database.Pool
}

// PersonalRecordID extrai o id numérico do registo (0 se ausente).
func PersonalRecordID(info *models.UserPersonalInformation) int {
	if info == nil || !info.Id.Valid || info.Id.Int == nil {
		return 0
	}
	return int(info.Id.Int.Int64())
}

func (upr *UserPersonalRepository) FindByUserID(ctx context.Context, userID int64) (*models.UserPersonalInformation, error) {
	var info models.UserPersonalInformation

	query := `
		select
			id
		,	user_id
		,	first_name
		,	last_name
		,	phone_number
		,	address_id
		,	created_at
		,	updated_at
		from 
			public.users_personal_information
		where 1=1
			and user_id = $1;
	`
	row := upr.db.QueryRow(ctx, query, userID)
	if err := row.Scan(
		&info.Id,
		&info.UserId,
		&info.FirstName,
		&info.LastName,
		&info.PhoneNumber,
		&info.AddressId,
		&info.CreatedAt,
		&info.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	return &info, nil
}

func (upr *UserPersonalRepository) Create(ctx context.Context, userID int64, firstName, lastName, phoneNumber string) (*models.UserPersonalInformation, error) {
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	phoneNumber = strings.TrimSpace(phoneNumber)

	var info models.UserPersonalInformation
	query := `
		insert into public.users_personal_information (user_id, first_name, last_name, phone_number) values ($1, $2, $3, $4)
		returning id, user_id, first_name, last_name, phone_number, created_at;
	`
	row := upr.db.QueryRow(ctx, query, userID, firstName, lastName, phoneNumber)
	if err := row.Scan(
		&info.Id,
		&info.UserId,
		&info.FirstName,
		&info.LastName,
		&info.PhoneNumber,
		&info.CreatedAt,
	); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	return &info, nil
}

func (upr *UserPersonalRepository) Update(ctx context.Context, userID int64, id int, firstName, lastName, phoneNumber string) (*models.UserPersonalInformation, error) {
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	phoneNumber = strings.TrimSpace(phoneNumber)

	var info models.UserPersonalInformation
	query := `
		update public.users_personal_information
		set
			first_name = $1,
			last_name = $2,
			phone_number = $3,
			updated_at = $4
		where id = $5
			and user_id = $6
		returning id, user_id, first_name, last_name, phone_number, created_at, updated_at;
	`
	row := upr.db.QueryRow(ctx, query, firstName, lastName, phoneNumber, time.Now(), id, userID)
	if err := row.Scan(
		&info.Id,
		&info.UserId,
		&info.FirstName,
		&info.LastName,
		&info.PhoneNumber,
		&info.CreatedAt,
		&info.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	return &info, nil
}

type UserSessionRepository struct {
	db database.Pool
}

func NewUserSessionRepository(db database.Pool) *UserSessionRepository {
	return &UserSessionRepository{db: db}
}

func (usr *UserSessionRepository) Create(ctx context.Context, userID int64, sessionID, userAgent, ipAddress string) error {
	userAgent = strings.TrimSpace(userAgent)
	ipAddress = strings.TrimSpace(ipAddress)
	sessionID = strings.TrimSpace(sessionID)

	tx, err := usr.db.Begin(ctx)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	revokeSameDeviceQuery := `
		update public.user_sessions
		set revoked_at = current_timestamp
		where user_id = $1
		  and user_agent = $2
		  and revoked_at is null
		  and (
			(nullif($3::text, '') is null and ip_address is null)
			or host(ip_address) = nullif($3::text, '')
		  );
	`
	if _, err := tx.Exec(ctx, revokeSameDeviceQuery, userID, userAgent, ipAddress); err != nil {
		return apperrors.NewRepositoryError(err)
	}

	createQuery := `
		insert into public.user_sessions (user_id, session_id, user_agent, ip_address)
		values ($1, $2::uuid, $3, nullif($4, '')::inet);
	`
	if _, err := tx.Exec(ctx, createQuery, userID, sessionID, userAgent, ipAddress); err != nil {
		return apperrors.NewRepositoryError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (usr *UserSessionRepository) ListActive(ctx context.Context, userID int64, limit int) ([]models.UserSession, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		select
			user_sessions.id,
			user_sessions.user_id,
			user_sessions.session_id::text,
			user_sessions.user_agent,
			coalesce(user_sessions.ip_address::text, ''),
			user_sessions.created_at,
			user_sessions.last_seen_at,
			user_sessions.revoked_at
		from
			public.user_sessions
		where
			user_sessions.user_id = $1
			and user_sessions.revoked_at is null
			and user_sessions.last_seen_at > current_timestamp - interval '30 minutes'
		order by
			user_sessions.last_seen_at desc,
			user_sessions.created_at desc,
			user_sessions.id desc
		limit $2;
	`

	rows, err := usr.db.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	sessions := make([]models.UserSession, 0)
	for rows.Next() {
		var session models.UserSession
		if err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.SessionID,
			&session.UserAgent,
			&session.IPAddress,
			&session.CreatedAt,
			&session.LastSeenAt,
			&session.RevokedAt,
		); err != nil {
			return nil, apperrors.NewRepositoryError(err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	return sessions, nil
}

func (usr *UserSessionRepository) IsActive(ctx context.Context, userID int64, sessionID string) (bool, error) {
	var active bool

	query := `
		select exists (
			select 1
			from 
				public.user_sessions
			where
				user_sessions.user_id = $1
				and user_sessions.session_id = $2::uuid
				and user_sessions.revoked_at is null
				and user_sessions.last_seen_at > current_timestamp - interval '30 minutes'
		);
	`
	row := usr.db.QueryRow(ctx, query, userID, strings.TrimSpace(sessionID))
	if err := row.Scan(&active); err != nil {
		return false, apperrors.NewRepositoryError(err)
	}

	return active, nil
}

func (usr *UserSessionRepository) Touch(ctx context.Context, sessionID string) error {
	query := `
		update public.user_sessions
		set
			last_seen_at = current_timestamp
		where
			user_sessions.session_id = $1::uuid
			and user_sessions.revoked_at is null;
	`
	session, err := usr.db.Exec(ctx, query, strings.TrimSpace(sessionID))
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if session.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

func (usr *UserSessionRepository) RevokeOther(ctx context.Context, userID int64, currentSessionID string) error {
	query := `
		update public.user_sessions
		set 
			revoked_at = current_timestamp
		where
			user_sessions.user_id = $1
			and user_sessions.session_id <> $2::uuid
			and user_sessions.revoked_at is null;
	`
	if _, err := usr.db.Exec(ctx, query, userID, strings.TrimSpace(currentSessionID)); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (usr *UserSessionRepository) Revoke(ctx context.Context, userID int64, sessionID string) error {
	query := `
		update public.user_sessions
		set 
			revoked_at = current_timestamp
		where
			user_sessions.user_id = $1
			and user_sessions.session_id = $2::uuid
			and user_sessions.revoked_at is null;
	`
	result, err := usr.db.Exec(ctx, query, userID, strings.TrimSpace(sessionID))
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

type TwoFactorRepository struct {
	db database.Pool
}

func NewTwoFactorRepository(db database.Pool) *TwoFactorRepository {
	return &TwoFactorRepository{db: db}
}

func (r *TwoFactorRepository) GetStatus(ctx context.Context, userID int64) (*models.TwoFactorStatus, error) {
	var status models.TwoFactorStatus
	var method pgtype.Text
	var enabledAt, disabledAt pgtype.Timestamp

	query := `
		select
			id
		,	email
		,	active
		,	two_factor_enabled
		,	two_factor_method
		,	two_factor_totp_secret_encrypted
		,	two_factor_enabled_at
		,	two_factor_disabled_at
		from 
			public.users
		where 1=1
			and id = $1;
	`
	row := r.db.QueryRow(ctx, query, userID)
	if err := row.Scan(
		&status.UserID,
		&status.Email,
		&status.Active,
		&status.Enabled,
		&method,
		&status.TOTPSecretEncrypted,
		&enabledAt,
		&disabledAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	if method.Valid {
		status.Method = method.String
	}
	status.EnabledAt = twoFactorTimePtr(enabledAt)
	status.DisabledAt = twoFactorTimePtr(disabledAt)
	return &status, nil
}

func (r *TwoFactorRepository) CreateChallenge(ctx context.Context, userID int64, method, purpose, codeHash string, encryptedSecret []byte, ttl time.Duration) (*models.TwoFactorChallenge, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	queryUpdate := `
		update public.users_two_factor_challenges
		set
			consumed_at = current_timestamp
		,	updated_at = current_timestamp
		where user_id = $1
			and method = $2
			and purpose = $3
			and consumed_at is null;
	`

	if _, err := tx.Exec(ctx, queryUpdate, userID, method, purpose); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}

	challenge := &models.TwoFactorChallenge{
		UserID:              userID,
		Method:              method,
		Purpose:             purpose,
		CodeHash:            codeHash,
		TOTPSecretEncrypted: encryptedSecret,
		ExpiresAt:           time.Now().UTC().Add(ttl),
	}

	queryInsert := `
		insert into public.users_two_factor_challenges (user_id, method, purpose, code_hash, totp_secret_encrypted, expires_at) 
		values ($1, $2, $3, $4, $5, $6) 
		returning id, created_at;
	`

	row := tx.QueryRow(ctx, queryInsert, userID, method, purpose, nullableString(codeHash), nullableBytes(encryptedSecret), challenge.ExpiresAt)
	var createdAt pgtype.Timestamp
	if err := row.Scan(&challenge.ID, &createdAt); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	challenge.CreatedAt = twoFactorTimePtr(createdAt)

	if err = tx.Commit(ctx); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	return challenge, nil
}

func (r *TwoFactorRepository) GetActiveChallenge(ctx context.Context, userID int64, method, purpose string) (*models.TwoFactorChallenge, error) {
	var challenge models.TwoFactorChallenge
	var codeHash pgtype.Text
	var expiresAt, consumedAt, createdAt, updatedAt pgtype.Timestamp

	query := `
		select
			id
		,	user_id
		,	method
		,	purpose
		,	code_hash
		,	totp_secret_encrypted
		,	expires_at
		,	consumed_at
		,	attempt_count
		,	created_at
		,	updated_at
		from 
			public.users_two_factor_challenges
		where 1=1
			and user_id = $1
			and method = $2
			and purpose = $3
			and consumed_at is null
			and expires_at > current_timestamp
		order by id desc
		limit 1;
	`

	row := r.db.QueryRow(ctx, query, userID, method, purpose)
	if err := row.Scan(
		&challenge.ID,
		&challenge.UserID,
		&challenge.Method,
		&challenge.Purpose,
		&codeHash,
		&challenge.TOTPSecretEncrypted,
		&expiresAt,
		&consumedAt,
		&challenge.AttemptCount,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	if codeHash.Valid {
		challenge.CodeHash = codeHash.String
	}
	if expiresAt.Valid {
		challenge.ExpiresAt = expiresAt.Time.UTC()
	}
	challenge.ConsumedAt = twoFactorTimePtr(consumedAt)
	challenge.CreatedAt = twoFactorTimePtr(createdAt)
	challenge.UpdatedAt = twoFactorTimePtr(updatedAt)
	return &challenge, nil
}

func (r *TwoFactorRepository) IncrementActiveChallengeAttempts(ctx context.Context, challengeID int64, maxAttempts int) error {
	query := `
		update public.users_two_factor_challenges
		set
			attempt_count = attempt_count + 1
		,	updated_at = current_timestamp
		where id = $1
			and consumed_at is null
			and expires_at > current_timestamp
			and attempt_count < $2;
	`

	tag, err := r.db.Exec(ctx, query, challengeID, maxAttempts)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *TwoFactorRepository) ConsumeActiveChallenge(ctx context.Context, challengeID, userID int64, method, purpose string, maxAttempts int) error {
	query := `
		update public.users_two_factor_challenges
		set
			consumed_at = current_timestamp
		,	updated_at = current_timestamp
		where id = $1
			and user_id = $2
			and method = $3
			and purpose = $4
			and consumed_at is null
			and expires_at > current_timestamp
			and attempt_count < $5;
	`

	tag, err := r.db.Exec(ctx, query, challengeID, userID, method, purpose, maxAttempts)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *TwoFactorRepository) ActivateEmail(ctx context.Context, userID int64) error {
	return r.activate(ctx, userID, models.TwoFactorMethodEmail, nil)
}

func (r *TwoFactorRepository) ActivateTOTP(ctx context.Context, userID int64, encryptedSecret []byte) error {
	return r.activate(ctx, userID, models.TwoFactorMethodTOTP, encryptedSecret)
}

func (r *TwoFactorRepository) activate(ctx context.Context, userID int64, method string, encryptedSecret []byte) error {
	query := `
		update public.users
		set
			two_factor_enabled = true
		,	two_factor_method = $2
		,	two_factor_totp_secret_encrypted = $3
		,	two_factor_enabled_at = current_timestamp
		,	two_factor_disabled_at = null
		,	updated_at = current_timestamp
		where id = $1
			and active = true;
	`

	tag, err := r.db.Exec(ctx, query, userID, method, nullableBytes(encryptedSecret))
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrUserInactive
	}
	return nil
}

func (r *TwoFactorRepository) Disable(ctx context.Context, userID int64) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	query := `
		update public.users
		set
			two_factor_enabled = false
		,	two_factor_method = null
		,	two_factor_totp_secret_encrypted = null
		,	two_factor_disabled_at = current_timestamp
		,	updated_at = current_timestamp
		where id = $1;
	`
	tag, err := tx.Exec(ctx, query, userID)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `delete from public.users_recovery_codes where user_id = $1;`, userID); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (r *TwoFactorRepository) DeleteExpiredChallenges(ctx context.Context) error {
	query := `
		delete from public.users_two_factor_challenges
		where expires_at < current_timestamp - interval '1 day'
			or (
				consumed_at is not null
				and consumed_at < current_timestamp - interval '1 day'
			);
	`
	_, err := r.db.Exec(ctx, query)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func twoFactorTimePtr(ts pgtype.Timestamp) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time.UTC()
	return &t
}

type RecoveryCodeRepository struct {
	db database.Pool
}

func NewRecoveryCodeRepository(db database.Pool) *RecoveryCodeRepository {
	return &RecoveryCodeRepository{db: db}
}

func (r *RecoveryCodeRepository) Replace(ctx context.Context, userID int64, codeHashes []string) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from public.users_recovery_codes where user_id = $1;`, userID); err != nil {
		return apperrors.NewRepositoryError(err)
	}

	const query = `
		insert into public.users_recovery_codes (user_id, code_hash)
		values ($1, $2);
	`
	for _, codeHash := range codeHashes {
		if _, err := tx.Exec(ctx, query, userID, codeHash); err != nil {
			return apperrors.NewRepositoryError(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

func (r *RecoveryCodeRepository) Use(ctx context.Context, userID int64, codeHash string) error {
	const query = `
		update public.users_recovery_codes
		set used_at = current_timestamp
		where user_id = $1
			and code_hash = $2
			and used_at is null;
	`
	tag, err := r.db.Exec(ctx, query, userID, codeHash)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *RecoveryCodeRepository) Delete(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `delete from public.users_recovery_codes where user_id = $1;`, userID)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}
