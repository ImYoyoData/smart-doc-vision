<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton,
  NCheckbox,
  NCheckboxGroup,
  NDrawer,
  NDrawerContent,
  NInput,
  NInputNumber,
  NSelect,
  NSpin,
  NSwitch,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  CheckmarkCircleOutline,
  DownloadOutline,
  GlobeOutline,
  HelpCircleOutline,
  KeyOutline,
  LayersOutline,
  RefreshOutline,
  SendOutline,
  SparklesOutline,
  ShieldCheckmarkOutline,
  SwapHorizontalOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import CodeBlock from './CodeBlock.vue'
import CopyButton from './CopyButton.vue'
import StatusTag from './StatusTag.vue'
import { auditApi, redisConnectionApi } from '@/api'
import { copyText } from '@/utils/clipboard'
import {
  INTEGRATION_METHODS,
  buildMethodPrompt,
  buildSelectedPrompt,
  buildSkillPrompt,
  type IntegrationMethod,
  type IntegrationPromptContext,
} from '@/utils/integrationPrompt'
import { useAuthStore } from '@/stores/auth'
import type { IntegrationGuide, RedisConnection, TriggerSource } from '@/api/types'

const props = defineProps<{
  show: boolean
  auditId: number | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  updated: []
}>()

const message = useMessage()
const auth = useAuthStore()
const router = useRouter()
const loading = ref(false)
const saving = ref(false)
const guide = ref<IntegrationGuide | null>(null)
const activeTab = ref('overview')
const selectedCredentialId = ref<number | null>(null)
const testing = ref(false)
const aiMethods = ref<IntegrationMethod[]>(['OPENAPI'])

const tabs = [
  { key: 'overview', label: '总览', icon: KeyOutline },
  { key: 'openapi', label: '开放 API', icon: GlobeOutline },
  { key: 'redis', label: 'Redis', icon: LayersOutline },
  { key: 'websocket', label: 'WebSocket', icon: SwapHorizontalOutline },
  { key: 'callback', label: '结果回调', icon: SendOutline },
  { key: 'permissions', label: '权限边界', icon: ShieldCheckmarkOutline },
]

const redisForm = reactive({
  connection_id: null as number | null,
  db: 0,
  stream: '',
  group: '',
  start_position: '$',
  batch_size: 16,
  max_inflight: 1,
})
const connections = ref<RedisConnection[]>([])
const loadingConnections = ref(false)
const callbackForm = reactive({
  enabled: false,
  url: '',
  secret: '',
  results: ['REJECT'] as string[],
  include_content: true,
})
const showCallbackSecret = ref(false)

const connectionOptions = computed(() => connections.value.map((connection) => {
  const unavailable = connection.status !== 'ACTIVE' || !connection.last_test_ok || connection.health !== 'OK'
  const suffix = connection.status !== 'ACTIVE'
    ? ' · 已停用'
    : !connection.last_test_ok || connection.health !== 'OK'
      ? ' · 未通过测试'
      : ''
  return {
    label: `${connection.name} · ${connection.addr}${suffix}`,
    value: connection.id,
    disabled: unavailable,
  }
}))

const triggerMap = computed<Record<string, TriggerSource>>(() => {
  const map: Record<string, TriggerSource> = {}
  for (const trigger of guide.value?.triggers || []) map[trigger.type] = trigger
  return map
})

const credentialOptions = computed(() =>
  (guide.value?.credentials.eligible || []).map((credential) => ({
    label: `${credential.name} · ${credential.app_key}`,
    value: credential.id,
  })),
)
const selectedCredential = computed(() => guide.value?.credentials.selected || null)
const signedExamplesAvailable = computed(() => Boolean(selectedCredential.value))
const canWrite = computed(() => auth.can('audit:write'))
const appBase = import.meta.env.BASE_URL
const docsURL = computed(() => `${window.location.origin}${appBase}help`)

const openapiTrigger = computed(() => triggerMap.value.OPENAPI)
const redisTrigger = computed(() => triggerMap.value.REDIS)
const wsTrigger = computed(() => triggerMap.value.WEBSOCKET)

const wsURL = computed(() => {
  if (!guide.value) return ''
  const base = guide.value.openapi.base_url || window.location.origin
  const protocol = base.startsWith('https://') ? 'wss' : 'ws'
  return `${base.replace(/^https?:\/\//, `${protocol}://`)}${guide.value.websocket.signed_url}`
})

const endpointURL = computed(() => {
  if (!guide.value) return ''
  return `${guide.value.openapi.base_url || window.location.origin}${guide.value.openapi.path}`
})

const callbackSample = computed(() =>
  guide.value ? JSON.stringify(guide.value.callback.sample_payload, null, 2) : '',
)
const wsFrames = computed(() => {
  if (!guide.value) return ''
  return `// 提交\n${JSON.stringify(guide.value.websocket.submit_frame, null, 2)}\n\n// 受理\n${JSON.stringify(
    guide.value.websocket.ack_frame,
    null,
    2,
  )}`
})
const aiMethodOptions = INTEGRATION_METHODS
const aiCopyLabel = computed(() => {
  if (!aiMethods.value.length) return '请选择接入方式'
  if (aiMethods.value.length === 1) return `复制${aiMethodLabel(aiMethods.value[0])}提示词`
  return `复制 ${aiMethods.value.length} 种方式组合提示词`
})
const promptContext = computed<IntegrationPromptContext | null>(() => {
  const g = guide.value
  if (!g) return null
  return {
    origin: window.location.origin,
    projectCode: g.project.code,
    auditCode: g.audit.code,
    auditName: g.audit.name,
    docsURL: `${window.location.origin}${appBase}help`,
    credentialName: selectedCredential.value?.name,
    appKey: selectedCredential.value?.app_key,
    openapiPath: g.openapi.path,
    openapiEnabled: g.openapi.enabled,
    sampleBody: g.openapi.sample_body_raw,
    sampleCurl: g.openapi.sample_curl,
    redisEnabled: g.redis.enabled,
    redisStream: redisForm.stream || g.redis.stream,
    redisGroup: redisForm.group || g.redis.group,
    websocketEnabled: g.websocket.enabled,
    websocketPath: g.websocket.path,
    websocketURL: wsURL.value,
    callback: {
      enabled: g.callback.enabled,
      url: g.callback.url,
      results: g.callback.results,
      includeContent: g.callback.include_content,
      secretSet: g.callback.secret_set,
    },
    decisionMode: g.audit.decision_mode,
    categories: g.audit.categories || [],
  }
})

async function load(credentialId?: number | null) {
  if (!props.auditId) return
  loading.value = true
  try {
    const targetCredential = credentialId ?? selectedCredentialId.value
    guide.value = await auditApi.integration(props.auditId, targetCredential || undefined)
    selectedCredentialId.value = guide.value.credentials.selected?.id ?? null
    redisForm.connection_id = redisTrigger.value?.redis_connection_id || guide.value.redis.connection_id || null
    redisForm.db = redisTrigger.value?.redis_db ?? guide.value.redis.db ?? 0
    redisForm.stream = redisTrigger.value?.stream || guide.value.redis.stream || ''
    redisForm.group = redisTrigger.value?.consumer_group || guide.value.redis.group || ''
    redisForm.start_position = redisTrigger.value?.start_position || guide.value.redis.start_position || '$'
    redisForm.batch_size = redisTrigger.value?.batch_size || guide.value.redis.batch_size || 16
    redisForm.max_inflight = redisTrigger.value?.max_inflight || guide.value.redis.max_inflight || 1
    callbackForm.enabled = guide.value.callback.enabled
    callbackForm.url = guide.value.callback.url || ''
    callbackForm.results = guide.value.callback.results?.filter((r) => r !== 'REVIEW').length ? guide.value.callback.results.filter((r) => r !== 'REVIEW') : ['REJECT']
    callbackForm.include_content = guide.value.callback.include_content
    showCallbackSecret.value = Boolean(guide.value.callback.secret_set)
    await loadConnections()
    const activeMethods = (guide.value.triggers || [])
      .filter((trigger) => trigger.status === 'ACTIVE')
      .map((trigger) => trigger.type as IntegrationMethod)
      .filter((method) => INTEGRATION_METHODS.some((item) => item.key === method))
    aiMethods.value = activeMethods.length ? activeMethods : ['OPENAPI']
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载接入指南失败')
  } finally {
    loading.value = false
  }
}

async function loadConnections() {
  const projectID = guide.value?.project.id
  if (!projectID) return
  loadingConnections.value = true
  try {
    const data = await redisConnectionApi.list(projectID)
    connections.value = data.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载 Redis 连接失败')
  } finally {
    loadingConnections.value = false
  }
}

async function saveRedis(enable?: boolean): Promise<boolean> {
  if (!redisTrigger.value) return false
  const targetStatus = enable === undefined ? redisTrigger.value.status : enable ? 'ACTIVE' : 'DISABLED'
  if (targetStatus === 'ACTIVE') {
    if (!redisForm.connection_id) {
      message.warning('启用 Redis 触发前请选择项目 Redis 连接')
      return false
    }
    if (!redisForm.stream.trim()) {
      message.warning('请填写要监听的 Redis Stream')
      return false
    }
  }
  saving.value = true
  try {
    await auditApi.updateTrigger(redisTrigger.value.id, {
      status: targetStatus,
      redis_connection_id: redisForm.connection_id || undefined,
      redis_db: redisForm.db,
      stream: redisForm.stream.trim(),
      consumer_group: redisForm.group.trim(),
      start_position: redisForm.start_position,
      batch_size: redisForm.batch_size,
      max_inflight: redisForm.max_inflight,
    })
    message.success(targetStatus === 'ACTIVE' ? 'Redis 触发已启用' : 'Redis 触发配置已保存')
    await load()
    emit('updated')
    return true
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
    return false
  } finally {
    saving.value = false
  }
}

async function toggleTrigger(type: 'OPENAPI' | 'REDIS' | 'WEBSOCKET', value: boolean) {
  const trigger = triggerMap.value[type]
  if (!trigger) return
  if (type === 'REDIS') {
    await saveRedis(value)
    return
  }
  saving.value = true
  try {
    await auditApi.updateTrigger(trigger.id, { status: value ? 'ACTIVE' : 'DISABLED' })
    message.success(value ? '触发方式已启用' : '触发方式已停用')
    await load()
    emit('updated')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
    await load()
  } finally {
    saving.value = false
  }
}

async function saveCallback() {
  if (callbackForm.enabled && !callbackForm.url.trim()) {
    message.warning('启用回调前请填写回调地址')
    return
  }
  if (callbackForm.enabled && !callbackForm.results.length) {
    message.warning('启用回调时至少选择一个回调结论')
    return
  }
  saving.value = true
  try {
    const payload: Record<string, unknown> = {
      callback_enabled: callbackForm.enabled,
      callback_url: callbackForm.url.trim(),
      callback_results: callbackForm.results,
      callback_include_content: callbackForm.include_content,
    }
    if (callbackForm.secret) payload.callback_secret = callbackForm.secret
    await auditApi.update(props.auditId!, payload)
    message.success('回调配置已保存')
    callbackForm.secret = ''
    await load()
    emit('updated')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function openDocs(section = 'overview') {
  emit('update:show', false)
  window.location.href = `${appBase}help#help-${section}`
}

function goCredentials() {
  emit('update:show', false)
  router.push({ name: 'credentials' })
}

function selectCredential(value: number) {
  selectedCredentialId.value = value
  load(value)
}

function aiMethodLabel(method: IntegrationMethod): string {
  return INTEGRATION_METHODS.find((item) => item.key === method)?.label || method
}

function buildAIPrompt(method?: IntegrationMethod): string {
  const context = promptContext.value
  if (!context) return ''
  if (method) return buildMethodPrompt(method, context)
  if (!aiMethods.value.length) return ''
  return buildSelectedPrompt(aiMethods.value, context)
}

async function copyAIPrompt(method?: IntegrationMethod) {
  if (!method && !aiMethods.value.length) {
    message.warning('请至少选择一种接入方式')
    return
  }
  const text = buildAIPrompt(method)
  if (!text) return
  const ok = await copyText(text)
  if (ok) {
    message.success(
      method
        ? `${aiMethodLabel(method)}专用 AI 提示词已复制`
        : `已复制 ${aiMethods.value.length} 种方式的组合 AI 提示词`,
    )
  } else {
    message.error('复制失败，请手动下载 Skill 文档')
  }
}

function downloadAISkill() {
  const context = promptContext.value
  if (!context) return
  const text = buildSkillPrompt(context)
  const blob = new Blob([text], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `smartdoc-integration-${guide.value?.audit.code || 'audit'}.md`
  a.click()
  URL.revokeObjectURL(url)
}

function selectAllAiMethods() {
  aiMethods.value = INTEGRATION_METHODS.map((item) => item.key)
}

function clearAiMethods() {
  aiMethods.value = []
}

async function copyDocsURL() {
  const ok = await copyText(docsURL.value)
  if (ok) message.success('文档地址已复制')
}

function triggerLabel(type: string) {
  return { OPENAPI: '开放 API', REDIS: 'Redis Stream', WEBSOCKET: 'WebSocket' }[type] || type
}

watch(
  () => [props.show, props.auditId],
  ([visible, id]) => {
    if (visible && id) {
      activeTab.value = 'overview'
      selectedCredentialId.value = null
      load()
    } else {
      showCallbackSecret.value = false
    }
  },
  { immediate: true },
)
</script>

<template>
  <n-drawer :show="show" width="min(780px, 100vw)" placement="right" @update:show="emit('update:show', $event)">
    <n-drawer-content :native-scrollbar="false" closable>
      <template #header>
        <div class="guide-header">
          <div>
            <div class="guide-kicker">INTEGRATION</div>
            <div class="guide-title">审核接入指南</div>
          </div>
          <div v-if="guide" class="guide-audit">
            <span class="mono">{{ guide.project.code }} / {{ guide.audit.code }}</span>
            <copy-button :text="guide.audit.code" label="复制审核编码" size="tiny" />
          </div>
        </div>
      </template>

      <n-spin :show="loading">
        <div v-if="guide" class="guide-body">
          <div class="audit-identity">
            <div class="identity-icon"><ShieldCheckmarkOutline /></div>
            <div class="identity-copy">
              <h2>{{ guide.audit.name }}</h2>
              <p>{{ guide.project.name }} · 项目编码 <code>{{ guide.project.code }}</code> · 审核编码 <code>{{ guide.audit.code }}</code></p>
              <span v-if="guide.audit.decision_mode === 'CLASSIFY' || guide.audit.decision_mode === 'CLASSIFY_ONLY'" class="identity-mode">
                {{ guide.audit.decision_mode === 'CLASSIFY_ONLY' ? '纯分类器 · 全部 PASS' : '分类审核' }} · {{ (guide.audit.categories || []).length }} 个类型 · 结果含 category / category_confidence
              </span>
            </div>
            <n-button quaternary circle @click="load()">
              <template #icon><RefreshOutline /></template>
            </n-button>
          </div>

          <div class="guide-tabs">
            <button
              v-for="tab in tabs"
              :key="tab.key"
              type="button"
              :class="{ active: activeTab === tab.key, warning: tab.key === 'callback' }"
              @click="activeTab = tab.key"
            >
              <component :is="tab.icon" />{{ tab.label }}
            </button>
          </div>

          <div v-if="activeTab === 'overview'" class="tab-pane">
            <div v-if="!canWrite" class="guide-notice warning">
              <WarningOutline />
              <span>当前账号为只读权限，可以查看接入指南，但不能修改触发开关、配置或回调。</span>
            </div>
            <div class="guide-notice">
              <GlobeOutline />
              <div>
                <strong>先启用触发方式，再按示例调用</strong>
                <p>每个审核配置独立控制三种接入方式。AppKey/AppSecret 在「接入凭证」中创建，并按项目内的审核配置授权；Redis 触发使用「数据连接」中配置的项目 Redis。</p>
              </div>
              <n-button text type="primary" @click="openDocs('overview')">完整文档</n-button>
            </div>

            <div class="input-contract">
              <span class="contract-pill">多文本 + 多图片</span>
              <span>当前版本仅支持 text / image，可同时提交多段文本和多张图片；<strong>不支持视频、音频</strong>。JEV 原生多模态，通用大模型需启用图片理解。</span>
            </div>

            <div class="trigger-grid">
              <div v-for="tab in tabs.slice(1, 4)" :key="tab.key" class="trigger-toggle-card">
                <div class="trigger-toggle-top">
                  <span class="trigger-icon"><component :is="tab.icon" /></span>
                  <div>
                    <strong>{{ triggerLabel(tab.key.toUpperCase()) }}</strong>
                    <span>{{ tab.key === 'openapi' ? '业务主动提交' : tab.key === 'redis' ? 'Stream 队列消费' : '长连接推送 + 回推结果' }}</span>
                  </div>
                  <n-switch
                    size="small"
                    :value="triggerMap[tab.key.toUpperCase()]?.status === 'ACTIVE'"
                    :loading="saving" :disabled="!canWrite"
                    @update:value="toggleTrigger(tab.key.toUpperCase() as any, $event)"
                  />
                </div>
                <div class="trigger-status">
                  <StatusTag :status="triggerMap[tab.key.toUpperCase()]?.status === 'ACTIVE' ? 'ACTIVE' : 'DISABLED'" type="provider" />
                  <span v-if="triggerMap[tab.key.toUpperCase()]?.last_error" class="trigger-error">{{ triggerMap[tab.key.toUpperCase()]?.last_error }}</span>
                </div>
                <button class="trigger-detail-link" type="button" @click="activeTab = tab.key">查看配置与示例 →</button>
              </div>
            </div>

            <h3 class="guide-section-title">接入凭证</h3>
            <div class="credential-card">
              <template v-if="selectedCredential">
                <div class="credential-row">
                  <div class="credential-selector">
                    <span>当前凭证</span>
                    <n-select
                      :value="selectedCredential.id"
                      :options="credentialOptions"
                      size="small"
                      @update:value="selectCredential"
                    />
                  </div>
                  <n-button size="tiny" secondary @click="goCredentials">管理凭证 / 新建</n-button>
                </div>
                <div class="credential-row">
                  <div><span>AppKey</span><code>{{ selectedCredential.app_key }}</code></div>
                  <copy-button :text="selectedCredential.app_key" label="复制 AppKey" size="tiny" />
                </div>
                <div class="credential-row">
                  <div><span>AppSecret</span><code>{{ selectedCredential.secret_masked || '未配置' }}</code></div>
                  <span class="scope-hint">在「接入凭证」中轮换</span>
                </div>
                <p>{{ guide.credentials.scope }}</p>
              </template>
              <div v-else class="no-credential">
                <WarningOutline />
                <div>
                  <strong>没有可访问此审核配置的凭证</strong>
                  <p>请先到「接入凭证」创建 AppKey，并将其授权给「{{ guide.audit.name }}」。未授权的凭证调用会返回 403 AUDIT_OUT_OF_SCOPE。</p>
                </div>
                <n-button type="primary" size="small" @click="goCredentials">去创建凭证</n-button>
              </div>
            </div>

            <h3 class="guide-section-title">AI 对接助手</h3>
            <div class="ai-skill-card">
              <span class="ai-skill-icon"><SparklesOutline /></span>
              <div>
                <strong>按触发方式生成专用 AI 提示词</strong>
                <p>选择一种或多种方式再复制，提示词会对应生成签名、队列或长连接的实现要求；统一 Skill 会询问 AI 用哪种方式。</p>
              </div>
              <div class="ai-skill-actions">
                <n-button size="small" secondary :disabled="!aiMethods.length" @click="copyAIPrompt()">{{ aiCopyLabel }}</n-button>
                <n-button size="small" type="primary" @click="downloadAISkill">
                  <template #icon><DownloadOutline /></template>
                  下载统一 Skill.md
                </n-button>
                <n-button size="small" quaternary @click="copyDocsURL">复制文档地址</n-button>
              </div>
              <div class="ai-method-picker">
                <div class="ai-method-head">
                  <span>要生成提示词的接入方式</span>
                  <div>
                    <n-button text size="tiny" type="primary" @click="selectAllAiMethods">全选</n-button>
                    <n-button text size="tiny" @click="clearAiMethods">清空</n-button>
                  </div>
                </div>
                <n-checkbox-group v-model:value="aiMethods">
                  <n-checkbox v-for="method in aiMethodOptions" :key="method.key" :value="method.key">
                    {{ method.label }}<small>{{ method.short }}</small>
                  </n-checkbox>
                </n-checkbox-group>
              </div>
            </div>
            <h3 class="guide-section-title">推荐接入方式</h3>
            <div class="recommend-grid">
              <div><span class="rec-icon"><GlobeOutline /></span><strong>已有服务端</strong><p>开放 API + HMAC，业务主动提交并拿到 task_no。</p></div>
              <div><span class="rec-icon"><LayersOutline /></span><strong>已有 Redis</strong><p>启用 Redis 触发，业务向 Stream 写入标准 TaskInput。</p></div>
              <div><span class="rec-icon"><SwapHorizontalOutline /></span><strong>需要实时结果</strong><p>WebSocket 提交，平台主动推送终态结果帧。</p></div>
            </div>
          </div>

          <div v-else-if="activeTab === 'openapi'" class="tab-pane">
            <div class="pane-title-row">
              <div><h3>开放 API</h3><p>项目级 HMAC 签名；只允许访问本项目且已启用开放 API 的审核。</p></div>
              <n-switch :value="openapiTrigger?.status === 'ACTIVE'" :loading="saving" :disabled="!canWrite" @update:value="toggleTrigger('OPENAPI', $event)" />
            </div>
            <div class="method-ai-row">
              <span>提示词只包含开放 API 的签名、提交与查询要求</span>
              <n-button size="tiny" secondary @click="copyAIPrompt('OPENAPI')">
                <template #icon><SparklesOutline /></template>
                复制开放 API 专用提示词
              </n-button>
            </div>
            <div class="endpoint-box"><span>POST</span><code>{{ endpointURL }}</code><copy-button :text="endpointURL" label="复制地址" size="tiny" /></div>
            <template v-if="guide.openapi.signing_available">
              <div class="field-label-row"><span>请求 Header（示例签名有效期 5 分钟）</span><copy-button :text="JSON.stringify(guide.openapi.headers, null, 2)" label="复制 Header" size="tiny" /></div>
              <CodeBlock :code="JSON.stringify(guide.openapi.headers, null, 2)" title="Headers" />
              <div class="field-label-row"><span>可直接尝试的 cURL 示例</span><copy-button :text="guide.openapi.sample_curl" label="复制 cURL" size="tiny" /></div>
              <CodeBlock :code="guide.openapi.sample_curl" title="cURL" />
            </template>
            <div v-else class="guide-notice warning">
              <WarningOutline />
              <span>当前没有已授权到本审核配置的凭证，签名示例不可用。</span>
              <n-button text type="primary" @click="goCredentials">去接入凭证创建/授权</n-button>
            </div>
            <div class="field-label-row"><span>请求体：多文本 + 多图片（不支持视频）</span><copy-button :text="guide.openapi.sample_body_raw" label="复制 Body" size="tiny" /></div>
            <CodeBlock :code="guide.openapi.sample_body_raw" title="JSON Body" />
            <div class="guide-note">
              <HelpCircleOutline />
              <span>{{ guide.openapi.signing_note }} 查询结果：<code>{{ guide.openapi.query_task_path }}</code>。签名与请求体严格绑定，业务系统需按 Canonical 规范自行计算。</span>
              <n-button text type="primary" @click="openDocs('openapi')">查看签名算法</n-button>
            </div>
          </div>

          <div v-else-if="activeTab === 'redis'" class="tab-pane">
            <div class="pane-title-row">
              <div><h3>Redis Stream</h3><p>选择当前项目已启用且测试成功的 Redis 连接，再指定 DB 和 Stream；启用后约 5 秒内生效。</p></div>
              <n-switch :value="redisTrigger?.status === 'ACTIVE'" :loading="saving" :disabled="!canWrite" @update:value="toggleTrigger('REDIS', $event)" />
            </div>
            <div class="method-ai-row">
              <span>提示词只包含消息生产端，不会让 AI 生成 HTTP 签名代码</span>
              <n-button size="tiny" secondary @click="copyAIPrompt('REDIS')">
                <template #icon><SparklesOutline /></template>
                复制 Redis 专用提示词
              </n-button>
            </div>
            <div class="form-grid">
              <label class="full">
                <span>Redis 连接</span>
                <n-select
                  v-model:value="redisForm.connection_id"
                  :options="connectionOptions"
                  :loading="loadingConnections"
                  clearable
                  placeholder="选择当前项目已启用且测试成功的连接"
                />
              </label>
              <label><span>DB 编号</span><n-input-number v-model:value="redisForm.db" :min="0" :max="255" /></label>
              <label>
                <span>消费起点</span>
                <n-select
                  v-model:value="redisForm.start_position"
                  :options="[
                    { label: '$ · 只消费新消息', value: '$' },
                    { label: '0 · 从头消费', value: '0' },
                  ]"
                />
              </label>
              <label><span>批量大小</span><n-input-number v-model:value="redisForm.batch_size" :min="1" :max="32" /></label>
              <label class="full"><span>Stream</span><n-input v-model:value="redisForm.stream" placeholder="sdv:project:audit:code" /></label>
              <label><span>消费组</span><n-input v-model:value="redisForm.group" placeholder="留空自动生成" /></label>
              <label><span>最大并发处理数</span><n-input-number v-model:value="redisForm.max_inflight" :min="1" :max="32" /></label>
            </div>
            <div v-if="!loadingConnections && !connectionOptions.some((item) => !item.disabled)" class="guide-notice warning">
              <WarningOutline />
              <span>当前项目没有已启用且测试成功的 Redis 连接，请先到「数据连接」创建并测试。</span>
              <n-button text type="primary" @click="openDocs('overview')">去数据连接</n-button>
            </div>
            <div class="pane-actions">
              <n-button size="small" :loading="saving" :disabled="!canWrite" @click="saveRedis()">保存配置</n-button>
              <span v-if="redisTrigger?.last_error" class="inline-warning"><WarningOutline /> {{ redisTrigger.last_error }}</span>
              <span v-else class="inline-ok"><CheckmarkCircleOutline /> Stream: {{ redisForm.stream || '未配置' }} · Group: {{ redisForm.group || '自动生成' }}</span>
            </div>
            <div class="field-label-row"><span>消息格式：单个 data 字段（或 payload），值为 TaskInput JSON</span><copy-button :text="JSON.stringify(guide.redis.example, null, 2)" label="复制消息示例" size="tiny" /></div>
            <CodeBlock :code="JSON.stringify(guide.redis.example, null, 2)" title="TaskInput Message" />
            <CodeBlock :code="guide.redis.xadd_command" title="XADD Command" />
            <div class="guide-note"><HelpCircleOutline /><span>一个 Stream 只允许一个审核消费。非法消息会被 ACK 并写入 <code>{{ redisForm.stream }}:dlq</code>。消息格式、消费组和 PEL 说明请查阅对接文档。</span><n-button text type="primary" @click="openDocs('redis')">查看完整规范</n-button></div>
          </div>

          <div v-else-if="activeTab === 'websocket'" class="tab-pane">
            <div class="pane-title-row">
              <div><h3>WebSocket</h3><p>连接鉴权后提交任务；平台会主动推送终态结果。</p></div>
              <n-switch :value="wsTrigger?.status === 'ACTIVE'" :loading="saving" :disabled="!canWrite" @update:value="toggleTrigger('WEBSOCKET', $event)" />
            </div>
            <div class="method-ai-row">
              <span>提示词包含连接签名、ack/result 帧关联和重连要求</span>
              <n-button size="tiny" secondary @click="copyAIPrompt('WEBSOCKET')">
                <template #icon><SparklesOutline /></template>
                复制 WebSocket 专用提示词
              </n-button>
            </div>
            <template v-if="guide.websocket.signing_available">
              <div class="endpoint-box"><span>WS</span><code>{{ wsURL }}</code><copy-button :text="wsURL" label="复制连接地址" size="tiny" /></div>
              <div class="guide-note"><WarningOutline /><span>示例签名 5 分钟内有效且 Nonce 只能使用一次。生产集成请自行签名，签名串为 <code>{{ guide.websocket.canonical }}</code>。</span></div>
            </template>
            <div v-else class="guide-notice warning">
              <WarningOutline />
              <span>当前没有已授权到本审核配置的凭证，WebSocket 连接示例不可用。</span>
              <n-button text type="primary" @click="goCredentials">去接入凭证创建/授权</n-button>
            </div>
            <div class="field-label-row"><span>帧协议（data 支持多个 text / image）</span><copy-button :text="wsFrames" label="复制帧示例" size="tiny" /></div>
            <CodeBlock :code="wsFrames" title="Frames" />
            <div class="guide-note"><HelpCircleOutline /><span>平台会先发送 ready 帧；心跳支持 ping/pong；结果帧在任务进入终态后主动推送。</span><n-button text type="primary" @click="openDocs('websocket')">查看完整协议</n-button></div>
          </div>

          <div v-else-if="activeTab === 'callback'" class="tab-pane">
            <div class="pane-title-row">
              <div><h3>审核结果回调</h3><p>任务进入终态后 POST 到业务接口，支持 HMAC 签名与内容回传。</p></div>
              <n-switch v-model:value="callbackForm.enabled" :disabled="!canWrite" />
            </div>
            <div class="form-grid">
              <label class="full"><span>回调 URL</span><n-input v-model:value="callbackForm.url" placeholder="https://your-service/api/audit-callback" /></label>
              <label class="full"><span>签名 Secret</span><n-input v-model:value="callbackForm.secret" type="password" show-password-on="click" :placeholder="showCallbackSecret ? '已配置，留空保留' : '可选；配置后回调带 HMAC 签名'" /></label>
              <label class="full"><span>回调结论</span>
                <n-checkbox-group v-model:value="callbackForm.results">
                  <n-checkbox value="PASS">通过</n-checkbox>
                  <n-checkbox value="REJECT">不通过</n-checkbox>
                </n-checkbox-group>
              </label>
              <label class="checkbox-field"><span>回传原内容</span><n-switch v-model:value="callbackForm.include_content" /></label>
            </div>
            <div class="pane-actions">
              <n-button size="small" type="primary" :loading="saving" :disabled="!canWrite" @click="saveCallback">保存回调配置</n-button>
              <span v-if="showCallbackSecret" class="inline-ok"><CheckmarkCircleOutline /> 已配置签名 Secret</span>
            </div>
            <div class="field-label-row"><span>回调体示例（含不合格原因与原始内容）</span><copy-button :text="callbackSample" label="复制 Payload" size="tiny" /></div>
            <CodeBlock :code="callbackSample" title="Callback Payload" />
            <div class="guide-note"><HelpCircleOutline /><span>{{ guide.callback.signature }}。失败按 1s / 2s / 4s 重试 3 次，回调状态写入任务详情。</span><n-button text type="primary" @click="openDocs('callback')">查看验签示例</n-button></div>
          </div>

          <div v-else class="tab-pane">
            <h3>权限边界</h3>
            <div class="permission-list">
              <div><strong>凭证范围</strong><p>{{ guide.permissions.credential_scope }}</p></div>
              <div><strong>可访问对象</strong><p>{{ guide.permissions.access }}</p></div>
              <div><strong>管理权限</strong><p>{{ guide.permissions.admin }}</p></div>
            </div>
            <div class="guide-notice">
              <ShieldCheckmarkOutline />
              <div><strong>安全默认开启</strong><p>AppSecret 加密存储、HMAC 时间窗、Nonce 防重放、审核配置级触发开关、项目数据范围校验、操作审计。</p></div>
            </div>
            <div class="quick-links">
              <n-button secondary @click="openDocs('auth')">签名与凭证规范</n-button>
              <n-button secondary @click="openDocs('mysql')">为什么没有 MySQL 触发</n-button>
            </div>
          </div>
        </div>
      </n-spin>
    </n-drawer-content>
  </n-drawer>

</template>

<style scoped>
.guide-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  width: 100%;
}

.guide-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.guide-title {
  margin-top: 2px;
  color: var(--ink);
  font-size: 15px;
  font-weight: 650;
}

.guide-audit {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--muted-light);
  font-size: 9px;
}

.guide-body {
  padding-bottom: 20px;
}

.audit-identity {
  display: flex;
  align-items: center;
  gap: 11px;
  margin-bottom: 15px;
  padding: 14px 16px;
  border: 1px solid #e8eef8;
  border-radius: 13px;
  background: linear-gradient(130deg, #f8fbff, #fff);
}

.identity-icon {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.identity-icon :deep(svg) {
  width: 19px;
  height: 19px;
}

.identity-copy {
  flex: 1;
  min-width: 0;
}

.identity-copy h2 {
  margin: 0;
  color: var(--ink);
  font-size: 16px;
  font-weight: 650;
}

.identity-copy p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10px;
}

.identity-copy code {
  padding: 1px 4px;
  border-radius: 4px;
  background: #f1f5f9;
  font-family: var(--mono);
  font-size: 9px;
}

.identity-mode {
  display: inline-block;
  margin-top: 6px;
  padding: 2px 8px;
  border-radius: 999px;
  background: #f2ecff;
  color: #6d4fc4;
  font-size: 9px;
}

.guide-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  margin-bottom: 15px;
  padding: 3px;
  border-radius: 10px;
  background: #f4f7fb;
}

.guide-tabs button {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 7px 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--muted);
  font-size: 10.5px;
  white-space: nowrap;
  cursor: pointer;
}

.guide-tabs button :deep(svg) {
  width: 13px;
  height: 13px;
}

.guide-tabs button.active {
  background: #fff;
  color: var(--brand-strong);
  box-shadow: var(--shadow-sm);
  font-weight: 550;
}

.guide-tabs button.warning {
  color: #a16207;
}

.guide-tabs button.warning.active {
  color: #92400e;
}

.tab-pane h3 {
  margin: 0 0 3px;
  color: var(--ink);
  font-size: 15px;
  font-weight: 650;
}

.tab-pane > p {
  margin: 0 0 13px;
  color: var(--muted);
  font-size: 11px;
}

.guide-notice {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-bottom: 15px;
  padding: 11px 13px;
  border: 1px solid #dce7fb;
  border-radius: 10px;
  background: #f8fbff;
  color: #475569;
}

.guide-notice > :deep(svg) {
  width: 15px;
  height: 15px;
  color: var(--brand);
  flex-shrink: 0;
  margin-top: 1px;
}

.guide-notice div {
  flex: 1;
}

.guide-notice strong {
  display: block;
  color: #1e4b9e;
  font-size: 10.5px;
  font-weight: 600;
}

.guide-notice p {
  margin: 3px 0 0;
  color: #64748b;
  font-size: 9.5px;
  line-height: 1.65;
}

.guide-notice code {
  font-family: var(--mono);
  font-size: 9px;
}

.trigger-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 9px;
}

.trigger-toggle-card {
  padding: 12px;
  border: 1px solid #edf1f7;
  border-radius: 11px;
  background: #fff;
}

.trigger-toggle-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.trigger-icon {
  display: flex;
  width: 29px;
  height: 29px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.trigger-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.trigger-toggle-top > div {
  flex: 1;
}

.trigger-toggle-top strong {
  display: block;
  color: var(--text);
  font-size: 10.5px;
  font-weight: 600;
}

.trigger-toggle-top > div > span {
  display: block;
  color: var(--muted-light);
  font-size: 8.5px;
}

.trigger-status {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}

.trigger-error {
  overflow: hidden;
  color: #b91c1c;
  font-size: 8.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trigger-detail-link {
  margin-top: 8px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--brand);
  font-size: 9px;
  cursor: pointer;
}

.guide-section-title {
  margin: 20px 0 8px;
  color: var(--muted-light);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.09em;
  text-transform: uppercase;
}

.credential-card {
  padding: 13px 15px;
  border: 1px solid #edf1f7;
  border-radius: 11px;
  background: #fbfcfe;
}

.credential-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 0;
  border-bottom: 1px dashed #edf1f7;
}

.credential-row:last-of-type {
  border-bottom: 0;
}

.credential-row span {
  display: block;
  color: var(--muted-light);
  font-size: 8.5px;
}

.credential-row code {
  display: block;
  margin-top: 2px;
  color: #334155;
  font-family: var(--mono);
  font-size: 10px;
}

.credential-card > p {
  margin: 8px 0 0;
  color: var(--muted-light);
  font-size: 9px;
}

.secret-reveal {
  margin-top: 10px;
  padding: 10px;
  border: 1px solid #f5e3b9;
  border-radius: 9px;
  background: #fffbf2;
}

.secret-reveal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.secret-reveal-head strong {
  color: #92400e;
  font-size: 9.5px;
}

.secret-value {
  display: block;
  margin: 7px 0 4px;
  padding: 7px 8px;
  border-radius: 6px;
  background: #fff;
  color: #475569;
  font-family: var(--mono);
  font-size: 10px;
  word-break: break-all;
}

.secret-reveal > span {
  color: #a16207;
  font-size: 8.5px;
}

.recommend-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.recommend-grid > div {
  padding: 10px 11px;
  border: 1px solid #edf1f7;
  border-radius: 9px;
}

.rec-icon {
  color: var(--brand);
}

.rec-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.recommend-grid strong {
  display: block;
  margin-top: 4px;
  color: var(--text);
  font-size: 10px;
  font-weight: 550;
}

.recommend-grid p {
  margin: 3px 0 0;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.6;
}

.pane-title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 13px;
}

.pane-title-row p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10px;
}

.method-ai-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: -2px 0 12px;
  padding: 8px 10px;
  border: 1px solid #ece8fb;
  border-radius: 8px;
  background: #fbfaff;
}

.method-ai-row > span {
  color: #7c6aa8;
  font-size: 9px;
  line-height: 1.5;
}

.endpoint-box {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 10px 0;
  padding: 9px 11px;
  border: 1px solid #e5ebf4;
  border-radius: 8px;
  background: #f8fafc;
}

.endpoint-box > span {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-family: var(--mono);
  font-size: 8px;
  font-weight: 650;
}

.endpoint-box code {
  flex: 1;
  color: #475569;
  font-family: var(--mono);
  font-size: 9.5px;
  word-break: break-all;
}

.field-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin: 12px 0 6px;
}

.field-label-row > span {
  color: var(--muted);
  font-size: 9.5px;
  font-weight: 550;
}

.guide-note {
  display: flex;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 11px;
  padding: 9px 11px;
  border-radius: 8px;
  background: #f8fafc;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.6;
}

.guide-note :deep(svg) {
  width: 12px;
  height: 12px;
  margin-top: 1px;
  flex-shrink: 0;
}

.guide-note > span {
  flex: 1;
  min-width: 220px;
}

.guide-note code {
  font-family: var(--mono);
  font-size: 8.5px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px 12px;
  margin-bottom: 12px;
}

.form-grid label {
  display: block;
}

.form-grid label.full {
  grid-column: 1 / -1;
}

.form-grid label > span {
  display: block;
  margin-bottom: 4px;
  color: var(--text);
  font-size: 10px;
  font-weight: 500;
}

.form-grid .checkbox-field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 2px;
}

.form-grid .checkbox-field > span {
  margin-bottom: 0;
}

.pane-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}

.inline-warning {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #b91c1c;
  font-size: 9px;
}

.inline-ok {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #15803d;
  font-size: 9px;
}

.inline-warning :deep(svg),
.inline-ok :deep(svg) {
  width: 11px;
  height: 11px;
}

.permission-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 14px;
}

.permission-list > div {
  padding: 11px 13px;
  border: 1px solid #edf1f7;
  border-radius: 9px;
}

.permission-list strong {
  display: block;
  color: var(--text);
  font-size: 10.5px;
  font-weight: 600;
}

.permission-list p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.65;
}

.quick-links {
  display: flex;
  gap: 8px;
  margin-top: 13px;
}

.secret-modal {
  width: min(500px, calc(100vw - 32px));
  padding: 22px;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.secret-modal-head {
  display: flex;
  align-items: center;
  gap: 9px;
  margin-bottom: 12px;
  color: var(--brand);
}

.secret-modal-head :deep(svg) {
  width: 20px;
  height: 20px;
}

.secret-modal-head strong {
  display: block;
  color: var(--ink);
  font-size: 14px;
}

.secret-modal-head span {
  display: block;
  color: var(--muted);
  font-size: 10px;
}

.secret-modal > code {
  display: block;
  padding: 10px;
  border-radius: 8px;
  background: #f8fafc;
  color: #334155;
  font-family: var(--mono);
  font-size: 10px;
  word-break: break-all;
}

.secret-modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 15px;
}


.input-contract {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 14px;
  padding: 9px 12px;
  border: 1px solid #e5ebf4;
  border-radius: 9px;
  background: #fbfcfe;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.65;
}

.contract-pill {
  flex-shrink: 0;
  padding: 2px 7px;
  border-radius: 5px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 8.5px;
  font-weight: 600;
}

.input-contract strong {
  color: #b45309;
}

.credential-selector {
  flex: 1;
  min-width: 0;
}

.credential-selector > span {
  display: block;
  margin-bottom: 4px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.credential-selector :deep(.n-select) {
  max-width: 390px;
}

.scope-hint {
  color: var(--muted-light);
  font-size: 8.5px;
}

.no-credential {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  padding: 4px;
  color: #92400e;
}

.no-credential > :deep(svg) {
  width: 16px;
  height: 16px;
  margin-top: 2px;
  flex-shrink: 0;
}

.no-credential > div {
  flex: 1;
}

.no-credential strong {
  display: block;
  font-size: 10.5px;
  font-weight: 600;
}

.no-credential p {
  margin: 3px 0 0;
  color: #a16207;
  font-size: 9.5px;
  line-height: 1.6;
}

.guide-notice.warning {
  border-color: #f3e1bb;
  background: #fffbf2;
  color: #a16207;
}

.guide-notice.warning > :deep(svg) {
  color: #b45309;
}

.ai-skill-card {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 10px;
  padding: 13px 14px;
  border: 1px solid #e8e2fb;
  border-radius: 11px;
  background: linear-gradient(120deg, #fbfaff, #fff);
}

.ai-skill-icon {
  display: flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--purple-soft);
  color: var(--purple);
  flex-shrink: 0;
}

.ai-skill-icon :deep(svg) {
  width: 15px;
  height: 15px;
}

.ai-skill-card > div:nth-child(2) {
  flex: 1;
}

.ai-skill-card strong {
  display: block;
  color: #4c1d95;
  font-size: 10.5px;
  font-weight: 600;
}

.ai-skill-card p {
  margin: 3px 0 0;
  color: #7c6aa8;
  font-size: 9px;
  line-height: 1.6;
}

.ai-skill-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  justify-content: flex-end;
  max-width: 300px;
}

.ai-method-picker {
  width: calc(100% - 42px);
  flex-basis: calc(100% - 42px);
  margin: 2px 0 0 42px;
  padding-top: 10px;
  border-top: 1px dashed #e8e2fb;
}

.ai-method-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 7px;
  color: #7c6aa8;
  font-size: 9px;
}

.ai-method-head > div {
  display: flex;
  gap: 8px;
}

.ai-method-picker :deep(.n-checkbox) {
  margin-right: 14px;
  margin-bottom: 3px;
  color: #5b4a80;
  font-size: 10px;
}

.ai-method-picker :deep(.n-checkbox small) {
  margin-left: 5px;
  color: #a49bb8;
  font-size: 8px;
}

@media (max-width: 620px) {
  .ai-skill-card {
    flex-wrap: wrap;
  }

  .ai-skill-actions {
    max-width: none;
    justify-content: flex-start;
    padding-left: 42px;
  }

  .ai-method-picker {
    margin-left: 0;
  }
}

@media (max-width: 600px) {
  .trigger-grid,
  .recommend-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .guide-audit {
    display: none;
  }
}
</style>
