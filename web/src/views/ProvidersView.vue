<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  NButton,
  NInput,
  NInputNumber,
  NModal,
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
import type { Provider } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const workspace = useWorkspaceStore()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const testingId = ref<number | null>(null)
const modalShow = ref(false)
const editing = ref<Provider | null>(null)
const errorText = ref('')
const providers = ref<Provider[]>([])

const form = reactive({
  name: '',
  kind: 'OPENAI_COMPATIBLE',
  baseUrl: 'https://api.openai.com/v1',
  model: 'gpt-4o-mini',
  apiKey: '',
  prompt: '',
  supportsVision: false,
  maxTokens: 2400,
  maxImages: 6,
  temperature: 0.1,
  status: 'ACTIVE',
})

const isJEV = computed(() => form.kind === 'JEV')
const isAnthropic = computed(() => form.kind === 'ANTHROPIC')
const activeProvider = computed(() => providers.value.find((p) => p.id === testingId.value))
const baseUrlHint = computed(() => {
  if (form.kind === 'JEV') return 'JEV 会追加 /systemone（若 BaseURL 已含 /v1）。'
  if (form.kind === 'ANTHROPIC') return 'Anthropic 会追加 /messages（若 BaseURL 已含 /v1）；图片会下载并转成 base64 图片块提交。'
  return 'OpenAI 兼容模型会追加 /chat/completions；支持自定义代理地址。'
})

function providerKindLabel(kind?: string) {
  if (kind === 'JEV') return 'JEV 决策模型'
  if (kind === 'ANTHROPIC') return '通用大模型 · Anthropic 协议'
  return '通用大模型 · OpenAI 协议'
}

function resetForm() {
  form.name = ''
  form.kind = 'OPENAI_COMPATIBLE'
  form.baseUrl = 'https://api.openai.com/v1'
  form.model = 'gpt-4o-mini'
  form.apiKey = ''
  form.prompt = ''
  form.supportsVision = false
  form.maxTokens = 2400
  form.maxImages = 6
  form.temperature = 0.1
  form.status = 'ACTIVE'
  errorText.value = ''
  editing.value = null
}

function chooseKind(kind: string) {
  if (editing.value) return
  form.kind = kind
  if (kind === 'JEV') {
    form.baseUrl = 'https://api.typesafe.ai/v1'
    form.model = 'jev-latest'
    form.supportsVision = true
    form.maxImages = 8
    form.maxTokens = 0
    form.temperature = 0
    form.name = form.name || 'JEV 决策模型'
    form.prompt = form.prompt || '使用 TypeSafe Jev System One 进行结构化裁决，输出 Choice/Noul 概率，不生成自由文本。'
  } else if (kind === 'ANTHROPIC') {
    form.baseUrl = 'https://api.anthropic.com/v1'
    form.model = 'claude-sonnet-4-5'
    form.supportsVision = true
    form.maxImages = 6
    form.maxTokens = 2400
    form.temperature = 0.1
    form.name = form.name || 'Anthropic Claude'
    form.prompt = form.prompt || '使用 Anthropic Messages API 进行档案图档审核，按要求的 JSON 结构返回结论。'
  } else {
    form.baseUrl = 'https://api.openai.com/v1'
    form.model = 'gpt-4o-mini'
    form.supportsVision = false
    form.maxTokens = 2400
    form.temperature = 0.1
    form.maxImages = 6
    form.name = ['JEV 决策模型', 'Anthropic Claude'].includes(form.name) ? '' : form.name
    form.prompt = form.prompt.startsWith('使用 TypeSafe') || form.prompt.startsWith('使用 Anthropic') ? '' : form.prompt
  }
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
  form.supportsVision = Boolean(provider.config?.supports_vision)
  form.maxTokens = provider.config?.max_tokens ?? (provider.kind === 'JEV' ? 0 : 2400)
  form.maxImages = provider.config?.max_images ?? 6
  form.temperature = provider.config?.temperature ?? 0.1
  form.status = provider.status
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
    max_tokens: form.maxTokens,
    temperature: form.temperature,
    max_images: form.maxImages,
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

async function test(provider: Provider) {
  testingId.value = provider.id
  try {
    const result = await providerApi.test(provider.id)
    message.success(`连接成功 · ${result.latency_ms} ms${result.model ? ` · ${result.model}` : ''}`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '连接测试失败', { duration: 6000 })
  } finally {
    testingId.value = null
    await load()
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

watch(() => workspace.currentProjectId, load)
onMounted(load)
</script>

<template>
  <div class="page providers-page">
    <PageHeader
      eyebrow="MODEL GATEWAY"
      title="模型接入"
      subtitle="统一管理通用大模型（OpenAI 兼容 / Anthropic 协议）与 JEV 决策模型，为审核流程提供可切换、可审计的推理能力。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新增模型接入
        </n-button>
      </template>
    </PageHeader>

    <section class="mode-banner">
      <div class="mode-card generic">
        <span class="mode-icon"><CloudOutline /></span>
        <div>
          <div class="mode-title">通用大模型 <span>OpenAI 协议 · Chat Completions</span></div>
          <p>适合文本审核、文档语义分析与多模态视觉模型。按模型能力决定是否开启图片理解。</p>
        </div>
        <span class="mode-badge">CHAT COMPLETIONS</span>
      </div>
      <div class="mode-card anthropic">
        <span class="mode-icon"><LinkOutline /></span>
        <div>
          <div class="mode-title">通用大模型 <span>Anthropic 协议 · Messages API</span></div>
          <p>原生兼容 Claude 的 /v1/messages 协议，文本与 base64 图片块一起提交，适合多模态审核。</p>
        </div>
        <span class="mode-badge">/v1/messages</span>
      </div>
      <div class="mode-card jev">
        <span class="mode-icon"><SparklesOutline /></span>
        <div>
          <div class="mode-title">JEV 决策模型 <span>TypeSafe System One</span></div>
          <p>结构化裁决引擎，输出 Choice / Score / Noul 与置信度，原生支持图片资产，不生成自由文本。</p>
        </div>
        <span class="mode-badge">/v1/systemone</span>
      </div>
    </section>

    <div v-if="!workspace.currentProjectId" class="panel">
      <EmptyState title="请选择项目" description="模型接入按项目隔离，先在左上角选择一个项目。" action="前往项目空间" @action="$router.push({ name: 'projects' })" />
    </div>

    <template v-else>
      <div class="provider-grid fade-list">
        <article
          v-for="provider in providers"
          :key="provider.id"
          class="provider-card"
          :class="provider.kind === 'JEV' ? 'jev' : provider.kind === 'ANTHROPIC' ? 'anthropic' : 'generic'"
        >
          <div class="provider-top">
            <span class="provider-icon">
              <SparklesOutline v-if="provider.kind === 'JEV'" />
              <LinkOutline v-else-if="provider.kind === 'ANTHROPIC'" />
              <CloudOutline v-else />
            </span>
            <div class="provider-heading">
              <div class="provider-name-row">
                <h3>{{ provider.name }}</h3>
                <StatusTag :status="provider.kind" type="kind" />
              </div>
              <span class="provider-model mono">{{ provider.model }}</span>
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
              {{ provider.kind === 'JEV' || provider.config?.supports_vision ? '支持图片' : '纯文本' }}
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
              :disabled="testingId !== null"
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

        <button class="provider-add-card" type="button" @click="openCreate">
          <span class="add-circle"><AddOutline /></span>
          <strong>接入新的模型</strong>
          <span>OpenAI / Anthropic / JEV System One</span>
        </button>
      </div>

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
              :class="{ active: form.kind === 'OPENAI_COMPATIBLE', disabled: Boolean(editing) }"
              @click="chooseKind('OPENAI_COMPATIBLE')"
            >
              <span class="kind-icon generic"><CloudOutline /></span>
              <span>
                <strong>通用大模型 · OpenAI 协议</strong>
                <small>兼容 /v1/chat/completions</small>
              </span>
              <span v-if="form.kind === 'OPENAI_COMPATIBLE'" class="kind-check">✓</span>
            </button>
            <button
              type="button"
              class="kind-card"
              :class="{ active: form.kind === 'ANTHROPIC', disabled: Boolean(editing) }"
              @click="chooseKind('ANTHROPIC')"
            >
              <span class="kind-icon anthropic"><LinkOutline /></span>
              <span>
                <strong>通用大模型 · Anthropic 协议</strong>
                <small>Messages API · /v1/messages</small>
              </span>
              <span v-if="form.kind === 'ANTHROPIC'" class="kind-check">✓</span>
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

          <div v-if="isJEV" class="jev-note">
            <SparklesOutline />
            <div>
              <strong>JEV 采用结构化问题而非提示词生成</strong>
              <p>平台会向 <code>/v1/systemone</code> 发送一个 Choice 裁决问题与两个 Noul 风险问题，并自动把图片转成 base64 资产；结果由概率和置信度归一化为风险分。</p>
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

          <div class="two-col">
            <div class="field">
              <label>模型 ID <span class="required">*</span></label>
              <n-input v-model:value="form.model" placeholder="gpt-4o-mini / jev-latest" />
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
                  <span>通用模型需模型本身支持视觉输入</span>
                </div>
                <n-switch v-model:value="form.supportsVision" :disabled="isJEV" />
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
                  <strong>最大输出 Token</strong>
                  <span>JEV 无需设置</span>
                </div>
                <n-input-number v-model:value="form.maxTokens" :min="0" :max="8000" :step="200" size="small" style="width: 100px" />
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
.mode-banner {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 18px;
}

.mode-card.anthropic {
  border-color: #dff3ec;
  background: linear-gradient(120deg, #f7fffc, #fff);
}

.mode-card.anthropic .mode-icon {
  background: #e8f8f2;
  color: #0f766e;
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

.mode-badge {
  margin-left: auto;
  padding: 2px 7px;
  border-radius: 6px;
  background: #f8fafc;
  color: #94a3b8;
  font-family: var(--mono);
  font-size: 8px;
  white-space: nowrap;
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

.provider-add-card {
  display: flex;
  min-height: 250px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 6px;
  border: 1px dashed #d5ddea;
  border-radius: 17px;
  background: rgba(255, 255, 255, 0.55);
  color: var(--muted);
  cursor: pointer;
  transition: all 0.18s ease;
}

.provider-add-card:hover {
  border-color: #aac4f8;
  background: #f9fbff;
  color: var(--brand);
}

.add-circle {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  margin-bottom: 4px;
  border-radius: 50%;
  background: #f1f5fb;
  color: var(--muted);
}

.provider-add-card:hover .add-circle {
  background: var(--brand-soft);
  color: var(--brand);
}

.add-circle :deep(svg) {
  width: 18px;
  height: 18px;
}

.provider-add-card strong {
  font-size: 12.5px;
  font-weight: 550;
}

.provider-add-card > span:last-child {
  color: var(--muted-light);
  font-size: 10px;
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
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 18px;
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

@media (max-width: 1400px) {
  .mode-badge {
    display: none;
  }
}

@media (max-width: 1100px) {
  .provider-grid,
  .mode-banner {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .two-col,
  .config-grid,
  .kind-selector {
    grid-template-columns: 1fr;
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
