import { useState } from 'react'
import { Avatar, Button, Drawer, Layout, Menu } from '@arco-design/web-react'
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
  KeyRound,
  UserCircle,
  Shield,
  Building2,
  Briefcase,
  Contact,
  Mic2,
} from 'lucide-react'
import { useSidebar } from '@/contexts/SidebarContext'
import { useSiteConfig } from '@/contexts/SiteConfigContext'
import { useAuthStore } from '@/stores/authStore'
import { useTranslation } from '@/i18n'

const Sider = Layout.Sider

type NavDef = {
  labelKey: string
  href: string
  icon: typeof Users
  /** 租户登录：仅当 effective 权限含该菜单码时展示（平台管理员不按此项过滤） */
  tenantMenuCode?: string
  /** 满足任一权限码即可展示（用于菜单/API 权限并存） */
  tenantMenuAnyOf?: string[]
}

const navigation: NavDef[] = [
  { labelKey: 'nav.overview', href: '/overview', icon: LayoutDashboard, tenantMenuCode: 'menu.workspace.overview' },
  { labelKey: 'nav.sipUsers', href: '/sip-users', icon: Users },
  { labelKey: 'nav.callRecords', href: '/call-records', icon: Phone, tenantMenuCode: 'menu.tel.records' },
  { labelKey: 'nav.numberPool', href: '/number-pool', icon: Hash, tenantMenuCode: 'menu.res.pool' },
  { labelKey: 'nav.sipTrunks', href: '/sip-trunks', icon: RadioTower },
  { labelKey: 'nav.sipTrunkNumbers', href: '/sip-trunk-numbers', icon: ListOrdered },
  { labelKey: 'nav.tenantManagement', href: '/tenant-management', icon: Briefcase },
  { labelKey: 'nav.platformAdmins', href: '/platform-admins', icon: Shield },
  { labelKey: 'nav.outboundTasks', href: '/outbound-tasks', icon: PhoneCall, tenantMenuCode: 'menu.res.outbound' },
  { labelKey: 'nav.scriptManager', href: '/script-manager', icon: FileText, tenantMenuCode: 'menu.res.script' },
  {
    labelKey: 'nav.voiceClones',
    href: '/voice-clones',
    icon: Mic2,
    tenantMenuAnyOf: ['menu.res.voice', 'api.voice.read', 'api.voice.write'],
  },
  { labelKey: 'nav.webAgents', href: '/web-agents', icon: Headphones, tenantMenuCode: 'menu.tel.webseat' },
  { labelKey: 'nav.accessKeys', href: '/access-keys', icon: KeyRound, tenantMenuCode: 'menu.acc.keys' },
  { labelKey: 'nav.tenantMembers', href: '/tenant-members', icon: Contact, tenantMenuCode: 'menu.org.members' },
  { labelKey: 'nav.departments', href: '/departments', icon: Building2, tenantMenuCode: 'menu.org.dept' },
  { labelKey: 'nav.rolePermissions', href: '/role-permissions', icon: Shield, tenantMenuCode: 'menu.org.role' },
]

function tenantMaySeeItem(menuCodes: readonly string[] | undefined, item: NavDef): boolean {
  const required = item.tenantMenuAnyOf?.length
    ? item.tenantMenuAnyOf
    : item.tenantMenuCode
      ? [item.tenantMenuCode]
      : []
  if (!required.length) return false
  const list = menuCodes ?? []
  if (!list.length) return false
  return required.some((code) => list.includes(code))
}

const platformAdminMenuHrefs = new Set([
  '/sip-users',
  '/call-records',
  '/sip-trunks',
  '/sip-trunk-numbers',
  '/tenant-management',
  '/platform-admins',
])

const tenantHiddenHrefs = new Set([
  '/sip-users',
  '/sip-trunks',
  '/sip-trunk-numbers',
  '/tenant-management',
])

function selectedMenuKey(pathname: string, items: NavDef[]): string {
  const hit =
    items.find((n) => pathname === n.href || pathname.startsWith(`${n.href}/`)) ?? items[0] ?? navigation[0]
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
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const isPlatformAdmin = Boolean(user?.isPlatformAdmin || user?.principal === 'platform')
  const isTenantUser = user?.principal === 'tenant'
  const dn = String(user?.displayName || '').trim()
  const un = String(user?.username || '').trim()
  const em = String(user?.email || '').trim()
  const sidebarUserLabel = dn || un || em || t('nav.me')
  const avatarUrl = isTenantUser ? String(user?.avatarUrl || '').trim() : ''
  const menuCodes = (user?.permissionCodes as string[] | undefined) ?? []
  const visibleNavigation = isPlatformAdmin
    ? navigation.filter((n) => platformAdminMenuHrefs.has(n.href))
    : navigation.filter((n) => {
        if (tenantHiddenHrefs.has(n.href)) return false
        return tenantMaySeeItem(menuCodes, n)
      })
  const siteName = config?.SITE_NAME || '灵语'
  const logoUrl = '/icon-lingyu.png'
  const selected =
    visibleNavigation.length > 0 ? selectedMenuKey(location.pathname, visibleNavigation) : location.pathname

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
            justifyContent: collapsed ? 'center' : 'flex-start',
            width: collapsed ? '100%' : 'auto',
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
        className={collapsed ? 'ling-sidebar-menu--collapsed-icons' : undefined}
        collapse={collapsed}
        style={{ flex: 1, border: 'none', width: '100%' }}
        selectedKeys={[selected]}
        onClickMenuItem={(key) => {
          navigate(key)
          onNavigate?.()
        }}
      >
        {visibleNavigation.map((item) => {
          const Icon = item.icon
          const label = t(item.labelKey)
          return (
            <Menu.Item key={item.href} title={collapsed ? label : undefined}>
              {collapsed ? (
                <span className="ling-sidebar-menu-icon-cell" aria-hidden>
                  <Icon size={18} strokeWidth={2} />
                </span>
              ) : (
                <span className="ling-sidebar-menu-item-label">
                  <Icon size={18} strokeWidth={2} aria-hidden />
                  {label}
                </span>
              )}
            </Menu.Item>
          )
        })}
      </Menu>

      <div
        style={{
          padding: collapsed ? '10px 0' : '10px 12px',
          borderTop: '1px solid var(--color-border)',
          display: 'flex',
          justifyContent: collapsed ? 'center' : 'stretch',
          width: '100%',
          boxSizing: 'border-box',
        }}
      >
        <div
          role="button"
          tabIndex={0}
          className="ling-sidebar-profile-hit"
          onClick={() => {
            navigate('/profile')
            onNavigate?.()
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              navigate('/profile')
              onNavigate?.()
            }
          }}
          style={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            justifyContent: collapsed ? 'center' : 'flex-start',
            gap: 10,
            width: collapsed ? 44 : '100%',
            maxWidth: '100%',
            minHeight: 40,
            padding: collapsed ? 0 : '4px 8px',
            cursor: 'pointer',
            borderRadius: 8,
            boxSizing: 'border-box',
          }}
        >
          <Avatar size={32} style={{ flexShrink: 0, backgroundColor: 'var(--color-fill-3)' }}>
            {isTenantUser && avatarUrl ? (
              <img alt="" src={avatarUrl} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
            ) : (
              <UserCircle size={22} strokeWidth={2} color="var(--color-text-2)" />
            )}
          </Avatar>
          {!collapsed && (
            <span
              style={{
                flex: 1,
                minWidth: 0,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                textAlign: 'left',
                fontSize: 13,
                fontWeight: 500,
                color: 'var(--color-text-1)',
              }}
            >
              {sidebarUserLabel}
            </span>
          )}
        </div>
      </div>
    </>
  )
}

const Sidebar = () => {
  const { isCollapsed } = useSidebar()
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
        className="ling-sidebar-sider hidden lg:block !bg-card !border-border"
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
          boxSizing: 'border-box',
        }}
      >
        <div className="ling-sidebar-root" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
          <NavMenuBody collapsed={isCollapsed} />
        </div>
      </Sider>
    </>
  )
}

export default Sidebar
