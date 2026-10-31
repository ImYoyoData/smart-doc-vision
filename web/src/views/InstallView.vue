<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCheckbox,
  NInput,
  NInputNumber,
  NModal,
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
import BrandMark from '@/components/BrandMark.vue'
import { installApi, type DatabaseMode, type InstallDefaults, type InstallProbeResult, type InstallRequest } from '@/api/install'
import type { SchemaReport } from '@/api/schema'
import { useInstallStore } from '@/stores/install'
import { useSiteStore } from '@/stores/site'

const store = useInstallStore()
const site = useSiteStore()
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
const databaseReport = ref<SchemaReport | null>(null)
const databaseModal = ref(false)
const databaseChoice = ref<'repair' | 'reset'>('repair')
const confirmModal = ref(false)
const databaseAck = ref(false)

const stepTitles = ['介绍与协议', '数据库', 'Redis', '文件存储', '管理员', '检测并安装']
const stepDescriptions = [
  '阅读并同意使用协议',
  'MySQL 或 PostgreSQL',
  '队列与缓存',
  '本地目录或对象存储',
  '创建首个账号',
  '检测环境并写入配置',
]

function onStepChange(current: number) {
  if (checking.value || installing.value) return
  const target = current - 1
  if (target < 0 || target > activeStep.value) return
  stepError.value = ''
  activeStep.value = target
}

const form = reactive({
  database: {
    driver: 'mysql' as 'mysql' | 'postgres',
    host: '127.0.0.1',
    port: 3306 as number | null,
    user: 'root',
    password: '',
    database: 'smart-doc-vision',
    charset: 'utf8mb4',
    sslmode: 'disable',
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
  site: {
    name: '',
    name_en: '',
    logo: '',
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
const databaseLabel = computed(() => (form.database.driver === 'postgres' ? 'PostgreSQL' : 'MySQL'))
const sslModeOptions = [
  { label: 'disable（不加密，内网常用）', value: 'disable' },
  { label: 'require（要求加密）', value: 'require' },
  { label: 'verify-ca（校验证书）', value: 'verify-ca' },
  { label: 'verify-full（校验证书与主机名）', value: 'verify-full' },
]

function onDatabaseDriverChange(value: string | number | null) {
  const driver = value === 'postgres' ? 'postgres' : 'mysql'
  const previousDefaultPort = form.database.driver === 'postgres' ? 5432 : 3306
  form.database.driver = driver
  if (form.database.port === previousDefaultPort || !form.database.port) {
    form.database.port = driver === 'postgres' ? 5432 : 3306
  }
  if (!form.database.user || form.database.user === 'root' || form.database.user === 'postgres') {
    form.database.user = driver === 'postgres' ? 'postgres' : 'root'
  }
}

const canProceed = computed(() => {
  switch (activeStep.value) {
    case 0:
      return agreed.value
    case 1:
      return Boolean(
        form.database.host.trim() &&
          form.database.user.trim() &&
          form.database.database.trim() &&
          form.database.port &&
          form.database.port > 0 &&
          form.database.port <= 65535,
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
  Object.assign(form.database, defaults.database)
  form.database.password = ''
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
  form.site.name = defaults.site?.name || '智审图档'
  form.site.name_en = defaults.site?.name_en || 'SmartDoc Vision'
  form.site.logo = defaults.site?.logo || ''
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

function buildPayload(mode: DatabaseMode | '' = '', confirmReset = false, confirmRepair = false): InstallRequest {
  return {
    agreement: agreed.value,
    database: {
      driver: form.database.driver,
      host: form.database.host.trim(),
      port: Number(form.database.port || (form.database.driver === 'postgres' ? 5432 : 3306)),
      user: form.database.user.trim(),
      password: form.database.password,
      database: form.database.database.trim(),
      charset: form.database.driver === 'mysql' ? form.database.charset.trim() || 'utf8mb4' : '',
      sslmode: form.database.driver === 'postgres' ? form.database.sslmode : '',
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
    site: {
      name: form.site.name.trim(),
      name_en: form.site.name_en.trim(),
      logo: form.site.logo.trim(),
    },
    database_mode: mode,
    confirm_reset: confirmReset,
    confirm_repair: confirmRepair,
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
    if (result.database_error) {
      installError.value = result.database_error
      return
    }
    if (!result.ok) {
      installError.value = '环境检测未通过，请根据下方提示修正后重试'
      return
    }
    if (result.database?.existing) {
      databaseReport.value = result.database
      databaseChoice.value = 'repair'
      databaseModal.value = true
      return
    }
    await executeInstall('create', false)
  } catch (error) {
    installError.value = error instanceof Error ? error.message : '安装失败'
  } finally {
    checking.value = false
  }
}

async function executeInstall(mode: DatabaseMode, confirmed: boolean) {
  installError.value = ''
  installing.value = true
  try {
    const installedResult = await installApi.install(
      buildPayload(mode, mode === 'reset' && confirmed, mode === 'repair' && confirmed),
    )
    installedInfo.value = installedResult
    installed.value = true
    databaseModal.value = false
    confirmModal.value = false
  } catch (error) {
    installError.value = error instanceof Error ? error.message : '安装失败'
    confirmModal.value = false
  } finally {
    installing.value = false
  }
}

function openDatabaseConfirm() {
  databaseAck.value = false
  confirmModal.value = true
}

async function confirmDatabaseInstall() {
  if (!databaseAck.value || installing.value) return
  const mode = databaseChoice.value
  confirmModal.value = false
  databaseModal.value = false
  await executeInstall(mode, true)
}

function reportColumn(change: { table: string; column: string; expected?: string }) {
  return `${change.table}.${change.column}${change.expected ? ` → ${change.expected}` : ''}`
}

function reportModified(change: { table: string; column: string; expected?: string; actual?: string }) {
  return `${change.table}.${change.column}：${change.actual || '—'} → ${change.expected || '—'}`
}

const reportExistingTables = computed(() => databaseReport.value?.existing_tables || [])
const reportMissingTables = computed(() => databaseReport.value?.missing_tables || [])
const reportAddedColumns = computed(() => databaseReport.value?.added_columns || [])
const reportModifiedColumns = computed(() => databaseReport.value?.modified_columns || [])
const reportAddedIndexes = computed(() => databaseReport.value?.added_indexes || [])
const reportExtraTables = computed(() => databaseReport.value?.extra_tables || [])
const reportExtraColumns = computed(() => databaseReport.value?.extra_columns || [])
const reportWarnings = computed(() => databaseReport.value?.warnings || [])

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
        <BrandMark :size="52" />
        <div class="brand-copy">
          <div class="eyebrow">{{ site.nameEn }} · SETUP</div>
          <h1>{{ site.displayName }} · 系统安装</h1>
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
        <n-steps
          :current="activeStep + 1"
          size="small"
          class="install-steps"
          @update:current="onStepChange"
        >
          <n-step
            v-for="(title, index) in stepTitles"
            :key="title"
            :title="title"
            :description="stepDescriptions[index]"
            :disabled="index > activeStep || checking || installing"
          />
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
              <h3>{{ site.displayName }}（{{ site.nameEn }}）用户使用协议与免责声明</h3>
              <p>一、软件许可。{{ site.displayName }} 以 MIT 许可证开源发布。您可以在遵守 MIT 许可证条款的前提下自由使用、复制、修改、合并、发布、分发、再许可和/或销售本软件的副本。</p>
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
                <h2>数据库</h2>
                <p>支持 MySQL 与 PostgreSQL，用于保存账号、项目、审核配置和任务记录。数据库不存在时会自动创建。</p>
              </div>
            </div>
            <n-radio-group :value="form.database.driver" class="database-switch" @update:value="onDatabaseDriverChange">
              <n-radio value="mysql">MySQL</n-radio>
              <n-radio value="postgres">PostgreSQL</n-radio>
            </n-radio-group>
            <div class="form-grid">
              <label>主机地址<em>*</em><n-input v-model:value="form.database.host" placeholder="127.0.0.1" /></label>
              <label>端口<em>*</em>
                <n-input-number
                  v-model:value="form.database.port"
                  :min="1"
                  :max="65535"
                  :show-button="false"
                  :placeholder="form.database.driver === 'postgres' ? '5432' : '3306'"
                />
              </label>
              <label>用户名<em>*</em>
                <n-input v-model:value="form.database.user" :placeholder="form.database.driver === 'postgres' ? 'postgres' : 'root'" />
              </label>
              <label>密码<n-input v-model:value="form.database.password" type="password" show-password-on="click" placeholder="无密码可留空" /></label>
              <label>数据库名<em>*</em><n-input v-model:value="form.database.database" placeholder="smart-doc-vision" /></label>
              <label v-if="form.database.driver === 'mysql'">字符集<n-input v-model:value="form.database.charset" placeholder="utf8mb4" /></label>
              <label v-else>SSL 模式<n-select v-model:value="form.database.sslmode" :options="sslModeOptions" /></label>
            </div>
            <n-alert type="info" :bordered="false">
              {{ databaseLabel }} 账号需要具备建库、建表和读写权限。安装时会自动创建数据库（MySQL 使用 CREATE DATABASE IF NOT EXISTS，PostgreSQL 使用 CREATE DATABASE）并创建全部数据表。
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

            <div class="brand-subhead">
              <h3>站点品牌（可选）</h3>
              <p>留空使用平台默认名称和内置图标；安装后也可以修改 .env 中的 SITE_NAME、SITE_NAME_EN、SITE_LOGO。</p>
            </div>
            <div class="form-grid">
              <label>网站名称<n-input v-model:value="form.site.name" placeholder="留空使用默认：智审图档" maxlength="64" /></label>
              <label>英文名称<n-input v-model:value="form.site.name_en" placeholder="留空使用默认：SmartDoc Vision" maxlength="64" /></label>
              <label class="span-2">Logo 地址或路径<n-input v-model:value="form.site.logo" placeholder="https://... 或服务器本地图片路径；留空使用内置图标" maxlength="1024" /></label>
            </div>
            <p class="hint">自定义 Logo 支持 https 链接、data:image 数据或服务器上可读的 png/jpg/webp/svg 图片路径。修改 .env 后需要重启服务生效。</p>
          </template>

          <template v-else>
            <div class="step-head">
              <span class="step-icon"><CheckmarkCircleOutline /></span>
              <div>
                <h2>检测并安装</h2>
                <p>系统将依次检测数据库、Redis 和文件存储，全部通过后创建表结构、基础数据和管理员账号。</p>
              </div>
            </div>

            <div class="review-list">
              <div><span>{{ databaseLabel }}</span><strong>{{ form.database.host }}:{{ form.database.port }} / {{ form.database.database }}</strong></div>
              <div><span>Redis</span><strong>{{ redisSummary() }}</strong></div>
              <div><span>文件存储</span><strong>{{ storageSummary() }}</strong></div>
              <div><span>管理员</span><strong>{{ form.admin.username }}</strong></div>
              <div><span>站点品牌</span><strong>{{ form.site.name || '默认平台品牌' }}{{ form.site.logo ? ' · 自定义 Logo' : '' }}</strong></div>
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

    <n-modal
      v-model:show="databaseModal"
      preset="card"
      :mask-closable="false"
      style="width: min(860px, 94vw)"
      title="检测到数据库已有业务表"
    >
      <div class="db-modal-body">
        <n-alert type="warning" :bordered="false">
          目标数据库中已经存在本系统的表。请选择“校验并修复”保留现有数据，或“清空并重新安装”重建。
          两种操作都需要再次确认。
        </n-alert>
        <div class="db-summary">
          <span>已有业务表 <strong>{{ reportExistingTables.length }}</strong> 张</span>
          <span>程序结构版本 <strong>{{ databaseReport?.schema_version || '—' }}</strong></span>
          <span>数据库记录版本 <strong>{{ databaseReport?.database_version || '未记录' }}</strong></span>
        </div>
        <n-alert v-if="databaseReport?.missing_core" type="error" :bordered="false">
          核心用户表或关键字段缺失。校验修复会补齐表结构，但现有账号可能无法保留。
        </n-alert>

        <div class="db-diff">
          <section v-if="reportMissingTables.length">
            <h4>将新建表</h4>
            <div class="diff-chips">
              <span v-for="table in reportMissingTables" :key="table" class="diff-chip">{{ table }}</span>
            </div>
          </section>
          <section v-if="reportAddedColumns.length">
            <h4>将新增列</h4>
            <ul>
              <li v-for="change in reportAddedColumns" :key="`${change.table}.${change.column}`">
                {{ reportColumn(change) }}
              </li>
            </ul>
          </section>
          <section v-if="reportModifiedColumns.length">
            <h4>将调整列（可能影响数据）</h4>
            <ul>
              <li v-for="change in reportModifiedColumns" :key="`${change.table}.${change.column}`">
                {{ reportModified(change) }}
                <em v-if="change.reason">{{ change.reason }}</em>
              </li>
            </ul>
          </section>
          <section v-if="reportAddedIndexes.length">
            <h4>将新增索引</h4>
            <ul>
              <li v-for="index in reportAddedIndexes" :key="`${index.table}.${index.name}`">
                {{ index.table }} · {{ index.name }}<span v-if="index.columns">（{{ index.columns }}）</span>
              </li>
            </ul>
          </section>
          <section v-if="reportExtraTables.length || reportExtraColumns.length">
            <h4>模型之外，保留不删除</h4>
            <div class="diff-chips">
              <span v-for="table in reportExtraTables" :key="table" class="diff-chip muted">{{ table }}</span>
              <span v-for="change in reportExtraColumns" :key="`${change.table}.${change.column}`" class="diff-chip muted">
                {{ change.table }}.{{ change.column }}
              </span>
            </div>
          </section>
        </div>

        <div v-if="reportWarnings.length" class="db-warnings">
          <p v-for="warning in reportWarnings" :key="warning">{{ warning }}</p>
        </div>

        <n-radio-group v-model:value="databaseChoice" class="db-options">
          <label class="db-option" :class="{ active: databaseChoice === 'repair' }" @click="databaseChoice = 'repair'">
            <n-radio value="repair" />
            <div>
              <strong>验证并修复表结构（推荐，保留数据）</strong>
              <p>补齐缺失的表、字段和索引；只调整与模型不一致的列。不会删除模型之外的列或表。</p>
            </div>
          </label>
          <label class="db-option danger" :class="{ active: databaseChoice === 'reset' }" @click="databaseChoice = 'reset'">
            <n-radio value="reset" />
            <div>
              <strong>清空数据表并重新安装</strong>
              <p>删除本系统已知业务表及全部数据后重建。无关表保留；操作不可恢复，需要二次确认。</p>
            </div>
          </label>
        </n-radio-group>

        <div class="modal-actions">
          <n-button secondary :disabled="installing" @click="databaseModal = false">取消</n-button>
          <n-button type="primary" :disabled="installing" @click="openDatabaseConfirm">继续</n-button>
        </div>
      </div>
    </n-modal>

    <n-modal
      v-model:show="confirmModal"
      preset="card"
      :mask-closable="false"
      style="width: min(560px, 92vw)"
      :title="databaseChoice === 'reset' ? '再次确认：清空数据库' : '再次确认：修复数据库结构'"
    >
      <div class="confirm-body">
        <n-alert :type="databaseChoice === 'reset' ? 'error' : 'warning'" :bordered="false">
          {{
            databaseChoice === 'reset'
              ? '此操作会删除本系统已知表的全部数据（包括账号、项目、审核配置和任务），且不可恢复。'
              : '兼容修复会新增缺失的表、字段和索引，并按模型调整部分列类型；调整可能导致超长数据被截断。'
          }}
        </n-alert>
        <p v-if="databaseChoice === 'reset'" class="confirm-list">
          建议先完成数据库备份。系统不会删除模型之外的表。
        </p>
        <n-checkbox v-model:checked="databaseAck">
          {{
            databaseChoice === 'reset'
              ? '我已备份数据库，并确认清空以上数据表'
              : '我已备份数据库，并了解列类型调整风险'
          }}
        </n-checkbox>
        <div class="modal-actions">
          <n-button secondary :disabled="installing" @click="confirmModal = false">返回</n-button>
          <n-button
            :type="databaseChoice === 'reset' ? 'error' : 'primary'"
            :disabled="!databaseAck || installing"
            :loading="installing"
            @click="confirmDatabaseInstall"
          >
            确认并执行
          </n-button>
        </div>
      </div>
    </n-modal>
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

.install-steps :deep(.n-step-description) {
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 10px;
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

.storage-switch,
.database-switch {
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

.brand-subhead {
  margin: 18px 0 10px;
  padding-top: 14px;
  border-top: 1px solid var(--border);
}

.brand-subhead h3 {
  margin: 0 0 4px;
  font-size: 14px;
}

.brand-subhead p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.6;
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

.db-modal-body,
.confirm-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.db-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  padding: 10px 13px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-soft);
  color: var(--muted);
  font-size: 10.5px;
}

.db-summary strong {
  color: var(--text);
  font-family: var(--mono);
  font-weight: 620;
}

.db-diff {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 260px;
  overflow-y: auto;
  padding-right: 4px;
}

.db-diff h4 {
  margin: 0 0 6px;
  color: var(--ink);
  font-size: 11px;
  font-weight: 620;
}

.db-diff ul {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 4px 16px;
  margin: 0;
  padding-left: 17px;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.6;
}

.db-diff li em {
  margin-left: 5px;
  color: var(--muted-light);
  font-style: normal;
}

.diff-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.diff-chip {
  padding: 2px 7px;
  border: 1px solid var(--border);
  border-radius: 5px;
  background: var(--surface-soft);
  color: var(--text);
  font-family: var(--mono);
  font-size: 9.5px;
}

.diff-chip.muted {
  color: var(--muted-light);
  text-decoration: line-through;
}

.db-warnings {
  padding: 10px 13px;
  border: 1px solid rgba(239, 68, 68, 0.22);
  border-radius: 9px;
  background: var(--red-soft);
}

.db-warnings p {
  margin: 2px 0;
  color: var(--red);
  font-size: 10px;
  line-height: 1.6;
}

.db-options {
  display: grid;
  gap: 9px;
}

.db-option {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--card);
  cursor: pointer;
  transition: border-color 0.18s ease, background 0.18s ease;
}

.db-option.active {
  border-color: var(--brand);
  background: var(--brand-softer);
}

.db-option.danger.active {
  border-color: var(--red);
  background: var(--red-soft);
}

.db-option strong {
  display: block;
  color: var(--ink);
  font-size: 11.5px;
  font-weight: 620;
}

.db-option p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.6;
}

.confirm-list {
  margin: 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.65;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 2px;
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

  .install-steps :deep(.n-step-description) {
    display: none;
  }
}
</style>
