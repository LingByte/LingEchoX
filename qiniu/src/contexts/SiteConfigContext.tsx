import { createContext, useContext, useState, useEffect, ReactNode } from 'react'
import { type SiteConfig } from '@/services/adminApi'
import { useLocalStorage } from '@/hooks/useLocalStorage'

interface SiteConfigContextType {
  config: SiteConfig | null
  loading: boolean
  error: Error | null
  refresh: () => Promise<void>
  clearCache: () => void
}

const SiteConfigContext = createContext<SiteConfigContextType | undefined>(undefined)

// 缓存键名
const SITE_CONFIG_CACHE_KEY = 'lingstorage_site_config'
const SITE_CONFIG_CACHE_TIMESTAMP_KEY = 'lingstorage_site_config_timestamp'

// 缓存有效期（毫秒）- 24小时
const CACHE_DURATION = 24 * 60 * 60 * 1000

export const SiteConfigProvider = ({ children }: { children: ReactNode }) => {
  const [config, setConfig] = useState<SiteConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  
  // 使用本地存储缓存配置
  const [cachedConfig, setCachedConfig] = useLocalStorage<SiteConfig | null>(SITE_CONFIG_CACHE_KEY, null)
  const [cacheTimestamp, setCacheTimestamp] = useLocalStorage<number>(SITE_CONFIG_CACHE_TIMESTAMP_KEY, 0)

  // 检查缓存是否有效
  const isCacheValid = () => {
    if (!cachedConfig || !cacheTimestamp) return false
    const now = Date.now()
    return (now - cacheTimestamp) < CACHE_DURATION
  }

  // 根据主题更新 head logo（favicon / apple-touch-icon）
  const applyThemeHeadLogo = () => {
    const isDark =
      document.documentElement.classList.contains('dark') ||
      window.matchMedia('(prefers-color-scheme: dark)').matches
    const logoUrl = isDark ? '/logo-white.png' : '/logo-grey.png'

    let faviconLink = document.querySelector("link[rel~='icon']") as HTMLLinkElement | null
    if (!faviconLink) {
      faviconLink = document.createElement('link')
      faviconLink.rel = 'icon'
      document.head.appendChild(faviconLink)
    }
    faviconLink.href = logoUrl

    let appleIcon = document.querySelector("link[rel='apple-touch-icon']") as HTMLLinkElement | null
    if (!appleIcon) {
      appleIcon = document.createElement('link')
      appleIcon.rel = 'apple-touch-icon'
      document.head.appendChild(appleIcon)
    }
    appleIcon.href = logoUrl
  }

  // 应用配置到页面
  const applyConfigToPage = (siteConfig: SiteConfig) => {
    // 更新页面标题
    if (siteConfig.SITE_NAME) {
      document.title = siteConfig.SITE_NAME
      // 同时更新 meta description
      const metaDescription = document.querySelector('meta[name="description"]')
      if (metaDescription) {
        metaDescription.setAttribute('content', siteConfig.SITE_NAME)
      }
      // 更新 apple-mobile-web-app-title
      const appleTitle = document.querySelector('meta[name="apple-mobile-web-app-title"]')
      if (appleTitle) {
        appleTitle.setAttribute('content', siteConfig.SITE_NAME)
      }
    }
    
    applyThemeHeadLogo()
  }

  // 获取默认配置
  const getDefaultConfig = (): SiteConfig => ({
    SITE_NAME: '七牛云联络中心',
    SITE_DESCRIPTION: '管理后台登录',
    SITE_TERMS_URL: '',
    SITE_URL: '',
      SITE_LOGO_URL: '/logo-grey.png',
  })

  const fetchConfig = async (forceRefresh = false) => {
    try {
      setLoading(true)
      setError(null)
      
      // 如果不是强制刷新且缓存有效，使用缓存
      if (!forceRefresh && isCacheValid() && cachedConfig) {
        console.log('使用缓存的站点配置')
        setConfig(cachedConfig)
        applyConfigToPage(cachedConfig)
        setLoading(false)
        return
      }

      // 当前项目不依赖 /public/site-config 接口，避免无效 404 请求，直接使用默认配置
      const defaultConfig = getDefaultConfig()
      setConfig(defaultConfig)
      setCachedConfig(defaultConfig)
      setCacheTimestamp(Date.now())
      applyConfigToPage(defaultConfig)
      
    } catch (err) {
      console.error('获取站点配置失败:', err)
      setError(err instanceof Error ? err : new Error('Failed to load site config'))
      
      // 如果有缓存，即使过期也使用缓存作为降级方案
      if (cachedConfig) {
        console.log('使用过期缓存作为降级方案')
        setConfig(cachedConfig)
        applyConfigToPage(cachedConfig)
      } else {
        // 设置默认值
        const defaultConfig = getDefaultConfig()
        setConfig(defaultConfig)
        applyConfigToPage(defaultConfig)
      }
    } finally {
      setLoading(false)
    }
  }

  // 清除缓存
  const clearCache = () => {
    console.log('清除站点配置缓存')
    setCachedConfig(null)
    setCacheTimestamp(0)
  }

  useEffect(() => {
    fetchConfig()

    const observer = new MutationObserver(() => {
      applyThemeHeadLogo()
    })
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })

    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const onMediaChange = () => applyThemeHeadLogo()
    media.addEventListener('change', onMediaChange)

    return () => {
      observer.disconnect()
      media.removeEventListener('change', onMediaChange)
    }
  }, [])

  return (
    <SiteConfigContext.Provider value={{ 
      config, 
      loading, 
      error, 
      refresh: () => fetchConfig(true), // 强制刷新
      clearCache 
    }}>
      {children}
    </SiteConfigContext.Provider>
  )
}

export const useSiteConfig = () => {
  const context = useContext(SiteConfigContext)
  if (context === undefined) {
    throw new Error('useSiteConfig must be used within a SiteConfigProvider')
  }
  return context
}
