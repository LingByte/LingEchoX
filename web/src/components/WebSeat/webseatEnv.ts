import { buildWebSocketURL, getApiBaseURL, getApiMountPath } from '@/config/apiConfig'

/** Path under API prefix, matches backend constants.LingechoWebSeatPathPrefix */
export const lingechoWebSeatV1 = 'lingecho/webseat/v1'

/**
 * Absolute HTTP base for WebSeat REST (join/hangup/reject), same as main API (VITE_API_BASE_URL).
 * No separate VITE_SIP_WEBSEAT_HTTP_BASE — WebSeat is always on the main Gin server.
 */
export function webSeatHttpBase(): string {
  return normalizeLoopbackBase(normalizeBaseURL(getApiBaseURL()))
}

/** e.g. POST .../lingecho/webseat/v1/join */
export function webSeatV1URL(httpBase: string, segment: string): string {
  const base = (httpBase || '').replace(/\/$/, '')
  const s = segment.replace(/^\//, '')
  return `${base}/${lingechoWebSeatV1}/${s}`
}

/** Shared secret for GET .../lingecho/webseat/v1/ws?token= — mirrors server SIP_WEBSEAT_WS_TOKEN */
export function webSeatWsToken(): string {
  const v = import.meta.env.VITE_SIP_WEBSEAT_WS_TOKEN
  return typeof v === 'string' ? v.trim() : ''
}

/** WebSocket URL for agent signaling; uses VITE_API_BASE_URL + VITE_WS_BASE_URL like other app WS. */
export function buildWebSeatWebSocketURL(token: string): string {
  const path = webSeatWsPathUnderApi()
  let ws = buildWebSocketURL(path)
  if (token) {
    const u = new URL(ws)
    u.searchParams.set('token', token)
    ws = u.toString()
  }
  return ws
}

/** Full path under API mount: /api/lingecho/webseat/v1/ws (mount from VITE_API_BASE_URL). */
function webSeatWsPathUnderApi(): string {
  const mount = getApiMountPath()
  return `${mount}/${lingechoWebSeatV1}/ws`.replace(/\/+/g, '/')
}

function normalizeBaseURL(base: string): string {
  const s = (base || '').trim()
  if (!s) return s
  if (s.startsWith('/')) {
    if (typeof window !== 'undefined' && window.location?.origin) return `${window.location.origin}${s}`.replace(/\/$/, '')
    return s
  }
  if (s.startsWith('//')) {
    if (typeof window !== 'undefined' && window.location?.protocol) return `${window.location.protocol}${s}`.replace(/\/$/, '')
    return `https:${s}`.replace(/\/$/, '')
  }
  if (/^https?:\/\//i.test(s)) return s
  if (/^wss?:\/\//i.test(s)) return s.replace(/^ws/i, 'http')
  return `http://${s}`
}

function normalizeLoopbackBase(base: string): string {
  if (typeof window === 'undefined' || !window.location?.hostname) return base
  let u: URL
  try {
    u = new URL(base)
  } catch {
    return base
  }
  const h = u.hostname.toLowerCase()
  const isLoopback = h === '127.0.0.1' || h === 'localhost' || h === '::1'
  if (!isLoopback) return base
  const pageHost = window.location.hostname
  if (!pageHost || pageHost === 'localhost' || pageHost === '127.0.0.1') return base
  u.hostname = pageHost
  return u.toString().replace(/\/$/, '')
}
