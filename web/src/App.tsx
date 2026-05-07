import { lazy, Suspense } from 'react'
import { Spin } from '@arco-design/web-react'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import ErrorBoundary from '@/components/ErrorBoundary/ErrorBoundary'
import PWAInstaller from '@/components/PWA/PWAInstaller'
import DevErrorHandler from '@/components/Dev/DevErrorHandler'
import { SidebarProvider } from '@/contexts/SidebarContext'
import { SiteConfigProvider } from '@/contexts/SiteConfigContext'
import { WebSeatProvider } from '@/components/WebSeat/WebSeatProvider'
import { useAuthStore } from '@/stores/authStore'

const Overview = lazy(() => import('@/pages/Overview'))
const SIPUsers = lazy(() => import('@/pages/SIPUsers'))
const CallRecords = lazy(() => import('@/pages/CallRecords'))
const NumberPool = lazy(() => import('@/pages/NumberPool'))
const OutboundTasks = lazy(() => import('@/pages/OutboundTasks'))
const ScriptManager = lazy(() => import('@/pages/ScriptManager'))
const ScriptManagerNew = lazy(() => import('@/pages/ScriptManagerNew'))
const WebAgents = lazy(() => import('@/pages/WebAgents'))
const SIPTrunks = lazy(() => import('@/pages/SIPTrunks'))
const SIPTrunkNumbers = lazy(() => import('@/pages/SIPTrunkNumbers'))
const TenantLogin = lazy(() => import('@/pages/TenantLogin'))
const TenantRegister = lazy(() => import('@/pages/TenantRegister'))
const Profile = lazy(() => import('@/pages/Profile'))

function RequireAuth({ children }: { children: JSX.Element }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const token = useAuthStore((s) => s.token)
  const localToken = typeof window !== 'undefined' ? localStorage.getItem('auth_token') : null
  if (!isAuthenticated && !token && !localToken) {
    return <Navigate to="/login" replace />
  }
  return children
}

function RequirePlatform({ children }: { children: JSX.Element }) {
  const user = useAuthStore((s) => s.user)
  const isPlatform = Boolean(user?.isPlatformAdmin || user?.principal === 'platform')
  if (!isPlatform) {
    return <Navigate to="/overview" replace />
  }
  return children
}

function App() {
  return (
    <ErrorBoundary>
      <SiteConfigProvider>
        <SidebarProvider>
          <Router>
            <WebSeatProvider>
              <div style={{ minHeight: '100vh', background: 'var(--color-bg-1)' }}>
                <Suspense
                  fallback={
                    <div style={{ padding: 48, display: 'flex', justifyContent: 'center' }}>
                      <Spin size={32} tip="页面加载中..." />
                    </div>
                  }
                >
                  <Routes>
                    <Route path="/login" element={<TenantLogin />} />
                    <Route path="/register" element={<TenantRegister />} />
                    <Route path="/tenant/login" element={<Navigate to="/login" replace />} />
                    <Route path="/tenant/register" element={<Navigate to="/register" replace />} />
                    <Route path="/overview" element={<RequireAuth><Overview /></RequireAuth>} />
                    <Route path="/profile" element={<RequireAuth><Profile /></RequireAuth>} />
                    <Route path="/sip-users" element={<RequireAuth><SIPUsers /></RequireAuth>} />
                    <Route path="/call-records" element={<RequireAuth><CallRecords /></RequireAuth>} />
                    <Route path="/number-pool" element={<RequireAuth><NumberPool /></RequireAuth>} />
                    <Route path="/outbound-tasks" element={<RequireAuth><OutboundTasks /></RequireAuth>} />
                    <Route path="/script-manager/new" element={<RequireAuth><ScriptManagerNew /></RequireAuth>} />
                    <Route path="/script-manager" element={<RequireAuth><ScriptManager /></RequireAuth>} />
                    <Route path="/web-agents" element={<RequireAuth><WebAgents /></RequireAuth>} />
                    <Route path="/sip-trunks" element={<RequireAuth><RequirePlatform><SIPTrunks /></RequirePlatform></RequireAuth>} />
                    <Route path="/sip-trunk-numbers" element={<RequireAuth><RequirePlatform><SIPTrunkNumbers /></RequirePlatform></RequireAuth>} />
                    <Route path="/" element={<Navigate to="/overview" replace />} />
                    <Route path="*" element={<Navigate to="/overview" replace />} />
                  </Routes>
                </Suspense>
                <PWAInstaller showOnLoad={true} delay={5000} position="bottom-right" />
                <DevErrorHandler />
              </div>
            </WebSeatProvider>
          </Router>
      </SidebarProvider>
      </SiteConfigProvider>
    </ErrorBoundary>
  )
}

export default App
