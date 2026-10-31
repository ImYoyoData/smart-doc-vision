<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCheckbox,
  NInput,
  NInputNumber,
  NRadio,
  NRadioGroup,
  NSelect,
  NSpin,
  NStep,
  NSteps,
  NTag,
} from 'naive-ui'
import {
  AlertCircleOutline,
  ArrowBackOutline,
  ArrowForwardOutline,
  CheckmarkCircleOutline,
  CloudUploadOutline,
  DocumentTextOutline,
  LinkOutline,
  RefreshOutline,
  ServerOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import { installApi, type InstallDefaults, type InstallProbeResult, type InstallRequest } from '@/api/install'
import { useInstallStore } from '@/stores/install'

const store = useInstallStore()
const status = computed(() => store.status)
const loading = ref(true)
const loadError = ref('')
const activeStep = ref(0)
const agreementRead = ref(false)
const agreed = ref(false)
const agreementBox = ref<HTMLElement | null>(null)
const checking = ref(false)
const installing = ref(false)
const probes = ref<InstallProbeResult[]>([])
const installError = ref('')
const installed = ref(false)
const installedInfo = ref<{ installed_at: string; version: string; env_file: string } | null>(null)
const stepError = ref('')

const stepTitles = ['介绍与协议', 'MySQL', 'Redis', '文件存储', '管理员', '检测并安装']

const form = reactive({
  mysql: {
    host: '127.0.0.1',
    port: 3306 as number | null,
    user: 'root',
    password: '',
    database: 'smart-doc-vision',
    charset: 'utf8mb4',
  },
  redis: {
    host: '127.0.0.1',
    port: 6379 as number | null,
    password: '',
    db: 3 as number | null,
  },
  storage: {
    driver: 'local',
    local_dir: '',
    public_base_url: '',
    s3: {
      provider: 'aliyun',
      endpoint: '',
      region: 'cn-hangzhou',
      bucket: '',
      access_key: '',
      secret_key: '',
      prefix: '',
      force_path_style: false,
    },
  },
  admin: {
    username: 'admin',
    password: '',
    confirm: '',
  },
})

const providerOptions = [
  { label: '阿里云 OSS（S3 兼容）', value: 'aliyun', region: 'cn-hangzhou', hint: 'https://s3.oss-cn-hangzhou.aliyuncs.com' },
  { label: '腾讯云 COS（S3 兼容）', value: 'tencent', region: 'ap-guangzhou', hint: 'https://cos.ap-guangzhou.myqcloud.com' },
  { label: '七牛云 Kodo（S3 兼容）', value: 'qiniu', region: 'cn-east-1', hint: 'https://s3.cn-east-1.qiniucs.com' },
  { label: 'AWS S3', value: 'aws', region: 'us-east-1', hint: 'https://s3.us-east-1.amazonaws.com' },
  { label: 'MinIO / 自定义 S3', value: 'minio', region: '', hint: 'https://minio.example.com' },
]

const selectedProvider = computed(() => providerOptions.find((item) => item.value === form.storage.s3.provider))
const endpointPlaceholder = computed(() =>
  selectedProvider.value ? `留空使用默认：${selectedProvider.value.hint}` : 'https://s3.example.com',
)
const configMissing = computed(() => status.value?.state === 'config_missing')

const canProceed = computed(() => {
  switch (activeStep.value) {
    case 0:
      return agreed.value
    case 1:
      return Boolean(
        form.mysql.host.trim() &&
          form.mysql.user.trim() &&
          form.mysql.database.trim() &&
          form.mysql.port &&
          form.mysql.port > 0 &&
          form.mysql.port <= 65535,
      )
    case 2:
      return Boolean(form.redis.host.trim() && form.redis.port && form.redis.port > 0 && form.redis.port <= 65535)
    case 3:
      if (form.storage.driver === 'local') return Boolean(form.storage.local_dir.trim())
      if (!form.storage.s3.bucket.trim() || !form.storage.s3.access_key.trim() || !form.storage.s3.secret_key.trim()) {
        return false
      }
      return !['minio', 'custom'].includes(form.storage.s3.provider) || Boolean(form.storage.s3.endpoint.trim())
    case 4:
      return (
        /^[A-Za-z0-9_.-]{3,32}$/.test(form.admin.username.trim()) &&
        form.admin.password.length >= 8 &&
        form.admin.password.length <= 72 &&
        form.admin.password === form.admin.confirm
      )
    default:
      return agreed.value
  }
})

function applyDefaults(defaults: InstallDefaults) {
  Object.assign(form.mysql, defaults.mysql)
  form.mysql.password = ''
  Object.assign(form.redis, defaults.redis)
  form.redis.password = ''
  form.storage.driver = defaults.storage.driver === 's3' ? 's3' : 'local'
  form.storage.local_dir = defaults.storage.local_dir || status.value?.system.local_storage_dir || ''
  form.storage.public_base_url = defaults.storage.public_base_url || ''
  Object.assign(form.storage.s3, defaults.storage.s3)
  if (!form.storage.s3.region && selectedProvider.value) {
    form.storage.s3.region = selectedProvider.value.region
  }
  form.admin.username = defaults.admin.username || 'admin'
  form.admin.password = ''
  form.admin.confirm = ''
}

async function initialize() {
  loading.value = true
  loadError.value = ''
  try {
    await store.load(true)
    if (!status.value) {
      loadError.value = '无法连接安装服务，请确认后端已启动'
      return
    }
    if (status.value.state === 'needs_install') {
      applyDefaults(status.value.defaults)
    }
  } finally {
    loading.value = false
  }
  if (status.value?.state === 'needs_install') {
    await nextTick()
    checkAgreementOverflow()
  }
}

function checkAgreementOverflow() {
  const box = agreementBox.value
  if (!box) return
  if (box.scrollHeight <= box.clientHeight + 8) {
    agreementRead.value = true
  }
}

function onAgreementScroll() {
  const box = agreementBox.value
  if (!box) return
  if (box.scrollTop + box.clientHeight >= box.scrollHeight - 12) {
    agreementRead.value = true
  }
}

function onProviderChange(value: string | number | null) {
  const provider = providerOptions.find((item) => item.value === String(value))
  if (provider) {
    form.storage.s3.provider = provider.value
    form.storage.s3.region = provider.region
    form.storage.s3.endpoint = ''
  }
}

function nextStep() {
  stepError.value = ''
  if (!canProceed.value) {
    stepError.value = '请完善当前步骤的必填项后再继续'
    return
  }
  if (activeStep.value < stepTitles.length - 1) {
    activeStep.value += 1
    if (activeStep.value === 0) {
      void nextTick(checkAgreementOverflow)
    }
  }
}

function prevStep() {
  stepError.value = ''
  if (activeStep.value > 0) activeStep.value -= 1
}

function buildPayload(): InstallRequest {
  return {
    agreement: agreed.value,
    mysql: {
      host: form.mysql.host.trim(),
      port: Number(form.mysql.port || 3306),
      user: form.mysql.user.trim(),
      password: form.mysql.password,
      database: form.mysql.database.trim(),
      charset: form.mysql.charset.trim() || 'utf8mb4',
    },
    redis: {
      host: form.redis.host.trim(),
      port: Number(form.redis.port || 6379),
      password: form.redis.password,
      db: Number(form.redis.db ?? 3),
    },
    storage: {
      driver: form.storage.driver,
      local_dir: form.storage.local_dir.trim(),
      public_base_url: form.storage.public_base_url.trim(),
      s3: {
        provider: form.storage.s3.provider,
        endpoint: form.storage.s3.endpoint.trim(),
        region: form.storage.s3.region.trim(),
        bucket: form.storage.s3.bucket.trim(),
        access_key: form.storage.s3.access_key.trim(),
        secret_key: form.storage.s3.secret_key.trim(),
        prefix: form.storage.s3.prefix.trim(),
        force_path_style: form.storage.s3.force_path_style,
      },
    },
    admin: {
      username: form.admin.username.trim(),
      password: form.admin.password,
    },
  }
}

async function runInstall() {
  stepError.value = ''
  installError.value = ''
  probes.value = []
  if (!agreed.value) {
    stepError.value = '请先阅读并同意用户使用协议'
    return
  }
  checking.value = true
  try {
    const result = await installApi.test(buildPayload())
    probes.value = result.results
    if (!result.ok) {
      installError.value = '环境检测未通过，请根据下方提示修正后重试'
      return
    }
    installing.value = true
    const installedResult = await installApi.install(buildPayload())
    installedInfo.value = installedResult
    installed.value = true
  } catch (error) {
    installError.value = error instanceof Error ? error.message : '安装失败'
  } finally {
    checking.value = false
    installing.value = false
  }
}

function reloadPage() {
  window.location.reload()
}

function storageSummary() {
  if (form.storage.driver === 'local') return form.storage.local_dir
  return `${selectedProvider.value?.label || form.storage.s3.provider} / ${form.storage.s3.bucket}`
}

function redisSummary() {
  return `${form.redis.host}:${form.redis.port} · DB ${form.redis.db}`
}

onMounted(initialize)
</script>

<template>
  <div class="install-page">
    <div class="install-shell">
      <header class="install-head">
        <div class="brand-mark">智</div>
        <div class="brand-copy">
          <div class="eyebrow">SMARTDOC VISION · SETUP</div>
          <h1>智审图档 · 系统安装</h1>
          <p>首次启动检测到尚未配置数据库与存储，请完成安装向导。</p>
        </div>
        <span v-if="status" class="version-chip">v{{ status.version }}</span>
      </header>

      <div v-if="loading" class="state-card">
        <n-spin size="small" />
        <span>正在读取系统环境…</span>
      </div>

      <div v-else-if="loadError" class="state-card error">
        <AlertCircleOutline class="state-icon" />
        <div>
          <strong>{{ loadError }}</strong>
          <p>请确认后端服务已启动，或刷新页面重试。</p>
        </div>
        <n-button secondary @click="initialize">
          <template #icon><RefreshOutline /></template>
          重试
        </n-button>
      </div>

      <template v-else-if="configMissing">
        <section class="state-card error-block">
          <AlertCircleOutline class="state-icon" />
          <div>
            <strong>检测到安装标记，但配置文件缺失</strong>
            <p>
              系统已经安装过，为防止误重装，安装向导已禁用。请恢复原有的 <code>.env</code> 配置文件后重启服务。
            </p>
            <p class="path-line">安装标记：<code>{{ status?.marker_path || '.install.txt' }}</code></p>
            <p class="path-line">期望配置：<code>{{ status?.env_file || '.env' }}</code></p>
          </div>
          <n-button secondary @click="reloadPage">
            <template #icon><RefreshOutline /></template>
            刷新
          </n-button>
        </section>
      </template>

      <template v-else-if="installed">
        <section class="state-card success-block">
          <CheckmarkCircleOutline class="state-icon success" />
          <div>
            <strong>安装完成</strong>
            <p>数据库表结构、基础数据、管理员账号与配置文件均已生成。</p>
            <p class="path-line">安装时间：<code>{{ installedInfo?.installed_at }}</code></p>
            <p class="path-line">配置文件：<code>{{ installedInfo?.env_file }}</code></p>
            <p class="path-line">安装标记：<code>.install.txt</code></p>
            <p class="muted">刷新页面后即可使用刚设置的管理员账号登录。</p>
          </div>
          <n-button type="primary" @click="reloadPage">
            <template #icon><RefreshOutline /></template>
            刷新页面
          </n-button>
        </section>
      </template>

      <template v-else>
        <n-steps :current="activeStep + 1" size="small" class="install-steps">
          <n-step v-for="title in stepTitles" :key="title" :title="title" />
        </n-steps>

        <section class="install-card">
          <template v-if="activeStep === 0">
            <div class="step-head">
              <span class="step-icon"><DocumentTextOutline /></span>
              <div>
                <h2>开始之前</h2>
                <p>请确认服务器环境，并完整阅读用户使用协议。</p>
              </div>
            </div>

            <div class="system-grid">
              <div class="system-item"><span>操作系统</span><strong>{{ status?.system.os }} / {{ status?.system.arch }}</strong></div>
              <div class="system-item"><span>Go 运行时</span><strong>{{ status?.system.go_version }}</strong></div>
              <div class="system-item"><span>运行目录</span><strong class="mono">{{ status?.system.working_dir }}</strong></div>
              <div class="system-item"><span>默认本地存储</span><strong class="mono">{{ status?.system.local_storage_dir }}</strong></div>
              <div class="system-item"><span>配置文件</span><strong class="mono">{{ status?.system.env_file }}</strong></div>
              <div class="system-item">
                <span>运行环境</span>
                <strong>
                  独立进程
                  <n-tag v-if="status?.system.docker" size="small" type="warning" class="env-tag">Docker</n-tag>
                </strong>
              </div>
            </div>

            <div ref="agreementBox" class="agreement-box" @scroll="onAgreementScroll">
              <h3>智审图档（SmartDoc Vision）用户使用协议与免责声明</h3>
              <p>一、软件许可。智审图档以 MIT 许可证开源发布。您可以在遵守 MIT 许可证条款的前提下自由使用、复制、修改、合并、发布、分发、再许可和/或销售本软件的副本。</p>
              <p>二、按“现状”提供。本软件按“现状”提供，不附带任何明示或默示的担保，包括但不限于对适销性、特定用途适用性以及不侵权的担保。无论在任何情况下，作者或版权持有人均不对因本软件或本软件的使用或其他交易而产生的任何索赔、损害或其他责任负责，无论是在合同诉讼、侵权行为还是其他方面。</p>
              <p>三、使用风险自担。您理解并同意：部署、配置、使用本系统所产生的全部风险由您自行承担。包括但不限于数据丢失、数据泄露、审核结论错误、业务中断、合规风险、法律风险以及因模型服务、数据库、Redis、对象存储或网络环境导致的任何损失。</p>
              <p>四、审核结论仅供参考。本系统调用第三方或本地大模型输出审核建议，模型结果可能存在错误、遗漏或偏差。您不得将系统输出作为唯一依据用于作出具有法律效力的决定；涉及档案合规、行政认定、司法程序等场景时，应由具备相应资质的专业人员复核。</p>
              <p>五、数据与隐私。您应确保提交审核的文本、图片和其他资料具有合法来源，并已取得必要的授权。您应自行负责数据库、Redis、对象存储和备份的访问控制、加密与保留策略。开发者不接触、不存储、不控制您在部署实例中处理的业务数据。</p>
              <p>六、第三方服务。本系统可以接入第三方大模型、对象存储、邮件等外部服务。这些服务受其各自条款与隐私政策约束，相关费用、可用性、数据处理行为由您与对应服务商约定，开发者不承担任何责任。</p>
              <p>七、责任限制。在适用法律允许的最大范围内，开发者、贡献者与版权持有人对因使用或无法使用本软件造成的任何直接、间接、偶然、特殊、惩罚性或后果性损害不承担任何责任，即使已被告知发生此类损害的可能性。</p>
              <p>八、协议变更。本协议可能随项目版本更新而调整。继续使用新版本即视为接受更新后的协议。若您不同意本协议的任何内容，请立即停止使用并删除本软件。</p>
              <p class="agreement-end">—— 协议全文结束 ——</p>
            </div>

            <n-checkbox v-model:checked="agreed" :disabled="!agreementRead">
              我已完整阅读并同意上述用户使用协议与免责声明
            </n-checkbox>
            <p v-if="!agreementRead" class="hint">请将协议内容滚动到底部后再勾选同意。</p>
          </template>

          <template v-else-if="activeStep === 1">
            <div class="step-head">
              <span class="step-icon"><ServerOutline /></span>
              <div>
                <h2>MySQL 数据库</h2>
                <p>用于保存账号、项目、审核配置和任务记录。数据库不存在时会自动创建。</p>
              </div>
            </div>
            <div class="form-grid">
              <label>主机地址<em>*</em><n-input v-model:value="form.mysql.host" placeholder="127.0.0.1" /></label>
              <label>端口<em>*</em><n-input-number v-model:value="form.mysql.port" :min="1" :max="65535" :show-button="false" /></label>
              <label>用户名<em>*</em><n-input v-model:value="form.mysql.user" placeholder="root" /></label>
              <label>密码<n-input v-model:value="form.mysql.password" type="password" show-password-on="click" placeholder="无密码可留空" /></label>
              <label>数据库名<em>*</em><n-input v-model:value="form.mysql.database" placeholder="smart-doc-vision" /></label>
              <label>字符集<n-input v-model:value="form.mysql.charset" placeholder="utf8mb4" /></label>
            </div>
            <n-alert type="info" :bordered="false">
              账号需要具备建库、建表和读写权限。安装时会执行 CREATE DATABASE IF NOT EXISTS 并创建全部数据表。
            </n-alert>
          </template>

          <template v-else-if="activeStep === 2">
            <div class="step-head">
              <span class="step-icon"><LinkOutline /></span>
              <div>
                <h2>Redis</h2>
                <p>用于任务队列、去重锁、运行监控与缓存。密码可以为空。</p>
              </div>
            </div>
            <div class="form-grid">
              <label>主机地址<em>*</em><n-input v-model:value="form.redis.host" placeholder="127.0.0.1" /></label>
              <label>端口<em>*</em><n-input-number v-model:value="form.redis.port" :min="1" :max="65535" :show-button="false" /></label>
              <label>密码<n-input v-model:value="form.redis.password" type="password" show-password-on="click" placeholder="无密码可留空" /></label>
              <label>数据库编号<em>*</em><n-input-number v-model:value="form.redis.db" :min="0" :max="15" :show-button="false" /></label>
            </div>
            <n-alert type="info" :bordered="false">
              Redis 必须允许当前服务器访问。请避免与其他业务共用同一个数据库编号。
            </n-alert>
          </template>

          <template v-else-if="activeStep === 3">
            <div class="step-head">
              <span class="step-icon"><CloudUploadOutline /></span>
              <div>
                <h2>文件存储</h2>
                <p>上传的图片和文件将保存到这里，数据库中只保留对象路径，不再写入图片内容。</p>
              </div>
            </div>

            <n-radio-group v-model:value="form.storage.driver" class="storage-switch">
              <n-radio value="local">本地文件存储</n-radio>
              <n-radio value="s3">对象存储（S3 兼容）</n-radio>
            </n-radio-group>

            <template v-if="form.storage.driver === 'local'">
              <div class="form-grid single">
                <label>本地目录<em>*</em><n-input v-model:value="form.storage.local_dir" placeholder="例如 /app/data/uploads" /></label>
              </div>
              <n-alert type="info" :bordered="false">
                默认目录为 <code>{{ status?.system.local_storage_dir }}</code>。图片会保存到该目录下按任务月份划分的子目录中。
              </n-alert>
              <n-alert v-if="status?.system.docker" type="warning" :bordered="false">
                {{ status?.system.docker_mount_hint }}
              </n-alert>
            </template>

            <template v-else>
              <div class="form-grid">
                <label>服务商<em>*</em>
                  <n-select
                    :value="form.storage.s3.provider"
                    :options="providerOptions.map((item) => ({ label: item.label, value: item.value }))"
                    @update:value="onProviderChange"
                  />
                </label>
                <label>Region<n-input v-model:value="form.storage.s3.region" :placeholder="selectedProvider?.region || '按服务商填写'" /></label>
                <label class="span-2">Endpoint<n-input v-model:value="form.storage.s3.endpoint" :placeholder="endpointPlaceholder" /></label>
                <label>Bucket<em>*</em><n-input v-model:value="form.storage.s3.bucket" placeholder="smartdoc-uploads" /></label>
                <label>路径前缀<n-input v-model:value="form.storage.s3.prefix" placeholder="可选，例如 smartdoc" /></label>
                <label>AccessKey<em>*</em><n-input v-model:value="form.storage.s3.access_key" placeholder="Access Key ID" /></label>
                <label>SecretKey<em>*</em><n-input v-model:value="form.storage.s3.secret_key" type="password" show-password-on="click" placeholder="Secret Access Key" /></label>
              </div>
              <div class="inline-check">
                <n-checkbox v-model:checked="form.storage.s3.force_path_style">使用 Path-Style 访问（部分自建 S3 需要）</n-checkbox>
              </div>
              <n-alert type="info" :bordered="false">
                阿里云 OSS、腾讯云 COS、七牛云 Kodo 均通过 S3 兼容接口接入。Endpoint 留空时按服务商和 Region 自动推导；私有 Bucket 会生成带时效的预签名地址供页面预览。
              </n-alert>
            </template>

            <div class="form-grid single public-base">
              <label>公开访问前缀（可选）<n-input v-model:value="form.storage.public_base_url" placeholder="例如 https://cdn.example.com/uploads" /></label>
            </div>
            <p class="hint">配置公开访问前缀后，系统直接返回该前缀 + 对象路径；不配置时本地存储使用签名地址，对象存储使用预签名地址。</p>
          </template>

          <template v-else-if="activeStep === 4">
            <div class="step-head">
              <span class="step-icon"><ShieldCheckmarkOutline /></span>
              <div>
                <h2>超级管理员</h2>
                <p>创建第一个后台账号，用于登录系统并继续配置模型与审核规则。</p>
              </div>
            </div>
            <div class="form-grid">
              <label>账号<em>*</em><n-input v-model:value="form.admin.username" placeholder="admin" /></label>
              <label>密码<em>*</em><n-input v-model:value="form.admin.password" type="password" show-password-on="click" placeholder="至少 8 位" /></label>
              <label>确认密码<em>*</em><n-input v-model:value="form.admin.confirm" type="password" show-password-on="click" placeholder="再次输入密码" /></label>
            </div>
            <p class="hint">密码长度为 8~72 位。安装完成后请妥善保存，后续可在个人中心修改。</p>
          </template>

          <template v-else>
            <div class="step-head">
              <span class="step-icon"><CheckmarkCircleOutline /></span>
              <div>
                <h2>检测并安装</h2>
                <p>系统将依次检测 MySQL、Redis 和文件存储，全部通过后创建表结构、基础数据和管理员账号。</p>
              </div>
            </div>

            <div class="review-list">
              <div><span>MySQL</span><strong>{{ form.mysql.host }}:{{ form.mysql.port }} / {{ form.mysql.database }}</strong></div>
              <div><span>Redis</span><strong>{{ redisSummary() }}</strong></div>
              <div><span>文件存储</span><strong>{{ storageSummary() }}</strong></div>
              <div><span>管理员</span><strong>{{ form.admin.username }}</strong></div>
            </div>

            <div v-if="probes.length" class="probe-list">
              <div v-for="probe in probes" :key="probe.key" class="probe-item" :class="{ ok: probe.ok, fail: !probe.ok }">
                <CheckmarkCircleOutline v-if="probe.ok" class="probe-icon" />
                <AlertCircleOutline v-else class="probe-icon" />
                <div class="probe-copy">
                  <strong>{{ probe.label }}</strong>
                  <p>{{ probe.message }}</p>
                  <p v-if="probe.note" class="probe-note">{{ probe.note }}</p>
                </div>
                <span class="probe-time">{{ probe.latency_ms }} ms</span>
              </div>
            </div>

            <n-alert v-if="installError" type="error" :bordered="false">{{ installError }}</n-alert>
            <n-alert type="warning" :bordered="false">
              安装过程会写入 <code>.env</code> 与 <code>.install.txt</code>，并创建数据库表、基础数据和超级管理员账号。请勿重复提交。
            </n-alert>

            <n-button type="primary" size="large" block :loading="checking || installing" @click="runInstall">
              <template #icon><CloudUploadOutline /></template>
              {{ installing ? '正在创建数据表与管理员账号…' : checking ? '正在检测环境…' : '开始检测并安装' }}
            </n-button>
          </template>

          <p v-if="stepError" class="step-error">{{ stepError }}</p>
        </section>

        <footer class="wizard-actions">
          <n-button secondary :disabled="activeStep === 0 || checking || installing" @click="prevStep">
            <template #icon><ArrowBackOutline /></template>
            上一步
          </n-button>
          <n-button
            v-if="activeStep < stepTitles.length - 1"
            type="primary"
            :disabled="!canProceed"
            @click="nextStep"
          >
            下一步
            <template #icon><ArrowForwardOutline /></template>
          </n-button>
        </footer>
      </template>
    </div>
  </div>
</template>

<style scoped>
.install-page {
  min-height: 100vh;
  padding: 34px 20px 60px;
  background:
    radial-gradient(circle at 12% 8%, var(--brand-soft), transparent 32%),
    radial-gradient(circle at 88% 0%, rgba(56, 189, 248, 0.12), transparent 30%),
    var(--canvas);
}

.install-shell {
  width: min(1080px, 100%);
  margin: 0 auto;
}

.install-head {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 22px;
}

.brand-mark {
  display: flex;
  width: 52px;
  height: 52px;
  align-items: center;
  justify-content: center;
  border-radius: 15px;
  background: linear-gradient(140deg, var(--brand), var(--brand-strong));
  color: #fff;
  font-size: 22px;
  font-weight: 700;
  box-shadow: 0 10px 24px rgba(43, 108, 246, 0.28);
  flex: none;
}

.brand-copy {
  min-width: 0;
  flex: 1;
}

.eyebrow {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.16em;
}

.brand-copy h1 {
  margin: 3px 0 2px;
  color: var(--ink);
  font-size: 22px;
  font-weight: 680;
  letter-spacing: -0.01em;
}

.brand-copy p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
}

.version-chip {
  padding: 4px 10px;
  border: 1px solid var(--border);
  border-radius: 99px;
  background: var(--card);
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9.5px;
  flex: none;
}

.state-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 26px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
  color: var(--muted);
  font-size: 12px;
}

.state-card strong {
  display: block;
  margin-bottom: 4px;
  color: var(--ink);
  font-size: 14px;
}

.state-card p {
  margin: 4px 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.65;
}

.state-card.error-block,
.state-card.success-block {
  align-items: flex-start;
}

.state-card > div {
  min-width: 0;
  flex: 1;
}

.state-icon {
  width: 26px;
  height: 26px;
  color: var(--red);
  flex: none;
}

.state-icon.success {
  color: var(--green);
}

.state-card code,
.path-line code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--text);
  font-family: var(--mono);
  font-size: 10px;
  word-break: break-all;
}

.path-line {
  font-size: 10.5px !important;
}

.muted {
  color: var(--muted-light) !important;
}

.install-steps {
  margin-bottom: 16px;
  padding: 14px 18px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--card);
}

.install-card {
  padding: 24px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
}

.step-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 20px;
}

.step-icon {
  display: flex;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.step-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.step-head h2 {
  margin: 1px 0 3px;
  color: var(--ink);
  font-size: 16px;
  font-weight: 650;
}

.step-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.6;
}

.system-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
  margin-bottom: 18px;
}

.system-item {
  padding: 11px 13px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-soft);
}

.system-item span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
  letter-spacing: 0.04em;
}

.system-item strong {
  display: block;
  overflow: hidden;
  margin-top: 3px;
  color: var(--text);
  font-size: 11px;
  font-weight: 620;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.system-item .mono {
  font-family: var(--mono);
  font-size: 10px;
}

.env-tag {
  margin-left: 6px;
}

.agreement-box {
  height: 260px;
  overflow-y: auto;
  margin-bottom: 14px;
  padding: 18px 20px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface-soft);
  scroll-behavior: smooth;
}

.agreement-box h3 {
  margin: 0 0 12px;
  color: var(--ink);
  font-size: 13px;
  font-weight: 650;
}

.agreement-box p {
  margin: 0 0 11px;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.85;
}

.agreement-end {
  color: var(--muted-light) !important;
  text-align: center;
  letter-spacing: 0.08em;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 18px;
  margin-bottom: 16px;
}

.form-grid.single {
  grid-template-columns: 1fr;
}

.form-grid .span-2 {
  grid-column: span 2;
}

.form-grid label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  color: var(--muted);
  font-size: 10.5px;
}

.form-grid label em {
  margin-left: 3px;
  color: var(--red);
  font-style: normal;
}

.storage-switch {
  display: flex;
  gap: 24px;
  margin-bottom: 18px;
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--surface-soft);
}

.inline-check {
  margin: 2px 0 14px;
}

.public-base {
  margin-top: 16px;
}

.hint {
  margin: 8px 0 0;
  color: var(--muted-light);
  font-size: 10px;
  line-height: 1.6;
}

.install-card :deep(.n-alert) {
  margin-bottom: 12px;
}

.review-list {
  display: grid;
  gap: 8px;
  margin-bottom: 16px;
}

.review-list > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 10px 13px;
  border: 1px dashed var(--border);
  border-radius: 9px;
  background: var(--surface-soft);
}

.review-list span {
  color: var(--muted);
  font-size: 10.5px;
}

.review-list strong {
  overflow: hidden;
  color: var(--text);
  font-family: var(--mono);
  font-size: 10.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.probe-list {
  display: grid;
  gap: 8px;
  margin-bottom: 14px;
}

.probe-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 11px 13px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-soft);
}

.probe-item.ok {
  border-color: rgba(34, 197, 94, 0.28);
  background: var(--green-soft);
}

.probe-item.fail {
  border-color: rgba(239, 68, 68, 0.28);
  background: var(--red-soft);
}

.probe-icon {
  width: 17px;
  height: 17px;
  margin-top: 1px;
  color: var(--muted);
  flex: none;
}

.probe-item.ok .probe-icon {
  color: var(--green);
}

.probe-item.fail .probe-icon {
  color: var(--red);
}

.probe-copy {
  min-width: 0;
  flex: 1;
}

.probe-copy strong {
  color: var(--ink);
  font-size: 11px;
}

.probe-copy p {
  margin: 2px 0 0;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.55;
}

.probe-note {
  color: var(--muted-light) !important;
}

.probe-time {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
  flex: none;
}

.step-error {
  margin: 12px 0 0;
  color: var(--red);
  font-size: 10.5px;
}

.wizard-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
}

@media (max-width: 720px) {
  .install-page {
    padding: 20px 12px 40px;
  }

  .install-head {
    align-items: flex-start;
  }

  .brand-copy h1 {
    font-size: 18px;
  }

  .install-card {
    padding: 18px 15px;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }

  .form-grid .span-2 {
    grid-column: span 1;
  }

  .install-steps {
    overflow-x: auto;
  }
}
</style>
