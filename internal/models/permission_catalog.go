package models

import (
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// RBAC 权限类型：模块（折叠分组）、菜单（页面）、按钮、接口、数据范围
const (
	PermissionKindModule = "module"
	PermissionKindMenu   = "menu"
	PermissionKindButton = "button"
	PermissionKindAPI    = "api"
	PermissionKindData   = "data"
)

// PermissionCatalogRow 全局权限目录中的一条定义（按 code 幂等同步）。
type PermissionCatalogRow struct {
	Code        string
	Name        string
	Description string
	Kind        string
	ParentCode  string
	Resource    string
	Action      string
}

// DefaultPermissionCatalog 树形目录：模块 → 页面菜单 → 按钮 / 接口 / 数据（接口权限挂在对应页面下，不单列为孤立分组）。
// 说明：平台管理员专属路由（如租户管理、SIP 中继维护）不在此目录分配；个人资料修改不为租户 RBAC 项。
func DefaultPermissionCatalog() []PermissionCatalogRow {
	return []PermissionCatalogRow{
		// --- 访问与安全 ---
		{Kind: PermissionKindModule, Code: "mod.acc", Name: "访问与安全", ParentCode: ""},
		{Kind: PermissionKindMenu, Code: "menu.acc.keys", Name: "访问管理", ParentCode: "mod.acc", Resource: "/access-keys", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.acc.keys.view", Name: "查看密钥", ParentCode: "menu.acc.keys", Resource: "credentials", Action: "view"},
		{Kind: PermissionKindButton, Code: "btn.acc.keys.create", Name: "新建访问密钥", ParentCode: "menu.acc.keys", Resource: "credentials", Action: "create"},
		{Kind: PermissionKindButton, Code: "btn.acc.keys.delete", Name: "删除访问密钥", ParentCode: "menu.acc.keys", Resource: "credentials", Action: "delete"},
		{Kind: PermissionKindAPI, Code: "api.credentials.read", Name: "密钥·读", ParentCode: "menu.acc.keys", Resource: "credentials", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.credentials.write", Name: "密钥·写", ParentCode: "menu.acc.keys", Resource: "credentials", Action: "write"},

		// --- 租户与组织 ---
		{Kind: PermissionKindModule, Code: "mod.org", Name: "租户与组织", ParentCode: ""},
		{Kind: PermissionKindMenu, Code: "menu.org.members", Name: "成员管理", ParentCode: "mod.org", Resource: "/tenant-members", Action: "page"},
		{Kind: PermissionKindAPI, Code: "api.tenant_users.read", Name: "租户成员·读", ParentCode: "menu.org.members", Resource: "tenant-users", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.tenant_users.write", Name: "租户成员·写", ParentCode: "menu.org.members", Resource: "tenant-users", Action: "write"},
		{Kind: PermissionKindMenu, Code: "menu.org.dept", Name: "部门", ParentCode: "mod.org", Resource: "/departments", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.org.dept.view", Name: "查看部门", ParentCode: "menu.org.dept", Resource: "tenant-org.groups", Action: "view"},
		{Kind: PermissionKindButton, Code: "btn.org.dept.create", Name: "新建部门", ParentCode: "menu.org.dept", Resource: "tenant-org.groups", Action: "create"},
		{Kind: PermissionKindButton, Code: "btn.org.dept.edit", Name: "编辑部门", ParentCode: "menu.org.dept", Resource: "tenant-org.groups", Action: "edit"},
		{Kind: PermissionKindButton, Code: "btn.org.dept.delete", Name: "删除部门", ParentCode: "menu.org.dept", Resource: "tenant-org.groups", Action: "delete"},
		{Kind: PermissionKindButton, Code: "btn.org.dept.assign", Name: "分配部门", ParentCode: "menu.org.dept", Resource: "tenant-org.groups", Action: "assign"},
		{Kind: PermissionKindAPI, Code: "api.tenant_org.read", Name: "组织权限·读", ParentCode: "menu.org.dept", Resource: "tenant-org", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.tenant_org.write", Name: "组织权限·写", ParentCode: "menu.org.dept", Resource: "tenant-org", Action: "write"},
		{Kind: PermissionKindMenu, Code: "menu.org.role", Name: "角色与权限", ParentCode: "mod.org", Resource: "/role-permissions", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.org.role.view", Name: "查看角色", ParentCode: "menu.org.role", Resource: "tenant-org.roles", Action: "view"},
		{Kind: PermissionKindButton, Code: "btn.org.role.create", Name: "新建角色", ParentCode: "menu.org.role", Resource: "tenant-org.roles", Action: "create"},
		{Kind: PermissionKindButton, Code: "btn.org.role.edit", Name: "编辑角色", ParentCode: "menu.org.role", Resource: "tenant-org.roles", Action: "edit"},
		{Kind: PermissionKindButton, Code: "btn.org.role.delete", Name: "删除角色", ParentCode: "menu.org.role", Resource: "tenant-org.roles", Action: "delete"},
		{Kind: PermissionKindButton, Code: "btn.org.role.assign_perm", Name: "分配权限", ParentCode: "menu.org.role", Resource: "tenant-org.roles", Action: "assign_perm"},
		{Kind: PermissionKindButton, Code: "btn.org.role.assign_user", Name: "分配角色/部门", ParentCode: "menu.org.role", Resource: "tenant-org.roles", Action: "assign_user"},

		// --- 接口与数据策略（数据范围） ---
		{Kind: PermissionKindModule, Code: "mod.policy", Name: "接口与数据策略", ParentCode: ""},
		{Kind: PermissionKindMenu, Code: "menu.policy.data", Name: "数据范围", ParentCode: "mod.policy", Resource: "", Action: "group"},
		{Kind: PermissionKindData, Code: "data.scope.all", Name: "全部数据", ParentCode: "menu.policy.data", Resource: "scope", Action: "all"},
		{Kind: PermissionKindData, Code: "data.scope.department", Name: "本部门", ParentCode: "menu.policy.data", Resource: "scope", Action: "department"},
		{Kind: PermissionKindData, Code: "data.scope.self", Name: "本人", ParentCode: "menu.policy.data", Resource: "scope", Action: "self"},

		// --- 号码与任务 ---
		{Kind: PermissionKindModule, Code: "mod.res", Name: "号码与任务", ParentCode: ""},
		{Kind: PermissionKindMenu, Code: "menu.res.pool", Name: "号码池", ParentCode: "mod.res", Resource: "/number-pool", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.res.pool.view", Name: "查看号码池", ParentCode: "menu.res.pool", Resource: "number-pool", Action: "view"},
		{Kind: PermissionKindAPI, Code: "api.sip.numbers.read", Name: "号码·读", ParentCode: "menu.res.pool", Resource: "sip-center.trunk-numbers", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.sip.numbers.write", Name: "号码·写", ParentCode: "menu.res.pool", Resource: "sip-center.trunk-numbers", Action: "write"},
		{Kind: PermissionKindMenu, Code: "menu.res.outbound", Name: "外呼任务", ParentCode: "mod.res", Resource: "/outbound-tasks", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.res.outbound.view", Name: "查看外呼任务", ParentCode: "menu.res.outbound", Resource: "outbound-tasks", Action: "view"},
		{Kind: PermissionKindButton, Code: "btn.res.outbound.create", Name: "新建外呼任务", ParentCode: "menu.res.outbound", Resource: "outbound-tasks", Action: "create"},
		{Kind: PermissionKindButton, Code: "btn.res.outbound.delete", Name: "删除外呼任务", ParentCode: "menu.res.outbound", Resource: "outbound-tasks", Action: "delete"},
		{Kind: PermissionKindAPI, Code: "api.sip.campaigns.read", Name: "外呼·读", ParentCode: "menu.res.outbound", Resource: "sip-center.campaigns", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.sip.campaigns.write", Name: "外呼·写", ParentCode: "menu.res.outbound", Resource: "sip-center.campaigns", Action: "write"},
		{Kind: PermissionKindMenu, Code: "menu.res.script", Name: "脚本管理", ParentCode: "mod.res", Resource: "/script-manager", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.res.script.view", Name: "查看脚本", ParentCode: "menu.res.script", Resource: "scripts", Action: "view"},
		{Kind: PermissionKindButton, Code: "btn.res.script.create", Name: "新建脚本", ParentCode: "menu.res.script", Resource: "scripts", Action: "create"},
		{Kind: PermissionKindButton, Code: "btn.res.script.delete", Name: "删除脚本", ParentCode: "menu.res.script", Resource: "scripts", Action: "delete"},
		{Kind: PermissionKindAPI, Code: "api.sip.scripts.read", Name: "话术·读", ParentCode: "menu.res.script", Resource: "sip-center.scripts", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.sip.scripts.write", Name: "话术·写", ParentCode: "menu.res.script", Resource: "sip-center.scripts", Action: "write"},

		// --- 电话业务 ---
		{Kind: PermissionKindModule, Code: "mod.tel", Name: "电话业务", ParentCode: ""},
		{Kind: PermissionKindMenu, Code: "menu.tel.records", Name: "通话记录", ParentCode: "mod.tel", Resource: "/call-records", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.tel.records.view", Name: "查看记录", ParentCode: "menu.tel.records", Resource: "call-records", Action: "view"},
		{Kind: PermissionKindButton, Code: "btn.tel.records.export", Name: "通话导出", ParentCode: "menu.tel.records", Resource: "call-records", Action: "export"},
		{Kind: PermissionKindButton, Code: "btn.tel.records.playback", Name: "播放录音", ParentCode: "menu.tel.records", Resource: "call-records", Action: "playback"},
		{Kind: PermissionKindAPI, Code: "api.sip.calls.read", Name: "通话·读", ParentCode: "menu.tel.records", Resource: "sip-center.calls", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.sip.calls.write", Name: "通话·写", ParentCode: "menu.tel.records", Resource: "sip-center.calls", Action: "write"},
		{Kind: PermissionKindMenu, Code: "menu.tel.webseat", Name: "Web 坐席", ParentCode: "mod.tel", Resource: "/web-agents", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.tel.webseat.use", Name: "使用坐席（上线下线）", ParentCode: "menu.tel.webseat", Resource: "web-agents", Action: "use"},
		{Kind: PermissionKindButton, Code: "btn.tel.acd.create", Name: "新建 ACD 池条目", ParentCode: "menu.tel.webseat", Resource: "acd-pool", Action: "create"},
		{Kind: PermissionKindButton, Code: "btn.tel.acd.delete", Name: "删除 ACD 池条目", ParentCode: "menu.tel.webseat", Resource: "acd-pool", Action: "delete"},
		{Kind: PermissionKindAPI, Code: "api.sip.acd.read", Name: "ACD·读", ParentCode: "menu.tel.webseat", Resource: "sip-center.acd-pool", Action: "read"},
		{Kind: PermissionKindAPI, Code: "api.sip.acd.write", Name: "ACD·写", ParentCode: "menu.tel.webseat", Resource: "sip-center.acd-pool", Action: "write"},

		// --- 工作台 ---
		{Kind: PermissionKindModule, Code: "mod.workspace", Name: "工作台", ParentCode: ""},
		{Kind: PermissionKindMenu, Code: "menu.workspace.overview", Name: "工作台", ParentCode: "mod.workspace", Resource: "/overview", Action: "page"},
		{Kind: PermissionKindButton, Code: "btn.workspace.overview.view", Name: "查看工作台", ParentCode: "menu.workspace.overview", Resource: "/overview", Action: "view"},
	}
}

// SyncPermissionCatalog 将目录写入 permissions 表（按 code 幂等插入或更新）。生产环境也会执行，不依赖 SeedNonProd。
func SyncPermissionCatalog(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	defs := DefaultPermissionCatalog()
	sort.SliceStable(defs, func(i, j int) bool {
		ci := strings.Count(defs[i].Code, ".")
		cj := strings.Count(defs[j].Code, ".")
		if ci != cj {
			return ci < cj
		}
		return defs[i].Code < defs[j].Code
	})
	for _, def := range defs {
		if def.Code == "" {
			continue
		}
		var row Permission
		err := db.Unscoped().Where("code = ?", def.Code).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p := Permission{
				Code:        def.Code,
				Name:        def.Name,
				Description: def.Description,
				Kind:        def.Kind,
				ParentCode:  def.ParentCode,
				Resource:    def.Resource,
				Action:      def.Action,
			}
			p.SetCreateInfo("catalog")
			if err := db.Create(&p).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		u := map[string]any{
			"deleted_at":  nil,
			"name":        def.Name,
			"description": def.Description,
			"kind":        def.Kind,
			"parent_code": def.ParentCode,
			"resource":    def.Resource,
			"action":      def.Action,
		}
		if err := db.Unscoped().Model(&Permission{}).Where("id = ?", row.ID).Updates(u).Error; err != nil {
			return err
		}
	}
	return nil
}

// BackfillAdminRolePermissions 将每个租户的系统「管理员」角色与当前权限目录全量对齐（新增目录项会自动并入；启动时与迁移后执行）。
func BackfillAdminRolePermissions(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	var ids []uint
	if err := db.Model(&Permission{}).Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	var roles []TenantRole
	if err := db.Where("name = ? AND is_system = ?", TenantAdminRoleName, true).
		Find(&roles).Error; err != nil {
		return err
	}
	for _, r := range roles {
		if err := ReplaceTenantRolePermissions(db, r.ID, ids, "catalog"); err != nil {
			return err
		}
	}
	return nil
}
