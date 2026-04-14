import { createContext, useContext, type ReactNode } from 'react'

export type WebSeatWsState = 'disabled' | 'idle' | 'connecting' | 'open' | 'closed'

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
