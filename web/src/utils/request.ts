import axiosInstance from '@/utils/axios'
import { t } from '@/i18n'
import { InternalAxiosRequestConfig, AxiosResponse } from 'axios'

// 通用响应类型
export interface ApiResponse<T = any> {
  code: number
  msg: string
  data: T
}

// 请求函数 - 返回完整的响应结构
const request = async <T = any>(
  url: string,
  options: Partial<InternalAxiosRequestConfig> = {}
): Promise<ApiResponse<T>> => {
  try {
    const response: AxiosResponse<ApiResponse<T>> = await axiosInstance({
      url,
      ...options,
    })
    
    // 返回完整的响应结构，让业务层处理
    return response.data
  } catch (error: any) {
    // 如果是axios错误，尝试从响应中获取错误信息
    if (error.response?.data) {
      const errorData = error.response.data
      // 处理不同的错误格式
      if (errorData.error) {
        // 格式: {"error": "email has exists"}
        throw {
          code: error.response.status || 500,
          msg: errorData.error,
          data: null
        }
      } else if (errorData.code !== undefined) {
        // 格式: {code, msg, data}
        throw errorData
      } else {
        // 其他格式，尝试提取错误信息
        throw {
          code: error.response.status || 500,
          msg: errorData.message || errorData.msg || errorData.error || t('request.failed'),
          data: null
        }
      }
    }
    
    // 网络错误处理
    let errorMessage = t('request.networkFailed')
    if (error.code === 'ERR_CONNECTION_REFUSED') {
      errorMessage = t('request.connectionRefused')
    } else if (error.code === 'ECONNABORTED') {
      errorMessage = t('request.timeout')
    } else if (error.message) {
      errorMessage = error.message
    }
    
    throw {
      code: -1,
      msg: errorMessage,
      data: null
    }
  }
}

// GET 请求
export const get = <T = any>(url: string, config?: Partial<InternalAxiosRequestConfig>): Promise<ApiResponse<T>> => {
  return request<T>(url, { ...config, method: 'GET' })
}

// POST 请求
export const post = <T = any>(url: string, data?: any, config?: Partial<InternalAxiosRequestConfig>): Promise<ApiResponse<T>> => {
  return request<T>(url, {
    ...config,
    method: 'POST',
    data,
  })
}

// PUT 请求
export const put = <T = any>(url: string, data?: any, config?: Partial<InternalAxiosRequestConfig>): Promise<ApiResponse<T>> => {
  return request<T>(url, {
    ...config,
    method: 'PUT',
    data,
  })
}

// DELETE 请求
export const del = <T = any>(url: string, config?: Partial<InternalAxiosRequestConfig>): Promise<ApiResponse<T>> => {
  return request<T>(url, { ...config, method: 'DELETE' })
}

// PATCH 请求
export const patch = <T = any>(url: string, data?: any, config?: Partial<InternalAxiosRequestConfig>): Promise<ApiResponse<T>> => {
  return request<T>(url, {
    ...config,
    method: 'PATCH',
    data,
  })
}

// 导出 request 对象和类型
export { request }
export default request
