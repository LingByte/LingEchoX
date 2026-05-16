package constants

import "time"

// JWT role claims for tenant and platform principals.
const (
	JWTRoleTenantAdmin   = "tenant_admin"
	JWTRoleTenantMember  = "tenant_member"
	JWTRolePlatformSuper = "platform_super"
)

// TenantAccessTokenTTL is the default tenant/platform login access token lifetime.
const TenantAccessTokenTTL = 24 * time.Hour

// TenantStatusActive / TenantStatusSuspended are tenant lifecycle values.
const (
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
)
