<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
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
import type { MFASetup } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useWorkspaceStore } from '@/stores/workspace'

const auth = useAuthStore()
const workspace = useWorkspaceStore()
const router = useRouter()
const message = useMessage()
const saving = ref(false)
const form = reactive({ old: '', next: '', confirm: '' })
const errorText = ref('')
const mfaEnabled = ref(Boolean(auth.user?.mfa_enabled))
const mfaSetupData = ref<MFASetup | null>(null)
const mfaCode = ref('')
const mfaPassword = ref('')
const mfaConfirmCode = ref('')
const mfaError = ref('')
const mfaBusy = ref(false)

onMounted(async () => {
  try {
    const status = await authApi.mfaStatus()
    mfaEnabled.value = status.enabled
    if (auth.user) auth.user.mfa_enabled = status.enabled
  } catch {
    // status is non-critical for the rest of the profile page
  }
})

async function startMFASetup() {
  mfaError.value = ''
  mfaBusy.value = true
  try {
    mfaSetupData.value = await authApi.mfaSetup()
    mfaCode.value = ''
  } catch (error) {
    mfaError.value = error instanceof Error ? error.message : '生成动态验证码密钥失败'
  } finally {
    mfaBusy.value = false
  }
}

async function enableMFA() {
  mfaError.value = ''
  if (!/^\d{6}$/.test(mfaCode.value.trim())) {
    mfaError.value = '请输入 6 位动态验证码'
    return
  }
  mfaBusy.value = true
  try {
    await authApi.mfaEnable(mfaCode.value.trim())
    mfaEnabled.value = true
    mfaSetupData.value = null
    mfaCode.value = ''
    if (auth.user) auth.user.mfa_enabled = true
    message.success('动态验证码已启用，下次登录需要二次验证')
  } catch (error) {
    mfaError.value = error instanceof Error ? error.message : '启用失败'
  } finally {
    mfaBusy.value = false
  }
}

async function disableMFA() {
  mfaError.value = ''
  if (!mfaPassword.value || !/^\d{6}$/.test(mfaConfirmCode.value.trim())) {
    mfaError.value = '请输入登录密码和 6 位动态验证码'
    return
  }
  mfaBusy.value = true
  try {
    await authApi.mfaDisable(mfaPassword.value, mfaConfirmCode.value.trim())
    mfaEnabled.value = false
    mfaPassword.value = ''
    mfaConfirmCode.value = ''
    if (auth.user) auth.user.mfa_enabled = false
    message.success('动态验证码已关闭')
  } catch (error) {
    mfaError.value = error instanceof Error ? error.message : '关闭失败'
  } finally {
    mfaBusy.value = false
  }
}

async function copyMFASecret() {
  if (!mfaSetupData.value) return
  try {
    await navigator.clipboard.writeText(mfaSetupData.value.secret)
    message.success('密钥已复制')
  } catch {
    message.warning('浏览器禁止读取剪贴板，请手动选择复制')
  }
}

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
    message.success('密码已修改，请重新登录')
    auth.clear()
    router.replace('/login')
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

    <section class="panel mfa-panel">
      <div class="panel-head">
        <div>
          <h3 class="panel-title">两步验证（TOTP）</h3>
          <p class="panel-subtitle">启用后登录需要输入身份验证器生成的 6 位动态验证码；丢失验证器可联系超级管理员重置。</p>
        </div>
        <span class="mfa-status" :class="{ on: mfaEnabled }">{{ mfaEnabled ? '已启用' : '未启用' }}</span>
      </div>

      <div v-if="mfaEnabled" class="mfa-body">
        <p class="mfa-copy">关闭后该账号仅凭密码登录。为确认是本人操作，需要同时验证当前密码和动态验证码。</p>
        <form class="mfa-form" @submit.prevent="disableMFA">
          <n-input v-model:value="mfaPassword" type="password" show-password-on="click" placeholder="当前登录密码" />
          <n-input v-model:value="mfaConfirmCode" maxlength="6" placeholder="6 位动态验证码" :input-props="{ inputmode: 'numeric' }" />
          <n-button type="warning" secondary :loading="mfaBusy" attr-type="submit">关闭两步验证</n-button>
        </form>
      </div>

      <div v-else class="mfa-body">
        <template v-if="!mfaSetupData">
          <p class="mfa-copy">使用 Google Authenticator、Microsoft Authenticator、1Password 等应用扫描或导入密钥，为账号增加一层保护。</p>
          <n-button type="primary" secondary :loading="mfaBusy" @click="startMFASetup">生成绑定密钥</n-button>
        </template>
        <template v-else>
          <div class="mfa-secret-box">
            <span>手动密钥</span>
            <strong class="mono">{{ mfaSetupData.secret }}</strong>
            <n-button size="tiny" quaternary @click="copyMFASecret">复制密钥</n-button>
          </div>
          <div class="mfa-uri mono">{{ mfaSetupData.otpauth_url }}</div>
          <div class="mfa-form">
            <n-input v-model:value="mfaCode" maxlength="6" placeholder="输入验证器上的 6 位验证码" :input-props="{ inputmode: 'numeric', autocomplete: 'one-time-code' }" @keyup.enter="enableMFA" />
            <n-button type="primary" :loading="mfaBusy" @click="enableMFA">验证并启用</n-button>
            <n-button quaternary :disabled="mfaBusy" @click="mfaSetupData = null">取消</n-button>
          </div>
        </template>
      </div>
      <div v-if="mfaError" class="form-error mfa-error">{{ mfaError }}</div>
    </section>

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

.mfa-panel {
  margin-top: 14px;
}

.mfa-status {
  padding: 3px 9px;
  border-radius: 999px;
  background: #f1f5f9;
  color: #64748b;
  font-size: 9.5px;
}

.mfa-status.on {
  background: #e9f9ef;
  color: #15803d;
}

.mfa-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 4px 18px 18px;
}

.mfa-copy {
  margin: 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.7;
}

.mfa-form {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.mfa-form :deep(.n-input) {
  width: 210px;
}

.mfa-secret-box {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
  padding: 10px 12px;
  border: 1px solid #e4ecf8;
  border-radius: 9px;
  background: #f8fbff;
}

.mfa-secret-box span {
  color: var(--muted);
  font-size: 9.5px;
}

.mfa-secret-box strong {
  color: #1e4b9e;
  font-size: 12px;
  letter-spacing: 0.06em;
}

.mfa-uri {
  overflow-wrap: anywhere;
  color: var(--muted-light);
  font-size: 9px;
}

.mfa-error {
  margin: 0 18px 16px;
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
