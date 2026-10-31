<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { NButton, NInput, useMessage } from 'naive-ui'
import {
  CloudDownloadOutline,
  ChevronDownOutline,
  CloudUploadOutline,
  LinkOutline,
  LockClosedOutline,
  ColorPaletteOutline,
  RefreshOutline,
  SaveOutline,
  ServerOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import BrandMark from '@/components/BrandMark.vue'
import PageHeader from '@/components/PageHeader.vue'
import { systemConfigApi } from '@/api'
import type { SystemConfigData, SystemConfigGroup } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { useUpdateStore } from '@/stores/update'

const message = useMessage()
const auth = useAuthStore()
const site = useSiteStore()
const update = useUpdateStore()
const loading = ref(false)
const data = ref<SystemConfigData | null>(null)
const errorText = ref('')
const branding = reactive({ name: '', name_en: '', logo: '' })
const brandingLoading = ref(false)
const brandingSaving = ref(false)
const brandingError = ref('')
const brandingExpanded = ref(false)
const logoInput = ref<HTMLInputElement | null>(null)
const logoPreview = ref('')
const logoDragging = ref(false)

const groupIcons: Record<string, unknown> = {
  runtime: SettingsOutline,
  database: ServerOutline,
  redis: LinkOutline,
  security: ShieldCheckmarkOutline,
  ai: SparklesOutline,
  storage: CloudUploadOutline,
}

const runtimeFields = computed(() => data.value?.groups.find((group) => group.key === 'runtime')?.fields || [])

function runtimeValue(key: string) {
  return runtimeFields.value.find((field) => field.key === key)?.value || '—'
}

function groupIcon(group: SystemConfigGroup) {
  return groupIcons[group.key] || SettingsOutline
}

async function loadBranding() {
  if (!auth.can('system:config')) return
  brandingLoading.value = true
  brandingError.value = ''
  try {
    const data = await systemConfigApi.getBranding()
    branding.name = data.name || ''
    branding.name_en = data.name_en || ''
    branding.logo = data.logo || ''
    logoPreview.value = site.logo || (data.logo.startsWith('data:') || /^https?:\/\//i.test(data.logo) ? data.logo : '')
  } catch (error) {
    brandingError.value = error instanceof Error ? error.message : '加载站点品牌失败'
  } finally {
    brandingLoading.value = false
  }
}

async function saveBranding() {
  if (!auth.can('system:config') || brandingSaving.value) return
  brandingSaving.value = true
  brandingError.value = ''
  try {
    const saved = await systemConfigApi.updateBranding({
      name: branding.name.trim(),
      name_en: branding.name_en.trim(),
      logo: branding.logo.trim(),
    })
    branding.name = saved.name
    branding.name_en = saved.name_en
    branding.logo = saved.logo
    await site.load(true)
    logoPreview.value = site.logo
    message.success('站点品牌已更新，立即生效')
  } catch (error) {
    brandingError.value = error instanceof Error ? error.message : '保存站点品牌失败'
  } finally {
    brandingSaving.value = false
  }
}

function focusLogoDrop(event: MouseEvent) {
  ;(event.currentTarget as HTMLElement).focus()
}

function pickLogo() {
  if (!brandingLoading.value && !brandingSaving.value) logoInput.value?.click()
}

function clearLogo() {
  branding.logo = ''
  logoPreview.value = ''
}

function loadLogoImage(source: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('无法读取该图片，请更换 PNG、JPG、WebP 或 SVG'))
    image.src = source
  })
}

async function processLogo(file: Blob, name = 'logo') {
  if (file.size > 8 * 1024 * 1024) {
    brandingError.value = 'Logo 文件不能超过 8MB'
    return
  }
  const isSvg = file.type === 'image/svg+xml' || /\.svg$/i.test(name)
  if (!file.type.startsWith('image/') && !isSvg) {
    brandingError.value = '请选择 PNG、JPG、WebP 或 SVG 图片'
    return
  }
  const objectURL = URL.createObjectURL(file)
  try {
    const image = await loadLogoImage(objectURL)
    const size = 256
    const width = image.naturalWidth || 512
    const height = image.naturalHeight || 512
    const scale = Math.max(size / width, size / height)
    const drawWidth = width * scale
    const drawHeight = height * scale
    const canvas = document.createElement('canvas')
    canvas.width = size
    canvas.height = size
    const context = canvas.getContext('2d')
    if (!context) throw new Error('浏览器不支持图片裁剪')
    context.clearRect(0, 0, size, size)
    context.drawImage(image, (size - drawWidth) / 2, (size - drawHeight) / 2, drawWidth, drawHeight)
    let dataURL = canvas.toDataURL('image/webp', 0.86)
    if (!dataURL.startsWith('data:image/webp')) dataURL = canvas.toDataURL('image/png')
    branding.logo = dataURL
    logoPreview.value = dataURL
    brandingError.value = ''
  } catch (error) {
    brandingError.value = error instanceof Error ? error.message : 'Logo 处理失败'
  } finally {
    URL.revokeObjectURL(objectURL)
  }
}

function onLogoFileChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (file) void processLogo(file, file.name)
  ;(event.target as HTMLInputElement).value = ''
}

function onLogoDrop(event: DragEvent) {
  logoDragging.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) void processLogo(file, file.name)
}

function onLogoPaste(event: ClipboardEvent) {
  const items = Array.from(event.clipboardData?.items || [])
  const fileItem = items.find((item) => item.kind === 'file' && item.type.startsWith('image/'))
  if (fileItem) {
    const file = fileItem.getAsFile()
    if (file) {
      void processLogo(file, file.name)
      return
    }
  }
  const text = event.clipboardData?.getData('text/plain') || ''
  if (/<svg[\s>]/i.test(text)) {
    void processLogo(new Blob([text], { type: 'image/svg+xml' }), 'pasted-logo.svg')
  }
}

async function load() {
  loading.value = true
  errorText.value = ''
  try {
    data.value = await systemConfigApi.get()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '加载系统配置失败'
    message.error(errorText.value)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
  void loadBranding()
})
</script>

<template>
  <div class="page system-config-page">
    <PageHeader
      eyebrow="SYSTEM CONFIG"
      title="系统配置"
      subtitle="只读查看当前进程启动时加载的配置；密码和密钥字段已脱敏。"
    >
      <template #actions>
        <n-button
          v-if="auth.can('system:config')"
          secondary
          :loading="update.loading"
          @click="update.openAndCheck(true)"
        >
          <template #icon><CloudDownloadOutline /></template>
          检查更新
        </n-button>
        <n-button secondary :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <div class="config-banner">
      <span class="banner-icon"><LockClosedOutline /></span>
      <div class="banner-copy">
        <strong>只读配置视图</strong>
        <p>配置来自进程启动时解析的 .env / config.yml；修改后需要重启服务才会生效。</p>
        <p v-if="data" class="config-source">
          环境文件：<code>{{ data.env_file || '未使用' }}</code>
          <span class="source-sep">·</span>
          YAML：<code>{{ data.config_file || '未使用' }}</code>
        </p>
      </div>
      <span class="banner-badge">READ ONLY</span>
    </div>

    <div v-if="loading" class="config-loading">正在读取配置…</div>
    <div v-else-if="errorText" class="config-empty">{{ errorText }}</div>
    <template v-else-if="data">
      <div class="config-summary">
        <div class="summary-card">
          <span>运行环境</span>
          <strong>{{ runtimeValue('APP_ENV') }}</strong>
        </div>
        <div class="summary-card">
          <span>版本</span>
          <strong class="mono">v{{ data.version }}</strong>
        </div>
        <div class="summary-card">
          <span>服务端口</span>
          <strong class="mono">{{ runtimeValue('SERVER_PORT') }}</strong>
        </div>
        <div class="summary-card">
          <span>审核 Worker</span>
          <strong class="mono">{{ runtimeValue('WORKER_COUNT') }}</strong>
        </div>
      </div>

      <section v-if="auth.can('system:config')" class="branding-editor">
        <div class="branding-head" role="button" tabindex="0" @click="brandingExpanded = !brandingExpanded" @keydown.enter.prevent="brandingExpanded = !brandingExpanded">
          <span class="config-icon"><ColorPaletteOutline /></span>
          <div class="branding-heading">
            <h3>站点品牌</h3>
            <p>在线修改网站名称和 Logo，保存后立即生效；配置保存在 <code>data/branding.json</code>，无需重启。</p>
            <div class="branding-current">
              <BrandMark :size="30" />
              <strong>{{ site.displayName }}</strong>
              <span>{{ site.nameEn }}</span>
              <small>{{ site.logo ? '自定义 Logo' : '内置图标' }}</small>
            </div>
          </div>
          <span class="branding-badge">ONLINE</span>
          <button class="branding-toggle" type="button" :title="brandingExpanded ? '收起' : '展开编辑'" @click.stop="brandingExpanded = !brandingExpanded">
            <ChevronDownOutline :class="{ rotated: brandingExpanded }" />
          </button>
        </div>
        <div v-if="brandingExpanded" class="branding-body">
        <div class="branding-form">
          <label>
            <span>网站名称</span>
            <n-input v-model:value="branding.name" maxlength="64" placeholder="例如 智审图档" :disabled="brandingLoading" />
          </label>
          <label>
            <span>英文名称</span>
            <n-input v-model:value="branding.name_en" maxlength="64" placeholder="例如 SmartDoc Vision" :disabled="brandingLoading" />
          </label>
          <label class="branding-logo-field">
            <span>Logo</span>
            <div
              class="logo-drop"
              :class="{ dragging: logoDragging, 'has-logo': logoPreview }"
              tabindex="0"
              @click="focusLogoDrop"
              @dragover.prevent="logoDragging = true"
              @dragleave.prevent="logoDragging = false"
              @drop.prevent="onLogoDrop"
              @paste.prevent="onLogoPaste"
            >
              <img v-if="logoPreview" :src="logoPreview" alt="站点 Logo 预览" />
              <div v-else class="logo-placeholder">
                <CloudUploadOutline />
                <strong>上传、拖入或粘贴 Logo</strong>
                <span>PNG / JPG / WebP / SVG · 自动居中裁剪 1:1 · 压缩到 256×256</span>
                <button type="button" class="logo-choose" @click.stop="pickLogo">选择图片</button>
              </div>
              <template v-if="logoPreview">
                <button type="button" class="logo-clear" title="移除 Logo" @click.stop="clearLogo">移除</button>
                <button type="button" class="logo-replace" @click.stop="pickLogo">更换</button>
              </template>
            </div>
            <input ref="logoInput" type="file" accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml" hidden @change="onLogoFileChange" />
          </label>
        </div>
        <div class="branding-preview">
          <BrandMark :size="46" />
          <div>
            <strong>{{ branding.name || '未填写' }}</strong>
            <span>{{ branding.name_en || '未填写英文名称' }}</span>
          </div>
          <small>名称留空使用平台默认名称；Logo 留空使用内置图标。上传或粘贴后会自动裁剪为 1:1 并压缩。修改后请刷新其他已打开页面。</small>
        </div>
        <div v-if="brandingError" class="branding-error">{{ brandingError }}</div>
        <div class="branding-actions">
          <n-button type="primary" :loading="brandingSaving" :disabled="brandingLoading" @click="saveBranding">
            <template #icon><SaveOutline /></template>
            保存品牌设置
          </n-button>
        </div>
        </div>
      </section>

      <div class="config-grid">
        <section v-for="group in data.groups" :key="group.key" class="config-card">
          <div class="config-card-head">
            <span class="config-icon">
              <component :is="groupIcon(group)" />
            </span>
            <div>
              <h3>{{ group.title }}</h3>
              <p>{{ group.description }}</p>
            </div>
          </div>
          <div class="config-fields">
            <div v-for="field in group.fields" :key="field.key" class="config-field">
              <span class="config-label">{{ field.label }}</span>
              <div class="config-value">
                <code v-if="!field.sensitive">{{ field.value }}</code>
                <span v-else class="secret-value">
                  <LockClosedOutline />
                  {{ field.value }}
                </span>
                <span
                  v-if="field.sensitive && field.configured"
                  class="secret-badge configured"
                >已脱敏</span>
              </div>
            </div>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.config-banner {
  display: flex;
  align-items: flex-start;
  gap: 13px;
  margin-bottom: 16px;
  padding: 15px 17px;
  border: 1px solid var(--brand-border);
  border-radius: 14px;
  background: linear-gradient(130deg, var(--brand-softer), var(--card));
}

.banner-icon {
  display: flex;
  width: 36px;
  height: 36px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.banner-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.banner-copy {
  min-width: 0;
  flex: 1;
}

.banner-copy strong {
  display: block;
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 620;
}

.banner-copy p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.6;
}

.config-source {
  color: var(--muted-light) !important;
}

.config-source code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--text);
  font-family: var(--mono);
  font-size: 9.5px;
}

.source-sep {
  margin: 0 5px;
  color: var(--muted-light);
}

.banner-badge {
  padding: 3px 8px;
  border-radius: 99px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-family: var(--mono);
  font-size: 8px;
  font-weight: 650;
  letter-spacing: 0.08em;
  white-space: nowrap;
  flex: none;
}

.config-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}

.summary-card {
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
}

.summary-card span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
  letter-spacing: 0.04em;
}

.summary-card strong {
  display: block;
  overflow: hidden;
  margin-top: 3px;
  color: var(--ink);
  font-size: 15px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.summary-card .mono {
  font-family: var(--mono);
  font-size: 13px;
}

.branding-editor {
  margin-bottom: 16px;
  padding: 16px 18px;
  border: 1px solid var(--brand-border);
  border-radius: 14px;
  background: linear-gradient(130deg, var(--brand-softer), var(--card));
}

.branding-head {
  display: flex;
  align-items: flex-start;
  gap: 11px;
}

.branding-head {
  cursor: pointer;
}

.branding-heading {
  min-width: 0;
  flex: 1;
}

.branding-current {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-top: 7px;
  color: var(--muted-light);
  font-size: 9px;
}

.branding-current strong {
  color: var(--ink);
  font-size: 12px;
  font-weight: 600;
}

.branding-current span {
  color: var(--muted);
}

.branding-current small {
  color: var(--brand);
  font-size: 8.5px;
}

.branding-toggle {
  display: flex;
  width: 28px;
  height: 28px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card);
  color: var(--muted);
  cursor: pointer;
  flex: none;
}

.branding-toggle:hover {
  border-color: var(--brand-border);
  color: var(--brand);
}

.branding-toggle :deep(svg) {
  width: 15px;
  height: 15px;
  transition: transform 0.18s ease;
}

.branding-toggle :deep(svg.rotated) {
  transform: rotate(180deg);
}

.branding-body {
  animation: branding-expand 0.18s ease;
}

@keyframes branding-expand {
  from { opacity: 0; transform: translateY(-3px); }
  to { opacity: 1; transform: translateY(0); }
}

.branding-head h3 {
  margin: 0 0 3px;
  color: var(--ink);
  font-size: 14px;
  font-weight: 650;
}

.branding-head p {
  margin: 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.6;
}

.branding-head code {
  padding: 1px 4px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--text);
  font-family: var(--mono);
  font-size: 9.5px;
}

.branding-badge {
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-family: var(--mono);
  font-size: 8px;
  font-weight: 650;
  flex: none;
}

.logo-drop {
  position: relative;
  display: flex;
  min-height: 132px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  padding: 14px;
  border: 1px dashed var(--border-strong);
  border-radius: 12px;
  background: var(--surface-soft);
  cursor: pointer;
  transition: border-color 0.16s ease, background 0.16s ease;
}

.logo-drop:hover,
.logo-drop.dragging {
  border-color: var(--brand-border);
  background: var(--brand-softer);
}

.logo-drop img {
  display: block;
  width: 96px;
  height: 96px;
  border-radius: 18px;
  object-fit: cover;
  box-shadow: var(--shadow-sm);
}

.logo-placeholder {
  display: flex;
  align-items: center;
  flex-direction: column;
  gap: 5px;
  color: var(--muted);
  text-align: center;
}

.logo-placeholder :deep(svg) {
  width: 24px;
  height: 24px;
  color: var(--brand);
}

.logo-placeholder strong {
  color: var(--text);
  font-size: 12px;
}

.logo-placeholder span {
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.5;
}

.logo-choose {
  margin-top: 3px;
  padding: 4px 9px;
  border: 1px solid var(--brand-border);
  border-radius: 6px;
  background: var(--card);
  color: var(--brand-strong);
  font-size: 9.5px;
  cursor: pointer;
}

.logo-clear,
.logo-replace {
  position: absolute;
  top: 8px;
  padding: 2px 7px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card);
  color: var(--muted);
  font-size: 9px;
  cursor: pointer;
}

.logo-clear {
  right: 8px;
}

.logo-replace {
  right: 48px;
}

.logo-clear:hover {
  border-color: var(--red-border);
  color: var(--red);
}

.logo-replace:hover {
  border-color: var(--brand-border);
  color: var(--brand);
}

.branding-form {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 15px;
}

.branding-form label {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.branding-form label > span {
  color: var(--muted);
  font-size: 10.5px;
  font-weight: 550;
}

.branding-logo-field {
  grid-column: 1 / -1;
}

.branding-preview {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 13px;
  padding-top: 13px;
  border-top: 1px solid var(--border);
}

.branding-preview > div {
  min-width: 0;
  flex: 1;
}

.branding-preview strong {
  display: block;
  color: var(--ink);
  font-size: 13px;
}

.branding-preview > div > span {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9px;
}

.branding-preview small {
  max-width: 330px;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.5;
}

.branding-error {
  margin-top: 10px;
  padding: 8px 10px;
  border: 1px solid var(--red-border);
  border-radius: 8px;
  background: var(--red-soft);
  color: var(--red);
  font-size: 10.5px;
}

.branding-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.config-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  align-items: stretch;
  gap: 14px;
}

.config-card {
  display: flex;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
}

.config-card-head {
  display: flex;
  align-items: flex-start;
  gap: 11px;
  padding: 16px 17px 13px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-soft);
}

.config-icon {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.config-icon :deep(svg) {
  width: 16px;
  height: 16px;
}

.config-card-head h3 {
  margin: 1px 0 2px;
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 620;
}

.config-card-head p {
  margin: 0;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.5;
}

.config-fields {
  display: flex;
  flex: 1;
  flex-direction: column;
  padding: 2px 17px 8px;
}

.config-field {
  display: flex;
  flex: 1 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  min-height: 38px;
  padding: 7px 0;
  border-bottom: 1px dashed var(--border);
}

.config-field:last-child {
  border-bottom: 0;
}

.config-label {
  color: var(--muted);
  font-size: 10.5px;
  flex: none;
}

.config-value {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.config-value code {
  overflow: hidden;
  color: var(--text);
  font-family: var(--mono);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 280px;
}

.secret-value {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  overflow: hidden;
  color: var(--text);
  font-family: var(--mono);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.secret-value :deep(svg) {
  width: 11px;
  height: 11px;
  color: var(--muted-light);
  flex: none;
}

.secret-badge {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--muted-light);
  font-size: 8px;
  flex: none;
}

.secret-badge.configured {
  background: var(--green-soft);
  color: var(--green);
}

.config-loading,
.config-empty {
  padding: 60px 0;
  color: var(--muted-light);
  font-size: 11px;
  text-align: center;
}

@media (max-width: 720px) {
.config-grid {
    grid-template-columns: 1fr;
  }

  .config-field {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }

  .config-value {
    width: 100%;
    justify-content: flex-start;
  }

  .config-value code {
    max-width: 100%;
  }
}
</style>
