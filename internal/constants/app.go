package constants

// App-wide env keys, site config keys, and HTTP header names.

const (
	LLMUsage                      = "llm.usage"
	SigInitSystemConfig           = "system.init"
	ENVConfigCacheSize            = "CONFIG_CACHE_SIZE"
	ENVConfigCacheExpired         = "CONFIG_CACHE_EXPIRED"
	ENVSessionField               = "SESSION_FIELD"
	ENVSessionSecret              = "SESSION_SECRET"
	ENVSessionExpireDays          = "SESSION_EXPIRE_DAYS"
	ENVDBDriver                   = "DB_DRIVER"
	ENVDsn                        = "DSN"
	DbField                       = "_lingecho_db"
	KEYSiteName                   = "SITE_NAME"
	KEYSiteURL                    = "SITE_URL"
	KEYSiteDescription            = "SITE_DESCRIPTION"
	KEYSiteLogoURL                = "SITE_LOGO_URL"
	KEYSiteFaviconURL             = "SITE_FAVICON_URL"
	KEYSiteTermsURL               = "SITE_TERMS_URL"
	KEYSiteSigninURL              = "SITE_SIGNIN_URL"
	KEYSiteSignupURL              = "SITE_SIGNUP_URL"
	KEYSiteLogoutURL              = "SITE_LOGOUT_URL"
	KEYSiteResetPasswordURL       = "SITE_RESET_PASSWORD_URL"
	KEYSiteSigninAPI              = "SITE_SIGNIN_API"
	KEYSiteSignupAPI              = "SITE_SIGNUP_API"
	KEYSiteResetPasswordDoneAPI   = "SITE_RESET_PASSWORD_DONE_API"
	KEYSiteLoginNext              = "SITE_LOGIN_NEXT"
	KEYSiteUserIDType             = "SITE_USER_ID_TYPE"
	AuthorizationPrefix           = "Bearer "
	CredentialAPIKey              = "X-API-KEY"
	CredentialAPISecret           = "X-API-SECRET"
	LingechoWebSeatPathPrefix     = "lingecho/webseat/v1"
	LingechoVoiceDialogPathPrefix = "lingecho/voice-dialog/v1"

	KEYVoiceCloneXunfeiConfig     = "VOICE_CLONE_XUNFEI_CONFIG"
	KEYVoiceCloneVolcengineConfig = "VOICE_CLONE_VOLCENGINE_CONFIG"
)

// Legacy aliases.
const (
	ENV_CONFIG_CACHE_SIZE            = ENVConfigCacheSize
	ENV_CONFIG_CACHE_EXPIRED         = ENVConfigCacheExpired
	ENV_SESSION_FIELD                = ENVSessionField
	ENV_SESSION_SECRET               = ENVSessionSecret
	ENV_SESSION_EXPIRE_DAYS          = ENVSessionExpireDays
	ENV_DB_DRIVER                    = ENVDBDriver
	ENV_DSN                          = ENVDsn
	KEY_SITE_NAME                    = KEYSiteName
	KEY_SITE_URL                     = KEYSiteURL
	KEY_SITE_DESCRIPTION             = KEYSiteDescription
	KEY_SITE_LOGO_URL                = KEYSiteLogoURL
	KEY_SITE_FAVICON_URL             = KEYSiteFaviconURL
	KEY_SITE_TERMS_URL               = KEYSiteTermsURL
	KEY_SITE_SIGNIN_URL              = KEYSiteSigninURL
	KEY_SITE_SIGNUP_URL              = KEYSiteSignupURL
	KEY_SITE_LOGOUT_URL              = KEYSiteLogoutURL
	KEY_SITE_RESET_PASSWORD_URL      = KEYSiteResetPasswordURL
	KEY_SITE_SIGNIN_API              = KEYSiteSigninAPI
	KEY_SITE_SIGNUP_API              = KEYSiteSignupAPI
	KEY_SITE_RESET_PASSWORD_DONE_API = KEYSiteResetPasswordDoneAPI
	KEY_SITE_LOGIN_NEXT              = KEYSiteLoginNext
	KEY_SITE_USER_ID_TYPE            = KEYSiteUserIDType
)
