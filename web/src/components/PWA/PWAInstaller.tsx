import { useState, useEffect } from 'react'
import { Zap, WifiOff, Sparkles, X } from 'lucide-react'
import { cn } from '@/utils/cn.ts'

interface BeforeInstallPromptEvent extends Event {
  prompt(): Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

declare global {
  interface Navigator {
    standalone?: boolean
  }
}

interface PWAInstallerProps {
  className?: string
  showOnLoad?: boolean
  delay?: number
  position?: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right'
}

const PWAInstaller = ({
  className = '',
  showOnLoad = true,
  delay = 3000,
  position = 'bottom-right',
}: PWAInstallerProps) => {
  const [deferredPrompt, setDeferredPrompt] = useState<BeforeInstallPromptEvent | null>(null)
  const [isVisible, setIsVisible] = useState(false)
  const [isInstalled, setIsInstalled] = useState(false)
  const [isInstalling, setIsInstalling] = useState(false)

  useEffect(() => {
    const checkInstalled = () => {
      if (window.matchMedia('(display-mode: standalone)').matches) {
        setIsInstalled(true)
        return
      }
      if (window.navigator.standalone === true) {
        setIsInstalled(true)
      }
    }
    checkInstalled()
  }, [])

  useEffect(() => {
    const handleBeforeInstallPrompt = (e: Event) => {
      e.preventDefault()
      setDeferredPrompt(e as BeforeInstallPromptEvent)
      if (showOnLoad) {
        setTimeout(() => setIsVisible(true), delay)
      }
    }
    window.addEventListener('beforeinstallprompt', handleBeforeInstallPrompt)
    return () => window.removeEventListener('beforeinstallprompt', handleBeforeInstallPrompt)
  }, [showOnLoad, delay])

  useEffect(() => {
    const handleAppInstalled = () => {
      setIsInstalled(true)
      setIsVisible(false)
      setDeferredPrompt(null)
    }
    window.addEventListener('appinstalled', handleAppInstalled)
    return () => window.removeEventListener('appinstalled', handleAppInstalled)
  }, [])

  const handleInstall = async () => {
    if (!deferredPrompt) return
    setIsInstalling(true)
    try {
      await deferredPrompt.prompt()
      await deferredPrompt.userChoice
    } catch (error) {
      console.error('安装过程中出错:', error)
    } finally {
      setIsInstalling(false)
      setDeferredPrompt(null)
      setIsVisible(false)
    }
  }

  const handleClose = () => setIsVisible(false)

  const getPositionStyles = () => {
    const base = 'fixed z-50'
    switch (position) {
      case 'top-left':
        return `${base} top-4 left-4`
      case 'top-right':
        return `${base} top-4 right-4`
      case 'bottom-left':
        return `${base} bottom-4 left-4`
      case 'bottom-right':
      default:
        return `${base} bottom-4 right-4`
    }
  }

  if (isInstalled || !deferredPrompt) return null

  return (
    isVisible && (
      <div className={cn('max-w-sm w-full', getPositionStyles(), className)}>
        <div
          className="rounded-xl shadow-2xl overflow-hidden border"
          style={{
            background: 'var(--color-bg-2)',
            borderColor: 'var(--color-border)',
          }}
        >
          <div
            className="p-4 flex items-center justify-between"
            style={{
              background: 'linear-gradient(110deg, #5B21B6 0%, #7C3AED 45%, #4C1D95 100%)',
              color: '#fff',
            }}
          >
            <div>
              <h3 className="font-bold text-sm">安装应用</h3>
              <p className="text-xs opacity-90">获得更好的体验</p>
            </div>
            <button type="button" onClick={handleClose} className="text-white/70 hover:text-white p-1">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="p-4">
            <div className="space-y-3">
              <div className="flex items-start gap-3">
                <div className="w-8 h-8 rounded-full flex items-center justify-center shrink-0 mt-0.5 bg-green-100">
                  <Zap className="w-4 h-4 text-green-600" />
                </div>
                <div>
                  <h4 className="font-semibold text-sm" style={{ color: 'var(--color-text-1)' }}>
                    更快访问
                  </h4>
                  <p className="text-xs" style={{ color: 'var(--color-text-3)' }}>
                    无需打开浏览器，直接启动应用
                  </p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <div className="w-8 h-8 rounded-full flex items-center justify-center shrink-0 mt-0.5 bg-violet-100">
                  <WifiOff className="w-4 h-4 text-violet-600" />
                </div>
                <div>
                  <h4 className="font-semibold text-sm" style={{ color: 'var(--color-text-1)' }}>
                    离线使用
                  </h4>
                  <p className="text-xs" style={{ color: 'var(--color-text-3)' }}>
                    支持离线访问，随时随地使用
                  </p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <div className="w-8 h-8 rounded-full flex items-center justify-center shrink-0 mt-0.5 bg-purple-100">
                  <Sparkles className="w-4 h-4 text-purple-600" />
                </div>
                <div>
                  <h4 className="font-semibold text-sm" style={{ color: 'var(--color-text-1)' }}>
                    原生体验
                  </h4>
                  <p className="text-xs" style={{ color: 'var(--color-text-3)' }}>
                    享受接近原生应用的流畅体验
                  </p>
                </div>
              </div>
            </div>

            <div className="mt-4 flex gap-2">
              <button
                type="button"
                onClick={handleInstall}
                disabled={isInstalling}
                className={cn(
                  'flex-1 font-semibold py-2.5 px-4 rounded-lg text-sm text-white transition-all',
                  'disabled:opacity-50 disabled:cursor-not-allowed active:scale-[0.98]'
                )}
                style={{
                  background: 'linear-gradient(110deg, #5B21B6 0%, #7C3AED 100%)',
                }}
              >
                {isInstalling ? (
                  <span className="inline-flex items-center justify-center gap-2">
                    <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    安装中...
                  </span>
                ) : (
                  '立即安装'
                )}
              </button>
              <button
                type="button"
                onClick={handleClose}
                className="px-4 py-2.5 font-medium text-sm transition-colors"
                style={{ color: 'var(--color-text-3)' }}
              >
                稍后
              </button>
            </div>
          </div>
        </div>
      </div>
    )
  )
}

export default PWAInstaller
