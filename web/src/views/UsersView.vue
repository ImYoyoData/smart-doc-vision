<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  NButton,
  NInput,
  NModal,
  NSelect,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  AddOutline,
  CheckmarkCircleOutline,
  KeyOutline,
  PersonOutline,
  RefreshOutline,
  ShieldCheckmarkOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import { userApi } from '@/api'
import type { RoleDefinition, User } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'

const workspace = useWorkspaceStore()
const auth = useAuthStore()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const users = ref<User[]>([])
const roles = ref<RoleDefinition[]>([])
const modalShow = ref(false)
const editing = ref<User | null>(null)
const errorText = ref('')
const form = reactive({
  username: '',
  display_name: '',
  password: '',
  role: 'VIEWER',
  status: 'ACTIVE',
  project_ids: [] as number[],
})

const roleOptions = computed(() => roles.value.map((r) => ({ label: `${r.name}（${r.code}）`, value: r.code })))
const projectOptions = computed(() => workspace.projects.map((p) => ({ label: p.name, value: p.id })))
const selectedRole = computed(() => roles.value.find((r) => r.code === form.role))
const activeUsers = computed(() => users.value.filter((u) => u.status === 'ACTIVE').length)

const menuLabels: Record<string, string> = {
  dashboard: '仪表盘', analytics: '数据报表', reviews: '待议复核', tasks: '审核任务',
  audits: '审核配置', providers: '模型接入', credentials: '接入凭证', projects: '项目空间',
  logs: '审计日志', recycle: '回收站', users: '用户与权限', help: '对接文档', profile: '个人中心',
}

async function load() {
  loading.value = true
  try {
    await workspace.loadProjects(true)
    const data = await userApi.list()
    users.value = data.items
    roles.value = data.roles || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载账号失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.username = ''
  form.display_name = ''
  form.password = ''
  form.role = 'VIEWER'
  form.status = 'ACTIVE'
  form.project_ids = workspace.projects.map((p) => p.id)
  errorText.value = ''
  modalShow.value = true
}

function openEdit(user: User) {
  editing.value = user
  form.username = user.username
  form.display_name = user.display_name
  form.password = ''
  form.role = user.role
  form.status = user.status
  form.project_ids = [...(user.project_ids || [])]
  errorText.value = ''
  modalShow.value = true
}

async function save() {
  errorText.value = ''
  if (!editing.value && !form.username.trim()) {
    errorText.value = '请填写登录账号'
    return
  }
  if (!editing.value && !form.password) {
    errorText.value = '请设置初始密码'
    return
  }
  if (form.project_ids.length === 0 && form.role !== 'SUPER_ADMIN') {
    errorText.value = '非超级管理员至少需要授权一个项目'
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      const payload: Record<string, unknown> = {
        display_name: form.display_name.trim(),
        role: form.role,
        status: form.status,
        project_ids: form.project_ids,
      }
      if (form.password) payload.password = form.password
      await userApi.update(editing.value.id, payload)
      message.success('账号已更新')
    } else {
      await userApi.create({
        username: form.username.trim(),
        display_name: form.display_name.trim(),
        password: form.password,
        role: form.role,
        status: form.status,
        project_ids: form.project_ids,
      })
      message.success('账号已创建')
    }
    modalShow.value = false
    await load()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function toggleStatus(user: User) {
  if (user.id === auth.user?.id) {
    message.warning('不能停用当前登录账号')
    return
  }
  const next = user.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE'
  dialog.warning({
    title: next === 'DISABLED' ? '停用后台账号' : '启用后台账号',
    content: next === 'DISABLED'
      ? `停用「${user.username}」后该账号将立即无法登录，但历史审计记录保留。`
      : `确认启用「${user.username}」？`,
    positiveText: next === 'DISABLED' ? '停用' : '启用',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await userApi.update(user.id, { status: next, project_ids: user.project_ids || [], role: user.role })
        message.success(next === 'DISABLED' ? '账号已停用' : '账号已启用')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '状态更新失败')
      }
    },
  })
}

onMounted(load)
</script>

<template>
  <div class="page users-page">
    <PageHeader
      eyebrow="ACCESS CONTROL"
      title="用户与权限"
      subtitle="创建后台账号、分配角色、设定项目数据范围，并查看角色对应的菜单与功能权限。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新建后台账号
        </n-button>
      </template>
    </PageHeader>

    <div class="security-banner">
      <ShieldCheckmarkOutline />
      <div>
        <strong>账号权限 = 角色权限 + 项目数据范围</strong>
        <p>角色决定能操作什么和能看到哪些菜单；项目范围决定能查看哪些项目、审核配置和任务。超级管理员可访问全部项目。</p>
      </div>
      <span class="user-count">{{ users.length }} 个账号 · {{ activeUsers }} 个启用</span>
    </div>

    <section class="panel">
      <div class="panel-head">
        <div><h3 class="panel-title">后台账号</h3><p class="panel-subtitle">密码使用 bcrypt 存储；超级管理员可重置其他账号密码</p></div>
        <n-button text type="primary" @click="openCreate">新建账号</n-button>
      </div>
      <div v-if="users.length" class="user-list">
        <div v-for="user in users" :key="user.id" class="user-row">
          <span class="user-avatar"><PersonOutline /></span>
          <div class="user-main">
            <div class="user-title">
              <strong>{{ user.display_name || user.username }}</strong>
              <span class="mono">@{{ user.username }}</span>
              <StatusTag :status="user.status" type="provider" />
              <span class="role-chip">{{ roles.find((r) => r.code === user.role)?.name || user.role }}</span>
              <span v-if="user.id === auth.user?.id" class="self-chip">当前账号</span>
            </div>
            <div class="user-meta">
              <span>项目范围：{{ user.role === 'SUPER_ADMIN' ? '全部项目' : (user.project_ids || []).map((id) => workspace.projects.find((p) => p.id === id)?.name || `#${id}`).join('、') || '无' }}</span>
              <span>最近登录：{{ user.last_login_at ? user.last_login_at.replace('T', ' ').slice(0, 19) : '从未登录' }}</span>
            </div>
          </div>
          <div class="user-actions">
            <n-button size="small" quaternary @click="openEdit(user)">编辑</n-button>
            <n-button size="small" quaternary @click="toggleStatus(user)">{{ user.status === 'ACTIVE' ? '停用' : '启用' }}</n-button>
          </div>
        </div>
      </div>
      <EmptyState v-else-if="!loading" compact title="暂无账号" description="创建一个后台账号并分配角色。" action="新建账号" @action="openCreate">
        <template #icon><PersonOutline /></template>
      </EmptyState>
    </section>

    <section class="panel role-panel">
      <div class="panel-head">
        <div><h3 class="panel-title">角色与菜单权限</h3><p class="panel-subtitle">固定角色，可创建账号时选择；菜单按角色自动隐藏</p></div>
      </div>
      <div class="role-grid">
        <article v-for="role in roles" :key="role.code" class="role-card">
          <div class="role-head"><strong>{{ role.name }}</strong><code>{{ role.code }}</code></div>
          <p>{{ role.description }}</p>
          <div class="role-menus">
            <span v-for="menu in role.menus" :key="menu">{{ menuLabels[menu] || menu }}</span>
          </div>
          <div class="role-permissions">
            {{ role.permissions.length ? role.permissions.join(' · ') : '只读' }}
          </div>
        </article>
      </div>
    </section>

    <n-modal v-model:show="modalShow" :mask-closable="!saving">
      <div class="user-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT ACCOUNT' : 'NEW ACCOUNT' }}</div>
            <h2>{{ editing ? '编辑后台账号' : '新建后台账号' }}</h2>
            <p>账号的角色决定菜单与操作权限，项目范围决定可操作的数据。</p>
          </div>
          <span class="modal-icon"><KeyOutline /></span>
        </div>
        <div class="modal-body">
          <div class="two-col">
            <div class="field">
              <label>登录账号 <span v-if="!editing" class="required">*</span></label>
              <n-input v-model:value="form.username" :disabled="Boolean(editing)" placeholder="zhangsan" />
            </div>
            <div class="field">
              <label>显示名称</label>
              <n-input v-model:value="form.display_name" placeholder="张三" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>{{ editing ? '重置密码（留空不修改）' : '初始密码' }} <span v-if="!editing" class="required">*</span></label>
              <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="至少 10 位，含字母和数字" />
            </div>
            <div class="field">
              <label>角色</label>
              <n-select v-model:value="form.role" :options="roleOptions" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>账号状态</label>
              <n-select v-model:value="form.status" :options="[{ label: '启用', value: 'ACTIVE' }, { label: '停用', value: 'DISABLED' }]" />
            </div>
            <div class="field">
              <label>授权项目</label>
              <n-select
                v-model:value="form.project_ids"
                multiple
                filterable
                :options="projectOptions"
                :disabled="form.role === 'SUPER_ADMIN'"
                placeholder="选择项目范围"
              />
            </div>
          </div>
          <div v-if="selectedRole" class="role-preview">
            <div class="role-preview-head"><ShieldCheckmarkOutline /><strong>{{ selectedRole.name }} 的菜单权限</strong></div>
            <div class="role-menus"><span v-for="menu in selectedRole.menus" :key="menu">{{ menuLabels[menu] || menu }}</span></div>
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">{{ editing ? '保存账号' : '创建账号' }}</n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.security-banner {
  display: flex;
  align-items: flex-start;
  gap: 11px;
  margin-bottom: 15px;
  padding: 14px 16px;
  border: 1px solid #dce7fb;
  border-radius: 12px;
  background: linear-gradient(120deg, #f8fbff, #fff);
}

.security-banner :deep(svg) {
  width: 18px;
  height: 18px;
  margin-top: 2px;
  color: var(--brand);
  flex-shrink: 0;
}

.security-banner div {
  flex: 1;
}

.security-banner strong {
  display: block;
  color: #1e4b9e;
  font-size: 12px;
  font-weight: 600;
}

.security-banner p {
  margin: 3px 0 0;
  color: #64748b;
  font-size: 10.5px;
  line-height: 1.7;
}

.user-count {
  flex-shrink: 0;
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.user-list {
  padding: 6px 16px 13px;
}

.user-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 3px;
  border-bottom: 1px solid #f2f5f9;
}

.user-row:last-child {
  border-bottom: 0;
}

.user-avatar {
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

.user-avatar :deep(svg) {
  width: 16px;
  height: 16px;
}

.user-main {
  flex: 1;
  min-width: 0;
}

.user-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.user-title strong {
  color: var(--text);
  font-size: 11.5px;
  font-weight: 600;
}

.user-title .mono {
  color: var(--muted-light);
  font-size: 9px;
}

.role-chip {
  padding: 2px 6px;
  border-radius: 4px;
  background: #f1f5f9;
  color: #64748b;
  font-size: 8.5px;
}

.self-chip {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--green-soft);
  color: #15803d;
  font-size: 8.5px;
}

.user-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.user-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.role-panel {
  margin-top: 14px;
}

.role-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 9px;
  padding: 4px 16px 16px;
}

.role-card {
  padding: 12px;
  border: 1px solid #edf1f7;
  border-radius: 10px;
  background: #fbfcfe;
}

.role-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.role-head strong {
  color: var(--text);
  font-size: 10.5px;
  font-weight: 600;
}

.role-head code {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 7.5px;
}

.role-card p {
  margin: 4px 0 7px;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.6;
}

.role-menus {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
}

.role-menus span {
  padding: 1px 5px;
  border-radius: 4px;
  background: #f1f5f9;
  color: #64748b;
  font-size: 8px;
}

.role-permissions {
  margin-top: 7px;
  padding-top: 6px;
  border-top: 1px dashed #eaf0f7;
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 7.5px;
  line-height: 1.5;
  word-break: break-all;
}

.user-modal {
  width: min(620px, calc(100vw - 32px));
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
  flex-shrink: 0;
}

.modal-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.modal-body {
  padding: 18px 24px;
}

.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.field {
  margin-bottom: 14px;
}

.field label {
  display: block;
  margin-bottom: 5px;
  color: var(--text);
  font-size: 11px;
  font-weight: 550;
}

.required {
  color: var(--red);
}

.role-preview {
  padding: 10px 12px;
  border-radius: 8px;
  background: #f8fafc;
}

.role-preview-head {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 6px;
  color: #475569;
}

.role-preview-head :deep(svg) {
  width: 13px;
  height: 13px;
}

.role-preview-head strong {
  font-size: 9.5px;
  font-weight: 550;
}

.modal-error {
  margin-top: 9px;
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

@media (max-width: 1050px) {
  .role-grid {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 650px) {
  .role-grid,
  .two-col {
    grid-template-columns: 1fr;
  }
  .user-meta {
    flex-direction: column;
    gap: 2px;
  }
}
</style>
