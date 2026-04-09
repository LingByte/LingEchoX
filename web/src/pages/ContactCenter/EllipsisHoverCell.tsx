import Popover from '@/components/UI/Popover'
import { cn } from '@/utils/cn'

interface EllipsisHoverCellProps {
  text: string | null | undefined
  className?: string
  lines?: 2 | 3
  mono?: boolean
}

export function EllipsisHoverCell({ text, className, lines = 2, mono }: EllipsisHoverCellProps) {
  const raw = text?.trim() ?? ''
  if (!raw) return <span className={cn('text-muted-foreground', className)}>—</span>

  return (
    <Popover
      trigger="hover"
      placement="top"
      className="block w-full min-w-0 max-w-full"
      contentClassName="max-w-[min(24rem,calc(100vw-2rem))] bg-card border-border shadow-xl"
      content={<div className="whitespace-pre-wrap break-words text-sm text-foreground leading-relaxed">{raw}</div>}
    >
      <span
        className={cn(
          'block w-full min-w-0 overflow-hidden text-left',
          lines === 2 && 'line-clamp-2',
          lines === 3 && 'line-clamp-3',
          mono && 'font-mono text-xs break-all',
          !mono && 'break-words',
          className
        )}
      >
        {raw}
      </span>
    </Popover>
  )
}
