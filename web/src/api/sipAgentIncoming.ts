import { getApiBaseURL } from '@/config/apiConfig'
import { parseSSEBlock } from '@/utils/sseClient'
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

export type SIPAgentIncomingSSEHandlers = {
  onSnapshot: (data: SIPAgentIncomingPoll) => void
  onReady?: () => void
  onError?: (err: Error) => void
}

/** SSE stream for SIP seat incoming state (replaces polling when used from the hook). */
export function subscribeSIPAgentIncomingSSE(
  acdTargetIds: (number | string)[],
  handlers: SIPAgentIncomingSSEHandlers,
): () => void {
  const ids = acdTargetIds
    .map((id) => formatACDTargetIdParam(id))
    .filter((id): id is string => Boolean(id))
    .join(',')
  if (!ids) {
    return () => {}
  }

  const controller = new AbortController()
  const token = typeof localStorage !== 'undefined' ? localStorage.getItem('auth_token') : null
  const url = `${getApiBaseURL()}/sip-center/sip-agent/incoming/stream?acdTargetIds=${encodeURIComponent(ids)}`

  void (async () => {
    try {
      const res = await fetch(url, {
        method: 'GET',
        headers: {
          Accept: 'text/event-stream',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        credentials: 'include',
        signal: controller.signal,
      })
      if (!res.ok || !res.body) {
        throw new Error(`SSE failed: HTTP ${res.status}`)
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''

      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        let sep: number
        while ((sep = buf.indexOf('\n\n')) >= 0) {
          const block = buf.slice(0, sep)
          buf = buf.slice(sep + 2)
          parseSSEBlock(block, (event, data) => {
            if (event === 'heartbeat') return
            let payload: unknown
            try {
              payload = JSON.parse(data)
            } catch {
              return
            }
            if (event === 'ready') {
              handlers.onReady?.()
              return
            }
            if (event === 'snapshot' && payload && typeof payload === 'object') {
              handlers.onSnapshot(payload as SIPAgentIncomingPoll)
            }
          })
        }
      }
    } catch (e) {
      if (!controller.signal.aborted) {
        handlers.onError?.(e instanceof Error ? e : new Error(String(e)))
      }
    }
  })()

  return () => controller.abort()
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
