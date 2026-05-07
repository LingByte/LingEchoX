import { del, get, post, put, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export interface TenantRow {
  id: number
  name: string
  slug: string
  description?: string
  status?: string
  createdAt?: string
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

export async function getTenant(id: number): Promise<ApiResponse<{ tenant: TenantRow }>> {
  return get(`/tenants/${id}`)
}

export async function createTenantPlatform(body: {
  companyName: string
  adminEmail: string
  adminPassword: string
  adminDisplayName?: string
  tenantDescription?: string
}): Promise<ApiResponse<{ tenant: TenantRow; adminUser: Record<string, unknown>; roleId: number }>> {
  return post('/tenants', body)
}

export async function updateTenantPlatform(
  id: number,
  body: { name?: string; description?: string; status?: string },
): Promise<ApiResponse<{ tenant: TenantRow }>> {
  return put(`/tenants/${id}`, body)
}

export async function deleteTenantPlatform(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/tenants/${id}`)
}
