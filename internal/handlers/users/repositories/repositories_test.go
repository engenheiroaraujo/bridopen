package repositories

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return mock
}

func ts() time.Time {
	return time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
}

func num(id int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(id), Valid: true}
}

func expectCreateAuthToken(mock pgxmock.PgxPoolIface) {
	mock.ExpectExec(`update public.users_confirmation_tokens`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectQuery(`insert into public.users_confirmation_tokens`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "expires_at"}).
			AddRow("99", ts(), ts().Add(time.Hour)))
}

// TestPersonalRecordID verifica que PersonalRecordID extrai o id numérico ou zero quando inválido/nil.
func TestPersonalRecordID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, PersonalRecordID(nil))
	assert.Equal(t, 0, PersonalRecordID(&models.UserPersonalInformation{}))

	info := &models.UserPersonalInformation{
		Id: pgtype.Numeric{Int: big.NewInt(42), Valid: true},
	}
	assert.Equal(t, 42, PersonalRecordID(info))
}

// TestNullableBytesAndString verifica conversões nullableBytes e nullableString para nil/vazio/valor.
func TestNullableBytesAndString(t *testing.T) {
	t.Parallel()

	assert.Nil(t, nullableBytes(nil))
	assert.Nil(t, nullableBytes([]byte{}))
	assert.Equal(t, []byte("abc"), nullableBytes([]byte("abc")))

	assert.Nil(t, nullableString(""))
	assert.Equal(t, "ok", nullableString("ok"))
}

// TestTwoFactorTimePtrAndTimePtr verifica que timePtr/twoFactorTimePtr devolvem nil ou ponteiro para timestamp válido.
func TestTwoFactorTimePtrAndTimePtr(t *testing.T) {
	t.Parallel()

	assert.Nil(t, twoFactorTimePtr(pgtype.Timestamp{}))
	assert.Nil(t, timePtr(pgtype.Timestamp{}))

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	got := twoFactorTimePtr(pgtype.Timestamp{Time: now, Valid: true})
	require.NotNil(t, got)
	assert.True(t, got.Equal(now))

	got = timePtr(pgtype.Timestamp{Time: now, Valid: true})
	require.NotNil(t, got)
	assert.True(t, got.Equal(now))
}

// TestTwoFactorConstants verifica as constantes de método 2FA (email e totp).
func TestTwoFactorConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "email", models.TwoFactorMethodEmail)
	assert.Equal(t, "totp", models.TwoFactorMethodTOTP)
}

// TestExportUserData verifica ExportUserData no sucesso, usuário inexistente e erros de consulta.
func TestExportUserData(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`as name`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "name", "email", "phone", "active", "2fa", "method", "created", "updated",
			}).AddRow(int64(7), "A B", "a@b.c", "9", true, false, "", ts(), ts()))
		mock.ExpectQuery(`from public.notes`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "color", "created_at", "updated_at"}).
				AddRow(int64(1), "T", "C", "#fff", ts(), nil).
				AddRow(int64(2), "T2", nil, "#000", ts(), ts()))
		data, err := repo.ExportUserData(ctx, 7)
		require.NoError(t, err)
		require.Len(t, data.Notes, 2)
		assert.Equal(t, "C", data.Notes[0].Content)
		assert.Empty(t, data.Notes[1].Content)
		assert.Nil(t, data.Notes[0].UpdatedAt)
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`as name`).WithArgs(int64(7)).WillReturnError(pgx.ErrNoRows)
		_, err := repo.ExportUserData(ctx, 7)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("user error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`as name`).WithArgs(int64(7)).WillReturnError(errors.New("x"))
		_, err := repo.ExportUserData(ctx, 7)
		require.Error(t, err)
	})

	t.Run("notes query/scan/rows errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`as name`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "name", "email", "phone", "active", "2fa", "method", "created", "updated",
			}).AddRow(int64(7), "A", "a@b.c", "", true, false, "", nil, nil))
		mock.ExpectQuery(`from public.notes`).WithArgs(int64(7)).WillReturnError(errors.New("notes"))
		_, err := repo.ExportUserData(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`as name`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "name", "email", "phone", "active", "2fa", "method", "created", "updated",
			}).AddRow(int64(7), "A", "a@b.c", "", true, false, "", nil, nil))
		mock.ExpectQuery(`from public.notes`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "color", "created_at", "updated_at"}).
				AddRow("bad", 1, 2, 3, 4, 5))
		_, err = repo.ExportUserData(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`as name`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "name", "email", "phone", "active", "2fa", "method", "created", "updated",
			}).AddRow(int64(7), "A", "a@b.c", "", true, false, "", nil, nil))
		mock.ExpectQuery(`from public.notes`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "color", "created_at", "updated_at"}).
				AddRow(int64(1), "T", "C", "#fff", ts(), ts()).CloseError(errors.New("r")))
		_, err = repo.ExportUserData(ctx, 7)
		require.Error(t, err)
	})
}

// TestDeleteUserAndDataAndListKeys verifica exclusão de usuário/dados e listagem de chaves de anexos.
func TestDeleteUserAndDataAndListKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("delete success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()
		require.NoError(t, repo.DeleteUserAndData(ctx, 7))
	})

	t.Run("delete errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		require.Error(t, repo.DeleteUserAndData(ctx, 7))

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users`).WithArgs(int64(7)).WillReturnError(errors.New("del"))
		mock.ExpectRollback()
		require.Error(t, repo.DeleteUserAndData(ctx, 7))

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectRollback()
		require.ErrorIs(t, repo.DeleteUserAndData(ctx, 7), apperrors.ErrNotFound)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		require.Error(t, repo.DeleteUserAndData(ctx, 7))
	})

	t.Run("list attachment keys", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("k1").AddRow("  ").AddRow("k2"))
		keys, err := repo.ListAttachmentStorageKeys(ctx, 7)
		require.NoError(t, err)
		assert.Equal(t, []string{"k1", "k2"}, keys)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).WithArgs(int64(7)).WillReturnError(errors.New("q"))
		_, err = repo.ListAttachmentStorageKeys(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow(struct{}{}))
		_, err = repo.ListAttachmentStorageKeys(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("k").CloseError(errors.New("r")))
		_, err = repo.ListAttachmentStorageKeys(ctx, 7)
		require.Error(t, err)
	})
}

// TestAccountBasicsAndEmailInUse verifica FindAccountByID, IsEmailInUse e consultas básicas de conta.
func TestAccountBasicsAndEmailInUse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewUserRepository(mock)
	mock.ExpectQuery(`pending_recovery_email`).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"email", "recovery", "pending", "created"}).
			AddRow("a@b.c", "r@b.c", nil, ts()))
	user, err := repo.FindAccountBasics(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, "a@b.c", user.Email.String)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`pending_recovery_email`).WithArgs(int64(7)).WillReturnError(pgx.ErrNoRows)
	_, err = repo.FindAccountBasics(ctx, 7)
	require.ErrorIs(t, err, apperrors.ErrNotFound)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`pending_recovery_email`).WithArgs(int64(7)).WillReturnError(errors.New("x"))
	_, err = repo.FindAccountBasics(ctx, 7)
	require.Error(t, err)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`select exists`).
		WithArgs("x@y.z", int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	inUse, err := repo.EmailAddressInUse(ctx, 7, "x@y.z")
	require.NoError(t, err)
	assert.True(t, inUse)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`select exists`).WithArgs("x@y.z", int64(7)).WillReturnError(errors.New("x"))
	_, err = repo.EmailAddressInUse(ctx, 7, "x@y.z")
	require.Error(t, err)
}

// TestRecoveryEmailFlows verifica fluxos de begin/confirm/cancel de e-mail de recuperação.
func TestRecoveryEmailFlows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("begin change", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`pending_recovery_email = \$1`).
			WithArgs("r@b.c", int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email"}).AddRow("7", "a@b.c"))
		expectCreateAuthToken(mock)
		mock.ExpectCommit()
		email, tok, err := repo.BeginRecoveryEmailChange(ctx, 7, "r@b.c", "raw")
		require.NoError(t, err)
		assert.Equal(t, "a@b.c", email)
		assert.Equal(t, "raw", tok)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, _, err = repo.BeginRecoveryEmailChange(ctx, 7, "r@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`pending_recovery_email = \$1`).
			WithArgs("r@b.c", int64(7)).
			WillReturnError(pgx.ErrNoRows)
		mock.ExpectRollback()
		_, _, err = repo.BeginRecoveryEmailChange(ctx, 7, "r@b.c", "raw")
		require.ErrorIs(t, err, apperrors.ErrNotFound)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`pending_recovery_email = \$1`).
			WithArgs("r@b.c", int64(7)).
			WillReturnError(errors.New("x"))
		mock.ExpectRollback()
		_, _, err = repo.BeginRecoveryEmailChange(ctx, 7, "r@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`pending_recovery_email = \$1`).
			WithArgs("r@b.c", int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email"}).AddRow("7", "a@b.c"))
		mock.ExpectExec(`update public.users_confirmation_tokens`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("tok"))
		mock.ExpectRollback()
		_, _, err = repo.BeginRecoveryEmailChange(ctx, 7, "r@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`pending_recovery_email = \$1`).
			WithArgs("r@b.c", int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email"}).AddRow("7", "a@b.c"))
		expectCreateAuthToken(mock)
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, _, err = repo.BeginRecoveryEmailChange(ctx, 7, "r@b.c", "raw")
		require.Error(t, err)
	})

	t.Run("cancel change", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`pending_recovery_email = null`).
			WithArgs(int64(7), "r@b.c").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).
			WithArgs(int64(7), TokenPurposeRecoveryEmailConfirmation).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()
		require.NoError(t, repo.CancelRecoveryEmailChange(ctx, 7, "r@b.c"))

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		require.Error(t, repo.CancelRecoveryEmailChange(ctx, 7, "r@b.c"))

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`pending_recovery_email = null`).
			WithArgs(int64(7), "r@b.c").
			WillReturnError(errors.New("upd"))
		mock.ExpectRollback()
		require.Error(t, repo.CancelRecoveryEmailChange(ctx, 7, "r@b.c"))

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`pending_recovery_email = null`).
			WithArgs(int64(7), "r@b.c").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).
			WithArgs(int64(7), TokenPurposeRecoveryEmailConfirmation).
			WillReturnError(errors.New("tok"))
		mock.ExpectRollback()
		require.Error(t, repo.CancelRecoveryEmailChange(ctx, 7, "r@b.c"))

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`pending_recovery_email = null`).
			WithArgs(int64(7), "r@b.c").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).
			WithArgs(int64(7), TokenPurposeRecoveryEmailConfirmation).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		require.Error(t, repo.CancelRecoveryEmailChange(ctx, 7, "r@b.c"))
	})

	t.Run("confirm by token", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`pending_recovery_email is not null`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeRecoveryEmailConfirmation).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "pending", "tid"}).
				AddRow(int64(7), "a@b.c", "r@b.c", int64(3)))
		mock.ExpectBegin()
		mock.ExpectExec(`recovery_email = pending_recovery_email`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).WithArgs(int64(7), TokenPurposeRecoveryEmailConfirmation).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectCommit()
		primary, recovery, id, err := repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.NoError(t, err)
		assert.Equal(t, "a@b.c", primary)
		assert.Equal(t, "r@b.c", recovery)
		assert.Equal(t, int64(7), id)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`pending_recovery_email is not null`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeRecoveryEmailConfirmation).
			WillReturnError(pgx.ErrNoRows)
		_, _, _, err = repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.ErrorIs(t, err, apperrors.ErrInvalidTokenOrUserAlreadyConfirmed)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`pending_recovery_email is not null`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeRecoveryEmailConfirmation).
			WillReturnError(errors.New("x"))
		_, _, _, err = repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.Error(t, err)

		setup := func() (pgxmock.PgxPoolIface, *UserRepository) {
			m := newMockPool(t)
			m.ExpectQuery(`pending_recovery_email is not null`).
				WithArgs(pgxmock.AnyArg(), TokenPurposeRecoveryEmailConfirmation).
				WillReturnRows(pgxmock.NewRows([]string{"id", "email", "pending", "tid"}).
					AddRow(int64(7), "a@b.c", "r@b.c", int64(3)))
			m.ExpectBegin()
			return m, NewUserRepository(m)
		}

		mock, repo = setup()
		mock.ExpectBegin().WillReturnError(errors.New("begin")) // não funciona — Begin já esperado
		// refazer corretamente:
		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`pending_recovery_email is not null`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeRecoveryEmailConfirmation).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "pending", "tid"}).
				AddRow(int64(7), "a@b.c", "r@b.c", int64(3)))
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, _, _, err = repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.Error(t, err)

		mock, repo = setup()
		mock.ExpectExec(`recovery_email = pending_recovery_email`).WithArgs(int64(7)).WillReturnError(errors.New("u"))
		mock.ExpectRollback()
		_, _, _, err = repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.Error(t, err)

		mock, repo = setup()
		mock.ExpectExec(`recovery_email = pending_recovery_email`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnError(errors.New("t"))
		mock.ExpectRollback()
		_, _, _, err = repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.Error(t, err)

		mock, repo = setup()
		mock.ExpectExec(`recovery_email = pending_recovery_email`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).WithArgs(int64(7), TokenPurposeRecoveryEmailConfirmation).WillReturnError(errors.New("bulk"))
		mock.ExpectRollback()
		_, _, _, err = repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.Error(t, err)

		mock, repo = setup()
		mock.ExpectExec(`recovery_email = pending_recovery_email`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).WithArgs(int64(7), TokenPurposeRecoveryEmailConfirmation).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, _, _, err = repo.ConfirmRecoveryEmailByToken(ctx, "raw")
		require.Error(t, err)
	})
}

// TestPasswordAuthDataAndUpdate verifica leitura de dados de autenticação e atualização de senha.
func TestPasswordAuthDataAndUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewUserRepository(mock)
	mock.ExpectQuery(`users.password`).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"email", "password", "active"}).AddRow("a@b.c", "hash", true))
	email, hash, active, err := repo.FindPasswordAuthData(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, "a@b.c", email)
	assert.Equal(t, "hash", hash)
	assert.True(t, active)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`users.password`).WithArgs(int64(7)).WillReturnError(pgx.ErrNoRows)
	_, _, _, err = repo.FindPasswordAuthData(ctx, 7)
	require.ErrorIs(t, err, apperrors.ErrNotFound)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`users.password`).WithArgs(int64(7)).WillReturnError(errors.New("x"))
	_, _, _, err = repo.FindPasswordAuthData(ctx, 7)
	require.Error(t, err)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectBegin()
	mock.ExpectExec(`update public.users`).
		WithArgs("new", int64(7)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`purpose = \$2`).
		WithArgs(int64(7), TokenPurposePasswordReset).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectCommit()
	require.NoError(t, repo.UpdatePasswordByID(ctx, 7, "new"))

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectBegin().WillReturnError(errors.New("begin"))
	require.Error(t, repo.UpdatePasswordByID(ctx, 7, "new"))

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectBegin()
	mock.ExpectExec(`update public.users`).WithArgs("new", int64(7)).WillReturnError(errors.New("u"))
	mock.ExpectRollback()
	require.Error(t, repo.UpdatePasswordByID(ctx, 7, "new"))

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectBegin()
	mock.ExpectExec(`update public.users`).
		WithArgs("new", int64(7)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectRollback()
	require.ErrorIs(t, repo.UpdatePasswordByID(ctx, 7, "new"), apperrors.ErrNotFound)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectBegin()
	mock.ExpectExec(`update public.users`).
		WithArgs("new", int64(7)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`purpose = \$2`).
		WithArgs(int64(7), TokenPurposePasswordReset).
		WillReturnError(errors.New("tok"))
	mock.ExpectRollback()
	require.Error(t, repo.UpdatePasswordByID(ctx, 7, "new"))

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectBegin()
	mock.ExpectExec(`update public.users`).
		WithArgs("new", int64(7)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`purpose = \$2`).
		WithArgs(int64(7), TokenPurposePasswordReset).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectCommit().WillReturnError(errors.New("commit"))
	mock.ExpectRollback()
	require.Error(t, repo.UpdatePasswordByID(ctx, 7, "new"))
}

// TestFindByEmail verifica FindByEmail para e-mail existente, ausente e erro de query.
func TestFindByEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewUserRepository(mock)
	mock.ExpectQuery(`select count`).WithArgs("a@b.c").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(int64(1)))
	ok, err := repo.FindByEmail(ctx, "a@b.c")
	require.NoError(t, err)
	assert.True(t, ok)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`select count`).WithArgs("a@b.c").
		WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(int64(0)))
	ok, err = repo.FindByEmail(ctx, "a@b.c")
	require.NoError(t, err)
	assert.False(t, ok)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`select count`).WithArgs("a@b.c").WillReturnError(errors.New("x"))
	_, err = repo.FindByEmail(ctx, "a@b.c")
	require.Error(t, err)
}

// TestFindByUser verifica FindByUser no sucesso, não encontrado e erro.
func TestFindByUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewUserRepository(mock)
	mock.ExpectQuery(`users.email = \$1`).
		WithArgs("a@b.c").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "email", "password", "active", "recovery_email", "two_factor_enabled", "two_factor_method", "secret", "first_name", "last_name",
		}).AddRow("7", "a@b.c", "hash", true, "r@b.c", false, nil, nil, "A", "B"))
	user, personal, err := repo.FindByUser(ctx, "a@b.c")
	require.NoError(t, err)
	assert.Equal(t, "a@b.c", user.Email.String)
	assert.Equal(t, "A", personal.FirstName.String)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`users.email = \$1`).WithArgs("x").WillReturnError(pgx.ErrNoRows)
	_, _, err = repo.FindByUser(ctx, "x")
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func expectFindByUser(mock pgxmock.PgxPoolIface, email string, active bool, recovery string) {
	var recoveryArg any = nil
	if recovery != "" {
		recoveryArg = recovery
	}
	mock.ExpectQuery(`users.email = \$1`).
		WithArgs(email).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "email", "password", "active", "recovery_email", "two_factor_enabled", "two_factor_method", "secret", "first_name", "last_name",
		}).AddRow("7", email, "hash", active, recoveryArg, false, nil, nil, "A", "B"))
}

// TestCreateResetPasswordToken verifica CreateResetPasswordToken no sucesso e ramos de erro da transação.
func TestCreateResetPasswordToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("user missing", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`users.email = \$1`).WithArgs("x").WillReturnError(pgx.ErrNoRows)
		tok, recipients, err := repo.CreateResetPasswordToken(ctx, "x", "raw")
		require.NoError(t, err)
		assert.Empty(t, tok)
		assert.Nil(t, recipients)
	})

	t.Run("find error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`users.email = \$1`).WithArgs("x").WillReturnError(errors.New("db"))
		_, _, err := repo.CreateResetPasswordToken(ctx, "x", "raw")
		require.Error(t, err)
	})

	t.Run("inactive", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", false, "")
		_, _, err := repo.CreateResetPasswordToken(ctx, "a@b.c", "raw")
		require.ErrorIs(t, err, apperrors.ErrUserInactive)
	})

	t.Run("success with recovery email", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", true, "r@b.c")
		mock.ExpectBegin()
		expectCreateAuthToken(mock)
		mock.ExpectCommit()
		tok, recipients, err := repo.CreateResetPasswordToken(ctx, "a@b.c", "raw")
		require.NoError(t, err)
		assert.Equal(t, "raw", tok)
		assert.Equal(t, []string{"a@b.c", "r@b.c"}, recipients)
	})

	t.Run("begin/token/commit errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", true, "")
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, _, err := repo.CreateResetPasswordToken(ctx, "a@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", true, "")
		mock.ExpectBegin()
		mock.ExpectExec(`update public.users_confirmation_tokens`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("tok"))
		mock.ExpectRollback()
		_, _, err = repo.CreateResetPasswordToken(ctx, "a@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", true, "a@b.c") // same recovery -> single recipient
		mock.ExpectBegin()
		expectCreateAuthToken(mock)
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, _, err = repo.CreateResetPasswordToken(ctx, "a@b.c", "raw")
		require.Error(t, err)
	})
}

// TestCreateEmailConfirmationToken verifica CreateEmailConfirmationToken (reenvio) no sucesso e erros.
func TestCreateEmailConfirmationToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("missing user", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`users.email = \$1`).WithArgs("x").WillReturnError(pgx.ErrNoRows)
		tok, recipients, err := repo.CreateEmailConfirmationToken(ctx, "x", "raw")
		require.NoError(t, err)
		assert.Empty(t, tok)
		assert.Nil(t, recipients)
	})

	t.Run("already active", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", true, "")
		tok, recipients, err := repo.CreateEmailConfirmationToken(ctx, "a@b.c", "raw")
		require.NoError(t, err)
		assert.Empty(t, tok)
		assert.Nil(t, recipients)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", false, "")
		mock.ExpectBegin()
		expectCreateAuthToken(mock)
		mock.ExpectCommit()
		tok, recipients, err := repo.CreateEmailConfirmationToken(ctx, "a@b.c", "raw")
		require.NoError(t, err)
		assert.Equal(t, "raw", tok)
		assert.Equal(t, []string{"a@b.c"}, recipients)
	})

	t.Run("errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`users.email = \$1`).WithArgs("a@b.c").WillReturnError(errors.New("db"))
		_, _, err := repo.CreateEmailConfirmationToken(ctx, "a@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", false, "")
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, _, err = repo.CreateEmailConfirmationToken(ctx, "a@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", false, "")
		mock.ExpectBegin()
		mock.ExpectExec(`update public.users_confirmation_tokens`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("tok"))
		mock.ExpectRollback()
		_, _, err = repo.CreateEmailConfirmationToken(ctx, "a@b.c", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		expectFindByUser(mock, "a@b.c", false, "")
		mock.ExpectBegin()
		expectCreateAuthToken(mock)
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, _, err = repo.CreateEmailConfirmationToken(ctx, "a@b.c", "raw")
		require.Error(t, err)
	})
}

// TestGetUserConfirmationByToken verifica GetUserConfirmationByToken no sucesso, não encontrado e erro.
func TestGetUserConfirmationByToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewUserRepository(mock)
	mock.ExpectQuery(`users_confirmation_tokens.token_hash`).
		WithArgs(pgxmock.AnyArg(), TokenPurposeEmailConfirmation).
		WillReturnRows(pgxmock.NewRows([]string{
			"uid", "email", "password", "active", "tid", "tuid", "thash", "purpose", "confirmed", "created", "updated", "expires",
		}).AddRow("7", "a@b.c", "p", false, "1", "7", "h", TokenPurposeEmailConfirmation, false, ts(), ts(), ts()))
	user, token, email, err := repo.GetUserConfirmationByToken(ctx, "raw", TokenPurposeEmailConfirmation)
	require.NoError(t, err)
	assert.Equal(t, "a@b.c", email)
	assert.NotNil(t, user)
	assert.NotNil(t, token)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`users_confirmation_tokens.token_hash`).
		WithArgs(pgxmock.AnyArg(), "p").
		WillReturnError(pgx.ErrNoRows)
	_, _, _, err = repo.GetUserConfirmationByToken(ctx, "raw", "p")
	require.ErrorIs(t, err, pgx.ErrNoRows)

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectQuery(`users_confirmation_tokens.token_hash`).
		WithArgs(pgxmock.AnyArg(), "p").
		WillReturnError(errors.New("x"))
	_, _, _, err = repo.GetUserConfirmationByToken(ctx, "raw", "p")
	require.Error(t, err)
}

// TestUserCreate verifica Create de usuário com pessoais/token e ramos de erro da transação.
func TestUserCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.users`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow("7", ts()))
		mock.ExpectQuery(`insert into public.users_personal_information`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"user_id", "first_name", "last_name", "created_at"}).
				AddRow("7", "A", "B", ts()))
		expectCreateAuthToken(mock)
		mock.ExpectCommit()
		user, personal, tok, err := repo.Create(ctx, "A", "B", "a@b.c", "pw", "raw")
		require.NoError(t, err)
		assert.Equal(t, "raw", tok)
		assert.Equal(t, "A", personal.FirstName.String)
		assert.True(t, user.Id.Valid)
	})

	t.Run("errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, _, _, err := repo.Create(ctx, "A", "B", "a@b.c", "pw", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.users`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("user"))
		mock.ExpectRollback()
		_, _, _, err = repo.Create(ctx, "A", "B", "a@b.c", "pw", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.users`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow("7", ts()))
		mock.ExpectQuery(`insert into public.users_personal_information`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("personal"))
		mock.ExpectRollback()
		_, _, _, err = repo.Create(ctx, "A", "B", "a@b.c", "pw", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.users`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow("7", ts()))
		mock.ExpectQuery(`insert into public.users_personal_information`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"user_id", "first_name", "last_name", "created_at"}).
				AddRow("7", "A", "B", ts()))
		mock.ExpectExec(`update public.users_confirmation_tokens`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("tok"))
		mock.ExpectRollback()
		_, _, _, err = repo.Create(ctx, "A", "B", "a@b.c", "pw", "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.users`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow("7", ts()))
		mock.ExpectQuery(`insert into public.users_personal_information`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"user_id", "first_name", "last_name", "created_at"}).
				AddRow("7", "A", "B", ts()))
		expectCreateAuthToken(mock)
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, _, _, err = repo.Create(ctx, "A", "B", "a@b.c", "pw", "raw")
		require.Error(t, err)
	})
}

// TestCreateAuthTokenInsertError verifica falha de insert em createAuthToken.
func TestCreateAuthTokenInsertError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mock := newMockPool(t)
	repo := NewUserRepository(mock)
	mock.ExpectBegin()
	tx, err := mock.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	user := &models.User{Id: pgtype.Numeric{Int: big.NewInt(7), Valid: true}}
	mock.ExpectExec(`update public.users_confirmation_tokens`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectQuery(`insert into public.users_confirmation_tokens`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("ins"))
	_, err = repo.createAuthToken(tx, ctx, user, "raw", TokenPurposeEmailConfirmation, emailConfirmationTokenTTL)
	require.Error(t, err)
}

// TestPutPasswordByToken verifica PutPasswordByToken no sucesso e erros de token/transação.
func TestPutPasswordByToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`purpose = \$2`).
			WithArgs(pgxmock.AnyArg(), TokenPurposePasswordReset).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
		mock.ExpectBegin()
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.users`).WithArgs("new", int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).WithArgs(int64(7), TokenPurposePasswordReset).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectCommit()
		email, id, err := repo.PutPasswordByToken(ctx, "new", "raw")
		require.NoError(t, err)
		assert.Equal(t, "a@b.c", email)
		assert.Equal(t, int64(7), id)
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`purpose = \$2`).
			WithArgs(pgxmock.AnyArg(), TokenPurposePasswordReset).
			WillReturnError(pgx.ErrNoRows)
		_, _, err := repo.PutPasswordByToken(ctx, "new", "raw")
		require.ErrorIs(t, err, apperrors.ErrInvalidTokenOrUserAlreadyConfirmed)
	})

	t.Run("lookup error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`purpose = \$2`).
			WithArgs(pgxmock.AnyArg(), TokenPurposePasswordReset).
			WillReturnError(errors.New("x"))
		_, _, err := repo.PutPasswordByToken(ctx, "new", "raw")
		require.Error(t, err)
	})

	t.Run("tx errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`purpose = \$2`).
			WithArgs(pgxmock.AnyArg(), TokenPurposePasswordReset).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, _, err := repo.PutPasswordByToken(ctx, "new", "raw")
		require.Error(t, err)

		setup := func() (pgxmock.PgxPoolIface, *UserRepository) {
			m := newMockPool(t)
			m.ExpectQuery(`purpose = \$2`).
				WithArgs(pgxmock.AnyArg(), TokenPurposePasswordReset).
				WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
			m.ExpectBegin()
			return m, NewUserRepository(m)
		}

		mock, repo = setup()
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnError(errors.New("tok"))
		mock.ExpectRollback()
		_, _, err = repo.PutPasswordByToken(ctx, "new", "raw")
		require.Error(t, err)

		mock, repo = setup()
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.users`).WithArgs("new", int64(7)).WillReturnError(errors.New("pw"))
		mock.ExpectRollback()
		_, _, err = repo.PutPasswordByToken(ctx, "new", "raw")
		require.Error(t, err)

		mock, repo = setup()
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.users`).WithArgs("new", int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).WithArgs(int64(7), TokenPurposePasswordReset).WillReturnError(errors.New("bulk"))
		mock.ExpectRollback()
		_, _, err = repo.PutPasswordByToken(ctx, "new", "raw")
		require.Error(t, err)

		mock, repo = setup()
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.users`).WithArgs("new", int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`purpose = \$2`).WithArgs(int64(7), TokenPurposePasswordReset).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, _, err = repo.PutPasswordByToken(ctx, "new", "raw")
		require.Error(t, err)
	})
}

// TestConfirmUserByToken verifica ConfirmUserByToken no sucesso e erros de token/transação.
func TestConfirmUserByToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`users.active = false`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeEmailConfirmation).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
		mock.ExpectBegin()
		mock.ExpectExec(`active = true`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()
		email, id, err := repo.ConfirmUserByToken(ctx, "raw")
		require.NoError(t, err)
		assert.Equal(t, "a@b.c", email)
		assert.Equal(t, int64(7), id)
	})

	t.Run("errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserRepository(mock)
		mock.ExpectQuery(`users.active = false`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeEmailConfirmation).
			WillReturnError(errors.New("x"))
		_, _, err := repo.ConfirmUserByToken(ctx, "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`users.active = false`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeEmailConfirmation).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, _, err = repo.ConfirmUserByToken(ctx, "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`users.active = false`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeEmailConfirmation).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
		mock.ExpectBegin()
		mock.ExpectExec(`active = true`).WithArgs(int64(7)).WillReturnError(errors.New("user"))
		mock.ExpectRollback()
		_, _, err = repo.ConfirmUserByToken(ctx, "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`users.active = false`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeEmailConfirmation).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
		mock.ExpectBegin()
		mock.ExpectExec(`active = true`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnError(errors.New("tok"))
		mock.ExpectRollback()
		_, _, err = repo.ConfirmUserByToken(ctx, "raw")
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewUserRepository(mock)
		mock.ExpectQuery(`users.active = false`).
			WithArgs(pgxmock.AnyArg(), TokenPurposeEmailConfirmation).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "tid"}).AddRow(int64(7), "a@b.c", int64(3)))
		mock.ExpectBegin()
		mock.ExpectExec(`active = true`).WithArgs(int64(7)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`where id = \$1`).WithArgs(int64(3)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, _, err = repo.ConfirmUserByToken(ctx, "raw")
		require.Error(t, err)
	})
}

// TestDeleteExpiredTokens verifica DeleteExpiredTokens no sucesso e erro de delete.
func TestDeleteExpiredTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewUserRepository(mock)
	mock.ExpectExec(`delete from public.users_confirmation_tokens`).
		WillReturnResult(pgxmock.NewResult("DELETE", 2))
	require.NoError(t, repo.DeleteExpiredTokens(ctx))

	mock = newMockPool(t)
	repo = NewUserRepository(mock)
	mock.ExpectExec(`delete from public.users_confirmation_tokens`).WillReturnError(errors.New("x"))
	require.Error(t, repo.DeleteExpiredTokens(ctx))
}

// TestUserPersonalFindByUserID verifica FindByUserID no sucesso, nil quando não há linhas e erro.
func TestUserPersonalFindByUserID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`from\s+public.users_personal_information`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "user_id", "first_name", "last_name", "phone_number", "address_id", "created_at", "updated_at",
			}).AddRow("1", "7", "A", "B", "1", nil, ts(), ts()))
		info, err := repo.FindByUserID(ctx, 7)
		require.NoError(t, err)
		require.Equal(t, "A", info.FirstName.String)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`from\s+public.users_personal_information`).WithArgs(int64(7)).WillReturnError(pgx.ErrNoRows)
		info, err := repo.FindByUserID(ctx, 7)
		require.NoError(t, err)
		require.Nil(t, info)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`from\s+public.users_personal_information`).WithArgs(int64(7)).WillReturnError(errors.New("x"))
		_, err := repo.FindByUserID(ctx, 7)
		require.Error(t, err)
	})
}

// TestUserPersonalCreate verifica Create de dados pessoais no sucesso e erro de insert.
func TestUserPersonalCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`insert into public.users_personal_information`).
			WithArgs(int64(7), "A", "B", "9").
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "user_id", "first_name", "last_name", "phone_number", "created_at",
			}).AddRow("1", "7", "A", "B", "9", ts()))
		info, err := repo.Create(ctx, 7, " A ", " B ", " 9 ")
		require.NoError(t, err)
		require.Equal(t, "A", info.FirstName.String)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`insert into public.users_personal_information`).
			WithArgs(int64(7), "A", "B", "").
			WillReturnError(errors.New("x"))
		_, err := repo.Create(ctx, 7, "A", "B", "")
		require.Error(t, err)
	})
}

// TestUserPersonalUpdate verifica Update de dados pessoais no sucesso, não encontrado e erro.
func TestUserPersonalUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`update public.users_personal_information`).
			WithArgs("A", "B", "9", pgxmock.AnyArg(), 1, int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "user_id", "first_name", "last_name", "phone_number", "created_at", "updated_at",
			}).AddRow("1", "7", "A", "B", "9", ts(), ts()))
		info, err := repo.Update(ctx, 7, 1, " A ", " B ", " 9 ")
		require.NoError(t, err)
		require.NotNil(t, info)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`update public.users_personal_information`).
			WithArgs("A", "B", "", pgxmock.AnyArg(), 1, int64(7)).
			WillReturnError(pgx.ErrNoRows)
		_, err := repo.Update(ctx, 7, 1, "A", "B", "")
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserPersonalRepository(mock)
		mock.ExpectQuery(`update public.users_personal_information`).
			WithArgs("A", "B", "", pgxmock.AnyArg(), 1, int64(7)).
			WillReturnError(errors.New("x"))
		_, err := repo.Update(ctx, 7, 1, "A", "B", "")
		require.Error(t, err)
	})
}

// TestRecoveryCodeReplace verifica Replace de códigos de recuperação na transação e ramos de erro.
func TestRecoveryCodeReplace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users_recovery_codes`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectExec(`insert into public.users_recovery_codes`).
			WithArgs(int64(7), "h1").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`insert into public.users_recovery_codes`).
			WithArgs(int64(7), "h2").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		require.NoError(t, repo.Replace(ctx, 7, []string{"h1", "h2"}))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		require.Error(t, repo.Replace(ctx, 7, nil))
	})

	t.Run("delete error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users_recovery_codes`).WithArgs(int64(7)).WillReturnError(errors.New("del"))
		mock.ExpectRollback()
		require.Error(t, repo.Replace(ctx, 7, nil))
	})

	t.Run("insert error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users_recovery_codes`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectExec(`insert into public.users_recovery_codes`).
			WithArgs(int64(7), "h1").
			WillReturnError(errors.New("ins"))
		mock.ExpectRollback()
		require.Error(t, repo.Replace(ctx, 7, []string{"h1"}))
	})

	t.Run("commit error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`delete from public.users_recovery_codes`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		require.Error(t, repo.Replace(ctx, 7, nil))
	})
}

// TestRecoveryCodeUseAndDelete verifica Use e Delete de códigos de recuperação (sucesso, not found e erro).
func TestRecoveryCodeUseAndDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("use success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectExec(`update public.users_recovery_codes`).
			WithArgs(int64(7), "hash").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		require.NoError(t, repo.Use(ctx, 7, "hash"))
	})

	t.Run("use not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectExec(`update public.users_recovery_codes`).
			WithArgs(int64(7), "hash").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		require.ErrorIs(t, repo.Use(ctx, 7, "hash"), apperrors.ErrNotFound)
	})

	t.Run("use error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectExec(`update public.users_recovery_codes`).
			WithArgs(int64(7), "hash").
			WillReturnError(errors.New("x"))
		require.Error(t, repo.Use(ctx, 7, "hash"))
	})

	t.Run("delete success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectExec(`delete from public.users_recovery_codes`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("DELETE", 2))
		require.NoError(t, repo.Delete(ctx, 7))
	})

	t.Run("delete error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewRecoveryCodeRepository(mock)
		mock.ExpectExec(`delete from public.users_recovery_codes`).
			WithArgs(int64(7)).
			WillReturnError(errors.New("x"))
		require.Error(t, repo.Delete(ctx, 7))
	})
}

// TestUserSessionCreate verifica Create de sessão (revoga duplicata, insert) e erros da transação.
func TestUserSessionCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sid := "11111111-1111-1111-1111-111111111111"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.user_sessions`).
			WithArgs(int64(7), "ua", "1.1.1.1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectExec(`insert into public.user_sessions`).
			WithArgs(int64(7), sid, "ua", "1.1.1.1").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		require.NoError(t, repo.Create(ctx, 7, " "+sid+" ", " ua ", " 1.1.1.1 "))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		require.Error(t, repo.Create(ctx, 7, sid, "ua", ""))
	})

	t.Run("revoke error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.user_sessions`).
			WithArgs(int64(7), "ua", "").
			WillReturnError(errors.New("upd"))
		mock.ExpectRollback()
		require.Error(t, repo.Create(ctx, 7, sid, "ua", ""))
	})

	t.Run("insert error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.user_sessions`).
			WithArgs(int64(7), "ua", "").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectExec(`insert into public.user_sessions`).
			WithArgs(int64(7), sid, "ua", "").
			WillReturnError(errors.New("ins"))
		mock.ExpectRollback()
		require.Error(t, repo.Create(ctx, 7, sid, "ua", ""))
	})

	t.Run("commit error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.user_sessions`).
			WithArgs(int64(7), "ua", "").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectExec(`insert into public.user_sessions`).
			WithArgs(int64(7), sid, "ua", "").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		require.Error(t, repo.Create(ctx, 7, sid, "ua", ""))
	})
}

// TestUserSessionListActive verifica ListActive no sucesso, vazio e erro de query.
func TestUserSessionListActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success default limit", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectQuery(`from\s+public.user_sessions`).
			WithArgs(int64(7), 10).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "user_id", "session_id", "user_agent", "ip", "created_at", "last_seen_at", "revoked_at",
			}).AddRow(int64(1), int64(7), "sid", "ua", "1.1.1.1", ts(), ts(), nil))
		list, err := repo.ListActive(ctx, 7, 0)
		require.NoError(t, err)
		require.Len(t, list, 1)
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectQuery(`from\s+public.user_sessions`).WithArgs(int64(7), 5).WillReturnError(errors.New("q"))
		_, err := repo.ListActive(ctx, 7, 5)
		require.Error(t, err)
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectQuery(`from\s+public.user_sessions`).
			WithArgs(int64(7), 5).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "user_id", "session_id", "user_agent", "ip", "created_at", "last_seen_at", "revoked_at",
			}).AddRow("bad", 1, 2, 3, 4, 5, 6, 7))
		_, err := repo.ListActive(ctx, 7, 5)
		require.Error(t, err)
	})

	t.Run("rows err", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectQuery(`from\s+public.user_sessions`).
			WithArgs(int64(7), 5).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "user_id", "session_id", "user_agent", "ip", "created_at", "last_seen_at", "revoked_at",
			}).AddRow(int64(1), int64(7), "sid", "ua", "", ts(), ts(), nil).CloseError(errors.New("r")))
		_, err := repo.ListActive(ctx, 7, 5)
		require.Error(t, err)
	})
}

// TestUserSessionIsActiveTouchRevoke verifica IsActive, Touch e Revoke/RevokeOthers no sucesso e erros.
func TestUserSessionIsActiveTouchRevoke(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sid := "11111111-1111-1111-1111-111111111111"

	t.Run("is active", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectQuery(`select exists`).
			WithArgs(int64(7), sid).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		ok, err := repo.IsActive(ctx, 7, " "+sid+" ")
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("is active error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectQuery(`select exists`).WithArgs(int64(7), sid).WillReturnError(errors.New("x"))
		_, err := repo.IsActive(ctx, 7, sid)
		require.Error(t, err)
	})

	t.Run("touch", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectExec(`update public.user_sessions`).
			WithArgs(sid).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		require.NoError(t, repo.Touch(ctx, sid))

		mock = newMockPool(t)
		repo = NewUserSessionRepository(mock)
		mock.ExpectExec(`update public.user_sessions`).
			WithArgs(sid).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		require.ErrorIs(t, repo.Touch(ctx, sid), apperrors.ErrNotFound)

		mock = newMockPool(t)
		repo = NewUserSessionRepository(mock)
		mock.ExpectExec(`update public.user_sessions`).WithArgs(sid).WillReturnError(errors.New("x"))
		require.Error(t, repo.Touch(ctx, sid))
	})

	t.Run("revoke other", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectExec(`session_id <> \$2`).
			WithArgs(int64(7), sid).
			WillReturnResult(pgxmock.NewResult("UPDATE", 2))
		require.NoError(t, repo.RevokeOther(ctx, 7, sid))

		mock = newMockPool(t)
		repo = NewUserSessionRepository(mock)
		mock.ExpectExec(`session_id <> \$2`).WithArgs(int64(7), sid).WillReturnError(errors.New("x"))
		require.Error(t, repo.RevokeOther(ctx, 7, sid))
	})

	t.Run("revoke", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewUserSessionRepository(mock)
		mock.ExpectExec(`session_id = \$2`).
			WithArgs(int64(7), sid).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		require.NoError(t, repo.Revoke(ctx, 7, sid))

		mock = newMockPool(t)
		repo = NewUserSessionRepository(mock)
		mock.ExpectExec(`session_id = \$2`).
			WithArgs(int64(7), sid).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		require.ErrorIs(t, repo.Revoke(ctx, 7, sid), apperrors.ErrNotFound)

		mock = newMockPool(t)
		repo = NewUserSessionRepository(mock)
		mock.ExpectExec(`session_id = \$2`).WithArgs(int64(7), sid).WillReturnError(errors.New("x"))
		require.Error(t, repo.Revoke(ctx, 7, sid))
	})
}

// TestTwoFactorGetStatus verifica GetStatus do 2FA no sucesso, não encontrado e erro.
func TestTwoFactorGetStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectQuery(`from\s+public.users`).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "email", "active", "enabled", "method", "secret", "enabled_at", "disabled_at",
			}).AddRow(int64(7), "a@b.c", true, true, "email", []byte("sec"), ts(), nil))
		st, err := repo.GetStatus(ctx, 7)
		require.NoError(t, err)
		require.Equal(t, "email", st.Method)
		require.NotNil(t, st.EnabledAt)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectQuery(`from\s+public.users`).WithArgs(int64(7)).WillReturnError(pgx.ErrNoRows)
		_, err := repo.GetStatus(ctx, 7)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectQuery(`from\s+public.users`).WithArgs(int64(7)).WillReturnError(errors.New("x"))
		_, err := repo.GetStatus(ctx, 7)
		require.Error(t, err)
	})
}

// TestTwoFactorCreateChallenge verifica CreateChallenge no sucesso e erros de insert.
func TestTwoFactorCreateChallenge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.users_two_factor_challenges`).
			WithArgs(int64(7), models.TwoFactorMethodEmail, models.TwoFactorPurposeLogin).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectQuery(`insert into public.users_two_factor_challenges`).
			WithArgs(int64(7), models.TwoFactorMethodEmail, models.TwoFactorPurposeLogin, "hash", nil, pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(3), ts()))
		mock.ExpectCommit()
		ch, err := repo.CreateChallenge(ctx, 7, models.TwoFactorMethodEmail, models.TwoFactorPurposeLogin, "hash", nil, time.Minute)
		require.NoError(t, err)
		require.Equal(t, int64(3), ch.ID)
	})

	t.Run("begin/update/insert/commit errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, err := repo.CreateChallenge(ctx, 7, "email", "login", "h", []byte("s"), time.Minute)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login").
			WillReturnError(errors.New("upd"))
		mock.ExpectRollback()
		_, err = repo.CreateChallenge(ctx, 7, "email", "login", "h", nil, time.Minute)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectQuery(`insert into public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login", "h", []byte("s"), pgxmock.AnyArg()).
			WillReturnError(errors.New("ins"))
		mock.ExpectRollback()
		_, err = repo.CreateChallenge(ctx, 7, "email", "login", "h", []byte("s"), time.Minute)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectQuery(`insert into public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login", nil, nil, pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), ts()))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, err = repo.CreateChallenge(ctx, 7, "email", "login", "", nil, time.Minute)
		require.Error(t, err)
	})
}

// TestTwoFactorGetActiveChallenge verifica GetActiveChallenge no sucesso, não encontrado e erro.
func TestTwoFactorGetActiveChallenge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectQuery(`from\s+public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login").
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "user_id", "method", "purpose", "code_hash", "secret", "expires_at", "consumed_at", "attempt_count", "created_at", "updated_at",
			}).AddRow(int64(1), int64(7), "email", "login", "hash", []byte("s"), ts(), nil, 0, ts(), ts()))
		ch, err := repo.GetActiveChallenge(ctx, 7, "email", "login")
		require.NoError(t, err)
		require.Equal(t, "hash", ch.CodeHash)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectQuery(`from\s+public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login").
			WillReturnError(pgx.ErrNoRows)
		_, err := repo.GetActiveChallenge(ctx, 7, "email", "login")
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectQuery(`from\s+public.users_two_factor_challenges`).
			WithArgs(int64(7), "email", "login").
			WillReturnError(errors.New("x"))
		_, err := repo.GetActiveChallenge(ctx, 7, "email", "login")
		require.Error(t, err)
	})
}

// TestTwoFactorChallengeMutations verifica IncrementActiveChallengeAttempts e ConsumeActiveChallenge.
func TestTwoFactorChallengeMutations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("increment", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectExec(`attempt_count = attempt_count \+ 1`).
			WithArgs(int64(3), 5).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		require.NoError(t, repo.IncrementActiveChallengeAttempts(ctx, 3, 5))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`attempt_count = attempt_count \+ 1`).
			WithArgs(int64(3), 5).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		require.ErrorIs(t, repo.IncrementActiveChallengeAttempts(ctx, 3, 5), apperrors.ErrNotFound)

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`attempt_count = attempt_count \+ 1`).
			WithArgs(int64(3), 5).
			WillReturnError(errors.New("x"))
		require.Error(t, repo.IncrementActiveChallengeAttempts(ctx, 3, 5))
	})

	t.Run("consume", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectExec(`consumed_at = current_timestamp`).
			WithArgs(int64(3), int64(7), "email", "login", 5).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		require.NoError(t, repo.ConsumeActiveChallenge(ctx, 3, 7, "email", "login", 5))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`consumed_at = current_timestamp`).
			WithArgs(int64(3), int64(7), "email", "login", 5).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		require.ErrorIs(t, repo.ConsumeActiveChallenge(ctx, 3, 7, "email", "login", 5), apperrors.ErrNotFound)

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`consumed_at = current_timestamp`).
			WithArgs(int64(3), int64(7), "email", "login", 5).
			WillReturnError(errors.New("x"))
		require.Error(t, repo.ConsumeActiveChallenge(ctx, 3, 7, "email", "login", 5))
	})
}

// TestTwoFactorActivateDisableExpired verifica ActivateEmail/TOTP, Disable e DeleteExpiredChallenges.
func TestTwoFactorActivateDisableExpired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("activate email/totp", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectExec(`two_factor_enabled = true`).
			WithArgs(int64(7), models.TwoFactorMethodEmail, nil).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		require.NoError(t, repo.ActivateEmail(ctx, 7))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`two_factor_enabled = true`).
			WithArgs(int64(7), models.TwoFactorMethodTOTP, []byte("sec")).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		require.NoError(t, repo.ActivateTOTP(ctx, 7, []byte("sec")))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`two_factor_enabled = true`).
			WithArgs(int64(7), models.TwoFactorMethodEmail, nil).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		require.ErrorIs(t, repo.ActivateEmail(ctx, 7), apperrors.ErrUserInactive)

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`two_factor_enabled = true`).
			WithArgs(int64(7), models.TwoFactorMethodEmail, nil).
			WillReturnError(errors.New("x"))
		require.Error(t, repo.ActivateEmail(ctx, 7))
	})

	t.Run("disable", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`two_factor_enabled = false`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`delete from public.users_recovery_codes`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()
		require.NoError(t, repo.Disable(ctx, 7))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		require.Error(t, repo.Disable(ctx, 7))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`two_factor_enabled = false`).WithArgs(int64(7)).WillReturnError(errors.New("upd"))
		mock.ExpectRollback()
		require.Error(t, repo.Disable(ctx, 7))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`two_factor_enabled = false`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectRollback()
		require.ErrorIs(t, repo.Disable(ctx, 7), apperrors.ErrNotFound)

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`two_factor_enabled = false`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`delete from public.users_recovery_codes`).WithArgs(int64(7)).WillReturnError(errors.New("del"))
		mock.ExpectRollback()
		require.Error(t, repo.Disable(ctx, 7))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`two_factor_enabled = false`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`delete from public.users_recovery_codes`).
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		require.Error(t, repo.Disable(ctx, 7))
	})

	t.Run("delete expired", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewTwoFactorRepository(mock)
		mock.ExpectExec(`delete from public.users_two_factor_challenges`).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
		require.NoError(t, repo.DeleteExpiredChallenges(ctx))

		mock = newMockPool(t)
		repo = NewTwoFactorRepository(mock)
		mock.ExpectExec(`delete from public.users_two_factor_challenges`).WillReturnError(errors.New("x"))
		require.Error(t, repo.DeleteExpiredChallenges(ctx))
	})
}
