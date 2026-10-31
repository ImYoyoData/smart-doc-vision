import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { schemaApi, type SchemaStatus } from '@/api/schema'

export const useSchemaStore = defineStore('schema', () => {
  const status = ref<SchemaStatus | null>(null)
  const checked = ref(false)
  const loading = ref(false)
  const required = computed(() => status.value?.required === true)
  let pending: Promise<SchemaStatus | null> | null = null

  async function load(force = false): Promise<SchemaStatus | null> {
    if (checked.value && !force) return status.value
    if (pending) return pending
    loading.value = true
    pending = (async () => {
      try {
        status.value = await schemaApi.status()
      } catch {
        // Older backends do not expose the schema status endpoint; treat them
        // as compatible and keep the normal application flow working.
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

  function reset() {
    status.value = null
    checked.value = false
  }

  return { status, checked, loading, required, load, reset }
})
