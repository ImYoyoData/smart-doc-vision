<script setup lang="ts">
import { computed } from 'vue'
import { NPopover } from 'naive-ui'
import type { AssistantUsage } from '@/api/types'

const props = defineProps<{
  usage: AssistantUsage | null
}>()

const contextWindow = computed(() => props.usage?.context_window || 128000)
const contextTokens = computed(() => props.usage?.context_tokens || 0)
const contextPercent = computed(() => {
  if (!contextWindow.value) return 0
  return Math.min(100, Math.round((contextTokens.value / contextWindow.value) * 1000) / 10)
})
const contextTone = computed(() => {
  if (contextPercent.value >= 85) return 'danger'
  if (contextPercent.value >= 60) return 'warn'
  return 'ok'
})
const contextRingOffset = computed(() => {
  const circumference = 2 * Math.PI * 15
  return circumference * (1 - Math.min(1, contextTokens.value / contextWindow.value))
})

function formatTokens(value?: number) {
  if (!value || value <= 0) return '0'
  return value.toLocaleString('en-US')
}
</script>

<template>
  <n-popover trigger="click" placement="top-end" :show-arrow="false" raw>
    <template #trigger>
      <button
        type="button"
        class="ai-context-usage"
        :class="contextTone"
        :title="`上下文 ${formatTokens(contextTokens)} / ${formatTokens(contextWindow)} tokens`"
        aria-label="查看上下文用量"
      >
        <svg viewBox="0 0 36 36" class="ai-context-ring" aria-hidden="true">
          <circle class="ring-track" cx="18" cy="18" r="15" />
          <circle
            class="ring-fill"
            cx="18"
            cy="18"
            r="15"
            stroke-dasharray="94.25"
            :stroke-dashoffset="contextRingOffset"
          />
        </svg>
        <span>{{ contextPercent }}%</span>
      </button>
    </template>
    <div class="ai-context-pop">
      <div class="context-pop-head">
        <span>上下文用量</span>
        <strong>{{ formatTokens(contextTokens) }} / {{ formatTokens(contextWindow) }}</strong>
      </div>
      <div class="context-pop-bar">
        <span :class="contextTone" :style="{ width: `${Math.max(contextPercent, contextTokens ? 2 : 0)}%` }" />
      </div>
      <div class="context-pop-grid">
        <div><span>窗口</span><strong>{{ formatTokens(contextWindow) }}</strong></div>
        <div><span>已用</span><strong>{{ formatTokens(contextTokens) }}</strong></div>
        <div><span>剩余</span><strong>{{ formatTokens(Math.max(0, contextWindow - contextTokens)) }}</strong></div>
        <div><span>请求次数</span><strong>{{ usage?.requests || 0 }}</strong></div>
        <div><span>累计输入</span><strong>{{ formatTokens(usage?.input_tokens) }}</strong></div>
        <div><span>累计输出</span><strong>{{ formatTokens(usage?.output_tokens) }}</strong></div>
        <div><span>缓存命中</span><strong>{{ formatTokens(usage?.cached_tokens) }}</strong></div>
        <div><span>累计总量</span><strong>{{ formatTokens(usage?.total_tokens) }}</strong></div>
      </div>
      <div class="context-pop-note">
        {{ usage?.model || '尚未调用模型' }} · 按最近一次请求的输入 + 输出估算；接近上限会自动压缩历史对话。
      </div>
    </div>
  </n-popover>
</template>

<style scoped>
.ai-context-usage {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 7px 3px 5px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface-soft);
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9px;
  cursor: pointer;
  flex: none;
}

.ai-context-usage:hover {
  border-color: var(--brand-border);
  color: var(--brand);
}

.ai-context-ring {
  width: 18px;
  height: 18px;
  transform: rotate(-90deg);
}

.ai-context-ring .ring-track {
  fill: none;
  stroke: var(--border);
  stroke-width: 3;
}

.ai-context-ring .ring-fill {
  fill: none;
  stroke: var(--brand);
  stroke-width: 3;
  stroke-linecap: round;
  transition: stroke-dashoffset 0.3s ease;
}

.ai-context-usage.warn .ring-fill {
  stroke: var(--amber);
}

.ai-context-usage.danger {
  color: var(--red);
}

.ai-context-usage.danger .ring-fill {
  stroke: var(--red);
}

.ai-context-pop {
  width: 276px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--card);
  box-shadow: var(--shadow-md);
  color: var(--text);
}

.context-pop-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
}

.context-pop-head span {
  color: var(--muted);
  font-size: 10px;
}

.context-pop-head strong {
  color: var(--ink);
  font-family: var(--mono);
  font-size: 10px;
}

.context-pop-bar {
  height: 5px;
  margin: 9px 0 10px;
  overflow: hidden;
  border-radius: 99px;
  background: var(--surface-strong);
}

.context-pop-bar span {
  display: block;
  height: 100%;
  border-radius: 99px;
  background: var(--brand);
}

.context-pop-bar span.warn {
  background: var(--amber);
}

.context-pop-bar span.danger {
  background: var(--red);
}

.context-pop-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 7px 12px;
}

.context-pop-grid div {
  min-width: 0;
}

.context-pop-grid span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
}

.context-pop-grid strong {
  display: block;
  overflow: hidden;
  color: var(--text);
  font-family: var(--mono);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-pop-note {
  margin-top: 10px;
  padding-top: 9px;
  border-top: 1px solid var(--border);
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.55;
}
</style>
