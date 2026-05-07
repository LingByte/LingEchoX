import { type ReactNode } from 'react'
import { ConfigProvider } from '@arco-design/web-react'
import zhCN from '@arco-design/web-react/es/locale/zh-CN'
import enUS from '@arco-design/web-react/es/locale/en-US'
import { useThemeStore } from '@/stores/themeStore'
import { useLocaleStore } from '@/stores/localeStore'

/** Deep purple primary — aligns with Arco Design token `primaryColor`. */
const ARCO_BRAND_THEME = {
  primaryColor: '#5B21B6',
}

export function ArcoAppProvider({ children }: { children: ReactNode }) {
  const isDark = useThemeStore((s) => s.isDark)
  const locale = useLocaleStore((s) => s.locale)
  const arcoLocale = locale === 'en-US' ? enUS : zhCN
  return (
    <ConfigProvider locale={arcoLocale} theme={ARCO_BRAND_THEME}>
      <div className={isDark ? 'arco-theme-dark' : undefined} style={{ minHeight: '100%' }}>
        {children}
      </div>
    </ConfigProvider>
  )
}
