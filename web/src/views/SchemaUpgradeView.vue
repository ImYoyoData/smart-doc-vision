<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCheckbox, NModal, NSpin, NTag } from 'naive-ui'
import {
  AlertCircleOutline,
  LogOutOutline,
  RefreshOutline,
  ShieldCheckmarkOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import { schemaApi } from '@/api/schema'
import { useAuthStore } from '@/stores/auth'
import { useSchemaStore } from '@/stores/schema'

const auth = useAuthStore()
const schema = useSchemaStore()
const errorText = ref('')
const showConfirm = ref(false)
const acknowledged = ref(false)
const applying = ref(false)

const status = computed(() => schema.status)
const report = computed(() => status.value?.report || null)
const isSuperAdmin = computed(() => auth.user?.role === 'SUPER_ADMIN')
const canApply = computed(
  () => Boolean(status.value?.can_apply) && isSuperAdmin.value && !report.value?.missing_core,
)
const missingTables = computed(() => report.value?.missing_tables || [])
const addedColumns = computed(() => report.value?.added_columns || [])
const modifiedColumns = computed(() => report.value?.modified_columns || [])
const addedIndexes = computed(() => report.value?.added_indexes || [])
const extraTables = computed(() => report.value?.extra_tables || [])
const extraColumns = computed(() => report.value?.extra_columns || [])
const warnings = computed(() => report.value?.warnings || [])

function reportColumn(change: { table: string; column: string; expected?: string }) {
  return `${change.table}.${change.column}${change.expected ? ` → ${change.expected}` : ''}`
}

function reportModified(change: { table: string; column: string; expected?: string; actual?: string; reason?: string }) {
  return `${change.table}.${change.column}：${change.actual || '—'} → ${change.expected || '—'}${change.reason ? `（${change.reason}）` : ''}`
}

async function load() {
  errorText.value = ''
  await schema.load(true)
  if (!status.value) {
    errorText.value = '无法读取数据库结构状态，请刷新页面或查看服务日志'
  }
}

function openConfirm() {
  acknowledged.value = false
  showConfirm.value = true
}

async function applyUpgrade() {
  if (!acknowledged.value || applying.value) return
  applying.value = true
  errorText.value = ''
  try {
    await schemaApi.apply()
    window.location.reload()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '兼容升级失败'
    showConfirm.value = false
  } finally {
    applying.value = false
  }
}

async function logout() {
  await auth.logout()
  window.location.href = `${import.meta.env.BASE_URL}login`
}

onMounted(load)
</script>

<template>
  <div class="schema-page">
    <div class="schema-shell">
      <header class="schema-head">
        <div class="brand-mark">智</div>
        <div>
          <div class="eyebrow">DATABASE COMPATIBILITY</div>
          <h1>数据库兼容升级</h1>
          <p>当前程序的表结构与数据库不一致，业务功能已暂停，仅允许超级管理员确认兼容操作。</p>
        </div>
        <n-button secondary @click="logout">
          <template #icon><LogOutOutline /></template>
          退出登录
        </n-button>
      </header>

      <div v-if="schema.loading" class="state-card">
        <n-spin size="small" />
        <span>正在读取数据库结构…</span>
      </div>

      <div v-else-if="errorText && !status" class="state-card error">
        <AlertCircleOutline class="state-icon" />
        <div>
          <strong>{{ errorText }}</strong>
          <p>请确认数据库可连接，或重启服务后重试。</p>
        </div>
        <n-button secondary @click="load">
          <template #icon><RefreshOutline /></template>
          重试
        </n-button>
      </div>

      <template v-else-if="status && report">
        <section class="schema-card">
          <div class="schema-alert" :class="{ danger: report.missing_core }">
            <AlertCircleOutline class="schema-alert-icon" />
            <div>
              <strong v-if="report.missing_core">核心用户表或超级管理员账号缺失</strong>
              <strong v-else>检测到数据库结构差异</strong>
              <p v-if="report.missing_core">
                当前无法通过网页登录进行在线修复。请恢复数据库备份，或删除安装标记后重新安装。
              </p>
              <p v-else>
                确认后系统会新增缺失的表、字段和索引，并按当前模型调整部分列定义。修复不会主动删除数据。
              </p>
            </div>
            <n-tag size="small" :type="report.missing_core ? 'error' : 'warning'">
              {{ report.missing_core ? 'BLOCKED' : 'UPGRADE REQUIRED' }}
            </n-tag>
          </div>

          <div class="version-grid">
            <div><span>程序结构版本</span><strong>{{ report.schema_version }}</strong></div>
            <div><span>数据库记录版本</span><strong>{{ report.database_version || '未记录' }}</strong></div>
            <div><span>已有业务表</span><strong>{{ report.existing_tables?.length || 0 }}</strong></div>
            <div><span>超级管理员</span><strong>{{ report.has_super_admin ? '可用' : '不可用' }}</strong></div>
          </div>

          <n-alert v-if="!isSuperAdmin" type="warning" :bordered="false">
            当前账号不是超级管理员，只能查看升级信息，不能执行兼容升级。
          </n-alert>

          <div class="diff-grid">
            <section v-if="missingTables.length">
              <h3>将新建表</h3>
              <div class="chips">
                <code v-for="table in missingTables" :key="table">{{ table }}</code>
              </div>
            </section>
            <section v-if="addedColumns.length">
              <h3>将新增字段</h3>
              <ul>
                <li v-for="change in addedColumns" :key="`${change.table}.${change.column}`">
                  {{ reportColumn(change) }}
                </li>
              </ul>
            </section>
            <section v-if="modifiedColumns.length">
              <h3>将调整字段（可能影响数据）</h3>
              <ul class="danger-list">
                <li v-for="change in modifiedColumns" :key="`${change.table}.${change.column}`">
                  {{ reportModified(change) }}
                </li>
              </ul>
            </section>
            <section v-if="addedIndexes.length">
              <h3>将新增索引</h3>
              <ul>
                <li v-for="index in addedIndexes" :key="`${index.table}.${index.name}`">
                  {{ index.table }} · {{ index.name }}<span v-if="index.columns">（{{ index.columns }}）</span>
                </li>
              </ul>
            </section>
            <section v-if="extraTables.length || extraColumns.length">
              <h3>模型之外，保留不删除</h3>
              <div class="chips muted">
                <code v-for="table in extraTables" :key="table">{{ table }}</code>
                <code v-for="change in extraColumns" :key="`${change.table}.${change.column}`">
                  {{ change.table }}.{{ change.column }}
                </code>
              </div>
            </section>
          </div>

          <div v-if="warnings.length" class="warning-box">
            <p v-for="warning in warnings" :key="warning">
              <WarningOutline />
              {{ warning }}
            </p>
          </div>

          <div class="effect-note">
            <strong>数据影响</strong>
            <p>兼容升级使用非破坏性迁移：只新增表、字段和索引，并按模型调整列类型/可空性，不删除模型之外的列或表。</p>
            <p>如果“将调整字段”非空，请先备份数据库；极端情况下列类型调整可能截断超长数据。</p>
          </div>

          <n-alert v-if="errorText" type="error" :bordered="false">{{ errorText }}</n-alert>

          <div class="schema-actions">
            <n-button secondary @click="load">
              <template #icon><RefreshOutline /></template>
              刷新状态
            </n-button>
            <n-button type="primary" :disabled="!canApply" @click="openConfirm">
              <template #icon><ShieldCheckmarkOutline /></template>
              确认兼容并升级
            </n-button>
          </div>
        </section>
      </template>

      <n-modal
        v-model:show="showConfirm"
        preset="card"
        :mask-closable="false"
        style="width: min(560px, 92vw)"
        title="再次确认：执行数据库兼容升级"
      >
        <div class="confirm-body">
          <n-alert type="warning" :bordered="false">
            系统将修改数据库结构并重新启动业务服务。请确认已完成数据库备份。
          </n-alert>
          <ul class="confirm-summary">
            <li>新建表 {{ missingTables.length }} 个</li>
            <li>新增字段 {{ addedColumns.length }} 个</li>
            <li>调整字段 {{ modifiedColumns.length }} 个</li>
            <li>新增索引 {{ addedIndexes.length }} 个</li>
          </ul>
          <n-checkbox v-model:checked="acknowledged">
            我已备份数据库，并确认执行兼容升级
          </n-checkbox>
          <div class="confirm-actions">
            <n-button secondary :disabled="applying" @click="showConfirm = false">取消</n-button>
            <n-button
              type="primary"
              :disabled="!acknowledged || applying"
              :loading="applying"
              @click="applyUpgrade"
            >
              确认并执行
            </n-button>
          </div>
        </div>
      </n-modal>
    </div>
  </div>
</template>

<style scoped>
.schema-page {
  min-height: 100vh;
  padding: 34px 20px 60px;
  background:
    radial-gradient(circle at 12% 8%, var(--brand-soft), transparent 32%),
    radial-gradient(circle at 88% 0%, rgba(239, 68, 68, 0.08), transparent 30%),
    var(--canvas);
}

.schema-shell {
  width: min(1080px, 100%);
  margin: 0 auto;
}

.schema-head {
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

.schema-head > div:nth-child(2) {
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

.schema-head h1 {
  margin: 3px 0 2px;
  color: var(--ink);
  font-size: 22px;
  font-weight: 680;
}

.schema-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
}

.state-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 26px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--card);
  color: var(--muted);
  font-size: 12px;
}

.state-card.error {
  align-items: flex-start;
}

.state-card > div {
  flex: 1;
}

.state-card strong {
  display: block;
  color: var(--ink);
  font-size: 14px;
}

.state-card p {
  margin: 4px 0 0;
  font-size: 11px;
}

.state-icon {
  width: 26px;
  height: 26px;
  color: var(--red);
  flex: none;
}

.schema-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
}

.schema-alert {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid rgba(245, 158, 11, 0.3);
  border-radius: 12px;
  background: rgba(245, 158, 11, 0.08);
}

.schema-alert.danger {
  border-color: rgba(239, 68, 68, 0.3);
  background: var(--red-soft);
}

.schema-alert-icon {
  width: 22px;
  height: 22px;
  margin-top: 1px;
  color: #d97706;
  flex: none;
}

.schema-alert.danger .schema-alert-icon {
  color: var(--red);
}

.schema-alert > div {
  flex: 1;
}

.schema-alert strong {
  color: var(--ink);
  font-size: 13px;
}

.schema-alert p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.65;
}

.version-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
}

.version-grid > div {
  padding: 11px 13px;
  border: 1px dashed var(--border);
  border-radius: 10px;
  background: var(--surface-soft);
}

.version-grid span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
}

.version-grid strong {
  display: block;
  margin-top: 3px;
  color: var(--text);
  font-family: var(--mono);
  font-size: 12px;
}

.diff-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 14px;
}

.diff-grid section {
  padding: 13px 15px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--surface-soft);
}

.diff-grid h3 {
  margin: 0 0 8px;
  color: var(--ink);
  font-size: 11px;
  font-weight: 620;
}

.diff-grid ul {
  margin: 0;
  padding-left: 17px;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.8;
}

.danger-list {
  color: var(--red) !important;
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.chips code {
  padding: 2px 7px;
  border: 1px solid var(--border);
  border-radius: 5px;
  background: var(--card);
  color: var(--text);
  font-family: var(--mono);
  font-size: 9.5px;
}

.chips.muted code {
  color: var(--muted-light);
  text-decoration: line-through;
}

.warning-box {
  padding: 11px 14px;
  border: 1px solid rgba(239, 68, 68, 0.22);
  border-radius: 10px;
  background: var(--red-soft);
}

.warning-box p {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin: 2px 0;
  color: var(--red);
  font-size: 10px;
  line-height: 1.6;
}

.warning-box svg {
  width: 12px;
  height: 12px;
  margin-top: 2px;
  flex: none;
}

.effect-note {
  padding: 12px 15px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-soft);
}

.effect-note strong {
  color: var(--ink);
  font-size: 11px;
}

.effect-note p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 10px;
  line-height: 1.65;
}

.schema-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.confirm-body {
  display: flex;
  flex-direction: column;
  gap: 13px;
}

.confirm-summary {
  margin: 0;
  padding-left: 18px;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.8;
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 720px) {
  .schema-page {
    padding: 20px 12px 40px;
  }

  .schema-head {
    flex-wrap: wrap;
  }

  .schema-head > button {
    margin-left: auto;
  }
}
</style>
