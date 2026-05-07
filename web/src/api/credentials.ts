import { get, post, put, del, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export type CredentialStatus = 'active' | 'disabled'

export interface CredentialRow {
  id: number
  tenantId: number
  name: string
  accessKey: string
  status: CredentialStatus
  allowIp?: string
  permissionCodes?: string[]
  createdAt?: string
  updatedAt?: string
  createBy?: string
}

// secretKey 仅在创建响应中出现一次，丢失后只能重新创建。
export interface CredentialCreateResult extends CredentialRow {
  secretKey: string
  notice?: string
}

export interface CredentialListQuery {
  page?: number
  size?: number
  status?: CredentialStatus
  name?: string
}

export interface CredentialCreateBody {
  name?: string
  allowIp?: string
  permissionCodes?: string[]
}

export interface CredentialUpdateBody {
  name?: string
  allowIp?: string
  permissionCodes?: string[]
}

export async function listCredentials(query: CredentialListQuery = {}): Promise<ApiResponse<Paginated<CredentialRow>>> {
  const params = new URLSearchParams({
    page: String(query.page ?? 1),
    size: String(query.size ?? 20),
  })
  if (query.status) params.set('status', query.status)
  if (query.name) params.set('name', query.name)
  return get(`/credentials?${params.toString()}`)
}

export async function createCredential(body: CredentialCreateBody): Promise<ApiResponse<CredentialCreateResult>> {
  return post('/credentials', body)
}

export async function updateCredential(id: number, body: CredentialUpdateBody): Promise<ApiResponse<{ id: number }>> {
  return put(`/credentials/${id}`, body)
}

export async function disableCredential(id: number): Promise<ApiResponse<{ id: number; status: CredentialStatus }>> {
  return post(`/credentials/${id}/disable`, {})
}

export async function enableCredential(id: number): Promise<ApiResponse<{ id: number; status: CredentialStatus }>> {
  return post(`/credentials/${id}/enable`, {})
}

export async function deleteCredential(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/credentials/${id}`)
}
