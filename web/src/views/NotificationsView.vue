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
  CloseCircleOutline,
  CreateOutline,
  FlashOutline,
  MailOutline,
  RefreshOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusTag from '@/components/StatusTag.vue'
import { notificationApi } from '@/api'
import type { NotificationChannel } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useWorkspaceStore } from '@/stores/workspace'
import { isValidEmail } from '@/utils/validation'

const workspace = useWorkspaceStore()
const auth = useAuthStore()
const message = useMessage()
const dialog = useDialog()

const channels = ref<NotificationChannel[]>([])
const loading = ref(false)
const modalShow = ref(false)
const editing = ref<NotificationChannel | null>(null)
const saving = ref(false)
const errorText = ref('')
const testShow = ref(false)
const testTarget = ref<NotificationChannel | null>(null)
const testTo = ref('')
const testSending = ref(false)
const testResult = ref<{ ok: boolean; message: string } | null>(null)

const canWrite = computed(() => auth.can('notification:write'))
const hasProject = computed(() => Boolean(workspace.currentProjectId))

const providerOptions = [
  { label: '自定义 SMTP', value: 'custom' },
  { label: 'QQ 邮箱', value: 'qq' },
  { label: '163 邮箱', value: '163' },
  { label: '126 邮箱', value: '126' },
  { label: 'Gmail', value: 'gmail' },
  { label: 'Outlook / Microsoft 365', value: 'outlook' },
]
const providerLabels: Record<string, string> = {
  custom: '自定义 SMTP',
  qq: 'QQ 邮箱',
  '163': '163 邮箱',
  '126': '126 邮箱',
  gmail: 'Gmail',
  outlook: 'Outlook / Microsoft 365',
}
const providerPresets: Record<string, { host: string; port: number; security: string }> = {
  qq: { host: 'smtp.qq.com', port: 465, security: 'ssl' },
  '163': { host: 'smtp.163.com', port: 465, security: 'ssl' },
  '126': { host: 'smtp.126.com', port: 465, security: 'ssl' },
  gmail: { host: 'smtp.gmail.com', port: 587, security: 'starttls' },
  outlook: { host: 'smtp.office365.com', port: 587, security: 'starttls' },
}
const securityOptions = [
  { label: 'SSL（通常 465）', value: 'ssl' },
  { label: 'STARTTLS（通常 587）', value: 'starttls' },
  { label: '不加密', value: 'none' },
]
const securityLabels: Record<string, string> = {
  ssl: 'SSL',
  starttls: 'STARTTLS',
  none: '不加密',
}

const form = reactive({
  name: '',
  provider: 'custom',
  status: 'DISABLED',
  host: '',
  port: 465,
  security: 'ssl',
  username: '',
  password: '',
  fromName: '',
  fromAddress: '',
  timeoutSec: 10,
})

function providerLabel(provider: string) {
  return providerLabels[provider] || provider || '自定义 SMTP'
}

function securityLabel(security: string) {
  return securityLabels[security] || security || '—'
}

function onProviderChange(value: string) {
  form.provider = value
  const preset = providerPresets[value]
  if (preset) {
    form.host = preset.host
    form.port = preset.port
    form.security = preset.security
  }
}

function onSecurityChange(value: string) {
  form.security = value
  if ([25, 465, 587].includes(form.port)) {
    form.port = value === 'ssl' ? 465 : value === 'none' ? 25 : 587
  }
}

function fromText(channel: NotificationChannel) {
  const config = channel.config
  if (!config) return '—'
  return config.from_name ? `${config.from_name} <${config.from_address}>` : config.from_address
}

function lastTestText(channel: NotificationChannel) {
  if (!channel.last_test_at) return '尚未测试'
  const status = channel.last_test_ok ? '成功' : '失败'
  return `${status} · ${new Date(channel.last_test_at).toLocaleString()}`
}

function lastTestTone(channel: NotificationChannel) {
  if (!channel.last_test_at) return 'muted'
  return channel.last_test_ok ? 'ok' : 'error'
}

async function load() {
  if (!workspace.currentProjectId) {
    channels.value = []
    return
  }
  loading.value = true
  try {
    const data = await notificationApi.list(workspace.currentProjectId)
    channels.value = data.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载通知渠道失败')
  } finally {
    loading.value = false
  }
}

function resetForm() {
  editing.value = null
  form.name = ''
  form.provider = 'custom'
  form.status = 'DISABLED'
  form.host = ''
  form.port = 465
  form.security = 'ssl'
  form.username = ''
  form.password = ''
  form.fromName = ''
  form.fromAddress = ''
  form.timeoutSec = 10
  errorText.value = ''
}

function openCreate() {
  if (!canWrite.value) {
    message.warning('当前角色无权配置通知渠道')
    return
  }
  if (!hasProject.value) {
    message.warning('请先选择项目')
    return
  }
  resetForm()
  modalShow.value = true
}

function openEdit(channel: NotificationChannel) {
  editing.value = channel
  form.name = channel.name
  form.provider = channel.provider || 'custom'
  form.status = channel.status
  form.host = channel.config?.host || ''
  form.port = channel.config?.port || 465
  form.security = channel.config?.security || 'ssl'
  form.username = channel.config?.username || ''
  form.password = ''
  form.fromName = channel.config?.from_name || ''
  form.fromAddress = channel.config?.from_address || ''
  form.timeoutSec = channel.config?.timeout_sec || 10
  errorText.value = ''
  modalShow.value = true
}

async function save() {
  if (!workspace.currentProjectId) return
  errorText.value = ''
  if (!form.name.trim()) {
    errorText.value = '请填写渠道名称'
    return
  }
  if (!form.host.trim()) {
    errorText.value = '请填写 SMTP 主机'
    return
  }
  if (!form.username.trim()) {
    errorText.value = '请填写 SMTP 用户名'
    return
  }
  if (!form.port || form.port < 1 || form.port > 65535) {
    errorText.value = 'SMTP 端口需为 1~65535'
    return
  }
  if (!form.timeoutSec || form.timeoutSec < 5 || form.timeoutSec > 60) {
    errorText.value = '超时时间需为 5~60 秒'
    return
  }
  if (!form.fromAddress.trim() || !isValidEmail(form.fromAddress)) {
    errorText.value = '发件邮箱格式不正确'
    return
  }
  if (!form.password && !editing.value) {
    errorText.value = '请填写 SMTP 密码或授权码'
    return
  }
  saving.value = true
  try {
    const config: Record<string, unknown> = {
      host: form.host.trim(),
      port: form.port,
      username: form.username.trim(),
      from_name: form.fromName.trim(),
      from_address: form.fromAddress.trim(),
      security: form.security,
      timeout_sec: form.timeoutSec,
    }
    if (form.password) config.password = form.password
    const payload: Record<string, unknown> = {
      name: form.name.trim(),
      type: 'EMAIL',
      provider: form.provider,
      status: form.status,
      config,
    }
    if (editing.value) {
      await notificationApi.update(editing.value.id, payload)
      message.success('通知渠道已更新')
    } else {
      await notificationApi.create(workspace.currentProjectId, payload)
      message.success('通知渠道已创建')
    }
    modalShow.value = false
    await load()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggle(channel: NotificationChannel, enabled: boolean) {
  try {
    await notificationApi.setStatus(channel.id, enabled ? 'ACTIVE' : 'DISABLED')
    message.success(enabled ? '通知渠道已启用' : '通知渠道已停用')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
    await load()
  }
}

function remove(channel: NotificationChannel) {
  dialog.warning({
    title: '删除通知渠道',
    content: `确认删除「${channel.name}」？删除后后续通知将无法使用该发送账号。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await notificationApi.remove(channel.id)
        message.success('通知渠道已删除')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      }
    },
  })
}

function openTest(channel: NotificationChannel) {
  testTarget.value = channel
  testTo.value = channel.config?.from_address || ''
  testResult.value = null
  testShow.value = true
}

async function sendTest() {
  if (!testTarget.value) return
  if (!testTo.value.trim() || !isValidEmail(testTo.value)) {
    testResult.value = { ok: false, message: '收件邮箱格式不正确' }
    return
  }
  testSending.value = true
  testResult.value = null
  try {
    const result = await notificationApi.test(testTarget.value.id, testTo.value.trim())
    testResult.value = { ok: true, message: result.message }
    await load()
  } catch (error) {
    testResult.value = { ok: false, message: error instanceof Error ? error.message : '测试发送失败' }
  } finally {
    testSending.value = false
  }
}

watch(() => workspace.currentProjectId, load)
onMounted(load)
</script>

<template>
  <div class="page notifications-page">
    <PageHeader
      eyebrow="NOTIFICATION CHANNELS"
      title="通知渠道"
      subtitle="配置邮件等通知发送账号；后续会按渠道类型扩展更多通知方式。"
    >
      <template #actions>
        <n-button secondary :loading="loading" title="刷新" aria-label="刷新" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button v-if="canWrite" type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新建渠道
        </n-button>
      </template>
    </PageHeader>

    <div class="channel-banner">
      <span class="banner-icon"><MailOutline /></span>
      <div>
        <strong>当前支持邮箱（SMTP）</strong>
        <p>每个项目可以配置多个发送账号，适配 QQ、163、Gmail 等不同邮箱服务商；后续通知能力会从这里选择已启用的渠道。</p>
      </div>
    </div>

    <div v-if="channels.length" class="channel-grid fade-list">
      <article v-for="channel in channels" :key="channel.id" class="channel-card">
        <div class="channel-top">
          <span class="channel-icon"><MailOutline /></span>
          <div class="channel-heading">
            <div class="channel-name-row">
              <h3>{{ channel.name }}</h3>
              <StatusTag :status="channel.status" type="provider" />
            </div>
            <span class="channel-provider">{{ providerLabel(channel.provider) }} · 邮箱</span>
          </div>
          <n-switch
            :value="channel.status === 'ACTIVE'"
            size="small"
            :disabled="!canWrite"
            @update:value="toggle(channel, Boolean($event))"
          />
        </div>

        <div class="channel-endpoint">
          <span class="mono">{{ channel.config?.host || '—' }}:{{ channel.config?.port || '—' }}</span>
          <span class="channel-security">{{ securityLabel(channel.config?.security) }}</span>
        </div>

        <div class="channel-from">
          <span>发件人</span>
          <strong :title="fromText(channel)">{{ fromText(channel) }}</strong>
        </div>

        <div class="channel-test" :class="lastTestTone(channel)">
          <component :is="channel.last_test_at ? (channel.last_test_ok ? CheckmarkCircleOutline : CloseCircleOutline) : WarningOutline" />
          <span>{{ lastTestText(channel) }}</span>
        </div>
        <p v-if="channel.last_test_message && !channel.last_test_ok" class="channel-error">{{ channel.last_test_message }}</p>

        <div class="channel-actions">
          <n-button size="small" secondary :disabled="!canWrite" @click="openTest(channel)">
            <template #icon><FlashOutline /></template>
            测试发送
          </n-button>
          <n-button size="small" quaternary :disabled="!canWrite" @click="openEdit(channel)">
            <template #icon><CreateOutline /></template>
            编辑
          </n-button>
          <n-button size="small" quaternary class="delete-btn" :disabled="!canWrite" title="删除渠道" aria-label="删除渠道" @click="remove(channel)">
            <template #icon><TrashOutline /></template>
          </n-button>
        </div>
      </article>
    </div>

    <EmptyState
      v-else-if="!loading"
      title="还没有通知渠道"
      description="先配置一个邮箱发送账号；后续通知功能会从这里选择已启用的渠道。"
      action="新建通知渠道"
      @action="openCreate"
    >
      <template #icon><MailOutline /></template>
    </EmptyState>

    <n-modal v-model:show="modalShow" :mask-closable="!saving">
      <div class="channel-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT CHANNEL' : 'NEW CHANNEL' }}</div>
            <h2>{{ editing ? '编辑通知渠道' : '新建通知渠道' }}</h2>
            <p>当前支持邮箱渠道；后续会扩展短信、Webhook 等类型。</p>
          </div>
          <div class="modal-head-actions">
            <span class="modal-icon"><MailOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="modalShow = false">×</button>
          </div>
        </div>
        <div class="modal-body">
          <div class="two-col">
            <div class="field">
              <label>渠道名称 <span class="required">*</span></label>
              <n-input v-model:value="form.name" placeholder="例如 业务通知邮箱" maxlength="64" />
            </div>
            <div class="field">
              <label>渠道类型</label>
              <n-select :value="'EMAIL'" :options="[{ label: '邮箱 / SMTP', value: 'EMAIL' }]" disabled />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>邮箱服务商</label>
              <n-select :value="form.provider" :options="providerOptions" @update:value="onProviderChange" />
            </div>
            <div class="field">
              <label>加密方式</label>
              <n-select :value="form.security" :options="securityOptions" @update:value="onSecurityChange" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>SMTP 主机 <span class="required">*</span></label>
              <n-input v-model:value="form.host" placeholder="smtp.qq.com" class="mono-input" />
            </div>
            <div class="field">
              <label>端口</label>
              <n-input-number v-model:value="form.port" :min="1" :max="65535" style="width: 100%" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>发件邮箱 <span class="required">*</span></label>
              <n-input v-model:value="form.fromAddress" placeholder="notice@example.com" />
            </div>
            <div class="field">
              <label>发件人名称</label>
              <n-input v-model:value="form.fromName" placeholder="智审图档通知" maxlength="64" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>SMTP 用户名 <span class="required">*</span></label>
              <n-input v-model:value="form.username" placeholder="通常是完整邮箱地址" />
            </div>
            <div class="field">
              <label>{{ editing ? '密码 / 授权码（留空不修改）' : '密码 / 授权码' }} <span v-if="!editing" class="required">*</span></label>
              <n-input
                v-model:value="form.password"
                type="password"
                show-password-on="click"
                :placeholder="editing?.has_password ? '已配置，留空保留' : '邮箱授权码，不是登录密码'"
              />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>超时（秒）</label>
              <n-input-number v-model:value="form.timeoutSec" :min="5" :max="60" style="width: 100%" />
            </div>
            <div class="field">
              <label>状态</label>
              <n-select v-model:value="form.status" :options="[{ label: '启用', value: 'ACTIVE' }, { label: '停用', value: 'DISABLED' }]" />
            </div>
          </div>
          <div class="channel-hint">
            <WarningOutline />
            <span>QQ、163、126 等邮箱通常需要先在邮箱设置中开启 SMTP，并使用「授权码」而不是网页登录密码。</span>
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">{{ editing ? '保存修改' : '创建渠道' }}</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="testShow" :mask-closable="!testSending">
      <div class="test-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">TEST DELIVERY</div>
            <h2>测试发送</h2>
            <p>通过「{{ testTarget?.name || '通知渠道' }}」发送一封测试邮件。</p>
          </div>
          <div class="modal-head-actions">
            <span class="modal-icon"><FlashOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="testShow = false">×</button>
          </div>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>收件邮箱</label>
            <n-input v-model:value="testTo" placeholder="name@example.com" :disabled="testSending" />
          </div>
          <div class="test-hint">建议先发给自己或测试邮箱，确认 SMTP 账号和授权码可用。</div>
          <div v-if="testResult" class="test-result" :class="{ ok: testResult.ok }">
            <component :is="testResult.ok ? CheckmarkCircleOutline : CloseCircleOutline" />
            <span>{{ testResult.message }}</span>
          </div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="testSending" @click="testShow = false">关闭</n-button>
          <n-button type="primary" :loading="testSending" @click="sendTest">发送测试</n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.channel-banner {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 16px;
  padding: 14px 16px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface-soft);
}

.banner-icon {
  display: flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.banner-icon :deep(svg) {
  width: 16px;
  height: 16px;
}

.channel-banner strong {
  display: block;
  color: var(--ink);
  font-size: 12px;
  font-weight: 600;
}

.channel-banner p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.6;
}

.channel-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  align-items: stretch;
  gap: 14px;
}

.channel-card {
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

.channel-card:hover {
  transform: translateY(-2px);
  border-color: var(--brand-border);
  box-shadow: var(--shadow-md);
}

.channel-top {
  display: flex;
  align-items: flex-start;
  gap: 11px;
}

.channel-icon {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: linear-gradient(145deg, #edf4ff, #e0ebff);
  color: var(--brand);
  flex: none;
}

.channel-icon :deep(svg) {
  width: 19px;
  height: 19px;
}

.channel-heading {
  min-width: 0;
  flex: 1;
}

.channel-name-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.channel-name-row h3 {
  overflow: hidden;
  margin: 0;
  color: var(--ink);
  font-size: 14.5px;
  font-weight: 620;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.channel-name-row :deep(.status-pill) {
  height: 20px;
  padding: 0 7px;
  gap: 4px;
  font-size: 10px;
}

.channel-name-row :deep(.status-pill i) {
  width: 5px;
  height: 5px;
}

.channel-provider {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9.5px;
}

.channel-endpoint {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 14px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--surface-soft);
  color: var(--muted);
  font-size: 10px;
}

.channel-endpoint .mono {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.channel-security {
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 8.5px;
  flex: none;
}

.channel-from {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-top: 10px;
  color: var(--muted-light);
  font-size: 9px;
}

.channel-from strong {
  overflow: hidden;
  color: var(--text);
  font-size: 10px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.channel-test {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-top: 10px;
  color: var(--muted-light);
  font-size: 9.5px;
}

.channel-test :deep(svg) {
  width: 12px;
  height: 12px;
  flex: none;
}

.channel-test.ok {
  color: #15803d;
}

.channel-test.error {
  color: #b91c1c;
}

.channel-error {
  display: -webkit-box;
  margin: 6px 0 0;
  overflow: hidden;
  color: #b91c1c;
  font-size: 9px;
  line-height: 1.5;
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.channel-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: auto;
  padding-top: 14px;
}

.channel-actions .delete-btn {
  margin-left: auto;
  color: var(--muted-light);
}

.channel-modal {
  width: min(720px, calc(100vw - 32px));
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.test-modal {
  width: min(480px, calc(100vw - 32px));
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  padding: 22px 24px 16px;
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

.modal-head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
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
  flex: none;
}

.modal-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.modal-close {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--muted-light);
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}

.modal-body {
  max-height: min(72vh, 700px);
  overflow-y: auto;
  padding: 20px 24px;
}

.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.field {
  margin-bottom: 15px;
}

.field > label {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 6px;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 550;
}

.required {
  color: var(--red);
}

.channel-hint {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  padding: 9px 11px;
  border-radius: 8px;
  background: var(--amber-soft);
  color: #a16207;
  font-size: 10px;
  line-height: 1.6;
}

.channel-hint :deep(svg) {
  width: 13px;
  height: 13px;
  margin-top: 2px;
  flex: none;
}

.modal-error {
  margin-top: 12px;
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
  padding: 14px 24px 20px;
}

.test-hint {
  color: var(--muted-light);
  font-size: 10px;
  line-height: 1.6;
}

.test-result {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  margin-top: 12px;
  padding: 9px 11px;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 10.5px;
  line-height: 1.5;
}

.test-result.ok {
  background: var(--green-soft);
  color: #15803d;
}

.test-result :deep(svg) {
  width: 13px;
  height: 13px;
  margin-top: 2px;
  flex: none;
}

@media (max-width: 640px) {
  .two-col {
    grid-template-columns: 1fr;
  }

  .modal-head,
  .modal-body,
  .modal-footer {
    padding-left: 18px;
    padding-right: 18px;
  }
}

@media (max-width: 560px) {
  .channel-card {
    padding: 16px;
  }
}
</style>
