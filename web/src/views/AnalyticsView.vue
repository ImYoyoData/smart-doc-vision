<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NSelect, useMessage } from 'naive-ui'
import type { EChartsOption } from 'echarts'
import {
  BarChartOutline,
  CheckmarkCircleOutline,
  ChevronForwardOutline,
  CloseCircleOutline,
  RefreshOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import MetricCard from '@/components/MetricCard.vue'
import EChart from '@/components/EChart.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusTag from '@/components/StatusTag.vue'
import { analyticsApi, auditApi } from '@/api'
import type { AnalyticsData, Audit, NameValue } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'

const workspace = useWorkspaceStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const message = useMessage()
const data = ref<AnalyticsData | null>(null)
const audits = ref<Audit[]>([])
const loading = ref(false)
const initialProject = Number(route.query.project_id || 0) || workspace.currentProjectId || 0
const projectFilter = ref(initialProject)
const auditFilter = ref(Number(route.query.audit_id || 0) || 0)
const range = ref('7d')
const reasonMode = ref<'labels' | 'reasons' | 'rules' | 'errors'>('labels')

const reasonModes = [
  { key: 'labels', label: '风险标签' },
  { key: 'reasons', label: '具体原因' },
  { key: 'rules', label: '命中规则' },
  { key: 'errors', label: '执行错误' },
] as const

const projectOptions = computed(() => [
  ...(auth.user?.role === 'SUPER_ADMIN' ? [{ label: '全部项目', value: 0 }] : []),
  ...workspace.projects.map((p) => ({ label: p.name, value: p.id })),
])
const auditOptions = computed(() => [
  { label: '全部审核配置', value: 0 },
  ...audits.value.map((a) => ({ label: a.name, value: a.id })),
])
const overview = computed(() => data.value?.overview)
const hasData = computed(() => (overview.value?.total || 0) > 0)
const reasonData = computed<NameValue[]>(() => {
  const reasons = data.value?.reasons
  if (!reasons) return []
  return reasons[reasonMode.value] || []
})
const reasonModeLabel = computed(
  () => reasonModes.find((m) => m.key === reasonMode.value)?.label || '原因',
)

const axisStyle = {
  axisLine: { lineStyle: { color: '#e8edf5' } },
  axisTick: { show: false },
  axisLabel: { color: '#94a3b8', fontSize: 10 },
  splitLine: { lineStyle: { color: '#f1f4f9' } },
}

const trendOption = computed<EChartsOption>(() => {
  const trend = data.value?.trend || []
  return {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    legend: {
      right: 0,
      top: 0,
      itemWidth: 8,
      itemHeight: 8,
      textStyle: { color: '#64748b', fontSize: 10 },
      data: ['通过', '不通过', '待议', '失败'],
    },
    grid: { left: 4, right: 8, top: 34, bottom: 4, containLabel: true },
    xAxis: { type: 'category', data: trend.map((p) => p.label), ...axisStyle },
    yAxis: { type: 'value', minInterval: 1, ...axisStyle },
    series: [
      { name: '通过', type: 'bar', stack: 'total', data: trend.map((p) => p.pass), barMaxWidth: 24, itemStyle: { color: '#34c27d', borderRadius: [0, 0, 0, 0] } },
      { name: '不通过', type: 'bar', stack: 'total', data: trend.map((p) => p.reject), itemStyle: { color: '#f16b6b' } },
      { name: '待议', type: 'bar', stack: 'total', data: trend.map((p) => p.review), itemStyle: { color: '#f7bd54' } },
      { name: '失败', type: 'bar', stack: 'total', data: trend.map((p) => p.failed), itemStyle: { color: '#cbd5e1', borderRadius: [4, 4, 0, 0] } },
    ],
  }
})

const distributionOption = computed<EChartsOption>(() => {
  const dist = data.value?.distribution || []
  const color: Record<string, string> = { PASS: '#28b875', REJECT: '#ef6464', REVIEW: '#f3b544', FAILED: '#cbd5e1' }
  return {
    tooltip: { trigger: 'item' },
    series: [
      {
        type: 'pie',
        radius: ['57%', '78%'],
        center: ['50%', '46%'],
        avoidLabelOverlap: true,
        itemStyle: { borderColor: '#fff', borderWidth: 3, borderRadius: 5 },
        label: { show: false },
        data: dist.map((d) => ({ name: d.name, value: d.value, itemStyle: { color: color[d.key] || '#94a3b8' } })),
      },
    ],
    graphic: [
      {
        type: 'text',
        left: 'center',
        top: '37%',
        style: {
          text: String(overview.value?.total || 0),
          fill: '#0f172a',
          fontSize: 24,
          fontWeight: 700,
          fontFamily: 'monospace',
          textAlign: 'center',
        },
      },
      {
        type: 'text',
        left: 'center',
        top: '51%',
        style: { text: '审核总量', fill: '#94a3b8', fontSize: 10, textAlign: 'center' },
      },
    ],
  }
})

const auditBarOption = computed<EChartsOption>(() => {
  const rows = (data.value?.by_audit || []).slice(0, 8)
  return {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 4, right: 34, top: 14, bottom: 4, containLabel: true },
    xAxis: { type: 'value', minInterval: 1, ...axisStyle },
    yAxis: {
      type: 'category',
      data: rows.map((r) => r.audit_name).reverse(),
      ...axisStyle,
      axisLabel: { color: '#64748b', fontSize: 10, width: 110, overflow: 'truncate' },
    },
    series: [
      {
        name: '任务量',
        type: 'bar',
        data: rows.map((r) => r.total).reverse(),
        barMaxWidth: 16,
        itemStyle: { color: '#a8c4fa', borderRadius: [0, 6, 6, 0] },
        label: { show: true, position: 'right', color: '#64748b', fontSize: 9 },
      },
    ],
  }
})

const passRateOption = computed<EChartsOption>(() => {
  const rows = (data.value?.by_audit || []).slice(0, 8)
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 4, right: 12, top: 14, bottom: 4, containLabel: true },
    xAxis: { type: 'category', data: rows.map((r) => r.audit_name), ...axisStyle, axisLabel: { color: '#94a3b8', fontSize: 9, rotate: 18, width: 90, overflow: 'truncate' } },
    yAxis: { type: 'value', max: 100, ...axisStyle, axisLabel: { formatter: '{value}%', color: '#94a3b8', fontSize: 9 } },
    series: [
      {
        type: 'line',
        smooth: true,
        symbolSize: 6,
        data: rows.map((r) => r.pass_rate),
        lineStyle: { width: 2, color: '#2b6cf6' },
        itemStyle: { color: '#2b6cf6' },
        areaStyle: { color: 'rgba(43,108,246,.08)' },
        label: { show: true, formatter: '{c}%', color: '#2b6cf6', fontSize: 9 },
      },
    ],
  }
})

const reasonOption = computed<EChartsOption>(() => {
  const items = reasonData.value.slice(0, 8)
  return {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 4, right: 36, top: 8, bottom: 4, containLabel: true },
    xAxis: { type: 'value', minInterval: 1, ...axisStyle },
    yAxis: {
      type: 'category',
      data: items.map((i) => i.name).reverse(),
      ...axisStyle,
      axisLabel: { color: '#64748b', fontSize: 9.5, width: 170, overflow: 'truncate' },
    },
    series: [
      {
        type: 'bar',
        data: items.map((i) => i.value).reverse(),
        barMaxWidth: 14,
        itemStyle: {
          borderRadius: [0, 5, 5, 0],
          color: reasonMode.value === 'errors' ? '#ef8b8b' : reasonMode.value === 'rules' ? '#b79cf5' : '#f3b544',
        },
        label: { show: true, position: 'right', color: '#94a3b8', fontSize: 9 },
      },
    ],
  }
})

const categoryOption = computed<EChartsOption>(() => {
  const rows = [...(data.value?.categories || [])].reverse()
  return {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 4, right: 22, top: 8, bottom: 4, containLabel: true },
    xAxis: { type: 'value', minInterval: 1, ...axisStyle },
    yAxis: { type: 'category', data: rows.map((r) => r.name), ...axisStyle, axisLabel: { color: '#64748b', fontSize: 10, width: 110, overflow: 'truncate' } },
    series: [
      {
        type: 'bar',
        data: rows.map((r) => r.value),
        barMaxWidth: 22,
        itemStyle: { color: '#a58af2', borderRadius: [0, 5, 5, 0] },
        label: { show: true, position: 'right', color: '#94a3b8', fontSize: 9 },
      },
    ],
  }
})

const sourceOption = computed<EChartsOption>(() => {
  const source = (data.value?.sources || []).map((item) => ({ ...item, name: sourceLabel(item.name) }))
  const media = data.value?.media_types || []
  return {
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 0, textStyle: { color: '#64748b', fontSize: 10 }, itemWidth: 8, itemHeight: 8 },
    grid: { left: 4, right: 8, top: 30, bottom: 4, containLabel: true },
    xAxis: { type: 'category', data: [...source.map((s) => s.name), ...media.map((m) => m.name)], ...axisStyle },
    yAxis: { type: 'value', minInterval: 1, ...axisStyle },
    series: [
      {
        type: 'bar',
        barMaxWidth: 20,
        data: [
          ...source.map((s) => ({ value: s.value, itemStyle: { color: '#7aa6f9' } })),
          ...media.map((m) => ({ value: m.value, itemStyle: { color: '#b79cf5' } })),
        ],
        itemStyle: { borderRadius: [5, 5, 0, 0] },
        label: { show: true, position: 'top', color: '#94a3b8', fontSize: 9 },
      },
    ],
  }
})

const latencyOption = computed<EChartsOption>(() => {
  const items = data.value?.latency_buckets || []
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 4, right: 8, top: 8, bottom: 4, containLabel: true },
    xAxis: { type: 'category', data: items.map((i) => i.name), ...axisStyle },
    yAxis: { type: 'value', minInterval: 1, ...axisStyle },
    series: [
      {
        type: 'bar',
        data: items.map((i) => i.value),
        barMaxWidth: 28,
        itemStyle: { color: '#9cc6b0', borderRadius: [5, 5, 0, 0] },
        label: { show: true, position: 'top', color: '#94a3b8', fontSize: 9 },
      },
    ],
  }
})

function sourceLabel(source?: string) {
  const map: Record<string, string> = {
    ADMIN: '后台提交', OPENAPI: '开放 API', REDIS: 'Redis', WEBSOCKET: 'WebSocket', POLLING: '定时轮询',
  }
  return map[source || ''] || source || '—'
}

function formatDuration(ms?: number | null) {
  if (!ms) return '—'
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`
}

async function loadAudits() {
  try {
    const data = broadProjectFilter() ? await auditApi.list({ page: 1, page_size: 100 }) : await auditApi.list({ project_id: projectFilter.value, page: 1, page_size: 100 })
    audits.value = data.items
  } catch {
    audits.value = []
  }
}

function broadProjectFilter() {
  return !projectFilter.value
}

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { range: range.value }
    if (projectFilter.value) params.project_id = projectFilter.value
    if (auditFilter.value) params.audit_id = auditFilter.value
    data.value = await analyticsApi.get(params)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载报表失败')
  } finally {
    loading.value = false
  }
}

async function applyProject() {
  auditFilter.value = 0
  await Promise.all([loadAudits(), load()])
}

function setRange(value: string) {
  range.value = value
  load()
}

function goReview() {
  router.push({ name: 'reviews' })
}

watch(auditFilter, load)
watch(projectFilter, applyProject)
watch(range, load)

onMounted(async () => {
  await Promise.all([loadAudits(), load()])
})
</script>

<template>
  <div class="page analytics-page">
    <PageHeader
      eyebrow="ANALYTICS"
      title="数据报表"
      subtitle="按项目、审核配置与时间范围分析审核量、通过率、未通过原因、错误与延迟分布。"
    >
      <template #actions>
        <n-button v-if="(overview?.pending || 0) > 0" type="warning" secondary @click="goReview">
          <template #icon><TimeOutline /></template>
          待复核 {{ overview?.pending }}
        </n-button>
        <n-button quaternary circle :loading="loading" title="刷新" aria-label="刷新" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
      </template>
    </PageHeader>

    <section class="filter-panel">
      <div class="filter-group">
        <span>项目</span>
        <n-select v-model:value="projectFilter" :options="projectOptions" style="width: 200px" />
      </div>
      <div class="filter-group">
        <span>审核配置</span>
        <n-select v-model:value="auditFilter" :options="auditOptions" filterable style="width: 210px" />
      </div>
      <div class="range-tabs">
        <button v-for="item in [{ key: 'today', label: '今日' }, { key: '7d', label: '近 7 天' }, { key: '30d', label: '近 30 天' }, { key: '90d', label: '近 90 天' }]" :key="item.key" type="button" :class="{ active: range === item.key }" @click="setRange(item.key)">{{ item.label }}</button>
      </div>
      <span class="filter-note">
        数据范围：{{ data?.range.from?.slice(0, 10) || '—' }} ~ 今日 · 样本 {{ data?.sample_size ?? 0 }} 条
      </span>
    </section>

    <div class="metric-grid">
      <MetricCard label="审核总量" :value="overview?.total ?? 0" :hint="`通过 ${overview?.pass ?? 0} · 驳回 ${overview?.reject ?? 0}`" tone="blue" :loading="loading">
        <template #icon><BarChartOutline /></template>
      </MetricCard>
      <MetricCard label="已判定通过率" :value="overview?.pass_rate ?? 0" suffix="%" :hint="`待议率 ${overview?.review_rate ?? 0}%`" tone="green" :loading="loading">
        <template #icon><CheckmarkCircleOutline /></template>
      </MetricCard>
      <MetricCard label="不通过" :value="overview?.reject ?? 0" :hint="`执行失败 ${overview?.failed ?? 0} 条`" tone="red" :loading="loading">
        <template #icon><CloseCircleOutline /></template>
      </MetricCard>
      <MetricCard label="待人工复核" :value="overview?.pending ?? 0" :hint="`平均耗时 ${formatDuration(overview?.avg_duration_ms)}`" tone="amber" :loading="loading">
        <template #icon><TimeOutline /></template>
      </MetricCard>
    </div>

    <div class="chart-grid main-charts">
      <section class="panel chart-panel wide">
        <div class="panel-head">
          <div><h3 class="panel-title">审核任务趋势</h3><p class="panel-subtitle">按日期堆叠的结论分布</p></div>
        </div>
        <div class="panel-body"><EChart :option="trendOption" height="275px" :loading="loading" /></div>
      </section>
      <section class="panel chart-panel">
        <div class="panel-head"><div><h3 class="panel-title">审核率分布</h3><p class="panel-subtitle">结果构成</p></div></div>
        <div class="panel-body"><EChart :option="distributionOption" height="275px" :loading="loading" /></div>
      </section>
    </div>

    <div class="chart-grid two-charts">
      <section class="panel chart-panel">
        <div class="panel-head"><div><h3 class="panel-title">审核配置任务量</h3><p class="panel-subtitle">任务量最高的审核配置</p></div></div>
        <div class="panel-body"><EChart :option="auditBarOption" height="255px" :loading="loading" /></div>
      </section>
      <section class="panel chart-panel">
        <div class="panel-head"><div><h3 class="panel-title">审核配置通过率</h3><p class="panel-subtitle">按审核配置对比通过率</p></div></div>
        <div class="panel-body"><EChart :option="passRateOption" height="255px" :loading="loading" /></div>
      </section>
    </div>

    <div class="chart-grid two-charts reason-charts">
      <section class="panel chart-panel">
        <div class="panel-head">
          <div><h3 class="panel-title">未通过与待议原因</h3><p class="panel-subtitle">帮助定位主要风险来源</p></div>
          <div class="reason-tabs">
            <button v-for="mode in reasonModes" :key="mode.key" type="button" :class="{ active: reasonMode === mode.key }" @click="reasonMode = mode.key">{{ mode.label }}</button>
          </div>
        </div>
        <div class="panel-body">
          <EChart v-if="reasonData.length" :option="reasonOption" height="250px" :loading="loading" />
          <EmptyState v-else compact :title="`暂无${reasonModeLabel}数据`" description="当前时间范围内没有对应记录。" />
        </div>
      </section>
      <section class="panel chart-panel">
        <div class="panel-head"><div><h3 class="panel-title">触发来源 / 媒体类型</h3><p class="panel-subtitle">不同接入方式和图档类型占比</p></div></div>
        <div class="panel-body"><EChart :option="sourceOption" height="250px" :loading="loading" /></div>
      </section>
    </div>

    <section v-if="(data?.categories?.length || 0) > 0" class="panel category-section">
      <div class="panel-head">
        <div><h3 class="panel-title">分类审核分布</h3><p class="panel-subtitle">分类审核配置的类别命中数量</p></div>
        <span class="category-total">共 {{ data?.categories?.reduce((sum, item) => sum + item.value, 0) || 0 }} 条分类结果</span>
      </div>
      <div class="panel-body"><EChart :option="categoryOption" height="230px" :loading="loading" /></div>
    </section>

    <div class="chart-grid two-charts">
      <section class="panel chart-panel">
        <div class="panel-head"><div><h3 class="panel-title">审核耗时分布</h3><p class="panel-subtitle">端到端处理耗时</p></div></div>
        <div class="panel-body"><EChart :option="latencyOption" height="210px" :loading="loading" /></div>
      </section>
      <section class="panel chart-panel project-chart">
        <div class="panel-head">
          <div><h3 class="panel-title">{{ projectFilter ? '审核配置明细' : '项目维度对比' }}</h3><p class="panel-subtitle">任务量、通过率与待复核</p></div>
          <n-button text type="primary" @click="goReview">进入复核 <ChevronForwardOutline class="arrow" /></n-button>
        </div>
        <div class="panel-body no-pad">
          <div v-if="!projectFilter && data?.by_project?.length" class="mini-table">
            <div class="mini-table-head"><span>项目</span><span>任务量</span><span>通过率</span><span>待复核</span></div>
            <div v-for="p in data.by_project.slice(0, 6)" :key="p.project_id" class="mini-table-row">
              <strong>{{ p.project_name }}</strong>
              <span>{{ p.total }}</span>
              <span class="rate">{{ p.pass_rate }}%</span>
              <span>{{ p.failed || 0 }} 失败</span>
            </div>
          </div>
          <div v-else-if="data?.by_audit?.length" class="mini-table">
            <div class="mini-table-head"><span>审核配置</span><span>任务量</span><span>通过率</span><span>待复核</span></div>
            <div v-for="a in data.by_audit.slice(0, 6)" :key="a.audit_id" class="mini-table-row">
              <strong>{{ a.audit_name }}</strong>
              <span>{{ a.total }}</span>
              <span class="rate">{{ a.pass_rate }}%</span>
              <span>{{ a.pending }} 条</span>
            </div>
          </div>
          <EmptyState v-else compact title="暂无明细" description="当前时间范围内还没有任务数据。" />
        </div>
      </section>
    </div>

    <section v-if="(data?.pending_tasks?.length || 0) > 0" class="panel pending-strip">
      <div class="pending-head">
        <div><h3 class="panel-title">待复核优先队列</h3><p class="panel-subtitle">按等待时间升序，建议优先处理</p></div>
        <n-button type="warning" secondary @click="goReview">全部复核任务</n-button>
      </div>
      <div class="pending-list">
        <button v-for="task in data?.pending_tasks" :key="task.id" type="button" class="pending-item" @click="goReview">
          <span class="mono">{{ task.task_no }}</span>
          <span>{{ task.audit_name }}</span>
          <span>{{ sourceLabel(task.source_type) }}</span>
          <span class="wait">{{ task.created_at?.replace('T', ' ').slice(5, 16) }}</span>
          <StatusTag status="REVIEW_PENDING" type="task" />
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.filter-panel {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
  margin-bottom: 14px;
  padding: 12px 16px;
  border: 1px solid var(--border);
  border-radius: 13px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 7px;
}

.filter-group > span {
  color: var(--muted-light);
  font-size: 10px;
}

.range-tabs {
  display: flex;
  gap: 2px;
  padding: 2px;
  border-radius: 8px;
  background: #f4f7fb;
}

.range-tabs button,
.reason-tabs button {
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  font-size: 10px;
  padding: 5px 9px;
  cursor: pointer;
  white-space: nowrap;
}

.range-tabs button.active {
  background: #fff;
  color: var(--brand);
  box-shadow: var(--shadow-sm);
  font-weight: 550;
}

.filter-note {
  margin-left: auto;
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.chart-grid {
  display: grid;
  gap: 14px;
  margin-bottom: 14px;
}

.main-charts {
  grid-template-columns: minmax(0, 1.7fr) minmax(300px, 0.8fr);
}

.two-charts {
  grid-template-columns: 1fr 1fr;
}

.chart-panel {
  min-width: 0;
}

.category-section {
  margin-bottom: 14px;
}

.category-total {
  color: var(--muted-light);
  font-size: 10px;
}

.chart-panel .panel-body {
  padding-top: 12px;
}

.reason-tabs {
  display: flex;
  gap: 2px;
  padding: 2px;
  border-radius: 7px;
  background: #f4f7fb;
}

.reason-tabs button.active {
  background: #fff;
  color: #b45309;
  box-shadow: var(--shadow-sm);
  font-weight: 550;
}

.panel-body.no-pad {
  padding: 0;
}

.project-chart .panel-body {
  padding: 10px 0 0;
}

.mini-table {
  padding: 0 16px 10px;
}

.mini-table-head,
.mini-table-row {
  display: grid;
  grid-template-columns: minmax(120px, 1.5fr) 0.5fr 0.6fr 0.6fr;
  gap: 8px;
  align-items: center;
  padding: 7px 0;
  font-size: 10px;
}

.mini-table-head {
  color: var(--muted-light);
  font-size: 8.5px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid #f0f3f8;
}

.mini-table-row {
  color: var(--muted);
  border-bottom: 1px dashed #f3f5f9;
}

.mini-table-row:last-child {
  border-bottom: 0;
}

.mini-table-row strong {
  overflow: hidden;
  color: var(--text);
  font-size: 10px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mini-table-row .rate {
  color: var(--brand);
  font-family: var(--mono);
  font-weight: 600;
}

.arrow {
  width: 10px;
  height: 10px;
  vertical-align: -1px;
}

.pending-strip {
  margin-top: 2px;
  overflow: hidden;
}

.pending-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 16px 20px 10px;
}

.pending-list {
  padding: 0 12px 12px;
}

.pending-item {
  display: grid;
  grid-template-columns: 1.4fr 1.2fr 0.7fr 0.9fr auto;
  gap: 10px;
  align-items: center;
  width: 100%;
  padding: 9px 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--muted);
  font-size: 10px;
  text-align: left;
  cursor: pointer;
}

.pending-item:hover {
  background: #fffbef;
}

.pending-item .mono {
  color: var(--text);
  font-size: 9.5px;
}

.pending-item .wait {
  color: #b45309;
  font-family: var(--mono);
  font-size: 9px;
}

@media (max-width: 1100px) {
  .metric-grid {
    grid-template-columns: 1fr 1fr;
  }

  .main-charts,
  .two-charts {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 650px) {
  .metric-grid {
    grid-template-columns: 1fr;
  }

  .filter-note {
    width: 100%;
    margin-left: 0;
  }

  .filter-group {
    width: 100%;
  }

  .filter-group :deep(.n-select) {
    flex: 1;
  }

  .pending-item {
    grid-template-columns: 1fr auto;
  }

  .pending-item > span:nth-child(2),
  .pending-item > span:nth-child(3),
  .pending-item > span:nth-child(4) {
    display: none;
  }
}
</style>
