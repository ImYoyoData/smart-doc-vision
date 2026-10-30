<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  NButton,
  NCheckbox,
  NCheckboxGroup,
  NInput,
  NModal,
  NPopover,
  NSelect,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  AddOutline,
  CheckmarkCircleOutline,
  CreateOutline,
  KeyOutline,
  PersonOutline,
  RefreshOutline,
  ShieldCheckmarkOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import { auditApi, userApi } from '@/api'
import type { Audit, RoleDefinition, User } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'
import { isValidEmail } from '@/utils/validation'

const workspace = useWorkspaceStore()
const auth = useAuthStore()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const users = ref<User[]>([])
const audits = ref<Audit[]>([])
const roles = ref<RoleDefinition[]>([])
const allPermissions = ref<string[]>([])
const allMenus = ref<string[]>([])
const roleModalShow = ref(false)
const editingRole = ref<RoleDefinition | null>(null)
const roleSaving = ref(false)
const roleError = ref('')
const roleForm = reactive({
  code: '',
  name: '',
  description: '',
  permissions: [] as string[],
  menus: [] as string[],
})
const modalShow = ref(false)
const editing = ref<User | null>(null)
const errorText = ref('')
const form = reactive({
  username: '',
  display_name: '',
  email: '',
  password: '',
  role: 'VIEWER',
  status: 'ACTIVE',
  project_ids: [] as number[],
  audit_ids: [] as number[],
})

const roleOptions = computed(() => roles.value.map((r) => ({ label: `${r.name}（${r.code}）`, value: r.code })))
const projectOptions = computed(() => workspace.projects.map((p) => ({ label: p.name, value: p.id })))
const auditOptions = computed(() => {
  const selected = new Set(form.project_ids)
  return audits.value
    .filter((audit) => selected.has(audit.project_id))
    .map((audit) => ({ label: `${audit.name} · ${audit.code}`, value: audit.id }))
})

watch(
  () => [...form.project_ids],
  (ids) => {
    const allowed = new Set(audits.value.filter((audit) => ids.includes(audit.project_id)).map((audit) => audit.id))
    form.audit_ids = form.audit_ids.filter((id) => allowed.has(id))
  },
)

function userAudits(user: User) {
  return (user.audit_ids || []).map((id) => {
    const audit = audits.value.find((item) => item.id === id)
    return {
      id,
      name: audit?.name || `审核 #${id}`,
      code: audit?.code || '已删除/不可用',
    }
  })
}
const selectedRole = computed(() => roles.value.find((r) => r.code === form.role))
const activeUsers = computed(() => users.value.filter((u) => u.status === 'ACTIVE').length)

const ENV_LABELS: Record<string, string> = { dev: '开发', test: '测试', staging: '预发', prod: '生产' }
const VISIBLE_SCOPE_LIMIT = 2

function userScopeProjects(user: User) {
  return (user.project_ids || []).map((id) => {
    const project = workspace.projects.find((p) => p.id === id)
    return { id, name: project?.name || `项目 #${id}`, env: project?.env || '' }
  })
}

function visibleScopeProjects(user: User) {
  return userScopeProjects(user).slice(0, VISIBLE_SCOPE_LIMIT)
}

function hiddenScopeCount(user: User) {
  return Math.max(0, userScopeProjects(user).length - VISIBLE_SCOPE_LIMIT)
}

function envLabel(env: string) {
  return ENV_LABELS[env] || '未设置'
}

const menuLabels: Record<string, string> = {
  dashboard: '仪表盘', analytics: '数据报表', reviews: '待议复核', tasks: '审核任务',
  audits: '审核配置', providers: '模型接入', credentials: '接入凭证', connections: '数据连接', notifications: '通知渠道', projects: '项目空间',
  logs: '审计日志', recycle: '回收站', users: '用户与权限', monitor: '运行监控',
  'system-config': '系统配置', help: '对接文档', profile: '个人中心',
}

const permissionLabels: Record<string, string> = {
  'project:create': '创建项目', 'project:update': '修改项目', 'project:delete': '删除项目', 'project:restore': '恢复项目',
  'provider:write': '管理模型接入', 'audit:write': '管理审核配置', 'credential:write': '管理接入凭证',
  'notification:write': '管理通知渠道',
  'task:submit': '提交审核任务', 'task:review': '人工复核', 'task:retry': '重试任务', 'user:manage': '管理账号',
  'log:read': '查看审计日志', 'recycle:write': '回收站操作', 'system:monitor': '查看运行监控',
  'system:config': '查看系统配置',
}

async function loadAuditOptions() {
  const items: Audit[] = []
  for (let page = 1; page <= 20; page += 1) {
    const data = await auditApi.list({ page, page_size: 100 })
    items.push(...data.items)
    if (!data.has_more) break
  }
  return items
}

async function load() {
  loading.value = true
  try {
    await workspace.loadProjects(true)
    const [data, meta, auditItems] = await Promise.all([
      userApi.list(),
      userApi.roles(),
      loadAuditOptions(),
    ])
    users.value = data.items
    audits.value = auditItems
    roles.value = data.roles || meta.items || []
    allPermissions.value = meta.all_permissions || []
    allMenus.value = meta.all_menus || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载账号失败')
  } finally {
    loading.value = false
  }
}

function openRoleCreate() {
  editingRole.value = null
  roleForm.code = ''
  roleForm.name = ''
  roleForm.description = ''
  roleForm.permissions = ['log:read']
  roleForm.menus = ['dashboard', 'help', 'profile']
  roleError.value = ''
  roleModalShow.value = true
}

function openRoleEdit(role: RoleDefinition) {
  if (role.builtin) return
  editingRole.value = role
  roleForm.code = role.code
  roleForm.name = role.name
  roleForm.description = role.description
  roleForm.permissions = [...role.permissions]
  roleForm.menus = [...role.menus]
  roleError.value = ''
  roleModalShow.value = true
}

async function saveRole() {
  roleError.value = ''
  if (!editingRole.value && !/^[A-Z][A-Z0-9_]{2,31}$/.test(roleForm.code.trim())) {
    roleError.value = '角色编码需以大写字母开头，3~32 位，仅支持大写字母、数字和下划线'
    return
  }
  if (!roleForm.name.trim()) {
    roleError.value = '请填写角色名称'
    return
  }
  if (!roleForm.permissions.length || !roleForm.menus.length) {
    roleError.value = '至少选择一项操作权限和一个可见菜单'
    return
  }
  roleSaving.value = true
  try {
    const payload = {
      code: roleForm.code.trim(),
      name: roleForm.name.trim(),
      description: roleForm.description.trim(),
      permissions: roleForm.permissions,
      menus: roleForm.menus,
    }
    if (editingRole.value) {
      await userApi.updateRole(editingRole.value.code, payload)
      message.success('角色已更新')
    } else {
      await userApi.createRole(payload)
      message.success('自定义角色已创建')
    }
    roleModalShow.value = false
    await load()
  } catch (error) {
    roleError.value = error instanceof Error ? error.message : '保存角色失败'
  } finally {
    roleSaving.value = false
  }
}

function deleteRole(role: RoleDefinition) {
  if (role.builtin) return
  dialog.warning({
    title: '删除自定义角色',
    content: `删除「${role.name}」后不可恢复；仍有账号使用该角色时会被拒绝。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await userApi.deleteRole(role.code)
        message.success('角色已删除')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除角色失败')
      }
    },
  })
}

function openCreate() {
  editing.value = null
  form.username = ''
  form.display_name = ''
  form.email = ''
  form.password = ''
  form.role = 'VIEWER'
  form.status = 'ACTIVE'
  form.project_ids = workspace.projects.map((p) => p.id)
  form.audit_ids = []
  errorText.value = ''
  modalShow.value = true
}

function openEdit(user: User) {
  editing.value = user
  form.username = user.username
  form.display_name = user.display_name
  form.email = user.email || ''
  form.password = ''
  form.role = user.role
  form.status = user.status
  form.project_ids = [...(user.project_ids || [])]
  form.audit_ids = [...(user.audit_ids || [])]
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
  const email = form.email.trim()
  if (email && !isValidEmail(email)) {
    errorText.value = '邮箱地址格式不正确，请填写类似 name@example.com 的地址'
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      const payload: Record<string, unknown> = {
        display_name: form.display_name.trim(),
        email: form.email.trim(),
        role: form.role,
        status: form.status,
        project_ids: form.project_ids,
        audit_ids: form.role === 'SUPER_ADMIN' ? [] : form.audit_ids,
      }
      if (form.password) payload.password = form.password
      await userApi.update(editing.value.id, payload)
      message.success('账号已更新')
    } else {
      await userApi.create({
        username: form.username.trim(),
        display_name: form.display_name.trim(),
        email: form.email.trim(),
        password: form.password,
        role: form.role,
        status: form.status,
        project_ids: form.project_ids,
        audit_ids: form.role === 'SUPER_ADMIN' ? [] : form.audit_ids,
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

function resetMfa(user: User) {
  if (user.id === auth.user?.id) {
    message.warning('当前账号请在「个人中心」关闭或重新绑定两步验证')
    return
  }
  if (!user.mfa_enabled) {
    message.info('该账号未开启两步验证，无需重置；用户绑定验证器后才需要此操作')
    return
  }
  dialog.warning({
    title: '重置两步验证',
    content: `确认清除「${user.display_name || user.username}」的两步验证绑定？清除后该账号可直接用密码登录，现有登录会话会立即失效，用户登录后可重新绑定验证器。`,
    positiveText: '重置并解除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await userApi.resetMfa(user.id)
        if (editing.value?.id === user.id) editing.value.mfa_enabled = false
        message.success(`已清除「${user.display_name || user.username}」的两步验证，现有会话已失效`)
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '重置失败')
      }
    },
  })
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
        <n-button quaternary @click="openRoleCreate">
          <template #icon><CreateOutline /></template>
          新建角色
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
        <div><h3 class="panel-title">后台账号</h3><p class="panel-subtitle">密码使用 bcrypt 存储；超级管理员可重置其他账号密码和两步验证</p></div>
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
              <span v-if="user.mfa_enabled" class="mfa-chip" title="已开启两步验证">2FA</span>
              <span v-if="user.id === auth.user?.id" class="self-chip">当前账号</span>
            </div>
            <div class="user-meta">
              <span :class="{ 'meta-empty': !user.email }">邮箱：{{ user.email || '未填写' }}</span>
              <span class="scope-meta">
                <span class="scope-label">项目范围</span>
                <span v-if="user.role === 'SUPER_ADMIN'" class="scope-tag all">全部项目</span>
                <template v-else>
                  <span
                    v-for="project in visibleScopeProjects(user)"
                    :key="project.id"
                    class="scope-tag"
                    :class="[project.env, { missing: !project.env }]"
                    :title="project.env ? `${project.name} · ${envLabel(project.env)}环境` : project.name"
                  >
                    <i v-if="project.env"></i>{{ project.name }}
                  </span>
                  <n-popover v-if="hiddenScopeCount(user) > 0" trigger="click" placement="top-start" :width="340">
                    <template #trigger>
                      <button
                        class="scope-tag more"
                        type="button"
                        :title="`查看全部 ${userScopeProjects(user).length} 个项目`"
                        :aria-label="`查看全部 ${userScopeProjects(user).length} 个项目`"
                      >+{{ hiddenScopeCount(user) }}</button>
                    </template>
                    <div class="scope-popover">
                      <strong>全部项目范围（{{ userScopeProjects(user).length }}）</strong>
                      <div class="scope-popover-list">
                        <span
                          v-for="project in userScopeProjects(user)"
                          :key="project.id"
                          class="scope-tag"
                          :class="[project.env, { missing: !project.env }]"
                          :title="project.env ? `${project.name} · ${envLabel(project.env)}环境` : project.name"
                        >
                          <i v-if="project.env"></i>{{ project.name }}
                        </span>
                      </div>
                    </div>
                  </n-popover>
                  <span v-if="!userScopeProjects(user).length" class="scope-empty">无</span>
                </template>
              </span>
              <span class="scope-meta audit-scope-meta">
                <span class="scope-label">审核范围</span>
                <span v-if="user.role === 'SUPER_ADMIN' || !userAudits(user).length" class="audit-scope-tag all">全部</span>
                <template v-else>
                  <span class="audit-scope-tag" :title="userAudits(user)[0]?.name">{{ userAudits(user)[0]?.name }}</span>
                  <n-popover v-if="userAudits(user).length > 1" trigger="click" placement="top-start" :width="360">
                    <template #trigger>
                      <button
                        class="audit-scope-tag more"
                        type="button"
                        :title="`查看全部 ${userAudits(user).length} 个审核配置`"
                        :aria-label="`查看全部 ${userAudits(user).length} 个审核配置`"
                      >+{{ userAudits(user).length - 1 }}</button>
                    </template>
                    <div class="audit-scope-popover">
                      <strong>审核配置范围（{{ userAudits(user).length }}）</strong>
                      <div class="audit-scope-popover-list">
                        <span v-for="audit in userAudits(user)" :key="audit.id" class="audit-scope-item">
                          <span>{{ audit.name }}</span>
                          <small>{{ audit.code }}</small>
                        </span>
                      </div>
                    </div>
                  </n-popover>
                </template>
              </span>
              <span :class="{ 'mfa-on': user.mfa_enabled }">两步验证：{{ user.mfa_enabled ? '已开启' : '未开启' }}</span>
              <span>最近登录：{{ user.last_login_at ? user.last_login_at.replace('T', ' ').slice(0, 19) : '从未登录' }}</span>
            </div>
          </div>
          <div class="user-actions">
            <n-button size="small" quaternary @click="openEdit(user)">编辑</n-button>
            <n-button
              size="small"
              quaternary
              type="warning"
              :disabled="user.id === auth.user?.id || !user.mfa_enabled"
              :title="user.id === auth.user?.id
                ? '当前账号请在个人中心管理两步验证'
                : user.mfa_enabled
                  ? '清除该账号的两步验证绑定，帮助用户恢复登录'
                  : '该账号未开启两步验证，用户绑定后才需要重置'"
              @click="resetMfa(user)"
            >
              <template #icon><KeyOutline /></template>
              重置 2FA
            </n-button>
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
        <div><h3 class="panel-title">角色与菜单权限</h3><p class="panel-subtitle">内置角色随版本升级维护；自定义角色可自由组合权限与菜单</p></div>
        <n-button size="small" secondary @click="openRoleCreate">
          <template #icon><AddOutline /></template>
          自定义角色
        </n-button>
      </div>
      <div class="role-grid">
        <article v-for="role in roles" :key="role.code" class="role-card">
          <div class="role-head">
            <div class="role-head-main">
              <strong>{{ role.name }}</strong>
              <code>{{ role.code }}</code>
              <span v-if="role.builtin" class="builtin-chip">内置</span>
            </div>
            <div v-if="!role.builtin" class="role-actions">
              <button type="button" title="编辑角色" @click="openRoleEdit(role)"><CreateOutline /></button>
              <button type="button" title="删除角色" @click="deleteRole(role)"><TrashOutline /></button>
            </div>
          </div>
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
          <div class="modal-head-actions">
            <span class="modal-icon"><KeyOutline /></span>
            <button class="modal-close" type="button" title="关闭" aria-label="关闭" @click="modalShow = false">×</button>
          </div>
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
          <div class="field">
            <label>邮箱地址</label>
            <n-input v-model:value="form.email" placeholder="name@example.com" />
            <div class="form-hint">用于账号联系与通知，可留空。</div>
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
          <div v-if="form.role !== 'SUPER_ADMIN'" class="field">
            <label>审核配置范围</label>
            <n-select
              v-model:value="form.audit_ids"
              multiple
              filterable
              clearable
              :max-tag-count="3"
              :options="auditOptions"
              placeholder="不选表示全部审核配置"
            />
            <div class="form-hint">不选择时默认可查看账号项目范围内的全部审核配置；选择后只能查看、提交和复核这些审核配置。</div>
          </div>
          <div v-if="selectedRole" class="role-preview">
            <div class="role-preview-head"><ShieldCheckmarkOutline /><strong>{{ selectedRole.name }} 的菜单权限</strong></div>
            <div class="role-menus"><span v-for="menu in selectedRole.menus" :key="menu">{{ menuLabels[menu] || menu }}</span></div>
          </div>
          <div v-if="editing" class="mfa-admin-panel" :class="{ enabled: editing.mfa_enabled }">
            <span class="mfa-admin-icon"><KeyOutline /></span>
            <div class="mfa-admin-copy">
              <strong>两步验证（2FA）</strong>
              <p v-if="editing.id === auth.user?.id">当前账号请在「个人中心」开启、关闭或重新绑定两步验证。</p>
              <p v-else-if="editing.mfa_enabled">该账号已绑定验证器。用户丢失验证器无法登录时，可在此清除绑定帮助其恢复密码登录。</p>
              <p v-else>该账号尚未绑定验证器，用户可在「个人中心」自行开启。</p>
            </div>
            <n-button
              v-if="editing.mfa_enabled"
              size="small"
              type="warning"
              secondary
              :disabled="editing.id === auth.user?.id"
              @click="resetMfa(editing)"
            >
              重置 2FA
            </n-button>
            <n-button v-else size="small" secondary disabled title="该账号尚未绑定验证器，无需重置">
              重置 2FA
            </n-button>
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">{{ editing ? '保存账号' : '创建账号' }}</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal v-model:show="roleModalShow" :mask-closable="!roleSaving">
      <div class="user-modal role-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editingRole ? 'EDIT ROLE' : 'NEW ROLE' }}</div>
            <h2>{{ editingRole ? '编辑自定义角色' : '新建自定义角色' }}</h2>
            <p>菜单决定可见入口，操作权限决定可执行的动作；数据范围仍由账号授权的项目决定。</p>
          </div>
          <span class="modal-icon"><ShieldCheckmarkOutline /></span>
        </div>
        <div class="modal-body">
          <div class="two-col">
            <div class="field">
              <label>角色编码 <span v-if="!editingRole" class="required">*</span></label>
              <n-input v-model:value="roleForm.code" :disabled="Boolean(editingRole)" placeholder="ARCHIVE_OPERATOR" />
            </div>
            <div class="field">
              <label>角色名称 <span class="required">*</span></label>
              <n-input v-model:value="roleForm.name" placeholder="档案运营" />
            </div>
          </div>
          <div class="field">
            <label>角色说明</label>
            <n-input v-model:value="roleForm.description" placeholder="说明该角色的使用场景与边界" />
          </div>
          <div class="role-editor-grid">
            <div class="field">
              <label>可见菜单 <span class="optional">至少一项</span></label>
              <div class="check-panel">
                <n-checkbox-group v-model:value="roleForm.menus">
                  <n-checkbox
                    v-for="menu in allMenus"
                    :key="menu"
                    :value="menu"
                    :label="menuLabels[menu] || menu"
                  />
                </n-checkbox-group>
              </div>
            </div>
            <div class="field">
              <label>操作权限 <span class="optional">至少一项</span></label>
              <div class="check-panel">
                <n-checkbox-group v-model:value="roleForm.permissions">
                  <n-checkbox
                    v-for="permission in allPermissions"
                    :key="permission"
                    :value="permission"
                    :label="permissionLabels[permission] || permission"
                  />
                </n-checkbox-group>
              </div>
            </div>
          </div>
          <p class="role-tip">运行监控依赖「查看运行监控」权限；只勾选菜单而未勾选权限时，接口仍会拒绝访问。</p>
          <div v-if="roleError" class="modal-error">{{ roleError }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="roleSaving" @click="roleModalShow = false">取消</n-button>
          <n-button type="primary" :loading="roleSaving" @click="saveRole">{{ editingRole ? '保存角色' : '创建角色' }}</n-button>
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

.mfa-chip {
  padding: 2px 6px;
  border-radius: 4px;
  background: #e9f9ef;
  color: #15803d;
  font-family: var(--mono);
  font-size: 8px;
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
  align-items: center;
  gap: 12px;
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.user-meta > span {
  display: inline-flex;
  min-height: 20px;
  align-items: center;
}

.user-meta .mfa-on {
  color: #15803d;
}

.scope-meta {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
}

.scope-label {
  color: var(--muted-light);
}

.scope-tag {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--surface-strong);
  color: var(--muted);
  font-size: 9.5px;
  font-weight: 550;
  line-height: 1.5;
}

.scope-tag i {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.85;
}

.scope-tag.prod {
  background: var(--red-soft);
  color: var(--red);
}

.scope-tag.staging {
  background: var(--amber-soft);
  color: #b45309;
}

.scope-tag.test {
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.scope-tag.dev {
  background: var(--surface-strong);
  color: var(--muted);
}

.scope-tag.all {
  background: var(--green-soft);
  color: #15803d;
}

.scope-tag.missing {
  background: var(--surface-strong);
  color: var(--muted-light);
}

.scope-tag.more {
  border: 1px dashed var(--border);
  background: transparent;
  color: var(--brand-strong);
  font-family: inherit;
  cursor: pointer;
}

.scope-tag.more:hover {
  border-color: var(--brand-border);
  background: var(--brand-soft);
}

.scope-popover {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.scope-popover > strong {
  color: var(--ink);
  font-size: 11.5px;
  font-weight: 600;
}

.scope-popover-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-width: 300px;
}

.scope-empty {
  color: var(--muted-light);
}

.audit-scope-tag {
  display: inline-flex;
  max-width: 170px;
  align-items: center;
  overflow: hidden;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 9.5px;
  font-weight: 550;
  line-height: 1.5;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audit-scope-tag.all {
  background: var(--green-soft);
  color: var(--green);
}

.audit-scope-tag.more {
  border: 1px dashed var(--brand-border);
  background: transparent;
  font-family: inherit;
  cursor: pointer;
}

.audit-scope-tag.more:hover {
  background: var(--brand-soft);
}

.audit-scope-popover {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.audit-scope-popover > strong {
  color: var(--ink);
  font-size: 11.5px;
  font-weight: 600;
}

.audit-scope-popover-list {
  display: flex;
  max-height: 240px;
  flex-direction: column;
  gap: 5px;
  overflow-y: auto;
  padding-right: 2px;
}

.audit-scope-item {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  padding: 5px 8px;
  border-radius: 6px;
  background: var(--surface-soft);
}

.audit-scope-item > span {
  overflow: hidden;
  color: var(--text);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audit-scope-item > small {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.5px;
  flex: none;
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
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}

.role-head-main {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.builtin-chip {
  padding: 1px 5px;
  border-radius: 4px;
  background: #eef2f7;
  color: #7c8798;
  font-size: 8px;
}

.role-actions {
  display: flex;
  gap: 2px;
}

.role-actions button {
  display: flex;
  width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  padding: 0;
}

.role-actions button:hover {
  background: #f1f5f9;
  color: var(--brand);
}

.role-actions :deep(svg) {
  width: 12px;
  height: 12px;
}

.role-modal {
  width: min(720px, calc(100vw - 30px));
}

.role-editor-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.check-panel {
  max-height: 210px;
  overflow: auto;
  padding: 9px 10px;
  border: 1px solid #edf1f7;
  border-radius: 9px;
  background: #fbfcfe;
}

.check-panel :deep(.n-checkbox-group) {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.optional {
  margin-left: 5px;
  color: var(--muted-light);
  font-size: 9px;
  font-weight: 400;
}

.role-tip {
  margin: 0;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.7;
}

@media (max-width: 720px) {
  .role-editor-grid {
    grid-template-columns: 1fr;
  }
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

.mfa-admin-panel {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 9px;
  background: var(--surface-soft);
}

.mfa-admin-panel.enabled {
  border-color: rgba(21, 128, 61, 0.2);
  background: #f6fdf9;
}

.mfa-admin-icon {
  display: grid;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  place-items: center;
  border-radius: 8px;
  background: var(--surface-strong);
  color: var(--muted);
}

.mfa-admin-panel.enabled .mfa-admin-icon {
  background: #e9f9ef;
  color: #15803d;
}

.mfa-admin-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.mfa-admin-copy {
  flex: 1;
  min-width: 0;
}

.mfa-admin-copy strong {
  color: var(--ink);
  font-size: 10px;
  font-weight: 600;
}

.mfa-admin-copy p {
  margin: 2px 0 0;
  color: var(--muted);
  font-size: 9px;
  line-height: 1.5;
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
