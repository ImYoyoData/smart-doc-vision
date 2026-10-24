<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NModal, NSelect, NSpin, NTooltip, useThemeVars } from 'naive-ui'
import { EditorView, basicSetup } from 'codemirror'
import { Compartment } from '@codemirror/state'
import { javascript, javascriptLanguage } from '@codemirror/lang-javascript'
import { autocompletion, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { oneDark } from '@codemirror/theme-one-dark'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import {
  AlertCircleOutline,
  BuildOutline,
  ChevronForwardOutline,
  CloseOutline,
  CodeSlashOutline,
  CreateOutline,
  DocumentTextOutline,
  SaveOutline,
  SendOutline,
  SparklesOutline,
  TerminalOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import { providerApi, scriptApi } from '@/api'
import type { Provider } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  show: boolean
  script: string
  remark: string
  auditId: number
}>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'update:script', value: string): void
  (e: 'update:remark', value: string): void
}>()

const workspace = useWorkspaceStore()
const themeVars = useThemeVars()
const editorHost = ref<HTMLElement | null>(null)
let editorView: EditorView | null = null
let syncing = false
const editableCompartment = new Compartment()

function isDarkColor(value: string) {
  const hex = value.trim().match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/i)
  if (hex) {
    let digits = hex[1]
    if (digits.length === 3) digits = digits.split('').map((part) => part + part).join('')
    const r = parseInt(digits.slice(0, 2), 16)
    const g = parseInt(digits.slice(2, 4), 16)
    const b = parseInt(digits.slice(4, 6), 16)
    return 0.2126 * r + 0.7152 * g + 0.0722 * b < 128
  }
  const parts = value.match(/[\d.]+/g)
  if (!parts || parts.length < 3) return false
  const [r, g, b] = parts.map(Number)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b < 128
}

const isDark = computed(() => isDarkColor(themeVars.value.bodyColor))

interface AiTool {
  id: string
  name: string
  label: string
  status: string
  detail?: string
}

interface AiMessage {
  role: 'user' | 'assistant'
  content: string
  script?: string
  scriptValid?: boolean
  scriptError?: string
  applied?: boolean
}

const aiOpen = ref(false)
const aiMessages = ref<AiMessage[]>([])
const aiTools = ref<AiTool[]>([])
const aiInput = ref('')
const aiBusy = ref(false)
const aiError = ref('')
const aiStatus = ref('')
const aiProviderId = ref<number | null>(null)
const aiProviders = ref<Provider[]>([])
const aiReasoning = ref('auto')
const aiLastPrompt = ref('')
const apiShow = ref(false)
const initialScript = ref(props.script)
const initialRemark = ref(props.remark)
const closeConfirmShow = ref(false)
let aiAbort: AbortController | null = null
const aiSuggestions = [
  '检查当前脚本并修复潜在问题',
  '审核通过后把结果写回业务数据库',
  '审核不通过时调用 HTTP 接口通知业务方',
  '把最终审核结果推送到 Redis Stream',
]

const dirty = computed(() => props.script !== initialScript.value || props.remark !== initialRemark.value)
const scriptDirty = computed(() => props.script !== initialScript.value)
const closeConfirmTitle = computed(() => {
  if (aiBusy.value && dirty.value) return 'AI 正在执行，脚本尚未保存'
  if (aiBusy.value) return 'AI 正在执行中'
  if (dirty.value) return '脚本尚未保存'
  return '关闭编辑框'
})
const closeConfirmMessage = computed(() => {
  if (aiBusy.value && dirty.value) {
    return '关闭编辑框会停止当前 AI 请求。你可以保存当前修改后关闭，或放弃本次修改。'
  }
  if (aiBusy.value) {
    return '当前 AI 脚本助手仍在生成内容，停止后将关闭编辑框。'
  }
  if (dirty.value) {
    return '当前脚本或备注有修改，请选择保存后关闭或放弃修改。'
  }
  return '当前没有未保存的修改，可以安全关闭。'
})
const closePrimaryText = computed(() => {
  if (aiBusy.value && dirty.value) return '停止 AI 并保存关闭'
  if (aiBusy.value) return '停止 AI 并关闭'
  if (dirty.value) return '保存并关闭'
  return '关闭'
})

interface ApiDocItem {
  name: string
  detail: string
  fields?: string
}

interface ApiDocGroup {
  title: string
  items: ApiDocItem[]
}

const apiDocGroups: ApiDocGroup[] = [
  {
    title: '脚本入口',
    items: [
      { name: '顶层代码', detail: '直接编写即可，平台已注入全局 ctx，不需要定义 main 函数' },
      { name: '返回值', detail: '把对象/数组表达式放在最后一行并加行首分号，例如 ;({ ok: true })；不要使用顶层 return' },
      { name: 'function main(ctx)', detail: '旧脚本兼容；定义后平台会调用它并记录返回值' },
      { name: 'console / ctx.log', detail: '输出写入脚本执行日志，便于排查' },
    ],
  },
  {
    title: '任务与审核',
    items: [
      { name: 'ctx.task', detail: '当前任务', fields: 'task_no / biz_key / source_type / input / status' },
      { name: 'ctx.audit', detail: '当前审核配置', fields: 'code / name / categories / decision_mode' },
      { name: 'ctx.audit.category(code)', detail: '按分类 code 取分类对象，不存在返回 null' },
      { name: 'ctx.result', detail: '最终结论：PASS / REJECT / REVIEW' },
      { name: 'ctx.detail', detail: '摘要、风险等级、风险分、分类等结论字段' },
      { name: 'ctx.payload', detail: '平台标准结果 JSON，可直接回传' },
      { name: 'ctx.variables()', detail: '模板变量对象，保留数字和数组的真实类型' },
      { name: 'ctx.get(path)', detail: '按路径读取上下文，例如 audit.code' },
    ],
  },
  {
    title: '数据库',
    items: [
      { name: 'ctx.mysql.connect(options)', detail: 'MySQL 连接；最多同时打开 2 个连接', fields: 'host / port / user / password / database / tls' },
      { name: 'ctx.pg.connect(options)', detail: 'PostgreSQL 连接；脚本结束自动关闭', fields: 'host / port / user / password / database / sslmode' },
      { name: 'db.query(sql, params?)', detail: '执行查询，返回行对象数组；受项目 DB 行数上限约束' },
      { name: 'db.exec(sql, params?)', detail: '执行写入，返回 { rows_affected, last_insert_id }' },
      { name: 'db.begin()', detail: '开启事务，返回 { query, exec, commit, rollback }' },
      { name: 'db.close()', detail: '立即关闭连接；脚本结束时也会统一释放' },
    ],
  },
  {
    title: 'HTTP 与加密',
    items: [
      { name: 'ctx.http.request(options)', detail: '仅支持 HTTP/HTTPS，不支持 socket', fields: 'url / method / headers / body / timeout_ms' },
      { name: 'HTTP 返回值', detail: '返回 { status, ok, body, body_base64, headers, bytes }' },
      { name: 'ctx.crypto.base64Encode(value)', detail: 'Base64 编码字符串' },
      { name: 'ctx.crypto.base64Decode(value)', detail: 'Base64 解码为 ArrayBuffer' },
      { name: 'ctx.crypto.hexEncode(value)', detail: '十六进制编码' },
      { name: 'ctx.crypto.sha256(value)', detail: 'SHA-256 小写十六进制' },
      { name: 'ctx.crypto.hmacSha256(key, value)', detail: 'HMAC-SHA256 小写十六进制' },
      { name: 'ctx.crypto.randomBytes(size)', detail: '安全随机字节，返回 ArrayBuffer' },
      { name: 'ctx.crypto.uuid()', detail: '随机 UUID v4' },
    ],
  },
  {
    title: '日志、密钥与返回值',
    items: [
      { name: 'ctx.log(...args)', detail: '写入当前脚本执行日志' },
      { name: 'console.log / info / warn / error', detail: '同样写入执行日志，并标注日志级别' },
      { name: 'ctx.secret(name)', detail: '读取当前项目密钥；推荐用它保存数据库密码和 Token' },
      { name: 'return value', detail: '返回值写入脚本执行记录；需可 JSON 序列化' },
      { name: '运行限制', detail: '不能读写文件、下载文件、使用 socket、eval 或定时器；超时、OOM 会按失败记录' },
    ],
  },
]

const apiExample = `const db = ctx.mysql.connect({
  host: '127.0.0.1',
  port: 3306,
  user: ctx.secret('DB_USER'),
  password: ctx.secret('DB_PASSWORD'),
  database: 'biz'
})
const rows = db.query(
  'select id from orders where task_no = ?',
  [ctx.task.task_no]
)
ctx.log('matched rows', rows.length)
;({ matched: rows.length, result: ctx.result })`

const aiProviderOptions = computed(() =>
  aiProviders.value.map((provider) => ({
    label: `${provider.name} · ${provider.model}`,
    value: provider.id,
  })),
)

const aiReasoningOptions = [
  { label: '自动', value: 'auto' },
  { label: '快速', value: 'low' },
  { label: '标准', value: 'medium' },
  { label: '深度', value: 'high' },
]

const ctxCompletions = [
  { label: 'ctx', type: 'variable', detail: '脚本上下文' },
  { label: 'ctx.task', type: 'property', detail: '当前任务', info: 'task_no / biz_key / source_type / input' },
  { label: 'ctx.task.task_no', type: 'property', detail: '任务号' },
  { label: 'ctx.task.biz_key', type: 'property', detail: '业务主键' },
  { label: 'ctx.task.input', type: 'property', detail: 'text/image 输入与 ext' },
  { label: 'ctx.project', type: 'property', detail: '当前项目', info: 'id / code / name' },
  { label: 'ctx.audit', type: 'property', detail: '当前审核配置' },
  { label: 'ctx.audit.code', type: 'property', detail: '审核编码' },
  { label: 'ctx.audit.name', type: 'property', detail: '审核名称' },
  { label: 'ctx.audit.categories', type: 'property', detail: '分类映射' },
  { label: 'ctx.audit.category(code)', type: 'method', detail: '按分类 code 取分类' },
  { label: 'ctx.audit.hasCategory(code)', type: 'method', detail: '判断分类是否存在' },
  { label: 'ctx.result', type: 'property', detail: 'PASS / REJECT / REVIEW' },
  { label: 'ctx.detail', type: 'property', detail: '摘要、风险、分类等结论' },
  { label: 'ctx.payload', type: 'property', detail: '标准结果 JSON' },
  { label: 'ctx.variables()', type: 'method', detail: '模板变量，带真实类型' },
  { label: 'ctx.get(path)', type: 'method', detail: '按路径读取，例如 audit.code' },
  { label: 'ctx.secret(name)', type: 'method', detail: '读取项目密钥' },
  { label: 'ctx.log(...)', type: 'method', detail: '写入执行日志' },
  { label: 'ctx.mysql.connect(options)', type: 'method', detail: '独立 MySQL 连接，脚本结束自动关闭' },
  { label: 'ctx.pg.connect(options)', type: 'method', detail: '独立 PostgreSQL 连接' },
  { label: 'ctx.http.request(options)', type: 'method', detail: 'HTTP/HTTPS 请求，不支持 socket' },
  { label: 'ctx.crypto.base64Encode(value)', type: 'method', detail: 'Base64 编码' },
  { label: 'ctx.crypto.base64Decode(value)', type: 'method', detail: 'Base64 解码为 ArrayBuffer' },
  { label: 'ctx.crypto.hexEncode(value)', type: 'method', detail: '十六进制编码' },
  { label: 'ctx.crypto.sha256(value)', type: 'method', detail: 'SHA-256 十六进制' },
  { label: 'ctx.crypto.hmacSha256(key, value)', type: 'method', detail: 'HMAC-SHA256' },
  { label: 'ctx.crypto.randomBytes(size)', type: 'method', detail: '安全随机字节' },
  { label: 'ctx.crypto.uuid()', type: 'method', detail: '随机 UUID' },
  { label: 'console.log', type: 'method', detail: '写入执行日志' },
  { label: 'console.warn', type: 'method', detail: '写入警告日志' },
  { label: 'console.error', type: 'method', detail: '写入错误日志' },
]

function completionSource(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/[\w.$]*/)
  if (!word || (word.from === word.to && !context.explicit)) return null
  return { from: word.from, options: ctxCompletions }
}

function createEditor() {
  if (!editorHost.value || editorView) return
  editorView = new EditorView({
    doc: props.script,
    extensions: [
      basicSetup,
      javascript(),
      javascriptLanguage.data.of({ autocomplete: completionSource }),
      autocompletion(),
      editableCompartment.of(EditorView.editable.of(!aiBusy.value)),
      isDark.value ? oneDark : [],
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !syncing) {
          emit('update:script', update.state.doc.toString())
        }
      }),
    ],
    parent: editorHost.value,
  })
}

function setEditorValue(value: string) {
  if (!editorView) return
  const current = editorView.state.doc.toString()
  if (current === value) return
  syncing = true
  editorView.dispatch({ changes: { from: 0, to: current.length, insert: value } })
  syncing = false
}

function setEditorEditable(editable: boolean) {
  editorView?.dispatch({ effects: editableCompartment.reconfigure(EditorView.editable.of(editable)) })
}

watch(() => props.script, (value) => setEditorValue(value))
watch(() => props.show, (value) => {
  if (value) {
    initialScript.value = props.script
    initialRemark.value = props.remark
    nextTick(() => {
      createEditor()
      setEditorValue(props.script)
      editorView?.focus()
    })
  } else {
    closeConfirmShow.value = false
    apiShow.value = false
    stopAiRequest()
  }
}, { immediate: true })

watch(isDark, () => {
  if (!props.show || !editorView) return
  const value = editorView.state.doc.toString()
  editorView.destroy()
  editorView = null
  nextTick(() => {
    createEditor()
    setEditorValue(value)
  })
})

onMounted(() => {
  createEditor()
})

onBeforeUnmount(() => {
  aiAbort?.abort()
  editorView?.destroy()
  editorView = null
})

function stopAiRequest() {
  if (aiAbort) {
    aiAbort.abort()
    aiAbort = null
  }
  aiBusy.value = false
  aiStatus.value = ''
  setEditorEditable(true)
}

function closeNow() {
  closeConfirmShow.value = false
  apiShow.value = false
  emit('update:show', false)
}

function requestClose() {
  if (!dirty.value && !aiBusy.value) {
    closeNow()
    return
  }
  closeConfirmShow.value = true
}

function requestSaveClose() {
  if (!aiBusy.value) {
    closeNow()
    return
  }
  closeConfirmShow.value = true
}

function discardAndClose() {
  emit('update:script', initialScript.value)
  emit('update:remark', initialRemark.value)
  setEditorValue(initialScript.value)
  stopAiRequest()
  closeNow()
}

function saveAndClose() {
  const value = editorView?.state.doc.toString()
  if (typeof value === 'string') emit('update:script', value)
  stopAiRequest()
  closeNow()
}

function toggleAi() {
  if (aiOpen.value) {
    aiOpen.value = false
    return
  }
  void openAi()
}

function renderMarkdown(content: string) {
  const html = marked.parse(content || '', { breaks: true, gfm: true })
  return DOMPurify.sanitize(typeof html === 'string' ? html : '')
}

async function loadProviders(force = false) {
  if (!workspace.currentProjectId) return
  if (!force && aiProviders.value.length) return
  try {
    const data = await providerApi.list({ project_id: workspace.currentProjectId })
    aiProviders.value = data.items.filter((provider) => provider.status === 'ACTIVE' && provider.kind !== 'JEV')
    if (!aiProviderId.value || !aiProviders.value.some((provider) => provider.id === aiProviderId.value)) {
      aiProviderId.value = aiProviders.value[0]?.id || null
    }
    if (!aiProviders.value.length) {
      aiError.value = '当前项目没有启用的通用大模型，请先在模型接入中启用 OpenAI 兼容或 Anthropic 模型'
    }
  } catch (error) {
    aiError.value = normalizeAiError(error, '加载通用大模型失败')
  }
}

async function openAi() {
  aiOpen.value = true
  aiError.value = ''
  await loadProviders(true)
}

function normalizeAiError(error: unknown, fallback = '脚本助手请求失败') {
  if (!(error instanceof Error)) return fallback
  if (/failed to fetch|networkerror|load failed|network request failed/i.test(error.message)) {
    return '无法连接后端服务，请检查网络或后端是否已启动'
  }
  return error.message || fallback
}

function retryAi() {
  if (!aiLastPrompt.value || aiBusy.value) return
  const last = aiMessages.value[aiMessages.value.length - 1]
  if (last?.role === 'user' && last.content === aiLastPrompt.value) aiMessages.value.pop()
  aiInput.value = aiLastPrompt.value
  void sendAi()
}

async function sendAi() {
  const text = aiInput.value.trim()
  if (!text || aiBusy.value) return
  if (!aiProviderId.value || !aiProviders.value.some((provider) => provider.id === aiProviderId.value)) {
    await loadProviders(true)
  }
  if (!aiProviderId.value) {
    aiError.value = '当前项目没有启用的通用大模型，请先在模型接入中启用'
    return
  }
  aiInput.value = ''
  aiError.value = ''
  aiLastPrompt.value = text
  aiTools.value = []
  aiMessages.value.push({ role: 'user', content: text })
  aiBusy.value = true
  aiStatus.value = 'AI 正在读取脚本…'
  setEditorEditable(false)
  const controller = new AbortController()
  aiAbort = controller
  let payload: Record<string, any> | null = null
  let streamError = ''
  let timedOut = false
  let applied = false
  const timeout = window.setTimeout(() => {
    timedOut = true
    controller.abort()
  }, 300_000)
  try {
    await scriptApi.assistantStream({
      project_id: workspace.currentProjectId,
      provider_id: aiProviderId.value,
      audit_id: props.auditId || 0,
      current_script: props.script,
      reasoning: aiReasoning.value === 'auto' ? undefined : aiReasoning.value,
      messages: aiMessages.value.map((message) => ({ role: message.role, content: message.content })),
    }, {
      signal: controller.signal,
      onEvent: (event, data) => {
        const value = data as Record<string, any>
        if (event === 'step' && value?.label) {
          aiStatus.value = String(value.label)
        } else if (event === 'tool' && value?.id) {
          upsertAiTool({
            id: String(value.id),
            name: String(value.name || 'tool'),
            label: String(value.label || value.name || '工具调用'),
            status: String(value.status || 'running'),
            detail: value.detail ? String(value.detail) : '',
          })
          if (value.status === 'running') aiStatus.value = `正在${value.label || '调用工具'}…`
        } else if (event === 'script' && typeof value?.script === 'string') {
          if (applyScriptFromAi(value.script)) applied = true
          if (value.summary) aiStatus.value = String(value.summary)
        } else if (event === 'reply') {
          payload = value
        } else if (event === 'error') {
          streamError = String(value?.message || '脚本助手请求失败')
        }
      },
    })
    if (timedOut) {
      aiError.value = '模型响应超时（300 秒），请重试或换用更快的模型'
      return
    }
    if (controller.signal.aborted) return
    if (streamError) throw new Error(streamError)
    const result = payload as Record<string, any> | null
    if (!result) throw new Error('模型没有返回结果，连接可能已中断，请重试')
    const resultScript = String(result.script || '')
    if (resultScript && applyScriptFromAi(resultScript)) applied = true
    if (resultScript) applied = applied || Boolean(result.changed)
    aiMessages.value.push({
      role: 'assistant',
      content: String(result.reply || '已更新脚本'),
      script: result.changed ? resultScript : '',
      scriptValid: Boolean(result.script_valid),
      scriptError: String(result.script_error || ''),
      applied,
    })
  } catch (error) {
    if (timedOut) {
      aiError.value = '模型响应超时（300 秒），请重试或换用更快的模型'
    } else if (!controller.signal.aborted && aiAbort === controller) {
      aiError.value = normalizeAiError(error)
    }
  } finally {
    window.clearTimeout(timeout)
    if (aiAbort === controller) {
      aiAbort = null
      aiBusy.value = false
      aiStatus.value = ''
      setEditorEditable(true)
    }
  }
}

function applyScriptFromAi(script: string) {
  const current = editorView?.state.doc.toString() ?? props.script
  if (current === script) return false
  emit('update:script', script)
  setEditorValue(script)
  return true
}

function upsertAiTool(tool: AiTool) {
  const index = aiTools.value.findIndex((item) => item.id === tool.id)
  if (index >= 0) {
    aiTools.value[index] = tool
  } else {
    aiTools.value.push(tool)
  }
}

function scriptLineTotal(script?: string) {
  const text = (script || '').trim()
  return text ? text.split(/\r?\n/).length : 0
}

function handleAiKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault()
  void sendAi()
}
</script>

<template>
  <n-modal :show="show" :mask-closable="false" @update:show="(value: boolean) => { if (!value) requestClose() }">
    <div class="script-editor-modal" :class="{ 'script-theme-light': !isDark }">
      <div class="script-editor-head">
        <div class="script-editor-title">
          <span class="script-editor-mark">
            <n-icon :component="CodeSlashOutline" />
          </span>
          <div class="script-editor-heading">
            <div class="script-editor-heading-row">
              <span class="script-editor-kicker">JS RESULT ACTION</span>
              <strong>编辑脚本</strong>
            </div>
            <div class="script-editor-sub">
              <span class="script-editor-sub-dot" />
              全局 <code>ctx</code> 直接可用 · 结束自动释放连接
            </div>
          </div>
        </div>
        <div class="script-editor-remark">
          <n-input
            :value="remark"
            size="small"
            maxlength="128"
            placeholder="添加备注（可选，例如：订单回调校验）"
            :input-props="{ 'aria-label': '脚本备注' }"
            @update:value="(value: string) => emit('update:remark', value)"
          >
            <template #prefix><n-icon :component="CreateOutline" /></template>
          </n-input>
        </div>
        <div class="script-editor-actions">
          <n-button size="small" secondary @click="apiShow = true">
            <template #icon><n-icon :component="DocumentTextOutline" /></template>
            API 文档
          </n-button>
          <n-button
            size="small"
            secondary
            :class="{ 'is-active': aiOpen }"
            @click="toggleAi"
          >
            <template #icon><n-icon :component="SparklesOutline" /></template>
            AI 脚本助手
            <span v-if="aiOpen || aiBusy" class="ai-state-dot" :class="{ running: aiBusy }" />
          </n-button>
          <n-button size="small" type="primary" @click="requestSaveClose">
            <template #icon><n-icon :component="SaveOutline" /></template>
            保存脚本
          </n-button>
          <span class="script-editor-divider" />
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button circle quaternary @click="requestClose">
                <template #icon><n-icon :component="CloseOutline" /></template>
              </n-button>
            </template>
            关闭
          </n-tooltip>
        </div>
      </div>

      <div class="script-editor-toolbar">
        <div class="editor-file-tab">
          <span v-if="scriptDirty" class="editor-modified-dot" title="脚本有未保存修改" />
          <span class="editor-lang">JS</span>
          <span class="editor-file-name">main.js</span>
          <span class="editor-file-meta">· UTF-8</span>
        </div>
        <div class="editor-toolbar-meta">
          <span v-if="aiBusy" class="editor-ai-lock"><span class="editor-ai-lock-dot" />AI 编辑中 · 只读</span>
          <span class="editor-sandbox"><span class="editor-sandbox-dot" />受限沙箱</span>
          <span class="editor-entry-hint">全局 ctx 已注入</span>
        </div>
      </div>

      <div class="script-editor-body">
        <div ref="editorHost" class="script-editor-host" />

        <Transition name="script-ai-slide">
          <aside v-if="aiOpen" class="script-ai-panel">
            <div class="script-ai-head">
              <div class="script-ai-title">
                <span class="script-ai-title-icon">
                  <n-icon :component="SparklesOutline" />
                </span>
                <div>
                  <strong>AI 脚本助手</strong>
                  <small>业务脚本 Skill · 可读写 · 可联网</small>
                </div>
              </div>
              <button type="button" class="script-ai-collapse" @click="aiOpen = false">
                收起
                <n-icon :component="ChevronForwardOutline" />
              </button>
            </div>
            <div class="script-ai-warning">
              <n-icon :component="WarningOutline" />
              <span>AI 会直接修改当前脚本，可 <kbd>Ctrl</kbd>+<kbd>Z</kbd> 撤销。请使用 <code>ctx.secret()</code>，不要在脚本里写明文密码。</span>
            </div>
            <div class="script-ai-messages">
              <div v-if="!aiMessages.length" class="script-ai-empty">
                <p>AI 已加载当前审核配置和脚本 API，可直接读写编辑器、联网查资料；不确定时会先向你确认。</p>
                <div class="script-ai-starters">
                  <button
                    v-for="suggestion in aiSuggestions"
                    :key="suggestion"
                    type="button"
                    class="script-ai-starter"
                    @click="aiInput = suggestion"
                  >
                    <span>{{ suggestion }}</span>
                    <n-icon :component="ChevronForwardOutline" />
                  </button>
                </div>
              </div>
              <div v-for="(message, index) in aiMessages" :key="index" class="script-ai-message" :class="message.role">
                <span class="script-ai-avatar">{{ message.role === 'user' ? '我' : 'AI' }}</span>
                <div class="script-ai-bubble">
                  <div class="script-ai-markdown" v-html="renderMarkdown(message.content)" />
                  <div v-if="message.script" class="script-ai-code">
                    <div class="script-ai-code-head">
                      <span>{{ message.applied ? '已写入编辑器' : '当前脚本' }} · {{ scriptLineTotal(message.script) }} 行</span>
                      <span v-if="message.scriptValid" class="script-ai-ok">语法通过</span>
                      <span v-else-if="message.scriptError" class="script-ai-error">语法错误</span>
                    </div>
                    <p v-if="message.applied" class="script-ai-code-note">AI 已自动应用修改，按 Ctrl+Z 可撤销。</p>
                    <p v-if="message.scriptError" class="script-ai-code-error">{{ message.scriptError }}</p>
                  </div>
                </div>
              </div>
              <div v-if="aiTools.length" class="script-ai-tool-log">
                <div class="script-ai-tool-caption">
                  <n-icon :component="BuildOutline" />
                  工具调用
                </div>
                <div v-for="tool in aiTools" :key="tool.id" class="script-ai-tool-item" :class="tool.status">
                  <span class="script-ai-tool-dot" />
                  <span class="script-ai-tool-label">{{ tool.label }}</span>
                  <span v-if="tool.detail" class="script-ai-tool-detail">{{ tool.detail }}</span>
                </div>
              </div>
              <div v-if="aiBusy" class="script-ai-status">
                <n-spin :size="12" />
                <span>{{ aiStatus || 'AI 正在调用工具…' }}</span>
              </div>
            </div>
            <div class="script-ai-composer">
              <div class="script-ai-composer-box">
                <n-input
                  v-model:value="aiInput"
                  type="textarea"
                  :autosize="{ minRows: 2, maxRows: 4 }"
                  :disabled="aiBusy"
                  placeholder="描述脚本需求，Enter 发送，Shift+Enter 换行"
                  :input-props="{ onKeydown: handleAiKeydown }"
                />
                <div class="script-ai-composer-foot">
                  <n-select
                    v-model:value="aiProviderId"
                    size="tiny"
                    :options="aiProviderOptions"
                    :disabled="aiBusy || !aiProviderOptions.length"
                    placeholder="选择通用大模型"
                    class="script-ai-model-select"
                  />
                  <div class="script-ai-composer-actions">
                    <div class="script-ai-reasoning" title="模型推理难度：自动由供应商决定，快速/标准/深度逐级增加推理投入">
                      <span class="script-ai-reasoning-label">推理难度</span>
                      <n-select
                        v-model:value="aiReasoning"
                        size="tiny"
                        :options="aiReasoningOptions"
                        :disabled="aiBusy"
                        :consistent-menu-width="false"
                        :show-checkmark="false"
                        :menu-props="{ style: { minWidth: '104px' } }"
                        class="script-ai-reasoning-select"
                        aria-label="模型推理难度"
                      />
                    </div>
                    <n-button size="tiny" type="primary" :loading="aiBusy" :disabled="!aiInput.trim() || !aiProviderId" @click="sendAi">
                      <template #icon><n-icon :component="SendOutline" /></template>
                      发送
                    </n-button>
                  </div>
                </div>
              </div>
              <div v-if="aiError" class="script-ai-error-row">
                <span class="script-ai-error">{{ aiError }}</span>
                <n-button v-if="aiLastPrompt && !aiBusy" size="tiny" secondary @click="retryAi">重试</n-button>
              </div>
            </div>
          </aside>
        </Transition>

        <Transition name="script-ai-fade">
          <button
            v-if="!aiOpen"
            type="button"
            class="script-ai-tab"
            aria-label="打开 AI 脚本助手"
            @click="openAi"
          >
            <n-icon :component="SparklesOutline" />
            <span>AI 脚本助手</span>
          </button>
        </Transition>
      </div>

      <div class="script-editor-foot">
        <span class="script-editor-foot-note">
          <n-icon :component="TerminalOutline" />
          脚本执行日志可在审核配置的「脚本日志」中查看
        </span>
        <span class="script-editor-foot-meta">JS · 受限沙箱</span>
      </div>
    </div>
  </n-modal>

  <n-modal v-model:show="apiShow" :mask-closable="true">
    <div class="script-api-modal" :class="{ 'script-theme-dark': isDark }">
      <div class="script-api-head">
        <div>
          <div class="script-api-kicker">SCRIPT API REFERENCE</div>
          <h3>脚本 API 文档</h3>
          <p>脚本运行在受限沙箱中，支持数据库、HTTP 和加密编码；不支持文件读写、socket、eval 与定时器。</p>
        </div>
        <n-button circle quaternary @click="apiShow = false">
          <template #icon><n-icon :component="CloseOutline" /></template>
        </n-button>
      </div>
      <div class="script-api-body">
        <section v-for="group in apiDocGroups" :key="group.title" class="script-api-group">
          <h4>{{ group.title }}</h4>
          <div v-for="item in group.items" :key="item.name" class="script-api-item">
            <code>{{ item.name }}</code>
            <p>{{ item.detail }}</p>
            <span v-if="item.fields">{{ item.fields }}</span>
          </div>
        </section>
        <section class="script-api-example">
          <h4>完整示例</h4>
          <pre>{{ apiExample }}</pre>
        </section>
      </div>
      <div class="script-api-foot">
        <span>具体限制值以项目设置的「JS 脚本动作限制」为准。</span>
        <n-button size="small" type="primary" @click="apiShow = false">知道了</n-button>
      </div>
    </div>
  </n-modal>

  <n-modal v-model:show="closeConfirmShow" :mask-closable="false">
    <div class="script-confirm-modal" :class="{ 'script-theme-dark': isDark }">
      <div class="script-confirm-body">
        <span class="script-confirm-icon" :class="{ danger: aiBusy }">
          <n-icon :component="aiBusy ? AlertCircleOutline : WarningOutline" />
        </span>
        <div>
          <h3>{{ closeConfirmTitle }}</h3>
          <p>{{ closeConfirmMessage }}</p>
        </div>
      </div>
      <div class="script-confirm-actions">
        <n-button @click="closeConfirmShow = false">取消</n-button>
        <n-button v-if="dirty" type="error" secondary @click="discardAndClose">放弃修改并关闭</n-button>
        <n-button type="primary" @click="saveAndClose">{{ closePrimaryText }}</n-button>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.script-editor-modal {
  display: flex;
  width: 100vw;
  height: 100vh;
  flex-direction: column;
  overflow: hidden;
  background: #0f172a;
  color: #e2e8f0;
}

.script-editor-head {
  display: flex;
  min-height: 68px;
  align-items: center;
  gap: 18px;
  padding: 10px 16px 10px 18px;
  border-bottom: 1px solid #24324a;
  background: linear-gradient(180deg, #142036, #111c30);
}

.script-editor-title {
  display: flex;
  min-width: 300px;
  align-items: center;
  gap: 10px;
}

.script-editor-mark {
  display: flex;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: 1px solid #29405f;
  border-radius: 10px;
  background: #0d1829;
  color: #8fb4ec;
}

.script-editor-mark :deep(svg) {
  width: 18px;
  height: 18px;
}

.script-editor-heading {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.script-editor-kicker {
  color: #7ea2d6;
  font-family: var(--mono);
  font-size: 8px;
  font-weight: 650;
  letter-spacing: 0.14em;
}

.script-editor-heading-row {
  display: flex;
  min-width: 0;
  align-items: baseline;
  gap: 10px;
}

.script-editor-heading-row strong {
  flex-shrink: 0;
  color: #f8fafc;
  font-size: 15px;
  font-weight: 650;
}

.script-editor-heading-row span {
  overflow: hidden;
  color: #8fa4c4;
  font-size: 9.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.script-editor-remark {
  display: flex;
  width: min(300px, 26vw);
  flex: 0 1 300px;
  align-items: center;
}

.script-editor-remark :deep(.n-input) {
  --n-color: rgba(255, 255, 255, 0.04) !important;
  --n-color-focus: rgba(255, 255, 255, 0.08) !important;
  --n-text-color: #e2e8f0 !important;
  --n-placeholder-color: #8fa4c4 !important;
  --n-border: 1px solid #24324a !important;
  --n-border-hover: 1px solid #3d557a !important;
  --n-border-focus: 1px solid #52709c !important;
  --n-box-shadow-focus: none !important;
}

.script-editor-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 7px;
  margin-left: auto;
}

.script-editor-actions :deep(.n-button:not(.n-button--primary-type)) {
  --n-text-color: #c5d3e8 !important;
  --n-text-color-hover: #fff !important;
  --n-text-color-pressed: #fff !important;
  --n-text-color-focus: #fff !important;
  --n-color: rgba(255, 255, 255, 0.04) !important;
  --n-color-hover: rgba(255, 255, 255, 0.1) !important;
  --n-color-pressed: rgba(255, 255, 255, 0.14) !important;
  --n-color-focus: rgba(255, 255, 255, 0.1) !important;
}

.script-editor-divider {
  width: 1px;
  height: 20px;
  margin: 0 2px;
  background: #2a3a55;
}

.script-editor-body {
  position: relative;
  display: flex;
  min-height: 0;
  flex: 1;
}

.script-editor-host {
  min-width: 0;
  flex: 1;
}

.script-editor-host :deep(.cm-editor) {
  height: 100%;
  font-size: 13px;
}

.script-editor-host :deep(.cm-scroller) {
  overflow: auto;
  font-family: var(--mono);
}

.script-ai-panel {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 20;
  display: flex;
  width: min(410px, 92vw);
  flex-direction: column;
  border-left: 1px solid #2b3f5f;
  background: linear-gradient(180deg, #0c1627, #0a1220);
  box-shadow: -24px 0 50px rgba(2, 8, 23, 0.55);
}

.script-ai-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid #1f2d44;
}

.script-ai-title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.script-ai-title-icon {
  display: flex;
  width: 27px;
  height: 27px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: 1px solid #2f4f7f;
  border-radius: 8px;
  background: linear-gradient(145deg, #1c3f6d, #12294a);
  color: #bfdbfe;
}

.script-ai-title-icon :deep(svg) {
  width: 15px;
  height: 15px;
}

.script-ai-title > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 1px;
}

.script-ai-title small {
  overflow: hidden;
  color: #7d8faa;
  font-size: 8.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.script-ai-tab {
  position: absolute;
  top: 50%;
  right: 0;
  z-index: 18;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 9px;
  transform: translateY(-50%);
  border: 1px solid #33517a;
  border-right: 0;
  border-radius: 12px 0 0 12px;
  background: linear-gradient(180deg, #1b3f6e, #102744);
  box-shadow: -10px 12px 30px rgba(2, 8, 23, 0.45), inset 0 0 0 1px rgba(147, 197, 253, 0.08);
  color: #dbeafe;
  cursor: pointer;
  transition: background 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}

.script-ai-tab:hover {
  transform: translateY(-50%) translateX(-3px);
  background: linear-gradient(180deg, #24528c, #15325a);
  box-shadow: -12px 16px 36px rgba(2, 8, 23, 0.55), 0 0 24px rgba(96, 165, 250, 0.18);
}

.script-ai-tab:focus-visible {
  outline: 2px solid #93c5fd;
  outline-offset: 2px;
}

.script-ai-tab :deep(svg) {
  width: 17px;
  height: 17px;
  flex-shrink: 0;
}

.script-ai-tab span {
  font-size: 10.5px;
  font-weight: 650;
  letter-spacing: 0.16em;
  writing-mode: vertical-rl;
}

.script-ai-slide-enter-active,
.script-ai-slide-leave-active {
  transition: transform 0.24s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.18s ease;
}

.script-ai-slide-enter-from,
.script-ai-slide-leave-to {
  transform: translateX(100%);
  opacity: 0;
}

.script-ai-fade-enter-active,
.script-ai-fade-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.script-ai-fade-enter-from,
.script-ai-fade-leave-to {
  transform: translateY(-50%) translateX(8px);
  opacity: 0;
}

.script-ai-head strong {
  color: #f8fafc;
  font-size: 12px;
}

.script-ai-head :deep(.n-button) {
  --n-text-color: #94a3b8 !important;
  --n-text-color-hover: #fff !important;
  --n-text-color-focus: #fff !important;
  --n-text-color-pressed: #fff !important;
}

.script-ai-warning {
  margin: 8px 10px 0;
  padding: 7px 9px;
  border: 1px solid #3b2f1f;
  border-radius: 7px;
  background: #1d1811;
  color: #fbbf24;
  font-size: 9.5px;
  line-height: 1.6;
}

.script-ai-warning code {
  color: #fde68a;
  font-family: var(--mono);
}

.script-ai-warning kbd {
  padding: 0 3px;
  border: 1px solid currentColor;
  border-radius: 3px;
  font-family: var(--mono);
  font-size: 8.5px;
  opacity: 0.85;
}

.script-ai-messages {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 10px;
  overflow-y: auto;
  padding: 10px;
}

.script-ai-empty {
  color: #64748b;
  font-size: 10px;
  line-height: 1.7;
}

.script-ai-empty p {
  margin: 0 0 9px;
}

.script-ai-suggestions {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.script-ai-suggestions button {
  padding: 7px 9px;
  border: 1px solid #243a5c;
  border-radius: 8px;
  background: #0f1d33;
  color: #a9bfdc;
  cursor: pointer;
  font-size: 9.5px;
  line-height: 1.4;
  text-align: left;
  transition: border-color 0.16s ease, background 0.16s ease, color 0.16s ease;
}

.script-ai-suggestions button:hover {
  border-color: #3b5f91;
  background: #13253f;
  color: #dbeafe;
}

.script-ai-message {
  display: flex;
  align-items: flex-start;
  gap: 7px;
}

.script-ai-message.user {
  flex-direction: row-reverse;
}

.script-ai-avatar {
  display: flex;
  width: 22px;
  height: 22px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  background: #1e3a5f;
  color: #bfdbfe;
  font-size: 8px;
}

.script-ai-message.user .script-ai-avatar {
  background: #14532d;
  color: #bbf7d0;
}

.script-ai-bubble {
  max-width: 86%;
  padding: 8px 10px;
  border: 1px solid #1f2d44;
  border-radius: 4px 10px 10px;
  background: #111c30;
  color: #cbd5e1;
  font-size: 10.5px;
  line-height: 1.7;
}

.script-ai-message.user .script-ai-bubble {
  border-color: #1e3a5f;
  border-radius: 10px 4px 10px 10px;
  background: #10233d;
}

.script-ai-markdown :deep(p) {
  margin: 0 0 5px;
}

.script-ai-markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.script-ai-markdown :deep(code) {
  padding: 1px 4px;
  border-radius: 4px;
  background: #17243a;
  color: #e2e8f0;
  font-family: var(--mono);
  font-size: 9.5px;
}

.script-ai-code {
  margin-top: 7px;
  border: 1px solid #1f2d44;
  border-radius: 7px;
  background: #0b1425;
}

.script-ai-code-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 8px;
  color: #94a3b8;
  font-size: 9px;
}

.script-ai-code-head .script-ai-ok,
.script-ai-code-head .script-ai-error {
  padding: 0;
  font-size: 9px;
}

.script-ai-code-note {
  margin: 0;
  padding: 0 8px 7px;
  color: #7c8ba1;
  font-size: 9px;
  line-height: 1.5;
}

.script-ai-code-error {
  margin: 0;
  padding: 0 8px 7px;
  color: #f87171;
  font-family: var(--mono);
  font-size: 9px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.script-ai-tool-log {
  margin-top: 3px;
  padding: 7px 8px;
  border: 1px dashed #2b3f5f;
  border-radius: 8px;
  background: rgba(43, 108, 246, 0.05);
}

.script-ai-tool-caption {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 4px;
  color: #94a3b8;
  font-size: 9px;
}

.script-ai-tool-caption :deep(svg) {
  width: 11px;
  height: 11px;
}

.script-ai-tool-item {
  display: flex;
  align-items: baseline;
  gap: 6px;
  color: #cbd5e1;
  font-size: 9px;
  line-height: 1.55;
}

.script-ai-tool-dot {
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  margin-top: 4px;
  border-radius: 50%;
  background: #4ade80;
}

.script-ai-tool-item.running .script-ai-tool-dot {
  background: #fbbf24;
  animation: ai-dot-pulse 1s ease-in-out infinite;
}

.script-ai-tool-item.error .script-ai-tool-dot {
  background: #f87171;
}

.script-ai-tool-label {
  flex-shrink: 0;
  font-weight: 600;
}

.script-ai-tool-detail {
  overflow: hidden;
  color: #7c8ba1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.script-ai-error {
  display: block;
  padding: 4px 7px;
  color: #f87171;
  font-size: 9px;
}

.script-ai-ok {
  display: block;
  padding: 4px 7px;
  color: #4ade80;
  font-size: 9px;
}

.script-ai-status {
  display: flex;
  align-items: center;
  gap: 7px;
  color: #94a3b8;
  font-size: 10px;
}

.script-ai-composer {
  padding: 10px;
  border-top: 1px solid #1f2d44;
}

.script-ai-composer :deep(.n-input),
.script-ai-composer :deep(.n-select) {
  --n-color: #0b1425 !important;
  --n-color-focus: #0b1425 !important;
  --n-color-disabled: #0b1425 !important;
  --n-text-color: #e2e8f0 !important;
  --n-text-color-disabled: #64748b !important;
  --n-placeholder-color: #64748b !important;
  --n-border: 1px solid #24324a !important;
  --n-border-hover: 1px solid #3d557a !important;
  --n-border-focus: 1px solid #52709c !important;
  --n-border-disabled: 1px solid #1f2d44 !important;
  --n-box-shadow-focus: none !important;
}

.script-ai-composer-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 7px;
}

.script-ai-model-select {
  max-width: 160px;
  flex: 1;
  min-width: 0;
}

.script-ai-composer-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 6px;
}

.script-ai-reasoning {
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 1px 3px 1px 7px;
  border: 1px solid #24324a;
  border-radius: 7px;
  background: #0b1425;
}

.script-ai-reasoning-label {
  color: #7c8ba1;
  font-size: 10px;
  white-space: nowrap;
}

.script-ai-reasoning :deep(.n-select) {
  width: 68px;
}

.script-ai-error-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 7px;
  padding: 5px 5px 5px 9px;
  border: 1px solid rgba(248, 113, 113, 0.24);
  border-radius: 8px;
  background: rgba(248, 113, 113, 0.07);
}

.script-ai-error-row .script-ai-error {
  flex: 1;
  padding: 0;
  line-height: 1.45;
}

.script-api-modal {
  display: flex;
  width: min(900px, calc(100vw - 32px));
  max-height: calc(100vh - 40px);
  flex-direction: column;
  overflow: hidden;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.script-api-head {
  display: flex;
  flex-shrink: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px 16px;
  border-bottom: 1px solid #f0f3f8;
  background: linear-gradient(130deg, #f8fbff, #fff);
}

.script-api-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.script-api-head h3 {
  margin: 4px 0 3px;
  color: var(--ink);
  font-size: 19px;
  font-weight: 650;
  letter-spacing: -0.02em;
}

.script-api-head p {
  margin: 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.6;
}

.script-api-body {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 16px;
  overflow-y: auto;
  padding: 18px 24px;
}

.script-api-group,
.script-api-example {
  padding: 14px 15px;
  border: 1px solid #edf1f7;
  border-radius: 12px;
  background: #fbfcfe;
}

.script-api-group h4,
.script-api-example h4 {
  margin: 0 0 7px;
  color: #334155;
  font-size: 11px;
  font-weight: 650;
}

.script-api-item {
  padding: 7px 0;
  border-top: 1px dashed #e8eef7;
}

.script-api-item:first-of-type {
  padding-top: 0;
  border-top: 0;
}

.script-api-item code {
  display: inline-block;
  padding: 2px 5px;
  border-radius: 4px;
  background: #eef4ff;
  color: #1d4ed8;
  font-family: var(--mono);
  font-size: 9.5px;
  word-break: break-all;
}

.script-api-item p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.6;
}

.script-api-item span {
  display: block;
  margin-top: 3px;
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.8px;
  line-height: 1.6;
  word-break: break-all;
}

.script-api-example {
  grid-column: 1 / -1;
}

.script-api-example pre {
  max-height: 260px;
  margin: 0;
  overflow: auto;
  padding: 12px 13px;
  border-radius: 8px;
  background: #0f172a;
  color: #cbd5e1;
  font-family: var(--mono);
  font-size: 10px;
  line-height: 1.65;
  white-space: pre-wrap;
}

.script-api-foot {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 24px;
  border-top: 1px solid #f0f3f8;
  background: #fff;
}

.script-api-foot span {
  color: var(--muted-light);
  font-size: 9.5px;
}

.script-editor-modal.script-theme-light {
  background: var(--canvas);
  color: var(--text);
}

.script-theme-light .script-editor-head {
  border-bottom-color: var(--border);
  background: var(--card);
}

.script-theme-light .script-editor-mark {
  border-color: #c7d9fb;
  background: var(--brand-soft);
  color: var(--brand);
}

.script-theme-light .script-editor-kicker {
  color: var(--brand);
}

.script-theme-light .script-editor-heading-row strong {
  color: var(--ink);
}

.script-theme-light .script-editor-heading-row span {
  color: var(--muted);
}

.script-theme-light .script-editor-remark :deep(.n-input) {
  --n-color: #f8fafc !important;
  --n-color-focus: #fff !important;
  --n-text-color: var(--ink) !important;
  --n-placeholder-color: var(--muted-light) !important;
  --n-border: 1px solid var(--border) !important;
  --n-border-hover: 1px solid #a9c1fa !important;
  --n-border-focus: 1px solid var(--brand) !important;
}

.script-theme-light .script-editor-actions :deep(.n-button:not(.n-button--primary-type)) {
  --n-text-color: var(--text) !important;
  --n-text-color-hover: var(--ink) !important;
  --n-text-color-pressed: var(--ink) !important;
  --n-text-color-focus: var(--ink) !important;
  --n-color: #f8fafc !important;
  --n-color-hover: #eef3fb !important;
  --n-color-pressed: #e8eef9 !important;
  --n-color-focus: #eef3fb !important;
}

.script-theme-light .script-editor-divider {
  background: var(--border);
}

.script-theme-light .script-editor-host {
  background: #fff;
}

.script-theme-light .script-ai-panel {
  border-left-color: var(--border);
  background: var(--card);
  box-shadow: -20px 0 46px rgba(15, 23, 42, 0.12);
}

.script-theme-light .script-ai-head {
  border-bottom-color: var(--border);
}

.script-theme-light .script-ai-head strong {
  color: var(--ink);
}

.script-theme-light .script-ai-head :deep(.n-button) {
  --n-text-color: var(--muted) !important;
  --n-text-color-hover: var(--ink) !important;
  --n-text-color-focus: var(--ink) !important;
  --n-text-color-pressed: var(--ink) !important;
}

.script-theme-light .script-ai-title-icon {
  border-color: #c7d9fb;
  background: var(--brand-soft);
  color: var(--brand);
}

.script-theme-light .script-ai-title small {
  color: var(--muted-light);
}

.script-theme-light .script-ai-tab {
  border-color: var(--border-strong);
  background: var(--card);
  box-shadow: -8px 10px 24px rgba(15, 23, 42, 0.12), inset 3px 0 0 var(--brand);
  color: var(--brand);
}

.script-theme-light .script-ai-tab:hover {
  border-color: #c7d9fb;
  background: var(--brand-soft);
  box-shadow: -10px 14px 28px rgba(15, 23, 42, 0.16), inset 3px 0 0 var(--brand-strong);
  color: var(--brand-strong);
}

.script-theme-light .script-ai-tab:focus-visible {
  outline-color: var(--brand);
}

.script-theme-light .script-ai-warning {
  border-color: #fde68a;
  background: var(--amber-soft);
  color: #b45309;
}

.script-theme-light .script-ai-warning code {
  color: #92400e;
}

.script-theme-light .script-ai-empty {
  color: var(--muted);
}

.script-theme-light .script-ai-suggestions button {
  border-color: var(--border);
  background: #f8fafc;
  color: var(--text);
}

.script-theme-light .script-ai-suggestions button:hover {
  border-color: #c7d9fb;
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.script-theme-light .script-ai-avatar {
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.script-theme-light .script-ai-message.user .script-ai-avatar {
  background: var(--green-soft);
  color: #15803d;
}

.script-theme-light .script-ai-bubble {
  border-color: var(--border);
  background: #f8fafc;
  color: var(--text);
}

.script-theme-light .script-ai-message.user .script-ai-bubble {
  border-color: #c7d9fb;
  background: var(--brand-soft);
  color: #1e3a8a;
}

.script-theme-light .script-ai-markdown :deep(code) {
  background: #eef4ff;
  color: #1d4ed8;
}

.script-theme-light .script-ai-code {
  border-color: var(--border);
  background: #fff;
}

.script-theme-light .script-ai-code-head {
  border-bottom-color: var(--border);
  color: var(--muted);
}

.script-theme-light .script-ai-code-note {
  color: var(--muted);
}

.script-theme-light .script-ai-code-error {
  color: var(--red);
}

.script-theme-light .script-ai-error {
  color: var(--red);
}

.script-theme-light .script-ai-ok {
  color: var(--green);
}

.script-theme-light .script-ai-status {
  color: var(--muted);
}

.script-theme-light .script-ai-tool-log {
  border-color: var(--border);
  background: var(--brand-softer);
}

.script-theme-light .script-ai-tool-caption,
.script-theme-light .script-ai-tool-detail {
  color: var(--muted);
}

.script-theme-light .script-ai-tool-item {
  color: var(--text);
}

.script-theme-light .editor-ai-lock {
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.script-theme-light .script-ai-composer {
  border-top-color: var(--border);
}

.script-theme-light .script-ai-composer :deep(.n-input),
.script-theme-light .script-ai-composer :deep(.n-select) {
  --n-color: #fff !important;
  --n-color-focus: #fff !important;
  --n-color-disabled: #f8fafc !important;
  --n-text-color: var(--ink) !important;
  --n-text-color-disabled: var(--muted-light) !important;
  --n-placeholder-color: var(--muted-light) !important;
  --n-border: 1px solid #e3e8f1 !important;
  --n-border-hover: 1px solid #a9c1fa !important;
  --n-border-focus: 1px solid var(--brand) !important;
  --n-border-disabled: 1px solid #eef1f6 !important;
  --n-box-shadow-focus: none !important;
}

.script-api-modal.script-theme-dark {
  background: #0c1627;
  box-shadow: 0 30px 80px -20px rgba(2, 8, 23, 0.65);
}

.script-api-modal.script-theme-dark .script-api-head {
  border-bottom-color: #1f2d44;
  background: #0c1627;
}

.script-api-modal.script-theme-dark .script-api-head h3 {
  color: #f8fafc;
}

.script-api-modal.script-theme-dark .script-api-head p {
  color: #94a3b8;
}

.script-api-modal.script-theme-dark .script-api-group,
.script-api-modal.script-theme-dark .script-api-example {
  border-color: #1f2d44;
  background: #111c30;
}

.script-api-modal.script-theme-dark .script-api-group h4,
.script-api-modal.script-theme-dark .script-api-example h4 {
  color: #cbd5e1;
}

.script-api-modal.script-theme-dark .script-api-item {
  border-top-color: #1f2d44;
}

.script-api-modal.script-theme-dark .script-api-item code {
  background: #17243a;
  color: #bfdbfe;
}

.script-api-modal.script-theme-dark .script-api-item p {
  color: #94a3b8;
}

.script-api-modal.script-theme-dark .script-api-item span {
  color: #64748b;
}

.script-api-modal.script-theme-dark .script-api-example pre {
  background: #0b1425;
  color: #cbd5e1;
}

.script-api-modal.script-theme-dark .script-api-foot {
  border-top-color: #1f2d44;
  background: #0c1627;
}

.script-api-modal.script-theme-dark .script-api-foot span {
  color: #64748b;
}

.script-confirm-modal {
  width: min(470px, calc(100vw - 32px));
  padding: 20px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 24px 70px -18px rgba(15, 23, 42, 0.35);
}

.script-confirm-body {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.script-confirm-icon {
  display: flex;
  width: 38px;
  height: 38px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: 1px solid #fde68a;
  border-radius: 10px;
  background: var(--amber-soft);
  color: #d97706;
}

.script-confirm-icon.danger {
  border-color: #fecaca;
  background: var(--red-soft);
  color: var(--red);
}

.script-confirm-icon :deep(svg) {
  width: 19px;
  height: 19px;
}

.script-confirm-modal h3 {
  margin: 1px 0 0;
  color: var(--ink);
  font-size: 15px;
  font-weight: 650;
}

.script-confirm-modal p {
  margin: 6px 0 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.7;
}

.script-confirm-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 18px;
}

.script-confirm-modal.script-theme-dark {
  border-color: #24324a;
  background: #0c1627;
  box-shadow: 0 24px 70px -18px rgba(2, 8, 23, 0.65);
}

.script-confirm-modal.script-theme-dark h3 {
  color: #f8fafc;
}

.script-confirm-modal.script-theme-dark p {
  color: #94a3b8;
}

.script-confirm-modal.script-theme-dark .script-confirm-icon {
  border-color: #3b2f1f;
  background: #1d1811;
  color: #fbbf24;
}

.script-confirm-modal.script-theme-dark .script-confirm-icon.danger {
  border-color: #5f2626;
  background: #2a1414;
  color: #f87171;
}

@media (max-width: 900px) {
  .script-editor-head {
    flex-wrap: wrap;
  }

  .script-editor-title {
    flex: 1;
  }

  .script-editor-remark {
    width: auto;
    max-width: none;
    order: 3;
    flex-basis: 100%;
  }

  .script-ai-panel {
    width: min(410px, 88vw);
  }

  .script-api-body {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .script-editor-head {
    gap: 10px;
    padding: 10px 12px;
  }

  .script-editor-title {
    min-width: 0;
  }

  .script-editor-mark {
    display: none;
  }

  .script-editor-heading-row span {
    display: none;
  }

  .script-editor-actions {
    gap: 4px;
  }

  .script-ai-panel {
    width: 100%;
    border-left: 0;
  }

  .script-ai-tab {
    padding: 12px 8px;
  }

  .script-api-modal {
    width: calc(100vw - 16px);
    max-height: calc(100vh - 16px);
  }

  .script-api-head {
    padding: 16px 16px 13px;
  }

  .script-api-head h3 {
    font-size: 16px;
  }

  .script-api-body {
    padding: 14px 16px;
  }

  .script-api-foot {
    padding: 10px 16px;
  }

  .script-api-foot span {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .script-ai-tab,
  .script-ai-suggestions button,
  .script-ai-slide-enter-active,
  .script-ai-slide-leave-active,
  .script-ai-fade-enter-active,
  .script-ai-fade-leave-active {
    transition: none;
  }
}
</style>

<style scoped>
/* 2026 script editor refresh: clearer hierarchy, editor tab strip and polished AI panel. */
.script-editor-head {
  min-height: 64px;
  gap: 16px;
  padding: 9px 16px;
  border-bottom: 1px solid var(--border);
  background: var(--card);
}

.script-editor-title {
  min-width: 250px;
  gap: 11px;
}

.script-editor-mark {
  width: 36px;
  height: 36px;
  border: 1px solid var(--brand-border);
  border-radius: 10px;
  background: var(--brand-soft);
  color: var(--brand);
  box-shadow: var(--shadow-sm);
}

.script-editor-heading {
  gap: 3px;
}

.script-editor-heading-row {
  align-items: center;
  gap: 8px;
}

.script-editor-kicker {
  display: inline-flex;
  align-items: center;
  padding: 2px 6px;
  border: 1px solid var(--brand-border);
  border-radius: 4px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 8px;
  letter-spacing: 0.1em;
}

.script-editor-heading-row strong {
  color: var(--ink);
  font-size: 15px;
}

.script-editor-sub {
  display: flex;
  align-items: center;
  gap: 5px;
  overflow: hidden;
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.script-editor-sub code {
  padding: 1px 4px;
  border-radius: 4px;
  background: var(--surface-soft);
  color: var(--text);
  font-family: var(--mono);
  font-weight: 600;
}

.script-editor-sub-dot {
  width: 6px;
  height: 6px;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--green);
  box-shadow: 0 0 0 3px rgba(52, 211, 153, 0.12);
}

.script-editor-remark {
  width: min(320px, 26vw);
  flex: 0 1 320px;
}

.script-editor-modal .script-editor-remark :deep(.n-input) {
  --n-color: var(--surface-soft) !important;
  --n-color-focus: var(--card) !important;
  --n-text-color: var(--text) !important;
  --n-placeholder-color: var(--muted-light) !important;
  --n-border: 1px solid var(--border) !important;
  --n-border-hover: 1px solid var(--brand-border) !important;
  --n-border-focus: 1px solid var(--brand) !important;
  --n-box-shadow-focus: none !important;
}

.script-editor-modal .script-editor-remark :deep(.n-input__prefix) {
  color: var(--muted-light);
}

.script-editor-actions {
  gap: 6px;
}

.script-editor-modal .script-editor-actions :deep(.n-button:not(.n-button--primary-type)) {
  --n-text-color: var(--text) !important;
  --n-text-color-hover: var(--ink) !important;
  --n-text-color-pressed: var(--ink) !important;
  --n-text-color-focus: var(--ink) !important;
  --n-color: var(--card) !important;
  --n-color-hover: var(--brand-soft) !important;
  --n-color-pressed: var(--brand-soft) !important;
  --n-color-focus: var(--brand-soft) !important;
  --n-border: 1px solid var(--border) !important;
  --n-border-hover: 1px solid var(--brand-border) !important;
  --n-border-pressed: 1px solid var(--brand-border) !important;
  --n-border-focus: 1px solid var(--brand-border) !important;
}

.script-editor-modal .script-editor-actions :deep(.n-button.is-active) {
  --n-text-color: var(--brand-strong) !important;
  --n-text-color-hover: var(--brand-strong) !important;
  --n-text-color-pressed: var(--brand-strong) !important;
  --n-text-color-focus: var(--brand-strong) !important;
  --n-color: var(--brand-soft) !important;
  --n-color-hover: var(--brand-soft) !important;
  --n-color-pressed: var(--brand-soft) !important;
  --n-color-focus: var(--brand-soft) !important;
  --n-border: 1px solid var(--brand-border) !important;
  --n-border-hover: 1px solid var(--brand) !important;
  --n-border-pressed: 1px solid var(--brand) !important;
  --n-border-focus: 1px solid var(--brand) !important;
}

.script-editor-modal .script-editor-divider {
  background: var(--border);
}

.ai-state-dot {
  width: 5px;
  height: 5px;
  margin-left: 2px;
  border-radius: 50%;
  background: var(--brand);
}

.ai-state-dot.running {
  background: var(--green);
  animation: ai-dot-pulse 1.2s ease-in-out infinite;
}

@keyframes ai-dot-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.script-editor-toolbar {
  display: flex;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 0 12px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-soft);
}

.editor-file-tab {
  display: flex;
  height: 34px;
  align-items: center;
  gap: 6px;
  padding: 0 13px;
  border-top: 2px solid var(--brand);
  border-right: 1px solid var(--border);
  border-left: 1px solid var(--border);
  background: var(--card);
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9.5px;
}

.editor-modified-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--amber);
}

.editor-lang {
  color: #b45309;
  font-size: 9px;
  font-weight: 700;
}

.script-editor-modal:not(.script-theme-light) .editor-lang {
  color: var(--amber);
}

.editor-file-name {
  color: var(--ink);
}

.editor-file-meta {
  color: var(--muted-light);
}

.editor-toolbar-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.editor-sandbox {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border: 1px solid var(--green-border);
  border-radius: 999px;
  background: var(--green-soft);
  color: var(--green);
  font-size: 9px;
}

.editor-sandbox-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--green);
}

.editor-ai-lock {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--brand-soft);
  color: var(--brand);
  font-size: 9px;
}

.editor-ai-lock-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--brand);
  animation: ai-dot-pulse 1.2s ease-in-out infinite;
}

.editor-entry-hint {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.script-editor-foot {
  display: flex;
  min-height: 28px;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 14px;
  border-top: 1px solid var(--border);
  background: var(--card);
  color: var(--muted);
  font-size: 9.5px;
}

.script-editor-foot-note {
  display: flex;
  align-items: center;
  gap: 6px;
}

.script-editor-foot-note :deep(svg) {
  width: 12px;
  height: 12px;
  color: var(--brand);
}

.script-editor-foot-meta {
  color: var(--muted-light);
  font-family: var(--mono);
}

.script-editor-modal .script-ai-panel {
  width: min(420px, 92vw);
  border-left: 1px solid var(--border);
  background: var(--card);
  box-shadow: -18px 0 40px rgba(15, 23, 42, 0.14);
}

.script-editor-modal .script-ai-head {
  padding: 11px 13px;
  border-bottom: 1px solid var(--border);
  background: var(--card);
}

.script-editor-modal .script-ai-title-icon {
  border: 0;
  background: linear-gradient(135deg, var(--brand), var(--purple));
  color: #fff;
  box-shadow: 0 4px 12px rgba(43, 108, 246, 0.22);
}

.script-editor-modal .script-ai-head strong {
  color: var(--ink);
}

.script-editor-modal .script-ai-title small {
  color: var(--muted-light);
}

.script-ai-collapse {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 3px 7px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  font-size: 9.5px;
}

.script-ai-collapse:hover {
  background: var(--surface-hover);
  color: var(--ink);
}

.script-ai-collapse :deep(svg) {
  width: 12px;
  height: 12px;
}

.script-editor-modal .script-ai-warning {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin: 9px 11px 0;
  padding: 8px 9px;
  border: 1px solid var(--amber-border);
  border-radius: 8px;
  background: var(--amber-soft);
  color: #b45309;
  font-size: 9.5px;
}

.script-editor-modal:not(.script-theme-light) .script-ai-warning {
  color: var(--amber);
}

.script-ai-warning :deep(svg) {
  width: 13px;
  height: 13px;
  flex-shrink: 0;
  margin-top: 1px;
}

.script-editor-modal .script-ai-warning code {
  background: var(--amber-soft-2);
  color: #92400e;
}

.script-editor-modal:not(.script-theme-light) .script-ai-warning code {
  color: var(--amber);
}

.script-editor-modal .script-ai-messages {
  padding: 11px;
}

.script-ai-empty p {
  margin: 0;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.7;
}

.script-ai-starters {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 9px;
}

.script-ai-starter {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 9px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card);
  color: var(--text);
  cursor: pointer;
  font-size: 9.5px;
  text-align: left;
  transition: border-color 0.16s ease, background 0.16s ease, color 0.16s ease;
}

.script-ai-starter:hover {
  border-color: var(--brand-border);
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.script-ai-starter :deep(svg) {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
  color: var(--muted-light);
}

.script-ai-starter:hover :deep(svg) {
  color: var(--brand);
}

.script-editor-modal .script-ai-avatar {
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.script-editor-modal .script-ai-message.user .script-ai-avatar {
  background: var(--green-soft);
  color: var(--green);
}

.script-editor-modal .script-ai-bubble {
  border-color: var(--border);
  background: var(--surface-soft);
  color: var(--text);
}

.script-editor-modal .script-ai-message.user .script-ai-bubble {
  border-color: var(--brand-border);
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.script-editor-modal .script-ai-markdown :deep(code) {
  background: var(--surface-strong);
  color: var(--text);
}

.script-editor-modal .script-ai-code {
  border-color: var(--border);
  background: var(--card);
}

.script-editor-modal .script-ai-code-head {
  border-bottom-color: var(--border);
  color: var(--muted);
}

.script-editor-modal .script-ai-code-note {
  color: var(--muted);
}

.script-editor-modal .script-ai-code-error {
  color: var(--red);
}

.script-editor-modal .script-ai-error {
  color: var(--red);
}

.script-editor-modal .script-ai-ok {
  color: var(--green);
}

.script-editor-modal .script-ai-status {
  color: var(--muted);
}

.script-editor-modal .script-ai-tool-log {
  border-color: var(--border);
  background: var(--brand-softer);
}

.script-editor-modal .script-ai-tool-caption,
.script-editor-modal .script-ai-tool-detail {
  color: var(--muted);
}

.script-editor-modal .script-ai-tool-item {
  color: var(--text);
}

.script-editor-modal .script-ai-composer {
  padding: 11px;
  border-top: 1px solid var(--border);
  background: var(--card);
}

.script-ai-composer-box {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card);
  transition: border-color 0.16s ease, box-shadow 0.16s ease;
}

.script-ai-composer-box:focus-within {
  border-color: var(--brand);
  box-shadow: 0 0 0 3px rgba(43, 108, 246, 0.1);
}

.script-editor-modal .script-ai-composer-box :deep(.n-input) {
  --n-color: var(--card) !important;
  --n-color-focus: var(--card) !important;
  --n-text-color: var(--text) !important;
  --n-placeholder-color: var(--muted-light) !important;
  --n-border: 1px solid transparent !important;
  --n-border-hover: 1px solid transparent !important;
  --n-border-focus: 1px solid transparent !important;
  --n-box-shadow-focus: none !important;
}

.script-ai-composer-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 8px;
  border-top: 1px solid var(--border);
  background: var(--surface-soft);
}

.script-editor-modal .script-ai-composer-foot :deep(.n-select) {
  --n-color: var(--card) !important;
  --n-color-active: var(--card) !important;
  --n-color-focus: var(--card) !important;
  --n-text-color: var(--muted) !important;
  --n-border: 1px solid transparent !important;
  --n-border-hover: 1px solid var(--border) !important;
  --n-border-active: 1px solid var(--border) !important;
  --n-box-shadow-active: none !important;
  --n-box-shadow-focus: none !important;
}

.script-editor-modal .script-ai-reasoning {
  border-color: var(--border);
  background: var(--card);
}

.script-editor-modal .script-ai-reasoning-label {
  color: var(--muted);
}

.script-editor-modal .script-ai-error-row {
  border-color: rgba(220, 38, 38, 0.16);
  background: rgba(220, 38, 38, 0.05);
}

.script-editor-modal .script-ai-tab {
  border-color: var(--border-strong);
  background: var(--card);
  box-shadow: -8px 10px 24px rgba(15, 23, 42, 0.12), inset 3px 0 0 var(--brand);
  color: var(--brand);
}

.script-editor-modal .script-ai-tab:hover {
  border-color: var(--brand-border);
  background: var(--brand-soft);
  box-shadow: -10px 14px 28px rgba(15, 23, 42, 0.16), inset 3px 0 0 var(--brand-strong);
  color: var(--brand-strong);
}

@media (max-width: 900px) {
  .script-editor-toolbar {
    padding: 0 10px;
  }

  .editor-entry-hint {
    display: none;
  }
}

@media (max-width: 640px) {
  .script-editor-sub {
    display: none;
  }

  .script-editor-toolbar {
    height: 32px;
  }

  .editor-file-tab {
    height: 32px;
  }

  .script-editor-foot-meta {
    display: none;
  }

  .script-editor-modal .script-ai-panel {
    width: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ai-state-dot.running {
    animation: none;
  }
}
</style>
