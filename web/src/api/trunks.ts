import { get, post, put, del, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

// providerCode 由后端在创建时分配，前端只读，不参与请求体。
export interface TrunkRow {
  id: number
  name: string
  description?: string
  prefix?: string
  local_addr?: string
  providerCode?: string
  numbers?: TrunkNumberRow[]
  createdAt?: string
  updatedAt?: string
}

export interface TrunkNumberRow {
  id: number
  trunkId: number
  tenantId?: number
  number: string
  callerDisplayName?: string
  prefix?: string
  description?: string
  direction?: string
  outboundTrunkNumberId?: number
  status?: string
  concurrent?: number
  callInConcurrent?: number
  isTransferRelay?: boolean
  effectiveTime?: string | null
  expirationTime?: string | null
  providerCode?: string
  voiceDialogWsUrl?: string
  createdAt?: string
  updatedAt?: string
}

export async function listTrunks(page = 1, size = 20, opts?: { name?: string }): Promise<ApiResponse<Paginated<TrunkRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.name) q.set('name', opts.name)
  return get(`/sip-center/trunks?${q.toString()}`)
}

export async function getTrunk(id: number): Promise<ApiResponse<TrunkRow>> {
  return get(`/sip-center/trunks/${id}`)
}

export async function createTrunk(body: {
  name: string
  description?: string
  prefix?: string
  local_addr?: string
}): Promise<ApiResponse<TrunkRow>> {
  return post('/sip-center/trunks', body)
}

export async function updateTrunk(id: number, body: {
  name: string
  description?: string
  prefix?: string
  local_addr?: string
}): Promise<ApiResponse<TrunkRow>> {
  return put(`/sip-center/trunks/${id}`, body)
}

export async function deleteTrunk(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/trunks/${id}`)
}

export async function listTrunkNumbers(
  page = 1,
  size = 20,
  opts?: { trunkId?: number; number?: string; tenantId?: number },
): Promise<ApiResponse<Paginated<TrunkNumberRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.trunkId != null && opts.trunkId > 0) q.set('trunkId', String(opts.trunkId))
  if (opts?.number) q.set('number', opts.number)
  if (opts?.tenantId != null && opts.tenantId > 0) q.set('tenantId', String(opts.tenantId))
  return get(`/sip-center/trunk-numbers?${q.toString()}`)
}

export async function getTrunkNumber(id: number): Promise<ApiResponse<TrunkNumberRow>> {
  return get(`/sip-center/trunk-numbers/${id}`)
}

export async function createTrunkNumber(body: {
  trunkId: number
  tenantId?: number
  number: string
  callerDisplayName?: string
  prefix?: string
  description?: string
  direction?: string
  outboundTrunkNumberId?: number
  status?: string
  concurrent?: number
  callInConcurrent?: number
  isTransferRelay?: boolean
  effectiveTime?: string | null
  expirationTime?: string | null
  voiceDialogWsUrl?: string
}): Promise<ApiResponse<TrunkNumberRow>> {
  return post('/sip-center/trunk-numbers', body)
}

export async function updateTrunkNumber(id: number, body: {
  trunkId: number
  tenantId?: number
  number: string
  callerDisplayName?: string
  prefix?: string
  description?: string
  direction?: string
  outboundTrunkNumberId?: number
  status?: string
  concurrent?: number
  callInConcurrent?: number
  isTransferRelay?: boolean
  effectiveTime?: string | null
  expirationTime?: string | null
  voiceDialogWsUrl?: string
}): Promise<ApiResponse<TrunkNumberRow>> {
  return put(`/sip-center/trunk-numbers/${id}`, body)
}

export async function deleteTrunkNumber(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/trunk-numbers/${id}`)
}

export async function fetchTrunksForSelect(maxTotal = 500): Promise<TrunkRow[]> {
  const out: TrunkRow[] = []
  const size = 100
  let page = 1
  while (out.length < maxTotal) {
    const res = await listTrunks(page, size)
    if (res.code !== 200 || !res.data?.list?.length) break
    out.push(...res.data.list)
    if (res.data.list.length < size) break
    page += 1
  }
  return out.slice(0, maxTotal)
}
