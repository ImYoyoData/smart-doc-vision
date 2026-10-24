<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NModal, NSelect, NSpin, useDialog, useMessage } from 'naive-ui'
import { auditApi } from '@/api'
import type { ScriptExecution } from '@/api/types'

const props = defineProps<{
  show: boolean
  auditId: number
  auditName: string
}>()
const emit = defineEmits<{ (e: 'update:show', value: boolean): void }>()

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const detailLoading = ref(false)
const error = ref('')
const items = ref<ScriptExecution[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const status = ref<string | null>(null)
const selected = ref<ScriptExecution | null>(null)

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '成功', value: 'SUCCESS' },
  { label: '脚本错误', value: 'SCRIPT_ERROR' },
  { label: '超时', value: 'TIMEOUT' },
  { label: '内存超限', value: 'OOM' },
  { label: '宿主错误', value: 'HOST_ERROR' },
]

const statusLabels: Record<string, { label: string; tone: string }> = {
  SUCCESS: { label: '成功', tone: 'ok' },
  SCRIPT_ERROR: { label: '脚本错误', tone: 'error' },
  TIMEOUT: { label: '超时', tone: 'warn' },
  OOM: { label: '内存超限', tone: 'error' },
  HOST_ERROR: { label: '宿主错误', tone: 'warn' },
}

const hasMore = computed(() => page.value * pageSize < total.value)

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString() : '—'
}

function formatBytes(value?: number) {
  if (!value || value <= 0) return '—'
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  if (value >= 1024) return `${Math.round(value / 1024)} KB`
  return `${value} B`
}

function statusInfo(value: string) {
  return statusLabels[value] || { label: value || '—', tone: 'muted' }
}

async function load() {
  if (!props.auditId) return
  loading.value = true
  error.value = ''
  try {
    const data = await auditApi.scriptExecutions(props.auditId, {
      page: page.value,
      page_size: pageSize,
      ...(status.value ? { status: status.value } : {}),
    })
    items.value = data.items
    total.value = data.total
    if (selected.value && !items.value.some((item) => item.id === selected.value?.id)) {
      selected.value = null
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载脚本日志失败'
  } finally {
    loading.value = false
  }
}

async function openDetail(item: ScriptExecution) {
  detailLoading.value = true
  error.value = ''
  try {
    selected.value = await auditApi.scriptExecution(item.id)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载脚本详情失败'
  } finally {
    detailLoading.value = false
  }
}

function changePage(offset: number) {
  const next = page.value + offset
  if (next < 1) return
  page.value = next
  void load()
}

function clearLogs() {
  dialog.warning({
    title: '清理脚本日志',
    content: `确认清理「${props.auditName}」的全部脚本执行日志？该操作不可恢复。`,
    positiveText: '清理',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await auditApi.clearScriptExecutions(props.auditId)
        message.success('脚本日志已清理')
        selected.value = null
        page.value = 1
        await load()
      } catch (err) {
        error.value = err instanceof Error ? err.message : '清理脚本日志失败'
      }
    },
  })
}

watch(() => props.show, (visible) => {
  if (visible) {
    page.value = 1
    selected.value = null
    void load()
  }
})

watch(status, () => {
  page.value = 1
  void load()
})
</script>

<template>
  <n-modal :show="show" :mask-closable="true" @update:show="(value: boolean) => emit('update:show', value)">
    <div class="script-logs-modal">
      <div class="modal-head">
        <div>
          <div class="modal-kicker">JS EXECUTION LOGS</div>
          <h2>脚本日志 · {{ auditName }}</h2>
          <p>记录每次 JS 结果动作的开始时间、耗时、状态、日志和返回值。</p>
        </div>
        <n-button size="small" type="error" secondary @click="clearLogs">清理日志</n-button>
      </div>
      <div class="script-logs-body">
        <div class="script-logs-list">
          <div class="script-logs-toolbar">
            <n-select v-model:value="status" :options="statusOptions" size="small" style="width: 150px" />
            <span class="muted">共 {{ total }} 条</span>
            <n-button size="tiny" quaternary :disabled="page <= 1" @click="changePage(-1)">上一页</n-button>
            <n-button size="tiny" quaternary :disabled="!hasMore" @click="changePage(1)">下一页</n-button>
          </div>
          <div v-if="loading" class="script-logs-empty"><n-spin size="small" /> 正在加载…</div>
          <div v-else-if="!items.length" class="script-logs-empty">暂无脚本执行记录。</div>
          <button
            v-for="item in items"
            :key="item.id"
            type="button"
            class="script-log-item"
            :class="{ active: selected?.id === item.id }"
            @click="openDetail(item)"
          >
            <div class="script-log-main">
              <strong>{{ item.remark || `动作 ${item.action_index + 1}` }}</strong>
              <span>{{ item.task_no || `任务 #${item.task_id}` }}</span>
            </div>
            <div class="script-log-meta">
              <span class="log-status" :class="statusInfo(item.status).tone">{{ statusInfo(item.status).label }}</span>
              <span>{{ item.duration_ms }} ms</span>
              <span>{{ formatTime(item.started_at) }}</span>
            </div>
          </button>
        </div>

        <div class="script-logs-detail">
          <div v-if="detailLoading" class="script-logs-empty"><n-spin size="small" /> 正在加载详情…</div>
          <div v-else-if="!selected" class="script-logs-empty">选择左侧一条记录查看日志。</div>
          <template v-else>
            <div class="detail-head">
              <strong>{{ selected.remark || `动作 ${selected.action_index + 1}` }}</strong>
              <span class="log-status" :class="statusInfo(selected.status).tone">{{ statusInfo(selected.status).label }}</span>
            </div>
            <div class="detail-grid">
              <div><span>任务号</span><strong>{{ selected.task_no || '—' }}</strong></div>
              <div><span>开始时间</span><strong>{{ formatTime(selected.started_at) }}</strong></div>
              <div><span>耗时</span><strong>{{ selected.duration_ms }} ms</strong></div>
              <div><span>内存</span><strong>{{ formatBytes(selected.memory_bytes) }}</strong></div>
              <div><span>脚本哈希</span><code>{{ selected.script_hash.slice(0, 16) }}…</code></div>
            </div>
            <div v-if="selected.error_message" class="detail-error">{{ selected.error_message }}</div>
            <div class="detail-block">
              <label>执行日志</label>
              <pre>{{ selected.log_text || '（无日志）' }}</pre>
            </div>
            <div v-if="selected.result_json" class="detail-block">
              <label>返回值</label>
              <pre>{{ JSON.stringify(selected.result_json, null, 2) }}</pre>
            </div>
          </template>
        </div>
      </div>
      <div v-if="error" class="modal-error">{{ error }}</div>
      <div class="modal-footer">
        <n-button @click="emit('update:show', false)">关闭</n-button>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.script-logs-modal {
  display: flex;
  width: min(1080px, calc(100vw - 24px));
  max-height: min(780px, calc(100vh - 24px));
  flex-direction: column;
  overflow: hidden;
  border-radius: 16px;
  background: #fff;
}

.script-logs-body {
  display: flex;
  min-height: 0;
  flex: 1;
}

.script-logs-list {
  display: flex;
  width: 420px;
  flex-shrink: 0;
  flex-direction: column;
  overflow-y: auto;
  border-right: 1px solid #eef2f8;
}

.script-logs-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid #eef2f8;
}

.script-logs-toolbar .muted {
  flex: 1;
  color: var(--muted-light);
  font-size: 9px;
}

.script-logs-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 28px 14px;
  color: var(--muted-light);
  font-size: 10px;
  text-align: center;
}

.script-log-item {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: 10px 12px;
  border: 0;
  border-bottom: 1px solid #f1f4f9;
  background: #fff;
  text-align: left;
  cursor: pointer;
}

.script-log-item:hover,
.script-log-item.active {
  background: #f5f9ff;
}

.script-log-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.script-log-main strong {
  overflow: hidden;
  color: var(--ink);
  font-size: 10.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.script-log-main span {
  color: var(--muted-light);
  font-size: 9px;
}

.script-log-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--muted-light);
  font-size: 9px;
}

.log-status {
  padding: 1px 6px;
  border-radius: 99px;
  font-size: 8.5px;
}

.log-status.ok {
  background: #ecfdf3;
  color: #15803d;
}

.log-status.error {
  background: #fef2f2;
  color: #b91c1c;
}

.log-status.warn {
  background: #fffbeb;
  color: #b45309;
}

.log-status.muted {
  background: #f3f4f6;
  color: #6b7280;
}

.script-logs-detail {
  min-width: 0;
  flex: 1;
  overflow-y: auto;
  padding: 14px 16px;
}

.detail-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.detail-head strong {
  color: var(--ink);
  font-size: 12px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 12px;
}

.detail-grid > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.detail-grid span {
  color: var(--muted-light);
  font-size: 9px;
}

.detail-grid strong,
.detail-grid code {
  color: var(--ink);
  font-size: 10px;
  font-family: var(--mono);
}

.detail-error {
  margin-bottom: 10px;
  padding: 8px 10px;
  border: 1px solid #fecaca;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 10px;
  white-space: pre-wrap;
}

.detail-block {
  margin-bottom: 12px;
}

.detail-block label {
  display: block;
  margin-bottom: 5px;
  color: var(--muted-light);
  font-size: 9px;
}

.detail-block pre {
  max-height: 260px;
  margin: 0;
  overflow: auto;
  padding: 9px 10px;
  border: 1px solid #e8eef7;
  border-radius: 8px;
  background: #f8fafd;
  color: #334155;
  font-family: var(--mono);
  font-size: 9.5px;
  line-height: 1.6;
  white-space: pre-wrap;
}

@media (max-width: 760px) {
  .script-logs-body {
    flex-direction: column;
  }

  .script-logs-list {
    width: 100%;
    max-height: 260px;
    border-right: 0;
    border-bottom: 1px solid #eef2f8;
  }
}
</style>
