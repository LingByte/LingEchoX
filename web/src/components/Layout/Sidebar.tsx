import { useState } from 'react'
import { Button, Drawer, Layout, Menu } from '@arco-design/web-react'
import {
  IconMenuFold,
  IconMenuUnfold,
} from '@arco-design/web-react/icon'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import {
  LayoutDashboard,
  Users,
  Phone,
  Hash,
  PhoneCall,
  FileText,
  Headphones,
  Menu as MenuIcon,
  RadioTower,
  ListOrdered,
} from 'lucide-react'
import { useSidebar } from '@/contexts/SidebarContext'
import { useSiteConfig } from '@/contexts/SiteConfigContext'
import { useWebSeat } from '@/components/WebSeat/WebSeatContext'
import { useAuthStore } from '@/stores/authStore'

const Sider = Layout.Sider

type NavDef = { name: string; href: string; icon: typeof Users }

const navigation: NavDef[] = [
  { name: '概览', href: '/overview', icon: LayoutDashboard },
  { name: 'SIP 用户', href: '/sip-users', icon: Users },
  { name: '通话记录', href: '/call-records', icon: Phone },
  { name: '号码池', href: '/number-pool', icon: Hash },
  { name: '中继线路', href: '/sip-trunks', icon: RadioTower },
  { name: '中继号码', href: '/sip-trunk-numbers', icon: ListOrdered },
  { name: '外呼任务', href: '/outbound-tasks', icon: PhoneCall },
  { name: '脚本管理', href: '/script-manager', icon: FileText },
  { name: 'Web 坐席', href: '/web-agents', icon: Headphones },
]

function selectedMenuKey(pathname: string): string {
  const hit =
    navigation.find((n) => pathname === n.href || pathname.startsWith(`${n.href}/`)) ?? navigation[0]
  return hit.href
}

function NavMenuBody({
  collapsed,
  onNavigate,
}: {
  collapsed: boolean
  onNavigate?: () => void
}) {
  const location = useLocation()
  const navigate = useNavigate()
  const { config } = useSiteConfig()
  const { configured, wsState, wsStatusText, goOnline } = useWebSeat()
  const user = useAuthStore((s) => s.user)
  const siteName = config?.SITE_NAME || '灵语'
  const logoUrl = '/icon-lingyu.png'
  const selected = selectedMenuKey(location.pathname)

  return (
    <>
      <div
        style={{
          height: 64,
          display: 'flex',
          alignItems: 'center',
          justifyContent: collapsed ? 'center' : 'flex-start',
          padding: collapsed ? 0 : '0 16px',
          borderBottom: '1px solid var(--color-border)',
        }}
      >
        <Link
          to="/overview"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            textDecoration: 'none',
            color: 'var(--color-text-1)',
            overflow: 'hidden',
          }}
          onClick={() => onNavigate?.()}
        >
          <img src={logoUrl} alt={siteName} style={{ width: collapsed ? 28 : 36, height: collapsed ? 28 : 36 }} />
          {!collapsed && (
            <span style={{ fontWeight: 600, fontSize: 17, whiteSpace: 'nowrap' }}>{siteName}</span>
          )}
        </Link>
      </div>

      <Menu
        collapse={collapsed}
        style={{ flex: 1, border: 'none' }}
        selectedKeys={[selected]}
        onClickMenuItem={(key) => {
          navigate(key)
          onNavigate?.()
        }}
      >
        {navigation.map((item) => {
          const Icon = item.icon
          return (
            <Menu.Item key={item.href}>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
                <Icon size={18} strokeWidth={2} />
                {!collapsed && item.name}
              </span>
            </Menu.Item>
          )
        })}
      </Menu>

      {!collapsed && (
        <div style={{ padding: '12px', borderTop: '1px solid var(--color-border)' }}>
          <div
            style={{
              borderRadius: 8,
              padding: 12,
              background: 'var(--color-fill-2)',
              border: '1px solid var(--color-border)',
            }}
          >
            <div style={{ fontSize: 12, fontWeight: 500, color: 'var(--color-text-1)' }}>
              {configured
                ? `WS：${wsState === 'open' ? '已上线' : wsStatusText.replace(/^WS：/, '')}`
                : 'WS：未配置'}
            </div>
            <div style={{ marginTop: 6, fontSize: 11, color: 'var(--color-text-3)' }}>
              {wsState === 'open' ? '连接正常，可接听来电' : '请点击「上线」建立连接'}
            </div>
            {configured && wsState !== 'open' && (
              <Button type="primary" size="mini" style={{ marginTop: 10 }} onClick={() => void goOnline()}>
                上线
              </Button>
            )}
          </div>
        </div>
      )}
      <div style={{ padding: collapsed ? '8px' : '10px 12px', borderTop: '1px solid var(--color-border)' }}>
        <Button
          type="text"
          long={!collapsed}
          size="small"
          onClick={() => {
            navigate('/profile')
            onNavigate?.()
          }}
          style={{ justifyContent: collapsed ? 'center' : 'flex-start' }}
        >
          {collapsed ? '我' : `个人中心 · ${String(user?.displayName || user?.email || '我')}`}
        </Button>
      </div>
    </>
  )
}

const Sidebar = () => {
  const { isCollapsed, toggleCollapse } = useSidebar()
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <>
      <Button
        type="secondary"
        size="small"
        icon={<MenuIcon size={18} />}
        className="lg:hidden"
        style={{ position: 'fixed', top: 16, left: 16, zIndex: 100 }}
        onClick={() => setMobileOpen(true)}
      />

      <Drawer
        title={null}
        visible={mobileOpen}
        placement="left"
        width={280}
        footer={null}
        closable
        onCancel={() => setMobileOpen(false)}
        className="lg:hidden"
      >
        <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
          <NavMenuBody collapsed={false} onNavigate={() => setMobileOpen(false)} />
        </div>
      </Drawer>

      <Sider
        className="hidden lg:block"
        collapsible
        trigger={null}
        collapsed={isCollapsed}
        width={220}
        collapsedWidth={80}
        style={{
          height: '100vh',
          position: 'fixed',
          left: 0,
          top: 0,
          borderRight: '1px solid var(--color-border)',
          boxSizing: 'border-box',
          background: 'var(--color-bg-2)',
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
          <NavMenuBody collapsed={isCollapsed} />
          <div style={{ padding: '8px 12px 12px' }}>
            <Button
              type="secondary"
              size="small"
              long
              icon={isCollapsed ? <IconMenuUnfold /> : <IconMenuFold />}
              onClick={toggleCollapse}
            >
              {!isCollapsed ? '收起' : ''}
            </Button>
          </div>
        </div>
      </Sider>
    </>
  )
}

export default Sidebar
