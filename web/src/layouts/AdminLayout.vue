<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NDropdown, useMessage } from 'naive-ui'
import dayjs from 'dayjs'
import {
  AddOutline,
  BarChartOutline,
  CheckmarkDoneOutline,
  ChevronDownOutline,
  CloudOutline,
  HelpCircleOutline,
  KeyOutline,
  LinkOutline,
  DocumentTextOutline,
  FolderOpenOutline,
  GridOutline,
  LayersOutline,
  LogOutOutline,
  MenuOutline,
  NotificationsOutline,
  PeopleOutline,
  PersonCircleOutline,
  ReceiptOutline,
  ScanOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
  SpeedometerOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import BrandMark from '@/components/BrandMark.vue'
import { useAuthStore } from '@/stores/auth'
import { useWorkspaceStore } from '@/stores/workspace'
import { metaApi } from '@/api'
import { useReviewStore } from '@/stores/review'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const auth = useAuthStore()
const workspace = useWorkspaceStore()
const review = useReviewStore()
const sidebarOpen = ref(false)
const version = ref('1.0.0')
const metaLoading = ref(false)

const sections = [
  {
    label: '工作台',
    items: [
      { name: 'dashboard', label: '仪表盘', icon: GridOutline },
      { name: 'analytics', label: '数据报表', icon: BarChartOutline },
    ],
  },
  {
    label: '审核中心',
    items: [
      { name: 'reviews', label: '待议复核', icon: CheckmarkDoneOutline },
      { name: 'tasks', label: '审核任务', icon: ScanOutline },
      { name: 'audits', label: '审核配置', icon: ShieldCheckmarkOutline },
    ],
  },
  {
    label: '资源配置',
    items: [
      { name: 'providers', label: '模型接入', icon: CloudOutline },
      { name: 'connections', label: '数据连接', icon: LinkOutline },
      { name: 'credentials', label: '接入凭证', icon: KeyOutline },
      { name: 'projects', label: '项目空间', icon: FolderOpenOutline },
    ],
  },
  {
    label: '安全治理',
    items: [
      { name: 'logs', label: '审计日志', icon: ReceiptOutline },
      { name: 'recycle', label: '回收站', icon: TrashOutline },
    ],
  },
  {
    label: '系统管理',
    items: [
      { name: 'users', label: '用户与权限', icon: PeopleOutline },
      { name: 'monitor', label: '运行监控', icon: SpeedometerOutline },
    ],
  },
]

const visibleSections = computed(() =>
  sections
    .map((section) => ({ ...section, items: section.items.filter((item) => auth.canMenu(item.name)) }))
    .filter((section) => section.items.length > 0),
)
const pageTitle = computed(() => String(route.meta.title || '仪表盘'))
const pageSection = computed(() => String(route.meta.section || '工作台'))
const today = dayjs().format('YYYY.MM.DD')
const avatarText = computed(() => (auth.displayName || 'U').slice(0, 1).toUpperCase())

const canCreateProject = computed(() => auth.user?.role === 'SUPER_ADMIN')
const projectOptions = computed(() => {
  const items = workspace.projects.map((p) => ({
    label: `${p.name}　${p.env.toUpperCase()}`,
    key: `project:${p.id}`,
  }))
  if (!canCreateProject.value) {
    return items.length ? items : [{ label: '暂无可访问项目', key: 'no-project', disabled: true }]
  }
  return [
    ...items,
    ...(items.length ? [{ type: 'divider' as const, key: 'd1' }] : []),
    { label: '＋ 新建项目', key: 'create-project' },
  ]
})

const userOptions = [
  { label: '个人中心', key: 'profile' },
  { label: '对接文档', key: 'help' },
  { type: 'divider' as const, key: 'user-divider' },
  { label: '退出登录', key: 'logout' },
]

function navigate(name: string) {
  router.push({ name })
  sidebarOpen.value = false
}

function handleProject(key: string) {
  if (key === 'create-project') {
    if (canCreateProject.value) router.push({ name: 'projects', query: { create: '1' } })
    return
  }
  if (key.startsWith('project:')) {
    const id = Number(key.split(':')[1])
    if (id && id !== workspace.currentProjectId) {
      workspace.setCurrentProject(id)
    }
  }
}

function handleUser(key: string) {
  if (key === 'profile') {
    router.push({ name: 'profile' })
    return
  }
  if (key === 'help') {
    router.push({ name: 'help' })
    return
  }
  if (key === 'logout') {
    auth.logout().finally(() => router.replace('/login'))
  }
}

async function bootstrap() {
  metaLoading.value = true
  try {
    await workspace.loadProjects(true)
    await auth.loadMe().catch(() => undefined)
    if (route.meta.menu && !auth.canMenu(String(route.meta.menu))) {
      await router.replace({ name: 'dashboard' })
    }
    await review.refresh(workspace.currentProjectId)
    const meta = await metaApi.system()
    version.value = meta.version
  } catch (error) {
    if (error instanceof Error) message.error(error.message)
  } finally {
    metaLoading.value = false
  }
}

let reviewTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  bootstrap()
  reviewTimer = setInterval(() => review.refresh(workspace.currentProjectId), 30000)
})
onBeforeUnmount(() => {
  if (reviewTimer) clearInterval(reviewTimer)
})
watch(() => workspace.currentProjectId, (id) => review.refresh(id))
</script>

<template>
  <div class="app-shell">
    <div v-if="sidebarOpen" class="sidebar-overlay" @click="sidebarOpen = false"></div>
    <aside class="sidebar" :class="{ open: sidebarOpen }">
      <div class="sidebar-brand" @click="navigate('dashboard')">
        <BrandMark :size="38" />
        <div class="brand-copy">
          <div class="brand-name">智审图档</div>
          <div class="brand-en">SMARTDOC VISION</div>
        </div>
      </div>

      <div class="project-switch-wrap">
        <n-dropdown trigger="click" :options="projectOptions" placement="bottom-start" @select="handleProject">
          <button class="project-switch" type="button" :title="workspace.currentProject?.name || '选择项目'">
            <span class="project-icon">
              <LayersOutline />
            </span>
            <span class="project-copy">
              <span class="project-label">
                当前项目
                <span v-if="workspace.currentProject" class="env-mini" :class="workspace.currentProject.env">
                  {{ workspace.currentProject.env.toUpperCase() }}
                </span>
              </span>
              <span class="project-name">{{ workspace.currentProject?.name || '选择项目' }}</span>
            </span>
            <ChevronDownOutline class="chev" />
          </button>
        </n-dropdown>
      </div>

      <nav class="nav">
        <div v-for="section in visibleSections" :key="section.label" class="nav-section">
          <div class="nav-section-title">{{ section.label }}</div>
          <button
            v-for="item in section.items"
            :key="item.name"
            type="button"
            class="nav-item"
            :class="{ active: route.name === item.name }"
            @click="navigate(item.name)"
          >
            <component :is="item.icon" class="nav-icon" />
            <span>{{ item.label }}</span>
            <span v-if="item.name === 'reviews' && review.pending > 0" class="nav-badge">{{ review.pending > 99 ? '99+' : review.pending }}</span>
          </button>
        </div>
      </nav>

      <div class="sidebar-footer">
        <div class="footer-line">
          <span class="live-dot"></span>
          <span>审核引擎运行中</span>
        </div>
        <div class="footer-meta">
          <span>SmartDoc Vision v{{ version }}</span>
          <span class="mono">Redis DB 3</span>
        </div>
      </div>
    </aside>

    <div class="main-shell">
      <header class="topbar">
        <div class="topbar-left">
          <button class="mobile-menu icon-btn" type="button" @click="sidebarOpen = true">
            <MenuOutline />
          </button>
          <div class="crumbs">
            <span>{{ pageSection }}</span>
            <span class="crumb-sep">/</span>
            <strong>{{ pageTitle }}</strong>
          </div>
        </div>
        <div class="topbar-right">
          <div class="date-chip">{{ today }}</div>
          <button v-if="auth.canMenu('reviews')" class="topbar-review" :class="{ active: route.name === 'reviews' }" type="button" @click="navigate('reviews')">
            <NotificationsOutline />
            <span>待复核</span>
            <b v-if="review.pending > 0">{{ review.pending }}</b>
          </button>
          <button v-if="auth.canMenu('help')" class="icon-btn topbar-help" type="button" title="对接文档" @click="navigate('help')">
            <HelpCircleOutline />
          </button>
          <div class="top-divider"></div>
          <n-dropdown trigger="click" :options="userOptions" placement="bottom-end" @select="handleUser">
            <button class="user-chip" type="button">
              <span class="avatar">{{ avatarText }}</span>
              <span class="user-copy">
                <span class="user-name">{{ auth.displayName }}</span>
                <span class="user-role">{{ auth.roleLabel }}</span>
              </span>
              <ChevronDownOutline class="user-chev" />
            </button>
          </n-dropdown>
        </div>
      </header>

      <main class="content">
        <router-view v-slot="{ Component }">
          <transition name="view-fade" mode="out-in">
            <component :is="Component" :key="route.name" />
          </transition>
        </router-view>
      </main>
    </div>
  </div>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
  background: var(--canvas);
}

.sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  z-index: 40;
  display: flex;
  width: 238px;
  flex-direction: column;
  border-right: 1px solid var(--border);
  background: #fff;
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 11px;
  height: 74px;
  padding: 0 20px;
  cursor: pointer;
}

.brand-copy {
  min-width: 0;
}

.brand-name {
  color: var(--ink);
  font-size: 17px;
  font-weight: 680;
  letter-spacing: 0.02em;
  line-height: 1.25;
}

.brand-en {
  color: #9aa7bb;
  font-size: 8.5px;
  font-weight: 600;
  letter-spacing: 0.16em;
}

.project-switch-wrap {
  padding: 4px 12px 14px;
}

.project-switch {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 9px;
  padding: 9px 10px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: #fbfcff;
  color: var(--text);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.16s ease, box-shadow 0.16s ease;
}

.project-switch:hover {
  border-color: #c7d6f7;
  box-shadow: 0 4px 14px -8px rgba(43, 108, 246, 0.35);
}

.project-icon {
  display: flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
  flex-shrink: 0;
}

.project-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

.project-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.project-label {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.35;
}

.env-mini {
  padding: 0 4px;
  border-radius: 3px;
  background: #f1f5f9;
  color: #7c8798;
  font-family: var(--mono);
  font-size: 7px;
  font-weight: 600;
  letter-spacing: 0.03em;
}

.env-mini.prod {
  background: var(--red-soft);
  color: #b91c1c;
}
.env-mini.staging {
  background: var(--amber-soft);
  color: #b45309;
}
.env-mini.test {
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.project-name {
  overflow: hidden;
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 550;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chev {
  width: 13px;
  height: 13px;
  color: var(--muted-light);
  flex-shrink: 0;
}

.nav {
  flex: 1;
  padding: 2px 12px 12px;
  overflow-y: auto;
}

.nav-section {
  margin-bottom: 18px;
}

.nav-section-title {
  padding: 0 11px 7px;
  color: #a3aec0;
  font-size: 9.5px;
  font-weight: 650;
  letter-spacing: 0.13em;
  text-transform: uppercase;
}

.nav-item {
  position: relative;
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  height: 38px;
  margin: 2px 0;
  padding: 0 11px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: #59677c;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  text-align: left;
  transition: all 0.16s ease;
}

.nav-item:hover {
  background: #f5f8fe;
  color: var(--ink);
}

.nav-item.active {
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-weight: 600;
}

.nav-item.active::before {
  content: '';
  position: absolute;
  left: -12px;
  width: 3px;
  height: 18px;
  border-radius: 0 3px 3px 0;
  background: var(--brand);
}

.nav-icon {
  width: 17px;
  height: 17px;
  opacity: 0.88;
}

.nav-dot {
  width: 5px;
  height: 5px;
  margin-left: auto;
  border-radius: 50%;
  background: var(--brand);
}

.nav-badge {
  min-width: 19px;
  height: 19px;
  margin-left: auto;
  padding: 0 5px;
  border-radius: 999px;
  background: #fff4df;
  color: #b45309;
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  line-height: 19px;
  text-align: center;
}

.topbar-review {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 10px;
  border: 1px solid #f3dfb8;
  border-radius: 9px;
  background: #fffbf2;
  color: #a16207;
  font-size: 11px;
  font-weight: 550;
  cursor: pointer;
  transition: all 0.16s ease;
}

.topbar-review:hover,
.topbar-review.active {
  border-color: #eccf94;
  background: #fff6e5;
}

.topbar-review :deep(svg) {
  width: 14px;
  height: 14px;
}

.topbar-review b {
  min-width: 17px;
  height: 17px;
  padding: 0 4px;
  border-radius: 99px;
  background: #f59e0b;
  color: #fff;
  font-family: var(--mono);
  font-size: 9px;
  line-height: 17px;
  text-align: center;
}

.topbar-help {
  width: 31px;
  height: 31px;
}

.topbar-help :deep(svg) {
  width: 16px;
  height: 16px;
}

.sidebar-footer {
  padding: 14px 20px 18px;
  border-top: 1px solid #f0f3f8;
}

.footer-line {
  display: flex;
  align-items: center;
  gap: 7px;
  color: #5f6e84;
  font-size: 11px;
}

.live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.12);
}

.footer-meta {
  display: flex;
  justify-content: space-between;
  margin-top: 7px;
  color: #a6b0c0;
  font-size: 9.5px;
}

.main-shell {
  display: flex;
  min-height: 100vh;
  flex-direction: column;
  padding-left: 238px;
}

.topbar {
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  height: 64px;
  align-items: center;
  justify-content: space-between;
  padding: 0 32px;
  border-bottom: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(12px);
}

.topbar-left,
.topbar-right {
  display: flex;
  align-items: center;
  gap: 14px;
}

.crumbs {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--muted-light);
  font-size: 12.5px;
}

.crumbs strong {
  color: var(--text);
  font-weight: 600;
}

.crumb-sep {
  color: #c2cad6;
}

.date-chip {
  color: var(--muted-light);
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.05em;
}

.top-divider {
  width: 1px;
  height: 22px;
  background: var(--border);
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 4px 6px 4px 4px;
  border: 1px solid transparent;
  border-radius: 12px;
  background: transparent;
  cursor: pointer;
  transition: background 0.16s ease;
}

.user-chip:hover {
  background: #f7f9fd;
}

.avatar {
  display: flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: linear-gradient(140deg, #dbe7ff, #b7cdfb);
  color: #1e4db5;
  font-size: 13px;
  font-weight: 650;
}

.user-copy {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.3;
}

.user-name {
  color: var(--ink);
  font-size: 12px;
  font-weight: 550;
}

.user-role {
  color: var(--muted-light);
  font-size: 9.5px;
}

.user-chev {
  width: 12px;
  height: 12px;
  color: var(--muted-light);
}

.content {
  flex: 1;
  min-width: 0;
}

.mobile-menu {
  display: none;
}

.sidebar-overlay {
  display: none;
}

.view-fade-enter-active,
.view-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.view-fade-enter-from {
  opacity: 0;
  transform: translateY(4px);
}

.view-fade-leave-to {
  opacity: 0;
}

@media (max-width: 960px) {
  .sidebar {
    transform: translateX(-100%);
    transition: transform 0.22s ease;
    box-shadow: 20px 0 60px rgba(15, 23, 42, 0.12);
  }

  .sidebar.open {
    transform: translateX(0);
  }

  .sidebar-overlay {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 35;
    background: rgba(15, 23, 42, 0.28);
    backdrop-filter: blur(2px);
  }

  .main-shell {
    padding-left: 0;
  }

  .mobile-menu {
    display: inline-flex;
  }

  .topbar {
    padding: 0 18px;
  }

  .date-chip,
  .top-divider,
  .topbar-review span {
    display: none;
  }

  .user-role {
    display: none;
  }
}

@media (max-width: 560px) {
  .user-copy {
    display: none;
  }

  .crumbs > span:first-child,
  .crumb-sep {
    display: none;
  }
}
</style>
