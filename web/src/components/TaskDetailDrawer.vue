<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  NButton,
  NCollapse,
  NCollapseItem,
  NDrawer,
  NDrawerContent,
  NImage,
  NInput,
  NModal,
  NRadio,
  NRadioGroup,
  NSelect,
  NSpin,
  useDialog,
  useMessage,
} from 'naive-ui'
import dayjs from 'dayjs'
import {
  AlertCircleOutline,
  ArrowBackOutline,
  CheckmarkCircleOutline,
  DocumentTextOutline,
  DownloadOutline,
  FlashOutline,
  ImagesOutline,
  RefreshOutline,
  SendOutline,
  ShieldCheckmarkOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import StatusTag from './StatusTag.vue'
import CopyButton from './CopyButton.vue'
import ReviewContext from './ReviewContext.vue'
import { taskApi } from '@/api'
import type { Task, TaskDetail, TaskEvent } from '@/api/types'
import { REVIEW_REASON_OPTIONS } from '@/constants/reviewReasons'

const props = defineProps<{
  show: boolean
  taskId: number | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  updated: []
}>()

const message = useMessage()
const dialog = useDialog()
const detail = ref<TaskDetail | null>(null)
const loading = ref(false)
const retrying = ref(false)
const reviewShow = ref(false)
const reviewLoading = ref(false)
const reviewError = ref('')
const reviewForm = reactive({
  conclusion: 'PASS',
  reasonCode: 'MANUAL_CONFIRMED',
  remark: '',
})
const reasonOptions = REVIEW_REASON_OPTIONS

const task = computed<Task | null>(() => detail.value?.task || null)
const events = computed<TaskEvent[]>(() => detail.value?.events || [])
const images = computed(() => (task.value?.input?.media || []).filter((m) => m.type === 'image' && m.url))
const texts = computed(() => (task.value?.input?.media || []).filter((m) => m.type === 'text' && m.content))

const scorePercent = computed(() => {
  const score = task.value?.result_detail?.score ?? task.value?.model_result?.score
  return Math.round(Math.min(1, Math.max(0, Number(score || 0))) * 100)
})

const riskTone = computed(() => {
  const level = task.value?.result_detail?.risk_level || task.value?.model_result?.risk_level
  if (level === 'high') return 'high'
  if (level === 'medium') return 'medium'
  return 'low'
})

const timeline = computed(() => {
  const list = [...events.value]
  if (task.value && !list.length) {
    list.push({
      id: 0,
      task_id: task.value.id,
      project_id: task.value.project_id,
      from_status: '',
      to_status: task.value.status,
      message: '任务已创建',
      cost_ms: 0,
      created_at: task.value.created_at,
    })
  }
  return list
})

function formatTime(value?: string | null) {
  return value ? dayjs(value).format('YYYY-MM-DD HH:mm:ss') : '—'
}

function sourceLabel(source?: string) {
  const map: Record<string, string> = {
    ADMIN: '后台提交', OPENAPI: '开放 API', REDIS: 'Redis', WEBSOCKET: 'WebSocket', POLLING: '定时轮询',
  }
  return map[source || ''] || source || '—'
}

function formatDuration(ms?: number | null) {
  if (!ms) return '—'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

async function load() {
  if (!props.taskId) return
  loading.value = true
  try {
    detail.value = await taskApi.get(props.taskId)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载任务失败')
  } finally {
    loading.value = false
  }
}

function retry() {
  if (!task.value) return
  dialog.warning({
    title: '重新执行审核任务',
    content: '任务将重新进入审核队列，可能产生新的模型调用费用。确认继续？',
    positiveText: '重新执行',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (!task.value) return
      retrying.value = true
      try {
        await taskApi.retry(task.value.id)
        message.success('任务已重新入队')
        await load()
        emit('updated')
      } catch (error) {
        message.error(error instanceof Error ? error.message : '重试失败')
      } finally {
        retrying.value = false
      }
    },
  })
}

function openReview() {
  reviewForm.conclusion = 'PASS'
  reviewForm.reasonCode = 'MANUAL_CONFIRMED'
  if (task.value?.result_detail?.model_error) reviewForm.reasonCode = 'MODEL_ERROR'
  else if (task.value?.rule_trace?.trace?.some((group) => group.hit && group.on_hit_result === 'REVIEW')) reviewForm.reasonCode = 'RULE_REVIEW'
  else {
    const confidence = task.value?.result_detail?.category_confidence ?? task.value?.model_result?.category_confidence
    if (confidence !== undefined && confidence !== null && confidence < 0.6) reviewForm.reasonCode = 'CLASSIFICATION_UNCERTAIN'
  }
  reviewForm.remark = ''
  reviewError.value = ''
  reviewShow.value = true
}

async function submitReview() {
  if (!task.value) return
  reviewLoading.value = true
  reviewError.value = ''
  try {
    await taskApi.review(task.value.id, {
      conclusion: reviewForm.conclusion,
      reason_code: reviewForm.reasonCode,
      remark: reviewForm.remark,
    })
    message.success('人工复核已提交')
    reviewShow.value = false
    await load()
    emit('updated')
  } catch (error) {
    reviewError.value = error instanceof Error ? error.message : '提交失败'
  } finally {
    reviewLoading.value = false
  }
}

function exportJSON() {
  if (!task.value) return
  const blob = new Blob([JSON.stringify(detail.value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${task.value.task_no}.json`
  a.click()
  URL.revokeObjectURL(url)
}

watch(
  () => [props.show, props.taskId],
  ([visible, id]) => {
    if (visible && id) load()
    if (!visible) detail.value = null
  },
  { immediate: true },
)
</script>

<template>
  <n-drawer :show="show" width="min(760px, 100vw)" placement="right" @update:show="emit('update:show', $event)">
    <n-drawer-content :native-scrollbar="false" closable>
      <template #header>
        <div class="drawer-header-row">
          <div class="drawer-title">
            <span>任务详情</span>
            <span v-if="task" class="mono">{{ task.task_no }}</span>
            <CopyButton v-if="task" :text="task.task_no" size="tiny" label="" title="复制任务号" />
          </div>
          <div v-if="task" class="header-actions">
            <n-button size="small" quaternary circle title="导出 JSON" aria-label="导出 JSON" @click="exportJSON">
              <template #icon><DownloadOutline /></template>
            </n-button>
            <n-button size="small" quaternary circle :loading="loading" title="刷新任务" aria-label="刷新任务" @click="load">
              <template #icon><RefreshOutline /></template>
            </n-button>
            <n-button v-if="task.status === 'REVIEW_PENDING'" size="small" type="primary" @click="openReview">
              人工复核
            </n-button>
            <n-button v-else-if="task.status === 'FAILED'" size="small" type="warning" secondary :loading="retrying" @click="retry">
              重试任务
            </n-button>
          </div>
        </div>
      </template>

      <n-spin :show="loading">
        <div v-if="task" class="detail-wrap">
          <section class="result-hero" :class="task.result.toLowerCase()">
            <div class="hero-top">
              <div class="hero-badge">
                <CheckmarkCircleOutline v-if="task.result === 'PASS'" />
                <AlertCircleOutline v-else-if="task.result === 'REJECT'" />
                <ShieldCheckmarkOutline v-else />
              </div>
              <div class="hero-copy">
                <div class="hero-label">审核结论</div>
                <h2>{{ task.result === 'PASS' ? '通过' : task.result === 'REJECT' ? '不通过' : task.result === 'REVIEW' ? '待人工复核' : '未判定' }}</h2>
                <p>{{ task.result_detail?.summary || task.error_message || '审核引擎尚未给出结论。' }}</p>
                <div v-if="task.result_detail?.category" class="hero-category">
                  <span>{{ task.result_detail.category_name || task.result_detail.category }}</span>
                  <code>{{ task.result_detail.category }}</code>
                  <em>{{ Math.round(Number(task.result_detail.category_confidence || 0) * 100) }}%</em>
                </div>
              </div>
              <div class="hero-tags">
                <StatusTag :status="task.status" type="task" />
                <StatusTag :status="task.result" type="result" />
              </div>
            </div>
            <div class="hero-metrics">
              <div>
                <span>风险分</span>
                <strong>{{ scorePercent }}<i>/100</i></strong>
              </div>
              <div>
                <span>风险等级</span>
                <strong class="risk" :class="riskTone">{{ riskTone === 'high' ? '高' : riskTone === 'medium' ? '中' : '低' }}</strong>
              </div>
              <div>
                <span>审核耗时</span>
                <strong>{{ formatDuration(task.duration_ms) }}</strong>
              </div>
              <div>
                <span>推理模型</span>
                <strong class="model-name">{{ task.result_detail?.model || task.provider_name || '—' }}</strong>
              </div>
            </div>
          </section>

          <div class="meta-grid">
            <div><span>业务键</span><strong class="mono">{{ task.biz_key || '—' }}</strong></div>
            <div><span>request_id</span><strong class="mono">{{ task.request_id || '—' }}</strong></div>
            <div><span>项目 / 审核</span><strong>{{ task.project_name || '—' }} · {{ task.audit_name || '—' }}</strong></div>
            <div><span>来源</span><strong>{{ sourceLabel(task.source_type) }}</strong></div>
            <div><span>媒体类型</span><strong>{{ task.media_type }}</strong></div>
            <div><span>创建时间</span><strong>{{ formatTime(task.created_at) }}</strong></div>
            <div><span>完成时间</span><strong>{{ formatTime(task.finished_at) }}</strong></div>
          </div>

          <section class="detail-section">
            <h3 class="section-label">审核依据</h3>
            <div class="tag-row">
              <span v-for="label in task.result_detail?.labels || task.model_result?.labels || []" :key="label" class="soft-tag">{{ label }}</span>
              <span v-if="!(task.result_detail?.labels || task.model_result?.labels || []).length" class="muted small">无标签</span>
            </div>
            <ul v-if="task.result_detail?.reasons?.length" class="reason-list">
              <li v-for="(reason, index) in task.result_detail.reasons" :key="index">{{ reason }}</li>
            </ul>
            <div v-if="task.result_detail?.model_error" class="inline-error">
              <AlertCircleOutline />
              <span>{{ task.result_detail.model_error }}</span>
            </div>
            <div v-if="task.result_detail?.manual_review" class="manual-note">
              <ShieldCheckmarkOutline />
              <div>
                <strong>人工复核 · {{ task.result_detail.manual_review.conclusion }}</strong>
                <p>
                  {{ task.result_detail.manual_review.reviewer }} · {{ task.result_detail.manual_review.reason_code }}
                  <template v-if="task.result_detail.manual_review.remark"> · {{ task.result_detail.manual_review.remark }}</template>
                </p>
              </div>
            </div>
            <div v-if="task.error_message" class="inline-error">
              <AlertCircleOutline />
              <span>{{ task.error_message }}</span>
            </div>

            <div v-if="task.result_detail?.callback" class="callback-result" :class="{ ok: task.result_detail.callback.ok }">
              <div class="callback-result-head">
                <SendOutline />
                <div>
                  <strong>结果回调 {{ task.result_detail.callback.ok ? '成功' : '失败' }}</strong>
                  <span>{{ task.result_detail.callback.target_url || '—' }}</span>
                </div>
                <span class="callback-badge">{{ task.result_detail.callback.transport === 'websocket' ? 'WS' : (task.result_detail.callback.http_status || 'ERR') }}</span>
              </div>
              <div class="callback-meta">
                <span>签名字段：{{ task.result_detail.callback.signed ? '已签名' : '未签名' }}</span>
                <span>尝试：{{ task.result_detail.callback.attempts || 0 }} 次</span>
                <span>通道：{{ task.result_detail.callback.transport === 'websocket' ? 'WebSocket' : 'HTTP' }}</span>
              </div>
              <p v-if="task.result_detail.callback.error" class="callback-error">{{ task.result_detail.callback.error }}</p>
            </div>

            <div v-if="task.result_detail?.routes?.length" class="route-results">
              <div class="route-results-head">
                <FlashOutline />
                <strong>结果动作</strong>
                <span>{{ task.result_detail.routes.filter((item) => item.ok).length }} / {{ task.result_detail.routes.length }} 成功</span>
              </div>
              <div v-for="(item, index) in task.result_detail.routes" :key="index" class="route-item" :class="{ ok: item.ok }">
                <span class="route-type">{{ (item.type || 'action').toUpperCase() }}</span>
                <span class="route-state">{{ item.ok ? '成功' : '失败' }}</span>
                <span class="route-duration">{{ item.duration_ms || 0 }} ms</span>
                <p v-if="item.error" class="route-error">{{ item.error }}</p>
              </div>
            </div>
          </section>

          <section v-if="texts.length || images.length" class="detail-section">
            <h3 class="section-label">图档内容</h3>
            <div v-for="(item, i) in texts" :key="`t-${i}`" class="text-doc">
              <div class="text-doc-head"><DocumentTextOutline /> {{ item.name || '文档文本' }}</div>
              <pre>{{ item.content }}</pre>
            </div>
            <div v-if="images.length" class="image-grid">
              <div v-for="(img, i) in images" :key="`i-${i}`" class="image-card">
                <n-image
                  :src="img.url"
                  :alt="img.name || `图档 ${i + 1}`"
                  :img-props="{ loading: 'lazy' }"
                />
                <span><ImagesOutline /> 图档 {{ i + 1 }}</span>
              </div>
            </div>
          </section>

          <section v-if="task.rule_trace?.trace?.length" class="detail-section">
            <h3 class="section-label">规则命中</h3>
            <div class="rule-match">
              <span>匹配规则</span>
              <strong>{{ task.rule_trace.matched_rule || '未命中规则，采用模型结论' }}</strong>
              <span class="rule-final">{{ task.rule_trace.final_result }}</span>
            </div>
            <div class="rule-groups">
              <details v-for="(group, index) in task.rule_trace.trace" :key="index" class="rule-group" :open="group.hit">
                <summary>
                  <span class="rule-state" :class="{ hit: group.hit }">{{ group.hit ? 'HIT' : 'MISS' }}</span>
                  <span class="rule-name">{{ group.name }}</span>
                  <span class="rule-logic">{{ group.logic }}</span>
                  <span class="rule-result">{{ group.on_hit_result }}</span>
                </summary>
                <div class="rule-items">
                  <div v-for="(item, j) in group.items" :key="j" class="rule-item">
                    <span class="rule-field mono">{{ item.field }}</span>
                    <span class="rule-op">{{ item.operator }}</span>
                    <span class="rule-expected">{{ JSON.stringify(item.expected) }}</span>
                    <span class="rule-arrow">→</span>
                    <span class="rule-actual" :class="{ hit: item.hit }">{{ JSON.stringify(item.actual) }}</span>
                  </div>
                </div>
              </details>
            </div>
          </section>

          <section class="detail-section">
            <h3 class="section-label">状态时间线</h3>
            <div class="timeline">
              <div v-for="(event, i) in timeline" :key="event.id || i" class="timeline-item">
                <span class="timeline-dot" :class="{ last: i === timeline.length - 1 }"></span>
                <div class="timeline-copy">
                  <div>
                    <StatusTag :status="event.to_status" type="task" />
                    <span class="timeline-time">{{ formatTime(event.created_at) }}</span>
                  </div>
                  <p>{{ event.message }}</p>
                  <span v-if="event.cost_ms" class="timeline-cost">{{ formatDuration(event.cost_ms) }}</span>
                </div>
              </div>
            </div>
          </section>

          <section class="detail-section">
            <h3 class="section-label">原始数据</h3>
            <n-collapse>
              <n-collapse-item title="模型结构化输出" name="model">
                <pre class="code-block">{{ JSON.stringify(task.model_result, null, 2) }}</pre>
              </n-collapse-item>
              <n-collapse-item title="任务输入" name="input">
                <pre class="code-block">{{ JSON.stringify(task.input, null, 2) }}</pre>
              </n-collapse-item>
            </n-collapse>
          </section>
        </div>

        <div v-else-if="!loading" class="drawer-empty">
          <ArrowBackOutline />
          <p>选择一条任务查看审核详情</p>
        </div>
      </n-spin>
    </n-drawer-content>
  </n-drawer>

  <n-modal v-model:show="reviewShow">
    <div class="review-modal">
      <div class="review-head">
        <div>
          <div class="review-kicker">HUMAN REVIEW</div>
          <h3>人工复核</h3>
          <p>复核结论将覆盖模型判定，并触发对应结果路由动作。</p>
        </div>
        <span class="review-icon"><ShieldCheckmarkOutline /></span>
      </div>
      <div class="review-body">
        <ReviewContext :task="task" compact class="modal-review-context" />
        <label class="field-label">复核结论</label>
        <n-radio-group v-model:value="reviewForm.conclusion" class="review-radio">
          <n-radio value="PASS" class="radio-card pass">
            <strong>通过</strong>
            <span>确认档案可放行</span>
          </n-radio>
          <n-radio value="REJECT" class="radio-card reject">
            <strong>不通过</strong>
            <span>确认存在风险或不合规</span>
          </n-radio>
        </n-radio-group>
        <label class="field-label">原因码</label>
        <n-select
          v-model:value="reviewForm.reasonCode"
          :options="reasonOptions"
          filterable
          tag
          :consistent-menu-width="false"
          placeholder="选择或输入原因码"
          style="width: 210px"
        />
        <label class="field-label">复核备注</label>
        <n-input v-model:value="reviewForm.remark" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" placeholder="写明判断依据、核对了哪些内容、为什么通过或不通过" />
        <div v-if="reviewError" class="review-error">{{ reviewError }}</div>
        <div class="review-actions">
          <n-button @click="reviewShow = false">取消</n-button>
          <n-button type="primary" :loading="reviewLoading" @click="submitReview">提交复核</n-button>
        </div>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.drawer-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
  color: var(--ink);
  font-size: 15px;
  font-weight: 600;
}

.drawer-title .mono {
  color: var(--muted-light);
  font-size: 10px;
  font-weight: 400;
}

.drawer-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 7px;
}

.detail-wrap {
  padding-bottom: 30px;
}

.result-hero {
  padding: 20px;
  border: 1px solid #e1e8f4;
  border-radius: 16px;
  background: linear-gradient(130deg, #f8fbff, #fdfefe);
}

.result-hero.reject {
  border-color: #fbdcdc;
  background: linear-gradient(130deg, #fff8f8, #fffdfd);
}

.result-hero.review {
  border-color: #f8e8c2;
  background: linear-gradient(130deg, #fffcf3, #fffefa);
}

.hero-top {
  display: flex;
  align-items: flex-start;
  gap: 13px;
}

.hero-badge {
  display: flex;
  width: 44px;
  height: 44px;
  align-items: center;
  justify-content: center;
  border-radius: 13px;
  background: var(--green-soft);
  color: var(--green);
  flex-shrink: 0;
}

.reject .hero-badge {
  background: var(--red-soft);
  color: var(--red);
}
.review .hero-badge {
  background: var(--amber-soft);
  color: #d97706;
}

.hero-badge :deep(svg) {
  width: 22px;
  height: 22px;
}

.hero-copy {
  flex: 1;
  min-width: 0;
}

.hero-label {
  color: var(--muted-light);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.09em;
  text-transform: uppercase;
}

.hero-copy h2 {
  margin: 2px 0 5px;
  color: var(--ink);
  font-size: 21px;
  font-weight: 680;
  letter-spacing: -0.02em;
}

.hero-copy p {
  margin: 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.7;
}

.hero-category {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--purple-soft);
  color: var(--purple);
  font-size: 10px;
}

.hero-category code {
  color: var(--purple);
  font-family: var(--mono);
  font-size: 8.5px;
}

.hero-category em {
  color: var(--muted);
  font-size: 9px;
  font-style: normal;
}

.hero-tags {
  display: flex;
  align-items: flex-end;
  flex-direction: column;
  gap: 5px;
  flex-shrink: 0;
}

.hero-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}

.hero-metrics span {
  display: block;
  color: var(--muted-light);
  font-size: 10px;
}

.hero-metrics strong {
  display: block;
  margin-top: 2px;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 16px;
  font-weight: 650;
}

.hero-metrics i {
  color: var(--muted-light);
  font-size: 9px;
  font-style: normal;
}

.hero-metrics .risk.high {
  color: var(--red);
}
.hero-metrics .risk.medium {
  color: #d97706;
}
.hero-metrics .risk.low {
  color: var(--green);
}

.hero-metrics .model-name {
  overflow: hidden;
  font-size: 10.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  margin: 20px 2px;
  padding: 17px 18px;
  border: 1px solid var(--border);
  border-radius: 13px;
  background: var(--surface-soft);
}

.meta-grid span {
  display: block;
  margin-bottom: 3px;
  color: var(--muted-light);
  font-size: 10px;
}

.meta-grid strong {
  color: var(--text);
  font-size: 11.5px;
  font-weight: 500;
  word-break: break-all;
}

.detail-section {
  margin-top: 24px;
}

.tag-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.soft-tag {
  padding: 3px 9px;
  border-radius: 999px;
  background: var(--surface-strong);
  color: var(--text);
  font-size: 11px;
}

.reason-list {
  margin: 12px 0 0;
  padding-left: 18px;
  color: var(--text);
  font-size: 12px;
  line-height: 1.75;
}

.reason-list li {
  margin: 4px 0;
}

.inline-error {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  margin-top: 10px;
  padding: 10px 12px;
  border: 1px solid var(--red-border);
  border-radius: 10px;
  background: var(--red-soft);
  color: var(--red);
  font-size: 11.5px;
}

.inline-error :deep(svg) {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  margin-top: 2px;
}

.callback-result {
  margin-top: 12px;
  padding: 11px 13px;
  border: 1px solid var(--red-border);
  border-radius: 10px;
  background: var(--red-soft);
}

.callback-result.ok {
  border-color: var(--green-border);
  background: var(--green-soft);
}

.callback-result-head {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.callback-result-head > :deep(svg) {
  width: 15px;
  height: 15px;
  margin-top: 2px;
  color: var(--red);
}

.callback-result.ok .callback-result-head > :deep(svg) {
  color: var(--green);
}

.callback-result-head > div {
  flex: 1;
  min-width: 0;
}

.callback-result-head strong {
  display: block;
  color: var(--red);
  font-size: 11px;
  font-weight: 600;
}

.callback-result.ok .callback-result-head strong {
  color: var(--green);
}

.callback-result-head span {
  display: block;
  overflow: hidden;
  color: var(--muted-light);
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.callback-badge {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--text) !important;
  font-family: var(--mono);
  font-size: 8.5px !important;
  flex-shrink: 0;
}

.callback-meta {
  display: flex;
  gap: 12px;
  margin-top: 6px;
  color: var(--muted-light);
  font-size: 9px;
}

.callback-error {
  margin: 6px 0 0;
  color: var(--red);
  font-size: 9px;
  word-break: break-all;
}

.route-results {
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-soft);
}

.route-results-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  font-size: 11px;
}

.route-results-head :deep(svg) {
  width: 14px;
  height: 14px;
  color: var(--brand);
}

.route-results-head strong {
  font-weight: 600;
}

.route-results-head span {
  margin-left: auto;
  color: var(--muted);
  font-size: 10px;
}

.route-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-radius: 7px;
  background: var(--red-soft);
  font-size: 10px;
}

.route-item + .route-item {
  margin-top: 4px;
}

.route-item.ok {
  background: var(--green-soft);
}

.route-type {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--surface-strong);
  font-family: var(--mono);
  font-size: 9px;
  color: var(--muted);
}

.route-state {
  font-weight: 550;
  color: var(--red);
}

.route-item.ok .route-state {
  color: var(--green);
}

.route-duration {
  margin-left: auto;
  color: var(--muted);
}

.route-error {
  flex-basis: 100%;
  margin: 2px 0 0;
  color: var(--red);
  font-size: 9px;
  word-break: break-all;
}

.manual-note {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-top: 12px;
  padding: 11px 13px;
  border: 1px solid var(--green-border);
  border-radius: 10px;
  background: var(--green-soft);
  color: var(--green);
}

.manual-note :deep(svg) {
  width: 16px;
  height: 16px;
  margin-top: 2px;
}

.manual-note strong {
  font-size: 12px;
}

.manual-note p {
  margin: 2px 0 0;
  color: var(--green);
  font-size: 10.5px;
}

.text-doc {
  margin-bottom: 10px;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 12px;
}

.text-doc-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 13px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-soft);
  color: var(--muted);
  font-size: 10.5px;
}

.text-doc-head :deep(svg) {
  width: 13px;
  height: 13px;
}

.text-doc pre {
  max-height: 240px;
  margin: 0;
  overflow: auto;
  padding: 13px;
  color: var(--text);
  font-family: inherit;
  font-size: 12px;
  line-height: 1.8;
  white-space: pre-wrap;
}

.image-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 9px;
}

.image-card {
  display: block;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--surface-soft);
}

.image-card img {
  display: block;
  width: 100%;
  height: 122px;
  object-fit: contain;
}

.image-card :deep(.n-image) {
  display: block;
  width: 100%;
}

.image-card :deep(.n-image img) {
  display: block;
  width: 100%;
  height: 122px;
  object-fit: contain;
  cursor: zoom-in;
}

.image-card span {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 9px;
  color: var(--muted);
  font-size: 9.5px;
}

.image-card span :deep(svg) {
  width: 11px;
  height: 11px;
}

.rule-match {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 9px;
  padding: 9px 12px;
  border-radius: 9px;
  background: var(--surface-soft);
  color: var(--muted);
  font-size: 11px;
}

.rule-match strong {
  color: var(--text);
  font-weight: 550;
}

.rule-final {
  margin-left: auto;
  color: var(--brand);
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 600;
}

.rule-group {
  margin-bottom: 7px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card);
}

.rule-group summary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 11px;
  cursor: pointer;
  list-style: none;
  font-size: 11.5px;
}

.rule-group summary::-webkit-details-marker {
  display: none;
}

.rule-state {
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--surface-strong);
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
}

.rule-state.hit {
  background: var(--brand-soft);
  color: var(--brand);
}

.rule-name {
  flex: 1;
  color: var(--text);
}

.rule-logic {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.rule-result {
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9px;
}

.rule-items {
  padding: 2px 11px 10px;
  border-top: 1px solid #f4f6fa;
}

.rule-item {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 5px 0;
  color: var(--muted);
  font-size: 10px;
  border-bottom: 1px dashed #f5f7fa;
}

.rule-item:last-child {
  border-bottom: 0;
}

.rule-field {
  min-width: 118px;
  color: #475569;
}

.rule-op {
  color: var(--muted-light);
}

.rule-expected {
  color: #475569;
}

.rule-arrow {
  color: #cbd5e1;
}

.rule-actual {
  color: #64748b;
}

.rule-actual.hit {
  color: var(--brand-strong);
  font-weight: 550;
}

.timeline {
  padding-left: 4px;
}

.timeline-item {
  position: relative;
  display: flex;
  gap: 13px;
  padding-bottom: 16px;
}

.timeline-item:not(:last-child)::before {
  content: '';
  position: absolute;
  top: 8px;
  bottom: 0;
  left: 4px;
  width: 1px;
  background: #e8edf5;
}

.timeline-dot {
  position: relative;
  z-index: 1;
  width: 9px;
  height: 9px;
  margin-top: 6px;
  border-radius: 50%;
  border: 2px solid #fff;
  background: #b8c4d6;
  box-shadow: 0 0 0 1px #dbe2ec;
  flex-shrink: 0;
}

.timeline-dot.last {
  background: var(--brand);
  box-shadow: 0 0 0 1px #b9cdfb;
}

.timeline-copy {
  flex: 1;
}

.timeline-copy > div {
  display: flex;
  align-items: center;
  gap: 8px;
}

.timeline-time {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.timeline-copy p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 11.5px;
}

.timeline-cost {
  display: inline-block;
  margin-top: 2px;
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.drawer-empty {
  display: flex;
  height: 260px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  color: var(--muted-light);
}

.drawer-empty :deep(svg) {
  width: 28px;
  height: 28px;
}

.modal-review-context {
  margin-bottom: 16px;
}

.review-modal {
  width: min(540px, calc(100vw - 32px));
  max-height: calc(100vh - 40px);
  overflow-y: auto;
  border-radius: 18px;
  background: var(--card);
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.review-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 22px 24px 16px;
  border-bottom: 1px solid var(--border);
}

.review-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.13em;
}

.review-head h3 {
  margin: 4px 0 3px;
  font-size: 19px;
  font-weight: 650;
}

.review-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11.5px;
}

.review-icon {
  display: flex;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--green-soft);
  color: var(--green);
}

.review-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.review-body {
  padding: 16px 22px 18px;
}

.field-label {
  display: block;
  margin: 0 0 7px;
  color: var(--text);
  font-size: 12px;
  font-weight: 550;
}

.field-label:not(:first-child) {
  margin-top: 16px;
}

.review-radio {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 9px;
  width: 100%;
}

.review-radio :deep(.n-radio) {
  display: flex;
  align-items: flex-start;
  padding: 11px 13px;
  border: 1px solid var(--border);
  border-radius: 10px;
}

.review-radio :deep(.n-radio--checked) {
  border-color: var(--brand-border);
  background: var(--brand-softer);
}

.radio-card strong {
  display: block;
  color: var(--ink);
  font-size: 12px;
}

.radio-card span {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9.5px;
}

.review-error {
  margin-top: 13px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--red-soft);
  color: var(--red);
  font-size: 11px;
}

.review-actions {
  display: flex;
  justify-content: flex-end;
  gap: 9px;
  margin-top: 20px;
}

@media (max-width: 600px) {
  .hero-metrics {
    grid-template-columns: 1fr 1fr;
  }

  .meta-grid {
    grid-template-columns: 1fr;
  }

  .image-grid {
    grid-template-columns: 1fr 1fr;
  }

  .hero-top {
    flex-wrap: wrap;
  }

  .hero-tags {
    width: 100%;
    align-items: flex-start;
    flex-direction: row;
  }
}
</style>
