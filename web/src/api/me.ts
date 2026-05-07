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

export interface MePayload {
  user: MeUser
  tenant: MeTenant
}

export async function fetchMe(): Promise<ApiResponse<MePayload>> {
  return get<MePayload>('/me')
}

export async function updateMe(body: {
  displayName?: string
  phone?: string
  username?: string
}): Promise<ApiResponse<MeUser>> {
  return put<MeUser>('/me', body)
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
