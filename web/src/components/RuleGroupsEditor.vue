<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { NButton, NDynamicTags, NInput, NInputNumber, NSelect, NSwitch, NTag } from 'naive-ui'
import { AddOutline, ArrowDownOutline, ArrowUpOutline, TrashOutline } from '@vicons/ionicons5'

type RuleResult = 'PASS' | 'REJECT' | 'REVIEW'

interface RuleItem {
  field: string
  operator: string
  value: unknown
}

interface RuleGroup {
  name: string
  priority: number
  logic: 'AND' | 'OR'
  stop_on_hit: boolean
  on_hit_result: RuleResult
  items: RuleItem[]
}

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'change'): void
}>()

const fieldOptions = [
  { label: '模型结论 · model.result', value: 'model.result' },
  { label: '风险分 · model.score', value: 'model.score' },
  { label: '风险等级 · model.risk_level', value: 'model.risk_level' },
  { label: '分类编码 · model.category', value: 'model.category' },
  { label: '分类名称 · model.category_name', value: 'model.category_name' },
  { label: '分类置信度 · model.category_confidence', value: 'model.category_confidence' },
  { label: '模型摘要 · model.summary', value: 'model.summary' },
  { label: '首个标签 · model.label', value: 'model.label' },
  { label: '全部标签 · model.labels', value: 'model.labels' },
  { label: '文本内容 · media.text', value: 'media.text' },
  { label: '文本长度 · media.text_len', value: 'media.text_len' },
  { label: '媒体数量 · media.count', value: 'media.count' },
  { label: '图片数量 · media.image_count', value: 'media.image_count' },
  { label: '媒体类型 · media.type', value: 'media.type' },
  { label: '来源类型 · task.source_type', value: 'task.source_type' },
  { label: '请求 ID · task.request_id', value: 'task.request_id' },
  { label: '业务主键 · task.biz_key', value: 'task.biz_key' },
]

const operatorOptions = [
  { label: '等于 eq', value: 'eq' },
  { label: '不等于 ne', value: 'ne' },
  { label: '大于 gt', value: 'gt' },
  { label: '大于等于 gte', value: 'gte' },
  { label: '小于 lt', value: 'lt' },
  { label: '小于等于 lte', value: 'lte' },
  { label: '区间 between', value: 'between' },
  { label: '属于 in', value: 'in' },
  { label: '不属于 not_in', value: 'not_in' },
  { label: '包含 contains', value: 'contains' },
  { label: '不包含 not_contains', value: 'not_contains' },
  { label: '包含任一 contains_any', value: 'contains_any' },
  { label: '包含全部 contains_all', value: 'contains_all' },
  { label: '前缀 starts_with', value: 'starts_with' },
  { label: '后缀 ends_with', value: 'ends_with' },
  { label: '正则 regex', value: 'regex' },
  { label: '为空 is_null', value: 'is_null' },
  { label: '非空 is_not_null', value: 'is_not_null' },
  { label: '内容为空 is_empty', value: 'is_empty' },
  { label: '内容非空 is_not_empty', value: 'is_not_empty' },
]

const logicOptions = [
  { label: '全部满足 · AND', value: 'AND' },
  { label: '任一满足 · OR', value: 'OR' },
]

const resultOptions = [
  { label: '通过 PASS', value: 'PASS' },
  { label: '不通过 REJECT', value: 'REJECT' },
  { label: '待议 REVIEW', value: 'REVIEW' },
]

const numericFields = new Set([
  'model.score',
  'model.category_confidence',
  'media.text_len',
  'media.count',
  'media.image_count',
])

const noValueOperators = new Set(['is_null', 'is_not_null', 'is_empty', 'is_not_empty'])
const listOperators = new Set(['in', 'not_in', 'contains_any', 'contains_all'])

const enumFields: Record<string, string[]> = {
  'model.result': ['PASS', 'REJECT', 'REVIEW'],
  'model.risk_level': ['low', 'medium', 'high'],
  'media.type': ['text', 'image', 'mixed'],
  'task.source_type': ['OPENAPI', 'ADMIN', 'REDIS', 'WEBSOCKET'],
}

const draft = ref<RuleGroup[]>([])
const expandedGroup = ref<number | null>(null)
const advanced = ref(false)
const rawText = ref('')
const rawError = ref('')
let syncing = false

const totalConditions = computed(() =>
  draft.value.reduce((total, group) => total + group.items.length, 0),
)

function serialize(groups: RuleGroup[]) {
  return JSON.stringify(groups, null, 2)
}

function normalizeResult(value: unknown): RuleResult {
  const result = String(value || '').toUpperCase()
  return result === 'PASS' || result === 'REJECT' ? result : 'REVIEW'
}

function normalizeGroup(input: any, index: number): RuleGroup {
  const priority = Number(input?.priority)
  return {
    name: String(input?.name || `规则组 ${index + 1}`),
    priority: Number.isFinite(priority) ? Math.round(priority) : (index + 1) * 10,
    logic: String(input?.logic || '').toUpperCase() === 'OR' ? 'OR' : 'AND',
    stop_on_hit: Boolean(input?.stop_on_hit),
    on_hit_result: normalizeResult(input?.on_hit_result),
    items: Array.isArray(input?.items)
      ? input.items.map((item: any) => ({
          field: String(item?.field || ''),
          operator: String(item?.operator || 'eq').toLowerCase(),
          value: item?.value,
        }))
      : [],
  }
}

function normalizeJson(text: string): RuleGroup[] {
  const parsed = JSON.parse(text || '[]')
  if (!Array.isArray(parsed)) throw new Error('规则组必须是数组')
  return parsed.map(normalizeGroup)
}

function syncFromProp(value: string) {
  const parsedText = value?.trim() ? value : '[]'
  if (parsedText === serialize(draft.value)) return
  try {
    const groups = normalizeJson(parsedText)
    syncing = true
    draft.value = groups
    expandedGroup.value = null
    rawError.value = ''
    if (advanced.value) rawText.value = parsedText
    void nextTick(() => {
      syncing = false
    })
  } catch (error) {
    rawError.value = error instanceof Error ? `规则 JSON 无效：${error.message}` : '规则 JSON 无效'
    advanced.value = true
    rawText.value = parsedText
    syncing = false
  }
}

watch(() => props.modelValue, syncFromProp, { immediate: true })
watch(
  draft,
  () => {
    if (syncing) return
    const text = serialize(draft.value)
    if (text === props.modelValue) return
    emit('update:modelValue', text)
    emit('change')
  },
  { deep: true },
)

function toggleAdvanced(value: boolean) {
  if (value) {
    rawText.value = serialize(draft.value)
    rawError.value = ''
    advanced.value = true
    return
  }
  try {
    const groups = normalizeJson(rawText.value || '[]')
    syncing = true
    draft.value = groups
    expandedGroup.value = null
    rawError.value = ''
    advanced.value = false
    void nextTick(() => {
      syncing = false
    })
  } catch (error) {
    rawError.value = error instanceof Error ? `规则 JSON 无效：${error.message}` : '规则 JSON 无效'
  }
}

function applyRaw(text: string) {
  rawText.value = text
  try {
    const groups = normalizeJson(text)
    syncing = true
    draft.value = groups
    rawError.value = ''
    emit('update:modelValue', text.trim() ? text : '[]')
    emit('change')
    void nextTick(() => {
      syncing = false
    })
  } catch (error) {
    rawError.value = error instanceof Error ? `规则 JSON 无效：${error.message}` : '规则 JSON 无效'
    emit('update:modelValue', text)
    emit('change')
  }
}

function newItem(): RuleItem {
  return { field: 'model.result', operator: 'eq', value: 'PASS' }
}

function addGroup() {
  const highest = draft.value.reduce((max, group) => Math.max(max, Number(group.priority) || 0), 0)
  draft.value.push({
    name: `规则组 ${draft.value.length + 1}`,
    priority: highest + 10,
    logic: 'AND',
    stop_on_hit: true,
    on_hit_result: 'REVIEW',
    items: [newItem()],
  })
  expandedGroup.value = draft.value.length - 1
}

function toggleGroup(index: number) {
  expandedGroup.value = expandedGroup.value === index ? null : index
}

function removeGroup(index: number) {
  if (expandedGroup.value === index) {
    expandedGroup.value = null
  } else if (expandedGroup.value !== null && expandedGroup.value > index) {
    expandedGroup.value -= 1
  }
  draft.value.splice(index, 1)
}

function addItem(group: RuleGroup) {
  group.items.push(newItem())
}

function removeItem(group: RuleGroup, index: number) {
  group.items.splice(index, 1)
}

function moveItem(group: RuleGroup, index: number, offset: number) {
  const target = index + offset
  if (target < 0 || target >= group.items.length) return
  const [item] = group.items.splice(index, 1)
  group.items.splice(target, 0, item)
}

function isNumericField(field: string) {
  return numericFields.has(field)
}

function hasNoValue(operator: string) {
  return noValueOperators.has(operator)
}

function isListOperator(operator: string) {
  return listOperators.has(operator)
}

function enumOptions(field: string) {
  return (enumFields[field] || []).map((value) => ({ label: value, value }))
}

function isEnumEquality(item: RuleItem) {
  return (item.operator === 'eq' || item.operator === 'ne') && enumOptions(item.field).length > 0
}

function resetValue(item: RuleItem) {
  if (hasNoValue(item.operator)) {
    item.value = null
    return
  }
  if (item.operator === 'between') {
    item.value = [0, 1]
    return
  }
  if (isListOperator(item.operator)) {
    item.value = []
    return
  }
  if (isNumericField(item.field)) {
    item.value = 0
    return
  }
  const options = enumOptions(item.field)
  item.value = options.length ? options[0].value : ''
}

function setField(item: RuleItem, value: unknown) {
  item.field = value == null ? '' : String(value)
  resetValue(item)
}

function setOperator(item: RuleItem, value: unknown) {
  item.operator = value == null ? 'eq' : String(value)
  resetValue(item)
}

function numericValue(item: RuleItem): number | null {
  const value = Number(item.value)
  return Number.isFinite(value) ? value : null
}

function setNumeric(item: RuleItem, value: number | null) {
  item.value = value ?? 0
}

function enumValue(item: RuleItem) {
  return item.value == null ? '' : String(item.value)
}

function setEnum(item: RuleItem, value: unknown) {
  item.value = value == null ? '' : String(value)
}

function textValue(item: RuleItem) {
  if (typeof item.value === 'string') return item.value
  if (item.value == null) return ''
  if (typeof item.value === 'number' || typeof item.value === 'boolean') return String(item.value)
  return JSON.stringify(item.value)
}

function setText(item: RuleItem, value: string) {
  item.value = value
}

function listValue(item: RuleItem): string[] {
  if (Array.isArray(item.value)) return item.value.map((value) => String(value))
  if (item.value == null || item.value === '') return []
  return [String(item.value)]
}

function setList(item: RuleItem, values: string[]) {
  item.value = values
}

function rangeValue(item: RuleItem, index: number): number | null {
  if (!Array.isArray(item.value)) return null
  const value = Number(item.value[index])
  return Number.isFinite(value) ? value : null
}

function setRange(item: RuleItem, index: number, value: number | null) {
  const values = Array.isArray(item.value) ? [...item.value] : [0, 1]
  values[index] = value ?? 0
  item.value = values
}

function valuePlaceholder(item: RuleItem) {
  if (item.operator === 'regex') return '例如 ^INV-\\d+$'
  if (item.operator === 'contains' || item.operator === 'not_contains') return '输入要包含的子串'
  if (item.operator === 'starts_with' || item.operator === 'ends_with') return '输入匹配文本'
  return '比较值'
}

const fieldSummaryLabels: Record<string, string> = {
  'model.result': '模型结论',
  'model.score': '风险分',
  'model.risk_level': '风险等级',
  'model.category': '分类编码',
  'model.category_name': '分类名称',
  'model.category_confidence': '分类置信度',
  'model.summary': '模型摘要',
  'model.label': '首个标签',
  'model.labels': '全部标签',
  'media.text': '文本内容',
  'media.text_len': '文本长度',
  'media.count': '媒体数量',
  'media.image_count': '图片数量',
  'media.type': '媒体类型',
  'task.source_type': '来源类型',
  'task.request_id': '请求 ID',
  'task.biz_key': '业务主键',
}

const operatorSymbols: Record<string, string> = {
  eq: '=',
  ne: '≠',
  gt: '>',
  gte: '≥',
  lt: '<',
  lte: '≤',
}

const valueSummaryLabels: Record<string, string> = {
  PASS: '通过',
  REJECT: '不通过',
  REVIEW: '待议',
  high: '高',
  medium: '中',
  low: '低',
  text: '纯文本',
  image: '纯图片',
  mixed: '图文',
}

function summaryValue(value: unknown): string {
  if (Array.isArray(value)) return value.map(summaryValue).join('、')
  if (value == null) return '空'
  if (typeof value === 'object') return JSON.stringify(value)
  const text = String(value)
  return valueSummaryLabels[text] || text
}

function conditionSummary(item: RuleItem): string {
  if (!item.field) return '未选择字段'
  const field = fieldSummaryLabels[item.field] || item.field
  const value = summaryValue(item.value)
  switch (item.operator) {
    case 'eq':
    case 'ne':
    case 'gt':
    case 'gte':
    case 'lt':
    case 'lte':
      return `${field} ${operatorSymbols[item.operator] || item.operator} ${value}`
    case 'between': {
      const values = Array.isArray(item.value) ? item.value : []
      return `${field} 在 ${summaryValue(values[0])} ~ ${summaryValue(values[1])} 之间`
    }
    case 'in':
      return `${field} 属于 [${value}]`
    case 'not_in':
      return `${field} 不属于 [${value}]`
    case 'contains':
      return `${field} 包含 ${value}`
    case 'not_contains':
      return `${field} 不包含 ${value}`
    case 'contains_any':
      return `${field} 包含任一 [${value}]`
    case 'contains_all':
      return `${field} 包含全部 [${value}]`
    case 'starts_with':
      return `${field} 以 ${value} 开头`
    case 'ends_with':
      return `${field} 以 ${value} 结尾`
    case 'regex':
      return `${field} 匹配 ${value}`
    case 'is_null':
      return `${field} 为 null`
    case 'is_not_null':
      return `${field} 不为 null`
    case 'is_empty':
      return `${field} 为空`
    case 'is_not_empty':
      return `${field} 不为空`
    default:
      return `${field} ${item.operator} ${value}`
  }
}

function groupSummary(group: RuleGroup) {
  if (!group.items.length) return '暂无条件，此规则组不会命中'
  return group.items.map(conditionSummary).join(group.logic === 'OR' ? ' 或 ' : ' 且 ')
}

function resultTagType(result: string) {
  if (result === 'PASS') return 'success'
  if (result === 'REJECT') return 'error'
  return 'warning'
}
</script>

<template>
  <div class="rule-groups-editor">
    <div class="rules-toolbar">
      <span class="rules-note">
        规则组按优先级执行。点击「编辑」展开条件，未展开时只显示摘要。
      </span>
      <div class="rules-toolbar-right">
        <span class="rule-count">{{ draft.length }} 组 · {{ totalConditions }} 条件</span>
        <span class="advanced-label">高级 JSON</span>
        <n-switch :value="advanced" size="small" @update:value="toggleAdvanced" />
      </div>
    </div>

    <n-input
      v-if="advanced"
      :value="rawText"
      type="textarea"
      :autosize="{ minRows: 10, maxRows: 24 }"
      class="rule-json-input code-input"
      spellcheck="false"
      @update:value="applyRaw"
    />
    <p v-if="advanced && rawError" class="rules-error">{{ rawError }}</p>

    <template v-else>
      <div v-if="!draft.length" class="rules-empty">
        <strong>还没有规则组</strong>
        <p>规则组会按优先级依次判断，未命中时使用页面下方的默认结论。</p>
        <n-button size="small" dashed @click="addGroup">
          <template #icon><AddOutline /></template>
          添加规则组
        </n-button>
      </div>

      <article v-for="(group, groupIndex) in draft" :key="groupIndex" class="rule-card" :class="{ expanded: expandedGroup === groupIndex }">
        <div class="rule-summary">
          <span class="rule-index">{{ groupIndex + 1 }}</span>
          <div class="rule-summary-main">
            <div class="rule-summary-title">
              <strong>{{ group.name || '未命名规则组' }}</strong>
              <n-tag size="small" :bordered="false" :type="resultTagType(group.on_hit_result)">
                {{ group.on_hit_result }}
              </n-tag>
              <span class="rule-summary-meta">
                优先级 {{ group.priority }} · {{ group.logic === 'OR' ? '任一满足' : '全部满足' }} · {{ group.stop_on_hit ? '命中后停止' : '命中后继续' }}
              </span>
            </div>
            <p class="rule-summary-text" :title="groupSummary(group)">{{ groupSummary(group) }}</p>
          </div>
          <div class="rule-summary-actions">
            <n-button size="tiny" secondary @click="toggleGroup(groupIndex)">
              {{ expandedGroup === groupIndex ? '收起' : '编辑' }}
            </n-button>
            <n-button quaternary size="tiny" type="error" title="删除规则组" @click="removeGroup(groupIndex)">
              <template #icon><TrashOutline /></template>
            </n-button>
          </div>
        </div>

        <div v-if="expandedGroup === groupIndex" class="rule-card-body">
          <div class="rule-name-setting">
            <label>规则组名称</label>
            <n-input v-model:value="group.name" size="small" placeholder="规则组名称" />
          </div>

          <div class="rule-settings">
            <div class="rule-setting">
              <label>优先级</label>
              <n-input-number
                :value="group.priority"
                size="small"
                :min="0"
                :max="9999"
                style="width: 100%"
                @update:value="(value) => (group.priority = value ?? 0)"
              />
            </div>
            <div class="rule-setting">
              <label>条件关系</label>
              <n-select v-model:value="group.logic" :options="logicOptions" size="small" />
            </div>
            <div class="rule-setting">
              <label>命中结论</label>
              <n-select v-model:value="group.on_hit_result" :options="resultOptions" size="small" />
            </div>
            <div class="rule-setting rule-stop">
              <div>
                <strong>命中后停止</strong>
                <span>不再执行后续规则</span>
              </div>
              <n-switch v-model:value="group.stop_on_hit" size="small" />
            </div>
          </div>

          <div class="conditions">
            <div class="conditions-head">
              <span>条件 <em>{{ group.items.length }}</em></span>
              <n-button text type="primary" size="tiny" @click="addItem(group)">
                <template #icon><AddOutline /></template>
                添加条件
              </n-button>
            </div>

            <div v-if="!group.items.length" class="condition-empty">
              没有条件，此规则组不会命中。
            </div>
            <div v-else class="condition-table">
              <div class="condition-labels">
                <span>字段</span>
                <span>操作符</span>
                <span>比较值</span>
                <span></span>
              </div>
              <div v-for="(item, itemIndex) in group.items" :key="itemIndex" class="condition-row">
                <n-select
                  :value="item.field"
                  :options="fieldOptions"
                  size="small"
                  filterable
                  tag
                  placeholder="选择或输入字段"
                  @update:value="(value) => setField(item, value)"
                />
                <n-select
                  :value="item.operator"
                  :options="operatorOptions"
                  size="small"
                  @update:value="(value) => setOperator(item, value)"
                />
                <div class="condition-value">
                  <span v-if="hasNoValue(item.operator)" class="no-value">无需比较值</span>
                  <div v-else-if="item.operator === 'between'" class="range-value">
                    <n-input-number
                      :value="rangeValue(item, 0)"
                      size="small"
                      :show-button="false"
                      placeholder="下限"
                      @update:value="(value) => setRange(item, 0, value)"
                    />
                    <span>~</span>
                    <n-input-number
                      :value="rangeValue(item, 1)"
                      size="small"
                      :show-button="false"
                      placeholder="上限"
                      @update:value="(value) => setRange(item, 1, value)"
                    />
                  </div>
                  <n-dynamic-tags
                    v-else-if="isListOperator(item.operator)"
                    :value="listValue(item)"
                    size="small"
                    @update:value="(values: string[]) => setList(item, values)"
                  />
                  <n-select
                    v-else-if="isEnumEquality(item)"
                    :value="enumValue(item)"
                    :options="enumOptions(item.field)"
                    size="small"
                    filterable
                    tag
                    @update:value="(value) => setEnum(item, value)"
                  />
                  <n-input-number
                    v-else-if="isNumericField(item.field)"
                    :value="numericValue(item)"
                    size="small"
                    :show-button="false"
                    style="width: 100%"
                    @update:value="(value) => setNumeric(item, value)"
                  />
                  <n-input
                    v-else
                    :value="textValue(item)"
                    size="small"
                    :placeholder="valuePlaceholder(item)"
                    @update:value="(value) => setText(item, value)"
                  />
                </div>
                <div class="condition-ops">
                  <n-button quaternary size="tiny" :disabled="itemIndex === 0" title="上移条件" @click="moveItem(group, itemIndex, -1)">
                    <template #icon><ArrowUpOutline /></template>
                  </n-button>
                  <n-button quaternary size="tiny" :disabled="itemIndex === group.items.length - 1" title="下移条件" @click="moveItem(group, itemIndex, 1)">
                    <template #icon><ArrowDownOutline /></template>
                  </n-button>
                  <n-button quaternary size="tiny" type="error" title="删除条件" @click="removeItem(group, itemIndex)">
                    <template #icon><TrashOutline /></template>
                  </n-button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </article>

      <n-button v-if="draft.length" dashed block size="small" @click="addGroup">
        <template #icon><AddOutline /></template>
        添加规则组
      </n-button>
    </template>
  </div>
</template>

<style scoped>
.rule-groups-editor {
  padding: 10px 11px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: #fbfcfe;
}

.rules-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 9px;
}

.rules-note {
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.5;
}

.rules-toolbar-right {
  display: flex;
  align-items: center;
  gap: 7px;
  flex-shrink: 0;
}

.rule-count {
  padding: 2px 6px;
  border-radius: 99px;
  background: #eef2f8;
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9px;
  white-space: nowrap;
}

.advanced-label {
  color: var(--muted);
  font-size: 10px;
}

.rule-json-input :deep(textarea) {
  font-family: var(--mono);
  font-size: 10.5px;
  line-height: 1.55;
}

.rules-error {
  margin: 6px 0 0;
  color: #d03050;
  font-size: 10.5px;
}

.rule-card {
  margin-bottom: 8px;
  padding: 9px 10px;
  border: 1px solid var(--border);
  border-radius: 9px;
  background: #fff;
}

.rule-card.expanded {
  border-color: var(--brand-border);
}

.rule-summary {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.rule-summary-main {
  min-width: 0;
  flex: 1;
}

.rule-summary-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.rule-summary-title strong {
  color: var(--text);
  font-size: 11.5px;
  font-weight: 620;
}

.rule-summary-meta {
  color: var(--muted-light);
  font-size: 9px;
}

.rule-summary-text {
  display: -webkit-box;
  overflow: hidden;
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.55;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.rule-summary-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: none;
}

.rule-card-body {
  margin-top: 9px;
  padding-top: 9px;
  border-top: 1px dashed var(--border);
}

.rule-name-setting {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-bottom: 8px;
}

.rule-name-setting label {
  color: #6a7686;
  font-size: 9.5px;
}

.rule-index {
  display: inline-flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  flex: none;
}

.rule-settings {
  display: flex;
  flex-wrap: wrap;
  gap: 7px 9px;
  margin-bottom: 9px;
}

.rule-setting {
  display: flex;
  min-width: 105px;
  flex: 1 1 115px;
  flex-direction: column;
  gap: 3px;
}

.rule-setting > label {
  color: #6a7686;
  font-size: 9.5px;
}

.rule-stop {
  min-width: 175px;
  flex: 1.4 1 180px;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-top: 15px;
}

.rule-stop strong,
.rule-stop span {
  display: block;
}

.rule-stop strong {
  color: var(--text);
  font-size: 10px;
  font-weight: 550;
}

.rule-stop span {
  margin-top: 1px;
  color: var(--muted-light);
  font-size: 9px;
}

.conditions {
  padding-top: 8px;
  border-top: 1px dashed var(--border);
}

.conditions-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 5px;
}

.conditions-head > span {
  color: var(--muted);
  font-size: 10px;
}

.conditions-head em {
  color: var(--text);
  font-style: normal;
  font-weight: 600;
}

.condition-labels,
.condition-row {
  display: grid;
  grid-template-columns: minmax(120px, 1.2fr) minmax(100px, 0.9fr) minmax(135px, 1.45fr) auto;
  gap: 5px;
  align-items: center;
}

.condition-labels {
  margin-bottom: 3px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.condition-row {
  margin-bottom: 5px;
}

.condition-value {
  min-width: 0;
}

.condition-value > :deep(.n-input),
.condition-value > :deep(.n-input-number),
.condition-value > :deep(.n-select) {
  width: 100%;
}

.condition-value :deep(.n-dynamic-tags) {
  min-height: 30px;
}

.range-value {
  display: flex;
  align-items: center;
  gap: 4px;
}

.range-value :deep(.n-input-number) {
  min-width: 0;
  flex: 1;
}

.range-value > span {
  color: var(--muted-light);
  font-size: 10px;
}

.no-value {
  display: inline-flex;
  min-height: 30px;
  align-items: center;
  color: var(--muted-light);
  font-size: 9.5px;
}

.condition-ops {
  display: flex;
  gap: 0;
  justify-content: flex-end;
}

.condition-empty,
.rules-empty {
  padding: 16px 12px;
  border: 1px dashed var(--border);
  border-radius: 8px;
  color: var(--muted);
  font-size: 10.5px;
  text-align: center;
}

.rules-empty strong {
  display: block;
  color: var(--text);
  font-size: 11px;
}

.rules-empty p {
  margin: 4px 0 9px;
  font-size: 10px;
}

@media (max-width: 720px) {
  .rules-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }

  .rules-toolbar-right {
    width: 100%;
    justify-content: flex-end;
  }

  .condition-labels {
    display: none;
  }

  .condition-row {
    grid-template-columns: 1fr auto;
  }

  .condition-row > :deep(.n-select) {
    grid-column: 1 / -1;
  }

  .condition-value {
    grid-column: 1;
  }

  .condition-ops {
    grid-column: 2;
  }

  .rule-stop {
    padding-top: 0;
  }
}
</style>
