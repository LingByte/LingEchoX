package constants

// RBAC permission kind values (stored on permissions.kind).
const (
	PermissionKindModule = "module"
	PermissionKindMenu     = "menu"
	PermissionKindButton   = "button"
	PermissionKindAPI      = "api"
	PermissionKindData     = "data"
)

// Built-in permission catalog codes (global RBAC).
const (
	PermAPISIPCallsRead      = "api.sip.calls.read"
	PermAPISIPACDRead        = "api.sip.acd.read"
	PermAPISIPACDWrite       = "api.sip.acd.write"
	PermAPISIPScriptsRead    = "api.sip.scripts.read"
	PermAPISIPScriptsWrite   = "api.sip.scripts.write"
	PermAPISIPCampaignsRead  = "api.sip.campaigns.read"
	PermAPISIPCampaignsWrite = "api.sip.campaigns.write"
	PermAPISIPNumbersRead    = "api.sip.numbers.read"

	PermAPITenantOrgRead   = "api.tenant_org.read"
	PermAPITenantOrgWrite  = "api.tenant_org.write"
	PermAPITenantUsersRead = "api.tenant_users.read"
	PermAPITenantUsersWrite = "api.tenant_users.write"

	PermAPICredentialsRead  = "api.credentials.read"
	PermAPICredentialsWrite = "api.credentials.write"

	PermAPIVoiceRead  = "api.voice.read"
	PermAPIVoiceWrite = "api.voice.write"

	PermMenuWorkspaceOverview = "menu.workspace.overview"
	PermMenuTelRecords        = "menu.tel.records"
	PermMenuTelWebseat        = "menu.tel.webseat"
	PermMenuResPool           = "menu.res.pool"
	PermMenuResOutbound       = "menu.res.outbound"
	PermMenuResScript         = "menu.res.script"
	PermMenuResVoice          = "menu.res.voice"
	PermMenuAccKeys           = "menu.acc.keys"
	PermMenuOrgMembers        = "menu.org.members"
	PermMenuOrgDept           = "menu.org.dept"
	PermMenuOrgRole           = "menu.org.role"
)
