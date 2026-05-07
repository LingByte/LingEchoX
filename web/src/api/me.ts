import { get, post, put, type ApiResponse } from '@/utils/request'

export interface MeTenant {
  id: number
  name: string
  slug: string
  status?: string
}

export interface MeUser {
  id: number
  tenantId: number
  email: string
  phone?: string
  username?: string
  displayName?: string
  status?: string
}

export interface PlatformAdminMe {
  id: number
  email: string
  displayName?: string
  status?: string
}

export type MePayload =
  | { principal: 'tenant'; user: MeUser; tenant: MeTenant; platformAdmin?: undefined }
  | { principal: 'platform'; platformAdmin: PlatformAdminMe; user?: undefined; tenant?: undefined }

export async function fetchMe(): Promise<ApiResponse<MePayload>> {
  return get<MePayload>('/me')
}

export async function updateMe(body: {
  displayName?: string
  phone?: string
  username?: string
}): Promise<ApiResponse<MeUser | PlatformAdminMe>> {
  return put<MeUser | PlatformAdminMe>('/me', body)
}

export async function updateMyPassword(body: {
  oldPassword: string
  newPassword: string
}): Promise<ApiResponse<{ id: number }>> {
  return put<{ id: number }>('/me/password', body)
}

export async function logoutApi(): Promise<ApiResponse<{ loggedOut: boolean }>> {
  return post<{ loggedOut: boolean }>('/auth/logout')
}
