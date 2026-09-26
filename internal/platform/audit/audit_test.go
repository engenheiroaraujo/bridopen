package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEventDefaultsResultAndMapsFields verifica defaults de Result e o mapeamento de operação, módulo e object em NewEvent.
func TestNewEventDefaultsResultAndMapsFields(t *testing.T) {
	t.Parallel()

	event := NewEvent(10, EventMFAEnabled, EntityUser, 10, "")
	assert.Equal(t, ResultSuccess, event.Result)
	assert.Equal(t, OperationConfirm, event.Operation)
	assert.Equal(t, ModuleAuth, event.Module)
	assert.Equal(t, "user:10", event.Object)

	failed := NewEvent(1, EventMFADisableFailed, EntityUser, 1, ResultFailure)
	assert.Equal(t, ResultFailure, failed.Result)
	assert.Equal(t, OperationProcess, failed.Operation)
}

// TestOperationForAllBranches verifica o mapeamento de códigos de evento para Operation em todos os ramos.
func TestOperationForAllBranches(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		EventAuthLoginSuccess:           OperationLogin,
		EventAuthLoginFailure:           OperationLogin,
		EventMFALoginValidated:          OperationLogin,
		EventMFALoginFailed:             OperationLogin,
		EventAuthLogout:                 OperationLogout,
		EventAuthSessionExpired:         OperationLogout,
		EventUserPasswordChanged:        OperationPasswordReset,
		EventUserPasswordResetRequested: OperationPasswordReset,
		EventMFASetupStarted:            OperationCreate,
		EventUserCreated:                OperationCreate,
		EventDataCreated:                OperationCreate,
		EventFileUploaded:               OperationCreate,
		EventPermissionProfileAssigned:  OperationCreate,
		EventPermissionGranted:          OperationCreate,
		EventIntegrationStarted:         OperationCreate,
		EventSystemStarted:              OperationCreate,
		EventSystemServiceStarted:       OperationCreate,
		EventBackupStarted:              OperationCreate,
		EventBackupRestoreStarted:       OperationCreate,
		EventLGPDConsentGranted:         OperationCreate,
		EventLGPDSubjectRequest:         OperationCreate,
		EventLGPDTermsAccepted:          OperationCreate,
		EventMFAValidated:               OperationConfirm,
		EventMFAEnabled:                 OperationConfirm,
		EventMFADisableStarted:          OperationConfirm,
		EventDocumentValidated:          OperationConfirm,
		EventSecurityCertificateValid:   OperationConfirm,
		EventDataExportCompleted:        OperationExport,
		EventAuditTrailExported:         OperationExport,
		EventMFADisabled:                OperationDelete,
		EventUserDeleted:                OperationDelete,
		EventDataDeleted:                OperationDelete,
		EventFileDeleted:                OperationDelete,
		EventPermissionProfileRemoved:   OperationDelete,
		EventPermissionRevoked:          OperationDelete,
		EventUserUpdated:                OperationUpdate,
		EventDataUpdated:                OperationUpdate,
		EventFileUpdated:                OperationUpdate,
		EventSystemConfigChanged:        OperationUpdate,
		EventSystemVersionUpdated:       OperationUpdate,
		EventErrorUnexpected:            OperationProcess,
	}

	for code, want := range cases {
		assert.Equal(t, want, operationFor(code), code)
	}
}

// TestModuleForAllBranches verifica o mapeamento de módulo conforme código do evento e tipo de entidade.
func TestModuleForAllBranches(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ModuleAuth, moduleFor(EventAuthLoginSuccess, EntityNote))
	assert.Equal(t, ModuleAuth, moduleFor(EventMFAEnabled, EntityUser))
	assert.Equal(t, ModuleAuth, moduleFor(EventUserPasswordChanged, ""))
	assert.Equal(t, ModuleNotes, moduleFor(EventDataCreated, EntityNote))
	assert.Equal(t, ModuleNotes, moduleFor(EventDataCreated, EntityNoteTag))
	assert.Equal(t, ModuleNotes, moduleFor(EventDataCreated, EntityNoteAttachment))
	assert.Equal(t, ModuleUsers, moduleFor(EventDataCreated, EntityUser))
	assert.Equal(t, ModuleUsers, moduleFor(EventDataCreated, EntityPersonalData))
	assert.Equal(t, ModuleApp, moduleFor(EventDataCreated, "other"))
}

// TestObjectRef verifica a formatação da referência de objeto (entity e id).
func TestObjectRef(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", objectRef("", 10))
	assert.Equal(t, "user", objectRef(EntityUser, 0))
	assert.Equal(t, "note:7", objectRef(EntityNote, 7))
}

type fakeAuditRepo struct {
	events []Event
	err    error
}

func (f *fakeAuditRepo) Record(_ context.Context, event Event) error {
	f.events = append(f.events, event)
	return f.err
}

// TestNewRecorderNoop verifica que NewRecorder(nil) retorna um recorder no-op seguro.
func TestNewRecorderNoop(t *testing.T) {
	t.Parallel()

	rec := NewRecorder(nil)
	require.IsType(t, noopRecorder{}, rec)
	rec.Record(context.Background(), Event{Code: EventAuthLogout})
	rec.RecordSuccess(context.Background(), 1, EventAuthLogout, EntityUser, 1)
	rec.RecordFailure(context.Background(), 1, EventAuthLoginFailure, EntityUser, 1)
}

// TestRecorderSuccessAndFailure verifica que RecordSuccess e RecordFailure gravam Result correto no repositório.
func TestRecorderSuccessAndFailure(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{}
	rec := NewRecorder(repo)
	rec.RecordSuccess(context.Background(), 9, EventMFAEnabled, EntityUser, 9)
	rec.RecordFailure(context.Background(), 9, EventMFAEnableFailed, EntityUser, 9)

	require.Len(t, repo.events, 2)
	assert.Equal(t, ResultSuccess, repo.events[0].Result)
	assert.Equal(t, ResultFailure, repo.events[1].Result)
}

// TestRecorderLogsWarningOnRepoError verifica que erro do repositório não interrompe o fluxo do recorder.
func TestRecorderLogsWarningOnRepoError(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{err: errors.New("db down")}
	rec := NewRecorder(repo)
	rec.Record(context.Background(), Event{Code: EventErrorDatabase})
	require.Len(t, repo.events, 1)
}

// TestNullableHelpers verifica que nullableInt64 e nullableString convertem zero/vazio em nil.
func TestNullableHelpers(t *testing.T) {
	t.Parallel()

	assert.Nil(t, nullableInt64(0))
	assert.Equal(t, int64(3), nullableInt64(3))
	assert.Nil(t, nullableString(""))
	assert.Equal(t, "ok", nullableString("ok"))
}

// TestNewPostgresRepositoryAndRecord verifica criação do repositório e gravação bem-sucedida via sp_events_insert.
func TestNewPostgresRepositoryAndRecord(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewPostgresRepository(mock)
	require.NotNil(t, repo)

	mock.ExpectExec(`call auditoria.sp_events_insert`).
		WithArgs(
			EventMFAEnabled,
			int64(10),
			"127.0.0.1",
			OperationConfirm,
			"user:10",
			ModuleAuth,
			nil,
			ResultSuccess,
		).
		WillReturnResult(pgxmock.NewResult("CALL", 1))

	err = repo.Record(context.Background(), Event{
		Code:      "  " + EventMFAEnabled + "  ",
		UserID:    10,
		IP:        "127.0.0.1",
		Operation: OperationConfirm,
		Object:    "user:10",
		Module:    ModuleAuth,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPostgresRepositoryRecordValidationAndError verifica rejeição de código vazio e propagação de erro do banco ao gravar.
func TestPostgresRepositoryRecordValidationAndError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewPostgresRepository(mock)
	err = repo.Record(context.Background(), Event{Code: "   "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event code vazio")

	mock.ExpectExec(`call auditoria.sp_events_insert`).
		WithArgs(EventErrorDatabase, nil, nil, nil, nil, nil, nil, ResultSuccess).
		WillReturnError(assert.AnError)

	err = repo.Record(context.Background(), Event{Code: EventErrorDatabase})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit record")
	require.NoError(t, mock.ExpectationsWereMet())
}
