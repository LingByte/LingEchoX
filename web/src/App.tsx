import { lazy, Suspense } from 'react'
import { Spin } from '@arco-design/web-react'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import ErrorBoundary from '@/components/ErrorBoundary/ErrorBoundary'
import PWAInstaller from '@/components/PWA/PWAInstaller'
import DevErrorHandler from '@/components/Dev/DevErrorHandler'
import { SidebarProvider } from '@/contexts/SidebarContext'
import { SiteConfigProvider } from '@/contexts/SiteConfigContext'
import { WebSeatProvider } from '@/components/WebSeat/WebSeatProvider'

const SIPUsers = lazy(() => import('@/pages/SIPUsers'))
const CallRecords = lazy(() => import('@/pages/CallRecords'))
const NumberPool = lazy(() => import('@/pages/NumberPool'))
const OutboundTasks = lazy(() => import('@/pages/OutboundTasks'))
const ScriptManager = lazy(() => import('@/pages/ScriptManager'))
const ScriptManagerNew = lazy(() => import('@/pages/ScriptManagerNew'))
const WebAgents = lazy(() => import('@/pages/WebAgents'))
const SIPTrunks = lazy(() => import('@/pages/SIPTrunks'))
const SIPTrunkNumbers = lazy(() => import('@/pages/SIPTrunkNumbers'))

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
                    <Route path="/sip-users" element={<SIPUsers />} />
                    <Route path="/call-records" element={<CallRecords />} />
                    <Route path="/number-pool" element={<NumberPool />} />
                    <Route path="/outbound-tasks" element={<OutboundTasks />} />
                    <Route path="/script-manager/new" element={<ScriptManagerNew />} />
                    <Route path="/script-manager" element={<ScriptManager />} />
                    <Route path="/web-agents" element={<WebAgents />} />
                    <Route path="/sip-trunks" element={<SIPTrunks />} />
                    <Route path="/sip-trunk-numbers" element={<SIPTrunkNumbers />} />
                    <Route path="/" element={<Navigate to="/sip-users" replace />} />
                    <Route path="*" element={<Navigate to="/sip-users" replace />} />
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
