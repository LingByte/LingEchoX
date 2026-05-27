package constants

// Security-related environment variable names (fail-closed unless explicitly true).

const (
	ENVUploadsRecordingsPublic      = "UPLOADS_RECORDINGS_PUBLIC"
	ENVWebSeatAllowEmptyToken       = "SIP_WEBSEAT_ALLOW_EMPTY_TOKEN"
	ENVVoiceDialogAllowEmptyToken   = "VOICE_DIALOG_ALLOW_EMPTY_TOKEN"
	ENVTenantSelfRegister           = "TENANT_SELF_REGISTER"
	ENVCredentialAllowEmptyAllowIP  = "CREDENTIAL_ALLOW_EMPTY_ALLOW_IP" // dev-only: AK/SK without IP allowlist
)
