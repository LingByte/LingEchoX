export function webSeatHttpBase(): string {
  const v = import.meta.env.VITE_SIP_WEBSEAT_HTTP_BASE
  const raw = typeof v === 'string' ? v.trim().replace(/\/$/, '') : ''
  if (!raw) return ''
  return normalizeLoopbackBase(normalizeBaseURL(raw))
}

export function webSeatWsToken(): string {
  const v = import.meta.env.VITE_SIP_WEBSEAT_WS_TOKEN
  return typeof v === 'string' ? v.trim() : ''
}

export function webSeatWsBase(): string {
  const v = import.meta.env.VITE_SIP_WEBSEAT_WS_BASE
  const raw = typeof v === 'string' ? v.trim().replace(/\/$/, '') : ''
  if (!raw) return ''
  return normalizeLoopbackBase(normalizeBaseURL(raw))
}

export function buildWebSeatWebSocketURL(httpBase: string, token: string, wsBase?: string): string {
  const pickedBase = (wsBase || '').trim() || httpBase
  const root = normalizeBaseURL(pickedBase).replace(/\/$/, '')
  const base = root.endsWith('/') ? root : `${root}/`
  let u: URL
  try {
    u = new URL('webseat/v1/ws', base)
  } catch {
    const origin = typeof window !== 'undefined' && window.location?.origin ? window.location.origin : 'http://localhost'
    u = new URL('webseat/v1/ws', `${origin}/`)
  }
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
  if (token) u.searchParams.set('token', token)
  return u.toString()
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
