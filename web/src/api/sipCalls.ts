import { get, type ApiResponse } from '@/utils/request'
import { getApiBaseURL } from '@/config/apiConfig'
import type { Paginated } from '@/api/types'

export interface SIPCallDialogTurn {
  asrText?: string
  llmText?: string
  asrProvider?: string
  ttsProvider?: string
  llmModel?: string
  at?: string
  trigger?: string
  scriptStepId?: string
  routeIntent?: string
  llmFirstMs?: number
  llmWallMs?: number
  ttsMs?: number
  pipelineMs?: number
}

export interface SIPCallRow {
  id: number
  callId: string
  fromHeader?: string
  toHeader?: string
  cseqInvite?: string
  direction?: string
  state?: string
  codec?: string
  payloadType?: number
  clockRate?: number
  remoteAddr?: string
  remoteRtpAddr?: string
  localRtpAddr?: string
  recordingUrl?: string
  recordingRawBytes?: number
  recordingWavBytes?: number
  byeInitiator?: string
  durationSec?: number
  endStatus?: string
  failureReason?: string
  inviteAt?: string
  ackAt?: string
  byeAt?: string
  endedAt?: string
  turnCount?: number
  firstTurnAt?: string
  lastTurnAt?: string
  hadSipTransfer?: boolean
  hadWebSeat?: boolean
  turns?: SIPCallDialogTurn[]
  createdAt?: string
  updatedAt?: string
}

export async function getSIPCall(id: number): Promise<ApiResponse<SIPCallRow>> {
  return get(`/sip-center/calls/${id}`)
}

export async function listSIPCalls(page = 1, size = 20, opts?: { callId?: string; state?: string }): Promise<ApiResponse<Paginated<SIPCallRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.callId) q.set('callId', opts.callId)
  if (opts?.state) q.set('state', opts.state)
  return get(`/sip-center/calls?${q.toString()}`)
}

export function resolveSipRecordingUrl(url?: string | null): string {
  if (!url) return ''
  const u = url.trim()
  if (/^https?:\/\//i.test(u)) return u
  const base = getApiBaseURL().replace(/\/$/, '')
  return u.startsWith('/') ? `${base}${u}` : `${base}/${u}`
}

export function sipAiEndStatusI18nKey(status?: string | null): string {
  const s = (status || '').trim()
  const map: Record<string, string> = {
    completed_remote: '对端挂断（未转接）',
    completed_local: '本端挂断（未转接）',
    after_transfer_remote: '曾转接 · 对端挂断',
    after_transfer_local: '曾转接 · 本端挂断',
  }
  return map[s] || '—'
}
