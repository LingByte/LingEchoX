import { useState, useEffect, useRef } from 'react'
import { RefreshCw, MousePointer2 } from 'lucide-react'
import { get, post } from '@/utils/request'
import { getApiBaseURL } from '@/config/apiConfig'

export interface CaptchaData {
  id: string
  type: 'image' | 'click'
  data: any
  expires: string
}

interface CaptchaProps {
  onVerify: (captchaId: string, captchaType: string, captchaData: any) => void
  onError?: (error: string) => void
}

const Captcha = ({ onVerify, onError }: CaptchaProps) => {
  const [captcha, setCaptcha] = useState<CaptchaData | null>(null)
  const [loading, setLoading] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [verified, setVerified] = useState(false)
  
  // Image captcha
  const [imageCode, setImageCode] = useState('')
  
  // Click captcha
  const [clickedPositions, setClickedPositions] = useState<Array<{ x: number; y: number }>>([])
  const clickImageRef = useRef<HTMLImageElement>(null)

  // Load captcha
  const loadCaptcha = async () => {
    setLoading(true)
    setError(null)
    try {
      const response = await get<CaptchaData>(
        `${getApiBaseURL()}/auth/captcha`
      )
      
      if (response.code === 200 && response.data) {
        // 清理重复的 base64 前缀
        const cleanData = { ...response.data }
        if (cleanData.data) {
          // 处理所有可能的图片字段
          const imageFields = ['image']
          imageFields.forEach(field => {
            if (cleanData.data[field] && typeof cleanData.data[field] === 'string') {
              // 如果已经包含 data:image/png;base64, 前缀，移除重复的部分
              let imgData = cleanData.data[field]
              // 移除所有重复的 data:image/png;base64, 前缀，只保留最后一个
              while (imgData.includes('data:image/png;base64,data:image/png;base64,')) {
                imgData = imgData.replace('data:image/png;base64,data:image/png;base64,', 'data:image/png;base64,')
              }
              cleanData.data[field] = imgData
            }
          })
        }
        setCaptcha(cleanData)
        // Reset states based on type
        setImageCode('')
        setClickedPositions([])
        setVerified(false)
      } else {
        throw new Error(response.msg || 'Failed to load captcha')
      }
    } catch (err: any) {
      const errorMsg = err?.msg || err?.message || 'Failed to load captcha'
      setError(errorMsg)
      onError?.(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadCaptcha()
  }, [])

  // Verify captcha
  const verifyCaptcha = async (data: any) => {
    if (!captcha) return

    setVerifying(true)
    try {
      const response = await post(`${getApiBaseURL()}/auth/captcha/verify`, {
        id: captcha.id,
        type: captcha.type,
        data: data
      })

      if (response.code === 200 && response.data?.valid) {
        // Verification successful, call onVerify with the data
        // For image type, data is the code string
        // For other types, data is position/positions object/array
        onVerify(captcha.id, captcha.type, data)
        setError(null)
        setVerified(true)
      } else {
        throw new Error(response.msg || 'Verification failed')
      }
    } catch (err: any) {
      const errorMsg = err?.msg || err?.message || 'Verification failed'
      setError(errorMsg)
      onError?.(errorMsg)
      // Reload captcha on failure
      loadCaptcha()
    } finally {
      setVerifying(false)
    }
  }

  // Handle image captcha submit
  const handleImageSubmit = () => {
    if (!imageCode.trim()) {
      setError('Please enter the captcha code')
      return
    }
    // For image captcha, verify first, then call onVerify on success
    if (captcha) {
      verifyCaptcha(imageCode)
    }
  }

  // Handle click captcha
  const handleClickImage = (e: React.MouseEvent<HTMLImageElement>) => {
    if (!clickImageRef.current) return
    const rect = clickImageRef.current.getBoundingClientRect()
    const img = clickImageRef.current
    
    // 计算点击位置相对于图片的坐标
    // 需要考虑图片的实际显示尺寸和原始尺寸的缩放比例
    const scaleX = img.naturalWidth / rect.width
    const scaleY = img.naturalHeight / rect.height
    
    const x = Math.round((e.clientX - rect.left) * scaleX)
    const y = Math.round((e.clientY - rect.top) * scaleY)
    
    const newPositions = [...clickedPositions, { x, y }]
    setClickedPositions(newPositions)
    
    // If we've clicked enough positions, verify
    if (captcha?.data?.count && newPositions.length >= captcha.data.count) {
      verifyCaptcha(newPositions)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center p-8">
        <RefreshCw className="w-5 h-5 animate-spin text-slate-400" />
        <span className="ml-2 text-sm text-slate-500">Loading captcha...</span>
      </div>
    )
  }

  if (!captcha) {
    return (
      <div className="text-center p-4">
        <p className="text-sm text-red-500 mb-2">{error || 'Failed to load captcha'}</p>
        <button
          onClick={loadCaptcha}
          className="text-sm text-blue-600 hover:text-blue-700 underline"
        >
          Retry
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Image Captcha */}
      {captcha.type === 'image' && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <label className="text-sm font-medium text-slate-700 dark:text-slate-300">
              Verification Code
            </label>
            <button
              type="button"
              onClick={loadCaptcha}
              className="text-xs text-blue-600 hover:text-blue-700 flex items-center gap-1"
            >
              <RefreshCw className="w-3 h-3" />
              Refresh
            </button>
          </div>
          {verified ? (
            <div className="flex items-center gap-2 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-md">
              <svg className="w-5 h-5 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
              <span className="text-sm text-green-600 dark:text-green-400">Verification successful</span>
            </div>
          ) : (
            <div className="flex items-center gap-3">
              <img
                src={captcha.data?.image?.startsWith('data:') ? captcha.data.image : `data:image/png;base64,${captcha.data?.image}`}
                alt="Captcha"
                className="h-12 border border-slate-200 dark:border-slate-700 rounded"
              />
              <input
                type="text"
                value={imageCode}
                onChange={(e) => setImageCode(e.target.value)}
                placeholder="Enter code"
                className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-slate-700 dark:text-white"
                disabled={verifying}
                onKeyPress={(e) => {
                  if (e.key === 'Enter' && imageCode.trim() && !verifying) {
                    handleImageSubmit()
                  }
                }}
              />
              <button
                type="button"
                onClick={handleImageSubmit}
                disabled={verifying || !imageCode.trim()}
                className="px-4 py-2 bg-blue-600 text-white rounded-md text-sm hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {verifying ? 'Verifying...' : 'Verify'}
              </button>
            </div>
          )}
        </div>
      )}

      {/* Click Captcha */}
      {captcha.type === 'click' && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <label className="text-sm font-medium text-slate-700 dark:text-slate-300 flex items-center gap-2">
              <MousePointer2 className="w-4 h-4" />
              Click on the specified positions ({clickedPositions.length}/{captcha.data?.count || 0})
            </label>
            <button
              type="button"
              onClick={loadCaptcha}
              className="text-xs text-blue-600 hover:text-blue-700 flex items-center gap-1"
            >
              <RefreshCw className="w-3 h-3" />
              Refresh
            </button>
          </div>
          {verified ? (
            <div className="flex items-center gap-2 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-md">
              <svg className="w-5 h-5 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
              <span className="text-sm text-green-600 dark:text-green-400">Verification successful</span>
            </div>
          ) : (
            <div className="relative">
            <img
              ref={clickImageRef}
              src={captcha.data?.image?.startsWith('data:') ? captcha.data.image : `data:image/png;base64,${captcha.data?.image}`}
              alt="Click captcha"
              className="w-full border border-slate-200 dark:border-slate-700 rounded-md cursor-crosshair"
              onClick={handleClickImage}
            />
              {clickedPositions.map((pos, idx) => {
                // 计算标记的显示位置（考虑图片缩放）
                const img = clickImageRef.current
                if (!img) return null
                const rect = img.getBoundingClientRect()
                const scaleX = rect.width / (img.naturalWidth || rect.width)
                const scaleY = rect.height / (img.naturalHeight || rect.height)
                const displayX = pos.x * scaleX
                const displayY = pos.y * scaleY
                return (
                  <div
                    key={idx}
                    className="absolute w-4 h-4 bg-blue-500 rounded-full border-2 border-white transform -translate-x-1/2 -translate-y-1/2 pointer-events-none"
                    style={{ left: `${displayX}px`, top: `${displayY}px` }}
                  />
                )
              })}
              {verifying && (
                <div className="absolute inset-0 bg-black/20 rounded-md flex items-center justify-center">
                  <RefreshCw className="w-6 h-6 animate-spin text-white" />
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {error && (
        <p className="text-xs text-red-500 mt-2">{error}</p>
      )}
    </div>
  )
}

export default Captcha
