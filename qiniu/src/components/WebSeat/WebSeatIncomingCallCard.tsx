import { useEffect, useRef, useState } from 'react'
import { Phone, PhoneOff, Volume2 } from 'lucide-react'
import { cn } from '@/utils/cn'

const RING_SRC = '/ringing.wav'

export function WebSeatIncomingCallCard({
  callId,
  onAnswer,
  onReject,
  className,
}: {
  callId: string
  onAnswer: () => void
  onReject: () => void
  className?: string
}) {
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const [ringNeedsGesture, setRingNeedsGesture] = useState(false)
  useEffect(() => {
    let cancelled = false
    const a = new Audio(RING_SRC)
    a.preload = 'auto'
    a.volume = 0.55
    audioRef.current = a
    const tryPlay = () => {
      if (cancelled) return
      a.loop = true
      void a.play().then(() => !cancelled && setRingNeedsGesture(false)).catch(() => !cancelled && setRingNeedsGesture(true))
    }
    if (a.readyState >= HTMLMediaElement.HAVE_ENOUGH_DATA) tryPlay()
    else {
      a.addEventListener('canplaythrough', tryPlay, { once: true })
      a.load()
    }
    return () => {
      cancelled = true
      a.pause()
      a.removeAttribute('src')
      a.load()
      audioRef.current = null
    }
  }, [callId])

  return (
    <div className={cn('w-[min(calc(100vw-2rem),20rem)] overflow-hidden rounded-2xl border border-border bg-card shadow-2xl', className)}>
      <div className="flex items-center gap-3 border-b border-border/80 bg-muted/40 px-4 py-3">
        <div className="relative flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
          <span className="absolute inset-0 rounded-full bg-primary/20 animate-ping opacity-75 [animation-duration:1.4s]" />
          <Phone className="relative h-6 w-6" strokeWidth={2} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-foreground">Web 坐席来电</p>
          <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">{callId}</p>
          {ringNeedsGesture && (
            <button type="button" className="mt-2 inline-flex items-center gap-1 rounded-md border border-border bg-background px-2 py-1 text-xs" onClick={() => void audioRef.current?.play()}>
              <Volume2 className="h-3.5 w-3.5" /> 点击开启铃声
            </button>
          )}
        </div>
      </div>
      <div className="flex gap-2 p-3">
        <button type="button" className="flex flex-1 items-center justify-center gap-1.5 rounded-xl bg-emerald-600 px-3 py-2.5 text-sm font-medium text-white hover:bg-emerald-700" onClick={() => { audioRef.current?.pause(); onAnswer() }}>
          <Phone className="h-4 w-4" /> 接听
        </button>
        <button type="button" className="flex flex-1 items-center justify-center gap-1.5 rounded-xl border border-border bg-background px-3 py-2.5 text-sm font-medium text-foreground hover:bg-muted" onClick={() => { audioRef.current?.pause(); onReject() }}>
          <PhoneOff className="h-4 w-4" /> 拒接
        </button>
      </div>
    </div>
  )
}
