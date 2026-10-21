<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    status?: string | null
    type?: 'task' | 'result' | 'project' | 'audit' | 'provider' | 'env' | 'kind' | 'log'
  }>(),
  { status: '', type: 'task' },
)

const maps: Record<string, Record<string, { label: string; tone: string; pulse?: boolean }>> = {
  task: {
    QUEUED: { label: '排队中', tone: 'blue' },
    PROCESSING: { label: '审核中', tone: 'blue', pulse: true },
    REVIEW_PENDING: { label: '待人工复核', tone: 'amber' },
    SUCCESS: { label: '已完成', tone: 'green' },
    FAILED: { label: '执行失败', tone: 'red' },
    STOPPED: { label: '已停止', tone: 'gray' },
    DUPLICATED: { label: '重复任务', tone: 'gray' },
  },
  result: {
    PASS: { label: '通过', tone: 'green' },
    REJECT: { label: '不通过', tone: 'red' },
    REVIEW: { label: '待议', tone: 'amber' },
    UNKNOWN: { label: '未判定', tone: 'gray' },
  },
  project: {
    ACTIVE: { label: '运行中', tone: 'green' },
    PAUSED: { label: '已暂停', tone: 'amber' },
    DISABLED: { label: '已停用', tone: 'gray' },
    DELETED: { label: '回收站', tone: 'gray' },
  },
  audit: {
    ACTIVE: { label: '已启用', tone: 'green' },
    PAUSED: { label: '已暂停', tone: 'amber' },
    DRAFT: { label: '草稿', tone: 'gray' },
    DELETED: { label: '已删除', tone: 'gray' },
  },
  provider: {
    ACTIVE: { label: '已启用', tone: 'green' },
    DISABLED: { label: '已停用', tone: 'gray' },
  },
  env: {
    prod: { label: '生产', tone: 'red' },
    staging: { label: '预发', tone: 'amber' },
    test: { label: '测试', tone: 'blue' },
    dev: { label: '开发', tone: 'gray' },
  },
  kind: {
    OPENAI_COMPATIBLE: { label: '通用大模型', tone: 'blue' },
    ANTHROPIC: { label: 'Anthropic Claude', tone: 'green' },
    JEV: { label: 'JEV 决策模型', tone: 'purple' },
  },
  log: {
    SUCCESS: { label: '成功', tone: 'green' },
    FAILED: { label: '失败', tone: 'red' },
  },
}

const info = computed(() => {
  const key = props.status || ''
  return maps[props.type]?.[key] || { label: key || '—', tone: 'gray' }
})
</script>

<template>
  <span class="status-pill" :class="[info.tone, { pulse: info.pulse }]">
    <i></i>
    {{ info.label }}
  </span>
</template>
