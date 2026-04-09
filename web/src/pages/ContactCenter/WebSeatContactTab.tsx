import Button from '@/components/UI/Button'
import { useWebSeat } from '@/components/WebSeat/WebSeatContext'
import { WebSeatTerminalLog } from '@/pages/ContactCenter/WebSeatTerminalLog'

export default function WebSeatContactTab() {
  const {
    configured,
    wsState,
    wsStatusText,
    presenceWsClients,
    signalLog,
    rxLog,
    hangupDisabled,
    hangup,
    reconnectWebSocket,
    goOnline,
    goOffline,
  } = useWebSeat()

  if (!configured) {
    return (
      <div className="rounded-lg border border-amber-500/40 bg-amber-50/80 dark:bg-amber-950/30 p-4 text-sm text-foreground">
        <p className="font-medium">未配置 Web 坐席网关</p>
        <p className="mt-2 text-muted-foreground whitespace-pre-wrap">请在 `.env` 中设置 `VITE_SIP_WEBSEAT_HTTP_BASE` 后刷新页面。</p>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">Web 坐席用于接听从 ACD 转接过来的实时通话，支持上线/下线、重连和强制挂断。</p>
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-stretch">
        <div className="min-w-0 flex-1 rounded-lg border border-violet-500/30 bg-[#070b10] px-3 py-2.5 font-mono text-xs text-violet-200/90 shadow-[0_0_20px_rgba(139,92,246,0.12)] sm:max-w-xl">
          <div className="mb-1 flex items-center gap-2 text-[10px] uppercase tracking-[0.18em] text-violet-400/60">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-violet-400 shadow-[0_0_8px_rgba(167,139,250,0.8)]" /> LINK / STATUS
          </div>
          <p className="break-words text-violet-100/90">{wsStatusText}</p>
          {wsState === 'open' && (
            <p className="mt-2 text-[11px] text-violet-300/70">WS 客户端数: <span className="font-mono text-violet-200/90">{presenceWsClients}</span></p>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" size="sm" disabled={wsState === 'open' || wsState === 'connecting'} onClick={() => void goOnline()}>上线</Button>
          <Button type="button" variant="secondary" size="sm" onClick={() => void goOffline()}>下线</Button>
          <Button type="button" variant="outline" size="sm" onClick={() => reconnectWebSocket()}>重连 WS</Button>
          <Button type="button" variant="destructive" size="sm" disabled={hangupDisabled} onClick={() => hangup()}>挂断</Button>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-stretch">
        <WebSeatTerminalLog accent="signal" title="信令日志" body={signalLog} hint="SIP/WebRTC 信令流与状态变化" />
        <WebSeatTerminalLog accent="rx" title="接收音频日志" body={rxLog} hint="远端音频能量监测（用于排查无声问题）" />
      </div>
    </div>
  )
}
