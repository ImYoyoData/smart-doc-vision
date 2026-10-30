import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { updateApi } from '@/api'
import type { UpdateCheck } from '@/api/types'
import { useAuthStore } from '@/stores/auth'

export const useUpdateStore = defineStore('update', () => {
  const auth = useAuthStore()
  const info = ref<UpdateCheck | null>(null)
  const loading = ref(false)
  const error = ref('')
  const checked = ref(false)
  const showModal = ref(false)
  const savingAutoCheck = ref(false)

  const hasUpdate = computed(() => Boolean(info.value?.has_update))
  const autoCheck = computed(() => Boolean(auth.user?.auto_update_check))

  async function check(force = false) {
    if (loading.value) return
    loading.value = true
    error.value = ''
    try {
      info.value = await updateApi.check(force)
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : '检查更新失败'
    } finally {
      loading.value = false
      checked.value = true
    }
  }

  async function openAndCheck(force = false) {
    showModal.value = true
    if (!info.value || force) await check(force)
  }

  async function setAutoCheck(enabled: boolean) {
    if (savingAutoCheck.value) return
    savingAutoCheck.value = true
    error.value = ''
    try {
      const result = await updateApi.setAutoCheck(enabled)
      auth.patchUser({ auto_update_check: result.enabled })
      if (result.enabled) await check(true)
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : '保存自动检查设置失败'
    } finally {
      savingAutoCheck.value = false
    }
  }

  function open() {
    showModal.value = true
  }

  function close() {
    showModal.value = false
  }

  return {
    info,
    loading,
    error,
    checked,
    showModal,
    savingAutoCheck,
    hasUpdate,
    autoCheck,
    check,
    openAndCheck,
    setAutoCheck,
    open,
    close,
  }
})
