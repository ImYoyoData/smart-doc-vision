import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { authApi } from '@/api'
import { REFRESH_KEY, TOKEN_KEY, USER_KEY } from '@/api/http'
import type { LoginResult, User } from '@/api/types'

function readUser(): User | null {
  try {
    const raw = localStorage.getItem(USER_KEY)
    return raw ? (JSON.parse(raw) as User) : null
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem(TOKEN_KEY) || '')
  const refreshToken = ref(localStorage.getItem(REFRESH_KEY) || '')
  const user = ref<User | null>(readUser())
  const hydrated = ref(false)

  const isAuthenticated = computed(() => Boolean(token.value))
  const displayName = computed(() => user.value?.display_name || user.value?.username || '未登录')
  const permissions = computed(() => user.value?.permissions || [])
  const menus = computed(() => user.value?.menus || [])

  function canMenu(menu: string) {
    if (!menus.value.length) return true
    return menus.value.includes(menu)
  }

  function can(permission: string) {
    if (!permissions.value.length) return true
    return permissions.value.includes(permission)
  }

  const roleLabel = computed(() => {
    const map: Record<string, string> = {
      SUPER_ADMIN: '超级管理员',
      PROJECT_ADMIN: '项目管理员',
      AUDITOR: '审核配置员',
      REVIEWER: '复核员',
      VIEWER: '只读用户',
    }
    return user.value ? map[user.value.role] || user.value.role : ''
  })

  function hydrate() {
    if (hydrated.value) return
    token.value = localStorage.getItem(TOKEN_KEY) || ''
    refreshToken.value = localStorage.getItem(REFRESH_KEY) || ''
    user.value = readUser()
    hydrated.value = true
  }

  function applySession(data: LoginResult) {
    token.value = data.access_token
    refreshToken.value = data.refresh_token
    user.value = data.user
    localStorage.setItem(TOKEN_KEY, data.access_token)
    localStorage.setItem(REFRESH_KEY, data.refresh_token)
    localStorage.setItem(USER_KEY, JSON.stringify(data.user))
  }

  async function login(username: string, password: string) {
    const data = await authApi.login({ username, password })
    applySession(data)
    return data
  }

  async function loadMe() {
    if (!token.value) return
    const data = await authApi.me()
    user.value = data.user
    localStorage.setItem(USER_KEY, JSON.stringify(data.user))
  }

  function clear() {
    token.value = ''
    refreshToken.value = ''
    user.value = null
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(REFRESH_KEY)
    localStorage.removeItem(USER_KEY)
    localStorage.removeItem('sdv.project')
  }

  async function logout() {
    try {
      await authApi.logout()
    } catch {
      // local session is cleared regardless
    }
    clear()
  }

  return {
    token,
    refreshToken,
    user,
    isAuthenticated,
    displayName,
    roleLabel,
    permissions,
    menus,
    canMenu,
    can,
    login,
    loadMe,
    hydrate,
    clear,
    logout,
  }
})
