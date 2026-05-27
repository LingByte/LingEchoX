import { useCallback } from 'react'
import { useLocaleStore, type AppLocale } from '@/stores/localeStore'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'

type Dict = Record<string, unknown>

const catalogs: Record<AppLocale, Dict> = {
  'zh-CN': zhCN as Dict,
  'en-US': enUS as Dict,
}

function lookup(dict: Dict, key: string): string | undefined {
  const parts = key.split('.')
  let cur: unknown = dict
  for (const p of parts) {
    if (cur == null || typeof cur !== 'object') return undefined
    cur = (cur as Dict)[p]
  }
  return typeof cur === 'string' ? cur : undefined
}

export function translate(
  locale: AppLocale,
  key: string,
  params?: Record<string, string | number>,
): string {
  let msg = lookup(catalogs[locale], key) ?? lookup(catalogs['zh-CN'], key) ?? key
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      msg = msg.split(`{${k}}`).join(String(v))
    }
  }
  return msg
}

export function getLocale(): AppLocale {
  return useLocaleStore.getState().locale
}

/** Non-React helper (axios, utilities). */
export function t(key: string, params?: Record<string, string | number>): string {
  return translate(getLocale(), key, params)
}

export function useTranslation() {
  const locale = useLocaleStore((s) => s.locale)
  const tr = useCallback(
    (key: string, params?: Record<string, string | number>) => translate(locale, key, params),
    [locale],
  )
  return { t: tr, locale }
}

export function syncDocumentLanguage(locale: AppLocale) {
  if (typeof document === 'undefined') return
  document.documentElement.lang = locale === 'en-US' ? 'en' : 'zh-CN'
}
