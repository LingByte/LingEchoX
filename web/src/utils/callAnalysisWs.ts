import { buildWebSocketURL, getApiMountPath } from '@/config/apiConfig'

/** WebSocket path under API mount: /api/sip-center/call-analysis/ws */
export function callAnalysisWebSocketURL(): string {
  const mount = getApiMountPath()
  const path = `${mount}/sip-center/call-analysis/ws`.replace(/\/+/g, '/')
  let url = buildWebSocketURL(path)
  const tok = typeof localStorage !== 'undefined' ? localStorage.getItem('auth_token') : ''
  if (tok) {
    const u = new URL(url)
    u.searchParams.set('auth_token', tok)
    url = u.toString()
  }
  return url
}
