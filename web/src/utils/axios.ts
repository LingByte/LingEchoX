import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse } from 'axios'
import { useAuthStore } from '../stores/authStore'
import { getApiBaseURL } from '../config/apiConfig'
import { genReqId, X_REQ_ID_HEADER } from '@/utils/reqId'

// 创建axios实例
const axiosInstance: AxiosInstance = axios.create({
  // 统一走配置的后端地址；当 url 为绝对地址时 axios 会优先使用 url 本身
  baseURL: getApiBaseURL(),
  timeout: 1000000,
  headers: {
    'Content-Type': 'application/json',
  },
  withCredentials: true, // 重要：允许发送和接收 cookies（session）
})

// 请求拦截器
axiosInstance.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    // 添加认证token
    const token = localStorage.getItem('auth_token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    } else {
      delete config.headers.Authorization
    }
    
    // 如果是FormData，让浏览器自动设置Content-Type（包含boundary）
    if (config.data instanceof FormData) {
      delete config.headers['Content-Type']
    }

    // 与后端 RequestIDMiddleware 对齐，便于日志串联（未显式传入时自动生成）
    const headers = config.headers as Record<string, string | undefined>
    if (!headers[X_REQ_ID_HEADER] && !headers['X-Req-ID']) {
      headers[X_REQ_ID_HEADER] = genReqId()
    }
    
    // 添加请求时间戳
    if (config.params) {
      config.params._t = Date.now()
    } else {
      config.params = { _t: Date.now() }
    }
    
    // 添加调试信息
    // @ts-ignore
    console.log('Making request to:', config.baseURL + config.url, {
      method: config.method,
      'x-reqid': headers[X_REQ_ID_HEADER] ?? headers['X-Req-ID'],
      params: config.params,
    })
    
    return config
  },
  (error) => {
    console.error('Request interceptor error:', error)
    return Promise.reject(error)
  }
)

// 响应拦截器 - 只处理通用错误，不处理业务逻辑
axiosInstance.interceptors.response.use(
  (response: AxiosResponse) => {
    const rid =
      response.headers[X_REQ_ID_HEADER] ??
      response.headers['x-reqid'] ??
      response.headers['X-Req-ID']
    if (rid && import.meta.env.DEV) {
      console.debug('[x-reqid]', rid, response.config.method, response.config.url)
    }
    return response
  },
  (error) => {
      console.error('Response interceptor error:', error)
    // 处理网络错误和HTTP状态码错误
    if (error.response) {
        console.log('Response status:', error.response.status)
      // 服务器返回了错误状态码
      const status = error.response.status

      switch (status) {
        case 401:
            console.log('Unauthorized')
            useAuthStore.getState().clearUser()
            // window.location.href = '/'
            console.log('Unauthorized: Please log in')
          break
        case 403:
          console.error('Forbidden: Access denied')
          break
        case 404:
          console.error('Not Found: API endpoint not found')
          break
        case 500:
          console.error('Internal Server Error')
          break
        default:
          console.error(`HTTP Error ${status}:`, error.response.data)
      }
    } else if (error.request) {
      // 网络错误 - 连接被拒绝或超时
      console.error('Network Error:', error.message)
    } else {
      // 其他错误
      console.error('Error:', error.message)
    }
    
    return Promise.reject(error)
  }
)

export default axiosInstance
