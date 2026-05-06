import { type ReactNode } from 'react'
import { ConfigProvider } from '@arco-design/web-react'
import zhCN from '@arco-design/web-react/es/locale/zh-CN'
import { useThemeStore } from '@/stores/themeStore'

/** Deep purple primary — aligns with Arco Design token `primaryColor`. */
const ARCO_BRAND_THEME = {
  primaryColor: '#5B21B6',
}

export function ArcoAppProvider({ children }: { children: ReactNode }) {
  const isDark = useThemeStore((s) => s.isDark)
  return (
    <ConfigProvider locale={zhCN} theme={ARCO_BRAND_THEME}>
      <div className={isDark ? 'arco-theme-dark' : undefined} style={{ minHeight: '100%' }}>
        {children}
      </div>
    </ConfigProvider>
  )
}
