import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { fetchMe, logoutApi } from '@/api/me'

// 用户类型定义（可以根据实际需求修改）
export interface User {
  id: string | number
  username?: string
  email?: string
  avatar?: string
  [key: string]: any
}

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  isLoading: boolean
  token: string | null
  login: (token: string, user?: User) => Promise<boolean>
  logout: () => Promise<void>
  setLoading: (loading: boolean) => void
  updateProfile: (data: Partial<User>) => void
  clearUser: () => void
  refreshUserInfo: () => Promise<void>
}

// @ts-ignore
export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      token: null,

      login: async (token: string, user?: User) => {
        set({ isLoading: true })
        try {
          // 存储token
          localStorage.setItem('auth_token', token)
          
          // 存储用户信息
          if (user) {
            localStorage.setItem('auth_user', JSON.stringify(user))
          }
          
          set({
            isAuthenticated: true, 
            isLoading: false,
            token: token,
            user: user || null
          })
          
          return true
        } catch (error) {
          set({ isLoading: false })
          console.error('Login failed:', error)
          return false
        }
      },

      logout: async () => {
        try {
          await logoutApi()
        } catch (error) {
          console.error('Logout API error:', error)
        } finally {
          localStorage.removeItem('auth_token')
          localStorage.removeItem('auth_user')
          set({ user: null, isAuthenticated: false, token: null })
        }
      },

      setLoading: (loading: boolean) => {
        set({ isLoading: loading })
      },

      updateProfile: (data: Partial<User>) => {
        const { user } = get()
        if (user) {
          set({ user: { ...user, ...data } })
        }
      },

      // 清除用户信息方法
      clearUser: () => {
        localStorage.removeItem('auth_token')
        localStorage.removeItem('auth_user')
        set({ user: null, isAuthenticated: false, token: null })
      },

      // 从 token 恢复，并尽量使用 /me 刷新当前用户信息
      refreshUserInfo: async () => {
        const token = localStorage.getItem('auth_token')
        if (!token) return
        set({ isAuthenticated: true, token })
        try {
          const res = await fetchMe()
          if (res.code === 200 && res.data) {
            const data = res.data
            if (data.principal === 'platform' && data.platformAdmin) {
              const a = data.platformAdmin
              const user = {
                id: a.id,
                email: a.email,
                displayName: a.displayName,
                isPlatformAdmin: true,
                principal: 'platform' as const,
              }
              localStorage.setItem('auth_user', JSON.stringify(user))
              set({ user, isAuthenticated: true, token })
              return
            }
            if (data.principal === 'tenant' && data.user) {
              const user = {
                ...data.user,
                tenantSlug: data.tenant?.slug,
                tenantName: data.tenant?.name,
                principal: 'tenant' as const,
              }
              localStorage.setItem('auth_user', JSON.stringify(user))
              set({ user, isAuthenticated: true, token })
              return
            }
          }
        } catch {
          // fallthrough to local cache
        }
        const storedUser = localStorage.getItem('auth_user')
        if (storedUser) {
          try {
            set({ user: JSON.parse(storedUser), isAuthenticated: true, token })
            return
          } catch {}
        }
        set({ user: null, isAuthenticated: false, token: null })
        localStorage.removeItem('auth_token')
      }
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({ 
        user: state.user, 
        isAuthenticated: state.isAuthenticated,
        token: state.token
      }),
    }
  )
)
