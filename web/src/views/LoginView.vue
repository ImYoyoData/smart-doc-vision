<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput, useMessage } from 'naive-ui'
import {
  ArrowForwardOutline,
  CheckmarkCircleOutline,
  DocumentTextOutline,
  EyeOutline,
  LockClosedOutline,
  ScanOutline,
  ShieldCheckmarkOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import BrandMark from '@/components/BrandMark.vue'
import { useAuthStore } from '@/stores/auth'
import { useWorkspaceStore } from '@/stores/workspace'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const auth = useAuthStore()
const workspace = useWorkspaceStore()

const username = ref(localStorage.getItem('sdv.last_user') || '')
const password = ref('')
const loading = ref(false)
const errorText = ref('')

onMounted(() => {
  if (auth.isAuthenticated) router.replace('/dashboard')
})

async function submit() {
  if (!username.value.trim() || !password.value) {
    errorText.value = '请输入账号和密码'
    return
  }
  loading.value = true
  errorText.value = ''
  try {
    await auth.login(username.value.trim(), password.value)
    localStorage.setItem('sdv.last_user', username.value.trim())
    await workspace.loadProjects(true).catch(() => undefined)
    message.success('登录成功，欢迎回来')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.replace(redirect)
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '登录失败'
  } finally {
    loading.value = false
  }
}

function forgot() {
  message.info('请联系系统管理员重置密码。管理员可在服务端通过 ADMIN_PASSWORD 重新初始化。')
}
</script>

<template>
  <div class="login-page">
    <section class="login-visual">
      <div class="grid-overlay"></div>
      <div class="orb orb-a"></div>
      <div class="orb orb-b"></div>

      <div class="visual-top">
        <BrandMark :size="46" />
        <div>
          <div class="visual-brand">智审图档</div>
          <div class="visual-en">SMARTDOC VISION</div>
        </div>
      </div>

      <div class="visual-main">
        <div class="visual-badge">
          <SparklesOutline />
          <span>多模型驱动的档案智能审核平台</span>
        </div>
        <h1>让每一份图档，<br /><span>都经过可信的智能审核。</span></h1>
        <p class="visual-desc">
          连接通用大模型与 JEV 决策模型，自动完成档案内容安全、完整性与敏感信息审核，灰区结论无缝流转人工复核。
        </p>

        <div class="feature-row">
          <div class="feature">
            <div class="feature-icon"><ScanOutline /></div>
            <div>
              <strong>结构化裁决</strong>
              <span>Choice / Noul 概率输出</span>
            </div>
          </div>
          <div class="feature">
            <div class="feature-icon"><ShieldCheckmarkOutline /></div>
            <div>
              <strong>规则与人工兜底</strong>
              <span>阈值策略 · 复核闭环</span>
            </div>
          </div>
        </div>

        <div class="preview-card">
          <div class="preview-head">
            <div class="preview-doc">
              <DocumentTextOutline />
            </div>
            <div class="preview-title">
              <strong>建设工程档案 · 扫描件</strong>
              <span>SDV202610210001 · 正在审核</span>
            </div>
            <div class="preview-live"><i></i> LIVE</div>
          </div>
          <div class="preview-body">
            <div class="skeleton-lines">
              <span style="width: 88%"></span>
              <span style="width: 72%"></span>
              <span style="width: 94%"></span>
            </div>
            <div class="preview-verdict">
              <div class="ring">
                <span>96</span>
                <small>置信度</small>
              </div>
              <div class="verdict-copy">
                <strong><CheckmarkCircleOutline /> 审核建议：进入人工复核</strong>
                <span>检测到 2 处关键要素无法辨认，建议复核后再归档。</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="visual-foot">
        <span>SMARTDOC VISION</span>
        <span>Go · Gin · MySQL · Redis</span>
      </div>
    </section>

    <section class="login-form-side">
      <div class="form-wrap">
        <div class="mobile-brand">
          <BrandMark :size="42" />
          <div>
            <div class="visual-brand">智审图档</div>
            <div class="visual-en">SMARTDOC VISION</div>
          </div>
        </div>
        <div class="form-heading">
          <div class="login-kicker">管理控制台</div>
          <h2>欢迎回来</h2>
          <p>登录后开始配置审核流程与查看任务态势。</p>
        </div>

        <form class="login-form" @submit.prevent="submit">
          <label class="field">
            <span class="field-label">账号</span>
            <n-input
              v-model:value="username"
              size="large"
              placeholder="请输入管理员账号"
              :input-props="{ autocomplete: 'username' }"
              @keyup.enter="submit"
            />
          </label>
          <label class="field">
            <span class="field-label">密码</span>
            <n-input
              v-model:value="password"
              type="password"
              show-password-on="click"
              size="large"
              placeholder="请输入登录密码"
              :input-props="{ autocomplete: 'current-password' }"
              @keyup.enter="submit"
            >
              <template #prefix>
                <LockClosedOutline class="input-icon" />
              </template>
            </n-input>
          </label>

          <div v-if="errorText" class="login-error">{{ errorText }}</div>

          <n-button type="primary" size="large" block :loading="loading" attr-type="submit" class="login-button">
            <template #icon><EyeOutline /></template>
            登录控制台
          </n-button>

          <div class="form-meta">
            <span>首次登录账号由 <code>ADMIN_USERNAME</code> / <code>ADMIN_PASSWORD</code> 初始化</span>
            <button type="button" class="forgot" @click="forgot">忘记密码？</button>
          </div>
        </form>

        <div class="secure-note">
          <ShieldCheckmarkOutline />
          <span>密钥加密存储 · 登录失败锁定 · 全量操作审计</span>
        </div>
      </div>
      <div class="form-footer">智审图档 SmartDoc Vision · 让审核可追溯、可复用、可进化</div>
    </section>
  </div>
</template>

<style scoped>
.login-page {
  display: grid;
  min-height: 100vh;
  grid-template-columns: minmax(0, 1.15fr) minmax(440px, 0.85fr);
  background: #fff;
}

.login-visual {
  position: relative;
  display: flex;
  min-height: 100vh;
  flex-direction: column;
  overflow: hidden;
  padding: 36px clamp(36px, 5vw, 72px) 28px;
  color: #fff;
  background:
    radial-gradient(1100px 600px at 10% 5%, rgba(76, 130, 255, 0.35), transparent 60%),
    radial-gradient(900px 500px at 95% 95%, rgba(36, 96, 240, 0.28), transparent 60%),
    linear-gradient(150deg, #081a36 0%, #0b2450 48%, #0c1f42 100%);
}

.grid-overlay {
  position: absolute;
  inset: 0;
  opacity: 0.05;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.7) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.7) 1px, transparent 1px);
  background-size: 58px 58px;
  mask-image: radial-gradient(ellipse at 45% 40%, black, transparent 76%);
}

.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(2px);
  pointer-events: none;
}

.orb-a {
  top: 12%;
  right: 10%;
  width: 280px;
  height: 280px;
  background: radial-gradient(circle, rgba(87, 147, 255, 0.16), transparent 68%);
}

.orb-b {
  bottom: -90px;
  left: -50px;
  width: 340px;
  height: 340px;
  background: radial-gradient(circle, rgba(43, 108, 246, 0.2), transparent 70%);
}

.visual-top,
.visual-main,
.visual-foot {
  position: relative;
  z-index: 1;
}

.visual-top {
  display: flex;
  align-items: center;
  gap: 13px;
}

.visual-brand {
  color: #fff;
  font-size: 18px;
  font-weight: 650;
  letter-spacing: 0.03em;
  line-height: 1.3;
}

.visual-en {
  color: rgba(255, 255, 255, 0.42);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.2em;
}

.visual-main {
  flex: 1;
  max-width: 660px;
  padding-top: clamp(44px, 8vh, 96px);
}

.visual-badge {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 5px 11px;
  border: 1px solid rgba(255, 255, 255, 0.13);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  color: #bdd2ff;
  font-size: 11.5px;
  backdrop-filter: blur(10px);
}

.visual-badge :deep(svg) {
  width: 13px;
  height: 13px;
}

h1 {
  margin: 24px 0 18px;
  color: #fff;
  font-size: clamp(32px, 3.4vw, 50px);
  font-weight: 620;
  letter-spacing: -0.035em;
  line-height: 1.28;
}

h1 span {
  color: #8fb7ff;
}

.visual-desc {
  max-width: 530px;
  margin: 0;
  color: rgba(226, 236, 255, 0.62);
  font-size: 14px;
  line-height: 1.85;
}

.feature-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin: 30px 0 34px;
}

.feature {
  display: flex;
  min-width: 190px;
  align-items: center;
  gap: 10px;
}

.feature-icon {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.06);
  color: #a9c6ff;
}

.feature-icon :deep(svg) {
  width: 16px;
  height: 16px;
}

.feature strong {
  display: block;
  color: rgba(255, 255, 255, 0.9);
  font-size: 12.5px;
  font-weight: 550;
}

.feature span {
  color: rgba(210, 225, 255, 0.45);
  font-size: 10.5px;
}

.preview-card {
  max-width: 500px;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 16px;
  background: linear-gradient(145deg, rgba(255, 255, 255, 0.095), rgba(255, 255, 255, 0.045));
  box-shadow: 0 30px 80px -32px rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(16px);
}

.preview-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.preview-doc {
  display: flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: rgba(126, 170, 255, 0.16);
  color: #bcd4ff;
}

.preview-doc :deep(svg) {
  width: 15px;
  height: 15px;
}

.preview-title {
  flex: 1;
}

.preview-title strong {
  display: block;
  color: rgba(255, 255, 255, 0.9);
  font-size: 11.5px;
  font-weight: 550;
}

.preview-title span {
  color: rgba(214, 228, 255, 0.35);
  font-family: var(--mono);
  font-size: 9.5px;
}

.preview-live {
  display: flex;
  align-items: center;
  gap: 5px;
  color: #7fe0a8;
  font-family: var(--mono);
  font-size: 9px;
  letter-spacing: 0.08em;
}

.preview-live i {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: #7fe0a8;
  box-shadow: 0 0 0 3px rgba(127, 224, 168, 0.12);
}

.preview-body {
  display: flex;
  gap: 20px;
  padding: 16px;
}

.skeleton-lines {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 8px;
  padding-top: 3px;
}

.skeleton-lines span {
  height: 6px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.09);
}

.preview-verdict {
  display: flex;
  width: 210px;
  align-items: center;
  gap: 11px;
  padding: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 11px;
  background: rgba(8, 20, 40, 0.35);
}

.ring {
  display: flex;
  width: 43px;
  height: 43px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  border-radius: 50%;
  background: conic-gradient(#f7bd54 0 96%, rgba(255, 255, 255, 0.1) 96% 100%);
  color: #ffd889;
  flex-shrink: 0;
  position: relative;
}

.ring::before {
  content: '';
  position: absolute;
  inset: 4px;
  border-radius: 50%;
  background: #0b1e3b;
}

.ring span,
.ring small {
  position: relative;
  z-index: 1;
}

.ring span {
  font-family: var(--mono);
  font-size: 11px;
  line-height: 1.1;
}

.ring small {
  color: rgba(255, 255, 255, 0.38);
  font-size: 6px;
}

.verdict-copy {
  min-width: 0;
}

.verdict-copy strong {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #ffd889;
  font-size: 10px;
  font-weight: 550;
}

.verdict-copy strong :deep(svg) {
  width: 11px;
  height: 11px;
}

.verdict-copy span {
  display: block;
  margin-top: 3px;
  color: rgba(226, 236, 255, 0.42);
  font-size: 8.5px;
  line-height: 1.5;
}

.visual-foot {
  display: flex;
  justify-content: space-between;
  padding-top: 22px;
  color: rgba(255, 255, 255, 0.24);
  font-family: var(--mono);
  font-size: 9px;
  letter-spacing: 0.08em;
}

.login-form-side {
  display: flex;
  min-height: 100vh;
  flex-direction: column;
  justify-content: center;
  padding: 40px clamp(32px, 6vw, 88px);
  background:
    radial-gradient(600px 400px at 80% 10%, rgba(43, 108, 246, 0.04), transparent 70%),
    #fff;
}

.form-wrap {
  width: 100%;
  max-width: 390px;
  margin: auto;
}

.mobile-brand {
  display: none;
  align-items: center;
  gap: 12px;
  margin-bottom: 38px;
}

.mobile-brand .visual-brand {
  color: var(--ink);
}

.mobile-brand .visual-en {
  color: var(--muted-light);
}

.login-kicker {
  color: var(--brand);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.form-heading h2 {
  margin: 8px 0 6px;
  color: var(--ink);
  font-size: 30px;
  font-weight: 680;
  letter-spacing: -0.035em;
}

.form-heading p {
  margin: 0;
  color: var(--muted);
  font-size: 13px;
}

.login-form {
  margin-top: 34px;
}

.field {
  display: block;
  margin-bottom: 18px;
}

.field-label {
  display: block;
  margin-bottom: 7px;
  color: var(--text);
  font-size: 12.5px;
  font-weight: 550;
}

.input-icon {
  width: 15px;
  height: 15px;
  color: var(--muted-light);
}

.login-error {
  margin: -4px 0 14px;
  padding: 9px 12px;
  border: 1px solid #fee2e2;
  border-radius: 9px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 12px;
}

.login-button {
  margin-top: 8px;
  height: 46px !important;
  font-size: 13.5px;
  letter-spacing: 0.01em;
}

.form-meta {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-top: 16px;
  color: var(--muted-light);
  font-size: 10.5px;
  line-height: 1.6;
}

.form-meta code {
  padding: 1px 4px;
  border-radius: 4px;
  background: #f1f5f9;
  font-family: var(--mono);
  font-size: 9.5px;
}

.forgot {
  flex-shrink: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--brand);
  font-size: 10.5px;
  cursor: pointer;
}

.secure-note {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-top: 44px;
  padding-top: 18px;
  border-top: 1px solid #eef1f6;
  color: var(--muted-light);
  font-size: 10.5px;
}

.secure-note :deep(svg) {
  width: 14px;
  height: 14px;
  color: #94a3b8;
}

.form-footer {
  margin-top: auto;
  padding-top: 34px;
  color: #b0b9c6;
  font-size: 10px;
  text-align: center;
}

@media (max-width: 980px) {
  .login-page {
    display: block;
  }

  .login-visual {
    display: none;
  }

  .login-form-side {
    min-height: 100vh;
  }

  .mobile-brand {
    display: flex;
  }

  .form-wrap {
    max-width: 420px;
  }
}
</style>
