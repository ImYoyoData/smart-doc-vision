<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  NButton,
  NInput,
  NSelect,
  NSpin,
  useDialog,
  useMessage,
} from 'naive-ui'
import dayjs from 'dayjs'
import {
  AlertCircleOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  DocumentTextOutline,
  ImagesOutline,
  RefreshOutline,
  ShieldCheckmarkOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import ReviewContext from '@/components/ReviewContext.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import { auditApi, taskApi } from '@/api'
import type { Audit, Task, TaskDetail } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useReviewStore } from '@/stores/review'
import { REVIEW_REASON_OPTIONS } from '@/constants/reviewReasons'

const workspace = useWorkspaceStore()
const reviewStore = useReviewStore()
const route = useRoute()
const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const detailLoading = ref(false)
const submitting = ref(false)
const queue = ref<Task[]>([])
const total = ref(0)
const audits = ref<Audit[]>([])
const selectedId = ref<number | null>(null)
const detail = ref<TaskDetail | null>(null)
const auditFilter = ref<number | null>(null)
const reasonCode = ref('MANUAL_CONFIRMED')
const remark = ref('')
const reasonOptions = REVIEW_REASON_OPTIONS
const initialProjectId = Number(route.query.project_id)
if (Number.isFinite(initialProjectId) && initialProjectId > 0 && initialProjectId !== workspace.currentProjectId) {
  workspace.setCurrentProject(initialProjectId)
}
const initialAuditId = Number(route.query.audit_id)
if (Number.isFinite(initialAuditId) && initialAuditId > 0) {
  auditFilter.value = initialAuditId
}
const initialTaskId = Number(route.query.task_id)

const selectedTask = computed(() => detail.value?.task || queue.value.find((t) => t.id === selectedId.value) || null)
const selectedAudit = computed(() => audits.value.find((a) => a.id === selectedTask.value?.audit_id) || null)
const images = computed(() => (selectedTask.value?.input?.media || []).filter((m) => m.type === 'image' && m.url))
const texts = computed(() => (selectedTask.value?.input?.media || []).filter((m) => m.type === 'text' && m.content))
const ruleTrace = computed(() => selectedTask.value?.rule_trace?.trace || [])
const labels = computed(
  () => selectedTask.value?.result_detail?.labels || selectedTask.value?.model_result?.labels || [],
)
const reasons = computed(
  () => selectedTask.value?.result_detail?.reasons || selectedTask.value?.model_result?.reasons || [],
)
const riskScore = computed(() => {
  const score = selectedTask.value?.result_detail?.score ?? selectedTask.value?.model_result?.score ?? 0
  return Math.round(Number(score) * 100)
})
const waitTime = computed(() => {
  if (!selectedTask.value) return '—'
  const minutes = Math.max(0, dayjs().diff(dayjs(selectedTask.value.created_at), 'minute'))
  if (minutes < 60) return `${minutes} 分钟`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时 ${minutes % 60} 分`
  return `${Math.floor(hours / 24)} 天 ${hours % 24} 小时`
})
const byAudit = computed(() => reviewStore.summary?.by_audit || [])
const auditOptions = computed(() => audits.value.map((a) => ({ label: a.name, value: a.id })))

function formatDuration(ms?: number | null) {
  if (!ms) return '—'
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)}s`
}

function waitLabel(task: Task) {
  const minutes = Math.max(0, dayjs().diff(dayjs(task.created_at), 'minute'))
  if (minutes < 60) return `${minutes} 分钟`
  if (minutes < 24 * 60) return `${Math.floor(minutes / 60)} 小时`
  return `${Math.floor(minutes / (24 * 60))} 天`
}

function sourceLabel(source?: string) {
  const map: Record<string, string> = {
    ADMIN: '后台提交',
    OPENAPI: '开放 API',
    REDIS: 'Redis',
    POLLING: '定时轮询',
    WEBSOCKET: 'WebSocket',
  }
  return map[source || ''] || source || '—'
}

async function loadAudits() {
  if (!workspace.currentProjectId) {
    audits.value = []
    return
  }
  try {
    const data = await auditApi.list({ project_id: workspace.currentProjectId, page_size: 100 })
    audits.value = data.items
  } catch {
    audits.value = []
  }
}

async function loadQueue() {
  if (!workspace.currentProjectId) {
    queue.value = []
    total.value = 0
    return
  }
  loading.value = true
  try {
    const params: Record<string, unknown> = {
      project_id: workspace.currentProjectId,
      status: 'REVIEW_PENDING',
      page: 1,
      page_size: 100,
      sort: 'created_at:asc',
    }
    if (auditFilter.value) params.audit_id = auditFilter.value
    const data = await taskApi.list(params)
    queue.value = data.items
    total.value = data.total
    if (selectedId.value && !data.items.some((t) => t.id === selectedId.value)) {
      selectedId.value = null
      detail.value = null
    }
    if (!selectedId.value && data.items.length) {
      const target = Number.isFinite(initialTaskId) && initialTaskId > 0
        ? data.items.find((task) => task.id === initialTaskId)
        : undefined
      await selectTask(target || data.items[0])
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载复核队列失败')
  } finally {
    loading.value = false
  }
}

function suggestReasonCode(task: Task) {
  const detail = task.result_detail
  if (detail?.model_error) return 'MODEL_ERROR'
  if (task.rule_trace?.trace?.some((group) => group.hit && group.on_hit_result === 'REVIEW')) return 'RULE_REVIEW'
  const confidence = detail?.category_confidence ?? task.model_result?.category_confidence
  if (confidence !== undefined && confidence !== null && confidence < 0.6) return 'CLASSIFICATION_UNCERTAIN'
  return 'MANUAL_CONFIRMED'
}

async function selectTask(task: Task) {
  selectedId.value = task.id
  reasonCode.value = suggestReasonCode(task)
  remark.value = ''
  detailLoading.value = true
  try {
    detail.value = await taskApi.get(task.id)
    if (detail.value?.task) reasonCode.value = suggestReasonCode(detail.value.task)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载任务详情失败')
  } finally {
    detailLoading.value = false
  }
}

async function refreshAll() {
  await Promise.all([loadQueue(), reviewStore.refresh(workspace.currentProjectId)])
}

function submitReview(conclusion: 'PASS' | 'REJECT') {
  if (!selectedTask.value) return
  const task = selectedTask.value
  dialog.warning({
    title: conclusion === 'PASS' ? '确认通过该图档' : '确认不通过该图档',
    content:
      conclusion === 'PASS'
        ? `复核通过后，任务将按 PASS 路由执行回调/动作。任务号 ${task.task_no}。`
        : `复核不通过后，将按 REJECT 路由回调业务方，并传回原因、标签和原始内容。任务号 ${task.task_no}。`,
    positiveText: conclusion === 'PASS' ? '确认通过' : '确认不通过',
    negativeText: '取消',
    onPositiveClick: async () => {
      submitting.value = true
      try {
        await taskApi.review(task.id, {
          conclusion,
          reason_code: reasonCode.value || (conclusion === 'PASS' ? 'MANUAL_CONFIRMED' : 'RISK_CONFIRMED'),
          remark: remark.value,
        })
        message.success(conclusion === 'PASS' ? '已通过并执行回调' : '已驳回并执行回调')
        selectedId.value = null
        detail.value = null
        await refreshAll()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '复核提交失败')
      } finally {
        submitting.value = false
      }
    },
  })
}

watch(() => workspace.currentProjectId, async () => {
  selectedId.value = null
  detail.value = null
  auditFilter.value = null
  await Promise.all([loadQueue(), loadAudits(), reviewStore.refresh(workspace.currentProjectId)])
})

watch(auditFilter, () => {
  selectedId.value = null
  detail.value = null
  loadQueue()
})

onMounted(async () => {
  await Promise.all([loadQueue(), loadAudits(), reviewStore.refresh(workspace.currentProjectId)])
})
</script>

<template>
  <div class="page review-page">
    <PageHeader
      eyebrow="HUMAN REVIEW"
      title="人工复核工作台"
      subtitle="模型无法确定的图档会进入这里。复核结论将触发对应结果路由并回调业务方。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" @click="refreshAll">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button type="primary" @click="$router.push({ name: 'audits', query: { create: '1' } })">
          <template #icon><ShieldCheckmarkOutline /></template>
          新建审核配置
        </n-button>
      </template>
    </PageHeader>

    <div class="review-banner">
      <span class="review-banner-icon"><AlertCircleOutline /></span>
      <div>
        <strong>{{ total }} 条图档等待人工确认</strong>
        <p>系统已按等待时长升序排列。复核后会立即执行 PASS / REJECT 回调，并通过 WebSocket / 查询接口对外可见。</p>
      </div>
      <div v-if="byAudit.length" class="review-audit-chips">
        <span v-for="item in byAudit.slice(0, 4)" :key="item.audit_id">{{ item.audit_name }} <b>{{ item.count }}</b></span>
      </div>
    </div>

    <div class="review-layout">
      <aside class="panel review-queue">
        <div class="queue-head">
          <div>
            <h3>复核队列</h3>
            <span>按进入时间升序 · 共 {{ total }} 条</span>
          </div>
          <n-select
            v-model:value="auditFilter"
            :options="auditOptions"
            clearable
            placeholder="全部审核"
            size="small"
            class="audit-filter"
          />
        </div>
        <div class="queue-list">
          <button
            v-for="task in queue"
            :key="task.id"
            type="button"
            class="queue-item"
            :class="{ active: task.id === selectedId }"
            @click="selectTask(task)"
          >
            <span class="queue-accent"></span>
            <span class="queue-main">
              <span class="queue-top">
                <span class="queue-no mono">{{ task.task_no }}</span>
                <span class="queue-wait"><TimeOutline />{{ waitLabel(task) }}</span>
              </span>
              <span class="queue-audit">{{ task.audit_name || '—' }}</span>
              <span class="queue-meta">
                <span>{{ sourceLabel(task.source_type) }}</span>
                <span>{{ task.media_type }}</span>
                <span class="mono">{{ task.biz_key || '无业务键' }}</span>
              </span>
            </span>
          </button>
          <div v-if="!queue.length && !loading" class="queue-empty">
            <CheckmarkCircleOutline />
            <strong>复核队列已清空</strong>
            <span>新的待议任务会自动出现在这里</span>
          </div>
        </div>
      </aside>

      <section class="panel review-workbench">
        <n-spin :show="detailLoading" class="workbench-spin">
          <div v-if="selectedTask" class="workbench-shell">
            <div class="workbench-head">
              <div>
                <div class="workbench-kicker">REVIEW TASK</div>
                <h2>{{ selectedTask.task_no }}</h2>
                <div class="workbench-sub">
                  {{ selectedAudit?.name || selectedTask.audit_name || '—' }} · {{ sourceLabel(selectedTask.source_type) }} ·
                  已等待 <b>{{ waitTime }}</b>
                </div>
              </div>
              <div class="workbench-status">
                <StatusTag status="REVIEW_PENDING" type="task" />
                <StatusTag :status="selectedTask.result" type="result" />
              </div>
            </div>

            <div class="workbench-scroll">
              <div class="workbench-body">
              <div class="workbench-content">
                <h3 class="section-label">图档内容</h3>
                <div v-for="(item, i) in texts" :key="`text-${i}`" class="review-text">
                  <div class="review-text-head"><DocumentTextOutline /> {{ item.name || `文本 ${i + 1}` }}</div>
                  <pre>{{ item.content }}</pre>
                </div>
                <div v-if="images.length" class="review-images">
                  <a v-for="(img, i) in images" :key="`img-${i}`" :href="img.url" target="_blank" rel="noreferrer">
                    <img :src="img.url" :alt="`图档 ${i + 1}`" loading="lazy" />
                    <span><ImagesOutline /> 图档 {{ i + 1 }}</span>
                  </a>
                </div>
                <div v-if="!texts.length && !images.length" class="muted small">任务没有可展示的媒体内容。</div>

                <h3 class="section-label">规则命中</h3>
                <div class="review-rules">
                  <div v-for="(group, i) in ruleTrace" :key="i" class="review-rule" :class="{ hit: group.hit }">
                    <div class="review-rule-head">
                      <span class="rule-flag">{{ group.hit ? 'HIT' : 'MISS' }}</span>
                      <strong>{{ group.name }}</strong>
                      <span>{{ group.logic }} → {{ group.on_hit_result }}</span>
                    </div>
                    <div class="rule-detail-list">
                      <span v-for="(item, j) in group.items" :key="j">
                        <code>{{ item.field }}</code> {{ item.operator }}
                        <em>{{ JSON.stringify(item.expected) }}</em> → {{ JSON.stringify(item.actual) }}
                      </span>
                    </div>
                  </div>
                </div>
              </div>

              <aside class="review-analysis">
                <div v-if="selectedTask.result_detail?.category" class="category-card">
                  <span class="category-label">分类结果</span>
                  <strong>{{ selectedTask.result_detail.category_name || selectedTask.result_detail.category }}</strong>
                  <code>{{ selectedTask.result_detail.category }} · {{ Math.round(Number(selectedTask.result_detail.category_confidence || 0) * 100) }}%</code>
                </div>
                <div class="score-card">
                  <div class="score-label">模型风险分</div>
                  <div class="score-number" :class="riskScore >= 75 ? 'high' : riskScore >= 40 ? 'medium' : 'low'">
                    {{ riskScore }}<small>/100</small>
                  </div>
                  <div class="score-track"><span :style="{ width: `${riskScore}%` }"></span></div>
                  <div class="score-note">{{ selectedTask.result_detail?.summary || '模型未提供结论摘要' }}</div>
                </div>

                <h4 class="analysis-title">风险标签</h4>
                <div class="tag-cloud">
                  <span v-for="label in labels" :key="label">{{ label }}</span>
                  <span v-if="!labels.length" class="muted small">无</span>
                </div>

                <h4 class="analysis-title">判定依据</h4>
                <ul class="analysis-reasons">
                  <li v-for="(reason, i) in reasons" :key="i">{{ reason }}</li>
                  <li v-if="!reasons.length" class="muted">无详细原因</li>
                </ul>

                <div class="analysis-meta">
                  <div><span>模型</span><strong class="mono">{{ selectedTask.result_detail?.model || '—' }}</strong></div>
                  <div><span>耗时</span><strong>{{ formatDuration(selectedTask.duration_ms) }}</strong></div>
                  <div><span>业务键</span><strong class="mono">{{ selectedTask.biz_key || '—' }}</strong></div>
                  <div><span>创建时间</span><strong>{{ selectedTask.created_at?.replace('T', ' ').slice(0, 19) }}</strong></div>
                </div>
              </aside>
            </div>

              <ReviewContext :task="selectedTask" class="review-context-block" />
            </div>
            <div class="review-action-bar">
              <div class="review-form">
                <n-select
                  v-model:value="reasonCode"
                  :options="reasonOptions"
                  filterable
                  tag
                  size="small"
                  placeholder="原因码"
                  :consistent-menu-width="false"
                  style="width: 168px; flex-shrink: 0"
                />
                <n-input v-model:value="remark" type="textarea" :autosize="{ minRows: 2, maxRows: 3 }" placeholder="写明判断依据、核对了哪些内容、为什么通过或不通过" size="small" style="flex: 1; min-width: 0" />
              </div>
              <div class="review-buttons">
                <n-button size="large" :loading="submitting" @click="submitReview('REJECT')">
                  <template #icon><CloseCircleOutline /></template>
                  不通过
                </n-button>
                <n-button size="large" type="primary" :loading="submitting" @click="submitReview('PASS')">
                  <template #icon><CheckmarkCircleOutline /></template>
                  通过并回调
                </n-button>
              </div>
            </div>
          </div>

          <EmptyState
            v-else
            title="没有待复核任务"
            description="当前项目的审核结果都已处理，或模型还没有产生 REVIEW 结论。"
          >
            <template #icon><ShieldCheckmarkOutline /></template>
            <n-button type="primary" secondary @click="refreshAll">刷新队列</n-button>
          </EmptyState>
        </n-spin>
      </section>
    </div>
  </div>
</template>

<style scoped>
.review-banner {
  display: flex;
  align-items: center;
  gap: 13px;
  margin-bottom: 15px;
  padding: 14px 18px;
  border: 1px solid #f3dfb8;
  border-radius: 13px;
  background: linear-gradient(120deg, #fffbf2, #fffefa);
}

.review-banner-icon {
  display: flex;
  width: 36px;
  height: 36px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: #fef3c7;
  color: #b45309;
  flex-shrink: 0;
}

.review-banner-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.review-banner strong {
  display: block;
  color: #78350f;
  font-size: 12.5px;
  font-weight: 650;
}

.review-banner p {
  margin: 2px 0 0;
  color: #a16207;
  font-size: 10.5px;
}

.review-audit-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-left: auto;
}

.review-audit-chips span {
  padding: 3px 8px;
  border-radius: 999px;
  background: rgba(245, 158, 11, 0.1);
  color: #a16207;
  font-size: 9.5px;
}

.review-audit-chips b {
  margin-left: 3px;
  font-family: var(--mono);
}

.review-layout {
  display: grid;
  grid-template-columns: 360px minmax(0, 1fr);
  gap: 14px;
  align-items: stretch;
  height: clamp(400px, calc(100vh - 305px), 920px);
}

.review-queue {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
}

.queue-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 16px 16px 12px;
  border-bottom: 1px solid #f0f3f8;
}

.queue-head h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 620;
}

.queue-head span {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9.5px;
}

.audit-filter {
  width: 130px;
}

.queue-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 7px;
}

.queue-item {
  position: relative;
  display: flex;
  width: 100%;
  align-items: stretch;
  gap: 0;
  margin-bottom: 4px;
  padding: 11px 11px 11px 14px;
  border: 1px solid transparent;
  border-radius: 10px;
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: all 0.16s ease;
}

.queue-item:hover {
  background: #fafcff;
}

.queue-item.active {
  border-color: #f2d9a5;
  background: #fffbf3;
}

.queue-accent {
  width: 3px;
  border-radius: 3px;
  background: #f1d7a2;
  margin-right: 10px;
  flex-shrink: 0;
}

.queue-item.active .queue-accent {
  background: #f59e0b;
}

.queue-main {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.queue-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.queue-no {
  color: var(--ink);
  font-size: 10px;
  font-weight: 600;
}

.queue-wait {
  display: flex;
  align-items: center;
  gap: 3px;
  color: #b45309;
  font-size: 8.5px;
  flex-shrink: 0;
}

.queue-wait :deep(svg) {
  width: 10px;
  height: 10px;
}

.queue-audit {
  margin-top: 3px;
  overflow: hidden;
  color: var(--text);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.queue-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 5px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.queue-meta span {
  padding: 1px 5px;
  border-radius: 4px;
  background: #f8fafc;
}

.queue-empty {
  display: flex;
  height: 100%;
  min-height: 260px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 4px;
  color: var(--green);
}

.queue-empty :deep(svg) {
  width: 26px;
  height: 26px;
  margin-bottom: 4px;
}

.queue-empty strong {
  font-size: 12px;
  font-weight: 550;
}

.queue-empty span {
  color: var(--muted-light);
  font-size: 10px;
}

.review-workbench {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  container-type: inline-size;
}

.workbench-spin {
  flex: 1;
  min-height: 0;
}

.workbench-spin :deep(.n-spin-container),
.workbench-spin :deep(.n-spin-content) {
  height: 100%;
  min-height: 0;
}

.workbench-spin :deep(.n-spin-content) {
  display: flex;
  flex-direction: column;
}

.workbench-shell {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
}

.workbench-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.review-workbench :deep(.empty-state) {
  height: 100%;
}

.workbench-head {
  display: flex;
  flex-shrink: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 20px 15px;
  border-bottom: 1px solid #f0f3f8;
  border-radius: 18px 18px 0 0;
  background: linear-gradient(120deg, #fffdf8, #fff);
}

.workbench-kicker {
  color: #b45309;
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.workbench-head h2 {
  margin: 3px 0 2px;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 16px;
  font-weight: 650;
}

.workbench-sub {
  color: var(--muted);
  font-size: 10px;
}

.workbench-sub b {
  color: #b45309;
  font-weight: 550;
}

.workbench-status {
  display: flex;
  gap: 5px;
}

.workbench-body {
  display: grid;
  grid-template-columns: minmax(0, 1.5fr) minmax(260px, 0.82fr);
  gap: 22px;
  padding: 18px 20px 22px;
}

.workbench-content {
  min-width: 0;
}

.review-text {
  margin-bottom: 10px;
  overflow: hidden;
  border: 1px solid #edf1f7;
  border-radius: 11px;
}

.review-text-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 12px;
  border-bottom: 1px solid #f1f4f9;
  background: #fbfcfe;
  color: var(--muted);
  font-size: 9.5px;
}

.review-text-head :deep(svg) {
  width: 12px;
  height: 12px;
}

.review-text pre {
  max-height: 210px;
  margin: 0;
  overflow: auto;
  padding: 12px;
  color: var(--text);
  font-family: inherit;
  font-size: 11.5px;
  line-height: 1.8;
  white-space: pre-wrap;
}

.review-images {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.review-images a {
  overflow: hidden;
  border: 1px solid #edf1f7;
  border-radius: 10px;
  background: #f8fafc;
}

.review-images img {
  display: block;
  width: 100%;
  height: 150px;
  object-fit: cover;
}

.review-images span {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 9px;
  color: var(--muted);
  font-size: 9px;
}

.review-images span :deep(svg) {
  width: 10px;
  height: 10px;
}

.review-rules {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.review-rule {
  padding: 9px 11px;
  border: 1px solid #edf1f7;
  border-radius: 9px;
  background: #fff;
}

.review-rule.hit {
  border-color: #f0dcb3;
  background: #fffdf8;
}

.review-rule-head {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--muted);
  font-size: 10px;
}

.rule-flag {
  padding: 1px 5px;
  border-radius: 4px;
  background: #f1f5f9;
  color: #94a3b8;
  font-family: var(--mono);
  font-size: 8px;
  font-weight: 650;
}

.review-rule.hit .rule-flag {
  background: #fef3c7;
  color: #b45309;
}

.review-rule-head strong {
  flex: 1;
  color: var(--text);
  font-size: 10.5px;
  font-weight: 550;
}

.rule-detail-list {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed #f2f5f9;
}

.rule-detail-list span {
  display: block;
  padding: 2px 0;
  color: var(--muted-light);
  font-size: 8.5px;
}

.rule-detail-list code {
  color: #475569;
  font-family: var(--mono);
}

.rule-detail-list em {
  color: #64748b;
  font-style: normal;
}

.review-analysis {
  min-width: 0;
  padding-left: 20px;
  border-left: 1px solid #f0f3f8;
}

.category-card {
  margin-bottom: 10px;
  padding: 12px 14px;
  border: 1px solid #e7dcff;
  border-radius: 12px;
  background: #fbf8ff;
}

.category-label {
  display: block;
  color: #8a79ae;
  font-size: 9px;
}

.category-card strong {
  display: block;
  margin-top: 3px;
  color: #4c357f;
  font-size: 14px;
  font-weight: 650;
}

.category-card code {
  display: inline-block;
  margin-top: 4px;
  padding: 2px 6px;
  border-radius: 5px;
  background: #f1eaff;
  color: #6d4fc4;
  font-family: var(--mono);
  font-size: 8.5px;
}

.score-card {
  padding: 14px;
  border: 1px solid #f5e5c5;
  border-radius: 12px;
  background: #fffdf7;
}

.score-label {
  color: #a16207;
  font-size: 9.5px;
}

.score-number {
  margin-top: 2px;
  color: #b45309;
  font-family: var(--mono);
  font-size: 30px;
  font-weight: 700;
  letter-spacing: -0.04em;
}

.score-number small {
  color: var(--muted-light);
  font-size: 10px;
  font-weight: 400;
}

.score-number.high {
  color: var(--red);
}
.score-number.low {
  color: var(--green);
}

.score-track {
  height: 4px;
  margin: 7px 0 8px;
  overflow: hidden;
  border-radius: 99px;
  background: #f4ead6;
}

.score-track span {
  display: block;
  height: 100%;
  border-radius: 99px;
  background: linear-gradient(90deg, #fbbf24, #f59e0b);
}

.score-note {
  color: #8a6d3b;
  font-size: 9.5px;
  line-height: 1.6;
}

.analysis-title {
  margin: 16px 0 7px;
  color: var(--muted-light);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.tag-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
}

.tag-cloud span {
  padding: 3px 8px;
  border-radius: 6px;
  background: #f1f5f9;
  color: #475569;
  font-size: 9.5px;
}

.analysis-reasons {
  margin: 0;
  padding-left: 16px;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.7;
}

.analysis-reasons li {
  margin-bottom: 4px;
}

.analysis-meta {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-top: 16px;
  padding-top: 13px;
  border-top: 1px solid #f0f3f8;
}

.analysis-meta span {
  display: block;
  color: var(--muted-light);
  font-size: 8.5px;
}

.analysis-meta strong {
  display: block;
  overflow: hidden;
  margin-top: 1px;
  color: var(--text);
  font-size: 9.5px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.review-context-block {
  margin: 0 20px 14px;
}

.review-action-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.review-action-head strong {
  display: block;
  color: var(--ink);
  font-size: 12px;
  font-weight: 650;
}

.review-action-head span {
  display: block;
  margin-top: 2px;
  color: var(--muted);
  font-size: 9.5px;
}

.review-action-bar {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 12px;
  padding: 12px 18px;
  border-top: 1px solid #f1e7d4;
  border-radius: 0 0 18px 18px;
  background: rgba(255, 253, 248, 0.96);
  backdrop-filter: blur(10px);
  box-shadow: 0 -8px 24px -16px rgba(15, 23, 42, 0.2);
}

.review-form {
  display: flex;
  flex: 1;
  min-width: 0;
  gap: 8px;
}

.review-buttons {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.review-buttons :deep(.n-button) {
  height: 38px;
}

@container (max-width: 780px) {
  .workbench-body {
    grid-template-columns: 1fr;
    gap: 16px;
  }

  .review-analysis {
    padding-left: 0;
    padding-top: 14px;
    border-left: 0;
    border-top: 1px solid #f0f3f8;
  }
}

@container (max-width: 620px) {
  .review-action-bar {
    align-items: stretch;
    flex-direction: column;
    gap: 9px;
  }

  .review-buttons {
    justify-content: flex-end;
  }
}

@media (max-width: 1200px) {
  .review-layout {
    grid-template-columns: 330px minmax(0, 1fr);
  }

  .workbench-body {
    grid-template-columns: 1fr;
  }

  .review-analysis {
    padding-left: 0;
    border-left: 0;
    border-top: 1px solid #f0f3f8;
    padding-top: 14px;
  }
}

@media (max-width: 900px) {
  .review-layout {
    grid-template-columns: 1fr;
    height: auto;
  }

  .review-queue,
  .review-workbench {
    height: auto;
  }

  .queue-list {
    max-height: 320px;
  }

  .workbench-scroll {
    overflow: visible;
  }

  .review-workbench :deep(.empty-state) {
    height: auto;
  }

  .review-banner {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .review-audit-chips {
    width: 100%;
    margin-left: 0;
  }

  .review-action-bar {
    align-items: stretch;
    flex-direction: column;
  }

  .review-form {
    flex-wrap: wrap;
  }

  .review-form :deep(.n-input) {
    width: 100% !important;
  }

  .review-buttons {
    justify-content: flex-end;
  }
}
</style>
