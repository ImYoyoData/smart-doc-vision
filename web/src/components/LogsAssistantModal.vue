<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { NButton, NInput, NModal, NSpin, NTag } from 'naive-ui'
import {
  CloseOutline,
  SearchOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import VoiceInputButton from '@/components/VoiceInputButton.vue'
import { logApi } from '@/api'
import type { LogsAssistantFilter } from '@/api/types'

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

const suggestions = [
  '最近 7 天登录失败的是谁，失败了多少次？',
  '统计最近 30 天每种操作类型的次数',
  '查一下 admin 今天的全部操作',
  '有没有任务失败或者开放 API 提交失败的日志？',
]

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
      messages: [...history, { role: 'user', content: prompt }],
      filters: props.filters,
    })
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
            <p class="logs-ai-text">{{ item.content }}</p>
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
          <div class="logs-ai-composer-actions">
            <VoiceInputButton :disabled="loading" @transcript="appendInput" />
            <n-button type="primary" :loading="loading" :disabled="!input.trim()" @click="send()">
              发送
            </n-button>
          </div>
        </div>
        <div class="logs-ai-foot-meta">
          <span>只读查询 · 支持 Enter 发送 · 语音输入依赖浏览器 Web Speech API</span>
          <button v-if="messages.length" type="button" :disabled="loading" @click="clearChat">清空对话</button>
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
  align-items: flex-end;
  gap: 9px;
}

.logs-ai-composer > :first-child {
  flex: 1;
}

.logs-ai-composer-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: none;
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

  .logs-ai-composer {
    align-items: stretch;
    flex-direction: column;
  }

  .logs-ai-composer-actions {
    justify-content: flex-end;
  }

  .logs-ai-foot-meta {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
