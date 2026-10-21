import { defineStore } from 'pinia'
import { ref } from 'vue'
import { reviewApi } from '@/api'
import type { ReviewSummary } from '@/api/types'

export const useReviewStore = defineStore('review', () => {
  const pending = ref(0)
  const summary = ref<ReviewSummary | null>(null)
  const loading = ref(false)

  async function refresh(projectId?: number | null) {
    loading.value = true
    try {
      summary.value = await reviewApi.summary(projectId)
      pending.value = summary.value.pending
    } catch {
      // The badge should never block navigation if the summary endpoint fails.
    } finally {
      loading.value = false
    }
  }

  return { pending, summary, loading, refresh }
})
