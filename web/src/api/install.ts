import axios from 'axios'
import type { SchemaReport } from './schema'

export type InstallState = 'installed' | 'needs_install' | 'config_missing'
export type DatabaseMode = 'create' | 'repair' | 'reset'

export interface InstallMySQL {
  host: string
  port: number
  user: string
  password: string
  database: string
  charset: string
}

export interface InstallRedis {
  host: string
  port: number
  password: string
  db: number
}

export interface InstallS3 {
  provider: string
  endpoint: string
  region: string
  bucket: string
  access_key: string
  secret_key: string
  prefix: string
  force_path_style: boolean
}

export interface InstallStorage {
  driver: string
  local_dir: string
  public_base_url: string
  s3: InstallS3
}

export interface InstallAdmin {
  username: string
  password: string
}

export interface InstallSite {
  name: string
  name_en: string
  logo: string
}

export interface InstallRequest {
  agreement: boolean
  mysql: InstallMySQL
  redis: InstallRedis
  storage: InstallStorage
  admin: InstallAdmin
  site: InstallSite
  database_mode: DatabaseMode | ''
  confirm_reset: boolean
  confirm_repair: boolean
}

export interface InstallSystemInfo {
  version: string
  os: string
  arch: string
  working_dir: string
  env_file: string
  default_env_path: string
  docker: boolean
  local_storage_dir: string
  docker_mount_hint?: string
}

export interface InstallDefaults {
  mysql: InstallMySQL
  redis: InstallRedis
  storage: InstallStorage
  admin: InstallAdmin
  site: InstallSite
}

export interface InstallStatus {
  state: InstallState
  version: string
  installed_at?: string
  marker_path?: string
  env_file?: string
  configured: boolean
  system: InstallSystemInfo
  defaults: InstallDefaults
}

export interface InstallProbeResult {
  key: string
  label: string
  ok: boolean
  message: string
  note?: string
  latency_ms: number
}

export interface InstallTestResult {
  ok: boolean
  results: InstallProbeResult[]
  database?: SchemaReport | null
  database_error?: string
}

export interface InstallResult {
  installed: boolean
  installed_at: string
  version: string
  env_file: string
}

interface Envelope<T> {
  code: number | string
  message: string
  data: T
}

const client = axios.create({
  baseURL: '/api',
  timeout: 30_000,
  headers: { 'Content-Type': 'application/json' },
})

function describeError(error: unknown): Error {
  if (axios.isAxiosError(error)) {
    const body = error.response?.data as Envelope<unknown> | undefined
    if (body?.message) return new Error(body.message)
    if (error.code === 'ECONNABORTED') return new Error('请求超时，请检查服务是否正常运行')
    if (!error.response) return new Error('无法连接安装服务，请确认后端已启动')
    return new Error(`安装请求失败（HTTP ${error.response.status}）`)
  }
  return error instanceof Error ? error : new Error('安装请求失败')
}

async function request<T>(config: Parameters<typeof client.request>[0]): Promise<T> {
  try {
    const response = await client.request<Envelope<T>>(config)
    const body = response.data
    if (!body || body.code !== 0) {
      throw new Error(body?.message || '安装请求失败')
    }
    return body.data
  } catch (error) {
    throw describeError(error)
  }
}

export const installApi = {
  status: () => request<InstallStatus>({ method: 'GET', url: '/install/status', timeout: 5_000 }),
  test: (payload: InstallRequest) => request<InstallTestResult>({ method: 'POST', url: '/install/test', data: payload }),
  install: (payload: InstallRequest) =>
    request<InstallResult>({ method: 'POST', url: '/install', data: payload, timeout: 300_000 }),
}
