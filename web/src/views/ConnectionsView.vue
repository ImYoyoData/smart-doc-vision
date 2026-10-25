<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  NButton,
  NInput,
  NModal,
  NSelect,
  NSpin,
  NSwitch,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  AddOutline,
  CreateOutline,
  LinkOutline,
  PauseOutline,
  PlayOutline,
  RefreshOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import { redisConnectionApi } from '@/api'
import type { RedisConnection } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'

const workspace = useWorkspaceStore()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const testingId = ref<number | null>(null)
const connections = ref<RedisConnection[]>([])
const modalShow = ref(false)
const editing = ref<RedisConnection | null>(null)
const errorText = ref('')

const form = reactive({
  name: '',
  addr: '127.0.0.1:6379',
  username: '',
  password: '',
  tls: false,
  status: 'DISABLED',
  autoResume: true,
})

const hasProject = computed(() => Boolean(workspace.currentProjectId))

function healthLabel(connection: RedisConnection) {
  if (connection.health === 'OK') return '健康'
  if (connection.health === 'UNHEALTHY') return '不可用'
  return '未测试'
}

function healthTone(connection: RedisConnection) {
  if (connection.health === 'OK') return 'ok'
  if (connection.health === 'UNHEALTHY') return 'error'
  return 'muted'
}

function lastTestText(connection: RedisConnection) {
  if (!connection.last_test_at) return '尚未测试'
  return `${connection.last_test_ok ? '成功' : '失败'} · ${connection.last_test_ms} ms · ${new Date(connection.last_test_at).toLocaleString()}`
}

async function load() {
  if (!workspace.currentProjectId) {
    connections.value = []
    return
  }
  loading.value = true
  try {
    const data = await redisConnectionApi.list(workspace.currentProjectId)
    connections.value = data.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载 Redis 连接失败')
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.name = ''
  form.addr = '127.0.0.1:6379'
  form.username = ''
  form.password = ''
  form.tls = false
  form.status = 'DISABLED'
  form.autoResume = true
  errorText.value = ''
  editing.value = null
}

function openCreate() {
  if (!hasProject.value) {
    message.warning('请先选择项目')
    return
  }
  resetForm()
  modalShow.value = true
}

function openEdit(connection: RedisConnection) {
  editing.value = connection
  form.name = connection.name
  form.addr = connection.addr
  form.username = connection.username || ''
  form.password = ''
  form.tls = connection.tls
  form.status = connection.status
  form.autoResume = connection.auto_resume_triggers
  errorText.value = ''
  modalShow.value = true
}

async function save() {
  if (!workspace.currentProjectId) return
  errorText.value = ''
  if (!form.name.trim() || !form.addr.trim()) {
    errorText.value = '请填写连接名称和地址'
    return
  }
  saving.value = true
  try {
    const payload: Record<string, unknown> = {
      name: form.name.trim(),
      addr: form.addr.trim(),
      username: form.username.trim(),
      tls: form.tls,
      status: form.status,
      auto_resume_triggers: form.autoResume,
    }
    if (form.password) payload.password = form.password
    if (editing.value) {
      await redisConnectionApi.update(editing.value.id, payload)
      message.success('Redis 连接已更新')
    } else {
      await redisConnectionApi.create(workspace.currentProjectId, payload)
      message.success('Redis 连接已创建，请先测试连接')
    }
    modalShow.value = false
    await load()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function test(connection: RedisConnection) {
  testingId.value = connection.id
  try {
    const result = await redisConnectionApi.test(connection.id)
    message.success(`连接成功 · ${result.latency_ms} ms`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '连接测试失败', { duration: 6000 })
  } finally {
    testingId.value = null
    await load()
  }
}

async function setStatus(connection: RedisConnection, status: string) {
  try {
    await redisConnectionApi.update(connection.id, { status })
    message.success(status === 'ACTIVE' ? '连接已启用，请测试后再启用触发源' : '连接已停用，相关触发源已自动暂停')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
  }
}

function remove(connection: RedisConnection) {
  dialog.warning({
    title: '删除 Redis 连接',
    content: `确认删除「${connection.name}」？被触发源或结果动作引用时无法删除。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await redisConnectionApi.remove(connection.id)
        message.success('Redis 连接已删除')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      }
    },
  })
}

watch(() => workspace.currentProjectId, () => {
  void load()
})

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="page connections-page">
    <PageHeader
      eyebrow="DATA CONNECTIONS"
      title="数据连接"
      subtitle="项目级 Redis 连接，供审核触发和结果动作复用。平台自身缓存使用的 Redis 与这里配置的业务 Redis 相互独立。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新增 Redis 连接
        </n-button>
      </template>
    </PageHeader>

    <div class="connection-note">
      <LinkOutline />
      <span>一个项目可以配置多个 Redis 实例。具体监听的 DB 编号和 Stream 在审核触发中填写；结果动作也会复用这里的连接。</span>
    </div>

    <div v-if="!hasProject" class="panel">
      <EmptyState
        title="请选择项目"
        description="数据连接按项目隔离，先在左上角选择一个项目。"
        action="前往项目空间"
        @action="$router.push({ name: 'projects' })"
      />
    </div>

    <div v-else-if="loading" class="panel connection-loading">
      <n-spin size="small" />
      <span>正在加载 Redis 连接…</span>
    </div>

    <div v-else-if="!connections.length" class="panel">
      <EmptyState
        title="还没有 Redis 连接"
        description="先添加业务 Redis 地址并完成连接测试，然后在审核配置的 Redis 触发中选择它。"
        action="新增 Redis 连接"
        @action="openCreate"
      />
    </div>

    <div v-else class="connection-grid">
      <article v-for="connection in connections" :key="connection.id" class="connection-card">
        <div class="connection-head">
          <span class="connection-icon"><LinkOutline /></span>
          <div class="connection-title">
            <div class="connection-name-row">
              <h3>{{ connection.name }}</h3>
              <span class="connection-status" :class="connection.status === 'ACTIVE' ? 'active' : 'disabled'">
                {{ connection.status === 'ACTIVE' ? '已启用' : '已停用' }}
              </span>
              <span class="connection-health" :class="healthTone(connection)">{{ healthLabel(connection) }}</span>
            </div>
            <span class="connection-addr mono">{{ connection.addr }}</span>
          </div>
        </div>

        <div class="connection-meta">
          <div><span>用户名</span><strong>{{ connection.username || '未设置' }}</strong></div>
          <div><span>TLS</span><strong>{{ connection.tls ? '开启' : '关闭' }}</strong></div>
          <div><span>自动恢复触发</span><strong>{{ connection.auto_resume_triggers ? '开启' : '关闭' }}</strong></div>
          <div><span>密码</span><strong>{{ connection.has_password ? '已配置' : '未设置' }}</strong></div>
        </div>

        <div class="connection-test" :class="{ failed: connection.last_test_at && !connection.last_test_ok }">
          <span>最近测试</span>
          <strong>{{ lastTestText(connection) }}</strong>
          <small v-if="connection.last_test_message">{{ connection.last_test_message }}</small>
        </div>

        <div class="connection-actions">
          <n-button size="small" secondary :loading="testingId === connection.id" @click="test(connection)">测试连接</n-button>
          <n-button
            v-if="connection.status === 'ACTIVE'"
            size="small"
            quaternary
            @click="setStatus(connection, 'DISABLED')"
          >
            <template #icon><PauseOutline /></template>
            停用
          </n-button>
          <n-button v-else size="small" quaternary @click="setStatus(connection, 'ACTIVE')">
            <template #icon><PlayOutline /></template>
            启用
          </n-button>
          <n-button size="small" quaternary @click="openEdit(connection)">
            <template #icon><CreateOutline /></template>
            编辑
          </n-button>
          <n-button size="small" quaternary type="error" @click="remove(connection)">
            <template #icon><TrashOutline /></template>
          </n-button>
        </div>
      </article>
    </div>

    <n-modal v-model:show="modalShow" :mask-closable="!saving">
      <div class="connection-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT REDIS CONNECTION' : 'NEW REDIS CONNECTION' }}</div>
            <h2>{{ editing ? '编辑 Redis 连接' : '新增 Redis 连接' }}</h2>
            <p>密码加密存储；启用前请先测试连接。DB 编号在触发源中单独选择。</p>
          </div>
          <div class="modal-head-actions">
            <span class="modal-icon"><LinkOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="modalShow = false">×</button>
          </div>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>连接名称 <span class="required">*</span></label>
            <n-input v-model:value="form.name" placeholder="例如 业务 Redis - 合同档案" maxlength="64" />
          </div>
          <div class="field">
            <label>Redis 地址 <span class="required">*</span></label>
            <n-input v-model:value="form.addr" placeholder="127.0.0.1:6379" class="mono-input" />
            <div class="form-hint">host:port；不写端口时默认 6379。</div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>用户名</label>
              <n-input v-model:value="form.username" placeholder="可选，Redis 6 ACL 用户" />
            </div>
            <div class="field">
              <label>密码</label>
              <n-input
                v-model:value="form.password"
                type="password"
                show-password-on="click"
                :placeholder="editing && editing.has_password ? '留空保留现有密码' : '可选'"
              />
            </div>
          </div>
          <div class="two-col">
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
            <div class="switch-field">
              <div>
                <strong>自动恢复触发源</strong>
                <span>网络故障恢复后自动继续监听</span>
              </div>
              <n-switch v-model:value="form.autoResume" />
            </div>
          </div>
          <div class="switch-field">
            <div>
              <strong>TLS</strong>
              <span>使用 TLS 连接 Redis</span>
            </div>
            <n-switch v-model:value="form.tls" />
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">保存连接</n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.connection-note {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 16px;
  padding: 10px 12px;
  border: 1px solid #dbe7fb;
  border-radius: 10px;
  background: #f6faff;
  color: #4b6a9b;
  font-size: 10px;
  line-height: 1.7;
}

.connection-note svg {
  width: 15px;
  height: 15px;
  flex-shrink: 0;
  margin-top: 1px;
}

.connection-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 40px;
  color: var(--muted);
  font-size: 10px;
}

.connection-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.connection-card {
  padding: 16px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 8px 24px -18px rgba(15, 23, 42, 0.25);
}

.connection-head {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.connection-icon {
  display: flex;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: #edf4ff;
  color: #2b6cf6;
}

.connection-icon svg {
  width: 16px;
  height: 16px;
}

.connection-title {
  min-width: 0;
  flex: 1;
}

.connection-name-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.connection-name-row h3 {
  margin: 0;
  color: var(--ink);
  font-size: 13px;
}

.connection-status,
.connection-health {
  padding: 1px 7px;
  border-radius: 99px;
  font-size: 8.5px;
}

.connection-status.active {
  background: #ecfdf3;
  color: #15803d;
}

.connection-status.disabled {
  background: #f3f4f6;
  color: #6b7280;
}

.connection-health.ok {
  background: #eefbf3;
  color: #15803d;
}

.connection-health.error {
  background: #fef2f2;
  color: #b91c1c;
}

.connection-health.muted {
  background: #f7f5ff;
  color: #6d5bb5;
}

.connection-addr {
  display: block;
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 9.5px;
}

.connection-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin: 14px 0;
  padding: 10px;
  border-radius: 9px;
  background: #f8fafd;
}

.connection-meta > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.connection-meta span {
  color: var(--muted-light);
  font-size: 8.5px;
}

.connection-meta strong {
  color: var(--ink);
  font-size: 10px;
}

.connection-test {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border: 1px solid #e8eef7;
  border-radius: 8px;
  background: #fff;
  font-size: 9.5px;
}

.connection-test span {
  color: var(--muted-light);
}

.connection-test strong {
  color: var(--muted);
  font-weight: 500;
}

.connection-test small {
  color: var(--muted-light);
  font-size: 8.5px;
  word-break: break-all;
}

.connection-test.failed {
  border-color: #fecaca;
  background: #fff7f7;
}

.connection-test.failed strong,
.connection-test.failed small {
  color: #b91c1c;
}

.connection-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 12px;
}

.connection-modal {
  width: min(640px, calc(100vw - 24px));
  overflow: hidden;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  padding: 22px 24px 17px;
  border-bottom: 1px solid #f0f3f8;
  background: linear-gradient(130deg, #f8fbff, #fff);
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
  color: var(--ink);
  font-size: 19px;
  font-weight: 650;
  letter-spacing: -0.02em;
}

.modal-head p {
  margin: 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.6;
}

.modal-icon {
  display: flex;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--brand);
  color: #fff;
  box-shadow: 0 8px 18px -6px rgba(43, 108, 246, 0.5);
}

.modal-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.modal-body {
  display: flex;
  max-height: min(70vh, 620px);
  flex-direction: column;
  gap: 15px;
  overflow-y: auto;
  padding: 20px 24px;
}

.field {
  min-width: 0;
}

.field label {
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

.form-hint {
  margin-top: 5px;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.5;
}

.mono-input :deep(input) {
  font-family: var(--mono);
  font-size: 11px;
}

.two-col {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 14px;
}

.two-col .switch-field {
  padding-top: 24px;
}

.switch-field {
  display: flex;
  min-height: 34px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 12px;
  border: 1px solid #edf1f7;
  border-radius: 10px;
  background: #fbfcfe;
}

.switch-field strong {
  display: block;
  color: var(--text);
  font-size: 11px;
  font-weight: 550;
}

.switch-field span {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.4;
}

.modal-error {
  padding: 9px 11px;
  border: 1px solid #fecaca;
  border-radius: 9px;
  background: #fff7f7;
  color: #b91c1c;
  font-size: 10px;
  line-height: 1.5;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 24px;
  border-top: 1px solid #f0f3f8;
  background: #fff;
}

@media (max-width: 900px) {
  .connection-grid {
    grid-template-columns: 1fr;
  }

  .connection-meta {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .modal-head {
    padding: 18px 18px 14px;
  }

  .modal-icon {
    width: 36px;
    height: 36px;
  }

  .modal-body {
    max-height: calc(100vh - 230px);
    padding: 16px 18px;
  }

  .modal-footer {
    padding: 12px 18px;
  }

  .two-col {
    grid-template-columns: 1fr;
    gap: 15px;
  }

  .two-col .switch-field {
    padding-top: 9px;
  }
}
</style>
