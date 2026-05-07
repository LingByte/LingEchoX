import { post, type ApiResponse } from '@/utils/request'

export interface TenantAuthTenant {
  id: number
  name: string
  slug: string
  status?: string
}

export interface TenantAuthUser {
  id: number
  tenantId: number
  email: string
  displayName?: string
  status?: string
}

export interface PlatformAdminAuth {
  id: number
  email: string
  displayName?: string
  status?: string
}

export type LoginPrincipal = 'tenant' | 'platform'

/** Same endpoint `/login`; shape depends on principal. */
export type TenantAuthPayload =
  | {
      principal: 'tenant'
      token: string
      expiresIn: number
      tenant: TenantAuthTenant
      user: TenantAuthUser
      platformAdmin?: undefined
    }
  | {
      principal: 'platform'
      token: string
      expiresIn: number
      platformAdmin: PlatformAdminAuth
      tenant?: undefined
      user?: undefined
    }

export interface TenantRegisterBody {
  companyName: string
  adminEmail: string
  adminPassword: string
  adminDisplayName?: string
  tenantDescription?: string
}

export interface TenantLoginBody {
  email: string
  password: string
}

export async function registerTenant(body: TenantRegisterBody): Promise<ApiResponse<TenantAuthPayload>> {
  return post<TenantAuthPayload>('/register', body)
}

export async function tenantLogin(body: TenantLoginBody): Promise<ApiResponse<TenantAuthPayload>> {
  return post<TenantAuthPayload>('/login', body)
}
