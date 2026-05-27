import { ReactNode, useEffect, useState } from 'react'
import { Button, Select } from '@arco-design/web-react'
import { IconMoon, IconSun, IconMenuFold, IconMenuUnfold } from '@arco-design/web-react/icon'
import Sidebar from './Sidebar.tsx'
import SIPAgentIncomingBell from '@/components/SIPAgentIncomingBell'
import { useThemeStore } from '@/stores/themeStore'
import { useSidebar } from '@/contexts/SidebarContext'
import { useLocaleStore } from '@/stores/localeStore'

interface AdminLayoutProps {
  children: ReactNode
  title?: string
  description?: string
  actions?: ReactNode
  /** When true, omit the sticky top bar (title strip + locale/theme/collapse). Use with Sidebar extras on specific routes. */
  hideHeader?: boolean
}

const BaseLayout = ({ children, title, description, actions, hideHeader }: AdminLayoutProps) => {
  const { toggleMode, isDark } = useThemeStore()
  const locale = useLocaleStore((s) => s.locale)
  const setLocale = useLocaleStore((s) => s.setLocale)
  const { isCollapsed, toggleCollapse } = useSidebar()
  const [isLg, setIsLg] = useState(() =>
    typeof window !== 'undefined' ? window.matchMedia('(min-width: 1024px)').matches : false
  )

  useEffect(() => {
    const q = window.matchMedia('(min-width: 1024px)')
    const sync = () => setIsLg(q.matches)
    sync()
    q.addEventListener('change', sync)
    return () => q.removeEventListener('change', sync)
  }, [])

  const marginLeft = isLg ? (isCollapsed ? 80 : 220) : 0

  return (
    <div className="min-h-screen bg-background text-foreground">
      <Sidebar />
      <div
        className="min-h-screen bg-background"
        style={{
          marginLeft,
          transition: 'margin-left 0.2s ease',
        }}
      >
        {!hideHeader && (
          <header
            className="sticky top-0 z-10 flex h-16 items-center border-b border-border bg-card px-6 box-border"
          >
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%', gap: 16 }}>
              <div style={{ minWidth: 0, flex: 1 }}>
                {title && (
                  <div className="text-lg font-semibold text-foreground">{title}</div>
                )}
                {description && (
                  <div className="mt-0.5 text-[13px] text-muted-foreground">{description}</div>
                )}
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
                <SIPAgentIncomingBell />
                <Select
                  size="small"
                  value={locale}
                  style={{ width: 98 }}
                  options={[
                    { value: 'zh-CN', label: '中文' },
                    { value: 'en-US', label: 'English' },
                  ]}
                  onChange={(v) => setLocale(v as 'zh-CN' | 'en-US')}
                />
                <Button
                  type="secondary"
                  size="small"
                  icon={isDark ? <IconSun /> : <IconMoon />}
                  onClick={toggleMode}
                />
                {actions}
                {isLg && (
                  <Button
                    type="secondary"
                    size="small"
                    icon={isCollapsed ? <IconMenuUnfold /> : <IconMenuFold />}
                    onClick={toggleCollapse}
                    title={isCollapsed ? '展开侧栏' : '收起侧栏'}
                  />
                )}
              </div>
            </div>
          </header>
        )}
        <main style={{ padding: '24px 24px 40px' }}>{children}</main>
      </div>
    </div>
  )
}

export default BaseLayout
