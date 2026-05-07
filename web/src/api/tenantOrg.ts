import { del, get, post, put, type ApiResponse } from '@/utils/request'

export interface OrgPermission {
  id: number
  code: string
  name: string
  description?: string
  /** module | menu | button | api | data */
  kind?: string
  /** Parent permission code (catalog tree); empty for top-level modules */
  parentCode?: string
  resource?: string
  action?: string
}

export interface OrgGroup {
  id: number
  name: string
  isDefault?: boolean
}

export interface OrgRole {
  id: number
  name: string
  description?: string
  isSystem?: boolean
}

export interface OrgRoleDetail extends OrgRole {
  permissionIds: number[]
}

export async function listOrgPermissions(): Promise<ApiResponse<{ list: OrgPermission[] }>> {
  return get('/sip-center/tenant-org/permissions')
}

export async function listOrgGroups(): Promise<ApiResponse<{ list: OrgGroup[] }>> {
  return get('/sip-center/tenant-org/groups')
}

export async function createOrgGroup(body: { name: string; isDefault?: boolean }): Promise<ApiResponse<OrgGroup>> {
  return post('/sip-center/tenant-org/groups', body)
}

export async function updateOrgGroup(
  id: number,
  body: { name: string; isDefault?: boolean },
): Promise<ApiResponse<OrgGroup>> {
  return put(`/sip-center/tenant-org/groups/${id}`, body)
}

export async function deleteOrgGroup(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/tenant-org/groups/${id}`)
}

export async function listOrgRoles(): Promise<ApiResponse<{ list: OrgRole[] }>> {
  return get('/sip-center/tenant-org/roles')
}

export async function getOrgRole(id: number): Promise<ApiResponse<OrgRoleDetail>> {
  return get(`/sip-center/tenant-org/roles/${id}`)
}

export async function createOrgRole(body: { name: string; description?: string }): Promise<ApiResponse<OrgRole>> {
  return post('/sip-center/tenant-org/roles', body)
}

export async function updateOrgRole(
  id: number,
  body: { name: string; description?: string },
): Promise<ApiResponse<{ id: number }>> {
  return put(`/sip-center/tenant-org/roles/${id}`, body)
}

export async function deleteOrgRole(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/tenant-org/roles/${id}`)
}

export async function putOrgRolePermissions(
  roleId: number,
  body: { permissionIds: number[] },
): Promise<ApiResponse<{ roleId: number }>> {
  return put(`/sip-center/tenant-org/roles/${roleId}/permissions`, body)
}

export async function putOrgTenantUserRoles(
  userId: number,
  body: { roleIds: number[] },
): Promise<ApiResponse<Record<string, unknown>>> {
  return put(`/sip-center/tenant-org/users/${userId}/roles`, body)
}

export async function putOrgTenantUserGroups(
  userId: number,
  body: { groupIds: number[] },
): Promise<ApiResponse<Record<string, unknown>>> {
  return put(`/sip-center/tenant-org/users/${userId}/groups`, body)
}
