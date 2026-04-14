import { ReactNode, useEffect, useState } from 'react'
import { motion } from 'framer-motion'
import AdminSidebar from './AdminSidebar'
import { ChevronLeft, ChevronRight, Moon, Sun } from 'lucide-react'
import { useThemeStore } from '@/stores/themeStore'
import { useSidebar } from '@/contexts/SidebarContext'
import Button from '../UI/Button'

interface AdminLayoutProps {
  children: ReactNode
  title?: string
  description?: string
  actions?: ReactNode
}

const AdminLayout = ({ children, title, description, actions }: AdminLayoutProps) => {
  const { toggleMode, isDark } = useThemeStore()
  const { isCollapsed, toggleCollapse } = useSidebar()
  const [isDesktop, setIsDesktop] = useState(false)

  useEffect(() => {
    const checkDesktop = () => {
      setIsDesktop(window.innerWidth >= 1024)
    }
    checkDesktop()
    window.addEventListener('resize', checkDesktop)
    return () => window.removeEventListener('resize', checkDesktop)
  }, [])

  return (
    <div className="min-h-screen bg-gradient-to-br from-sky-50 via-blue-50/50 to-cyan-50/40 dark:from-slate-950 dark:via-slate-900 dark:to-slate-950">
      <AdminSidebar />
      
      {/* 主内容区 - 移动端无左边距，桌面端根据侧边栏状态调整 */}
      <div
        className="transition-all duration-200 ease-in-out"
        style={{
          marginLeft: isDesktop ? (isCollapsed ? '80px' : '220px') : '0px',
        }}
      >
        {/* 顶部导航栏 */}
        <header className="sticky top-0 z-20 bg-white/80 dark:bg-slate-950/80 backdrop-blur-xl border-b border-slate-200/50 dark:border-slate-800/50 shadow-sm">
          <div className="px-4 sm:px-6 lg:px-8">
            <div className="flex items-center justify-between h-16">
              {/* 左侧：Logo、标题和描述 */}
              <div className="flex items-center gap-4 flex-1 min-w-0">
                <Button
                  variant="ghost"
                  size="sm"
                  className="hidden lg:inline-flex"
                  onClick={toggleCollapse}
                  leftIcon={isCollapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
                  title={isCollapsed ? '展开侧边栏' : '折叠侧边栏'}
                />
                <div className="flex-1 min-w-0">
                  {title && (
                    <h1 className="text-xl font-semibold text-slate-900 dark:text-white">
                      {title}
                    </h1>
                  )}
                  {description && (
                    <p className="text-sm text-slate-500 dark:text-slate-400 mt-0.5">
                      {description}
                    </p>
                  )}
                </div>
              </div>

              {/* 右侧：操作按钮 */}
              <div className="flex items-center gap-2">
                {/* 主题切换 */}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={toggleMode}
                  leftIcon={isDark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
                />

                {/* 自定义操作 */}
                {actions}
              </div>
            </div>
          </div>
        </header>

        {/* 页面内容 */}
        <main className="px-4 sm:px-6 lg:px-8 py-6 lg:py-8">
          <motion.div
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.3 }}
          >
            {children}
          </motion.div>
        </main>
      </div>
    </div>
  )
}

export default AdminLayout

