<script setup lang="ts">
import { computed, watch } from 'vue'
import { NButton, NModal } from 'naive-ui'
import dayjs from 'dayjs'
import type { InAppMessage } from '@/api/types'

const props = defineProps<{
  show: boolean
  message: InAppMessage | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  read: [message: InAppMessage]
  action: [message: InAppMessage]
}>()

const typeLabel = computed(() => (props.message?.type === 'AUDIT_REVIEW_PENDING' ? '待议复核' : '系统消息'))

function formatTime(value?: string | null) {
  return value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '—'
}

watch(
  () => [props.show, props.message?.id, props.message?.read] as const,
  ([show, , read]) => {
    if (show && props.message && !read) emit('read', props.message)
  },
  { immediate: true },
)
</script>

<template>
  <n-modal :show="show" @update:show="(value) => emit('update:show', value)">
    <div class="message-detail-modal">
      <div class="message-detail-head">
        <div>
          <div class="detail-kicker">MESSAGE</div>
          <h2>{{ message?.title }}</h2>
          <p>{{ typeLabel }} · {{ formatTime(message?.created_at) }}</p>
        </div>
        <button type="button" class="detail-close" title="关闭" aria-label="关闭" @click="emit('update:show', false)">×</button>
      </div>
      <div class="message-detail-body">
        <p class="message-detail-text">{{ message?.body || '这条消息没有正文。' }}</p>
        <div class="message-detail-meta">
          <span>状态：{{ message?.read ? '已读' : '未读' }}</span>
          <span v-if="message?.task_id">任务 #{{ message.task_id }}</span>
        </div>
      </div>
      <div class="message-detail-footer">
        <n-button @click="emit('update:show', false)">关闭</n-button>
        <n-button v-if="message?.link" type="primary" @click="message && emit('action', message)">前往处理</n-button>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.message-detail-modal {
  display: flex;
  width: min(760px, calc(100vw - 32px));
  max-height: min(80vh, 760px);
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
  border-radius: 18px 18px 0 0;
  background: var(--surface-soft);
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
</style>
