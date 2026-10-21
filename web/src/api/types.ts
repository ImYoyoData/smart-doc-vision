export interface Paged<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  has_more: boolean
}

export interface User {
  id: number
  username: string
  display_name: string
  role: string
  status: string
  last_login_at?: string | null
  project_ids?: number[]
  permissions?: string[]
  menus?: string[]
}

export interface RoleDefinition {
  code: string
  name: string
  description: string
  permissions: string[]
  menus: string[]
}

export interface LoginResult {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export interface CaptchaChallenge {
  id: string
  background: string
  piece: string
  width: number
  height: number
  piece_size: number
  piece_y: number
  expires_in: number
}

export interface Project {
  id: number
  name: string
  code: string
  env: 'dev' | 'test' | 'staging' | 'prod' | string
  status: string
  description: string
  owner_id: number
  retention_days: number
  deleted_at?: string | null
  created_at: string
  updated_at: string
  audit_count: number
  provider_count: number
  today_tasks: number
}

export interface ProviderConfig {
  supports_vision?: boolean
  max_tokens?: number
  temperature?: number
  max_images?: number
}

export interface Provider {
  id: number
  project_id: number
  name: string
  kind: 'OPENAI_COMPATIBLE' | 'ANTHROPIC' | 'JEV' | string
  base_url: string
  model: string
  prompt: string
  config: ProviderConfig | null
  status: string
  last_test_at?: string | null
  last_test_ok: boolean
  last_test_ms: number
  last_test_message: string
  created_at: string
  updated_at: string
  api_key_masked: string
  has_api_key: boolean
}

export interface RuleItem {
  field: string
  operator: string
  value: unknown
}

export interface RuleGroup {
  name: string
  priority: number
  logic: string
  stop_on_hit: boolean
  on_hit_result: string
  items: RuleItem[]
}

export type AuditDecisionMode = 'REVIEW' | 'CLASSIFY'

export interface AuditCategory {
  code: string
  name: string
  description?: string
  result: string
}

export interface Audit {
  id: number
  project_id: number
  name: string
  code: string
  description: string
  media_types: string[]
  decision_mode: AuditDecisionMode | string
  categories: AuditCategory[] | null
  provider_id: number
  provider_name?: string
  provider_kind?: string
  review_prompt: string
  rules: RuleGroup[]
  routes: Record<string, unknown>
  default_result: string
  on_model_fail: string
  dedupe_window_sec: number
  callback_enabled: boolean
  callback_url: string
  callback_results: string[]
  callback_headers: Record<string, string>
  callback_include_content: boolean
  callback_secret_set: boolean
  status: string
  version: number
  today_tasks: number
  active_triggers?: string[]
  project_name?: string
  created_at: string
  updated_at: string
  deleted_at?: string | null
}

export interface TriggerSource {
  id: number
  project_id: number
  audit_id: number
  type: string
  name: string
  config: Record<string, any>
  priority: number
  status: string
  secret_set: boolean
  cursor: string
  last_consume_at?: string | null
  last_run_at?: string | null
  last_error: string
  fail_count: number
  created_at: string
  updated_at: string
}

export interface TriggerUpdate {
  name?: string
  config?: Record<string, any>
  secret?: string
  status?: string
  priority?: number
}

export interface CredentialScope {
  id: number
  name: string
  code: string
  status: string
  exists: boolean
}

export interface APICredential {
  id: number
  project_id: number
  name: string
  app_key: string
  app_secret_masked: string
  secret_set: boolean
  status: string
  audit_ids: number[]
  scopes: CredentialScope[]
  last_used_at?: string | null
  created_at: string
  updated_at: string
}

export interface CredentialPublic {
  id: number
  name: string
  app_key: string
  secret_masked: string
  secret_set: boolean
}

export interface IntegrationGuide {
  audit: {
    id: number
    name: string
    code: string
    status: string
    decision_mode?: string
    categories?: AuditCategory[] | null
  }
  project: { id: number; name: string; code: string }
  credentials: {
    eligible: CredentialPublic[]
    selected: CredentialPublic | null
    manage_path: string
    scope: string
  }
  input_contract: {
    media_types: string[]
    supports_multi_text: boolean
    supports_multi_image: boolean
    supports_video: boolean
    max_images: number
    note: string
  }
  docs_path: string
  openapi: {
    enabled: boolean
    base_url?: string
    path: string
    method: string
    signing_available: boolean
    headers: Record<string, string>
    canonical: string
    sample_body: Record<string, any>
    sample_body_raw: string
    sample_curl: string
    body_sha256: string
    signing_note: string
    query_task_path: string
  }
  redis: {
    enabled: boolean
    stream: string
    group: string
    batch_size: number
    message_field: string
    example: Record<string, any>
    xadd_command: string
    note: string
  }
  websocket: {
    enabled: boolean
    path: string
    signing_available: boolean
    signed_url: string
    query: string[]
    canonical: string
    submit_frame: Record<string, any>
    ack_frame: Record<string, any>
  }
  polling: {
    enabled: boolean
    config: Record<string, any>
    response_contract: Record<string, any>
    note: string
  }
  callback: {
    enabled: boolean
    url: string
    results: string[]
    include_content: boolean
    secret_set: boolean
    headers: Record<string, string>
    signature: string
    sample_payload: Record<string, any>
  }
  permissions: Record<string, string>
  triggers: TriggerSource[]
}

export interface MediaItem {
  type: 'text' | 'image' | string
  content?: string
  url?: string
  name?: string
}

export interface TaskInput {
  biz_key?: string
  media: MediaItem[]
  ext?: Record<string, unknown>
  callback_url?: string
}

export interface Verdict {
  result: string
  risk_level: string
  score: number
  labels: string[]
  summary: string
  reasons?: string[]
  category?: string
  category_name?: string
  category_confidence?: number
  raw?: Record<string, unknown>
  provider?: string
  model?: string
  latency_ms?: number
  meta?: Record<string, unknown>
}

export interface RuleTraceItem {
  field: string
  operator: string
  expected: unknown
  actual: unknown
  hit: boolean
}

export interface RuleTraceGroup {
  name: string
  priority: number
  logic: string
  hit: boolean
  skipped?: boolean
  on_hit_result: string
  items: RuleTraceItem[]
}

export interface TaskResultDetail {
  summary?: string
  reasons?: string[]
  labels?: string[]
  risk_level?: string
  score?: number
  provider?: string
  model?: string
  latency_ms?: number
  category?: string
  category_name?: string
  category_confidence?: number
  callback?: {
    ok?: boolean
    http_status?: number
    attempts?: number
    error?: string
    target_url?: string
    signed?: boolean
    response?: string
    result?: string
  } | null
  manual_review?: {
    reviewer?: string
    conclusion?: string
    reason_code?: string
    remark?: string
    reviewed_at?: string
  }
  model_error?: string
}

export interface Task {
  id: number
  task_no: string
  project_id: number
  audit_id: number
  source_type: string
  biz_key: string
  dedupe_hash?: string
  media_type: string
  status: string
  result: string
  input: TaskInput | null
  model_result: Verdict | null
  rule_trace: {
    final_result?: string
    matched_rule?: string
    trace?: RuleTraceGroup[]
  } | null
  result_detail: TaskResultDetail | null
  provider_id: number
  provider_name?: string
  audit_name?: string
  project_name?: string
  error_message: string
  retry_count: number
  duration_ms: number
  reviewer_id?: number
  reviewed_at?: string | null
  review_note?: string
  started_at?: string | null
  finished_at?: string | null
  created_at: string
  updated_at: string
}

export interface TaskEvent {
  id: number
  task_id: number
  project_id: number
  from_status: string
  to_status: string
  message: string
  cost_ms: number
  created_at: string
}

export interface TaskDetail {
  task: Task
  events: TaskEvent[]
}

export interface TrendPoint {
  date: string
  label: string
  total: number
  pass: number
  reject: number
  review: number
  failed: number
}

export interface DashboardData {
  overview: {
    today_tasks: number
    yesterday_tasks: number
    tasks_7d: number
    pass_rate: number
    review_pending: number
    failed_today: number
    avg_duration_ms: number
    active_audits: number
    active_projects: number
  }
  trend: TrendPoint[]
  distribution: Array<{ key: string; name: string; value: number }>
  recent_tasks: Task[]
  providers: Array<{
    id: number
    project_id: number
    name: string
    kind: string
    model: string
    status: string
    last_test_ok: boolean
    last_test_at?: string | null
    last_test_ms: number
  }>
  generated_at: string
}

export interface AuditLog {
  id: number
  project_id?: number | null
  operator_id?: number | null
  operator: string
  operator_ip: string
  action: string
  object_type: string
  object_id: string
  result: string
  detail: Record<string, unknown> | null
  trace_id: string
  created_at: string
}

export interface RecycleItem {
  type: string
  object_type: string
  id: number
  name: string
  code: string
  project_id: number
  project_name: string
  status: string
  deleted_at: string
  expire_at: string
  remaining_days: number
  task_count: number
  can_restore: boolean
}

export interface RuleTemplate {
  key: string
  name: string
  description: string
  kind?: AuditDecisionMode | string
}


export interface AnalyticsTrendPoint {
  date: string
  label: string
  total: number
  pass: number
  reject: number
  review: number
  failed: number
}

export interface AuditBreakdown {
  audit_id: number
  audit_name: string
  audit_code: string
  project_id: number
  total: number
  pass: number
  reject: number
  review: number
  failed: number
  pending: number
  pass_rate: number
  avg_duration_ms: number
}

export interface ProjectBreakdown {
  project_id: number
  project_name: string
  project_code: string
  total: number
  pass: number
  reject: number
  review: number
  failed: number
  pass_rate: number
}

export interface NameValue {
  name: string
  value: number
}

export interface AnalyticsData {
  range: { key: string; days: number; from: string; to: string }
  scope: { project_id: number; audit_id: number }
  overview: {
    total: number
    pass: number
    reject: number
    review: number
    failed: number
    pending: number
    queued: number
    pass_rate: number
    review_rate: number
    avg_duration_ms: number
  }
  trend: AnalyticsTrendPoint[]
  by_audit: AuditBreakdown[]
  by_project: ProjectBreakdown[]
  distribution: Array<{ key: string; name: string; value: number }>
  reasons: {
    labels: NameValue[]
    reasons: NameValue[]
    rules: NameValue[]
    errors: NameValue[]
  }
  sources: NameValue[]
  categories: NameValue[]
  media_types: NameValue[]
  statuses: NameValue[]
  latency_buckets: NameValue[]
  pending_tasks: Task[]
  sample_size: number
  sampled: boolean
  generated_at: string
}

export interface ReviewSummary {
  pending: number
  oldest_at?: string | null
  recent: Task[]
  by_audit: Array<{ audit_id: number; audit_name: string; count: number }>
}


