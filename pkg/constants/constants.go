package constants

import "time"

const (
	DefaultICETimeout = 10 * time.Second
	DefaultStreamID   = "ling-echo"
	DefaultCodec      = "pcmu"
	WebRTCOffer       = "offer"
	WebRTCAnswer      = "answer"
	WebRTCCandidate   = "candidate"
)

const (
	CodecPCMU = "pcmu"
	CodecPCMA = "pcma"
	CodecG722 = "g722"
	CodecOPUS = "opus"
	CodecG711 = "g711"
	// 视频编解码器
	CodecH264 = "h264"
	CodecVP8  = "vp8"
	CodecVP9  = "vp9"
	CodecAV1  = "av1"
)

// Default Value: 1024
const ENV_CONFIG_CACHE_SIZE = "CONFIG_CACHE_SIZE"

// Default Value: 10s
const ENV_CONFIG_CACHE_EXPIRED = "CONFIG_CACHE_EXPIRED"

// Gin session field name
const ENV_SESSION_FIELD = "SESSION_FIELD"

// Session
const ENV_SESSION_SECRET = "SESSION_SECRET"
const ENV_SESSION_EXPIRE_DAYS = "SESSION_EXPIRE_DAYS"

// DB
const ENV_DB_DRIVER = "DB_DRIVER"
const ENV_DSN = "DSN"
const DbField = "_lingecho_db"
const UserField = "_lingecho_uid"
const GroupField = "_lingecho_gid"
const TzField = "_lingecho_tz"
const AssetsField = "_lingecho_assets"
const TemplatesField = "_lingecho_templates"

const KEY_VERIFY_EMAIL_EXPIRED = "VERIFY_EMAIL_EXPIRED"
const KEY_AUTH_TOKEN_EXPIRED = "AUTH_TOKEN_EXPIRED"
const KEY_SITE_NAME = "SITE_NAME"
const KEY_SITE_ADMIN = "SITE_ADMIN"
const KEY_SITE_URL = "SITE_URL"
const KEY_SITE_KEYWORDS = "SITE_KEYWORDS"
const KEY_SITE_DESCRIPTION = "SITE_DESCRIPTION"
const KEY_SITE_GA = "SITE_GA"

const KEY_SITE_LOGO_URL = "SITE_LOGO_URL"
const KEY_SITE_FAVICON_URL = "SITE_FAVICON_URL"
const KEY_SITE_TERMS_URL = "SITE_TERMS_URL"
const KEY_SITE_PRIVACY_URL = "SITE_PRIVACY_URL"
const KEY_SITE_SIGNIN_URL = "SITE_SIGNIN_URL"
const KEY_SITE_SIGNUP_URL = "SITE_SIGNUP_URL"
const KEY_SITE_LOGOUT_URL = "SITE_LOGOUT_URL"
const KEY_SITE_RESET_PASSWORD_URL = "SITE_RESET_PASSWORD_URL"
const KEY_SITE_SIGNIN_API = "SITE_SIGNIN_API"
const KEY_SITE_SIGNUP_API = "SITE_SIGNUP_API"
const KEY_SITE_RESET_PASSWORD_DONE_API = "SITE_RESET_PASSWORD_DONE_API"
const KEY_SITE_LOGIN_NEXT = "SITE_LOGIN_NEXT"
const KEY_SITE_USER_ID_TYPE = "SITE_USER_ID_TYPE"
const KEY_USER_ACTIVATED = "USER_ACTIVATED"
const KEY_STORAGE_KIND = "STORAGE_KIND"

// Search configuration keys
const KEY_SEARCH_ENABLED = "SEARCH_ENABLED"
const KEY_SEARCH_PATH = "SEARCH_PATH"
const KEY_SEARCH_BATCH_SIZE = "SEARCH_BATCH_SIZE"
const KEY_SEARCH_INDEX_SCHEDULE = "SEARCH_INDEX_SCHEDULE"

const ENV_STATIC_PREFIX = "STATIC_PREFIX"
const ENV_STATIC_ROOT = "STATIC_ROOT"
