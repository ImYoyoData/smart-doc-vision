<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NInput, useMessage } from 'naive-ui'
import {
  HelpCircleOutline,
  LockClosedOutline,
  LogOutOutline,
  PersonOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import { authApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useWorkspaceStore } from '@/stores/workspace'

const auth = useAuthStore()
const workspace = useWorkspaceStore()
const router = useRouter()
const message = useMessage()
const saving = ref(false)
const form = reactive({ old: '', next: '', confirm: '' })
const errorText = ref('')

const scopedProjects = computed(() => {
  if (!auth.user) return []
  if (auth.user.role === 'SUPER_ADMIN') return workspace.projects
  const ids = auth.user.project_ids || []
  return workspace.projects.filter((p) => ids.includes(p.id))
})

async function changePassword() {
  errorText.value = ''
  if (!form.old || !form.next) {
    errorText.value = '请输入当前密码和新密码'
    return
  }
  if (form.next.length < 10) {
    errorText.value = '新密码至少 10 位，且必须包含字母和数字'
    return
  }
  if (form.next !== form.confirm) {
    errorText.value = '两次输入的新密码不一致'
    return
  }
  saving.value = true
  try {
    await authApi.changePassword({ old_password: form.old, new_password: form.next })
    form.old = ''
    form.next = ''
    form.confirm = ''
    message.success('密码已修改')
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '修改失败'
  } finally {
    saving.value = false
  }
}

async function logout() {
  await auth.logout()
  router.replace('/login')
}
</script>

<template>
  <div class="page profile-page">
    <PageHeader eyebrow="MY ACCOUNT" title="个人中心" subtitle="查看账号身份、项目范围并修改登录密码。">
      <template #actions>
        <n-button secondary @click="router.push({ name: 'help' })">
          <template #icon><HelpCircleOutline /></template>
          对接文档
        </n-button>
        <n-button @click="logout">
          <template #icon><LogOutOutline /></template>
          退出登录
        </n-button>
      </template>
    </PageHeader>

    <div class="profile-grid">
      <section class="panel identity-panel">
        <div class="profile-avatar"><PersonOutline /></div>
        <h2>{{ auth.displayName }}</h2>
        <span class="profile-username mono">@{{ auth.user?.username }}</span>
        <span class="profile-role"><ShieldCheckmarkOutline />{{ auth.roleLabel }}</span>
        <div class="identity-meta">
          <div><span>账号状态</span><strong>{{ auth.user?.status === 'ACTIVE' ? '正常' : auth.user?.status }}</strong></div>
          <div><span>最近登录</span><strong>{{ auth.user?.last_login_at ? auth.user.last_login_at.replace('T', ' ').slice(0, 19) : '—' }}</strong></div>
          <div><span>项目范围</span><strong>{{ auth.user?.role === 'SUPER_ADMIN' ? '全部项目' : `${scopedProjects.length} 个项目` }}</strong></div>
        </div>
        <div class="project-scope">
          <span v-for="project in scopedProjects" :key="project.id">{{ project.name }}</span>
          <span v-if="!scopedProjects.length" class="muted">暂无项目授权</span>
        </div>
      </section>

      <section class="panel security-panel">
        <div class="panel-head">
          <div><h3 class="panel-title">修改登录密码</h3><p class="panel-subtitle">密码至少 10 位，包含字母和数字；修改后当前会话保持有效</p></div>
          <span class="lock-icon"><LockClosedOutline /></span>
        </div>
        <form class="password-form" @submit.prevent="changePassword">
          <label><span>当前密码</span><n-input v-model:value="form.old" type="password" show-password-on="click" placeholder="输入当前登录密码" /></label>
          <label><span>新密码</span><n-input v-model:value="form.next" type="password" show-password-on="click" placeholder="至少 10 位，含字母和数字" /></label>
          <label><span>确认新密码</span><n-input v-model:value="form.confirm" type="password" show-password-on="click" placeholder="再次输入新密码" /></label>
          <div v-if="errorText" class="form-error">{{ errorText }}</div>
          <div class="password-actions"><n-button type="primary" :loading="saving" attr-type="submit">保存新密码</n-button></div>
        </form>
      </section>
    </div>

    <section class="panel account-note">
      <ShieldCheckmarkOutline />
      <div>
        <strong>账号和权限由超级管理员管理</strong>
        <p>超级管理员可在「用户与权限」中创建账号、选择角色与项目范围，并查看每个角色对应的菜单权限。当前角色的菜单由服务端下发并在导航中自动过滤。</p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.profile-grid {
  display: grid;
  grid-template-columns: 310px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}

.identity-panel {
  display: flex;
  align-items: center;
  flex-direction: column;
  padding: 28px 20px 20px;
  text-align: center;
}

.profile-avatar {
  display: flex;
  width: 72px;
  height: 72px;
  align-items: center;
  justify-content: center;
  border-radius: 20px;
  background: linear-gradient(145deg, #dbe8ff, #b9cdf9);
  color: #2354b8;
}

.profile-avatar :deep(svg) {
  width: 32px;
  height: 32px;
}

.identity-panel h2 {
  margin: 14px 0 2px;
  color: var(--ink);
  font-size: 19px;
  font-weight: 650;
}

.profile-username {
  color: var(--muted-light);
  font-size: 10px;
}

.profile-role {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-top: 9px;
  padding: 3px 9px;
  border-radius: 999px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-size: 9.5px;
}

.profile-role :deep(svg) {
  width: 12px;
  height: 12px;
}

.identity-meta {
  display: grid;
  width: 100%;
  grid-template-columns: 1fr;
  gap: 0;
  margin-top: 21px;
  border-top: 1px solid #f0f3f8;
}

.identity-meta > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 9px 2px;
  border-bottom: 1px dashed #f2f5f9;
}

.identity-meta span {
  color: var(--muted-light);
  font-size: 9.5px;
}

.identity-meta strong {
  color: var(--text);
  font-size: 9.5px;
  font-weight: 500;
}

.project-scope {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 4px;
  margin-top: 16px;
}

.project-scope span {
  padding: 2px 7px;
  border-radius: 5px;
  background: #f8fafc;
  color: #64748b;
  font-size: 8.5px;
}

.security-panel {
  padding-bottom: 4px;
}

.lock-icon {
  display: flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--amber-soft);
  color: #d97706;
}

.lock-icon :deep(svg) {
  width: 15px;
  height: 15px;
}

.password-form {
  max-width: 440px;
  padding: 12px 22px 20px;
}

.password-form label {
  display: block;
  margin-bottom: 13px;
}

.password-form label > span {
  display: block;
  margin-bottom: 5px;
  color: var(--text);
  font-size: 11px;
  font-weight: 550;
}

.form-error {
  margin-bottom: 12px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--red-soft);
  color: #b91c1c;
  font-size: 10px;
}

.password-actions {
  padding-top: 2px;
}

.account-note {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-top: 14px;
  padding: 15px 17px;
}

.account-note :deep(svg) {
  width: 17px;
  height: 17px;
  margin-top: 2px;
  color: var(--brand);
  flex-shrink: 0;
}

.account-note strong {
  display: block;
  color: var(--text);
  font-size: 10.5px;
  font-weight: 600;
}

.account-note p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.7;
}

@media (max-width: 800px) {
  .profile-grid {
    grid-template-columns: 1fr;
  }
}
</style>
