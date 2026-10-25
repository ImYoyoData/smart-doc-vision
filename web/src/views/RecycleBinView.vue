<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { NButton, NInput, NModal, NPagination, NSelect, useDialog, useMessage } from 'naive-ui'
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

const pageKeys = computed(() => new Set(items.value.map((i) => `${i.type}:${i.id}`)))
const selectedOnPage = computed(() => items.value.filter((i) => selectedKeys.value.has(`${i.type}:${i.id}`)))
const allOnPageSelected = computed(
  () => items.value.length > 0 && items.value.every((i) => selectedKeys.value.has(`${i.type}:${i.id}`)),
)

function itemKey(item: RecycleItem) {
  return `${item.type}:${item.id}`
}

function toggleItem(item: RecycleItem) {
  const key = itemKey(item)
  if (selectedKeys.value.has(key)) selectedKeys.value.delete(key)
  else selectedKeys.value.add(key)
  selectedKeys.value = new Set(selectedKeys.value)
}

function togglePage() {
  const next = new Set(selectedKeys.value)
  if (allOnPageSelected.value) {
    for (const key of pageKeys.value) next.delete(key)
  } else {
    for (const key of pageKeys.value) next.add(key)
  }
  selectedKeys.value = next
}

function clearSelection() {
  selectedKeys.value = new Set()
}

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
      subtitle="项目与审核配置删除后进入回收站，保留期内可恢复；到期自动永久清理，超级管理员也可立即永久删除。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" title="刷新" aria-label="刷新" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
      </template>
    </PageHeader>

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
        <label class="select-all">
          <input type="checkbox" :checked="allOnPageSelected" @change="togglePage" />
          本页全选
        </label>
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
        <table class="data-table">
          <thead>
            <tr>
              <th class="check-col"></th>
              <th>名称</th>
              <th>类型</th>
              <th>所属项目</th>
              <th>删除时间</th>
              <th>剩余保留</th>
              <th>关联</th>
              <th class="action-col" style="text-align: right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="`${item.type}:${item.id}`" :class="{ selected: selectedKeys.has(`${item.type}:${item.id}`) }">
              <td class="check-col">
                <input
                  type="checkbox"
                  :checked="selectedKeys.has(`${item.type}:${item.id}`)"
                  @change="toggleItem(item)"
                />
              </td>
              <td>
                <div class="item-name">
                  <span class="item-icon" :class="{ audit: item.type === 'AUDIT' }">
                    <FolderOpenOutline v-if="item.type === 'PROJECT'" />
                    <ShieldCheckmarkOutline v-else />
                  </span>
                  <div>
                    <div class="cell-title">{{ item.name }}</div>
                    <div class="cell-sub mono">{{ item.code }}</div>
                  </div>
                </div>
              </td>
              <td>
                <span class="type-chip" :class="{ audit: item.type === 'AUDIT' }">
                  {{ item.type === 'PROJECT' ? '项目' : '审核配置' }}
                </span>
              </td>
              <td><span class="ink-soft">{{ item.project_name }}</span></td>
              <td><span class="date mono">{{ item.deleted_at?.replace('T', ' ').slice(0, 19) }}</span></td>
              <td>
                <span class="remaining" :class="remainingClass(item.remaining_days)">
                  {{ item.remaining_days > 0 ? `${item.remaining_days} 天` : '已过期' }}
                </span>
              </td>
              <td>
                <span class="relation">{{ item.task_count }} 个任务</span>
              </td>
              <td class="action-col" style="text-align: right">
                <n-button
                  size="small"
                  type="primary"
                  secondary
                  :loading="restoringId === `${item.type}:${item.id}`"
                  :disabled="!item.can_restore"
                  @click="restore(item)"
                >
                  恢复
                </n-button>
                <n-button
                  v-if="canPurge"
                  size="small"
                  quaternary
                  class="purge-btn"
                  :loading="purgingId === `${item.type}:${item.id}`"
                  @click="purge(item)"
                >
                  永久删除
                </n-button>
              </td>
            </tr>
          </tbody>
        </table>
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

      <div class="pagination-row">
        <span>保留期内可恢复 · 到期对象由后台每 6 小时自动永久清理</span>
        <n-pagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-sizes="[20, 50, 100]"
          show-size-picker
          :item-count="total"
        />
      </div>
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
  border-bottom: 1px solid #f0f3f8;
  background: #f8fafc;
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
  background: #f8fafc;
}

.result-row {
  display: flex;
  flex-direction: column;
  padding: 6px 0;
  border-bottom: 1px dashed #e2e8f0;
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
