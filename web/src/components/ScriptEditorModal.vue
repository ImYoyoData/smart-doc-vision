<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NModal, NSelect, NSpin, NTooltip } from 'naive-ui'
import { EditorView, basicSetup } from 'codemirror'
import { javascript, javascriptLanguage } from '@codemirror/lang-javascript'
import { autocompletion, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { oneDark } from '@codemirror/theme-one-dark'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import {
  CloseOutline,
  CodeSlashOutline,
  DocumentTextOutline,
  SparklesOutline,
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
const editorHost = ref<HTMLElement | null>(null)
let editorView: EditorView | null = null
let syncing = false

interface AiMessage {
  role: 'user' | 'assistant'
  content: string
  script?: string
  scriptValid?: boolean
  scriptError?: string
}

const aiOpen = ref(false)
const aiMessages = ref<AiMessage[]>([])
const aiInput = ref('')
const aiBusy = ref(false)
const aiError = ref('')
const aiStatus = ref('')
const aiProviderId = ref<number | null>(null)
const aiProviders = ref<Provider[]>([])
const apiShow = ref(false)
const aiSuggestions = [
  '审核通过后把结果写回业务数据库',
  '审核不通过时调用 HTTP 接口通知业务方',
  '把最终审核结果推送到 Redis Stream',
]

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

const apiExample = `function main(ctx) {
  const db = ctx.mysql.connect({
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
  return { matched: rows.length, result: ctx.result }
}`

const aiProviderOptions = computed(() =>
  aiProviders.value.map((provider) => ({
    label: `${provider.name} · ${provider.model}`,
    value: provider.id,
  })),
)

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
      oneDark,
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

watch(() => props.script, (value) => setEditorValue(value))
watch(() => props.show, (value) => {
  if (value) {
    nextTick(() => {
      createEditor()
      setEditorValue(props.script)
      editorView?.focus()
    })
  } else {
    apiShow.value = false
  }
})

onMounted(() => {
  createEditor()
})

onBeforeUnmount(() => {
  editorView?.destroy()
  editorView = null
})

function close() {
  apiShow.value = false
  emit('update:show', false)
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

async function loadProviders() {
  if (!workspace.currentProjectId || aiProviders.value.length) return
  try {
    const data = await providerApi.list({ project_id: workspace.currentProjectId })
    aiProviders.value = data.items.filter((provider) => provider.status === 'ACTIVE' && provider.kind !== 'JEV')
    if (!aiProviderId.value || !aiProviders.value.some((provider) => provider.id === aiProviderId.value)) {
      aiProviderId.value = aiProviders.value[0]?.id || null
    }
  } catch (error) {
    aiError.value = error instanceof Error ? error.message : '加载通用大模型失败'
  }
}

async function openAi() {
  aiOpen.value = true
  await loadProviders()
}

async function sendAi() {
  const text = aiInput.value.trim()
  if (!text || aiBusy.value) return
  if (!aiProviderId.value) {
    aiError.value = '当前项目没有启用的通用大模型'
    return
  }
  aiInput.value = ''
  aiError.value = ''
  aiMessages.value.push({ role: 'user', content: text })
  aiBusy.value = true
  aiStatus.value = 'AI 正在生成脚本…'
  let payload: Record<string, any> | null = null
  try {
    await scriptApi.assistantStream({
      project_id: workspace.currentProjectId,
      provider_id: aiProviderId.value,
      audit_id: props.auditId || 0,
      current_script: props.script,
      messages: aiMessages.value.map((message) => ({ role: message.role, content: message.content })),
    }, {
      onEvent: (event, data) => {
        const value = data as Record<string, any>
        if (event === 'step' && value?.label) {
          aiStatus.value = String(value.label)
        } else if (event === 'reply') {
          payload = value
        } else if (event === 'error') {
          throw new Error(String(value?.message || '脚本助手请求失败'))
        }
      },
    })
    const result = payload as Record<string, any> | null
    if (!result) throw new Error('模型没有返回结果')
    aiMessages.value.push({
      role: 'assistant',
      content: String(result.reply || '已生成脚本'),
      script: String(result.script || ''),
      scriptValid: Boolean(result.script_valid),
      scriptError: String(result.script_error || ''),
    })
  } catch (error) {
    aiError.value = error instanceof Error ? error.message : '脚本助手请求失败'
  } finally {
    aiBusy.value = false
    aiStatus.value = ''
  }
}

function applyAiScript(message: AiMessage) {
  if (!message.script) return
  emit('update:script', message.script)
  setEditorValue(message.script)
}

function handleAiKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault()
  void sendAi()
}
</script>

<template>
  <n-modal :show="show" :mask-closable="false" @update:show="(value: boolean) => { if (!value) close() }">
    <div class="script-editor-modal">
      <div class="script-editor-head">
        <div class="script-editor-title">
          <span class="script-editor-mark">
            <n-icon :component="CodeSlashOutline" />
          </span>
          <div class="script-editor-heading">
            <div class="script-editor-kicker">JS RESULT ACTION</div>
            <div class="script-editor-heading-row">
              <strong>编辑脚本</strong>
              <span>入口 function main(ctx) · 结束自动释放连接</span>
            </div>
          </div>
        </div>
        <div class="script-editor-remark">
          <span>备注</span>
          <n-input
            :value="remark"
            size="small"
            maxlength="128"
            placeholder="例如 写回账单系统"
            @update:value="(value: string) => emit('update:remark', value)"
          />
        </div>
        <div class="script-editor-actions">
          <n-button size="small" quaternary @click="apiShow = true">
            <template #icon><n-icon :component="DocumentTextOutline" /></template>
            API 文档
          </n-button>
          <n-button
            size="small"
            :type="aiOpen ? 'primary' : 'default'"
            :secondary="!aiOpen"
            @click="toggleAi"
          >
            <template #icon><n-icon :component="SparklesOutline" /></template>
            AI 脚本助手
          </n-button>
          <n-button size="small" type="primary" @click="close">保存脚本</n-button>
          <span class="script-editor-divider" />
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button circle quaternary @click="close">
                <template #icon><n-icon :component="CloseOutline" /></template>
              </n-button>
            </template>
            关闭
          </n-tooltip>
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
                  <small>读取当前脚本 · 生成后手动应用</small>
                </div>
              </div>
              <n-button quaternary size="tiny" @click="aiOpen = false">收起</n-button>
            </div>
            <div class="script-ai-warning">
              AI 会读取当前脚本。请使用 <code>ctx.secret()</code>，不要在脚本里写明文密码。
            </div>
            <div class="script-ai-messages">
              <div v-if="!aiMessages.length" class="script-ai-empty">
                <p>描述你想要的脚本，AI 会结合当前审核配置生成实现。</p>
                <div class="script-ai-suggestions">
                  <button
                    v-for="suggestion in aiSuggestions"
                    :key="suggestion"
                    type="button"
                    @click="aiInput = suggestion"
                  >
                    {{ suggestion }}
                  </button>
                </div>
              </div>
              <div v-for="(message, index) in aiMessages" :key="index" class="script-ai-message" :class="message.role">
                <span class="script-ai-avatar">{{ message.role === 'user' ? '我' : 'AI' }}</span>
                <div class="script-ai-bubble">
                  <div class="script-ai-markdown" v-html="renderMarkdown(message.content)" />
                  <div v-if="message.script" class="script-ai-code">
                    <div class="script-ai-code-head">
                      <span>生成脚本</span>
                      <n-button size="tiny" type="primary" secondary @click="applyAiScript(message)">应用到编辑器</n-button>
                    </div>
                    <pre>{{ message.script.slice(0, 1200) }}{{ message.script.length > 1200 ? '\n…' : '' }}</pre>
                    <span v-if="message.scriptError" class="script-ai-error">语法错误：{{ message.scriptError }}</span>
                    <span v-else-if="message.scriptValid" class="script-ai-ok">语法检查通过</span>
                  </div>
                </div>
              </div>
              <div v-if="aiBusy" class="script-ai-status">
                <n-spin :size="12" />
                <span>{{ aiStatus || 'AI 正在生成脚本…' }}</span>
              </div>
            </div>
            <div class="script-ai-composer">
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
                  style="max-width: 220px"
                />
                <n-button size="tiny" type="primary" :loading="aiBusy" :disabled="!aiInput.trim() || !aiProviderId" @click="sendAi">
                  发送
                </n-button>
              </div>
              <div v-if="aiError" class="script-ai-error">{{ aiError }}</div>
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
    </div>
  </n-modal>

  <n-modal v-model:show="apiShow" :mask-closable="true">
    <div class="script-api-modal">
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
  max-width: 400px;
  flex: 1;
  align-items: center;
  gap: 8px;
}

.script-editor-remark > span {
  flex-shrink: 0;
  color: #8fa4c4;
  font-size: 9.5px;
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
  padding: 5px 7px;
  border-bottom: 1px solid #1f2d44;
  color: #94a3b8;
  font-size: 9px;
}

.script-ai-code pre {
  max-height: 180px;
  margin: 0;
  overflow: auto;
  padding: 7px;
  color: #cbd5e1;
  font-family: var(--mono);
  font-size: 9.5px;
  line-height: 1.55;
  white-space: pre-wrap;
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

@media (max-width: 900px) {
  .script-editor-head {
    flex-wrap: wrap;
  }

  .script-editor-title {
    flex: 1;
  }

  .script-editor-remark {
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
