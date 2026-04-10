import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject, type ReactNode } from 'react'
import { useAuthStore } from '@/stores/authStore'
import { clearWebSeatAcdPoolAnchor, ensureWebSeatAcdPoolRowOnline, postWebSeatAcdHeartbeat, setWebSeatAcdPoolRowOffline } from '@/api/sipContactCenter'
import { showAlert } from '@/utils/notification'
import { WebSeatContext, type WebSeatContextValue, type WebSeatWsState } from './WebSeatContext'
import { getUserMediaAudioOnly } from './getUserMediaCompat'
import { buildWebSeatWebSocketURL, webSeatHttpBase, webSeatWsBase, webSeatWsToken } from './webseatEnv'
import { WebSeatIncomingCallCard } from './WebSeatIncomingCallCard'

const WEBSEAT_ACD_HEARTBEAT_MS = 30_000
const MAX_SIGNAL_LINES = 400
const MAX_RX_LINES = 250

function appendLog(prev: string, line: string, maxLines: number): string {
  const next = prev + line + '\n'
  const parts = next.split('\n')
  if (parts.length > maxLines) return parts.slice(-maxLines).join('\n')
  return next
}

function waitForWebSocketOpen(wsRef: MutableRefObject<WebSocket | null>, timeoutMs: number): Promise<void> {
  const start = Date.now()
  return new Promise((resolve, reject) => {
    const id = window.setInterval(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        clearInterval(id); resolve(); return
      }
      if (Date.now() - start >= timeoutMs) {
        clearInterval(id); reject(new Error('WebSocket open timeout'))
      }
    }, 80)
  })
}

export function WebSeatProvider({ children }: { children: ReactNode }) {
  const user = useAuthStore((s) => s.user)
  const httpBase = useMemo(() => webSeatHttpBase(), [])
  const wsBase = useMemo(() => webSeatWsBase(), [])
  const wsToken = useMemo(() => webSeatWsToken(), [])
  const configured = httpBase.length > 0
  const [wsState, setWsState] = useState<WebSeatWsState>(configured ? 'idle' : 'disabled')
  const [wsStatusText, setWsStatusText] = useState(configured ? 'WS：未上线（请点击「上线」建立连接）' : 'WS：未配置')
  const [presenceWsClients, setPresenceWsClients] = useState(0)
  const [presenceOnline, setPresenceOnline] = useState(false)
  const [signalLog, setSignalLog] = useState('')
  const [rxLog, setRxLog] = useState('')
  const [inCall, setInCall] = useState(false)
  const [hangupDisabled, setHangupDisabled] = useState(true)
  const [pendingIncomingCallId, setPendingIncomingCallId] = useState<string | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const wsCloseIntentRef = useRef<'user-offline' | null>(null)
  const acdHeartbeatTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const activeCallIdRef = useRef<string | null>(null)
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const localStreamRef = useRef<MediaStream | null>(null)
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null)

  const logSignal = useCallback((...args: unknown[]) => {
    const line = args.map((a) => (typeof a === 'string' ? a : JSON.stringify(a))).join(' ')
    const ts = new Date().toISOString().slice(11, 23)
    setSignalLog((p) => appendLog(p, `[${ts}] ${line}`, MAX_SIGNAL_LINES))
  }, [])

  const logRx = useCallback((...args: unknown[]) => {
    const line = args.map((a) => (typeof a === 'string' ? a : JSON.stringify(a))).join(' ')
    const ts = new Date().toISOString().slice(11, 23)
    setRxLog((p) => appendLog(p, `[${ts}] ${line}`, MAX_RX_LINES))
  }, [])

  const stopAcdHeartbeat = useCallback(() => {
    if (acdHeartbeatTimerRef.current != null) {
      clearInterval(acdHeartbeatTimerRef.current)
      acdHeartbeatTimerRef.current = null
    }
  }, [])

  const closeWsConnection = useCallback((intent?: 'user-offline') => {
    wsCloseIntentRef.current = intent || null
    if (wsRef.current) {
      try { wsRef.current.close() } catch {}
      wsRef.current = null
    }
    setPresenceWsClients(0)
    setPresenceOnline(false)
    logSignal('WS closed by client', intent || '')
  }, [logSignal])

  const connectWebSocket = useCallback(() => {
    if (!configured) return
    closeWsConnection()
    const url = buildWebSeatWebSocketURL(httpBase, wsToken, wsBase)
    setWsState('connecting')
    setWsStatusText('WS：连接中...')
    try {
      const ws = new WebSocket(url)
      wsRef.current = ws
      logSignal('WS connect start', url.replace(/token=[^&]+/, 'token=***'))
      ws.onopen = () => {
        setWsState('open')
        setWsStatusText('WS：已上线（等待来电）')
        logSignal('WS open')
      }
      ws.onclose = () => {
        wsRef.current = null
        stopAcdHeartbeat()
        setWsState('closed')
        const intent = wsCloseIntentRef.current
        wsCloseIntentRef.current = null
        setWsStatusText(intent === 'user-offline' ? 'WS：已下线' : 'WS：连接断开')
        logSignal('WS close event', intent || 'unexpected')
      }
      ws.onerror = () => {
        logSignal('WS error event')
      }
      ws.onmessage = (ev) => {
        logRx('WS <-', String(ev.data || ''))
        try {
          const data = JSON.parse(ev.data as string) as { type?: string; call_id?: string; ws_clients?: number; online?: boolean }
          if (data?.type === 'presence') {
            setPresenceWsClients(typeof data.ws_clients === 'number' ? data.ws_clients : 0)
            setPresenceOnline(Boolean(data.online))
            logSignal('presence', { ws_clients: data.ws_clients, online: data.online })
          }
          if (data?.type === 'incoming' && data.call_id) {
            const cid = String(data.call_id)
            setPendingIncomingCallId(cid)
            logSignal('incoming call', cid)
          }
        } catch {}
      }
    } catch {
      setWsState('closed')
      setWsStatusText('WS：连接失败')
      logSignal('WS create failed')
    }
  }, [closeWsConnection, configured, httpBase, logRx, logSignal, stopAcdHeartbeat, wsBase, wsToken])

  const goOnline = useCallback(async () => {
    if (!configured) return
    stopAcdHeartbeat()
    try {
      connectWebSocket()
      await waitForWebSocketOpen(wsRef, 15_000)
      const operatorKey = (user?.email && String(user.email).trim()) || (user?.id != null ? String(user.id) : '')
      const displayLabel = `${(user?.username || user?.email || '坐席')}-Web`
      const tid = await ensureWebSeatAcdPoolRowOnline({ displayLabel, operatorKey })
      logSignal('ACD online row ready', { tid, operatorKey, displayLabel })
      void postWebSeatAcdHeartbeat(tid).catch(() => {})
      // @ts-ignore
      acdHeartbeatTimerRef.current = window.setInterval(() => {
        void postWebSeatAcdHeartbeat(tid)
          .then(() => logSignal('heartbeat ok', tid))
          .catch(() => logSignal('heartbeat failed', tid))
      }, WEBSEAT_ACD_HEARTBEAT_MS)
      window.dispatchEvent(new CustomEvent('soulnexus-acd-refresh'))
      logSignal('goOnline success')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '上线失败', 'error')
      logSignal('goOnline failed', e instanceof Error ? e.message : String(e))
      closeWsConnection()
    }
  }, [closeWsConnection, configured, connectWebSocket, logSignal, stopAcdHeartbeat, user?.email, user?.id, user?.username])

  const goOffline = useCallback(async () => {
    stopAcdHeartbeat()
    try {
      await setWebSeatAcdPoolRowOffline()
      window.dispatchEvent(new CustomEvent('soulnexus-acd-refresh'))
      logSignal('ACD set offline success')
    } catch {}
    closeWsConnection('user-offline')
    logSignal('goOffline done')
  }, [closeWsConnection, logSignal, stopAcdHeartbeat])

  const reconnectWebSocket = useCallback(() => {
    if (configured) connectWebSocket()
  }, [configured, connectWebSocket])

  const hangup = useCallback(() => {
    const cid = activeCallIdRef.current
    const pc = pcRef.current
    const ls = localStreamRef.current
    const ra = remoteAudioRef.current
    const stayOnline = acdHeartbeatTimerRef.current != null
    if (cid && httpBase) {
      void fetch(`${httpBase}/webseat/v1/hangup`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ call_id: cid }),
      }).catch(() => {})
    }
    if (pc) {
      try {
        pc.getSenders().forEach((s) => s.track?.stop())
        pc.close()
      } catch {}
      pcRef.current = null
    }
    if (ls) {
      try {
        ls.getTracks().forEach((t) => t.stop())
      } catch {}
      localStreamRef.current = null
    }
    if (ra) {
      try {
        ra.pause()
        ra.removeAttribute('src')
        ra.srcObject = null
      } catch {}
      remoteAudioRef.current = null
    }
    activeCallIdRef.current = null
    setInCall(false)
    setHangupDisabled(true)
    logSignal('manual hangup sent', cid)
    // 仅挂断通话：保持 ACD 在线与 WS 订阅来电；若链路异常断开则自动重连
    if (stayOnline) {
      const w = wsRef.current
      if (w && w.readyState === WebSocket.OPEN) {
        setWsState('open')
        setWsStatusText('WS：已上线（等待来电）')
      } else {
        logSignal('hangup: WS not open while staying online, reconnecting')
        connectWebSocket()
      }
    }
  }, [connectWebSocket, httpBase, logSignal])

  const answerIncoming = useCallback(async () => {
    const cid = pendingIncomingCallId
    if (!cid || !httpBase) return
    setPendingIncomingCallId(null)
    try {
      logSignal('answer start', cid)
      setInCall(false)
      setHangupDisabled(true)

      // clean previous session first
      if (pcRef.current) {
        try {
          pcRef.current.getSenders().forEach((s) => s.track?.stop())
          pcRef.current.close()
        } catch {}
        pcRef.current = null
      }
      if (localStreamRef.current) {
        try {
          localStreamRef.current.getTracks().forEach((t) => t.stop())
        } catch {}
        localStreamRef.current = null
      }
      if (remoteAudioRef.current) {
        try {
          remoteAudioRef.current.pause()
          remoteAudioRef.current.removeAttribute('src')
          remoteAudioRef.current.srcObject = null
        } catch {}
        remoteAudioRef.current = null
      }

      const localStream = await getUserMediaAudioOnly()
      logSignal('getUserMedia ok')
      localStreamRef.current = localStream
      const pc = new RTCPeerConnection({ iceServers: [{ urls: 'stun:stun.l.google.com:19302' }] })
      pcRef.current = pc
      pc.onconnectionstatechange = () => logSignal('pc.connectionState', pc.connectionState)
      pc.oniceconnectionstatechange = () => logSignal('pc.iceConnectionState', pc.iceConnectionState)
      pc.ontrack = (ev) => {
        const track = ev.track
        logRx('ontrack', {
          kind: track.kind,
          id: track.id,
          muted: track.muted,
          enabled: track.enabled,
          readyState: track.readyState,
        })
        if (track.kind !== 'audio') return
        const stream = ev.streams[0] ?? new MediaStream([track])
        let audio = remoteAudioRef.current
        if (!audio) {
          audio = new Audio()
          audio.setAttribute('playsinline', 'true')
          audio.autoplay = true
          audio.muted = false
          audio.volume = 1
          remoteAudioRef.current = audio
        }
        audio.srcObject = stream
        void audio.play()
          .then(() => logSignal('remote audio playing'))
          .catch((err: unknown) =>
            logSignal('remote audio play failed', err instanceof Error ? err.message : String(err))
          )
      }
      localStream.getTracks().forEach((track) => pc.addTrack(track, localStream))
      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)
      logSignal('setLocalDescription offer')
      const ld = pc.localDescription
      if (!ld) throw new Error('no localDescription')
      const res = await fetch(`${httpBase}/webseat/v1/join`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ call_id: cid, sdp: ld.sdp, type: ld.type, candidates: [] }),
      })
      logSignal('POST /webseat/v1/join', res.status)
      const ans = await res.json()
      if (!ans.sdp || !ans.type) throw new Error('bad answer')
      await pc.setRemoteDescription({ type: ans.type, sdp: ans.sdp })
      activeCallIdRef.current = cid
      setInCall(true)
      setHangupDisabled(false)
      logSignal('answer success', cid)
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '接听失败', 'error')
      logSignal('answer failed', e instanceof Error ? e.message : String(e))
      setInCall(false)
      setHangupDisabled(true)
    }
  }, [httpBase, logRx, logSignal, pendingIncomingCallId])

  const rejectIncoming = useCallback(async () => {
    const cid = pendingIncomingCallId
    if (!cid || !httpBase) return
    setPendingIncomingCallId(null)
    try {
      const res = await fetch(`${httpBase}/webseat/v1/reject`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ call_id: cid }),
      })
      logSignal('reject sent', cid, res.status)
    } catch {}
  }, [httpBase, logSignal, pendingIncomingCallId])

  useEffect(() => {
    if (!configured) return
    return () => {
      if (pcRef.current) {
        try {
          pcRef.current.getSenders().forEach((s) => s.track?.stop())
          pcRef.current.close()
        } catch {}
        pcRef.current = null
      }
      if (localStreamRef.current) {
        try {
          localStreamRef.current.getTracks().forEach((t) => t.stop())
        } catch {}
        localStreamRef.current = null
      }
      if (remoteAudioRef.current) {
        try {
          remoteAudioRef.current.pause()
          remoteAudioRef.current.removeAttribute('src')
          remoteAudioRef.current.srcObject = null
        } catch {}
        remoteAudioRef.current = null
      }
      stopAcdHeartbeat()
      closeWsConnection()
      void setWebSeatAcdPoolRowOffline().catch(() => {})
      clearWebSeatAcdPoolAnchor()
      logSignal('provider cleanup done')
    }
  }, [closeWsConnection, configured, logSignal, stopAcdHeartbeat])

  const ctxValue: WebSeatContextValue = useMemo(() => ({
    configured,
    wsState,
    wsStatusText,
    presenceWsClients,
    presenceOnline,
    signalLog,
    rxLog,
    inCall,
    hangupDisabled,
    pendingIncomingCallId,
    hangup,
    reconnectWebSocket,
    goOnline,
    goOffline,
  }), [configured, goOffline, goOnline, hangup, hangupDisabled, inCall, pendingIncomingCallId, presenceOnline, presenceWsClients, reconnectWebSocket, rxLog, signalLog, wsState, wsStatusText])

  return (
    <WebSeatContext.Provider value={ctxValue}>
      {children}
      {configured && pendingIncomingCallId && (
        <div className="pointer-events-none fixed bottom-4 right-4 z-[300] flex max-w-[calc(100vw-2rem)] justify-end sm:bottom-6 sm:right-6">
          <WebSeatIncomingCallCard
            className="pointer-events-auto"
            callId={pendingIncomingCallId}
            onAnswer={() => void answerIncoming()}
            onReject={() => void rejectIncoming()}
          />
        </div>
      )}
    </WebSeatContext.Provider>
  )
}
