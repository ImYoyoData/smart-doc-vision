<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NIcon, NInput, NInputNumber, NSelect, NSwitch, NTabPane, NTabs, NTag } from 'naive-ui'
import { AddOutline, ArrowDownOutline, ArrowUpOutline, TrashOutline } from '@vicons/ionicons5'

interface RouteAction {
  type: string
  method?: string
  url?: string
  headers?: Record<string, string>
  params?: Record<string, string>
  body?: string
  timeout_ms?: number
  command?: string
  target?: string
  payload?: string
  dsn_env?: string
  query?: string
  args?: unknown[]
}

interface RouteConfig {
  exec_mode?: string
  on_error?: string
  actions: RouteAction[]
}

type RouteMap = Record<string, RouteConfig>

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const results = [
  { key: 'PASS', label: '通过 PASS' },
  { key: 'REJECT', label: '不通过 REJECT' },
  { key: 'REVIEW', label: '待议 REVIEW' },
]
const actionTypes = [
  { label: 'HTTP 回调', value: 'http' },
  { label: 'WebSocket 回调', value: 'websocket' },
  { label: 'Redis 写入', value: 'redis' },
  { label: 'MySQL 更新', value: 'mysql' },
]
const methodOptions = ['POST', 'PUT', 'PATCH', 'GET', 'DELETE'].map((value) => ({ label: value, value }))
const redisCommands = [
  { label: 'PUBLISH 频道', value: 'publish' },
  { label: 'SET 键值', value: 'set' },
  { label: 'XADD 消息流', value: 'xadd' },
]
const onErrorOptions = [
  { label: 'continue · 忽略失败继续执行', value: 'continue' },
  { label: 'stop · 停止后续动作', value: 'stop' },
  { label: 'to_review · 转人工复核', value: 'to_review' },
]

const active = ref('PASS')
const advanced = ref(false)
const rawText = ref('')
const rawError = ref('')
const draft = ref<RouteMap>(emptyRoutes())
let syncing = false

function emptyRoutes(): RouteMap {
  return {
    PASS: { exec_mode: 'sequential', on_error: 'continue', actions: [] },
    REJECT: { exec_mode: 'sequential', on_error: 'continue', actions: [] },
    REVIEW: { exec_mode: 'sequential', on_error: 'to_review', actions: [] },
  }
}

function normalizeAction(input: any): RouteAction {
  const action: RouteAction = { type: String(input?.type || 'http') }
  if (action.type === 'http') {
    action.method = input?.method || 'POST'
    action.url = input?.url || ''
    action.headers = { ...(input?.headers || {}) }
    action.params = { ...(input?.params || {}) }
    action.body = input?.body || ''
    action.timeout_ms = Number(input?.timeout_ms) || 10000
  } else if (action.type === 'websocket') {
    action.url = input?.url || ''
    action.headers = { ...(input?.headers || {}) }
    action.payload = input?.payload || ''
    action.timeout_ms = Number(input?.timeout_ms) || 10000
  } else if (action.type === 'redis') {
    action.command = input?.command || 'publish'
    action.target = input?.target || ''
    action.payload = input?.payload || ''
  } else if (action.type === 'mysql') {
    action.dsn_env = input?.dsn_env || 'CALLBACK_DSN'
    action.query = input?.query || ''
    action.args = Array.isArray(input?.args) ? input.args : []
  }
  return action
}

function normalize(value: any): RouteMap {
  const routes = emptyRoutes()
  if (!value || typeof value !== 'object' || Array.isArray(value)) return routes
  for (const item of results) {
    const source = value[item.key]
    if (!source || typeof source !== 'object') continue
    routes[item.key] = {
      exec_mode: 'sequential',
      on_error: source.on_error || (item.key === 'REVIEW' ? 'to_review' : 'continue'),
      actions: Array.isArray(source.actions) ? source.actions.map(normalizeAction) : [],
    }
  }
  return routes
}

function serialize(routes: RouteMap): string {
  return JSON.stringify(routes, null, 2)
}

function commit() {
  if (syncing) return
  emit('update:modelValue', serialize(draft.value))
}

function syncFromProp(value: string) {
  const parsedText = value || serialize(emptyRoutes())
  if (parsedText === serialize(draft.value)) return
  try {
    syncing = true
    draft.value = normalize(JSON.parse(parsedText))
    if (advanced.value) rawText.value = parsedText
    rawError.value = ''
  } catch {
    rawError.value = '当前内容不是合法 JSON，已保留编辑内容'
  } finally {
    syncing = false
  }
}

watch(() => props.modelValue, syncFromProp, { immediate: true })
watch(draft, commit, { deep: true })

function newAction(type: string): RouteAction {
  if (type === 'websocket') return { type, url: '', headers: {}, payload: '', timeout_ms: 10000 }
  if (type === 'redis') return { type, command: 'publish', target: '', payload: '' }
  if (type === 'mysql') return { type, dsn_env: 'CALLBACK_DSN', query: '', args: [] }
  return { type: 'http', method: 'POST', url: '', headers: {}, params: {}, body: '', timeout_ms: 10000 }
}

function addAction(type: string) {
  draft.value[active.value].actions.push(newAction(type))
}

function removeAction(index: number) {
  draft.value[active.value].actions.splice(index, 1)
}

function moveAction(index: number, offset: number) {
  const actions = draft.value[active.value].actions
  const target = index + offset
  if (target < 0 || target >= actions.length) return
  const [item] = actions.splice(index, 1)
  actions.splice(target, 0, item)
}

function kvText(map?: Record<string, string>): string {
  return Object.entries(map || {})
    .map(([key, value]) => `${key}: ${value}`)
    .join('\n')
}

function setKV(action: RouteAction, field: 'headers' | 'params', text: string) {
  const map: Record<string, string> = {}
  for (const line of text.split(/\r?\n/)) {
    const index = line.indexOf(':')
    if (index <= 0) continue
    const key = line.slice(0, index).trim()
    if (key) map[key] = line.slice(index + 1).trim()
  }
  action[field] = map
}

function argsText(action: RouteAction): string {
  return (action.args || []).map((value) => String(value)).join('\n')
}

function setArgs(action: RouteAction, text: string) {
  action.args = text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
}

function toggleAdvanced(value: boolean) {
  advanced.value = value
  if (value) rawText.value = serialize(draft.value)
}

function applyRaw(text: string) {
  rawText.value = text
  try {
    const parsed = JSON.parse(text)
    draft.value = normalize(parsed)
    rawError.value = ''
    emit('update:modelValue', text)
  } catch (error) {
    rawError.value = error instanceof Error ? error.message : 'JSON 解析失败'
  }
}

function actionTitle(action: RouteAction, index: number): string {
  const label = actionTypes.find((item) => item.value === action.type)?.label || action.type
  if (action.type === 'http') return `${index + 1}. ${label} · ${action.method || 'POST'} ${action.url || '未填写地址'}`
  if (action.type === 'websocket') return `${index + 1}. ${label} · ${action.url || '未填写地址'}`
  if (action.type === 'redis') return `${index + 1}. ${label} · ${(action.command || 'publish').toUpperCase()} ${action.target || '未填写目标'}`
  return `${index + 1}. ${label} · ${action.dsn_env || '未填写 DSN 环境变量'}`
}
</script>

<template>
  <div class="routes-editor">
    <div class="routes-toolbar">
      <span class="toolbar-note">按最终结论执行回调动作，模板变量支持 <code>${task_no}</code>、<code>${result}</code>、<code>${score}</code>、<code>${category}</code> 等。</span>
      <div class="toolbar-actions">
        <span class="advanced-label">高级 JSON</span>
        <n-switch :value="advanced" size="small" @update:value="toggleAdvanced" />
      </div>
    </div>

    <n-input
      v-if="advanced"
      :value="rawText"
      type="textarea"
      :autosize="{ minRows: 10, maxRows: 22 }"
      class="code-input"
      spellcheck="false"
      @update:value="applyRaw"
    />
    <p v-if="advanced && rawError" class="routes-error">{{ rawError }}</p>

    <n-tabs v-else v-model:value="active" type="segment" animated>
      <n-tab-pane v-for="item in results" :key="item.key" :name="item.key" :tab="`${item.label}（${draft[item.key].actions.length}）`">
        <div class="result-head">
          <div class="field-inline">
            <span>动作失败时</span>
            <n-select
              v-model:value="draft[item.key].on_error"
              :options="onErrorOptions"
              size="small"
              style="width: 260px"
            />
          </div>
          <n-select
            :value="null"
            :options="actionTypes"
            size="small"
            placeholder="+ 添加动作"
            style="width: 150px"
            @update:value="addAction"
          />
        </div>

        <div v-if="!draft[item.key].actions.length" class="routes-empty">
          暂无动作。该结论只落库，不向外部发送通知。
        </div>

        <div v-for="(action, index) in draft[item.key].actions" :key="index" class="action-card">
          <div class="action-head">
            <div class="action-title">
              <n-tag size="small" :bordered="false" type="info">{{ action.type.toUpperCase() }}</n-tag>
              <span>{{ actionTitle(action, index) }}</span>
            </div>
            <div class="action-ops">
              <n-button quaternary size="tiny" :disabled="index === 0" @click="moveAction(index, -1)">
                <template #icon><n-icon :component="ArrowUpOutline" /></template>
              </n-button>
              <n-button quaternary size="tiny" :disabled="index === draft[item.key].actions.length - 1" @click="moveAction(index, 1)">
                <template #icon><n-icon :component="ArrowDownOutline" /></template>
              </n-button>
              <n-button quaternary size="tiny" type="error" @click="removeAction(index)">
                <template #icon><n-icon :component="TrashOutline" /></template>
              </n-button>
            </div>
          </div>

          <div v-if="action.type === 'http'" class="action-grid">
            <div class="field-inline">
              <n-select v-model:value="action.method" :options="methodOptions" size="small" style="width: 110px" />
              <n-input v-model:value="action.url" size="small" placeholder="https://example.com/callback" />
            </div>
            <div class="two-col">
              <div class="field">
                <label>查询参数 <span>每行 key: value，支持模板变量</span></label>
                <n-input :value="kvText(action.params)" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" placeholder="source: smartdoc" @update:value="(v) => setKV(action, 'params', v)" />
              </div>
              <div class="field">
                <label>请求头 <span>每行 key: value</span></label>
                <n-input :value="kvText(action.headers)" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" placeholder="Authorization: Bearer token" @update:value="(v) => setKV(action, 'headers', v)" />
              </div>
            </div>
            <div class="field">
              <label>请求体 <span>留空则发送标准结果 JSON</span></label>
              <n-input v-model:value="action.body" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" class="code-input" placeholder='{"task_no":"${task_no}","result":"${result}"}' />
            </div>
            <div class="field-inline">
              <span>超时</span>
              <n-input-number v-model:value="action.timeout_ms" size="small" :min="1000" :max="30000" :step="1000" style="width: 140px" />
              <span class="muted">毫秒</span>
            </div>
          </div>

          <div v-else-if="action.type === 'websocket'" class="action-grid">
            <div class="field">
              <label>WebSocket 地址 <span>仅支持 ws:// 或 wss://</span></label>
              <n-input v-model:value="action.url" size="small" placeholder="wss://example.com/audit/callback" />
            </div>
            <div class="field">
              <label>握手请求头 <span>每行 key: value，可用于鉴权</span></label>
              <n-input :value="kvText(action.headers)" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" placeholder="Authorization: Bearer token" @update:value="(v) => setKV(action, 'headers', v)" />
            </div>
            <div class="field">
              <label>消息内容 <span>留空则发送标准结果 JSON</span></label>
              <n-input v-model:value="action.payload" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" class="code-input" placeholder='{"task_no":"${task_no}","result":"${result}"}' />
            </div>
            <div class="field-inline">
              <span>超时</span>
              <n-input-number v-model:value="action.timeout_ms" size="small" :min="1000" :max="30000" :step="1000" style="width: 140px" />
              <span class="muted">毫秒</span>
            </div>
          </div>

          <div v-else-if="action.type === 'redis'" class="action-grid">
            <div class="field-inline">
              <n-select v-model:value="action.command" :options="redisCommands" size="small" style="width: 160px" />
              <n-input v-model:value="action.target" size="small" placeholder="频道 / 键 / Stream 名称，支持模板变量" />
            </div>
            <div class="field">
              <label>消息内容 <span>留空则发送标准结果 JSON</span></label>
              <n-input v-model:value="action.payload" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" class="code-input" placeholder='{"task_no":"${task_no}","result":"${result}"}' />
            </div>
          </div>

          <div v-else class="action-grid">
            <div class="field">
              <label>DSN 环境变量 <span>例如 CALLBACK_DSN，避免把连接串写进配置</span></label>
              <n-input v-model:value="action.dsn_env" size="small" placeholder="CALLBACK_DSN" />
            </div>
            <div class="field">
              <label>SQL 语句 <span>建议使用参数占位符 ?</span></label>
              <n-input v-model:value="action.query" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" class="code-input" placeholder="UPDATE orders SET audit_result = ?, audit_note = ? WHERE order_no = ?" />
            </div>
            <div class="field">
              <label>SQL 参数 <span>每行一个，支持模板变量</span></label>
              <n-input :value="argsText(action)" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" class="code-input" placeholder="${result}&#10;${summary}&#10;${biz_key}" @update:value="(v) => setArgs(action, v)" />
            </div>
          </div>
        </div>
      </n-tab-pane>
    </n-tabs>

    <div class="routes-footer">
      <n-icon :component="AddOutline" />
      <span>HTTP 动作受 SSRF 防护；本地联调时可设置 <code>MEDIA_ALLOW_PRIVATE=true</code>。</span>
    </div>
  </div>
</template>

<style scoped>
.routes-editor {
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 10px 12px;
  background: #fbfcfe;
}
.routes-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.toolbar-note {
  font-size: 12px;
  color: var(--muted);
  line-height: 1.5;
}
.toolbar-note code,
.routes-footer code {
  padding: 1px 4px;
  border-radius: 4px;
  background: #eef2f8;
  font-size: 11px;
}
.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.advanced-label {
  font-size: 12px;
  color: var(--muted);
}
.result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}
.field-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--muted);
}
.routes-empty {
  padding: 16px;
  border: 1px dashed var(--border);
  border-radius: 8px;
  color: var(--muted);
  font-size: 12px;
  text-align: center;
}
.action-card {
  border: 1px solid var(--border);
  border-radius: 10px;
  background: #fff;
  padding: 10px 12px;
  margin-bottom: 10px;
}
.action-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.action-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  min-width: 0;
}
.action-title span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.action-ops {
  display: flex;
  gap: 2px;
}
.action-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.field label {
  font-size: 12px;
  color: #5a6472;
}
.field label span {
  color: var(--muted);
  font-weight: 400;
}
.muted {
  color: var(--muted);
  font-size: 12px;
}
.routes-error {
  margin: 6px 0 0;
  color: #d03050;
  font-size: 12px;
}
.routes-footer {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  font-size: 12px;
  color: var(--muted);
}
@media (max-width: 720px) {
  .two-col {
    grid-template-columns: 1fr;
  }
  .result-head {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
