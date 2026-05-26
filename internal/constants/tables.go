package constants

// GORM table names (shared by models and raw SQL joins).

const (
	SIPUserTableName              = "sip_users"
	SIPCallTableName              = "sip_calls"
	SIPCampaignTableName          = "sip_campaigns"
	SIPCampaignContactTableName   = "sip_campaign_contacts"
	SIPCallAttemptTableName       = "sip_call_attempts"
	SIPScriptRunTableName         = "sip_script_runs"
	SIPCampaignEventTableName     = "sip_campaign_events"
	SIPScriptTemplateTableName    = "sip_script_templates"
	ACDPoolTargetTableName        = "acd_pool_targets"
	SIPACDTransferOfferTableName  = "sip_acd_transfer_offers"
	SIPTrunkTableName             = "sip_trunks"
	SIPTrunkNumberTableName       = "sip_trunk_numbers"
	TenantTableName               = "tenants"
	TenantGroupTableName          = "tenant_groups"
	TenantUserTableName           = "tenant_users"
	TenantUserGroupTableName      = "tenant_user_groups"
	PermissionTableName           = "permissions"
	TenantRoleTableName           = "tenant_roles"
	TenantRolePermissionTableName = "tenant_role_permissions"
	TenantUserRoleTableName       = "tenant_user_roles"
	CredentialTableName           = "credential"
	PlatformAdminTableName        = "platform_admins"
)

// Legacy aliases (avoid breaking imports during migration).
const (
	SIP_USER_TABLE_NAME               = SIPUserTableName
	SIP_CALL_TABLE_NAME               = SIPCallTableName
	SIP_CAMPAIGN_TABLE_NAME           = SIPCampaignTableName
	SIP_CAMPAIGN_CONTACT_TABLE_NAME   = SIPCampaignContactTableName
	SIP_CALL_ATTEMPT_TABLE_NAME       = SIPCallAttemptTableName
	SIP_SCRIPT_RUN_TABLE_NAME         = SIPScriptRunTableName
	SIP_CAMPAIGN_EVENT_TABLE_NAME     = SIPCampaignEventTableName
	SIP_SCRIPT_TEMPLATE_TABLE_NAME    = SIPScriptTemplateTableName
	ACD_POOL_TARGET_TABLE_NAME        = ACDPoolTargetTableName
	SIP_ACD_TRANSFER_OFFER_TABLE_NAME = SIPACDTransferOfferTableName
	SIP_TRUNK_TABLE_NAME              = SIPTrunkTableName
	SIP_TRUNK_NUMBER_TABLE_NAME       = SIPTrunkNumberTableName
	TENANT_TABLE_NAME                 = TenantTableName
	TENANT_GROUP_TABLE_NAME           = TenantGroupTableName
	TENANT_USER_TABLE_NAME            = TenantUserTableName
	TENANT_USER_GROUP_TABLE_NAME      = TenantUserGroupTableName
	PERMISSION_TABLE_NAME             = PermissionTableName
	TENANT_ROLE_TABLE_NAME            = TenantRoleTableName
	TENANT_ROLE_PERMISSION_TABLE_NAME = TenantRolePermissionTableName
	TENANT_USER_ROLE_TABLE_NAME       = TenantUserRoleTableName
	CREDENTIAL_TABLE_NAME             = CredentialTableName
	PLATFORM_ADMIN_TABLE_NAME         = PlatformAdminTableName
)
