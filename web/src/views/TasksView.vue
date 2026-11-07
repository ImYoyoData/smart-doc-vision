<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NButton,
  NCheckbox,
  NCheckboxGroup,
  NDataTable,
  NDatePicker,
  NInput,
  NPopover,
  NSelect,
  useDialog,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import dayjs from 'dayjs'
import {
  AddOutline,
  ArrowForwardOutline,
  BookmarkOutline,
  DocumentTextOutline,
  EyeOutline,
  FlashOutline,
  ImagesOutline,
  RefreshOutline,
  SearchOutline,
  SettingsOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import PageSkeleton from '@/components/PageSkeleton.vue'
import { useInitialLoad } from '@/composables/useInitialLoad'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import TaskSubmitModal from '@/components/TaskSubmitModal.vue'
import TaskDetailDrawer from '@/components/TaskDetailDrawer.vue'
import { auditApi, taskApi } from '@/api'
import type { Audit, Task } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'
import { sortersToQuery } from '@/utils/table'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const workspace = useWorkspaceStore()
const auth = useAuthStore()

const tasks = ref<Task[]>([])
const audits = ref<Audit[]>([])
const loading = ref(false)
const { showSkeleton } = useInitialLoad(loading)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const auditId = ref<number | null>(null)
const statuses = ref<string[]>([])
const results = ref<string[]>([])
const sources = ref<string[]>([])
const dateRange = ref<[number, number] | null>(null)
const submitShow = ref(false)
const detailShow = ref(false)
const selectedTaskId = ref<number | null>(null)
const retryingId = ref<number | null>(null)
const savedViews = ref<SavedView[]>([])
const savedViewId = ref<string | null>(null)
const newViewName = ref('')
const showSaveView = ref(false)
const visibleColumnKeys = ref<string[]>([])
const ALL_COLUMNS = [
  { key: 'task', label: '任务' },
  { key: 'audit', label: '审核配置' },
  { key: 'media', label: '媒体' },
  { key: 'status', label: '状态' },
  { key: 'result', label: '结论' },
  { key: 'model', label: '模型' },
  { key: 'duration', label: '耗时' },
  { key: 'created', label: '创建时间' },
]
const VIEW_STORAGE = 'sdv:task-views:v1'
const COLUMN_STORAGE = 'sdv:task-columns:v1'
let searchTimer: ReturnType<typeof setTimeout> | null = null

interface SavedView {
  id: string
  name: string
  projectId: number
  keyword: string
  auditId: number | null
  statuses: string[]
  results: string[]
  sources: string[]
  dateRange: [number, number] | null
}

const savedViewOptions = computed(() =>
  savedViews.value
    .filter((view) => view.projectId === workspace.currentProjectId)
    .map((view) => ({ label: view.name, value: view.id })),
)

function isColumnVisible(key: string) {
  return visibleColumnKeys.value.includes(key)
}

const taskSort = ref('')

function onTaskSorterChange(value: unknown) {
  const next = sortersToQuery(value)
  if (next === taskSort.value) return
  taskSort.value = next
  if (page.value === 1) {
    load()
  } else {
    page.value = 1
  }
}

const taskPagination = computed(() => ({
  page: page.value,
  pageSize: pageSize.value,
  itemCount: total.value,
  pageSizes: [20, 50, 100],
  showSizePicker: true,
  showQuickJumper: true,
  onChange: (value: number) => {
    page.value = value
  },
  onUpdatePageSize: (value: number) => {
    pageSize.value = value
  },
}))

function loadLayout() {
  try {
    const rawViews = localStorage.getItem(VIEW_STORAGE)
    if (rawViews) savedViews.value = JSON.parse(rawViews) as SavedView[]
    const rawColumns = localStorage.getItem(COLUMN_STORAGE)
    if (rawColumns) {
      const parsed = JSON.parse(rawColumns) as string[]
      visibleColumnKeys.value = Array.isArray(parsed)
        ? ALL_COLUMNS.map((c) => c.key).filter((key) => parsed.includes(key))
        : ALL_COLUMNS.map((c) => c.key)
    } else {
      visibleColumnKeys.value = ALL_COLUMNS.map((c) => c.key)
    }
  } catch {
    visibleColumnKeys.value = ALL_COLUMNS.map((c) => c.key)
  }
  if (!visibleColumnKeys.value.length) visibleColumnKeys.value = ['task', 'status', 'result']
}

function persistLayout() {
  try {
    localStorage.setItem(VIEW_STORAGE, JSON.stringify(savedViews.value))
    localStorage.setItem(COLUMN_STORAGE, JSON.stringify(visibleColumnKeys.value))
  } catch {
    // storage may be unavailable in private mode; layout is best effort
  }
}

function saveCurrentView() {
  const name = newViewName.value.trim()
  if (!name) {
    message.warning('请填写视图名称')
    return
  }
  if (!workspace.currentProjectId) return
  const view: SavedView = {
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    name,
    projectId: workspace.currentProjectId,
    keyword: keyword.value,
    auditId: auditId.value,
    statuses: [...statuses.value],
    results: [...results.value],
    sources: [...sources.value],
    dateRange: dateRange.value ? [dateRange.value[0], dateRange.value[1]] : null,
  }
  savedViews.value = [view, ...savedViews.value].slice(0, 30)
  savedViewId.value = view.id
  newViewName.value = ''
  showSaveView.value = false
  persistLayout()
  message.success(`已保存视图「${name}」`)
}

function applySavedView(id: string | null) {
  savedViewId.value = id
  if (!id) return
  const view = savedViews.value.find((item) => item.id === id)
  if (!view) return
  keyword.value = view.keyword
  auditId.value = view.auditId
  statuses.value = [...view.statuses]
  results.value = [...view.results]
  sources.value = [...view.sources]
  dateRange.value = view.dateRange ? [view.dateRange[0], view.dateRange[1]] : null
  page.value = 1
  load()
}

function deleteSavedView() {
  if (!savedViewId.value) return
  savedViews.value = savedViews.value.filter((view) => view.id !== savedViewId.value)
  savedViewId.value = null
  persistLayout()
  message.success('视图已删除')
}

const auditOptions = computed(() => audits.value.map((a) => ({ label: a.name, value: a.id })))

const statusOptions = [
  { label: '排队中', value: 'QUEUED' },
  { label: '审核中', value: 'PROCESSING' },
  { label: '待人工复核', value: 'REVIEW_PENDING' },
  { label: '已完成', value: 'SUCCESS' },
  { label: '执行失败', value: 'FAILED' },
  { label: '已停止', value: 'STOPPED' },
]

const resultOptions = [
  { label: '通过', value: 'PASS' },
  { label: '不通过', value: 'REJECT' },
  { label: '待议', value: 'REVIEW' },
  { label: '未判定', value: 'UNKNOWN' },
]

const sourceOptions = [
  { label: '后台提交', value: 'ADMIN' },
  { label: '开放 API', value: 'OPENAPI' },
  { label: 'Redis', value: 'REDIS' },
  { label: 'WebSocket', value: 'WEBSOCKET' },
]

function sourceLabel(source?: string) {
  const map: Record<string, string> = {
    ADMIN: '后台提交', OPENAPI: '开放 API', REDIS: 'Redis', WEBSOCKET: 'WebSocket', POLLING: '定时轮询',
  }
  return map[source || ''] || source || '—'
}

const taskColumns = computed<DataTableColumns<Task>>(() => {
  const columns: DataTableColumns<Task> = []
  if (isColumnVisible('task')) {
    columns.push({
      title: '任务',
      key: 'task_no',
      width: 220,
      align: 'center',
      resizable: true,
      sorter: { multiple: 1, compare: 'default' },
      render: (row) => h('div', [
        h('div', { class: 'cell-title mono' }, row.task_no),
        h('div', { class: 'cell-sub' }, [
          row.biz_key || '无业务键',
          row.request_id ? h('span', { class: 'mono' }, ` · ${row.request_id}`) : null,
        ]),
      ]),
    })
  }
  if (isColumnVisible('audit')) {
    columns.push({
      title: '审核配置',
      key: 'audit_name',
      width: 160,
      align: 'center',
      resizable: true,
      render: (row) => h('div', [
        h('div', { class: 'cell-title' }, row.audit_name || '—'),
        h('div', { class: 'cell-sub' }, sourceLabel(row.source_type)),
      ]),
    })
  }
  if (isColumnVisible('media')) {
    columns.push({
      title: '媒体',
      key: 'media_type',
      width: 105,
      align: 'center',
      resizable: true,
      render: (row) => {
        const icon = row.media_type === 'text'
          ? DocumentTextOutline
          : row.media_type === 'image'
            ? ImagesOutline
            : FlashOutline
        return h('span', { class: 'media-chip' }, [h(icon), row.media_type])
      },
    })
  }
  if (isColumnVisible('status')) {
    columns.push({
      title: '状态',
      key: 'status',
      width: 100,
      align: 'center',
      resizable: true,
      sorter: { multiple: 1, compare: 'default' },
      render: (row) => h(StatusTag, { status: row.status, type: 'task' }),
    })
  }
  if (isColumnVisible('result')) {
    columns.push({
      title: '结论',
      key: 'result',
      width: 100,
      align: 'center',
      resizable: true,
      sorter: { multiple: 1, compare: 'default' },
      render: (row) => h(StatusTag, { status: row.result, type: 'result' }),
    })
  }
  if (isColumnVisible('model')) {
    columns.push({
      title: '模型',
      key: 'provider_name',
      width: 150,
      align: 'center',
      resizable: true,
      render: (row) => h('div', { class: 'cell-title model-cell' }, row.provider_name || '—'),
    })
  }
  if (isColumnVisible('duration')) {
    columns.push({
      title: '耗时',
      key: 'duration_ms',
      width: 95,
      align: 'center',
      resizable: true,
      sorter: { multiple: 1, compare: 'default' },
      render: (row) => h('span', { class: 'duration mono' },
        row.duration_ms ? `${(row.duration_ms / 1000).toFixed(2)}s` : '—'),
    })
  }
  if (isColumnVisible('created')) {
    columns.push({
      title: '创建时间',
      key: 'created_at',
      width: 165,
      align: 'center',
      resizable: true,
      sorter: { multiple: 1, compare: 'default' },
      render: (row) => h('span', { class: 'task-time' },
        row.created_at?.replace('T', ' ').slice(0, 19) || '—'),
    })
  }
  columns.push({
    title: '操作',
    key: 'actions',
    width: 150,
    align: 'center',
    fixed: 'right',
    resizable: false,
    render: (row) => h('div', { class: 'row-actions' }, [
      h(NButton, {
        size: 'tiny',
        quaternary: true,
        onClick: (event: MouseEvent) => {
          event.stopPropagation()
          openTask(row)
        },
      }, { icon: () => h(EyeOutline), default: () => '查看' }),
      row.status === 'FAILED' && auth.can('task:retry')
        ? h(NButton, {
            size: 'tiny',
            quaternary: true,
            type: 'warning',
            loading: retryingId.value === row.id,
            onClick: (event: MouseEvent) => {
              event.stopPropagation()
              retry(row)
            },
          }, { icon: () => h(RefreshOutline), default: () => '重试' })
        : null,
    ]),
  })
  return columns
})

function taskRowProps(row: Task) {
  return {
    class: 'clickable-row',
    onClick: () => openTask(row),
  }
}

const taskRowKey = (row: Task) => row.id

async function loadAudits() {
  if (!workspace.currentProjectId) {
    audits.value = []
    return
  }
  try {
    const data = await auditApi.list({ project_id: workspace.currentProjectId, page: 1, page_size: 100 })
    audits.value = data.items
  } catch {
    audits.value = []
  }
}

async function load() {
  if (!workspace.currentProjectId) {
    tasks.value = []
    total.value = 0
    return
  }
  loading.value = true
  try {
    const params: Record<string, unknown> = {
      project_id: workspace.currentProjectId,
      page: page.value,
      page_size: pageSize.value,
    }
    if (keyword.value.trim()) params.keyword = keyword.value.trim()
    if (auditId.value) params.audit_id = auditId.value
    if (statuses.value.length) params.status = statuses.value.join(',')
    if (results.value.length) params.result = results.value.join(',')
    if (sources.value.length) params.source_type = sources.value.join(',')
    if (dateRange.value) {
      params.start = dayjs(dateRange.value[0]).format('YYYY-MM-DD')
      params.end = dayjs(dateRange.value[1]).format('YYYY-MM-DD')
    }
    if (taskSort.value) params.sort = taskSort.value
    const data = await taskApi.list(params)
    tasks.value = data.items
    total.value = data.total
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载任务失败')
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  keyword.value = ''
  auditId.value = null
  statuses.value = []
  results.value = []
  sources.value = []
  dateRange.value = null
  page.value = 1
  load()
}

function openTask(task: Task) {
  selectedTaskId.value = task.id
  detailShow.value = true
}

function openSubmit() {
  submitShow.value = true
}

async function handleCreated(task: Task, duplicated: boolean) {
  await load()
  message.info(duplicated ? `命中去重，打开原任务 ${task.task_no}` : `任务 ${task.task_no} 已创建`)
  if (!duplicated) {
    selectedTaskId.value = task.id
    detailShow.value = true
  }
}

function retry(task: Task) {
  dialog.warning({
    title: '重试任务',
    content: '将重新调用审核模型，可能产生额外调用费用。确认继续？',
    positiveText: '重试',
    negativeText: '取消',
    onPositiveClick: async () => {
      retryingId.value = task.id
      try {
        await taskApi.retry(task.id)
        message.success('任务已重新入队')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '重试失败')
      } finally {
        retryingId.value = null
      }
    },
  })
}

function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 350)
}

function applyRouteQuery() {
  if (route.query.submit === '1') {
    submitShow.value = true
    router.replace({ name: 'tasks' })
  }
  if (typeof route.query.task === 'string') {
    const id = Number(route.query.task)
    if (id) {
      selectedTaskId.value = id
      detailShow.value = true
    }
  }
}

watch(visibleColumnKeys, persistLayout, { deep: true })
watch(() => workspace.currentProjectId, async () => {
  page.value = 1
  auditId.value = null
  savedViewId.value = null
  await Promise.all([load(), loadAudits()])
  applyRouteQuery()
})

watch([auditId, statuses, results, sources, dateRange], () => {
  page.value = 1
  load()
})

watch(page, () => {
  load()
  scrollToTasksTop()
})
watch(pageSize, () => {
  page.value = 1
  load()
})

function scrollToTasksTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(async () => {
  loadLayout()
  await Promise.all([load(), loadAudits()])
  applyRouteQuery()
})

onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer)
})
</script>

<template>
  <div class="page tasks-page">
    <PageHeader
      eyebrow="AUDIT TASKS"
      title="审核任务"
      tour-key="tasks"
      subtitle="查看每一次图档审核的状态、结论、规则命中与完整链路事件。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" title="刷新" aria-label="刷新" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button v-if="auth.can('task:submit')" type="primary" @click="openSubmit">
          <template #icon><AddOutline /></template>
          发起审核
        </n-button>
      </template>
    </PageHeader>

    <section class="panel">
      <div class="task-toolbar" data-tour="tasks-toolbar">
        <n-input
          v-model:value="keyword"
          placeholder="任务号 / 业务键 / request_id"
          clearable
          class="search-input"
          @input="onSearchInput"
          @clear="onSearchInput"
        >
          <template #prefix><SearchOutline /></template>
        </n-input>
        <n-select
          v-model:value="auditId"
          :options="auditOptions"
          clearable
          filterable
          placeholder="审核配置"
          style="width: 190px"
        />
        <n-select
          v-model:value="statuses"
          :options="statusOptions"
          multiple
          clearable
          placeholder="任务状态"
          max-tag-count="responsive"
          style="width: 190px"
        />
        <n-select
          v-model:value="results"
          :options="resultOptions"
          multiple
          clearable
          placeholder="审核结论"
          max-tag-count="responsive"
          style="width: 160px"
        />
        <n-select
          v-model:value="sources"
          :options="sourceOptions"
          multiple
          clearable
          placeholder="触发来源"
          max-tag-count="responsive"
          style="width: 170px"
        />
        <n-date-picker
          v-model:value="dateRange"
          type="daterange"
          clearable
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          style="width: 245px"
        />
        <button class="reset-btn" type="button" @click="resetFilters">重置</button>
        <n-select
          :value="savedViewId"
          :options="savedViewOptions"
          clearable
          placeholder="已保存视图"
          style="width: 150px"
          @update:value="applySavedView"
        />
        <n-popover v-model:show="showSaveView" trigger="click" placement="bottom-end" raw>
          <template #trigger>
            <n-button quaternary size="small" title="保存当前筛选视图">
              <template #icon><BookmarkOutline /></template>
            </n-button>
          </template>
          <div class="save-view-pop">
            <strong>保存当前视图</strong>
            <n-input v-model:value="newViewName" size="small" placeholder="例如：待复核的合同审核" @keyup.enter="saveCurrentView" />
            <n-button type="primary" size="small" block @click="saveCurrentView">保存</n-button>
          </div>
        </n-popover>
        <n-button v-if="savedViewId" quaternary size="small" title="删除当前视图" @click="deleteSavedView">
          <template #icon><TrashOutline /></template>
        </n-button>
        <n-popover trigger="click" placement="bottom-end" :width="180">
          <template #trigger>
            <n-button quaternary size="small" title="显示列">
              <template #icon><SettingsOutline /></template>
            </n-button>
          </template>
          <div class="column-settings">
            <strong>显示列</strong>
            <n-checkbox-group v-model:value="visibleColumnKeys">
              <n-checkbox v-for="column in ALL_COLUMNS" :key="column.key" :value="column.key" :label="column.label" />
            </n-checkbox-group>
          </div>
        </n-popover>
        <div class="toolbar-spacer"></div>
        <span class="total-badge">共 <strong>{{ total }}</strong> 条任务</span>
      </div>

      <div v-if="tasks.length" class="data-table-wrap" data-tour="tasks-table">
        <n-data-table
          :columns="taskColumns"
          :data="tasks"
          :row-key="taskRowKey"
          :row-props="taskRowProps"
          :pagination="taskPagination"
          :scroll-x="1240"
          max-height="calc(100vh - 330px)"
          remote
          striped
          size="small"
          :bordered="false"
          class="sdv-data-table"
          @update:sorter="onTaskSorterChange"
        />
      </div>

      <EmptyState
        v-else-if="!loading"
        title="暂无审核任务"
        description="提交一份图档文本或图片，审核引擎会异步完成分析与规则判定。"
        :action="auth.can('task:submit') ? '发起第一次审核' : undefined"
        @action="openSubmit"
      >
        <template #icon><FlashOutline /></template>
      </EmptyState>

      <PageSkeleton v-else-if="showSkeleton" variant="table" :rows="6" />
    </section>

    <TaskSubmitModal
      v-model:show="submitShow"
      :project-id="workspace.currentProjectId"
      @created="handleCreated"
    />
    <TaskDetailDrawer
      v-if="detailShow"
      v-model:show="detailShow"
      :task-id="selectedTaskId"
      @updated="load"
    />
  </div>
</template>

<style scoped>
.task-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
  padding: 16px 18px;
  border-bottom: 1px solid #f0f3f8;
}

.search-input {
  width: 210px;
}

.reset-btn {
  padding: 0 4px;
  border: 0;
  background: none;
  color: var(--muted);
  font-size: 11.5px;
  cursor: pointer;
}

.reset-btn:hover {
  color: var(--brand);
}

.save-view-pop {
  display: flex;
  width: 230px;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
}

.save-view-pop strong,
.column-settings strong {
  color: var(--text);
  font-size: 11px;
}

.column-settings {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.column-settings :deep(.n-checkbox-group) {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.toolbar-spacer {
  flex: 1;
}

.total-badge {
  color: var(--muted-light);
  font-size: 11px;
}

.total-badge strong {
  color: var(--ink);
  font-family: var(--mono);
  font-weight: 600;
}

.task-table {
  font-size: 11px;
}

.task-table th {
  padding: 10px 9px;
  font-size: 9px;
  letter-spacing: 0.03em;
}

.task-table td {
  padding: 10px 9px;
}

.task-table .cell-title {
  font-size: 10.5px;
}

.task-table .cell-sub {
  font-size: 9px;
}

.task-table .model-cell {
  max-width: 120px;
  font-size: 10.5px;
}

.media-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border-radius: 5px;
  background: #f8fafc;
  color: #64748b;
  font-size: 10px;
}

.media-chip :deep(svg) {
  width: 11px;
  height: 11px;
}

.model-cell {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11.5px;
}

.duration {
  color: var(--muted);
  font-size: 10.5px;
}

.task-time {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9.5px;
}

.loading-table {
  padding: 16px 20px;
}

.loading-row {
  height: 48px;
  margin-bottom: 8px;
  border-radius: 8px;
  background: linear-gradient(90deg, #f7f9fc, #fcfdff, #f7f9fc);
  background-size: 200% 100%;
  animation: shimmer 1.4s ease infinite;
}

@keyframes shimmer {
  to {
    background-position: -200% 0;
  }
}

@media (max-width: 900px) {
  .search-input {
    width: 100%;
  }

  .save-view-pop {
  display: flex;
  width: 230px;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
}

.save-view-pop strong,
.column-settings strong {
  color: var(--text);
  font-size: 11px;
}

.column-settings {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.column-settings :deep(.n-checkbox-group) {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.toolbar-spacer {
    display: none;
  }
}

@media (max-width: 560px) {
  .task-toolbar {
    padding: 12px;
  }

  .task-toolbar :deep(.n-select),
  .task-toolbar :deep(.n-date-picker) {
    width: 100% !important;
    flex: 1 1 100%;
  }

  .total-badge {
    width: 100%;
    text-align: right;
  }
}
</style>
