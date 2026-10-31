<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NInput, NInputNumber, NModal, NSelect, NSwitch, NTag, useMessage } from 'naive-ui'
import { auditApi, redisConnectionApi } from '@/api'
import type { RedisConnection, TriggerSource, TriggerUpdate } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{ show: boolean; trigger: TriggerSource | null }>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'updated'): void
}>()
const message = useMessage()
const router = useRouter()
const workspace = useWorkspaceStore()
const saving = ref(false)
const testing = ref(false)
const testResult = ref<Record<string, unknown> | null>(null)
const testError = ref('')
const connections = ref<RedisConnection[]>([])
const loadingConnections = ref(false)

const form = reactive({
  name: '',
  status: 'DISABLED',
  priority: 40,
  secret: '',
  config: {} as Record<string, any>,
  redisConnectionId: null as number | null,
  redisDb: 0,
  stream: '',
  consumerGroup: '',
  startPosition: '$',
  batchSize: 16,
  maxInflight: 1,
})

const typeLabels: Record<string, string> = {
  OPENAPI: '开放 API',
  WEBSOCKET: 'WebSocket',
  REDIS: 'Redis Stream',
}
const type = computed(() => props.trigger?.type || '')
const title = computed(() => `${typeLabels[type.value] || type.value} · 触发源配置`)
const isRedis = computed(() => type.value === 'REDIS')

const statusOptions = [
  { label: '启用 ACTIVE', value: 'ACTIVE' },
  { label: '暂停 PAUSED', value: 'PAUSED' },
  { label: '停用 DISABLED', value: 'DISABLED' },
]
const startPositionOptions = [
  { label: '$ · 只消费新消息（推荐）', value: '$' },
  { label: '0 · 从头消费积压消息', value: '0' },
]
const connectionOptions = computed(() => connections.value.map((connection) => {
  const unavailable = connection.status !== 'ACTIVE' || !connection.last_test_ok || connection.health !== 'OK'
  const suffix = connection.status !== 'ACTIVE'
    ? ' · 已停用'
    : !connection.last_test_ok || connection.health !== 'OK'
      ? ' · 未通过测试'
      : ''
  return {
    label: `${connection.name} · ${connection.addr} · DB ${connection.db}${suffix}`,
    value: connection.id,
    disabled: unavailable,
  }
}))
const hasEligibleConnection = computed(() => connections.value.some((connection) =>
  connection.status === 'ACTIVE' && connection.last_test_ok && connection.health === 'OK'))

function onRedisConnectionChange(value: string | number | null) {
  const id = typeof value === 'number' ? value : Number(value || 0)
  form.redisConnectionId = id || null
  const connection = connections.value.find((item) => item.id === id)
  if (connection) form.redisDb = connection.db ?? 0
}

function pauseReasonLabel(reason: string) {
  const labels: Record<string, string> = {
    MANUAL: '手动暂停',
    CONNECTION_DISABLED: 'Redis 连接已停用',
    CONNECTION_UNHEALTHY: 'Redis 连接不可用',
    FAILURE_LIMIT: '连续失败达到上限',
  }
  return labels[reason] || reason
}

function applyTrigger(trigger: TriggerSource) {
  form.name = trigger.name
  form.status = trigger.status
  form.priority = trigger.priority
  form.secret = ''
  form.config = { ...(trigger.config || {}) }
  form.redisConnectionId = trigger.redis_connection_id || null
  form.redisDb = trigger.redis_db || 0
  form.stream = trigger.stream || ''
  form.consumerGroup = trigger.consumer_group || ''
  form.startPosition = trigger.start_position || '$'
  form.batchSize = trigger.batch_size || 16
  form.maxInflight = trigger.max_inflight || 1
  testResult.value = null
  testError.value = ''
}

watch(
  () => props.trigger,
  (trigger) => {
    if (trigger) applyTrigger(trigger)
  },
  { immediate: true },
)

watch(
  () => props.show,
  (visible) => {
    if (!visible) return
    if (props.trigger) applyTrigger(props.trigger)
    void loadConnections()
  },
)

async function loadConnections() {
  if (!workspace.currentProjectId || !isRedis.value) return
  loadingConnections.value = true
  try {
    const data = await redisConnectionApi.list(workspace.currentProjectId)
    connections.value = data.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载 Redis 连接失败')
  } finally {
    loadingConnections.value = false
  }
}

function close() {
  emit('update:show', false)
}

function goConnections() {
  emit('update:show', false)
  router.push({ name: 'connections' })
}

async function save() {
  if (!props.trigger) return
  if (isRedis.value && form.status === 'ACTIVE') {
    if (!form.redisConnectionId) {
      message.warning('启用 Redis 触发前请选择项目 Redis 连接')
      return
    }
    if (!form.stream.trim()) {
      message.warning('请填写要监听的 Redis Stream')
      return
    }
  }
  saving.value = true
  try {
    const payload: TriggerUpdate = {
      name: form.name,
      status: form.status,
      priority: form.priority,
    }
    if (isRedis.value) {
      payload.redis_connection_id = form.redisConnectionId || undefined
      payload.redis_db = form.redisDb
      payload.stream = form.stream.trim()
      payload.consumer_group = form.consumerGroup.trim()
      payload.start_position = form.startPosition
      payload.batch_size = form.batchSize
      payload.max_inflight = form.maxInflight
    } else {
      payload.config = form.config
    }
    if (form.secret.trim()) payload.secret = form.secret.trim()
    await auditApi.updateTrigger(props.trigger.id, payload)
    message.success('触发源配置已保存')
    emit('updated')
    close()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存触发源失败')
  } finally {
    saving.value = false
  }
}

async function test() {
  if (!props.trigger) return
  testing.value = true
  testError.value = ''
  testResult.value = null
  try {
    testResult.value = await auditApi.testTrigger(props.trigger.id)
  } catch (error) {
    testError.value = error instanceof Error ? error.message : '测试失败'
  } finally {
    testing.value = false
  }
}

function formatTime(value?: string | null): string {
  if (!value) return '—'
  return new Date(value).toLocaleString()
}
</script>

<template>
  <n-modal :show="props.show" :mask-closable="false" @update:show="(value) => emit('update:show', value)">
    <div class="trigger-modal">
      <header class="modal-head">
        <div>
          <h3>{{ title }}</h3>
          <p>配置触发方式、运行状态与接入参数。</p>
        </div>
        <n-tag size="small" :bordered="false" type="info">{{ type }}</n-tag>
      </header>

      <div class="modal-body">
        <div class="two-col">
          <div class="field">
            <label>触发源名称</label>
            <n-input v-model:value="form.name" size="small" placeholder="例如：业务系统 Redis 队列" />
          </div>
          <div class="field">
            <label>状态</label>
            <n-select v-model:value="form.status" :options="statusOptions" size="small" />
          </div>
        </div>
        <div class="field">
          <label>优先级 <span>数值越小越靠前展示</span></label>
          <n-input-number v-model:value="form.priority" size="small" :min="0" :max="1000" style="width: 160px" />
        </div>

        <template v-if="isRedis">
          <div v-if="props.trigger?.pause_reason" class="connection-hint paused">
            当前暂停原因：{{ pauseReasonLabel(props.trigger.pause_reason) }}
            <template v-if="props.trigger.auto_paused">（平台自动暂停）</template>
          </div>
          <div class="field">
            <label>Redis 连接 <span>只能选择当前项目已启用且测试成功的连接</span></label>
            <n-select
              :value="form.redisConnectionId"
              @update:value="onRedisConnectionChange"
              :options="connectionOptions"
              :loading="loadingConnections"
              size="small"
              clearable
              placeholder="选择项目 Redis 连接"
            />
            <div v-if="!loadingConnections && !hasEligibleConnection" class="connection-hint">
              当前项目没有可用 Redis 连接。请先到
              <n-button text type="primary" size="tiny" @click="goConnections">数据连接</n-button>
              新增并测试连接。
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>DB 编号 <span>默认来自连接，可覆盖</span></label>
              <n-input-number v-model:value="form.redisDb" size="small" :min="0" :max="255" style="width: 100%" />
            </div>
            <div class="field">
              <label>消费起点</label>
              <n-select v-model:value="form.startPosition" :options="startPositionOptions" size="small" />
            </div>
          </div>
          <div class="field">
            <label>Stream <span>要监听的队列名</span></label>
            <n-input v-model:value="form.stream" size="small" placeholder="例如 sdv:archive:contract" class="mono-input" />
          </div>
          <div class="two-col">
            <div class="field">
              <label>消费组 <span>留空自动生成</span></label>
              <n-input v-model:value="form.consumerGroup" size="small" placeholder="sdv-cg-1" />
            </div>
            <div class="field">
              <label>批量大小 <span>1~32</span></label>
              <n-input-number v-model:value="form.batchSize" size="small" :min="1" :max="32" style="width: 100%" />
            </div>
          </div>
          <div class="field">
            <label>最大并发处理数 <span>1~32</span></label>
            <n-input-number v-model:value="form.maxInflight" size="small" :min="1" :max="32" style="width: 160px" />
          </div>
          <p class="hint">
            一个 <code>(连接, DB, Stream)</code> 只能被一个审核消费。消息格式：<code>XADD stream * request_id "..." callback_url "https://..." data '{"request_id":"...","biz_key":"...","media":[{"type":"text","content":"..."}]}'</code>。request_id 用于关联提交与 HTTP 回调。
          </p>
        </template>

        <template v-else-if="type === 'OPENAPI'">
          <div class="field">
            <label>同步等待时长（毫秒）<span>0~10000，mode=sync 时生效</span></label>
            <n-input-number v-model:value="form.config.sync_wait_ms" size="small" :min="0" :max="10000" :step="500" style="width: 180px" />
          </div>
          <p class="hint">提交地址与 HMAC 示例可在「接入指南」中查看。</p>
        </template>

        <template v-else>
          <p class="hint">WebSocket 触发使用项目凭证签名连接，启用后即可通过 <code>/open/v1/ws/...</code> 提交任务并实时接收结果。</p>
        </template>

        <div class="runtime">
          <div><span>最近运行</span><strong>{{ formatTime(props.trigger?.last_run_at) }}</strong></div>
          <div><span>最近消费</span><strong>{{ formatTime(props.trigger?.last_consume_at) }}</strong></div>
          <div><span>连续失败</span><strong>{{ props.trigger?.fail_count ?? 0 }}</strong></div>
          <div><span>最近错误</span><strong class="error-text">{{ props.trigger?.last_error || '—' }}</strong></div>
        </div>

        <div v-if="testResult || testError" class="test-result">
          <strong>测试结果</strong>
          <pre v-if="testError" class="test-error">{{ testError }}</pre>
          <pre v-else>{{ JSON.stringify(testResult, null, 2) }}</pre>
        </div>
      </div>

      <footer class="modal-foot">
        <n-button size="small" :loading="testing" @click="test">测试连接</n-button>
        <div class="foot-right">
          <n-button size="small" @click="close">取消</n-button>
          <n-button size="small" type="primary" :loading="saving" @click="save">保存配置</n-button>
        </div>
      </footer>
    </div>
  </n-modal>
</template>

<style scoped>
.trigger-modal {
  width: min(680px, calc(100vw - 32px));
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  border-radius: 14px;
  background: var(--card);
  box-shadow: var(--shadow-lg, 0 18px 48px rgba(20, 32, 52, 0.18));
}
.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px 10px;
}
.modal-head h3 {
  margin: 0 0 4px;
  font-size: 16px;
}
.modal-head p {
  margin: 0;
  color: var(--muted);
  font-size: 12px;
}
.modal-body {
  padding: 4px 20px 12px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.field label {
  font-size: 12px;
  color: var(--muted);
}
.field label span {
  color: var(--muted);
  font-weight: 400;
}
.hint {
  margin: 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.6;
}
.hint code {
  padding: 1px 4px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--text);
  font-size: 11px;
  word-break: break-all;
}
.connection-hint {
  padding: 8px 10px;
  border: 1px solid var(--brand-border);
  border-radius: 8px;
  background: var(--brand-softer);
  color: var(--brand-strong);
  font-size: 11px;
  line-height: 1.7;
}
.connection-hint.paused {
  border-color: var(--amber-border);
  background: var(--amber-soft);
  color: var(--amber-ink);
}
.runtime {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 14px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--surface-soft);
  font-size: 12px;
}
.runtime div {
  display: flex;
  gap: 8px;
}
.runtime span {
  color: var(--muted);
  flex-shrink: 0;
}
.error-text {
  color: var(--red);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.test-result {
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 8px 10px;
  font-size: 12px;
}
.test-result pre {
  margin: 6px 0 0;
  max-height: 180px;
  overflow: auto;
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
}
.test-error {
  color: var(--red);
}
.modal-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 12px 20px 16px;
  border-top: 1px solid var(--border);
}
.foot-right {
  display: flex;
  gap: 8px;
}
@media (max-width: 640px) {
  .two-col,
  .runtime {
    grid-template-columns: 1fr;
  }
}
</style>
