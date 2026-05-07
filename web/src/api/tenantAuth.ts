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

export interface TenantAuthPayload {
  token: string
  expiresIn: number
  tenant: TenantAuthTenant
  user: TenantAuthUser
}

export interface TenantRegisterBody {
  companyName: string
  slug?: string
  adminEmail: string
  adminPassword: string
  adminDisplayName?: string
  tenantDescription?: string
}

export interface TenantLoginBody {
  tenantSlug: string
  email: string
  password: string
}

export async function registerTenant(body: TenantRegisterBody): Promise<ApiResponse<TenantAuthPayload>> {
  return post<TenantAuthPayload>('/register', body)
}

export async function tenantLogin(body: TenantLoginBody): Promise<ApiResponse<TenantAuthPayload>> {
  return post<TenantAuthPayload>('/login', body)
}
