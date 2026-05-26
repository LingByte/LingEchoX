import { get, type ApiResponse } from '@/utils/request'

/** Snowflake IDs are serialized as strings — never compare with > 0. */
export function formatACDTargetIdParam(id: number | string | undefined | null): string | undefined {
  if (id == null) return undefined
  const s = String(id).trim()
  if (!s || s === '0' || s === 'undefined' || s === 'null') return undefined
  return s
}

export type SIPAgentIncomingPoll = {
  incoming: boolean
  acdTargetId: number | string
  seatName?: string
  targetValue?: string
  routeType?: string
  callId?: string
  callerNumber?: string
  phase?: string
  since?: string
}

/** Poll whether a SIP ACD seat has an inbound transfer ringing toward it. */
export async function pollSIPAgentIncoming(params: {
  acdTargetId?: number | string
  name?: string
  targetValue?: string
}): Promise<ApiResponse<SIPAgentIncomingPoll>> {
  const q = new URLSearchParams()
  const id = formatACDTargetIdParam(params.acdTargetId)
  if (id) q.set('acdTargetId', id)
  if (params.name?.trim()) q.set('name', params.name.trim())
  if (params.targetValue?.trim()) q.set('targetValue', params.targetValue.trim())
  return get(`/sip-center/sip-agent/incoming?${q.toString()}`)
}

export type SIPACDTransferOfferRow = {
  id: number | string
  tenantId: number | string
  inboundCallId: string
  outboundCallId?: string
  acdPoolTargetId: number | string
  trunkNumberId?: number | string
  callerNumber?: string
  phase: string
  startedAt: string
  endedAt?: string
}

export async function listSIPAgentIncomingLogs(
  acdTargetId: number | string,
  page = 1,
  size = 20,
) {
  const id = formatACDTargetIdParam(acdTargetId)
  if (!id) {
    return Promise.reject(new Error('invalid acdTargetId'))
  }
  const q = new URLSearchParams({
    acdTargetId: id,
    page: String(page),
    size: String(size),
  })
  return get<ApiResponse<{ list: SIPACDTransferOfferRow[]; total: number; page: number; size: number }>>(
    `/sip-center/sip-agent/incoming/logs?${q.toString()}`,
  )
}
