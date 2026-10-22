<script setup lang="ts">
import { computed } from 'vue'
import { AlertCircleOutline, InformationCircleOutline, ShieldCheckmarkOutline } from '@vicons/ionicons5'
import StatusTag from './StatusTag.vue'
import type { Task } from '@/api/types'

const props = withDefaults(defineProps<{
  task: Task | null
  compact?: boolean
}>(), {
  compact: false,
})

const detail = computed(() => props.task?.result_detail || null)
const score = computed(() => {
  const value = detail.value?.score ?? props.task?.model_result?.score ?? 0
  return Math.round(Math.max(0, Math.min(1, Number(value))) * 100)
})
const confidence = computed(() => {
  const category = detail.value?.category || props.task?.model_result?.category
  if (!category) return null
  const value = detail.value?.category_confidence ?? props.task?.model_result?.category_confidence
  return value === undefined || value === null ? null : Math.round(Number(value) * 100)
})
const riskLevel = computed(() => detail.value?.risk_level || props.task?.model_result?.risk_level || '—')
const riskTone = computed(() => {
  if (riskLevel.value === 'high') return 'high'
  if (riskLevel.value === 'medium') return 'medium'
  return 'low'
})
const trace = computed(() => props.task?.rule_trace?.trace || [])
const matchedRules = computed(() => trace.value.filter((group) => group.hit))
const reviewRules = computed(() => matchedRules.value.filter((group) => group.on_hit_result === 'REVIEW'))
const labels = computed(() => detail.value?.labels || props.task?.model_result?.labels || [])
const reasons = computed(() => detail.value?.reasons || props.task?.model_result?.reasons || [])

const reviewReasons = computed(() => {
  if (!props.task) return []
  const out: string[] = []
  const modelError = detail.value?.model_error
  if (modelError) out.push(`模型调用异常，按兜底策略进入人工复核：${modelError}`)
  for (const group of reviewRules.value) {
    out.push(`规则「${group.name}」命中，且配置的命中结果为 REVIEW`)
  }
  if (confidence.value !== null && confidence.value < 60) {
    out.push(`分类置信度仅 ${confidence.value}%，模型对类型判断不够确定`)
  }
  if (score.value >= 30 && score.value < 85) {
    out.push(`模型风险分 ${score.value} 处于灰区，未达到自动放行或自动驳回条件`)
  }
  if (props.task.result === 'REVIEW') {
    const summary = detail.value?.summary || props.task.model_result?.summary
    out.push(`模型最终结论为 REVIEW${summary ? `：${summary}` : '，表示信息不足或无法确认'}`)
  }
  if (!out.length && matchedRules.value.length) {
    out.push('命中规则未直接给出终审结论，任务转入人工复核')
  }
  if (!out.length) {
    out.push('审核配置或规则要求该任务由人工确认后再给出终审结论')
  }
  return [...new Set(out)]
})

const decisionHints = computed(() => {
  const hints: string[] = []
  if (detail.value?.category) {
    hints.push(`分类结果：${detail.value.category_name || detail.value.category}${confidence.value !== null ? `（置信度 ${confidence.value}%）` : ''}`)
  }
  if (labels.value.length) hints.push(`重点关注：${labels.value.slice(0, 6).join('、')}`)
  if (reasons.value.length) hints.push(`模型依据：${reasons.value.slice(0, 2).join('；')}`)
  if (detail.value?.model_error) hints.push('模型异常：请以原始内容和业务规则为准')
  return hints
})
</script>

<template>
  <section v-if="task" class="review-context" :class="{ compact }">
    <div class="context-head">
      <span class="context-icon"><ShieldCheckmarkOutline /></span>
      <div>
        <strong>复核上下文</strong>
        <span>系统为什么把这条任务转到这里，以及当前模型已经给出的判断</span>
      </div>
      <StatusTag :status="task.status" type="task" />
    </div>

    <div class="review-reasons">
      <div v-for="(reason, index) in reviewReasons" :key="index" class="review-reason">
        <InformationCircleOutline />
        <span>{{ reason }}</span>
      </div>
    </div>

    <div class="context-facts">
      <div>
        <span>当前结论</span>
        <StatusTag :status="task.result" type="result" />
      </div>
      <div>
        <span>风险分</span>
        <strong class="risk-score" :class="riskTone">{{ score }}<small>/100</small></strong>
      </div>
      <div>
        <span>风险等级</span>
        <strong class="risk-level" :class="riskTone">{{ riskLevel === 'high' ? '高' : riskLevel === 'medium' ? '中' : riskLevel === 'low' ? '低' : riskLevel }}</strong>
      </div>
      <div v-if="confidence !== null">
        <span>分类置信度</span>
        <strong>{{ confidence }}%</strong>
      </div>
      <div>
        <span>规则命中</span>
        <strong>{{ matchedRules.length ? `${matchedRules.length} 条` : '无' }}</strong>
      </div>
      <div>
        <span>复核等待</span>
        <strong>{{ task.created_at ? task.created_at.replace('T', ' ').slice(0, 16) : '—' }} 起</strong>
      </div>
    </div>

    <div v-if="!compact && decisionHints.length" class="decision-hints">
      <strong>复核时建议核对</strong>
      <ul>
        <li v-for="hint in decisionHints" :key="hint">{{ hint }}</li>
      </ul>
    </div>

    <div v-if="task.result_detail?.model_error" class="model-error">
      <AlertCircleOutline />
      <span>{{ task.result_detail.model_error }}</span>
    </div>
  </section>
</template>

<style scoped>
.review-context {
  display: flex;
  flex-direction: column;
  gap: 11px;
  padding: 14px 16px;
  border: 1px solid #f0dfbd;
  border-radius: 12px;
  background: linear-gradient(140deg, #fffdf8, #fffaf0);
}

.review-context.compact {
  gap: 8px;
  padding: 11px 12px;
}

.context-head {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.context-head > div {
  min-width: 0;
  flex: 1;
}

.context-icon {
  display: flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  background: #fef3c7;
  color: #b45309;
  flex-shrink: 0;
}

.context-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.context-head strong {
  display: block;
  color: #78350f;
  font-size: 11.5px;
  font-weight: 650;
}

.context-head span {
  display: block;
  color: #a16207;
  font-size: 9.5px;
  line-height: 1.55;
}

.review-reasons {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.review-reason {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 7px 9px;
  border-radius: 7px;
  background: rgba(245, 158, 11, 0.08);
  color: #854d0e;
  font-size: 10px;
  line-height: 1.6;
}

.review-reason :deep(svg) {
  width: 12px;
  height: 12px;
  margin-top: 2px;
  flex-shrink: 0;
}

.context-facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 7px;
}

.context-facts > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
  padding: 6px 8px;
  border: 1px solid rgba(240, 223, 189, 0.7);
  border-radius: 7px;
  background: rgba(255, 255, 255, 0.66);
}

.context-facts span {
  color: #a16207;
  font-size: 8.5px;
}

.context-facts strong {
  color: var(--text);
  font-size: 10px;
  font-weight: 550;
}

.risk-score small {
  margin-left: 2px;
  color: var(--muted-light);
  font-size: 8px;
  font-weight: 400;
}

.risk-score.high,
.risk-level.high {
  color: #b91c1c;
}

.risk-score.medium,
.risk-level.medium {
  color: #b45309;
}

.risk-score.low,
.risk-level.low {
  color: #15803d;
}

.decision-hints {
  padding: 8px 10px;
  border: 1px solid #e8eef7;
  border-radius: 8px;
  background: #f8fbff;
}

.decision-hints strong {
  color: #2854a8;
  font-size: 9.5px;
}

.decision-hints ul {
  margin: 5px 0 0;
  padding-left: 15px;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.7;
}

.model-error {
  display: flex;
  align-items: flex-start;
  gap: 5px;
  color: #b91c1c;
  font-size: 9.5px;
  line-height: 1.5;
}

.model-error :deep(svg) {
  width: 12px;
  height: 12px;
  margin-top: 1px;
  flex-shrink: 0;
}

@media (max-width: 720px) {
  .context-facts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
