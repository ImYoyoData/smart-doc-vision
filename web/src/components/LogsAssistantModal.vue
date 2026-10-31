<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { NButton, NInput, NModal, NSelect, NSpin, NTag } from 'naive-ui'
import {
  CloseOutline,
  SearchOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import VoiceInputButton from '@/components/VoiceInputButton.vue'
import { logApi, providerApi } from '@/api'
import type { LogsAssistantFilter, Provider } from '@/api/types'

type ChatRole = 'user' | 'assistant'

interface ChatMessage {
  role: ChatRole
  content: string
  filter?: LogsAssistantFilter
  total?: number
  tools?: string[]
}

const props = defineProps<{
  show: boolean
  projectId: number
  filters: LogsAssistantFilter
}>()

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
  (event: 'apply', filter: LogsAssistantFilter): void
}>()

const visible = computed({
  get: () => props.show,
  set: (value: boolean) => emit('update:show', value),
})

const messages = ref<ChatMessage[]>([])
const input = ref('')
const loading = ref(false)
const errorText = ref('')
const listRef = ref<HTMLElement | null>(null)
const providers = ref<Provider[]>([])
const providerId = ref<number | null>(0)
const loadingProviders = ref(false)
const reasoning = ref('')
const contextTokens = ref(0)
const contextWindow = ref(128000)
const compressed = ref(false)

const providerOptions = computed(() => [
  { label: '自动选择（按模型优先级）', value: 0 },
  ...providers.value.map((provider) => ({
    label: `${provider.name} · ${provider.model}`,
    value: provider.id,
  })),
])

const reasoningOptions = [
  { label: '自动', value: '' },
  { label: '低', value: 'low' },
  { label: '中', value: 'medium' },
  { label: '高', value: 'high' },
]

const contextPercent = computed(() => {
  if (!contextWindow.value) return 0
  return Math.min(100, Math.round((contextTokens.value / contextWindow.value) * 1000) / 10)
})

const contextTone = computed(() => {
  if (contextPercent.value >= 85) return 'danger'
  if (contextPercent.value >= 60) return 'warn'
  return 'ok'
})

const contextRingOffset = computed(() => {
  const circumference = 2 * Math.PI * 15
  return circumference * (1 - Math.min(1, contextTokens.value / contextWindow.value))
})

function formatTokens(value?: number) {
  if (!value || value <= 0) return '0'
  return value.toLocaleString('en-US')
}

const suggestions = [
  '最近 7 天登录失败的是谁，失败了多少次？',
  '统计最近 30 天每种操作类型的次数',
  '查一下 admin 今天的全部操作',
  '有没有任务失败或者开放 API 提交失败的日志？',
]

function renderMarkdown(content: string) {
  const html = marked.parse(content || '', { breaks: true, gfm: true })
  return DOMPurify.sanitize(typeof html === 'string' ? html : '')
}

async function loadProviders() {
  if (loadingProviders.value || !props.projectId) return
  loadingProviders.value = true
  try {
    const data = await providerApi.list({ project_id: props.projectId, page: 1, page_size: 100 })
    providers.value = (data.items || []).filter(
      (provider) =>
        provider.status === 'ACTIVE' &&
        (provider.kind === 'OPENAI_COMPATIBLE' || provider.kind === 'ANTHROPIC'),
    )
    if (providerId.value && !providers.value.some((provider) => provider.id === providerId.value)) {
      providerId.value = 0
    }
  } catch {
    providers.value = []
  } finally {
    loadingProviders.value = false
  }
}

watch(
  () => props.show,
  (value) => {
    if (value) void loadProviders()
  },
)

function scrollToBottom() {
  void nextTick(() => {
    const el = listRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

function close() {
  if (loading.value) return
  visible.value = false
}

function clearChat() {
  if (loading.value) return
  messages.value = []
  errorText.value = ''
  contextTokens.value = 0
  contextWindow.value = 128000
  compressed.value = false
}

function appendInput(text: string) {
  const current = input.value.trim()
  input.value = current ? `${current} ${text}` : text
}

function toolLabel(tool: string): string {
  if (tool === 'search_audit_logs') return '明细查询'
  if (tool === 'aggregate_audit_logs') return '统计分析'
  if (tool === 'get_audit_log_detail') return '查看详情'
  if (tool === 'apply_log_filters') return '筛选条件'
  return tool
}

function filterActive(filter?: LogsAssistantFilter): boolean {
  if (!filter) return false
  return Boolean(
    filter.operator ||
      filter.keyword ||
      filter.object_type ||
      filter.result ||
      filter.start ||
      filter.end ||
      (filter.actions && filter.actions.length),
  )
}

function applyFilter(filter?: LogsAssistantFilter) {
  if (!filterActive(filter)) return
  emit('apply', filter || {})
  visible.value = false
}

async function send(forced?: string) {
  if (loading.value) return
  const prompt = (forced ?? input.value).trim()
  if (!prompt) {
    errorText.value = '请输入要查询的内容'
    return
  }
  errorText.value = ''
  const history = messages.value.map((item) => ({ role: item.role, content: item.content }))
  messages.value.push({ role: 'user', content: prompt })
  input.value = ''
  loading.value = true
  scrollToBottom()
  try {
    const data = await logApi.assistant({
      project_id: props.projectId || 0,
      provider_id: providerId.value || 0,
      reasoning: reasoning.value,
      messages: [...history, { role: 'user', content: prompt }],
      filters: props.filters,
    })
    contextTokens.value = data.context_tokens || 0
    contextWindow.value = data.context_window || 128000
    compressed.value = data.compressed
    messages.value.push({
      role: 'assistant',
      content: data.reply,
      filter: data.filter,
      total: data.total,
      tools: data.tools,
    })
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '日志查询助手暂时不可用'
  } finally {
    loading.value = false
    scrollToBottom()
  }
}
</script>

<template>
  <n-modal v-model:show="visible" :mask-closable="!loading" @update:show="(value: boolean) => { if (!value) close() }">
    <div class="logs-ai-modal">
      <header class="logs-ai-head">
        <span class="logs-ai-icon"><SparklesOutline /></span>
        <div class="logs-ai-heading">
          <div class="logs-ai-kicker">AUDIT LOGS COPILOT</div>
          <h2>AI 查询助手</h2>
          <p>用自然语言查日志、圈定筛选条件或做数量与趋势分析；支持语音输入。</p>
        </div>
        <button class="logs-ai-close" type="button" title="关闭" aria-label="关闭" :disabled="loading" @click="close">
          <CloseOutline />
        </button>
      </header>

      <div ref="listRef" class="logs-ai-body">
        <div v-if="!messages.length" class="logs-ai-empty">
          <span class="logs-ai-empty-icon"><SearchOutline /></span>
          <strong>你可以这样问</strong>
          <p>助手会调用只读查询工具，不会修改或删除任何日志。</p>
          <div class="logs-ai-suggestions">
            <button v-for="item in suggestions" :key="item" type="button" @click="send(item)">
              {{ item }}
            </button>
          </div>
        </div>

        <div v-for="(item, index) in messages" :key="index" class="logs-ai-msg" :class="item.role">
          <span v-if="item.role === 'assistant'" class="logs-ai-avatar"><SparklesOutline /></span>
          <div class="logs-ai-bubble">
            <div v-if="item.role === 'assistant'" class="logs-ai-markdown" v-html="renderMarkdown(item.content)"></div>
            <p v-else class="logs-ai-text">{{ item.content }}</p>
            <div v-if="item.role === 'assistant' && (item.tools?.length || item.total !== undefined)" class="logs-ai-meta">
              <n-tag v-if="item.total !== undefined" size="small" :bordered="false">匹配 {{ item.total }} 条</n-tag>
              <n-tag v-for="tool in item.tools || []" :key="tool" size="small" :bordered="false">
                {{ toolLabel(tool) }}
              </n-tag>
            </div>
            <div v-if="item.role === 'assistant' && filterActive(item.filter)" class="logs-ai-apply">
              <n-button size="small" type="primary" secondary @click="applyFilter(item.filter)">
                应用到筛选表格
              </n-button>
              <span>应用后关闭助手并刷新列表</span>
            </div>
          </div>
        </div>

        <div v-if="loading" class="logs-ai-msg assistant">
          <span class="logs-ai-avatar"><SparklesOutline /></span>
          <div class="logs-ai-bubble logs-ai-loading">
            <n-spin size="small" />
            <span>正在查询日志并分析…</span>
          </div>
        </div>

        <div v-if="errorText" class="logs-ai-error">{{ errorText }}</div>
      </div>

      <footer class="logs-ai-foot">
        <div class="logs-ai-composer">
          <n-input
            v-model:value="input"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 5 }"
            placeholder="例如：最近 7 天登录失败的账号有哪些？"
            :disabled="loading"
            @keyup.enter.exact.prevent="send()"
          />
          <div class="logs-ai-composer-bar">
            <div class="logs-ai-model-row">
              <span class="logs-ai-model-label">模型</span>
              <n-select
                v-model:value="providerId"
                :options="providerOptions"
                :loading="loadingProviders"
                :placeholder="providerOptions.length ? '选择模型' : '当前项目暂无通用模型'"
                size="small"
                :disabled="loading || !providerOptions.length"
                placement="top-start"
                class="logs-ai-model-select"
              />
              <span class="logs-ai-model-label">思考</span>
              <n-select
                v-model:value="reasoning"
                :options="reasoningOptions"
                size="small"
                :disabled="loading"
                placement="top-start"
                class="logs-ai-reasoning-select"
              />
              <VoiceInputButton :disabled="loading" @transcript="appendInput" />
            </div>
            <div class="logs-ai-actions">
              <button
                type="button"
                class="logs-ai-context"
                :class="contextTone"
                :title="`上下文 ${formatTokens(contextTokens)} / ${formatTokens(contextWindow)} tokens；接近上限时自动压缩`"
                aria-label="查看上下文用量"
              >
                <svg viewBox="0 0 36 36" class="logs-ai-context-ring" aria-hidden="true">
                  <circle class="ring-track" cx="18" cy="18" r="15" />
                  <circle
                    class="ring-fill"
                    cx="18"
                    cy="18"
                    r="15"
                    stroke-dasharray="94.25"
                    :stroke-dashoffset="contextRingOffset"
                  />
                </svg>
                <span>{{ contextPercent }}%</span>
              </button>
              <span v-if="compressed" class="logs-ai-compressed">已自动压缩</span>
              <button v-if="messages.length" type="button" class="logs-ai-clear" :disabled="loading" @click="clearChat">
                清空
              </button>
              <n-button type="primary" size="small" :loading="loading" :disabled="!input.trim()" @click="send()">
                发送
              </n-button>
            </div>
          </div>
        </div>
        <div class="logs-ai-foot-meta">
          <span>只读查询 · Enter 发送 · 上下文 {{ formatTokens(contextTokens) }} / {{ formatTokens(contextWindow) }} tokens</span>
          <span>语音输入依赖浏览器 Web Speech API</span>
        </div>
      </footer>
    </div>
  </n-modal>
</template>

<style scoped>
.logs-ai-modal {
  width: min(760px, calc(100vw - 28px));
  max-height: min(86vh, 820px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--card);
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.4);
}

.logs-ai-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 20px 22px 14px;
  border-bottom: 1px solid var(--border);
  background: linear-gradient(130deg, var(--brand-softer), var(--card));
}

.logs-ai-icon {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: linear-gradient(145deg, var(--brand-soft), var(--brand-softer));
  color: var(--brand);
  flex: none;
}

.logs-ai-icon :deep(svg) {
  width: 19px;
  height: 19px;
}

.logs-ai-heading {
  min-width: 0;
  flex: 1;
}

.logs-ai-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.logs-ai-heading h2 {
  margin: 3px 0 4px;
  color: var(--ink);
  font-size: 19px;
  font-weight: 660;
}

.logs-ai-heading p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.6;
}

.logs-ai-close {
  display: flex;
  width: 28px;
  height: 28px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--muted-light);
  cursor: pointer;
  flex: none;
}

.logs-ai-close:hover {
  background: var(--surface-soft);
  color: var(--text);
}

.logs-ai-close :deep(svg) {
  width: 17px;
  height: 17px;
}

.logs-ai-body {
  flex: 1;
  min-height: 120px;
  overflow-y: auto;
  padding: 18px 22px;
}

.logs-ai-empty {
  display: flex;
  align-items: center;
  flex-direction: column;
  padding: 18px 8px 10px;
  text-align: center;
}

.logs-ai-empty-icon {
  display: flex;
  width: 42px;
  height: 42px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: var(--brand-soft);
  color: var(--brand);
}

.logs-ai-empty-icon :deep(svg) {
  width: 20px;
  height: 20px;
}

.logs-ai-empty strong {
  margin-top: 11px;
  color: var(--ink);
  font-size: 13px;
}

.logs-ai-empty p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 10.5px;
}

.logs-ai-suggestions {
  display: flex;
  max-width: 620px;
  flex-wrap: wrap;
  justify-content: center;
  gap: 7px;
  margin-top: 14px;
}

.logs-ai-suggestions button {
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-soft);
  color: var(--text);
  font-size: 10.5px;
  text-align: left;
  cursor: pointer;
}

.logs-ai-suggestions button:hover {
  border-color: var(--brand-border);
  background: var(--brand-softer);
  color: var(--brand-strong);
}

.logs-ai-msg {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 14px;
}

.logs-ai-msg.user {
  justify-content: flex-end;
}

.logs-ai-avatar {
  display: flex;
  width: 28px;
  height: 28px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.logs-ai-avatar :deep(svg) {
  width: 15px;
  height: 15px;
}

.logs-ai-bubble {
  max-width: min(620px, 88%);
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--surface-soft);
}

.logs-ai-msg.user .logs-ai-bubble {
  border-color: var(--brand-border);
  background: var(--brand-soft);
}

.logs-ai-text {
  margin: 0;
  color: var(--text);
  font-size: 12px;
  line-height: 1.75;
  white-space: pre-wrap;
  word-break: break-word;
}

.logs-ai-markdown {
  color: var(--text);
  font-size: 12px;
  line-height: 1.75;
  word-break: break-word;
}

.logs-ai-markdown :deep(p) {
  margin: 0 0 8px;
}

.logs-ai-markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.logs-ai-markdown :deep(ul),
.logs-ai-markdown :deep(ol) {
  margin: 6px 0 8px;
  padding-left: 18px;
}

.logs-ai-markdown :deep(li) {
  margin: 2px 0;
}

.logs-ai-markdown :deep(strong) {
  color: var(--ink);
  font-weight: 650;
}

.logs-ai-markdown :deep(code) {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--text);
  font-family: var(--mono);
  font-size: 10.5px;
}

.logs-ai-markdown :deep(pre) {
  max-width: 100%;
  margin: 8px 0;
  overflow-x: auto;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-strong);
}

.logs-ai-markdown :deep(pre code) {
  padding: 0;
  background: transparent;
  white-space: pre;
}

.logs-ai-markdown :deep(blockquote) {
  margin: 8px 0;
  padding: 6px 10px;
  border-left: 3px solid var(--brand-border);
  background: var(--brand-softer);
  color: var(--muted);
}

.logs-ai-markdown :deep(a) {
  color: var(--brand);
  text-decoration: underline;
}

.logs-ai-markdown :deep(table) {
  display: block;
  max-width: 100%;
  margin: 8px 0;
  overflow-x: auto;
  border-collapse: collapse;
  font-size: 11px;
}

.logs-ai-markdown :deep(th),
.logs-ai-markdown :deep(td) {
  padding: 5px 8px;
  border: 1px solid var(--border);
  text-align: left;
}

.logs-ai-markdown :deep(th) {
  background: var(--surface-soft);
  color: var(--muted);
  font-weight: 600;
}

.logs-ai-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 8px;
}

.logs-ai-apply {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
  padding-top: 9px;
  border-top: 1px solid var(--border);
}

.logs-ai-apply span {
  color: var(--muted-light);
  font-size: 9px;
}

.logs-ai-loading {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--muted);
}

.logs-ai-error {
  margin: 8px 0 2px;
  padding: 9px 11px;
  border: 1px solid var(--red-border);
  border-radius: 8px;
  background: var(--red-soft);
  color: var(--red);
  font-size: 11px;
}

.logs-ai-foot {
  padding: 12px 22px 15px;
  border-top: 1px solid var(--border);
}

.logs-ai-composer {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.logs-ai-composer-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.logs-ai-model-row {
  display: flex;
  flex: 1 1 430px;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.logs-ai-model-label {
  color: var(--muted);
  font-size: 10px;
  white-space: nowrap;
}

.logs-ai-model-select {
  max-width: 280px;
  flex: 1;
}

.logs-ai-reasoning-select {
  width: 92px;
  flex: none;
}

.logs-ai-actions {
  display: flex;
  align-items: center;
  gap: 7px;
  flex: none;
}

.logs-ai-context {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 7px 3px 5px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface-soft);
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9px;
  cursor: pointer;
}

.logs-ai-context:hover {
  border-color: var(--brand-border);
  color: var(--brand);
}

.logs-ai-context-ring {
  width: 18px;
  height: 18px;
  transform: rotate(-90deg);
}

.logs-ai-context-ring .ring-track {
  fill: none;
  stroke: var(--border);
  stroke-width: 3;
}

.logs-ai-context-ring .ring-fill {
  fill: none;
  stroke: var(--brand);
  stroke-width: 3;
  stroke-linecap: round;
  transition: stroke-dashoffset 0.3s ease;
}

.logs-ai-context.warn .ring-fill {
  stroke: var(--amber);
}

.logs-ai-context.danger {
  color: var(--red);
}

.logs-ai-context.danger .ring-fill {
  stroke: var(--red);
}

.logs-ai-compressed {
  padding: 2px 6px;
  border-radius: 999px;
  background: var(--amber-soft);
  color: var(--amber-ink);
  font-size: 9px;
  white-space: nowrap;
}

.logs-ai-clear {
  padding: 3px 6px;
  border: 0;
  background: transparent;
  color: var(--muted-light);
  font-size: 10px;
  cursor: pointer;
}

.logs-ai-clear:hover {
  color: var(--brand);
}

.logs-ai-clear:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.logs-ai-foot-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 8px;
  color: var(--muted-light);
  font-size: 9px;
}

.logs-ai-foot-meta button {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--muted-light);
  font-size: 9px;
  cursor: pointer;
}

.logs-ai-foot-meta button:hover {
  color: var(--brand);
}

.logs-ai-foot-meta button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

@media (max-width: 640px) {
  .logs-ai-head,
  .logs-ai-body,
  .logs-ai-foot {
    padding-left: 16px;
    padding-right: 16px;
  }

  .logs-ai-composer-bar {
    align-items: stretch;
    flex-direction: column;
  }

  .logs-ai-model-row {
    flex-basis: auto;
    flex-wrap: wrap;
  }

  .logs-ai-model-select {
    max-width: none;
  }

  .logs-ai-actions {
    justify-content: flex-end;
  }

  .logs-ai-foot-meta {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
