<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import {
  LinkOutline,
  LockClosedOutline,
  RefreshOutline,
  ServerOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import { systemConfigApi } from '@/api'
import type { SystemConfigData, SystemConfigGroup } from '@/api/types'

const message = useMessage()
const loading = ref(false)
const data = ref<SystemConfigData | null>(null)
const errorText = ref('')

const groupIcons: Record<string, unknown> = {
  runtime: SettingsOutline,
  mysql: ServerOutline,
  redis: LinkOutline,
  security: ShieldCheckmarkOutline,
  ai: SparklesOutline,
}

const runtimeFields = computed(() => data.value?.groups.find((group) => group.key === 'runtime')?.fields || [])

function runtimeValue(key: string) {
  return runtimeFields.value.find((field) => field.key === key)?.value || '—'
}

function groupIcon(group: SystemConfigGroup) {
  return groupIcons[group.key] || SettingsOutline
}

async function load() {
  loading.value = true
  errorText.value = ''
  try {
    data.value = await systemConfigApi.get()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '加载系统配置失败'
    message.error(errorText.value)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page system-config-page">
    <PageHeader
      eyebrow="SYSTEM CONFIG"
      title="系统配置"
      subtitle="只读查看当前进程启动时加载的配置；密码和密钥字段已脱敏。"
    >
      <template #actions>
        <n-button secondary :loading="loading" @click="load">
          <template #icon><RefreshOutline /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <div class="config-banner">
      <span class="banner-icon"><LockClosedOutline /></span>
      <div class="banner-copy">
        <strong>只读配置视图</strong>
        <p>配置来自进程启动时解析的 .env / config.yml；修改后需要重启服务才会生效。</p>
        <p v-if="data" class="config-source">
          环境文件：<code>{{ data.env_file || '未使用' }}</code>
          <span class="source-sep">·</span>
          YAML：<code>{{ data.config_file || '未使用' }}</code>
        </p>
      </div>
      <span class="banner-badge">READ ONLY</span>
    </div>

    <div v-if="loading" class="config-loading">正在读取配置…</div>
    <div v-else-if="errorText" class="config-empty">{{ errorText }}</div>
    <template v-else-if="data">
      <div class="config-summary">
        <div class="summary-card">
          <span>运行环境</span>
          <strong>{{ runtimeValue('APP_ENV') }}</strong>
        </div>
        <div class="summary-card">
          <span>版本</span>
          <strong class="mono">v{{ data.version }}</strong>
        </div>
        <div class="summary-card">
          <span>服务端口</span>
          <strong class="mono">{{ runtimeValue('SERVER_PORT') }}</strong>
        </div>
        <div class="summary-card">
          <span>审核 Worker</span>
          <strong class="mono">{{ runtimeValue('WORKER_COUNT') }}</strong>
        </div>
      </div>

      <div class="config-grid">
        <section v-for="group in data.groups" :key="group.key" class="config-card">
          <div class="config-card-head">
            <span class="config-icon">
              <component :is="groupIcon(group)" />
            </span>
            <div>
              <h3>{{ group.title }}</h3>
              <p>{{ group.description }}</p>
            </div>
          </div>
          <div class="config-fields">
            <div v-for="field in group.fields" :key="field.key" class="config-field">
              <span class="config-label">{{ field.label }}</span>
              <div class="config-value">
                <code v-if="!field.sensitive">{{ field.value }}</code>
                <span v-else class="secret-value">
                  <LockClosedOutline />
                  {{ field.value }}
                </span>
                <span
                  v-if="field.sensitive && field.configured"
                  class="secret-badge configured"
                >已脱敏</span>
              </div>
            </div>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.config-banner {
  display: flex;
  align-items: flex-start;
  gap: 13px;
  margin-bottom: 16px;
  padding: 15px 17px;
  border: 1px solid var(--brand-border);
  border-radius: 14px;
  background: linear-gradient(130deg, var(--brand-softer), var(--card));
}

.banner-icon {
  display: flex;
  width: 36px;
  height: 36px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.banner-icon :deep(svg) {
  width: 17px;
  height: 17px;
}

.banner-copy {
  min-width: 0;
  flex: 1;
}

.banner-copy strong {
  display: block;
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 620;
}

.banner-copy p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.6;
}

.config-source {
  color: var(--muted-light) !important;
}

.config-source code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--text);
  font-family: var(--mono);
  font-size: 9.5px;
}

.source-sep {
  margin: 0 5px;
  color: var(--muted-light);
}

.banner-badge {
  padding: 3px 8px;
  border-radius: 99px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-family: var(--mono);
  font-size: 8px;
  font-weight: 650;
  letter-spacing: 0.08em;
  white-space: nowrap;
  flex: none;
}

.config-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}

.summary-card {
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
}

.summary-card span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
  letter-spacing: 0.04em;
}

.summary-card strong {
  display: block;
  overflow: hidden;
  margin-top: 3px;
  color: var(--ink);
  font-size: 15px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.summary-card .mono {
  font-family: var(--mono);
  font-size: 13px;
}

.config-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  align-items: start;
  gap: 14px;
}

.config-card {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: var(--card);
  box-shadow: var(--shadow-sm);
}

.config-card-head {
  display: flex;
  align-items: flex-start;
  gap: 11px;
  padding: 16px 17px 13px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-soft);
}

.config-icon {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.config-icon :deep(svg) {
  width: 16px;
  height: 16px;
}

.config-card-head h3 {
  margin: 1px 0 2px;
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 620;
}

.config-card-head p {
  margin: 0;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.5;
}

.config-fields {
  padding: 2px 17px 8px;
}

.config-field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  min-height: 38px;
  padding: 7px 0;
  border-bottom: 1px dashed var(--border);
}

.config-field:last-child {
  border-bottom: 0;
}

.config-label {
  color: var(--muted);
  font-size: 10.5px;
  flex: none;
}

.config-value {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.config-value code {
  overflow: hidden;
  color: var(--text);
  font-family: var(--mono);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 280px;
}

.secret-value {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  overflow: hidden;
  color: var(--text);
  font-family: var(--mono);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.secret-value :deep(svg) {
  width: 11px;
  height: 11px;
  color: var(--muted-light);
  flex: none;
}

.secret-badge {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-strong);
  color: var(--muted-light);
  font-size: 8px;
  flex: none;
}

.secret-badge.configured {
  background: var(--green-soft);
  color: var(--green);
}

.config-loading,
.config-empty {
  padding: 60px 0;
  color: var(--muted-light);
  font-size: 11px;
  text-align: center;
}

@media (max-width: 720px) {
  .config-grid {
    grid-template-columns: 1fr;
  }

  .config-field {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }

  .config-value {
    width: 100%;
    justify-content: flex-start;
  }

  .config-value code {
    max-width: 100%;
  }
}
</style>
