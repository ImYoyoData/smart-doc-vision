<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NModal, NSelect, NSpin, type InputInst } from 'naive-ui'
import { CloseOutline, SparklesOutline } from '@vicons/ionicons5'
import VoiceInputButton from '@/components/VoiceInputButton.vue'
import { auditApi } from '@/api'
import type { AuditFieldAssistantResult } from '@/api/types'

const props = defineProps<{
  show: boolean
  field: 'rules' | 'review_prompt'
  current: string
  context: Record<string, unknown>
  projectId: number | null
  providerOptions: Array<{ label: string; value: number }>
  initialProviderId?: number | null
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'apply', value: string): void
}>()

const instructionRef = ref<InputInst | null>(null)
const instruction = ref('')
const providerId = ref<number | null>(null)
const busy = ref(false)
const error = ref('')
const result = ref<AuditFieldAssistantResult | null>(null)
let requestSeq = 0

const isRules = computed(() => props.field === 'rules')
const kicker = computed(() => (isRules.value ? 'AI RULE BUILDER' : 'AI PROMPT POLISH'))
const title = computed(() => (isRules.value ? '让 AI 生成审核规则' : '让 AI 优化审核提示词'))
const description = computed(() =>
  isRules.value
    ? '描述你希望如何判定，AI 会结合当前审核类型和分类，生成可直接执行的规则 JSON。'
    : '描述需要强调或调整的审核点，AI 会在当前审核要求的基础上重写一版提示词。',
)
const placeholder = computed(() =>
  isRules.value
    ? '例如：风险分 ≥ 0.8 直接驳回；分类置信度低于 0.65 转人工；命中敏感信息标签直接驳回'
    : '例如：更严格检查缺页、印章和签名；发现身份证号必须驳回；无法确认时转人工',
)
const examples = computed(() =>
  isRules.value
    ? ['高风险直接驳回，低风险快速通过', '低置信度分类结果转人工', '增加敏感信息和缺页规则']
    : ['提高审核严格度，明确驳回红线', '补充缺页、印章和签名检查', '优化通过 / 驳回 / 转人工边界'],
)
const resultLabel = computed(() => (isRules.value ? '生成的规则 JSON' : '优化后的审核要求'))
const applyLabel = computed(() => (isRules.value ? '应用到规则' : '应用到审核要求'))
const resultMeta = computed(() => {
  if (!result.value) return ''
  if (isRules.value) {
    try {
      const parsed = JSON.parse(result.value.text)
      return `${Array.isArray(parsed) ? parsed.length : 0} 组规则`
    } catch {
      return 'JSON'
    }
  }
  return `${result.value.text.length} 字`
})

watch(
  () => props.show,
  (value) => {
    requestSeq += 1
    busy.value = false
    if (!value) return
    instruction.value = ''
    error.value = ''
    result.value = null
    providerId.value = pickProvider(props.initialProviderId)
    nextTick(() => instructionRef.value?.focus())
  },
  { immediate: true },
)

watch(
  () => props.providerOptions,
  (options) => {
    if (!providerId.value || !options.some((item) => item.value === providerId.value)) {
      providerId.value = pickProvider(props.initialProviderId)
    }
  },
  { deep: true },
)

onBeforeUnmount(() => {
  requestSeq += 1
})

function pickProvider(preferred?: number | null): number | null {
  if (preferred && props.providerOptions.some((item) => item.value === preferred)) {
    return preferred
  }
  return props.providerOptions[0]?.value ?? null
}

function close() {
  requestSeq += 1
  emit('update:show', false)
}

function handleKeydown(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
    event.preventDefault()
    void generate()
  }
}

function applyVoiceInput(text: string) {
  instruction.value = instruction.value ? `${instruction.value} ${text}` : text
  nextTick(() => instructionRef.value?.focus())
}

async function generate() {
  const text = instruction.value.trim()
  if (!text || busy.value) return
  if (!props.projectId) {
    error.value = '请先选择项目'
    return
  }
  if (!providerId.value) {
    error.value = '当前项目没有可用的通用大模型'
    return
  }
  const seq = ++requestSeq
  const current = result.value?.text || props.current
  busy.value = true
  error.value = ''
  result.value = null
  try {
    const data = await auditApi.assistantField({
      project_id: props.projectId,
      provider_id: providerId.value,
      field: props.field,
      instruction: text,
      current,
      context: props.context,
    })
    if (seq !== requestSeq) return
    result.value = data
  } catch (requestError) {
    if (seq !== requestSeq) return
    error.value = requestError instanceof Error ? requestError.message : 'AI 生成失败，请重试'
  } finally {
    if (seq === requestSeq) busy.value = false
  }
}

function apply() {
  if (!result.value) return
  emit('apply', result.value.text)
  close()
}
</script>

<template>
  <n-modal :show="show" :mask-closable="!busy" @update:show="(value: boolean) => { if (!value) close() }">
    <div class="field-ai-modal">
      <div class="field-ai-head">
        <span class="field-ai-icon">
          <n-icon :component="SparklesOutline" />
        </span>
        <div class="field-ai-heading">
          <span class="field-ai-kicker">{{ kicker }}</span>
          <h3>{{ title }}</h3>
          <p>{{ description }}</p>
        </div>
        <n-button quaternary circle title="关闭" aria-label="关闭" @click="close">
          <template #icon><n-icon :component="CloseOutline" /></template>
        </n-button>
      </div>

      <div class="field-ai-body">
        <div class="field-ai-section">
          <div class="field-ai-label">
            <span>告诉 AI 你的要求</span>
            <small>会读取当前内容，不会自动保存</small>
          </div>
          <n-input
            ref="instructionRef"
            v-model:value="instruction"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 6 }"
            :disabled="busy"
            :placeholder="placeholder"
            :input-props="{ onKeydown: handleKeydown }"
          />
          <div class="field-ai-examples">
            <button
              v-for="example in examples"
              :key="example"
              type="button"
              :disabled="busy"
              @click="instruction = example"
            >
              {{ example }}
            </button>
            <VoiceInputButton :disabled="busy" @transcript="applyVoiceInput" />
          </div>
          <div class="field-ai-model">
            <span>生成模型</span>
            <n-select
              v-model:value="providerId"
              size="small"
              :options="providerOptions"
              :disabled="busy || !providerOptions.length"
              placement="top-start"
              placeholder="选择通用大模型"
            />
          </div>
        </div>

        <div class="field-ai-output">
          <div v-if="result" class="field-ai-result">
            <div class="field-ai-result-head">
              <span>{{ resultLabel }}</span>
              <span class="field-ai-result-meta">{{ resultMeta }}</span>
            </div>
            <pre v-if="isRules" class="field-ai-code">{{ result.text }}</pre>
            <div v-else class="field-ai-prompt">{{ result.text }}</div>
            <p v-if="result.explanation" class="field-ai-explanation">{{ result.explanation }}</p>
          </div>
          <div v-else class="field-ai-empty">
            <n-spin v-if="busy" :size="18" />
            <n-icon v-else :component="SparklesOutline" />
            <span>{{ busy ? 'AI 正在生成，请稍候…' : '输入需求后点击「生成」，结果会先展示在这里，确认后再应用。' }}</span>
          </div>
          <div v-if="error" class="field-ai-error">{{ error }}</div>
        </div>
      </div>

      <div class="field-ai-foot">
        <span>应用后仍需保存审核配置才会生效。</span>
        <div class="field-ai-actions">
          <n-button @click="close">取消</n-button>
          <n-button v-if="result" secondary :disabled="busy" @click="generate">重新生成</n-button>
          <n-button v-if="result" type="primary" :disabled="busy" @click="apply">{{ applyLabel }}</n-button>
          <n-button v-else type="primary" :loading="busy" :disabled="!instruction.trim() || !providerId" @click="generate">生成</n-button>
        </div>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.field-ai-modal {
  display: flex;
  width: min(700px, calc(100vw - 32px));
  max-height: calc(100vh - 40px);
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--card);
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.field-ai-head {
  display: flex;
  flex-shrink: 0;
  align-items: flex-start;
  gap: 12px;
  padding: 20px 22px 16px;
  border-bottom: 1px solid var(--border);
  background: linear-gradient(130deg, #f8fbff, var(--card));
}

.field-ai-icon {
  display: flex;
  width: 38px;
  height: 38px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: 1px solid #c7d9fb;
  border-radius: 11px;
  background: var(--brand-soft);
  color: var(--brand);
}

.field-ai-icon :deep(svg) {
  width: 19px;
  height: 19px;
}

.field-ai-heading {
  min-width: 0;
  flex: 1;
}

.field-ai-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.field-ai-heading h3 {
  margin: 4px 0 3px;
  color: var(--ink);
  font-size: 18px;
  font-weight: 650;
  letter-spacing: -0.01em;
}

.field-ai-heading p {
  margin: 0;
  color: var(--muted);
  font-size: 11.5px;
  line-height: 1.6;
}

.field-ai-body {
  min-height: 0;
  overflow-y: auto;
  padding: 16px 22px;
}

.field-ai-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.field-ai-label {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
}

.field-ai-label span {
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 600;
}

.field-ai-label small {
  color: var(--muted-light);
  font-size: 9.5px;
}

.field-ai-examples {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.field-ai-examples button {
  padding: 4px 8px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: #f8fafc;
  color: var(--muted);
  cursor: pointer;
  font-size: 9.5px;
  transition: border-color 0.16s ease, background 0.16s ease, color 0.16s ease;
}

.field-ai-examples button:hover:not(:disabled) {
  border-color: #c7d9fb;
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.field-ai-examples button:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.field-ai-model {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-top: 2px;
}

.field-ai-model > span {
  flex-shrink: 0;
  color: var(--muted);
  font-size: 10.5px;
}

.field-ai-model :deep(.n-select) {
  flex: 1;
  max-width: 360px;
}

.field-ai-output {
  margin-top: 14px;
}

.field-ai-result {
  overflow: hidden;
  border: 1px solid #dbe6f7;
  border-radius: 12px;
  background: #f8fbff;
}

.field-ai-result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 11px;
  border-bottom: 1px solid #e5edf9;
  color: var(--text);
  font-size: 10.5px;
  font-weight: 600;
}

.field-ai-result-meta {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 400;
}

.field-ai-code,
.field-ai-prompt {
  max-height: 230px;
  margin: 0;
  overflow: auto;
  padding: 12px;
  color: #334155;
  font-family: var(--mono);
  font-size: 10.5px;
  line-height: 1.7;
  white-space: pre-wrap;
}

.field-ai-prompt {
  font-family: inherit;
  font-size: 11.5px;
  line-height: 1.8;
}

.field-ai-explanation {
  margin: 0;
  padding: 0 12px 11px;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.65;
}

.field-ai-empty {
  display: flex;
  min-height: 118px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 18px;
  border: 1px dashed var(--border-strong);
  border-radius: 12px;
  background: #fbfcfe;
  color: var(--muted-light);
  font-size: 10.5px;
  text-align: center;
}

.field-ai-empty :deep(svg) {
  width: 17px;
  height: 17px;
  color: var(--brand);
}

.field-ai-error {
  margin-top: 8px;
  padding: 8px 10px;
  border: 1px solid #fecaca;
  border-radius: 8px;
  background: var(--red-soft);
  color: var(--red);
  font-size: 10.5px;
  line-height: 1.6;
}

.field-ai-foot {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 22px 16px;
  border-top: 1px solid var(--border);
  background: #fff;
}

.field-ai-foot > span {
  color: var(--muted-light);
  font-size: 9.5px;
}

.field-ai-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 8px;
}

@media (max-width: 640px) {
  .field-ai-modal {
    width: calc(100vw - 16px);
    max-height: calc(100vh - 16px);
  }

  .field-ai-head {
    padding: 16px 16px 13px;
  }

  .field-ai-heading h3 {
    font-size: 16px;
  }

  .field-ai-body {
    padding: 14px 16px;
  }

  .field-ai-foot {
    flex-wrap: wrap;
    padding: 10px 16px 14px;
  }

  .field-ai-foot > span {
    width: 100%;
  }

  .field-ai-actions {
    width: 100%;
    justify-content: flex-end;
  }

  .field-ai-code,
  .field-ai-prompt {
    max-height: 190px;
  }
}
</style>
