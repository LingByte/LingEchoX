import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type AppLocale = 'zh-CN' | 'en-US'

interface LocaleState {
  locale: AppLocale
  setLocale: (locale: AppLocale) => void
  toggleLocale: () => void
}

export const useLocaleStore = create<LocaleState>()(
  persist(
    (set, get) => ({
      locale: 'zh-CN',
      setLocale: (locale) => set({ locale }),
      toggleLocale: () => set({ locale: get().locale === 'zh-CN' ? 'en-US' : 'zh-CN' }),
    }),
    { name: 'locale-storage' }
  )
)
