<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NInput, NModal, NSelect, NSwitch, useDialog, useMessage } from 'naive-ui'
import {
  AddOutline,
  CheckmarkCircleOutline,
  HelpCircleOutline,
  KeyOutline,
  RefreshOutline,
  RepeatOutline,
  ShieldCheckmarkOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import CopyButton from '@/components/CopyButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import { auditApi, credentialApi } from '@/api'
import type { APICredential, Audit } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const workspace = useWorkspaceStore()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const credentials = ref<APICredential[]>([])
const audits = ref<Audit[]>([])
const modalShow = ref(false)
const editing = ref<APICredential | null>(null)
const form = reactive({ name: '', audit_ids: [] as number[], status: 'ACTIVE' })
const secretShow = ref(false)
const issuedSecret = ref({ name: '', app_key: '', app_secret: '' })
const errorText = ref('')

const auditOptions = computed(() =>
  audits.value.map((a) => ({
    label: `${a.name} · ${a.code}${a.status !== 'ACTIVE' ? `（${a.status}）` : ''}`,
    value: a.id,
  })),
)
const activeCount = computed(() => credentials.value.filter((c) => c.status === 'ACTIVE').length)
const activeAuditCount = computed(() => audits.value.filter((a) => a.status === 'ACTIVE').length)

async function load() {
  if (!workspace.currentProjectId) {
    credentials.value = []
    audits.value = []
    return
  }
  loading.value = true
  try {
    const [credentialData, auditData] = await Promise.all([
      credentialApi.list(workspace.currentProjectId),
      auditApi.list({ project_id: workspace.currentProjectId, page: 1, page_size: 100 }),
    ])
    credentials.value = credentialData.items
    audits.value = auditData.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载项目凭证失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  if (!workspace.currentProjectId) {
    message.warning('请先选择项目')
    return
  }
  editing.value = null
  form.name = ''
  form.audit_ids = audits.value.filter((a) => a.status === 'ACTIVE').map((a) => a.id)
  form.status = 'ACTIVE'
  errorText.value = ''
  modalShow.value = true
}

function openEdit(credential: APICredential) {
  editing.value = credential
  form.name = credential.name
  form.audit_ids = [...credential.audit_ids]
  form.status = credential.status
  errorText.value = ''
  modalShow.value = true
}

async function save() {
  errorText.value = ''
  if (!form.name.trim()) {
    errorText.value = '请填写凭证名称'
    return
  }
  if (!form.audit_ids.length) {
    errorText.value = '请至少授权一个审核配置'
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await credentialApi.update(workspace.currentProjectId!, editing.value.id, {
        name: form.name.trim(),
        audit_ids: form.audit_ids,
        status: form.status,
      })
      message.success('凭证授权已更新')
    } else {
      const result = await credentialApi.create(workspace.currentProjectId!, {
        name: form.name.trim(),
        audit_ids: form.audit_ids,
        status: form.status,
      })
      issuedSecret.value = {
        name: result.credential.name,
        app_key: result.credential.app_key,
        app_secret: result.app_secret,
      }
      secretShow.value = true
      message.success('凭证已创建，请保存一次性显示的 AppSecret')
    }
    modalShow.value = false
    await load()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggle(credential: APICredential, value: boolean) {
  try {
    await credentialApi.update(workspace.currentProjectId!, credential.id, { status: value ? 'ACTIVE' : 'DISABLED' })
    credential.status = value ? 'ACTIVE' : 'DISABLED'
    message.success(value ? '凭证已启用' : '凭证已停用')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
    await load()
  }
}

function rotate(credential: APICredential) {
  dialog.warning({
    title: '轮换凭证密钥',
    content: `轮换「${credential.name}」后旧 AppSecret 立即失效，所有使用该凭证的开放 API 与 WebSocket 连接都会验签失败。确认继续？`,
    positiveText: '轮换密钥',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const result = await credentialApi.rotate(workspace.currentProjectId!, credential.id)
        issuedSecret.value = { name: credential.name, app_key: result.app_key, app_secret: result.app_secret }
        secretShow.value = true
        message.success('密钥已轮换')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '轮换失败')
      }
    },
  })
}

function remove(credential: APICredential) {
  dialog.warning({
    title: '删除接入凭证',
    content: `删除「${credential.name}」后，所有使用该 AppKey 的业务请求会立即失败。确认继续？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await credentialApi.remove(workspace.currentProjectId!, credential.id)
        message.success('凭证已删除')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      }
    },
  })
}

function openAuditReport(scopeId: number) {
  router.push({
    name: 'analytics',
    query: { project_id: String(workspace.currentProjectId), audit_id: String(scopeId) },
  })
}

watch(() => workspace.currentProjectId, load)
onMounted(load)
</script>

<template>
  <div class="page credentials-page">
    <PageHeader
      eyebrow="ACCESS CREDENTIALS"
      title="接入凭证"
      subtitle="为开放 API 与 WebSocket 管理独立 AppKey，并按审核配置精确授权访问范围。"
    >
      <template #actions>
        <n-button secondary @click="router.push({ name: 'help' })">
          <template #icon><HelpCircleOutline /></template>
          对接文档
        </n-button>
        <n-button quaternary circle :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新建凭证
        </n-button>
      </template>
    </PageHeader>

    <div class="credential-notice">
      <ShieldCheckmarkOutline />
      <div>
        <strong>AppKey 是项目级凭证，但访问权限按审核配置授权</strong>
        <p>调用时必须同时满足：凭证有效 → 项目匹配 → 审核配置存在且启用 → 该审核配置在当前凭证的授权列表内。审核配置删除后立即无法访问。</p>
      </div>
    </div>

    <div v-if="credentials.length" class="credential-summary">
      <div><span>凭证总数</span><strong>{{ credentials.length }}</strong></div>
      <div><span>启用中</span><strong>{{ activeCount }}</strong></div>
      <div><span>已启用审核</span><strong>{{ activeAuditCount }}</strong></div>
      <n-button text type="primary" @click="router.push({ name: 'audits' })">管理审核配置 →</n-button>
    </div>

    <div v-if="credentials.length" class="credential-grid fade-list">
      <article v-for="credential in credentials" :key="credential.id" class="credential-card">
        <div class="cred-top">
          <span class="cred-icon"><KeyOutline /></span>
          <div class="cred-heading">
            <div class="cred-name-row">
              <h3>{{ credential.name }}</h3>
              <StatusTag :status="credential.status" type="provider" />
            </div>
            <span class="cred-created">创建于 {{ credential.created_at?.slice(0, 10) }}</span>
          </div>
          <n-switch :value="credential.status === 'ACTIVE'" size="small" @update:value="toggle(credential, $event)" />
        </div>

        <div class="cred-key-block">
          <span>AppKey</span>
          <div class="cred-key-line">
            <code>{{ credential.app_key }}</code>
            <copy-button :text="credential.app_key" label="复制" size="tiny" />
          </div>
        </div>
        <div class="cred-key-block">
          <span>AppSecret</span>
          <div class="cred-key-line">
            <code>{{ credential.app_secret_masked || '未配置' }}</code>
            <n-button size="tiny" secondary @click="rotate(credential)">
              <template #icon><RepeatOutline /></template>
              轮换
            </n-button>
          </div>
        </div>

        <div class="scope-head">
          <span>授权审核配置</span>
          <n-button text type="primary" @click="openEdit(credential)">编辑授权</n-button>
        </div>
        <div class="scope-list">
          <button
            v-for="scope in credential.scopes"
            :key="scope.id"
            type="button"
            class="scope-chip"
            :class="{ missing: !scope.exists }"
            @click="scope.exists ? openAuditReport(scope.id) : undefined"
          >
            <span>{{ scope.name || `审核 #${scope.id}` }}</span>
            <small>{{ scope.exists ? scope.code : '已删除/不可用' }}</small>
          </button>
          <span v-if="!credential.scopes.length" class="scope-missing">未授权任何审核配置</span>
        </div>

        <div class="cred-foot">
          <span class="last-used">最近使用：{{ credential.last_used_at ? credential.last_used_at.replace('T', ' ').slice(0, 19) : '从未使用' }}</span>
          <div class="cred-actions">
            <n-button size="tiny" quaternary @click="openEdit(credential)">编辑</n-button>
            <n-button size="tiny" quaternary class="danger" @click="remove(credential)">
              <template #icon><TrashOutline /></template>
            </n-button>
          </div>
        </div>
      </article>

      <button class="credential-add" type="button" @click="openCreate">
        <span><AddOutline /></span>
        <strong>新建接入凭证</strong>
        <small>创建 AppKey 并选择可访问的审核配置</small>
      </button>
    </div>

    <EmptyState
      v-else-if="!loading"
      title="还没有接入凭证"
      description="创建一条项目凭证并授权具体审核配置，业务方才能调用开放 API 或 WebSocket。"
      action="新建凭证"
      @action="openCreate"
    >
      <template #icon><KeyOutline /></template>
    </EmptyState>

    <n-modal v-model:show="modalShow" :mask-closable="!saving">
      <div class="cred-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT CREDENTIAL' : 'NEW CREDENTIAL' }}</div>
            <h2>{{ editing ? '编辑凭证授权' : '新建项目凭证' }}</h2>
            <p>没有勾选的审核配置，该凭证调用时会返回 403 AUDIT_OUT_OF_SCOPE。</p>
          </div>
          <span class="modal-icon"><KeyOutline /></span>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>凭证名称 <span class="required">*</span></label>
            <n-input v-model:value="form.name" placeholder="例如 业务系统 A · 生产" maxlength="64" />
          </div>
          <div class="field">
            <label>允许访问的审核配置 <span class="required">*</span></label>
            <n-select
              v-model:value="form.audit_ids"
              multiple
              filterable
              :options="auditOptions"
              placeholder="选择审核配置"
              :max-tag-count="6"
            />
            <div class="form-hint">审核配置删除后授权自动失效；如果之后恢复且授权 ID 仍在，访问会恢复。</div>
          </div>
          <div class="field">
            <label>状态</label>
            <n-select v-model:value="form.status" :options="[{ label: '启用', value: 'ACTIVE' }, { label: '停用', value: 'DISABLED' }]" />
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">{{ editing ? '保存授权' : '创建凭证' }}</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="secretShow">
      <div class="secret-modal">
        <div class="secret-head">
          <span class="secret-icon"><CheckmarkCircleOutline /></span>
          <div><h3>凭证已生成</h3><p>{{ issuedSecret.name }} · AppSecret 只显示这一次</p></div>
        </div>
        <div class="secret-row">
          <span>AppKey</span>
          <div><code>{{ issuedSecret.app_key }}</code><copy-button :text="issuedSecret.app_key" label="复制" size="tiny" /></div>
        </div>
        <div class="secret-row">
          <span>AppSecret</span>
          <div><code>{{ issuedSecret.app_secret }}</code><copy-button :text="issuedSecret.app_secret" label="复制" size="tiny" /></div>
        </div>
        <div class="secret-warning"><WarningOutline /> 请立即保存到业务系统的密钥管理服务。关闭后无法再次查看；丢失只能重新轮换。</div>
        <div class="secret-actions"><n-button type="primary" @click="secretShow = false">我已保存</n-button></div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.credential-notice {
  display: flex;
  align-items: flex-start;
  gap: 11px;
  margin-bottom: 15px;
  padding: 14px 16px;
  border: 1px solid #dce7fb;
  border-radius: 12px;
  background: linear-gradient(120deg, #f8fbff, #fff);
}

.credential-notice :deep(svg) {
  width: 18px;
  height: 18px;
  margin-top: 2px;
  color: var(--brand);
  flex-shrink: 0;
}

.credential-notice strong {
  display: block;
  color: #1e4b9e;
  font-size: 12px;
  font-weight: 600;
}

.credential-notice p {
  margin: 3px 0 0;
  color: #64748b;
  font-size: 10.5px;
  line-height: 1.7;
}

.credential-summary {
  display: flex;
  align-items: center;
  gap: 26px;
  margin-bottom: 14px;
  padding: 12px 18px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.credential-summary > div span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
}

.credential-summary > div strong {
  display: block;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 17px;
  font-weight: 650;
}

.credential-summary > :last-child {
  margin-left: auto;
}

.credential-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.credential-card {
  display: flex;
  flex-direction: column;
  padding: 19px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.cred-top {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.cred-icon {
  display: flex;
  width: 37px;
  height: 37px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.cred-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.cred-heading {
  flex: 1;
  min-width: 0;
}

.cred-name-row {
  display: flex;
  align-items: center;
  gap: 7px;
}

.cred-name-row h3 {
  margin: 0;
  color: var(--ink);
  font-size: 14px;
  font-weight: 620;
}

.cred-created {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9px;
}

.cred-key-block {
  margin-top: 12px;
}

.cred-key-block > span {
  display: block;
  margin-bottom: 4px;
  color: var(--muted-light);
  font-size: 9px;
}

.cred-key-line {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cred-key-line code {
  flex: 1;
  overflow: hidden;
  padding: 6px 9px;
  border-radius: 7px;
  background: #f8fafc;
  color: #475569;
  font-family: var(--mono);
  font-size: 9.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.scope-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 14px;
  padding-top: 11px;
  border-top: 1px dashed #edf1f7;
}

.scope-head > span {
  color: var(--muted-light);
  font-size: 9px;
  font-weight: 550;
}

.scope-list {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 7px;
}

.scope-chip {
  display: flex;
  align-items: baseline;
  gap: 4px;
  padding: 3px 7px;
  border: 1px solid #e5ebf4;
  border-radius: 6px;
  background: #fbfcfe;
  color: #475569;
  font-size: 9px;
  cursor: pointer;
}

.scope-chip:hover {
  border-color: #bcd0f8;
  background: #f8fbff;
}

.scope-chip small {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 7.5px;
}

.scope-chip.missing {
  border-color: #f2dede;
  background: #fff9f9;
  color: #b45353;
  cursor: default;
}

.scope-missing {
  color: var(--muted-light);
  font-size: 9px;
}

.cred-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: auto;
  padding-top: 14px;
}

.last-used {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.5px;
}

.cred-actions {
  display: flex;
  gap: 2px;
}

.cred-actions .danger {
  color: #b0bac8;
}

.cred-actions .danger:hover {
  color: var(--red);
}

.credential-add {
  display: flex;
  min-height: 220px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 5px;
  border: 1px dashed #d5ddea;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.55);
  color: var(--muted);
  cursor: pointer;
}

.credential-add:hover {
  border-color: #aac4f8;
  background: #f9fbff;
  color: var(--brand);
}

.credential-add > span {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: #f1f5fb;
}

.credential-add > span :deep(svg) {
  width: 17px;
  height: 17px;
}

.credential-add strong {
  font-size: 12px;
  font-weight: 550;
}

.credential-add small {
  color: var(--muted-light);
  font-size: 9px;
}

.cred-modal,
.secret-modal {
  width: min(520px, calc(100vw - 32px));
  border-radius: 17px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px 16px;
  border-bottom: 1px solid #f0f3f8;
}

.modal-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.modal-head h2 {
  margin: 4px 0 3px;
  font-size: 19px;
  font-weight: 650;
}

.modal-head p {
  margin: 0;
  color: var(--muted);
  font-size: 10.5px;
}

.modal-icon {
  display: flex;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: var(--brand);
  color: #fff;
}

.modal-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.modal-body {
  padding: 19px 24px;
}

.field {
  margin-bottom: 15px;
}

.field label {
  display: block;
  margin-bottom: 6px;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 550;
}

.required {
  color: var(--red);
}

.modal-error {
  margin-top: 8px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--red-soft);
  color: #b91c1c;
  font-size: 10px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 24px;
  border-top: 1px solid #f0f3f8;
}

.secret-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 22px 24px 15px;
}

.secret-icon {
  display: flex;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--green-soft);
  color: var(--green);
}

.secret-icon :deep(svg) {
  width: 19px;
  height: 19px;
}

.secret-head h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 650;
}

.secret-head p {
  margin: 2px 0 0;
  color: var(--muted);
  font-size: 10px;
}

.secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 24px 10px;
  padding: 9px 10px;
  border-radius: 8px;
  background: #f8fafc;
}

.secret-row > span {
  width: 70px;
  color: var(--muted-light);
  font-size: 9px;
  flex-shrink: 0;
}

.secret-row > div {
  display: flex;
  flex: 1;
  min-width: 0;
  align-items: center;
  gap: 7px;
}

.secret-row code {
  overflow: hidden;
  flex: 1;
  color: #334155;
  font-family: var(--mono);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.secret-warning {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin: 14px 24px;
  padding: 9px 10px;
  border: 1px solid #f4e1bb;
  border-radius: 8px;
  background: #fffbf2;
  color: #a16207;
  font-size: 9.5px;
  line-height: 1.6;
}

.secret-warning :deep(svg) {
  width: 13px;
  height: 13px;
  margin-top: 1px;
  flex-shrink: 0;
}

.secret-actions {
  display: flex;
  justify-content: flex-end;
  padding: 0 24px 20px;
}

@media (max-width: 1000px) {
  .credential-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 600px) {
  .credential-summary {
    flex-wrap: wrap;
    gap: 14px;
  }
  .credential-summary > :last-child {
    margin-left: 0;
  }
}
</style>
