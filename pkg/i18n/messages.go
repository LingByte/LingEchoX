package i18n

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// Message keys — use with TGin(c, KeyXxx) or response.FailI18n.
const (
	KeySuccess = "common.success"

	KeyInvalidBody           = "common.invalid_body"
	KeyNotFound              = "common.not_found"
	KeyUnauthorized          = "common.unauthorized"
	KeyForbidden             = "common.forbidden"
	KeyInternalError         = "common.internal_error"
	KeyNoFieldsToUpdate      = "common.no_fields_to_update"
	KeyInvalidParams         = "common.invalid_params"
	KeyDatabaseUnavailable   = "common.database_unavailable"

	KeyAuthInvalidCredentials = "auth.invalid_credentials"
	KeyAuthNeedsTotp          = "auth.needs_totp"
	KeyAuthInvalidTotp        = "auth.invalid_totp"
	KeyAuthJWTNotReady        = "auth.jwt_not_ready"
	KeyAuthMissingToken       = "auth.missing_token"
	KeyAuthInvalidToken       = "auth.invalid_token"
	KeyAuthLogoutSuccess      = "auth.logout_success"

	KeyTenantRegisterDisabled = "tenant.register_disabled"
	KeyTenantEmailExists      = "tenant.email_exists"
	KeyTenantInvalidEmail     = "tenant.invalid_email"
	KeyTenantNotFound         = "tenant.not_found"
	KeyTenantSuspended        = "tenant.suspended"
	KeyTenantUserUnavailable  = "tenant.user_unavailable"
	KeyTenantSignTokenFailed  = "tenant.sign_token_failed"

	KeyPermInsufficient           = "perm.insufficient"
	KeyPermInsufficientCode       = "perm.insufficient_code"
	KeyPermInsufficientCredential = "perm.insufficient_credential"
	KeyPermNeedTenantUser         = "perm.need_tenant_user"
	KeyPermNeedTenantContext      = "perm.need_tenant_context"
	KeyPermPlatformNoTenantRBAC   = "perm.platform_no_tenant_rbac"
	KeyPermInvalidCredential      = "perm.invalid_credential"

	KeyCredPermRequired     = "credential.permission_required"
	KeyCredPermEmptyArray   = "credential.permission_empty"
	KeyCredAllowIPRequired  = "credential.allow_ip_required"
	KeyCredAllowIPEmpty     = "credential.allow_ip_empty"
	KeyCredInvalidPerm      = "credential.invalid_permission"
	KeyCredNameEmpty        = "credential.name_empty"
	KeyCredNoticeSecretOnce = "credential.secret_once"

	KeyUploadsRecordingAuth = "uploads.recording_auth_required"

	KeyOrgInvalidPermID  = "org.invalid_permission_id"
	KeyOrgInvalidRoleID  = "org.invalid_role_id"
	KeyOrgAdminRoleFixed = "org.admin_role_fixed"

	KeyPasswordWrong   = "account.password_wrong"
	KeyPasswordChanged = "account.password_changed"
	KeyTotpAlreadyOn   = "account.totp_already_on"
	KeyTotpNotOn       = "account.totp_not_on"
	KeyTotpInvalidCode = "account.totp_invalid"
	KeyTotpEnabled     = "account.totp_enabled"
	KeyTotpDisabled    = "account.totp_disabled"
	KeyTotpSetupFirst  = "account.totp_setup_first"

	KeyValidationUsernameShort  = "validation.username_short"
	KeyValidationUsernameFormat = "validation.username_format"
	KeyValidationPasswordShort    = "validation.password_short"
	KeyValidationCaptchaRequired  = "validation.captcha_required"
	KeyValidationCaptchaInvalid   = "validation.captcha_invalid"
)

var zhCN = map[string]string{
	KeySuccess: "success",

	KeyInvalidBody:           "请求参数无效",
	KeyNotFound:              "未找到",
	KeyUnauthorized:          "未授权",
	KeyForbidden:             "禁止访问",
	KeyInternalError:         "服务器内部错误",
	KeyNoFieldsToUpdate:      "没有可更新的字段",
	KeyInvalidParams:         "请求参数无效",
	KeyDatabaseUnavailable:   "数据库不可用",

	KeyAuthInvalidCredentials: "邮箱或密码错误",
	KeyAuthNeedsTotp:          "需要两步验证码",
	KeyAuthInvalidTotp:        "两步验证码错误",
	KeyAuthJWTNotReady:        "服务未就绪：JWT 密钥未初始化",
	KeyAuthMissingToken:       "缺少授权令牌",
	KeyAuthInvalidToken:       "令牌无效或已过期",
	KeyAuthLogoutSuccess:      "已退出登录",

	KeyTenantRegisterDisabled: "自助注册已关闭，请联系管理员开通租户",
	KeyTenantEmailExists:      "该邮箱已被注册",
	KeyTenantInvalidEmail:     "邮箱格式错误",
	KeyTenantNotFound:         "组织不存在或已被停用",
	KeyTenantSuspended:        "组织已暂停服务",
	KeyTenantUserUnavailable:  "账号不可用",
	KeyTenantSignTokenFailed:  "签发登录凭证失败",

	KeyPermInsufficient:           "权限不足",
	KeyPermInsufficientCode:       "权限不足：%s",
	KeyPermInsufficientCredential: "权限不足（访问密钥）",
	KeyPermNeedTenantUser:         "需要租户用户登录",
	KeyPermNeedTenantContext:      "需要租户上下文",
	KeyPermPlatformNoTenantRBAC:   "平台管理员请使用平台专用接口，不能调用租户 RBAC 接口",
	KeyPermInvalidCredential:      "访问密钥无效",

	KeyCredPermRequired:     "permissionCodes 必填，至少指定一项权限（不可用空数组或省略）",
	KeyCredAllowIPRequired:  "allowIp 必填（逗号分隔客户端 IP）；开发环境可设 CREDENTIAL_ALLOW_EMPTY_ALLOW_IP=true",
	KeyCredPermEmptyArray:   "permissionCodes 不能为空数组",
	KeyCredAllowIPEmpty:     "allowIp 不能为空",
	KeyCredInvalidPerm:      "permissionCodes 无效",
	KeyCredNameEmpty:        "名称不能为空",
	KeyCredNoticeSecretOnce: "secretKey 仅显示一次，请妥善保存",

	KeyUploadsRecordingAuth: "访问录音需要 Bearer 登录；开发环境可设 UPLOADS_RECORDINGS_PUBLIC=true（勿用于生产）",

	KeyOrgInvalidPermID:  "无效的权限 id",
	KeyOrgInvalidRoleID:  "无效的角色 id",
	KeyOrgAdminRoleFixed: "系统「管理员」角色固定拥有全部权限，不可在此修改",

	KeyPasswordWrong:   "密码错误",
	KeyPasswordChanged: "密码修改成功，请重新登录",
	KeyTotpAlreadyOn:   "两步验证已开启",
	KeyTotpNotOn:       "两步验证未开启",
	KeyTotpInvalidCode: "验证码无效",
	KeyTotpEnabled:     "两步验证已开启",
	KeyTotpDisabled:    "已关闭两步验证",
	KeyTotpSetupFirst:  "请先生成二维码",

	KeyValidationUsernameShort:  "用户名至少需要2个字符",
	KeyValidationUsernameFormat: "用户名只能包含字母（包括中文）、数字、下划线和连字符",
	KeyValidationPasswordShort:  "密码至少需要8个字符",
	KeyValidationCaptchaRequired: "请输入验证码",
	KeyValidationCaptchaInvalid:  "验证码错误",
}

var enUS = map[string]string{
	KeySuccess: "success",

	KeyInvalidBody:         "Invalid request body",
	KeyNotFound:            "Not found",
	KeyUnauthorized:        "Unauthorized",
	KeyForbidden:           "Forbidden",
	KeyInternalError:       "Internal server error",
	KeyNoFieldsToUpdate:    "No fields to update",
	KeyInvalidParams:       "Invalid parameters",
	KeyDatabaseUnavailable: "Database unavailable",

	KeyAuthInvalidCredentials: "Invalid email or password",
	KeyAuthNeedsTotp:          "Two-factor code required",
	KeyAuthInvalidTotp:        "Invalid two-factor code",
	KeyAuthJWTNotReady:        "Service not ready: JWT keys not initialized",
	KeyAuthMissingToken:       "Missing authorization token",
	KeyAuthInvalidToken:       "Invalid or expired token",
	KeyAuthLogoutSuccess:      "Logged out",

	KeyTenantRegisterDisabled: "Self-registration is disabled; contact your administrator",
	KeyTenantEmailExists:      "This email is already registered",
	KeyTenantInvalidEmail:     "Invalid email format",
	KeyTenantNotFound:         "Organization not found or disabled",
	KeyTenantSuspended:        "Organization is suspended",
	KeyTenantUserUnavailable:  "Account is not available",
	KeyTenantSignTokenFailed:  "Failed to issue access token",

	KeyPermInsufficient:           "Insufficient permissions",
	KeyPermInsufficientCode:       "Insufficient permission: %s",
	KeyPermInsufficientCredential: "Insufficient permissions (access key)",
	KeyPermNeedTenantUser:         "Tenant user login required",
	KeyPermNeedTenantContext:      "Tenant context required",
	KeyPermPlatformNoTenantRBAC:   "Platform admins must use platform APIs, not tenant RBAC routes",
	KeyPermInvalidCredential:      "Invalid access key",

	KeyCredPermRequired:     "permissionCodes is required with at least one permission",
	KeyCredAllowIPRequired:  "allowIp is required (comma-separated IPs); set CREDENTIAL_ALLOW_EMPTY_ALLOW_IP=true for dev only",
	KeyCredPermEmptyArray:   "permissionCodes cannot be an empty array",
	KeyCredAllowIPEmpty:     "allowIp cannot be empty",
	KeyCredInvalidPerm:      "Invalid permissionCodes",
	KeyCredNameEmpty:        "Name cannot be empty",
	KeyCredNoticeSecretOnce: "secretKey is shown once; store it safely",

	KeyUploadsRecordingAuth: "Recording access requires Bearer authentication",

	KeyOrgInvalidPermID:  "Invalid permission id",
	KeyOrgInvalidRoleID:  "Invalid role id",
	KeyOrgAdminRoleFixed: "The system Admin role has all permissions and cannot be edited here",

	KeyPasswordWrong:   "Incorrect password",
	KeyPasswordChanged: "Password updated; please sign in again",
	KeyTotpAlreadyOn:   "Two-factor authentication is already enabled",
	KeyTotpNotOn:       "Two-factor authentication is not enabled",
	KeyTotpInvalidCode: "Invalid verification code",
	KeyTotpEnabled:     "Two-factor authentication enabled",
	KeyTotpDisabled:    "Two-factor authentication disabled",
	KeyTotpSetupFirst:  "Generate a QR code first",

	KeyValidationUsernameShort:  "Username must be at least 2 characters",
	KeyValidationUsernameFormat: "Username may only contain letters, numbers, underscores, and hyphens",
	KeyValidationPasswordShort:  "Password must be at least 8 characters",
	KeyValidationCaptchaRequired: "Captcha is required",
	KeyValidationCaptchaInvalid:  "Invalid captcha",
}

func catalog(locale string) map[string]string {
	if NormalizeLocale(locale) == LocaleEnUS {
		return enUS
	}
	return zhCN
}

// T returns a localized string for the given locale and key.
func T(locale, key string, args ...any) string {
	msg, ok := catalog(locale)[key]
	if !ok || msg == "" {
		if fb, ok := zhCN[key]; ok {
			msg = fb
		} else {
			return key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// TGin resolves locale from gin context.
func TGin(c *gin.Context, key string, args ...any) string {
	return T(LocaleFromGin(c), key, args...)
}
