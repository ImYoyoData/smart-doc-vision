import { defineStore } from 'pinia'
import { ref } from 'vue'
import { installApi, type InstallStatus } from '@/api/install'

export const useInstallStore = defineStore('install', () => {
  const status = ref<InstallStatus | null>(null)
  const checked = ref(false)
  const loading = ref(false)
  let pending: Promise<InstallStatus | null> | null = null

  async function load(force = false): Promise<InstallStatus | null> {
    if (checked.value && !force) return status.value
    if (pending) return pending
    loading.value = true
    pending = (async () => {
      try {
        status.value = await installApi.status()
      } catch {
        // An older backend may not expose the installer API; treat it as an
        // installed instance so the normal login flow keeps working.
        status.value = null
      } finally {
        checked.value = true
        loading.value = false
        pending = null
      }
      return status.value
    })()
    return pending
  }

  function isInstallerState() {
    return status.value?.state === 'needs_install' || status.value?.state === 'config_missing'
  }

  return { status, checked, loading, load, isInstallerState }
})
