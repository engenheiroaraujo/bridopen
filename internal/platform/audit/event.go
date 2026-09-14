package audit

import "fmt"

// Event representa os dados que a aplicacao entrega para auditoria.sp_events_insert.
// A procedure resolve o tipo do evento, a mensagem padrao e a gravacao final.
type Event struct {
	Code      string
	UserID    int64
	IP        string
	Operation string
	Object    string
	Module    string
	Message   string
	Result    string
}

const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

const (
	CategoryAuthentication = "autenticacao"
	CategoryUser           = "usuario"
	CategoryPermission     = "permissao"
	CategoryData           = "dados"
	CategoryFile           = "arquivo"
	CategoryDocument       = "documento"
	CategoryIntegration    = "integracao"
	CategorySystem         = "sistema"
	CategoryBackup         = "backup"
	CategorySecurity       = "seguranca"
	CategoryError          = "erro"
	CategoryPrivacy        = "privacidade"
	CategoryAudit          = "auditoria"
)

const (
	OperationLogin         = "login"
	OperationLogout        = "logout"
	OperationPasswordReset = "password_reset"
	OperationCreate        = "create"
	OperationConfirm       = "confirm"
	OperationExport        = "export"
	OperationDelete        = "delete"
	OperationUpdate        = "update"
	OperationProcess       = "process"
)

const (
	ModuleApp   = "app"
	ModuleAuth  = "auth"
	ModuleNotes = "notes"
	ModuleUsers = "users"
)

const (
	EntityUser           = "user"
	EntityPersonalData   = "personaldata"
	EntityNote           = "note"
	EntityNoteTag        = "note_tag"
	EntityNoteAttachment = "note_attachment"
)

const (
	EventAuthLoginSuccess    = "AUTH001"
	EventAuthLoginFailure    = "AUTH002"
	EventAuthLogout          = "AUTH003"
	EventAuthSessionExpired  = "AUTH004"
	EventAuthAccountBlocked  = "AUTH005"
	EventAuthAccountUnlocked = "AUTH006"
)

const (
	EventMFAValidated      = "MFA001"
	EventMFAInvalid        = "MFA002"
	EventMFASetupStarted   = "MFA003"
	EventMFAEnabled        = "MFA004"
	EventMFAEnableFailed   = "MFA005"
	EventMFALoginValidated = "MFA006"
	EventMFALoginFailed    = "MFA007"
	EventMFADisabled       = "MFA008"
	EventMFADisableFailed  = "MFA009"
	EventMFADisableStarted = "MFA010"
)

const (
	EventUserCreated                = "USER001"
	EventUserUpdated                = "USER002"
	EventUserDeleted                = "USER003"
	EventUserPasswordChanged        = "USER004"
	EventUserPasswordResetRequested = "USER005"
)

const (
	EventPermissionProfileAssigned = "PERM001"
	EventPermissionProfileRemoved  = "PERM002"
	EventPermissionGranted         = "PERM003"
	EventPermissionRevoked         = "PERM004"
	EventPermissionUnauthorized    = "PERM005"
)

const (
	EventDataCreated         = "DATA001"
	EventDataUpdated         = "DATA002"
	EventDataDeleted         = "DATA003"
	EventDataViewed          = "DATA004"
	EventDataRestored        = "DATA005"
	EventDataImportCompleted = "DATA006"
	EventDataImportFailed    = "DATA007"
	EventDataExportCompleted = "DATA008"
	EventDataExportFailed    = "DATA009"
)

const (
	EventFileUploaded       = "FILE001"
	EventFileUpdated        = "FILE002"
	EventFileDeleted        = "FILE003"
	EventFileDownloaded     = "FILE004"
	EventFileInvalid        = "FILE005"
	EventFileTooLarge       = "FILE006"
	EventFileThreatDetected = "FILE007"
)

const (
	EventDocumentSigned           = "DOC001"
	EventDocumentValidated        = "DOC002"
	EventDocumentValidationFailed = "DOC003"
	EventDocumentCancelled        = "DOC004"
)

const (
	EventIntegrationStarted         = "INT001"
	EventIntegrationCompleted       = "INT002"
	EventIntegrationFailed          = "INT003"
	EventIntegrationExternalTimeout = "INT004"
	EventIntegrationInvalidResponse = "INT005"
	EventIntegrationUnavailable     = "INT006"
)

const (
	EventSystemStarted            = "SYS001"
	EventSystemFinished           = "SYS002"
	EventSystemServiceStarted     = "SYS003"
	EventSystemServiceInterrupted = "SYS004"
	EventSystemConfigChanged      = "SYS005"
	EventSystemVersionUpdated     = "SYS006"
)

const (
	EventBackupStarted         = "BKP001"
	EventBackupCompleted       = "BKP002"
	EventBackupFailed          = "BKP003"
	EventBackupRestoreStarted  = "BKP004"
	EventBackupRestoreComplete = "BKP005"
	EventBackupRestoreFailed   = "BKP006"
)

const (
	EventSecurityAccessBlocked      = "SEC001"
	EventSecurityRestrictedResource = "SEC002"
	EventSecurityTokenGenerated     = "SEC003"
	EventSecurityTokenRevoked       = "SEC004"
	EventSecurityCertificateValid   = "SEC005"
	EventSecurityCertificateExpired = "SEC006"
	EventSecurityCryptoKeyGenerated = "SEC007"
	EventSecurityCryptoKeyRevoked   = "SEC008"
	EventSecurityIncidentReported   = "SEC009"
)

const (
	EventErrorValidation        = "ERR001"
	EventErrorProcessing        = "ERR002"
	EventErrorDatabase          = "ERR003"
	EventErrorExternal          = "ERR004"
	EventErrorUnexpected        = "ERR005"
	EventErrorOperationCanceled = "ERR006"
	EventErrorResourceNotFound  = "ERR007"
	EventErrorLimitExceeded     = "ERR008"
)

const (
	EventLGPDConsentGranted = "LGPD001"
	EventLGPDConsentRevoked = "LGPD002"
	EventLGPDDataAnonymized = "LGPD003"
	EventLGPDSubjectRequest = "LGPD004"
	EventLGPDTermsAccepted  = "LGPD005"
)

const (
	EventAuditTrailViewed   = "AUD001"
	EventAuditTrailExported = "AUD002"
)

func NewEvent(userID int64, eventCode, entityType string, entityID int64, result string) Event {
	if result == "" {
		result = ResultSuccess
	}

	return Event{
		Code:      eventCode,
		UserID:    userID,
		Operation: operationFor(eventCode),
		Object:    objectRef(entityType, entityID),
		Module:    moduleFor(eventCode, entityType),
		Result:    result,
	}
}

func operationFor(eventCode string) string {
	switch eventCode {
	case EventAuthLoginSuccess, EventAuthLoginFailure, EventMFALoginValidated, EventMFALoginFailed:
		return OperationLogin
	case EventAuthLogout, EventAuthSessionExpired:
		return OperationLogout
	case EventUserPasswordChanged, EventUserPasswordResetRequested:
		return OperationPasswordReset
	case EventMFASetupStarted, EventUserCreated, EventDataCreated, EventFileUploaded, EventPermissionProfileAssigned, EventPermissionGranted, EventIntegrationStarted, EventSystemStarted, EventSystemServiceStarted, EventBackupStarted, EventBackupRestoreStarted, EventLGPDConsentGranted, EventLGPDSubjectRequest, EventLGPDTermsAccepted:
		return OperationCreate
	case EventMFAValidated, EventMFAEnabled, EventMFADisableStarted, EventDocumentValidated, EventSecurityCertificateValid:
		return OperationConfirm
	case EventDataExportCompleted, EventAuditTrailExported:
		return OperationExport
	case EventMFADisabled, EventUserDeleted, EventDataDeleted, EventFileDeleted, EventPermissionProfileRemoved, EventPermissionRevoked:
		return OperationDelete
	case EventUserUpdated, EventDataUpdated, EventFileUpdated, EventSystemConfigChanged, EventSystemVersionUpdated:
		return OperationUpdate
	default:
		return OperationProcess
	}
}

func moduleFor(eventCode, entityType string) string {
	switch eventCode {
	case EventAuthLoginSuccess, EventAuthLoginFailure, EventAuthLogout, EventAuthSessionExpired, EventAuthAccountBlocked, EventAuthAccountUnlocked, EventMFAValidated, EventMFAInvalid, EventMFASetupStarted, EventMFAEnabled, EventMFAEnableFailed, EventMFALoginValidated, EventMFALoginFailed, EventMFADisabled, EventMFADisableFailed, EventMFADisableStarted, EventUserPasswordChanged, EventUserPasswordResetRequested:
		return ModuleAuth
	}
	switch entityType {
	case EntityNote, EntityNoteTag, EntityNoteAttachment:
		return ModuleNotes
	case EntityUser, EntityPersonalData:
		return ModuleUsers
	default:
		return ModuleApp
	}
}

func objectRef(entityType string, entityID int64) string {
	if entityType == "" {
		return ""
	}
	if entityID == 0 {
		return entityType
	}
	return fmt.Sprintf("%s:%d", entityType, entityID)
}
