import { createContext, useContext, type ReactNode } from 'react'
import type { TrunkNumberRow } from '@/api/trunks'

export type WebSeatWsState = 'disabled' | 'idle' | 'connecting' | 'open' | 'closed'

/** Selected SIP trunk number for Web 坐席 ACD row (persisted locally). */
export type WebSeatTrunkPick = { id: number; label: string }

export interface WebSeatContextValue {
  configured: boolean
  wsState: WebSeatWsState
  wsStatusText: string
  presenceWsClients: number
  presenceOnline: boolean
  signalLog: string
  rxLog: string
  inCall: boolean
  hangupDisabled: boolean
  pendingIncomingCallId: string | null
  /** 上次上线成功后的中继号码（下线后仍保留，用于展示） */
  trunkPick: WebSeatTrunkPick | null
  /** 横幅展示的号码说明（优先当前下拉框所选） */
  trunkPickSummary: string
  /** 分配给本租户的中继号码列表（用于下拉框） */
  trunkCandidates: TrunkNumberRow[]
  trunkListLoading: boolean
  /** 即将用于上线的中继号码行 id（请先选好再点「上线」） */
  selectedTrunkNumberId: number | undefined
  setSelectedTrunkNumberId: (id: number) => void
  hangup: () => void
  reconnectWebSocket: () => void
  goOnline: () => Promise<void>
  goOffline: () => Promise<void>
}

const defaultValue: WebSeatContextValue = {
  configured: false,
  wsState: 'disabled',
  wsStatusText: 'WS：未配置',
  presenceWsClients: 0,
  presenceOnline: false,
  signalLog: '',
  rxLog: '',
  inCall: false,
  hangupDisabled: true,
  pendingIncomingCallId: null,
  trunkPick: null,
  trunkPickSummary: '未选择中继号码',
  trunkCandidates: [],
  trunkListLoading: false,
  selectedTrunkNumberId: undefined,
  setSelectedTrunkNumberId: () => {},
  hangup: () => {},
  reconnectWebSocket: () => {},
  goOnline: async () => {},
  goOffline: async () => {},
}

export const WebSeatContext = createContext<WebSeatContextValue>(defaultValue)

export function useWebSeat(): WebSeatContextValue {
  return useContext(WebSeatContext)
}

export function WebSeatConsumer({ children }: { children: (v: WebSeatContextValue) => ReactNode }) {
  const v = useWebSeat()
  return <>{children(v)}</>
}
