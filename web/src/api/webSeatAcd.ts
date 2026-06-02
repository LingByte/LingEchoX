import { post, type ApiResponse } from '@/utils/request'
import { formatACDTargetIdParam } from '@/api/sipAgentIncoming'
import {
  listACDPoolTargets,
  getACDPoolTarget,
  updateACDPoolTarget,
  createACDPoolTarget,
} from '@/api/acdPool'

const WEBSEAT_ACD_POOL_ROW_SESSION_KEY = 'soulnexus.webseat.acdPoolTargetId'

/** Snowflake ACD row id — always string in JS (never Number: loses precision above 2^53-1). */
export type WebSeatAcdTargetId = string

function anchorSessionKey(trunkNumberId: number): string {
  return `${WEBSEAT_ACD_POOL_ROW_SESSION_KEY}:${trunkNumberId}`
}

function normalizeAcdTargetId(id: number | string | undefined | null): WebSeatAcdTargetId | null {
  const s = formatACDTargetIdParam(id)
  return s ?? null
}

function readAnchoredWebSeatAcdPoolId(trunkNumberId: number): WebSeatAcdTargetId | null {
  if (typeof sessionStorage === 'undefined') return null
  try {
    const s = sessionStorage.getItem(anchorSessionKey(trunkNumberId))?.trim()
    return normalizeAcdTargetId(s)
  } catch {
    return null
  }
}

function writeAnchoredWebSeatAcdPoolId(trunkNumberId: number, id: WebSeatAcdTargetId): void {
  if (typeof sessionStorage === 'undefined') return
  sessionStorage.setItem(anchorSessionKey(trunkNumberId), id)
}

/** Clears stored ACD row anchor for one trunk (or every trunk prefix if called with clearAllAnchors). */
export function clearWebSeatAcdPoolAnchor(trunkNumberId?: number): void {
  if (typeof sessionStorage === 'undefined') return
  if (trunkNumberId != null && trunkNumberId > 0) {
    sessionStorage.removeItem(anchorSessionKey(trunkNumberId))
    return
  }
  try {
    const keys: string[] = []
    for (let i = 0; i < sessionStorage.length; i++) {
      const k = sessionStorage.key(i)
      if (k && k.startsWith(WEBSEAT_ACD_POOL_ROW_SESSION_KEY)) keys.push(k)
    }
    keys.forEach((k) => sessionStorage.removeItem(k))
  } catch {
    sessionStorage.removeItem(WEBSEAT_ACD_POOL_ROW_SESSION_KEY)
  }
}

function normOpKey(s: string): string {
  return s.trim().toLowerCase()
}

function compareAcdTargetIds(a: number | string, b: number | string): number {
  const sa = formatACDTargetIdParam(a) ?? ''
  const sb = formatACDTargetIdParam(b) ?? ''
  try {
    const ba = BigInt(sa || '0')
    const bb = BigInt(sb || '0')
    if (ba < bb) return -1
    if (ba > bb) return 1
    return 0
  } catch {
    return sa.localeCompare(sb)
  }
}

async function findWebAcdRowIdForOperator(
  operatorKey: string,
  trunkNumberId: number,
): Promise<WebSeatAcdTargetId | null> {
  const k = normOpKey(operatorKey)
  if (!k || !trunkNumberId) return null
  const res = await listACDPoolTargets(1, 100, { routeType: 'web', trunkNumberId })
  if (res.code !== 200 || !res.data?.list?.length) return null
  const mine = res.data.list.filter((r) => normOpKey(r.createBy || '') === k)
  if (!mine.length) return null
  mine.sort((a, b) => compareAcdTargetIds(a.id, b.id))
  return normalizeAcdTargetId(mine[0]!.id)
}

export async function postWebSeatAcdHeartbeat(targetId: number | string): Promise<void> {
  const id = normalizeAcdTargetId(targetId)
  if (!id) throw new Error('invalid acd target id for heartbeat')
  // Send string — JSON number cannot represent Snowflake ids exactly in JS.
  const r: ApiResponse<{ ok?: boolean }> = await post('/sip-center/acd-pool/web-seat/heartbeat', { targetId: id })
  if (r.code !== 200) throw new Error(r.msg || 'web seat heartbeat failed')
}

export async function ensureWebSeatAcdPoolRowOnline(opts: {
  displayLabel: string
  operatorKey: string
  trunkNumberId: number
}): Promise<WebSeatAcdTargetId> {
  const tnId = opts.trunkNumberId
  if (!tnId || tnId <= 0) {
    throw new Error('请先选择中继号码后再上线')
  }
  const label = opts.displayLabel.trim() || 'Web'
  const existing = await findWebAcdRowIdForOperator(opts.operatorKey, tnId)
  let targetId: WebSeatAcdTargetId | null = existing
  if (targetId == null) {
    const anchor = readAnchoredWebSeatAcdPoolId(tnId)
    if (anchor != null) {
      const cur = await getACDPoolTarget(anchor)
      if (
        cur.code === 200 &&
        cur.data?.routeType === 'web' &&
        Number(cur.data.trunkNumberId) === tnId
      ) {
        targetId = anchor
      }
    }
  }
  if (targetId != null) {
    const cur = await getACDPoolTarget(targetId)
    if (cur.code === 200 && cur.data?.routeType === 'web') {
      const r = cur.data
      const wt = r.weight != null && r.weight > 0 ? r.weight : 10
      const u = await updateACDPoolTarget(targetId, {
        name: label,
        trunkNumberId: tnId,
        routeType: 'web',
        sipSource: '',
        targetValue: '',
        weight: wt,
        workState: 'available',
        shiftSchedule: r.shiftSchedule ?? '',
      })
      if (u.code !== 200) throw new Error(u.msg || 'update web seat acd failed')
      writeAnchoredWebSeatAcdPoolId(tnId, targetId)
      return targetId
    }
    clearWebSeatAcdPoolAnchor(tnId)
  }
  const c = await createACDPoolTarget({
    name: label,
    trunkNumberId: tnId,
    routeType: 'web',
    sipSource: '',
    targetValue: '',
    weight: 10,
    workState: 'available',
  })
  if (c.code !== 200 || !c.data?.id) throw new Error(c.msg || 'create web seat acd failed')
  const createdId = normalizeAcdTargetId(c.data.id)
  if (!createdId) throw new Error('create web seat acd returned invalid id')
  writeAnchoredWebSeatAcdPoolId(tnId, createdId)
  return createdId
}

export async function setWebSeatAcdPoolRowOffline(trunkNumberId: number): Promise<void> {
  if (!trunkNumberId || trunkNumberId <= 0) return
  const anchor = readAnchoredWebSeatAcdPoolId(trunkNumberId)
  if (anchor == null) return
  const cur = await getACDPoolTarget(anchor)
  if (cur.code !== 200 || !cur.data || cur.data.routeType !== 'web') {
    clearWebSeatAcdPoolAnchor(trunkNumberId)
    return
  }
  const r = cur.data
  const u = await updateACDPoolTarget(anchor, {
    name: r.name || '',
    trunkNumberId: trunkNumberId,
    routeType: 'web',
    sipSource: '',
    targetValue: '',
    weight: r.weight ?? 10,
    workState: 'offline',
    shiftSchedule: r.shiftSchedule ?? '',
  })
  if (u.code !== 200) throw new Error(u.msg || 'set web seat acd offline failed')
}
