<script setup lang="ts">
import { computed } from 'vue'
import { ArrowUpOutline, ArrowDownOutline } from '@vicons/ionicons5'

const props = withDefaults(
  defineProps<{
    label: string
    value: string | number
    suffix?: string
    hint?: string
    trend?: number | null
    tone?: 'blue' | 'green' | 'amber' | 'red' | 'purple'
    loading?: boolean
  }>(),
  { suffix: '', hint: '', trend: null, tone: 'blue', loading: false },
)

const trendText = computed(() => {
  if (props.trend === null || props.trend === undefined) return ''
  return `${props.trend > 0 ? '+' : ''}${props.trend}%`
})
</script>

<template>
  <div class="metric-card" :class="tone">
    <div class="metric-top">
      <span class="metric-label">{{ label }}</span>
      <span class="metric-icon">
        <slot name="icon" />
      </span>
    </div>
    <div class="metric-value" :class="{ skeleton: loading }">
      <span v-if="!loading">{{ value }}</span>
      <span v-if="!loading && suffix" class="metric-suffix">{{ suffix }}</span>
    </div>
    <div class="metric-foot">
      <span v-if="hint" class="metric-hint">{{ hint }}</span>
      <span v-if="trend !== null" class="metric-trend" :class="{ down: Number(trend) < 0 }">
        <component :is="Number(trend) < 0 ? ArrowDownOutline : ArrowUpOutline" />
        {{ trendText }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.metric-card {
  position: relative;
  overflow: hidden;
  padding: 18px 20px 17px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
}

.metric-card::after {
  content: '';
  position: absolute;
  top: -42px;
  right: -34px;
  width: 110px;
  height: 110px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(43, 108, 246, 0.08), transparent 68%);
  pointer-events: none;
}

.metric-card.green::after {
  background: radial-gradient(circle, rgba(22, 163, 74, 0.09), transparent 68%);
}
.metric-card.amber::after {
  background: radial-gradient(circle, rgba(245, 158, 11, 0.11), transparent 68%);
}
.metric-card.red::after {
  background: radial-gradient(circle, rgba(220, 38, 38, 0.09), transparent 68%);
}
.metric-card.purple::after {
  background: radial-gradient(circle, rgba(124, 58, 237, 0.09), transparent 68%);
}

.metric-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.metric-label {
  color: var(--muted);
  font-size: 12.5px;
  font-weight: 500;
}

.metric-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: var(--brand-soft);
  color: var(--brand);
}

.metric-icon :deep(svg) {
  width: 16px;
  height: 16px;
}

.green .metric-icon {
  background: var(--green-soft);
  color: var(--green);
}
.amber .metric-icon {
  background: var(--amber-soft);
  color: #d97706;
}
.red .metric-icon {
  background: var(--red-soft);
  color: var(--red);
}
.purple .metric-icon {
  background: var(--purple-soft);
  color: var(--purple);
}

.metric-value {
  display: flex;
  align-items: baseline;
  gap: 5px;
  min-height: 46px;
  margin-top: 12px;
  color: var(--ink);
  font-size: 30px;
  font-weight: 680;
  letter-spacing: -0.035em;
  line-height: 1.2;
}

.metric-value.skeleton {
  width: 72px;
  border-radius: 8px;
  background: linear-gradient(90deg, #eef2f8, #f7f9fc, #eef2f8);
  background-size: 200% 100%;
  animation: shimmer 1.4s ease infinite;
}

@keyframes shimmer {
  to {
    background-position: -200% 0;
  }
}

.metric-suffix {
  color: var(--muted);
  font-size: 13px;
  font-weight: 500;
}

.metric-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 19px;
  margin-top: 5px;
}

.metric-hint {
  color: var(--muted-light);
  font-size: 11.5px;
}

.metric-trend {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 1px 6px;
  border-radius: 99px;
  color: #15803d;
  background: var(--green-soft);
  font-size: 10.5px;
  font-weight: 600;
}

.metric-trend.down {
  color: #b91c1c;
  background: var(--red-soft);
}

.metric-trend :deep(svg) {
  width: 10px;
  height: 10px;
}
</style>
