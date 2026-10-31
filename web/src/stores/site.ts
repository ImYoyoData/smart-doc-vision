import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

type SiteResponse = {
  name?: string
  name_en?: string
  logo?: string
}

const DEFAULT_NAME = '智审图档'
const DEFAULT_NAME_EN = 'SmartDoc Vision'

function applyFavicon(logo: string) {
  if (!logo || typeof document === 'undefined') return
  let link = document.querySelector<HTMLLinkElement>('link[rel~="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = logo
}

export const useSiteStore = defineStore('site', () => {
  const name = ref(DEFAULT_NAME)
  const nameEn = ref(DEFAULT_NAME_EN)
  const logo = ref('')
  const checked = ref(false)
  let pending: Promise<void> | null = null

  const displayName = computed(() => name.value || DEFAULT_NAME)

  async function load(force = false) {
    if (force) {
      checked.value = false
      pending = null
    }
    if (checked.value) return
    if (pending) return pending
    pending = (async () => {
      try {
        const response = await fetch('/api/site', {
          headers: { Accept: 'application/json' },
          cache: 'no-store',
        })
        if (response.ok) {
          const body = (await response.json()) as { data?: SiteResponse }
          const data = body.data
          if (data?.name?.trim()) name.value = data.name.trim()
          if (data?.name_en?.trim()) nameEn.value = data.name_en.trim()
          logo.value = (data?.logo || '').trim()
          applyFavicon(logo.value)
        }
      } catch {
        // Branding is optional; defaults keep the UI usable if the probe fails.
      } finally {
        checked.value = true
        pending = null
      }
    })()
    return pending
  }

  return { name, nameEn, logo, checked, displayName, load }
})
