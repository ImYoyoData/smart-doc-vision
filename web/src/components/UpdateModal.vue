<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NModal, NSwitch } from 'naive-ui'
import dayjs from 'dayjs'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import {
  ArrowUpCircleOutline,
  CheckmarkCircleOutline,
  DownloadOutline,
  LogoGithub,
  RefreshOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import { useUpdateStore } from '@/stores/update'
import type { UpdateAsset } from '@/api/types'

const update = useUpdateStore()

const assetLabels: Record<string, string> = {
  'windows-amd64': 'Windows x64',
  'linux-amd64': 'Linux x64',
  'linux-arm64': 'Linux ARM64',
}

const title = computed(() => {
  if (update.loading && !update.info) return '正在检查更新'
  if (update.error && !update.info) return '检查更新失败'
  if (update.hasUpdate) return `发现新版本 v${update.info?.latest_version || ''}`
  if (update.info) return '当前已是最新版本'
  return '检查更新'
})

const primaryAssets = computed(() =>
  (update.info?.assets || []).filter(
    (asset) => !asset.name.toLowerCase().includes('upx') && Boolean(assetLabels[asset.kind]),
  ),
)

const notesHtml = computed(() => {
  const notes = update.info?.notes?.trim()
  if (!notes) return ''
  const html = marked.parse(notes, { breaks: true, gfm: true })
  return DOMPurify.sanitize(typeof html === 'string' ? html : '')
})

const releaseURL = computed(() => {
  if (update.info?.release_url) return update.info.release_url
  return `${update.info?.repo_url || 'https://github.com/ImYoyoData/smart-doc-vision'}/releases/latest`
})

function formatTime(value?: string) {
  return value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '—'
}

function formatSize(bytes: number) {
  if (!bytes || bytes < 0) return ''
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}

function assetLabel(asset: UpdateAsset) {
  return assetLabels[asset.kind] || asset.name
}

function close() {
  update.close()
}
</script>

<template>
  <n-modal
    :show="update.showModal"
    :auto-focus="false"
    @update:show="(value) => (update.showModal = value)"
  >
    <div class="update-modal">
      <div class="update-head">
        <div class="update-head-main">
          <span class="update-icon" :class="{ ready: update.hasUpdate }">
            <ArrowUpCircleOutline v-if="update.hasUpdate" />
            <CheckmarkCircleOutline v-else-if="update.info" />
            <RefreshOutline v-else />
          </span>
          <div>
            <div class="update-kicker">RELEASE</div>
            <h2>{{ title }}</h2>
            <p v-if="update.info">
              当前 v{{ update.info.current_version }} · 检查于 {{ formatTime(update.info.checked_at) }}
            </p>
            <p v-else>从 GitHub Releases 获取最新版本信息。</p>
          </div>
        </div>
        <button type="button" class="update-close" title="关闭" aria-label="关闭" @click="close">×</button>
      </div>

      <div class="update-body">
        <div class="auto-check-row">
          <div class="auto-check-copy">
            <strong>自动检查更新</strong>
            <p>默认关闭。开启后进入后台时会静默检查 GitHub Release。</p>
          </div>
          <n-switch
            :value="update.autoCheck"
            :loading="update.savingAutoCheck"
            @update:value="(value) => update.setAutoCheck(value)"
          />
        </div>

        <div v-if="update.error" class="update-error">{{ update.error }}</div>

        <div v-if="update.loading && !update.info" class="update-loading">
          <span class="loading-dot"></span>
          正在向 GitHub 查询最新版本…
        </div>

        <template v-else-if="update.info">
          <div class="version-strip">
            <div class="version-chip">
              <span>当前版本</span>
              <strong>v{{ update.info.current_version }}</strong>
            </div>
            <span class="version-arrow">→</span>
            <div class="version-chip latest">
              <span>最新版本</span>
              <strong>v{{ update.info.latest_version }}</strong>
            </div>
            <div v-if="update.info.published_at" class="release-date">
              发布于 {{ formatTime(update.info.published_at) }}
            </div>
          </div>

          <template v-if="update.hasUpdate">
            <div v-if="notesHtml" class="release-notes markdown-body" v-html="notesHtml"></div>
            <p v-else class="update-copy">该版本没有附带更新说明，可前往 GitHub Release 查看详情。</p>

            <div v-if="primaryAssets.length" class="download-grid">
              <a
                v-for="asset in primaryAssets"
                :key="asset.name"
                class="download-card"
                :href="asset.url"
                target="_blank"
                rel="noopener noreferrer"
              >
                <DownloadOutline />
                <span>
                  <strong>{{ assetLabel(asset) }}</strong>
                  <small>{{ asset.name }}</small>
                </span>
                <em v-if="formatSize(asset.size)">{{ formatSize(asset.size) }}</em>
              </a>
            </div>

            <div v-if="update.info.checksums_url" class="checksum-line">
              <ShieldCheckmarkOutline />
              <span>下载后可使用</span>
              <a :href="update.info.checksums_url" target="_blank" rel="noopener noreferrer">
                SHA256SUMS.txt
              </a>
              <span>校验文件完整性。</span>
            </div>

            <div class="upgrade-guide">
              <h3>升级指引</h3>
              <p class="upgrade-warning">
                升级前请备份数据库、<code>.env</code> 和当前程序。程序启动时会自动迁移数据库结构，请勿覆盖 <code>.env</code>。
              </p>
              <details open>
                <summary>原生二进制</summary>
                <ol>
                  <li>下载与当前系统和架构匹配的文件，并核对 SHA256。</li>
                  <li>停止正在运行的服务，保留同目录的 <code>.env</code>。</li>
                  <li>替换程序文件后重新启动服务，登录后台确认版本号。</li>
                </ol>
                <p class="guide-code">
                  Linux：<code>sha256sum -c SHA256SUMS.txt --ignore-missing</code><br />
                  Windows：<code>certutil -hashfile &lt;文件名&gt; SHA256</code>
                </p>
              </details>
              <details>
                <summary>Docker Compose</summary>
                <ol>
                  <li>先备份数据库卷和 <code>.env</code>，再拉取新的代码或镜像。</li>
                  <li>使用仓库源码构建时执行 <code>git pull &amp;&amp; docker compose up -d --build</code>。</li>
                  <li>使用已发布镜像时执行 <code>docker compose pull &amp;&amp; docker compose up -d</code>。</li>
                </ol>
                <p class="guide-code">不要把 Release 二进制复制进容器；容器重建后会被镜像内容覆盖。</p>
              </details>
              <details>
                <summary>宝塔 / 进程守护</summary>
                <ol>
                  <li>在宝塔进程守护管理器或 Docker 管理器中先停止对应服务并完成备份。</li>
                  <li>替换 Linux 二进制，或重新构建/拉取 Docker 镜像。</li>
                  <li>重新启动服务，并在「系统配置」中确认版本号。</li>
                </ol>
              </details>
            </div>
          </template>

          <div v-else class="up-to-date">
            <CheckmarkCircleOutline />
            <div>
              <strong>已经是最新版本</strong>
              <p>当前版本 v{{ update.info.current_version }} 与 GitHub 最新 Release 一致。</p>
            </div>
          </div>
        </template>

        <div v-else-if="!update.loading" class="update-empty">
          <p>尚未获取版本信息。</p>
          <n-button secondary @click="update.check(true)">立即检查</n-button>
        </div>
      </div>

      <div class="update-footer">
        <span class="repo-hint">
          <LogoGithub />
          仓库与 Release 均来自 GitHub
        </span>
        <div class="update-actions">
          <n-button quaternary @click="close">关闭</n-button>
          <n-button secondary :loading="update.loading" @click="update.check(true)">
            <template #icon><RefreshOutline /></template>
            重新检查
          </n-button>
          <a
            v-if="update.hasUpdate"
            class="update-primary"
            :href="releaseURL"
            target="_blank"
            rel="noopener noreferrer"
          >
            <LogoGithub />
            打开 GitHub Release
          </a>
        </div>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.update-modal {
  display: flex;
  width: min(860px, calc(100vw - 32px));
  max-height: min(88vh, 860px);
  flex-direction: column;
  overflow: hidden;
  border-radius: 20px;
  background: var(--card);
  box-shadow: 0 32px 90px -24px rgba(15, 23, 42, 0.42);
}

.update-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-soft);
}

.update-head-main {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 12px;
}

.update-icon {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: var(--surface-strong);
  color: var(--muted);
  flex: none;
}

.update-icon.ready {
  background: var(--brand-soft);
  color: var(--brand);
}

.update-icon :deep(svg) {
  width: 19px;
  height: 19px;
}

.update-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.13em;
}

.update-head h2 {
  overflow: hidden;
  margin: 5px 0 4px;
  color: var(--ink);
  font-size: 20px;
  font-weight: 660;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.update-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
}

.update-close {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--muted-light);
  font-size: 21px;
  line-height: 1;
  cursor: pointer;
  flex: none;
}

.update-body {
  overflow-y: auto;
  padding: 18px 24px 22px;
}

.auto-check-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  margin-bottom: 14px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-soft);
}

.auto-check-copy {
  min-width: 0;
}

.auto-check-copy strong {
  display: block;
  color: var(--ink);
  font-size: 11px;
  font-weight: 620;
}

.auto-check-copy p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.55;
}

.update-error {
  margin-bottom: 14px;
  padding: 10px 12px;
  border: 1px solid var(--red-border, rgba(220, 38, 38, 0.2));
  border-radius: 9px;
  background: var(--red-soft, rgba(220, 38, 38, 0.06));
  color: var(--red, #b91c1c);
  font-size: 11px;
  line-height: 1.6;
}

.update-loading {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 38px 0;
  color: var(--muted);
  font-size: 11.5px;
}

.loading-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--brand);
  box-shadow: 0 0 0 4px var(--brand-soft);
}

.version-strip {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface-soft);
}

.version-chip span {
  display: block;
  color: var(--muted-light);
  font-size: 9px;
}

.version-chip strong {
  display: block;
  margin-top: 3px;
  color: var(--text);
  font-family: var(--mono);
  font-size: 15px;
  font-weight: 650;
}

.version-chip.latest strong {
  color: var(--brand);
}

.version-arrow {
  color: var(--muted-light);
  font-size: 16px;
}

.release-date {
  color: var(--muted-light);
  font-size: 9.5px;
  white-space: nowrap;
}

.release-notes {
  max-height: 230px;
  overflow-y: auto;
  margin-top: 14px;
  padding: 14px 16px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--card);
  color: var(--text);
  font-size: 11.5px;
  line-height: 1.75;
}

.release-notes :deep(h1),
.release-notes :deep(h2),
.release-notes :deep(h3) {
  margin: 12px 0 6px;
  color: var(--ink);
  font-size: 13px;
}

.release-notes :deep(h1:first-child),
.release-notes :deep(h2:first-child),
.release-notes :deep(h3:first-child) {
  margin-top: 0;
}

.release-notes :deep(p),
.release-notes :deep(ul),
.release-notes :deep(ol) {
  margin: 6px 0;
}

.release-notes :deep(ul),
.release-notes :deep(ol) {
  padding-left: 19px;
}

.release-notes :deep(code) {
  padding: 1px 4px;
  border-radius: 4px;
  background: var(--surface-strong);
  font-family: var(--mono);
  font-size: 10px;
}

.release-notes :deep(pre) {
  overflow-x: auto;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--surface-soft);
}

.release-notes :deep(pre code) {
  padding: 0;
  background: transparent;
}

.release-notes :deep(a) {
  color: var(--brand);
}

.update-copy {
  margin: 14px 0 0;
  color: var(--muted);
  font-size: 11px;
}

.download-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
  margin-top: 14px;
}

.download-card {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 9px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card);
  color: var(--text);
  text-decoration: none;
  transition: border-color 0.15s ease, background 0.15s ease;
}

.download-card:hover {
  border-color: var(--brand-border);
  background: var(--brand-softer);
}

.download-card :deep(svg) {
  width: 16px;
  height: 16px;
  color: var(--brand);
  flex: none;
}

.download-card span {
  min-width: 0;
  flex: 1;
}

.download-card strong,
.download-card small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.download-card strong {
  font-size: 11px;
  font-weight: 620;
}

.download-card small {
  margin-top: 2px;
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 8.5px;
}

.download-card em {
  color: var(--muted-light);
  font-size: 8.5px;
  font-style: normal;
  flex: none;
}

.checksum-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  margin-top: 10px;
  color: var(--muted);
  font-size: 10px;
}

.checksum-line :deep(svg) {
  width: 13px;
  height: 13px;
  color: var(--green, #16a34a);
}

.checksum-line a {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9.5px;
}

.upgrade-guide {
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}

.upgrade-guide h3 {
  margin: 0 0 7px;
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 630;
}

.upgrade-warning {
  margin: 0 0 11px;
  padding: 9px 11px;
  border-radius: 8px;
  background: var(--amber-soft, rgba(245, 158, 11, 0.08));
  color: var(--muted);
  font-size: 10px;
  line-height: 1.65;
}

.upgrade-warning code,
.guide-code code {
  padding: 1px 4px;
  border-radius: 4px;
  background: var(--surface-strong);
  font-family: var(--mono);
  font-size: 9.5px;
}

.upgrade-guide details {
  border-top: 1px dashed var(--border);
  padding: 9px 0;
}

.upgrade-guide summary {
  color: var(--text);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
}

.upgrade-guide ol {
  margin: 8px 0 0;
  padding-left: 18px;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.8;
}

.guide-code {
  margin: 7px 0 0;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.8;
}

.up-to-date {
  display: flex;
  align-items: center;
  gap: 11px;
  margin-top: 15px;
  padding: 14px 15px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--green-soft, rgba(22, 163, 74, 0.07));
}

.up-to-date :deep(svg) {
  width: 22px;
  height: 22px;
  color: var(--green, #16a34a);
  flex: none;
}

.up-to-date strong {
  color: var(--ink);
  font-size: 11.5px;
}

.up-to-date p {
  margin: 3px 0 0;
  color: var(--muted);
  font-size: 10px;
}

.update-empty {
  display: flex;
  min-height: 120px;
  align-items: center;
  justify-content: center;
  gap: 14px;
  color: var(--muted);
  font-size: 11px;
}

.update-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 13px 24px 17px;
  border-top: 1px solid var(--border);
}

.repo-hint {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--muted-light);
  font-size: 9px;
}

.repo-hint :deep(svg) {
  width: 13px;
  height: 13px;
}

.update-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.update-primary {
  display: inline-flex;
  height: 34px;
  align-items: center;
  gap: 6px;
  padding: 0 14px;
  border-radius: 6px;
  background: var(--brand);
  color: #fff;
  font-size: 11px;
  font-weight: 550;
  text-decoration: none;
}

.update-primary :deep(svg) {
  width: 14px;
  height: 14px;
}

.update-primary:hover {
  background: var(--brand-strong);
}

@media (max-width: 720px) {
  .update-head {
    padding: 18px 18px 14px;
  }

  .update-head h2 {
    font-size: 17px;
    white-space: normal;
  }

  .update-body {
    padding: 16px 18px 18px;
  }

  .version-strip {
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  }

  .release-date {
    grid-column: 1 / -1;
  }

  .update-footer {
    align-items: flex-start;
    flex-direction: column;
    padding: 12px 18px 16px;
  }

  .repo-hint {
    display: none;
  }

  .update-actions {
    width: 100%;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}
</style>
