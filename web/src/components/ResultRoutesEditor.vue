<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NIcon, NInput, NInputNumber, NPopover, NSelect, NSwitch, NTabPane, NTabs, NTag } from 'naive-ui'
import { AddOutline, ArrowDownOutline, ArrowUpOutline, HelpCircleOutline, TrashOutline } from '@vicons/ionicons5'

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

// 模板变量说明。替换后一律是字符串，这里标注的是原始数据类型与取值形式。
const templateVarGroups = [
  {
    title: '任务与来源',
    vars: [
      { name: 'task_no', desc: '任务号，平台生成的任务唯一编号', form: '字符串，SDV + 14 位时间 + 6 位大写 hex，如 SDV20261023233001A1B2C3' },
      { name: 'task_id', desc: '任务内部自增 ID', form: '整数，如 1024' },
      { name: 'biz_key', desc: '业务方提交时携带的业务主键', form: '字符串，可能为空' },
      { name: 'source_type', desc: '任务来源', form: '枚举：OPENAPI / ADMIN / REDIS / POLLING / WEBSOCKET' },
      { name: 'project_code', desc: '项目编码', form: '字符串，如 arch-review' },
      { name: 'project_id', desc: '项目内部 ID', form: '整数' },
      { name: 'audit_code', desc: '审核配置编码', form: '字符串，如 doc-audit' },
      { name: 'audit_id', desc: '审核配置内部 ID', form: '整数' },
    ],
  },
  {
    title: '结论与风险',
    vars: [
      { name: 'result', desc: '最终结论，人工复核后以人工结论为准', form: '枚举：PASS / REJECT / REVIEW' },
      { name: 'summary', desc: '一句话结论摘要', form: '字符串' },
      { name: 'reasons', desc: '判定原因列表', form: '字符串数组；模板替换后是空格分隔的裸文本' },
      { name: 'labels', desc: '命中标签列表', form: '字符串数组；模板替换后是空格分隔的裸文本' },
      { name: 'risk_level', desc: '风险等级', form: '枚举：low / medium / high' },
      { name: 'score', desc: '风险分，越高越危险（不是通过率）', form: '0~1 小数，如 0.12；模型失败兜底时按结论取 0.2 / 0.5 / 0.9' },
      { name: 'review_note', desc: '人工复核备注', form: '字符串，未复核时为空' },
    ],
  },
  {
    title: '分类',
    vars: [
      { name: 'category', desc: '分类编码，机器可读', form: '字符串，如 contract；仅分类类审核有值，普通审核通常为空' },
      { name: 'category_name', desc: '分类中文名', form: '字符串，如 合同协议' },
      { name: 'category_confidence', desc: '分类置信度', form: '0~1 小数，如 0.88' },
    ],
  },
]

// 模板里展示变量占位符用：避免在 SFC 里直接写 ${...} 引起插值歧义。
const tpl = (name: string) => '${' + name + '}'

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
      <span class="toolbar-note">
        按最终结论执行回调动作，模板变量支持 <code>${task_no}</code>、<code>${result}</code>、<code>${score}</code>、<code>${category}</code> 等。
        <n-popover trigger="click" placement="bottom-start" :width="560" :show-arrow="false">
          <template #trigger>
            <button type="button" class="vars-trigger">
              <n-icon :component="HelpCircleOutline" :size="14" />
              变量说明
            </button>
          </template>
          <div class="vars-panel">
            <p class="vars-intro">
              模板按纯文本替换。变量原始类型只在「请求体 / 消息内容 / SQL 参数」留空、发送标准结果 JSON 时保持；写进 URL、Header、SQL 时一律按字符串拼接。
            </p>
            <div v-for="group in templateVarGroups" :key="group.title" class="vars-group">
              <div class="vars-group-title">{{ group.title }}</div>
              <div v-for="item in group.vars" :key="item.name" class="vars-item">
                <div class="vars-item-head">
                  <code class="vars-item-name">{{ tpl(item.name) }}</code>
                  <span class="vars-item-desc">{{ item.desc }}</span>
                </div>
                <div class="vars-item-form">{{ item.form }}</div>
              </div>
            </div>
            <p class="vars-foot">
              注意：<code>{{ tpl('reasons') }}</code> / <code>{{ tpl('labels') }}</code> 是数组，替换后为空格分隔的裸文本，塞进 JSON 会破坏格式 —— 需要结构化字段时请留空请求体，走标准结果 JSON。<code>{{ tpl('score') }}</code> 是风险分，不是通过率。
            </p>
          </div>
        </n-popover>
      </span>
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
.vars-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: 2px;
  padding: 1px 6px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: #fff;
  color: #3a4a5f;
  font-size: 12px;
  line-height: 18px;
  cursor: pointer;
  vertical-align: baseline;
}
.vars-trigger:hover {
  border-color: #8fb4e8;
  color: #1f6fd0;
  background: #f2f7ff;
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

<!-- popover 内容会被 teleport 到 body，scoped 样式命中不到，故用唯一前缀类名单独声明。 -->
<style>
.vars-panel {
  max-height: 420px;
  overflow-y: auto;
  font-size: 12px;
  line-height: 1.55;
  color: #33415c;
}
.vars-intro,
.vars-foot {
  margin: 0 0 8px;
  color: #6b7787;
}
.vars-foot {
  margin: 8px 0 0;
  padding-top: 8px;
  border-top: 1px dashed var(--border, #e3e8ef);
}
.vars-group + .vars-group {
  margin-top: 10px;
}
.vars-group-title {
  margin-bottom: 2px;
  font-size: 12px;
  font-weight: 600;
  color: #1f2d3d;
}
.vars-item {
  padding: 5px 0;
  border-top: 1px solid #eef1f6;
}
.vars-group-title + .vars-item {
  border-top: none;
}
.vars-item-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.vars-item-name {
  flex-shrink: 0;
}
.vars-item-desc {
  color: #33415c;
}
.vars-item-form {
  margin-top: 1px;
  padding-left: 2px;
  color: #6b7787;
  font-size: 11.5px;
}
.vars-panel code {
  padding: 1px 4px;
  border-radius: 4px;
  background: #eef2f8;
  font-size: 11px;
}
</style>
