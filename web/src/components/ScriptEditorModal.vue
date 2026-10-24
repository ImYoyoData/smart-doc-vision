<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NInput, NModal, NSelect, NSpin } from 'naive-ui'
import { EditorView, basicSetup } from 'codemirror'
import { javascript, javascriptLanguage } from '@codemirror/lang-javascript'
import { autocompletion, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { oneDark } from '@codemirror/theme-one-dark'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
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
  { label: 'ctx.crypto.sha256(value)', type: 'method', detail: 'SHA-256 十六进制' },
  { label: 'ctx.crypto.hmacSha256(key, value)', type: 'method', detail: 'HMAC-SHA256' },
  { label: 'ctx.crypto.randomBytes(size)', type: 'method', detail: '安全随机字节' },
  { label: 'ctx.crypto.uuid()', type: 'method', detail: '随机 UUID' },
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
      setEditorValue(props.script)
      editorView?.focus()
    })
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
  emit('update:show', false)
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
          <div class="script-editor-kicker">JS RESULT ACTION</div>
          <strong>编辑脚本</strong>
          <span>入口 function main(ctx)，脚本结束自动关闭数据库连接</span>
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
          <n-button size="small" secondary @click="openAi">AI 助手</n-button>
          <n-button size="small" @click="close">取消</n-button>
          <n-button size="small" type="primary" @click="close">保存脚本</n-button>
        </div>
      </div>

      <div class="script-editor-body">
        <div ref="editorHost" class="script-editor-host" />

        <aside v-if="aiOpen" class="script-ai-panel">
          <div class="script-ai-head">
            <strong>AI 脚本助手</strong>
            <n-button quaternary size="tiny" @click="aiOpen = false">收起</n-button>
          </div>
          <div class="script-ai-warning">
            AI 会读取当前脚本。请使用 <code>ctx.secret()</code>，不要在脚本里写明文密码。
          </div>
          <div class="script-ai-messages">
            <div v-if="!aiMessages.length" class="script-ai-empty">
              描述你想要的脚本，例如“审核结果为 REJECT 时更新订单表并回调业务系统”。
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
  align-items: center;
  gap: 18px;
  padding: 12px 18px;
  border-bottom: 1px solid #24324a;
  background: #111c30;
}

.script-editor-title {
  display: flex;
  flex-direction: column;
  min-width: 240px;
  gap: 2px;
}

.script-editor-kicker {
  color: #7ea2d6;
  font-size: 9px;
  letter-spacing: 0.14em;
}

.script-editor-title strong {
  color: #f8fafc;
  font-size: 16px;
}

.script-editor-title span {
  color: #94a3b8;
  font-size: 10px;
}

.script-editor-remark {
  display: flex;
  max-width: 420px;
  flex: 1;
  align-items: center;
  gap: 8px;
}

.script-editor-remark > span {
  flex-shrink: 0;
  color: #94a3b8;
  font-size: 10px;
}

.script-editor-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.script-editor-body {
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
  display: flex;
  width: 390px;
  flex-shrink: 0;
  flex-direction: column;
  border-left: 1px solid #24324a;
  background: #0b1425;
}

.script-ai-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid #1f2d44;
}

.script-ai-head strong {
  color: #f8fafc;
  font-size: 12px;
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

.script-ai-composer-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 7px;
}

@media (max-width: 900px) {
  .script-editor-head {
    flex-wrap: wrap;
  }

  .script-editor-remark {
    max-width: none;
    order: 3;
    flex-basis: 100%;
  }

  .script-ai-panel {
    width: min(390px, 70vw);
  }
}
</style>
