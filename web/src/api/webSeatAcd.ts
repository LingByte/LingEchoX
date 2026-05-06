import { post, type ApiResponse } from '@/utils/request'
import {
  listACDPoolTargets,
  getACDPoolTarget,
  updateACDPoolTarget,
  createACDPoolTarget,
} from '@/api/acdPool'

const WEBSEAT_ACD_POOL_ROW_SESSION_KEY = 'soulnexus.webseat.acdPoolTargetId'

function readAnchoredWebSeatAcdPoolId(): number | null {
  if (typeof sessionStorage === 'undefined') return null
  try {
    const s = sessionStorage.getItem(WEBSEAT_ACD_POOL_ROW_SESSION_KEY)
    if (!s) return null
    const id = parseInt(s, 10)
    return Number.isFinite(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

function writeAnchoredWebSeatAcdPoolId(id: number): void {
  if (typeof sessionStorage === 'undefined') return
  sessionStorage.setItem(WEBSEAT_ACD_POOL_ROW_SESSION_KEY, String(id))
}

export function clearWebSeatAcdPoolAnchor(): void {
  if (typeof sessionStorage === 'undefined') return
  sessionStorage.removeItem(WEBSEAT_ACD_POOL_ROW_SESSION_KEY)
}

function normOpKey(s: string): string {
  return s.trim().toLowerCase()
}

async function findWebAcdRowIdForOperator(operatorKey: string): Promise<number | null> {
  const k = normOpKey(operatorKey)
  if (!k) return null
  const res = await listACDPoolTargets(1, 100, { routeType: 'web' })
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

export async function ensureWebSeatAcdPoolRowOnline(opts: { displayLabel: string; operatorKey: string }): Promise<number> {
  const label = opts.displayLabel.trim() || 'Web'
  const existing = await findWebAcdRowIdForOperator(opts.operatorKey)
  let targetId: number | null = existing
  if (targetId == null) {
    const anchor = readAnchoredWebSeatAcdPoolId()
    if (anchor != null) {
      const cur = await getACDPoolTarget(anchor)
      if (cur.code === 200 && cur.data?.routeType === 'web') targetId = anchor
    }
  }
  if (targetId != null) {
    const cur = await getACDPoolTarget(targetId)
    if (cur.code === 200 && cur.data?.routeType === 'web') {
      const r = cur.data
      const wt = r.weight != null && r.weight > 0 ? r.weight : 10
      const u = await updateACDPoolTarget(targetId, { name: label, routeType: 'web', sipSource: '', targetValue: '', weight: wt, workState: 'available' })
      if (u.code !== 200) throw new Error(u.msg || 'update web seat acd failed')
      writeAnchoredWebSeatAcdPoolId(targetId)
      return targetId
    }
    clearWebSeatAcdPoolAnchor()
  }
  const c = await createACDPoolTarget({ name: label, routeType: 'web', sipSource: '', targetValue: '', weight: 10, workState: 'available' })
  if (c.code !== 200 || !c.data?.id) throw new Error(c.msg || 'create web seat acd failed')
  writeAnchoredWebSeatAcdPoolId(c.data.id)
  return c.data.id
}

export async function setWebSeatAcdPoolRowOffline(): Promise<void> {
  const anchor = readAnchoredWebSeatAcdPoolId()
  if (anchor == null) return
  const cur = await getACDPoolTarget(anchor)
  if (cur.code !== 200 || !cur.data || cur.data.routeType !== 'web') {
    clearWebSeatAcdPoolAnchor()
    return
  }
  const r = cur.data
  const u = await updateACDPoolTarget(anchor, { name: r.name || '', routeType: 'web', sipSource: '', targetValue: '', weight: r.weight ?? 10, workState: 'offline' })
  if (u.code !== 200) throw new Error(u.msg || 'set web seat acd offline failed')
}
