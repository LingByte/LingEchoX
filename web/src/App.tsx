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
const AccessKeys = lazy(() => import('@/pages/AccessKeys'))
const TenantLogin = lazy(() => import('@/pages/TenantLogin'))
const TenantRegister = lazy(() => import('@/pages/TenantRegister'))
const Profile = lazy(() => import('@/pages/Profile'))
const TenantDepartments = lazy(() => import('@/pages/TenantDepartments'))
const TenantRolePermissions = lazy(() => import('@/pages/TenantRolePermissions'))
const TenantManagement = lazy(() => import('@/pages/TenantManagement'))
const PlatformAdmins = lazy(() => import('@/pages/PlatformAdmins'))
const TenantAiConfig = lazy(() => import('@/pages/TenantAiConfig'))
const TenantMembers = lazy(() => import('@/pages/TenantMembers'))

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

// RequireTenant 限制为非平台管理员的租户用户访问；平台管理员被引导回到 SIP 用户页面（其菜单首项）。
function RequireTenant({ children }: { children: JSX.Element }) {
  const user = useAuthStore((s) => s.user)
  const isPlatform = Boolean(user?.isPlatformAdmin || user?.principal === 'platform')
  if (isPlatform) {
    return <Navigate to="/sip-users" replace />
  }
  return children
}

// HomeRedirect 让平台管理员落到菜单首项 /sip-users，租户继续走工作台 /overview。
function HomeRedirect() {
  const user = useAuthStore((s) => s.user)
  const isPlatform = Boolean(user?.isPlatformAdmin || user?.principal === 'platform')
  return <Navigate to={isPlatform ? '/sip-users' : '/overview'} replace />
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
                    <Route path="/sip-users" element={<RequireAuth><RequirePlatform><SIPUsers /></RequirePlatform></RequireAuth>} />
                    <Route path="/call-records" element={<RequireAuth><CallRecords /></RequireAuth>} />
                    <Route path="/number-pool" element={<RequireAuth><RequireTenant><NumberPool /></RequireTenant></RequireAuth>} />
                    <Route path="/outbound-tasks" element={<RequireAuth><RequireTenant><OutboundTasks /></RequireTenant></RequireAuth>} />
                    <Route path="/script-manager/new" element={<RequireAuth><RequireTenant><ScriptManagerNew /></RequireTenant></RequireAuth>} />
                    <Route path="/script-manager" element={<RequireAuth><RequireTenant><ScriptManager /></RequireTenant></RequireAuth>} />
                    <Route path="/web-agents" element={<RequireAuth><RequireTenant><WebAgents /></RequireTenant></RequireAuth>} />
                    <Route path="/sip-trunks" element={<RequireAuth><RequirePlatform><SIPTrunks /></RequirePlatform></RequireAuth>} />
                    <Route path="/sip-trunk-numbers" element={<RequireAuth><RequirePlatform><SIPTrunkNumbers /></RequirePlatform></RequireAuth>} />
                    <Route
                      path="/tenant-management"
                      element={
                        <RequireAuth>
                          <RequirePlatform>
                            <TenantManagement />
                          </RequirePlatform>
                        </RequireAuth>
                      }
                    />
                    <Route
                      path="/platform-admins"
                      element={
                        <RequireAuth>
                          <RequirePlatform>
                            <PlatformAdmins />
                          </RequirePlatform>
                        </RequireAuth>
                      }
                    />
                    <Route
                      path="/tenant-management/:tenantId/ai"
                      element={
                        <RequireAuth>
                          <RequirePlatform>
                            <TenantAiConfig />
                          </RequirePlatform>
                        </RequireAuth>
                      }
                    />
                    <Route path="/access-keys" element={<RequireAuth><RequireTenant><AccessKeys /></RequireTenant></RequireAuth>} />
                    <Route
                      path="/tenant-members"
                      element={
                        <RequireAuth>
                          <RequireTenant>
                            <TenantMembers />
                          </RequireTenant>
                        </RequireAuth>
                      }
                    />
                    <Route
                      path="/departments"
                      element={
                        <RequireAuth>
                          <RequireTenant>
                            <TenantDepartments />
                          </RequireTenant>
                        </RequireAuth>
                      }
                    />
                    <Route
                      path="/role-permissions"
                      element={
                        <RequireAuth>
                          <RequireTenant>
                            <TenantRolePermissions />
                          </RequireTenant>
                        </RequireAuth>
                      }
                    />
                    <Route path="/" element={<RequireAuth><HomeRedirect /></RequireAuth>} />
                    <Route path="*" element={<RequireAuth><HomeRedirect /></RequireAuth>} />
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
