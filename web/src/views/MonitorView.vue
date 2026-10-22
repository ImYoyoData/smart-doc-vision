<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NSelect, useMessage } from 'naive-ui'
import dayjs from 'dayjs'
import type { EChartsOption } from 'echarts'
import {
  AlertCircleOutline,
  FlashOutline,
  LayersOutline,
  RefreshOutline,
  ServerOutline,
  SpeedometerOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import MetricCard from '@/components/MetricCard.vue'
import EChart from '@/components/EChart.vue'
import EmptyState from '@/components/EmptyState.vue'
import { monitorApi } from '@/api'
import type { MonitorData } from '@/api/types'

const message = useMessage()
const data = ref<MonitorData | null>(null)
const loading = ref(false)
const range = ref('1h')
const lastUpdated = ref('')

const rangeOptions = [
  { label: '最近 1 小时', value: '1h' },
  { label: '最近 6 小时', value: '6h' },
  { label: '最近 24 小时', value: '24h' },
]

const current = computed(() => data.value?.current)
const history = computed(() => data.value?.history || [])
const queue = computed(() => data.value?.queue || [])
const queueTotals = computed(() =>
  queue.value.reduce(
    (acc, item) => ({ active: acc.active + item.active, pending: acc.pending + item.pending }),
    { active: 0, pending: 0 },
  ),
)
const taskEntries = computed(() => {
  const tasks = data.value?.tasks || {}
  const labels: Record<string, string> = {
    QUEUED: '排队中',
    PROCESSING: '执行中',
    REVIEW_PENDING: '待复核',
    SUCCESS: '成功',
    FAILED: '失败',
  }
  return Object.entries(labels).map(([key, label]) => ({ key, label, value: tasks[key] || 0 }))
})
const hasHistory = computed(() => history.value.length > 1)
const hostCpu = computed(() => Number(current.value?.host_cpu_percent || 0))
const hostMem = computed(() => Number(current.value?.host_mem_percent || 0))

function formatUptime(seconds?: number) {
  if (!seconds || seconds < 0) return '—'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days} 天 ${hours} 小时`
  if (hours > 0) return `${hours} 小时 ${minutes} 分`
  return `${minutes} 分 ${seconds % 60} 秒`
}

function formatNumber(value?: number) {
  if (value === null || value === undefined) return '—'
  return value.toLocaleString('zh-CN')
}

function axisTime(value: string) {
  const moment = dayjs(value)
  return range.value === '24h' ? moment.format('MM-DD HH:mm') : moment.format('HH:mm:ss')
}

function queuePercent(active: number, limit: number) {
  if (!limit) return 0
  return Math.min(100, Math.round((active / limit) * 100))
}

let inFlight = false

async function load(silent = false) {
  // Never stack polls: a slow response must not pile up requests.
  if (inFlight) return
  inFlight = true
  if (!silent) loading.value = true
  try {
    data.value = await monitorApi.get(range.value)
    lastUpdated.value = dayjs().format('HH:mm:ss')
  } catch (error) {
    if (!silent) message.error(error instanceof Error ? error.message : '加载运行监控失败')
  } finally {
    inFlight = false
    if (!silent) loading.value = false
  }
}

const axisStyle = {
  axisLine: { lineStyle: { color: '#e8edf5' } },
  axisTick: { show: false },
  axisLabel: { color: '#94a3b8', fontSize: 10 },
  splitLine: { lineStyle: { color: '#f1f4f9' } },
}

const labels = computed(() => history.value.map((point) => axisTime(point.time)))

const requestOption = computed<EChartsOption>(() => ({
  animation: false,
  tooltip: { trigger: 'axis' },
  legend: {
    right: 0,
    top: 0,
    itemWidth: 8,
    itemHeight: 8,
    textStyle: { color: '#64748b', fontSize: 10 },
  },
  grid: { left: 4, right: 10, top: 34, bottom: 4, containLabel: true },
  xAxis: { type: 'category', data: labels.value, ...axisStyle },
  yAxis: { type: 'value', minInterval: 1, ...axisStyle },
  series: [
    {
      name: '请求数',
      type: 'bar',
      data: history.value.map((point) => point.requests),
      barMaxWidth: 18,
      itemStyle: { color: '#2b6cf6', borderRadius: [3, 3, 0, 0] },
    },
    {
      name: '错误数',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.errors),
      lineStyle: { width: 1.6, color: '#ef6464' },
      itemStyle: { color: '#ef6464' },
    },
  ],
}))

const concurrencyOption = computed<EChartsOption>(() => ({
  animation: false,
  tooltip: { trigger: 'axis' },
  legend: {
    right: 0,
    top: 0,
    itemWidth: 8,
    itemHeight: 8,
    textStyle: { color: '#64748b', fontSize: 10 },
  },
  grid: { left: 4, right: 10, top: 34, bottom: 4, containLabel: true },
  xAxis: { type: 'category', data: labels.value, boundaryGap: false, ...axisStyle },
  yAxis: { type: 'value', minInterval: 1, ...axisStyle },
  series: [
    {
      name: '处理中请求',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.in_flight),
      lineStyle: { width: 2, color: '#2b6cf6' },
      itemStyle: { color: '#2b6cf6' },
      areaStyle: { color: 'rgba(43, 108, 246, 0.12)' },
    },
    {
      name: '协程数',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.goroutines),
      lineStyle: { width: 1.5, color: '#8b5cf6' },
      itemStyle: { color: '#8b5cf6' },
    },
    {
      name: '线程数',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.threads),
      lineStyle: { width: 1.5, color: '#f59e0b' },
      itemStyle: { color: '#f59e0b' },
    },
  ],
}))

const memoryOption = computed<EChartsOption>(() => ({
  animation: false,
  tooltip: { trigger: 'axis', valueFormatter: (value) => `${value} MB` },
  legend: {
    right: 0,
    top: 0,
    itemWidth: 8,
    itemHeight: 8,
    textStyle: { color: '#64748b', fontSize: 10 },
    data: ['堆内存', '堆向系统申请', '进程总内存'],
  },
  grid: { left: 4, right: 10, top: 34, bottom: 4, containLabel: true },
  xAxis: { type: 'category', data: labels.value, boundaryGap: false, ...axisStyle },
  yAxis: { type: 'value', ...axisStyle },
  series: [
    {
      name: '堆内存',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.heap_alloc_mb),
      lineStyle: { width: 2, color: '#28b875' },
      itemStyle: { color: '#28b875' },
      areaStyle: { color: 'rgba(40, 184, 117, 0.13)' },
    },
    {
      name: '堆向系统申请',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.heap_sys_mb),
      lineStyle: { width: 1.5, color: '#2b6cf6' },
      itemStyle: { color: '#2b6cf6' },
    },
    {
      name: '进程总内存',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.sys_mb),
      lineStyle: { width: 1.5, color: '#94a3b8', type: 'dashed' },
      itemStyle: { color: '#94a3b8' },
    },
  ],
}))

const latencyOption = computed<EChartsOption>(() => ({
  animation: false,
  tooltip: { trigger: 'axis', valueFormatter: (value) => `${value} ms` },
  grid: { left: 4, right: 10, top: 18, bottom: 4, containLabel: true },
  xAxis: { type: 'category', data: labels.value, boundaryGap: false, ...axisStyle },
  yAxis: { type: 'value', ...axisStyle },
  series: [
    {
      name: '平均响应耗时',
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: history.value.map((point) => point.avg_latency_ms),
      lineStyle: { width: 2, color: '#f59e0b' },
      areaStyle: { color: 'rgba(245, 158, 11, 0.12)' },
    },
  ],
}))

watch(range, () => load())

let timer: ReturnType<typeof setInterval> | null = null

function tick() {
  // Hidden tabs do not need fresh chart data; skip the request entirely.
  if (document.visibilityState === 'visible') load(true)
}

function onVisibilityChange() {
  if (document.visibilityState === 'visible') load(true)
}

onMounted(() => {
  load()
  timer = setInterval(tick, 10000)
  document.addEventListener('visibilitychange', onVisibilityChange)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div class="page monitor-page">
    <PageHeader
      eyebrow="SYSTEM MONITOR"
      title="运行监控"
      subtitle="查看服务进程内存、协程与线程、请求吞吐、并发数和项目 Worker 队列；采样保留 24 小时。"
    >
      <template #actions>
        <span class="live-chip"><span class="live-dot"></span>每 10 秒刷新</span>
        <n-select v-model:value="range" :options="rangeOptions" style="width: 145px" />
        <n-button quaternary circle :loading="loading" @click="load()">
          <template #icon><RefreshOutline /></template>
        </n-button>
      </template>
    </PageHeader>

    <section class="monitor-metrics">
      <MetricCard
        label="运行时间"
        :value="formatUptime(current?.uptime_seconds)"
        :hint="current ? `启动于 ${dayjs(current.started_at).format('MM-DD HH:mm')}` : '加载中'"
        tone="blue"
        :loading="!current"
      >
        <template #icon><TimeOutline /></template>
      </MetricCard>
      <MetricCard
        label="堆内存占用"
        :value="current?.heap_alloc_mb ?? '—'"
        suffix="MB"
        :hint="current ? `进程总内存 ${current.sys_mb} MB · GC ${current.gc_count} 次` : '加载中'"
        tone="green"
        :loading="!current"
      >
        <template #icon><ServerOutline /></template>
      </MetricCard>
      <MetricCard
        label="协程 / 线程"
        :value="current ? `${current.goroutines} / ${current.threads}` : '—'"
        :hint="current ? `GOMAXPROCS ${current.gomaxprocs} · CPU ${current.num_cpu} 核` : '加载中'"
        tone="purple"
        :loading="!current"
      >
        <template #icon><LayersOutline /></template>
      </MetricCard>
      <MetricCard
        label="当前并发"
        :value="current?.in_flight ?? '—'"
        :hint="`项目执行中 ${queueTotals.active} · 排队 ${queueTotals.pending}`"
        tone="amber"
        :loading="!current"
      >
        <template #icon><FlashOutline /></template>
      </MetricCard>
      <MetricCard
        label="累计请求"
        :value="formatNumber(current?.requests_total)"
        :hint="current ? `错误响应 ${formatNumber(current.errors_total)} 次` : '加载中'"
        tone="blue"
        :loading="!current"
      >
        <template #icon><SpeedometerOutline /></template>
      </MetricCard>
      <MetricCard
        label="平均响应耗时"
        :value="current?.avg_latency_ms ?? '—'"
        suffix="ms"
        :hint="current ? `GC 累计暂停 ${current.gc_pause_total_ms} ms` : '加载中'"
        tone="red"
        :loading="!current"
      >
        <template #icon><AlertCircleOutline /></template>
      </MetricCard>
    </section>

    <div v-if="!hasHistory" class="monitor-hint">
      <SpeedometerOutline />
      <div>
        <strong>正在采集运行指标</strong>
        <p>采样间隔为 {{ data?.sample_interval_seconds || 10 }} 秒，稍后即可看到趋势曲线。累计请求量会立即更新。</p>
      </div>
    </div>

    <div class="monitor-grid">
      <section class="panel">
        <div class="panel-head">
          <div><h3 class="panel-title">请求量与错误</h3><p class="panel-subtitle">按采样区间统计的请求数，错误数为 HTTP 4xx/5xx</p></div>
        </div>
        <div class="panel-body"><EChart :option="requestOption" height="260px" :loading="loading" /></div>
      </section>
      <section class="panel">
        <div class="panel-head">
          <div><h3 class="panel-title">并发与运行时线程</h3><p class="panel-subtitle">处理中请求、Go 协程与操作系统线程</p></div>
        </div>
        <div class="panel-body"><EChart :option="concurrencyOption" height="260px" :loading="loading" /></div>
      </section>
      <section class="panel">
        <div class="panel-head">
          <div><h3 class="panel-title">内存占用</h3><p class="panel-subtitle">Go 堆内存、向系统申请的堆空间与进程总内存</p></div>
        </div>
        <div class="panel-body"><EChart :option="memoryOption" height="260px" :loading="loading" /></div>
      </section>
      <section class="panel">
        <div class="panel-head">
          <div><h3 class="panel-title">响应耗时</h3><p class="panel-subtitle">每个采样区间内全部完成请求的平均耗时</p></div>
        </div>
        <div class="panel-body"><EChart :option="latencyOption" height="260px" :loading="loading" /></div>
      </section>
    </div>

    <div class="monitor-bottom">
      <section class="panel">
        <div class="panel-head">
          <div><h3 class="panel-title">项目 Worker 队列</h3><p class="panel-subtitle">各项目独立并发池的实时占用，排队任务不会跨项目抢占</p></div>
          <span class="queue-total">执行中 {{ queueTotals.active }} · 排队 {{ queueTotals.pending }}</span>
        </div>
        <div class="panel-body task-strip">
          <span v-for="entry in taskEntries" :key="entry.key" class="task-chip">
            {{ entry.label }} <b>{{ formatNumber(entry.value) }}</b>
          </span>
        </div>
        <div v-if="queue.length" class="queue-list">
          <div v-for="item in queue" :key="item.project_id" class="queue-row">
            <div class="queue-main">
              <div class="queue-title">
                <strong>{{ item.project_name }}</strong>
                <code>{{ item.project_code || `#${item.project_id}` }}</code>
              </div>
              <span class="queue-meta">执行 {{ item.active }} / 上限 {{ item.limit }} · 排队 {{ item.pending }}</span>
            </div>
            <div class="queue-track">
              <span :style="{ width: `${queuePercent(item.active, item.limit)}%` }" :class="{ full: item.active >= item.limit && item.limit > 0 }"></span>
            </div>
          </div>
        </div>
        <EmptyState v-else compact title="暂无项目队列" description="提交任务后，这里会显示各项目的并发占用和排队情况。" />
      </section>

      <section class="panel">
        <div class="panel-head">
          <div><h3 class="panel-title">主机资源与依赖</h3><p class="panel-subtitle">主机 CPU/内存仅供参考；下方为数据库与缓存状态</p></div>
        </div>
        <div class="panel-body dep-list">
          <div v-if="current?.host_available" class="host-strip">
            <div class="host-line">
              <div class="host-head">
                <span>主机 CPU 使用率</span>
                <b>{{ hostCpu.toFixed(1) }}%</b>
              </div>
              <div class="host-track"><i :style="{ width: `${Math.min(100, hostCpu)}%` }"></i></div>
            </div>
            <div class="host-line">
              <div class="host-head">
                <span>主机内存占用</span>
                <b>{{ hostMem.toFixed(1) }}%</b>
                <em>{{ current.host_mem_used_mb }} / {{ current.host_mem_total_mb }} MB</em>
              </div>
              <div class="host-track mem"><i :style="{ width: `${Math.min(100, hostMem)}%` }"></i></div>
            </div>
          </div>
          <div class="dep-item" :class="{ down: data && !data.dependencies.mysql }">
            <span class="dep-icon"><ServerOutline /></span>
            <div class="dep-body">
              <strong>MySQL {{ data ? (data.dependencies.mysql ? '正常' : '不可用') : '—' }}</strong>
              <span class="dep-meta">
                连接 {{ data?.mysql_pool.open_connections ?? 0 }} 个 · 使用中 {{ data?.mysql_pool.in_use ?? 0 }}
                · 空闲 {{ data?.mysql_pool.idle ?? 0 }} · 等待 {{ data?.mysql_pool.wait_count ?? 0 }} 次
              </span>
            </div>
          </div>
          <div class="dep-item" :class="{ down: data && !data.dependencies.redis }">
            <span class="dep-icon"><LayersOutline /></span>
            <div class="dep-body">
              <strong>Redis {{ data ? (data.dependencies.redis ? '正常' : '不可用') : '—' }}</strong>
              <span class="dep-meta">
                连接 {{ data?.redis_pool.total_conns ?? 0 }} 个 · 空闲 {{ data?.redis_pool.idle_conns ?? 0 }}
                · 命中 {{ formatNumber(data?.redis_pool.hits) }} · 未命中 {{ formatNumber(data?.redis_pool.misses) }}
              </span>
            </div>
          </div>
          <div class="dep-note">
            <TimeOutline />
            <span>
              采样间隔 {{ data?.sample_interval_seconds || 10 }} 秒，保留 {{ data?.retention_hours || 24 }} 小时
              <template v-if="lastUpdated"> · 最近刷新 {{ lastUpdated }}</template>
            </span>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.live-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 9px;
  border: 1px solid #dce7fb;
  border-radius: 8px;
  background: #f8fbff;
  color: #4b6b9e;
  font-size: 10px;
}

.live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.12);
}

.monitor-metrics {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 11px;
  margin-bottom: 14px;
}

.monitor-hint {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border: 1px solid #dce7fb;
  border-radius: 11px;
  background: #f8fbff;
}

.monitor-hint :deep(svg) {
  width: 17px;
  height: 17px;
  margin-top: 2px;
  color: var(--brand);
  flex-shrink: 0;
}

.monitor-hint strong {
  display: block;
  color: #1e4b9e;
  font-size: 11px;
  font-weight: 600;
}

.monitor-hint p {
  margin: 3px 0 0;
  color: #64748b;
  font-size: 10px;
}

.monitor-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 13px;
}

.monitor-grid .panel {
  min-width: 0;
}

.monitor-bottom {
  display: grid;
  grid-template-columns: 1.35fr 1fr;
  gap: 13px;
  margin-top: 13px;
}

.queue-total {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9.5px;
}

.task-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding-bottom: 8px;
}

.task-chip {
  padding: 4px 8px;
  border-radius: 6px;
  background: #f5f8fd;
  color: #64748b;
  font-size: 9.5px;
}

.task-chip b {
  color: var(--ink);
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 650;
}

.queue-list {
  padding: 0 16px 15px;
}

.queue-row {
  padding: 9px 0;
  border-bottom: 1px solid #f2f5f9;
}

.queue-row:last-child {
  border-bottom: 0;
}

.queue-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.queue-title {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}

.queue-title strong {
  overflow: hidden;
  color: var(--text);
  font-size: 11px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.queue-title code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.5px;
}

.queue-meta {
  color: var(--muted-light);
  font-size: 9px;
  white-space: nowrap;
}

.queue-track {
  height: 5px;
  margin-top: 6px;
  overflow: hidden;
  border-radius: 99px;
  background: #eef2f8;
}

.queue-track span {
  display: block;
  height: 100%;
  border-radius: 99px;
  background: var(--brand);
  transition: width 0.3s ease;
}

.queue-track span.full {
  background: #ef6464;
}

.dep-list {
  display: flex;
  flex-direction: column;
  gap: 9px;
}

.host-strip {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 2px 2px 10px;
  border-bottom: 1px dashed #eaf0f7;
}

.host-line {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.host-head {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--muted);
  font-size: 9.5px;
}

.host-head b {
  margin-left: auto;
  color: var(--text);
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 600;
}

.host-head em {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.5px;
  font-style: normal;
}

.host-track {
  height: 4px;
  overflow: hidden;
  border-radius: 99px;
  background: #eef2f8;
}

.host-track i {
  display: block;
  height: 100%;
  border-radius: 99px;
  background: #c3d4f5;
  transition: width 0.3s ease;
}

.host-track.mem i {
  background: #bfe6d1;
}

.dep-item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 10px 11px;
  border: 1px solid #e6f5ec;
  border-radius: 9px;
  background: #f8fdfa;
}

.dep-body {
  min-width: 0;
}

.dep-item.down {
  border-color: #f5dfdf;
  background: #fff9f9;
}

.dep-icon {
  display: flex;
  width: 28px;
  height: 28px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
  line-height: 0;
}

.dep-item.down .dep-icon {
  background: var(--red-soft);
  color: #b91c1c;
}

.dep-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.dep-item strong {
  display: block;
  color: var(--text);
  font-size: 10.5px;
  font-weight: 600;
}

.dep-meta {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 8.5px;
  line-height: 1.6;
}

.dep-note {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.dep-note :deep(svg) {
  width: 12px;
  height: 12px;
}

@media (max-width: 1400px) {
  .monitor-metrics {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 1050px) {
  .monitor-grid,
  .monitor-bottom {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .monitor-metrics {
    grid-template-columns: 1fr 1fr;
  }
}
</style>
