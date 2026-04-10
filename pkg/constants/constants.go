package constants

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

const (
	// sender: *LLMUsageInfo, params: ...any (additional context)
	LLMUsage                        = "llm.usage"
	SigInitSystemConfig             = "system.init"
	SIP_USER_TABLE_NAME             = "sip_users"
	SIP_CALL_TABLE_NAME             = "sip_calls"
	SIP_CAMPAIGN_TABLE_NAME         = "sip_campaigns"
	SIP_CAMPAIGN_CONTACT_TABLE_NAME = "sip_campaign_contacts"
	SIP_CALL_ATTEMPT_TABLE_NAME     = "sip_call_attempts"
	SIP_SCRIPT_RUN_TABLE_NAME       = "sip_script_runs"
	SIP_CAMPAIGN_EVENT_TABLE_NAME   = "sip_campaign_events"
	SIP_SCRIPT_TEMPLATE_TABLE_NAME  = "sip_script_templates"
	ACD_POOL_TARGET_TABLE_NAME      = "acd_pool_targets" // ACD: unified SIP + Web routing pool (targets + weights)
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
const KEY_SITE_NAME = "SITE_NAME"
const KEY_SITE_URL = "SITE_URL"
const KEY_SITE_DESCRIPTION = "SITE_DESCRIPTION"
const KEY_SITE_LOGO_URL = "SITE_LOGO_URL"
const KEY_SITE_FAVICON_URL = "SITE_FAVICON_URL"
const KEY_SITE_TERMS_URL = "SITE_TERMS_URL"
const KEY_SITE_SIGNIN_URL = "SITE_SIGNIN_URL"
const KEY_SITE_SIGNUP_URL = "SITE_SIGNUP_URL"
const KEY_SITE_LOGOUT_URL = "SITE_LOGOUT_URL"
const KEY_SITE_RESET_PASSWORD_URL = "SITE_RESET_PASSWORD_URL"
const KEY_SITE_SIGNIN_API = "SITE_SIGNIN_API"
const KEY_SITE_SIGNUP_API = "SITE_SIGNUP_API"
const KEY_SITE_RESET_PASSWORD_DONE_API = "SITE_RESET_PASSWORD_DONE_API"
const KEY_SITE_LOGIN_NEXT = "SITE_LOGIN_NEXT"
const KEY_SITE_USER_ID_TYPE = "SITE_USER_ID_TYPE"

// Search configuration keys
const KEY_SEARCH_ENABLED = "SEARCH_ENABLED"
const KEY_SEARCH_PATH = "SEARCH_PATH"
const KEY_SEARCH_BATCH_SIZE = "SEARCH_BATCH_SIZE"
const KEY_SEARCH_INDEX_SCHEDULE = "SEARCH_INDEX_SCHEDULE"

const AUTHORIZATION_PREFIX = "Bearer "
const CREDENTIAL_API_KEY = "X-API-KEY"
const CREDENTIAL_API_SECRET = "X-API-SECRET"

const (
	EnvCampaignHTTPAddr  = "SIP_CAMPAIGN_HTTP_ADDR"  // e.g. :9082
	EnvCampaignHTTPToken = "SIP_CAMPAIGN_HTTP_TOKEN" // optional
)
