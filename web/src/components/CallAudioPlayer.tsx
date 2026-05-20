import { useRef, useState, useEffect } from 'react'
import { loadProgressiveWavWaveform } from '@/utils/progressiveWavWaveform'

const BAR_COUNT = 100
const PLACEHOLDER_BAR = 14

function decorativeWaveformBars(): number[] {
  return Array.from({ length: BAR_COUNT }, (_, i) => 35 + Math.sin((i / BAR_COUNT) * Math.PI * 6) * 15 + Math.random() * 10)
}

interface CallAudioPlayerProps {
  callId: string
  audioUrl: string
  hasAudio: boolean
  durationSeconds: number | null
}

export default function CallAudioPlayer({
  callId: _,
  audioUrl,
  hasAudio,
  durationSeconds,
}: CallAudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null)
  const waveformRef = useRef<HTMLDivElement>(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [waveformData, setWaveformData] = useState<number[]>([])
  const [waveformProgress, setWaveformProgress] = useState(0)
  const [waveformLoading, setWaveformLoading] = useState(false)

  const duration = durationSeconds || 0

  useEffect(() => {
    if (!audioUrl?.trim()) {
      setWaveformData([])
      setWaveformProgress(0)
      setWaveformLoading(false)
      return
    }

    const ac = new AbortController()
    setWaveformData([])
    setWaveformProgress(0)
    setWaveformLoading(true)

    void loadProgressiveWavWaveform(
      audioUrl.trim(),
      ({ bars, progress }) => {
        setWaveformData(bars)
        setWaveformProgress(progress)
        if (progress >= 1) setWaveformLoading(false)
      },
      { barCount: BAR_COUNT, signal: ac.signal }
    ).catch(() => {
      if (ac.signal.aborted) return
      setWaveformData(decorativeWaveformBars())
      setWaveformProgress(1)
      setWaveformLoading(false)
    })

    return () => ac.abort()
  }, [audioUrl])

  useEffect(() => {
    const audio = audioRef.current
    if (!audio) return
    const onTime = () => setCurrentTime(audio.currentTime)
    const onEnd = () => {
      setIsPlaying(false)
      setCurrentTime(0)
    }
    const onLoad = () => setIsLoading(true)
    const onCanPlay = () => setIsLoading(false)
    const onError = () => {
      setError('音频加载失败')
      setIsLoading(false)
    }
    const onPlay = () => setIsPlaying(true)
    const onPause = () => setIsPlaying(false)
    audio.addEventListener('timeupdate', onTime)
    audio.addEventListener('ended', onEnd)
    audio.addEventListener('loadstart', onLoad)
    audio.addEventListener('canplay', onCanPlay)
    audio.addEventListener('error', onError)
    audio.addEventListener('play', onPlay)
    audio.addEventListener('pause', onPause)
    return () => {
      audio.removeEventListener('timeupdate', onTime)
      audio.removeEventListener('ended', onEnd)
      audio.removeEventListener('loadstart', onLoad)
      audio.removeEventListener('canplay', onCanPlay)
      audio.removeEventListener('error', onError)
      audio.removeEventListener('play', onPlay)
      audio.removeEventListener('pause', onPause)
    }
  }, [])

  const togglePlayPause = async () => {
    const audio = audioRef.current
    if (!audio) return
    if (isPlaying) audio.pause()
    else await audio.play()
  }

  const handleWaveformClick = async (e: React.MouseEvent<HTMLDivElement>) => {
    const audio = audioRef.current
    const waveform = waveformRef.current
    if (!audio || !waveform || !duration) return
    const rect = waveform.getBoundingClientRect()
    const percentage = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    audio.currentTime = percentage * duration
    if (!isPlaying) await audio.play()
  }

  const formatTime = (seconds: number) => `${Math.floor(seconds / 60)}:${Math.floor(seconds % 60).toString().padStart(2, '0')}`
  if (!hasAudio) return null
  const progress = duration > 0 ? (currentTime / duration) * 100 : 0
  const receivedBars = Math.floor(waveformProgress * BAR_COUNT)

  const barHeight = (i: number) => {
    if (i < receivedBars && waveformData[i] != null) return Math.max(20, waveformData[i])
    return PLACEHOLDER_BAR
  }

  return (
    <div className="bg-white dark:bg-gray-800 shadow rounded-lg p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-base font-semibold text-gray-900 dark:text-white">通话录音</h3>
        <div className="text-sm text-gray-500 dark:text-gray-400 font-mono">
          {formatTime(currentTime)} / {formatTime(duration)}
          {waveformLoading && (
            <span className="ml-2 text-xs text-muted-foreground">
              波形 {Math.round(waveformProgress * 100)}%
            </span>
          )}
        </div>
      </div>
      <audio ref={audioRef} src={audioUrl} preload="metadata" />
      {error && <div className="mb-3 text-sm text-red-500">{error}</div>}
      <div className="flex items-center gap-3">
        <button
          onClick={() => void togglePlayPause()}
          disabled={isLoading}
          className="flex-shrink-0 w-10 h-10 rounded-full bg-black hover:bg-gray-800 disabled:bg-gray-400 text-white flex items-center justify-center transition-colors"
        >
          {isPlaying ? 'II' : '▶'}
        </button>
        <div
          ref={waveformRef}
          onClick={(e) => void handleWaveformClick(e)}
          className="flex-1 h-14 relative bg-gray-100 dark:bg-gray-700 rounded cursor-pointer overflow-hidden group"
        >
          {waveformLoading && (
            <div
              className="absolute inset-y-0 left-0 bg-primary/10 z-[5] pointer-events-none transition-[width] duration-150"
              style={{ width: `${waveformProgress * 100}%` }}
            />
          )}
          <div className="absolute inset-0 flex items-center justify-around px-1 z-10">
            {Array.from({ length: BAR_COUNT }, (_, i) => (
              <div
                key={i}
                className={`w-0.5 rounded-full transition-[height] duration-75 ${
                  (i / BAR_COUNT) * 100 < progress ? 'bg-black dark:bg-white' : 'bg-gray-300 dark:bg-gray-600'
                } ${i >= receivedBars ? 'opacity-40' : ''}`}
                style={{ height: `${barHeight(i)}%` }}
              />
            ))}
          </div>
          <div className="absolute top-0 bottom-0 w-0.5 bg-red-500 z-20" style={{ left: `${progress}%` }} />
        </div>
      </div>
    </div>
  )
}
