<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { NButton, NInput, NInputNumber, NModal, NSelect, NSwitch, NTag, useMessage } from 'naive-ui'
import { auditApi } from '@/api'
import type { TriggerSource } from '@/api/types'

const props = defineProps<{ show: boolean; trigger: TriggerSource | null }>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'updated'): void
}>()
const message = useMessage()
const saving = ref(false)
const testing = ref(false)
const testResult = ref<Record<string, unknown> | null>(null)
const testError = ref('')
const headersText = ref('')

const form = reactive({
  name: '',
  status: 'DISABLED',
  priority: 40,
  secret: '',
  config: {} as Record<string, any>,
})

const typeLabels: Record<string, string> = {
  OPENAPI: '开放 API',
  WEBSOCKET: 'WebSocket',
  REDIS: 'Redis Stream',
  POLLING: 'HTTP 定时轮询',
}
const type = computed(() => props.trigger?.type || '')
const title = computed(() => `${typeLabels[type.value] || type.value} · 触发源配置`)

const statusOptions = [
  { label: '启用 ACTIVE', value: 'ACTIVE' },
  { label: '暂停 PAUSED', value: 'PAUSED' },
  { label: '停用 DISABLED', value: 'DISABLED' },
]
const methodOptions = ['GET', 'POST'].map((value) => ({ label: value, value }))

watch(
  () => props.trigger,
  (trigger) => {
    if (!trigger) return
    form.name = trigger.name
    form.status = trigger.status
    form.priority = trigger.priority
    form.secret = ''
    form.config = { ...(trigger.config || {}) }
    headersText.value = Object.entries(form.config.headers || {})
      .map(([key, value]) => `${key}: ${value}`)
      .join('\n')
    testResult.value = null
    testError.value = ''
  },
  { immediate: true },
)

function close() {
  emit('update:show', false)
}

function parseHeaders(): Record<string, string> {
  const map: Record<string, string> = {}
  for (const line of headersText.value.split(/\r?\n/)) {
    const index = line.indexOf(':')
    if (index <= 0) continue
    const key = line.slice(0, index).trim()
    if (key) map[key] = line.slice(index + 1).trim()
  }
  return map
}

async function save() {
  if (!props.trigger) return
  saving.value = true
  try {
    const config = { ...form.config }
    if (type.value === 'POLLING') config.headers = parseHeaders()
    await auditApi.updateTrigger(props.trigger.id, {
      name: form.name,
      status: form.status,
      priority: form.priority,
      config,
      ...(form.secret.trim() ? { secret: form.secret.trim() } : {}),
    })
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

        <template v-if="type === 'REDIS'">
          <div class="two-col">
            <div class="field">
              <label>Stream</label>
              <n-input v-model:value="form.config.stream" size="small" placeholder="sdv:project:audit:review" />
            </div>
            <div class="field">
              <label>消费组</label>
              <n-input v-model:value="form.config.group" size="small" placeholder="sdv-cg-1" />
            </div>
          </div>
          <div class="field">
            <label>批量大小 <span>1~32</span></label>
            <n-input-number v-model:value="form.config.batch_size" size="small" :min="1" :max="32" style="width: 160px" />
          </div>
          <p class="hint">消息格式：<code>XADD stream * data '{"biz_key":"...","media":[{"type":"text","content":"..."}]}'</code></p>
        </template>

        <template v-else-if="type === 'POLLING'">
          <div class="field">
            <label>轮询地址</label>
            <n-input v-model:value="form.config.url" size="small" placeholder="https://example.com/api/pending-tasks" />
          </div>
          <div class="two-col">
            <div class="field">
              <label>请求方法</label>
              <n-select v-model:value="form.config.method" :options="methodOptions" size="small" />
            </div>
            <div class="field">
              <label>轮询间隔（秒）<span>10~86400</span></label>
              <n-input-number v-model:value="form.config.interval_sec" size="small" :min="10" :max="86400" style="width: 100%" />
            </div>
          </div>
          <div class="three-col">
            <div class="field">
              <label>列表字段路径</label>
              <n-input v-model:value="form.config.items_path" size="small" placeholder="items" />
            </div>
            <div class="field">
              <label>游标字段路径</label>
              <n-input v-model:value="form.config.cursor_path" size="small" placeholder="next_cursor" />
            </div>
            <div class="field">
              <label>游标参数名</label>
              <n-input v-model:value="form.config.cursor_param" size="small" placeholder="cursor" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>鉴权 Header 名称 <span>值来自下方密钥</span></label>
              <n-input v-model:value="form.config.auth_header" size="small" placeholder="Authorization" />
            </div>
            <div class="field">
              <label>鉴权密钥 <span>留空保持原值</span></label>
              <n-input v-model:value="form.secret" type="password" show-password-on="click" size="small" placeholder="Bearer token 或签名密钥" />
            </div>
          </div>
          <div class="field">
            <label>附加请求头 <span>每行 key: value</span></label>
            <n-input v-model:value="headersText" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" size="small" placeholder="X-Tenant: demo" />
          </div>
          <div class="field-inline">
            <n-switch v-model:value="form.config.allow_private" size="small" />
            <span>允许访问内网地址（仅联调环境建议开启）</span>
          </div>
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
  width: min(640px, calc(100vw - 32px));
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  border-radius: 14px;
  background: #fff;
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
.three-col {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
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
.field-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--muted);
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
  background: #eef2f8;
  font-size: 11px;
  word-break: break-all;
}
.runtime {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 14px;
  padding: 10px 12px;
  border-radius: 10px;
  background: #f7f9fc;
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
  color: #d03050;
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
  color: #d03050;
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
  .three-col,
  .runtime {
    grid-template-columns: 1fr;
  }
}
</style>
