import { useEffect, useRef } from 'react'
import { cn } from '@/utils/cn'

type Accent = 'signal' | 'rx'

const accentStyles: Record<Accent, { border: string; glow: string; text: string; bar: string; dot: string }> = {
  signal: {
    border: 'border-emerald-500/35',
    glow: 'shadow-[0_0_24px_rgba(16,185,129,0.12),inset_0_1px_0_rgba(16,185,129,0.08)]',
    text: 'text-emerald-400/95',
    bar: 'border-emerald-500/25 bg-emerald-950/40',
    dot: 'bg-emerald-400',
  },
  rx: {
    border: 'border-cyan-500/35',
    glow: 'shadow-[0_0_24px_rgba(34,211,238,0.12),inset_0_1px_0_rgba(34,211,238,0.08)]',
    text: 'text-cyan-300/95',
    bar: 'border-cyan-500/25 bg-cyan-950/40',
    dot: 'bg-cyan-400',
  },
}

export function WebSeatTerminalLog({
  title,
  body,
  hint,
  accent = 'signal',
}: {
  title: string
  body: string
  hint?: string
  accent?: Accent
}) {
  const preRef = useRef<HTMLPreElement>(null)
  const a = accentStyles[accent]
  useEffect(() => {
    const el = preRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [body])
  const display = (body || '').trim() || '—'
  return (
    <div className={cn('relative overflow-hidden rounded-lg border bg-[#070b10]', a.border, a.glow)}>
      <div className={cn('relative z-[2] flex items-center gap-2 border-b px-3 py-2', a.bar)}>
        <span className="font-mono text-[10px] font-semibold uppercase tracking-[0.2em] text-white/50">{title}</span>
        <span className="ml-auto flex items-center gap-1.5 font-mono text-[9px] text-white/35">
          <span className={cn('h-1 w-1 animate-pulse rounded-full', a.dot)} /> STREAM
        </span>
      </div>
      {hint ? <p className="relative z-[2] border-b border-white/5 px-3 py-1.5 font-mono text-[10px] leading-snug text-white/40"><span className="text-white/25"># </span>{hint}</p> : null}
      <pre ref={preRef} className={cn('relative z-[2] max-h-[min(14rem,42vh)] overflow-x-auto overflow-y-auto px-3 py-2.5 font-mono text-[11px] leading-relaxed', a.text)}>
        {display}
      </pre>
      <div className="relative z-[2] border-t border-white/5 px-3 py-1 font-mono text-[10px] text-white/25">
        <span className={cn(accent === 'rx' ? 'text-cyan-500/70' : 'text-emerald-500/65')}>{'>'}</span> <span className={cn('inline-block h-3 w-2 animate-pulse opacity-80', accent === 'rx' ? 'bg-cyan-400' : 'bg-emerald-400')} />
      </div>
    </div>
  )
}
