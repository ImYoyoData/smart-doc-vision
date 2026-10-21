import axios, { AxiosError, type AxiosRequestConfig } from 'axios'

export const TOKEN_KEY = 'sdv.access_token'
export const REFRESH_KEY = 'sdv.refresh_token'
export const USER_KEY = 'sdv.user'

interface ApiEnvelope<T> {
  code: number | string
  message: string
  data: T
  trace_id?: string
  cost_ms?: number
}

export class ApiError extends Error {
  code: string
  status?: number
  traceId?: string
  data?: unknown

  constructor(message: string, code = 'ERROR', status?: number, traceId?: string, data?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.traceId = traceId
    this.data = data
  }
}

const http = axios.create({
  baseURL: '/api/admin/v1',
  timeout: 180_000,
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (response) => {
    const body = response.data as ApiEnvelope<unknown>
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      return Promise.reject(new ApiError(body.message || '请求失败', String(body.code), response.status, body.trace_id, body.data))
    }
    return response
  },
  (error: AxiosError<ApiEnvelope<unknown>>) => {
    const status = error.response?.status
    const body = error.response?.data
    if (status === 401) {
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(REFRESH_KEY)
      localStorage.removeItem(USER_KEY)
      const base = import.meta.env.BASE_URL
      const loginPath = `${base}login`
      const currentPath = window.location.pathname
      if (!currentPath.startsWith(loginPath)) {
        const relativePath = base !== '/' && currentPath.startsWith(base)
          ? `/${currentPath.slice(base.length)}`
          : currentPath
        const redirect = encodeURIComponent(relativePath + window.location.search)
        window.location.href = `${loginPath}?redirect=${redirect}`
      }
    }
    const message =
      (body && typeof body === 'object' && body.message) ||
      (status ? `请求失败（HTTP ${status}）` : error.message === 'Network Error' ? '无法连接后端服务' : error.message)
    return Promise.reject(new ApiError(message, String(body?.code || 'HTTP_ERROR'), status, body?.trace_id, body?.data))
  },
)

export async function request<T>(config: AxiosRequestConfig): Promise<T> {
  const response = await http.request<ApiEnvelope<T>>(config)
  return response.data.data
}

export async function requestBlob(config: AxiosRequestConfig): Promise<Blob> {
  const response = await http.request<Blob>({ ...config, responseType: 'blob' })
  return response.data
}

export const get = <T>(url: string, params?: Record<string, unknown>) => request<T>({ method: 'GET', url, params })
export const post = <T>(url: string, data?: unknown) => request<T>({ method: 'POST', url, data })
export const put = <T>(url: string, data?: unknown) => request<T>({ method: 'PUT', url, data })
export const patch = <T>(url: string, data?: unknown) => request<T>({ method: 'PATCH', url, data })
export const del = <T>(url: string, params?: Record<string, unknown>) => request<T>({ method: 'DELETE', url, params })
