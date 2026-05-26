import { listACDPoolTargets, type ACDPoolTargetRow } from '@/api/acdPool'
import type { SIPAgentIncomingPoll } from '@/api/sipAgentIncoming'
import type { User } from '@/stores/authStore'

export const SIP_INCOMING_POLL_MS = 2000

const norm = (s?: string | null) => String(s ?? '').trim().toLowerCase()

/** Whether an ACD SIP row belongs to the logged-in tenant user. */
export function acdSeatMatchesUser(row: ACDPoolTargetRow, user: User | null | undefined): boolean {
  if (!row || !user) return false
  if (norm(row.routeType) !== 'sip') return false

  const username = norm(user.username)
  const displayName = norm(user.displayName as string | undefined)
  const emailLocal = norm(user.email?.split('@')[0])

  const targetValue = norm(row.targetValue)
  const seatName = norm(row.name)

  if (targetValue) {
    if (username && targetValue === username) return true
    if (emailLocal && targetValue === emailLocal) return true
  }
  if (seatName) {
    if (displayName && seatName === displayName) return true
    if (username && seatName === username) return true
  }
  return false
}

export async function fetchAllSipACDSeats(): Promise<ACDPoolTargetRow[]> {
  const size = 100
  let page = 1
  let all: ACDPoolTargetRow[] = []
  let total = 0

  for (let guard = 0; guard < 50; guard++) {
    const res = await listACDPoolTargets(page, size, { routeType: 'sip' })
    if (res.code !== 200 || !res.data) break
    const list = res.data.list || []
    total = res.data.total || 0
    all = all.concat(list)
    if (all.length >= total || list.length === 0) break
    page += 1
  }
  return all
}

export function callerDisplay(poll?: SIPAgentIncomingPoll | null): string {
  const n = poll?.callerNumber?.trim()
  return n || '未知号码'
}
