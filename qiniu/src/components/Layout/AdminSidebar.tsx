import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Users,
  Phone,
  Hash,
  PhoneCall,
  FileText,
  Headphones,
  Menu,
  X,
} from 'lucide-react'
import { useSidebar } from '@/contexts/SidebarContext'
import { useSiteConfig } from '@/contexts/SiteConfigContext'
import { cn } from '@/utils/cn'
import { useThemeStore } from '@/stores/themeStore'
import { useWebSeat } from '@/components/WebSeat/WebSeatContext'

interface NavItem {
  name: string
  href: string
  icon: React.ComponentType<{ className?: string }>
  badge?: number
  children?: NavItem[]
}

const AdminSidebar = () => {
  const { isCollapsed } = useSidebar()
  const [isMobileOpen, setIsMobileOpen] = useState(false)
  const location = useLocation()
  const { config } = useSiteConfig()
  const { isDark } = useThemeStore()
  const { configured, wsState, wsStatusText, goOnline } = useWebSeat()
  const siteName = config?.SITE_NAME || '七牛云联络中心'
  const logoUrl = isDark ? '/logo-white.png' : '/logo-grey.png'

  const navigation: NavItem[] = [
    { name: 'SIP 用户', href: '/sip-users', icon: Users },
    { name: '通话记录', href: '/call-records', icon: Phone },
    { name: '号码池', href: '/number-pool', icon: Hash },
    { name: '外呼任务', href: '/outbound-tasks', icon: PhoneCall },
    { name: '脚本管理', href: '/script-manager', icon: FileText },
    { name: 'Web 坐席', href: '/web-agents', icon: Headphones },
  ]

  const isActive = (path: string) => {
    return location.pathname === path || location.pathname.startsWith(path + '/')
  }

  const SidebarContent = ({ showLogo = true }: { showLogo?: boolean }) => {
    const { config: sidebarConfig } = useSiteConfig()
    const currentSiteName = sidebarConfig?.SITE_NAME || '七牛云联络中心'
    const sidebarLogoUrl = (isDark ? '/logo-white.png' : '/logo-grey.png')
    
    return (
      <>
        {/* Logo区域 - 只在桌面端显示，移动端不显示（因为移动端侧边栏已经有logo了） */}
        {showLogo && (
          <div className="h-16 flex items-center justify-between px-4 border-b border-slate-200 dark:border-slate-700">
            {!isCollapsed && (
              <Link to="/sip-users" className="flex items-center gap-3 group">
                <div className="relative">
                  <div className="absolute inset-0 bg-gradient-to-br from-sky-400 via-blue-500 to-cyan-500 rounded-lg blur-sm opacity-50 group-hover:opacity-75 transition-opacity" />
                  <div className="relative w-10 h-10 rounded-lg from-sky-400 via-blue-500 to-cyan-500 flex items-center justify-center shadow-lg">
                    <img 
                      src={sidebarLogoUrl} 
                      alt={currentSiteName} 
                      className="w-9 h-9 object-contain"
                      onError={(e) => {
                        const target = e.target as HTMLImageElement
                        target.style.display = 'none'
                        const parent = target.parentElement
                        if (parent) {
                          parent.innerHTML = '<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>'
                        }
                      }}
                    />
                  </div>
                </div>
                <span className="font-bold text-lg bg-clip-text whitespace-nowrap">
                  {currentSiteName}
                </span>
              </Link>
            )}
          {isCollapsed && (
            <div className="relative w-8 h-10 rounded-lg flex items-center justify-center mx-auto">
              <img 
                src={sidebarLogoUrl} 
                alt={currentSiteName} 
                className="w-7 h-7 object-contain"
                onError={(e) => {
                  const target = e.target as HTMLImageElement
                  target.style.display = 'none'
                  const parent = target.parentElement
                  if (parent) {
                    parent.innerHTML = '<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>'
                  }
                }}
              />
            </div>
          )}
        </div>
      )}

      {/* 导航菜单 */}
      <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        {navigation.map((item) => {
          const Icon = item.icon
          const itemActive = isActive(item.href)

          return (
            <Link
              key={item.name}
              to={item.href}
              className={cn(
                'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors',
                itemActive
                  ? 'bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400'
                  : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
                isCollapsed && 'justify-center'
              )}
              title={isCollapsed ? item.name : ''}
            >
              <Icon className="w-5 h-5" />
              {!isCollapsed && (
                <span className="flex-1 whitespace-nowrap">{item.name}</span>
              )}
              {item.badge && !isCollapsed && (
                <span className="px-2 py-0.5 text-xs font-medium bg-blue-100 dark:bg-blue-900 text-blue-600 dark:text-blue-400 rounded-full">
                  {item.badge}
                </span>
              )}
            </Link>
          )
        })}
      </nav>

      {!isCollapsed && (
        <div className="px-3 pb-3">
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/60 p-2.5">
            <p className="text-xs font-medium text-slate-700 dark:text-slate-200">
              {configured
                ? `WS：${wsState === 'open' ? '已上线' : wsStatusText.replace(/^WS：/, '')}`
                : 'WS：未配置'}
            </p>
            <p className="mt-1 text-[11px] text-slate-500 dark:text-slate-400">
              {wsState === 'open' ? '连接正常，可接听来电' : '请点击「上线」建立连接'}
            </p>
            {configured && wsState !== 'open' && (
              <button
                type="button"
                onClick={() => void goOnline()}
                className="mt-2 text-[11px] rounded px-2 py-1 bg-blue-600 text-white hover:bg-blue-700"
              >
                上线
              </button>
            )}
          </div>
        </div>
      )}

    </>
    )
  }

  return (
    <>
      {/* 移动端菜单按钮 */}
      <button
        onClick={() => setIsMobileOpen(true)}
        className="lg:hidden fixed top-4 left-4 z-50 p-2 rounded-lg bg-white dark:bg-slate-800 shadow-lg border border-slate-200 dark:border-slate-700"
      >
        <Menu className="w-5 h-5 text-slate-700 dark:text-slate-300" />
      </button>

      {/* 移动端遮罩 */}
      <AnimatePresence>
        {isMobileOpen && (
          <>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              onClick={() => setIsMobileOpen(false)}
              className="lg:hidden fixed inset-0 bg-black/50 z-40"
            />
            <motion.aside
              initial={{ x: -280 }}
              animate={{ x: 0 }}
              exit={{ x: -280 }}
              transition={{ type: 'spring', damping: 25, stiffness: 200 }}
              className="lg:hidden fixed left-0 top-0 bottom-0 w-70 bg-white/95 dark:bg-slate-950/95 backdrop-blur-xl border-r border-slate-200/50 dark:border-slate-800/50 shadow-xl z-50 flex flex-col"
            >
              <div className="h-16 flex items-center justify-between px-4 border-b border-slate-200 dark:border-slate-700">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-sky-400 via-blue-500 to-cyan-500 flex items-center justify-center shadow-lg">
                    <img 
                      src={logoUrl} 
                      alt={siteName} 
                      className="w-6 h-6 object-contain"
                      onError={(e) => {
                        const target = e.target as HTMLImageElement
                        target.style.display = 'none'
                        const parent = target.parentElement
                        if (parent) {
                          parent.innerHTML = '<svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>'
                        }
                      }}
                    />
                  </div>
                  <span className="font-bold text-lg bg-gradient-to-r from-sky-600 via-blue-600 to-cyan-600 bg-clip-text text-transparent">
                    {siteName}
                  </span>
                </div>
                <button
                  onClick={() => setIsMobileOpen(false)}
                  className="p-2 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  <X className="w-5 h-5 text-slate-700 dark:text-slate-300" />
                </button>
              </div>
              <SidebarContent showLogo={false} />
            </motion.aside>
          </>
        )}
      </AnimatePresence>

      {/* 桌面端侧边栏 */}
      <motion.aside
        initial={false}
        animate={{ width: isCollapsed ? 80 : 220 }}
        transition={{ duration: 0.2, ease: 'easeInOut' }}
        className="hidden lg:flex flex-col bg-white/95 dark:bg-slate-950/95 backdrop-blur-xl border-r border-slate-200/50 dark:border-slate-800/50 shadow-lg fixed left-0 top-0 bottom-0 z-30"
      >
        <SidebarContent showLogo={true} />
      </motion.aside>

    </>
  )
}

export default AdminSidebar

