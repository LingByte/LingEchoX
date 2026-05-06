import { get, del, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export interface SIPUserRow {
  id: number
  username: string
  domain: string
  contactUri?: string
  remoteIp?: string
  remotePort?: number
  online?: boolean
  expiresAt?: string
  lastSeenAt?: string
  userAgent?: string
  createdAt?: string
  updatedAt?: string
}

export async function listSIPUsers(page = 1, size = 20): Promise<ApiResponse<Paginated<SIPUserRow>>> {
  return get(`/sip-center/users?page=${page}&size=${size}`)
}

export async function fetchSIPUsersForSelect(maxTotal = 500): Promise<SIPUserRow[]> {
  const out: SIPUserRow[] = []
  const size = 100
  let page = 1
  while (out.length < maxTotal) {
    const res = await listSIPUsers(page, size)
    if (res.code !== 200 || !res.data?.list?.length) break
    out.push(...res.data.list)
    if (res.data.list.length < size) break
    page += 1
  }
  return out.slice(0, maxTotal)
}

export async function deleteSIPUser(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/users/${id}`)
}
