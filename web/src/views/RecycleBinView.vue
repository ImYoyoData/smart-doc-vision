<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { NButton, NInput, NPagination, NSelect, useDialog, useMessage } from 'naive-ui'
import {
  ArchiveOutline,
  FolderOpenOutline,
  RefreshOutline,
  SearchOutline,
  ShieldCheckmarkOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import { auditApi, projectApi, recycleApi } from '@/api'
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
const restoringId = ref<string | null>(null)
const purgingId = ref<string | null>(null)
const canPurge = computed(() => auth.user?.role === 'SUPER_ADMIN')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

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
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载回收站失败')
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
        <n-button quaternary circle :loading="loading" @click="load">
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
        <span class="total-count">共 {{ total }} 条</span>
      </div>

      <div v-if="items.length" class="data-table-wrap">
        <table class="data-table">
          <thead>
            <tr>
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
            <tr v-for="item in items" :key="`${item.type}:${item.id}`">
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
          :page-count="pageCount"
          :page-sizes="[20, 50, 100]"
          show-size-picker
          :item-count="total"
        />
      </div>
    </section>
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
