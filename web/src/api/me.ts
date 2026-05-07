import { get, post, put, type ApiResponse } from '@/utils/request'

export interface MeTenant {
  id: number
  name: string
  slug: string
  status?: string
}

export interface MeTenantGroup {
  id: number
  name: string
}

export interface MeUser {
  id: number
  tenantId: number
  email: string
  phone?: string
  username?: string
  displayName?: string
  status?: string
  avatarUrl?: string
  lastLogin?: string
  lastLoginIp?: string
  source?: string
  loginCount?: number
  totpEnabled?: boolean
  tenantGroup?: MeTenantGroup | null
  createdAt?: string
}

export interface PlatformAdminMe {
  id: number
  email: string
  displayName?: string
  status?: string
}

export type MePayload =
  | {
      principal: 'tenant'
      user: MeUser
      tenant: MeTenant
      /** 菜单与接口权限码（侧边栏过滤）；不在「个人资料」中展示 */
      permissionCodes?: string[]
      platformAdmin?: undefined
    }
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

export async function uploadMyAvatar(file: File): Promise<ApiResponse<{ avatarUrl: string; user: MeUser }>> {
  const fd = new FormData()
  fd.append('file', file)
  return post<{ avatarUrl: string; user: MeUser }>('/me/avatar', fd)
}

export async function setupTotp(): Promise<
  ApiResponse<{ secret: string; url: string; qrDataUrl: string }>
> {
  return post<{ secret: string; url: string; qrDataUrl: string }>('/me/totp/setup')
}

export async function enableTotp(body: { secret: string; code: string }): Promise<ApiResponse<MeUser>> {
  return post<MeUser>('/me/totp/enable', body)
}

export async function disableTotp(body: { password: string; code: string }): Promise<ApiResponse<MeUser>> {
  return post<MeUser>('/me/totp/disable', body)
}

export async function logoutApi(): Promise<ApiResponse<{ loggedOut: boolean }>> {
  return post<{ loggedOut: boolean }>('/auth/logout')
}
