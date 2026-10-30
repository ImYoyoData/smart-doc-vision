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
  email?: string
  email_notify_enabled?: boolean
  auto_update_check?: boolean
  role: string
  status: string
  mfa_enabled?: boolean
  last_login_at?: string | null
  project_ids?: number[]
  audit_ids?: number[]
  permissions?: string[]
  menus?: string[]
}

export interface RoleDefinition {
  code: string
  name: string
  description: string
  permissions: string[]
  menus: string[]
  builtin?: boolean
}

export interface LoginResult {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export interface MFAChallenge {
  mfa_required: true
  mfa_ticket: string
  expires_in: number
}

export interface MFAStatus {
  enabled: boolean
}

export interface MFASetup {
  secret: string
  otpauth_url: string
  issuer: string
  account: string
  expires_in: number
  digits: number
  period: number
  algorithm: string
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
  max_tasks_per_day: number
  max_concurrent_tasks: number
  openapi_rate_limit: number
  js_max_memory_mb: number
  js_timeout_ms: number
  js_max_log_bytes: number
  js_max_result_bytes: number
  js_max_http_requests: number
  js_max_http_response_bytes: number
  js_max_db_rows: number
  js_max_concurrency: number
  deleted_at?: string | null
  created_at: string
  updated_at: string
  audit_count: number
  provider_count: number
  today_tasks: number
}

export interface ProjectSecret {
  id: number
  name: string
  created_at: string
  updated_at: string
}

export interface ScriptSimulationResult {
  status: string
  error_type?: string
  error_message?: string
  duration_ms: number
  logs: string[]
  output?: unknown
  variables: Record<string, unknown>
  memory_bytes: number
  timeout_ms: number
  memory_mb: number
}

export interface ScriptExecution {
  id: number
  project_id: number
  audit_id: number
  task_id: number
  task_no: string
  action_index: number
  remark: string
  script_hash: string
  started_at: string
  duration_ms: number
  status: 'SUCCESS' | 'SCRIPT_ERROR' | 'TIMEOUT' | 'OOM' | 'HOST_ERROR' | string
  error_type: string
  error_message: string
  log_text: string
  memory_bytes: number
  result_json: unknown
  created_at: string
}

export interface ProviderConfig {
  supports_vision?: boolean
  max_tokens?: number
  temperature?: number
  max_images?: number
  context_window?: number
  supports_thinking?: boolean
  prefer_reasoning?: boolean
  headers?: Record<string, string>
}

export interface ProviderModel {
  id: string
  name?: string
  context_window: number
  context_window_known: boolean
  max_output_tokens: number
  max_output_known: boolean
  supports_vision: boolean
  vision_known: boolean
  supports_thinking: boolean
  thinking_known: boolean
  source: 'metadata' | 'unknown' | string
}

export interface ProviderUsageSummary {
  requests: number
  input_tokens: number
  cached_tokens: number
  output_tokens: number
  total_tokens: number
  last_used_at?: string | null
}

export interface ProviderUsageLog {
  id: number
  project_id: number
  provider_id: number
  provider_name: string
  kind: string
  model: string
  source: string
  audit_id: number
  task_id: number
  input_tokens: number
  cached_tokens: number
  output_tokens: number
  total_tokens: number
  duration_ms: number
  status: string
  error_message?: string
  created_at: string
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
  priority: number
  last_test_at?: string | null
  last_test_ok: boolean
  last_test_ms: number
  last_test_message: string
  breaker_until?: string | null
  breaker_reason: string
  failure_count: number
  created_at: string
  updated_at: string
  api_key_masked: string
  has_api_key: boolean
  usage?: ProviderUsageSummary
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

export type AuditDecisionMode = 'REVIEW' | 'CLASSIFY' | 'CLASSIFY_ONLY'

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
  review_mode?: 'INTERNAL' | 'EXTERNAL'
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

export interface RedisConnection {
  id: number
  project_id: number
  name: string
  addr: string
  username: string
  tls: boolean
  status: 'ACTIVE' | 'DISABLED' | string
  health: 'UNKNOWN' | 'OK' | 'UNHEALTHY' | string
  auto_resume_triggers: boolean
  last_test_at?: string | null
  last_test_ok: boolean
  last_test_ms: number
  last_test_message: string
  has_password: boolean
  created_at: string
  updated_at: string
}

export interface NotificationEmailConfig {
  host: string
  port: number
  username: string
  from_name: string
  from_address: string
  security: string
  timeout_sec: number
}

export interface NotificationChannel {
  id: number
  project_id: number
  name: string
  type: string
  provider: string
  status: string
  config: NotificationEmailConfig
  has_password: boolean
  last_test_at?: string | null
  last_test_ok: boolean
  last_test_message: string
  created_at: string
  updated_at: string
}

export interface InAppMessage {
  id: number
  type: string
  title: string
  body: string
  link: string
  project_id: number
  audit_id: number
  task_id: number
  read: boolean
  read_at?: string | null
  created_at: string
}

export interface InAppMessagePage extends Paged<InAppMessage> {
  unread_count: number
}

export interface SystemConfigField {
  key: string
  label: string
  value: string
  sensitive: boolean
  configured: boolean
}

export interface SystemConfigGroup {
  key: string
  title: string
  description: string
  fields: SystemConfigField[]
}

export interface SystemConfigData {
  version: string
  env_file: string
  config_file: string
  groups: SystemConfigGroup[]
}

export interface UpdateAsset {
  name: string
  size: number
  url: string
  kind: string
}

export interface UpdateCheck {
  current_version: string
  latest_version: string
  has_update: boolean
  release_name: string
  release_url: string
  published_at: string
  notes: string
  assets: UpdateAsset[]
  checksums_url: string
  repo_url: string
  checked_at: string
}

export interface NotificationPreference {
  email_notify_enabled: boolean
  has_email: boolean
  email: string
  has_channel: boolean
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
  redis_connection_id?: number | null
  redis_db: number
  stream: string
  consumer_group: string
  start_position: string
  batch_size: number
  max_inflight: number
  pause_reason: string
  auto_paused: boolean
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
  redis_connection_id?: number
  redis_db?: number
  stream?: string
  consumer_group?: string
  start_position?: string
  batch_size?: number
  max_inflight?: number
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
    connection_id?: number | null
    connection_name?: string
    connection_addr?: string
    db?: number
    stream: string
    group: string
    start_position?: string
    batch_size: number
    max_inflight?: number
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
    transport?: string
    http_status?: number
    attempts?: number
    error?: string
    target_url?: string
    signed?: boolean
    response?: string
    result?: string
  } | null
  routes?: Array<{
    index?: number
    type?: string
    ok?: boolean
    duration_ms?: number
    error?: string
  }>
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

export interface AuditAssistantMessage {
  role: 'user' | 'assistant'
  content: string
  images?: string[]
  steps?: AuditAssistantStep[]
  pending?: boolean
}

export interface AuditAssistantStep {
  id: string
  label: string
  detail?: string
  status: 'running' | 'done' | 'error'
  at: number
}

export interface AuditAssistantConfig {
  name: string
  code: string
  description: string
  media_types: string[]
  decision_mode: string
  categories: AuditCategory[]
  provider_id: number
  review_prompt: string
  rules: RuleGroup[]
  routes: Record<string, unknown>
  default_result: string
  on_model_fail: string
  review_mode?: 'INTERNAL' | 'EXTERNAL'
  dedupe_window_sec: number
  triggers?: Record<string, string>
  redis_trigger?: {
    connection_id: number
    db: number
    stream: string
    consumer_group?: string
    start_position?: string
    batch_size?: number
    max_inflight?: number
  }
}

export interface AuditAssistantResult {
  reply: string
  ready: boolean
  model?: string
  provider_id?: number
  provider_name?: string
  config?: AuditAssistantConfig
  steps?: AuditAssistantStep[]
  images?: number
}

export interface AuditFieldAssistantResult {
  field: 'rules' | 'review_prompt' | string
  text: string
  explanation: string
  provider_id: number
  provider_name: string
  model?: string
}

export interface RuleTemplate {
  key: string
  name: string
  description: string
  kind?: AuditDecisionMode | string
  rules?: RuleGroup[]
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



export interface MonitorPoint {
  time: string
  heap_alloc_mb: number
  heap_sys_mb: number
  sys_mb: number
  goroutines: number
  threads: number
  in_flight: number
  requests: number
  errors: number
  avg_latency_ms: number
  gc_count: number
  host_cpu_percent: number
  host_mem_percent: number
}

export interface MonitorCurrent {
  started_at: string
  uptime_seconds: number
  goroutines: number
  threads: number
  gomaxprocs: number
  num_cpu: number
  heap_alloc_mb: number
  heap_inuse_mb: number
  heap_sys_mb: number
  stack_inuse_mb: number
  sys_mb: number
  gc_count: number
  gc_pause_total_ms: number
  in_flight: number
  requests_total: number
  errors_total: number
  avg_latency_ms: number
  host_cpu_percent: number
  host_mem_percent: number
  host_mem_used_mb: number
  host_mem_total_mb: number
  host_available: boolean
  sample_interval_seconds: number
  retention_hours: number
}

export interface MonitorQueueItem {
  project_id: number
  project_name: string
  project_code: string
  active: number
  pending: number
  limit: number
}

export interface MonitorData {
  range: string
  since: string
  now: string
  sample_interval_seconds: number
  retention_hours: number
  current: MonitorCurrent
  history: MonitorPoint[]
  queue: MonitorQueueItem[]
  tasks: Record<string, number>
  dependencies: { mysql: boolean; redis: boolean }
  mysql_pool: {
    open_connections?: number
    in_use?: number
    idle?: number
    max_open?: number
    wait_count?: number
    wait_duration_ms?: number
  }
  redis_pool: {
    total_conns?: number
    idle_conns?: number
    hits?: number
    misses?: number
    timeouts?: number
  }
}
