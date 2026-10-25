<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import {
  NButton,
  NDatePicker,
  NInput,
  NModal,
  NPagination,
  NSelect,
  useMessage,
} from 'naive-ui'
import dayjs from 'dayjs'
import {
  DocumentTextOutline,
  EyeOutline,
  RefreshOutline,
  SearchOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import { logApi } from '@/api'
import type { AuditLog } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const workspace = useWorkspaceStore()
const message = useMessage()
const logs = ref<AuditLog[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const operator = ref('')
const action = ref<string[]>([])
const result = ref<string | null>(null)
const objectType = ref<string | null>(null)
const dateRange = ref<[number, number] | null>(null)
const detailShow = ref(false)
const selected = ref<AuditLog | null>(null)

const actionOptions = [
  { label: '登录', value: 'LOGIN' },
  { label: '登录失败', value: 'LOGIN_FAILED' },
  { label: '项目创建', value: 'PROJECT_CREATE' },
  { label: '项目修改', value: 'PROJECT_UPDATE' },
  { label: '项目删除', value: 'PROJECT_DELETE' },
  { label: '模型接入变更', value: 'PROVIDER_CREATE' },
  { label: '模型测试', value: 'PROVIDER_TEST' },
  { label: '审核配置变更', value: 'AUDIT_CREATE' },
  { label: '任务提交', value: 'TASK_SUBMIT' },
  { label: '任务完成', value: 'TASK_COMPLETE' },
  { label: '任务失败', value: 'TASK_FAILED' },
  { label: '人工复核', value: 'TASK_REVIEW' },
  { label: '任务重试', value: 'TASK_RETRY' },
]

const actionLabels: Record<string, string> = {
  LOGIN: '登录',
  LOGIN_FAILED: '登录失败',
  PROJECT_CREATE: '创建项目',
  PROJECT_UPDATE: '修改项目',
  PROJECT_STATUS: '项目状态变更',
  PROJECT_DELETE: '删除项目',
  PROJECT_RESTORE: '恢复项目',
  PROVIDER_CREATE: '创建模型接入',
  PROVIDER_UPDATE: '更新模型接入',
  PROVIDER_DELETE: '删除模型接入',
  PROVIDER_TEST: '测试模型连接',
  AUDIT_CREATE: '创建审核配置',
  AUDIT_UPDATE: '更新审核配置',
  AUDIT_STATUS: '审核启停',
  AUDIT_DELETE: '删除审核配置',
  AUDIT_CLONE: '复制审核配置',
  AUDIT_RESTORE: '恢复审核配置',
  TASK_SUBMIT: '提交审核任务',
  TASK_COMPLETE: '任务审核完成',
  TASK_FAILED: '任务执行失败',
  TASK_RETRY: '重试任务',
  TASK_BATCH_RETRY: '批量重试',
  TASK_REVIEW: '人工复核',
}

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = {
      project_id: workspace.currentProjectId || undefined,
      page: page.value,
      page_size: pageSize.value,
    }
    if (operator.value.trim()) params.operator = operator.value.trim()
    if (action.value.length) params.action = action.value.join(',')
    if (result.value) params.result = result.value
    if (objectType.value) params.object_type = objectType.value
    if (dateRange.value) {
      params.start = dayjs(dateRange.value[0]).format('YYYY-MM-DD')
      params.end = dayjs(dateRange.value[1]).format('YYYY-MM-DD')
    }
    const data = await logApi.list(params)
    logs.value = data.items
    total.value = data.total
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载审计日志失败')
  } finally {
    loading.value = false
  }
}

function reset() {
  operator.value = ''
  action.value = []
  result.value = null
  objectType.value = null
  dateRange.value = null
  page.value = 1
  load()
}

function openDetail(log: AuditLog) {
  selected.value = log
  detailShow.value = true
}

function actionLabel(value: string) {
  return actionLabels[value] || value
}

watch([action, result, objectType, dateRange, () => workspace.currentProjectId], () => {
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
  <div class="page logs-page">
    <PageHeader
      eyebrow="AUDIT TRAIL"
      title="审计日志"
      subtitle="记录登录、配置变更、模型测试、任务流转与人工复核等全部敏感操作。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
      </template>
    </PageHeader>

    <div class="retention-banner">
      <ShieldCheckmarkOutline />
      <span>日志默认按设计规范留存 180 天以上；包含操作人、IP、对象、结果与 trace_id，可用于全链路追溯。</span>
    </div>

    <section class="panel">
      <div class="log-toolbar">
        <n-input
          v-model:value="operator"
          placeholder="操作人"
          clearable
          class="operator-input"
          @keyup.enter="load"
          @clear="load"
        >
          <template #prefix><SearchOutline /></template>
        </n-input>
        <n-select
          v-model:value="action"
          :options="actionOptions"
          multiple
          clearable
          placeholder="操作类型"
          max-tag-count="responsive"
          style="width: 190px"
        />
        <n-select
          v-model:value="objectType"
          :options="[
            { label: '项目 PROJECT', value: 'PROJECT' },
            { label: '模型 PROVIDER', value: 'PROVIDER' },
            { label: '审核 AUDIT', value: 'AUDIT' },
            { label: '任务 TASK', value: 'TASK' },
            { label: '用户 USER', value: 'USER' },
          ]"
          clearable
          placeholder="对象类型"
          style="width: 140px"
        />
        <n-select
          v-model:value="result"
          :options="[
            { label: '成功', value: 'SUCCESS' },
            { label: '失败', value: 'FAILED' },
          ]"
          clearable
          placeholder="结果"
          style="width: 100px"
        />
        <n-date-picker v-model:value="dateRange" type="daterange" clearable style="width: 220px" />
        <n-button quaternary @click="reset">重置</n-button>
        <span class="total-count">共 {{ total }} 条</span>
      </div>

      <div v-if="logs.length" class="data-table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>时间</th>
              <th>操作人</th>
              <th>操作</th>
              <th>对象</th>
              <th>结果</th>
              <th>Trace ID</th>
              <th class="action-col" style="text-align: right">详情</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id">
              <td><span class="log-time mono">{{ log.created_at?.replace('T', ' ').slice(0, 19) }}</span></td>
              <td>
                <div class="operator-cell">
                  <span class="mini-avatar">{{ log.operator?.slice(0, 1).toUpperCase() }}</span>
                  <div>
                    <span class="cell-title">{{ log.operator }}</span>
                    <span class="cell-sub mono">{{ log.operator_ip || '—' }}</span>
                  </div>
                </div>
              </td>
              <td>
                <div class="cell-title">{{ actionLabel(log.action) }}</div>
                <div class="cell-sub mono">{{ log.action }}</div>
              </td>
              <td>
                <div class="cell-title">{{ log.object_type || '—' }}</div>
                <div class="cell-sub mono">{{ log.object_id || '—' }}</div>
              </td>
              <td><StatusTag :status="log.result" type="log" /></td>
              <td><span class="trace mono">{{ log.trace_id ? log.trace_id.slice(0, 12) : '—' }}</span></td>
              <td class="action-col" style="text-align: right">
                <n-button size="tiny" quaternary @click="openDetail(log)">
                  <template #icon><EyeOutline /></template>
                </n-button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <EmptyState v-else-if="!loading" compact title="没有匹配的审计记录" description="调整筛选条件后再试。">
        <template #icon><DocumentTextOutline /></template>
      </EmptyState>

      <div class="pagination-row">
        <span>共 {{ total }} 条记录</span>
        <n-pagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-sizes="[20, 50, 100]"
          show-size-picker
          :item-count="total"
        />
      </div>
    </section>

    <n-modal v-model:show="detailShow">
      <div class="log-modal">
        <div class="log-modal-head">
          <div>
            <div class="kicker">AUDIT LOG #{{ selected?.id }}</div>
            <h3>{{ selected ? actionLabel(selected.action) : '日志详情' }}</h3>
          </div>
          <StatusTag v-if="selected" :status="selected.result" type="log" />
        </div>
        <div v-if="selected" class="log-modal-body">
          <div class="kv-grid">
            <div><div class="kv-label">操作人</div><div class="kv-value">{{ selected.operator }}（{{ selected.operator_ip || '—' }}）</div></div>
            <div><div class="kv-label">时间</div><div class="kv-value mono">{{ selected.created_at }}</div></div>
            <div><div class="kv-label">对象</div><div class="kv-value">{{ selected.object_type }} / {{ selected.object_id }}</div></div>
            <div><div class="kv-label">Trace ID</div><div class="kv-value mono">{{ selected.trace_id || '—' }}</div></div>
          </div>
          <h4 class="section-label">详情</h4>
          <pre class="code-block">{{ JSON.stringify(selected.detail, null, 2) || 'null' }}</pre>
        </div>
        <div class="log-modal-foot">
          <n-button @click="detailShow = false">关闭</n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.retention-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  padding: 10px 14px;
  border: 1px solid #e1eaf8;
  border-radius: 10px;
  background: #f8fbff;
  color: #64748b;
  font-size: 10.5px;
}

.retention-banner :deep(svg) {
  width: 14px;
  height: 14px;
  color: var(--brand);
  flex-shrink: 0;
}

.log-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
  padding: 16px 18px;
  border-bottom: 1px solid #f0f3f8;
}

.operator-input {
  width: 150px;
}

.total-count {
  margin-left: auto;
  color: var(--muted-light);
  font-size: 11px;
}

.log-time {
  color: var(--muted);
  font-size: 9.5px;
}

.operator-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mini-avatar {
  display: flex;
  width: 27px;
  height: 27px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 10px;
  font-weight: 600;
}

.operator-cell .cell-title {
  display: block;
  font-size: 11.5px;
}

.operator-cell .cell-sub {
  display: block;
  font-size: 9px;
}

.trace {
  padding: 2px 6px;
  border-radius: 5px;
  background: #f8fafc;
  color: #94a3b8;
  font-size: 9px;
}

.log-modal {
  width: min(620px, calc(100vw - 32px));
  border-radius: 17px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.log-modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 22px 24px 16px;
  border-bottom: 1px solid #f0f3f8;
}

.kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.1em;
}

.log-modal-head h3 {
  margin: 4px 0 0;
  font-size: 18px;
  font-weight: 650;
}

.log-modal-body {
  max-height: 65vh;
  overflow-y: auto;
  padding: 20px 24px;
}

.log-modal-foot {
  display: flex;
  justify-content: flex-end;
  padding: 14px 24px;
  border-top: 1px solid #f0f3f8;
}

@media (max-width: 900px) {
  .operator-input {
    width: 100%;
  }

  .total-count {
    display: none;
  }
}
</style>
