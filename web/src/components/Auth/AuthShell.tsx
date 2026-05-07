import { ReactNode } from 'react'
import { Button, Select } from '@arco-design/web-react'
import { IconMoon, IconSun } from '@arco-design/web-react/icon'
import { useThemeStore } from '@/stores/themeStore'
import { useLocaleStore } from '@/stores/localeStore'

interface AuthShellProps {
  title: string
  subtitle?: string
  children: ReactNode
  footer?: ReactNode
}

export default function AuthShell({ title, subtitle, children, footer }: AuthShellProps) {
  const { toggleMode, isDark } = useThemeStore()
  const locale = useLocaleStore((s) => s.locale)
  const setLocale = useLocaleStore((s) => s.setLocale)

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '32px 16px',
        background: 'var(--color-bg-1)',
        boxSizing: 'border-box',
      }}
    >
      <div style={{ position: 'fixed', top: 20, right: 20, zIndex: 2 }}>
        <Select
          size="small"
          value={locale}
          style={{ width: 98, marginRight: 8 }}
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
      </div>
      <div
        style={{
          width: '100%',
          maxWidth: 420,
          borderRadius: 12,
          border: '1px solid var(--color-border)',
          background: 'var(--color-bg-2)',
          boxShadow: '0 12px 40px rgba(15, 23, 42, 0.06)',
          padding: '40px 36px 36px',
          boxSizing: 'border-box',
        }}
      >
        <div style={{ marginBottom: 28 }}>
          <h1
            style={{
              margin: 0,
              fontSize: 22,
              fontWeight: 600,
              letterSpacing: '-0.02em',
              color: 'var(--color-text-1)',
            }}
          >
            {title}
          </h1>
          {subtitle ? (
            <p style={{ margin: '10px 0 0', fontSize: 14, color: 'var(--color-text-3)', lineHeight: 1.5 }}>
              {subtitle}
            </p>
          ) : null}
        </div>
        {children}
        {footer ? <div style={{ marginTop: 24 }}>{footer}</div> : null}
      </div>
    </div>
  )
}
