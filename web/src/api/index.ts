import { del, get, patch, post, postSSE, put, request, requestBlob, type SSEHandlers } from './http'
import type {
  AnalyticsData,
  APICredential,
  Audit,
  AuditCategory,
  AuditLog,
  AuditAssistantResult,
  AuditAssistantMessage,
  AuditFieldAssistantResult,
  CaptchaChallenge,
  DashboardData,
  InAppMessagePage,
  IntegrationGuide,
  LoginResult,
  MFAChallenge,
  MFASetup,
  MFAStatus,
  MonitorData,
  NotificationChannel,
  NotificationPreference,
  Paged,
  Project,
  Provider,
  ProviderModel,
  ProviderUsageLog,
  ProviderUsageSummary,
  ProjectSecret,
  RecycleItem,
  RedisConnection,
  ScriptExecution,
  ScriptSimulationResult,
  ReviewSummary,
  RoleDefinition,
  RuleTemplate,
  SystemConfigData,
  UpdateCheck,
  Task,
  TaskDetail,
  TriggerSource,
  TriggerUpdate,
  User,
} from './types'

export const authApi = {
  captcha: () => get<CaptchaChallenge>('/auth/captcha'),
  login: (payload: { username: string; password: string; captcha_id: string; captcha_x: number }) =>
    post<LoginResult | MFAChallenge>('/auth/login', payload),
  verifyMFA: (ticket: string, code: string) => post<LoginResult>('/auth/mfa', { ticket, code }),
  mfaStatus: () => get<MFAStatus>('/auth/mfa'),
  mfaSetup: () => post<MFASetup>('/auth/mfa/setup'),
  mfaEnable: (code: string) => post<{ enabled: boolean }>('/auth/mfa/enable', { code }),
  mfaDisable: (password: string, code: string) => post<{ enabled: boolean }>('/auth/mfa/disable', { password, code }),
  refresh: (refreshToken: string) => post<LoginResult>('/auth/refresh', { refresh_token: refreshToken }),
  me: () => get<{ user: User }>('/auth/me'),
  logout: () => post<{ logged_out: boolean }>('/auth/logout'),
  changePassword: (payload: { old_password: string; new_password: string }) =>
    post<{ changed: boolean }>('/auth/change-password', payload),
  updateProfile: (email: string) => patch<{ user: User }>('/auth/profile', { email }),
}

export const userApi = {
  list: () => get<{ items: User[]; total: number; roles: RoleDefinition[] }>('/users'),
  create: (payload: Record<string, unknown>) => post<User>('/users', payload),
  update: (id: number, payload: Record<string, unknown>) => put<User>(`/users/${id}`, payload),
  resetMfa: (id: number) => post<{ reset: boolean }>(`/users/${id}/reset-mfa`),
  roles: () => get<{ items: RoleDefinition[]; all_permissions: string[]; all_menus: string[] }>('/meta/roles'),
  createRole: (payload: Record<string, unknown>) => post<{ role: RoleDefinition }>('/roles', payload),
  updateRole: (code: string, payload: Record<string, unknown>) => put<{ updated: boolean }>(`/roles/${code}`, payload),
  deleteRole: (code: string) => del<{ deleted: boolean }>(`/roles/${code}`),
}

export const projectApi = {
  list: (params?: Record<string, unknown>) => get<Paged<Project>>('/projects', params),
  get: (id: number) => get<Project>(`/projects/${id}`),
  create: (payload: Partial<Project>) => post<Project>('/projects', payload),
  update: (id: number, payload: Partial<Project>) => put<Project>(`/projects/${id}`, payload),
  setStatus: (id: number, status: string, reason?: string) => patch<Project>(`/projects/${id}/status`, { status, reason }),
  remove: (id: number) => del<{ deleted: boolean }>(`/projects/${id}`),
  restore: (id: number) => post<unknown>(`/projects/${id}/restore`),
  secrets: (id: number) => get<{ items: ProjectSecret[]; total: number }>(`/projects/${id}/secrets`),
  setSecret: (id: number, name: string, value: string) =>
    put<{ name: string; updated: boolean }>(`/projects/${id}/secrets/${encodeURIComponent(name)}`, { value }),
  deleteSecret: (id: number, name: string) =>
    del<{ deleted: boolean }>(`/projects/${id}/secrets/${encodeURIComponent(name)}`),
}

export const credentialApi = {
  list: (projectId: number) => get<{ items: APICredential[]; total: number }>(`/projects/${projectId}/credentials`),
  create: (projectId: number, payload: { name: string; audit_ids: number[]; status?: string }) =>
    post<{ credential: APICredential; app_secret: string; warning: string }>(`/projects/${projectId}/credentials`, payload),
  update: (projectId: number, credentialId: number, payload: { name?: string; audit_ids?: number[]; status?: string }) =>
    put<APICredential>(`/projects/${projectId}/credentials/${credentialId}`, payload),
  rotate: (projectId: number, credentialId: number) =>
    post<{ id: number; app_key: string; app_secret: string; warning: string }>(`/projects/${projectId}/credentials/${credentialId}/rotate`),
  remove: (projectId: number, credentialId: number) =>
    del<{ deleted: boolean }>(`/projects/${projectId}/credentials/${credentialId}`),
}

export const providerApi = {
  list: (params?: Record<string, unknown>) => get<{ items: Provider[]; total: number }>('/providers', params),
  get: (id: number) => get<Provider>(`/providers/${id}`),
  usage: (id: number, params?: Record<string, unknown>) =>
    get<{ items: ProviderUsageLog[]; total: number; summary: ProviderUsageSummary; kind: string; model: string }>(
      `/providers/${id}/usage`,
      params,
    ),
  create: (payload: Record<string, unknown>) => post<Provider>('/providers', payload),
  update: (id: number, payload: Record<string, unknown>) => put<Provider>(`/providers/${id}`, payload),
  remove: (id: number) => del<{ deleted: boolean }>(`/providers/${id}`),
  test: (id: number, signal?: AbortSignal) =>
    request<{ ok: boolean; latency_ms: number; message: string; model: string }>({
      method: 'POST',
      url: `/providers/${id}/test`,
      signal,
    }),
  models: (payload: {
    project_id: number
    provider_id?: number
    kind: string
    base_url: string
    api_key?: string
    headers?: Record<string, string>
  }) => post<{ items: ProviderModel[]; total: number }>('/providers/models', payload),
}

export const redisConnectionApi = {
  list: (projectId: number) =>
    get<{ items: RedisConnection[]; total: number }>(`/projects/${projectId}/redis-connections`),
  create: (projectId: number, payload: Record<string, unknown>) =>
    post<RedisConnection>(`/projects/${projectId}/redis-connections`, payload),
  update: (id: number, payload: Record<string, unknown>) =>
    put<RedisConnection>(`/redis-connections/${id}`, payload),
  remove: (id: number) => del<{ deleted: boolean }>(`/redis-connections/${id}`),
  test: (id: number) =>
    post<{ ok: boolean; latency_ms: number; health: string; message: string }>(`/redis-connections/${id}/test`),
}

export const notificationApi = {
  list: (projectId: number) =>
    get<{ items: NotificationChannel[]; total: number }>(`/projects/${projectId}/notification-channels`),
  create: (projectId: number, payload: Record<string, unknown>) =>
    post<NotificationChannel>(`/projects/${projectId}/notification-channels`, payload),
  update: (id: number, payload: Record<string, unknown>) =>
    put<NotificationChannel>(`/notification-channels/${id}`, payload),
  remove: (id: number) => del<{ deleted: boolean }>(`/notification-channels/${id}`),
  setStatus: (id: number, status: string) =>
    patch<NotificationChannel>(`/notification-channels/${id}/status`, { status }),
  test: (id: number, to: string) =>
    post<{ ok: boolean; to: string; message: string }>(`/notification-channels/${id}/test`, { to }),
}

export const systemConfigApi = {
  get: () => get<SystemConfigData>('/system/config'),
}

export const updateApi = {
  check: (refresh = false) =>
    get<UpdateCheck>('/system/update', refresh ? { refresh: 1 } : undefined),
  setAutoCheck: (enabled: boolean) =>
    put<{ enabled: boolean }>('/system/update/auto-check', { enabled }),
}

export const notificationPreferenceApi = {
  get: (projectId: number) =>
    get<NotificationPreference>('/notification-preferences', projectId ? { project_id: projectId } : undefined),
  update: (payload: { email_notify_enabled: boolean; project_id: number }) =>
    put<NotificationPreference>('/notification-preferences', payload),
}

export const messageApi = {
  list: (params?: Record<string, unknown>) => get<InAppMessagePage>('/messages', params),
  markRead: (id: number) => post<{ read: boolean; unread_count: number }>(`/messages/${id}/read`),
  markAllRead: () => post<{ read: boolean; unread_count: number }>('/messages/read-all'),
}

export const auditApi = {
  list: (params?: Record<string, unknown>) => get<Paged<Audit>>('/audits', params),
  get: (id: number) => get<{ audit: Audit; triggers: TriggerSource[] }>(`/audits/${id}`),
  create: (payload: Record<string, unknown>) => post<Audit>('/audits', payload),
  assistant: (payload: Record<string, unknown>) => post<AuditAssistantResult>('/audits/assistant', payload),
  assistantStream: (payload: Record<string, unknown>, handlers: SSEHandlers) =>
    postSSE('/audits/assistant/stream', payload, handlers),
  assistantField: (payload: Record<string, unknown>) =>
    post<AuditFieldAssistantResult>('/audits/assistant/field', payload),
  update: (id: number, payload: Record<string, unknown>) => put<Audit>(`/audits/${id}`, payload),
  setStatus: (id: number, status: string) => patch<{ status: string }>(`/audits/${id}/status`, { status }),
  clone: (id: number, payload: { name: string; code: string }) => post<Audit>(`/audits/${id}/clone`, payload),
  remove: (id: number) => del<{ deleted: boolean }>(`/audits/${id}`),
  restore: (id: number) => post<unknown>(`/audits/${id}/restore`),
  integration: (id: number, credentialId?: number | null) =>
    get<IntegrationGuide>(`/audits/${id}/integration`, credentialId ? { credential_id: credentialId } : undefined),
  exportTasks: (id: number, params?: Record<string, unknown>) =>
    requestBlob({ method: 'GET', url: `/audits/${id}/export`, params }),
  updateTrigger: (id: number, payload: TriggerUpdate) => put<TriggerSource>(`/trigger-sources/${id}`, payload),
  testTrigger: (id: number) => post<Record<string, unknown>>(`/trigger-sources/${id}/test`),
  scriptExecutions: (id: number, params?: Record<string, unknown>) =>
    get<Paged<ScriptExecution>>(`/audits/${id}/script-executions`, params),
  scriptExecution: (id: number) => get<ScriptExecution>(`/script-executions/${id}`),
  clearScriptExecutions: (id: number) => del<unknown>(`/audits/${id}/script-executions`),
}

export const scriptApi = {
  assistantStream: (payload: Record<string, unknown>, handlers: SSEHandlers) =>
    postSSE('/scripts/assistant/stream', payload, handlers),
  simulate: (payload: Record<string, unknown>) => post<ScriptSimulationResult>('/scripts/simulate', payload),
}

export const taskApi = {
  list: (params?: Record<string, unknown>) => get<Paged<Task>>('/tasks', params),
  get: (id: number) => get<TaskDetail>(`/tasks/${id}`),
  submit: (payload: Record<string, unknown>) =>
    post<{ task: Task; duplicated: boolean }>('/tasks', payload),
  retry: (id: number) => post<{ task_id: number; status: string }>(`/tasks/${id}/retry`),
  batchRetry: (taskIds: number[]) =>
    post<{ success: number; failed: Array<{ id: number; reason: string }> }>('/tasks/batch-retry', { task_ids: taskIds }),
  review: (id: number, payload: { conclusion: string; reason_code?: string; remark?: string }) =>
    post<{ task_id: number; status: string; result: string }>(`/tasks/${id}/review`, payload),
}

export const dashboardApi = {
  get: (projectId?: number | null) => get<DashboardData>('/dashboard', projectId ? { project_id: projectId } : undefined),
}

export const logApi = {
  list: (params?: Record<string, unknown>) => get<Paged<AuditLog>>('/audit-logs', params),
}

export const monitorApi = {
  get: (range: string) => get<MonitorData>('/system/monitor', { range }),
}

export const analyticsApi = {
  get: (params?: Record<string, unknown>) => get<AnalyticsData>('/analytics', params),
}

export const reviewApi = {
  summary: (projectId?: number | null) =>
    get<ReviewSummary>('/reviews/summary', projectId ? { project_id: projectId } : undefined),
}

export interface RecycleBatchResult {
  type: string
  id: number
  name?: string
  status: string
  reason?: string
}

export const recycleApi = {
  list: (params?: Record<string, unknown>) => get<Paged<RecycleItem>>('/recycle-bin', params),
  purge: (type: string, id: number) => del<{ purged: boolean }>(`/recycle-bin/${type}/${id}`),
  batchRestore: (payload: {
    items: { type: string; id: number }[]
    captcha_id: string
    captcha_x: number
  }) =>
    post<{ restored: number; total: number; results: RecycleBatchResult[] }>(
      '/recycle-bin/batch-restore',
      payload,
    ),
  batchPurge: (payload: {
    items: { type: string; id: number }[]
    captcha_id: string
    captcha_x: number
  }) =>
    post<{ purged: number; total: number; results: RecycleBatchResult[] }>(
      '/recycle-bin/batch-purge',
      payload,
    ),
  purgeAll: (payload: { captcha_id: string; captcha_x: number }) =>
    post<{ purged_projects: number; purged_audits: number; failed: number }>(
      '/recycle-bin/purge-all',
      payload,
    ),
}

export const metaApi = {
  ruleTemplates: () =>
    get<{
      templates: RuleTemplate[]
      default_review_prompt: string
      default_classify_prompt?: string
      example_categories?: AuditCategory[]
    }>('/meta/rule-templates'),
  system: () =>
    get<{
      name: string
      name_en: string
      version: string
      redis_db: number
      worker_count: number
      provider_kinds: Array<{ key: string; name: string; supports_images: string }>
    }>('/meta/system'),
  roles: () => get<{ items: RoleDefinition[] }>('/meta/roles'),
}
