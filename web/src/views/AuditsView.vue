<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import dayjs from 'dayjs'
import {
  NButton,
  NDatePicker,
  NDrawer,
  NDrawerContent,
  NDropdown,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  AddOutline,
  CloudOutline,
  CopyOutline,
  DownloadOutline,
  EllipsisHorizontalOutline,
  FlashOutline,
  RefreshOutline,
  SearchOutline,
  ShieldCheckmarkOutline,
  SparklesOutline,
  TextOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import TaskSubmitModal from '@/components/TaskSubmitModal.vue'
import AuditIntegrationDrawer from '@/components/AuditIntegrationDrawer.vue'
import { auditApi, metaApi, providerApi } from '@/api'
import { copyText } from '@/utils/clipboard'
import type { Audit, AuditCategory, Provider, RuleTemplate, TriggerSource } from '@/api/types'
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
const templates = ref<RuleTemplate[]>([])
const search = ref('')
const statusFilter = ref<string | null>('ACTIVE')
const modalShow = ref(false)
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
const decisionFilter = ref<string | null>(null)
const exportShow = ref(false)
const exportTarget = ref<Audit | null>(null)
const exporting = ref(false)

const defaultPrompt =
  '你是“智审图档”平台的档案图档审核专家。请依据档案管理规范、内容安全要求与审核要点，判断文档是否清晰完整、是否存在敏感或违规内容，以及印章、签名、日期等关键要素是否齐全。无法确认时返回 REVIEW。'

const defaultClassifyPrompt = ref(
  '你是“智审图档”平台的档案分类专家。请根据文本、OCR 内容和图片版面，从配置的分类列表中选择唯一最匹配的类别，并给出 0~1 的置信度和判断依据。无法可靠判断时选择其他类别，不要臆测。',
)

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
  decisionMode: 'REVIEW' as 'REVIEW' | 'CLASSIFY',
  categories: [] as AuditCategory[],
  providerId: null as number | null,
  reviewPrompt: defaultPrompt,
  strictness: 'standard',
  defaultResult: 'REVIEW',
  onModelFail: 'REVIEW',
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

const statusFilterOptions = [
  { label: '已启用', value: 'ACTIVE' },
  { label: '已暂停', value: 'PAUSED' },
  { label: '草稿', value: 'DRAFT' },
]
const decisionFilterOptions = [
  { label: '合规审核', value: 'REVIEW' },
  { label: '分类审核', value: 'CLASSIFY' },
]
const decisionFormOptions = [
  { label: '合规审核 · PASS / REJECT / REVIEW', value: 'REVIEW' },
  { label: '分类审核 · 按分类映射终审结论', value: 'CLASSIFY' },
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
  { label: 'HTTP 轮询', value: 'POLLING' },
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
  if (kind === 'JEV') return 'JEV'
  if (kind === 'ANTHROPIC') return 'Anthropic'
  return '通用大模型'
}

const providerOptions = computed(() =>
  providers.value.map((p) => ({
    label: `${p.name} · ${providerKindLabel(p.kind)}`,
    value: p.id,
    disabled: p.status !== 'ACTIVE',
  })),
)

const templateOptions = computed(() => [
  ...(editing.value ? [{ label: '保持当前规则', value: 'keep' }] : []),
  ...templates.value
    .filter((t) => (t.kind || 'REVIEW') === form.decisionMode)
    .map((t) => ({ label: `${t.name} · ${t.description}`, value: t.key })),
])

const activeCount = computed(() => audits.value.filter((a) => a.status === 'ACTIVE').length)
const classifyCount = computed(() => audits.value.filter((a) => a.decision_mode === 'CLASSIFY').length)
const todayCount = computed(() => audits.value.reduce((sum, a) => sum + (a.today_tasks || 0), 0))
const selectedProvider = computed(() => providers.value.find((p) => p.id === form.providerId))
const jevProviders = computed(() => providers.value.filter((p) => p.kind === 'JEV' && p.status === 'ACTIVE'))
const selectedTemplate = computed(() => templates.value.find((t) => t.key === form.strictness))
const exportSummary = computed(() => {
  const target = exportForm.range
    ? `${dayjs(exportForm.range[0]).format('MM-DD')} 至 ${dayjs(exportForm.range[1]).format('MM-DD')}`
    : '全部时间'
  const status = exportForm.statuses.length ? `${exportForm.statuses.length} 种状态` : '全部状态'
  const result = exportForm.results.length ? `${exportForm.results.length} 种结论` : '全部结论'
  const source = exportForm.sources.length ? `${exportForm.sources.length} 种来源` : '全部来源'
  return `${target} · ${status} · ${result} · ${source}`
})

function resetForm() {
  form.name = ''
  form.code = ''
  form.description = ''
  form.mediaTypes = ['text', 'image']
  form.decisionMode = 'REVIEW'
  form.categories = []
  form.providerId = providers.value.find((p) => p.status === 'ACTIVE')?.id || providers.value[0]?.id || null
  form.reviewPrompt = defaultPrompt
  form.strictness = 'standard'
  form.defaultResult = 'REVIEW'
  form.onModelFail = 'REVIEW'
  form.dedupeWindowSec = 86400
  errorText.value = ''
  editing.value = null
}

function openCreate() {
  if (!workspace.currentProjectId) {
    message.warning('请先选择项目')
    return
  }
  resetForm()
  modalShow.value = true
}

function openEdit(audit: Audit) {
  editing.value = audit
  form.name = audit.name
  form.code = audit.code
  form.description = audit.description || ''
  form.mediaTypes = [...(audit.media_types || [])]
  form.decisionMode = audit.decision_mode === 'CLASSIFY' ? 'CLASSIFY' : 'REVIEW'
  form.categories = (audit.categories || []).map((category) => ({ ...category }))
  form.providerId = audit.provider_id
  form.reviewPrompt = audit.review_prompt || defaultPrompt
  form.strictness = 'keep'
  form.defaultResult = audit.default_result
  form.onModelFail = audit.on_model_fail
  form.dedupeWindowSec = audit.dedupe_window_sec
  errorText.value = ''
  modalShow.value = true
}

function setDecisionMode(value: string | number) {
  const mode = value === 'CLASSIFY' ? 'CLASSIFY' : 'REVIEW'
  if (mode === form.decisionMode) return
  form.decisionMode = mode
  if (mode === 'CLASSIFY') {
    if (form.categories.length < 2) form.categories = exampleCategories.value.map((category) => ({ ...category }))
    form.strictness = 'classify'
    if (!editing.value) {
      form.reviewPrompt = defaultClassifyPrompt.value
      if (jevProviders.value.length) form.providerId = jevProviders.value[0].id
    }
  } else {
    form.categories = []
    form.strictness = 'standard'
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

function loadExampleCategories() {
  form.categories = exampleCategories.value.map((category) => ({ ...category }))
  message.success('已载入分类示例，可按业务调整')
}

function validateCategoryForm(): string {
  if (form.categories.length < 2) return '分类审核至少需要 2 个分类'
  if (form.categories.length > 20) return '分类数量不能超过 20 个'
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
      { label: audit.status === 'ACTIVE' ? '暂停审核' : '启用审核', key: 'status' },
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
    case 'status':
      toggleStatus(audit)
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

async function load() {
  if (!workspace.currentProjectId) {
    audits.value = []
    providers.value = []
    return
  }
  loading.value = true
  try {
    const [auditData, providerData] = await Promise.all([
      auditApi.list({ project_id: workspace.currentProjectId, page: 1, page_size: 100 }),
      providerApi.list({ project_id: workspace.currentProjectId }),
    ])
    audits.value = auditData.items
    providers.value = providerData.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载审核配置失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  errorText.value = ''
  if (!form.name.trim() || (!editing.value && !form.code.trim())) {
    errorText.value = '请填写审核名称与编码'
    return
  }
  if (!form.providerId) {
    errorText.value = '请选择模型供应商'
    return
  }
  if (!form.mediaTypes.length) {
    errorText.value = '至少选择一种媒体类型'
    return
  }
  if (form.decisionMode === 'CLASSIFY') {
    const error = validateCategoryForm()
    if (error) {
      errorText.value = error
      return
    }
  }
  const categories = form.categories.map((category) => ({
    code: category.code.trim(),
    name: category.name.trim(),
    description: (category.description || '').trim(),
    result: category.result,
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
    dedupe_window_sec: form.dedupeWindowSec,
    status: editing.value?.status === 'DRAFT' ? 'DRAFT' : 'ACTIVE',
  }
  if (form.strictness && form.strictness !== 'keep') payload.strictness = form.strictness
  saving.value = true
  try {
    if (editing.value) {
      await auditApi.update(editing.value.id, payload)
      message.success('审核配置已更新')
      modalShow.value = false
      await load()
    } else {
      const created = await auditApi.create(payload)
      message.success('审核配置已创建，请选择触发方式接入')
      modalShow.value = false
      await load()
      guideAuditId.value = created.id
      guideShow.value = true
    }
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggleStatus(audit: Audit) {
  const next = audit.status === 'ACTIVE' ? 'PAUSED' : 'ACTIVE'
  try {
    await auditApi.setStatus(audit.id, next)
    message.success(next === 'ACTIVE' ? '审核已启用' : '审核已暂停')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
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
  return audit.decision_mode === 'CLASSIFY' ? `分类审核 · ${(audit.categories || []).length} 类` : '合规审核'
}

function categoryResultLabel(result: string) {
  return { PASS: '通过', REJECT: '驳回', REVIEW: '转人工' }[result] || result
}

function triggerLabel(type: string) {
  return { OPENAPI: '开放 API', REDIS: 'Redis', WEBSOCKET: 'WebSocket', POLLING: '轮询' }[type] || type
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
        <n-button quaternary circle :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
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
        <strong>{{ activeCount }}</strong>
      </div>
      <div class="summary-divider"></div>
      <div>
        <span>分类流程</span>
        <strong>{{ classifyCount }}</strong>
      </div>
      <div class="summary-divider"></div>
      <div>
        <span>今日执行任务</span>
        <strong>{{ todayCount }}</strong>
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
      <span class="result-count">{{ filtered.length }} 条配置</span>
    </div>

    <div v-if="filtered.length" class="audit-list fade-list">
      <article v-for="audit in filtered" :key="audit.id" class="audit-card">
        <div class="audit-side" :class="{ active: audit.status === 'ACTIVE' }">
          <ShieldCheckmarkOutline />
        </div>
        <div class="audit-main">
          <div class="audit-title-row">
            <h3>{{ audit.name }}</h3>
            <span class="audit-code mono">{{ audit.code }}</span>
            <StatusTag :status="audit.status" type="audit" />
            <StatusTag v-if="audit.provider_kind" :status="audit.provider_kind" type="kind" />
            <span v-if="audit.decision_mode === 'CLASSIFY'" class="audit-mode-badge">
              <SparklesOutline />
              分类审核 · {{ (audit.categories || []).length }} 类
            </span>
          </div>
          <p class="audit-desc">{{ audit.description || '暂无说明' }}</p>
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
          </div>
          <div class="audit-trigger-row">
            <span class="trigger-row-label">已开启接入</span>
            <span v-for="type in audit.active_triggers || []" :key="type" class="trigger-chip">{{ triggerLabel(type) }}</span>
            <span v-if="!(audit.active_triggers || []).length" class="trigger-chip muted">暂无</span>
          </div>
          <div class="audit-prompt">{{ audit.review_prompt }}</div>
        </div>
        <div class="audit-actions">
          <n-button v-if="auth.can('task:submit')" size="small" type="primary" :disabled="audit.status !== 'ACTIVE'" @click="openSubmit(audit)">
            <template #icon><FlashOutline /></template>
            发起审核
          </n-button>
          <div class="action-row">
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

    <n-modal v-model:show="modalShow" :mask-closable="!saving">
      <div class="audit-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT AUDIT FLOW' : 'NEW AUDIT FLOW' }}</div>
            <h2>{{ editing ? '编辑审核配置' : '新建审核配置' }}</h2>
            <p>配置会立即进入版本管理，启用后可被任务中心和开放 API 使用。</p>
          </div>
          <span class="modal-icon"><ShieldCheckmarkOutline /></span>
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
          <div class="mode-picker" :class="{ classify: form.decisionMode === 'CLASSIFY' }">
            <div>
              <strong><SparklesOutline /> 审核类型</strong>
              <span>{{ form.decisionMode === 'CLASSIFY' ? '模型先选分类，系统再按分类映射终审结论，并支持分类置信度转人工。' : '模型直接给出 PASS / REJECT / REVIEW，按风险阈值和规则组判定。' }}</span>
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
              <div v-else-if="form.decisionMode === 'CLASSIFY' && selectedProvider?.kind !== 'JEV'" class="form-hint warn classify-recommend">
                <span>分类审核建议使用 JEV 决策模型：原生 choice 分类、概率可解释。</span>
                <n-button v-if="jevProviders.length" text size="tiny" type="primary" @click="useJevProvider">切换到 JEV</n-button>
              </div>
              <div v-else-if="form.decisionMode === 'CLASSIFY'" class="form-hint good">
                已选择 JEV：分类会通过 System One choice 输出编码与置信度。
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
          <div class="field">
            <label>审核要求</label>
            <n-input
              v-model:value="form.reviewPrompt"
              type="textarea"
              :autosize="{ minRows: 4, maxRows: 8 }"
              placeholder="告诉模型该审核什么、什么情况通过/驳回/转人工"
            />
            <div class="form-hint">这段要求会作为审核 Prompt 发送给通用模型，并作为 JEV 的 state 审核要求。</div>
          </div>
          <div v-if="form.decisionMode === 'CLASSIFY'" class="category-panel">
            <div class="category-head">
              <div>
                <strong>分类与终审结论映射</strong>
                <span>分类编码会写入结果的 category 字段；每个分类的映射结论由规则引擎直接采用，低置信度通过自动转人工。</span>
              </div>
              <n-button size="tiny" secondary @click="loadExampleCategories">
                <template #icon><SparklesOutline /></template>
                载入示例分类
              </n-button>
            </div>
            <div class="category-grid-head">
              <span>分类编码</span><span>显示名称</span><span>判定说明</span><span>映射结论</span><span></span>
            </div>
            <div class="category-list">
              <div v-for="(category, index) in form.categories" :key="index" class="category-row">
                <n-input v-model:value="category.code" size="small" placeholder="contract" class="mono-input" />
                <n-input v-model:value="category.name" size="small" placeholder="合同协议" />
                <n-input v-model:value="category.description" size="small" placeholder="判定依据与典型特征" />
                <n-select v-model:value="category.result" size="small" :options="categoryResultOptions" />
                <n-button quaternary circle size="tiny" :disabled="form.categories.length <= 2" @click="removeCategory(index)">
                  <template #icon><TrashOutline /></template>
                </n-button>
              </div>
            </div>
            <n-button dashed size="small" block @click="addCategory">
              <template #icon><AddOutline /></template>
              添加分类
            </n-button>
          </div>
          <div class="rule-template">
            <div class="rule-template-head">
              <div>
                <strong>{{ form.decisionMode === 'CLASSIFY' ? '分类规则模板' : '风险规则模板' }}</strong>
                <span>{{ form.decisionMode === 'CLASSIFY' ? '按分类映射结论，并对低置信度的通过结果转人工。' : '决定模型输出后的 PASS / REJECT / REVIEW 阈值。' }}仅支持多文本/多图片，不支持视频。</span>
              </div>
              <n-select
                v-model:value="form.strictness"
                :options="templateOptions"
                size="small"
                style="width: 190px"
              />
            </div>
            <div class="template-desc">
              <template v-for="t in templates" :key="t.key">
                <span v-if="form.strictness === t.key">{{ t.description }}</span>
              </template>
              <span v-if="form.strictness === 'keep'">保持当前已有规则，不会覆盖自定义条件。</span>
            </div>
          </div>
          <div class="three-col">
            <div class="field">
              <label>默认结论</label>
              <n-select
                v-model:value="form.defaultResult"
                :options="[
                  { label: '待议 REVIEW', value: 'REVIEW' },
                  { label: '通过 PASS', value: 'PASS' },
                  { label: '不通过 REJECT', value: 'REJECT' },
                ]"
              />
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
              <label>去重窗口（秒）</label>
              <n-input-number v-model:value="form.dedupeWindowSec" :min="0" :max="604800" :step="3600" style="width: 100%" />
            </div>
          </div>
          <div class="create-hint">
            <ShieldCheckmarkOutline />
            <span v-if="editing">保存后可在「接入指南」中查看当前四种触发方式、接口参数和回调配置。</span>
            <span v-else>创建后会自动生成 开放 API / Redis / WebSocket / HTTP 轮询 四种接入方式，并立即打开接入指南。</span>
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
          <n-button v-if="auth.can('audit:write')" type="primary" :loading="saving" @click="save">{{ editing ? '保存修改' : '创建并查看接入方式' }}</n-button>
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
          <span class="modal-icon"><CopyOutline /></span>
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
          <span class="modal-icon"><DownloadOutline /></span>
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

          <template v-if="detailAudit.decision_mode === 'CLASSIFY'">
            <h4 class="section-label">分类与结论映射</h4>
            <div class="detail-categories">
              <div v-for="category in detailAudit.categories || []" :key="category.code" class="detail-category">
                <code>{{ category.code }}</code>
                <div>
                  <strong>{{ category.name }}</strong>
                  <span>{{ category.description || '未填写判定说明' }}</span>
                </div>
                <span class="category-result" :class="category.result.toLowerCase()">{{ categoryResultLabel(category.result) }}</span>
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
              <div><strong>{{ trigger.name }}</strong><span>优先级 {{ trigger.priority }}</span></div>
              <StatusTag :status="trigger.status" type="provider" />
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
  </div>
</template>

<style scoped>
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
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.audit-card {
  display: flex;
  gap: 16px;
  padding: 18px 20px 17px;
  border: 1px solid var(--border);
  border-radius: 15px;
  background: #fff;
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.18s ease;
}

.audit-card:hover {
  box-shadow: var(--shadow-md);
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
  flex-shrink: 0;
}

.audit-side.active {
  background: linear-gradient(145deg, #edf4ff, #e1ebff);
  color: var(--brand);
}

.audit-side :deep(svg) {
  width: 19px;
  height: 19px;
}

.audit-main {
  min-width: 0;
  flex: 1;
}

.audit-title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
}

.audit-title-row h3 {
  margin: 0;
  color: var(--ink);
  font-size: 14.5px;
  font-weight: 620;
}

.audit-code {
  color: var(--muted-light);
  font-size: 9.5px;
}

.audit-desc {
  margin: 5px 0 8px;
  color: var(--muted);
  font-size: 11.5px;
}

.audit-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
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
  overflow: hidden;
  max-width: 780px;
  margin-top: 10px;
  color: #9aa5b5;
  font-size: 10px;
  line-height: 1.6;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audit-actions {
  display: flex;
  align-items: flex-end;
  flex-direction: column;
  justify-content: space-between;
  gap: 12px;
  flex-shrink: 0;
}

.action-row {
  display: flex;
  gap: 1px;
  color: var(--muted);
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

.template-desc {
  margin-top: 8px;
  color: var(--muted);
  font-size: 10px;
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

.action-row {
  align-items: center;
  gap: 4px;
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

  .audit-card {
    flex-wrap: wrap;
  }

  .audit-actions {
    width: 100%;
    align-items: center;
    flex-direction: row;
    justify-content: flex-end;
  }
}

@media (max-width: 620px) {
  .two-col,
  .three-col {
    grid-template-columns: 1fr;
  }

  .audit-actions {
    flex-wrap: wrap;
    justify-content: flex-start;
  }

  .audit-prompt {
    white-space: normal;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }

  .modal-head,
  .modal-body,
  .modal-footer {
    padding-left: 18px;
    padding-right: 18px;
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

  .action-row {
    flex-wrap: wrap;
  }

  .export-target {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
