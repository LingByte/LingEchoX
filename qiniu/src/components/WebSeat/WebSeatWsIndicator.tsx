import Popover from '@/components/UI/Popover'
import Button from '@/components/UI/Button'
import { cn } from '@/utils/cn'
import type { WebSeatWsState } from './WebSeatContext'

function dotClass(wsState: WebSeatWsState): string {
  if (wsState === 'open') return 'bg-emerald-500 shadow-[0_0_0_3px_rgba(16,185,129,0.35)]'
  if (wsState === 'connecting') return 'bg-amber-400 animate-pulse shadow-[0_0_0_3px_rgba(251,191,36,0.35)]'
  if (wsState === 'disabled') return 'bg-muted-foreground/40'
  return 'bg-red-500 shadow-[0_0_0_3px_rgba(239,68,68,0.35)]'
}

export function WebSeatWsIndicator({
  wsState,
  wsStatusText,
  presenceWsClients,
  onGoOnline,
  onGoOffline,
  onReconnect,
  className,
}: {
  wsState: WebSeatWsState
  wsStatusText: string
  presenceWsClients: number
  onGoOnline: () => void
  onGoOffline: () => void
  onReconnect: () => void
  className?: string
}) {
  const onlineBusy = wsState === 'open' || wsState === 'connecting'
  return (
    <Popover
      placement="bottom"
      trigger="click"
      contentClassName="min-w-[240px] p-0 overflow-hidden rounded-xl border border-border bg-card shadow-lg"
      content={
        <div className="p-3 text-sm space-y-3">
          <div>
            <p className="font-medium text-foreground leading-snug">{wsStatusText}</p>
            {wsState === 'open' && <p className="mt-1.5 text-xs text-muted-foreground">WS 客户端数: <span className="font-mono text-foreground">{presenceWsClients}</span></p>}
          </div>
          <div className="flex flex-col gap-2">
            <Button type="button" size="sm" className="w-full" disabled={onlineBusy} onClick={onGoOnline}>上线</Button>
            <Button type="button" size="sm" variant="secondary" className="w-full" onClick={onGoOffline}>下线</Button>
            <Button type="button" size="sm" variant="outline" className="w-full" disabled={wsState === 'connecting'} onClick={onReconnect}>重连 WS</Button>
          </div>
        </div>
      }
    >
      <button type="button" className={cn('inline-flex items-center gap-2 rounded-full border border-border bg-card/95 px-2.5 py-1.5 text-xs font-medium', className)} aria-label="Web 坐席线路">
        <span className={cn('h-2.5 w-2.5 shrink-0 rounded-full', dotClass(wsState))} />
        <span className="hidden sm:inline">Web 坐席线路</span>
      </button>
    </Popover>
  )
}
