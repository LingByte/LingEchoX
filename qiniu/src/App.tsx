import { lazy, Suspense } from 'react'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import ErrorBoundary from '@/components/ErrorBoundary/ErrorBoundary'
import PWAInstaller from '@/components/PWA/PWAInstaller'
import NotificationContainer from '@/components/UI/NotificationContainer'
import GlobalSearch from '@/components/UI/GlobalSearch'
import DevErrorHandler from '@/components/Dev/DevErrorHandler'
import { SidebarProvider } from '@/contexts/SidebarContext'
import { SiteConfigProvider } from '@/contexts/SiteConfigContext'
import { WebSeatProvider } from '@/components/WebSeat/WebSeatProvider'

const SIPUsers = lazy(() => import('@/pages/SIPUsers'))
const CallRecords = lazy(() => import('@/pages/CallRecords'))
const NumberPool = lazy(() => import('@/pages/NumberPool'))
const OutboundTasks = lazy(() => import('@/pages/OutboundTasks'))
const ScriptManager = lazy(() => import('@/pages/ScriptManager'))
const WebAgents = lazy(() => import('@/pages/WebAgents'))

function App() {
  return (
    <ErrorBoundary>
      <SiteConfigProvider>
        <SidebarProvider>
          <Router>
            <WebSeatProvider>
              <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
                <Suspense fallback={<div className="p-8 text-center text-slate-500">页面加载中...</div>}>
                  <Routes>
                    <Route path="/sip-users" element={<SIPUsers />} />
                    <Route path="/call-records" element={<CallRecords />} />
                    <Route path="/number-pool" element={<NumberPool />} />
                    <Route path="/outbound-tasks" element={<OutboundTasks />} />
                    <Route path="/script-manager" element={<ScriptManager />} />
                    <Route path="/web-agents" element={<WebAgents />} />
                    <Route path="/" element={<Navigate to="/sip-users" replace />} />
                    <Route path="*" element={<Navigate to="/sip-users" replace />} />
                  </Routes>
                </Suspense>
                <PWAInstaller showOnLoad={true} delay={5000} position="bottom-right" />
                <NotificationContainer />
                <DevErrorHandler />
                <GlobalSearch />
              </div>
            </WebSeatProvider>
          </Router>
      </SidebarProvider>
      </SiteConfigProvider>
    </ErrorBoundary>
  )
}

export default App
