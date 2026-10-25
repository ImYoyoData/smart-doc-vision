import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export const THEME_KEY = 'sdv.theme'
export type ThemeMode = 'light' | 'dark'

function readMode(): ThemeMode {
  if (typeof window === 'undefined') return 'light'
  try {
    return window.localStorage.getItem(THEME_KEY) === 'dark' ? 'dark' : 'light'
  } catch {
    return 'light'
  }
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(readMode())
  const isDark = computed(() => mode.value === 'dark')
  let transitionTimer: number | undefined

  function apply(next: ThemeMode, animate: boolean) {
    if (typeof document === 'undefined') return
    if (animate) {
      document.documentElement.classList.add('theme-transition')
      void document.documentElement.offsetHeight
      window.clearTimeout(transitionTimer)
      transitionTimer = window.setTimeout(() => {
        document.documentElement.classList.remove('theme-transition')
      }, 360)
    }
    mode.value = next
    document.documentElement.dataset.theme = next
    document.documentElement.style.colorScheme = next
    try {
      window.localStorage.setItem(THEME_KEY, next)
    } catch {
      // Theme still applies for this session when storage is unavailable.
    }
  }

  function init() {
    apply(mode.value, false)
  }

  function toggle() {
    apply(mode.value === 'dark' ? 'light' : 'dark', true)
  }

  return { mode, isDark, init, toggle }
})
