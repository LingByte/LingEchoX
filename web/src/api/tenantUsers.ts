import { get, post, put, del, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export interface TenantUserRow {
  id: number
  email?: string
  phone?: string
  username?: string
  displayName?: string
  status?: string
}

export async function listTenantUsers(
  page = 1,
  size = 100,
  opts?: { status?: string; search?: string },
): Promise<ApiResponse<Paginated<TenantUserRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.status) q.set('status', opts.status)
  if (opts?.search) q.set('search', opts.search)
  return get(`/tenant-users?${q.toString()}`)
}

export async function createTenantUser(body: {
  email: string
  password?: string
  phone?: string
  username?: string
  displayName?: string
  status?: string
}): Promise<ApiResponse<TenantUserRow>> {
  return post('/tenant-users', body)
}

export async function updateTenantUser(
  id: number,
  body: {
    email?: string
    phone?: string
    username?: string
    displayName?: string
    status?: string
  },
): Promise<ApiResponse<{ id: number }>> {
  return put(`/tenant-users/${id}`, body)
}

export async function updateTenantUserStatus(
  id: number,
  status: string,
): Promise<ApiResponse<{ id: number; status: string }>> {
  return put(`/tenant-users/${id}/status`, { status })
}

export async function deleteTenantUser(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/tenant-users/${id}`)
}
