<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { NButton, NDataTable, NInput, NModal, NSelect, useDialog, useMessage, type DataTableColumns } from 'naive-ui'
import {
  ArchiveOutline,
  FolderOpenOutline,
  RefreshOutline,
  SearchOutline,
  ShieldCheckmarkOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import PageSkeleton from '@/components/PageSkeleton.vue'
import { useInitialLoad } from '@/composables/useInitialLoad'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import SliderCaptcha from '@/components/SliderCaptcha.vue'
import { auditApi, projectApi, recycleApi, type RecycleBatchResult } from '@/api'
import type { RecycleItem } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'

const workspace = useWorkspaceStore()
const auth = useAuthStore()
const message = useMessage()
const dialog = useDialog()
const items = ref<RecycleItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const objectType = ref<string | null>(null)
const keyword = ref('')
const loading = ref(false)
const { showSkeleton } = useInitialLoad(loading)
const loadError = ref('')
const restoringId = ref<string | null>(null)
const purgingId = ref<string | null>(null)
const canPurge = computed(() => auth.user?.role === 'SUPER_ADMIN')

// 多选与滑块二次确认
const selectedKeys = ref<Set<string>>(new Set())
const confirmShow = ref(false)
const confirmMode = ref<'restore' | 'purge' | 'purge-all'>('restore')
const confirmBusy = ref(false)
const confirmCaptcha = ref<{ id: string; x: number } | null>(null)
const batchResults = ref<RecycleBatchResult[] | null>(null)

const selectedOnPage = computed(() => items.value.filter((i) => selectedKeys.value.has(`${i.type}:${i.id}`)))
const checkedRowKeys = computed(() => [...selectedKeys.value])

function itemKey(item: RecycleItem) {
  return `${item.type}:${item.id}`
}

function onCheckedRowKeysChange(keys: Array<string | number>) {
  selectedKeys.value = new Set(keys.map((key) => String(key)))
}

function clearSelection() {
  selectedKeys.value = new Set()
}

function formatDeletedAt(value?: string) {
  return value ? value.replace('T', ' ').slice(0, 19) : '—'
}

const recyclePagination = computed(() => ({
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

function recycleTypeLabel(type: string) {
  return type === 'PROJECT' ? '项目' : '审核配置'
}

function renderProjectAudits(row: RecycleItem) {
  const audits = row.audits || []
  return h('div', { class: 'recycle-children' }, [
    h('div', { class: 'recycle-children-head' }, `项目内审核配置（${audits.length}）`),
    ...audits.map((child) => h('div', { key: itemKey(child), class: 'recycle-child-row' }, [
      h('div', { class: 'recycle-child-name' }, [
        h('span', { class: 'item-icon audit' }, h(ShieldCheckmarkOutline)),
        h('div', [
          h('div', { class: 'cell-title' }, child.name),
          h('div', { class: 'cell-sub mono' }, child.code),
        ]),
      ]),
      h('span', { class: 'type-chip audit' }, recycleTypeLabel(child.type)),
      h('span', { class: 'date mono' }, formatDeletedAt(child.deleted_at)),
      h('span', { class: `remaining ${remainingClass(child.remaining_days)}` },
        child.remaining_days > 0 ? `${child.remaining_days} 天` : '已过期'),
      h('span', { class: 'relation' }, `${child.task_count} 个任务`),
      h('span', { class: 'child-locked' }, '需先恢复项目'),
    ])),
  ])
}

const recycleColumns = computed<DataTableColumns<RecycleItem>>(() => [
  {
    type: 'expand',
    width: 44,
    align: 'center',
    fixed: 'left',
    expandable: (row) => (row.audits?.length || 0) > 0,
    renderExpand: renderProjectAudits,
  },
  {
    type: 'selection',
    width: 44,
    align: 'center',
    fixed: 'left',
  },
  {
    title: '名称',
    key: 'name',
    width: 260,
    align: 'center',
    resizable: true,
    render: (row) => h('div', { class: 'recycle-name' }, [
      h('span', { class: ['item-icon', { audit: row.type === 'AUDIT' }] },
        h(row.type === 'PROJECT' ? FolderOpenOutline : ShieldCheckmarkOutline)),
      h('div', [
        h('div', { class: 'cell-title' }, row.name),
        h('div', { class: 'cell-sub mono' }, row.code),
      ]),
    ]),
  },
  {
    title: '类型',
    key: 'type',
    width: 110,
    align: 'center',
    resizable: true,
    render: (row) => h('span', { class: ['type-chip', { audit: row.type === 'AUDIT' }] }, recycleTypeLabel(row.type)),
  },
  {
    title: '所属项目',
    key: 'project_name',
    width: 170,
    align: 'center',
    resizable: true,
    render: (row) => h('span', { class: 'ink-soft' }, row.type === 'PROJECT' ? '—' : row.project_name),
  },
  {
    title: '删除时间',
    key: 'deleted_at',
    width: 170,
    align: 'center',
    resizable: true,
    render: (row) => h('span', { class: 'date mono' }, formatDeletedAt(row.deleted_at)),
  },
  {
    title: '剩余保留',
    key: 'remaining_days',
    width: 110,
    align: 'center',
    resizable: true,
    render: (row) => h('span', { class: `remaining ${remainingClass(row.remaining_days)}` },
      row.remaining_days > 0 ? `${row.remaining_days} 天` : '已过期'),
  },
  {
    title: '关联',
    key: 'task_count',
    width: 100,
    align: 'center',
    resizable: true,
    render: (row) => h('span', { class: 'relation' }, `${row.task_count} 个任务`),
  },
  {
    title: '操作',
    key: 'actions',
    width: 190,
    align: 'center',
    fixed: 'right',
    resizable: false,
    render: (row) => h('div', { class: 'row-actions' }, [
      h(NButton, {
        size: 'tiny',
        type: 'primary',
        secondary: true,
        disabled: !row.can_restore,
        loading: restoringId.value === itemKey(row),
        onClick: () => restore(row),
      }, { icon: () => h(RefreshOutline), default: () => '恢复' }),
      canPurge.value
        ? h(NButton, {
            size: 'tiny',
            quaternary: true,
            class: 'purge-btn',
            loading: purgingId.value === itemKey(row),
            onClick: () => purge(row),
          }, { icon: () => h(TrashOutline), default: () => '永久删除' })
        : null,
    ]),
  },
])

const confirmTitle = computed(() => {
  if (confirmMode.value === 'restore') return `恢复 ${selectedKeys.value.size} 个对象`
  if (confirmMode.value === 'purge') return `永久删除 ${selectedKeys.value.size} 个对象`
  return '清空回收站'
})

const confirmBody = computed(() => {
  if (confirmMode.value === 'restore')
    return '恢复后对象回到工作空间：项目恢复会连带恢复其下联动删除的审核配置；编码冲突或模型已删的对象会跳过并在结果中说明。'
  if (confirmMode.value === 'purge')
    return '选中的项目与审核配置及其任务、事件和关联数据将被永久删除，无法恢复；项目清理会连带其下审核配置。'
  return `回收站内全部 ${total.value} 个对象（项目及其任务/审核配置/模型接入、独立删除的审核配置）将被永久删除，无法恢复。`
})

function openConfirm(mode: 'restore' | 'purge' | 'purge-all') {
  if (mode !== 'purge-all' && selectedKeys.value.size === 0) {
    message.warning('请先勾选要操作的对象')
    return
  }
  if (mode === 'purge-all' && total.value === 0) {
    message.warning('回收站已是空的')
    return
  }
  confirmMode.value = mode
  confirmCaptcha.value = null
  batchResults.value = null
  confirmShow.value = true
}

async function submitConfirm() {
  if (!confirmCaptcha.value) {
    message.warning('请先完成滑块验证')
    return
  }
  confirmBusy.value = true
  try {
    const captcha = { captcha_id: confirmCaptcha.value.id, captcha_x: confirmCaptcha.value.x }
    if (confirmMode.value === 'restore') {
      const data = await recycleApi.batchRestore({
        items: [...selectedKeys.value].map((key) => {
          const [type, id] = key.split(':')
          return { type, id: Number(id) }
        }),
        ...captcha,
      })
      summarize(`成功恢复 ${data.restored}/${data.total} 个对象`, data.results)
    } else if (confirmMode.value === 'purge') {
      const data = await recycleApi.batchPurge({
        items: [...selectedKeys.value].map((key) => {
          const [type, id] = key.split(':')
          return { type, id: Number(id) }
        }),
        ...captcha,
      })
      summarize(`已永久删除 ${data.purged}/${data.total} 个对象`, data.results)
    } else {
      const data = await recycleApi.purgeAll(captcha)
      message.success(`已清空回收站：项目 ${data.purged_projects} 个 · 审核配置 ${data.purged_audits} 个${data.failed ? ` · 失败 ${data.failed} 个` : ''}`, { duration: 6000 })
      confirmShow.value = false
    }
    clearSelection()
    await Promise.all([load(), workspace.loadProjects(true)])
  } catch (error) {
    message.error(error instanceof Error ? error.message : '操作失败', { duration: 6000 })
  } finally {
    confirmBusy.value = false
  }
}

function summarize(headline: string, results: RecycleBatchResult[]) {
  const skipped = results.filter((r) => r.status !== 'restored' && r.status !== 'purged')
  if (!skipped.length) {
    message.success(headline)
    confirmShow.value = false
    return
  }
  batchResults.value = results
  message.warning(`${headline}，${skipped.length} 个被跳过，详见弹窗说明`, { duration: 6000 })
}

async function load() {
  loading.value = true
  try {
    const data = await recycleApi.list({
      object_type: objectType.value || undefined,
      keyword: keyword.value.trim() || undefined,
      page: page.value,
      page_size: pageSize.value,
    })
    items.value = data.items
    total.value = data.total
    loadError.value = ''
    // 清掉已不在任何页里的选中项
    const valid = new Set<string>()
    for (const key of selectedKeys.value) valid.add(key)
    selectedKeys.value = valid
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '加载回收站失败'
    message.error(loadError.value)
  } finally {
    loading.value = false
  }
}

function restore(item: RecycleItem) {
  dialog.info({
    title: item.type === 'PROJECT' ? '恢复项目到工作空间' : '恢复审核配置',
    content:
      item.type === 'PROJECT'
        ? `恢复「${item.name}」后，其下由删除项目联动删除的审核配置也会恢复，并重新开放任务提交。`
        : `恢复「${item.name}」后，审核配置将重新启用。若编码已被占用，恢复会失败。`,
    positiveText: '恢复',
    negativeText: '取消',
    onPositiveClick: async () => {
      const key = `${item.type}:${item.id}`
      restoringId.value = key
      try {
        if (item.type === 'PROJECT') {
          await projectApi.restore(item.id)
        } else {
          await auditApi.restore(item.id)
        }
        message.success('恢复成功')
        selectedKeys.value.delete(itemKey(item))
        selectedKeys.value = new Set(selectedKeys.value)
        await Promise.all([load(), workspace.loadProjects(true)])
      } catch (error) {
        message.error(error instanceof Error ? error.message : '恢复失败', { duration: 6000 })
      } finally {
        restoringId.value = null
      }
    },
  })
}

function purge(item: RecycleItem) {
  dialog.error({
    title: '永久删除',
    content: `「${item.name}」及其任务、事件和关联配置将被永久删除，无法恢复。确认继续？`,
    positiveText: '永久删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      const key = `${item.type}:${item.id}`
      purgingId.value = key
      try {
        await recycleApi.purge(item.type, item.id)
        message.success('已永久删除')
        selectedKeys.value.delete(itemKey(item))
        selectedKeys.value = new Set(selectedKeys.value)
        await Promise.all([load(), workspace.loadProjects(true)])
      } catch (error) {
        message.error(error instanceof Error ? error.message : '永久删除失败', { duration: 6000 })
      } finally {
        purgingId.value = null
      }
    },
  })
}

function remainingClass(days: number) {
  if (days <= 3) return 'danger'
  if (days <= 7) return 'warning'
  return ''
}

watch(objectType, () => {
  page.value = 1
  load()
})
watch(page, load)
watch(pageSize, () => {
  page.value = 1
  load()
})

onMounted(load)
</script>

<template>
  <div class="page recycle-page">
    <PageHeader
      eyebrow="RECYCLE BIN"
      title="回收站"
      tour-key="recycle"
      subtitle="项目与审核配置删除后进入回收站，保留期内可恢复；到期自动永久清理，超级管理员也可立即永久删除。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" title="刷新" aria-label="刷新" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
      </template>
    </PageHeader>
    <PageSkeleton v-if="showSkeleton" variant="table" />

    <div class="recycle-notice">
      <div class="notice-icon"><ArchiveOutline /></div>
      <div>
        <strong>回收站中的项目不会运行</strong>
        <p>Worker 停止消费、开放 API 拒绝提交、WebSocket 拒绝连接。保留期结束后可由管理员执行永久清理。</p>
      </div>
      <span class="notice-policy">默认保留 {{ workspace.currentProject?.retention_days || 30 }} 天</span>
    </div>

    <section class="panel">
      <div class="recycle-toolbar">
        <n-select
          v-model:value="objectType"
          :options="[
            { label: '项目 PROJECT', value: 'PROJECT' },
            { label: '审核 AUDIT', value: 'AUDIT' },
          ]"
          clearable
          placeholder="对象类型"
          style="width: 180px"
        />
        <n-input
          v-model:value="keyword"
          placeholder="名称 / 编码"
          clearable
          class="keyword-input"
          @keyup.enter="load"
          @clear="load"
        >
          <template #prefix><SearchOutline /></template>
        </n-input>
        <n-button type="primary" secondary @click="load">查询</n-button>
        <span v-if="!loadError" class="total-count">共 {{ total }} 条</span>
      </div>

      <div v-if="total > 0" class="batch-bar">
        <span class="selected-info">
          已选 <b>{{ selectedKeys.size }}</b> 个
          <template v-if="selectedOnPage.length">（本页 {{ selectedOnPage.length }} 个）</template>
        </span>
        <n-button
          size="small"
          type="primary"
          secondary
          :disabled="!selectedKeys.size"
          @click="openConfirm('restore')"
        >
          批量恢复
        </n-button>
        <n-button
          v-if="canPurge"
          size="small"
          quaternary
          class="purge-btn"
          :disabled="!selectedKeys.size"
          @click="openConfirm('purge')"
        >
          批量永久删除
        </n-button>
        <n-button v-if="canPurge" size="small" quaternary class="purge-btn" @click="openConfirm('purge-all')">
          清空回收站
        </n-button>
        <n-button v-if="selectedKeys.size" size="small" quaternary @click="clearSelection">取消选择</n-button>
      </div>

      <div v-if="items.length" class="data-table-wrap">
        <n-data-table
          :columns="recycleColumns"
          :data="items"
          :row-key="itemKey"
          :checked-row-keys="checkedRowKeys"
          :pagination="recyclePagination"
          :scroll-x="1200"
          max-height="calc(100vh - 360px)"
          remote
          striped
          size="small"
          :bordered="false"
          class="sdv-data-table"
          @update:checked-row-keys="onCheckedRowKeysChange"
        />
      </div>

      <EmptyState
        v-else-if="loadError"
        compact
        :title="`回收站加载失败：${loadError}`"
        description="请检查网络或后端服务后点击右上角刷新重试。"
      >
        <template #icon><WarningOutline /></template>
      </EmptyState>

      <EmptyState
        v-else-if="!loading"
        compact
        title="回收站是空的"
        description="被删除的项目与审核配置会出现在这里，并在保留期内支持恢复。"
      >
        <template #icon><TrashOutline /></template>
      </EmptyState>

      <p class="recycle-policy-foot">保留期内可恢复 · 到期对象由后台每 6 小时自动永久清理</p>
    </section>

    <n-modal :show="confirmShow" :mask-closable="false" :close-on-esc="false" display-directive="if">
      <div class="recycle-confirm">
        <div class="confirm-head">
          <span class="confirm-icon" :class="{ danger: confirmMode !== 'restore' }">
            <WarningOutline v-if="confirmMode !== 'restore'" />
            <ArchiveOutline v-else />
          </span>
          <div>
            <h3>{{ confirmTitle }}</h3>
            <p>{{ confirmBody }}</p>
          </div>
        </div>

        <div v-if="batchResults" class="batch-results">
          <div v-for="(r, i) in batchResults.filter(x => x.status === 'skipped')" :key="i" class="result-row">
            <span class="result-name">{{ r.name || `${r.type}#${r.id}` }}</span>
            <span class="result-reason">{{ r.reason }}</span>
          </div>
        </div>

        <div v-else class="confirm-captcha">
          <p class="captcha-tip">拖动滑块完成安全验证后才能执行{{ confirmMode === 'restore' ? '恢复' : '删除' }}</p>
          <SliderCaptcha v-model="confirmCaptcha" />
        </div>

        <div class="confirm-actions">
          <n-button :disabled="confirmBusy" @click="confirmShow = false">
            {{ batchResults ? '关闭' : '取消' }}
          </n-button>
          <n-button
            v-if="!batchResults"
            type="primary"
            :class="{ 'purge-confirm-btn': confirmMode !== 'restore' }"
            :loading="confirmBusy"
            :disabled="!confirmCaptcha"
            @click="submitConfirm"
          >
            {{ confirmMode === 'restore' ? '确认恢复' : confirmMode === 'purge' ? '确认永久删除' : '确认清空' }}
          </n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.recycle-notice {
  display: flex;
  align-items: center;
  gap: 13px;
  margin-bottom: 15px;
  padding: 15px 18px;
  border: 1px solid #f0e4c8;
  border-radius: 13px;
  background: #fffdf7;
}

.notice-icon {
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

.notice-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.recycle-notice strong {
  display: block;
  color: #78350f;
  font-size: 12px;
  font-weight: 600;
}

.recycle-notice p {
  margin: 2px 0 0;
  color: #a16207;
  font-size: 10.5px;
}

.notice-policy {
  margin-left: auto;
  flex-shrink: 0;
  color: #b45309;
  font-family: var(--mono);
  font-size: 9px;
}

.recycle-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
  padding: 16px 18px;
  border-bottom: 1px solid #f0f3f8;
}

.batch-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
  padding: 10px 18px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-soft);
}

.select-all {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--muted);
  font-size: 12px;
  cursor: pointer;
  user-select: none;
}

.selected-info {
  color: var(--muted);
  font-size: 12px;
}

.selected-info b {
  color: var(--brand-strong);
}

.check-col {
  width: 36px;
  text-align: center;
}

.check-col input,
.select-all input {
  width: 15px;
  height: 15px;
  accent-color: var(--brand);
  cursor: pointer;
}

.data-table tr.selected td {
  background: rgba(43, 108, 246, 0.05);
}

.recycle-confirm {
  width: min(460px, calc(100vw - 40px));
  padding: 22px;
  border-radius: 14px;
  background: var(--card, #fff);
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.22);
}

.confirm-head {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.confirm-icon {
  display: inline-flex;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: rgba(43, 108, 246, 0.12);
  color: var(--brand);
}

.confirm-icon.danger {
  background: rgba(220, 38, 38, 0.12);
  color: #dc2626;
}

.confirm-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.confirm-head h3 {
  margin: 2px 0 6px;
  font-size: 15px;
}

.confirm-head p {
  margin: 0;
  color: var(--muted, #64748b);
  font-size: 12.5px;
  line-height: 1.6;
}

.confirm-captcha {
  margin-top: 16px;
}

.captcha-tip {
  margin: 0 0 8px;
  color: var(--muted-light, #94a3b8);
  font-size: 11.5px;
}

.batch-results {
  max-height: 260px;
  margin-top: 14px;
  padding: 10px 12px;
  overflow: auto;
  border-radius: 10px;
  background: var(--surface-soft);
}

.result-row {
  display: flex;
  flex-direction: column;
  padding: 6px 0;
  border-bottom: 1px dashed var(--border);
  font-size: 12px;
}

.result-row:last-child {
  border-bottom: none;
}

.result-name {
  font-weight: 600;
  color: var(--text);
}

.result-reason {
  color: #b45309;
  font-size: 11.5px;
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}

.purge-confirm-btn {
  --n-color: #dc2626 !important;
  --n-color-hover: #b91c1c !important;
  --n-color-pressed: #991b1b !important;
  --n-color-focus: #b91c1c !important;
}

.keyword-input {
  width: 250px;
}

.total-count {
  margin-left: auto;
  color: var(--muted-light);
  font-size: 11px;
}

.item-name {
  display: flex;
  align-items: center;
  gap: 9px;
}

.item-icon {
  display: flex;
  width: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
}

.item-icon.audit {
  background: var(--purple-soft);
  color: var(--purple);
}

.item-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.type-chip {
  padding: 2px 8px;
  border-radius: 5px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 9.5px;
}

.type-chip.audit {
  background: var(--purple-soft);
  color: #6d28d9;
}

.date {
  color: var(--muted-light);
  font-size: 9.5px;
}

.remaining {
  display: inline-flex;
  padding: 2px 8px;
  border-radius: 5px;
  background: var(--green-soft);
  color: #15803d;
  font-family: var(--mono);
  font-size: 9.5px;
}

.remaining.warning {
  background: var(--amber-soft);
  color: #b45309;
}

.remaining.danger {
  background: var(--red-soft);
  color: #b91c1c;
}

.relation {
  color: var(--muted);
  font-size: 10.5px;
}

.purge-btn {
  margin-left: 4px;
  color: #b0bac8;
}

.purge-btn:hover {
  color: var(--red);
}

@media (max-width: 850px) {
  .recycle-notice {
    align-items: flex-start;
  }

  .notice-policy {
    display: none;
  }

  .keyword-input {
    width: 100%;
  }

  .total-count {
    display: none;
  }
}
</style>
