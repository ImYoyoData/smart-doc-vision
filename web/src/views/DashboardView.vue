<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NSelect, useMessage } from 'naive-ui'
import {
  AlertCircleOutline,
  ArrowForwardOutline,
  BarChartOutline,
  CheckmarkCircleOutline,
  CloudOutline,
  FlashOutline,
  RefreshOutline,
  ScanOutline,
  ShieldCheckmarkOutline,
  SparklesOutline,
  TimeOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import MetricCard from '@/components/MetricCard.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import TaskSubmitModal from '@/components/TaskSubmitModal.vue'
import { analyticsApi, auditApi, providerApi, taskApi } from '@/api'
import type { AnalyticsData, Audit, Provider, Task } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const message = useMessage()
const workspace = useWorkspaceStore()
const auth = useAuthStore()
const loading = ref(false)
const analytics = ref<AnalyticsData | null>(null)
const audits = ref<Audit[]>([])
const recentTasks = ref<Task[]>([])
const providers = ref<Provider[]>([])
const projectFilter = ref(workspace.currentProjectId || 0)
const auditFilter = ref(0)
const submitShow = ref(false)

const projectOptions = computed(() => [
  ...(auth.user?.role === 'SUPER_ADMIN' ? [{ label: '全部项目', value: 0 }] : []),
  ...workspace.projects.map((p) => ({ label: p.name, value: p.id })),
])
const auditOptions = computed(() => [
  { label: '全部审核配置', value: 0 },
  ...audits.value.map((a) => ({
    label: projectFilter.value === 0 && a.project_name ? `${a.project_name} · ${a.name}` : a.name,
    value: a.id,
  })),
])
const overview = computed(() => analytics.value?.overview)
const pendingTasks = computed(() => analytics.value?.pending_tasks || [])
const todayTotal = computed(() => overview.value?.total || 0)
const reviewAndReject = computed(() => (overview.value?.review || 0) + (overview.value?.reject || 0))

function formatDuration(ms?: number | null) {
  if (!ms) return '—'
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`
}

function formatNumber(n?: number | null) {
  if (n === null || n === undefined) return '—'
  if (n >= 10000) return `${(n / 10000).toFixed(1)}w`
  return n.toLocaleString('zh-CN')
}

function sourceLabel(source?: string) {
  return ({ ADMIN: '后台提交', OPENAPI: '开放 API', REDIS: 'Redis', WEBSOCKET: 'WebSocket', POLLING: '定时轮询' } as Record<string, string>)[source || ''] || source || '—'
}

function triggerLabel(type: string) {
  return ({ OPENAPI: 'API', REDIS: 'Redis', WEBSOCKET: 'WS', POLLING: '轮询' } as Record<string, string>)[type] || type
}

async function loadAudits() {
  try {
    const data = projectFilter.value
      ? await auditApi.list({ project_id: projectFilter.value, page: 1, page_size: 100 })
      : await auditApi.list({ page: 1, page_size: 100 })
    audits.value = data.items
  } catch {
    audits.value = []
  }
}

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { range: 'today' }
    if (projectFilter.value) params.project_id = projectFilter.value
    if (auditFilter.value) params.audit_id = auditFilter.value
    const taskParams: Record<string, unknown> = { page: 1, page_size: 7 }
    if (projectFilter.value) taskParams.project_id = projectFilter.value
    if (auditFilter.value) taskParams.audit_id = auditFilter.value
    const providerParams: Record<string, unknown> = {}
    if (projectFilter.value) providerParams.project_id = projectFilter.value
    const [analyticsData, taskData, providerData] = await Promise.all([
      analyticsApi.get(params),
      taskApi.list(taskParams),
      providerApi.list(providerParams),
    ])
    analytics.value = analyticsData
    recentTasks.value = taskData.items
    providers.value = providerData.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载工作台失败')
  } finally {
    loading.value = false
  }
}

async function onProjectChange() {
  auditFilter.value = 0
  await Promise.all([loadAudits(), load()])
}

function goReport() {
  const query: Record<string, string> = {}
  if (projectFilter.value) query.project_id = String(projectFilter.value)
  if (auditFilter.value) query.audit_id = String(auditFilter.value)
  router.push({ name: 'analytics', query })
}

function goReview() {
  router.push({ name: 'reviews' })
}

function goTasks(taskId?: number) {
  router.push({ name: 'tasks', query: taskId ? { task: String(taskId) } : undefined })
}

function goNewAudit() {
  router.push({ name: 'audits', query: { create: '1' } })
}

function goCredentials() {
  router.push({ name: 'credentials' })
}

function openSubmit() {
  if (!projectFilter.value && !workspace.currentProjectId) {
    message.warning('请先创建项目')
    return
  }
  submitShow.value = true
}

function auditReport(audit: Audit) {
  router.push({ name: 'analytics', query: { project_id: String(audit.project_id), audit_id: String(audit.id) } })
}

watch(projectFilter, onProjectChange)
watch(auditFilter, load)

onMounted(async () => {
  await Promise.all([loadAudits(), load()])
})
</script>

<template>
  <div class="page dashboard-page">
    <PageHeader
      eyebrow="WORKBENCH"
      title="审核工作台"
      subtitle="聚焦今天要处理的任务：人工复核、最近任务、审核配置与模型状态。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" title="刷新" aria-label="刷新" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button secondary @click="goReport">
          <template #icon><BarChartOutline /></template>
          数据报表
        </n-button>
        <n-button v-if="auth.can('audit:write')" secondary @click="goNewAudit">
          <template #icon><ShieldCheckmarkOutline /></template>
          新建审核配置
        </n-button>
        <n-button v-if="auth.can('task:submit')" type="primary" @click="openSubmit">
          <template #icon><FlashOutline /></template>
          发起审核
        </n-button>
      </template>
    </PageHeader>

    <section class="filter-bar">
      <div class="filter-item"><span>项目</span><n-select v-model:value="projectFilter" :options="projectOptions" style="width: 195px" /></div>
      <div class="filter-item"><span>审核配置</span><n-select v-model:value="auditFilter" :options="auditOptions" filterable style="width: 225px" /></div>
      <span class="filter-note">今日运营视图 · 详细图表请前往数据报表</span>
      <n-button text type="primary" class="report-link" @click="goReport">打开数据报表 <ArrowForwardOutline /></n-button>
    </section>

    <section class="hero">
      <div class="hero-main">
        <div class="hero-kicker"><span></span>TODAY</div>
        <h2>今日已处理 <strong>{{ formatNumber(todayTotal) }}</strong> 份图档</h2>
        <p>支持多文本 + 多图片审核，所有触发来源统一任务状态、人工复核和结果回调。</p>
      </div>
      <div class="hero-stats">
        <div><span>已判定通过率</span><strong>{{ overview?.pass_rate?.toFixed(1) ?? '—' }}<i>%</i></strong></div>
        <div><span>平均耗时</span><strong>{{ formatDuration(overview?.avg_duration_ms) }}</strong></div>
        <div><span>启用审核</span><strong>{{ audits.filter((a) => a.status === 'ACTIVE').length }}</strong></div>
      </div>
    </section>

    <section v-if="overview?.pending && auth.canMenu('reviews')" class="review-alert">
      <span class="review-alert-icon"><AlertCircleOutline /></span>
      <div>
        <strong>有 {{ overview.pending }} 条图档等待人工复核</strong>
        <p>进入复核工作台确认后，系统会按最终 PASS / REJECT 执行结果回调。</p>
      </div>
      <button class="review-alert-btn" type="button" @click="goReview">进入复核工作台 <ArrowForwardOutline /></button>
    </section>

    <div class="metric-grid">
      <MetricCard label="今日任务" :value="formatNumber(todayTotal)" hint="今日提交并受理" tone="blue" :loading="loading">
        <template #icon><ScanOutline /></template>
      </MetricCard>
      <MetricCard label="今日通过率" :value="overview ? overview.pass_rate.toFixed(1) : '—'" suffix="%" :hint="`通过 ${overview?.pass ?? 0} 条`" tone="green" :loading="loading">
        <template #icon><CheckmarkCircleOutline /></template>
      </MetricCard>
      <MetricCard label="不通过 / 待议" :value="reviewAndReject" :hint="overview ? `驳回 ${overview.reject} · 待议 ${overview.review}` : ''" tone="amber" :loading="loading">
        <template #icon><WarningOutline /></template>
      </MetricCard>
      <MetricCard label="待人工复核" :value="formatNumber(overview?.pending)" hint="点击进入复核工作台" tone="red" :loading="loading" @click="goReview">
        <template #icon><TimeOutline /></template>
      </MetricCard>
    </div>

    <div class="ops-grid">
      <section class="panel review-panel">
        <div class="panel-head">
          <div><h3 class="panel-title">待人工复核队列</h3><p class="panel-subtitle">按等待时间升序，复核后自动回调业务方</p></div>
          <n-button type="warning" secondary size="small" @click="goReview">进入复核工作台</n-button>
        </div>
        <div v-if="pendingTasks.length" class="queue-list">
          <button v-for="task in pendingTasks.slice(0, 5)" :key="task.id" type="button" class="queue-row" @click="goReview">
            <span class="queue-bar"></span>
            <span class="queue-main">
              <span class="queue-title"><span class="mono">{{ task.task_no }}</span><StatusTag status="REVIEW_PENDING" type="task" /></span>
              <span class="queue-sub">{{ task.audit_name }} · {{ sourceLabel(task.source_type) }} · {{ task.biz_key || '无业务键' }}</span>
            </span>
            <span class="queue-age">{{ task.created_at?.replace('T', ' ').slice(5, 16) }}</span>
            <ArrowForwardOutline class="row-arrow" />
          </button>
        </div>
        <EmptyState v-else compact title="暂无待复核任务" description="当前筛选范围内没有待议任务。" />
        <div class="shortcut-row">
          <button type="button" @click="goTasks()"><span><ScanOutline /></span><div><strong>全部任务</strong><small>查看状态、重试与详情</small></div></button>
          <button type="button" @click="goReport()"><span><BarChartOutline /></span><div><strong>未通过原因</strong><small>查看完整数据报表</small></div></button>
          <button type="button" @click="goCredentials"><span><ShieldCheckmarkOutline /></span><div><strong>接入凭证</strong><small>管理项目 AppKey 与授权范围</small></div></button>
        </div>
      </section>

      <section class="panel recent-panel">
        <div class="panel-head">
          <div><h3 class="panel-title">最近任务</h3><p class="panel-subtitle">最新提交与处理状态</p></div>
          <n-button text type="primary" @click="goTasks()">全部任务</n-button>
        </div>
        <div class="recent-list">
          <button v-for="task in recentTasks" :key="task.id" type="button" class="recent-row" @click="goTasks(task.id)">
            <span class="recent-icon" :class="task.result.toLowerCase()"><ScanOutline /></span>
            <span class="recent-main">
              <span class="recent-title"><span class="mono">{{ task.task_no }}</span><StatusTag :status="task.status" type="task" /></span>
              <span class="recent-sub">{{ task.audit_name }} · {{ sourceLabel(task.source_type) }} · {{ task.created_at?.replace('T', ' ').slice(5, 16) }}</span>
            </span>
            <StatusTag :status="task.result" type="result" />
          </button>
          <div v-if="!recentTasks.length" class="recent-empty">暂无任务</div>
        </div>
      </section>
    </div>

    <div class="ops-grid lower">
      <section class="panel audit-panel">
        <div class="panel-head">
          <div><h3 class="panel-title">审核配置快捷入口</h3><p class="panel-subtitle">已开启的触发方式与今日任务量</p></div>
          <n-button text type="primary" @click="router.push({ name: 'audits' })">管理审核配置</n-button>
        </div>
        <div class="audit-list">
          <div v-for="audit in audits.slice(0, 5)" :key="audit.id" class="audit-row">
            <span class="audit-icon"><ShieldCheckmarkOutline /></span>
            <div class="audit-main">
              <div class="audit-title-row">
                <strong>{{ audit.name }}</strong>
                <StatusTag :status="audit.status" type="audit" />
                <span v-for="trigger in audit.active_triggers || []" :key="trigger" class="trigger-mini">{{ triggerLabel(trigger) }}</span>
              </div>
              <span class="audit-sub">{{ audit.code }} · {{ audit.provider_name || '未绑定模型' }} · 今日 {{ audit.today_tasks || 0 }} 条</span>
            </div>
            <div class="audit-actions">
              <n-button size="tiny" quaternary @click="auditReport(audit)">报表</n-button>
              <n-button v-if="auth.can('audit:write')" size="tiny" secondary @click="router.push({ name: 'audits', query: { create: '1' } })">接入</n-button>
            </div>
          </div>
          <div v-if="!audits.length" class="recent-empty">还没有审核配置</div>
        </div>
      </section>

      <section class="panel provider-panel">
        <div class="panel-head">
          <div><h3 class="panel-title">模型状态</h3><p class="panel-subtitle">通用大模型与 JEV 多模态</p></div>
          <n-button text type="primary" @click="router.push({ name: 'providers' })">管理</n-button>
        </div>
        <div class="provider-list">
          <div v-for="provider in providers.slice(0, 4)" :key="provider.id" class="provider-row">
            <span class="provider-icon" :class="{ jev: provider.kind === 'JEV' }">
              <SparklesOutline v-if="provider.kind === 'JEV'" />
              <CloudOutline v-else />
            </span>
            <div class="provider-copy"><strong>{{ provider.name }}</strong><span class="mono">{{ provider.model }}</span></div>
            <div class="provider-state">
              <StatusTag :status="provider.status" type="provider" />
              <span class="test-note" :class="{ ok: provider.last_test_ok }">{{ provider.last_test_at ? (provider.last_test_ok ? `连通 ${provider.last_test_ms}ms` : '测试失败') : '未测试' }}</span>
            </div>
          </div>
          <div v-if="!providers.length" class="recent-empty">暂无模型接入</div>
        </div>
        <div class="provider-tip"><SparklesOutline /><span>仅支持多文本 + 多图片，不支持视频/音频。JEV 原生多模态，通用模型需开启图片理解。</span></div>
      </section>
    </div>

    <TaskSubmitModal v-model:show="submitShow" :project-id="projectFilter || workspace.currentProjectId" @created="load" />
  </div>
</template>

<style scoped>
.filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 14px;
  margin-bottom: 14px;
  padding: 11px 15px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.filter-item {
  display: flex;
  align-items: center;
  gap: 7px;
}

.filter-item > span {
  color: var(--muted-light);
  font-size: 10px;
}

.filter-note {
  color: var(--muted-light);
  font-size: 9.5px;
}

.report-link {
  margin-left: auto;
}

.report-link :deep(svg) {
  width: 11px;
  height: 11px;
  vertical-align: -1px;
}

.hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 28px;
  margin-bottom: 14px;
  padding: 20px 24px;
  border: 1px solid #dce7fb;
  border-radius: 16px;
  background: radial-gradient(700px 180px at 90% 10%, rgba(43, 108, 246, 0.09), transparent 70%), linear-gradient(120deg, #f7faff, #fdfefe 68%);
}

.hero-kicker {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 6px;
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.12em;
}

.hero-kicker span {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--brand);
  box-shadow: 0 0 0 3px rgba(43, 108, 246, 0.1);
}

.hero h2 {
  margin: 0;
  color: var(--ink);
  font-size: 17px;
  font-weight: 550;
}

.hero h2 strong {
  margin: 0 3px;
  color: var(--brand-strong);
  font-family: var(--mono);
  font-size: 25px;
  font-weight: 700;
}

.hero p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 11px;
}

.hero-stats {
  display: flex;
  flex-shrink: 0;
}

.hero-stats > div {
  min-width: 105px;
  padding: 0 20px;
  border-left: 1px solid #e1e9f6;
}

.hero-stats span {
  display: block;
  color: var(--muted-light);
  font-size: 9.5px;
}

.hero-stats strong {
  display: block;
  margin-top: 2px;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 18px;
  font-weight: 650;
}

.hero-stats i {
  color: var(--muted);
  font-size: 10px;
  font-style: normal;
}

.review-alert {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  padding: 13px 16px;
  border: 1px solid #f2ddb2;
  border-radius: 12px;
  background: linear-gradient(120deg, #fffbf2, #fffefa);
}

.review-alert-icon {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: #fef3c7;
  color: #b45309;
  flex-shrink: 0;
}

.review-alert-icon :deep(svg) {
  width: 16px;
  height: 16px;
}

.review-alert > div {
  flex: 1;
}

.review-alert strong {
  display: block;
  color: #78350f;
  font-size: 11.5px;
  font-weight: 600;
}

.review-alert p {
  margin: 2px 0 0;
  color: #a16207;
  font-size: 9.5px;
}

.review-alert-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 7px 11px;
  border: 1px solid #eecf94;
  border-radius: 8px;
  background: #fff9ed;
  color: #a16207;
  font-size: 10px;
  font-weight: 550;
  cursor: pointer;
  flex-shrink: 0;
}

.review-alert-btn :deep(svg) {
  width: 11px;
  height: 11px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.ops-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(320px, 0.9fr);
  gap: 14px;
  margin-bottom: 14px;
}

.ops-grid.lower {
  grid-template-columns: minmax(0, 1.25fr) minmax(320px, 0.9fr);
}

.queue-list {
  padding: 8px 12px;
}

.queue-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 10px 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.queue-row:hover {
  background: var(--amber-soft-2);
}

.queue-bar {
  width: 3px;
  height: 30px;
  border-radius: 3px;
  background: #f4c96b;
  flex-shrink: 0;
}

.queue-main {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.queue-title {
  display: flex;
  align-items: center;
  gap: 7px;
}

.queue-title .mono {
  color: var(--text);
  font-size: 9.5px;
}

.queue-sub {
  overflow: hidden;
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.queue-age {
  color: #b45309;
  font-family: var(--mono);
  font-size: 8.5px;
  flex-shrink: 0;
}

.row-arrow {
  width: 11px;
  height: 11px;
  color: var(--muted-light);
  flex-shrink: 0;
}

.shortcut-row {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 6px;
  margin: 6px 14px 15px;
  padding-top: 13px;
  border-top: 1px dashed #edf1f7;
}

.shortcut-row button {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 8px;
  border: 1px solid #edf1f7;
  border-radius: 8px;
  background: #fbfcfe;
  text-align: left;
  cursor: pointer;
}

.shortcut-row button:hover {
  border-color: #c6d7f8;
  background: #f8fbff;
}

.shortcut-row > button > span {
  display: flex;
  width: 27px;
  height: 27px;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.shortcut-row > button > span :deep(svg) {
  width: 13px;
  height: 13px;
}

.shortcut-row strong {
  display: block;
  color: var(--text);
  font-size: 9.5px;
  font-weight: 550;
}

.shortcut-row small {
  display: block;
  color: var(--muted-light);
  font-size: 8px;
}

.recent-list {
  padding: 6px 12px 13px;
}

.recent-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 9px;
  padding: 8px 6px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.recent-row:hover {
  background: var(--brand-softer);
}

.recent-icon {
  display: flex;
  width: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.recent-icon.reject {
  background: var(--red-soft);
  color: var(--red);
}

.recent-icon.review {
  background: var(--amber-soft);
  color: #d97706;
}

.recent-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.recent-main {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.recent-title {
  display: flex;
  align-items: center;
  gap: 7px;
}

.recent-title .mono {
  color: var(--text);
  font-size: 9.5px;
}

.recent-sub {
  overflow: hidden;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 8.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-empty {
  padding: 40px 10px;
  color: var(--muted-light);
  font-size: 10px;
  text-align: center;
}

.audit-list {
  padding: 6px 13px 13px;
}

.audit-row {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 10px 4px;
  border-bottom: 1px solid #f3f5f9;
}

.audit-row:last-child {
  border-bottom: 0;
}

.audit-icon {
  display: flex;
  width: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.audit-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.audit-main {
  flex: 1;
  min-width: 0;
}

.audit-title-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
}

.audit-title-row strong {
  color: var(--text);
  font-size: 10.5px;
  font-weight: 550;
}

.trigger-mini {
  padding: 1px 4px;
  border-radius: 3px;
  background: #f2f7ff;
  color: #4a72bf;
  font-family: var(--mono);
  font-size: 7px;
}

.audit-sub {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.audit-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.provider-list {
  padding: 6px 13px 10px;
}

.provider-row {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 3px;
  border-bottom: 1px solid #f3f5f9;
}

.provider-row:last-child {
  border-bottom: 0;
}

.provider-icon {
  display: flex;
  width: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
}

.provider-icon.jev {
  background: var(--purple-soft);
  color: var(--purple);
}

.provider-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.provider-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.provider-copy strong {
  color: var(--text);
  font-size: 10px;
  font-weight: 550;
}

.provider-copy span {
  overflow: hidden;
  color: var(--muted-light);
  font-size: 8.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-state {
  display: flex;
  align-items: flex-end;
  flex-direction: column;
  gap: 2px;
}

.test-note {
  color: var(--muted-light);
  font-size: 8px;
}

.test-note.ok {
  color: #15803d;
}

.provider-tip {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin: 0 16px 15px;
  padding: 8px 10px;
  border-radius: 7px;
  background: #f8fafc;
  color: var(--muted-light);
  font-size: 8.5px;
  line-height: 1.6;
}

.provider-tip :deep(svg) {
  width: 12px;
  height: 12px;
  margin-top: 1px;
  flex-shrink: 0;
}

@media (max-width: 1150px) {
  .metric-grid {
    grid-template-columns: 1fr 1fr;
  }

  .ops-grid,
  .ops-grid.lower {
    grid-template-columns: 1fr;
  }

  .hero {
    align-items: flex-start;
    flex-direction: column;
  }

  .hero-stats {
    width: 100%;
  }

  .hero-stats > div {
    flex: 1;
    min-width: 0;
    padding: 0 14px;
  }
}

@media (max-width: 650px) {
  .metric-grid {
    grid-template-columns: 1fr;
  }

  .filter-bar {
    align-items: stretch;
    flex-direction: column;
  }

  .filter-item {
    width: 100%;
    min-width: 0;
  }

  .filter-item > span {
    white-space: nowrap;
    flex-shrink: 0;
  }

  .filter-item :deep(.n-select) {
    width: auto !important;
    min-width: 0;
    flex: 1;
  }

  .filter-note {
    width: 100%;
  }

  .report-link {
    margin-left: 0;
  }

  .shortcut-row {
    grid-template-columns: 1fr;
  }

  .review-alert {
    flex-wrap: wrap;
  }

  .review-alert-btn {
    width: 100%;
    justify-content: center;
  }

  .queue-age {
    display: none;
  }
}
</style>
