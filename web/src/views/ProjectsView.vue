<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NButton,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  useDialog,
  useMessage,
} from 'naive-ui'
import {
  AddOutline,
  ArrowForwardOutline,
  BarChartOutline,
  CheckmarkCircleOutline,
  DocumentTextOutline,
  EllipsisHorizontalOutline,
  FolderOpenOutline,
  KeyOutline,
  LockClosedOutline,
  PauseOutline,
  PlayOutline,
  RefreshOutline,
  SearchOutline,
  ServerOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import ProjectSecretsModal from '@/components/ProjectSecretsModal.vue'
import { projectApi } from '@/api'
import type { Project } from '@/api/types'
import { useWorkspaceStore } from '@/stores/workspace'
import { useAuthStore } from '@/stores/auth'

const workspace = useWorkspaceStore()
const auth = useAuthStore()
const message = useMessage()
const dialog = useDialog()
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const saving = ref(false)
const modalShow = ref(false)
const editing = ref<Project | null>(null)
const errorText = ref('')
const keyword = ref('')
const statusFilter = ref<string | null>(null)
const secretsShow = ref(false)
const secretsProject = ref<Project | null>(null)

const form = reactive({
  name: '',
  code: '',
  env: 'dev',
  description: '',
  retentionDays: 30,
  maxTasksPerDay: 0,
  maxConcurrentTasks: 2,
  openapiRateLimit: 120,
  jsMaxMemoryMB: 64,
  jsTimeoutMS: 5000,
  jsMaxLogBytes: 32768,
  jsMaxResultBytes: 65536,
  jsMaxHTTPRequests: 20,
  jsMaxHTTPResponseBytes: 2097152,
  jsMaxDBRows: 1000,
  jsMaxConcurrency: 2,
})

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return workspace.projects.filter((p) => {
    if (statusFilter.value && p.status !== statusFilter.value) return false
    if (!k) return true
    return p.name.toLowerCase().includes(k) || p.code.toLowerCase().includes(k)
  })
})

const stats = computed(() => ({
  total: workspace.projects.length,
  active: workspace.projects.filter((p) => p.status === 'ACTIVE').length,
  audits: workspace.projects.reduce((sum, p) => sum + p.audit_count, 0),
  today: workspace.projects.reduce((sum, p) => sum + p.today_tasks, 0),
}))
const canCreateProject = computed(() => auth.user?.role === 'SUPER_ADMIN')
const canUpdateProject = computed(() => auth.can('project:update'))

function openCreate() {
  if (!canCreateProject.value) return
  editing.value = null
  Object.assign(form, {
    name: '',
    code: '',
    env: 'dev',
    description: '',
    retentionDays: 30,
    maxTasksPerDay: 0,
    maxConcurrentTasks: 2,
    openapiRateLimit: 120,
    jsMaxMemoryMB: 64,
    jsTimeoutMS: 5000,
    jsMaxLogBytes: 32768,
    jsMaxResultBytes: 65536,
    jsMaxHTTPRequests: 20,
    jsMaxHTTPResponseBytes: 2097152,
    jsMaxDBRows: 1000,
    jsMaxConcurrency: 2,
  })
  errorText.value = ''
  modalShow.value = true
}

function openEdit(project: Project) {
  editing.value = project
  Object.assign(form, {
    name: project.name,
    code: project.code,
    env: project.env,
    description: project.description || '',
    retentionDays: project.retention_days || 30,
    maxTasksPerDay: project.max_tasks_per_day || 0,
    maxConcurrentTasks: project.max_concurrent_tasks ?? 2,
    openapiRateLimit: project.openapi_rate_limit ?? 120,
    jsMaxMemoryMB: project.js_max_memory_mb ?? 64,
    jsTimeoutMS: project.js_timeout_ms ?? 5000,
    jsMaxLogBytes: project.js_max_log_bytes ?? 32768,
    jsMaxResultBytes: project.js_max_result_bytes ?? 65536,
    jsMaxHTTPRequests: project.js_max_http_requests ?? 20,
    jsMaxHTTPResponseBytes: project.js_max_http_response_bytes ?? 2097152,
    jsMaxDBRows: project.js_max_db_rows ?? 1000,
    jsMaxConcurrency: project.js_max_concurrency ?? 2,
  })
  errorText.value = ''
  modalShow.value = true
}

async function load() {
  loading.value = true
  try {
    await workspace.loadProjects(true)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载项目失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  errorText.value = ''
  if (!form.name.trim()) {
    errorText.value = '请填写项目名称'
    return
  }
  if (!editing.value && !/^[a-z][a-z0-9_-]{2,31}$/.test(form.code)) {
    errorText.value = '项目编码需以小写字母开头，仅含小写字母、数字、下划线或连字符，长度 3~32'
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await projectApi.update(editing.value.id, {
        name: form.name.trim(),
        env: form.env,
        description: form.description,
        retention_days: form.retentionDays,
        max_tasks_per_day: form.maxTasksPerDay,
        max_concurrent_tasks: form.maxConcurrentTasks,
        openapi_rate_limit: form.openapiRateLimit,
        js_max_memory_mb: form.jsMaxMemoryMB,
        js_timeout_ms: form.jsTimeoutMS,
        js_max_log_bytes: form.jsMaxLogBytes,
        js_max_result_bytes: form.jsMaxResultBytes,
        js_max_http_requests: form.jsMaxHTTPRequests,
        js_max_http_response_bytes: form.jsMaxHTTPResponseBytes,
        js_max_db_rows: form.jsMaxDBRows,
        js_max_concurrency: form.jsMaxConcurrency,
      })
      message.success('项目已更新')
    } else {
      const created = await projectApi.create({
        name: form.name.trim(),
        code: form.code,
        env: form.env,
        description: form.description,
        retention_days: form.retentionDays,
        max_tasks_per_day: form.maxTasksPerDay,
        max_concurrent_tasks: form.maxConcurrentTasks,
        openapi_rate_limit: form.openapiRateLimit,
        js_max_memory_mb: form.jsMaxMemoryMB,
        js_timeout_ms: form.jsTimeoutMS,
        js_max_log_bytes: form.jsMaxLogBytes,
        js_max_result_bytes: form.jsMaxResultBytes,
        js_max_http_requests: form.jsMaxHTTPRequests,
        js_max_http_response_bytes: form.jsMaxHTTPResponseBytes,
        js_max_db_rows: form.jsMaxDBRows,
        js_max_concurrency: form.jsMaxConcurrency,
      })
      message.success('项目创建成功')
      workspace.setCurrentProject(created.id)
    }
    modalShow.value = false
    await load()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function selectProject(project: Project) {
  workspace.setCurrentProject(project.id)
  message.success(`已切换到「${project.name}」`)
}

function goCredentials(project: Project) {
  workspace.setCurrentProject(project.id)
  router.push({ name: 'credentials' })
}

function openSecrets(project: Project) {
  secretsProject.value = project
  secretsShow.value = true
}

async function setStatus(project: Project, status: string) {
  try {
    await projectApi.setStatus(project.id, status)
    message.success(status === 'ACTIVE' ? '项目已恢复运行' : '项目已暂停')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
  }
}

function remove(project: Project) {
  dialog.warning({
    title: '将项目移入回收站',
    content: `「${project.name}」下的审核配置会一并软删除；项目将停止接收任务，可在回收站恢复。确认继续？`,
    positiveText: '移入回收站',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await projectApi.remove(project.id)
        message.success('项目已移入回收站')
        await load()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      }
    },
  })
}

watch(
  () => route.query.create,
  (value) => {
    if (value === '1') {
      openCreate()
      router.replace({ name: 'projects' })
    }
  },
  { immediate: true },
)

onMounted(load)
</script>

<template>
  <div class="page projects-page">
    <PageHeader
      eyebrow="PROJECTS"
      title="项目空间"
      subtitle="项目是配置、模型、审核任务与数据权限的隔离边界，每个项目拥有独立的运行上下文。"
    >
      <template #actions>
        <n-button quaternary circle :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
        </n-button>
        <n-button v-if="auth.user?.role === 'SUPER_ADMIN'" type="primary" @click="openCreate">
          <template #icon><AddOutline /></template>
          新建项目
        </n-button>
      </template>
    </PageHeader>

    <div class="project-summary">
      <div class="summary-item">
        <span class="summary-icon blue"><FolderOpenOutline /></span>
        <div><strong>{{ stats.total }}</strong><span>项目总数</span></div>
      </div>
      <div class="summary-item">
        <span class="summary-icon green"><CheckmarkCircleOutline /></span>
        <div><strong>{{ stats.active }}</strong><span>运行中</span></div>
      </div>
      <div class="summary-item">
        <span class="summary-icon purple"><ServerOutline /></span>
        <div><strong>{{ stats.audits }}</strong><span>审核配置</span></div>
      </div>
      <div class="summary-item">
        <span class="summary-icon amber"><DocumentTextOutline /></span>
        <div><strong>{{ stats.today }}</strong><span>今日任务</span></div>
      </div>
    </div>

    <div class="project-toolbar">
      <n-input v-model:value="keyword" placeholder="搜索项目名称或编码" clearable class="search-input">
        <template #prefix><SearchOutline /></template>
      </n-input>
      <n-select
        v-model:value="statusFilter"
        :options="[
          { label: '运行中', value: 'ACTIVE' },
          { label: '已暂停', value: 'PAUSED' },
          { label: '已停用', value: 'DISABLED' },
        ]"
        clearable
        placeholder="状态"
        style="width: 150px"
      />
      <span class="result-count">共 {{ filtered.length }} 个项目</span>
    </div>

    <div v-if="filtered.length" class="project-grid fade-list">
      <article
        v-for="project in filtered"
        :key="project.id"
        class="project-card"
        :class="{ current: project.id === workspace.currentProjectId, paused: project.status !== 'ACTIVE' }"
      >
        <div class="project-card-head">
          <span class="project-logo"><FolderOpenOutline /></span>
          <div class="project-title">
            <div class="title-row">
              <h3>{{ project.name }}</h3>
              <StatusTag :status="project.env" type="env" />
              <StatusTag :status="project.status" type="project" />
            </div>
            <span class="project-code mono">{{ project.code }}</span>
          </div>
          <button v-if="canUpdateProject" class="card-menu" type="button" @click="openEdit(project)">
            <EllipsisHorizontalOutline />
          </button>
        </div>

        <p class="project-desc">{{ project.description || '暂无项目说明。' }}</p>

        <div class="project-stats">
          <div><span>审核配置</span><strong>{{ project.audit_count }}</strong></div>
          <div><span>模型接入</span><strong>{{ project.provider_count }}</strong></div>
          <div><span>今日任务</span><strong>{{ project.today_tasks }}</strong></div>
          <div><span>保留期</span><strong>{{ project.retention_days }}<i>天</i></strong></div>
        </div>

        <div class="project-foot">
          <span class="project-created">创建于 {{ project.created_at?.slice(0, 10) }}</span>
          <div class="project-actions">
            <n-button v-if="canUpdateProject" size="tiny" secondary @click="openEdit(project)">项目设置</n-button>
            <template v-if="project.id === workspace.currentProjectId">
              <span class="current-badge"><CheckmarkCircleOutline /> 当前项目</span>
            </template>
            <template v-else>
              <n-button size="tiny" type="primary" secondary @click="selectProject(project)">
                切换 <ArrowForwardOutline class="arrow" />
              </n-button>
            </template>
            <n-button size="tiny" quaternary title="项目报表" @click="router.push({ name: 'analytics', query: { project_id: String(project.id) } })">
              <template #icon><BarChartOutline /></template>
            </n-button>
            <n-button size="tiny" quaternary title="项目接入凭证" @click="goCredentials(project)">
              <template #icon><KeyOutline /></template>
            </n-button>
            <n-button v-if="canUpdateProject" size="tiny" quaternary title="项目密钥" @click="openSecrets(project)">
              <template #icon><LockClosedOutline /></template>
              密钥
            </n-button>
            <n-button v-if="canUpdateProject && project.status === 'ACTIVE'" size="tiny" quaternary @click="setStatus(project, 'PAUSED')">
              <template #icon><PauseOutline /></template>
            </n-button>
            <n-button v-else-if="canUpdateProject" size="tiny" quaternary @click="setStatus(project, 'ACTIVE')">
              <template #icon><PlayOutline /></template>
            </n-button>
            <n-button v-if="auth.user?.role === 'SUPER_ADMIN'" size="tiny" quaternary class="danger" @click="remove(project)">
              <template #icon><TrashOutline /></template>
            </n-button>
          </div>
        </div>
      </article>

      <button v-if="auth.user?.role === 'SUPER_ADMIN'" class="project-add" type="button" @click="openCreate">
        <span><AddOutline /></span>
        <strong>创建新项目</strong>
        <small>隔离一套独立的审核与模型配置</small>
      </button>
    </div>

    <EmptyState
      v-else
      title="没有匹配的项目"
      :description="canCreateProject ? '调整筛选条件，或创建一个新的项目空间。' : '调整筛选条件，或联系超级管理员为你授权项目。'"
      :action="canCreateProject ? '新建项目' : ''"
      @action="openCreate"
    />

    <n-modal v-model:show="modalShow" :mask-closable="!saving">
      <div class="project-modal">
        <div class="modal-head">
          <div>
            <div class="modal-kicker">{{ editing ? 'EDIT PROJECT' : 'NEW PROJECT' }}</div>
            <h2>{{ editing ? '编辑项目' : '新建项目空间' }}</h2>
            <p>项目编码创建后不可修改，并会作为配置与队列的隔离前缀。</p>
          </div>
          <span class="modal-icon"><FolderOpenOutline /></span>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>项目名称 <span class="required">*</span></label>
            <n-input v-model:value="form.name" placeholder="例如 建设工程档案审核" maxlength="64" show-count />
          </div>
          <div class="two-col">
            <div class="field">
              <label>项目编码 <span v-if="!editing" class="required">*</span></label>
              <n-input v-model:value="form.code" :disabled="Boolean(editing)" placeholder="archive-prod" class="mono-input" />
              <div class="form-hint">小写字母开头，仅含字母、数字、_ 或 -，3~32 位。</div>
            </div>
            <div class="field">
              <label>环境</label>
              <n-select
                v-model:value="form.env"
                :options="[
                  { label: '开发 dev', value: 'dev' },
                  { label: '测试 test', value: 'test' },
                  { label: '预发 staging', value: 'staging' },
                  { label: '生产 prod', value: 'prod' },
                ]"
              />
            </div>
          </div>
          <div class="field">
            <label>项目说明</label>
            <n-input
              v-model:value="form.description"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 5 }"
              placeholder="描述该项目的审核场景、业务边界与负责人信息"
              maxlength="255"
              show-count
            />
          </div>
          <div class="two-col">
            <div class="field">
              <label>回收站保留天数</label>
              <n-input-number v-model:value="form.retentionDays" :min="7" :max="365" style="width: 100%" />
              <div class="form-hint">删除后的项目与审核配置将在回收站保留该天数。</div>
            </div>
            <div class="field">
              <label>每日任务配额</label>
              <n-input-number v-model:value="form.maxTasksPerDay" :min="0" :max="100000" style="width: 100%" />
              <div class="form-hint">0 表示不限制；所有触发来源合计计算。</div>
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>项目并发 Worker 上限</label>
              <n-input-number v-model:value="form.maxConcurrentTasks" :min="1" :max="32" style="width: 100%" />
              <div class="form-hint">该项目同时执行的任务数，受全局 Worker 总数约束；其他项目仍可并行。</div>
            </div>
            <div class="field">
              <label>开放 API 限流（次/分钟）</label>
              <n-input-number v-model:value="form.openapiRateLimit" :min="0" :max="10000" style="width: 100%" />
              <div class="form-hint">0 表示不限制；仅限制开放 API 提交。</div>
            </div>
          </div>
          <div class="form-section-title">JS 脚本动作限制</div>
          <div class="two-col">
            <div class="field">
              <label>单次最大内存（MB）</label>
              <n-input-number v-model:value="form.jsMaxMemoryMB" :min="8" :max="512" style="width: 100%" />
              <div class="form-hint">只限制 JS 分配；DB/HTTP 响应另有字节上限。</div>
            </div>
            <div class="field">
              <label>单次最大时长（ms）</label>
              <n-input-number v-model:value="form.jsTimeoutMS" :min="500" :max="60000" :step="500" style="width: 100%" />
              <div class="form-hint">超时强制中断，动作按失败处理。</div>
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>单次最大日志（字节）</label>
              <n-input-number v-model:value="form.jsMaxLogBytes" :min="1024" :max="1048576" :step="1024" style="width: 100%" />
            </div>
            <div class="field">
              <label>单次最大返回值（字节）</label>
              <n-input-number v-model:value="form.jsMaxResultBytes" :min="1024" :max="1048576" :step="1024" style="width: 100%" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>单次最大 HTTP 请求数</label>
              <n-input-number v-model:value="form.jsMaxHTTPRequests" :min="0" :max="100" style="width: 100%" />
            </div>
            <div class="field">
              <label>单次最大 HTTP 响应（字节）</label>
              <n-input-number v-model:value="form.jsMaxHTTPResponseBytes" :min="1024" :max="16777216" :step="1024" style="width: 100%" />
            </div>
          </div>
          <div class="two-col">
            <div class="field">
              <label>单次最大 DB 行数</label>
              <n-input-number v-model:value="form.jsMaxDBRows" :min="1" :max="10000" style="width: 100%" />
            </div>
            <div class="field">
              <label>项目并发脚本数</label>
              <n-input-number v-model:value="form.jsMaxConcurrency" :min="1" :max="16" style="width: 100%" />
            </div>
          </div>
          <div v-if="form.env === 'prod'" class="prod-warning">
            <span></span>
            当前为生产环境，项目内的配置变更将影响线上审核流程，建议启用二次确认与灰度发布。
          </div>
          <div v-if="errorText" class="modal-error">{{ errorText }}</div>
        </div>
        <div class="modal-footer">
          <n-button :disabled="saving" @click="modalShow = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">{{ editing ? '保存修改' : '创建项目' }}</n-button>
        </div>
      </div>
    </n-modal>

    <ProjectSecretsModal
      v-model:show="secretsShow"
      :project-id="secretsProject?.id || 0"
      :project-name="secretsProject?.name || ''"
    />
  </div>
</template>

<style scoped>
.project-summary {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 18px;
}

.summary-item {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 14px 17px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: #fff;
  box-shadow: var(--shadow-sm);
}

.summary-icon {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: var(--brand-soft);
  color: var(--brand);
}

.summary-icon.green {
  background: var(--green-soft);
  color: var(--green);
}
.summary-icon.purple {
  background: var(--purple-soft);
  color: var(--purple);
}
.summary-icon.amber {
  background: var(--amber-soft);
  color: #d97706;
}

.summary-icon :deep(svg) {
  width: 16px;
  height: 16px;
}

.summary-item strong {
  display: block;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 19px;
  font-weight: 650;
  line-height: 1.2;
}

.summary-item span {
  color: var(--muted-light);
  font-size: 10.5px;
}

.project-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}

.search-input {
  width: min(310px, 100%);
}

.result-count {
  margin-left: auto;
  color: var(--muted-light);
  font-size: 11px;
}

.project-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.project-card {
  display: flex;
  flex-direction: column;
  padding: 20px;
  border: 1px solid var(--border);
  border-radius: 17px;
  background: #fff;
  box-shadow: var(--shadow-sm);
  transition: all 0.18s ease;
}

.project-card:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow-md);
}

.project-card.current {
  border-color: #aac6fb;
  box-shadow: 0 0 0 3px rgba(43, 108, 246, 0.06), var(--shadow-sm);
}

.project-card.paused {
  background: #fffdf9;
}

.project-card-head {
  display: flex;
  align-items: flex-start;
  gap: 11px;
}

.project-logo {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: linear-gradient(145deg, #edf4ff, #e2ebff);
  color: var(--brand);
  flex-shrink: 0;
}

.project-logo :deep(svg) {
  width: 19px;
  height: 19px;
}

.project-title {
  min-width: 0;
  flex: 1;
}

.title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
}

.title-row h3 {
  overflow: hidden;
  margin: 0;
  color: var(--ink);
  font-size: 14.5px;
  font-weight: 620;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-code {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9.5px;
}

.card-menu {
  display: flex;
  width: 28px;
  height: 28px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--muted-light);
  cursor: pointer;
}

.card-menu:hover {
  background: #f4f7fb;
  color: var(--brand);
}

.card-menu :deep(svg) {
  width: 17px;
  height: 17px;
}

.project-desc {
  display: -webkit-box;
  min-height: 38px;
  margin: 13px 0 14px;
  overflow: hidden;
  color: var(--muted);
  font-size: 11.5px;
  line-height: 1.65;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.project-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-bottom: 15px;
  padding: 12px 14px;
  border-radius: 11px;
  background: #fbfcfe;
  border: 1px solid #f0f3f8;
}

.project-stats span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
}

.project-stats strong {
  display: block;
  margin-top: 1px;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 14px;
  font-weight: 620;
}

.project-stats i {
  margin-left: 1px;
  color: var(--muted-light);
  font-size: 8px;
  font-style: normal;
}

.project-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: auto;
  padding-top: 13px;
  border-top: 1px dashed #edf1f7;
}

.project-created {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 9px;
}

.project-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.current-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-right: 2px;
  color: #15803d;
  font-size: 9.5px;
  font-weight: 550;
}

.current-badge :deep(svg) {
  width: 12px;
  height: 12px;
}

.arrow {
  width: 10px;
  height: 10px;
}

.project-actions .danger {
  color: #b0bac8;
}

.project-actions .danger:hover {
  color: var(--red);
}

.project-add {
  display: flex;
  min-height: 260px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 5px;
  border: 1px dashed #d5ddea;
  border-radius: 17px;
  background: rgba(255, 255, 255, 0.5);
  color: var(--muted);
  cursor: pointer;
  transition: all 0.18s ease;
}

.project-add:hover {
  border-color: #aac4f8;
  background: #f9fbff;
  color: var(--brand);
}

.project-add > span {
  display: flex;
  width: 42px;
  height: 42px;
  align-items: center;
  justify-content: center;
  margin-bottom: 5px;
  border-radius: 50%;
  background: #f1f5fb;
  color: var(--muted);
}

.project-add:hover > span {
  background: var(--brand-soft);
  color: var(--brand);
}

.project-add > span :deep(svg) {
  width: 18px;
  height: 18px;
}

.project-add strong {
  font-size: 12.5px;
  font-weight: 550;
}

.project-add small {
  color: var(--muted-light);
  font-size: 9.5px;
}

.project-modal {
  display: flex;
  width: min(540px, calc(100vw - 32px));
  max-height: calc(100vh - 32px);
  flex-direction: column;
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.modal-head {
  display: flex;
  flex-shrink: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  padding: 23px 26px 17px;
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
  flex-shrink: 0;
}

.modal-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.modal-body {
  min-height: 0;
  overflow-y: auto;
  padding: 20px 26px;
}

.field {
  margin-bottom: 17px;
}

.field label {
  display: block;
  margin-bottom: 6px;
  color: var(--text);
  font-size: 12px;
  font-weight: 550;
}

.required {
  color: var(--red);
}

.two-col {
  display: grid;
  grid-template-columns: 1.2fr 0.8fr;
  gap: 14px;
}

.mono-input :deep(input) {
  font-family: var(--mono);
  font-size: 12px;
}

.prod-warning {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  padding: 10px 12px;
  border: 1px solid #fde8b6;
  border-radius: 9px;
  background: #fffbeb;
  color: #a16207;
  font-size: 10.5px;
  line-height: 1.6;
}

.prod-warning span {
  width: 5px;
  height: 5px;
  margin-top: 6px;
  border-radius: 50%;
  background: #eab308;
  flex-shrink: 0;
}

.modal-error {
  margin-top: 13px;
  padding: 9px 11px;
  border: 1px solid #fee2e2;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 11px;
}

.modal-footer {
  display: flex;
  flex-shrink: 0;
  justify-content: flex-end;
  gap: 8px;
  padding: 15px 26px;
  border-top: 1px solid #f0f3f8;
}


@media (max-width: 1000px) {
  .project-summary {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 720px) {
  .project-grid {
    grid-template-columns: 1fr;
  }

  .project-toolbar .result-count {
    display: none;
  }

  .project-actions {
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}

.form-section-title {
  margin: 18px 0 10px;
  padding-top: 14px;
  border-top: 1px solid #eef2f8;
  color: var(--ink);
  font-size: 11px;
  font-weight: 600;
}

@media (max-width: 520px) {
  .project-summary {
    grid-template-columns: 1fr 1fr;
  }

  .two-col {
    grid-template-columns: 1fr;
  }
}
</style>
