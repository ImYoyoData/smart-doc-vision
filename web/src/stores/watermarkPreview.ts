import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { WatermarkSettings } from '@/api/types'

// Temporary watermark used for live preview while the settings panel is open.
// It is intentionally not persisted: saving writes to the backend, leaving the
// page or collapsing the panel restores the saved settings.
export const useWatermarkPreviewStore = defineStore('watermarkPreview', () => {
  const settings = ref<WatermarkSettings | null>(null)

  function update(value: WatermarkSettings) {
    settings.value = { ...value }
  }

  function clear() {
    settings.value = null
  }

  return { settings, update, clear }
})
