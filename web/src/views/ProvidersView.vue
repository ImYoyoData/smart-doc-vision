<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  NButton,
  NInput,
  NInputNumber,
  NModal,
  NPopover,
  NSelect,
  NSwitch,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  AddOutline,
  CheckmarkCircleOutline,
  CloudOutline,
  CloseCircleOutline,
  FlashOutline,
  HelpCircleOutline,
  ImagesOutline,
  LinkOutline,
  RefreshOutline,
  SparklesOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import { providerApi } from '@/api'
import type { Provider, ProviderModel } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const workspace = useWorkspaceStore()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const testingId = ref<number | null>(null)
const testingAll = ref(false)
const testAllDone = ref(0)
const testAllTotal = ref(0)
const PROVIDER_TEST_TIMEOUT_MS = 30_000
const modalShow = ref(false)
const editing = ref<Provider | null>(null)
const errorText = ref('')
const providers = ref<Provider[]>([])
const modelOptions = ref<ProviderModel[]>([])
const modelsLoading = ref(false)
const modelsError = ref('')

const form = reactive({
  name: '',
  kind: 'OPENAI_COMPATIBLE',
  baseUrl: 'https://api.openai.com/v1',
  model: 'gpt-4o-mini',
  apiKey: '',
  prompt: '',
  supportsVision: false,
  supportsThinking: false,
  preferReasoning: false,
  contextWindow: null as number | null,
  maxTokens: null as number | null,
  maxImages: 6,
  temperature: 0.1,
  headersText: '',
  status: 'ACTIVE',
})

const isJEV = computed(() => form.kind === 'JEV')
const isAnthropic = computed(() => form.kind === 'ANTHROPIC')
const selectedModel = computed(() => modelOptions.value.find((item) => item.id === form.model))
const modelSelectOptions = computed(() => modelOptions.value.map((item) => ({
  label: modelOptionLabel(item),
  value: item.id,
})))
const activeProvider = computed(() => providers.value.find((p) => p.id === testingId.value))
const baseUrlHint = computed(() => {
  if (form.kind === 'JEV') return 'JEV 会追加 /systemone（若 BaseURL 已含 /v1）。'
  if (form.kind === 'ANTHROPIC') return 'Anthropic 会追加 /messages（若 BaseURL 已含 /v1）；图片会下载并转成 base64 图片块提交。'
  return 'OpenAI 兼容模型会追加 /chat/completions；支持自定义代理地址。'
})

function providerKindLabel(kind?: string) {
  if (kind === 'JEV') return 'JEV 决策模型'
  return '通用大模型'
}

function providerProtocolLabel(kind?: string) {
  if (kind === 'JEV') return 'JEV System One'
  return kind === 'ANTHROPIC' ? 'Anthropic 协议' : 'OpenAI 兼容协议'
}

function providerBreakerOpen(provider: Provider) {
  return Boolean(provider.breaker_until)
}

function providerBreakerLabel(provider: Provider) {
  if (!provider.breaker_until) return ''
  const remaining = new Date(provider.breaker_until).getTime() - Date.now()
  if (remaining <= 0) return '等待恢复探测'
  const minutes = Math.ceil(remaining / 60000)
  return minutes <= 1 ? '即将重试' : `${minutes} 分钟后重试`
}

type ProviderStatusFilter = 'all' | 'active' | 'available' | 'unavailable' | 'disabled'
type ProviderKindFilter = 'all' | 'general' | 'JEV'

const statusFilter = ref<ProviderStatusFilter>('all')
const kindFilter = ref<ProviderKindFilter>('all')
const kindFilterOptions = [
  { label: '全部类型', value: 'all' },
  { label: '通用大模型', value: 'general' },
  { label: 'JEV 决策模型', value: 'JEV' },
]

function providerAvailable(provider: Provider) {
  return provider.status === 'ACTIVE' && provider.last_test_ok && !providerBreakerOpen(provider)
}

function providerUnavailable(provider: Provider) {
  return provider.status === 'ACTIVE' && !providerAvailable(provider)
}

function providerRank(provider: Provider) {
  if (provider.status === 'ACTIVE' && providerAvailable(provider)) return 0
  if (provider.status === 'ACTIVE') return 1
  return 2
}

const sortedProviders = computed(() =>
  [...providers.value].sort((a, b) => providerRank(a) - providerRank(b)),
)

const statusFilters = computed(() => {
  const list = providers.value
  return [
    { key: 'all' as const, label: '全部', count: list.length, tone: 'all' },
    { key: 'active' as const, label: '已启用', count: list.filter((provider) => provider.status === 'ACTIVE').length, tone: 'enabled' },
    { key: 'available' as const, label: '可用', count: list.filter(providerAvailable).length, tone: 'available' },
    {
      key: 'unavailable' as const,
      label: '不可用',
      count: list.filter(providerUnavailable).length,
      tone: 'unavailable',
      hint: '已启用但测试失败、尚未测试或熔断中',
    },
    { key: 'disabled' as const, label: '已停用', count: list.filter((provider) => provider.status !== 'ACTIVE').length, tone: 'disabled' },
  ]
})

const filteredProviders = computed(() => sortedProviders.value.filter((provider) => {
  if (kindFilter.value === 'general' && provider.kind === 'JEV') return false
  if (kindFilter.value === 'JEV' && provider.kind !== 'JEV') return false
  if (statusFilter.value === 'active') return provider.status === 'ACTIVE'
  if (statusFilter.value === 'available') return providerAvailable(provider)
  if (statusFilter.value === 'unavailable') return providerUnavailable(provider)
  if (statusFilter.value === 'disabled') return provider.status !== 'ACTIVE'
  return true
}))

const batchTestTargets = computed(() =>
  filteredProviders.value.filter((provider) => provider.status === 'ACTIVE'),
)

function resetProviderFilters() {
  statusFilter.value = 'all'
  kindFilter.value = 'all'
}

watch(() => form.supportsThinking, (value) => {
  if (!value) form.preferReasoning = false
})

function formatTokenCount(value?: number) {
  if (!value || value <= 0) return '—'
  if (value >= 1_000_000) {
    const millions = value / 1_000_000
    return `${Number.isInteger(millions) ? millions : millions.toFixed(1)}M`
  }
  if (value >= 1_000) return `${Math.round(value / 1_000)}K`
  return String(value)
}

function modelOptionLabel(model: ProviderModel) {
  const parts = [model.id]
  if (model.context_window > 0) parts.push(`${formatTokenCount(model.context_window)} 上下文`)
  if (model.supports_vision) parts.push('图片')
  if (model.supports_thinking) parts.push('思考')
  return parts.join(' · ')
}

function modelSourceLabel(source?: string) {
  return source === 'metadata' ? '接口返回' : '能力未提供，请手动填写'
}

function parseProviderHeaders(text: string) {
  const headers: Record<string, string> = {}
  for (const line of text.split(/\r?\n/)) {
    const index = line.indexOf(':')
    if (index <= 0) continue
    const key = line.slice(0, index).trim()
    const value = line.slice(index + 1).trim()
    if (key) headers[key] = value
  }
  return headers
}

function headersToText(headers?: Record<string, string>) {
  return Object.entries(headers || {})
    .map(([key, value]) => `${key}: ${value}`)
    .join('\n')
}

function applySelectedModel() {
  const model = selectedModel.value
  if (!model) return
  form.supportsVision = model.vision_known ? model.supports_vision : false
  form.supportsThinking = model.thinking_known ? model.supports_thinking : false
  if (!model.thinking_known || !model.supports_thinking) form.preferReasoning = false
  form.contextWindow = model.context_window_known && model.context_window > 0 ? model.context_window : null
  form.maxTokens = model.max_output_known && model.max_output_tokens > 0 ? model.max_output_tokens : null
}

const kindDefaults: Record<string, {
  baseUrl: string
  model: string
  name: string
  prompt: string
  supportsVision: boolean
  maxTokens: number | null
  maxImages: number
  temperature: number
}> = {
  OPENAI_COMPATIBLE: {
    baseUrl: 'https://api.openai.com/v1',
    model: 'gpt-4o-mini',
    name: '通用大模型',
    prompt: '',
    supportsVision: false,
    maxTokens: null,
    maxImages: 6,
    temperature: 0.1,
  },
  ANTHROPIC: {
    baseUrl: 'https://api.anthropic.com/v1',
    model: 'claude-sonnet-4-5',
    name: '通用大模型',
    prompt: '',
    supportsVision: true,
    maxTokens: null,
    maxImages: 6,
    temperature: 0.1,
  },
  JEV: {
    baseUrl: 'https://api.typesafe.ai/v1',
    model: 'jev-latest',
    name: 'JEV 决策模型',
    prompt: '使用 TypeSafe Jev System One 进行结构化裁决，输出 Choice/Noul 概率，不生成自由文本。',
    supportsVision: true,
    maxTokens: null,
    maxImages: 8,
    temperature: 0,
  },
}

function resetForm() {
  form.name = ''
  form.kind = 'OPENAI_COMPATIBLE'
  form.baseUrl = 'https://api.openai.com/v1'
  form.model = 'gpt-4o-mini'
  form.apiKey = ''
  form.prompt = ''
  form.supportsVision = false
  form.supportsThinking = false
  form.preferReasoning = false
  form.contextWindow = null
  form.maxTokens = null
  form.maxImages = 6
  form.temperature = 0.1
  form.headersText = ''
  form.status = 'ACTIVE'
  modelOptions.value = []
  modelsError.value = ''
  modelsLoading.value = false
  errorText.value = ''
  editing.value = null
}

function chooseKind(kind: string) {
  if (editing.value) return
  const previous = kindDefaults[form.kind] || kindDefaults.OPENAI_COMPATIBLE
  const next = kindDefaults[kind] || kindDefaults.OPENAI_COMPATIBLE
  const keepBase = form.baseUrl && form.baseUrl !== previous.baseUrl
  const keepModel = form.model && form.model !== previous.model
  const keepName = form.name && form.name !== previous.name
  const keepPrompt = form.prompt && form.prompt !== previous.prompt
  form.kind = kind
  if (!keepBase) form.baseUrl = next.baseUrl
  if (!keepModel) form.model = next.model
  if (!keepName) form.name = next.name
  if (!keepPrompt) form.prompt = next.prompt
  form.supportsVision = next.supportsVision
  form.supportsThinking = false
  form.preferReasoning = false
  form.contextWindow = null
  form.maxTokens = next.maxTokens
  form.maxImages = next.maxImages
  form.temperature = next.temperature
  modelOptions.value = []
  modelsError.value = ''
}

function openCreate() {
  resetForm()
  if (!workspace.currentProjectId) {
    message.warning('请先创建并选择一个项目')
    return
  }
  form.status = 'DISABLED'
  modalShow.value = true
}

function openEdit(provider: Provider) {
  editing.value = provider
  form.name = provider.name
  form.kind = provider.kind
  form.baseUrl = provider.base_url
  form.model = provider.model
  form.apiKey = ''
  form.prompt = provider.prompt || ''
  form.supportsVision = provider.kind === 'JEV' ? true : Boolean(provider.config?.supports_vision)
  form.supportsThinking = Boolean(provider.config?.supports_thinking)
  form.preferReasoning = Boolean(provider.config?.prefer_reasoning)
  form.contextWindow = provider.config?.context_window ?? null
  form.maxTokens = provider.config?.max_tokens && provider.config.max_tokens > 0 ? provider.config.max_tokens : null
  form.maxImages = provider.config?.max_images ?? 6
  form.temperature = provider.config?.temperature ?? 0.1
  form.headersText = headersToText(provider.config?.headers)
  form.status = provider.status
  modelOptions.value = []
  modelsError.value = ''
  errorText.value = ''
  modalShow.value = true
}

async function load() {
  if (!workspace.currentProjectId) {
    providers.value = []
    return
  }
  loading.value = true
  try {
    const data = await providerApi.list({ project_id: workspace.currentProjectId })
    providers.value = data.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载模型供应商失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  errorText.value = ''
  if (!workspace.currentProjectId) {
    errorText.value = '请先选择项目'
    return
  }
  if (!form.name.trim()) {
    errorText.value = '请填写接入名称'
    return
  }
  if (!form.baseUrl.trim() || !form.model.trim()) {
    errorText.value = 'BaseURL 和模型 ID 不能为空'
    return
  }
  if (!editing.value && !form.apiKey.trim()) {
    errorText.value = '首次接入需要填写 API Key'
    return
  }
  const config = {
    supports_vision: form.supportsVision,
    supports_thinking: form.supportsThinking,
    prefer_reasoning: form.supportsThinking && form.preferReasoning,
    context_window: form.contextWindow ?? 0,
    max_tokens: form.maxTokens ?? 0,
    temperature: form.temperature,
    max_images: form.maxImages,
    headers: parseProviderHeaders(form.headersText),
  }
  const payload: Record<string, unknown> = {
    project_id: workspace.currentProjectId,
    name: form.name.trim(),
    kind: form.kind,
    base_url: form.baseUrl.trim(),
    model: form.model.trim(),
    prompt: form.prompt,
    config,
    status: form.status || (form.apiKey ? 'ACTIVE' : 'DISABLED'),
  }
  if (form.apiKey.trim()) payload.api_key = form.apiKey.trim()

  saving.value = true
  try {
    if (editing.value) {
      await providerApi.update(editing.value.id, payload)
      message.success('模型接入已更新')
    } else {
      await providerApi.create(payload)
      message.success('模型接入已创建')
    }
    modalShow.value = false
    await load()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function fetchModels() {
  if (isJEV.value) return
  if (!workspace.currentProjectId) {
    modelsError.value = '请先选择项目'
    return
  }
  if (!form.baseUrl.trim()) {
    modelsError.value = '请先填写 BaseURL'
    return
  }
  if (!editing.value && !form.apiKey.trim()) {
    modelsError.value = '请先填写 API Key'
    return
  }
  modelsLoading.value = true
  modelsError.value = ''
  try {
    const data = await providerApi.models({
      project_id: workspace.currentProjectId,
      provider_id: editing.value?.id,
      kind: form.kind,
      base_url: form.baseUrl.trim(),
      api_key: form.apiKey.trim() || undefined,
      headers: parseProviderHeaders(form.headersText),
    })
    modelOptions.value = data.items
    if (!data.items.length) {
      modelsError.value = '供应商没有返回可用模型'
      return
    }
    if (!data.items.some((item) => item.id === form.model)) {
      form.model = data.items[0].id
    }
    applySelectedModel()
    message.success(`已获取 ${data.items.length} 个模型`)
  } catch (error) {
    modelsError.value = error instanceof Error ? error.message : '获取模型列表失败'
  } finally {
    modelsLoading.value = false
  }
}

async function runProviderTest(providerId: number) {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), PROVIDER_TEST_TIMEOUT_MS)
  try {
    return await providerApi.test(providerId, controller.signal)
  } catch (error) {
    if (controller.signal.aborted) {
      throw new Error(`连接测试超时（${Math.round(PROVIDER_TEST_TIMEOUT_MS / 1000)} 秒），请检查网络或模型服务`)
    }
    throw error
  } finally {
    window.clearTimeout(timer)
  }
}

async function test(provider: Provider) {
  testingId.value = provider.id
  try {
    const result = await runProviderTest(provider.id)
    message.success(`连接成功 · ${result.latency_ms} ms${result.model ? ` · ${result.model}` : ''}`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '连接测试失败', { duration: 6000 })
  } finally {
    testingId.value = null
    await load()
  }
}

async function testAllProviders() {
  if (testingAll.value || testingId.value !== null) return
  const targets = batchTestTargets.value.slice()
  if (!targets.length) return
  testingAll.value = true
  testAllDone.value = 0
  testAllTotal.value = targets.length
  let success = 0
  let failed = 0
  let timedOut = 0
  let cursor = 0

  async function worker() {
    while (true) {
      const index = cursor
      cursor += 1
      if (index >= targets.length) return
      try {
        await runProviderTest(targets[index].id)
        success += 1
      } catch (error) {
        failed += 1
        if (error instanceof Error && error.message.includes('超时')) timedOut += 1
      } finally {
        testAllDone.value += 1
      }
    }
  }

  try {
    await Promise.all(
      Array.from({ length: Math.min(4, targets.length) }, () => worker()),
    )
    await load()
    const timeoutNote = timedOut > 0 ? `（其中 ${timedOut} 个超时）` : ''
    if (failed === 0) {
      message.success(`连接测试完成：${success} 个已启用模型全部正常`)
    } else if (success === 0) {
      message.error(`连接测试完成：${failed} 个已启用模型全部失败${timeoutNote}`, { duration: 6000 })
    } else {
      message.warning(`连接测试完成：${success} 个正常，${failed} 个失败${timeoutNote}`, { duration: 6000 })
    }
  } finally {
    testingAll.value = false
  }
}

async function toggle(provider: Provider, value: boolean) {
  try {
    await providerApi.update(provider.id, {
      name: provider.name,
      kind: provider.kind,
      base_url: provider.base_url,
      model: provider.model,
      prompt: provider.prompt,
      config: provider.config,
      status: value ? 'ACTIVE' : 'DISABLED',
    })
    provider.status = value ? 'ACTIVE' : 'DISABLED'
    message.success(value ? '供应商已启用' : '供应商已停用')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
    await load()
  }
}

function remove(provider: Provider) {
  dialog.warning({
    title: '删除模型接入',
    content: `确认删除「${provider.name}」？被审核配置引用的供应商无法删除。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await providerApi.remove(provider.id)
        message.success('已删除')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      }
    },
  })
}

function maskKey(provider: Provider) {
  return provider.api_key_masked || (provider.has_api_key ? '已配置密钥' : '未配置')
}

function formatTime(value?: string | null) {
  if (!value) return ''
  return value.replace('T', ' ').slice(0, 19)
}

watch(() => workspace.currentProjectId, () => {
  resetProviderFilters()
  void load()
})
onMounted(load)
</script>

<template>
  <div class="page providers-page">
    <PageHeader
      eyebrow="MODEL GATEWAY"
      title="模型接入"
      subtitle="统一管理通用大模型与 JEV 决策模型。通用大模型是一个业务分类，OpenAI 兼容 / Anthropic 只是两种 API 协议。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" :disabled="testingAll" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-popover trigger="click" placement="bottom-end" :width="440" :show-arrow="false">
          <template #trigger>
            <n-button quaternary>
              <template #icon><HelpCircleOutline /></template>
              类型说明
            </n-button>
          </template>
          <div class="mode-help">
            <div class="mode-card generic">
              <span class="mode-icon"><CloudOutline /></span>
              <div>
                <div class="mode-title">通用大模型 <span>OpenAI 兼容 · Anthropic</span></div>
                <p>统一的大模型分类，协议只是不同的 API 接入方式；按模型能力决定是否支持图片理解与思考。</p>
              </div>
            </div>
            <div class="mode-card jev">
              <span class="mode-icon"><SparklesOutline /></span>
              <div>
                <div class="mode-title">JEV 决策模型 <span>TypeSafe System One</span></div>
                <p>结构化裁决引擎，原生支持文本 + 图片，输出 Choice / Score / Noul 与置信度，不生成自由文本。</p>
              </div>
            </div>
          </div>
        </n-popover>
        <n-button type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新增模型接入
        </n-button>
      </template>
    </PageHeader>

    <div v-if="!workspace.currentProjectId" class="panel">
      <EmptyState title="请选择项目" description="模型接入按项目隔离，先在左上角选择一个项目。" action="前往项目空间" @action="$router.push({ name: 'projects' })" />
    </div>

    <template v-else>
      <div v-if="providers.length" class="provider-summary">
        <div class="summary-chips">
          <span class="summary-title">状态概览</span>
          <button
            v-for="filter in statusFilters"
            :key="filter.key"
            type="button"
            class="summary-chip"
            :class="[filter.tone, { 'is-selected': statusFilter === filter.key }]"
            :title="filter.hint"
            :aria-pressed="statusFilter === filter.key"
            @click="statusFilter = filter.key"
          >
            <span class="summary-dot"></span>
            <span>{{ filter.label }}</span>
            <strong>{{ filter.count }}</strong>
          </button>
        </div>
        <div class="summary-actions">
          <n-button
            size="small"
            secondary
            :loading="testingAll"
            :disabled="loading || !batchTestTargets.length || testingId !== null"
            :title="batchTestTargets.length ? `仅测试当前筛选中的 ${batchTestTargets.length} 个已启用连接` : '当前筛选没有已启用的连接'"
            @click="testAllProviders"
          >
            <template #icon><FlashOutline /></template>
            {{ testingAll ? `测试中 ${testAllDone}/${testAllTotal}` : `测试已启用连接（${batchTestTargets.length}）` }}
          </n-button>
          <n-button
            v-if="statusFilter !== 'all' || kindFilter !== 'all'"
            text
            size="tiny"
            type="primary"
            @click="resetProviderFilters"
          >
            重置筛选
          </n-button>
          <n-select v-model:value="kindFilter" :options="kindFilterOptions" size="small" class="summary-kind-select" />
        </div>
      </div>

      <div v-if="filteredProviders.length" class="provider-grid fade-list">
        <article
          v-for="provider in filteredProviders"
          :key="provider.id"
          class="provider-card"
          :class="provider.kind === 'JEV' ? 'jev' : provider.kind === 'ANTHROPIC' ? 'anthropic' : 'generic'"
        >
          <div class="provider-top">
            <span class="provider-icon">
              <SparklesOutline v-if="provider.kind === 'JEV'" />
              <CloudOutline v-else />
            </span>
            <div class="provider-heading">
              <div class="provider-name-row">
                <h3>{{ provider.name }}</h3>
                <StatusTag :status="provider.kind" type="kind" />
                <StatusTag :status="provider.status" type="provider" />
                <span v-if="providerBreakerOpen(provider)" class="provider-breaker" :title="provider.breaker_reason">
                  熔断中 · {{ providerBreakerLabel(provider) }}
                </span>
              </div>
              <span class="provider-model mono">{{ provider.model }}</span>
              <span class="provider-protocol">{{ providerProtocolLabel(provider.kind) }}</span>
            </div>
            <n-switch
              :value="provider.status === 'ACTIVE'"
              size="small"
              @update:value="toggle(provider, Boolean($event))"
            />
          </div>

          <div class="provider-endpoint">
            <LinkOutline />
            <span class="mono">{{ provider.base_url }}</span>
          </div>

          <div class="provider-capabilities">
            <span :class="{ on: provider.kind === 'JEV' || provider.config?.supports_vision }">
              <ImagesOutline />
              {{ provider.kind === 'JEV' ? '文本 + 图片' : provider.config?.supports_vision ? '支持图片' : '纯文本' }}
            </span>
            <span v-if="provider.config?.context_window">
              <CloudOutline />
              {{ formatTokenCount(provider.config.context_window) }} 上下文
            </span>
            <span v-if="provider.config?.supports_thinking" class="on">
              <SparklesOutline />
              支持思考
            </span>
            <span v-if="provider.config?.prefer_reasoning" class="on">
              <SparklesOutline />
              优先推理
            </span>
            <span v-if="provider.kind !== 'JEV'">
              <FlashOutline />
              max {{ provider.config?.max_tokens || 2200 }} tokens
            </span>
            <span v-else>
              <FlashOutline />
              Choice · Noul
            </span>
          </div>

          <div class="provider-health">
            <div class="health-main">
              <span class="health-label">连通状态</span>
              <div v-if="provider.last_test_at" class="health-result" :class="{ ok: provider.last_test_ok }">
                <component :is="provider.last_test_ok ? CheckmarkCircleOutline : CloseCircleOutline" />
                <strong>{{ provider.last_test_ok ? `正常 · ${provider.last_test_ms}ms` : '测试失败' }}</strong>
                <span>{{ formatTime(provider.last_test_at) }}</span>
              </div>
              <div v-else class="health-result idle">
                <WarningOutline />
                <strong>尚未测试</strong>
                <span>保存后执行连接测试</span>
              </div>
            </div>
            <div class="health-key">
              <span>密钥</span>
              <code>{{ maskKey(provider) }}</code>
            </div>
          </div>

          <div v-if="provider.last_test_message && !provider.last_test_ok" class="provider-error">
            {{ provider.last_test_message }}
          </div>

          <div class="provider-actions">
            <n-button
              size="small"
              :loading="testingId === provider.id"
              :disabled="testingId !== null || testingAll"
              @click="test(provider)"
            >
              <template #icon><FlashOutline /></template>
              测试连接
            </n-button>
            <n-button size="small" quaternary @click="openEdit(provider)">编辑</n-button>
            <n-button size="small" quaternary class="delete-btn" @click="remove(provider)">
              <template #icon><TrashOutline /></template>
            </n-button>
          </div>
        </article>
      </div>

      <EmptyState
        v-else-if="!loading && providers.length > 0"
        title="没有符合条件的模型接入"
        description="调整状态或类型筛选后再试，或清除筛选查看全部模型。"
        action="清除筛选"
        @action="resetProviderFilters"
      />

      <EmptyState
        v-if="!loading && providers.length === 0"
        title="还没有模型接入"
        description="接入通用大模型（OpenAI 兼容 / Anthropic 协议）或 JEV 决策模型，AI 审核流程才能运行。"
        action="新增模型接入"
        @action="openCreate"
      />
    </template>

    <n-modal v-model:show="modalShow" :mask-closable="!saving">
      <div class="provider-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT PROVIDER' : 'NEW PROVIDER' }}</div>
            <h2>{{ editing ? '编辑模型接入' : '新增模型接入' }}</h2>
            <p>密钥会使用 AES-256-GCM 加密后存储，接口仅返回脱敏值。</p>
          </div>
          <span class="modal-icon"><LinkOutline /></span>
        </div>

        <div class="modal-body">
          <div class="kind-selector">
            <button
              type="button"
              class="kind-card"
              :class="{ active: !isJEV, disabled: Boolean(editing) }"
              @click="chooseKind('OPENAI_COMPATIBLE')"
            >
              <span class="kind-icon generic"><CloudOutline /></span>
              <span>
                <strong>通用大模型</strong>
                <small>OpenAI 兼容 / Anthropic Messages，协议可切换</small>
              </span>
              <span v-if="!isJEV" class="kind-check">✓</span>
            </button>
            <button
              type="button"
              class="kind-card"
              :class="{ active: form.kind === 'JEV', disabled: Boolean(editing) }"
              @click="chooseKind('JEV')"
            >
              <span class="kind-icon jev"><SparklesOutline /></span>
              <span>
                <strong>JEV 决策模型</strong>
                <small>TypeSafe System One · Choice/Noul</small>
              </span>
              <span v-if="form.kind === 'JEV'" class="kind-check">✓</span>
            </button>
          </div>

          <div v-if="!isJEV" class="protocol-row">
            <span class="protocol-label">API 协议</span>
            <button
              type="button"
              class="protocol-option"
              :class="{ active: form.kind === 'OPENAI_COMPATIBLE' }"
              :disabled="Boolean(editing)"
              @click="chooseKind('OPENAI_COMPATIBLE')"
            >
              OpenAI 兼容 · /chat/completions
            </button>
            <button
              type="button"
              class="protocol-option"
              :class="{ active: form.kind === 'ANTHROPIC' }"
              :disabled="Boolean(editing)"
              @click="chooseKind('ANTHROPIC')"
            >
              Anthropic · /messages
            </button>
            <span v-if="editing" class="protocol-locked">协议创建后不可变更</span>
          </div>

          <div v-if="isJEV" class="jev-note">
            <SparklesOutline />
            <div>
              <strong>JEV 原生支持文本 + 图片，采用结构化问题而非提示词生成</strong>
              <p>平台会向 <code>/v1/systemone</code> 发送一个 Choice 裁决问题与两个 Noul 风险问题，文本与图片资产可同时提交；结果由概率和置信度归一化为风险分。</p>
            </div>
          </div>

          <div class="two-col">
            <div class="field">
              <label>接入名称 <span class="required">*</span></label>
              <n-input v-model:value="form.name" placeholder="例如 通用大模型 / JEV 生产" />
            </div>
            <div class="field">
              <label>状态</label>
              <n-select
                v-model:value="form.status"
                :options="[
                  { label: '启用', value: 'ACTIVE' },
                  { label: '停用', value: 'DISABLED' },
                ]"
              />
            </div>
          </div>

          <div class="field">
            <label>BaseURL <span class="required">*</span></label>
            <n-input v-model:value="form.baseUrl" placeholder="https://api.example.com/v1" />
            <div class="form-hint">{{ baseUrlHint }}</div>
          </div>

          <div class="field">
            <label>模型 ID <span class="required">*</span></label>
            <div class="model-picker">
              <n-select
                v-if="modelOptions.length"
                v-model:value="form.model"
                filterable
                tag
                :options="modelSelectOptions"
                placeholder="选择或输入模型 ID"
                @update:value="applySelectedModel"
              />
              <n-input v-else v-model:value="form.model" placeholder="gpt-4o-mini / claude-sonnet-4-5 / jev-latest" />
              <n-button v-if="!isJEV" secondary :loading="modelsLoading" @click="fetchModels">
                <template #icon><RefreshOutline /></template>
                获取模型
              </n-button>
            </div>
            <div v-if="modelsError" class="form-hint model-picker-error">{{ modelsError }}</div>
            <div v-if="selectedModel" class="model-caps">
              <span>上下文 <strong>{{ selectedModel.context_window_known ? formatTokenCount(selectedModel.context_window) : '未提供' }}</strong></span>
              <span>最大输出 <strong>{{ selectedModel.max_output_known ? formatTokenCount(selectedModel.max_output_tokens) : '未提供' }}</strong></span>
              <span :class="{ on: selectedModel.vision_known && selectedModel.supports_vision }">
                <ImagesOutline /> 图片 {{ selectedModel.vision_known ? (selectedModel.supports_vision ? '支持' : '不支持') : '未提供' }}
              </span>
              <span :class="{ on: selectedModel.thinking_known && selectedModel.supports_thinking }">
                <SparklesOutline /> 思考 {{ selectedModel.thinking_known ? (selectedModel.supports_thinking ? '支持' : '不支持') : '未提供' }}
              </span>
              <span class="model-caps-source">{{ modelSourceLabel(selectedModel.source) }}</span>
            </div>
            <div v-else-if="!isJEV" class="model-caps">
              <span>上下文 <strong>{{ form.contextWindow != null ? formatTokenCount(form.contextWindow) : '未填写' }}</strong></span>
              <span>最大输出 <strong>{{ form.maxTokens != null ? formatTokenCount(form.maxTokens) : '未填写' }}</strong></span>
              <span :class="{ on: form.supportsVision }">
                <ImagesOutline /> 图片 {{ form.supportsVision ? '支持' : '不支持' }}
              </span>
              <span :class="{ on: form.supportsThinking }">
                <SparklesOutline /> 思考 {{ form.supportsThinking ? '支持' : '不支持' }}
              </span>
            </div>
            <div class="form-hint">选择模型后自动带入上下文、最大输出、图片理解与思考能力，仍可手动调整。</div>
          </div>

          <div class="field">
            <label>API Key <span v-if="!editing" class="required">*</span></label>
            <n-input
              v-model:value="form.apiKey"
              type="password"
              show-password-on="click"
              :placeholder="editing && editing.has_api_key ? `留空保留现有密钥（${editing.api_key_masked}）` : '输入供应商 API Key'"
            />
          </div>

          <div class="field">
            <label>自定义请求头</label>
            <n-input
              v-model:value="form.headersText"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 4 }"
              placeholder="x-opencode-session: {{random}}"
            />
            <div class="form-hint">系统会默认附带 <code>x-opencode-session</code> 随机会话 ID；如需固定值或补充其他 Header，可按每行一个填写。测试连接会强制使用随机值，且不会覆盖 Authorization / x-api-key。</div>
          </div>

          <div class="field">
            <label>补充提示词 / 审核说明</label>
            <n-input
              v-model:value="form.prompt"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 5 }"
              placeholder="可选。会与审核配置中的审核要求一起传给模型。"
            />
          </div>

          <div class="config-panel">
            <div class="config-title">推理参数</div>
            <div class="config-grid">
              <div class="switch-field">
                <div>
                  <strong>图片理解</strong>
                  <span>{{ isJEV ? 'JEV 原生支持文本 + 图片' : '通用模型需模型本身支持视觉输入' }}</span>
                </div>
                <n-switch v-model:value="form.supportsVision" :disabled="isJEV" />
              </div>
              <div class="switch-field">
                <div>
                  <strong>支持推理</strong>
                  <span>模型本身支持推理 / 思考时开启</span>
                </div>
                <n-switch v-model:value="form.supportsThinking" :disabled="isJEV" />
              </div>
              <div class="switch-field">
                <div>
                  <strong>优先使用推理</strong>
                  <span>失败切换时优先选择支持推理的模型</span>
                </div>
                <n-switch v-model:value="form.preferReasoning" :disabled="!form.supportsThinking" />
              </div>
              <div class="switch-field">
                <div>
                  <strong>最大图片数</strong>
                  <span>超出时仅取前 N 张</span>
                </div>
                <n-input-number v-model:value="form.maxImages" :min="0" :max="20" size="small" style="width: 86px" />
              </div>
              <div class="switch-field">
                <div>
                  <strong>上下文窗口</strong>
                  <span>模型可接收的上下文长度</span>
                </div>
                <n-input-number v-model:value="form.contextWindow" :min="0" :max="10000000" :step="1000" size="small" style="width: 120px" />
              </div>
              <div class="switch-field">
                <div>
                  <strong>最大输出 Token</strong>
                  <span>按所选模型规格填写，JEV 无需设置</span>
                </div>
                <n-input-number v-model:value="form.maxTokens" :min="0" :max="200000" :step="1000" size="small" style="width: 120px" />
              </div>
              <div class="switch-field">
                <div>
                  <strong>温度</strong>
                  <span>0 ~ 1，越低越稳定</span>
                </div>
                <n-input-number v-model:value="form.temperature" :min="0" :max="1" :step="0.1" size="small" style="width: 86px" />
              </div>
            </div>
          </div>

          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>

        <div class="modal-footer">
          <div v-if="activeProvider" class="test-hint">上次测试：{{ activeProvider.last_test_message || '—' }}</div>
          <div class="footer-buttons">
            <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
            <n-button type="primary" :loading="saving" @click="save">保存接入</n-button>
          </div>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.mode-help {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mode-help .mode-card {
  padding: 12px 14px;
  box-shadow: none;
}

.mode-help .mode-title {
  font-size: 12px;
}

.mode-help .mode-card p {
  font-size: 10px;
}

.provider-modal {
  width: min(680px, calc(100vw - 24px));
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.mode-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 17px 19px;
  border: 1px solid var(--border);
  border-radius: 15px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.mode-card.jev {
  border-color: #e8e2fb;
  background: linear-gradient(120deg, #fbfaff, #fff);
}

.mode-icon {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.mode-card.jev .mode-icon {
  background: var(--purple-soft);
  color: var(--purple);
}

.mode-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.mode-title {
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 600;
}

.mode-title span {
  margin-left: 5px;
  color: var(--muted-light);
  font-size: 9.5px;
  font-weight: 400;
}

.mode-card p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.65;
}

.provider-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 14px;
}

.summary-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.summary-title {
  margin-right: 2px;
  color: var(--muted-light);
  font-size: 10px;
  font-weight: 600;
}

.summary-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: #fff;
  color: var(--muted);
  cursor: pointer;
  font-size: 11px;
  transition: border-color 0.16s ease, background 0.16s ease, color 0.16s ease, box-shadow 0.16s ease;
}

.summary-chip strong {
  color: var(--ink);
  font-size: 11px;
  font-weight: 650;
}

.summary-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #cbd5e1;
}

.summary-chip.enabled .summary-dot {
  background: var(--brand);
}

.summary-chip.available .summary-dot {
  background: var(--green);
}

.summary-chip.unavailable .summary-dot {
  background: var(--red);
}

.summary-chip.disabled .summary-dot {
  background: var(--muted-light);
}

.summary-chip:hover {
  border-color: #c7d9fb;
  background: #f8fbff;
}

.summary-chip.is-selected {
  border-color: var(--brand);
  background: var(--brand-soft);
  color: var(--brand-strong);
  box-shadow: 0 0 0 3px rgba(43, 108, 246, 0.08);
}

.summary-chip.is-selected strong {
  color: var(--brand-strong);
}

.summary-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-left: auto;
}

.summary-kind-select {
  width: 150px;
}

.provider-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.provider-card {
  display: flex;
  flex-direction: column;
  padding: 20px;
  border: 1px solid var(--border);
  border-radius: 17px;
  background: #fff;
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.18s ease, transform 0.18s ease;
}

.provider-card:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow-md);
}

.provider-card.jev {
  border-color: #ebe6fa;
}

.provider-card.anthropic {
  border-color: #e0f2eb;
}

.provider-card.anthropic .provider-icon {
  background: linear-gradient(145deg, #eefbf6, #dff5eb);
  color: #0f766e;
}

.provider-top {
  display: flex;
  align-items: center;
  gap: 11px;
}

.provider-icon {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: linear-gradient(145deg, #edf4ff, #e0ebff);
  color: var(--brand);
  flex-shrink: 0;
}

.provider-card.jev .provider-icon {
  background: linear-gradient(145deg, #f5f1ff, #ebe1ff);
  color: var(--purple);
}

.provider-icon :deep(svg) {
  width: 20px;
  height: 20px;
}

.provider-heading {
  min-width: 0;
  flex: 1;
}

.provider-name-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.provider-name-row h3 {
  overflow: hidden;
  margin: 0;
  color: var(--ink);
  font-size: 14.5px;
  font-weight: 620;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-model {
  display: block;
  overflow: hidden;
  margin-top: 1px;
  color: var(--muted-light);
  font-size: 9.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-protocol {
  display: inline-block;
  margin-top: 3px;
  padding: 1px 6px;
  border-radius: 99px;
  background: #eef3fc;
  color: #4b6a9b;
  font-size: 8.5px;
}

.provider-breaker {
  padding: 1px 6px;
  border: 1px solid #fecaca;
  border-radius: 99px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 8.5px;
  font-weight: 600;
  white-space: nowrap;
}

.provider-endpoint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 14px 0 12px;
  padding: 7px 9px;
  border-radius: 8px;
  background: #f8fafc;
  color: var(--muted);
  font-size: 10px;
}

.provider-endpoint :deep(svg) {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
}

.provider-endpoint span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-capabilities {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.provider-capabilities span {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 6px;
  background: #f6f8fb;
  color: #7c8798;
  font-size: 9.5px;
}

.provider-capabilities span.on {
  background: #f2f7ff;
  color: #3f69bb;
}

.provider-capabilities :deep(svg) {
  width: 11px;
  height: 11px;
}

.provider-health {
  display: flex;
  align-items: flex-start;
  gap: 18px;
  margin-top: 15px;
  padding-top: 13px;
  border-top: 1px dashed #edf1f7;
}

.health-main {
  min-width: 0;
  flex: 1;
}

.health-label,
.health-key > span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
  letter-spacing: 0.06em;
}

.health-result {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  margin-top: 4px;
  color: var(--muted);
  font-size: 9.5px;
}

.health-result :deep(svg) {
  width: 13px;
  height: 13px;
  color: var(--muted-light);
}

.health-result.ok :deep(svg) {
  color: var(--green);
}

.health-result.ok strong {
  color: #15803d;
}

.health-result strong {
  font-size: 10.5px;
  font-weight: 550;
}

.health-result span {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.5px;
}

.health-result.idle :deep(svg) {
  color: #d97706;
}

.health-key {
  flex-shrink: 0;
  text-align: right;
}

.health-key code {
  display: block;
  margin-top: 4px;
  padding: 2px 6px;
  border-radius: 5px;
  background: #f8fafc;
  color: #64748b;
  font-family: var(--mono);
  font-size: 9px;
}

.provider-error {
  margin-top: 9px;
  padding: 7px 9px;
  border-radius: 7px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 9.5px;
  line-height: 1.5;
}

.provider-actions {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-top: auto;
  padding-top: 14px;
}

.provider-actions .delete-btn {
  margin-left: auto;
  color: var(--muted-light);
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
  color: white;
  box-shadow: 0 8px 18px -7px rgba(43, 108, 246, 0.55);
  flex-shrink: 0;
}

.modal-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.modal-body {
  max-height: min(70vh, 660px);
  overflow-y: auto;
  padding: 20px 26px;
}

.kind-selector {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 12px;
}

.protocol-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 18px;
  padding: 8px 10px;
  border: 1px dashed #dfe7f3;
  border-radius: 9px;
  background: #fafcff;
}

.protocol-label {
  color: var(--muted-light);
  font-size: 9px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.protocol-option {
  padding: 4px 9px;
  border: 1px solid #e3e9f4;
  border-radius: 7px;
  background: #fff;
  color: var(--muted);
  font-family: inherit;
  font-size: 9.5px;
  cursor: pointer;
}

.protocol-option.active {
  border-color: #aac6fb;
  background: #f2f7ff;
  color: #2b6cf6;
}

.protocol-option:disabled {
  cursor: default;
  opacity: 0.75;
}

.protocol-locked {
  color: var(--muted-light);
  font-size: 9px;
}

.model-picker {
  display: flex;
  align-items: center;
  gap: 8px;
}

.model-picker > :first-child {
  min-width: 0;
  flex: 1;
}

.model-caps {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 7px;
}

.model-caps > span {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border-radius: 99px;
  background: #f4f7fc;
  color: var(--muted);
  font-size: 9px;
}

.model-caps > span.on {
  background: #eefbf3;
  color: #15803d;
}

.model-caps svg {
  width: 11px;
  height: 11px;
}

.model-caps strong {
  color: var(--ink);
  font-weight: 600;
}

.model-caps-source {
  background: #f7f5ff !important;
  color: #6d5bb5 !important;
}

.model-picker-error {
  color: #dc2626;
}

.kind-card {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 11px 12px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: #fff;
  text-align: left;
  cursor: pointer;
  transition: all 0.16s ease;
  position: relative;
}

.kind-card.active {
  border-color: #aac6fb;
  background: #f8fbff;
  box-shadow: 0 0 0 3px rgba(43, 108, 246, 0.06);
}

.kind-card.disabled {
  cursor: default;
}

.kind-icon {
  display: flex;
  width: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.kind-icon.jev {
  background: var(--purple-soft);
  color: var(--purple);
}

.kind-icon.anthropic {
  background: #e8f8f2;
  color: #0f766e;
}

.kind-icon :deep(svg) {
  width: 15px;
  height: 15px;
}

.kind-card strong {
  display: block;
  color: var(--ink);
  font-size: 11.5px;
  font-weight: 550;
}

.kind-card small {
  display: block;
  color: var(--muted-light);
  font-size: 8.5px;
}

.kind-check {
  position: absolute;
  top: 7px;
  right: 8px;
  color: var(--brand);
  font-size: 10px;
  font-weight: 700;
}

.jev-note {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-bottom: 18px;
  padding: 11px 13px;
  border: 1px solid #ebe4fc;
  border-radius: 10px;
  background: #fbfaff;
  color: #5b21b6;
}

.jev-note :deep(svg) {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  margin-top: 1px;
}

.jev-note strong {
  display: block;
  font-size: 10.5px;
  font-weight: 600;
}

.jev-note p {
  margin: 3px 0 0;
  color: #7c6aa8;
  font-size: 9.5px;
  line-height: 1.6;
}

.jev-note code {
  font-family: var(--mono);
  font-size: 8.5px;
}

.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
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

.config-panel {
  padding: 14px 15px;
  border: 1px solid #edf1f7;
  border-radius: 12px;
  background: #fbfcfe;
}

.config-title {
  margin-bottom: 11px;
  color: var(--muted-light);
  font-size: 9.5px;
  font-weight: 650;
  letter-spacing: 0.09em;
  text-transform: uppercase;
}

.config-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 13px 20px;
}

.switch-field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.switch-field strong {
  display: block;
  color: var(--text);
  font-size: 11px;
  font-weight: 500;
}

.switch-field span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.4;
}

.modal-error {
  margin-top: 14px;
  padding: 9px 11px;
  border: 1px solid #fee2e2;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 11px;
}

.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 15px 26px;
  border-top: 1px solid #f0f3f8;
}

.test-hint {
  overflow: hidden;
  color: var(--muted-light);
  font-size: 9.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.footer-buttons {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

@media (max-width: 1100px) {
  .provider-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .two-col,
  .config-grid,
  .kind-selector {
    grid-template-columns: 1fr;
  }

  .provider-summary {
    align-items: stretch;
  }

  .summary-actions {
    width: 100%;
    margin-left: 0;
    justify-content: space-between;
  }

  .summary-kind-select {
    width: auto;
    flex: 1;
  }

  .provider-modal {
    width: calc(100vw - 16px);
    max-height: calc(100vh - 16px);
  }

  .modal-body {
    max-height: none;
  }

  .modal-footer {
    flex-wrap: wrap;
  }

  .footer-buttons {
    width: 100%;
    justify-content: flex-end;
  }
}

@media (max-width: 560px) {
  .modal-head,
  .modal-body,
  .modal-footer {
    padding-left: 18px;
    padding-right: 18px;
  }

  .test-hint {
    display: none;
  }
}
</style>
