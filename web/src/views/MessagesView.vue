<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton,
  NInput,
  NModal,
  NPagination,
  NSelect,
  NSwitch,
  useMessage,
} from 'naive-ui'
import {
  CheckmarkDoneOutline,
  NotificationsOutline,
  RefreshOutline,
  SearchOutline,
  SettingsOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import dayjs from 'dayjs'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import { messageApi, notificationPreferenceApi } from '@/api'
import type { InAppMessage, NotificationPreference } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const router = useRouter()
const workspace = useWorkspaceStore()
const message = useMessage()

const items = ref<InAppMessage[]>([])
const total = ref(0)
const unread = ref(0)
const loading = ref(false)
const page = ref(1)
const pageSize = 20
const keyword = ref('')
const statusFilter = ref<'ALL' | 'UNREAD' | 'READ'>('ALL')
const typeFilter = ref('ALL')
const detailShow = ref(false)
const active = ref<InAppMessage | null>(null)
const settingsShow = ref(false)
const prefLoading = ref(false)
const prefSaving = ref(false)
const preference = ref<NotificationPreference | null>(null)
const prefError = ref('')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const canToggleEmail = computed(() => Boolean(preference.value?.has_email && preference.value?.has_channel))
const statusOptions = [
  { label: '全部状态', value: 'ALL' },
  { label: '未读', value: 'UNREAD' },
  { label: '已读', value: 'READ' },
]
const typeOptions = [
  { label: '全部类型', value: 'ALL' },
  { label: '待议复核', value: 'AUDIT_REVIEW_PENDING' },
]

function typeLabel(type?: string) {
  return type === 'AUDIT_REVIEW_PENDING' ? '待议复核' : '系统消息'
}

function formatTime(value?: string | null) {
  return value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '—'
}

async function load() {
  loading.value = true
  try {
    const data = await messageApi.list({
      page: page.value,
      page_size: pageSize,
      keyword: keyword.value.trim() || undefined,
      status: statusFilter.value === 'ALL' ? undefined : statusFilter.value,
      type: typeFilter.value === 'ALL' ? undefined : typeFilter.value,
    })
    items.value = data.items
    total.value = data.total
    unread.value = data.unread_count
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载消息失败')
  } finally {
    loading.value = false
  }
}

function applySearch() {
  page.value = 1
  void load()
}

function resetFilters() {
  keyword.value = ''
  statusFilter.value = 'ALL'
  typeFilter.value = 'ALL'
  page.value = 1
  void load()
}

async function markAllRead() {
  try {
    await messageApi.markAllRead()
    unread.value = 0
    for (const item of items.value) item.read = true
    window.dispatchEvent(new Event('sdv:messages-updated'))
    message.success('全部消息已标记为已读')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '操作失败')
  }
}

async function openSettings() {
  settingsShow.value = true
  prefLoading.value = true
  prefError.value = ''
  try {
    preference.value = await notificationPreferenceApi.get(workspace.currentProjectId || 0)
  } catch (error) {
    prefError.value = error instanceof Error ? error.message : '加载通知设置失败'
    preference.value = null
  } finally {
    prefLoading.value = false
  }
}

async function toggleEmailNotify(value: boolean) {
  if (!preference.value || !canToggleEmail.value || prefSaving.value) return
  prefSaving.value = true
  prefError.value = ''
  try {
    preference.value = await notificationPreferenceApi.update({
      email_notify_enabled: value,
      project_id: workspace.currentProjectId || 0,
    })
    message.success(value ? '已开启邮箱通知' : '已关闭邮箱通知')
  } catch (error) {
    prefError.value = error instanceof Error ? error.message : '更新通知设置失败'
  } finally {
    prefSaving.value = false
  }
}

function goProfile() {
  settingsShow.value = false
  router.push({ name: 'profile' })
}

async function openDetail(item: InAppMessage) {
  active.value = item
  detailShow.value = true
  if (item.read) return
  try {
    const data = await messageApi.markRead(item.id)
    item.read = true
    item.read_at = new Date().toISOString()
    unread.value = data.unread_count
    window.dispatchEvent(new Event('sdv:messages-updated'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : '标记已读失败')
  }
}

function goAction() {
  if (!active.value) return
  if (active.value.project_id) workspace.setCurrentProject(active.value.project_id)
  detailShow.value = false
  router.push(active.value.link || '/reviews')
}

function changePage(value: number) {
  page.value = value
  void load()
}

watch([statusFilter, typeFilter], () => {
  page.value = 1
  void load()
})

onMounted(load)
</script>

<template>
  <div class="page messages-page">
    <PageHeader
      eyebrow="INBOX"
      title="站内消息"
      subtitle="待议复核与系统通知会在这里汇总；打开消息详情后自动标记为已读。"
    >
      <template #actions>
        <n-button secondary @click="openSettings">
          <template #icon><SettingsOutline /></template>
          通知设置
        </n-button>
        <n-button secondary :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
          刷新
        </n-button>
        <n-button type="primary" :disabled="unread === 0" @click="markAllRead">
          <template #icon><CheckmarkDoneOutline /></template>
          全部已读
        </n-button>
      </template>
    </PageHeader>

    <section class="panel message-panel">
      <div class="message-filter-bar">
        <n-input
          v-model:value="keyword"
          class="message-search"
          clearable
          placeholder="搜索标题或内容"
          @keyup.enter="applySearch"
          @clear="applySearch"
        >
          <template #prefix><SearchOutline /></template>
        </n-input>
        <n-button secondary @click="applySearch">搜索</n-button>
        <n-select v-model:value="statusFilter" :options="statusOptions" style="width: 130px" />
        <n-select v-model:value="typeFilter" :options="typeOptions" style="width: 150px" />
        <n-button quaternary @click="resetFilters">重置</n-button>
        <span class="message-count">共 {{ total }} 条 · 全局未读 {{ unread }}</span>
      </div>

      <div v-if="loading" class="message-page-loading">正在加载…</div>
      <div v-else-if="items.length" class="message-table">
        <button
          v-for="item in items"
          :key="item.id"
          type="button"
          class="message-row"
          :class="{ unread: !item.read }"
          @click="openDetail(item)"
        >
          <span class="message-row-dot" :class="{ hidden: item.read }"></span>
          <span class="message-row-main">
            <span class="message-row-head">
              <strong>{{ item.title }}</strong>
              <span class="message-type">{{ typeLabel(item.type) }}</span>
              <span class="message-state" :class="{ unread: !item.read }">{{ item.read ? '已读' : '未读' }}</span>
            </span>
            <span class="message-row-body">{{ item.body }}</span>
          </span>
          <span class="message-row-time">{{ formatTime(item.created_at) }}</span>
        </button>
      </div>
      <EmptyState
        v-else
        title="没有符合条件的消息"
        description="调整搜索关键字或筛选条件后再试。"
        action="刷新"
        @action="load"
      >
        <template #icon><NotificationsOutline /></template>
      </EmptyState>

      <div v-if="total > 0" class="pagination-row sticky-pagination">
        <span>第 {{ page }} 页 · 每页 {{ pageSize }} 条</span>
        <n-pagination
          :page="page"
          :page-count="pageCount"
          :page-size="pageSize"
          show-quick-jumper
          @update:page="changePage"
        />
      </div>
    </section>

    <n-modal v-model:show="settingsShow">
      <div class="notify-settings-modal">
        <div class="notify-settings-head">
          <div>
            <div class="settings-kicker">PERSONAL SETTINGS</div>
            <h2>通知设置</h2>
            <p>配置当前账号的邮箱通知偏好，不会影响其他账号。</p>
          </div>
          <button type="button" class="detail-close" title="关闭" aria-label="关闭" @click="settingsShow = false">×</button>
        </div>
        <div class="notify-settings-body">
          <div v-if="prefLoading" class="message-page-loading">正在加载…</div>
          <template v-else-if="preference">
            <div class="notify-switch-row">
              <div>
                <strong>邮箱通知</strong>
                <p>开启后，所有新的站内消息都会同时发送到当前账号的邮箱 {{ preference.email || '' }}。</p>
              </div>
              <n-switch
                :value="preference.email_notify_enabled"
                :disabled="!canToggleEmail"
                :loading="prefSaving"
                @update:value="toggleEmailNotify"
              />
            </div>
            <div v-if="!preference.has_email" class="notify-reason">
              <WarningOutline />
              <span>你还未配置邮箱地址，暂时无法开启邮箱通知。</span>
              <n-button text type="primary" size="small" @click="goProfile">去个人中心配置</n-button>
            </div>
            <div v-if="!preference.has_channel" class="notify-reason">
              <WarningOutline />
              <span>当前项目还没有可用且测试通过的邮箱通知渠道，请联系管理员配置。</span>
            </div>
            <div v-if="canToggleEmail" class="notify-tip">
              该设置仅对当前账号生效；站内消息仍会保留，开启后所有新消息都会额外发送邮件提醒。
            </div>
          </template>
          <div v-if="prefError" class="modal-error">{{ prefError }}</div>
        </div>
        <div class="notify-settings-foot">
          <n-button @click="settingsShow = false">关闭</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="detailShow">
      <div class="message-detail-modal">
        <div class="message-detail-head">
          <div>
            <div class="detail-kicker">MESSAGE</div>
            <h2>{{ active?.title }}</h2>
            <p>{{ typeLabel(active?.type) }} · {{ formatTime(active?.created_at) }}</p>
          </div>
          <button type="button" class="detail-close" title="关闭" aria-label="关闭" @click="detailShow = false">×</button>
        </div>
        <div class="message-detail-body">
          <p class="message-detail-text">{{ active?.body || '这条消息没有正文。' }}</p>
          <div class="message-detail-meta">
            <span>状态：{{ active?.read ? '已读' : '未读' }}</span>
            <span v-if="active?.task_id">任务 #{{ active.task_id }}</span>
          </div>
        </div>
        <div class="message-detail-footer">
          <n-button @click="detailShow = false">关闭</n-button>
          <n-button v-if="active?.link" type="primary" @click="goAction">前往处理</n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.message-panel {
  padding: 0;
}

.message-filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
  padding: 16px 18px 0;
}

.message-search {
  width: min(300px, 100%);
}

.message-count {
  margin-left: auto;
  color: var(--muted-light);
  font-size: 10px;
}

.message-page-loading {
  padding: 42px 0;
  color: var(--muted-light);
  font-size: 11px;
  text-align: center;
}

.message-table {
  display: flex;
  flex-direction: column;
  padding: 0 18px;
  border-top: 1px solid var(--border);
}

.message-row {
  display: flex;
  width: 100%;
  align-items: flex-start;
  gap: 10px;
  padding: 13px 6px;
  border: 0;
  border-bottom: 1px solid var(--border);
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.message-row:hover {
  background: var(--brand-softer);
}

.message-row.unread {
  background: var(--brand-softer);
}

.message-row-dot {
  width: 7px;
  height: 7px;
  margin-top: 7px;
  border-radius: 50%;
  background: var(--brand);
  flex: none;
}

.message-row-dot.hidden {
  visibility: hidden;
}

.message-row-main {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 4px;
}

.message-row-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 7px;
}

.message-row-head strong {
  overflow: hidden;
  color: var(--text);
  font-size: 12px;
  font-weight: 550;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.message-row.unread .message-row-head strong {
  color: var(--ink);
  font-weight: 650;
}

.message-type {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 8.5px;
}

.message-state {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--muted-light);
  font-size: 8.5px;
}

.message-state.unread {
  background: var(--amber-soft);
  color: var(--amber);
}

.message-row-body {
  display: -webkit-box;
  overflow: hidden;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.55;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.message-row-time {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
  white-space: nowrap;
  flex: none;
}


.message-detail-modal {
  width: min(760px, calc(100vw - 32px));
  max-height: min(80vh, 760px);
  display: flex;
  flex-direction: column;
  border-radius: 18px;
  background: var(--card);
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.message-detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px 16px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-soft);
  border-radius: 18px 18px 0 0;
}

.detail-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.13em;
}

.message-detail-head h2 {
  margin: 5px 0 4px;
  color: var(--ink);
  font-size: 20px;
  font-weight: 660;
  line-height: 1.35;
}

.message-detail-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
}

.detail-close {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--muted-light);
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}

.message-detail-body {
  overflow-y: auto;
  padding: 20px 24px;
}

.message-detail-text {
  margin: 0;
  color: var(--text);
  font-size: 12.5px;
  line-height: 1.8;
  white-space: pre-wrap;
  word-break: break-word;
}

.message-detail-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 18px;
  padding-top: 12px;
  border-top: 1px dashed var(--border);
  color: var(--muted-light);
  font-size: 10px;
}

.message-detail-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 24px 20px;
  border-top: 1px solid var(--border);
}

.notify-settings-modal {
  width: min(560px, calc(100vw - 32px));
  border-radius: 18px;
  background: var(--card);
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.notify-settings-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px 16px;
  border-bottom: 1px solid var(--border);
  border-radius: 18px 18px 0 0;
  background: var(--surface-soft);
}

.settings-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.13em;
}

.notify-settings-head h2 {
  margin: 4px 0 3px;
  color: var(--ink);
  font-size: 20px;
  font-weight: 660;
}

.notify-settings-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
}

.notify-settings-body {
  padding: 20px 24px;
}

.notify-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 14px 0;
}

.notify-switch-row strong {
  display: block;
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 600;
}

.notify-switch-row p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.6;
}

.notify-reason {
  display: flex;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
  padding: 9px 11px;
  border-radius: 8px;
  background: var(--amber-soft);
  color: #a16207;
  font-size: 10.5px;
  line-height: 1.6;
}

.notify-reason :deep(svg) {
  width: 13px;
  height: 13px;
  margin-top: 2px;
  flex: none;
}

.notify-reason > span {
  min-width: 0;
  flex: 1;
}

.notify-tip {
  margin-top: 10px;
  color: var(--muted-light);
  font-size: 10px;
  line-height: 1.6;
}

.notify-settings-foot {
  display: flex;
  justify-content: flex-end;
  padding: 0 24px 20px;
}

.modal-error {
  margin-top: 12px;
  padding: 9px 11px;
  border: 1px solid #fee2e2;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 11px;
}

@media (max-width: 620px) {
  .message-count {
    width: 100%;
    margin-left: 0;
  }

  .message-row {
    flex-wrap: wrap;
  }

  .message-row-time {
    width: 100%;
    padding-left: 17px;
  }
}
</style>
