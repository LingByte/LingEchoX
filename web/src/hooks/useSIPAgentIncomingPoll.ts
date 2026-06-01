import { useCallback, useEffect, useRef, useState } from 'react'
import {
  formatACDTargetIdParam,
  pollSIPAgentIncoming,
  subscribeSIPAgentIncomingSSE,
  type SIPAgentIncomingPoll,
} from '@/api/sipAgentIncoming'
import type { ACDPoolTargetRow } from '@/api/acdPool'
import { callerDisplay } from '@/utils/sipAgentIncoming'
import { showAlert } from '@/utils/notification'

export function seatIdKey(id: number | string) {
  return formatACDTargetIdParam(id) ?? String(id)
}

type Options = {
  /** Show top-right toast when a new transfer ring starts */
  notify?: boolean
}

function applySnapshot(
  data: SIPAgentIncomingPoll,
  prev: Record<string, SIPAgentIncomingPoll>,
): Record<string, SIPAgentIncomingPoll> {
  const key = seatIdKey(data.acdTargetId)
  const next = { ...prev }
  if (data.incoming) {
    next[key] = data
  } else {
    delete next[key]
  }
  return next
}

export function useSIPAgentIncomingPoll(
  seats: ACDPoolTargetRow[],
  active: boolean,
  options?: Options,
) {
  const notify = options?.notify ?? false
  const [incomingBySeatId, setIncomingBySeatId] = useState<Record<string, SIPAgentIncomingPoll>>({})
  const notifiedRef = useRef<Set<string>>(new Set())
  const prevIncomingRef = useRef<Record<string, SIPAgentIncomingPoll>>({})
  const seatsRef = useRef(seats)
  seatsRef.current = seats

  const runNotify = useCallback(
    (next: Record<string, SIPAgentIncomingPoll>, prev: Record<string, SIPAgentIncomingPoll>) => {
      if (!notify) return
      const sipRows = seatsRef.current.filter((r) => r.routeType === 'sip')
      sipRows.forEach((r) => {
        const key = seatIdKey(r.id)
        const cur = next[key]
        const was = prev[key]
        if (cur?.incoming) {
          const callId = cur.callId || ''
          const nk = `${key}:${callId}`
          if ((!was?.incoming || was.callId !== callId) && !notifiedRef.current.has(nk)) {
            notifiedRef.current.add(nk)
            const label = r.name || r.targetValue || '值班坐席'
            showAlert(`${label} · 主叫 ${callerDisplay(cur)}`, 'warning', '400 来电', { duration: 8000 })
          }
        } else if (was?.incoming) {
          notifiedRef.current.delete(`${key}:${was.callId || ''}`)
        }
      })
    },
    [notify],
  )

  const handleSnapshot = useCallback(
    (data: SIPAgentIncomingPoll) => {
      setIncomingBySeatId((prev) => {
        const next = applySnapshot(data, prev)
        runNotify(next, prev)
        prevIncomingRef.current = next
        return next
      })
    },
    [runNotify],
  )

  /** Manual refresh via HTTP poll (SSE is the primary path). */
  const poll = useCallback(async () => {
    const sipRows = seats.filter((r) => r.routeType === 'sip')
    if (!sipRows.length) return
    const settled = await Promise.allSettled(
      sipRows.map((r) =>
        pollSIPAgentIncoming({
          acdTargetId: r.id,
          name: r.name || undefined,
          targetValue: r.targetValue || undefined,
        }),
      ),
    )
    const next: Record<string, SIPAgentIncomingPoll> = {}
    sipRows.forEach((r, i) => {
      const hit = settled[i]
      if (hit.status === 'fulfilled' && hit.value.code === 200 && hit.value.data?.incoming) {
        next[seatIdKey(r.id)] = hit.value.data
      }
    })
    const prev = prevIncomingRef.current
    runNotify(next, prev)
    prevIncomingRef.current = next
    setIncomingBySeatId(next)
  }, [seats, runNotify])

  useEffect(() => {
    if (!active) {
      setIncomingBySeatId({})
      prevIncomingRef.current = {}
      return
    }
    const sipRows = seats.filter((r) => r.routeType === 'sip')
    if (!sipRows.length) {
      setIncomingBySeatId({})
      prevIncomingRef.current = {}
      return
    }

    const ids = sipRows.map((r) => r.id)
    const unsubscribe = subscribeSIPAgentIncomingSSE(ids, {
      onSnapshot: handleSnapshot,
      onError: () => {
        void poll()
      },
    })

    return () => {
      unsubscribe()
    }
  }, [active, seats, handleSnapshot, poll])

  const incomingSeats = seats.filter(
    (r) => r.routeType === 'sip' && incomingBySeatId[seatIdKey(r.id)]?.incoming,
  )

  const hasIncoming = incomingSeats.length > 0

  return { incomingBySeatId, incomingSeats, hasIncoming, poll }
}
