import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { public: true, title: '登录' },
  },
  {
    path: '/',
    component: () => import('@/layouts/AdminLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('@/views/DashboardView.vue'),
        meta: { title: '仪表盘', section: '工作台', menu: 'dashboard' },
      },
      {
        path: 'tasks',
        name: 'tasks',
        component: () => import('@/views/TasksView.vue'),
        meta: { title: '审核任务', section: '审核中心', menu: 'tasks' },
      },
      {
        path: 'reviews',
        name: 'reviews',
        component: () => import('@/views/ReviewView.vue'),
        meta: { title: '待议复核', section: '审核中心', menu: 'reviews' },
      },
      {
        path: 'audits',
        name: 'audits',
        component: () => import('@/views/AuditsView.vue'),
        meta: { title: '审核配置', section: '审核中心', menu: 'audits' },
      },
      {
        path: 'analytics',
        name: 'analytics',
        component: () => import('@/views/AnalyticsView.vue'),
        meta: { title: '数据报表', section: '工作台', menu: 'analytics' },
      },
      {
        path: 'users',
        name: 'users',
        component: () => import('@/views/UsersView.vue'),
        meta: { title: '用户与权限', section: '系统管理', menu: 'users' },
      },
      {
        path: 'profile',
        name: 'profile',
        component: () => import('@/views/ProfileView.vue'),
        meta: { title: '个人中心', section: '账号', menu: 'profile' },
      },
      {
        path: 'help',
        name: 'help',
        component: () => import('@/views/HelpView.vue'),
        meta: { title: '对接文档', section: '帮助中心', menu: 'help' },
      },
      {
        path: 'projects',
        name: 'projects',
        component: () => import('@/views/ProjectsView.vue'),
        meta: { title: '项目空间', section: '资源配置', menu: 'projects' },
      },
      {
        path: 'providers',
        name: 'providers',
        component: () => import('@/views/ProvidersView.vue'),
        meta: { title: '模型接入', section: '资源配置', menu: 'providers' },
      },
      {
        path: 'credentials',
        name: 'credentials',
        component: () => import('@/views/CredentialsView.vue'),
        meta: { title: '接入凭证', section: '资源配置', menu: 'credentials' },
      },
      {
        path: 'logs',
        name: 'logs',
        component: () => import('@/views/LogsView.vue'),
        meta: { title: '审计日志', section: '安全治理', menu: 'logs' },
      },
      {
        path: 'recycle',
        name: 'recycle',
        component: () => import('@/views/RecycleBinView.vue'),
        meta: { title: '回收站', section: '安全治理', menu: 'recycle' },
      },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  auth.hydrate()
  if (!to.meta.public && !auth.isAuthenticated) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { path: '/dashboard' }
  }
  if (to.meta.menu && auth.user?.menus?.length && !auth.user.menus.includes(String(to.meta.menu))) {
    return { path: '/dashboard' }
  }
  document.title = `${String(to.meta.title || '')} · 智审图档`
  return true
})

export default router
