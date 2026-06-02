import { get, post, put, del, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export interface ACDPoolTargetRow {
  /** Snowflake id — API returns string (json:",string") */
  id: number | string
  trunkNumberId?: number
  name?: string
  createBy?: string
  routeType: string
  sipSource?: string
  targetValue?: string
  weight: number
  /** Lower = higher transfer priority; set via drag reorder in admin UI. */
  sortOrder?: number
  workState: string
  workStateAt?: string
  sipTrunkHost?: string
  sipTrunkPort?: number
  sipTrunkSignalingAddr?: string
  sipCallerId?: string
  sipCallerDisplayName?: string
  liveLineOnline?: boolean
  webSeatLastSeenAt?: string
  /** JSON: [{"weekdays":[1,2,3,4,5],"start":"09:00","end":"18:00"}]; weekdays 0=Sun..6=Sat; empty = 24/7 */
  shiftSchedule?: string
  /** JSON object for transfer_agent_brief placeholders, e.g. {"FactoryNumber":"F-1001"} */
  metaData?: string
  /** Plain-text admin note (max 128 chars); template placeholder {{Note}} */
  remark?: string
  createdAt?: string
  updatedAt?: string
}

export type ACDSipSource = 'internal' | 'trunk'
export const ACD_SIP_SOURCES: ACDSipSource[] = ['internal', 'trunk']
export type ACDRouteType = 'sip' | 'web'
export const ACD_ROUTE_TYPES: ACDRouteType[] = ['sip', 'web']
export const ACD_WORK_STATES = ['offline', 'available', 'ringing', 'busy', 'acw', 'break'] as const
export type ACDWorkState = (typeof ACD_WORK_STATES)[number]

export const ACD_DISPATCH_MODES = ['weight', 'round_robin'] as const
export type ACDDispatchMode = (typeof ACD_DISPATCH_MODES)[number]

export async function getACDDispatchMode(trunkNumberId: number): Promise<ApiResponse<{ trunkNumberId: number; acdDispatchMode: ACDDispatchMode }>> {
  const q = new URLSearchParams({ trunkNumberId: String(trunkNumberId) })
  return get(`/sip-center/acd-dispatch-mode?${q.toString()}`)
}

export async function updateACDDispatchMode(trunkNumberId: number, acdDispatchMode: ACDDispatchMode): Promise<ApiResponse<{ trunkNumberId: number; acdDispatchMode: ACDDispatchMode }>> {
  return put('/sip-center/acd-dispatch-mode', { trunkNumberId, acdDispatchMode })
}

export async function listACDPoolTargets(
  page = 1,
  size = 20,
  opts?: { routeType?: string; trunkNumberId?: number },
): Promise<ApiResponse<Paginated<ACDPoolTargetRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.routeType) q.set('routeType', opts.routeType)
  if (opts?.trunkNumberId != null && opts.trunkNumberId > 0) q.set('trunkNumberId', String(opts.trunkNumberId))
  return get(`/sip-center/acd-pool?${q.toString()}`)
}

export async function getACDPoolTarget(id: number | string): Promise<ApiResponse<ACDPoolTargetRow>> {
  const sid = String(id).trim()
  return get(`/sip-center/acd-pool/${sid}`)
}

export async function createACDPoolTarget(body: {
  name?: string
  trunkNumberId: number
  routeType: string
  sipSource?: string
  targetValue?: string
  sipTrunkHost?: string
  sipTrunkPort?: number
  sipTrunkSignalingAddr?: string
  sipCallerId?: string
  sipCallerDisplayName?: string
  weight?: number
  workState?: string
  shiftSchedule?: string
  remark?: string
  metaData?: string | Record<string, unknown>
}): Promise<ApiResponse<ACDPoolTargetRow>> {
  return post('/sip-center/acd-pool', body)
}

export async function updateACDPoolTarget(id: number | string, body: {
  name?: string
  trunkNumberId: number
  routeType: string
  sipSource?: string
  targetValue?: string
  sipTrunkHost?: string
  sipTrunkPort?: number
  sipTrunkSignalingAddr?: string
  sipCallerId?: string
  sipCallerDisplayName?: string
  weight?: number
  workState?: string
  shiftSchedule?: string
  remark?: string
  metaData?: string | Record<string, unknown>
}): Promise<ApiResponse<ACDPoolTargetRow>> {
  const sid = String(id).trim()
  return put(`/sip-center/acd-pool/${sid}`, body)
}

export async function deleteACDPoolTarget(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/acd-pool/${id}`)
}

export async function reorderACDPoolTargets(
  trunkNumberId: number,
  ids: Array<number | string>,
): Promise<ApiResponse<{ ok: boolean }>> {
  return post('/sip-center/acd-pool/reorder', { trunkNumberId, ids })
}
