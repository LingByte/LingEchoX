import { del, get, post, put, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export interface TenantRow {
  id: number
  name: string
  slug: string
  description?: string
  status?: string
  contactEmail?: string
  maxUserCount?: number
  createdAt?: string
}

/** 平台管理员 GET / PUT 租户详情时携带的 JSON（列表接口不返回） */
export interface TenantDetail extends TenantRow {
  asrConfig?: Record<string, unknown> | null
  ttsConfig?: Record<string, unknown> | null
  llmConfig?: Record<string, unknown> | null
}

export async function listTenants(
  page = 1,
  size = 100,
  opts?: { search?: string },
): Promise<ApiResponse<Paginated<TenantRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.search) q.set('search', opts.search)
  return get(`/tenants?${q.toString()}`)
}

export async function getTenant(id: number): Promise<ApiResponse<{ tenant: TenantDetail }>> {
  return get(`/tenants/${id}`)
}

export async function createTenantPlatform(body: {
  companyName: string
  adminEmail: string
  adminPassword: string
  adminDisplayName?: string
  tenantDescription?: string
  maxUserCount?: number
}): Promise<ApiResponse<{ tenant: TenantDetail; adminUser: Record<string, unknown>; roleId: number }>> {
  return post('/tenants', body)
}

export async function updateTenantPlatform(
  id: number,
  body: {
    name?: string
    description?: string
    status?: string
    contactEmail?: string
    maxUserCount?: number
    asrConfig?: Record<string, unknown> | null
    ttsConfig?: Record<string, unknown> | null
    llmConfig?: Record<string, unknown> | null
  },
): Promise<ApiResponse<{ tenant: TenantDetail }>> {
  return put(`/tenants/${id}`, body)
}

export async function deleteTenantPlatform(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/tenants/${id}`)
}
