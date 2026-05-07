import { Button, Select, Typography } from '@arco-design/web-react'
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
    trunkPick,
    trunkPickSummary,
    trunkCandidates,
    trunkListLoading,
    selectedTrunkNumberId,
    setSelectedTrunkNumberId,
  } = useWebSeat()

  const trunkSelectLocked = wsState === 'open' || wsState === 'connecting'

  if (!configured) {
    return (
      <div className="rounded-lg border border-amber-500/40 bg-amber-50/80 dark:bg-amber-950/30 p-4 text-sm text-foreground">
        <p className="font-medium">未配置 Web 坐席网关</p>
        <p className="mt-2 text-muted-foreground whitespace-pre-wrap">
          请检查 `VITE_API_BASE_URL`（默认 `/api`）。WebSocket 与 `VITE_WS_BASE_URL` 的配置方式与本站其它接口一致。可选：`VITE_SIP_WEBSEAT_WS_TOKEN`（与后端 `SIP_WEBSEAT_WS_TOKEN` 一致）。
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <Typography.Text type="secondary">
        Web 坐席用于接听从 ACD 转接过来的实时通话。请先在下方选择本次承接的中继号码（须与客户拨打的 DID、以及「号码池」里绑定在该号码上的
        Web 坐席行一致），再点击上线。
      </Typography.Text>
      <div className="rounded-lg border border-[var(--color-border-2)] bg-[var(--color-fill-2)] px-3 py-3 text-sm space-y-2">
        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-end">
          <div className="min-w-[260px] flex-1 max-w-md">
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 6 }}>
              中继号码（上线后不可切换，请先下线）
            </Typography.Text>
            <Select
              placeholder={trunkListLoading ? '加载中…' : '请选择号码'}
              loading={trunkListLoading}
              disabled={trunkSelectLocked || trunkCandidates.length === 0}
              value={selectedTrunkNumberId}
              onChange={(v) => setSelectedTrunkNumberId(v as number)}
              options={trunkCandidates.map((r) => ({
                value: r.id,
                label: `${String(r.number || '').trim() || `线路号码 id=${r.id}`} · id ${r.id}`,
              }))}
              style={{ width: '100%' }}
            />
          </div>
          <Typography.Text type="secondary" style={{ fontSize: 12 }} className="sm:pb-2">
            预览：<span className="text-foreground font-medium">{trunkPickSummary}</span>
            {trunkPick && trunkSelectLocked ? (
              <span className="block mt-1 text-[var(--color-text-3)]">上次会话已绑定 id {trunkPick.id}</span>
            ) : null}
          </Typography.Text>
        </div>
      </div>
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
          <Button type="primary" size="small" disabled={wsState === 'open' || wsState === 'connecting'} onClick={() => void goOnline()}>上线</Button>
          <Button size="small" onClick={() => void goOffline()}>下线</Button>
          <Button type="outline" size="small" onClick={() => reconnectWebSocket()}>重连 WS</Button>
          <Button status="danger" type="outline" size="small" disabled={hangupDisabled} onClick={() => hangup()}>挂断</Button>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-stretch">
        <WebSeatTerminalLog accent="signal" title="信令日志" body={signalLog} hint="SIP/WebRTC 信令流与状态变化" />
        <WebSeatTerminalLog accent="rx" title="接收音频日志" body={rxLog} hint="远端音频能量监测（用于排查无声问题）" />
      </div>
    </div>
  )
}
