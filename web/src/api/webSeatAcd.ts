import { post, type ApiResponse } from '@/utils/request'
import {
  listACDPoolTargets,
  getACDPoolTarget,
  updateACDPoolTarget,
  createACDPoolTarget,
} from '@/api/acdPool'

const WEBSEAT_ACD_POOL_ROW_SESSION_KEY = 'soulnexus.webseat.acdPoolTargetId'

function anchorSessionKey(trunkNumberId: number): string {
  return `${WEBSEAT_ACD_POOL_ROW_SESSION_KEY}:${trunkNumberId}`
}

function readAnchoredWebSeatAcdPoolId(trunkNumberId: number): number | null {
  if (typeof sessionStorage === 'undefined') return null
  try {
    const s = sessionStorage.getItem(anchorSessionKey(trunkNumberId))
    if (!s) return null
    const id = parseInt(s, 10)
    return Number.isFinite(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

function writeAnchoredWebSeatAcdPoolId(trunkNumberId: number, id: number): void {
  if (typeof sessionStorage === 'undefined') return
  sessionStorage.setItem(anchorSessionKey(trunkNumberId), String(id))
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

async function findWebAcdRowIdForOperator(operatorKey: string, trunkNumberId: number): Promise<number | null> {
  const k = normOpKey(operatorKey)
  if (!k || !trunkNumberId) return null
  const res = await listACDPoolTargets(1, 100, { routeType: 'web', trunkNumberId })
  if (res.code !== 200 || !res.data?.list?.length) return null
  const mine = res.data.list.filter((r) => normOpKey(r.createBy || '') === k)
  if (!mine.length) return null
  mine.sort((a, b) => a.id - b.id)
  return mine[0]!.id
}

export async function postWebSeatAcdHeartbeat(targetId: number): Promise<void> {
  const r: ApiResponse<{ ok?: boolean }> = await post('/sip-center/acd-pool/web-seat/heartbeat', { targetId })
  if (r.code !== 200) throw new Error(r.msg || 'web seat heartbeat failed')
}

export async function ensureWebSeatAcdPoolRowOnline(opts: {
  displayLabel: string
  operatorKey: string
  trunkNumberId: number
}): Promise<number> {
  const tnId = opts.trunkNumberId
  if (!tnId || tnId <= 0) {
    throw new Error('请先选择中继号码后再上线')
  }
  const label = opts.displayLabel.trim() || 'Web'
  const existing = await findWebAcdRowIdForOperator(opts.operatorKey, tnId)
  let targetId: number | null = existing
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
  writeAnchoredWebSeatAcdPoolId(tnId, c.data.id)
  return c.data.id
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
