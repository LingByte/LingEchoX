import { useEffect } from 'react'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import 'dayjs/locale/en'
import { useLocaleStore } from '@/stores/localeStore'
import { syncDocumentLanguage } from './index'

/** Keeps document.lang and dayjs locale in sync with the locale store. */
export default function I18nSync() {
  const locale = useLocaleStore((s) => s.locale)

  useEffect(() => {
    syncDocumentLanguage(locale)
    dayjs.locale(locale === 'en-US' ? 'en' : 'zh-cn')
  }, [locale])

  return null
}
