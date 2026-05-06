import { create } from 'zustand'
import { persist } from 'zustand/middleware'

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
          // 可以在这里调用登出API
          // const response = await logoutUser()
        } catch (error) {
          console.error('Logout API error:', error)
        } finally {
          // 清除本地存储
          localStorage.removeItem('auth_token')
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
        set({ user: null, isAuthenticated: false, token: null })
      },

      // 从本地恢复用户信息（SIP 控制台可不依赖独立账号接口）
      refreshUserInfo: async () => {
        const token = localStorage.getItem('auth_token')
        if (!token) return
        const storedUser = localStorage.getItem('auth_user')
        if (storedUser) {
          try {
            set({
              user: JSON.parse(storedUser),
              isAuthenticated: true,
              token,
            })
            return
          } catch {
            /* fallthrough */
          }
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
