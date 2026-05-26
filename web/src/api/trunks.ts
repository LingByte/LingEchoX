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
  // tenantId is a Snowflake string (Tenant.ID > 2^53). Backend serializes with json:"tenantId,string".
  tenantId?: string
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
  // welcomeAudioUrl 入局欢迎语音频 URL（http/https）。空字符串=回退到 scripts/welcome.wav。
  // 由用户直接粘贴外链 OR 通过 uploadTrunkNumberWelcomeAudio 上传 WAV 拿到平台 URL。
  welcomeAudioUrl?: string
  // transferRingingUrl 转接阶段回铃 WAV URL（http/https）。空字符串=回退到
  // SIP_TRANSFER_RINGING_WAV_PATH env / scripts/ringing.wav。与 welcomeAudioUrl
  // 同套校验、同种上传流程，只是平台落盘目录不同。
  transferRingingUrl?: string
  /** 坐席接听前 TTS 模板（可选，最长 256 字）。占位符 {{N}} {{NTail4}} {{Name}} */
  transferAgentBriefText?: string
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
  opts?: { trunkId?: number; number?: string; tenantId?: string },
): Promise<ApiResponse<Paginated<TrunkNumberRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.trunkId != null && opts.trunkId > 0) q.set('trunkId', String(opts.trunkId))
  if (opts?.number) q.set('number', opts.number)
  if (opts?.tenantId && opts.tenantId !== '0') q.set('tenantId', opts.tenantId)
  return get(`/sip-center/trunk-numbers?${q.toString()}`)
}

export async function getTrunkNumber(id: number): Promise<ApiResponse<TrunkNumberRow>> {
  return get(`/sip-center/trunk-numbers/${id}`)
}

export async function createTrunkNumber(body: {
  trunkId: number
  // tenantId is the Snowflake Tenant.ID as a string ("0" for unassigned platform pool).
  tenantId?: string
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
  welcomeAudioUrl?: string
  transferRingingUrl?: string
  transferAgentBriefText?: string
}): Promise<ApiResponse<TrunkNumberRow>> {
  return post('/sip-center/trunk-numbers', body)
}

export async function updateTrunkNumber(id: number, body: {
  trunkId: number
  // tenantId is the Snowflake Tenant.ID as a string ("0" for unassigned platform pool).
  tenantId?: string
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
  welcomeAudioUrl?: string
  transferRingingUrl?: string
  transferAgentBriefText?: string
}): Promise<ApiResponse<TrunkNumberRow>> {
  return put(`/sip-center/trunk-numbers/${id}`, body)
}

// uploadTrunkNumberWelcomeAudio 把 WAV 上传到后端 stores.Default()，
// 返回 { url, key, size }。前端拿到 url 后写入表单 welcomeAudioUrl 字段，
// 与「直接粘贴外链 URL」共用同一个保存字段（保存时由后端再次校验）。
export async function uploadTrunkNumberWelcomeAudio(file: File): Promise<ApiResponse<{ url: string; key: string; size: number }>> {
  const fd = new FormData()
  fd.append('file', file)
  return post('/sip-center/trunk-numbers/welcome-audio', fd)
}

// uploadTrunkNumberTransferRingingAudio 与 welcome-audio 完全等价，仅落盘
// 前缀不同（transfer-ringing-audio/）。后端做相同的 WAV magic 校验。
export async function uploadTrunkNumberTransferRingingAudio(file: File): Promise<ApiResponse<{ url: string; key: string; size: number }>> {
  const fd = new FormData()
  fd.append('file', file)
  return post('/sip-center/trunk-numbers/transfer-ringing-audio', fd)
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
