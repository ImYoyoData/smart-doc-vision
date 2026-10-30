<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import dayjs from 'dayjs'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import {
  NButton,
  NDatePicker,
  NDrawer,
  NDrawerContent,
  NImage,
  NIcon,
  NSpin,
  NDropdown,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSwitch,
  useDialog,
  useMessage,
  type InputInst,
} from 'naive-ui'
import {
  AddOutline,
  CheckmarkCircleOutline,
  CloudOutline,
  CloseCircleOutline,
  CloseOutline,
  CopyOutline,
  CreateOutline,
  DownloadOutline,
  EllipsisHorizontalOutline,
  FlashOutline,
  PersonOutline,
  RefreshOutline,
  SearchOutline,
  SendOutline,
  ShieldCheckmarkOutline,
  SparklesOutline,
  TextOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import TaskSubmitModal from '@/components/TaskSubmitModal.vue'
import AuditIntegrationDrawer from '@/components/AuditIntegrationDrawer.vue'
import AuditFieldAssistantModal from '@/components/AuditFieldAssistantModal.vue'
import ResultRoutesEditor from '@/components/ResultRoutesEditor.vue'
import RuleGroupsEditor from '@/components/RuleGroupsEditor.vue'
import TriggerManageModal from '@/components/TriggerManageModal.vue'
import ScriptLogsModal from '@/components/ScriptLogsModal.vue'
import VoiceInputButton from '@/components/VoiceInputButton.vue'
import { analyticsApi, auditApi, metaApi, providerApi } from '@/api'
import { copyText } from '@/utils/clipboard'
import type { Audit, AuditAssistantConfig, AuditAssistantMessage, AuditAssistantResult, AuditAssistantStep, AuditCategory, Provider, RuleTemplate, TriggerSource } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'

const workspace = useWorkspaceStore()
const auth = useAuthStore()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const audits = ref<Audit[]>([])
const providers = ref<Provider[]>([])
const auditPage = ref(1)
const auditTotal = ref(0)
const auditPageSize = 50
const loadingMore = ref(false)
const statusUpdatingId = ref<number | null>(null)
const hasMoreAudits = computed(() => audits.value.length < auditTotal.value)
const templates = ref<RuleTemplate[]>([])
const search = ref('')
const statusFilter = ref<string | null>('ACTIVE')
const modalShow = ref(false)
const promptModalShow = ref(false)
const categoryModalShow = ref(false)
const rulesModalShow = ref(false)
const routeModalShow = ref(false)
const policyModalShow = ref(false)
const editing = ref<Audit | null>(null)
const errorText = ref('')
const cloneShow = ref(false)
const cloneTarget = ref<Audit | null>(null)
const cloneForm = reactive({ name: '', code: '' })
const taskModalShow = ref(false)
const taskAuditId = ref<number | null>(null)
const guideShow = ref(false)
const guideAuditId = ref<number | null>(null)
const detailShow = ref(false)
const detailAudit = ref<Audit | null>(null)
const detailTriggers = ref<TriggerSource[]>([])
const triggerModalShow = ref(false)
const activeTrigger = ref<TriggerSource | null>(null)
const decisionFilter = ref<string | null>(null)
const exportShow = ref(false)
const exportTarget = ref<Audit | null>(null)
const exporting = ref(false)
const assistantShow = ref(false)
const assistantMode = ref<'create' | 'edit'>('create')
const assistantMessages = ref<AuditAssistantMessage[]>([])
const assistantInput = ref('')
const assistantBusy = ref(false)
const assistantError = ref('')
const assistantProviderId = ref<number | null>(null)
const assistantReadyConfig = ref<AuditAssistantConfig | null>(null)
const assistantChanges = ref<string[]>([])
const assistantSaved = ref(false)
const assistantPickShow = ref(false)
const assistantPickId = ref<number | null>(null)
const scriptLogsShow = ref(false)
const scriptLogsAudit = ref<Audit | null>(null)
const assistantChat = ref<HTMLElement | null>(null)
const assistantInputRef = ref<InputInst | null>(null)
const assistantImages = ref<AssistantImage[]>([])
const fieldAssistantShow = ref(false)
const fieldAssistantField = ref<'rules' | 'review_prompt'>('rules')
const fieldAssistantCurrent = ref('')
const fieldAssistantProviderId = ref<number | null>(null)
const fieldAssistantContext = ref<Record<string, unknown>>({})
let assistantAbort: AbortController | null = null

interface AssistantImage {
  id: string
  name: string
  dataUrl: string
}

function currentAssistantMessage(): AuditAssistantMessage | null {
  for (let index = assistantMessages.value.length - 1; index >= 0; index -= 1) {
    if (assistantMessages.value[index].role === 'assistant') {
      return assistantMessages.value[index]
    }
  }
  return null
}

function upsertAssistantStep(step: AuditAssistantStep) {
  const target = currentAssistantMessage()
  if (!target) return
  target.steps ||= []
  const index = target.steps.findIndex((item) => item.id === step.id)
  if (index >= 0) target.steps[index] = step
  else target.steps.push(step)
}

function dedupeAssistantSteps(steps: AuditAssistantStep[]) {
  const result: AuditAssistantStep[] = []
  for (const step of steps) {
    const index = result.findIndex((item) => item.id === step.id)
    if (index >= 0) result[index] = step
    else result.push(step)
  }
  return result
}

function assistantStatusText(message: AuditAssistantMessage) {
  const running = message.steps?.find((step) => step.status === 'running')
  switch (running?.id) {
    case 'context':
      return 'AI 正在读取当前配置…'
    case 'provider':
      return 'AI 正在准备模型…'
    case 'thinking':
      return 'AI 正在思考…'
    case 'draft':
      return 'AI 正在整理配置…'
    case 'save':
      return assistantMode.value === 'edit' ? '正在保存修改…' : '正在添加审核配置…'
    case 'access':
      return '正在应用接入方式…'
    default:
      return assistantMode.value === 'edit' ? 'AI 正在编辑审核配置…' : 'AI 正在思考…'
  }
}

function renderAssistantMarkdown(content: string) {
  const html = marked.parse(content || '', { breaks: true, gfm: true })
  return DOMPurify.sanitize(typeof html === 'string' ? html : '')
}

function readAssistantImage(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error || new Error('读取图片失败'))
    reader.readAsDataURL(file)
  })
}

async function handleAssistantPaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.items || [])
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))
  if (!files.length) return
  event.preventDefault()
  if (assistantImages.value.length + files.length > 4) {
    message.warning('一次最多粘贴 4 张图片')
    return
  }
  for (const file of files) {
    if (file.size > 6 * 1024 * 1024) {
      message.warning(`${file.name || '图片'} 超过 6MB，已跳过`)
      continue
    }
    try {
      const dataUrl = await readAssistantImage(file)
      assistantImages.value.push({
        id: `${Date.now()}-${assistantImages.value.length}-${file.name || 'pasted'}`,
        name: file.name || '粘贴图片',
        dataUrl,
      })
    } catch {
      message.warning('图片读取失败，请重试')
    }
  }
}

function removeAssistantImage(id: string) {
  assistantImages.value = assistantImages.value.filter((item) => item.id !== id)
}

onBeforeUnmount(() => {
  assistantAbort?.abort()
})
const hasGeneralProvider = computed(() =>
  providers.value.some((p) => p.status === 'ACTIVE' && p.kind !== 'JEV'),
)
const assistantProviderOptions = computed(() =>
  providers.value
    .filter((p) => p.status === 'ACTIVE' && p.kind !== 'JEV')
    .map((p) => ({
      label: `${p.name} · ${p.model}${providerCooling(p) ? '（熔断中）' : ''}`,
      value: p.id,
    })),
)
const assistantStarters = computed(() =>
  assistantMode.value === 'edit'
    ? ['把规则改得更严格', '增加敏感信息检查', '把接入方式改成 Redis', '优化审核要求提示词']
    : ['创建一条档案完整性合规审核', '按档案类型分类并低置信度转人工', '需要 Redis Stream 接入', '按合规审核创建并启用开放 API'],
)
const showAssistantStarters = computed(
  () => !assistantBusy.value && !assistantMessages.value.some((item) => item.role === 'user'),
)

function useAssistantStarter(text: string) {
  assistantInput.value = text
  nextTick(() => assistantInputRef.value?.focus())
}
const isClassifyMode = computed(() =>
  form.decisionMode === 'CLASSIFY' || form.decisionMode === 'CLASSIFY_ONLY',
)
const categoryExportOptions = [
  { label: '导出 CSV', key: 'csv' },
  { label: '导出 JSON', key: 'json' },
]

const defaultPrompt =
  '你是“智审图档”平台的档案图档审核专家。请依据档案管理规范、内容安全要求与审核要点，判断文档是否清晰完整、是否存在敏感或违规内容，以及印章、签名、日期等关键要素是否齐全。无法确认时返回 REVIEW。'

const defaultClassifyPrompt = ref(
  '你是“智审图档”平台的档案分类专家。请根据文本、OCR 内容和图片版面，从配置的分类列表中选择唯一最匹配的类别，并给出 0~1 的置信度和判断依据。无法可靠判断时选择其他类别，不要臆测。',
)
const defaultClassifierPrompt = ref(
  '你是“智审图档”平台的档案类型分类器。请根据文本、OCR 内容和图片版面，从配置的类型列表中选择唯一最匹配的类型编码，并给出 0~1 的置信度和判断依据。分类器不需要判定通过或驳回，平台会把成功分类的结论固定为 PASS；无法可靠判断时选择最接近的类型，不要臆测。',
)
const defaultRoutesText = JSON.stringify({
  PASS: { exec_mode: 'sequential', on_error: 'continue', actions: [] },
  REJECT: { exec_mode: 'sequential', on_error: 'continue', actions: [] },
  REVIEW: { exec_mode: 'sequential', on_error: 'to_review', actions: [] },
}, null, 2)

const classifierUnknownCategory: AuditCategory = {
  code: 'unknown',
  name: '未知',
  description: '无法判断，或不属于当前已配置的任何类型',
  result: 'PASS',
}

function ensureClassifierUnknownCategory() {
  if (form.decisionMode !== 'CLASSIFY_ONLY') return
  if (form.categories.some((category) => category.code.trim().toLowerCase() === 'unknown')) return
  form.categories.push({ ...classifierUnknownCategory })
}

const fallbackCategories: AuditCategory[] = [
  { code: 'contract', name: '合同协议', description: '合同、协议、补充条款、盖章的签约文件', result: 'PASS' },
  { code: 'invoice', name: '发票票据', description: '发票、收据、付款凭证、报销单据', result: 'PASS' },
  { code: 'engineering_drawing', name: '工程图纸', description: '施工图、竣工图、设计图纸、技术图样', result: 'PASS' },
  { code: 'id_document', name: '身份证件', description: '身份证、护照、驾驶证等个人证件影像', result: 'REJECT' },
  { code: 'other', name: '其他 / 无法识别', description: '不属于以上类别，或信息不足以可靠分类', result: 'REVIEW' },
]
const exampleCategories = ref<AuditCategory[]>(fallbackCategories)

const form = reactive({
  name: '',
  code: '',
  description: '',
  mediaTypes: ['text', 'image'] as string[],
  decisionMode: 'REVIEW' as 'REVIEW' | 'CLASSIFY' | 'CLASSIFY_ONLY',
  categories: [] as AuditCategory[],
  providerId: null as number | null,
  reviewPrompt: defaultPrompt,
  rulesPreset: 'standard',
  rulesText: '',
  routesText: defaultRoutesText,
  defaultResult: 'REVIEW',
  onModelFail: 'REVIEW',
  reviewMode: 'INTERNAL',
  dedupeWindowSec: 86400,
})

const exportForm = reactive({
  range: null as [number, number] | null,
  statuses: [] as string[],
  results: [] as string[],
  sources: [] as string[],
  mediaTypes: [] as string[],
  keyword: '',
})

// 编辑/新建配置弹窗的离开守卫：关闭、路由跳转、刷新页面前统一确认未保存修改与 AI 请求。
const formBaseline = ref('')
const leaveConfirmShow = ref(false)
const leaveTarget = ref('')
const leaveTitle = ref('')
const leaveMessage = ref('')
const leaveConfirmText = ref('放弃修改并离开')
let leaveAction: (() => void) | null = null
let leaveRouteResolve: ((value: boolean) => void) | null = null

function snapshotFormBaseline() {
  formBaseline.value = JSON.stringify(form)
}

const formDirty = computed(() => {
  if (!modalShow.value || !formBaseline.value) return false
  return JSON.stringify(form) !== formBaseline.value
})

const assistantBusyNow = computed(() => assistantShow.value && assistantBusy.value)

function openLeaveConfirm(title: string, body: string, confirmText: string, options: { action?: () => void; resolve?: (value: boolean) => void }) {
  leaveTitle.value = title
  leaveMessage.value = body
  leaveConfirmText.value = confirmText
  leaveAction = options.action || null
  leaveRouteResolve = options.resolve || null
  leaveConfirmShow.value = true
}

function cancelLeaveConfirm() {
  leaveConfirmShow.value = false
  const resolve = leaveRouteResolve
  leaveAction = null
  leaveRouteResolve = null
  resolve?.(false)
}

function acceptLeaveConfirm() {
  leaveConfirmShow.value = false
  const resolve = leaveRouteResolve
  const action = leaveAction
  leaveAction = null
  leaveRouteResolve = null
  if (assistantBusy.value) assistantAbort?.abort()
  resolve?.(true)
  action?.()
}

function requestCloseModal() {
  if (assistantBusyNow.value) {
    openLeaveConfirm(
      'AI 正在执行，确认离开？',
      '当前 AI 配置助手仍在生成内容，关闭会停止请求，尚未保存的结果不会保留。',
      '停止 AI 并关闭',
      { action: () => { modalShow.value = false } },
    )
    return
  }
  if (!formDirty.value) {
    modalShow.value = false
    return
  }
  openLeaveConfirm(
    '配置尚未保存，确认离开？',
    '当前审核配置有未保存的修改，离开后会丢失这些修改。',
    '放弃修改并离开',
    { action: () => { modalShow.value = false } },
  )
}

function handleModalShowChange(value: boolean) {
  if (value) {
    modalShow.value = true
    return
  }
  requestCloseModal()
}

onBeforeRouteLeave(() => {
  const dirty = formDirty.value
  const busy = assistantBusyNow.value
  if (!dirty && !busy) return true
  const body = dirty && busy
    ? '离开会停止当前 AI 请求，并丢失审核配置的未保存修改。'
    : busy
      ? '当前 AI 配置助手仍在生成内容，离开会停止请求，尚未保存的结果不会保留。'
      : '当前审核配置有未保存的修改，离开后会丢失这些修改。'
  leaveTarget.value = '离开当前页面'
  return new Promise<boolean>((resolve) => {
    openLeaveConfirm(busy ? 'AI 正在执行，确认离开？' : '配置尚未保存，确认离开？', body, busy ? '停止 AI 并离开' : '放弃修改并离开', { resolve })
  })
})

function handleBeforeUnload(event: BeforeUnloadEvent) {
  if (!formDirty.value && !assistantBusyNow.value) return
  event.preventDefault()
  event.returnValue = ''
}

onMounted(() => window.addEventListener('beforeunload', handleBeforeUnload))
onBeforeUnmount(() => window.removeEventListener('beforeunload', handleBeforeUnload))

const statusFilterOptions = [
  { label: '已启用', value: 'ACTIVE' },
  { label: '已暂停', value: 'PAUSED' },
  { label: '草稿', value: 'DRAFT' },
]
const decisionFilterOptions = [
  { label: '合规审核', value: 'REVIEW' },
  { label: '分类审核', value: 'CLASSIFY' },
  { label: '纯分类器', value: 'CLASSIFY_ONLY' },
]
const decisionFormOptions = [
  { label: '合规审核 · PASS / REJECT / REVIEW', value: 'REVIEW' },
  { label: '分类审核 · 按分类映射终审结论', value: 'CLASSIFY' },
  { label: '纯分类器 · 全部通过并返回类型编码', value: 'CLASSIFY_ONLY' },
]
const resultOptions = [
  { label: '通过 PASS', value: 'PASS' },
  { label: '不通过 REJECT', value: 'REJECT' },
  { label: '待议 REVIEW', value: 'REVIEW' },
  { label: '未知 UNKNOWN', value: 'UNKNOWN' },
]
const categoryResultOptions = [
  { label: '通过 PASS', value: 'PASS' },
  { label: '驳回 REJECT', value: 'REJECT' },
  { label: '转人工 REVIEW', value: 'REVIEW' },
]
const taskStatusOptions = [
  { label: '排队中 QUEUED', value: 'QUEUED' },
  { label: '处理中 PROCESSING', value: 'PROCESSING' },
  { label: '待复核 REVIEW_PENDING', value: 'REVIEW_PENDING' },
  { label: '成功 SUCCESS', value: 'SUCCESS' },
  { label: '失败 FAILED', value: 'FAILED' },
  { label: '已停止 STOPPED', value: 'STOPPED' },
]
const sourceOptions = [
  { label: '后台 ADMIN', value: 'ADMIN' },
  { label: '开放 API', value: 'OPENAPI' },
  { label: 'Redis Stream', value: 'REDIS' },
  { label: 'WebSocket', value: 'WEBSOCKET' },
]
const mediaFilterOptions = [
  { label: '纯文本', value: 'text' },
  { label: '纯图片', value: 'image' },
  { label: '图文混合', value: 'mixed' },
]

const filtered = computed(() => {
  const k = search.value.trim().toLowerCase()
  return audits.value.filter((a) => {
    if (statusFilter.value && a.status !== statusFilter.value) return false
    if (decisionFilter.value && (a.decision_mode || 'REVIEW') !== decisionFilter.value) return false
    if (!k) return true
    return a.name.toLowerCase().includes(k) || a.code.toLowerCase().includes(k)
  })
})

function providerKindLabel(kind?: string) {
  if (kind === 'JEV') return 'JEV 决策模型'
  if (kind === 'ANTHROPIC') return '通用大模型 · Anthropic 协议'
  return '通用大模型 · OpenAI 协议'
}

function providerCooling(provider: Provider) {
  return Boolean(provider.breaker_until)
}

const providerOptions = computed(() =>
  providers.value.map((p) => ({
    label: `${p.name} · ${p.model}${providerCooling(p) ? '（熔断中）' : ''}`,
    value: p.id,
    disabled: p.status !== 'ACTIVE',
  })),
)

const templateOptions = computed(() => [
  ...(editing.value ? [{ label: '保持当前规则', value: 'keep' }] : []),
  ...templates.value
    .filter((t) => (t.kind || 'REVIEW') === form.decisionMode)
    .map((t) => ({ label: `${t.name} · ${t.description}`, value: t.key })),
  { label: '自定义规则 · 可视化编辑', value: 'custom' },
])

function rulesTextForPreset(key: string) {
  const template = templates.value.find((t) => t.key === key)
  return JSON.stringify(template?.rules || [], null, 2)
}

function applyRulesPreset(key: string) {
  form.rulesPreset = key
  if (key !== 'keep' && key !== 'custom') form.rulesText = rulesTextForPreset(key)
}

const activeCount = ref(0)
const classifyCount = ref(0)
const todayCount = ref(0)
const summaryLoaded = ref(false)
const selectedProvider = computed(() => providers.value.find((p) => p.id === form.providerId))
const jevProviders = computed(() => providers.value.filter((p) => p.kind === 'JEV' && p.status === 'ACTIVE'))
const selectedTemplate = computed(() => templates.value.find((t) => t.key === form.rulesPreset))

const promptSummary = computed(() => {
  const text = form.reviewPrompt.replace(/\s+/g, ' ').trim()
  if (!text) return '尚未填写审核要求'
  return text.length > 150 ? `${text.slice(0, 150)}…` : text
})

const categorySummary = computed(() => {
  const items = form.categories.slice(0, 5).map((category) => ({
    code: category.code || '未命名',
    result: category.result || 'REVIEW',
  }))
  return { count: form.categories.length, items, more: Math.max(0, form.categories.length - items.length) }
})

const ruleSummary = computed(() => {
  try {
    const rules = JSON.parse(form.rulesText)
    if (!Array.isArray(rules)) throw new Error('规则组必须是数组')
    const groups = rules.length
    const conditions = rules.reduce(
      (total, group) => total + (Array.isArray(group?.items) ? group.items.length : 0),
      0,
    )
    const names = rules.slice(0, 3).map((group) => String(group?.name || '未命名规则'))
    const preview = names.length
      ? `${names.join('、')}${groups > names.length ? ` 等 ${groups} 组` : ''}`
      : '未配置规则组，将直接使用默认结论'
    return { groups, conditions, preview, invalid: false }
  } catch {
    return { groups: 0, conditions: 0, preview: '规则 JSON 需要修复', invalid: true }
  }
})

const routeSummary = computed(() => {
  try {
    const routes = JSON.parse(form.routesText)
    if (!routes || typeof routes !== 'object' || Array.isArray(routes)) {
      throw new Error('结果动作路由必须是对象')
    }
    const parts: string[] = []
    let total = 0
    for (const result of ['PASS', 'REJECT', 'REVIEW']) {
      const actions = Array.isArray(routes[result]?.actions) ? routes[result].actions.length : 0
      total += actions
      if (actions) parts.push(`${result} ${actions}`)
    }
    return {
      total,
      preview: total ? parts.join(' · ') : '未配置额外动作，结果仅落库',
      invalid: false,
    }
  } catch {
    return { total: 0, preview: '路由 JSON 需要修复', invalid: true }
  }
})

function resultLabel(value: string) {
  if (value === 'PASS') return '通过'
  if (value === 'REJECT') return '不通过'
  return '待议'
}

function formatDedupe(seconds: number) {
  if (!seconds) return '不去重'
  if (seconds % 86400 === 0) return `${seconds / 86400} 天`
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`
  return `${seconds} 秒`
}

const policySummary = computed(() =>
  [
    `默认 ${resultLabel(form.defaultResult)}`,
    `失败 ${resultLabel(form.onModelFail)}`,
    form.reviewMode === 'EXTERNAL' ? '外部复核' : '平台内复核',
    formatDedupe(form.dedupeWindowSec),
  ].join(' · '),
)

const exportSummary = computed(() => {
  const target = exportForm.range
    ? `${dayjs(exportForm.range[0]).format('MM-DD')} 至 ${dayjs(exportForm.range[1]).format('MM-DD')}`
    : '全部时间'
  const status = exportForm.statuses.length ? `${exportForm.statuses.length} 种状态` : '全部状态'
  const result = exportForm.results.length ? `${exportForm.results.length} 种结论` : '全部结论'
  const source = exportForm.sources.length ? `${exportForm.sources.length} 种来源` : '全部来源'
  return `${target} · ${status} · ${result} · ${source}`
})

function closeConfigEditors() {
  promptModalShow.value = false
  categoryModalShow.value = false
  rulesModalShow.value = false
  routeModalShow.value = false
  policyModalShow.value = false
}

function resetForm() {
  form.name = ''
  form.code = ''
  form.description = ''
  form.mediaTypes = ['text', 'image']
  form.decisionMode = 'REVIEW'
  form.categories = []
  form.providerId = providers.value.find((p) => p.status === 'ACTIVE')?.id || providers.value[0]?.id || null
  form.reviewPrompt = defaultPrompt
  form.rulesPreset = 'standard'
  form.rulesText = rulesTextForPreset('standard')
  form.routesText = defaultRoutesText
  form.defaultResult = 'REVIEW'
  form.onModelFail = 'REVIEW'
  form.reviewMode = 'INTERNAL'
  form.dedupeWindowSec = 86400
  errorText.value = ''
  editing.value = null
  snapshotFormBaseline()
}

function openCreate() {
  if (!workspace.currentProjectId) {
    message.warning('请先选择项目')
    return
  }
  closeConfigEditors()
  resetForm()
  modalShow.value = true
}

function openEdit(audit: Audit) {
  closeConfigEditors()
  editing.value = audit
  form.name = audit.name
  form.code = audit.code
  form.description = audit.description || ''
  form.mediaTypes = [...(audit.media_types || [])]
  form.decisionMode = audit.decision_mode === 'CLASSIFY_ONLY' ? 'CLASSIFY_ONLY' : audit.decision_mode === 'CLASSIFY' ? 'CLASSIFY' : 'REVIEW'
  form.categories = (audit.categories || []).map((category) => ({ ...category }))
  form.providerId = audit.provider_id
  form.reviewPrompt = audit.review_prompt || defaultPrompt
  form.rulesPreset = 'keep'
  form.rulesText = JSON.stringify(audit.rules || [], null, 2)
  form.routesText = JSON.stringify(audit.routes || JSON.parse(defaultRoutesText), null, 2)
  form.defaultResult = audit.decision_mode === 'CLASSIFY_ONLY' ? 'PASS' : audit.default_result
  form.onModelFail = audit.on_model_fail
  form.reviewMode = audit.review_mode === 'EXTERNAL' ? 'EXTERNAL' : 'INTERNAL'
  form.dedupeWindowSec = audit.dedupe_window_sec
  errorText.value = ''
  snapshotFormBaseline()
  modalShow.value = true
}

function setDecisionMode(value: string | number) {
  const mode = value === 'CLASSIFY_ONLY' ? 'CLASSIFY_ONLY' : value === 'CLASSIFY' ? 'CLASSIFY' : 'REVIEW'
  if (mode === form.decisionMode) return
  form.decisionMode = mode
  if (mode === 'CLASSIFY' || mode === 'CLASSIFY_ONLY') {
    if (mode === 'CLASSIFY_ONLY') form.defaultResult = 'PASS'
    if (form.categories.length < 2) {
      form.categories = exampleCategories.value.map((category) => ({
        ...category,
        result: mode === 'CLASSIFY_ONLY' ? 'PASS' : category.result,
      }))
    } else if (mode === 'CLASSIFY_ONLY') {
      form.categories = form.categories.map((category) => ({ ...category, result: 'PASS' }))
    }
    if (mode === 'CLASSIFY_ONLY') ensureClassifierUnknownCategory()
    form.rulesPreset = mode === 'CLASSIFY_ONLY' ? 'classifier' : 'classify'
    form.rulesText = mode === 'CLASSIFY_ONLY' ? '[]' : rulesTextForPreset('classify')
    if (!editing.value) {
      form.reviewPrompt = mode === 'CLASSIFY_ONLY' ? defaultClassifierPrompt.value : defaultClassifyPrompt.value
      if (jevProviders.value.length) form.providerId = jevProviders.value[0].id
    }
  } else {
    form.categories = []
    form.rulesPreset = 'standard'
    form.rulesText = rulesTextForPreset('standard')
    if (!editing.value) form.reviewPrompt = defaultPrompt
  }
}

function useJevProvider() {
  if (!jevProviders.value.length) {
    message.warning('当前项目没有启用的 JEV 模型，请先到「模型接入」配置')
    return
  }
  form.providerId = jevProviders.value[0].id
  message.success(`已切换为 ${jevProviders.value[0].name}`)
}

function addCategory() {
  let index = form.categories.length + 1
  let code = `category_${index}`
  while (form.categories.some((category) => category.code === code)) {
    index += 1
    code = `category_${index}`
  }
  form.categories.push({ code, name: `新分类 ${index}`, description: '', result: 'REVIEW' })
}

function removeCategory(index: number) {
  if (form.categories.length <= 2) {
    message.warning('分类审核至少保留 2 个分类')
    return
  }
  form.categories.splice(index, 1)
}

function csvEscape(value: string) {
  const text = value || ''
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text
}

function categoriesAsRows() {
  return form.categories.map((category) => ({
    code: category.code,
    name: category.name,
    description: category.description || '',
    result: form.decisionMode === 'CLASSIFY_ONLY' ? 'PASS' : category.result,
  }))
}

function onCategoryExport(key: string) {
  downloadCategories(key === 'json' ? 'json' : 'csv')
}

function downloadCategories(format: 'csv' | 'json') {
  if (!form.categories.length) {
    message.warning('当前没有可导出的分类类型')
    return
  }
  const rows = categoriesAsRows()
  let content = ''
  let mime = 'text/plain;charset=utf-8'
  if (format === 'json') {
    content = JSON.stringify(rows, null, 2)
    mime = 'application/json;charset=utf-8'
  } else {
    content = '\uFEFF' + ['code,name,description,result', ...rows.map((row) => [row.code, row.name, row.description, row.result].map(csvEscape).join(','))].join('\r\n')
    mime = 'text/csv;charset=utf-8'
  }
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${editing.value?.code || 'classifier'}-categories.${format}`
  link.click()
  URL.revokeObjectURL(url)
  message.success(`已导出 ${rows.length} 个分类类型`)
}

const categoryImportInput = ref<HTMLInputElement | null>(null)

function triggerCategoryImport() {
  categoryImportInput.value?.click()
}

function parseCSVRows(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false
  for (let i = 0; i < text.length; i += 1) {
    const char = text[i]
    if (quoted) {
      if (char === '"' && text[i + 1] === '"') {
        cell += '"'
        i += 1
      } else if (char === '"') {
        quoted = false
      } else {
        cell += char
      }
    } else if (char === '"') {
      quoted = true
    } else if (char === ',') {
      row.push(cell)
      cell = ''
    } else if (char === '\n' || char === '\r') {
      if (char === '\r' && text[i + 1] === '\n') i += 1
      row.push(cell)
      if (row.some((value) => value.trim() !== '')) rows.push(row)
      row = []
      cell = ''
    } else {
      cell += char
    }
  }
  row.push(cell)
  if (row.some((value) => value.trim() !== '')) rows.push(row)
  return rows
}

function normalizeImportedCategories(rows: Array<Record<string, string>>): AuditCategory[] {
  const pure = form.decisionMode === 'CLASSIFY_ONLY'
  const seen = new Set<string>()
  const out: AuditCategory[] = []
  for (const raw of rows) {
    const code = String(raw.code || raw['分类编码'] || raw['编码'] || '').trim().toLowerCase()
    const name = String(raw.name || raw['分类名称'] || raw['名称'] || '').trim()
    const description = String(raw.description || raw['说明'] || raw['判定说明'] || '').trim()
    const result = String(raw.result || raw['结论'] || raw['映射结论'] || '').trim().toUpperCase()
    if (!code && !name) continue
    if (!/^[a-z][a-z0-9_]{1,31}$/.test(code)) {
      throw new Error(`分类编码「${code || '(空)'}」不合法，需小写字母开头，仅含小写字母、数字下划线`)
    }
    if (!name) throw new Error(`分类 ${code} 缺少名称`)
    if (seen.has(code)) continue
    seen.add(code)
    const mapped = ['PASS', 'REJECT', 'REVIEW'].includes(result) ? result : (pure ? 'PASS' : 'REVIEW')
    out.push({ code, name, description, result: pure ? 'PASS' : mapped })
  }
  if (!out.length) throw new Error('文件里没有可导入的分类类型')
  if (out.length > (pure ? 200 : 20)) throw new Error(`分类数量不能超过 ${pure ? 200 : 20} 个`)
  return out
}

async function onCategoryFilePicked(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const text = await file.text()
    let rows: Array<Record<string, string>> = []
    if (file.name.toLowerCase().endsWith('.json') || text.trim().startsWith('[') || text.trim().startsWith('{')) {
      const parsed = JSON.parse(text)
      const list = Array.isArray(parsed) ? parsed : parsed.categories
      if (!Array.isArray(list)) throw new Error('JSON 需为分类数组或包含 categories 数组')
      rows = list
    } else {
      const parsed = parseCSVRows(text.replace(/^\uFEFF/, ''))
      if (parsed.length < 2) throw new Error('CSV 至少需要表头和一行分类')
      const header = parsed[0].map((value) => value.trim().toLowerCase())
      const indexOf = (names: string[]) => header.findIndex((value) => names.includes(value))
      const codeIndex = indexOf(['code', '分类编码', '编码'])
      const nameIndex = indexOf(['name', '分类名称', '名称'])
      const descIndex = indexOf(['description', '说明', '判定说明'])
      const resultIndex = indexOf(['result', '结论', '映射结论'])
      if (codeIndex < 0 || nameIndex < 0) throw new Error('CSV 表头需包含 code,name（或 分类编码,分类名称）')
      rows = parsed.slice(1).map((cells) => ({
        code: cells[codeIndex] || '',
        name: cells[nameIndex] || '',
        description: descIndex >= 0 ? cells[descIndex] || '' : '',
        result: resultIndex >= 0 ? cells[resultIndex] || '' : '',
      }))
    }
    const imported = normalizeImportedCategories(rows)
    const applyImport = () => {
      form.categories = imported
      ensureClassifierUnknownCategory()
      message.success(`已导入 ${imported.length} 个分类类型`)
    }
    if (form.categories.length > 0) {
      dialog.warning({
        title: '导入分类类型',
        content: `将用文件中的 ${imported.length} 个类型覆盖当前 ${form.categories.length} 个类型，是否继续？`,
        positiveText: '覆盖导入',
        negativeText: '取消',
        onPositiveClick: applyImport,
      })
    } else {
      applyImport()
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '导入分类失败')
  }
}

function loadExampleCategories() {
  const pure = form.decisionMode === 'CLASSIFY_ONLY'
  form.categories = exampleCategories.value.map((category) => ({ ...category, result: pure ? 'PASS' : category.result }))
  ensureClassifierUnknownCategory()
  message.success('已载入分类示例，可按业务调整')
}

function validateCategoryForm(): string {
  const pure = form.decisionMode === 'CLASSIFY_ONLY'
  if (pure && form.categories.length < 1) return '纯分类器至少需要 1 个分类类型'
  if (!pure && form.categories.length < 2) return '分类审核至少需要 2 个分类'
  if (form.categories.length > (pure ? 200 : 20)) return `分类数量不能超过 ${pure ? 200 : 20} 个`
  const seen = new Set<string>()
  for (const [index, category] of form.categories.entries()) {
    const code = category.code.trim()
    if (!/^[a-z][a-z0-9_]{1,31}$/.test(code)) {
      return `第 ${index + 1} 个分类编码需以小写字母开头，仅含小写字母、数字、下划线`
    }
    if (seen.has(code)) return `分类编码 ${code} 重复`
    seen.add(code)
    if (!category.name.trim()) return `请填写第 ${index + 1} 个分类名称`
    if (!['PASS', 'REJECT', 'REVIEW'].includes(category.result)) return `分类 ${code} 的映射结论不合法`
  }
  return ''
}

function auditMoreOptions(audit: Audit) {
  const options: Array<Record<string, unknown>> = [
    { label: '复制审核编码', key: 'copy' },
    { label: '查看详情', key: 'detail' },
    { label: '查看数据报表', key: 'analytics' },
  ]
  if (auth.can('audit:write')) {
    options.push(
      { type: 'divider', key: 'd1' },
      { label: '编辑配置', key: 'edit' },
      { label: '脚本日志', key: 'script-logs' },
      { label: '复制配置', key: 'clone' },
      { type: 'divider', key: 'd2' },
      { label: '移入回收站', key: 'delete' },
    )
  }
  return options
}

function handleAuditMore(key: string, audit: Audit) {
  switch (key) {
    case 'copy':
      copyText(audit.code).then((ok) => (ok ? message.success('审核编码已复制') : message.error('复制失败')))
      break
    case 'detail':
      openDetail(audit)
      break
    case 'analytics':
      router.push({ name: 'analytics', query: { project_id: String(audit.project_id), audit_id: String(audit.id) } })
      break
    case 'edit':
      openEdit(audit)
      break
    case 'script-logs':
      scriptLogsAudit.value = audit
      scriptLogsShow.value = true
      break
    case 'clone':
      openClone(audit)
      break
    case 'delete':
      remove(audit)
      break
  }
}

function resetExportFilters() {
  exportForm.range = null
  exportForm.statuses = []
  exportForm.results = []
  exportForm.sources = []
  exportForm.mediaTypes = []
  exportForm.keyword = ''
}

function openExport(audit: Audit) {
  exportTarget.value = audit
  resetExportFilters()
  exportShow.value = true
}

async function runExport() {
  if (!exportTarget.value) return
  exporting.value = true
  try {
    const params: Record<string, unknown> = {}
    if (exportForm.range) {
      params.start = dayjs(exportForm.range[0]).format('YYYY-MM-DD')
      params.end = dayjs(exportForm.range[1]).format('YYYY-MM-DD')
    }
    if (exportForm.statuses.length) params.status = exportForm.statuses.join(',')
    if (exportForm.results.length) params.result = exportForm.results.join(',')
    if (exportForm.sources.length) params.source_type = exportForm.sources.join(',')
    if (exportForm.mediaTypes.length) params.media_type = exportForm.mediaTypes.join(',')
    if (exportForm.keyword.trim()) params.keyword = exportForm.keyword.trim()
    const blob = await auditApi.exportTasks(exportTarget.value.id, params)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `smartdoc-${exportTarget.value.code}-${dayjs().format('YYYYMMDD-HHmm')}.csv`
    link.click()
    URL.revokeObjectURL(url)
    message.success('已按筛选条件导出 CSV（Excel 可直接打开）')
    exportShow.value = false
  } catch (error) {
    message.error(error instanceof Error ? error.message : '导出失败')
  } finally {
    exporting.value = false
  }
}

async function loadSummary() {
  const projectID = workspace.currentProjectId
  if (!projectID) {
    activeCount.value = 0
    classifyCount.value = 0
    todayCount.value = 0
    summaryLoaded.value = true
    return
  }
  try {
    const [activeData, classifyData, classifierData, analyticsData] = await Promise.all([
      auditApi.list({ project_id: projectID, status: 'ACTIVE', page: 1, page_size: 1 }),
      auditApi.list({ project_id: projectID, decision_mode: 'CLASSIFY', page: 1, page_size: 1 }),
      auditApi.list({ project_id: projectID, decision_mode: 'CLASSIFY_ONLY', page: 1, page_size: 1 }),
      analyticsApi.get({ range: 'today', project_id: projectID }).catch(() => null),
    ])
    if (workspace.currentProjectId !== projectID) return
    activeCount.value = activeData.total
    classifyCount.value = classifyData.total + classifierData.total
    todayCount.value = analyticsData?.overview?.total ?? 0
    summaryLoaded.value = true
  } catch {
    // Optional summary counters must not block the audit list.
  }
}

async function loadMoreAudits() {
  if (!workspace.currentProjectId || loadingMore.value || !hasMoreAudits.value) return
  loadingMore.value = true
  try {
    const data = await auditApi.list({
      project_id: workspace.currentProjectId,
      page: auditPage.value + 1,
      page_size: auditPageSize,
    })
    audits.value.push(...data.items)
    auditTotal.value = data.total
    auditPage.value += 1
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载更多审核配置失败')
  } finally {
    loadingMore.value = false
  }
}

async function load() {
  if (!workspace.currentProjectId) {
    audits.value = []
    providers.value = []
    auditTotal.value = 0
    activeCount.value = 0
    classifyCount.value = 0
    todayCount.value = 0
    summaryLoaded.value = true
    return
  }
  loading.value = true
  try {
    const [auditData, providerData] = await Promise.all([
      auditApi.list({ project_id: workspace.currentProjectId, page: 1, page_size: auditPageSize }),
      providerApi.list({ project_id: workspace.currentProjectId }),
    ])
    audits.value = auditData.items
    auditTotal.value = auditData.total
    auditPage.value = 1
    providers.value = providerData.items
    summaryLoaded.value = false
    void loadSummary()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载审核配置失败')
  } finally {
    loading.value = false
  }
}

async function save(openGuide = true): Promise<Audit | null> {
  errorText.value = ''
  if (!form.name.trim() || (!editing.value && !form.code.trim())) {
    errorText.value = '请填写审核名称与编码'
    return null
  }
  if (!form.providerId) {
    errorText.value = '请选择模型供应商'
    return null
  }
  if (!form.mediaTypes.length) {
    errorText.value = '至少选择一种媒体类型'
    return null
  }
  if (form.decisionMode === 'CLASSIFY' || form.decisionMode === 'CLASSIFY_ONLY') {
    const error = validateCategoryForm()
    if (error) {
      errorText.value = error
      return null
    }
  }
  let rules: unknown
  let routes: unknown
  try {
    rules = JSON.parse(form.rulesText)
    if (!Array.isArray(rules)) throw new Error('规则组 JSON 必须是数组')
  } catch (error) {
    errorText.value = error instanceof Error ? `规则组 JSON 无效：${error.message}` : '规则组 JSON 无效'
    return null
  }
  try {
    routes = JSON.parse(form.routesText)
    if (!routes || typeof routes !== 'object' || Array.isArray(routes)) throw new Error('结果动作路由 JSON 必须是对象')
  } catch (error) {
    errorText.value = error instanceof Error ? `结果动作路由 JSON 无效：${error.message}` : '结果动作路由 JSON 无效'
    return null
  }
  const categories = form.categories.map((category) => ({
    code: category.code.trim(),
    name: category.name.trim(),
    description: (category.description || '').trim(),
    result: form.decisionMode === 'CLASSIFY_ONLY' ? 'PASS' : category.result,
  }))
  const payload: Record<string, unknown> = {
    project_id: workspace.currentProjectId,
    name: form.name.trim(),
    code: form.code.trim(),
    description: form.description,
    media_types: form.mediaTypes,
    decision_mode: form.decisionMode,
    categories,
    provider_id: form.providerId,
    review_prompt: form.reviewPrompt,
    default_result: form.defaultResult,
    on_model_fail: form.onModelFail,
    review_mode: form.reviewMode,
    dedupe_window_sec: form.dedupeWindowSec,
    rules,
    routes,
    status: editing.value?.status === 'DRAFT' ? 'DRAFT' : 'ACTIVE',
  }
  saving.value = true
  try {
    if (editing.value) {
      const updated = await auditApi.update(editing.value.id, payload)
      message.success('审核配置已更新')
      modalShow.value = false
      await load()
      return updated
    }
    const created = await auditApi.create(payload)
    message.success('审核配置已创建')
    modalShow.value = false
    await load()
    if (openGuide) {
      guideAuditId.value = created.id
      guideShow.value = true
    }
    return created
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
    return null
  } finally {
    saving.value = false
  }
}

function auditModeLabel(mode?: string) {
  if (mode === 'CLASSIFY_ONLY') return '纯分类器'
  if (mode === 'CLASSIFY') return '分类审核'
  return '合规审核'
}

function describeAudit(audit: Audit) {
  const mode = auditModeLabel(audit.decision_mode)
  const media = (audit.media_types || []).join(' + ') || '文本'
  const categories = audit.categories?.length || 0
  const tail = audit.decision_mode === 'CLASSIFY_ONLY'
    ? ` · ${categories} 个类型 · 结论固定 PASS`
    : audit.decision_mode === 'CLASSIFY'
      ? ` · ${categories} 个分类 · 默认结论 ${audit.default_result}`
      : ` · 默认结论 ${audit.default_result}`
  return `${mode} · ${media}${tail}`
}

function openAssistantEditPicker() {
  if (!workspace.currentProjectId) {
    message.warning('请先选择项目')
    return
  }
  if (!hasGeneralProvider.value) {
    message.warning('AI 配置助手需要项目中至少有一个启用的通用大模型（OpenAI 兼容或 Anthropic 协议）')
    return
  }
  if (!audits.value.length) {
    message.warning('当前项目还没有可编辑的审核配置')
    return
  }
  assistantPickId.value = null
  assistantPickShow.value = true
}

function confirmAssistantEditPick() {
  const audit = audits.value.find((item) => item.id === assistantPickId.value)
  if (!audit) {
    message.warning('请选择要编辑的审核配置')
    return
  }
  assistantPickShow.value = false
  startAssistant('edit', audit)
}

function startAssistant(mode: 'create' | 'edit', audit?: Audit) {
  if (!workspace.currentProjectId) {
    message.warning('请先选择项目')
    return
  }
  if (!hasGeneralProvider.value) {
    message.warning('AI 配置助手需要项目中至少有一个启用的通用大模型（OpenAI 兼容或 Anthropic 协议）')
    return
  }
  const candidates = providers.value.filter((p) => p.status === 'ACTIVE' && p.kind !== 'JEV')
  const preferred = candidates.find((p) => p.id === assistantProviderId.value) || candidates[0]
  assistantProviderId.value = preferred?.id || null
  assistantMode.value = mode
  assistantReadyConfig.value = null
  assistantChanges.value = []
  assistantSaved.value = false
  assistantError.value = ''
  assistantInput.value = ''
  assistantImages.value = []
  assistantAbort?.abort()
  assistantAbort = null
  if (mode === 'create') {
    resetForm()
    editing.value = null
    if (assistantProviderId.value) form.providerId = assistantProviderId.value
  } else if (audit) {
    openEdit(audit)
    modalShow.value = false
  }
  assistantMessages.value = [{
    role: 'assistant',
    content: mode === 'create'
      ? '你好，我是审核配置助手。请告诉我：这次要审核什么类型的档案？哪些情况必须驳回？是否需要按类别分类？任务通过什么方式接入（开放 API / Redis Stream / WebSocket）？如果选 Redis，我会从当前项目已启用且测试成功的 Redis 连接中选择。'
      : `当前正在编辑「${audit?.name || form.name}」（${audit?.code || form.code}）。它现在是${audit ? describeAudit(audit) : '当前表单中的配置'}。请告诉我：目前哪里不符合要求、你希望改成什么？例如“把默认结论改成 REVIEW”“新增发票分类并映射为通过”。我会只修改你指定的部分，其余配置保持不变。`,
  }]
  assistantShow.value = true
}

function buildAssistantCurrentConfig() {
  let rules: unknown = []
  let routes: unknown = null
  try {
    rules = JSON.parse(form.rulesText)
  } catch {
    rules = []
  }
  try {
    routes = JSON.parse(form.routesText)
  } catch {
    routes = null
  }
  return {
    name: form.name,
    code: form.code,
    description: form.description,
    media_types: form.mediaTypes,
    decision_mode: form.decisionMode,
    categories: form.categories,
    review_prompt: form.reviewPrompt,
    rules,
    routes,
    default_result: form.defaultResult,
    on_model_fail: form.onModelFail,
    review_mode: form.reviewMode,
    dedupe_window_sec: form.dedupeWindowSec,
  }
}

async function scrollAssistantToBottom() {
  await nextTick()
  if (assistantChat.value) {
    assistantChat.value.scrollTop = assistantChat.value.scrollHeight
  }
}

function applyAssistantDraft(config: AuditAssistantConfig) {
  form.name = config.name || form.name
  form.code = editing.value?.code || config.code || form.code
  form.description = config.description || ''
  form.mediaTypes = (config.media_types || ['text', 'image']).filter((type) => type === 'text' || type === 'image')
  if (!form.mediaTypes.length) form.mediaTypes = ['text', 'image']
  form.decisionMode = config.decision_mode === 'CLASSIFY_ONLY' ? 'CLASSIFY_ONLY' : config.decision_mode === 'CLASSIFY' ? 'CLASSIFY' : 'REVIEW'
  form.categories = (config.categories || []).map((category) => ({
    ...category,
    result: form.decisionMode === 'CLASSIFY_ONLY' ? 'PASS' : category.result,
  }))
  if (editing.value) {
    form.providerId = editing.value.provider_id
  } else if (config.provider_id) {
    form.providerId = config.provider_id
  }
  form.reviewPrompt = config.review_prompt || form.reviewPrompt
  form.rulesPreset = form.decisionMode === 'CLASSIFY_ONLY' ? 'classifier' : form.decisionMode === 'CLASSIFY' ? 'classify' : 'standard'
  form.rulesText = JSON.stringify(config.rules || [], null, 2)
  form.routesText = JSON.stringify(config.routes || JSON.parse(defaultRoutesText), null, 2)
  if (form.decisionMode === 'CLASSIFY_ONLY') ensureClassifierUnknownCategory()
  form.defaultResult = form.decisionMode === 'CLASSIFY_ONLY' ? 'PASS' : config.default_result || 'REVIEW'
  form.onModelFail = config.on_model_fail || 'REVIEW'
  form.reviewMode = config.review_mode === 'EXTERNAL' ? 'EXTERNAL' : 'INTERNAL'
  form.dedupeWindowSec = config.dedupe_window_sec ?? 86400
}

function assistantTriggerText(triggers?: Record<string, string>): string {
  const active = Object.entries(triggers || {})
    .filter(([, status]) => status === 'ACTIVE')
    .map(([type]) => triggerLabel(type))
  return active.length ? active.join(' / ') : '全部关闭'
}

function summarizeAssistantTriggers(config: AuditAssistantConfig): string | null {
  if (!config.triggers) return null
  const beforeTypes = assistantMode.value === 'edit'
    ? (editing.value?.active_triggers || [])
    : ['OPENAPI']
  const before = beforeTypes.map((type) => triggerLabel(type)).sort()
  const after = Object.entries(config.triggers)
    .filter(([, status]) => status === 'ACTIVE')
    .map(([type]) => triggerLabel(type))
    .sort()
  if (before.join('|') === after.join('|')) return null
  return `接入方式：${before.length ? before.join(' / ') : '全部关闭'} → ${after.length ? after.join(' / ') : '全部关闭'}`
}

function summarizeAssistantChanges(config: AuditAssistantConfig) {
  const before = buildAssistantCurrentConfig()
  const changes: string[] = []
  if (config.name && config.name !== before.name) changes.push(`名称：${before.name || '未命名'} → ${config.name}`)
  if (config.description !== before.description) changes.push('简介已更新')
  const beforeMedia = [...(before.media_types || [])].sort().join('+')
  const afterMedia = [...(config.media_types || [])].sort().join('+')
  if (beforeMedia !== afterMedia) changes.push(`媒体类型：${beforeMedia || '无'} → ${afterMedia || '无'}`)
  if (config.decision_mode !== before.decision_mode) changes.push(`审核模式：${auditModeLabel(String(before.decision_mode))} → ${auditModeLabel(config.decision_mode)}`)
  const beforeCategories = (before.categories || []).map((item) => `${item.code}:${item.name}:${item.result}`).join('|')
  const afterCategories = (config.categories || []).map((item) => `${item.code}:${item.name}:${item.result}`).join('|')
  if (beforeCategories !== afterCategories) changes.push(`分类映射：${(before.categories || []).length} 个 → ${(config.categories || []).length} 个`)
  if (config.review_prompt !== before.review_prompt) changes.push('审核要求 Prompt 已更新')
  if (config.default_result !== before.default_result) changes.push(`默认结论：${before.default_result} → ${config.default_result}`)
  if (config.on_model_fail !== before.on_model_fail) changes.push(`模型失败兜底：${before.on_model_fail} → ${config.on_model_fail}`)
  if (Number(config.dedupe_window_sec) !== Number(before.dedupe_window_sec)) changes.push(`去重窗口：${before.dedupe_window_sec}s → ${config.dedupe_window_sec}s`)
  if (JSON.stringify(config.rules || []) !== JSON.stringify(before.rules || [])) changes.push('规则组已更新')
  if (JSON.stringify(config.routes || {}) !== JSON.stringify(before.routes || {})) changes.push('结果动作路由已更新')
  const triggerChange = summarizeAssistantTriggers(config)
  if (triggerChange) changes.push(triggerChange)
  return changes
}

async function applyAssistantTriggers(
  auditId: number,
  triggers: Record<string, string>,
  redisTrigger?: AuditAssistantConfig['redis_trigger'],
) {
  const desired = Object.entries(triggers).filter(([type, status]) =>
    ['OPENAPI', 'REDIS', 'WEBSOCKET'].includes(type)
    && (status === 'ACTIVE' || status === 'DISABLED'))
  if (!desired.length) return
  const detail = await auditApi.get(auditId)
  const failures: string[] = []
  for (const [type, status] of desired) {
    const trigger = detail.triggers.find((item) => item.type === type)
    if (!trigger) continue
    try {
      if (type === 'REDIS') {
        if (status === 'ACTIVE' && !redisTrigger) {
          if (trigger.status === 'ACTIVE' && trigger.redis_connection_id && trigger.stream) continue
          throw new Error('启用 Redis 接入需要先选择项目 Redis 连接并填写 Stream')
        }
        if (status === 'ACTIVE' && redisTrigger) {
          const unchanged = trigger.status === 'ACTIVE'
            && trigger.redis_connection_id === redisTrigger.connection_id
            && trigger.redis_db === redisTrigger.db
            && trigger.stream === redisTrigger.stream
            && trigger.start_position === (redisTrigger.start_position || '$')
          if (unchanged) continue
          await auditApi.updateTrigger(trigger.id, {
            status,
            redis_connection_id: redisTrigger.connection_id,
            redis_db: redisTrigger.db,
            stream: redisTrigger.stream,
            consumer_group: redisTrigger.consumer_group || '',
            start_position: redisTrigger.start_position || '$',
            batch_size: redisTrigger.batch_size || 16,
            max_inflight: redisTrigger.max_inflight || 1,
          })
        } else {
          if (trigger.status === status) continue
          await auditApi.updateTrigger(trigger.id, { status })
        }
      } else {
        if (trigger.status === status) continue
        await auditApi.updateTrigger(trigger.id, { status })
      }
    } catch (error) {
      failures.push(`${triggerLabel(type)}：${error instanceof Error ? error.message : '更新失败'}`)
    }
  }
  if (failures.length) throw new Error(`审核配置已保存，但接入方式更新失败（${failures.join('；')}）`)
}

async function applyAssistantConfig(config: AuditAssistantConfig) {
  const target = currentAssistantMessage()
  if (target) {
    target.pending = true
  }
  assistantChanges.value = assistantMode.value === 'edit'
    ? summarizeAssistantChanges(config)
    : (() => {
        const triggerChange = summarizeAssistantTriggers(config)
        return triggerChange ? [triggerChange] : []
      })()
  assistantReadyConfig.value = config
  applyAssistantDraft(config)
  upsertAssistantStep({ id: 'save', label: '保存审核配置', status: 'running', at: Date.now() })
  await scrollAssistantToBottom()
  const saved = await save(false)
  if (!saved) {
    upsertAssistantStep({ id: 'save', label: '保存审核配置', status: 'error', detail: errorText.value, at: Date.now() })
    if (target) {
      target.pending = false
    }
    assistantError.value = errorText.value || '已应用配置，但保存失败，请检查表单后手动保存'
    modalShow.value = true
    return
  }
  upsertAssistantStep({ id: 'save', label: '保存审核配置', status: 'done', detail: saved.name, at: Date.now() })
  if (config.triggers && Object.keys(config.triggers).length) {
    upsertAssistantStep({ id: 'access', label: '应用接入方式', status: 'running', at: Date.now() })
    try {
      await applyAssistantTriggers(saved.id, config.triggers, config.redis_trigger)
      await load()
      upsertAssistantStep({
        id: 'access', label: '应用接入方式', status: 'done',
        detail: assistantTriggerText(config.triggers), at: Date.now(),
      })
    } catch (error) {
      upsertAssistantStep({
        id: 'access', label: '应用接入方式', status: 'error',
        detail: error instanceof Error ? error.message : '更新失败', at: Date.now(),
      })
      if (target) {
        target.pending = false
      }
      assistantSaved.value = true
      assistantError.value = error instanceof Error ? error.message : '接入方式更新失败，请到接入指南中手动调整'
      return
    }
  }
  assistantSaved.value = true
  if (target) {
    target.pending = false
  }
  if (assistantMode.value === 'create') {
    assistantShow.value = false
    assistantMessages.value = []
    guideAuditId.value = saved.id
    guideShow.value = true
  }
}

function handleAssistantKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault()
  void sendAssistant()
}

function applyVoiceInput(text: string) {
  assistantInput.value = assistantInput.value ? `${assistantInput.value} ${text}` : text
  nextTick(() => assistantInputRef.value?.focus())
}

async function sendAssistant() {
  const text = assistantInput.value.trim()
  const images = assistantImages.value.map((item) => item.dataUrl)
  if ((!text && !images.length) || assistantBusy.value) return
  if (!assistantProviderId.value) {
    assistantError.value = '没有可用的通用大模型，请先到「模型接入」启用一个通用大模型'
    return
  }
  assistantInput.value = ''
  assistantImages.value = []
  assistantError.value = ''
  assistantBusy.value = true
  assistantMessages.value.push({ role: 'user', content: text, images })
  const assistantMessage: AuditAssistantMessage = {
    role: 'assistant', content: '', pending: true, steps: [],
  }
  assistantMessages.value.push(assistantMessage)
  await scrollAssistantToBottom()
  const controller = new AbortController()
  assistantAbort = controller
  let result: AuditAssistantResult | null = null
  try {
    await auditApi.assistantStream({
      project_id: workspace.currentProjectId,
      audit_id: editing.value?.id || 0,
      provider_id: assistantProviderId.value,
      messages: assistantMessages.value.map((item) => ({
        role: item.role,
        content: item.content,
        images: item.images || [],
      })),
      current_config: buildAssistantCurrentConfig(),
    }, {
      signal: controller.signal,
      onEvent: (event, data) => {
        const payload = data as Record<string, any>
        if (event === 'step') {
          upsertAssistantStep(payload as unknown as AuditAssistantStep)
          void scrollAssistantToBottom()
        } else if (event === 'reply') {
          result = payload as unknown as AuditAssistantResult
        } else if (event === 'error') {
          throw new Error(String(payload?.message || 'AI 配置助手请求失败'))
        }
      },
    })
    const finalResult = result as AuditAssistantResult | null
    if (!finalResult) throw new Error('模型没有返回结果')
    if (finalResult.steps?.length) {
      assistantMessage.steps = dedupeAssistantSteps(finalResult.steps)
    }
    assistantMessage.content = finalResult.reply
    assistantMessage.pending = false
    await scrollAssistantToBottom()
    if (finalResult.ready && finalResult.config) {
      await applyAssistantConfig(finalResult.config)
    }
  } catch (error) {
    if (controller.signal.aborted) return
    assistantMessage.pending = false
    if (!assistantMessage.content && !assistantMessage.steps?.length) {
      assistantMessages.value = assistantMessages.value.filter((item) => item !== assistantMessage)
    }
    assistantError.value = error instanceof Error ? error.message : 'AI 配置助手请求失败'
  } finally {
    if (assistantAbort === controller) assistantAbort = null
    assistantBusy.value = false
    await scrollAssistantToBottom()
  }
}

function closeAssistant() {
  if (assistantBusy.value || saving.value) return
  assistantAbort?.abort()
  assistantAbort = null
  assistantShow.value = false
  assistantMessages.value = []
  assistantImages.value = []
  assistantReadyConfig.value = null
  assistantChanges.value = []
  assistantSaved.value = false
}

function openFieldAssistant(field: 'rules' | 'review_prompt') {
  if (!hasGeneralProvider.value) {
    message.warning('需要先在模型接入中配置并启用通用大模型')
    return
  }
  if (!workspace.currentProjectId) {
    message.warning('请先选择项目')
    return
  }
  fieldAssistantField.value = field
  fieldAssistantCurrent.value = field === 'rules' ? form.rulesText : form.reviewPrompt
  fieldAssistantProviderId.value = assistantProviderId.value || assistantProviderOptions.value[0]?.value || null
  fieldAssistantContext.value = {
    name: form.name,
    description: form.description,
    decision_mode: form.decisionMode,
    media_types: form.mediaTypes,
    categories: form.categories.map((category) => ({
      code: category.code,
      name: category.name,
      result: category.result,
    })),
  }
  fieldAssistantShow.value = true
}

function applyFieldAssistant(text: string) {
  if (fieldAssistantField.value === 'rules') {
    form.rulesText = text
    form.rulesPreset = 'custom'
    message.success('AI 规则已应用，保存审核配置后生效')
    return
  }
  form.reviewPrompt = text
  message.success('AI 优化后的审核要求已应用，保存审核配置后生效')
}

async function setAuditStatus(audit: Audit, enabled: boolean) {
  if (statusUpdatingId.value !== null) return
  const next = enabled ? 'ACTIVE' : 'PAUSED'
  if (audit.status === next) return
  statusUpdatingId.value = audit.id
  try {
    await auditApi.setStatus(audit.id, next)
    message.success(enabled ? '审核已启用' : '审核已暂停')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
  } finally {
    statusUpdatingId.value = null
  }
}

function remove(audit: Audit) {
  dialog.warning({
    title: '删除审核配置',
    content: `「${audit.name}」将进入回收站，相关触发源会暂停接收任务。确认删除？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await auditApi.remove(audit.id)
        message.success('审核配置已移入回收站')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      }
    },
  })
}

function openClone(audit: Audit) {
  cloneTarget.value = audit
  cloneForm.name = `${audit.name} 副本`
  cloneForm.code = `${audit.code}_copy`
  cloneShow.value = true
}

async function submitClone() {
  if (!cloneTarget.value) return
  if (!cloneForm.name.trim() || !/^[a-z][a-z0-9_]{2,31}$/.test(cloneForm.code)) {
    message.warning('请填写名称，编码需为小写字母开头且仅含小写字母、数字、下划线')
    return
  }
  saving.value = true
  try {
    await auditApi.clone(cloneTarget.value.id, { name: cloneForm.name.trim(), code: cloneForm.code })
    message.success('审核配置已复制为草稿')
    cloneShow.value = false
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '复制失败')
  } finally {
    saving.value = false
  }
}

function openSubmit(audit: Audit) {
  taskAuditId.value = audit.id
  taskModalShow.value = true
}

function openGuide(audit: Audit) {
  guideAuditId.value = audit.id
  guideShow.value = true
}

function openTriggerManage(trigger: TriggerSource) {
  activeTrigger.value = trigger
  triggerModalShow.value = true
}

async function reloadDetail() {
  if (!detailAudit.value) return
  try {
    const data = await auditApi.get(detailAudit.value.id)
    detailAudit.value = data.audit
    detailTriggers.value = data.triggers
    const updated = data.triggers.find((item) => item.id === activeTrigger.value?.id)
    if (updated) activeTrigger.value = updated
  } catch (error) {
    message.error(error instanceof Error ? error.message : '刷新审核详情失败')
  }
}

async function openDetail(audit: Audit) {
  detailAudit.value = audit
  detailTriggers.value = []
  detailShow.value = true
  try {
    const data = await auditApi.get(audit.id)
    detailAudit.value = data.audit
    detailTriggers.value = data.triggers
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载详情失败')
  }
}

function mediaLabel(type: string) {
  return type === 'text' ? '文本' : type === 'image' ? '图片' : type === 'mixed' ? '混合' : type
}

function decisionModeLabel(audit: Audit) {
  if (audit.decision_mode === 'CLASSIFY_ONLY') return `纯分类器 · ${(audit.categories || []).length} 个类型 · 全部 PASS`
  return audit.decision_mode === 'CLASSIFY' ? `分类审核 · ${(audit.categories || []).length} 类` : '合规审核'
}

function categoryResultLabel(result: string) {
  return { PASS: '通过', REJECT: '驳回', REVIEW: '转人工' }[result] || result
}

function triggerLabel(type: string) {
  return { OPENAPI: '开放 API', REDIS: 'Redis', WEBSOCKET: 'WebSocket' }[type] || type
}

watch(() => workspace.currentProjectId, load)
onMounted(async () => {
  try {
    const meta = await metaApi.ruleTemplates()
    templates.value = meta.templates
    if (meta.default_classify_prompt) defaultClassifyPrompt.value = meta.default_classify_prompt
    if (meta.example_categories?.length) {
      exampleCategories.value = meta.example_categories
      if (!editing.value && form.decisionMode === 'REVIEW') form.categories = []
    }
    if (meta.default_review_prompt) {
      form.reviewPrompt = defaultPrompt
    }
  } catch {
    // optional metadata
  }
  await load()
})
</script>

<template>
  <div class="page audits-page">
    <PageHeader
      eyebrow="AUDIT FLOWS"
      title="审核配置"
      subtitle="把媒体类型、模型、审核要求与规则阈值编排成可复用的审核流程。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" title="刷新" aria-label="刷新" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button
          v-if="auth.can('audit:write')"
          secondary
          :disabled="!hasGeneralProvider || !audits.length"
          :title="hasGeneralProvider ? '选择一条配置，让 AI 在现有配置上修改' : '请先在模型接入中配置并启用通用大模型'"
          @click="openAssistantEditPicker"
        >
          <template #icon><CreateOutline /></template>
          AI 编辑配置
        </n-button>
        <n-button
          v-if="auth.can('audit:write')"
          secondary
          :disabled="!hasGeneralProvider"
          :title="hasGeneralProvider ? '通过对话生成审核配置' : '请先在模型接入中配置并启用通用大模型'"
          @click="startAssistant('create')"
        >
          <template #icon><SparklesOutline /></template>
          AI 创建配置
        </n-button>
        <n-button v-if="auth.can('audit:write')" type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新建审核配置
        </n-button>
      </template>
    </PageHeader>

    <div class="audit-summary">
      <div>
        <span>已启用流程</span>
        <strong :class="{ 'summary-loading': !summaryLoaded }">{{ summaryLoaded ? activeCount : '—' }}</strong>
      </div>
      <div class="summary-divider"></div>
      <div>
        <span>分类流程</span>
        <strong :class="{ 'summary-loading': !summaryLoaded }">{{ summaryLoaded ? classifyCount : '—' }}</strong>
      </div>
      <div class="summary-divider"></div>
      <div>
        <span>今日执行任务</span>
        <strong :class="{ 'summary-loading': !summaryLoaded }">{{ summaryLoaded ? todayCount : '—' }}</strong>
      </div>
      <div class="summary-divider"></div>
      <div>
        <span>可用模型</span>
        <strong>{{ providers.filter((p) => p.status === 'ACTIVE').length }}</strong>
      </div>
      <p class="summary-hint">
        <ShieldCheckmarkOutline />
        合规审核按风险阈值判定；分类审核按类别映射 + 置信度转人工
      </p>
    </div>

    <div class="audit-toolbar">
      <n-input v-model:value="search" placeholder="搜索审核名称或编码" clearable class="search-input">
        <template #prefix><SearchOutline /></template>
      </n-input>
      <n-select
        v-model:value="statusFilter"
        :options="statusFilterOptions"
        clearable
        placeholder="状态"
        style="width: 130px"
      />
      <n-select
        v-model:value="decisionFilter"
        :options="decisionFilterOptions"
        clearable
        placeholder="审核类型"
        style="width: 135px"
      />
      <span class="result-count">已显示 {{ filtered.length }} / 共 {{ auditTotal }} 条</span>
    </div>

    <div v-if="filtered.length" class="audit-list fade-list">
      <article v-for="audit in filtered" :key="audit.id" class="audit-card">
        <div class="audit-card-head">
          <div class="audit-side" :class="{ active: audit.status === 'ACTIVE' }">
            <ShieldCheckmarkOutline />
          </div>
          <div class="audit-heading">
            <h3 class="audit-name" :title="audit.name">{{ audit.name }}</h3>
            <div class="audit-badges">
              <span class="audit-code mono">{{ audit.code }}</span>
              <StatusTag :status="audit.status" type="audit" />
              <StatusTag v-if="audit.provider_kind" :status="audit.provider_kind" type="kind" />
              <span v-if="audit.decision_mode === 'CLASSIFY' || audit.decision_mode === 'CLASSIFY_ONLY'" class="audit-mode-badge" :class="{ classifier: audit.decision_mode === 'CLASSIFY_ONLY' }">
                <SparklesOutline />
                {{ audit.decision_mode === 'CLASSIFY_ONLY' ? '纯分类器' : '分类审核' }} · {{ (audit.categories || []).length }} {{ audit.decision_mode === 'CLASSIFY_ONLY' ? '个类型' : '类' }}
              </span>
            </div>
          </div>
          <n-switch
            class="audit-status-switch"
            :value="audit.status === 'ACTIVE'"
            size="small"
            :loading="statusUpdatingId === audit.id"
            :disabled="!auth.can('audit:write') || (statusUpdatingId !== null && statusUpdatingId !== audit.id)"
            :title="audit.status === 'ACTIVE' ? '点击暂停审核' : '点击启用审核'"
            :aria-label="audit.status === 'ACTIVE' ? '暂停审核' : '启用审核'"
            @update:value="setAuditStatus(audit, Boolean($event))"
          />
        </div>
        <p class="audit-desc" :title="audit.description || ''">{{ audit.description || '暂无说明' }}</p>
        <div class="audit-meta">
          <span class="meta-item">
            <CloudOutline />
            {{ audit.provider_name || '未绑定模型' }}
          </span>
          <span class="meta-item">
            <TextOutline />
            {{ (audit.media_types || []).map(mediaLabel).join(' / ') }}
          </span>
          <span class="meta-item">
            <FlashOutline />
            今日 {{ audit.today_tasks || 0 }} 条
          </span>
          <span class="meta-item">
            <CopyOutline />
            v{{ audit.version }}
          </span>
          <span class="meta-item">
            去重 {{ audit.dedupe_window_sec ? `${Math.round(audit.dedupe_window_sec / 3600)}h` : '关闭' }}
          </span>
          <span v-if="audit.review_mode === 'EXTERNAL'" class="meta-item" title="REVIEW 任务不进本平台复核台，由结果动作通知业务系统复核">外部复核</span>
        </div>
        <div class="audit-trigger-row">
          <span class="trigger-row-label">已开启接入</span>
          <span v-for="type in audit.active_triggers || []" :key="type" class="trigger-chip">{{ triggerLabel(type) }}</span>
          <span v-if="!(audit.active_triggers || []).length" class="trigger-chip muted">暂无</span>
        </div>
        <div class="audit-prompt" :title="audit.review_prompt">{{ audit.review_prompt }}</div>
        <div class="audit-actions">
          <n-button v-if="auth.can('task:submit')" class="audit-run" size="small" type="primary" :disabled="audit.status !== 'ACTIVE'" @click="openSubmit(audit)">
            <template #icon><FlashOutline /></template>
            发起审核
          </n-button>
          <div class="audit-action-row">
            <n-button size="small" secondary @click="openGuide(audit)">接入指南</n-button>
            <n-button size="small" quaternary @click="openExport(audit)">
              <template #icon><DownloadOutline /></template>
              导出记录
            </n-button>
            <n-dropdown trigger="click" :options="auditMoreOptions(audit)" @select="(key) => handleAuditMore(String(key), audit)">
              <n-button size="small" quaternary>
                <template #icon><EllipsisHorizontalOutline /></template>
                更多
              </n-button>
            </n-dropdown>
          </div>
        </div>
      </article>
    </div>

    <EmptyState
      v-else
      title="还没有审核配置"
      description="创建一条审核流程，将会审核要求、模型供应商和规则阈值组合起来。"
      action="新建审核配置"
      @action="openCreate"
    >
      <template #icon><ShieldCheckmarkOutline /></template>
    </EmptyState>

    <div v-if="hasMoreAudits" class="load-more-row">
      <n-button secondary :loading="loadingMore" @click="loadMoreAudits">
        加载更多（已显示 {{ audits.length }} / 共 {{ auditTotal }} 条）
      </n-button>
    </div>

    <n-modal :show="modalShow" :mask-closable="!saving" @update:show="handleModalShowChange">
      <div class="audit-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT AUDIT FLOW' : 'NEW AUDIT FLOW' }}</div>
            <h2>{{ editing ? '编辑审核配置' : '新建审核配置' }}</h2>
            <p>配置会立即进入版本管理，启用后可被任务中心和开放 API 使用。</p>
          </div>
          <div class="modal-head-actions">
            <span class="modal-icon"><ShieldCheckmarkOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="requestCloseModal">×</button>
          </div>
        </div>
        <div class="modal-body">
          <div class="two-col">
            <div class="field">
              <label>审核名称 <span class="required">*</span></label>
              <n-input v-model:value="form.name" placeholder="例如 工程档案智能审核" />
            </div>
            <div class="field">
              <label>审核编码 <span v-if="!editing" class="required">*</span></label>
              <n-input v-model:value="form.code" :disabled="Boolean(editing)" placeholder="archive_review" class="mono-input" />
            </div>
          </div>
          <div class="field">
            <label>说明</label>
            <n-input v-model:value="form.description" placeholder="这条审核流程适用的业务场景" maxlength="255" />
          </div>
          <div class="mode-picker" :class="{ classify: isClassifyMode, classifier: form.decisionMode === 'CLASSIFY_ONLY' }">
            <div>
              <strong><SparklesOutline /> 审核类型</strong>
              <span v-if="form.decisionMode === 'CLASSIFY_ONLY'">模型只返回分类类型编码；成功审核一律 PASS，类型写入结果的 category 字段，不做驳回判定。</span>
              <span v-else-if="form.decisionMode === 'CLASSIFY'">模型先选分类，系统再按分类映射终审结论，并支持分类置信度转人工。</span>
              <span v-else>模型直接给出 PASS / REJECT / REVIEW，按风险阈值和规则组判定。</span>
            </div>
            <n-select
              :value="form.decisionMode"
              :options="decisionFormOptions"
              size="small"
              style="width: 280px"
              @update:value="setDecisionMode"
            />
          </div>
          <div class="two-col">
            <div class="field">
              <label>模型供应商 <span class="required">*</span></label>
              <n-select v-model:value="form.providerId" :options="providerOptions" placeholder="选择用于审核的模型" />
              <div v-if="providers.every((p) => p.status !== 'ACTIVE')" class="form-hint warn">
                当前项目没有已启用的模型，请先前往「模型接入」配置。
              </div>
              <div v-else-if="isClassifyMode && selectedProvider?.kind !== 'JEV'" class="form-hint warn classify-recommend">
                <span>{{ form.decisionMode === 'CLASSIFY_ONLY' ? '纯分类器建议使用 JEV' : '分类审核建议使用 JEV' }}：原生 choice 分类、概率可解释。</span>
                <n-button v-if="jevProviders.length" text size="tiny" type="primary" @click="useJevProvider">切换到 JEV</n-button>
              </div>
              <div v-else-if="isClassifyMode" class="form-hint good">
                已选择 JEV：会通过 System One choice 输出类型编码与置信度。
              </div>
            </div>
            <div class="field">
              <label>媒体类型</label>
              <n-select
                v-model:value="form.mediaTypes"
                multiple
                :options="[
                  { label: '文本 text', value: 'text' },
                  { label: '图片 image', value: 'image' },
                ]"
                placeholder="选择媒体类型"
              />
            </div>
          </div>
          <div class="config-sections">
            <div class="config-sections-head">
              <strong>详细配置</strong>
              <span>按需打开独立弹窗编辑，主表单只保留摘要</span>
            </div>
            <div class="config-section-row">
              <div class="config-section-main">
                <div class="config-section-title">审核要求 <span>发送给模型的审核 Prompt</span></div>
                <p class="config-section-text">{{ promptSummary }}</p>
              </div>
              <n-button size="small" secondary aria-label="编辑审核要求" @click="promptModalShow = true">编辑</n-button>
            </div>
            <div v-if="isClassifyMode" class="config-section-row">
              <div class="config-section-main">
                <div class="config-section-title">分类类型 <span>{{ categorySummary.count }} 个</span></div>
                <div v-if="categorySummary.items.length" class="config-section-tags">
                  <n-tag v-for="(item, index) in categorySummary.items" :key="index" size="small" :bordered="false">
                    {{ item.code }}<template v-if="form.decisionMode === 'CLASSIFY'"> → {{ item.result }}</template>
                  </n-tag>
                  <span v-if="categorySummary.more" class="config-section-more">+{{ categorySummary.more }}</span>
                </div>
                <p v-else class="config-section-text">尚未配置分类类型</p>
              </div>
              <n-button size="small" secondary aria-label="编辑分类类型" @click="categoryModalShow = true">编辑</n-button>
            </div>
            <div class="config-section-row">
              <div class="config-section-main">
                <div class="config-section-title">
                  规则组
                  <span>{{ ruleSummary.groups }} 组 · {{ ruleSummary.conditions }} 个条件</span>
                </div>
                <p class="config-section-text" :class="{ 'config-section-error': ruleSummary.invalid }">{{ ruleSummary.preview }}</p>
              </div>
              <n-button size="small" secondary aria-label="编辑规则组" @click="rulesModalShow = true">编辑</n-button>
            </div>
            <div class="config-section-row">
              <div class="config-section-main">
                <div class="config-section-title">结果动作回调 <span>{{ routeSummary.total }} 个动作</span></div>
                <p class="config-section-text" :class="{ 'config-section-error': routeSummary.invalid }">{{ routeSummary.preview }}</p>
              </div>
              <n-button size="small" secondary aria-label="编辑结果动作" @click="routeModalShow = true">编辑</n-button>
            </div>
            <div class="config-section-row">
              <div class="config-section-main">
                <div class="config-section-title">运行策略 <span>默认结论 / 失败兜底 / 复核方式 / 去重</span></div>
                <p class="config-section-text">{{ policySummary }}</p>
              </div>
              <n-button size="small" secondary aria-label="编辑运行策略" @click="policyModalShow = true">编辑</n-button>
            </div>
          </div>
          <div class="create-hint">
            <ShieldCheckmarkOutline />
            <span v-if="editing">保存后可在「接入指南」中查看当前三种触发方式、接口参数和回调配置。</span>
            <span v-else>创建后会自动生成 开放 API / Redis / WebSocket 三种接入方式，并立即打开接入指南。Redis 需要先在「数据连接」配置并测试连接。</span>
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button v-if="editing && auth.can('audit:write')" secondary :disabled="!hasGeneralProvider" @click="startAssistant('edit', editing)">
            <template #icon><SparklesOutline /></template>
            AI 帮我修改
          </n-button>
          <n-button :disabled="saving" @click="requestCloseModal">取消</n-button>
          <n-button v-if="auth.can('audit:write')" type="primary" :loading="saving" @click="save()">{{ editing ? '保存修改' : '创建并查看接入方式' }}</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="promptModalShow">
      <div class="config-submodal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">REVIEW PROMPT</div>
            <h2>审核要求</h2>
            <p>这段要求会作为审核 Prompt 发送给通用模型，并作为 JEV 的 state 审核要求。</p>
          </div>
          <div class="modal-head-actions">
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="promptModalShow = false">×</button>
          </div>
        </div>
        <div class="submodal-body">
          <div class="field">
            <div class="field-label-row">
              <label>审核要求 Prompt</label>
              <n-button
                size="tiny"
                secondary
                :disabled="!hasGeneralProvider"
                title="根据你的描述优化当前审核要求"
                @click="openFieldAssistant('review_prompt')"
              >
                <template #icon><SparklesOutline /></template>
                AI 优化提示词
              </n-button>
            </div>
            <n-input
              v-model:value="form.reviewPrompt"
              type="textarea"
              :autosize="{ minRows: 12, maxRows: 24 }"
              placeholder="告诉模型该审核什么、什么情况通过/驳回/转人工"
            />
            <div class="form-hint">这段要求会作为审核 Prompt 发送给通用模型，并作为 JEV 的 state 审核要求。</div>
          </div>
        </div>
        <div class="modal-footer">
          <n-button type="primary" @click="promptModalShow = false">完成</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="categoryModalShow">
      <div class="config-submodal wide">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">CLASSIFICATION</div>
            <h2>分类类型</h2>
            <p>调整分类编码、判定说明和映射结论；导入导出不会影响其他配置。</p>
          </div>
          <div class="modal-head-actions">
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="categoryModalShow = false">×</button>
          </div>
        </div>
        <div class="submodal-body">
          <div class="category-panel in-modal">
            <div class="category-head">
              <div>
                <strong>{{ form.decisionMode === 'CLASSIFY_ONLY' ? '可返回的分类类型' : '分类与终审结论映射' }}</strong>
                <span v-if="form.decisionMode === 'CLASSIFY_ONLY'">这里预设模型可以返回的类型；成功审核的终审结论固定 PASS，类型编码和名称会写入结果的 category / category_name。</span>
                <span v-else>分类编码会写入结果的 category 字段；每个分类的映射结论由规则引擎直接采用，低置信度通过自动转人工。</span>
              </div>
              <div class="category-actions">
                <n-button size="tiny" secondary @click="triggerCategoryImport">
                  <template #icon><CloudOutline /></template>
                  导入分类
                </n-button>
                <n-dropdown :options="categoryExportOptions" trigger="click" @select="onCategoryExport">
                  <n-button size="tiny" secondary>
                    <template #icon><DownloadOutline /></template>
                    导出分类
                  </n-button>
                </n-dropdown>
                <n-button size="tiny" secondary @click="loadExampleCategories">
                  <template #icon><SparklesOutline /></template>
                  载入示例
                </n-button>
                <input
                  ref="categoryImportInput"
                  type="file"
                  accept=".csv,.json,text/csv,application/json"
                  hidden
                  @change="onCategoryFilePicked"
                />
              </div>
            </div>
            <div class="category-grid-head">
              <span>分类编码</span><span>显示名称</span><span>判定说明</span><span>{{ form.decisionMode === 'CLASSIFY_ONLY' ? '审核结论' : '映射结论' }}</span><span></span>
            </div>
            <div class="category-list">
              <div v-for="(category, index) in form.categories" :key="index" class="category-row">
                <n-input v-model:value="category.code" size="small" placeholder="contract" class="mono-input" />
                <n-input v-model:value="category.name" size="small" placeholder="合同协议" />
                <n-input v-model:value="category.description" size="small" placeholder="判定依据与典型特征" />
                <span v-if="form.decisionMode === 'CLASSIFY_ONLY'" class="fixed-pass">固定 PASS</span>
                <n-select v-else v-model:value="category.result" size="small" :options="categoryResultOptions" />
                <n-button
                  quaternary
                  circle
                  size="tiny"
                  :disabled="form.categories.length <= (form.decisionMode === 'CLASSIFY_ONLY' ? 1 : 2)"
                  @click="removeCategory(index)"
                >
                  <template #icon><TrashOutline /></template>
                </n-button>
              </div>
            </div>
            <n-button dashed size="small" block @click="addCategory">
              <template #icon><AddOutline /></template>
              添加分类
            </n-button>
            <div class="form-hint">支持 CSV（code,name,description,result）和 JSON（分类数组）导入；纯分类器会忽略 result 并统一使用 PASS。</div>
          </div>
        </div>
        <div class="modal-footer">
          <n-button type="primary" @click="categoryModalShow = false">完成</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="rulesModalShow">
      <div class="config-submodal wide rules-editor-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">RULE ENGINE</div>
            <h2>规则组</h2>
            <p>规则组按优先级执行；未命中时使用默认结论，命中后可选是否停止后续规则。</p>
          </div>
          <div class="modal-head-actions">
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="rulesModalShow = false">×</button>
          </div>
        </div>
        <div class="submodal-body">
          <div class="rule-editor-toolbar">
            <div class="rule-template-pick">
              <span>规则模板</span>
              <n-select
                :value="form.rulesPreset"
                :options="templateOptions"
                size="small"
                style="width: 240px"
                @update:value="applyRulesPreset"
              />
            </div>
            <n-button
              size="small"
              secondary
              :disabled="!hasGeneralProvider"
              title="根据需求生成可执行的规则"
              @click="openFieldAssistant('rules')"
            >
              <template #icon><SparklesOutline /></template>
              AI 生成规则
            </n-button>
          </div>
          <p class="rule-editor-desc">
            <span v-if="selectedTemplate">{{ selectedTemplate.description }}</span>
            <span v-else-if="form.rulesPreset === 'keep'">保持当前已有规则，不会覆盖自定义条件。</span>
            <span v-else-if="form.rulesPreset === 'custom'">当前为自定义规则，可在下方直接增删规则组和条件。</span>
            <span v-else>选择一个模板快速生成规则，也可以手动添加规则组。</span>
          </p>
          <RuleGroupsEditor
            v-model="form.rulesText"
            @change="form.rulesPreset = 'custom'"
          />
        </div>
        <div class="modal-footer">
          <n-button type="primary" @click="rulesModalShow = false">完成</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="routeModalShow">
      <div class="config-submodal routes">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">RESULT ACTIONS</div>
            <h2>结果动作回调</h2>
            <p>按最终结论向业务系统推送结果；未配置动作时只写入平台结果。</p>
          </div>
          <div class="modal-head-actions">
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="routeModalShow = false">×</button>
          </div>
        </div>
        <div class="submodal-body">
          <ResultRoutesEditor v-model="form.routesText" :audit-id="editing?.id || 0" />
        </div>
        <div class="modal-footer">
          <n-button type="primary" @click="routeModalShow = false">完成</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="policyModalShow">
      <div class="config-submodal compact">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">POLICY</div>
            <h2>运行策略</h2>
            <p>控制默认结论、模型失败兜底、人工复核方式和任务去重窗口。</p>
          </div>
          <div class="modal-head-actions">
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="policyModalShow = false">×</button>
          </div>
        </div>
        <div class="submodal-body">
          <div class="two-col">
            <div class="field">
              <label>默认结论</label>
              <n-select
                v-model:value="form.defaultResult"
                :disabled="form.decisionMode === 'CLASSIFY_ONLY'"
                :options="[
                  { label: '待议 REVIEW', value: 'REVIEW' },
                  { label: '通过 PASS', value: 'PASS' },
                  { label: '不通过 REJECT', value: 'REJECT' },
                ]"
              />
              <div v-if="form.decisionMode === 'CLASSIFY_ONLY'" class="form-hint">纯分类器成功审核固定 PASS；模型失败仍按右侧兜底策略。</div>
            </div>
            <div class="field">
              <label>模型失败兜底</label>
              <n-select
                v-model:value="form.onModelFail"
                :options="[
                  { label: '待议 REVIEW', value: 'REVIEW' },
                  { label: '通过 PASS', value: 'PASS' },
                  { label: '不通过 REJECT', value: 'REJECT' },
                ]"
              />
            </div>
            <div class="field">
              <label>人工复核方式</label>
              <n-select
                v-model:value="form.reviewMode"
                :options="[
                  { label: '平台内复核 · 待议任务进本平台复核台', value: 'INTERNAL' },
                  { label: '外部系统复核 · 不进复核台，按结果动作通知业务方', value: 'EXTERNAL' },
                ]"
              />
              <div v-if="form.reviewMode === 'EXTERNAL'" class="form-hint">
                REVIEW 结论的任务直接完成，不进入本平台待议复核队列；请在「结果动作回调」中配置通知，由业务系统自行人工复核。
              </div>
            </div>
            <div class="field">
              <label>去重窗口（秒，0 表示不去重）</label>
              <n-input-number v-model:value="form.dedupeWindowSec" :min="0" :max="604800" :step="3600" style="width: 100%" />
              <div class="form-hint">当前：{{ formatDedupe(form.dedupeWindowSec) }}</div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <n-button type="primary" @click="policyModalShow = false">完成</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="assistantPickShow">
      <div class="clone-modal assistant-pick-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">AI EDIT TARGET</div>
            <h2>选择要编辑的审核配置</h2>
            <p>AI 会先读取这条配置的当前模型、分类、规则和 Prompt，再确认要改成什么。</p>
          </div>
          <div class="modal-head-actions">
            <span class="modal-icon"><CreateOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="assistantPickShow = false">×</button>
          </div>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>编辑目标 <span class="required">*</span></label>
            <div class="assistant-pick-list">
              <button
                v-for="audit in audits"
                :key="audit.id"
                type="button"
                class="assistant-pick-item"
                :class="{ active: assistantPickId === audit.id }"
                @click="assistantPickId = audit.id"
              >
                <span class="assistant-pick-main">
                  <strong>{{ audit.name }}</strong>
                  <code>{{ audit.code }}</code>
                </span>
                <span class="assistant-pick-desc">{{ describeAudit(audit) }}</span>
                <span class="assistant-pick-provider">{{ audit.provider_name || '未绑定模型' }}</span>
              </button>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <n-button @click="assistantPickShow = false">取消</n-button>
          <n-button type="primary" :disabled="!assistantPickId" @click="confirmAssistantEditPick">开始修改</n-button>
        </div>
      </div>
    </n-modal>

    <AuditFieldAssistantModal
      v-model:show="fieldAssistantShow"
      :field="fieldAssistantField"
      :current="fieldAssistantCurrent"
      :context="fieldAssistantContext"
      :project-id="workspace.currentProjectId"
      :provider-options="assistantProviderOptions"
      :initial-provider-id="fieldAssistantProviderId"
      @apply="applyFieldAssistant"
    />

    <n-modal :show="assistantShow" :mask-closable="!assistantBusy && !saving" @update:show="(value: boolean) => { if (!value) closeAssistant() }">
      <div class="assistant-modal">
        <div class="modal-head assistant-head">
          <div class="assistant-heading">
            <span class="assistant-header-icon">
              <n-icon :component="SparklesOutline" />
            </span>
            <div>
              <div class="modal-kicker">AI CONFIG ASSISTANT</div>
              <h2>{{ assistantMode === 'edit' ? 'AI 帮我修改审核配置' : 'AI 帮我创建审核配置' }}</h2>
              <p>{{ assistantMode === 'edit' ? 'AI 会读取当前配置与接入方式，确认“现在是什么、要改成什么”后只修改你指定的部分。' : '通用大模型会先向你提问，信息足够后自动生成并保存配置，包括任务接入方式。' }}</p>
            </div>
          </div>
          <n-button quaternary circle title="关闭" aria-label="关闭" :disabled="assistantBusy || saving" @click="closeAssistant">
            <template #icon><n-icon :component="CloseOutline" /></template>
          </n-button>
        </div>
        <div v-if="assistantMode === 'edit' && editing" class="assistant-target">
          <span class="assistant-target-label">正在编辑</span>
          <strong>{{ editing.name }}</strong>
          <code>{{ editing.code }}</code>
          <span class="assistant-target-current">当前：{{ describeAudit(editing) }}</span>
        </div>
        <div ref="assistantChat" class="assistant-chat">
          <div
            v-for="(item, index) in assistantMessages"
            :key="index"
            class="assistant-message"
            :class="[item.role, { pending: item.pending }]"
          >
            <span class="assistant-avatar" :class="item.role">
              <n-icon :component="item.role === 'user' ? PersonOutline : SparklesOutline" />
            </span>
            <div v-if="item.role === 'user'" class="assistant-bubble">
              <div v-if="item.images?.length" class="assistant-message-images">
                <n-image v-for="(src, imageIndex) in item.images" :key="imageIndex" :src="src" width="88" />
              </div>
              <span v-if="item.content">{{ item.content }}</span>
            </div>
            <div v-else class="assistant-content">
              <div v-if="item.pending" class="assistant-progress">
                <div class="assistant-progress-head">
                  <n-spin :size="13" />
                  <span>{{ assistantStatusText(item) }}</span>
                </div>
                <div v-if="item.steps?.length" class="assistant-trace-list">
                  <div v-for="step in item.steps" :key="step.id" class="assistant-step" :class="step.status">
                    <span class="assistant-step-icon">
                      <n-spin v-if="step.status === 'running'" :size="11" />
                      <n-icon v-else-if="step.status === 'error'" :component="CloseCircleOutline" />
                      <n-icon v-else :component="CheckmarkCircleOutline" />
                    </span>
                    <span class="assistant-step-label">{{ step.label }}</span>
                    <span v-if="step.detail" class="assistant-step-detail">{{ step.detail }}</span>
                  </div>
                </div>
              </div>
              <div v-if="item.content" class="assistant-answer markdown-body" v-html="renderAssistantMarkdown(item.content)"></div>
              <div v-if="index === assistantMessages.length - 1 && assistantReadyConfig" class="assistant-ready">
                <strong>{{ assistantMode === 'edit' ? '已按你的要求更新并保存' : '配置已生成并创建' }}：{{ assistantReadyConfig.name }}</strong>
                <span>{{ auditModeLabel(assistantReadyConfig.decision_mode) }} · {{ assistantReadyConfig.categories.length }} 个分类 · {{ assistantReadyConfig.media_types.join(' + ') }}</span>
                <span v-if="assistantReadyConfig.triggers">接入方式：{{ assistantTriggerText(assistantReadyConfig.triggers) }}</span>
                <ul v-if="assistantMode === 'edit' && assistantChanges.length" class="assistant-changes">
                  <li v-for="change in assistantChanges" :key="change">{{ change }}</li>
                </ul>
                <span v-else-if="assistantMode === 'edit'">未检测到与当前配置的字段差异。</span>
              </div>
            </div>
          </div>
          <div v-if="showAssistantStarters" class="assistant-starters">
            <span class="assistant-starters-title">可以这样开始</span>
            <div class="assistant-starter-list">
              <button
                v-for="starter in assistantStarters"
                :key="starter"
                type="button"
                @click="useAssistantStarter(starter)"
              >
                <n-icon :component="FlashOutline" />
                {{ starter }}
              </button>
            </div>
          </div>
        </div>
        <div class="assistant-composer">
          <div v-if="assistantImages.length" class="assistant-attachments">
            <div v-for="image in assistantImages" :key="image.id" class="assistant-attachment">
              <n-image :src="image.dataUrl" width="64" />
              <button type="button" class="assistant-attachment-remove" title="移除图片" @click="removeAssistantImage(image.id)">
                <CloseCircleOutline />
              </button>
              <span class="assistant-attachment-name">{{ image.name }}</span>
            </div>
          </div>
          <n-input
            ref="assistantInputRef"
            v-model:value="assistantInput"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 5 }"
            :disabled="assistantBusy || saving"
            placeholder="回答 AI 的问题，或直接说「按你的建议生成」；支持粘贴截图，Enter 发送，Shift+Enter 换行"
            :input-props="{ onKeydown: handleAssistantKeydown, onPaste: handleAssistantPaste }"
            class="assistant-input"
          />
          <div class="assistant-composer-bar">
            <div class="assistant-model-row">
              <span class="assistant-model-label">生成模型</span>
              <n-select
                v-model:value="assistantProviderId"
                size="small"
                :options="assistantProviderOptions"
                :disabled="assistantBusy || saving"
                placement="top-start"
                placeholder="选择通用大模型"
                class="assistant-model-select"
              />
              <VoiceInputButton :disabled="assistantBusy || saving" @transcript="applyVoiceInput" />
            </div>
            <div class="assistant-actions">
              <span class="assistant-note">
                {{ assistantImages.length ? `已附加 ${assistantImages.length} 张图片（最多 4 张）` : assistantSaved && assistantMode === 'edit' ? '修改已保存；可以继续描述调整，或关闭窗口' : hasGeneralProvider ? '使用项目内启用的通用大模型，不会调用 JEV' : '需要先配置通用大模型' }}
              </span>
              <n-button :disabled="assistantBusy || saving" @click="closeAssistant">关闭</n-button>
              <n-button type="primary" :loading="assistantBusy || saving" :disabled="(!assistantInput.trim() && !assistantImages.length) || assistantBusy || saving" @click="sendAssistant">
                <template #icon><n-icon :component="SendOutline" /></template>
                发送
              </n-button>
            </div>
          </div>
          <div v-if="assistantError" class="modal-error">{{ assistantError }}</div>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="cloneShow">
      <div class="clone-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">CLONE FLOW</div>
            <h2>复制审核配置</h2>
            <p>所有规则、路由与提示词会被复制，新配置为草稿状态。</p>
          </div>
          <div class="modal-head-actions">
            <span class="modal-icon"><CopyOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="cloneShow = false">×</button>
          </div>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>新名称</label>
            <n-input v-model:value="cloneForm.name" />
          </div>
          <div class="field">
            <label>新编码</label>
            <n-input v-model:value="cloneForm.code" class="mono-input" />
          </div>
        </div>
        <div class="modal-footer">
          <n-button @click="cloneShow = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="submitClone">复制</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="exportShow">
      <div class="export-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">EXPORT AUDIT RECORDS</div>
            <h2>导出审核记录</h2>
            <p>仅导出当前审核配置的任务；不选条件时导出全部记录，最多 100000 行。</p>
          </div>
          <div class="modal-head-actions">
            <span class="modal-icon"><DownloadOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="exportShow = false">×</button>
          </div>
        </div>
        <div class="modal-body">
          <div v-if="exportTarget" class="export-target">
            <div>
              <strong>{{ exportTarget.name }}</strong>
              <span>{{ exportTarget.code }} · {{ decisionModeLabel(exportTarget) }}</span>
            </div>
            <span class="export-target-count">今日 {{ exportTarget.today_tasks || 0 }} 条</span>
          </div>
          <div class="field">
            <label>时间范围</label>
            <n-date-picker v-model:value="exportForm.range" type="daterange" clearable style="width: 100%" />
          </div>
          <div class="two-col">
            <div class="field">
              <label>任务状态（可多选）</label>
              <n-select v-model:value="exportForm.statuses" multiple clearable :options="taskStatusOptions" placeholder="全部状态" />
            </div>
            <div class="field">
              <label>审核结论（可多选）</label>
              <n-select v-model:value="exportForm.results" multiple clearable :options="resultOptions" placeholder="全部结论" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>触发来源（可多选）</label>
              <n-select v-model:value="exportForm.sources" multiple clearable :options="sourceOptions" placeholder="全部来源" />
            </div>
            <div class="field">
              <label>媒体类型（可多选）</label>
              <n-select v-model:value="exportForm.mediaTypes" multiple clearable :options="mediaFilterOptions" placeholder="全部媒体" />
            </div>
          </div>
          <div class="field">
            <label>任务号 / 业务键关键字</label>
            <n-input v-model:value="exportForm.keyword" clearable placeholder="可选，支持模糊匹配 task_no 或 biz_key" />
          </div>
          <div class="export-summary">
            <span>当前范围</span>
            <strong>{{ exportSummary }}</strong>
            <span v-if="exportForm.mediaTypes.length"> · {{ exportForm.mediaTypes.map(mediaLabel).join(' / ') }}</span>
          </div>
          <div class="export-note">
            CSV 采用 UTF-8 BOM，Excel / WPS 可直接打开；分类审核会额外导出分类编码、分类名称和分类置信度。
          </div>
        </div>
        <div class="modal-footer">
          <n-button @click="exportShow = false">取消</n-button>
          <n-button quaternary @click="resetExportFilters">重置条件</n-button>
          <n-button type="primary" :loading="exporting" @click="runExport">
            <template #icon><DownloadOutline /></template>
            按条件导出
          </n-button>
        </div>
      </div>
    </n-modal>

    <n-drawer v-model:show="detailShow" width="min(640px, 100vw)" placement="right">
      <n-drawer-content :native-scrollbar="false" closable title="审核配置详情">
        <template v-if="detailAudit">
          <div class="detail-hero">
            <div class="detail-icon"><ShieldCheckmarkOutline /></div>
            <div>
              <h3>{{ detailAudit.name }}</h3>
              <span class="mono">{{ detailAudit.code }} · v{{ detailAudit.version }}</span>
            </div>
            <div class="detail-tags">
              <StatusTag :status="detailAudit.status" type="audit" />
              <StatusTag v-if="detailAudit.provider_kind" :status="detailAudit.provider_kind" type="kind" />
            </div>
          </div>
          <div class="detail-kv">
            <div><span>审核类型</span><strong>{{ decisionModeLabel(detailAudit) }}</strong></div>
            <div><span>模型供应商</span><strong>{{ detailAudit.provider_name || '—' }}</strong></div>
            <div><span>媒体类型</span><strong>{{ (detailAudit.media_types || []).map(mediaLabel).join(' / ') }}</strong></div>
            <div><span>默认结论</span><strong>{{ detailAudit.default_result }}</strong></div>
            <div><span>模型失败兜底</span><strong>{{ detailAudit.on_model_fail }}</strong></div>
            <div><span>去重窗口</span><strong>{{ detailAudit.dedupe_window_sec }} 秒</strong></div>
            <div><span>今日任务</span><strong>{{ detailAudit.today_tasks }}</strong></div>
          </div>

          <h4 class="section-label">审核要求</h4>
          <div class="prompt-card">{{ detailAudit.review_prompt }}</div>

          <template v-if="detailAudit.decision_mode === 'CLASSIFY' || detailAudit.decision_mode === 'CLASSIFY_ONLY'">
            <h4 class="section-label">{{ detailAudit.decision_mode === 'CLASSIFY_ONLY' ? '分类类型（结论固定 PASS）' : '分类与结论映射' }}</h4>
            <div class="detail-categories">
              <div v-for="category in detailAudit.categories || []" :key="category.code" class="detail-category">
                <code>{{ category.code }}</code>
                <div>
                  <strong>{{ category.name }}</strong>
                  <span>{{ category.description || '未填写判定说明' }}</span>
                </div>
                <span class="category-result" :class="(detailAudit.decision_mode === 'CLASSIFY_ONLY' ? 'pass' : category.result.toLowerCase())">{{ detailAudit.decision_mode === 'CLASSIFY_ONLY' ? '通过 PASS' : categoryResultLabel(category.result) }}</span>
              </div>
            </div>
          </template>

          <h4 class="section-label">规则组（按优先级）</h4>
          <div class="detail-rules">
            <div v-for="(rule, i) in detailAudit.rules" :key="i" class="detail-rule">
              <div class="detail-rule-head">
                <span class="rule-index">{{ rule.priority }}</span>
                <strong>{{ rule.name }}</strong>
                <span class="rule-logic">{{ rule.logic }}</span>
                <span class="rule-output">{{ rule.on_hit_result }}</span>
              </div>
              <div class="detail-rule-items">
                <span v-for="(item, j) in rule.items" :key="j" class="rule-chip">
                  <code>{{ item.field }}</code> {{ item.operator }} {{ JSON.stringify(item.value) }}
                </span>
              </div>
            </div>
          </div>

          <h4 class="section-label">触发源</h4>
          <div v-if="detailTriggers.length" class="trigger-list">
            <div v-for="trigger in detailTriggers" :key="trigger.id" class="trigger-row">
              <span class="trigger-type">{{ trigger.type }}</span>
              <div class="trigger-info">
                <strong>{{ trigger.name }}</strong>
                <span>
                  优先级 {{ trigger.priority }}
                  <template v-if="trigger.last_consume_at"> · 最近消费 {{ new Date(trigger.last_consume_at).toLocaleString() }}</template>
                  <template v-if="trigger.last_error"> · <em class="trigger-error">{{ trigger.last_error }}</em></template>
                </span>
              </div>
              <StatusTag :status="trigger.status" type="provider" />
              <n-button size="tiny" secondary @click="openTriggerManage(trigger)">配置</n-button>
            </div>
          </div>
          <p v-else class="muted small">暂无触发源。</p>
        </template>
      </n-drawer-content>
    </n-drawer>

    <TaskSubmitModal
      v-model:show="taskModalShow"
      :project-id="workspace.currentProjectId"
      :default-audit-id="taskAuditId"
      @created="load"
    />
    <AuditIntegrationDrawer
      v-model:show="guideShow"
      :audit-id="guideAuditId"
      @updated="load"
    />
    <TriggerManageModal
      v-model:show="triggerModalShow"
      :trigger="activeTrigger"
      @updated="reloadDetail"
    />
    <ScriptLogsModal
      v-model:show="scriptLogsShow"
      :audit-id="scriptLogsAudit?.id || 0"
      :audit-name="scriptLogsAudit?.name || ''"
    />

    <n-modal :show="leaveConfirmShow" :mask-closable="false" :close-on-esc="false">
      <div class="leave-confirm-modal">
        <div class="leave-confirm-body">
          <span class="leave-confirm-icon"><WarningOutline /></span>
          <div>
            <h3>{{ leaveTitle }}</h3>
            <p>{{ leaveMessage }}</p>
            <p v-if="leaveTarget" class="leave-confirm-target">即将前往：{{ leaveTarget }}</p>
          </div>
        </div>
        <div class="leave-confirm-actions">
          <n-button @click="cancelLeaveConfirm">留在当前页</n-button>
          <n-button type="error" @click="acceptLeaveConfirm">{{ leaveConfirmText }}</n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.leave-confirm-modal {
  width: min(420px, calc(100vw - 40px));
  padding: 22px;
  border-radius: 14px;
  background: var(--card, #fff);
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.22);
}

.leave-confirm-body {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.leave-confirm-icon {
  display: inline-flex;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: rgba(240, 173, 78, 0.16);
  color: #d97706;
}

.leave-confirm-body h3 {
  margin: 2px 0 6px;
  font-size: 15px;
}

.leave-confirm-body p {
  margin: 0 0 4px;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--muted, #64748b);
}

.leave-confirm-target {
  color: var(--muted-light, #94a3b8);
}

.leave-confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}

.audit-summary {
  display: flex;
  align-items: center;
  gap: 22px;
  margin-bottom: 16px;
  padding: 15px 20px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.audit-summary > div:not(.summary-divider) {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.audit-summary > div:not(.summary-divider) > span {
  color: var(--muted-light);
  font-size: 10.5px;
}

.audit-summary strong {
  color: var(--ink);
  font-family: var(--mono);
  font-size: 18px;
  font-weight: 650;
}

.summary-divider {
  width: 1px;
  height: 20px;
  background: #eef1f6;
}

.summary-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 0 auto;
  color: var(--muted-light);
  font-size: 10px;
}

.summary-hint :deep(svg) {
  width: 13px;
  height: 13px;
}

.audit-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.search-input {
  width: min(310px, 100%);
}

.result-count {
  margin-left: auto;
  color: var(--muted-light);
  font-size: 11px;
}

.audit-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  align-items: stretch;
  gap: 14px;
}

.audit-card {
  display: flex;
  min-width: 0;
  flex-direction: column;
  padding: 18px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: #fff;
  box-shadow: var(--shadow-sm);
  transition: transform 0.18s ease, box-shadow 0.18s ease, border-color 0.18s ease;
}

.audit-card:hover {
  transform: translateY(-2px);
  border-color: var(--brand-border);
  box-shadow: var(--shadow-md);
}

.audit-card-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.audit-side {
  display: flex;
  width: 42px;
  height: 42px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: #f3f5f9;
  color: #94a3b8;
  flex: none;
}

.audit-side.active {
  background: linear-gradient(145deg, #edf4ff, #e1ebff);
  color: var(--brand);
}

.audit-side :deep(svg) {
  width: 19px;
  height: 19px;
}

.audit-heading {
  min-width: 0;
  flex: 1;
}

.audit-status-switch {
  flex: none;
  margin-top: 3px;
}

.audit-name {
  margin: 0;
  overflow: hidden;
  color: var(--ink);
  font-size: 15px;
  font-weight: 650;
  letter-spacing: -0.01em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audit-badges {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 7px;
}

.audit-badges :deep(.status-pill) {
  height: 20px;
  padding: 0 7px;
  gap: 4px;
  font-size: 10px;
}

.audit-badges :deep(.status-pill i) {
  width: 5px;
  height: 5px;
}

.audit-code {
  padding: 2px 6px;
  border-radius: 5px;
  background: var(--surface-soft);
  color: var(--muted-light);
  font-size: 9.5px;
}

.audit-desc {
  display: -webkit-box;
  min-height: 37px;
  margin: 12px 0 0;
  overflow: hidden;
  color: var(--muted);
  font-size: 11.5px;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.audit-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border-radius: 5px;
  background: #f8fafc;
  color: #758196;
  font-size: 9.5px;
}

.meta-item :deep(svg) {
  width: 11px;
  height: 11px;
}

.audit-trigger-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 9px;
}

.trigger-row-label {
  color: var(--muted-light);
  font-size: 8.5px;
}

.trigger-chip {
  padding: 2px 7px;
  border-radius: 5px;
  background: #f2f7ff;
  color: #3f69bb;
  font-size: 8.5px;
}

.trigger-chip.muted {
  background: #f5f7fa;
  color: var(--muted-light);
}

.audit-prompt {
  display: -webkit-box;
  margin: 10px 0 12px;
  padding: 8px 10px;
  overflow: hidden;
  border-radius: 8px;
  background: var(--surface-soft);
  color: #8b96a8;
  font-size: 10px;
  line-height: 1.55;
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.audit-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}

.audit-run {
  width: 100%;
}

.audit-action-row {
  display: flex;
  align-items: center;
  gap: 4px;
}

.audit-action-row :deep(.n-button) {
  min-width: 0;
  flex: 1;
}

.delete-btn {
  color: #b0bac8;
}

.delete-btn:hover {
  color: var(--red);
}

.audit-modal {
  width: min(700px, calc(100vw - 32px));
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.config-submodal {
  display: flex;
  width: min(760px, calc(100vw - 32px));
  max-height: min(88vh, 920px);
  flex-direction: column;
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.config-submodal.wide {
  width: min(1000px, calc(100vw - 32px));
}

.config-submodal.rules-editor-modal {
  height: min(620px, calc(100vh - 40px));
}

.config-submodal.rules-editor-modal .submodal-body {
  scrollbar-gutter: stable;
}

.config-submodal.routes {
  width: min(960px, calc(100vw - 32px));
}

.config-submodal.compact {
  width: min(680px, calc(100vw - 32px));
}

.config-submodal .modal-head,
.config-submodal .modal-footer {
  flex: none;
}

.submodal-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 18px 24px 20px;
}

.submodal-body .field:last-child {
  margin-bottom: 0;
}

.rule-template.in-modal,
.category-panel.in-modal {
  margin-bottom: 0;
}

.rule-editor-toolbar {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 6px;
}

.rule-template-pick {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rule-template-pick > span {
  color: var(--muted);
  font-size: 10px;
  white-space: nowrap;
}

.rule-editor-desc {
  margin: 0 0 10px;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.55;
}

.config-sections {
  overflow: hidden;
  margin: 2px 0 16px;
  border: 1px solid #edf1f7;
  border-radius: 12px;
  background: #fbfcfe;
}

.config-sections-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 13px;
  border-bottom: 1px solid #edf1f7;
  background: #f8fafd;
}

.config-sections-head strong {
  color: var(--text);
  font-size: 11px;
  font-weight: 620;
}

.config-sections-head span {
  color: var(--muted-light);
  font-size: 9.5px;
}

.config-section-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 13px;
  border-bottom: 1px dashed #edf1f7;
}

.config-section-row:last-child {
  border-bottom: 0;
}

.config-section-main {
  min-width: 0;
  flex: 1;
}

.config-section-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  color: var(--text);
  font-size: 11px;
  font-weight: 600;
}

.config-section-title span {
  color: var(--muted-light);
  font-size: 9.5px;
  font-weight: 400;
}

.config-section-text {
  display: -webkit-box;
  overflow: hidden;
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.55;
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.config-section-error {
  color: #d03050 !important;
}

.config-section-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}

.config-section-tags :deep(.n-tag) {
  font-size: 9px;
}

.config-section-more {
  align-self: center;
  color: var(--muted-light);
  font-size: 9px;
}

.config-section-row :deep(.n-button) {
  flex: none;
}

@media (max-width: 720px) {
  .config-sections-head {
    align-items: flex-start;
    flex-direction: column;
    gap: 3px;
  }

  .config-section-row {
    align-items: flex-start;
  }

  .config-submodal {
    width: calc(100vw - 20px);
    max-height: calc(100vh - 20px);
  }

  .submodal-body {
    padding: 15px 18px 18px;
  }

  .config-submodal .modal-head {
    padding: 18px;
  }

  .config-submodal .modal-footer {
    padding: 12px 18px 16px;
  }
}

.clone-modal {
  width: min(450px, calc(100vw - 32px));
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  padding: 23px 26px 17px;
  border-bottom: 1px solid #f0f3f8;
  background: linear-gradient(130deg, #f8fbff, #fff);
  border-radius: 18px 18px 0 0;
}

.modal-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.13em;
}

.modal-head h2 {
  margin: 4px 0 3px;
  color: var(--ink);
  font-size: 20px;
  font-weight: 660;
  letter-spacing: -0.025em;
}

.modal-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11.5px;
}

.modal-icon {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--brand);
  color: #fff;
  box-shadow: 0 8px 18px -7px rgba(43, 108, 246, 0.55);
  flex-shrink: 0;
}

.modal-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.modal-body {
  max-height: min(72vh, 700px);
  overflow-y: auto;
  padding: 20px 26px;
}

.field {
  margin-bottom: 16px;
}

.field label {
  display: block;
  margin-bottom: 6px;
  color: var(--text);
  font-size: 12px;
  font-weight: 550;
}

.field-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 6px;
}

.field-label-row label {
  margin-bottom: 0;
}

.required {
  color: var(--red);
}

.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.three-col {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 14px;
}

.mono-input :deep(input) {
  font-family: var(--mono);
  font-size: 12px;
}

.form-hint.warn {
  color: #b45309;
}

.rule-template {
  margin-bottom: 16px;
  padding: 13px 14px;
  border: 1px solid #edf1f7;
  border-radius: 11px;
  background: #fbfcfe;
}

.rule-template-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}

.rule-template-head strong {
  display: block;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 550;
}

.rule-template-head span {
  display: block;
  color: var(--muted-light);
  font-size: 9.5px;
}

.rule-head-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 8px;
}

.template-desc {
  margin-top: 8px;
  color: var(--muted);
  font-size: 10px;
}

.json-editor-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 12px;
  margin-top: 12px;
}

.json-editor-grid .field label span {
  margin-left: 6px;
  color: var(--muted-light);
  font-size: 9px;
  font-weight: 400;
}

.code-input :deep(textarea) {
  font-family: var(--mono);
  font-size: 10.5px;
  line-height: 1.55;
}

@media (max-width: 900px) {
  .json-editor-grid {
    grid-template-columns: 1fr;
  }

  .rule-template-head {
    flex-wrap: wrap;
  }

  .rule-head-actions {
    width: 100%;
    justify-content: flex-end;
  }
}

.create-hint {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 9px 11px;
  border-radius: 8px;
  background: #f8fafc;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.6;
}

.create-hint :deep(svg) {
  width: 13px;
  height: 13px;
  margin-top: 1px;
  color: var(--brand);
  flex-shrink: 0;
}

.modal-error {
  margin-top: 3px;
  padding: 9px 11px;
  border: 1px solid #fee2e2;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 11px;
}

.assistant-modal {
  display: flex;
  width: min(720px, calc(100vw - 30px));
  max-height: min(820px, calc(100vh - 36px));
  flex-direction: column;
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.assistant-head {
  flex-shrink: 0;
}

.assistant-target {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
  padding: 9px 20px;
  border-bottom: 1px solid #e8eef7;
  background: #f6f9ff;
}

.assistant-target-label {
  padding: 2px 6px;
  border-radius: 4px;
  background: #e3ecff;
  color: #2854a8;
  font-size: 8.5px;
}

.assistant-target strong {
  color: var(--text);
  font-size: 11px;
}

.assistant-target code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.assistant-target-current {
  width: 100%;
  color: var(--muted);
  font-size: 9.5px;
}

.assistant-pick-modal {
  width: min(560px, calc(100vw - 30px));
}

.assistant-pick-list {
  display: flex;
  flex-direction: column;
  gap: 7px;
  max-height: 320px;
  overflow-y: auto;
}

.assistant-pick-item {
  display: flex;
  width: 100%;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid #e6ecf6;
  border-radius: 9px;
  background: #fff;
  color: var(--text);
  cursor: pointer;
  text-align: left;
}

.assistant-pick-item:hover {
  border-color: #c9daf7;
  background: #f8fbff;
}

.assistant-pick-item.active {
  border-color: var(--brand);
  background: #f2f7ff;
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.08);
}

.assistant-pick-main {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
}

.assistant-pick-main strong {
  font-size: 11.5px;
  font-weight: 600;
}

.assistant-pick-main code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.assistant-pick-desc {
  flex: 1;
  color: var(--muted);
  font-size: 9.5px;
}

.assistant-pick-provider {
  color: var(--muted-light);
  font-size: 9px;
}

.assistant-changes {
  margin: 5px 0 0;
  padding-left: 16px;
  color: #3f6b50;
  font-size: 9.5px;
  line-height: 1.75;
}

.assistant-chat {
  display: flex;
  min-height: 260px;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  padding: 18px 20px;
  background: #f8fafd;
}

.assistant-message {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.assistant-message.user {
  flex-direction: row-reverse;
}

.assistant-avatar {
  display: flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #e8eefb;
  color: #2854a8;
  font-family: var(--mono);
  font-size: 8px;
  flex-shrink: 0;
}

.assistant-message.user .assistant-avatar {
  background: #e7f7ed;
  color: #18794e;
}

.assistant-bubble {
  max-width: 78%;
  padding: 9px 12px;
  border: 1px solid #e8eef7;
  border-radius: 4px 12px 12px;
  background: #fff;
  color: var(--text);
  font-size: 11px;
  line-height: 1.75;
  white-space: pre-wrap;
}

.assistant-message.user .assistant-bubble {
  border-color: #d9e8ff;
  border-radius: 12px 4px 12px 12px;
  background: #f2f7ff;
}

.assistant-thinking {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--muted);
}

.assistant-ready {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 10px 12px;
  border: 1px solid #cdebd9;
  border-radius: 9px;
  background: #f2fcf6;
}

.assistant-ready strong {
  color: #166534;
  font-size: 11px;
}

.assistant-ready span {
  color: #4b7a5c;
  font-size: 9.5px;
}

.assistant-composer {
  flex-shrink: 0;
  padding: 12px 16px 16px;
  border-top: 1px solid #eef2f8;
}

.assistant-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.assistant-note {
  flex: 1;
  color: var(--muted-light);
  font-size: 9px;
}

.assistant-composer .modal-error {
  margin-top: 8px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 15px 26px;
  border-top: 1px solid #f0f3f8;
}

.detail-hero {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 18px;
  border-bottom: 1px solid #eef1f6;
}

.detail-icon {
  display: flex;
  width: 44px;
  height: 44px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: var(--brand-soft);
  color: var(--brand);
}

.detail-icon :deep(svg) {
  width: 21px;
  height: 21px;
}

.detail-hero > div:nth-child(2) {
  flex: 1;
}

.detail-hero h3 {
  margin: 0;
  color: var(--ink);
  font-size: 16px;
  font-weight: 620;
}

.detail-hero .mono {
  color: var(--muted-light);
  font-size: 9.5px;
}

.detail-tags {
  display: flex;
  gap: 5px;
}

.detail-kv {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 14px;
  padding: 18px 0;
}

.detail-kv span {
  display: block;
  color: var(--muted-light);
  font-size: 10px;
}

.detail-kv strong {
  display: block;
  margin-top: 2px;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 500;
}

.prompt-card {
  padding: 13px 14px;
  border: 1px solid #edf1f7;
  border-radius: 10px;
  background: #fbfcfe;
  color: var(--text);
  font-size: 11px;
  line-height: 1.75;
  white-space: pre-wrap;
}

.detail-rule {
  margin-bottom: 8px;
  padding: 11px 12px;
  border: 1px solid #edf1f7;
  border-radius: 10px;
}

.detail-rule-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rule-index {
  display: flex;
  width: 21px;
  height: 21px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  background: var(--brand-soft);
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 600;
}

.detail-rule-head strong {
  flex: 1;
  font-size: 11.5px;
  font-weight: 550;
}

.rule-logic {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.rule-output {
  padding: 1px 6px;
  border-radius: 5px;
  background: #f1f5f9;
  color: #475569;
  font-family: var(--mono);
  font-size: 8.5px;
}

.detail-rule-items {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 8px;
}

.rule-chip {
  padding: 2px 7px;
  border-radius: 5px;
  background: #f8fafc;
  color: var(--muted);
  font-size: 9px;
}

.rule-chip code {
  color: #475569;
  font-family: var(--mono);
}

.trigger-list {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.trigger-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 11px;
  border: 1px solid #edf1f7;
  border-radius: 9px;
}

.trigger-type {
  padding: 2px 7px;
  border-radius: 5px;
  background: var(--brand-soft);
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
}

.trigger-row > div {
  flex: 1;
  min-width: 0;
}

.trigger-info > span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trigger-error {
  color: #d03050;
  font-style: normal;
}

.trigger-row strong {
  display: block;
  font-size: 11px;
  font-weight: 500;
}

.trigger-row span {
  color: var(--muted-light);
  font-size: 9px;
}

.audit-mode-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border: 1px solid #e5dcff;
  border-radius: 5px;
  background: #f7f3ff;
  color: #6d4fc4;
  font-size: 8.5px;
  font-weight: 550;
}

.audit-mode-badge :deep(svg) {
  width: 10px;
  height: 10px;
}

.mode-picker {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  margin-bottom: 16px;
  padding: 12px 14px;
  border: 1px solid #e9eef6;
  border-radius: 11px;
  background: #fbfcfe;
}

.mode-picker.classify {
  border-color: #e6dcff;
  background: linear-gradient(120deg, #fbf8ff, #fff);
}

.mode-picker > div {
  min-width: 0;
}

.mode-picker strong {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 620;
}

.mode-picker strong :deep(svg) {
  width: 13px;
  height: 13px;
  color: var(--purple);
}

.mode-picker span {
  display: block;
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.6;
}

.form-hint.good {
  color: #15803d;
}

.classify-recommend {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.category-panel {
  margin-bottom: 16px;
  padding: 14px;
  border: 1px solid #e7deff;
  border-radius: 12px;
  background: #fcfbff;
}

.category-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.fixed-pass {
  display: inline-flex;
  height: 28px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  background: #ecfdf3;
  color: #15803d;
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 600;
}

.audit-mode-badge.classifier {
  background: #eef8f0;
  color: #16794a;
}

.mode-picker.classifier {
  border-color: #d7f0e0;
  background: #f7fffa;
}

.category-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  margin-bottom: 11px;
}

.category-head strong {
  display: block;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 620;
}

.category-head span {
  display: block;
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.55;
}

.category-grid-head,
.category-row {
  display: grid;
  grid-template-columns: 112px 118px minmax(0, 1fr) 108px 28px;
  gap: 6px;
  align-items: center;
}

.category-grid-head {
  margin-bottom: 5px;
  padding: 0 1px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.category-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 8px;
}

.category-row :deep(.n-input),
.category-row :deep(.n-base-selection) {
  font-size: 10px;
}

.export-modal {
  width: min(620px, calc(100vw - 32px));
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.export-target {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
  padding: 12px 14px;
  border: 1px solid #e9eef6;
  border-radius: 10px;
  background: #f8fafc;
}

.export-target strong {
  display: block;
  color: var(--ink);
  font-size: 12px;
  font-weight: 600;
}

.export-target span {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9px;
}

.export-target .export-target-count {
  margin: 0;
  color: var(--brand);
  font-size: 9px;
  white-space: nowrap;
}

.export-summary {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
  padding: 9px 11px;
  border-radius: 8px;
  background: #f2f7ff;
  color: #3f69bb;
  font-size: 9.5px;
}

.export-summary span {
  color: #7a8aa5;
}

.export-note {
  margin-top: 9px;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.6;
}

.detail-categories {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.detail-category {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 9px 10px;
  border: 1px solid #edf1f7;
  border-radius: 8px;
}

.detail-category code {
  width: 92px;
  color: #6d4fc4;
  font-family: var(--mono);
  font-size: 8.5px;
  flex-shrink: 0;
}

.detail-category > div {
  min-width: 0;
  flex: 1;
}

.detail-category strong {
  display: block;
  color: var(--text);
  font-size: 10.5px;
  font-weight: 550;
}

.detail-category span {
  display: block;
  overflow: hidden;
  color: var(--muted-light);
  font-size: 8.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.category-result {
  padding: 2px 7px;
  border-radius: 5px;
  font-size: 8.5px;
  flex-shrink: 0;
}

.category-result.pass {
  background: #eafaf2;
  color: #15803d;
}

.category-result.reject {
  background: #fef2f2;
  color: #b91c1c;
}

.category-result.review {
  background: #fff7e8;
  color: #a16207;
}

@media (max-width: 900px) {
  .audit-summary {
    flex-wrap: wrap;
    gap: 12px;
  }

  .summary-hint {
    width: 100%;
    margin-left: 0;
  }
}

@media (max-width: 620px) {
  .two-col,
  .three-col {
    grid-template-columns: 1fr;
  }

  .audit-card {
    padding: 16px;
  }

  .modal-head,
  .modal-body,
  .assistant-modal {
  display: flex;
  width: min(720px, calc(100vw - 30px));
  max-height: min(820px, calc(100vh - 36px));
  flex-direction: column;
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.assistant-head {
  flex-shrink: 0;
}

.assistant-target {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
  padding: 9px 20px;
  border-bottom: 1px solid #e8eef7;
  background: #f6f9ff;
}

.assistant-target-label {
  padding: 2px 6px;
  border-radius: 4px;
  background: #e3ecff;
  color: #2854a8;
  font-size: 8.5px;
}

.assistant-target strong {
  color: var(--text);
  font-size: 11px;
}

.assistant-target code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.assistant-target-current {
  width: 100%;
  color: var(--muted);
  font-size: 9.5px;
}

.assistant-pick-modal {
  width: min(560px, calc(100vw - 30px));
}

.assistant-pick-list {
  display: flex;
  flex-direction: column;
  gap: 7px;
  max-height: 320px;
  overflow-y: auto;
}

.assistant-pick-item {
  display: flex;
  width: 100%;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid #e6ecf6;
  border-radius: 9px;
  background: #fff;
  color: var(--text);
  cursor: pointer;
  text-align: left;
}

.assistant-pick-item:hover {
  border-color: #c9daf7;
  background: #f8fbff;
}

.assistant-pick-item.active {
  border-color: var(--brand);
  background: #f2f7ff;
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.08);
}

.assistant-pick-main {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
}

.assistant-pick-main strong {
  font-size: 11.5px;
  font-weight: 600;
}

.assistant-pick-main code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.assistant-pick-desc {
  flex: 1;
  color: var(--muted);
  font-size: 9.5px;
}

.assistant-pick-provider {
  color: var(--muted-light);
  font-size: 9px;
}

.assistant-changes {
  margin: 5px 0 0;
  padding-left: 16px;
  color: #3f6b50;
  font-size: 9.5px;
  line-height: 1.75;
}

.assistant-chat {
  display: flex;
  min-height: 260px;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  padding: 18px 20px;
  background: #f8fafd;
}

.assistant-message {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.assistant-message.user {
  flex-direction: row-reverse;
}

.assistant-avatar {
  display: flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #e8eefb;
  color: #2854a8;
  font-family: var(--mono);
  font-size: 8px;
  flex-shrink: 0;
}

.assistant-message.user .assistant-avatar {
  background: #e7f7ed;
  color: #18794e;
}

.assistant-bubble {
  max-width: 78%;
  padding: 9px 12px;
  border: 1px solid #e8eef7;
  border-radius: 4px 12px 12px;
  background: #fff;
  color: var(--text);
  font-size: 11px;
  line-height: 1.75;
  white-space: pre-wrap;
}

.assistant-message.user .assistant-bubble {
  border-color: #d9e8ff;
  border-radius: 12px 4px 12px 12px;
  background: #f2f7ff;
}

.assistant-thinking {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--muted);
}

.assistant-ready {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 10px 12px;
  border: 1px solid #cdebd9;
  border-radius: 9px;
  background: #f2fcf6;
}

.assistant-ready strong {
  color: #166534;
  font-size: 11px;
}

.assistant-ready span {
  color: #4b7a5c;
  font-size: 9.5px;
}

.assistant-composer {
  flex-shrink: 0;
  padding: 12px 16px 16px;
  border-top: 1px solid #eef2f8;
}

.assistant-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.assistant-note {
  flex: 1;
  color: var(--muted-light);
  font-size: 9px;
}

.assistant-composer .modal-error {
  margin-top: 8px;
}

.modal-footer {
    padding-left: 18px;
    padding-right: 18px;
  }

  .assistant-modal {
  display: flex;
  width: min(720px, calc(100vw - 30px));
  max-height: min(820px, calc(100vh - 36px));
  flex-direction: column;
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.assistant-head {
  flex-shrink: 0;
}

.assistant-target {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
  padding: 9px 20px;
  border-bottom: 1px solid #e8eef7;
  background: #f6f9ff;
}

.assistant-target-label {
  padding: 2px 6px;
  border-radius: 4px;
  background: #e3ecff;
  color: #2854a8;
  font-size: 8.5px;
}

.assistant-target strong {
  color: var(--text);
  font-size: 11px;
}

.assistant-target code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.assistant-target-current {
  width: 100%;
  color: var(--muted);
  font-size: 9.5px;
}

.assistant-pick-modal {
  width: min(560px, calc(100vw - 30px));
}

.assistant-pick-list {
  display: flex;
  flex-direction: column;
  gap: 7px;
  max-height: 320px;
  overflow-y: auto;
}

.assistant-pick-item {
  display: flex;
  width: 100%;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid #e6ecf6;
  border-radius: 9px;
  background: #fff;
  color: var(--text);
  cursor: pointer;
  text-align: left;
}

.assistant-pick-item:hover {
  border-color: #c9daf7;
  background: #f8fbff;
}

.assistant-pick-item.active {
  border-color: var(--brand);
  background: #f2f7ff;
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.08);
}

.assistant-pick-main {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
}

.assistant-pick-main strong {
  font-size: 11.5px;
  font-weight: 600;
}

.assistant-pick-main code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.assistant-pick-desc {
  flex: 1;
  color: var(--muted);
  font-size: 9.5px;
}

.assistant-pick-provider {
  color: var(--muted-light);
  font-size: 9px;
}

.assistant-changes {
  margin: 5px 0 0;
  padding-left: 16px;
  color: #3f6b50;
  font-size: 9.5px;
  line-height: 1.75;
}

.assistant-chat {
  display: flex;
  min-height: 260px;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  padding: 18px 20px;
  background: #f8fafd;
}

.assistant-message {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.assistant-message.user {
  flex-direction: row-reverse;
}

.assistant-avatar {
  display: flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #e8eefb;
  color: #2854a8;
  font-family: var(--mono);
  font-size: 8px;
  flex-shrink: 0;
}

.assistant-message.user .assistant-avatar {
  background: #e7f7ed;
  color: #18794e;
}

.assistant-bubble {
  max-width: 78%;
  padding: 9px 12px;
  border: 1px solid #e8eef7;
  border-radius: 4px 12px 12px;
  background: #fff;
  color: var(--text);
  font-size: 11px;
  line-height: 1.75;
  white-space: pre-wrap;
}

.assistant-message.user .assistant-bubble {
  border-color: #d9e8ff;
  border-radius: 12px 4px 12px 12px;
  background: #f2f7ff;
}

.assistant-thinking {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--muted);
}

.assistant-ready {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 10px 12px;
  border: 1px solid #cdebd9;
  border-radius: 9px;
  background: #f2fcf6;
}

.assistant-ready strong {
  color: #166534;
  font-size: 11px;
}

.assistant-ready span {
  color: #4b7a5c;
  font-size: 9.5px;
}

.assistant-composer {
  flex-shrink: 0;
  padding: 12px 16px 16px;
  border-top: 1px solid #eef2f8;
}

.assistant-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.assistant-note {
  flex: 1;
  color: var(--muted-light);
  font-size: 9px;
}

.assistant-composer .modal-error {
  margin-top: 8px;
}

.modal-footer {
    flex-wrap: wrap;
  }

  .mode-picker {
    align-items: stretch;
    flex-direction: column;
    gap: 10px;
  }

  .mode-picker :deep(.n-select) {
    width: 100% !important;
  }

  .category-grid-head {
    display: none;
  }

  .category-row {
    position: relative;
    grid-template-columns: 1fr 1fr;
    padding: 9px;
    border: 1px solid #eee7fa;
    border-radius: 8px;
    background: #fff;
  }

  .category-row > :nth-child(3),
  .category-row > :nth-child(4) {
    grid-column: 1 / -1;
  }

  .category-row > :nth-child(5) {
    position: absolute;
    top: 6px;
    right: 6px;
  }

  .export-target {
    align-items: flex-start;
    flex-direction: column;
  }
}

.assistant-message.assistant {
  align-items: flex-start;
}

.assistant-content {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 7px;
  padding-top: 2px;
}

.assistant-answer {
  color: var(--text);
  font-size: 11px;
  line-height: 1.8;
  word-break: break-word;
}

.assistant-pending-line {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--muted);
  font-size: 10.5px;
}

.assistant-answer.markdown-body {
  white-space: normal;
}

.assistant-answer.markdown-body p {
  margin: 0 0 6px;
}

.assistant-answer.markdown-body p:last-child {
  margin-bottom: 0;
}

.assistant-answer.markdown-body ul,
.assistant-answer.markdown-body ol {
  margin: 4px 0 6px;
  padding-left: 18px;
}

.assistant-answer.markdown-body li {
  margin: 2px 0;
}

.assistant-answer.markdown-body strong {
  color: #1d4ed8;
  font-weight: 600;
}

.assistant-answer.markdown-body code {
  padding: 1px 4px;
  border-radius: 4px;
  background: #eef2fa;
  color: #334155;
  font-family: var(--mono);
  font-size: 10px;
}

.assistant-answer.markdown-body pre {
  margin: 6px 0;
  padding: 8px 10px;
  overflow-x: auto;
  border: 1px solid #e3e9f4;
  border-radius: 7px;
  background: #f8fafd;
}

.assistant-answer.markdown-body pre code {
  padding: 0;
  background: transparent;
  font-size: 9.5px;
  line-height: 1.6;
}

.assistant-answer.markdown-body a {
  color: #2b6cf6;
}

.assistant-answer.markdown-body table {
  width: 100%;
  margin: 6px 0;
  border-collapse: collapse;
  font-size: 10px;
}

.assistant-answer.markdown-body th,
.assistant-answer.markdown-body td {
  padding: 4px 6px;
  border: 1px solid #e3e9f4;
  text-align: left;
}

.assistant-answer.markdown-body blockquote {
  margin: 6px 0;
  padding-left: 8px;
  border-left: 2px solid #cbd9f2;
  color: var(--muted);
}

.assistant-message-images {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 6px;
}

.assistant-message-images :deep(.n-image img) {
  border-radius: 6px;
}

.assistant-trace {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px 9px;
  border: 1px solid #e8eef7;
  border-radius: 9px;
  background: #fff;
}

.assistant-trace-toggle {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--muted);
  font-family: inherit;
  font-size: 9.5px;
  text-align: left;
  cursor: pointer;
}

.assistant-trace-toggle svg {
  width: 12px;
  height: 12px;
}

.assistant-trace-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-top: 5px;
  border-top: 1px solid #f0f3f8;
}

.assistant-step {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  color: var(--muted);
  font-size: 10px;
}

.assistant-step-icon {
  display: flex;
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
}

.assistant-step-icon svg {
  width: 13px;
  height: 13px;
}

.assistant-step.done .assistant-step-icon {
  color: #16a34a;
}

.assistant-step.error .assistant-step-icon {
  color: #dc2626;
}

.assistant-step.running .assistant-step-icon {
  color: #2b6cf6;
}

.assistant-step-label {
  flex-shrink: 0;
  color: var(--text);
  font-weight: 600;
}

.assistant-step-detail {
  overflow: hidden;
  color: var(--muted-light);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.assistant-attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}

.assistant-attachment {
  position: relative;
  display: flex;
  width: 72px;
  flex-direction: column;
  gap: 3px;
}

.assistant-attachment :deep(.n-image) {
  overflow: hidden;
  border-radius: 6px;
}

.assistant-attachment-remove {
  position: absolute;
  top: -6px;
  right: -6px;
  display: flex;
  width: 18px;
  height: 18px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid #fecaca;
  border-radius: 50%;
  background: #fff;
  color: #dc2626;
  cursor: pointer;
}

.assistant-attachment-remove svg {
  width: 12px;
  height: 12px;
}

.assistant-attachment-name {
  overflow: hidden;
  color: var(--muted-light);
  font-size: 8px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.assistant-model-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.assistant-model-label {
  color: var(--muted-light);
  font-size: 9.5px;
  white-space: nowrap;
}

.assistant-model-select {
  max-width: 360px;
  flex: 1;
}

/* AI 配置助手：视觉与阅读体验升级 */
.assistant-modal {
  width: min(880px, calc(100vw - 40px));
  max-height: min(860px, calc(100vh - 40px));
  border: 1px solid var(--border);
  border-radius: 20px;
  background: var(--card);
  box-shadow: 0 34px 90px -24px rgba(15, 23, 42, 0.42);
}

.assistant-head {
  align-items: flex-start;
  padding: 22px 24px 18px;
  border-bottom: 1px solid var(--border);
  background: linear-gradient(130deg, #f7faff 0%, #fff 65%);
}

.assistant-heading {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: flex-start;
  gap: 12px;
}

.assistant-heading > div {
  min-width: 0;
  flex: 1;
}

.assistant-header-icon {
  display: flex;
  width: 42px;
  height: 42px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: 1px solid #c7d9fb;
  border-radius: 12px;
  background: var(--brand-soft);
  color: var(--brand);
  box-shadow: 0 6px 16px -8px rgba(43, 108, 246, 0.45);
}

.assistant-header-icon :deep(svg) {
  width: 21px;
  height: 21px;
}

.assistant-heading h2 {
  margin: 4px 0;
  color: var(--ink);
  font-size: 19px;
  font-weight: 650;
  letter-spacing: -0.01em;
}

.assistant-heading p {
  max-width: 660px;
  margin: 0;
  color: var(--muted);
  font-size: 11.5px;
  line-height: 1.65;
}

.assistant-head :deep(.n-button) {
  --n-text-color: var(--muted) !important;
  --n-text-color-hover: var(--ink) !important;
}

.assistant-target {
  padding: 10px 24px;
  border-bottom: 1px solid var(--border);
  background: var(--brand-softer);
}

.assistant-target-label {
  padding: 3px 7px;
  border-radius: 5px;
  background: #e1ebff;
  color: #1d55d8;
  font-size: 9px;
}

.assistant-target strong {
  font-size: 11.5px;
}

.assistant-target code {
  font-size: 9.5px;
}

.assistant-target-current {
  color: var(--muted);
  font-size: 10px;
}

.assistant-chat {
  min-height: 360px;
  gap: 14px;
  padding: 20px 24px;
  background: linear-gradient(180deg, #f8fafd 0%, #f5f9ff 100%);
}

.assistant-message {
  gap: 10px;
}

.assistant-avatar {
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: var(--brand-soft);
  color: var(--brand);
  font-size: 0;
}

.assistant-avatar :deep(svg) {
  width: 16px;
  height: 16px;
}

.assistant-message.user .assistant-avatar {
  background: var(--green-soft);
  color: #15803d;
}

.assistant-bubble {
  max-width: 76%;
  padding: 10px 13px;
  border: 1px solid #d9e8ff;
  border-radius: 14px 4px 14px 14px;
  background: var(--brand-soft);
  color: #1e3a8a;
  font-size: 12px;
  line-height: 1.75;
  box-shadow: var(--shadow-sm);
}

.assistant-message.user .assistant-bubble {
  border-color: #d9e8ff;
  border-radius: 14px 4px 14px 14px;
  background: var(--brand-soft);
  color: #1e3a8a;
}

.assistant-content {
  display: flex;
  min-width: 0;
  max-width: 86%;
  flex: 0 1 auto;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 4px 14px 14px 14px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.assistant-answer {
  color: var(--text);
  font-size: 12px;
  line-height: 1.8;
}

.assistant-answer.markdown-body strong {
  color: var(--brand-strong);
}

.assistant-answer.markdown-body code {
  font-size: 10.5px;
}

.assistant-progress {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid #e1eaf8;
  border-radius: 10px;
  background: #f8fbff;
}

.assistant-progress-head {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--muted);
  font-size: 11px;
}

.assistant-trace-list {
  gap: 5px;
  padding-top: 8px;
  border-top: 1px solid #e8effa;
}

.assistant-step {
  gap: 7px;
  font-size: 10.5px;
}

.assistant-step-label {
  color: var(--text);
  font-weight: 550;
}

.assistant-step-detail {
  color: var(--muted-light);
  font-size: 10px;
}

.assistant-step-icon :deep(svg) {
  width: 12px;
  height: 12px;
}

.assistant-ready {
  gap: 5px;
  padding: 12px 13px;
  border: 1px solid #cdebd9;
  border-radius: 11px;
  background: #f2fcf6;
}

.assistant-ready strong {
  color: #166534;
  font-size: 12px;
}

.assistant-ready span {
  color: #4b7a5c;
  font-size: 10.5px;
}

.assistant-changes {
  font-size: 10.5px;
  line-height: 1.8;
}

.assistant-starters {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 2px;
  padding-left: 40px;
}

.assistant-starters-title {
  color: var(--muted-light);
  font-size: 9.5px;
}

.assistant-starter-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.assistant-starter-list button {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: #fff;
  color: var(--muted);
  cursor: pointer;
  font-size: 10px;
  transition: border-color 0.16s ease, background 0.16s ease, color 0.16s ease;
}

.assistant-starter-list button:hover {
  border-color: #c7d9fb;
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.assistant-starter-list button :deep(svg) {
  width: 13px;
  height: 13px;
}

.assistant-composer {
  padding: 14px 20px 18px;
  border-top: 1px solid var(--border);
  background: #fff;
}

.assistant-input :deep(textarea) {
  font-size: 12px;
  line-height: 1.7;
}

.assistant-composer-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
}

.assistant-model-row {
  display: flex;
  flex: 0 1 340px;
  align-items: center;
  gap: 8px;
  margin-top: 0;
}

.assistant-model-label {
  color: var(--muted);
  font-size: 10px;
}

.assistant-model-select {
  max-width: 300px;
  flex: 1;
}

.assistant-actions {
  flex: 1;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 0;
}

.assistant-note {
  flex: 1;
  color: var(--muted-light);
  font-size: 9.5px;
  text-align: right;
}

@media (max-width: 720px) {
  .assistant-modal {
    width: calc(100vw - 16px);
    max-height: calc(100vh - 16px);
  }

  .assistant-head {
    padding: 16px;
  }

  .assistant-heading h2 {
    font-size: 16px;
  }

  .assistant-heading p {
    font-size: 10.5px;
  }

  .assistant-header-icon {
    width: 36px;
    height: 36px;
  }

  .assistant-target {
    padding: 9px 16px;
  }

  .assistant-chat {
    min-height: 280px;
    gap: 12px;
    padding: 14px 16px;
  }

  .assistant-avatar {
    width: 26px;
    height: 26px;
  }

  .assistant-content,
  .assistant-bubble {
    max-width: 88%;
  }

  .assistant-starters {
    padding-left: 0;
  }

  .assistant-composer {
    padding: 12px 14px 14px;
  }

  .assistant-composer-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .assistant-model-row {
    flex-basis: auto;
  }

  .assistant-model-select {
    max-width: none;
  }

  .assistant-actions {
    justify-content: flex-end;
  }

  .assistant-note {
    display: none;
  }
}
</style>
